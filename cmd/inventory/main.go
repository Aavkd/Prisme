// Read-only discovery locally or over an existing SSH alias. Only the local API is mutated.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"prisme.local/platform/internal/inventory"
)

func run() error {
	alias := flag.String("ssh", "", "existing SSH alias (strict host verification)")
	python := flag.String("python", "python3", "local Python executable")
	output := flag.String("output", "", "private snapshot path for the unprivileged agent")
	groupReadable := flag.Bool("group-readable", false, "allow the collector group to read the snapshot (0640)")
	origin := flag.String("api", "", "HTTPS API origin to import an SSH snapshot")
	caFile := flag.String("ca", ".dev/pki/ca.pem", "trusted CA file")
	secretFile := flag.String("dev-secret-file", "", "development login secret file")
	tokenFile := flag.String("session-file", "", "existing short-lived API session file")
	name := flag.String("name", "", "display name for imported server")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	snapshot, err := inventory.Collect(ctx, *alias, *python)
	if err != nil {
		return err
	}
	if *output != "" {
		data, err := json.Marshal(snapshot)
		if err != nil {
			return err
		}
		mode := os.FileMode(0600)
		if *groupReadable {
			mode = 0640
		}
		if err = writeSnapshot(*output, data, mode); err != nil {
			return err
		}
	}
	if *origin == "" {
		log.Printf("Collected %d containers; Docker available: %v; partial warnings: %d", len(snapshot.Containers), snapshot.DockerAvailable, len(snapshot.Warnings))
		return nil
	}
	u, err := url.Parse(*origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || *alias == "" || *name == "" {
		return errors.New("import requires an HTTPS origin, SSH alias and display name")
	}
	ca, err := os.ReadFile(*caFile)
	if err != nil {
		return err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return errors.New("invalid CA")
	}
	client := &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	token := ""
	call := func(path string, data any) (map[string]any, error) {
		raw, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, "POST", *origin+path, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, errors.New("API connection failed")
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, errors.New("API import request rejected")
		}
		var out map[string]any
		err = json.NewDecoder(io.LimitReader(resp.Body, 32768)).Decode(&out)
		return out, err
	}
	if *tokenFile != "" {
		raw, e := os.ReadFile(*tokenFile)
		if e != nil {
			return e
		}
		token = strings.TrimSpace(string(raw))
	} else {
		raw, e := os.ReadFile(*secretFile)
		if e != nil {
			return e
		}
		session, e := call("/v1/dev/sessions", map[string]string{"secret": strings.TrimSpace(string(raw)), "device_name": "read-only-discovery"})
		if e != nil {
			return e
		}
		token, _ = session["access_token"].(string)
		defer func() {
			req, _ := http.NewRequest("DELETE", *origin+"/v1/session", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			resp, e := client.Do(req)
			if e == nil {
				resp.Body.Close()
			}
		}()
	}
	result, err := call("/v1/inventory-imports", map[string]any{"name": *name, "snapshot": snapshot})
	if err != nil {
		return err
	}
	log.Printf("Imported server %s: %d containers, %d warnings; source ssh-readonly", result["server_id"], len(snapshot.Containers), len(snapshot.Warnings))
	return nil
}

func writeSnapshot(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "inventory-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(mode); err == nil {
		_, err = tmp.Write(data)
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
	return os.Rename(name, path)
}
func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
