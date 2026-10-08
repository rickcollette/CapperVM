package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	capapi "capper/internal/api"
	"capper/internal/control"
	"capper/internal/controller"
	"capper/internal/network"
)

type apiOptions struct {
	*options
	listen         string
	withDaemon     bool
	staticRoot     string
	tlsCert        string
	tlsKey         string
	allowedOrigins []string
}

func apiCmd(opts *options) *cobra.Command {
	aopts := &apiOptions{options: opts, listen: "127.0.0.1:8686"}
	cmd := &cobra.Command{
		Use:   "api",
		Short: "Capper REST API and web console",
	}
	start := &cobra.Command{
		Use:   "start",
		Short: "start the Capper API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := network.CheckCapabilities(); err != nil {
				return err
			}
			return withController(opts, func(ctrl controller.Controller) error {
				var daemon *control.Daemon
				ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
				defer cancel()
				if aopts.withDaemon {
					dopts := control.DefaultDaemonOptions()
					daemon = control.NewDaemon(ctrl.Store, ctrl.Instances, dopts)
					control.WireLBCertificates(ctrl.Store, ctrl.CertMgr)
					control.StartCertRenewal(ctx, ctrl.CertMgr)
					go daemon.Run(ctx)
				}
				// Bootstrap admin: ensure a configured email is an active admin
				// so a fresh system has a first administrator (no self-registration).
				if email := strings.TrimSpace(os.Getenv("CAPPER_BOOTSTRAP_ADMIN")); email != "" && ctrl.Store.IAM != nil {
					if _, err := ctrl.Store.IAM.EnsureAdminUser(email, "google"); err != nil {
						fmt.Printf("warning: bootstrap admin %q: %v\n", email, err)
					} else {
						fmt.Printf("bootstrap admin ensured: %s\n", email)
					}
				}

				var allowedDomains []string
				if v := strings.TrimSpace(os.Getenv("CAPPER_ALLOWED_DOMAINS")); v != "" {
					for _, d := range strings.Split(v, ",") {
						if d = strings.TrimSpace(d); d != "" {
							allowedDomains = append(allowedDomains, d)
						}
					}
				}
				srv := capapi.NewServer(ctrl, capapi.Options{
					Project:             opts.project,
					StaticRoot:          aopts.staticRoot,
					Daemon:              daemon,
					AllowedOrigins:      aopts.allowedOrigins,
					ProxySecret:         os.Getenv("CAPPER_PROXY_SECRET"),
					AllowedEmailDomains: allowedDomains,
				})
				useTLS := aopts.tlsCert != "" && aopts.tlsKey != ""
				scheme := "http"
				if useTLS {
					scheme = "https"
				}
				fmt.Printf("Capper API listening on %s://%s\n", scheme, aopts.listen)
				if aopts.staticRoot != "" {
					fmt.Printf("Serving console static assets from %s\n", aopts.staticRoot)
				}
				errCh := make(chan error, 1)
				go func() {
					if useTLS {
						errCh <- srv.ListenAndServeTLS(aopts.listen, aopts.tlsCert, aopts.tlsKey)
					} else {
						errCh <- srv.ListenAndServe(aopts.listen)
					}
				}()
				select {
				case <-ctx.Done():
					fmt.Println("API server stopped.")
					return nil
				case err := <-errCh:
					return err
				}
			})
		},
	}
	start.Flags().StringVar(&aopts.listen, "listen", aopts.listen, "listen address")
	start.Flags().BoolVar(&aopts.withDaemon, "with-daemon", false, "also run control plane daemon (supervisor)")
	start.Flags().StringVar(&aopts.staticRoot, "console", "", "path to CapperWeb dist/ to serve as console")
	start.Flags().StringVar(&aopts.tlsCert, "tls-cert", "", "TLS certificate file (enables HTTPS; requires --tls-key)")
	start.Flags().StringVar(&aopts.tlsKey, "tls-key", "", "TLS private key file (enables HTTPS; requires --tls-cert)")
	start.Flags().StringSliceVar(&aopts.allowedOrigins, "allowed-origin", nil, "CORS allowlist origin permitted credentialed cross-origin access (repeatable; loopback always allowed)")
	cmd.AddCommand(start)
	return cmd
}
