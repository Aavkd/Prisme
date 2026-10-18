package edge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCloudflareScopedReadWriteDriftAndUncertain(t *testing.T) {
	root := t.TempDir()
	read := filepath.Join(root, "read")
	write := filepath.Join(root, "write")
	os.WriteFile(read, []byte("fixture-reader"), 0600)
	os.WriteFile(write, []byte("fixture-writer"), 0600)
	zone := strings.Repeat("a", 32)
	recordID := strings.Repeat("b", 32)
	account := strings.Repeat("c", 32)
	current := map[string]any{"id": recordID, "name": "pilot.test", "type": "A", "content": "192.0.2.1", "ttl": float64(60), "proxied": true, "comment": "keep", "tags": []any{"owner:pilot"}, "modified_on": "first"}
	writes := 0
	ambiguous := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "outside") {
			t.Error("scope escaped")
		}
		if r.Method == "PATCH" {
			writes++
			if r.Header.Get("Authorization") != "Bearer fixture-writer" {
				t.Error("read token used for mutation")
			}
			var patch map[string]any
			json.NewDecoder(r.Body).Decode(&patch)
			if len(patch) != 1 {
				t.Error("unapproved fields sent")
			}
			current["content"] = patch["content"]
			current["modified_on"] = "updated"
			if ambiguous {
				w.WriteHeader(503)
				return
			}
		} else if r.Header.Get("Authorization") != "Bearer fixture-reader" {
			t.Error("write token used for read")
		}
		var result any = current
		pages := 1
		switch r.URL.Path {
		case "/zones/" + zone:
			result = map[string]any{"name": "pilot.test", "status": "active"}
		case "/zones/" + zone + "/dns_records":
			pages = 2
			if r.URL.Query().Get("page") == "1" {
				result = []any{current}
			} else {
				result = []any{map[string]any{"name": "other.test"}}
			}
		case "/accounts/" + account + "/cfd_tunnel":
			result = []any{map[string]any{"id": "tunnel", "name": "edge", "status": "healthy", "tunnel_secret": "never-return", "connections": []any{}}}
		}
		json.NewEncoder(w).Encode(map[string]any{"success": true, "result": result, "result_info": map[string]any{"total_pages": pages}})
	}))
	defer server.Close()
	checks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	defer checks.Close()
	c := Config{Version: 1, CoordinationRoot: root, Zones: []Zone{{ID: zone, Account: account, ReadTokenFile: read, WriteTokenFile: write}}}
	target := Target{ID: "dns", Kind: "dns", Domain: "pilot.test", Server: "edge", Environment: "test", WriteEnabled: true, Zone: zone, Record: recordID, Checks: []Check{{URL: checks.URL, Host: "pilot.test", Status: 200, Contains: "ok"}}}
	c.Targets = []Target{target}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	d := &Native{CloudflareBase: server.URL}
	ctx := context.Background()
	inventory, err := d.Discover(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(inventory)
	if strings.Contains(string(b), "never-return") || !strings.Contains(string(b), "other.test") {
		t.Fatal("inventory pagination or redaction")
	}
	snapshot, _, err := d.Prepare(ctx, c, target, Request{Value: "192.0.2.2"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if writes != 0 {
		t.Fatal("planning mutated")
	}
	current["comment"] = "external"
	state, _, err := d.Apply(ctx, c, target, snapshot)
	if state != "failed" || err == nil || writes != 0 {
		t.Fatal("stale snapshot applied")
	}
	current["comment"] = "keep"
	state, _, err = d.Apply(ctx, c, target, snapshot)
	if err != nil || state != "succeeded" || writes != 1 || current["ttl"] != float64(60) {
		t.Fatalf("write: %s %v", state, err)
	}
	inverse, _, err := d.Prepare(ctx, c, target, Request{}, &snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if inverse.After.Record["content"] != "192.0.2.1" {
		t.Fatal("logical rollback lost")
	}
	ambiguous = true
	state, _, err = d.Apply(ctx, c, target, inverse)
	if state != "uncertain" || err == nil || writes != 2 {
		t.Fatal("ambiguous mutation retried or reported successful")
	}
}

func TestEdgeConfigurationAndDNSValidation(t *testing.T) {
	for _, v := range []string{"a\nreverse_proxy attacker", "a {", "127.0.0.1", ""} {
		if validDNS(map[string]any{"type": "CNAME"}, v) {
			t.Fatal("unsafe DNS accepted")
		}
	}
	if validDNS(map[string]any{"type": "TXT"}, "secret") {
		t.Fatal("unbounded DNS record kind")
	}
	root := t.TempDir()
	base := Config{Version: 1, CoordinationRoot: root, Targets: []Target{{ID: "pilot", Kind: "caddy", Domain: "pilot.test", Server: "edge", Environment: "test", Caddy: &Caddy{Binary: filepath.Join(root, "caddy"), SourceFile: filepath.Join(root, "Caddyfile"), ManagedFile: filepath.Join(root, "managed"), AdminURL: "http://127.0.0.1:2019", Upstreams: []string{"app:8080"}}}}}
	if base.Validate() != nil {
		t.Fatal("valid config refused")
	}
	for _, url := range []string{"https://public.example", "http://localhost:2019", "http://192.0.2.1:2019", "http://127.0.0.1:2019/evil"} {
		base.Targets[0].Caddy.AdminURL = url
		if base.Validate() == nil {
			t.Fatal("unsafe admin URL accepted")
		}
	}
}
