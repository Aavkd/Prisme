package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"time"

	"prisme.local/platform/internal/agent"
)

func run() error {
	var cfg agent.Config
	flag.StringVar(&cfg.URL, "url", "https://localhost:8443", "API HTTPS origin")
	flag.StringVar(&cfg.CAFile, "ca", ".dev/pki/ca.pem", "trusted development CA")
	flag.StringVar(&cfg.TokenFile, "token-file", ".dev/enrollment-token", "single-use token file")
	flag.StringVar(&cfg.StateDir, "state-dir", ".dev/agent", "private agent identity directory")
	flag.StringVar(&cfg.Name, "name", "test-agent", "agent display name")
	once := flag.Bool("once", false, "send one verified heartbeat and exit")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	client, err := agent.Connect(ctx, cfg)
	if err != nil {
		return err
	}
	defer client.HTTP.CloseIdleConnections()
	if err = client.Heartbeat(ctx); err != nil {
		return err
	}
	log.Print("agent enrolled; self:heartbeat accepted")
	if *once {
		return nil
	}
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			if err = client.Heartbeat(ctx); err != nil {
				log.Print("heartbeat unavailable; retrying in 30 seconds")
			}
		}
	}
}
func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
