package certmgr

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
	"time"
)

type CertManager struct {
	store      *Store
	config     *CertManagerConfig
	HTTPSolver *HTTPSolver
	dnsSolvers map[string]DNSSolver
	secretKey  []byte
}

func NewCertManager(store *Store, config *CertManagerConfig, secretKey []byte) *CertManager {
	return &CertManager{
		store:  store,
		config: config,
		HTTPSolver: NewHTTPSolver(),
		dnsSolvers: map[string]DNSSolver{
			"manual": &ManualDNSSolver{},
		},
		secretKey: secretKey,
	}
}

// GetStore returns the underlying store (used by API handlers).
func (m *CertManager) GetStore() *Store {
	return m.store
}

// Config returns the active certificate manager configuration.
func (m *CertManager) Config() *CertManagerConfig {
	return m.config
}

func (m *CertManager) RequestCertificate(ctx context.Context, req CertRequest) (*Certificate, error) {
	// Check rate limit
	domainRoot := extractDomainRoot(req.CommonName)
	issuedCount, windowStart, backoffUntil, err := m.store.GetRateLimit(domainRoot)
	if err == nil {
		if backoffUntil != "" {
			t, _ := time.Parse(time.RFC3339, backoffUntil)
			if time.Now().Before(t) {
				return nil, fmt.Errorf("ACME rate limit: in backoff until %s", backoffUntil)
			}
		}
		if windowStart != "" {
			t, _ := time.Parse(time.RFC3339, windowStart)
			if time.Since(t) < 7*24*time.Hour && issuedCount >= 50 {
				return nil, fmt.Errorf("ACME rate limit: 50 certificates per 7 days exceeded")
			}
		}
	}

	// Check for reusable cert
	if existing := m.findReusableCert(req.CommonName, req.SANs); existing != nil {
		return existing, nil
	}

	issuer := req.Issuer
	if issuer == "" {
		issuer = m.config.DefaultIssuer
	}

	// Block production ACME if not allowed
	if issuer == IssuerLetsEncrypt && !m.config.ProductionAllowed {
		return nil, fmt.Errorf("production ACME not allowed in this environment; set productionAllowed: true in cert-manager config")
	}

	cert := Certificate{
		Project:          req.Project,
		AccountID:        req.AccountID,
		Name:             req.Name,
		CommonName:       req.CommonName,
		SANs:             req.SANs,
		Issuer:           issuer,
		Status:           CertStatusPending,
		ValidationMethod: req.ValidationMethod,
		AutoRenew:        req.AutoRenew,
	}
	if cert.ValidationMethod == "" {
		cert.ValidationMethod = ValidationHTTP01
	}

	created, err := m.store.CreateCertificate(cert)
	if err != nil {
		return nil, fmt.Errorf("creating cert record: %w", err)
	}

	// Start ACME order in background only for ACME issuers. Non-ACME issuers
	// (internal CA, self-signed, or any non-letsencrypt issuer) must not spawn
	// a doomed ACME order — doing so wastes a goroutine and, on SQLite, the
	// failing background write contends with foreground requests.
	if issuer == IssuerLetsEncrypt || issuer == IssuerLetsEncryptStaging {
		acmeAccountName := req.ACMEAccountName
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			if err := m.fulfillACME(bgCtx, &created, acmeAccountName); err != nil {
				_ = m.store.UpdateCertificate(created.ID, map[string]any{
					"status":         CertStatusFailed,
					"failure_reason": err.Error(),
				})
				_ = m.store.SetBackoff(domainRoot, time.Now().Add(1*time.Hour).UTC().Format(time.RFC3339), err.Error())
			}
		}()
	}

	return &created, nil
}

