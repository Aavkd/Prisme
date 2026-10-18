package deployment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"prisme.local/platform/internal/coordination"
	"time"
)

type Driver interface {
	Execute(context.Context, Plan, string) (map[string]any, error)
	Verify(context.Context, Plan) error
}

// A typed, known health failure differs from losing the command transport.
var ErrHealth = errors.New("health_check_failed")
var ErrPrecondition = errors.New("precondition_failed")
var ErrUnknown = errors.New("execution_uncertain")

func (e *Engine) Run(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		for _, m := range e.Config.Targets {
			if ctx.Err() != nil {
				return
			}
			_ = e.RunTarget(ctx, m.ID)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// A session advisory lock serializes workers per target. Interrupted mutation
// steps are NEVER automatically replayed: recovery only observes the runtime.
func (e *Engine) RunTarget(ctx context.Context, target string) (resultError error) {
	if e.Driver == nil {
		return ErrPrecondition
	}
	conn, err := e.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var locked bool
	if err = conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,7465))`, target).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return nil
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(c, `SELECT pg_advisory_unlock(hashtextextended($1,7465))`, target)
	}()
	j, err := scanJob(conn.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM deployment_jobs WHERE target_id=$1 AND state IN ('ready','running','uncertain') ORDER BY created_at LIMIT 1`, target))
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	m, err := e.manifest(target)
	if err != nil {
		return err
	}
	if j.Plan.ManifestHash != Hash(m) || len(m.Blockers()) != 0 {
		return ErrPrecondition
	}
	var hostLease *coordination.Lease
	hostMutationStarted := false
	if e.Config.CoordinationRoot != "" {
		state, err := coordination.Read(e.Config.CoordinationRoot, m.Server)
		if err != nil || state.Active || state.Suspended || state.Epoch != j.Plan.CoordinationEpoch {
			return ErrPrecondition
		}
		hostLease, err = coordination.Acquire(e.Config.CoordinationRoot, m.Server, "normal", "deployment-worker")
		if err != nil {
			return ErrPrecondition
		}
		defer func() {
			stopped := !hostMutationStarted
			if hostMutationStarted && resultError == nil {
				c, done := context.WithTimeout(context.Background(), 3*time.Second)
				defer done()
				var state string
				stopped = e.DB.QueryRowContext(c, `SELECT state FROM deployment_jobs WHERE id=$1`, j.ID).Scan(&state) == nil && state == "succeeded"
			}
			if err := hostLease.Close(stopped); err != nil && resultError == nil {
				resultError = ErrUnknown
			}
		}()
		if hostLease.State.Epoch != j.Plan.CoordinationEpoch+1 {
			return ErrPrecondition
		}
	}
	if j.State == "ready" && m.BackupRequired {
		if j.Plan.BackupArtifact == "" {
			return e.finish(ctx, j, "failed", "verified_backup_required", false)
		}
		if _, err = backupEvidence(ctx, conn, m, j.Plan.BackupArtifact); err != nil {
			return e.finish(ctx, j, "failed", "verified_backup_required", false)
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(m.TimeoutSeconds)*time.Second)
	defer cancel()
	var automated bool
	if err = conn.QueryRowContext(ctx, `SELECT automation_run_id<>'' FROM deployment_jobs WHERE id=$1`, j.ID).Scan(&automated); err != nil {
		return err
	}
	if automated && !e.automationAllowed(runCtx, j.ID) {
		if j.State == "ready" {
			return e.finish(ctx, j, "cancelled", "automation_stopped", false)
		}
		return ErrPrecondition
	}
	if automated {
		go func() {
			t := time.NewTicker(100 * time.Millisecond)
			defer t.Stop()
			for {
				select {
				case <-runCtx.Done():
					return
				case <-t.C:
					if !e.automationAllowed(runCtx, j.ID) {
						cancel()
						return
					}
				}
			}
		}()
	}
	if j.State == "running" {
		if err = e.finish(ctx, j, "uncertain", "worker_interrupted", false); err != nil {
			return err
		}
		j.State = "uncertain"
	}
	if j.State == "uncertain" {
		// Health + immutable images + owned route + a full observation window are
		// required before an interrupted job can be promoted. No mutation here.
		if e.Driver.Verify(runCtx, j.Plan) != nil {
			return nil
		}
		if err = e.observe(runCtx, conn, j); err != nil {
			return nil
		}
		return e.finish(runCtx, j, "succeeded", "", true)
	}
	var fresh bool
	if err = conn.QueryRowContext(ctx, `SELECT expires_at>now() FROM deployment_jobs WHERE id=$1`, j.ID).Scan(&fresh); err != nil {
		return err
	}
	if !fresh {
		return e.finish(ctx, j, "failed", "approval_expired", false)
	}
	// Compete safely with cancellation, which is allowed until this transition.
	result, err := conn.ExecContext(ctx, `UPDATE deployment_jobs SET state='running',revision=revision+1,started_at=now(),heartbeat_at=now() WHERE id=$1 AND state='ready'`, j.ID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return nil
	}
	heartbeatCtx, stop := context.WithCancel(runCtx)
	defer stop()
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-t.C:
				if _, err := e.DB.ExecContext(heartbeatCtx, `UPDATE deployment_jobs SET heartbeat_at=now() WHERE id=$1 AND state='running'`, j.ID); err != nil {
					cancel()
					return
				}
			}
		}
	}()
	for i, step := range Steps {
		if automated && !e.automationAllowed(runCtx, j.ID) {
			return e.finish(ctx, j, "uncertain", "automation_stopped", false)
		}
		if hostLease != nil {
			if err = hostLease.Event(step, "started"); err != nil {
				return ErrUnknown
			}
		}
		if err = conn.PingContext(runCtx); err != nil {
			break
		}
		if err = e.event(runCtx, j.ID, step, "running", map[string]any{}); err != nil {
			break
		}
		if _, err = conn.ExecContext(runCtx, `UPDATE deployment_jobs SET step=$2 WHERE id=$1`, j.ID, i); err != nil {
			break
		}
		var evidence map[string]any
		if step == "promote" {
			return e.finish(runCtx, j, "succeeded", "", true)
		}
		if step == "observation" {
			err = e.observe(runCtx, conn, j)
			evidence = map[string]any{"seconds": m.ObservationSeconds}
		} else {
			hostMutationStarted = true
			evidence, err = e.Driver.Execute(runCtx, j.Plan, step)
		}
		if err != nil {
			state, code := "failed", "precondition_failed"
			if i >= 3 {
				state, code = "uncertain", "execution_uncertain"
			}
			if errors.Is(err, ErrHealth) && runCtx.Err() == nil {
				state, code = "failed", "health_check_failed"
			}
			if errors.Is(err, ErrPrecondition) && i < 3 {
				state, code = "failed", "precondition_failed"
			}
			c, done := context.WithTimeout(context.Background(), 5*time.Second)
			defer done()
			_ = e.event(c, j.ID, step, state, map[string]any{"code": code})
			return e.finish(c, j, state, code, false)
		}
		if (step == "start" && m.Strategy == "recreate") || (step == "traffic" && m.Strategy == "blue-green") {
			if _, err = conn.ExecContext(runCtx, `UPDATE deployment_targets SET active_job=$2 WHERE id=$1`, j.TargetID, j.ID); err != nil {
				break
			}
		}
		if err = e.event(runCtx, j.ID, step, "succeeded", evidence); err != nil {
			break
		}
	}
	// If persistence is unavailable, retain running; a new worker will classify
	// it as uncertain after acquiring the released lock. Never invent a result.
	return err
}
func (e *Engine) observe(ctx context.Context, conn *sql.Conn, j Job) error {
	until := time.Now().Add(time.Duration(j.Plan.Manifest.ObservationSeconds) * time.Second)
	for {
		if err := conn.PingContext(ctx); err != nil {
			return err
		}
		if err := e.Driver.Verify(ctx, j.Plan); err != nil {
			return err
		}
		if !time.Now().Before(until) {
			return nil
		}
		t := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}
func (e *Engine) event(ctx context.Context, id, step, state string, evidence map[string]any) error {
	if evidence == nil {
		evidence = map[string]any{}
	}
	b, _ := json.Marshal(evidence)
	_, err := e.DB.ExecContext(ctx, `INSERT INTO deployment_events(job_id,step,state,evidence) VALUES($1,$2,$3,$4)`, id, step, state, string(b))
	return err
}
func (e *Engine) finish(ctx context.Context, j Job, state, code string, promote bool) error {
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current, h string
	var rev int
	err = tx.QueryRowContext(ctx, `SELECT current_job,manifest_hash,revision FROM deployment_targets WHERE id=$1 FOR UPDATE`, j.TargetID).Scan(&current, &h, &rev)
	if err != nil {
		return err
	}
	if promote {
		if current != j.Plan.PreviousJob || rev != j.Plan.TargetRevision || h != j.Plan.ManifestHash {
			return ErrConflict
		}
		_, err = tx.ExecContext(ctx, `UPDATE deployment_targets SET current_job=$2,active_job=$2,revision=revision+1 WHERE id=$1`, j.TargetID, j.ID)
		if err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE deployment_jobs SET state=$2,error_code=$3,revision=revision+1,completed_at=now(),heartbeat_at=now() WHERE id=$1`, j.ID, state, code)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO deployment_events(job_id,step,state,evidence) VALUES($1,'result',$2,jsonb_build_object('code',$3::text))`, j.ID, state, code)
	if err != nil {
		return err
	}
	if err = e.audit(ctx, tx, "deployment."+state, j.ID); err != nil {
		return err
	}
	return tx.Commit()
}
