package observability

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type RuleState struct {
	Scope         *Scope     `json:"scope,omitempty"`
	Fingerprint   string     `json:"fingerprint"`
	Signal        string     `json:"signal"`
	Reason        string     `json:"reason"`
	LastSample    *time.Time `json:"last_sample"`
	Value         *float64   `json:"value"`
	PendingSince  *time.Time `json:"pending_since"`
	RecoverySince *time.Time `json:"recovery_since"`
	FailSamples   int        `json:"fail_samples"`
	GoodSamples   int        `json:"good_samples"`
	IncidentID    int64      `json:"incident_id"`
}

func (s *RuleState) resetEvidence() {
	s.PendingSince = nil
	s.RecoverySince = nil
	s.FailSamples = 0
	s.GoodSamples = 0
}

// Advance is independent of providers and SQL. Only fresh, distinct, consecutive
// samples establish failure/recovery; missing evidence preserves active incidents.
func Advance(state RuleState, r Rule, p *Point, now time.Time, inhibition string) (RuleState, string) {
	raw, _ := json.Marshal(r)
	sum := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(sum[:])
	if state.Fingerprint != fingerprint {
		state.resetEvidence()
		state.LastSample = nil
		state.Fingerprint = fingerprint
	}
	if !r.Enabled {
		inhibition = "disabled"
	}
	if r.MaintenanceUntil != nil && now.Before(*r.MaintenanceUntil) {
		inhibition = "maintenance"
	}
	if inhibition != "" {
		state.Signal = "inhibited"
		state.Reason = inhibition
		state.resetEvidence()
		return state, ""
	}
	if p == nil || p.Value == nil || p.At.After(now.Add(5*time.Second)) || now.Sub(p.At) > time.Duration(r.FreshnessSeconds)*time.Second {
		state.Signal = "unknown"
		state.Reason = "missing_or_stale_samples"
		state.Value = nil
		state.resetEvidence()
		return state, ""
	}
	if state.LastSample != nil && !p.At.After(*state.LastSample) {
		return state, ""
	}
	if state.LastSample != nil && p.At.Sub(*state.LastSample) > time.Duration(r.FreshnessSeconds)*time.Second {
		state.resetEvidence()
	}
	state.LastSample = &p.At
	state.Value = p.Value
	state.Reason = ""
	failing := r.Operator == "below" && *p.Value < r.OpenThreshold || r.Operator == "above" && *p.Value > r.OpenThreshold
	recovering := r.Operator == "below" && *p.Value >= r.ResolveThreshold || r.Operator == "above" && *p.Value <= r.ResolveThreshold
	if failing {
		state.Signal = "failing"
		state.RecoverySince = nil
		state.GoodSamples = 0
		if state.PendingSince == nil {
			at := p.At
			state.PendingSince = &at
		}
		state.FailSamples++
		if state.IncidentID == 0 && state.FailSamples >= r.MinSamples && p.At.Sub(*state.PendingSince) >= time.Duration(r.ForSeconds)*time.Second {
			return state, "detected"
		}
	} else if recovering {
		state.Signal = "healthy"
		state.PendingSince = nil
		state.FailSamples = 0
		if state.RecoverySince == nil {
			at := p.At
			state.RecoverySince = &at
		}
		state.GoodSamples++
		if state.IncidentID != 0 && state.GoodSamples >= r.MinSamples && p.At.Sub(*state.RecoverySince) >= time.Duration(r.RecoverySeconds)*time.Second {
			return state, "resolved"
		}
	} else {
		state.Signal = "unstable"
		state.resetEvidence()
	}
	return state, ""
}

type Engine struct {
	DB     *sql.DB
	Config Config
	Source *Source
	Now    func() time.Time
}

// Scope IDs used in incident history are immutable. Reassigning an ID would make
// historical links read another service's logs, so a new resource needs a new ID.
func (e *Engine) ValidateStoredScopes(ctx context.Context) error {
	rows, err := e.DB.QueryContext(ctx, "SELECT document FROM observation_rule_state")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var state RuleState
		if err = rows.Scan(&raw); err != nil {
			return err
		}
		if err = json.Unmarshal(raw, &state); err != nil {
			return err
		}
		if state.Scope != nil {
			if scope, ok := e.Config.Scope(state.Scope.ID); ok && scope != *state.Scope {
				return errors.New("observability scope IDs cannot be reassigned")
			}
		}
	}
	return rows.Err()
}

