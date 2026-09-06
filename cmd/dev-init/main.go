// dev-init creates only new local development material, never prints credentials.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"log"
	"os"
	"path/filepath"

	"prisme.local/platform/internal/pki"
)

func run() error {
	dir := flag.String("dir", ".dev", "new private development directory")
	flag.Parse()
	if _, err := os.Stat(*dir); !os.IsNotExist(err) {
		return errors.New("destination already exists; refusing to replace development identities")
	}
	if err := os.MkdirAll(*dir, 0700); err != nil {
		return err
	}
	if err := pki.Init(filepath.Join(*dir, "pki")); err != nil {
		return err
	}
	for _, name := range []string{"bootstrap-secret", "db-password"} {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return err
		}
		if err := pki.WriteNew(filepath.Join(*dir, name), []byte(hex.EncodeToString(b))); err != nil {
			return err
		}
	}
	log.Print("development CA, TLS identity and secret files created; no credentials logged")
	return nil
}
func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
