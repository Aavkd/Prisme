package deployment

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"prisme.local/platform/internal/coordination"
)

// Automation ownership follows a rollback into the deployment queue. Another
// deployment worker cannot execute that queued job after a global stop.
func (e *Engine) automationAllowed(ctx context.Context, job string) bool {
	var allowed bool
	err := e.DB.QueryRowContext(ctx, `SELECT automation_run_id='' OR EXISTS(
 SELECT 1 FROM automation_runs r,automation_control c WHERE r.id=deployment_jobs.automation_run_id
 AND r.state='running' AND NOT c.stopped AND (c.maintenance_until IS NULL OR c.maintenance_until<=now())
 AND (r.document->'policy'='null'::jsonb OR r.document->'policy'=(SELECT document FROM automation_definitions WHERE kind='policy' AND id=r.document->'policy'->>'id' ORDER BY version DESC LIMIT 1))
 ) FROM deployment_jobs WHERE id=$1`, job).Scan(&allowed)
	return err == nil && allowed
}

// ReconcileAutomation only retires a linked uncertain job after an explicit
// operator request and host reconciliation. It never replays or promotes it.
func (e *Engine) ReconcileAutomation(ctx context.Context, target, run string) error {
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var locked bool
	if err = tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,7465))`, target).Scan(&locked); err != nil || !locked {
		return ErrPrecondition
	}
	if _, err = tx.ExecContext(ctx, `UPDATE deployment_jobs SET state='failed',error_code='operator_reconciled',completed_at=now(),revision=revision+1 WHERE target_id=$1 AND automation_run_id=$2 AND state IN ('ready','running','uncertain')`, target, run); err != nil {
		return err
	}
	if err = e.audit(ctx, tx, "deployment.automation_reconciled", run); err != nil {
		return err
	}
	return tx.Commit()
}

// ActivePlan is restricted to the exact configured manifest. It includes failed
// deployments that actually reached traffic, which remediation must diagnose.
func (e *Engine) ActivePlan(ctx context.Context, target string) (Plan, error) {
	m, err := e.manifest(target)
	if err != nil || len(m.Blockers()) != 0 {
		return Plan{}, ErrPrecondition
	}
	var active string
	if err = e.DB.QueryRowContext(ctx, `SELECT active_job FROM deployment_targets WHERE id=$1 AND manifest_hash=$2`, target, Hash(m)).Scan(&active); err != nil || active == "" {
		return Plan{}, ErrPrecondition
	}
	j, err := e.Job(ctx, active)
	if err != nil || j.Plan.ManifestHash != Hash(m) {
		return Plan{}, ErrPrecondition
	}
	return j.Plan, nil
}

// RestartService shares both the deployment target lock and independent host
// guard. The fixed Docker verb can only address one verified, owned container.
func (e *Engine) RestartService(ctx context.Context, target, service string) (err error) {
	d, ok := e.Driver.(*Docker)
	if !ok || e.Config.CoordinationRoot == "" {
		return ErrPrecondition
	}
	conn, err := e.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var locked bool
	if err = conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,7465))`, target).Scan(&locked); err != nil || !locked {
		return ErrPrecondition
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(c, `SELECT pg_advisory_unlock(hashtextextended($1,7465))`, target)
	}()
	var busy bool
	if err = conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM deployment_jobs WHERE target_id=$1 AND state IN ('ready','running','uncertain'))`, target).Scan(&busy); err != nil || busy {
		return ErrPrecondition
	}
	p, err := e.ActivePlan(ctx, target)
	if err != nil {
		return err
	}
	found := false
	for _, s := range p.Manifest.Services {
		if s.Name == service && s.Stateless {
			found = true
		}
	}
	if !found {
		return ErrPrecondition
	}
	lease, err := coordination.Acquire(e.Config.CoordinationRoot, p.Manifest.Server, "normal", "automation-worker")
	if err != nil {
		return ErrPrecondition
	}
	started := false
	defer func() {
		closeErr := lease.Close(!started || err == nil)
		if closeErr != nil {
			err = ErrUnknown
		}
	}()
	if !d.secretsMatch(p) {
		return ErrPrecondition
	}
	ids, err := command(ctx, "ps", "-aq", "--filter", "label=com.docker.compose.project="+project(p), "--filter", "label=com.docker.compose.service="+service)
	if err != nil {
		return ErrPrecondition
	}
	container := strings.TrimSpace(string(ids))
	if container == "" || strings.ContainsAny(container, " \r\n\t") {
		return ErrPrecondition
	}
	b, err := command(ctx, "inspect", "--format", `{"image":{{json .Config.Image}},"owner":{{json (index .Config.Labels "dev.mantara.prisme.target")}}}`, container)
	if err != nil {
		return ErrPrecondition
	}
	var actual struct{ Image, Owner string }
	if json.Unmarshal(b, &actual) != nil || actual.Image != p.Images[service] || actual.Owner != target {
		return ErrPrecondition
	}
	if err = lease.Event("restart", "started"); err != nil {
		return ErrPrecondition
	}
	started = true
	if _, err = command(ctx, "restart", "--time", "10", container); err != nil {
		return err
	}
	return lease.Event("restart", "completed")
}
