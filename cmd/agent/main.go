package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"log"
	"os"
	"os/signal"
	"time"

	"prisme.local/platform/internal/agent"
	"prisme.local/platform/internal/inventory"
)

func run() error {
	var cfg agent.Config
	flag.StringVar(&cfg.URL, "url", "https://localhost:8443", "API HTTPS origin")
	flag.StringVar(&cfg.CAFile, "ca", ".dev/pki/ca.pem", "trusted development CA")
	flag.StringVar(&cfg.TokenFile, "token-file", ".dev/enrollment-token", "single-use token file")
	flag.StringVar(&cfg.StateDir, "state-dir", ".dev/agent", "private agent identity directory")
	flag.StringVar(&cfg.Name, "name", "test-agent", "agent display name")
	once := flag.Bool("once", false, "send one verified heartbeat and exit")
	inventoryFile := flag.String("inventory-file", "", "bounded snapshot from a separate read-only collector")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	client, err := agent.Connect(ctx, cfg)
	for failures := 1; err != nil; failures++ {
		if *once {
			return err
		}
		log.Print("agent connection unavailable; bounded reconnect pending")
		if !agent.Wait(ctx, agent.RetryDelay(failures)) {
			return nil
		}
		client, err = agent.Connect(ctx, cfg)
	}
	defer client.HTTP.CloseIdleConnections()
	publish := func() {
		if *inventoryFile == "" {
			return
		}
		allowed := false
		for _, cap := range client.Capabilities {
			if cap == "inventory:host" {
				allowed = true
			}
		}
		if !allowed {
			return
		}
		f, e := os.Open(*inventoryFile)
		if e != nil {
			log.Print("inventory file unavailable")
			return
		}
		defer f.Close()
		var snapshot inventory.Snapshot
		dec := json.NewDecoder(io.LimitReader(f, inventory.MaxBytes))
		dec.DisallowUnknownFields()
		if e = dec.Decode(&snapshot); e == nil {
			e = client.PublishInventory(ctx, snapshot)
		}
		if e != nil {
			log.Print("inventory rejected or unavailable; retaining last accepted observation")
		}
	}
	if *once {
		if err = client.Heartbeat(ctx); err != nil {
			return err
		}
		publish()
		return nil
	}
	failures := 0
	for {
		delay := 30 * time.Second
		if err = client.Heartbeat(ctx); err != nil {
			failures++
			delay = agent.RetryDelay(failures)
			log.Print("heartbeat unavailable; bounded reconnect pending")
		} else {
			failures = 0
			publish()
		}
		if !agent.Wait(ctx, delay) {
			return nil
		}
	}
}
func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
