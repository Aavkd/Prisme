package backup

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"prisme.local/platform/internal/coordination"
)

func (e *Engine) Run(ctx context.Context) {
	t := time.NewTicker(10 * time.Second)
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

// Acknowledgement is a local operator action, after inspecting and reconciling
// host processes/repository locks. It does not retry a command or unlock Restic.
func (e *Engine) Acknowledge(ctx context.Context, id string) error {
	j, err := e.Job(ctx, id)
	if err != nil || j.State != "uncertain" {
		return ErrConflict
	}
	p, err := e.Config.Policy(j.PolicyID)
	if err != nil {
		return err
	}
	servers := []string{p.Server}
	if j.Kind == "restore" {
		t, err := e.Config.Target(j.TargetID)
		if err != nil {
			return err
		}
		if t.Server != p.Server {
			servers = append(servers, t.Server)
		}
	}
	sort.Strings(servers)
	leases := []*coordination.Lease{}
	defer func() {
		for _, l := range leases {
			_ = l.Close(true)
		}
	}()
	for _, server := range servers {
		l, err := coordination.Acquire(e.Config.CoordinationRoot, server, "normal", "backup-operator")
		if err != nil {
			return err
		}
		leases = append(leases, l)
	}
	return e.finish(ctx, j, "failed", "operator_reconciled", Result{})
}

// One dedicated DB connection serializes all backup workers, including repository
// retention and restore. It is never returned to the pool with a live lock.
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
	if err = conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(731707)`).Scan(&locked); err != nil || !locked {
		return err
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(c, `SELECT pg_advisory_unlock(731707)`); err != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	// Never replay work that may have changed a repository or created a database.
	rows, err := conn.QueryContext(ctx, `SELECT `+columns+` FROM backup_jobs WHERE state='running'`)
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
		if err = e.finish(ctx, j, "uncertain", "worker_interrupted", Result{Evidence: map[string]any{}}); err != nil {
			return err
		}
	}
	if err = e.schedule(ctx); err != nil {
		return err
	}
	j, err := scanJob(conn.QueryRowContext(ctx, `SELECT `+columns+` FROM backup_jobs WHERE state='queued' ORDER BY created_at,id LIMIT 1`))
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	p, err := e.Config.Policy(j.PolicyID)
	if err != nil || !p.Enabled || j.ConfigHash != e.configHash(p) {
		return e.finish(ctx, j, "failed", "configuration_changed", Result{})
	}
	if !j.Scheduled && time.Now().After(j.ExpiresAt) {
		return e.finish(ctx, j, "failed", "approval_expired", Result{})
	}
	servers := []string{p.Server}
	if j.Kind == "restore" {
		t, _ := e.Config.Target(j.TargetID)
		if t.Server != p.Server {
			servers = append(servers, t.Server)
		}
	}
	sort.Strings(servers)
	leases := []*coordination.Lease{}
	stopped := true
	defer func() {
		for i := len(leases) - 1; i >= 0; i-- {
			_ = leases[i].Close(stopped)
		}
	}()
	for _, server := range servers {
		l, err := coordination.Acquire(e.Config.CoordinationRoot, server, "normal", "backup-worker")
		if err != nil {
			return err
		}
		leases = append(leases, l)
	}
	res, err := conn.ExecContext(ctx, `UPDATE backup_jobs SET state='running',started_at=now(),revision=revision+1 WHERE id=$1 AND state='queued'`, j.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(p.TimeoutSeconds)*time.Second)
	defer cancel()
	// Loss of DB authority cancels the command process group. There is no lease
	// timeout that can grant another worker permission to replay this mutation.
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				if conn.PingContext(runCtx) != nil {
					cancel()
					return
				}
			}
		}
	}()
	step := func(step string) error {
		for _, l := range leases {
			if err := l.Event(step, "started"); err != nil {
				return err
			}
		}
		tx, err := e.DB.BeginTx(runCtx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err = tx.ExecContext(runCtx, `UPDATE backup_jobs SET step=$2 WHERE id=$1`, j.ID, step); err != nil {
			return err
		}
		if _, err = tx.ExecContext(runCtx, `INSERT INTO backup_events(job_id,step) VALUES($1,$2)`, j.ID, step); err != nil {
			return err
		}
		if err = e.audit(runCtx, tx, "backup."+step, j.ID); err != nil {
			return err
		}
		return tx.Commit()
	}
	var a *Artifact
	if j.Kind == "retention" {
		rows, err := e.DB.QueryContext(runCtx, `SELECT snapshot_id FROM backup_artifacts WHERE policy_id=$1 AND expired_at IS NULL ORDER BY created_at DESC,id DESC LIMIT 10000`, p.ID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var snapshot string
			if err = rows.Scan(&snapshot); err != nil {
				rows.Close()
				return err
			}
			j.KnownSnapshots = append(j.KnownSnapshots, snapshot)
			if len(j.ProtectedSnapshots) < p.Retention.KeepLast {
				j.ProtectedSnapshots = append(j.ProtectedSnapshots, snapshot)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	if j.Kind == "restore" {
		v, err := scanArtifact(e.DB.QueryRowContext(runCtx, `SELECT `+artifactColumns+` FROM backup_artifacts WHERE id=$1 AND policy_id=$2 AND config_hash=$3 AND expired_at IS NULL`, j.ArtifactID, p.ID, j.ConfigHash))
		if err != nil {
			return e.finish(ctx, j, "failed", "artifact_unavailable", Result{})
		}
		a = &v
	}
	stopped = false
	result, runErr := e.Driver.Execute(runCtx, e.Config, p, j, a, step)
	state, code := "succeeded", ""
	if runErr != nil {
		state, code = "failed", "operation_failed"
		if errors.Is(runErr, ErrInvalid) {
			code = "precondition_failed"
		}
		if runCtx.Err() != nil {
			state, code = "uncertain", "execution_interrupted"
		}
	}
	// Known command completion can release the host even after a nonzero exit.
	// Cancellation keeps it blocked until the operator reconciles processes.
	stopped = runCtx.Err() == nil
	finishCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	if err = e.finish(finishCtx, j, state, code, result); err != nil {
		stopped = false
		return err
	}
	if state == "succeeded" && j.Kind == "backup" && j.Scheduled {
		_, err = e.prepare(finishCtx, Request{Policy: p.ID, Kind: "retention"}, "retention:"+j.ID, true)
	}
	return err
}
func (e *Engine) schedule(ctx context.Context) error {
	for _, p := range e.Config.Policies {
		if !p.Enabled {
			continue
		}
		var next, test time.Time
		var pending bool
		if err := e.DB.QueryRowContext(ctx, `SELECT next_backup_at,next_test_at,suspended OR EXISTS(SELECT 1 FROM backup_jobs WHERE policy_id=$1 AND state IN ('queued','running','uncertain')) FROM backup_policies WHERE id=$1`, p.ID).Scan(&next, &test, &pending); err != nil {
			return err
		}
		if pending {
			continue
		}
		var req Request
		var due time.Time
		// Restore tests take priority once a verified snapshot exists.
		if p.RestoreTestSeconds > 0 && !test.After(time.Now()) {
			var artifact string
			err := e.DB.QueryRowContext(ctx, `SELECT id FROM backup_artifacts WHERE policy_id=$1 AND expired_at IS NULL ORDER BY created_at DESC LIMIT 1`, p.ID).Scan(&artifact)
			if err == nil {
				req = Request{Policy: p.ID, Kind: "restore", Artifact: artifact, Target: p.TestTarget}
				due = test
			} else if err != sql.ErrNoRows {
				return err
			}
		}
		if req.Kind == "" && p.IntervalSeconds > 0 && !next.After(time.Now()) {
			req = Request{Policy: p.ID, Kind: "backup"}
			due = next
		}
		if req.Kind != "" {
			if _, err := e.prepare(ctx, req, scheduleKey(p, req.Kind, due), true); err != nil && !errors.Is(err, ErrConflict) {
				return err
			}
		}
	}
	return nil
}
func (e *Engine) finish(ctx context.Context, j Job, state, code string, result Result) error {
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if result.Evidence == nil {
		result.Evidence = map[string]any{}
	}
	raw, _ := json.Marshal(result.Evidence)
	if _, err = tx.ExecContext(ctx, `UPDATE backup_jobs SET state=$2,error_code=$3,evidence=$4,finished_at=now(),revision=revision+1 WHERE id=$1`, j.ID, state, code, string(raw)); err != nil {
		return err
	}
	if state == "succeeded" && j.Kind == "backup" {
		if !snapshotID.MatchString(result.Snapshot) || result.Bytes < 0 {
			return ErrInvalid
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO backup_artifacts(id,policy_id,job_id,config_hash,snapshot_id,bytes,created_at,verified_at) VALUES($1,$2,$3,$4,$5,$6,(SELECT started_at FROM backup_jobs WHERE id=$3),now())`, id(), j.PolicyID, j.ID, j.ConfigHash, result.Snapshot, result.Bytes); err != nil {
			return err
		}
	}
	if state == "succeeded" && j.Kind == "retention" {
		for _, protected := range j.ProtectedSnapshots {
			found := false
			for _, id := range result.Retained {
				if id == protected {
					found = true
				}
			}
			if !found {
				return ErrConflict
			}
		}
		rows, err := tx.QueryContext(ctx, `SELECT id,snapshot_id FROM backup_artifacts WHERE policy_id=$1 AND expired_at IS NULL`, j.PolicyID)
		if err != nil {
			return err
		}
		retained := map[string]bool{}
		for _, s := range result.Retained {
			retained[s] = true
		}
		expired := []string{}
		for rows.Next() {
			var id, s string
			if err = rows.Scan(&id, &s); err != nil {
				rows.Close()
				return err
			}
			if !retained[s] {
				expired = append(expired, id)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, id := range expired {
			if _, err = tx.ExecContext(ctx, `UPDATE backup_artifacts SET expired_at=now() WHERE id=$1`, id); err != nil {
				return err
			}
		}
	}
	if err = e.audit(ctx, tx, "backup."+state, j.ID); err != nil {
		return err
	}
	// Reuse incident/FCM delivery with an explicit rule id. No command output or
	// database contents are sent. A successful backup resolves backup failures.
	rule := "backup-" + j.PolicyID + "-" + j.Kind
	if state == "failed" || state == "uncertain" {
		var incident string
		err = tx.QueryRowContext(ctx, `INSERT INTO incidents(rule_id,scope_id,title,severity,status,opened_at,updated_at) VALUES($1,$2,'Backup operation requires attention','degraded','detected',now(),now()) ON CONFLICT(rule_id) WHERE status NOT IN ('resolved','closed') DO UPDATE SET updated_at=now(),revision=incidents.revision+1 RETURNING id::text`, rule, j.PolicyID).Scan(&incident)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO incident_events(incident_id,occurred_at,kind,actor,message) VALUES($1,now(),'detected','backup-worker',$2)`, incident, code+" · "+j.ID); err != nil {
			return err
		}
	} else if state == "succeeded" {
		if _, err = tx.ExecContext(ctx, `WITH resolved AS (UPDATE incidents SET status='resolved',resolved_at=now(),updated_at=now(),revision=revision+1 WHERE rule_id=$1 AND status NOT IN ('resolved','closed') RETURNING id) INSERT INTO incident_events(incident_id,occurred_at,kind,actor,message) SELECT id,now(),'resolved','backup-worker','Operation verified' FROM resolved`, rule); err != nil {
			return err
		}
	}
	return tx.Commit()
}
