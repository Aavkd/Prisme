package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prisme.local/platform/internal/automation"
	"prisme.local/platform/internal/deployment"
)

// This opt-in fixture owns only prisme-phase-nine-live-blue. A deliberately
// broken nginx document root survives restart; rollback recreates healthy Caddy.
func TestPhaseNineLiveComposeRemediation(t *testing.T) {
	if os.Getenv("PRISME_TEST_COMPOSE") != "1" {
		t.Skip("opt-in isolated Compose fixture")
	}
	f, de, _ := deploymentFixture(t)
	ctx := context.Background()
	docker := func(args ...string) string {
		t.Helper()
		b, err := exec.Command("docker", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("fixture docker %v: %s", args, b)
		}
		return strings.TrimSpace(string(b))
	}
	for i, tag := range []string{"caddy:2-alpine", "nginx:alpine"} {
		ref := docker("image", "inspect", "--format", "{{index .RepoDigests 0}}", tag)
		if _, err := f.db.Exec(`UPDATE github_releases SET document=jsonb_set(document,'{manifest,images,0,reference}',to_jsonb($2::text)) WHERE id=$1`, fmt.Sprintf("release-%d", i+1), ref); err != nil {
			t.Fatal(err)
		}
	}
	m := de.Config.Targets[0]
	m.ID = "phase-nine-live"
	m.Services[0].BluePort = 18751
	m.ObservationSeconds = 2
	m.TimeoutSeconds = 120
	de.Config.Targets = []deployment.Manifest{m}
	de.Config.CoordinationRoot = filepath.Join(t.TempDir(), "guard")
	de.Driver = &deployment.Docker{Root: de.Config.Root}
	if err := de.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, container := range strings.Fields(docker("ps", "-aq", "--filter", "label=com.docker.compose.project=prisme-phase-nine-live-blue")) {
			docker("rm", "-f", "-v", container)
		}
		for _, network := range strings.Fields(docker("network", "ls", "-q", "--filter", "label=com.docker.compose.project=prisme-phase-nine-live-blue")) {
			docker("network", "rm", network)
		}
	})
	runJob(t, de, approveJob(t, de, prepareJob(t, de, "release-1", "auto-healthy", false)), "succeeded")
	current := runJob(t, de, approveJob(t, de, prepareJob(t, de, "release-2", "auto-current", false)), "succeeded")
	container := docker("ps", "-q", "--filter", "label=com.docker.compose.project=prisme-phase-nine-live-blue")
	docker("exec", container, "rm", "/usr/share/nginx/html/index.html")
	if err := de.Driver.Verify(ctx, current.Plan); err != deployment.ErrHealth {
		t.Fatal("controlled outage was not observed", err)
	}
	c := automation.Config{Version: 1, Targets: []automation.Target{{ID: "pilot", Deployment: m.ID, Server: m.Server, Project: m.Project, Environment: m.Environment, Service: "web", Rule: "pilot-health"}}}
	driver := &automation.Native{Deployments: de}
	if err := driver.Validate(c); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	e := &automation.Engine{DB: f.db, Config: c, Driver: driver, Now: func() time.Time { return now }}
	p := defaultPolicy("autonomous")
	p.HealthyRelease = "release-1"
	p.TimeoutSeconds = 300
	b, _ := json.Marshal(p)
	if err := e.Publish(ctx, "policy", b, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	ctl, _ := e.Control(ctx)
	ctl.Stopped = false
	if _, err := e.SetControl(ctx, ctl, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	automationSignal(t, e, now, "failing", false)
	// Bind the synthetic observation to the actual manifest's project.
	if _, err := f.db.Exec(`UPDATE observation_rule_state SET document=jsonb_set(document,'{scope,project}',to_jsonb($1::text)) WHERE rule_id='pilot-health'`, m.Project); err != nil {
		t.Fatal(err)
	}
	automationTicks(t, e, &now, 25)
	runs := automationRuns(t, e)
	if len(runs) != 1 || runs[0].State != "succeeded" || runs[0].Attempts != 2 {
		t.Fatalf("native remediation: %+v", runs)
	}
	active, err := de.ActivePlan(ctx, m.ID)
	if err != nil || active.Release.ID != "release-1" || de.Driver.Verify(ctx, active) != nil {
		t.Fatal("healthy rollback not verified", err)
	}
	var jobs int
	if err = f.db.QueryRow(`SELECT count(*) FROM deployment_jobs WHERE automation_run_id=$1 AND state='succeeded'`, runs[0].ID).Scan(&jobs); err != nil || jobs != 1 {
		t.Fatal("rollback job correlation missing", err)
	}
	events, err := e.Events(ctx, runs[0].ID)
	if err != nil || len(events) < 10 {
		t.Fatal("timeline incomplete", err)
	}
	t.Logf("REAL Compose: controlled HTTP failure, owned service restart, healthy immutable rollback; %d attempts, %d timeline events", runs[0].Attempts, len(events))
}
