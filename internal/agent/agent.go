// Package agent implements outgoing enrollment, renewal and bounded observations.
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

	"prisme.local/platform/internal/inventory"
	"prisme.local/platform/internal/pki"
)

type Config struct{ URL, CAFile, TokenFile, StateDir, Name string }
type Client struct {
	HTTP         *http.Client
	URL, ID      string
	Capabilities []string
	stateDir     string
	keyPEM       []byte
	leaf         *x509.Certificate
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
	return &Client{HTTP: client, URL: base, ID: leaf.Subject.CommonName, stateDir: cfg.StateDir, keyPEM: keyPEM, leaf: leaf}, nil
}
func (c *Client) Heartbeat(ctx context.Context) error {
	var result struct {
		ID           string   `json:"agent_id"`
		Capabilities []string `json:"capabilities"`
	}
	if err := post(ctx, c.HTTP, c.URL, "/v1/agent/heartbeat", map[string]string{"protocol_version": "1"}, &result); err != nil {
		return err
	}
	if result.ID != c.ID {
		return errors.New("unexpected agent capability contract")
	}
	c.Capabilities = result.Capabilities
	if time.Until(c.leaf.NotAfter) < 8*time.Hour {
		return c.Renew(ctx)
	}
	return nil
}

// Renew retains the private key and atomically swaps the public certificate. The API
// accepts the preceding serial for ten minutes to recover a lost response or restart.
func (c *Client) Renew(ctx context.Context) error {
	key, err := pki.ParseKey(c.keyPEM)
	if err != nil {
		return err
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
	if err != nil {
		return err
	}
	csr := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}))
	var result struct {
		Certificate string `json:"certificate_pem"`
	}
	if err = post(ctx, c.HTTP, c.URL, "/v1/agent/renew", map[string]string{"csr_pem": csr}, &result); err != nil {
		return err
	}
	identity, err := tls.X509KeyPair([]byte(result.Certificate), c.keyPEM)
	if err != nil {
		return errors.New("invalid renewal")
	}
	leaf, err := x509.ParseCertificate(identity.Certificate[0])
	if err != nil || leaf.Subject.CommonName != c.ID {
		return errors.New("invalid renewal identity")
	}
	old := c.HTTP.Transport.(*http.Transport)
	cfg := old.TLSClientConfig.Clone()
	if _, err = leaf.Verify(x509.VerifyOptions{Roots: cfg.RootCAs, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		return errors.New("untrusted renewal")
	}
	tmp, err := os.CreateTemp(c.stateDir, "certificate-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write([]byte(result.Certificate))
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(name, filepath.Join(c.stateDir, "certificate.pem")); err != nil {
		return err
	}
	cfg.Certificates = []tls.Certificate{identity}
	c.HTTP.CloseIdleConnections()
	c.HTTP.Transport = &http.Transport{TLSClientConfig: cfg}
	c.leaf = leaf
	return nil
}
func (c *Client) PublishInventory(ctx context.Context, snapshot inventory.Snapshot) error {
	if err := snapshot.Validate(time.Now().UTC()); err != nil {
		return err
	}
	var result map[string]string
	return post(ctx, c.HTTP, c.URL, "/v1/agent/inventory", snapshot, &result)
}
