// control-plane operates on an explicitly named, offline restored database.
package main

import (
	"context"
	"flag"
	"log"
	"prisme.local/platform/internal/config"
	"prisme.local/platform/internal/controlplane"
	"prisme.local/platform/internal/store"
	"time"
)

func main() {
	action := flag.String("action", "inspect", "inspect, quarantine, or release")
	confirm := flag.String("confirm-database", "", "exact restored database name for offline mutations")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
	var database string
	if db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&database) != nil {
		log.Fatal("database unavailable")
	}
	if *action == "inspect" {
		if err = store.Ready(ctx, db); err != nil {
			log.Fatal("schema incompatible or recovery quarantined")
		}
		log.Print("schema compatible; recovery gate released (not a production qualification)")
		return
	}
	if *confirm == "" || *confirm != database {
		log.Fatal("exact offline restored database confirmation required")
	}
	switch *action {
	case "quarantine":
		err = controlplane.Quarantine(ctx, db)
	case "release":
		err = controlplane.Release(ctx, db)
	default:
		log.Fatal("unknown action")
	}
	if err != nil {
		log.Fatal("offline recovery operation failed")
	}
	log.Print("offline recovery operation completed")
}
