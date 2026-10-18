package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"prisme.local/platform/internal/deployment"
)

// Explicit opt-in only. Run inside the disposable host-network Docker fixture
// supplied by tool/test-deployments.ps1; never points at an imported pilot.
func TestPhaseFourLiveComposeBlueGreen(t *testing.T) {
	if os.Getenv("PRISME_TEST_COMPOSE") != "1" {
		t.Skip("opt-in isolated Compose/Caddy fixture")
	}
	f, e, _ := deploymentFixture(t)
	ctx := context.Background()
	docker := func(args ...string) string {
		t.Helper()
		b, err := exec.Command("docker", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("fixture docker %v: %s", args, b)
		}
		return strings.TrimSpace(string(b))
	}
	refs := []string{}
	for _, tag := range []string{"caddy:2-alpine", "nginx:alpine"} {
		docker("pull", tag)
		ref := docker("image", "inspect", "--format", "{{index .RepoDigests 0}}", tag)
		refs = append(refs, ref)
	}
	for i, ref := range refs {
		_, err := f.db.Exec(`UPDATE github_releases SET document=jsonb_set(document,'{manifest,images,0,reference}',to_jsonb($2::text)) WHERE id=$1`, fmt.Sprintf("release-%d", i+1), ref)
		if err != nil {
			t.Fatal(err)
		}
	}
	m := e.Config.Targets[0]
	m.ID = "phase-four-live"
	m.Strategy = "blue-green"
	m.CaddyURL = "http://127.0.0.1:18744"
	m.CaddyID = "prisme-live"
	m.PublicHealthURL = "http://127.0.0.1:18743/"
	m.IngressService = "web"
	m.Services[0].BluePort = 18741
	m.Services[0].GreenPort = 18742
	m.ObservationSeconds = 3
	m.TimeoutSeconds = 180
	e.Config.Targets = []deployment.Manifest{m}
	e.Driver = &deployment.Docker{Root: e.Config.Root}
	if err := e.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	var route map[string]any
	if err := json.Unmarshal([]byte(`{"admin":{"listen":"127.0.0.1:18744"},"apps":{"http":{"servers":{"fixture":{"listen":[":18743"],"routes":[{"handle":[{"@id":"prisme-live","handler":"reverse_proxy","upstreams":[{"dial":"127.0.0.1:1"}]}]}]}}}}}`), &route); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(route)
	path := filepath.Join(t.TempDir(), "caddy.json")
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	proxy := exec.Command("caddy", "run", "--config", path)
	proxy.Stdout = io.Discard
	proxy.Stderr = io.Discard
	if err := proxy.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = proxy.Process.Kill(); _ = proxy.Wait() })
	for i := 0; i < 50; i++ {
		resp, err := http.Get(m.CaddyURL + "/config/")
		if err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Cleanup(func() {
		// Remove only the two fixture Compose projects. No image/system pruning.
		for _, slot := range []string{"blue", "green"} {
			ids := strings.Fields(docker("ps", "-aq", "--filter", "label=com.docker.compose.project=prisme-"+m.ID+"-"+slot))
			for _, id := range ids {
				docker("rm", "-f", "-v", id)
			}
			for _, network := range strings.Fields(docker("network", "ls", "-q", "--filter", "label=com.docker.compose.project=prisme-"+m.ID+"-"+slot)) {
				docker("network", "rm", network)
			}
		}
	})
	runJob(t, e, approveJob(t, e, prepareJob(t, e, "release-1", "live-first", false)), "succeeded")
	var requests, failures atomic.Int64
	monitorCtx, stop := context.WithCancel(ctx)
	done := make(chan bool)
	defer func() { stop(); <-done }()
	go func() {
		defer close(done)
		client := &http.Client{Timeout: 2 * time.Second}
		for {
			select {
			case <-monitorCtx.Done():
				return
			default:
			}
			resp, err := client.Get(m.PublicHealthURL)
			requests.Add(1)
			if err != nil {
				failures.Add(1)
			} else {
				if resp.StatusCode != 200 {
					failures.Add(1)
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
			time.Sleep(25 * time.Millisecond)
		}
	}()
	second := runJob(t, e, approveJob(t, e, prepareJob(t, e, "release-2", "live-second", false)), "succeeded")
	if second.Plan.Slot != "green" {
		t.Fatal("candidate did not use green slot")
	}
	restored := runJob(t, e, approveJob(t, e, prepareJob(t, e, "release-1", "live-restore", true)), "succeeded")
	stop()
	<-done
	if restored.Plan.Slot != "blue" || failures.Load() != 0 || requests.Load() < 20 {
		t.Fatalf("continuity: requests=%d failures=%d slot=%s", requests.Load(), failures.Load(), restored.Plan.Slot)
	}
	if err := e.Driver.Verify(ctx, restored.Plan); err != nil {
		t.Fatal(err)
	}
	t.Logf("REAL Compose/Caddy: deployed %s -> %s -> restored %s; %d HTTP requests, %d failures; digest, audit and health verified", refs[0], refs[1], refs[0], requests.Load(), failures.Load())
}
