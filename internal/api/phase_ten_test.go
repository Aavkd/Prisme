package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"prisme.local/platform/internal/agent"
	gh "prisme.local/platform/internal/github"
	"prisme.local/platform/internal/inventory"
)

func TestPhaseTenOnboardingPreviewAndFirstDeployment(t *testing.T) {
	f, e, d := deploymentFixture(t)
	f.app.GitHub = &gh.Service{DB: f.db, Config: gh.Config{Repositories: []gh.Repository{{ID: "11", Name: "fixture/sounding-lines", Environment: "test", WorkflowID: "70"}}}}
	f.call("GET", "/v1/onboarding", nil, 401)
	f.dev()
	if f.call("GET", "/v1/onboarding", nil, 200)["enabled"] != true {
		t.Fatal("wizard unavailable")
	}
	req := map[string]any{"target": e.Config.Targets[0].ID, "release": "release-1"}
	first := f.call("POST", "/v1/onboarding", req, 201)
	id := first["job"].(map[string]any)["id"].(string)
	if len(d.steps) != 0 || first["job"].(map[string]any)["state"] != "awaiting_approval" {
		t.Fatal("wizard crossed approval boundary")
	}
	files := first["files"].(map[string]any)
	var compose map[string]any
	if json.Unmarshal([]byte(files["compose.json"].(string)), &compose) != nil || strings.Contains(files["compose.json"].(string), "\"build\"") {
		t.Fatal("invalid immutable preview")
	}
	// A lost response and a second click recover the same durable project/plan.
	if f.call("POST", "/v1/onboarding", req, 201)["job"].(map[string]any)["id"] != id {
		t.Fatal("onboarding duplicated")
	}
	req["release"] = "release-2"
	f.call("POST", "/v1/onboarding", req, 409)
	var count int
	if err := f.db.QueryRow(`SELECT count(*) FROM audit_events WHERE action='project.onboarded'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("registration audit duplicated", err)
	}
	j, err := e.Job(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	j = runJob(t, e, approveJob(t, e, j), "succeeded")
	projects := f.call("GET", "/v1/onboarding", nil, 200)["projects"].([]any)
	if len(projects) != 1 || projects[0].(map[string]any)["state"] != "succeeded" {
		t.Fatal("verified first deployment not shown")
	}
	// Observation runs through Driver.Verify and promotion stays in the engine, so
	// the driver sees every other step of deployment.Steps exactly once, in order.
	if want := []string{"preconditions", "configuration", "pull", "start", "health", "traffic"}; !slices.Equal(d.steps, want) {
		t.Fatalf("deployment checks bypassed: %v", d.steps)
	}
	var observed int
	if err := f.db.QueryRow(`SELECT count(*) FROM deployment_events WHERE job_id=$1 AND step='observation' AND state='succeeded'`, id).Scan(&observed); err != nil || observed != 1 {
		t.Fatal("post-deployment observation bypassed", err)
	}
}

func TestPhaseTenOnboardingRejectsUnmappedSource(t *testing.T) {
	f, e, _ := deploymentFixture(t)
	f.dev()
	f.call("POST", "/v1/onboarding", map[string]any{"target": e.Config.Targets[0].ID, "release": "release-1"}, 503)
	f.app.GitHub = &gh.Service{DB: f.db, Config: gh.Config{Repositories: []gh.Repository{{ID: "11", Name: "attacker/repo", Environment: "production", WorkflowID: "70"}}}}
	f.call("POST", "/v1/onboarding", map[string]any{"target": e.Config.Targets[0].ID, "release": "release-1"}, 409)
	var count int
	if err := f.db.QueryRow(`SELECT count(*) FROM deployment_jobs`).Scan(&count); err != nil || count != 0 {
		t.Fatal("unmapped repository planned", err)
	}
}

func TestPhaseTenAdmissionKeepsAuthenticatedOperatorAvailable(t *testing.T) {
	f := newFixture(t)
	f.dev()
	for range 119 {
		f.callAs(f.client, "", "GET", "/v1/me", nil, 401)
	}
	f.callAs(f.client, "", "GET", "/v1/me", nil, 429)
	f.call("GET", "/v1/me", nil, 200)
	f.call("GET", "/healthz", nil, 200)
	// Random bearer strings and forwarded addresses do not obtain new identities.
	r, _ := http.NewRequest("GET", f.server.URL+"/v1/me", nil)
	r.Header.Set("Authorization", "Bearer "+strings.Repeat("f", 64))
	r.Header.Set("X-Forwarded-For", "198.51.100.40")
	res, err := f.client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 429 {
		t.Fatal("unverified identity bypassed limit")
	}
}

func TestPhaseTenNetworkLossAndAgentReconnection(t *testing.T) {
	f := newFixture(t)
	f.dev()
	invite := f.call("POST", "/v1/enrollment-tokens", map[string]any{}, 201)
	path := filepath.Join(f.dir, "invite")
	if err := os.WriteFile(path, []byte(invite["token"].(string)), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := agent.Config{URL: f.server.URL, CAFile: filepath.Join(f.dir, "ca.pem"), TokenFile: path, StateDir: filepath.Join(f.dir, "agent"), Name: "network-fixture"}
	c, err := agent.Connect(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer c.HTTP.CloseIdleConnections()
	if err = c.Heartbeat(context.Background()); err != nil {
		t.Fatal(err)
	}
	var offline atomic.Bool
	transport := c.HTTP.Transport.(*http.Transport)
	dial := (&net.Dialer{Timeout: time.Second}).DialContext
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if offline.Load() {
			return nil, &net.OpError{Op: "dial", Net: network, Err: context.DeadlineExceeded}
		}
		return dial(ctx, network, address)
	}
	// Tear down existing TCP connections before making the network unavailable.
	c.HTTP.CloseIdleConnections()
	f.server.CloseClientConnections()
	offline.Store(true)
	if c.Heartbeat(context.Background()) == nil {
		t.Fatal("outage presented as live")
	}
	var seen time.Time
	if err = f.db.QueryRow(`SELECT last_seen_at FROM agents WHERE id=$1`, c.ID).Scan(&seen); err != nil {
		t.Fatal(err)
	}
	// A restart reuses the persisted certificate; no new enrollment is needed.
	offline.Store(false)
	restarted, err := agent.Connect(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.HTTP.CloseIdleConnections()
	if restarted.ID != c.ID {
		t.Fatal("network loss changed identity")
	}
	if err = restarted.Heartbeat(context.Background()); err != nil {
		t.Fatal(err)
	}
	var after time.Time
	if err = f.db.QueryRow(`SELECT last_seen_at FROM agents WHERE id=$1`, c.ID).Scan(&after); err != nil || after.Before(seen) {
		t.Fatal("observation not recovered", err)
	}
	if _, err = f.db.Exec(`UPDATE agents SET revoked=true WHERE id=$1`, c.ID); err != nil {
		t.Fatal(err)
	}
	if restarted.Heartbeat(context.Background()) == nil {
		t.Fatal("reconnection bypassed revocation")
	}
}

func TestPhaseTenTargetedFleetLoad(t *testing.T) {
	if os.Getenv("PRISME_TEST_LOAD") != "1" {
		t.Skip("set PRISME_TEST_LOAD=1 for targeted load")
	}
	f := newFixture(t)
	f.dev()
	for host := range 50 {
		id := fmt.Sprintf("%064x", host+1)
		snapshot := inventory.Snapshot{Protocol: "1", ObservedAt: time.Now().UTC(), Source: "agent", Host: inventory.Host{Hostname: fmt.Sprintf("fixture-%d", host), CPUs: 4}, DockerAvailable: true}
		for service := range 40 {
			snapshot.Containers = append(snapshot.Containers, inventory.Container{ID: fmt.Sprintf("%064x", host*40+service+1), Name: fmt.Sprintf("service-%d", service), Project: fmt.Sprintf("project-%d", host*4+service/10), Service: fmt.Sprintf("service-%d", service), State: "running", Health: "healthy"})
		}
		snapshot.Normalize()
		raw, _ := json.Marshal(snapshot)
		if _, err := f.db.Exec(`INSERT INTO agents(id,name,csr_hash,certificate_pem,serial,enrolled_at,expires_at,last_seen_at) VALUES($1,$2,'fixture','','fixture-'||$1,now(),now()+interval '1 day',now())`, id, snapshot.Host.Hostname); err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.Exec(`INSERT INTO server_inventory(agent_id,observed_at,document) VALUES($1,now(),$2)`, id, string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	const requests = 120
	durations := make(chan time.Duration, requests)
	errors := make(chan string, requests)
	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Go(func() {
			for i := worker; i < requests; i += 8 {
				r, _ := http.NewRequest("GET", f.server.URL+"/v1/servers", nil)
				r.Header.Set("Authorization", "Bearer "+f.token)
				start := time.Now()
				res, err := f.client.Do(r)
				if err != nil {
					errors <- "transport failure"
					continue
				}
				_, err = io.Copy(io.Discard, res.Body)
				res.Body.Close()
				if err != nil || res.StatusCode != 200 {
					errors <- fmt.Sprintf("HTTP %d", res.StatusCode)
					continue
				}
				durations <- time.Since(start)
			}
		})
	}
	wg.Wait()
	close(durations)
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	values := []time.Duration{}
	for d := range durations {
		values = append(values, d)
	}
	if len(values) != requests {
		t.Fatal("incomplete load results")
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	p95 := values[(len(values)*95+99)/100-1]
	t.Logf("fleet load: 50 hosts / 200 projects / 2000 services; %d TLS requests, concurrency=8, p50=%s p95=%s max=%s", requests, values[len(values)/2].Round(time.Millisecond), p95.Round(time.Millisecond), values[len(values)-1].Round(time.Millisecond))
	if p95 >= 2*time.Second {
		t.Fatal("current-view p95 exceeds two-second objective")
	}
}
