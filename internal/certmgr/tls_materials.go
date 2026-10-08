package certmgr

import "fmt"

// TLSMaterials returns the active PEM certificate chain and private key for a
// certificate referenced by ID or name. Used by load balancer TLS termination.
func (m *CertManager) TLSMaterials(certRef string) (certPEM, keyPEM []byte, err error) {
	if certRef == "" {
		return nil, nil, fmt.Errorf("empty certificate reference")
	}
	cert, err := m.store.GetCertificate(certRef)
	if err != nil {
		return nil, nil, err
	}
	if cert.ActiveVersionID == "" {
		return nil, nil, fmt.Errorf("certificate %q has no active version", certRef)
	}
	ver, err := m.store.GetCertVersion(cert.ActiveVersionID)
	if err != nil {
		return nil, nil, err
	}
	pem := ver.FullChainPEM
	if pem == "" {
		pem = ver.CertPEM + ver.ChainPEM
	}
	if pem == "" {
		return nil, nil, fmt.Errorf("certificate %q has no PEM data", certRef)
	}
	if ver.PrivateKeyRef == "" {
		return nil, nil, fmt.Errorf("certificate %q has no private key", certRef)
	}
	encrypted, err := m.store.LoadPrivateKey(ver.PrivateKeyRef)
	if err != nil {
		return nil, nil, err
	}
	key := decrypt(m.secretKey, encrypted)
	if len(key) == 0 {
		return nil, nil, fmt.Errorf("certificate %q private key decrypt failed", certRef)
	}
	return []byte(pem), key, nil
}
