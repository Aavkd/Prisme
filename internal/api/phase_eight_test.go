package api_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"prisme.local/platform/internal/coordination"
	"prisme.local/platform/internal/edge"
)

type edgeDriver struct {
	calls             int
	outcome           string
	invalid           bool
	observationFailed bool
}

func (d *edgeDriver) Observe(context.Context, edge.Config, edge.Target) (map[string]any, error) {
	if d.observationFailed {
		return nil, edge.ErrUnavailable
	}
	return map[string]any{"drift": false}, nil
}

func TestPhaseEightObservationScopeAndFreshness(t *testing.T) {
	_, _, e, d := edgeFixture(t)
	ctx := context.Background()
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := e.DB.Exec(`UPDATE edge_observations SET observed_at=now()-interval '5 minutes' WHERE id='pilot'`); err != nil {
		t.Fatal(err)
	}
	d.observationFailed = true
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	observations, err := e.Observations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range observations {
		v := raw.(map[string]any)
		if v["id"] == "pilot" && (v["stale"] != true || v["error_code"] != "observation_failed") {
			t.Fatal("failed collection refreshed old state")
		}
	}
	e.Config.Targets[0].Domain = "changed.test"
	observations, err = e.Observations(ctx)
	if err != nil || len(observations) != 0 {
		t.Fatal("old configuration inventory remained visible")
	}
}
func (*edgeDriver) Discover(context.Context, edge.Config) (map[string]any, error) {
	return map[string]any{"zones": []any{}}, nil
}
func (d *edgeDriver) Prepare(_ context.Context, _ edge.Config, _ edge.Target, r edge.Request, _ *edge.Snapshot) (edge.Snapshot, edge.Plan, error) {
	if d.invalid {
		return edge.Snapshot{}, edge.Plan{}, edge.ErrInvalid
	}
	return edge.Snapshot{Before: edge.State{Managed: "private-snapshot"}, After: edge.State{Managed: r.Value}}, edge.Plan{Before: "old", After: r.Value, Checks: []edge.Check{}}, nil
}
func (d *edgeDriver) Apply(context.Context, edge.Config, edge.Target, edge.Snapshot) (string, map[string]any, error) {
	d.calls++
	if d.outcome != "" {
		return d.outcome, map[string]any{}, edge.ErrUncertain
	}
	return "succeeded", map[string]any{"verified": true}, nil
}
func edgeFixture(t *testing.T) (*fixture, *virtualKey, *edge.Engine, *edgeDriver) {
	f, key := operationsFixture(t)
	c := edge.Config{Version: 1, CoordinationRoot: filepath.Join(f.dir, "guard"), Targets: []edge.Target{{ID: "pilot", Kind: "caddy", Server: "shared-edge", Environment: "test", Domain: "pilot.test", WriteEnabled: true, Caddy: &edge.Caddy{Binary: filepath.Join(f.dir, "caddy"), AdminURL: "http://127.0.0.1:2019", SourceFile: filepath.Join(f.dir, "Caddyfile"), ManagedFile: filepath.Join(f.dir, "pilot.caddy"), Upstreams: []string{"app:8080"}}, Checks: []edge.Check{{URL: "http://127.0.0.1:8080", Host: "pilot.test", Status: 200}}}}}
	if err := f.app.ConfigureEdge(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	e := f.app.Edge
	d := &edgeDriver{}
	e.Driver = d
	return f, key, e, d
}
func edgeJob(t *testing.T, v map[string]any) edge.Job {
	t.Helper()
	b, _ := json.Marshal(v["job"])
	var j edge.Job
	if err := json.Unmarshal(b, &j); err != nil {
		t.Fatal(err)
	}
	return j
}
func TestPhaseEightPlansProofAuditAndRollback(t *testing.T) {
	f, key, e, d := edgeFixture(t)
	ctx := context.Background()
	r := map[string]any{"target": "pilot", "value": "app:8080", "rollback": ""}
	j := edgeJob(t, f.call("POST", "/v1/edge/jobs", r, 201))
	duplicate := edgeJob(t, f.call("POST", "/v1/edge/jobs", r, 201))
	if j.ID != duplicate.ID {
		t.Fatal("request replay lost plan")
	}
	if j.State != "preparing" || d.calls != 0 {
		t.Fatal("unvalidated mutation")
	}
	f.call("POST", "/v1/edge/jobs/"+j.ID+"/decision", map[string]any{"action": "approve", "revision": j.Revision, "phrase": edge.Confirmation(j), "proof": ""}, 409)
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	j, _ = e.Job(ctx, j.ID)
	if j.State != "planned" {
		t.Fatal(j.State)
	}
	data := f.call("GET", "/v1/edge/jobs/"+j.ID, nil, 200)
	raw, _ := json.Marshal(data)
	if strings.Contains(string(raw), "private-snapshot") {
		t.Fatal("snapshot leaked")
	}
	decision := map[string]any{"action": "approve", "revision": j.Revision, "phrase": edge.Confirmation(j), "proof": ""}
	f.call("POST", "/v1/edge/jobs/"+j.ID+"/decision", decision, 403)
	decision["proof"] = operationProof(t, f, key, map[string]any{"action": "edge_approve", "target": j.ID, "payload": map[string]any{"revision": j.Revision}})
	decision["revision"] = j.Revision + 1
	f.call("POST", "/v1/edge/jobs/"+j.ID+"/decision", decision, 409)
	decision["revision"] = j.Revision
	f.call("POST", "/v1/edge/jobs/"+j.ID+"/decision", decision, 200)
	f.call("POST", "/v1/edge/jobs/"+j.ID+"/decision", decision, 409)
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	j, _ = e.Job(ctx, j.ID)
	if j.State != "succeeded" || d.calls != 1 {
		t.Fatalf("unexpected result %s / %d", j.State, d.calls)
	}
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if d.calls != 1 {
		t.Fatal("write replayed")
	}
	obs := f.call("GET", "/v1/edge", nil, 200)
	if len(obs["observations"].([]any)) != 2 {
		t.Fatal("missing inventory")
	}
	var n int
	if err := f.db.QueryRow(`SELECT count(*) FROM audit_events WHERE target=$1 AND action LIKE 'edge.%'`, j.ID).Scan(&n); err != nil || n < 5 {
		t.Fatal("missing audit")
	}
	restored, err := e.Prepare(ctx, edge.Request{Target: "pilot", Rollback: j.ID}, "rollback-edge-123456")
	if err != nil {
		t.Fatal(err)
	}
	if restored.State != "preparing" {
		t.Fatal("rollback bypassed validation")
	}
}
func TestPhaseEightUncertaintyBlocksHostAndNoReplay(t *testing.T) {
	_, _, e, d := edgeFixture(t)
	ctx := context.Background()
	j, err := e.Prepare(ctx, edge.Request{Target: "pilot", Value: "app:8080"}, "edge-uncertainty-123")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	j, _ = e.Job(ctx, j.ID)
	if _, err = e.Decide(ctx, j.ID, "approve", edge.Confirmation(j), j.Revision, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	d.outcome = "uncertain"
	if err = e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	j, _ = e.Job(ctx, j.ID)
	if j.State != "uncertain" {
		t.Fatal(j.State)
	}
	if err = e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if d.calls != 1 {
		t.Fatal("uncertain write replay")
	}
	state, err := coordination.Read(e.Config.CoordinationRoot, "shared-edge")
	if err != nil || !state.Active {
		t.Fatal("host guard released ambiguity")
	}
	if e.Acknowledge(ctx, j.ID) == nil {
		t.Fatal("unreconciled host acknowledged")
	}
	if err = coordination.Reconcile(e.Config.CoordinationRoot, "shared-edge", "test-operator", state.Epoch); err != nil {
		t.Fatal(err)
	}
	if err = e.Acknowledge(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	inverse, err := e.Prepare(ctx, edge.Request{Target: "pilot", Rollback: j.ID}, "reconciled-inverse-123456")
	if err != nil || inverse.State != "preparing" {
		t.Fatal("reconciled inverse unavailable", err)
	}
	if d.calls != 1 {
		t.Fatal("reconciliation replayed mutation")
	}
}
func TestPhaseEightCrashExpiryReadonlyAndInvalidPlan(t *testing.T) {
	_, _, e, d := edgeFixture(t)
	ctx := context.Background()
	j, err := e.Prepare(ctx, edge.Request{Target: "pilot", Value: "app:8080"}, "edge-crash-12345678")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.DB.Exec(`UPDATE edge_jobs SET state='running' WHERE id=$1`, j.ID); err != nil {
		t.Fatal(err)
	}
	if err = e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	j, _ = e.Job(ctx, j.ID)
	if j.State != "uncertain" || d.calls != 0 {
		t.Fatal("crash replayed")
	}
	e.Config.Targets[0].WriteEnabled = false
	if _, err = e.Prepare(ctx, edge.Request{Target: "pilot", Value: "app:8080"}, "edge-readonly-123456"); err == nil {
		t.Fatal("readonly prepared write")
	}
	e.Config.Targets[0].WriteEnabled = true
	d.invalid = true
	j, err = e.Prepare(ctx, edge.Request{Target: "pilot", Value: "app:8080"}, "edge-invalid-123456")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	j, _ = e.Job(ctx, j.ID)
	if j.State != "failed" {
		t.Fatal("invalid plan approved")
	}
}
