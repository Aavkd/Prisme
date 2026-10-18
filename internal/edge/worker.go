package edge

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"prisme.local/platform/internal/coordination"
)

func (e *Engine) Run(ctx context.Context) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		_ = e.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
func (e *Engine) refresh(ctx context.Context) error {
	observations := map[string]map[string]any{}
	codes := map[string]string{}
	for _, t := range e.Config.Targets {
		data, err := e.Driver.Observe(ctx, e.Config, t)
		if err != nil {
			codes[t.ID] = "observation_failed"
		} else {
			observations[t.ID] = data
		}
	}
	data, err := e.Driver.Discover(ctx, e.Config)
	if err != nil {
		codes["cloudflare"] = "observation_failed"
	} else {
		observations["cloudflare"] = data
	}
	for id, data := range observations {
		b, _ := json.Marshal(data)
		if _, err = e.DB.ExecContext(ctx, `INSERT INTO edge_observations(id,observed_at,data,config_hash) VALUES($1,now(),$2,$3) ON CONFLICT(id) DO UPDATE SET observed_at=now(),data=excluded.data,error_code='',config_hash=excluded.config_hash`, id, string(b), Hash(e.Config)); err != nil {
			return err
		}
	}
	for id, code := range codes {
		if _, err = e.DB.ExecContext(ctx, `INSERT INTO edge_observations(id,observed_at,data,error_code,config_hash) VALUES($1,now(),'{}',$2,$3) ON CONFLICT(id) DO UPDATE SET error_code=excluded.error_code`, id, code, Hash(e.Config)); err != nil {
			return err
		}
	}
	return nil
}

// A dedicated connection owns the singleton worker lock. Running jobs are never
// replayed after a crash. The independent host guard retains ambiguous mutations.
func (e *Engine) Tick(ctx context.Context) error {
	if e.Driver == nil {
		return ErrInvalid
	}
	conn, err := e.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var locked bool
	if err = conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(731810)`).Scan(&locked); err != nil || !locked {
		return err
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(c, `SELECT pg_advisory_unlock(731810)`); err != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	rows, err := conn.QueryContext(ctx, `SELECT `+columns+` FROM edge_jobs WHERE state='running'`)
	if err != nil {
		return err
	}
	interrupted := []Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			rows.Close()
			return err
		}
		interrupted = append(interrupted, j)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, j := range interrupted {
		j.State = "uncertain"
		j.ErrorCode = "worker_interrupted"
		if err = e.save(ctx, j); err != nil {
			return err
		}
	}
	j, err := scanJob(conn.QueryRowContext(ctx, `SELECT `+columns+` FROM edge_jobs WHERE state IN ('preparing','queued') ORDER BY CASE state WHEN 'queued' THEN 0 ELSE 1 END,created_at,id LIMIT 1`))
	if errors.Is(err, sql.ErrNoRows) {
		return e.refresh(ctx)
	}
	if err != nil {
		return err
	}
	t, err := e.Config.Target(j.Target)
	if err != nil || !t.WriteEnabled || j.ConfigHash != Hash(e.Config) || time.Now().After(j.ExpiresAt) {
		j.State = "failed"
		j.ErrorCode = "configuration_or_expiry"
		return e.save(ctx, j)
	}
	work, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if j.State == "preparing" {
		var previous *Snapshot
		if j.Request.Rollback != "" {
			old, err := e.Job(work, j.Request.Rollback)
			if err != nil || !RollbackAvailable(old) || old.Target != j.Target || old.ConfigHash != j.ConfigHash {
				j.State = "failed"
				j.ErrorCode = "rollback_unavailable"
				return e.save(ctx, j)
			}
			previous = &old.Snapshot
		}
		j.Snapshot, j.Plan, err = e.Driver.Prepare(work, e.Config, t, j.Request, previous)
		j.State = "planned"
		if err != nil {
			j.State = "failed"
			j.ErrorCode = "validation_unavailable"
			if errors.Is(err, ErrInvalid) {
				j.ErrorCode = "configuration_invalid"
			}
			if errors.Is(err, ErrConflict) {
				j.ErrorCode = "drift_conflict"
			}
		}
		return e.save(ctx, j)
	}
	lease, err := coordination.Acquire(e.Config.CoordinationRoot, t.Server, "normal", "edge-worker")
	if err != nil {
		j.State = "failed"
		j.ErrorCode = "host_reconciliation_required"
		return e.save(ctx, j)
	}
	stopped := false
	defer func() { _ = lease.Close(stopped) }()
	j.State = "running"
	if err = e.save(ctx, j); err != nil {
		stopped = true
		return err
	}
	j.Revision++
	state, evidence, applyErr := e.Driver.Apply(work, e.Config, t, j.Snapshot)
	j.State = state
	j.Result = evidence
	if state != "succeeded" && state != "rolled_back" && state != "failed" && state != "uncertain" {
		j.State = "uncertain"
	}
	if applyErr != nil {
		j.ErrorCode = "change_failed"
	}
	if j.State == "uncertain" {
		j.ErrorCode = "reconciliation_required"
	}
	finish, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	if err = e.save(finish, j); err != nil {
		return err
	}
	stopped = j.State != "uncertain"
	return nil
}

// Reconciliation is an explicit local operator action after inspecting provider
// state and clearing the independent host guard. It never repeats the mutation.
func (e *Engine) Acknowledge(ctx context.Context, id string) error {
	j, err := e.Job(ctx, id)
	if err != nil || j.State != "uncertain" {
		return ErrConflict
	}
	t, err := e.Config.Target(j.Target)
	if err != nil {
		return err
	}
	l, err := coordination.Acquire(e.Config.CoordinationRoot, t.Server, "normal", "edge-operator")
	if err != nil {
		return err
	}
	defer l.Close(true)
	j.State = "failed"
	j.ErrorCode = "operator_reconciled"
	return e.save(ctx, j)
}
