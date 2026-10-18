// deployer is an opt-in local executor; the API never gets the Docker socket.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"prisme.local/platform/internal/config"
	"prisme.local/platform/internal/deployment"
	"prisme.local/platform/internal/store"
	"runtime"
)

func main() {
	path := flag.String("config", "", "explicit deployment configuration file")
	manifestPath := flag.String("manifest", "", "validate a standalone project manifest without registering it")
	validate := flag.Bool("validate", false, "strictly validate manifests without accessing Docker or the database")
	flag.Parse()
	if *manifestPath != "" {
		var m deployment.Manifest
		b, err := os.ReadFile(*manifestPath)
		if err != nil || deployment.Decode(b, &m) != nil || m.Validate() != nil {
			log.Fatal("project manifest invalid")
		}
		log.Print("project manifest v1 valid")
		return
	}
	cfg, err := deployment.Load(*path)
	if err != nil {
		log.Fatal("deployment configuration invalid")
	}
	if *validate {
		log.Print("deployment manifest v1 valid")
		return
	}
	if runtime.GOOS != "linux" {
		log.Fatal("execution requires the local Linux Docker host; manifest validation is cross-platform")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	dsn, err := config.DatabaseURL()
	if err != nil {
		log.Fatal("database configuration unavailable")
	}
	db, err := store.Open(ctx, dsn)
	if err != nil {
		log.Fatal("database unavailable")
	}
	defer db.Close()
	if err = store.Ready(ctx, db); err != nil {
		log.Fatal("schema incompatible or recovery quarantined")
	}
	engine := &deployment.Engine{DB: db, Config: cfg, Driver: &deployment.Docker{Root: cfg.Root}}
	if err = engine.Initialize(ctx); err != nil {
		log.Fatal("deployment initialization failed")
	}
	log.Print("deployment worker ready for explicitly approved targets")
	engine.Run(ctx)
}
