package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"prisme.local/platform/internal/backup"
	"prisme.local/platform/internal/coordination"
	"prisme.local/platform/internal/deployment"
)

type backupDriver struct {
	mu               sync.Mutex
	calls            int
	fail             bool
	entered, release chan bool
}

func (d *backupDriver) Execute(ctx context.Context, c backup.Config, p backup.Policy, j backup.Job, a *backup.Artifact, step func(string) error) (backup.Result, error) {
	d.mu.Lock()
	d.calls++
	calls := d.calls
	d.mu.Unlock()
	if d.entered != nil {
		d.entered <- true
		select {
		case <-d.release:
		case <-ctx.Done():
			return backup.Result{}, ctx.Err()
		}
	}
	if err := step("integrity"); err != nil {
		return backup.Result{}, err
	}
	if d.fail {
		return backup.Result{}, backup.ErrCommand
	}
	result := backup.Result{Snapshot: strings.Repeat("a", 63) + string("0123456789abcdef"[calls%16]), Bytes: 4096, Evidence: map[string]any{"validation": "passed"}}
	result.Retained = j.ProtectedSnapshots
	if j.Kind == "restore" && a == nil {
		return result, backup.ErrInvalid
	}
	return result, nil
}
func backupFixture(t *testing.T) (*fixture, *virtualKey, *backup.Engine, *backupDriver) {
	f, key := operationsFixture(t)
	root := f.dir
	c := backup.Config{Version: 1, WorkRoot: filepath.Join(root, "work"), CoordinationRoot: filepath.Join(root, "guard"), Repositories: []backup.Repository{{ID: "repository", LocationFile: filepath.Join(root, "repository-file"), PasswordFile: filepath.Join(root, "password"), StorageHost: "source"}}, Policies: []backup.Policy{{ID: "pilot", Project: "pilot", Environment: "test", Server: "source", Repository: "repository", Kind: "files", Path: filepath.Join(root, "source"), Consistency: "crash", Enabled: true, TimeoutSeconds: 60, Retention: backup.Retention{KeepLast: 2}}}, Targets: []backup.Target{{ID: "test", Policy: "pilot", Server: "restore-host", Kind: "files", Root: filepath.Join(root, "restore"), TestOnly: true}}}
	if err := f.app.ConfigureBackups(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	d := &backupDriver{}
	e := f.app.Backups
	e.Driver = d
	return f, key, e, d
}
func backupJob(t *testing.T, v map[string]any) backup.Job {
	t.Helper()
	var j backup.Job
	b, _ := json.Marshal(v["job"])
	if json.Unmarshal(b, &j) != nil {
		t.Fatal("job response")
	}
	return j
}
func TestPhaseSevenBackupRestoreProofAndRetention(t *testing.T) {
	f, key, e, _ := backupFixture(t)
	ctx := context.Background()
	req := map[string]any{"policy": "pilot", "kind": "backup", "artifact": "", "target": ""}
	j := backupJob(t, f.call("POST", "/v1/backups/jobs", req, 201))
	duplicate := backupJob(t, f.call("POST", "/v1/backups/jobs", req, 201))
	if j.ID != duplicate.ID {
		t.Fatal("idempotency failed")
	}
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	as, err := e.Artifacts(ctx, 1)
	if err != nil || len(as) != 1 || as[0].Bytes != 4096 {
		t.Fatalf("artifact: %v", err)
	}
	policies, err := e.Policies(ctx)
	if err != nil || policies[0]["offsite_verified"] != false {
		t.Fatal("local copy presented as offsite")
	}
	data := f.call("GET", "/v1/backups", nil, 200)
	raw, _ := json.Marshal(data)
	if strings.Contains(string(raw), "password") || strings.Contains(string(raw), "repository-file") {
		t.Fatal("private config in overview")
	}
	r, err := e.Prepare(ctx, backup.Request{Policy: "pilot", Kind: "restore", Artifact: as[0].ID, Target: "test"}, "restore-idempotency-12345")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	current, _ := e.Job(ctx, r.ID)
	if current.State != "planned" {
		t.Fatal("restore ran before approval")
	}
	bound := f.call("GET", "/v1/backups/jobs/"+r.ID, nil, 200)["plan"].(map[string]any)
	if bound["artifact"].(map[string]any)["id"] != as[0].ID || bound["policy"].(map[string]any)["id"] != "pilot" || bound["target"].(map[string]any)["id"] != "test" {
		t.Fatal("restore detail lost bound review context")
	}
	decision := map[string]any{"action": "approve", "revision": r.Revision, "phrase": backup.Confirmation(r), "proof": ""}
	f.call("POST", "/v1/backups/jobs/"+r.ID+"/decision", decision, 403)
	intent := map[string]any{"action": "backup_approve", "target": r.ID, "payload": map[string]any{"revision": r.Revision}}
	proof := operationProof(t, f, key, intent)
	decision["proof"] = proof
	decision["revision"] = r.Revision + 1
	f.call("POST", "/v1/backups/jobs/"+r.ID+"/decision", decision, 409)
	decision["revision"] = r.Revision
	f.call("POST", "/v1/backups/jobs/"+r.ID+"/decision", decision, 200)
	f.call("POST", "/v1/backups/jobs/"+r.ID+"/decision", decision, 409)
	if err = e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	current, _ = e.Job(ctx, r.ID)
	if current.State != "succeeded" {
		t.Fatal(current.State)
	}
	ret, err := e.Prepare(ctx, backup.Request{Policy: "pilot", Kind: "retention"}, "retention-idempotent-123")
	if err != nil {
		t.Fatal(err)
	}
	intent["target"] = ret.ID
	intent["payload"] = map[string]any{"revision": ret.Revision}
	proof = operationProof(t, f, key, intent)
	f.call("POST", "/v1/backups/jobs/"+ret.ID+"/decision", map[string]any{"action": "approve", "revision": ret.Revision, "phrase": backup.Confirmation(ret), "proof": proof}, 200)
	if err = e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`UPDATE backup_artifacts SET expired_at=now() WHERE id=$1`, as[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err = e.Prepare(ctx, backup.Request{Policy: "pilot", Kind: "restore", Artifact: as[0].ID, Target: "test"}, "expired-artifact-12345678"); !errors.Is(err, backup.ErrConflict) {
		t.Fatal("expired artifact accepted")
	}
}
func TestPhaseSevenFailureAndInterruptedWorkNeverReplay(t *testing.T) {
	f, _, e, d := backupFixture(t)
	ctx := context.Background()
	d.fail = true
	j, err := e.Prepare(ctx, backup.Request{Policy: "pilot", Kind: "backup"}, "failure-idempotency-123")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	as, _ := e.Artifacts(ctx, 1)
	if len(as) != 0 {
		t.Fatal("failed snapshot advertised")
	}
	var n int
	if err = f.db.QueryRow(`SELECT count(*) FROM incidents WHERE rule_id='backup-pilot-backup' AND status='detected'`).Scan(&n); err != nil || n != 1 {
		t.Fatal("failure incident absent")
	}
	if _, err = f.db.Exec(`UPDATE backup_jobs SET state='running' WHERE id=$1`, j.ID); err != nil {
		t.Fatal(err)
	}
	if err = e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	j, _ = e.Job(ctx, j.ID)
	if j.State != "uncertain" || d.calls != 1 {
		t.Fatal("interrupted mutation replayed")
	}
	if _, err = e.Prepare(ctx, backup.Request{Policy: "pilot", Kind: "backup"}, "another-idempotency-123"); !errors.Is(err, backup.ErrConflict) {
		t.Fatal("uncertain job failed to reserve policy")
	}
	if err = e.Acknowledge(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	d.fail = false
	if _, err = e.Prepare(ctx, backup.Request{Policy: "pilot", Kind: "backup"}, "reconciled-idempotency-123"); err != nil {
		t.Fatal(err)
	}
	if err = e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if err = f.db.QueryRow(`SELECT count(*) FROM incidents WHERE rule_id='backup-pilot-backup' AND status='resolved'`).Scan(&n); err != nil || n != 1 {
		t.Fatal("success did not resolve failure")
	}
}
func TestPhaseSevenWorkerExclusionAndHostGuard(t *testing.T) {
	_, _, e, d := backupFixture(t)
	ctx := context.Background()
	if _, err := e.Prepare(ctx, backup.Request{Policy: "pilot", Kind: "backup"}, "concurrent-idempotency-123"); err != nil {
		t.Fatal(err)
	}
	l, err := coordination.Acquire(e.Config.CoordinationRoot, "source", "recovery", "operator")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Tick(ctx); err == nil {
		t.Fatal("recovery host lock ignored")
	}
	if d.calls != 0 {
		t.Fatal("driver called on blocked host")
	}
	if err = l.Close(true); err != nil {
		t.Fatal(err)
	}
	s, err := coordination.Read(e.Config.CoordinationRoot, "source")
	if err != nil {
		t.Fatal(err)
	}
	if err = coordination.Reconcile(e.Config.CoordinationRoot, "source", "operator", s.Epoch); err != nil {
		t.Fatal(err)
	}
	d.entered = make(chan bool, 1)
	d.release = make(chan bool)
	done := make(chan error, 1)
	go func() { done <- e.Tick(ctx) }()
	select {
	case <-d.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("worker didn't start")
	}
	if err = e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	close(d.release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if d.calls != 1 {
		t.Fatal("duplicate worker execution")
	}
}

func TestPhaseSevenSchedulesAreDurableAndDoNotCatchUpInABurst(t *testing.T) {
	f, _, e, _ := backupFixture(t)
	ctx := context.Background()
	p := e.Config.Policies[0]
	p.ID = "scheduled"
	p.Kind = "postgres"
	p.Path = ""
	p.Consistency = "logical"
	p.ServiceFile = filepath.Join(f.dir, "pg.conf")
	p.Service = "source"
	p.IntervalSeconds = 300
	p.RestoreTestSeconds = 3600
	p.TestTarget = "scheduled-test"
	e.Config.Policies = []backup.Policy{p}
	e.Config.Targets = []backup.Target{{ID: p.TestTarget, Policy: p.ID, Server: "test-host", Kind: "postgres", TestOnly: true, ServiceFile: p.ServiceFile, Service: "target", ValidationFile: filepath.Join(f.dir, "check.sql")}}
	if err := e.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := e.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	jobs, err := e.Jobs(ctx, 1)
	if err != nil || len(jobs) != 3 {
		t.Fatalf("scheduled backup, retention and restore: %v %d", err, len(jobs))
	}
	for _, j := range jobs {
		if !j.Scheduled || j.State != "succeeded" {
			t.Fatal("scheduled operation not verified")
		}
	}
	if err = e.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if err = e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	jobs, err = e.Jobs(ctx, 1)
	if err != nil || len(jobs) != 3 {
		t.Fatal("restart replayed a scheduled slot")
	}
}

func TestPhaseSevenDeploymentRequiresMatchingFreshRetainedBackup(t *testing.T) {
	f, d, driver := deploymentFixture(t)
	ctx := context.Background()
	m := d.Config.Targets[0]
	m.ID = "sounding-lines-backed"
	m.BackupRequired = true
	m.BackupPolicy = "sounding-lines-db"
	m.BackupMaxAgeSeconds = 300
	d.Config.Targets = []deployment.Manifest{m}
	if err := d.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	j := prepareJob(t, d, "release-1", "without-backup", false)
	if len(j.Plan.Blockers) == 0 {
		t.Fatal("deployment allowed without backup")
	}
	root := f.dir
	c := backup.Config{Version: 1, WorkRoot: root, CoordinationRoot: filepath.Join(root, "guard"), Repositories: []backup.Repository{{ID: "local", LocationFile: filepath.Join(root, "repo"), PasswordFile: filepath.Join(root, "key"), StorageHost: m.Server}}, Policies: []backup.Policy{{ID: m.BackupPolicy, Project: m.Project, Environment: m.Environment, Server: m.Server, Repository: "local", Kind: "files", Path: filepath.Join(root, "data"), Consistency: "crash", Enabled: true, TimeoutSeconds: 60, Retention: backup.Retention{KeepLast: 2}}}}
	e := &backup.Engine{DB: f.db, Config: c, Driver: &backupDriver{}}
	if err := e.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Prepare(ctx, backup.Request{Policy: m.BackupPolicy, Kind: "backup"}, "before-deployment-123456"); err != nil {
		t.Fatal(err)
	}
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	j = prepareJob(t, d, "release-1", "with-backup", false)
	if j.Plan.BackupArtifact == "" || len(j.Plan.Blockers) != 0 {
		t.Fatal("verified artifact not bound")
	}
	j = approveJob(t, d, j)
	if _, err := f.db.Exec(`UPDATE backup_artifacts SET created_at=now()-interval '1 hour' WHERE id=$1`, j.Plan.BackupArtifact); err != nil {
		t.Fatal(err)
	}
	runJob(t, d, j, "failed")
	if len(driver.steps) != 0 {
		t.Fatal("deployment mutated after backup expired")
	}
	if _, err := f.db.Exec(`UPDATE backup_artifacts SET created_at=now(),expired_at=now() WHERE id=$1`, j.Plan.BackupArtifact); err != nil {
		t.Fatal(err)
	}
	j = prepareJob(t, d, "release-1", "expired-backup", false)
	if len(j.Plan.Blockers) == 0 {
		t.Fatal("retention ignored")
	}
}
