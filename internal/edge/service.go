package edge

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

type Engine struct {
	DB     *sql.DB
	Config Config
	Driver Driver
	Audit  func(context.Context, *sql.Tx, string, string) error
}
type Job struct {
	ID         string         `json:"id"`
	Target     string         `json:"target"`
	Request    Request        `json:"request"`
	State      string         `json:"state"`
	Revision   int            `json:"revision"`
	CreatedAt  time.Time      `json:"created_at"`
	ExpiresAt  time.Time      `json:"expires_at"`
	Plan       Plan           `json:"plan"`
	Result     map[string]any `json:"result"`
	ErrorCode  string         `json:"error_code"`
	ConfigHash string         `json:"-"`
	Snapshot   Snapshot       `json:"-"`
}

func ID() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func Confirmation(j Job) string                    { return "APPLY " + j.Target + " " + j.ID[:8] }
func (e *Engine) Initialize(context.Context) error { return e.Config.Validate() }
func (e *Engine) audit(ctx context.Context, tx *sql.Tx, action, target string) error {
	if e.Audit != nil {
		return e.Audit(ctx, tx, action, target)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_events(actor,action,target,origin) VALUES('edge-worker',$1,$2,'system')`, action, target)
	return err
}

const columns = `id,target,request,state,revision,created_at,expires_at,plan,snapshot,result,error_code,config_hash`

type scanner interface{ Scan(...any) error }

func scanJob(row scanner) (Job, error) {
	var j Job
	var req, plan, snapshot, result []byte
	err := row.Scan(&j.ID, &j.Target, &req, &j.State, &j.Revision, &j.CreatedAt, &j.ExpiresAt, &plan, &snapshot, &result, &j.ErrorCode, &j.ConfigHash)
	if err != nil {
		return j, err
	}
	for _, v := range []struct {
		b   []byte
		out any
	}{{req, &j.Request}, {plan, &j.Plan}, {snapshot, &j.Snapshot}, {result, &j.Result}} {
		if err = json.Unmarshal(v.b, v.out); err != nil {
			return j, err
		}
	}
	if j.Plan.Checks == nil {
		j.Plan.Checks = []Check{}
	}
	return j, nil
}
func (e *Engine) Job(ctx context.Context, id string) (Job, error) {
	return scanJob(e.DB.QueryRowContext(ctx, `SELECT `+columns+` FROM edge_jobs WHERE id=$1`, id))
}
func (e *Engine) Jobs(ctx context.Context, page int) ([]Job, error) {
	rows, err := e.DB.QueryContext(ctx, `SELECT `+columns+` FROM edge_jobs ORDER BY created_at DESC,id DESC LIMIT 50 OFFSET $1`, (page-1)*50)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
func (e *Engine) Observations(ctx context.Context) ([]any, error) {
	rows, err := e.DB.QueryContext(ctx, `SELECT id,observed_at,data,error_code FROM edge_observations WHERE config_hash=$1 ORDER BY id`, Hash(e.Config))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id, code string
		var at time.Time
		var b []byte
		var data any
		if err = rows.Scan(&id, &at, &b, &code); err != nil {
			return nil, err
		}
		if json.Unmarshal(b, &data) != nil {
			return nil, ErrUnavailable
		}
		out = append(out, map[string]any{"id": id, "observed_at": at, "data": data, "error_code": code, "stale": time.Since(at) > 90*time.Second})
	}
	return out, rows.Err()
}
func (e *Engine) Prepare(ctx context.Context, r Request, key string) (Job, error) {
	t, err := e.Config.Target(r.Target)
	if err != nil || !t.WriteEnabled || len(key) < 16 || len(key) > 128 || len(r.Value) > 253 || (r.Rollback != "" && r.Value != "") || (r.Rollback == "" && r.Value == "") {
		return Job{}, ErrInvalid
	}
	if r.Rollback != "" {
		old, err := e.Job(ctx, r.Rollback)
		if err != nil || old.Target != t.ID || old.ConfigHash != Hash(e.Config) || !RollbackAvailable(old) {
			return Job{}, ErrConflict
		}
	}
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,731810))`, key); err != nil {
		return Job{}, err
	}
	var id, h string
	err = tx.QueryRowContext(ctx, `SELECT id,request_hash FROM edge_jobs WHERE request_key=$1`, key).Scan(&id, &h)
	if err == nil {
		if h != Hash(r) {
			return Job{}, ErrConflict
		}
		return scanJob(tx.QueryRowContext(ctx, `SELECT `+columns+` FROM edge_jobs WHERE id=$1`, id))
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Job{}, err
	}
	id = ID()
	b, _ := json.Marshal(r)
	if _, err = tx.ExecContext(ctx, `INSERT INTO edge_jobs(id,target,request_key,request_hash,config_hash,request,state) VALUES($1,$2,$3,$4,$5,$6,'preparing')`, id, t.ID, key, Hash(r), Hash(e.Config), string(b)); err != nil {
		return Job{}, err
	}
	if err = e.audit(ctx, tx, "edge.prepare", id); err != nil {
		return Job{}, err
	}
	if err = tx.Commit(); err != nil {
		return Job{}, err
	}
	return e.Job(ctx, id)
}
func (e *Engine) Decide(ctx context.Context, id, action, phrase string, revision int, proof func() error) (Job, error) {
	if action != "approve" && action != "cancel" {
		return Job{}, ErrInvalid
	}
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(731811)`); err != nil {
		return Job{}, err
	}
	j, err := scanJob(tx.QueryRowContext(ctx, `SELECT `+columns+` FROM edge_jobs WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return Job{}, err
	}
	if j.Revision != revision || j.State != "planned" || time.Now().After(j.ExpiresAt) || j.ConfigHash != Hash(e.Config) {
		return Job{}, ErrConflict
	}
	state := "cancelled"
	if action == "approve" {
		t, err := e.Config.Target(j.Target)
		if err != nil || !t.WriteEnabled || phrase != Confirmation(j) {
			return Job{}, ErrConflict
		}
		var pending bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM edge_jobs WHERE target=$1 AND state IN ('queued','running','uncertain'))`, j.Target).Scan(&pending); err != nil {
			return Job{}, err
		}
		if pending {
			return Job{}, ErrConflict
		}
		if proof == nil {
			return Job{}, ErrInvalid
		}
		if err = proof(); err != nil {
			return Job{}, err
		}
		state = "queued"
	}
	if _, err = tx.ExecContext(ctx, `UPDATE edge_jobs SET state=$2,revision=revision+1 WHERE id=$1`, id, state); err != nil {
		return Job{}, err
	}
	if err = e.audit(ctx, tx, "edge."+action, id); err != nil {
		return Job{}, err
	}
	if err = tx.Commit(); err != nil {
		return Job{}, err
	}
	return e.Job(ctx, id)
}
func (e *Engine) save(ctx context.Context, j Job) error {
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	p, _ := json.Marshal(j.Plan)
	s, _ := json.Marshal(j.Snapshot)
	if j.Result == nil {
		j.Result = map[string]any{}
	}
	r, _ := json.Marshal(j.Result)
	result, err := tx.ExecContext(ctx, `UPDATE edge_jobs SET state=$2,revision=revision+1,plan=$3,snapshot=$4,result=$5,error_code=$6 WHERE id=$1 AND revision=$7`, j.ID, j.State, string(p), string(s), string(r), j.ErrorCode, j.Revision)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return ErrConflict
	}
	if err = e.audit(ctx, tx, "edge."+j.State, j.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// A reconciled mutation can offer its inverse, but the provider must still match
// the saved after-state when the worker prepares the new plan.
func RollbackAvailable(j Job) bool {
	return j.State == "succeeded" || (j.State == "failed" && j.ErrorCode == "operator_reconciled")
}
