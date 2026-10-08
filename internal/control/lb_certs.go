package control

import (
	"context"
	"net/http"

	"capper/internal/certmgr"
	"capper/internal/store"
)

// WireLBCertificates connects certmgr to the in-process load balancer proxies:
// TLS material resolution, http-01 ACME challenges on HTTP LBs, and renewal.
func WireLBCertificates(st *store.Store, cm *certmgr.CertManager) {
	if st == nil || st.LB == nil || cm == nil {
		return
	}
	st.LB.SetCertResolver(func(ref string) ([]byte, []byte, error) {
		return cm.TLSMaterials(ref)
	})
	st.LB.SetACMEChallengeHandler(func(w http.ResponseWriter, r *http.Request) {
		cm.HTTPSolver.ServeChallenge(w, r)
	})
}

// StartCertRenewal runs the certmgr renewal scheduler until ctx is cancelled.
func StartCertRenewal(ctx context.Context, cm *certmgr.CertManager) {
	if cm == nil {
		return
	}
	sched := certmgr.NewRenewalScheduler(cm, cm.Config())
	go sched.Start(ctx)
}