func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		_ = e.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (e *Engine) Tick(ctx context.Context) error {
	for _, r := range e.Config.Rules {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		scope, _ := e.Config.Scope(r.ScopeID)
		now := e.Now()
		p, _ := e.Source.Latest(ctx, scope, r.Metric, now)
		if err := e.Apply(ctx, r, p, now); err != nil {
			return err
		}
	}
	return nil
}
func (e *Engine) Apply(ctx context.Context, r Rule, p *Point, now time.Time) error {
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// One lock covers dependency reads, state changes and incident/audit insertion.
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(731006)"); err != nil {
		return err
	}
	var state RuleState
	var doc []byte
	err = tx.QueryRowContext(ctx, "SELECT document FROM observation_rule_state WHERE rule_id=$1", r.ID).Scan(&doc)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if len(doc) > 0 {
		if err = json.Unmarshal(doc, &state); err != nil {
			return err
		}
	}
	if scope, ok := e.Config.Scope(r.ScopeID); ok {
		if state.Scope != nil && *state.Scope != scope {
			return errors.New("observability rule target cannot be reassigned")
		}
		state.Scope = &scope
	}
	inhibition := ""
	for _, id := range r.InhibitedBy {
		var parent RuleState
		var raw []byte
		var updated time.Time
		err = tx.QueryRowContext(ctx, "SELECT document,updated_at FROM observation_rule_state WHERE rule_id=$1", id).Scan(&raw, &updated)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if len(raw) > 0 {
			if err = json.Unmarshal(raw, &parent); err != nil {
				return err
			}
		}
		if parent.Signal != "healthy" || parent.IncidentID != 0 || now.Sub(updated) > 30*time.Second {
			inhibition = "dependency:" + id
			break
		}
	}
	previousSignal := state.Signal
	state, event := Advance(state, r, p, now, inhibition)
	if event == "detected" {
		// A recurrence within 24h reopens the same correlated incident unless closed.
		var id int64
		err = tx.QueryRowContext(ctx, "SELECT id FROM incidents WHERE rule_id=$1 AND status='resolved' AND resolved_at>$2 ORDER BY id DESC LIMIT 1", r.ID, now.Add(-24*time.Hour)).Scan(&id)
		if err == sql.ErrNoRows {
			err = tx.QueryRowContext(ctx, "INSERT INTO incidents(rule_id,scope_id,title,severity,status,opened_at,updated_at) VALUES($1,$2,$3,$4,'detected',$5,$5) RETURNING id", r.ID, r.ScopeID, r.Title, r.Severity, now).Scan(&id)
		} else if err == nil {
			event = "reopened"
			_, err = tx.ExecContext(ctx, "UPDATE incidents SET status='reopened',resolved_at=NULL,updated_at=$2,revision=revision+1 WHERE id=$1", id, now)
		}
		if err != nil {
			return err
		}
		state.IncidentID = id
	}
	if event == "resolved" {
		_, err = tx.ExecContext(ctx, "UPDATE incidents SET status='resolved',resolved_at=$2,updated_at=$2,revision=revision+1 WHERE id=$1", state.IncidentID, now)
		if err != nil {
			return err
		}
	}
	if event != "" {
		message := fmt.Sprintf("%s: %s; metric=%s; sample=%s", r.ID, event, r.Metric, p.At.Format(time.RFC3339))
		if err = eventAndAudit(ctx, tx, state.IncidentID, now, event, "system:alert-evaluator", message); err != nil {
			return err
		}
		if event == "resolved" {
			state.IncidentID = 0
			state.resetEvidence()
		}
	} else if state.IncidentID != 0 && previousSignal != state.Signal && (state.Signal == "unknown" || state.Signal == "inhibited" || previousSignal == "unknown" || previousSignal == "inhibited") {
		if err = eventAndAudit(ctx, tx, state.IncidentID, now, "signal_changed", "system:alert-evaluator", state.Signal+": "+state.Reason); err != nil {
			return err
		}
	}
	raw, _ := json.Marshal(state)
	_, err = tx.ExecContext(ctx, "INSERT INTO observation_rule_state(rule_id,document,updated_at) VALUES($1,$2,$3) ON CONFLICT(rule_id) DO UPDATE SET document=excluded.document,updated_at=excluded.updated_at", r.ID, string(raw), now)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func eventAndAudit(ctx context.Context, tx *sql.Tx, id int64, now time.Time, kind, actor, message string) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO incident_events(incident_id,occurred_at,kind,actor,message) VALUES($1,$2,$3,$4,$5)", id, now, kind, actor, Redact(message))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO audit_events(actor,action,target,origin,result,correlation_id) VALUES($1,$2,$3,'system','success',$3)", actor, "incident."+kind, fmt.Sprint(id))
	return err
}
