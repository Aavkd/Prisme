// backup is a separate opt-in Linux worker. The API has no backup credentials.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"prisme.local/platform/internal/backup"
	"prisme.local/platform/internal/config"
	"prisme.local/platform/internal/store"
)

func main() {
	path := flag.String("config", "", "explicit backup configuration")
	validate := flag.Bool("validate", false, "validate configuration without executing work")
	once := flag.Bool("once", false, "schedule and execute at most one job")
	ack := flag.String("acknowledge", "", "mark an uncertain job failed after host reconciliation")
	initRepository := flag.String("init-repository", "", "explicitly initialize this repository; never replace an existing repository")
	flag.Parse()
	c, err := backup.Load(*path)
	if err != nil {
		log.Fatal("backup configuration invalid")
	}
	if *validate {
		log.Print("backup configuration v1 valid")
		return
	}
	if runtime.GOOS != "linux" {
		log.Fatal("backup execution requires the explicitly configured Linux host")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *initRepository != "" {
		if backup.InitializeRepository(ctx, c, *initRepository) != nil {
			log.Fatal("repository initialization failed")
		}
		log.Print("repository initialized; store its password independently")
		return
	}
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
		log.Fatal("explicit migration required")
	}
	e := &backup.Engine{DB: db, Config: c, Driver: &backup.Restic{}}
	if err = e.Initialize(ctx); err != nil {
		log.Fatal("backup initialization failed")
	}
	if *ack != "" {
		if e.Acknowledge(ctx, *ack) != nil {
			log.Fatal("job or host still requires reconciliation")
		}
		log.Print("uncertain job acknowledged as failed; nothing replayed")
		return
	}
	if *once {
		if e.Tick(ctx) != nil {
			log.Fatal("backup cycle failed; inspect job and host state")
		}
		return
	}
	log.Print("backup worker ready for configured policies")
	e.Run(ctx)
}
