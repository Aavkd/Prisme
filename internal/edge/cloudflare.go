package edge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
)

type envelope struct {
	Success bool            `json:"success"`
	Result  json.RawMessage `json:"result"`
	Info    struct {
		TotalPages int `json:"total_pages"`
	} `json:"result_info"`
}

func (d *Native) cf(ctx context.Context, file, method, path string, payload any) (envelope, error) {
	var out envelope
	token, e := os.ReadFile(file)
	if e != nil || len(token) > 8192 {
		return out, ErrUnavailable
	}
	secret := strings.TrimSpace(string(token))
	if secret == "" || strings.ContainsAny(secret, "\r\n") {
		return out, ErrInvalid
	}
	base := d.CloudflareBase
	if base == "" {
		base = "https://api.cloudflare.com/client/v4"
	}
	b, _ := json.Marshal(payload)
	req, e := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(b))
	if e != nil {
		return out, ErrInvalid
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Content-Type", "application/json")
	resp, e := d.client().Do(req)
	if e != nil {
		if method != "GET" {
			return out, ErrUncertain
		}
		return out, ErrUnavailable
	}
	defer resp.Body.Close()
	raw, e := bounded(resp.Body)
	if e != nil || json.Unmarshal(raw, &out) != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 || !out.Success {
		// No automatic write retry: transport, 429, or server errors can be ambiguous.
		if method != "GET" {
			return out, ErrUncertain
		}
		return out, ErrUnavailable
	}
	return out, nil
}
func (d *Native) list(ctx context.Context, file, path string) ([]any, error) {
	out := []any{}
	totalBytes := 0
	for page := 1; page <= 100; page++ {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		v, e := d.cf(ctx, file, "GET", fmt.Sprintf("%s%spage=%d&per_page=50", path, sep, page), nil)
		if e != nil {
			return nil, e
		}
		totalBytes += len(v.Result)
		if totalBytes > 4*1024*1024 {
			return nil, ErrUnavailable
		}
		var rows []any
		if json.Unmarshal(v.Result, &rows) != nil {
			return nil, ErrUnavailable
		}
		out = append(out, rows...)
		if v.Info.TotalPages > 100 {
			return nil, ErrUnavailable
		}
		if (v.Info.TotalPages > 0 && page >= v.Info.TotalPages) || (v.Info.TotalPages == 0 && len(rows) < 50) {
			return out, nil
		}
	}
	return nil, ErrUnavailable
}
func (d *Native) Discover(ctx context.Context, c Config) (map[string]any, error) {
	zones := []any{}
	for _, z := range c.Zones {
		meta, e := d.cf(ctx, z.ReadTokenFile, "GET", "/zones/"+z.ID, nil)
		if e != nil {
			return nil, e
		}
		dns, e := d.list(ctx, z.ReadTokenFile, "/zones/"+z.ID+"/dns_records")
		if e != nil {
			return nil, e
		}
		tunnels, e := d.list(ctx, z.ReadTokenFile, "/accounts/"+z.Account+"/cfd_tunnel?is_deleted=false")
		if e != nil {
			return nil, e
		}
		// Keep only explicitly useful inventory fields; never expose tunnel secrets.
		publicTunnels := []any{}
		for _, raw := range tunnels {
			m, ok := raw.(map[string]any)
			if !ok {
				return nil, ErrUnavailable
			}
			publicTunnels = append(publicTunnels, map[string]any{"id": m["id"], "name": m["name"], "status": m["status"], "connections": m["connections"]})
		}
		var zone map[string]any
		if json.Unmarshal(meta.Result, &zone) != nil {
			return nil, ErrUnavailable
		}
		zones = append(zones, map[string]any{"id": z.ID, "account": z.Account, "name": zone["name"], "status": zone["status"], "dns": dns, "tunnels": publicTunnels, "write_configured": z.WriteTokenFile != ""})
	}
	b, _ := json.Marshal(zones)
	if len(b) > 8*1024*1024 {
		return nil, ErrUnavailable
	}
	return map[string]any{"zones": zones}, nil
}
func (d *Native) record(ctx context.Context, c Config, t Target) (map[string]any, error) {
	z, e := c.Zone(t.Zone)
	if e != nil {
		return nil, e
	}
	v, e := d.cf(ctx, z.ReadTokenFile, "GET", "/zones/"+t.Zone+"/dns_records/"+t.Record, nil)
	if e != nil {
		return nil, e
	}
	var record map[string]any
	if json.Unmarshal(v.Result, &record) != nil || record["id"] != t.Record || record["name"] != t.Domain {
		return nil, ErrConflict
	}
	return record, nil
}
func validDNS(record map[string]any, value string) bool {
	if len(value) > 253 || strings.TrimSpace(value) != value {
		return false
	}
	switch record["type"] {
	case "A":
		ip := net.ParseIP(value)
		return ip != nil && ip.To4() != nil
	case "AAAA":
		ip := net.ParseIP(value)
		return ip != nil && ip.To4() == nil
	case "CNAME":
		return host.MatchString(value) && strings.Contains(value, ".") && net.ParseIP(value) == nil
	}
	return false
}
func dnsState(m map[string]any) any {
	return map[string]any{"id": m["id"], "name": m["name"], "type": m["type"], "content": m["content"], "ttl": m["ttl"], "proxied": m["proxied"], "comment": m["comment"], "tags": m["tags"], "settings": m["settings"]}
}
func (d *Native) prepareDNS(ctx context.Context, c Config, t Target, r Request, rollback *Snapshot) (Snapshot, Plan, error) {
	before, e := d.record(ctx, c, t)
	if e != nil {
		return Snapshot{}, Plan{}, e
	}
	value := r.Value
	if rollback != nil {
		if Hash(dnsState(before)) != Hash(dnsState(rollback.After.Record)) {
			return Snapshot{}, Plan{}, ErrConflict
		}
		var ok bool
		value, ok = rollback.Before.Record["content"].(string)
		if !ok {
			return Snapshot{}, Plan{}, ErrInvalid
		}
	}
	if !validDNS(before, value) || before["content"] == value {
		return Snapshot{}, Plan{}, ErrInvalid
	}
	if _, e = d.checks(ctx, t.Checks); e != nil {
		return Snapshot{}, Plan{}, e
	}
	after := map[string]any{}
	for k, v := range before {
		after[k] = v
	}
	after["content"] = value
	a, _ := json.MarshalIndent(dnsState(before), "", "  ")
	b, _ := json.MarshalIndent(dnsState(after), "", "  ")
	return Snapshot{Before: State{Record: before}, After: State{Record: after}}, Plan{Before: string(a), After: string(b), Impact: "Change DNS destination for " + t.Domain + ". Cached answers can persist until TTL expiry; tunnel, Access and TLS settings are preserved.", Verification: "Read back the DNS record and run configured HTTP/HTTPS checks. Global DNS propagation is not guaranteed.", Rollback: "Prepare a new approved plan using the saved DNS content. Concurrent changes or an ambiguous write require reconciliation.", Checks: t.Checks}, nil
}
func (d *Native) applyDNS(ctx context.Context, c Config, t Target, s Snapshot) (string, map[string]any, error) {
	current, e := d.record(ctx, c, t)
	if e != nil {
		return "failed", nil, e
	}
	if Hash(current) != Hash(s.Before.Record) {
		return "failed", nil, ErrConflict
	}
	if _, e = d.checks(ctx, t.Checks); e != nil {
		return "failed", nil, e
	}
	z, e := c.Zone(t.Zone)
	if e != nil || z.WriteTokenFile == "" {
		return "failed", nil, ErrInvalid
	}
	// PATCH only the approved content: preserve TTL, proxying, comments and metadata.
	_, e = d.cf(ctx, z.WriteTokenFile, "PATCH", "/zones/"+t.Zone+"/dns_records/"+t.Record, map[string]any{"content": s.After.Record["content"]})
	if e != nil {
		return "uncertain", nil, ErrUncertain
	}
	read, e := d.record(ctx, c, t)
	if e != nil || Hash(dnsState(read)) != Hash(dnsState(s.After.Record)) {
		return "uncertain", nil, ErrUncertain
	}
	evidence, e := d.checks(ctx, t.Checks)
	if evidence == nil {
		evidence = map[string]any{}
	}
	evidence["record_verified"] = true
	evidence["global_propagation_verified"] = false
	if e != nil {
		return "uncertain", evidence, ErrUncertain
	}
	return "succeeded", evidence, nil
}
