package dns

import (
	"context"
	"fmt"
	"crypto/rand"
	"math/big"
	"net"
	"strings"
	"sync/atomic"
	"time"

	mdns "github.com/miekg/dns"
)

// LabelFunc resolves a selector (networkID, key, value) to a list of IP strings.
// Used for service record IP resolution without importing the network/resource packages.
type LabelFunc func(networkID, key, value string) []string

// HealthFilter is called for each candidate IP before it is returned in a
// health-aware or drain-aware service record response. Return false to exclude
// the IP (e.g. because its node/zone is unhealthy).
type HealthFilter func(ip string) bool

// Resolver implements mdns.Handler, answering queries from the Capper store.
type Resolver struct {
	store        *Store
	labelFunc    LabelFunc
	healthFilter HealthFilter // optional; applied for health-aware routing policies
	Upstreams    []string     // fallback upstream servers (ip:port)
	// NetworkID restricts resolution to zones belonging to this network only.
	// Empty string means all zones are considered (global resolver).
	NetworkID string

	// query counters — read via Stats()
	QueryTotal    atomic.Uint64
	QueryHit      atomic.Uint64 // answered locally
	QueryForward  atomic.Uint64 // forwarded to upstream
	QueryNXDomain atomic.Uint64 // NXDOMAIN returned
}

// SetHealthFilter installs a health-aware IP filter used by health-aware and
// drain-aware routing policies. Pass nil to disable filtering.
func (r *Resolver) SetHealthFilter(f HealthFilter) { r.healthFilter = f }

// NewResolver creates a Resolver. labelFunc may be nil (service records with
// label selectors will resolve to empty and be skipped).
func NewResolver(s *Store, labelFunc LabelFunc, upstreams []string) *Resolver {
	return &Resolver{store: s, labelFunc: labelFunc, Upstreams: upstreams}
}

// NewNetworkResolver creates a Resolver scoped to a single network, providing
// per-network DNS isolation and split-horizon support (same zone name on
// multiple networks resolves differently per querying network).
func NewNetworkResolver(s *Store, networkID string, labelFunc LabelFunc, upstreams []string) *Resolver {
	return &Resolver{store: s, labelFunc: labelFunc, Upstreams: upstreams, NetworkID: networkID}
}

// ServeDNS implements mdns.Handler.
func (r *Resolver) ServeDNS(w mdns.ResponseWriter, req *mdns.Msg) {
	r.QueryTotal.Add(1)

	m := new(mdns.Msg)
	m.SetReply(req)
	m.Authoritative = true
	m.RecursionAvailable = len(r.Upstreams) > 0

	if len(req.Question) == 0 {
		m.SetRcode(req, mdns.RcodeFormatError)
		w.WriteMsg(m)
		return
	}

	q := req.Question[0]
	qname := strings.ToLower(q.Name)
	qtype := mdns.TypeToString[q.Qtype]

	answers, auth := r.resolve(qname, qtype)
	if len(answers) > 0 {
		r.QueryHit.Add(1)
		m.Answer = answers
		m.Authoritative = auth
		w.WriteMsg(m)
		return
	}

	// Forward if no local answer and upstreams are configured.
	if len(r.Upstreams) > 0 {
		if forwarded := r.forward(req); forwarded != nil {
			r.QueryForward.Add(1)
			forwarded.Id = req.Id
			w.WriteMsg(forwarded)
			return
		}
	}

	r.QueryNXDomain.Add(1)
	m.SetRcode(req, mdns.RcodeNameError) // NXDOMAIN
	w.WriteMsg(m)
}

// ResolverStats is a point-in-time snapshot of DNS query counters.
type ResolverStats struct {
	Total    uint64
	Hit      uint64
	Forward  uint64
	NXDomain uint64
}

// Stats returns a snapshot of the query counters.
func (r *Resolver) Stats() ResolverStats {
	return ResolverStats{
		Total:    r.QueryTotal.Load(),
		Hit:      r.QueryHit.Load(),
		Forward:  r.QueryForward.Load(),
		NXDomain: r.QueryNXDomain.Load(),
	}
}

