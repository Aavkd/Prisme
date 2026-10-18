package backup

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

type Job struct {
	KnownSnapshots     []string       `json:"-"`
	ProtectedSnapshots []string       `json:"-"`
	ID                 string         `json:"id"`
	PolicyID           string         `json:"policy_id"`
	Kind               string         `json:"kind"`
	ArtifactID         string         `json:"artifact_id"`
	TargetID           string         `json:"target_id"`
	State              string         `json:"state"`
	Step               string         `json:"step"`
	Revision           int            `json:"revision"`
	Scheduled          bool           `json:"scheduled"`
	CreatedAt          time.Time      `json:"created_at"`
	ExpiresAt          time.Time      `json:"expires_at"`
	StartedAt          *time.Time     `json:"started_at"`
	FinishedAt         *time.Time     `json:"finished_at"`
	ErrorCode          string         `json:"error_code"`
	Evidence           map[string]any `json:"evidence"`
	ConfigHash         string         `json:"-"`
}
type Request struct {
	Policy   string `json:"policy"`
	Kind     string `json:"kind"`
	Artifact string `json:"artifact"`
	Target   string `json:"target"`
}
type Engine struct {
	DB     *sql.DB
	Config Config
	Driver Driver
	Audit  func(context.Context, *sql.Tx, string, string) error
}

