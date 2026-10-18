package deployment

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

func validManifest() Manifest {
	return Manifest{Version: 1, ID: "pilot-local", Project: "pilot", Environment: "test", Server: "localhost", RepositoryID: "1", Repository: "fixture/pilot", Branch: "main", Enabled: true, Strategy: "recreate", Migration: "none", ObservationSeconds: 1, TimeoutSeconds: 30, MinimumFreeBytes: 1024 * 1024, Services: []Service{{Name: "web", Port: 80, BluePort: 18401, HealthPath: "/", Stateless: true}}}
}
func TestManifestStrictValidation(t *testing.T) {
	m := validManifest()
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Manifest){func(m *Manifest) { m.Services[0].Name = "web;rm" }, func(m *Manifest) { m.Services[0].EnvFile = "../secret" }, func(m *Manifest) { m.Services[0].HealthPath = "//external.example" }, func(m *Manifest) { m.ObservationSeconds = 100 }, func(m *Manifest) { m.Strategy = "blue-green" }, func(m *Manifest) { m.Services[0].Port = 0 }} {
		candidate := validManifest()
		mutate(&candidate)
		if candidate.Validate() == nil {
			t.Fatalf("unsafe manifest accepted: %+v", candidate)
		}
	}
	b, _ := json.Marshal(m)
	b = append(b[:len(b)-1], []byte(`,"unknown":true}`)...)
	if Decode(b, &Manifest{}) == nil {
		t.Fatal("unknown fields ignored")
	}
	if Decode([]byte(`{} {}`), &Manifest{}) == nil {
		t.Fatal("trailing input accepted")
	}
	m.Legacy = true
	m.BackupRequired = true
	m.Migration = "incompatible"
	m.Enabled = false
	if len(m.Blockers()) != 4 {
		t.Fatal("missing policy blockers")
	}
}
func TestSecretSnapshotAndComposeRestrictions(t *testing.T) {
	root := t.TempDir()
	m := validManifest()
	m.Services[0].EnvFile = "web.env"
	if err := os.Mkdir(filepath.Join(root, "secrets"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "secrets", "web.env")
	os.WriteFile(path, []byte("PASSWORD=do-not-disclose"), 0600)
	hashes, err := SecretHashes(root, m)
	if err != nil {
		t.Fatal(err)
	}
	p := Plan{Manifest: m, Slot: "blue", SecretHashes: hashes, Images: map[string]string{"web": "example/web@sha256:" + strings.Repeat("a", 64)}}
	d := Docker{Root: root}
	b, err := d.Compose(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "do-not-disclose") || strings.Contains(string(b), "build") || strings.Contains(string(b), "volumes") {
		t.Fatal("unsafe generated compose")
	}
	if !d.secretsMatch(p) {
		t.Fatal("snapshot does not match")
	}
	os.WriteFile(path, []byte("PASSWORD=changed"), 0600)
	if d.secretsMatch(p) {
		t.Fatal("secret change not detected")
	}
}
func TestCaddyOwnedHandlerCASPreservesOtherFields(t *testing.T) {
	current := "127.0.0.1:1"
	patches := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.Header().Set("Etag", `"/config/apps/http/servers/pilot/routes/0/handle/0 abc"`)
			json.NewEncoder(w).Encode(map[string]any{"@id": "prisme-pilot", "handler": "reverse_proxy", "upstreams": []upstream{{Dial: current}}, "headers": map[string]any{"request": map[string]any{}}})
			return
		}
		if r.Method != "PATCH" || r.URL.Path != "/id/prisme-pilot/upstreams" || r.Header.Get("If-Match") == "" {
			t.Error("unscoped route mutation")
			w.WriteHeader(400)
			return
		}
		var u []upstream
		if json.NewDecoder(r.Body).Decode(&u) != nil || len(u) != 1 {
			t.Error("invalid upstreams")
		}
		patches++
		current = u[0].Dial
	}))
	defer server.Close()
	m := validManifest()
	m.Strategy = "blue-green"
	m.CaddyURL = server.URL
	m.CaddyID = "prisme-pilot"
	m.IngressService = "web"
	m.PublicHealthURL = server.URL
	m.Services[0].GreenPort = 18402
	p := Plan{Manifest: m, Slot: "blue"}
	d := Docker{}
	if err := d.switchTraffic(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if patches != 1 || current != "127.0.0.1:18401" {
		t.Fatal("route did not switch")
	}
	if d.switchTraffic(context.Background(), p) == nil || patches != 1 {
		t.Fatal("drift overwritten")
	}
	current = "external:80"
	if _, _, err := d.route(context.Background(), p, "blue"); err == nil {
		t.Fatal("external change not detected")
	}
}
