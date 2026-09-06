// Package agent implements outgoing enrollment and self-heartbeat only.
package agent

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"prisme.local/platform/internal/pki"
)

type Config struct{ URL, CAFile, TokenFile, StateDir, Name string }
type Client struct {
	HTTP    *http.Client
	URL, ID string
}

func post(ctx context.Context, c *http.Client, base, path string, body, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", base+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return errors.New("API connection failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return fmt.Errorf("API rejected request (HTTP %d)", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 32*1024)).Decode(out)
}
func Connect(ctx context.Context, cfg Config) (*Client, error) {
	u, err := url.Parse(cfg.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("an HTTPS origin is required")
	}
	base := u.Scheme + "://" + u.Host
	ca, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return nil, errors.New("invalid trust certificate")
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}
	client := &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{TLSClientConfig: tlsConfig}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if err = os.MkdirAll(cfg.StateDir, 0700); err != nil {
		return nil, err
	}
	keyPath := filepath.Join(cfg.StateDir, "key.pem")
	keyPEM, err := os.ReadFile(keyPath)
	var key *ecdsa.PrivateKey
	if os.IsNotExist(err) {
		key, keyPEM, err = pki.Key()
		if err == nil {
			err = pki.WriteNew(keyPath, keyPEM)
		}
	} else if err == nil {
		key, err = pki.ParseKey(keyPEM)
	}
	if err != nil {
		return nil, err
	}
	certPath := filepath.Join(cfg.StateDir, "certificate.pem")
	certPEM, err := os.ReadFile(certPath)
	if os.IsNotExist(err) {
		token, readErr := os.ReadFile(cfg.TokenFile)
		if readErr != nil {
			return nil, readErr
		}
		// Persist the exact CSR to make a lost enrollment response safely retryable.
		csrPath := filepath.Join(cfg.StateDir, "request.pem")
		csrPEM, readErr := os.ReadFile(csrPath)
		if os.IsNotExist(readErr) {
			der, e := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
			if e != nil {
				return nil, e
			}
			csrPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})
			readErr = pki.WriteNew(csrPath, csrPEM)
		}
		if readErr != nil {
			return nil, readErr
		}
		var result struct {
			Certificate string `json:"certificate_pem"`
		}
		err = post(ctx, client, base, "/v1/agent/enroll", map[string]string{"token": string(bytes.TrimSpace(token)), "name": cfg.Name, "csr_pem": string(csrPEM), "protocol_version": "1"}, &result)
		if err != nil {
			return nil, err
		}
		certPEM = []byte(result.Certificate)
		if _, err = tls.X509KeyPair(certPEM, keyPEM); err != nil {
			return nil, errors.New("invalid enrollment identity")
		}
		if err = pki.WriteNew(certPath, certPEM); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	identity, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	leaf, err := x509.ParseCertificate(identity.Certificate[0])
	if err != nil {
		return nil, err
	}
	if _, err = leaf.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		return nil, errors.New("agent certificate expired or untrusted; re-enroll with a new state directory")
	}
	tlsConfig = tlsConfig.Clone()
	tlsConfig.Certificates = []tls.Certificate{identity}
	client.CloseIdleConnections()
	client.Transport = &http.Transport{TLSClientConfig: tlsConfig}
	return &Client{HTTP: client, URL: base, ID: leaf.Subject.CommonName}, nil
}
func (c *Client) Heartbeat(ctx context.Context) error {
	var result struct {
		ID           string   `json:"agent_id"`
		Capabilities []string `json:"capabilities"`
	}
	if err := post(ctx, c.HTTP, c.URL, "/v1/agent/heartbeat", map[string]string{"protocol_version": "1"}, &result); err != nil {
		return err
	}
	if result.ID != c.ID || len(result.Capabilities) != 1 || result.Capabilities[0] != "self:heartbeat" {
		return errors.New("unexpected agent capability contract")
	}
	return nil
}
