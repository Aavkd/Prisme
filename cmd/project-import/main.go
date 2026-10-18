// project-import is a read-only comparison, not a deployment authorization.
package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"os"
	"prisme.local/platform/internal/inventory"
	"prisme.local/platform/internal/onboarding"
	"time"
)

func read(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		log.Fatal("evidence file unavailable")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, inventory.MaxBytes+1))
	if err != nil || len(b) > inventory.MaxBytes {
		log.Fatal("evidence file exceeds limit")
	}
	return b
}
func main() {
	project := flag.String("project", "", "exact Compose project name")
	declared := flag.String("declared", "", "private declared Compose JSON file")
	host := flag.String("host-compose", "", "private observed host Compose JSON file")
	snapshot := flag.String("inventory", "", "bounded inventory snapshot JSON file")
	flag.Parse()
	var inv inventory.Snapshot
	if json.Unmarshal(read(*snapshot), &inv) != nil {
		log.Fatal("inventory invalid")
	}
	r, err := onboarding.Compare(*project, read(*declared), read(*host), inv, time.Now().UTC())
	if err != nil {
		log.Fatal("project import evidence invalid")
	}
	if json.NewEncoder(os.Stdout).Encode(r) != nil {
		log.Fatal("report output failed")
	}
	if !r.Ready {
		os.Exit(2)
	}
}