func (m *CertManager) fulfillACME(ctx context.Context, cert *Certificate, acmeAccountName string) error {
	_ = m.store.UpdateCertificate(cert.ID, map[string]any{"status": CertStatusValidating})

	acmeAcc, err := m.store.GetACMEAccount(acmeAccountName)
	if err != nil {
		// Use first available account
		accs, _ := m.store.ListACMEAccounts()
		if len(accs) == 0 {
			return fmt.Errorf("no ACME account configured; create one with POST /api/v1/certificates/acme/accounts")
		}
		acmeAcc = accs[0]
	}

	acmeClient, err := NewACMEClientWrapper(ctx, acmeAcc, m.store, m.secretKey)
	if err != nil {
		return fmt.Errorf("creating ACME client: %w", err)
	}

	domains := []string{cert.CommonName}
	for _, san := range cert.SANs {
		if san != cert.CommonName {
			domains = append(domains, san)
		}
	}

	var solver interface {
		Present(domain, token, keyAuth string) error
		CleanUp(domain, token, keyAuth string) error
	}

	if cert.ValidationMethod == ValidationDNS01 {
		provider := m.config.DNS01.DefaultProvider
		if provider == "" {
			provider = "manual"
		}
		dnsSolver, ok := m.dnsSolvers[provider]
		if !ok {
			dnsSolver = m.dnsSolvers["manual"]
		}
		solver = &dnsSolverWrapper{dnsSolver}
	} else {
		solver = m.HTTPSolver
	}

	result, err := acmeClient.OrderCertificate(ctx, domains, solver)
	if err != nil {
		return err
	}

	// Store private key encrypted
	keyRef := "certkey_" + cert.ID
	encryptedKey := encrypt(m.secretKey, []byte(result.KeyPEM))
	if err := m.store.StorePrivateKey(keyRef, encryptedKey); err != nil {
		return fmt.Errorf("storing private key: %w", err)
	}

	// Create version record
	version, err := m.store.CreateCertVersion(CertificateVersion{
		CertificateID:     cert.ID,
		CertPEM:           result.CertPEM,
		ChainPEM:          result.ChainPEM,
		FullChainPEM:      result.FullChainPEM,
		PrivateKeyRef:     keyRef,
		FingerprintSHA256: result.Fingerprint,
		SerialNumber:      result.SerialNumber,
		NotBefore:         result.NotBefore,
		NotAfter:          result.NotAfter,
	})
	if err != nil {
		return fmt.Errorf("storing cert version: %w", err)
	}

	// Compute renew_after: notAfter - renewBefore (default 30 days)
	renewBefore := m.config.Renewal.RenewBefore
	if renewBefore == 0 {
		renewBefore = 30 * 24 * time.Hour
	}
	notAfter, _ := time.Parse(time.RFC3339, result.NotAfter)
	renewAfter := notAfter.Add(-renewBefore).UTC().Format(time.RFC3339)

	return m.store.UpdateCertificate(cert.ID, map[string]any{
		"status":            CertStatusIssued,
		"active_version_id": version.ID,
		"not_before":        result.NotBefore,
		"not_after":         result.NotAfter,
		"renew_after":       renewAfter,
		"failure_reason":    "",
	})
}

func (m *CertManager) RenewCertificate(ctx context.Context, certID string) error {
	cert, err := m.store.GetCertificate(certID)
	if err != nil {
		return err
	}
	_ = m.store.UpdateCertificate(certID, map[string]any{"status": CertStatusRenewing})

	req := CertRequest{
		Project:          cert.Project,
		AccountID:        cert.AccountID,
		Name:             cert.Name,
		CommonName:       cert.CommonName,
		SANs:             cert.SANs,
		Issuer:           cert.Issuer,
		ValidationMethod: cert.ValidationMethod,
		AutoRenew:        cert.AutoRenew,
	}
	_, err = m.RequestCertificate(ctx, req)
	return err
}

func (m *CertManager) ReissueCertificate(ctx context.Context, certID string) error {
	_ = m.store.UpdateCertificate(certID, map[string]any{"status": CertStatusPending, "failure_reason": ""})
	return m.RenewCertificate(ctx, certID)
}

func (m *CertManager) RevokeCertificate(ctx context.Context, certID, reason string) error {
	cert, err := m.store.GetCertificate(certID)
	if err != nil {
		return err
	}
	if cert.ActiveVersionID != "" {
		ver, err := m.store.GetCertVersion(cert.ActiveVersionID)
		if err == nil {
			accs, _ := m.store.ListACMEAccounts()
			if len(accs) > 0 {
				acmeClient, err := NewACMEClientWrapper(ctx, accs[0], m.store, m.secretKey)
				if err == nil {
					block, _ := pem.Decode([]byte(ver.CertPEM))
					if block != nil {
						_ = acmeClient.RevokeCertificate(ctx, block.Bytes)
					}
				}
			}
		}
	}
	return m.store.UpdateCertificate(certID, map[string]any{
		"status": CertStatusRevoked,
	})
}

