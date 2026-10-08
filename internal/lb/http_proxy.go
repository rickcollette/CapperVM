package lb

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// HTTPProxy is a reverse-proxy load balancer for HTTP/HTTPS listeners.
type HTTPProxy struct {
	spec         ProxySpec
	store        *Store
	certResolver CertResolver
	acmeHandler  ACMEChallengeHandler
	logPath      string
	srv          *http.Server
	cursor       uint64
	cancel       context.CancelFunc

	TotalRequests atomic.Uint64
	ActiveConns   atomic.Int64
}

func newHTTPProxy(spec ProxySpec, store *Store, certRes CertResolver, acme ACMEChallengeHandler, logPath string) *HTTPProxy {
	return &HTTPProxy{spec: spec, store: store, certResolver: certRes, acmeHandler: acme, logPath: logPath}
}

func (p *HTTPProxy) Start(ctx context.Context) error {
	ctx, p.cancel = context.WithCancel(ctx)

	var logFile *os.File
	if p.logPath != "" {
		f, err := os.OpenFile(p.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			logFile = f
			defer f.Close()
		}
	}

	rp := &httputil.ReverseProxy{
		Rewrite: p.rewrite,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{Timeout: dialTimeout}).DialContext,
		},
		ModifyResponse: func(resp *http.Response) error {
			if logFile != nil {
				fmt.Fprintf(logFile, "%s %s %s %d\n",
					time.Now().UTC().Format(time.RFC3339),
					resp.Request.Method,
					resp.Request.URL.String(),
					resp.StatusCode,
				)
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if logFile != nil {
				fmt.Fprintf(logFile, "%s %s %s error: %v\n",
					time.Now().UTC().Format(time.RFC3339),
					r.Method, r.URL.String(), err)
			}
			http.Error(w, fmt.Sprintf("lb: no healthy backend: %v", err), http.StatusBadGateway)
		},
	}

	var rootHandler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.TotalRequests.Add(1)
		p.ActiveConns.Add(1)
		defer p.ActiveConns.Add(-1)
		rp.ServeHTTP(w, r)
	})
	if p.acmeHandler != nil {
		rootHandler = wrapACMEHandler(rootHandler, p.acmeHandler)
	}

	p.srv = &http.Server{
		Addr:        p.spec.ListenAddr,
		Handler:     rootHandler,
		ReadTimeout: 30 * time.Second,
	}

	go func() {
		<-ctx.Done()
		_ = p.srv.Close()
	}()

	useTLS := p.spec.TLSCertName != "" || p.spec.Mode == ModeHTTPS
	if useTLS && p.certResolver != nil {
		cert, key, err := p.certResolver(p.spec.TLSCertName)
		if err == nil {
			tlsCert, err := tls.X509KeyPair(cert, key)
			if err == nil {
				p.srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{tlsCert}}
				if err := p.srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
					return err
				}
				return nil
			}
		}
		if p.spec.Mode == ModeHTTPS {
			return fmt.Errorf("lb %q: https mode requires a valid TLS certificate", p.spec.LB.Name)
		}
	}

	if p.spec.Mode == ModeHTTPS {
		return fmt.Errorf("lb %q: https mode requires tlsCertName and cert resolver", p.spec.LB.Name)
	}

	if err := p.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func wrapACMEHandler(next http.Handler, acme ACMEChallengeHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/.well-known/acme-challenge/") {
			acme(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (p *HTTPProxy) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
}

func (p *HTTPProxy) rewrite(pr *httputil.ProxyRequest) {
	var addrs []string
	var err error
	if p.spec.TargetGroupID != "" {
		addrs, err = p.store.ListTargetAddresses(p.spec.TargetGroupID)
	} else {
		var backends []Backend
		backends, err = p.store.ListBackends(p.spec.LB.ID)
		for _, b := range backends {
			addrs = append(addrs, b.Address)
		}
	}
	if err != nil || len(addrs) == 0 {
		return
	}
	var addr string
	if p.spec.LB.Algorithm == AlgoLeastConnections {
		addr = addrs[0]
	} else {
		idx := atomic.AddUint64(&p.cursor, 1) - 1
		addr = addrs[int(idx)%len(addrs)]
	}
	target, err := url.Parse("http://" + addr)
	if err != nil {
		return
	}
	pr.SetURL(target)
	pr.SetXForwarded()
}
