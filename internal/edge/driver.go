package edge

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

type State struct {
	Source  string          `json:"source,omitempty"`
	Managed string          `json:"managed,omitempty"`
	Active  json.RawMessage `json:"active,omitempty"`
	Record  map[string]any  `json:"record,omitempty"`
}
type Snapshot struct {
	Before State `json:"before"`
	After  State `json:"after"`
}
type Plan struct {
	Before       string  `json:"before"`
	After        string  `json:"after"`
	Impact       string  `json:"impact"`
	Verification string  `json:"verification"`
	Rollback     string  `json:"rollback"`
	Checks       []Check `json:"checks"`
}
type Driver interface {
	Observe(context.Context, Config, Target) (map[string]any, error)
	Discover(context.Context, Config) (map[string]any, error)
	Prepare(context.Context, Config, Target, Request, *Snapshot) (Snapshot, Plan, error)
	Apply(context.Context, Config, Target, Snapshot) (string, map[string]any, error)
}
type Native struct {
	CloudflareBase string
	Client         *http.Client
}

func (d *Native) client() *http.Client {
	if d.Client != nil {
		return d.Client
	}
	return &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func bounded(r io.Reader) ([]byte, error) {
	b, e := io.ReadAll(io.LimitReader(r, 2*1024*1024+1))
	if e != nil || len(b) > 2*1024*1024 {
		return nil, ErrUnavailable
	}
	return b, nil
}
func (d *Native) checks(ctx context.Context, checks []Check) (map[string]any, error) {
	results := []any{}
	failed := false
	for _, c := range checks {
		req, e := http.NewRequestWithContext(ctx, "GET", c.URL, nil)
		if e != nil {
			return nil, ErrInvalid
		}
		req.Host = c.Host
		resp, e := d.client().Do(req)
		ok := false
		status := 0
		tls := false
		if e == nil {
			status = resp.StatusCode
			tls = resp.TLS != nil && len(resp.TLS.VerifiedChains) > 0
			b, readErr := bounded(resp.Body)
			resp.Body.Close()
			ok = readErr == nil && status == c.Status && (c.Contains == "" || bytes.Contains(b, []byte(c.Contains)))
		}
		results = append(results, map[string]any{"url": c.URL, "host": c.Host, "status": status, "passed": ok, "tls_verified": tls})
		if !ok {
			failed = true
		}
	}
	evidence := map[string]any{"checks": results}
	if failed {
		return evidence, ErrUnavailable
	}
	return evidence, nil
}
func (d *Native) Observe(ctx context.Context, c Config, t Target) (map[string]any, error) {
	if t.Kind == "dns" {
		r, e := d.record(ctx, c, t)
		if e != nil {
			return nil, e
		}
		return map[string]any{"record": r}, nil
	}
	s, _, e := d.caddyState(ctx, t)
	if e != nil {
		return nil, e
	}
	adapted, e := d.adapt(ctx, t, s.Source, s.Managed)
	if e != nil {
		return nil, e
	}
	version, e := caddyCommand(ctx, t.Caddy, "version")
	if e != nil {
		return nil, e
	}
	var routes any
	_ = json.Unmarshal(s.Active, &routes)
	return map[string]any{"source": publicSource(s.Source), "source_hash": Hash(s.Source), "active_hash": Hash(jsonValue(s.Active)), "drift": Hash(jsonValue(adapted)) != Hash(jsonValue(s.Active)), "managed": s.Managed, "version": string(version), "configuration": publicConfig(routes)}, nil
}
func jsonValue(b []byte) any { var v any; _ = json.Unmarshal(b, &v); return v }
func (d *Native) Prepare(ctx context.Context, c Config, t Target, r Request, rollback *Snapshot) (Snapshot, Plan, error) {
	if t.Kind == "caddy" {
		return d.prepareCaddy(ctx, t, r, rollback)
	}
	return d.prepareDNS(ctx, c, t, r, rollback)
}
func (d *Native) Apply(ctx context.Context, c Config, t Target, s Snapshot) (string, map[string]any, error) {
	if t.Kind == "caddy" {
		return d.applyCaddy(ctx, t, s)
	}
	return d.applyDNS(ctx, c, t, s)
}