// resolve looks up name+type in the store and returns miekg RRs + whether
// this resolver is authoritative for the name.
func (r *Resolver) resolve(qname, qtype string) ([]mdns.RR, bool) {
	zones, err := r.store.FindZonesForName(qname)
	if err != nil || len(zones) == 0 {
		return nil, false
	}
	// Per-network isolation: when NetworkID is set, prefer zones belonging to
	// this network (split horizon). Fall back to global zones only if no
	// network-specific match exists.
	if r.NetworkID != "" {
		var networkZones []Zone
		for _, z := range zones {
			if z.NetworkID == r.NetworkID {
				networkZones = append(networkZones, z)
			}
		}
		if len(networkZones) > 0 {
			zones = networkZones
		}
	}
	zone := zones[0] // best (longest suffix) match

	// 1. Manual/static records.
	recs, _ := r.store.LookupRecords(zone.ID, qname, qtype)
	if len(recs) > 0 {
		return r.recordsToRR(recs), true
	}

	// 2. Service records.
	svc, found, _ := r.store.LookupService(zone.ID, qname)
	if found && (qtype == RecordTypeA || qtype == "*" || qtype == "") {
		ips := r.resolveService(svc)
		if len(ips) > 0 {
			var rrs []mdns.RR
			for _, ip := range ips {
				rrs = append(rrs, &mdns.A{
					Hdr: mdns.RR_Header{
						Name:   mdns.Fqdn(qname),
						Rrtype: mdns.TypeA,
						Class:  mdns.ClassINET,
						Ttl:    uint32(svc.TTL),
					},
					A: net.ParseIP(ip).To4(),
				})
			}
			return rrs, true
		}
	}

	// 3. System records: gateway.zone → first IP in zone's network gateway.
	if rr := r.resolveSystemRecord(zone, qname, qtype); rr != nil {
		return []mdns.RR{rr}, true
	}

	return nil, true // authoritative NXDOMAIN for this zone
}

// resolveService resolves a ServiceRecord's selector to IPs, applying the
// health filter when the routing policy requires it.
func (r *Resolver) resolveService(svc ServiceRecord) []string {
	if svc.SelectorType != SelectorTypeLabel || r.labelFunc == nil {
		return nil
	}
	ips := r.labelFunc(svc.NetworkID, svc.SelectorKey, svc.SelectorValue)
	if r.healthFilter == nil {
		return ips
	}
	switch svc.RoutingPolicy {
	case RoutingHealthAware, RoutingDrainAware, RoutingZoneLocal, RoutingRegionLocal:
		filtered := ips[:0]
		for _, ip := range ips {
			if r.healthFilter(ip) {
				filtered = append(filtered, ip)
			}
		}
		// Fall back to all IPs if every backend is filtered out (avoid total NXDOMAIN).
		if len(filtered) > 0 {
			return filtered
		}
	}
	return ips
}

// resolveSystemRecord handles auto-generated zone records (gateway, dns).
func (r *Resolver) resolveSystemRecord(zone Zone, qname, qtype string) mdns.RR {
	if zone.NetworkID == "" {
		return nil
	}
	if qtype != RecordTypeA && qtype != "" && qtype != "*" {
		return nil
	}
	// Look for a "gateway" or "dns" system record in the zone.
	label := strings.TrimSuffix(strings.ToLower(qname), "."+zone.Name+".")
	label = strings.TrimSuffix(label, ".")
	if label != "gateway" && label != "dns" {
		return nil
	}
	// Find a system record seeded into the store.
	recs, _ := r.store.LookupRecords(zone.ID, qname, RecordTypeA)
	if len(recs) > 0 && len(recs[0].Values) > 0 {
		return &mdns.A{
			Hdr: mdns.RR_Header{
				Name:   mdns.Fqdn(qname),
				Rrtype: mdns.TypeA,
				Class:  mdns.ClassINET,
				Ttl:    uint32(zone.DefaultTTL),
			},
			A: net.ParseIP(recs[0].Values[0]).To4(),
		}
	}
	return nil
}

// weightedSelect picks one record from a slice using weight-proportional
// random selection. Records with Weight == 0 are treated as weight 1.
func weightedSelect(recs []Record) Record {
	total := 0
	for _, rec := range recs {
		w := rec.Weight
		if w <= 0 {
			w = 1
		}
		total += w
	}
	nBig, _ := rand.Int(rand.Reader, big.NewInt(int64(total))); pick := int(nBig.Int64())
	cumulative := 0
	for _, rec := range recs {
		w := rec.Weight
		if w <= 0 {
			w = 1
		}
		cumulative += w
		if pick < cumulative {
			return rec
		}
	}
	return recs[0]
}

