package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"prisme.local/platform/internal/automation"
	"prisme.local/platform/internal/notifications"
	"prisme.local/platform/internal/observability"
)

type automationDriver struct {
	calls     []string
	healthy   bool
	uncertain bool
	after     func()
	entered   chan struct{}
}

func (d *automationDriver) Check(context.Context, automation.Target) error {
	if d.healthy {
		return nil
	}
	return automation.ErrHealth
}
func (d *automationDriver) Execute(ctx context.Context, _ automation.Target, s automation.Step, _ string) (map[string]any, error) {
	d.calls = append(d.calls, s.Action)
	if d.entered != nil {
		close(d.entered)
		<-ctx.Done()
		return nil, automation.ErrUncertain
	}
	if d.after != nil {
		d.after()
	}
	if d.uncertain {
		return nil, automation.ErrUncertain
	}
	return map[string]any{"completed": true}, nil
}

func TestPhaseNineGlobalStopInterruptsInflightMutation(t *testing.T) {
	f, key, e, d, now := automationFixture(t)
	automationPublish(t, f, key, "policy", defaultPolicy("autonomous"))
	automationStart(t, f, key, e)
	automationSignal(t, e, *now, "failing", false)
	d.entered = make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- e.Tick(context.Background()) }()
	select {
	case <-d.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("worker never entered mutation")
	}
	c, _ := e.Control(context.Background())
	c.Stopped = true
	if _, err := e.SetControl(context.Background(), c, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("global stop did not cancel worker")
	}
	r := automationRuns(t, e)[0]
	if r.State != "uncertain" || len(d.calls) != 1 {
		t.Fatal(r.State, d.calls)
	}
	if err := e.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(d.calls) != 1 {
		t.Fatal("cancelled mutation replayed")
	}
}

