package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"prisme.local/platform/internal/deployment"
	gh "prisme.local/platform/internal/github"
)

type deployDriver struct {
	mu          sync.Mutex
	steps       []string
	fail        string
	verifyError error
	entered     chan bool
	release     chan bool
}

func (d *deployDriver) Execute(ctx context.Context, p deployment.Plan, step string) (map[string]any, error) {
	d.mu.Lock()
	d.steps = append(d.steps, step)
	d.mu.Unlock()
	if step == "start" && d.entered != nil {
		d.entered <- true
		select {
		case <-d.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if step == d.fail {
		if step == "health" {
			return nil, deployment.ErrHealth
		}
		return nil, deployment.ErrUnknown
	}
	return map[string]any{"fixture": true}, nil
}
func (d *deployDriver) Verify(context.Context, deployment.Plan) error { return d.verifyError }
func deploymentFixture(t *testing.T) (*fixture, *deployment.Engine, *deployDriver) {
	f := newFixture(t)
	m := deployment.Manifest{Version: 1, ID: "sounding-lines-local", Project: "sounding-lines", Environment: "test", Server: "isolated-local", RepositoryID: "11", Repository: "fixture/sounding-lines", Branch: "main", Enabled: true, Strategy: "recreate", Migration: "none", ObservationSeconds: 1, TimeoutSeconds: 30, MinimumFreeBytes: 1024 * 1024, Services: []deployment.Service{{Name: "web", Port: 80, BluePort: 18741, HealthPath: "/", Stateless: true}}}
	cfg := deployment.Config{Version: 1, Root: t.TempDir(), Targets: []deployment.Manifest{m}}
	if err := f.app.ConfigureDeployments(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	driver := &deployDriver{}
	engine := f.app.Deployments
	engine.Driver = driver
	_, err := f.db.Exec(`INSERT INTO github_repositories(id,full_name,environment,workflow_id) VALUES('11','fixture/sounding-lines','test','70')`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		build := gh.Build{ID: fmt.Sprint(i), RepositoryID: "11", WorkflowID: "70", Commit: fmt.Sprintf("%040x", i), Branch: "main", Author: "fixture", Status: "completed", Conclusion: "success", Attempt: 1}
		b, _ := json.Marshal(build)
		_, err = f.db.Exec(`INSERT INTO github_builds(repository_id,id,updated_at,document) VALUES('11',$1,now(),$2)`, build.ID, string(b))
		if err != nil {
			t.Fatal(err)
		}
		r := gh.Release{ID: fmt.Sprintf("release-%d", i), RepositoryID: "11", ArtifactID: fmt.Sprint(i), Manifest: gh.Manifest{Version: 1, Repository: m.Repository, Commit: build.Commit, RunID: build.ID, RunAttempt: 1, Environment: m.Environment, Images: []gh.Image{{Service: "web", Reference: fmt.Sprintf("ghcr.io/fixture/web@sha256:%064x", i)}}}}
		b, _ = json.Marshal(r)
		_, err = f.db.Exec(`INSERT INTO github_releases(id,repository_id,run_id,run_attempt,artifact_id,document) VALUES($1,'11',$2,1,$2,$3)`, r.ID, build.ID, string(b))
		if err != nil {
			t.Fatal(err)
		}
	}
	return f, engine, driver
}
func prepareJob(t *testing.T, e *deployment.Engine, release, key string, rollback bool) deployment.Job {
	t.Helper()
	j, err := e.Prepare(context.Background(), deployment.Request{Target: e.Config.Targets[0].ID, Release: release, Rollback: rollback}, key)
	if err != nil {
		t.Fatal(err)
	}
	return j
}
func approveJob(t *testing.T, e *deployment.Engine, j deployment.Job) deployment.Job {
	t.Helper()
	j, err := e.Decide(context.Background(), j.ID, "approve", j.TargetID, "fixture-user", j.Revision)
	if err != nil {
		t.Fatal(err)
	}
	return j
}
func runJob(t *testing.T, e *deployment.Engine, j deployment.Job, state string) deployment.Job {
	t.Helper()
	if err := e.RunTarget(context.Background(), j.TargetID); err != nil {
		t.Fatal(err)
	}
	j, err := e.Job(context.Background(), j.ID)
	if err != nil || j.State != state {
		t.Fatalf("job: %s / %v, want %s", j.State, err, state)
	}
	return j
}

func TestPhaseFourApprovalAuditAndIdempotency(t *testing.T) {
	f, e, d := deploymentFixture(t)
	f.call("GET", "/v1/deployments", nil, 401)
	f.dev()
	req := map[string]any{"target": e.Config.Targets[0].ID, "release": "release-1", "rollback": false}
	out := f.call("POST", "/v1/deployments", req, 201)
	id := out["job"].(map[string]any)["id"].(string)
	if f.call("POST", "/v1/deployments", req, 201)["job"].(map[string]any)["id"] != id {
		t.Fatal("request duplicated")
	}
	req["release"] = "release-2"
	f.call("POST", "/v1/deployments", req, 409)
	if len(d.steps) != 0 {
		t.Fatal("planning executed runtime")
	}
	decision := map[string]any{"action": "approve", "revision": 1, "phrase": "wrong"}
	path := "/v1/deployments/" + id + "/decision"
	f.call("POST", path, decision, 409)
	decision["phrase"] = e.Config.Targets[0].ID
	f.call("POST", path, decision, 200)
	f.call("POST", path, decision, 200)
	j, _ := e.Job(context.Background(), id)
	runJob(t, e, j, "succeeded")
	detail := f.call("GET", "/v1/deployments/"+id, nil, 200)
	if len(detail["events"].([]any)) < 14 {
		t.Fatal("step proofs absent")
	}
	var audit int
	if err := f.db.QueryRow(`SELECT count(*) FROM audit_events WHERE target=$1 AND action IN ('deployment.planned','deployment.approve','deployment.succeeded')`, id).Scan(&audit); err != nil || audit != 3 {
		t.Fatalf("audit=%d %v", audit, err)
	}
}
func TestPhaseFourDeployRestoreAndHealthFailure(t *testing.T) {
	_, e, d := deploymentFixture(t)
	first := runJob(t, e, approveJob(t, e, prepareJob(t, e, "release-1", "first", false)), "succeeded")
	second := runJob(t, e, approveJob(t, e, prepareJob(t, e, "release-2", "second", false)), "succeeded")
	restore := prepareJob(t, e, "release-1", "rollback", true)
	if restore.Plan.PreviousRelease != second.Plan.Release.ID || restore.Plan.Images["web"] != first.Plan.Images["web"] {
		t.Fatal("rollback lost immutable image")
	}
	runJob(t, e, approveJob(t, e, restore), "succeeded")
	d.fail = "health"
	bad := runJob(t, e, approveJob(t, e, prepareJob(t, e, "release-3", "bad-health", false)), "failed")
	targets, _ := e.Targets(context.Background())
	if targets[0].CurrentJob == bad.ID {
		t.Fatal("failed release promoted")
	}
	d.fail = ""
	runJob(t, e, approveJob(t, e, prepareJob(t, e, "release-1", "restore-after-failure", true)), "succeeded")
}
func TestPhaseFourConflictingApprovalExpiryAndCancellation(t *testing.T) {
	f, e, _ := deploymentFixture(t)
	a := prepareJob(t, e, "release-1", "a", false)
	b := prepareJob(t, e, "release-2", "b", false)
	approveJob(t, e, a)
	if _, err := e.Decide(context.Background(), b.ID, "approve", b.TargetID, "fixture", b.Revision); !errors.Is(err, deployment.ErrConflict) {
		t.Fatal("parallel target approval accepted")
	}
	if _, err := e.Decide(context.Background(), a.ID, "cancel", "", "fixture", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE deployment_jobs SET expires_at=now()-interval '1 second' WHERE id=$1`, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Decide(context.Background(), b.ID, "approve", b.TargetID, "fixture", b.Revision); !errors.Is(err, deployment.ErrConflict) {
		t.Fatal("expired approval accepted")
	}
	c := approveJob(t, e, prepareJob(t, e, "release-3", "c", false))
	if _, err := f.db.Exec(`UPDATE deployment_jobs SET expires_at=now()-interval '1 second' WHERE id=$1`, c.ID); err != nil {
		t.Fatal(err)
	}
	runJob(t, e, c, "failed")
}
func TestPhaseFourRestartNeverReplaysMutation(t *testing.T) {
	_, e, d := deploymentFixture(t)
	j := approveJob(t, e, prepareJob(t, e, "release-1", "restart", false))
	d.fail = "start"
	d.verifyError = deployment.ErrHealth
	j = runJob(t, e, j, "uncertain")
	steps := len(d.steps)
	runJob(t, e, j, "uncertain")
	if len(d.steps) != steps {
		t.Fatal("uncertain mutation replayed")
	}
	next := prepareJob(t, e, "release-2", "next", false)
	if _, err := e.Decide(context.Background(), next.ID, "approve", next.TargetID, "fixture", next.Revision); err == nil {
		t.Fatal("uncertain target not fenced")
	}
	// A restarted engine can complete only after read-only evidence is available.
	restarted := &deployment.Engine{DB: e.DB, Config: e.Config, Driver: d}
	d.verifyError = nil
	runJob(t, restarted, j, "succeeded")
	if len(d.steps) != steps {
		t.Fatal("reconciliation executed mutations")
	}
}
func TestPhaseFourWorkersSerializeAndRejectRunningCancel(t *testing.T) {
	_, e, d := deploymentFixture(t)
	d.entered = make(chan bool, 1)
	d.release = make(chan bool)
	j := approveJob(t, e, prepareJob(t, e, "release-1", "serial", false))
	done := make(chan error, 1)
	go func() { done <- e.RunTarget(context.Background(), j.TargetID) }()
	select {
	case <-d.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not start")
	}
	if err := e.RunTarget(context.Background(), j.TargetID); err != nil {
		t.Fatal(err)
	}
	current, _ := e.Job(context.Background(), j.ID)
	if _, err := e.Decide(context.Background(), j.ID, "cancel", "", "fixture", current.Revision); err == nil {
		t.Fatal("unsafe cancellation accepted")
	}
	close(d.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for _, s := range d.steps {
		if s == "start" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d starts", n)
	}
}
func TestPhaseFourRejectsUnhealthyRollbackAndManifestDrift(t *testing.T) {
	_, e, _ := deploymentFixture(t)
	if _, err := e.Prepare(context.Background(), deployment.Request{Target: e.Config.Targets[0].ID, Release: "release-2", Rollback: true}, "unhealthy"); err == nil {
		t.Fatal("unproven rollback accepted")
	}
	j := prepareJob(t, e, "release-1", "drift", false)
	e.Config.Targets[0].Migration = "incompatible"
	if _, err := e.Decide(context.Background(), j.ID, "approve", j.TargetID, "fixture", j.Revision); err == nil {
		t.Fatal("manifest drift accepted")
	}
}

func TestPhaseFourDuplicateRequestsDoNotExhaustPool(t *testing.T) {
	_, e, _ := deploymentFixture(t)
	expected := prepareJob(t, e, "release-1", "concurrent-duplicate", false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	results := make(chan error, 16)
	for range 16 {
		go func() {
			j, err := e.Prepare(ctx, deployment.Request{Target: expected.TargetID, Release: "release-1"}, "concurrent-duplicate")
			if err == nil && j.ID != expected.ID {
				err = errors.New("duplicate job")
			}
			results <- err
		}()
	}
	for range 16 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}

func TestPhaseFourProductionApprovalBlocked(t *testing.T) {
	f, e, _ := deploymentFixture(t)
	m := e.Config.Targets[0]
	m.ID = "production-pilot"
	m.Environment = "production"
	if _, err := f.db.Exec(`UPDATE github_releases SET document=jsonb_set(document,'{manifest,environment}','"production"')`); err != nil {
		t.Fatal(err)
	}
	cfg := e.Config
	cfg.Targets = []deployment.Manifest{m}
	if err := f.app.ConfigureDeployments(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	f.dev()
	out := f.call("POST", "/v1/deployments", map[string]any{"target": m.ID, "release": "release-1", "rollback": false}, 201)
	id := out["job"].(map[string]any)["id"].(string)
	denied := f.call("POST", "/v1/deployments/"+id+"/decision", map[string]any{"action": "approve", "phrase": m.ID, "revision": 1}, 403)
	if denied["code"] != "production_step_up_unavailable" {
		t.Fatal("wrong production guard")
	}
}