// recordsToRR converts Capper Records to miekg RR values.
// When multiple A records share the same FQDN and all have non-zero weights,
// weighted routing selects one record per response.
func (r *Resolver) recordsToRR(recs []Record) []mdns.RR {
	// Group A records by FQDN to apply weighted routing.
	aByFQDN := make(map[string][]Record)
	var nonA []Record
	for _, rec := range recs {
		if strings.ToUpper(rec.Type) == RecordTypeA && rec.Weight > 0 {
			aByFQDN[rec.FQDN] = append(aByFQDN[rec.FQDN], rec)
		} else {
			nonA = append(nonA, rec)
		}
	}
	// Replace weighted groups with a single selected record.
	var effective []Record
	for _, group := range aByFQDN {
		effective = append(effective, weightedSelect(group))
	}
	effective = append(effective, nonA...)

	var out []mdns.RR
	for _, rec := range effective {
		ttl := uint32(rec.TTL)
		fqdn := mdns.Fqdn(rec.FQDN)
		switch strings.ToUpper(rec.Type) {
		case RecordTypeA:
			for _, v := range rec.Values {
				ip := net.ParseIP(v).To4()
				if ip == nil {
					continue
				}
				out = append(out, &mdns.A{
					Hdr: mdns.RR_Header{Name: fqdn, Rrtype: mdns.TypeA, Class: mdns.ClassINET, Ttl: ttl},
					A:   ip,
				})
			}
		case RecordTypeAAAA:
			for _, v := range rec.Values {
				ip := net.ParseIP(v)
				if ip == nil {
					continue
				}
				out = append(out, &mdns.AAAA{
					Hdr:  mdns.RR_Header{Name: fqdn, Rrtype: mdns.TypeAAAA, Class: mdns.ClassINET, Ttl: ttl},
					AAAA: ip,
				})
			}
		case RecordTypeCNAME:
			if len(rec.Values) > 0 {
				out = append(out, &mdns.CNAME{
					Hdr:    mdns.RR_Header{Name: fqdn, Rrtype: mdns.TypeCNAME, Class: mdns.ClassINET, Ttl: ttl},
					Target: mdns.Fqdn(rec.Values[0]),
				})
			}
		case RecordTypeTXT:
			out = append(out, &mdns.TXT{
				Hdr: mdns.RR_Header{Name: fqdn, Rrtype: mdns.TypeTXT, Class: mdns.ClassINET, Ttl: ttl},
				Txt: rec.Values,
			})
		case RecordTypeMX:
			for _, v := range rec.Values {
				out = append(out, &mdns.MX{
					Hdr:        mdns.RR_Header{Name: fqdn, Rrtype: mdns.TypeMX, Class: mdns.ClassINET, Ttl: ttl},
					Preference: uint16(rec.Priority),
					Mx:         mdns.Fqdn(v),
				})
			}
		}
	}
	return out
}

// forward sends the request to an upstream resolver and returns the response.
func (r *Resolver) forward(req *mdns.Msg) *mdns.Msg {
	c := &mdns.Client{Timeout: 3 * time.Second}
	for _, upstream := range r.Upstreams {
		if !strings.Contains(upstream, ":") {
			upstream = upstream + ":53"
		}
		resp, _, err := c.ExchangeContext(context.Background(), req, upstream)
		if err == nil {
			return resp
		}
	}
	return nil
}

// Query performs a direct in-process DNS lookup (no network, no daemon).
// Useful for `capper dns query` without a running daemon.
func (r *Resolver) Query(qname, qtype string) ([]mdns.RR, error) {
	qname = strings.ToLower(strings.TrimSuffix(qname, ".")) + "."
	rrs, _ := r.resolve(qname, strings.ToUpper(qtype))
	if len(rrs) == 0 && len(r.Upstreams) > 0 {
		req := new(mdns.Msg)
		qTypeCode, ok := mdns.StringToType[strings.ToUpper(qtype)]
		if !ok {
			qTypeCode = mdns.TypeA
		}
		req.SetQuestion(qname, qTypeCode)
		resp := r.forward(req)
		if resp != nil {
			return resp.Answer, nil
		}
	}
	return rrs, nil
}

// FormatRRs formats DNS resource records for human display.
func FormatRRs(rrs []mdns.RR) string {
	if len(rrs) == 0 {
		return "(no records)"
	}
	var sb strings.Builder
	for _, rr := range rrs {
		sb.WriteString(fmt.Sprintf("%s\n", rr))
	}
	return strings.TrimRight(sb.String(), "\n")
}
