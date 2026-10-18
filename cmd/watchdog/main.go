// The watchdog is provisioned independently, outside the control-plane host.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"prisme.local/platform/internal/notifications"
)

func run() error {
	path := flag.String("config", "", "private watchdog JSON configuration")
	flag.Parse()
	b, err := os.ReadFile(*path)
	if err != nil {
		return errors.New("watchdog configuration unavailable")
	}
	var cfg struct {
		URL             string                    `json:"url"`
		StateFile       string                    `json:"state_file"`
		CredentialsFile string                    `json:"credentials_file"`
		Tokens          []string                  `json:"tokens"`
		Threshold       int                       `json:"threshold"`
		Urgent          bool                      `json:"urgent"`
		Instance        string                    `json:"instance"`
		Preferences     notifications.Preferences `json:"preferences"`
	}
	// Defaults apply only to omitted fields; an explicit enabled:false stays disabled.
	cfg.Preferences = notifications.Preferences{Enabled: true, Timezone: "UTC"}
	if len(b) > 65536 || json.Unmarshal(b, &cfg) != nil || cfg.StateFile == "" {
		return errors.New("invalid watchdog configuration")
	}
	sender, err := notifications.NewFCM(cfg.CredentialsFile)
	if err != nil {
		return err
	}
	w := notifications.Watchdog{URL: cfg.URL, StateFile: cfg.StateFile, Tokens: cfg.Tokens, Threshold: cfg.Threshold, Urgent: cfg.Urgent, Instance: cfg.Instance, Sender: sender, Now: time.Now, Preferences: cfg.Preferences, HTTP: &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		if err = w.Tick(ctx); err != nil {
			log.Print("watchdog check or delivery unavailable")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
