package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"prisme.local/platform/internal/deployment"
)

func (s *Server) deploymentRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /v1/deployments", s.auth(s.deployments))
	m.HandleFunc("POST /v1/deployments", s.auth(s.prepareDeployment))
	m.HandleFunc("GET /v1/deployments/{job}", s.auth(s.deploymentDetail))
	m.HandleFunc("POST /v1/deployments/{job}/decision", s.auth(s.deploymentDecision))
}
func (s *Server) ConfigureDeployments(ctx context.Context, c deployment.Config) error {
	engine := &deployment.Engine{DB: s.DB, Config: c, Audit: func(ctx context.Context, tx *sql.Tx, action, target string) error {
		return audit(ctx, tx, "administrator", action, target)
	}}
	if err := engine.Initialize(ctx); err != nil {
		return err
	}
	s.Deployments = engine
	return nil
}
func deploymentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		problem(w, 404, "deployment_not_found")
	case errors.Is(err, deployment.ErrConflict):
		problem(w, 409, "deployment_conflict")
	case errors.Is(err, deployment.ErrInvalid):
		problem(w, 400, "invalid_deployment")
	default:
		problem(w, 503, "deployment_unavailable")
	}
}
func (s *Server) deployments(w http.ResponseWriter, r *http.Request, _ string) {
	if s.Deployments == nil {
		send(w, 200, map[string]any{"enabled": false, "targets": []any{}, "items": []any{}, "has_more": false})
		return
	}
	page, ok := githubPage(w, r)
	if !ok {
		return
	}
	targets, err := s.Deployments.Targets(r.Context())
	if err != nil {
		deploymentError(w, err)
		return
	}
	jobs, err := s.Deployments.List(r.Context(), page)
	if err != nil {
		deploymentError(w, err)
		return
	}
	send(w, 200, map[string]any{"enabled": true, "targets": targets, "items": jobs, "has_more": len(jobs) == 50})
}
func (s *Server) prepareDeployment(w http.ResponseWriter, r *http.Request, _ string) {
	if s.Deployments == nil {
		problem(w, 503, "deployments_disabled")
		return
	}
	var req deployment.Request
	if !body(w, r, &req) {
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if !keyPattern.MatchString(key) {
		problem(w, 400, "idempotency_key_required")
		return
	}
	j, err := s.Deployments.Prepare(r.Context(), req, key)
	if err != nil {
		deploymentError(w, err)
		return
	}
	send(w, 201, map[string]any{"job": j})
}
func (s *Server) deploymentDetail(w http.ResponseWriter, r *http.Request, _ string) {
	if s.Deployments == nil {
		problem(w, 503, "deployments_disabled")
		return
	}
	j, err := s.Deployments.Job(r.Context(), r.PathValue("job"))
	if err != nil {
		deploymentError(w, err)
		return
	}
	rows, err := s.DB.QueryContext(r.Context(), `SELECT step,state,evidence,created_at FROM deployment_events WHERE job_id=$1 ORDER BY id LIMIT 200`, j.ID)
	if err != nil {
		deploymentError(w, err)
		return
	}
	defer rows.Close()
	events := []any{}
	for rows.Next() {
		var step, state string
		var b json.RawMessage
		var at time.Time
		if err = rows.Scan(&step, &state, &b, &at); err != nil {
			deploymentError(w, err)
			return
		}
		events = append(events, map[string]any{"step": step, "state": state, "evidence": b, "created_at": at})
	}
	if err = rows.Err(); err != nil {
		deploymentError(w, err)
		return
	}
	send(w, 200, map[string]any{"job": j, "events": events})
}
func (s *Server) deploymentDecision(w http.ResponseWriter, r *http.Request, session string) {
	if s.Deployments == nil {
		problem(w, 503, "deployments_disabled")
		return
	}
	var req struct {
		Action   string `json:"action"`
		Revision int    `json:"revision"`
		Phrase   string `json:"phrase"`
		Proof    string `json:"proof"`
	}
	if !body(w, r, &req) {
		return
	}
	j, err := s.Deployments.Job(r.Context(), r.PathValue("job"))
	if err != nil {
		deploymentError(w, err)
		return
	}
	if req.Action == "approve" && j.Plan.Manifest.Environment == "production" && !opaque.MatchString(req.Proof) {
		problem(w, 403, "production_step_up_unavailable")
		return
	}
	metadata, _ := r.Context().Value(auditContextKey{}).(auditContext)
	var authorize func(*sql.Tx, deployment.Job) error
	if req.Action == "approve" && (j.Plan.Manifest.Environment == "production" || req.Proof != "") {
		authorize = func(tx *sql.Tx, current deployment.Job) error {
			var active bool
			if err := tx.QueryRowContext(r.Context(), `SELECT NOT d.revoked AND s.expires_at>$2 FROM devices d JOIN sessions s ON s.device_id=d.id WHERE s.token_hash=$1 FOR SHARE OF d,s`, session, s.Now()).Scan(&active); err != nil || !active {
				return deployment.ErrConflict
			}
			result, err := tx.ExecContext(r.Context(), `UPDATE action_challenges c SET consumed=true FROM sessions s,devices d WHERE c.id=$1 AND c.session_hash=$2 AND c.job_id=$3 AND c.revision=$4 AND c.approved AND NOT c.consumed AND c.expires_at>$5 AND s.token_hash=c.session_hash AND s.auth_mode='passkey' AND s.expires_at>$5 AND d.id=c.device_id AND NOT d.revoked`, req.Proof, session, current.ID, current.Revision, s.Now())
			if err != nil {
				return err
			}
			n, _ := result.RowsAffected()
			if n != 1 {
				return deployment.ErrConflict
			}
			return audit(r.Context(), tx, metadata.Actor, "action.proof_consumed", current.ID)
		}
	}
	j, err = s.Deployments.DecideAuthorized(r.Context(), j.ID, req.Action, req.Phrase, metadata.Actor, req.Revision, authorize)
	if err != nil {
		deploymentError(w, err)
		return
	}
	send(w, 200, map[string]any{"job": j})
}
