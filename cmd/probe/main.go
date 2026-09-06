package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"net/http"
	"os"
	"time"
)

func main() {
	url := flag.String("url", "https://localhost:8443/readyz", "health URL")
	ca := flag.String("ca", ".dev/pki/ca.pem", "trusted CA")
	flag.Parse()
	data, err := os.ReadFile(*ca)
	if err != nil {
		os.Exit(1)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(data) {
		os.Exit(1)
	}
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(*url)
	if err != nil {
		os.Exit(1)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		os.Exit(1)
	}
}
