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
	"prisme.local/platform/internal/automation"
	"prisme.local/platform/internal/backup"
	"prisme.local/platform/internal/config"
	"prisme.local/platform/internal/deployment"
	"prisme.local/platform/internal/edge"
	"prisme.local/platform/internal/github"
	"prisme.local/platform/internal/notifications"
	"prisme.local/platform/internal/observability"
	"prisme.local/platform/internal/operations"
	"prisme.local/platform/internal/pki"
	"prisme.local/platform/internal/store"
)

func run() error {
	migrate := flag.Bool("migrate", false, "apply versioned development database migrations and exit")
	flag.Parse()
	mode := os.Getenv("PRISME_MODE")
	if mode != "development" && mode != "identity" {
		return errors.New("explicit PRISME_MODE=development or identity required")
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
		log.Print("schema version ", store.Version, " ready")
		return nil
	}
	if err = store.Ready(ctx, db); err != nil {
		return err
	}
	secretPath := os.Getenv("PRISME_DEV_SECRET_FILE")
	if mode == "identity" {
		secretPath = os.Getenv("PRISME_IDENTITY_SECRET_FILE")
	}
	secret, err := config.Secret(secretPath)
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
	app := &api.Server{DB: db, CA: ca, Bootstrap: secret, Mode: mode, Now: func() time.Time { return time.Now().UTC() }}
	if origin := os.Getenv("PRISME_AUTH_ORIGIN"); origin != "" {
		app.WebAuthn, err = api.NewWebAuthn(origin)
		if err != nil {
			return err
		}
		app.IdentityBootstrap = secret
	} else if mode == "identity" {
		return errors.New("identity mode requires PRISME_AUTH_ORIGIN")
	}
	server := &http.Server{Addr: address, Handler: app.Handler(), TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, ClientCAs: roots, ClientAuth: tls.VerifyClientCertIfGiven}, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	stop, done := signal.NotifyContext(context.Background(), os.Interrupt)
	defer done()
	if path := os.Getenv("PRISME_OPERATIONS_CONFIG"); path != "" {
		cfg, err := operations.Load(path)
		if err != nil {
			return errors.New("operations configuration invalid")
		}
		if err = app.ConfigureOperations(stop, cfg); err != nil {
			return errors.New("operations initialization failed")
		}
	}
	if path := os.Getenv("PRISME_NOTIFICATION_CONFIG"); path != "" {
		cfg, err := notifications.Load(path)
		if err != nil {
			return err
		}
		sender, err := notifications.NewFCM(cfg.CredentialsFile)
		if err != nil {
			return err
		}
		app.Notifications = &notifications.Engine{DB: db, Config: cfg, Sender: sender, Now: app.Now}
		go app.Notifications.Run(stop)
	}
	if path := os.Getenv("PRISME_EDGE_CONFIG"); path != "" {
		cfg, err := edge.Load(path)
		if err != nil {
			return errors.New("edge configuration invalid")
		}
		if err = app.ConfigureEdge(stop, cfg); err != nil {
			return errors.New("edge initialization failed")
		}
	}
	if path := os.Getenv("PRISME_AUTOMATION_CONFIG"); path != "" {
		cfg, err := automation.Load(path)
		if err != nil {
			return errors.New("automation configuration invalid")
		}
		if err = app.ConfigureAutomation(stop, cfg); err != nil {
			return errors.New("automation initialization failed")
		}
	}
	if path := os.Getenv("PRISME_BACKUP_CONFIG"); path != "" {
		cfg, err := backup.Load(path)
		if err != nil {
			return errors.New("backup configuration invalid")
		}
		if err = app.ConfigureBackups(stop, cfg); err != nil {
			return errors.New("backup initialization failed")
		}
	}
	if path := os.Getenv("PRISME_DEPLOYMENT_CONFIG"); path != "" {
		cfg, err := deployment.Load(path)
		if err != nil {
			return err
		}
		if err = app.ConfigureDeployments(stop, cfg); err != nil {
			return err
		}
	}
	if path := os.Getenv("PRISME_GITHUB_CONFIG"); path != "" {
		cfg, key, webhookSecret, err := github.Load(path)
		if err != nil {
			return err
		}
		app.GitHub = &github.Service{DB: db, Config: cfg, Provider: github.NewClient(cfg, key), WebhookSecret: webhookSecret}
		if err = app.GitHub.Initialize(stop); err != nil {
			return errors.New("GitHub repository initialization failed")
		}
		go app.GitHub.Run(stop)
	}
	if path := os.Getenv("PRISME_OBSERVABILITY_CONFIG"); path != "" {
		cfg, err := observability.Load(path)
		if err != nil {
			return err
		}
		source, err := observability.NewSource(os.Getenv("PRISME_METRICS_URL"), os.Getenv("PRISME_LOGS_URL"), mode == "development")
		if err != nil {
			return err
		}
		app.Observability = &observability.Engine{DB: db, Config: cfg, Source: source, Now: app.Now}
		if err = app.Observability.ValidateStoredScopes(stop); err != nil {
			return err
		}
		go app.Observability.Run(stop)
	}
	go func() {
		<-stop.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
	log.Print("Prisme API listening with TLS (", mode, ")")
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
