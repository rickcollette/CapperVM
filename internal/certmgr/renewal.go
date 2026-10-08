package certmgr

import (
	"context"
	"log"
	"crypto/rand"
	"math/big"
	"time"
)

type RenewalScheduler struct {
	manager *CertManager
	config  *CertManagerConfig
}

func NewRenewalScheduler(m *CertManager, cfg *CertManagerConfig) *RenewalScheduler {
	return &RenewalScheduler{manager: m, config: cfg}
}

func (s *RenewalScheduler) Start(ctx context.Context) {
	interval := s.config.Renewal.CheckInterval
	if interval == 0 {
		interval = 6 * time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runRenewalSweep(ctx)
			s.runExpiryCheck(ctx)
		}
	}
}

func (s *RenewalScheduler) runRenewalSweep(ctx context.Context) {
	certs, err := s.manager.store.ListCertsDueForRenewal()
	if err != nil {
		log.Printf("certmgr renewal: listing certs: %v", err)
		return
	}
	jitterMax := s.config.Renewal.Jitter
	if jitterMax == 0 {
		jitterMax = 30 * time.Minute
	}
	for _, cert := range certs {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(jitterMax)))
		jitter := time.Duration(n.Int64())
		select {
		case <-ctx.Done():
			return
		case <-time.After(jitter):
		}
		log.Printf("certmgr: renewing cert %s (%s)", cert.ID, cert.CommonName)
		if err := s.manager.RenewCertificate(ctx, cert.ID); err != nil {
			log.Printf("certmgr: renewal failed for %s: %v", cert.ID, err)
			_ = s.manager.store.UpdateCertificate(cert.ID, map[string]any{
				"failure_reason": err.Error(),
			})
		}
	}
}

func (s *RenewalScheduler) runExpiryCheck(ctx context.Context) {
	certs, err := s.manager.store.ListExpiredCerts()
	if err != nil {
		return
	}
	for _, cert := range certs {
		_ = s.manager.store.UpdateCertificate(cert.ID, map[string]any{"status": CertStatusExpired})
		log.Printf("certmgr: cert %s (%s) marked expired", cert.ID, cert.CommonName)
	}
}