func (m *CertManager) ImportCertificate(ctx context.Context, req ImportCertRequest) (*Certificate, error) {
	block, _ := pem.Decode([]byte(req.CertPEM))
	if block == nil {
		return nil, fmt.Errorf("invalid certificate PEM")
	}
	x509Cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing certificate: %w", err)
	}

	var sans []string
	for _, n := range x509Cert.DNSNames {
		sans = append(sans, n)
	}

	cert := Certificate{
		Project:    req.Project,
		AccountID:  req.AccountID,
		Name:       req.Name,
		CommonName: x509Cert.Subject.CommonName,
		SANs:       sans,
		Issuer:     IssuerImported,
		Status:     CertStatusImported,
		AutoRenew:  false,
		NotBefore:  x509Cert.NotBefore.UTC().Format(time.RFC3339),
		NotAfter:   x509Cert.NotAfter.UTC().Format(time.RFC3339),
	}

	created, err := m.store.CreateCertificate(cert)
	if err != nil {
		return nil, err
	}

	keyRef := "certkey_" + created.ID
	if req.KeyPEM != "" {
		encryptedKey := encrypt(m.secretKey, []byte(req.KeyPEM))
		_ = m.store.StorePrivateKey(keyRef, encryptedKey)
	}

	version, err := m.store.CreateCertVersion(CertificateVersion{
		CertificateID:     created.ID,
		CertPEM:           req.CertPEM,
		ChainPEM:          req.ChainPEM,
		FullChainPEM:      req.CertPEM + req.ChainPEM,
		PrivateKeyRef:     keyRef,
		FingerprintSHA256: fingerprintSHA256(block.Bytes),
		SerialNumber:      x509Cert.SerialNumber.String(),
		NotBefore:         created.NotBefore,
		NotAfter:          created.NotAfter,
	})
	if err != nil {
		return &created, nil
	}

	_ = m.store.UpdateCertificate(created.ID, map[string]any{
		"active_version_id": version.ID,
	})
	created.ActiveVersionID = version.ID
	return &created, nil
}

func (m *CertManager) AttachToLoadBalancer(ctx context.Context, certID, lbID, hostname string) error {
	_, err := m.store.CreateBinding(CertificateBinding{
		CertificateID: certID,
		TargetType:    "lb",
		TargetID:      lbID,
		Hostname:      hostname,
		Status:        "active",
	})
	if err != nil {
		return err
	}
	return m.store.UpdateCertificate(certID, map[string]any{"status": CertStatusAttached})
}

func (m *CertManager) DetachFromLoadBalancer(ctx context.Context, certID, lbID, hostname string) error {
	bindings, err := m.store.ListBindings(certID)
	if err != nil {
		return err
	}
	for _, b := range bindings {
		if b.TargetID == lbID && (hostname == "" || b.Hostname == hostname) {
			_ = m.store.DeleteBinding(b.ID)
		}
	}
	return nil
}

func (m *CertManager) findReusableCert(commonName string, sans []string) *Certificate {
	certs, err := m.store.ListCertificates("", "", "")
	if err != nil {
		return nil
	}
	for _, c := range certs {
		if c.Status != CertStatusIssued && c.Status != CertStatusAttached && c.Status != CertStatusRenewing {
			continue
		}
		if c.NotAfter != "" {
			t, _ := time.Parse(time.RFC3339, c.NotAfter)
			if time.Now().After(t.Add(-30 * 24 * time.Hour)) {
				continue
			}
		}
		if certCoversAll(c, commonName, sans) {
			return &c
		}
	}
	return nil
}

func certCoversAll(cert Certificate, commonName string, sans []string) bool {
	covered := func(name string) bool {
		if cert.CommonName == name {
			return true
		}
		for _, san := range cert.SANs {
			if san == name {
				return true
			}
			if strings.HasPrefix(san, "*.") {
				parts := strings.SplitN(san, ".", 2)
				if len(parts) == 2 {
					suffix := "." + parts[1]
					if strings.HasSuffix(name, suffix) && !strings.Contains(strings.TrimSuffix(name, suffix), ".") {
						return true
					}
				}
			}
		}
		return false
	}
	if !covered(commonName) {
		return false
	}
	for _, san := range sans {
		if !covered(san) {
			return false
		}
	}
	return true
}

func extractDomainRoot(domain string) string {
	parts := strings.Split(domain, ".")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "." + parts[len(parts)-1]
	}
	return domain
}

// dnsSolverWrapper adapts DNSSolver (with context) to the solver interface used by OrderCertificate.
type dnsSolverWrapper struct {
	inner DNSSolver
}

func (d *dnsSolverWrapper) Present(domain, token, keyAuth string) error {
	return d.inner.Present(context.Background(), domain, token, keyAuth)
}

func (d *dnsSolverWrapper) CleanUp(domain, token, keyAuth string) error {
	return d.inner.CleanUp(context.Background(), domain, token, keyAuth)
}
