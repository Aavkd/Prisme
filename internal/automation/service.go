package automation

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

type Driver interface {
	Check(context.Context, Target) error
	Execute(context.Context, Target, Step, string) (map[string]any, error)
}
type Engine struct {
	DB     *sql.DB
	Config Config
	Driver Driver
	Now    func() time.Time
	Audit  func(context.Context, *sql.Tx, string, string) error
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now().UTC()
	}
	return time.Now().UTC()
}
func (e *Engine) Initialize(context.Context) error { return e.Config.Validate() }
func raw(v any) string                             { b, _ := json.Marshal(v); return string(b) }
func id() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func (e *Engine) transaction(ctx context.Context) (*sql.Tx, error) {
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(current_database()||'.'||current_schema(),731909))`); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}
func (e *Engine) audit(ctx context.Context, tx *sql.Tx, action, target string) error {
	if e.Audit != nil {
		return e.Audit(ctx, tx, "automation."+action, target)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_events(actor,action,target,origin) VALUES('automation-worker',$1,$2,'system')`, "automation."+action, target)
	return err
}
func (e *Engine) event(ctx context.Context, tx *sql.Tx, r Run, kind string, detail any) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO automation_events(run_id,kind,detail) VALUES($1,$2,$3)`, r.ID, kind, raw(detail)); err != nil {
		return err
	}
	if r.Incident != 0 {
		_, err := tx.ExecContext(ctx, `INSERT INTO incident_events(incident_id,occurred_at,kind,actor,message) VALUES($1,$2,'automation','automation-worker',$3)`, r.Incident, e.now(), fmt.Sprintf("%s · %s · step %d · %s", r.ID, kind, r.Step, r.ErrorCode))
		if err != nil {
			return err
		}
	}
	return e.audit(ctx, tx, kind, r.ID)
}

type query interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func control(ctx context.Context, q query) (Control, error) {
	var c Control
	err := q.QueryRowContext(ctx, `SELECT stopped,revision,maintenance_until FROM automation_control WHERE id=1`).Scan(&c.Stopped, &c.Revision, &c.MaintenanceUntil)
	return c, err
}
func (e *Engine) Control(ctx context.Context) (Control, error) { return control(ctx, e.DB) }
func (e *Engine) SetControl(ctx context.Context, c Control, authorize func() error) (Control, error) {
	tx, err := e.transaction(ctx)
	if err != nil {
		return c, err
	}
	defer tx.Rollback()
	old, err := control(ctx, tx)
	if err != nil {
		return c, err
	}
	if c.Revision != old.Revision {
		return c, ErrConflict
	}
	if !c.Stopped {
		if authorize == nil {
			return c, ErrBlocked
		}
		if err = authorize(); err != nil {
			return c, err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE automation_control SET stopped=$1,revision=revision+1,maintenance_until=$2 WHERE id=1`, c.Stopped, c.MaintenanceUntil); err != nil {
		return c, err
	}
	if err = e.audit(ctx, tx, "control", fmt.Sprint(c.Stopped)); err != nil {
		return c, err
	}
	// Pending runs are cancelled on stop. A command in flight is cancelled by the
	// worker and remains uncertain until the operator reconciles its host.
	if c.Stopped {
		rows, err := tx.QueryContext(ctx, `SELECT document FROM automation_runs WHERE state IN ('queued','approval','waiting')`)
		if err != nil {
			return c, err
		}
		runs, err := readRuns(rows)
		if err != nil {
			return c, err
		}
		for _, r := range runs {
			r.State = "cancelled"
			r.ErrorCode = "global_stop"
			if err = e.save(ctx, tx, &r, "cancelled", nil); err != nil {
				return c, err
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return c, err
	}
	return e.Control(ctx)
}
func (e *Engine) Publish(ctx context.Context, kind string, doc json.RawMessage, authorize func() error) error {
	var key, target string
	var version int
	var value any
	switch kind {
	case "policy":
		var p Policy
		if decode(doc, &p) != nil || p.Validate(e.Config) != nil {
			return ErrInvalid
		}
		key, target, version, value = p.ID, p.Target, p.Version, p
	case "runbook":
		var b Runbook
		if decode(doc, &b) != nil || b.Validate(e.Config) != nil {
			return ErrInvalid
		}
		key, target, version, value = b.ID, b.Target, b.Version, b
	default:
		return ErrInvalid
	}
	tx, err := e.transaction(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var last int
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(max(version),0) FROM automation_definitions WHERE kind=$1 AND id=$2`, kind, key).Scan(&last); err != nil {
		return err
	}
	if version != last+1 {
		return ErrConflict
	}
	if kind == "policy" {
		var occupied bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM (SELECT DISTINCT ON(id) id,document FROM automation_definitions WHERE kind='policy' ORDER BY id,version DESC) p WHERE id<>$1 AND document->>'target'=$2)`, key, target).Scan(&occupied); err != nil {
			return err
		}
		if occupied {
			return ErrConflict
		}
	}
	if authorize == nil {
		return ErrBlocked
	}
	if err = authorize(); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO automation_definitions(kind,id,version,document) VALUES($1,$2,$3,$4)`, kind, key, version, raw(value)); err != nil {
		return err
	}
	if kind == "policy" {
		if _, err = tx.ExecContext(ctx, `INSERT INTO automation_policy_state(id) VALUES($1) ON CONFLICT DO NOTHING`, key); err != nil {
			return err
		}
	}
	if err = e.audit(ctx, tx, "published", fmt.Sprintf("%s/%s/%d", kind, key, version)); err != nil {
		return err
	}
	return tx.Commit()
}
func decode(b []byte, v any) error { return strictDecode(b, v) }
func definition(ctx context.Context, q query, kind, key string, version int, v any) error {
	var b []byte
	var err error
	if version == 0 {
		err = q.QueryRowContext(ctx, `SELECT document FROM automation_definitions WHERE kind=$1 AND id=$2 ORDER BY version DESC LIMIT 1`, kind, key).Scan(&b)
	} else {
		err = q.QueryRowContext(ctx, `SELECT document FROM automation_definitions WHERE kind=$1 AND id=$2 AND version=$3`, kind, key, version).Scan(&b)
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
func scanRun(row interface{ Scan(...any) error }) (Run, error) {
	var r Run
	var b []byte
	err := row.Scan(&b)
	if err == nil {
		err = json.Unmarshal(b, &r)
	}
	return r, err
}
func readRuns(rows *sql.Rows) ([]Run, error) {
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (e *Engine) RunByID(ctx context.Context, key string) (Run, error) {
	return scanRun(e.DB.QueryRowContext(ctx, `SELECT document FROM automation_runs WHERE id=$1`, key))
}
func (e *Engine) save(ctx context.Context, tx *sql.Tx, r *Run, kind string, detail any) error {
	r.Revision++
	result, err := tx.ExecContext(ctx, `UPDATE automation_runs SET document=$2,state=$3,updated_at=$4 WHERE id=$1 AND (document->>'revision')::integer=$5`, r.ID, raw(r), r.State, e.now(), r.Revision-1)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return ErrConflict
	}
	return e.event(ctx, tx, *r, kind, detail)
}
func (e *Engine) prepare(ctx context.Context, tx *sql.Tx, req Request, key, origin string, policy *Policy, incident int64) (Run, error) {
	signature := Hash(req)
	if policy != nil {
		signature = Hash(policy)
	}
	var h string
	err := tx.QueryRowContext(ctx, `SELECT request_hash FROM automation_runs WHERE request_key=$1`, key).Scan(&h)
	if err == nil {
		if h != signature {
			return Run{}, ErrConflict
		}
		return scanRun(tx.QueryRowContext(ctx, `SELECT document FROM automation_runs WHERE request_key=$1`, key))
	}
	if err != sql.ErrNoRows {
		return Run{}, err
	}
	var b Runbook
	if policy != nil {
		b = policyBook(*policy)
	} else {
		if req.Version < 1 {
			return Run{}, ErrInvalid
		}
		if err = definition(ctx, tx, "runbook", req.Runbook, req.Version, &b); err != nil {
			return Run{}, err
		}
		b, err = b.Bind(req.Parameters)
		if err != nil {
			return Run{}, err
		}
	}
	if err = b.Validate(e.Config); err != nil {
		return Run{}, err
	}
	t, _ := e.Config.Target(b.Target)
	var busy bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM automation_runs WHERE target=$1 AND state IN ('queued','approval','running','waiting','uncertain'))`, b.Target).Scan(&busy); err != nil {
		return Run{}, err
	}
	if busy {
		return Run{}, ErrConflict
	}
	r := Run{ID: id(), Revision: 1, Target: b.Target, TargetHash: Hash(t), State: "queued", Origin: origin, Book: b, Policy: policy, Parameters: req.Parameters, Incident: incident, ApprovedStep: -1, Simulation: req.Simulation, DueAt: e.now(), Deadline: e.now().Add(time.Duration(b.TimeoutSeconds) * time.Second)}
	if policy != nil && policy.Mode == "supervised" {
		r.State = "observed"
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO automation_runs(id,request_key,request_hash,target,state,document) VALUES($1,$2,$3,$4,$5,$6)`, r.ID, key, signature, r.Target, r.State, raw(r)); err != nil {
		return r, err
	}
	if err = e.event(ctx, tx, r, "planned", map[string]any{"origin": origin, "version": b.Version, "simulation": r.Simulation}); err != nil {
		return r, err
	}
	return r, nil
}
func (e *Engine) Prepare(ctx context.Context, req Request, key string) (Run, error) {
	tx, err := e.transaction(ctx)
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback()
	r, err := e.prepare(ctx, tx, req, key, "manual", nil, 0)
	if err != nil {
		return r, err
	}
	return r, tx.Commit()
}
func (e *Engine) Decide(ctx context.Context, key, action string, revision int, authorize func() error) (Run, error) {
	tx, err := e.transaction(ctx)
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback()
	r, err := scanRun(tx.QueryRowContext(ctx, `SELECT document FROM automation_runs WHERE id=$1`, key))
	if err != nil {
		return r, err
	}
	if r.Revision != revision {
		return r, ErrConflict
	}
	switch action {
	case "approve":
		if r.State != "approval" || !e.now().Before(r.Deadline) {
			return r, ErrConflict
		}
		if authorize == nil {
			return r, ErrBlocked
		}
		if err = authorize(); err != nil {
			return r, err
		}
		r.ApprovedStep = r.Step
		until := e.now().Add(2 * time.Minute)
		r.ApprovedUntil = &until
		r.State = "queued"
		r.DueAt = e.now()
	case "cancel":
		if r.State != "queued" && r.State != "waiting" && r.State != "approval" {
			return r, ErrConflict
		}
		r.State = "cancelled"
	case "reconcile":
		if r.State != "uncertain" {
			return r, ErrConflict
		}
		if authorize == nil {
			return r, ErrBlocked
		}
		if err = authorize(); err != nil {
			return r, err
		}
		r.ErrorCode = "reconciliation_requested"
	default:
		return r, ErrInvalid
	}
	if err = e.save(ctx, tx, &r, action, nil); err != nil {
		return r, err
	}
	return r, tx.Commit()
}
func (e *Engine) Reset(ctx context.Context, key string, authorize func() error) error {
	tx, err := e.transaction(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var p Policy
	if err = definition(ctx, tx, "policy", key, 0, &p); err != nil {
		return err
	}
	var breaker bool
	var until *time.Time
	if err = tx.QueryRowContext(ctx, `SELECT breaker,cooldown_until FROM automation_policy_state WHERE id=$1`, key).Scan(&breaker, &until); err != nil {
		return err
	}
	if !breaker || until == nil || e.now().Before(*until) {
		return ErrConflict
	}
	var busy bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM automation_runs WHERE target=$1 AND state IN ('queued','approval','running','waiting','uncertain'))`, p.Target).Scan(&busy); err != nil {
		return err
	}
	if busy {
		return ErrConflict
	}
	if authorize == nil {
		return ErrBlocked
	}
	if err = authorize(); err != nil {
		return err
	}
	// Keep last_incident: resetting cannot restart the same unresolved outage.
	if _, err = tx.ExecContext(ctx, `UPDATE automation_policy_state SET breaker=false WHERE id=$1`, key); err != nil {
		return err
	}
	if err = e.audit(ctx, tx, "breaker_reset", key); err != nil {
		return err
	}
	return tx.Commit()
}
func (e *Engine) Schedule(ctx context.Context, s Schedule, authorize func() error) error {
	if !name.MatchString(s.ID) || s.Revision < 1 || s.EverySeconds < 60 || s.EverySeconds > 2678400 || s.Request.Version < 1 || s.NextAt.Before(e.now()) || s.NextAt.After(e.now().Add(366*24*time.Hour)) {
		return ErrInvalid
	}
	tx, err := e.transaction(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var b Runbook
	if err = definition(ctx, tx, "runbook", s.Request.Runbook, s.Request.Version, &b); err != nil {
		return err
	}
	if _, err = b.Bind(s.Request.Parameters); err != nil {
		return err
	}
	var previous []byte
	err = tx.QueryRowContext(ctx, `SELECT document FROM automation_schedules WHERE id=$1`, s.ID).Scan(&previous)
	rev := 0
	if err == nil {
		var old Schedule
		if json.Unmarshal(previous, &old) != nil {
			return ErrInvalid
		}
		rev = old.Revision
	} else if err != sql.ErrNoRows {
		return err
	}
	if s.Revision != rev+1 {
		return ErrConflict
	}
	if authorize == nil {
		return ErrBlocked
	}
	if err = authorize(); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO automation_schedules(id,document,next_at,enabled) VALUES($1,$2,$3,$4) ON CONFLICT(id) DO UPDATE SET document=$2,next_at=$3,enabled=$4`, s.ID, raw(s), s.NextAt, s.Enabled); err != nil {
		return err
	}
	if err = e.audit(ctx, tx, "scheduled", s.ID); err != nil {
		return err
	}
	return tx.Commit()
}
func (e *Engine) Overview(ctx context.Context, page int) (map[string]any, error) {
	c, err := e.Control(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"enabled": true, "control": c, "targets": e.Config.Targets}
	for key, q := range map[string]string{
		"definitions": `SELECT document FROM (SELECT DISTINCT ON(kind,id) document FROM automation_definitions ORDER BY kind,id,version DESC) d`,
		"schedules":   `SELECT document FROM automation_schedules ORDER BY id`,
		"policies":    `SELECT jsonb_build_object('id',id,'breaker',breaker,'cooldown_until',cooldown_until,'last_incident',last_incident) FROM automation_policy_state ORDER BY id`,
	} {
		rows, err := e.DB.QueryContext(ctx, q)
		if err != nil {
			return nil, err
		}
		values := []any{}
		for rows.Next() {
			var b []byte
			if err = rows.Scan(&b); err != nil {
				rows.Close()
				return nil, err
			}
			var v any
			if err = json.Unmarshal(b, &v); err != nil {
				rows.Close()
				return nil, err
			}
			values = append(values, v)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		out[key] = values
	}
	rows, err := e.DB.QueryContext(ctx, `SELECT document FROM automation_runs ORDER BY created_at DESC,id DESC LIMIT 50 OFFSET $1`, (page-1)*50)
	if err != nil {
		return nil, err
	}
	runs, err := readRuns(rows)
	out["runs"] = runs
	out["has_more"] = len(runs) == 50
	return out, err
}
func (e *Engine) Events(ctx context.Context, key string) ([]any, error) {
	rows, err := e.DB.QueryContext(ctx, `SELECT kind,detail,created_at FROM automation_events WHERE run_id=$1 ORDER BY id LIMIT 1000`, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var kind string
		var b []byte
		var at time.Time
		if err = rows.Scan(&kind, &b, &at); err != nil {
			return nil, err
		}
		var detail any
		if err = json.Unmarshal(b, &detail); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"kind": kind, "detail": detail, "at": at})
	}
	return out, rows.Err()
}
