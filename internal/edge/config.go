// Package edge owns bounded, planned Caddy and Cloudflare changes.
package edge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	ErrInvalid     = errors.New("edge_invalid")
	ErrConflict    = errors.New("edge_conflict")
	ErrUnavailable = errors.New("edge_unavailable")
	ErrUncertain   = errors.New("edge_uncertain")
	name           = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)
	providerID     = regexp.MustCompile(`^[a-f0-9]{32}$`)
	host           = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)
)

type Config struct {
	Version          int      `json:"version"`
	CoordinationRoot string   `json:"coordination_root"`
	Targets          []Target `json:"targets"`
	Zones            []Zone   `json:"zones"`
}
type Zone struct {
	ID             string `json:"id"`
	Account        string `json:"account"`
	ReadTokenFile  string `json:"read_token_file"`
	WriteTokenFile string `json:"write_token_file"`
}
type Target struct {
	ID           string  `json:"id"`
	Kind         string  `json:"kind"`
	Server       string  `json:"server"`
	Environment  string  `json:"environment"`
	WriteEnabled bool    `json:"write_enabled"`
	Domain       string  `json:"domain"`
	Caddy        *Caddy  `json:"caddy,omitempty"`
	Zone         string  `json:"zone,omitempty"`
	Record       string  `json:"record,omitempty"`
	Checks       []Check `json:"checks"`
}
type Caddy struct {
	HTTPOnly    bool     `json:"http_only"`
	Binary      string   `json:"binary"`
	AdminURL    string   `json:"admin_url"`
	SourceFile  string   `json:"source_file"`
	ManagedFile string   `json:"managed_file"`
	Upstreams   []string `json:"upstreams"`
	Snippets    []string `json:"snippets"`
}
type Check struct {
	URL      string `json:"url"`
	Host     string `json:"host"`
	Status   int    `json:"status"`
	Contains string `json:"contains"`
}
type Request struct {
	Target   string `json:"target"`
	Value    string `json:"value"`
	Rollback string `json:"rollback"`
}

func Hash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func Load(path string) (Config, error) {
	var c Config
	b, e := os.ReadFile(path)
	if e != nil || len(b) > 256*1024 {
		return c, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil || d.Decode(new(any)) != io.EOF {
		return c, ErrInvalid
	}
	return c, c.Validate()
}
func (c Config) Target(id string) (Target, error) {
	for _, t := range c.Targets {
		if t.ID == id {
			return t, nil
		}
	}
	return Target{}, ErrInvalid
}
func (c Config) Zone(id string) (Zone, error) {
	for _, z := range c.Zones {
		if z.ID == id {
			return z, nil
		}
	}
	return Zone{}, ErrInvalid
}
func (c Config) Validate() error {
	if c.Version != 1 || !filepath.IsAbs(c.CoordinationRoot) || len(c.Targets) > 50 || len(c.Zones) > 20 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, z := range c.Zones {
		if !providerID.MatchString(z.ID) || !providerID.MatchString(z.Account) || !filepath.IsAbs(z.ReadTokenFile) || seen[z.ID] {
			return ErrInvalid
		}
		if z.WriteTokenFile != "" && (!filepath.IsAbs(z.WriteTokenFile) || z.WriteTokenFile == z.ReadTokenFile) {
			return ErrInvalid
		}
		seen[z.ID] = true
	}
	seen = map[string]bool{}
	files := map[string]bool{}
	for _, t := range c.Targets {
		if !name.MatchString(t.ID) || !name.MatchString(t.Server) || !name.MatchString(t.Environment) || !host.MatchString(t.Domain) || len(t.Domain) > 253 || seen[t.ID] || t.ID == "cloudflare" {
			return ErrInvalid
		}
		seen[t.ID] = true
		if len(t.Checks) > 30 || (t.WriteEnabled && len(t.Checks) == 0) {
			return ErrInvalid
		}
		targetChecked := false
		for _, v := range t.Checks {
			u, e := url.Parse(v.URL)
			if e == nil && (v.Host == t.Domain || (v.Host == "" && u.Hostname() == t.Domain)) {
				targetChecked = true
			}
			if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Fragment != "" || v.Status < 200 || v.Status > 399 || len(v.Contains) > 1024 || strings.ContainsAny(v.Host, "\r\n") {
				return ErrInvalid
			}
		}
		if t.WriteEnabled && !targetChecked {
			return ErrInvalid
		}
		switch t.Kind {
		case "caddy":
			a := t.Caddy
			if a != nil && a.HTTPOnly && t.Environment != "test" {
				return ErrInvalid
			}
			if a == nil || t.Zone != "" || t.Record != "" || !filepath.IsAbs(a.Binary) || !filepath.IsAbs(a.SourceFile) || !filepath.IsAbs(a.ManagedFile) || a.SourceFile == a.ManagedFile || files[a.ManagedFile] || len(a.Upstreams) == 0 {
				return ErrInvalid
			}
			files[a.ManagedFile] = true
			if len(a.Snippets) > 20 {
				return ErrInvalid
			}
			for _, snippet := range a.Snippets {
				if !name.MatchString(snippet) {
					return ErrInvalid
				}
			}
			// A literal loopback address prevents DNS rebinding and public admin exposure.
			u, e := url.Parse(a.AdminURL)
			if e != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
				return ErrInvalid
			}
			ip := net.ParseIP(u.Hostname())
			if ip == nil || !ip.IsLoopback() {
				return ErrInvalid
			}
			if strings.ContainsAny(a.ManagedFile+a.SourceFile, "\r\n\"{}") {
				return ErrInvalid
			}
			for _, up := range a.Upstreams {
				h, p, e := net.SplitHostPort(up)
				if e != nil || (!host.MatchString(h) && net.ParseIP(h) == nil) || p == "" || strings.ContainsAny(up, " \t\r\n{}#$") {
					return ErrInvalid
				}
			}
		case "dns":
			z, e := c.Zone(t.Zone)
			if e != nil || t.Caddy != nil || !providerID.MatchString(t.Record) || (t.WriteEnabled && z.WriteTokenFile == "") {
				return ErrInvalid
			}
		default:
			return ErrInvalid
		}
	}
	return nil
}
func (c Config) PublicTargets() []any {
	out := []any{}
	for _, t := range c.Targets {
		v := map[string]any{"id": t.ID, "kind": t.Kind, "server": t.Server, "environment": t.Environment, "domain": t.Domain, "write_enabled": t.WriteEnabled, "checks": t.Checks, "choices": []string{}}
		if t.Caddy != nil {
			v["choices"] = t.Caddy.Upstreams
		}
		out = append(out, v)
	}
	return out
}