func TestPhaseNineCircuitNotificationIsDeduplicated(t *testing.T) {
	f, key, e, _, now := automationFixture(t)
	automationPublish(t, f, key, "policy", defaultPolicy("autonomous"))
	automationStart(t, f, key, e)
	automationSignal(t, e, *now, "failing", false)
	n := &notifications.Engine{DB: f.db, Now: e.Now, Config: notifications.Config{Rules: []notifications.RulePolicy{{RuleID: "pilot-health", Urgent: true}}}}
	if err := n.Schedule(context.Background()); err != nil {
		t.Fatal(err)
	}
	var before, after string
	_ = f.db.QueryRow(`SELECT state_key FROM notification_incidents`).Scan(&before)
	automationTicks(t, e, now, 20)
	if err := n.Schedule(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = f.db.QueryRow(`SELECT state_key FROM notification_incidents`).Scan(&after)
	if before == after {
		t.Fatal("circuit did not notify")
	}
	if err := n.Schedule(context.Background()); err != nil {
		t.Fatal(err)
	}
	var again string
	_ = f.db.QueryRow(`SELECT state_key FROM notification_incidents`).Scan(&again)
	if again != after {
		t.Fatal("circuit notification repeated")
	}
}

func TestPhaseNineClientTimestampProofBinding(t *testing.T) {
	f, key, e, _, now := automationFixture(t)
	c, _ := e.Control(context.Background())
	value := map[string]any{"stopped": false, "revision": c.Revision, "maintenance_until": now.Add(time.Hour).Truncate(time.Second).Format("2006-01-02T15:04:05.000000Z")}
	proof := operationProof(t, f, key, map[string]any{"action": "automation_control", "target": "global", "payload": value})
	f.call("POST", "/v1/automation/control", map[string]any{"control": value, "proof": proof}, 200)
}

func TestPhaseNineQueuedRollbackCannotEscapeStop(t *testing.T) {
	f, e, d := deploymentFixture(t)
	ctx := context.Background()
	runJob(t, e, approveJob(t, e, prepareJob(t, e, "release-1", "healthy-before-auto", false)), "succeeded")
	runJob(t, e, approveJob(t, e, prepareJob(t, e, "release-2", "current-before-auto", false)), "succeeded")
	j := approveJob(t, e, prepareJob(t, e, "release-1", "auto-stop-rollback", true))
	r := automation.Run{ID: fmt.Sprintf("%064x", 99), Revision: 1, Target: j.TargetID, State: "running"}
	b, _ := json.Marshal(r)
	if _, err := f.db.Exec(`INSERT INTO automation_runs(id,request_key,request_hash,target,state,document) VALUES($1,'rollback-stop','hash',$2,'running',$3)`, r.ID, r.Target, string(b)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE deployment_jobs SET automation_run_id=$2 WHERE id=$1`, j.ID, r.ID); err != nil {
		t.Fatal(err)
	}
	before := len(d.steps)
	if err := e.RunTarget(ctx, j.TargetID); err != nil {
		t.Fatal(err)
	}
	j, _ = e.Job(ctx, j.ID)
	if j.State != "cancelled" || len(d.steps) != before {
		t.Fatal("queued rollback escaped global stop", j.State)
	}
	if _, err := f.db.Exec(`UPDATE deployment_jobs SET state='uncertain' WHERE id=$1`, j.ID); err != nil {
		t.Fatal(err)
	}
	native := &automation.Native{Deployments: e}
	// No coordination root means the native reconciler must fail closed.
	if err := native.Reconcile(ctx, automation.Target{Server: "isolated-local", Deployment: j.TargetID}, r.ID); err == nil {
		t.Fatal("reconciled without a host guard")
	}
	if err := e.ReconcileAutomation(ctx, j.TargetID, r.ID); err != nil {
		t.Fatal(err)
	}
	j, _ = e.Job(ctx, j.ID)
	if j.State != "failed" || len(d.steps) != before {
		t.Fatal("reconciliation replayed or promoted")
	}
}
func automationFixture(t *testing.T) (*fixture, *virtualKey, *automation.Engine, *automationDriver, *time.Time) {
	f, key := operationsFixture(t)
	c := automation.Config{Version: 1, Targets: []automation.Target{{ID: "pilot", Deployment: "pilot-test", Server: "local", Project: "pilot", Environment: "test", Service: "web", Rule: "pilot-health"}}}
	if err := f.app.ConfigureAutomation(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	e := f.app.Automation
	e.Now = func() time.Time { return now }
	d := &automationDriver{}
	e.Driver = d
	return f, key, e, d, &now
}
func defaultPolicy(mode string) automation.Policy {
	return automation.Policy{ID: "repair", Version: 1, Target: "pilot", Mode: mode, Failures: 3, WindowSeconds: 60, FreshnessSeconds: 30, Actions: []string{"restart", "rollback"}, MaxAttempts: 2, BackoffSeconds: 1, ObservationSeconds: 2, TimeoutSeconds: 300, CooldownSeconds: 60, HealthyRelease: "healthy-release", Migration: "none", Notify: true}
}
func automationPublish(t *testing.T, f *fixture, key *virtualKey, kind string, doc any) {
	t.Helper()
	b, _ := json.Marshal(doc)
	var v map[string]any
	_ = json.Unmarshal(b, &v)
	proof := operationProof(t, f, key, map[string]any{"action": "automation_publish", "target": v["id"], "payload": map[string]any{"kind": kind, "document": v}})
	f.call("POST", "/v1/automation/definitions", map[string]any{"kind": kind, "document": v, "proof": proof}, 201)
}
func automationStart(t *testing.T, f *fixture, key *virtualKey, e *automation.Engine) {
	t.Helper()
	c, err := e.Control(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	c.Stopped = false
	b, _ := json.Marshal(c)
	var v map[string]any
	_ = json.Unmarshal(b, &v)
	proof := operationProof(t, f, key, map[string]any{"action": "automation_control", "target": "global", "payload": v})
	f.call("POST", "/v1/automation/control", map[string]any{"control": v, "proof": proof}, 200)
}
func automationSignal(t *testing.T, e *automation.Engine, now time.Time, signal string, stale bool) {
	t.Helper()
	ctx := context.Background()
	var incident int64
	if err := e.DB.QueryRowContext(ctx, `INSERT INTO incidents(rule_id,scope_id,title,severity,status,opened_at,updated_at) VALUES('pilot-health','pilot','Pilot health','critical','detected',$1,$1) ON CONFLICT(rule_id) WHERE status NOT IN ('resolved','closed') DO UPDATE SET updated_at=$1 RETURNING id`, now).Scan(&incident); err != nil {
		t.Fatal(err)
	}
	since := now.Add(-5 * time.Second)
	sample := now
	if stale {
		sample = now.Add(-time.Hour)
	}
	s := observability.RuleState{Signal: signal, FailSamples: 3, PendingSince: &since, LastSample: &sample, IncidentID: incident, Scope: &observability.Scope{ID: "pilot", Server: e.Config.Targets[0].Server, Project: "pilot", Environment: "test", Service: "web"}}
	b, _ := json.Marshal(s)
	if _, err := e.DB.ExecContext(ctx, `INSERT INTO observation_rule_state(rule_id,document,updated_at) VALUES('pilot-health',$1,$2) ON CONFLICT(rule_id) DO UPDATE SET document=$1,updated_at=$2`, string(b), now); err != nil {
		t.Fatal(err)
	}
}
func automationTicks(t *testing.T, e *automation.Engine, now *time.Time, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := e.Tick(context.Background()); err != nil {
			t.Fatal(err)
		}
		*now = now.Add(time.Second)
	}
}
func automationRuns(t *testing.T, e *automation.Engine) []automation.Run {
	t.Helper()
	v, err := e.Overview(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	return v["runs"].([]automation.Run)
}
func TestPhaseNineControlledFailureStopsAtExactLimit(t *testing.T) {
	f, key, e, d, now := automationFixture(t)
	automationPublish(t, f, key, "policy", defaultPolicy("autonomous"))
	automationSignal(t, e, *now, "failing", false)
	automationTicks(t, e, now, 2)
	if len(d.calls) != 0 || len(automationRuns(t, e)) != 0 {
		t.Fatal("default stop did not inhibit automation")
	}
	automationStart(t, f, key, e)
	automationTicks(t, e, now, 20)
	if !reflect.DeepEqual(d.calls, []string{"restart", "rollback"}) {
		t.Fatal(d.calls)
	}
	runs := automationRuns(t, e)
	if len(runs) != 1 || runs[0].State != "failed" || runs[0].Attempts != 2 {
		t.Fatal(runs)
	}
	var breaker bool
	if err := e.DB.QueryRow(`SELECT breaker FROM automation_policy_state WHERE id='repair'`).Scan(&breaker); err != nil || !breaker {
		t.Fatal("breaker missing", err)
	}
	events, err := e.Events(context.Background(), runs[0].ID)
	if err != nil || len(events) < 12 {
		t.Fatal("incomplete history", events, err)
	}
	var correlated int
	_ = e.DB.QueryRow(`SELECT count(*) FROM incident_events WHERE incident_id=$1 AND kind='automation'`, runs[0].Incident).Scan(&correlated)
	if correlated != len(events) {
		t.Fatal("uncorrelated timeline", correlated, len(events))
	}
	automationTicks(t, e, now, 100)
	if len(d.calls) != 2 {
		t.Fatal("unbounded retry")
	}
	if err = e.Reset(context.Background(), "repair", func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	automationSignal(t, e, *now, "failing", false)
	automationTicks(t, e, now, 10)
	if len(d.calls) != 2 {
		t.Fatal("breaker reset replayed same outage")
	}
}
func TestPhaseNineRecoveryAvoidsRollback(t *testing.T) {
	f, key, e, d, now := automationFixture(t)
	automationPublish(t, f, key, "policy", defaultPolicy("autonomous"))
	automationStart(t, f, key, e)
	automationSignal(t, e, *now, "failing", false)
	d.after = func() { d.healthy = true }
	automationTicks(t, e, now, 25)
	runs := automationRuns(t, e)
	if !reflect.DeepEqual(d.calls, []string{"restart"}) || len(runs) != 1 || runs[0].State != "succeeded" {
		t.Fatal(d.calls, runs)
	}
}
func TestPhaseNineModesAndBoundProofs(t *testing.T) {
	for _, mode := range []string{"supervised", "assisted"} {
		t.Run(mode, func(t *testing.T) {
			f, key, e, d, now := automationFixture(t)
			automationPublish(t, f, key, "policy", defaultPolicy(mode))
			automationStart(t, f, key, e)
			automationSignal(t, e, *now, "failing", false)
			automationTicks(t, e, now, 2)
			runs := automationRuns(t, e)
			if len(runs) != 1 || len(d.calls) != 0 {
				t.Fatal(runs, d.calls)
			}
			r := runs[0]
			if mode == "supervised" {
				if r.State != "observed" {
					t.Fatal(r.State)
				}
				return
			}
			if r.State != "approval" {
				t.Fatal(r.State)
			}
			decision := map[string]any{"action": "approve", "revision": r.Revision, "proof": ""}
			f.call("POST", "/v1/automation/runs/"+r.ID+"/decision", decision, 403)
			decision["proof"] = operationProof(t, f, key, map[string]any{"action": "automation_approve", "target": r.ID, "payload": map[string]any{"revision": r.Revision}})
			decision["revision"] = r.Revision + 1
			f.call("POST", "/v1/automation/runs/"+r.ID+"/decision", decision, 409)
			decision["revision"] = r.Revision
			f.call("POST", "/v1/automation/runs/"+r.ID+"/decision", decision, 200)
			automationTicks(t, e, now, 10)
			if !reflect.DeepEqual(d.calls, []string{"restart"}) {
				t.Fatal("approval authorized more than one action", d.calls)
			}
			r, _ = e.RunByID(context.Background(), r.ID)
			if r.State != "approval" {
				t.Fatal(r.State)
			}
		})
	}
}
func TestPhaseNineInhibitionAndVersionRevocation(t *testing.T) {
	f, key, e, d, now := automationFixture(t)
	p := defaultPolicy("autonomous")
	p.Dependencies = []string{"database-health"}
	automationPublish(t, f, key, "policy", p)
	automationStart(t, f, key, e)
	automationSignal(t, e, *now, "failing", false)
	automationTicks(t, e, now, 2)
	if len(automationRuns(t, e)) != 0 {
		t.Fatal("missing dependency authorized action")
	}
	p.Version++
	p.Dependencies = nil
	automationPublish(t, f, key, "policy", p)
	automationSignal(t, e, *now, "failing", true)
	automationTicks(t, e, now, 2)
	if len(d.calls) != 0 {
		t.Fatal("stale evidence authorized action")
	}
	p.Version++
	until := now.Add(time.Hour)
	p.MaintenanceUntil = &until
	automationPublish(t, f, key, "policy", p)
	automationSignal(t, e, *now, "failing", false)
	automationTicks(t, e, now, 2)
	if len(d.calls) != 0 {
		t.Fatal("maintenance ignored")
	}
	p.Version++
	p.MaintenanceUntil = nil
	p.Mode = "assisted"
	automationPublish(t, f, key, "policy", p)
	automationTicks(t, e, now, 2)
	r := automationRuns(t, e)[0]
	p.Version++
	p.Mode = "supervised"
	automationPublish(t, f, key, "policy", p)
	_, err := e.Decide(context.Background(), r.ID, "approve", r.Revision, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	automationTicks(t, e, now, 3)
	if len(d.calls) != 0 {
		t.Fatal("old policy executed")
	}
}
func TestPhaseNineInterruptedMutationNeverReplays(t *testing.T) {
	f, key, e, d, now := automationFixture(t)
	automationPublish(t, f, key, "policy", defaultPolicy("autonomous"))
	automationStart(t, f, key, e)
	automationSignal(t, e, *now, "failing", false)
	d.uncertain = true
	automationTicks(t, e, now, 5)
	r := automationRuns(t, e)[0]
	if r.State != "uncertain" || len(d.calls) != 1 {
		t.Fatal(r, d.calls)
	}
	automationTicks(t, e, now, 10)
	if len(d.calls) != 1 {
		t.Fatal("uncertain mutation replayed")
	}
}
func TestPhaseNineRunbooksSchedulesAndStop(t *testing.T) {
	f, key, e, d, now := automationFixture(t)
	ctx := context.Background()
	b := automation.Runbook{ID: "inspect", Version: 1, Target: "pilot", TimeoutSeconds: 300, Parameters: map[string]automation.Parameter{"pause": {Type: "integer", Min: 1, Max: 10}}, Steps: []automation.Step{{Action: "wait", When: "always", Parameter: "pause", TimeoutSeconds: 10}, {Action: "restart", When: "always", TimeoutSeconds: 10}, {Action: "check", When: "always", TimeoutSeconds: 10}}}
	automationPublish(t, f, key, "runbook", b)
	req := automation.Request{Runbook: "inspect", Version: 1, Parameters: map[string]any{"pause": float64(2)}, Simulation: true}
	r, err := e.Prepare(ctx, req, "simulation-request")
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := e.Prepare(ctx, req, "simulation-request")
	if err != nil || r.ID != duplicate.ID {
		t.Fatal("idempotency lost")
	}
	changed := req
	changed.Version = 2
	if _, err = e.Prepare(ctx, changed, "simulation-request"); err != automation.ErrConflict {
		t.Fatal(err)
	}
	automationStart(t, f, key, e)
	automationTicks(t, e, now, 5)
	r, _ = e.RunByID(ctx, r.ID)
	if r.State != "simulated" || len(d.calls) != 0 {
		t.Fatal(r.State, d.calls)
	}
	b.Version = 2
	b.Steps[0].Seconds = 10
	automationPublish(t, f, key, "runbook", b)
	s := automation.Schedule{ID: "hourly", Revision: 1, Request: req, EverySeconds: 60, NextAt: now.Add(time.Second), Enabled: true}
	if err = e.Schedule(ctx, s, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(3 * time.Hour)
	automationTicks(t, e, now, 1)
	runs := automationRuns(t, e)
	if len(runs) != 2 || runs[0].Book.Version != 1 {
		t.Fatal("schedule lost pinned version or replayed backlog", runs)
	}
	automationTicks(t, e, now, 5)
	if len(automationRuns(t, e)) != 2 {
		t.Fatal("schedule duplicated")
	}
	req.Simulation = false
	r, err = e.Prepare(ctx, req, "manual-pending")
	if err != nil {
		t.Fatal(err)
	}
	c, _ := e.Control(ctx)
	c.Stopped = true
	f.call("POST", "/v1/automation/control", map[string]any{"control": c, "proof": ""}, 200)
	r, _ = e.RunByID(ctx, r.ID)
	if r.State != "cancelled" {
		t.Fatal("stop did not cancel pending", r.State)
	}
	f.call("POST", "/v1/automation/control", map[string]any{"control": c, "proof": ""}, 409)
}
func TestPhaseNineValidation(t *testing.T) {
	_, _, e, _, _ := automationFixture(t)
	for _, change := range []func(*automation.Policy){func(p *automation.Policy) { p.MaxAttempts = 100 }, func(p *automation.Policy) { p.Actions = []string{"shell"} }, func(p *automation.Policy) { p.Migration = "destructive" }, func(p *automation.Policy) { p.HealthyRelease = "" }, func(p *automation.Policy) { p.Notify = false }} {
		p := defaultPolicy("autonomous")
		change(&p)
		if p.Validate(e.Config) == nil {
			t.Fatal(fmt.Sprintf("unsafe policy accepted: %+v", p))
		}
	}
}
