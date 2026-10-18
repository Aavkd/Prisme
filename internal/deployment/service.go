package deployment

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"prisme.local/platform/internal/coordination"
	gh "prisme.local/platform/internal/github"
)

var Steps = []string{"preconditions", "configuration", "pull", "start", "health", "traffic", "observation", "promote"}

type Plan struct {
	BackupArtifact    string            `json:"backup_artifact,omitempty"`
	CoordinationEpoch uint64            `json:"coordination_epoch"`
	SecretHashes      map[string]string `json:"secret_hashes"`
	PreviousActiveJob string            `json:"previous_active_job"`
	Version           int               `json:"version"`
	Manifest          Manifest          `json:"manifest"`
	ManifestHash      string            `json:"manifest_hash"`
	Release           gh.Release        `json:"release"`
	Build             gh.Build          `json:"build"`
	PreviousJob       string            `json:"previous_job"`
	PreviousRelease   string            `json:"previous_release"`
	PreviousImages    map[string]string `json:"previous_images"`
	Images            map[string]string `json:"images"`
	Slot              string            `json:"slot"`
	PreviousSlot      string            `json:"previous_slot"`
	TargetRevision    int               `json:"target_revision"`
	Rollback          bool              `json:"rollback"`
	Blockers          []string          `json:"blockers"`
	Steps             []string          `json:"steps"`
}
type Job struct {
	ID          string     `json:"id"`
	TargetID    string     `json:"target_id"`
	Plan        Plan       `json:"plan"`
	State       string     `json:"state"`
	Revision    int        `json:"revision"`
	Step        int        `json:"step"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
	HeartbeatAt *time.Time `json:"heartbeat_at"`
	ApprovedBy  string     `json:"approved_by"`
	ErrorCode   string     `json:"error_code"`
}
type Target struct {
	Manifest   Manifest `json:"manifest"`
	CurrentJob string   `json:"current_job"`
	Revision   int      `json:"revision"`
	Blockers   []string `json:"blockers"`
}
type Engine struct {
	DB     *sql.DB
	Config Config
	Driver Driver
	Audit  func(context.Context, *sql.Tx, string, string) error
}

func (e *Engine) audit(ctx context.Context, tx *sql.Tx, action, target string) error {
	if e.Audit != nil {
		return e.Audit(ctx, tx, action, target)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_events(actor,action,target,origin) VALUES('deployment-worker',$1,$2,'system')`, action, target)
	return err
}
func (e *Engine) Initialize(ctx context.Context) error {
	for _, m := range e.Config.Targets {
		if err := m.Validate(); err != nil {
			return err
		}
		b, _ := json.Marshal(m)
		_, err := e.DB.ExecContext(ctx, `INSERT INTO deployment_targets(id,manifest_hash,manifest) VALUES($1,$2,$3) ON CONFLICT(id) DO NOTHING`, m.ID, Hash(m), string(b))
		if err != nil {
			return err
		}
		var h string
		if err = e.DB.QueryRowContext(ctx, `SELECT manifest_hash FROM deployment_targets WHERE id=$1`, m.ID).Scan(&h); err != nil {
			return err
		}
		if h != Hash(m) {
			return errors.New("target manifest changed: register a new target revision explicitly")
		}
	}
	return nil
}
func (e *Engine) manifest(id string) (Manifest, error) {
	for _, m := range e.Config.Targets {
		if m.ID == id {
			return m, nil
		}
	}
	return Manifest{}, ErrInvalid
}
func (e *Engine) Targets(ctx context.Context) ([]Target, error) {
	out := []Target{}
	for _, m := range e.Config.Targets {
		v := Target{Manifest: m, Blockers: m.Blockers()}
		if m.BackupRequired && m.BackupPolicy != "" {
			if _, err := backupEvidence(ctx, e.DB, m, ""); err != nil {
				v.Blockers = append(v.Blockers, "verified_backup_required")
			}
		}
		if err := e.DB.QueryRowContext(ctx, `SELECT current_job,revision FROM deployment_targets WHERE id=$1 AND manifest_hash=$2`, m.ID, Hash(m)).Scan(&v.CurrentJob, &v.Revision); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

const jobColumns = `id,target_id,plan,state,revision,step,created_at,expires_at,heartbeat_at,approved_by,error_code`

type scanner interface{ Scan(...any) error }

func scanJob(row scanner) (Job, error) {
	var j Job
	var b []byte
	err := row.Scan(&j.ID, &j.TargetID, &b, &j.State, &j.Revision, &j.Step, &j.CreatedAt, &j.ExpiresAt, &j.HeartbeatAt, &j.ApprovedBy, &j.ErrorCode)
	if err == nil {
		err = json.Unmarshal(b, &j.Plan)
	}
	return j, err
}
func (e *Engine) Job(ctx context.Context, id string) (Job, error) {
	return scanJob(e.DB.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM deployment_jobs WHERE id=$1`, id))
}
func (e *Engine) List(ctx context.Context, page int) ([]Job, error) {
	rows, err := e.DB.QueryContext(ctx, `SELECT `+jobColumns+` FROM deployment_jobs ORDER BY created_at DESC,id DESC LIMIT 50 OFFSET $1`, (page-1)*50)
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

type Request struct {
	Target   string `json:"target"`
	Release  string `json:"release"`
	Rollback bool   `json:"rollback"`
}

func (e *Engine) Prepare(ctx context.Context, req Request, key string) (Job, error) {
	m, err := e.manifest(req.Target)
	if err != nil {
		return Job{}, err
	}
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback()
	// Serialize duplicate requests even if their payload chooses different targets.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,7464))`, key); err != nil {
		return Job{}, err
	}
	var existing, h string
	err = tx.QueryRowContext(ctx, `SELECT id,request_hash FROM deployment_jobs WHERE request_key=$1`, key).Scan(&existing, &h)
	if err == nil {
		if h != Hash(req) {
			return Job{}, ErrConflict
		}
		return scanJob(tx.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM deployment_jobs WHERE id=$1`, existing))
	}
	if err != sql.ErrNoRows {
		return Job{}, err
	}
	p := Plan{Version: 1, Manifest: m, ManifestHash: Hash(m), Slot: "blue", PreviousImages: map[string]string{}, Blockers: m.Blockers(), Steps: Steps, Rollback: req.Rollback}
	if m.BackupRequired && m.BackupPolicy != "" {
		p.BackupArtifact, err = backupEvidence(ctx, tx, m, "")
		if err != nil {
			p.Blockers = append(p.Blockers, "verified_backup_required")
		}
	}
	if e.Config.CoordinationRoot != "" {
		state, err := coordination.Read(e.Config.CoordinationRoot, m.Server)
		if err != nil {
			return Job{}, ErrPrecondition
		}
		p.CoordinationEpoch = state.Epoch
		if state.Active || state.Suspended {
			p.Blockers = append(p.Blockers, "host_reconciliation_required")
		}
	}
	err = tx.QueryRowContext(ctx, `SELECT current_job,active_job,revision FROM deployment_targets WHERE id=$1 AND manifest_hash=$2 FOR UPDATE`, m.ID, p.ManifestHash).Scan(&p.PreviousJob, &p.PreviousActiveJob, &p.TargetRevision)
	if err != nil {
		return Job{}, err
	}
	p.SecretHashes, err = SecretHashes(e.Config.Root, m)
	if err != nil {
		p.Blockers = append(p.Blockers, "secret_references_unavailable")
	}
	if p.PreviousJob != "" {
		prior, err := scanJob(tx.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM deployment_jobs WHERE id=$1`, p.PreviousJob))
		if err != nil {
			return Job{}, err
		}
		p.PreviousRelease = prior.Plan.Release.ID
		p.PreviousImages = prior.Plan.Images
		p.PreviousSlot = prior.Plan.Slot
		if m.Strategy == "blue-green" && p.PreviousSlot == "blue" {
			p.Slot = "green"
		}
	}
	if p.PreviousActiveJob != "" {
		active, err := scanJob(tx.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM deployment_jobs WHERE id=$1`, p.PreviousActiveJob))
		if err != nil {
			return Job{}, err
		}
		p.PreviousSlot = active.Plan.Slot
		p.Slot = "blue"
		if m.Strategy == "blue-green" && p.PreviousSlot == "blue" {
			p.Slot = "green"
		}
	}
	var b []byte
	if req.Rollback {
		// Only releases previously proven healthy on this exact manifest are eligible.
		var previous Plan
		err = tx.QueryRowContext(ctx, `SELECT plan FROM deployment_jobs WHERE target_id=$1 AND state='succeeded' AND plan->'release'->>'id'=$2 AND plan->>'manifest_hash'=$3 ORDER BY completed_at DESC LIMIT 1`, m.ID, req.Release, p.ManifestHash).Scan(&b)
		if err != nil {
			return Job{}, ErrInvalid
		}
		if json.Unmarshal(b, &previous) != nil {
			return Job{}, ErrInvalid
		}
		p.Release = previous.Release
		p.Build = previous.Build
		if Hash(p.SecretHashes) != Hash(previous.SecretHashes) {
			p.Blockers = append(p.Blockers, "rollback_secret_configuration_changed")
		}
	} else {
		err = tx.QueryRowContext(ctx, `SELECT document FROM github_releases WHERE id=$1 AND repository_id=$2`, req.Release, m.RepositoryID).Scan(&b)
		if err != nil {
			return Job{}, ErrInvalid
		}
		if json.Unmarshal(b, &p.Release) != nil {
			return Job{}, ErrInvalid
		}
		err = tx.QueryRowContext(ctx, `SELECT document FROM github_builds WHERE repository_id=$1 AND id=$2`, m.RepositoryID, p.Release.Manifest.RunID).Scan(&b)
		if err != nil {
			return Job{}, err
		}
		if json.Unmarshal(b, &p.Build) != nil {
			return Job{}, ErrInvalid
		}
		if p.Build.Conclusion != "success" || p.Build.Status != "completed" || p.Build.Branch != m.Branch || p.Build.Commit != p.Release.Manifest.Commit || p.Build.Attempt != p.Release.Manifest.RunAttempt {
			return Job{}, ErrInvalid
		}
	}
	p.Images, err = m.Images(p.Release)
	if err != nil {
		return Job{}, err
	}
	if req.Release == p.PreviousRelease && !(req.Rollback && p.PreviousActiveJob != p.PreviousJob) {
		return Job{}, ErrConflict
	}
	idBytes := make([]byte, 32)
	if _, err = rand.Read(idBytes); err != nil {
		return Job{}, err
	}
	id := hex.EncodeToString(idBytes)
	b, _ = json.Marshal(p)
	_, err = tx.ExecContext(ctx, `INSERT INTO deployment_jobs(id,target_id,request_key,request_hash,plan,state,expires_at) VALUES($1,$2,$3,$4,$5,'awaiting_approval',now()+interval '15 minutes')`, id, m.ID, key, Hash(req), string(b))
	if err != nil {
		return Job{}, err
	}
	if err = e.audit(ctx, tx, "deployment.planned", id); err != nil {
		return Job{}, err
	}
	if err = tx.Commit(); err != nil {
		return Job{}, err
	}
	return e.Job(ctx, id)
}

// Decide binds approval to the immutable plan, exact target phrase and revision.
func (e *Engine) Decide(ctx context.Context, id, action, phrase, actor string, revision int) (Job, error) {
	return e.DecideAuthorized(ctx, id, action, phrase, actor, revision, nil)
}

// authorize consumes an action-bound proof in the same transaction as approval.
func (e *Engine) DecideAuthorized(ctx context.Context, id, action, phrase, actor string, revision int, authorize func(*sql.Tx, Job) error) (Job, error) {
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback()
	// All transactions lock target before job to avoid approval/promotion deadlocks.
	var target string
	if err = tx.QueryRowContext(ctx, `SELECT target_id FROM deployment_jobs WHERE id=$1`, id).Scan(&target); err != nil {
		return Job{}, err
	}
	var current, active, h string
	var rev int
	if err = tx.QueryRowContext(ctx, `SELECT current_job,active_job,manifest_hash,revision FROM deployment_targets WHERE id=$1 FOR UPDATE`, target).Scan(&current, &active, &h, &rev); err != nil {
		return Job{}, err
	}
	j, err := scanJob(tx.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM deployment_jobs WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return Job{}, err
	}
	desired := "ready"
	if action == "cancel" {
		desired = "cancelled"
	} else if action != "approve" {
		return Job{}, ErrInvalid
	}
	if j.State == desired && j.Revision == revision+1 {
		return j, nil
	}
	if j.Revision != revision || (j.State != "awaiting_approval" && !(action == "cancel" && j.State == "ready")) {
		return Job{}, ErrConflict
	}
	if action == "approve" {
		var fresh bool
		err = tx.QueryRowContext(ctx, `SELECT expires_at>now() FROM deployment_jobs WHERE id=$1`, id).Scan(&fresh)
		if err != nil {
			return Job{}, err
		}
		m, err := e.manifest(target)
		if err != nil {
			return Job{}, err
		}
		if !fresh || len(m.Blockers()) != 0 || len(j.Plan.Blockers) != 0 || h != Hash(m) || h != j.Plan.ManifestHash || rev != j.Plan.TargetRevision || current != j.Plan.PreviousJob || active != j.Plan.PreviousActiveJob || phrase != target {
			return Job{}, ErrConflict
		}
		if m.BackupRequired {
			if j.Plan.BackupArtifact == "" {
				return Job{}, ErrConflict
			}
			if _, err = backupEvidence(ctx, tx, m, j.Plan.BackupArtifact); err != nil {
				return Job{}, ErrConflict
			}
		}
		var busy bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM deployment_jobs WHERE target_id=$1 AND state IN ('ready','running','uncertain'))`, target).Scan(&busy)
		if err != nil {
			return Job{}, err
		}
		if busy {
			return Job{}, ErrConflict
		}
	}
	if authorize != nil {
		if err = authorize(tx, j); err != nil {
			return Job{}, err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE deployment_jobs SET state=$2,revision=revision+1,approved_by=$3 WHERE id=$1`, id, desired, actor)
	if err != nil {
		return Job{}, err
	}
	if err = e.audit(ctx, tx, "deployment."+action, id); err != nil {
		return Job{}, err
	}
	if err = tx.Commit(); err != nil {
		return Job{}, err
	}
	return e.Job(ctx, id)
}
