// automation is an opt-in worker; it has no remotely supplied shell surface.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"prisme.local/platform/internal/automation"
	"prisme.local/platform/internal/config"
	"prisme.local/platform/internal/deployment"
	"prisme.local/platform/internal/store"
)

func main() {
	path := flag.String("config", "", "explicit automation targets")
	deploymentPath := flag.String("deployments", "", "existing deployment configuration")
	validate := flag.Bool("validate", false, "validate configuration without connecting or executing")
	once := flag.Bool("once", false, "execute one bounded worker tick")
	flag.Parse()
	c, err := automation.Load(*path)
	if err != nil {
		log.Fatal("automation configuration invalid")
	}
	dc, err := deployment.Load(*deploymentPath)
	if err != nil {
		log.Fatal("deployment configuration invalid")
	}
	de := &deployment.Engine{Config: dc, Driver: &deployment.Docker{Root: dc.Root}}
	driver := &automation.Native{Deployments: de}
	if driver.Validate(c) != nil {
		log.Fatal("automation target binding invalid")
	}
	if *validate {
		log.Print("automation v1 configuration valid")
		return
	}
	if runtime.GOOS != "linux" {
		log.Fatal("automation execution requires its configured Linux host")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	dsn, err := config.DatabaseURL()
	if err != nil {
		log.Fatal("database configuration unavailable")
	}
	db, err := store.Open(ctx, dsn)
	if err != nil {
		log.Fatal("database unavailable")
	}
	defer db.Close()
	if store.Ready(ctx, db) != nil {
		log.Fatal("explicit migration required")
	}
	de.DB = db
	if de.Initialize(ctx) != nil {
		log.Fatal("deployment initialization failed")
	}
	e := &automation.Engine{DB: db, Config: c, Driver: driver}
	if e.Initialize(ctx) != nil {
		log.Fatal("automation initialization failed")
	}
	if *once {
		if e.Tick(ctx) != nil {
			log.Fatal("automation tick failed; inspect execution history")
		}
		return
	}
	log.Print("automation worker ready; persistent global stop and policy controls apply")
	e.Run(ctx)
}
