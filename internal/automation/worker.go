package automation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"prisme.local/platform/internal/observability"
)

func policyBook(p Policy) Runbook {
	b := Runbook{ID: p.ID, Version: p.Version, Target: p.Target, Parameters: map[string]Parameter{}, TimeoutSeconds: p.TimeoutSeconds, MaintenanceUntil: p.MaintenanceUntil}
	restart, rollback := false, false
	for _, a := range p.Actions {
		restart = restart || a == "restart"
		rollback = rollback || a == "rollback"
	}
	n := p.MaxAttempts
	if rollback {
		n--
	}
	if !restart {
		n = 0
	}
	actions := []string{}
	for i := 0; i < n; i++ {
		actions = append(actions, "restart")
	}
	if rollback {
		actions = append(actions, "rollback")
	}
	for attempt, a := range actions {
		delay := min(p.BackoffSeconds*(1<<attempt), 3600)
		b.Steps = append(b.Steps, Step{Action: a, When: "failed", TimeoutSeconds: min(p.TimeoutSeconds, 3600), Release: p.HealthyRelease}, Step{Action: "wait", When: "failed", Seconds: delay, TimeoutSeconds: delay}, Step{Action: "check", When: "failed", Seconds: p.ObservationSeconds, TimeoutSeconds: min(p.TimeoutSeconds, 3600)})
	}
	b.Steps = append(b.Steps, Step{Action: "notify", When: "always", TimeoutSeconds: 5})
	return b
}
func sample(ctx context.Context, q query, rule string) (observability.RuleState, error) {
	var s observability.RuleState
	var b []byte
	err := q.QueryRowContext(ctx, `SELECT document FROM observation_rule_state WHERE rule_id=$1`, rule).Scan(&b)
	if err == nil {
		err = json.Unmarshal(b, &s)
	}
	return s, err
}
func fresh(s observability.RuleState, now time.Time, seconds int) bool {
	return s.LastSample != nil && !s.LastSample.After(now) && now.Sub(*s.LastSample) <= time.Duration(seconds)*time.Second
}
func (e *Engine) policyAllowed(ctx context.Context, q query, p Policy) error {
	if p.Validate(e.Config) != nil {
		return ErrBlocked
	}
	if p.MaintenanceUntil != nil && e.now().Before(*p.MaintenanceUntil) {
		return ErrBlocked
	}
	for _, rule := range p.Dependencies {
		s, err := sample(ctx, q, rule)
		if err != nil || !fresh(s, e.now(), p.FreshnessSeconds) || s.Signal != "healthy" {
			return ErrBlocked
		}
	}
	for _, rule := range p.Exclusions {
		s, err := sample(ctx, q, rule)
		if err != nil || !fresh(s, e.now(), p.FreshnessSeconds) || s.Signal != "healthy" {
			return ErrBlocked
		}
	}
	return nil
}
func (e *Engine) allowed(ctx context.Context, q query, r Run) error {
	c, err := control(ctx, q)
	if err != nil {
		return err
	}
	if c.Stopped || (c.MaintenanceUntil != nil && e.now().Before(*c.MaintenanceUntil)) {
		return ErrBlocked
	}
	t, err := e.Config.Target(r.Target)
	if err != nil || Hash(t) != r.TargetHash {
		return ErrBlocked
	}
	if r.Book.MaintenanceUntil != nil && e.now().Before(*r.Book.MaintenanceUntil) {
		return ErrBlocked
	}
	if r.Policy != nil {
		var latest Policy
		if err = definition(ctx, q, "policy", r.Policy.ID, 0, &latest); err != nil {
			return err
		}
		if Hash(latest) != Hash(r.Policy) || latest.Mode == "supervised" {
			return ErrBlocked
		}
		var breaker bool
		if err = q.QueryRowContext(ctx, `SELECT breaker FROM automation_policy_state WHERE id=$1`, latest.ID).Scan(&breaker); err != nil {
			return err
		}
		if breaker {
			return ErrBlocked
		}
		return e.policyAllowed(ctx, q, latest)
	}
	return nil
}
func (e *Engine) schedule(ctx context.Context) error {
	tx, err := e.transaction(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	c, err := control(ctx, tx)
	if err != nil {
		return err
	}
	if c.Stopped || (c.MaintenanceUntil != nil && e.now().Before(*c.MaintenanceUntil)) {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT document FROM automation_schedules WHERE enabled AND next_at<=$1 ORDER BY next_at LIMIT 100`, e.now())
	if err != nil {
		return err
	}
	schedules := []Schedule{}
	for rows.Next() {
		var b []byte
		var s Schedule
		if err = rows.Scan(&b); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal(b, &s); err != nil {
			rows.Close()
			return err
		}
		schedules = append(schedules, s)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, s := range schedules {
		_, err = e.prepare(ctx, tx, s.Request, fmt.Sprintf("schedule:%s:%d:%s", s.ID, s.Revision, s.NextAt.Format(time.RFC3339Nano)), "schedule", nil, 0)
		if err != nil && err != ErrConflict {
			return err
		}
		if err == ErrConflict {
			if err = e.audit(ctx, tx, "schedule_skipped_busy", s.ID); err != nil {
				return err
			}
		}
		// Coalesce missed ticks. Never replay a backlog after downtime/maintenance.
		s.NextAt = e.now().Add(time.Duration(s.EverySeconds) * time.Second)
		if _, err = tx.ExecContext(ctx, `UPDATE automation_schedules SET next_at=$2,document=$3 WHERE id=$1`, s.ID, s.NextAt, raw(s)); err != nil {
			return err
		}
	}
	rows, err = tx.QueryContext(ctx, `SELECT document FROM (SELECT DISTINCT ON(id) document FROM automation_definitions WHERE kind='policy' ORDER BY id,version DESC) p`)
	if err != nil {
		return err
	}
	policies := []Policy{}
	for rows.Next() {
		var b []byte
		var p Policy
		if err = rows.Scan(&b); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal(b, &p); err != nil {
			rows.Close()
			return err
		}
		policies = append(policies, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, p := range policies {
		if e.policyAllowed(ctx, tx, p) != nil {
			continue
		}
		t, _ := e.Config.Target(p.Target)
		s, err := sample(ctx, tx, t.Rule)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return err
		}
		if !fresh(s, e.now(), p.FreshnessSeconds) || s.Signal != "failing" || s.FailSamples < p.Failures || s.PendingSince == nil || s.LastSample.Sub(*s.PendingSince) > time.Duration(p.WindowSeconds)*time.Second || s.IncidentID == 0 {
			continue
		}
		if s.Scope == nil || s.Scope.Server != t.Server || s.Scope.Project != t.Project || s.Scope.Environment != t.Environment || s.Scope.Service != t.Service {
			continue
		}
		var active bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM incidents WHERE id=$1 AND rule_id=$2 AND status NOT IN ('resolved','closed'))`, s.IncidentID, t.Rule).Scan(&active); err != nil {
			return err
		}
		if !active {
			continue
		}
		var breaker bool
		var until *time.Time
		var previous int64
		if err = tx.QueryRowContext(ctx, `SELECT breaker,cooldown_until,last_incident FROM automation_policy_state WHERE id=$1`, p.ID).Scan(&breaker, &until, &previous); err != nil {
			return err
		}
		if breaker || previous == s.IncidentID || (until != nil && e.now().Before(*until)) {
			continue
		}
		r, err := e.prepare(ctx, tx, Request{}, fmt.Sprintf("policy:%s:incident:%d", p.ID, s.IncidentID), "policy", &p, s.IncidentID)
		if err == ErrConflict {
			continue
		}
		if err != nil {
			return err
		}
		r.LastFailed = true
		if err = e.save(ctx, tx, &r, "triggered", map[string]any{"rule": t.Rule, "samples": s.FailSamples, "last_sample": s.LastSample}); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE automation_policy_state SET last_incident=$2 WHERE id=$1`, p.ID, s.IncidentID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (e *Engine) finish(ctx context.Context, tx *sql.Tx, r *Run, state, code string) error {
	r.State = state
	r.ErrorCode = code
	if r.Policy != nil {
		broken := state == "failed" || state == "uncertain"
		if _, err := tx.ExecContext(ctx, `UPDATE automation_policy_state SET breaker=breaker OR $2,cooldown_until=$3 WHERE id=$1`, r.Policy.ID, broken, e.now().Add(time.Duration(r.Policy.CooldownSeconds)*time.Second)); err != nil {
			return err
		}
		if broken {
			if err := e.event(ctx, tx, *r, "circuit_open", map[string]any{"attempts": r.Attempts, "code": code}); err != nil {
				return err
			}
			// Existing incident notifications remain independent of remediation. Escalate
			// the incident so the notification worker can report required intervention.
			if r.Incident != 0 {
				if _, err := tx.ExecContext(ctx, `UPDATE incidents SET severity='critical',status='reopened',updated_at=$2,revision=revision+1 WHERE id=$1 AND status NOT IN ('resolved','closed')`, r.Incident, e.now()); err != nil {
					return err
				}
			}
		}
	}
	return e.save(ctx, tx, r, state, map[string]any{"code": code, "attempts": r.Attempts})
}
func (e *Engine) Run(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
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
func (e *Engine) Tick(ctx context.Context) error {
	if e.Driver == nil {
		return ErrBlocked
	}
	conn, err := e.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var locked bool
	if err = conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtextextended(current_database()||'.'||current_schema(),731910))`).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return nil
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(c, `SELECT pg_advisory_unlock(hashtextextended(current_database()||'.'||current_schema(),731910))`)
	}()
	if err = e.schedule(ctx); err != nil {
		return err
	}
	rows, err := conn.QueryContext(ctx, `SELECT document FROM automation_runs WHERE state IN ('queued','approval','waiting','running') OR (state='uncertain' AND document->>'error_code'='reconciliation_requested') ORDER BY created_at,id LIMIT 100`)
	if err != nil {
		return err
	}
	runs, err := readRuns(rows)
	if err != nil {
		return err
	}
	for _, r := range runs {
		if err = e.advance(ctx, r.ID); err != nil {
			return err
		}
	}
	return nil
}
func mutation(s Step) bool { return s.Action == "restart" || s.Action == "rollback" }
func (e *Engine) advance(ctx context.Context, key string) error {
	tx, err := e.transaction(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	r, err := scanRun(tx.QueryRowContext(ctx, `SELECT document FROM automation_runs WHERE id=$1`, key))
	if err != nil {
		return err
	}
	if r.State == "uncertain" {
		if r.ErrorCode != "reconciliation_requested" {
			return nil
		}
		reconciler, ok := e.Driver.(interface {
			Reconcile(context.Context, Target, string) error
		})
		if !ok {
			return nil
		}
		target, err := e.Config.Target(r.Target)
		if err != nil {
			return err
		}
		if reconciler.Reconcile(ctx, target, r.ID) != nil {
			return nil
		}
		if err = e.finish(ctx, tx, &r, "failed", "operator_reconciled"); err != nil {
			return err
		}
		return tx.Commit()
	}
	if r.State == "running" {
		if r.Step < len(r.Book.Steps) && mutation(r.Book.Steps[r.Step]) {
			if err = e.finish(ctx, tx, &r, "uncertain", "worker_interrupted"); err != nil {
				return err
			}
			return tx.Commit()
		}
		r.State = "queued" // Replaying a read or a durable wait is safe.
	}
	if !e.now().Before(r.Deadline) {
		if err = e.finish(ctx, tx, &r, "failed", "timeout"); err != nil {
			return err
		}
		return tx.Commit()
	}
	if e.allowed(ctx, tx, r) != nil {
		return nil
	}
	if r.State == "approval" {
		return nil
	}
	if e.now().Before(r.DueAt) {
		return nil
	}
	if r.Step >= len(r.Book.Steps) {
		state, code := "succeeded", ""
		if r.LastFailed {
			state, code = "failed", "health_check_failed"
		}
		if r.Simulation {
			state, code = "simulated", ""
		}
		if err = e.finish(ctx, tx, &r, state, code); err != nil {
			return err
		}
		return tx.Commit()
	}
	s := r.Book.Steps[r.Step]
	if (s.When == "failed" && !r.LastFailed) || (s.When == "succeeded" && r.LastFailed) {
		r.Step++
		if err = e.save(ctx, tx, &r, "skipped", map[string]any{"action": s.Action}); err != nil {
			return err
		}
		return tx.Commit()
	}
	if r.Simulation {
		r.Step++
		if err = e.save(ctx, tx, &r, "simulated_step", map[string]any{"action": s.Action}); err != nil {
			return err
		}
		return tx.Commit()
	}
	if (s.Action == "approval" || (mutation(s) && (r.Policy == nil || r.Policy.Mode != "autonomous"))) && (r.ApprovedStep != r.Step || r.ApprovedUntil == nil || !e.now().Before(*r.ApprovedUntil)) {
		r.State = "approval"
		if err = e.save(ctx, tx, &r, "approval_required", map[string]any{"action": s.Action}); err != nil {
			return err
		}
		return tx.Commit()
	}
	switch s.Action {
	case "wait":
		if r.State != "waiting" {
			r.State = "waiting"
			r.DueAt = e.now().Add(time.Duration(s.Seconds) * time.Second)
		} else {
			r.Step++
			r.State = "queued"
		}
		if err = e.save(ctx, tx, &r, "wait", map[string]any{"due_at": r.DueAt}); err != nil {
			return err
		}
		return tx.Commit()
	case "notify", "approval":
		r.Step++
		r.State = "queued"
		if err = e.save(ctx, tx, &r, s.Action, nil); err != nil {
			return err
		}
		return tx.Commit()
	}
	if mutation(s) {
		if r.Policy != nil && r.Attempts >= r.Policy.MaxAttempts {
			if err = e.finish(ctx, tx, &r, "failed", "attempt_limit"); err != nil {
				return err
			}
			return tx.Commit()
		}
		r.Attempts++
		r.HealthySince = nil
	}
	r.State = "running"
	if err = e.save(ctx, tx, &r, "step_started", map[string]any{"action": s.Action, "attempt": r.Attempts}); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	runCtx, cancel := context.WithTimeout(ctx, min(time.Duration(s.TimeoutSeconds)*time.Second, r.Deadline.Sub(e.now())))
	defer cancel()
	done := make(chan struct{})
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		t := time.NewTicker(100 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-runCtx.Done():
				return
			case <-t.C:
				if e.allowed(runCtx, e.DB, r) != nil {
					cancel()
					return
				}
			}
		}
	}()
	target, _ := e.Config.Target(r.Target)
	var evidence map[string]any
	if e.allowed(runCtx, e.DB, r) != nil {
		err = ErrBlocked
	} else if s.Action == "check" {
		err = e.Driver.Check(runCtx, target)
	} else {
		evidence, err = e.Driver.Execute(runCtx, target, s, fmt.Sprintf("automation_%s_%d", r.ID, r.Step))
	}
	close(done)
	<-watchDone
	if runCtx.Err() != nil && mutation(s) {
		err = ErrUncertain
	}
	if e.allowed(ctx, e.DB, r) != nil {
		if mutation(s) {
			err = ErrUncertain
		} else {
			err = ErrBlocked
		}
	}
	persist, persistCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer persistCancel()
	tx, txErr := e.transaction(persist)
	if txErr != nil {
		return txErr
	}
	defer tx.Rollback()
	if err != nil && !errors.Is(err, ErrHealth) {
		state, code := "failed", "precondition_failed"
		if mutation(s) && !errors.Is(err, ErrBlocked) {
			state, code = "uncertain", "execution_uncertain"
		}
		if txErr = e.event(persist, tx, r, "step_failed", map[string]any{"action": s.Action, "code": code, "evidence": evidence}); txErr != nil {
			return txErr
		}
		if txErr = e.finish(persist, tx, &r, state, code); txErr != nil {
			return txErr
		}
		return tx.Commit()
	}
	if s.Action == "check" {
		if r.LastCheckAt != nil && e.now().Sub(*r.LastCheckAt) > 10*time.Second {
			r.HealthySince = nil
		}
		checkedAt := e.now()
		r.LastCheckAt = &checkedAt
		if err != nil {
			r.HealthySince = nil
			r.LastFailed = true
			if r.Retry < s.Retries {
				r.Retry++
				r.DueAt = e.now().Add(time.Duration(1<<r.Retry) * time.Second)
			} else {
				r.Step++
				r.Retry = 0
			}
		} else {
			if r.HealthySince == nil {
				now := e.now()
				r.HealthySince = &now
			}
			if e.now().Sub(*r.HealthySince) >= time.Duration(s.Seconds)*time.Second {
				r.Step++
				r.LastFailed = false
				r.Retry = 0
				r.HealthySince = nil
			} else {
				r.DueAt = e.now().Add(time.Second)
			}
		}
	} else {
		r.Step++
		r.LastFailed = true
	} // A successful command still needs a check.
	r.State = "queued"
	if txErr = e.save(persist, tx, &r, "step_completed", map[string]any{"action": s.Action, "health_failed": errors.Is(err, ErrHealth), "evidence": evidence}); txErr != nil {
		return txErr
	}
	return tx.Commit()
}
