// recovery is a standalone native service: no PostgreSQL, Docker or Caddy.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"prisme.local/platform/internal/api"
	"prisme.local/platform/internal/operations"
	"prisme.local/platform/internal/recovery"
)

func main() {
	configPath := flag.String("config", "", "operations config; vault must be disabled")
	root := flag.String("identity-root", "", "independent private registry and audit directory")
	origin := flag.String("origin", "", "independent HTTPS origin")
	listen := flag.String("listen", "127.0.0.1:9443", "native TLS listener")
	cert := flag.String("cert", "", "TLS certificate file")
	key := flag.String("key", "", "TLS key file")
	device := flag.String("enroll-device", "", "prepare this device ID through host SSH")
	name := flag.String("name", "Prisme recovery", "device name")
	output := flag.String("code-file", "", "new private file for one-use enrollment code")
	recoverCode := flag.Bool("recovery-code", false, "generate a single-use recovery code instead of enrollment")
	revoke := flag.String("revoke-device", "", "revoke this device immediately")
	publicKey := flag.Bool("public-key", false, "print only the audit verification public key")
	flag.Parse()
	if *device != "" {
		if recovery.Provision(*root, *device, *name, *output, *recoverCode) != nil {
			log.Fatal("recovery provisioning failed")
		}
		log.Print("one-use code written to private file")
		return
	}
	webauthn, e := api.NewWebAuthn(*origin)
	if e != nil {
		log.Fatal("independent HTTPS origin required")
	}
	identity, e := recovery.New(*root, webauthn)
	if e != nil {
		log.Fatal("recovery identity unavailable")
	}
	if *publicKey {
		log.Print(identity.PublicKey())
		return
	}
	if *revoke != "" {
		if identity.Revoke(*revoke) != nil {
			log.Fatal("revocation failed")
		}
		return
	}
	cfg, e := operations.Load(*configPath)
	if e != nil || cfg.MasterKeyFile != "" {
		log.Fatal("recovery configuration invalid; central vault must be absent")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	svc := operations.New(ctx, cfg, identity.OperationsIdentity(), "recovery")
	svc.Revalidate = identity.Revalidate
	svc.Audit = identity.OperationsAudit(cfg)
	mux := http.NewServeMux()
	identity.Routes(mux)
	mux.Handle("/v1/operations/", svc.Handler())
	api.RegisterAuthAssets(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.URL.Path == "/healthz" || r.Header.Get("Upgrade") == "websocket" {
			mux.ServeHTTP(w, r)
			return
		}
		c, done := context.WithTimeout(r.Context(), 30*time.Second)
		defer done()
		mux.ServeHTTP(w, r.WithContext(c))
	})
	server := &http.Server{Addr: *listen, Handler: handler, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13}, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	go func() {
		<-ctx.Done()
		c, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		_ = server.Shutdown(c)
	}()
	log.Print("independent recovery service ready")
	if e = server.ListenAndServeTLS(*cert, *key); e != nil && e != http.ErrServerClosed {
		log.Fatal("recovery listener failed")
	}
}
