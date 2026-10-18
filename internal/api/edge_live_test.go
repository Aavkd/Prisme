package api_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prisme.local/platform/internal/edge"
)

func edgePort(t *testing.T) string {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	return l.Addr().String()
}

func TestPhaseEightLiveCaddySharedDomains(t *testing.T) {
	binary := os.Getenv("PRISME_TEST_CADDY_BINARY")
	if binary == "" {
		t.Skip("set PRISME_TEST_CADDY_BINARY for real isolated Caddy validation")
	}
	f, key := operationsFixture(t)
	root := t.TempDir()
	admin := edgePort(t)
	listen := edgePort(t)
	blue := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "healthy blue") }))
	defer blue.Close()
	green := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "healthy green") }))
	defer green.Close()
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer broken.Close()
	source := filepath.Join(root, "Caddyfile")
	block := filepath.Join(root, "pilot.caddy")
	historical := fmt.Sprintf("{\n admin %s\n auto_https off\n http_port %s\n}\nhttp://other.test {\n respond \"other untouched\"\n}\n", admin, strings.Split(listen, ":")[1])
	if err := os.WriteFile(source, []byte(historical), 0600); err != nil {
		t.Fatal(err)
	}
	historical += "\n(shared-security) {\n header X-Frame-Options DENY\n}\n"
	if err := os.WriteFile(source, []byte(historical), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "run", "--config", source, "--adapter", "caddyfile")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	client := &http.Client{Timeout: time.Second}
	ready := false
	for i := 0; i < 100; i++ {
		resp, err := client.Get("http://" + admin + "/config/")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				ready = true
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ready {
		t.Fatal("isolated Caddy did not start")
	}
	target := edge.Target{ID: "pilot", Kind: "caddy", Server: "edge-local", Environment: "test", Domain: "pilot.test", WriteEnabled: true, Caddy: &edge.Caddy{Binary: binary, AdminURL: "http://" + admin, SourceFile: source, ManagedFile: block, HTTPOnly: true, Upstreams: []string{strings.TrimPrefix(blue.URL, "http://"), strings.TrimPrefix(green.URL, "http://"), strings.TrimPrefix(broken.URL, "http://")}}, Checks: []edge.Check{{URL: "http://" + listen, Host: "other.test", Status: 200, Contains: "other untouched"}, {URL: "http://" + listen, Host: "pilot.test", Status: 200, Contains: "healthy"}}}
	c := edge.Config{Version: 1, CoordinationRoot: filepath.Join(root, "guard"), Targets: []edge.Target{target}}
	c.Targets[0].Caddy.Snippets = []string{"shared-security"}
	if err := f.app.ConfigureEdge(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	e := f.app.Edge
	e.Driver = &edge.Native{}
	ctx := context.Background()
	assertPilot := func(expected string) {
		t.Helper()
		req, _ := http.NewRequest("GET", "http://"+listen, nil)
		req.Host = "pilot.test"
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if err != nil || resp.StatusCode != 200 || string(body) != expected {
			t.Fatalf("pilot response: %d %q %v", resp.StatusCode, body, err)
		}
		if resp.Header.Get("X-Frame-Options") != "DENY" {
			t.Fatal("shared security snippet lost")
		}
	}
	prepare := func(value, rollback string) edge.Job {
		t.Helper()
		j, err := e.Prepare(ctx, edge.Request{Target: "pilot", Value: value, Rollback: rollback}, edge.ID())
		if err != nil {
			t.Fatal(err)
		}
		if err = e.Tick(ctx); err != nil {
			t.Fatal(err)
		}
		j, err = e.Job(ctx, j.ID)
		if err != nil || j.State != "planned" {
			t.Fatalf("plan %s: %s %v", j.ID, j.State, err)
		}
		return j
	}
	approve := func(j edge.Job) edge.Job {
		t.Helper()
		proof := operationProof(t, f, key, map[string]any{"action": "edge_approve", "target": j.ID, "payload": map[string]any{"revision": j.Revision}})
		f.call("POST", "/v1/edge/jobs/"+j.ID+"/decision", map[string]any{"action": "approve", "revision": j.Revision, "phrase": edge.Confirmation(j), "proof": proof}, 200)
		if err := e.Tick(ctx); err != nil {
			t.Fatal(err)
		}
		j, _ = e.Job(ctx, j.ID)
		return j
	}
	imported := approve(prepare(target.Caddy.Upstreams[0], ""))
	if imported.State != "succeeded" {
		t.Fatalf("import: %s / %s", imported.State, imported.ErrorCode)
	}
	assertPilot("healthy blue")
	saved, _ := os.ReadFile(source)
	if !strings.HasPrefix(string(saved), historical) {
		t.Fatal("historical source changed")
	}
	// Qualify the pilot alongside the unaffected domain for every subsequent plan.
	changed := approve(prepare(target.Caddy.Upstreams[1], ""))
	if changed.State != "succeeded" {
		t.Fatalf("change: %s", changed.State)
	}
	assertPilot("healthy green")
	inverse := approve(prepare("", changed.ID))
	if inverse.State != "succeeded" {
		t.Fatal("explicit rollback failed", inverse.State)
	}
	assertPilot("healthy blue")
	recovered := approve(prepare(target.Caddy.Upstreams[2], ""))
	if recovered.State != "rolled_back" {
		t.Fatal("unhealthy pilot did not rollback", recovered.State)
	}
	assertPilot("healthy blue")
	obs, err := e.Driver.Observe(ctx, e.Config, e.Config.Targets[0])
	if err != nil || obs["drift"] != false {
		t.Fatal("source/active not restored", err)
	}
	final, _ := os.ReadFile(source)
	if string(final) != string(saved) {
		t.Fatal("unrelated historical domains rewritten")
	}
	// A malformed historical edit cannot produce an approvable plan or reload.
	os.WriteFile(source, append(final, []byte("\ninvalid_directive {\n")...), 0600)
	j, err := e.Prepare(ctx, edge.Request{Target: "pilot", Value: target.Caddy.Upstreams[1]}, edge.ID())
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	j, _ = e.Job(ctx, j.ID)
	if j.State != "failed" {
		t.Fatal("invalid source accepted")
	}
}
