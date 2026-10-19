// pki creates and renews the production control-plane PKI. It never prints a
// private key; ca-base64 prints only the public CA certificate (DER, base64),
// as needed by Caddy's tls_trust_pool inline.
package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"prisme.local/platform/internal/pki"
)

func run() error {
	dir := flag.String("dir", "", "private PKI directory")
	name := flag.String("name", "Prisme control plane CA", "CA common name")
	dns := flag.String("dns", "prisme-api,localhost", "comma-separated server names")
	caYears := flag.Int("ca-years", 5, "CA validity in years")
	days := flag.Int("server-days", 397, "server certificate validity in days")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "usage: pki -dir DIR [flags] init|renew-server|ca-base64")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *dir == "" || flag.NArg() != 1 {
		flag.Usage()
		return errors.New("missing directory or command")
	}
	names := strings.Split(*dns, ",")
	switch flag.Arg(0) {
	case "init":
		if err := pki.InitProduction(*dir, *name, names, *caYears, *days); err != nil {
			return err
		}
		log.Print("CA and server certificate created; no key printed")
	case "renew-server":
		expires, err := pki.RenewServer(*dir, names, *days)
		if err != nil {
			return err
		}
		log.Print("server certificate renewed until ", expires.Format("2006-01-02"), "; restart the API")
	case "ca-base64":
		b, err := os.ReadFile(filepath.Join(*dir, "ca.pem"))
		if err != nil {
			return errors.New("CA certificate unavailable")
		}
		block, _ := pem.Decode(b)
		if block == nil || block.Type != "CERTIFICATE" {
			return errors.New("invalid CA certificate")
		}
		sum := sha256.Sum256(block.Bytes)
		log.Printf("CA SHA-256 fingerprint %X", sum)
		fmt.Println(base64.StdEncoding.EncodeToString(block.Bytes))
	default:
		flag.Usage()
		return errors.New("unknown command")
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
