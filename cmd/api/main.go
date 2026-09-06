package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"prisme.local/platform/internal/api"
	"prisme.local/platform/internal/config"
	"prisme.local/platform/internal/pki"
	"prisme.local/platform/internal/store"
)

func run() error {
	migrate := flag.Bool("migrate", false, "apply versioned development database migrations and exit")
	flag.Parse()
	if os.Getenv("PRISME_MODE") != "development" {
		return errors.New("phase 0 requires explicit PRISME_MODE=development; production auth is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	dsn, err := config.DatabaseURL()
	if err != nil {
		return errors.New("database configuration unavailable")
	}
	db, err := store.Open(ctx, dsn)
	if err != nil {
		return errors.New("database connection failed")
	}
	defer db.Close()
	if *migrate {
		if err = store.Migrate(ctx, db); err != nil {
			return errors.New("database migration failed")
		}
		log.Print("schema version 1 ready")
		return nil
	}
	var version int
	if err = db.QueryRowContext(ctx, "SELECT max(version) FROM schema_migrations").Scan(&version); err != nil || version != 1 {
		return errors.New("run api -migrate before starting")
	}
	secret, err := config.Secret(os.Getenv("PRISME_DEV_SECRET_FILE"))
	if err != nil {
		return errors.New("development secret unavailable")
	}
	ca, err := pki.Load(os.Getenv("PRISME_CA_FILE"), os.Getenv("PRISME_CA_KEY_FILE"))
	if err != nil {
		return errors.New("development CA unavailable")
	}
	cert, err := tls.LoadX509KeyPair(os.Getenv("PRISME_TLS_CERT_FILE"), os.Getenv("PRISME_TLS_KEY_FILE"))
	if err != nil {
		return errors.New("TLS identity unavailable")
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca.Certificate)
	address := os.Getenv("PRISME_LISTEN")
	if address == "" {
		address = "127.0.0.1:8443"
	}
	app := &api.Server{DB: db, CA: ca, Bootstrap: secret, Now: func() time.Time { return time.Now().UTC() }}
	server := &http.Server{Addr: address, Handler: app.Handler(), TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, ClientCAs: roots, ClientAuth: tls.VerifyClientCertIfGiven}, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	stop, done := signal.NotifyContext(context.Background(), os.Interrupt)
	defer done()
	go func() {
		<-stop.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
	log.Print("Prisme development API listening with TLS; production mode unavailable")
	err = server.ListenAndServeTLS("", "")
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return errors.New("API listener failed")
}
func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