func id() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func (e *Engine) configHash(p Policy) string {
	r, _ := e.Config.Repository(p.Repository)
	targets := []Target{}
	for _, t := range e.Config.Targets {
		if t.Policy == p.ID {
			targets = append(targets, t)
		}
	}
	return Hash(struct {
		Policy           Policy
		Repository       Repository
		Targets          []Target
		CoordinationRoot string
		WorkRoot         string
	}{p, r, targets, e.Config.CoordinationRoot, e.Config.WorkRoot})
}
func (e *Engine) Initialize(ctx context.Context) error {
	if err := e.Config.Validate(); err != nil {
		return err
	}
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, p := range e.Config.Policies {
		h := e.configHash(p)
		if _, err = tx.ExecContext(ctx, `INSERT INTO backup_policies(id,config_hash,project,environment,server,repository) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, p.ID, h, p.Project, p.Environment, p.Server, p.Repository); err != nil {
			return err
		}
		var existing string
		if err = tx.QueryRowContext(ctx, `SELECT config_hash FROM backup_policies WHERE id=$1`, p.ID).Scan(&existing); err != nil {
			return err
		}
		if existing != h {
			return ErrConflict
		}
	}
	return tx.Commit()
}
func (e *Engine) audit(ctx context.Context, tx *sql.Tx, action, target string) error {
	if e.Audit != nil {
		return e.Audit(ctx, tx, action, target)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_events(actor,action,target,origin) VALUES('backup-worker',$1,$2,'system')`, action, target)
	return err
}

const columns = `id,policy_id,kind,artifact_id,target_id,state,step,revision,scheduled,created_at,expires_at,started_at,finished_at,error_code,evidence,config_hash`

type scanner interface{ Scan(...any) error }

func scanJob(row scanner) (Job, error) {
	var j Job
	var b []byte
	err := row.Scan(&j.ID, &j.PolicyID, &j.Kind, &j.ArtifactID, &j.TargetID, &j.State, &j.Step, &j.Revision, &j.Scheduled, &j.CreatedAt, &j.ExpiresAt, &j.StartedAt, &j.FinishedAt, &j.ErrorCode, &b, &j.ConfigHash)
	if err == nil {
		err = json.Unmarshal(b, &j.Evidence)
	}
	return j, err
}
func (e *Engine) Job(ctx context.Context, id string) (Job, error) {
	return scanJob(e.DB.QueryRowContext(ctx, `SELECT `+columns+` FROM backup_jobs WHERE id=$1`, id))
}
func (e *Engine) Jobs(ctx context.Context, page int) ([]Job, error) {
	rows, err := e.DB.QueryContext(ctx, `SELECT `+columns+` FROM backup_jobs ORDER BY created_at DESC,id DESC LIMIT 50 OFFSET $1`, (page-1)*50)
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

const artifactColumns = `id,policy_id,job_id,snapshot_id,bytes,created_at,verified_at,expired_at`

func scanArtifact(row scanner) (Artifact, error) {
	var a Artifact
	err := row.Scan(&a.ID, &a.PolicyID, &a.JobID, &a.SnapshotID, &a.Bytes, &a.CreatedAt, &a.VerifiedAt, &a.ExpiredAt)
	return a, err
}

func (e *Engine) Artifact(ctx context.Context, id string) (Artifact, error) {
	return scanArtifact(e.DB.QueryRowContext(ctx, `SELECT `+artifactColumns+` FROM backup_artifacts WHERE id=$1`, id))
}
func (e *Engine) Artifacts(ctx context.Context, page int) ([]Artifact, error) {
	rows, err := e.DB.QueryContext(ctx, `SELECT `+artifactColumns+` FROM backup_artifacts ORDER BY created_at DESC,id DESC LIMIT 50 OFFSET $1`, (page-1)*50)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Artifact{}
	for rows.Next() {
		a, err := scanArtifact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (e *Engine) Prepare(ctx context.Context, req Request, key string) (Job, error) {
	return e.prepare(ctx, req, key, false)
}
func (e *Engine) prepare(ctx context.Context, req Request, key string, scheduled bool) (Job, error) {
	p, err := e.Config.Policy(req.Policy)
	if err != nil || !p.Enabled || len(key) < 16 || len(key) > 128 {
		return Job{}, ErrInvalid
	}
	if req.Kind != "backup" && req.Kind != "restore" && req.Kind != "check" && req.Kind != "retention" {
		return Job{}, ErrInvalid
	}
	if req.Kind != "restore" && (req.Artifact != "" || req.Target != "") {
		return Job{}, ErrInvalid
	}
	if req.Kind == "restore" {
		t, err := e.Config.Target(req.Target)
		if err != nil || t.Policy != p.ID || !snapshotID.MatchString(req.Artifact) {
			return Job{}, ErrInvalid
		}
	}
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,731709))`, key); err != nil {
		return Job{}, err
	}
	var existing, h string
	err = tx.QueryRowContext(ctx, `SELECT id,request_hash FROM backup_jobs WHERE request_key=$1`, key).Scan(&existing, &h)
	if err == nil {
		if h != Hash(req) {
			return Job{}, ErrConflict
		}
		return scanJob(tx.QueryRowContext(ctx, `SELECT `+columns+` FROM backup_jobs WHERE id=$1`, existing))
	}
	if err != sql.ErrNoRows {
		return Job{}, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT config_hash FROM backup_policies WHERE id=$1 FOR UPDATE`, p.ID).Scan(&h); err != nil {
		return Job{}, err
	}
	if h != e.configHash(p) {
		return Job{}, ErrConflict
	}
	var pending bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM backup_jobs WHERE policy_id=$1 AND state IN ('queued','running','uncertain'))`, p.ID).Scan(&pending); err != nil {
		return Job{}, err
	}
	if pending {
		return Job{}, ErrConflict
	}
	if req.Kind == "restore" {
		var ok bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM backup_artifacts WHERE id=$1 AND policy_id=$2 AND config_hash=$3 AND expired_at IS NULL)`, req.Artifact, p.ID, h).Scan(&ok); err != nil {
			return Job{}, err
		}
		if !ok {
			return Job{}, ErrConflict
		}
	}
	state := "queued"
	if !scheduled && (req.Kind == "restore" || req.Kind == "retention") {
		state = "planned"
	}
	jobID := id()
	if _, err = tx.ExecContext(ctx, `INSERT INTO backup_jobs(id,policy_id,config_hash,kind,artifact_id,target_id,state,request_key,request_hash,scheduled) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, jobID, p.ID, h, req.Kind, req.Artifact, req.Target, state, key, Hash(req), scheduled); err != nil {
		return Job{}, err
	}
	if err = e.audit(ctx, tx, "backup."+req.Kind+".requested", jobID); err != nil {
		return Job{}, err
	}
	if scheduled {
		column := "next_backup_at"
		seconds := p.IntervalSeconds
		if req.Kind == "restore" {
			column = "next_test_at"
			seconds = p.RestoreTestSeconds
		}
		if req.Kind != "retention" {
			_, err = tx.ExecContext(ctx, `UPDATE backup_policies SET `+column+`=now()+$2*interval '1 second' WHERE id=$1`, p.ID, seconds)
			if err != nil {
				return Job{}, err
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return Job{}, err
	}
	return e.Job(ctx, jobID)
}

// Authorization runs while the plan row is locked. Proofs bind job + revision;
// failed transactions consume the proof conservatively and never execute work.
func (e *Engine) Decide(ctx context.Context, jobID, action, phrase string, revision int, authorize func() error) (Job, error) {
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback()
	j, err := scanJob(tx.QueryRowContext(ctx, `SELECT `+columns+` FROM backup_jobs WHERE id=$1 FOR UPDATE`, jobID))
	if err != nil {
		return Job{}, err
	}
	if j.Revision != revision {
		return Job{}, ErrConflict
	}
	state := "cancelled"
	if action == "approve" {
		if j.State != "planned" || time.Now().After(j.ExpiresAt) || phrase != Confirmation(j) || authorize == nil {
			return Job{}, ErrConflict
		}
		p, err := e.Config.Policy(j.PolicyID)
		if err != nil || !p.Enabled || j.ConfigHash != e.configHash(p) {
			return Job{}, ErrConflict
		}
		var h string
		if err = tx.QueryRowContext(ctx, `SELECT config_hash FROM backup_policies WHERE id=$1 FOR UPDATE`, p.ID).Scan(&h); err != nil {
			return Job{}, err
		}
		var pending bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM backup_jobs WHERE policy_id=$1 AND state IN ('queued','running','uncertain'))`, p.ID).Scan(&pending); err != nil {
			return Job{}, err
		}
		if pending {
			return Job{}, ErrConflict
		}
		if err = authorize(); err != nil {
			return Job{}, err
		}
		state = "queued"
	} else if action != "cancel" || (j.State != "planned" && j.State != "queued") {
		return Job{}, ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `UPDATE backup_jobs SET state=$2,revision=revision+1 WHERE id=$1`, jobID, state); err != nil {
		return Job{}, err
	}
	if err = e.audit(ctx, tx, "backup."+action, jobID); err != nil {
		return Job{}, err
	}
	if err = tx.Commit(); err != nil {
		return Job{}, err
	}
	return e.Job(ctx, jobID)
}
func Confirmation(j Job) string {
	if j.Kind == "restore" {
		return "RESTORE " + j.TargetID
	}
	return "RETENTION " + j.PolicyID
}
func (e *Engine) Policies(ctx context.Context) ([]map[string]any, error) {
	out := []map[string]any{}
	for _, p := range e.Config.Policies {
		r, _ := e.Config.Repository(p.Repository)
		var at, next, nextTest sql.NullTime
		if err := e.DB.QueryRowContext(ctx, `SELECT (SELECT max(created_at) FROM backup_artifacts WHERE policy_id=$1 AND expired_at IS NULL),next_backup_at,next_test_at FROM backup_policies WHERE id=$1`, p.ID).Scan(&at, &next, &nextTest); err != nil {
			return nil, err
		}
		var last any
		if at.Valid {
			last = at.Time
		}
		offsite := r.Offsite && r.StorageHost != p.Server && at.Valid
		out = append(out, map[string]any{"id": p.ID, "project": p.Project, "environment": p.Environment, "server": p.Server, "kind": p.Kind, "repository": p.Repository, "storage_host": r.StorageHost, "offsite_configured": r.Offsite, "offsite_verified": offsite, "enabled": p.Enabled, "consistency": p.Consistency, "retention": p.Retention, "interval_seconds": p.IntervalSeconds, "restore_test_seconds": p.RestoreTestSeconds, "last_success_at": last, "next_backup_at": next.Time, "next_test_at": nextTest.Time})
	}
	return out, nil
}
func (e *Engine) PublicTargets() []map[string]any {
	out := []map[string]any{}
	for _, t := range e.Config.Targets {
		out = append(out, map[string]any{"id": t.ID, "policy": t.Policy, "server": t.Server, "kind": t.Kind, "test_only": t.TestOnly})
	}
	return out
}
func scheduleKey(p Policy, kind string, t time.Time) string {
	return fmt.Sprintf("schedule:%s:%s:%d", p.ID, kind, t.Unix())
}
