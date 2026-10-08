package api

import (
	"net/http"
	"strings"
)

// handleSearch implements GET /api/v1/search?q=&label=key=val&project=p&type=instances
//
// Returns a unified list of matching resources across instances, VPCs, subnets, images,
// DNS zones, LBs, and stacks. Supports:
//   - q: substring match on name/ID
//   - label: key=value label filter (repeatable)
//   - project: restrict to a single project
//   - type: comma-separated resource types to include
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(r.URL.Query().Get("q"))
	project := r.URL.Query().Get("project")
	labelParam := r.URL.Query().Get("label") // "key=value"
	typeFilter := r.URL.Query().Get("type")  // "instances,vpcs,subnets"

	wantType := map[string]bool{}
	if typeFilter != "" {
		for _, t := range strings.Split(typeFilter, ",") {
			wantType[strings.TrimSpace(t)] = true
		}
	}

	var labelKey, labelVal string
	if labelParam != "" {
		parts := strings.SplitN(labelParam, "=", 2)
		labelKey = parts[0]
		if len(parts) == 2 {
			labelVal = parts[1]
		}
	}

	type result struct {
		Type    string `json:"type"`
		ID      string `json:"id"`
		Name    string `json:"name"`
		Project string `json:"project"`
	}

	var results []result

	include := func(t string) bool {
		return len(wantType) == 0 || wantType[t]
	}
	matchName := func(name string) bool {
		if q == "" {
			return true
		}
		return strings.Contains(strings.ToLower(name), q)
	}
	matchLabel := func(labels map[string]string) bool {
		if labelKey == "" {
			return true
		}
		v, ok := labels[labelKey]
		if !ok {
			return false
		}
		return labelVal == "" || v == labelVal
	}
	matchProject := func(p string) bool {
		return project == "" || p == project
	}

	// Instances
	if include("instances") {
		insts, err := s.ctrl.Store.ListInstances()
		if err == nil {
			for _, inst := range insts {
				if !matchProject(inst.Labels["project"]) {
					continue
				}
				if !matchName(inst.Name) && !matchName(inst.ID) {
					continue
				}
				if !matchLabel(inst.Labels) {
					continue
				}
				results = append(results, result{Type: "instance", ID: inst.ID, Name: inst.Name, Project: inst.Labels["project"]})
			}
		}
	}

	// VPCs and subnets (flat networks are removed)
	if include("vpcs") || include("subnets") || include("networks") {
		vpcProject := project
		if vpcProject == "" {
			vpcProject = s.project
		}
		vpcs, err := s.ctrl.Store.VPC.ListVPCs(vpcProject)
		if err == nil {
			for _, v := range vpcs {
				if include("vpcs") || include("networks") {
					if matchName(v.Name) || matchName(v.ID) {
						results = append(results, result{Type: "vpc", ID: v.ID, Name: v.Name, Project: v.Project})
					}
				}
				if !include("subnets") && !include("networks") {
					continue
				}
				subs, serr := s.ctrl.Store.VPC.ListSubnets(v.ID)
				if serr != nil {
					continue
				}
				for _, sub := range subs {
					if !matchName(sub.Name) && !matchName(sub.ID) {
						continue
					}
					results = append(results, result{Type: "subnet", ID: sub.ID, Name: sub.Name, Project: v.Project})
				}
			}
		}
	}

	// Images
	if include("images") {
		imgs, err := s.ctrl.Store.ListImages()
		if err == nil {
			for _, img := range imgs {
				if !matchName(img.Name) && !matchName(img.ID) {
					continue
				}
				results = append(results, result{Type: "image", ID: img.ID, Name: img.Name})
			}
		}
	}

	// DNS zones
	if include("dns") {
		zones, err := s.ctrl.Store.DNS.ListZones(project)
		if err == nil {
			for _, z := range zones {
				if !matchName(z.Name) && !matchName(z.ID) {
					continue
				}
				results = append(results, result{Type: "dns-zone", ID: z.ID, Name: z.Name})
			}
		}
	}

	// Load balancers
	if include("lb") {
		lbs, err := s.ctrl.Store.LB.Store().List(project)
		if err == nil {
			for _, lb := range lbs {
				if !matchName(lb.Name) && !matchName(lb.ID) {
					continue
				}
				results = append(results, result{Type: "lb", ID: lb.ID, Name: lb.Name, Project: lb.Project})
			}
		}
	}

	// Stacks
	if include("stacks") {
		stacks, err := s.ctrl.Store.Stack.List(project)
		if err == nil {
			for _, st := range stacks {
				if !matchName(st.Name) && !matchName(st.ID) {
					continue
				}
				results = append(results, result{Type: "stack", ID: st.ID, Name: st.Name, Project: st.Project})
			}
		}
	}

	if results == nil {
		results = []result{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results, "count": len(results)})
}
