package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"prisme.local/platform/internal/backup"
	"prisme.local/platform/internal/operations"
)

func (s *Server) backupRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /v1/backups", s.auth(s.backups))
	m.HandleFunc("POST /v1/backups/jobs", s.auth(s.prepareBackup))
	m.HandleFunc("GET /v1/backups/jobs/{job}", s.auth(s.backupDetail))
	m.HandleFunc("POST /v1/backups/jobs/{job}/decision", s.auth(s.backupDecision))
}
func (s *Server) ConfigureBackups(ctx context.Context, c backup.Config) error {
	e := &backup.Engine{DB: s.DB, Config: c, Audit: func(ctx context.Context, tx *sql.Tx, action, target string) error {
		return audit(ctx, tx, "administrator", action, target)
	}}
	if err := e.Initialize(ctx); err != nil {
		return err
	}
	if s.Operations == nil {
		if err := s.ConfigureOperations(ctx, operations.Config{}); err != nil {
			return err
		}
	}
	s.Backups = e
	return nil
}
func backupError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		problem(w, 404, "backup_not_found")
	case errors.Is(err, backup.ErrInvalid):
		problem(w, 400, "backup_invalid")
	case errors.Is(err, backup.ErrConflict):
		problem(w, 409, "backup_conflict")
	case errors.Is(err, operations.ErrDenied):
		problem(w, 403, "fresh_proof_required")
	default:
		problem(w, 503, "backup_unavailable")
	}
}
func (s *Server) backups(w http.ResponseWriter, r *http.Request, _ string) {
	if s.Backups == nil {
		send(w, 200, map[string]any{"enabled": false, "policies": []any{}, "targets": []any{}, "jobs": []any{}, "artifacts": []any{}, "has_more": false})
		return
	}
	page, ok := githubPage(w, r)
	if !ok {
		return
	}
	policies, err := s.Backups.Policies(r.Context())
	if err != nil {
		backupError(w, err)
		return
	}
	jobs, err := s.Backups.Jobs(r.Context(), page)
	if err != nil {
		backupError(w, err)
		return
	}
	artifacts, err := s.Backups.Artifacts(r.Context(), page)
	if err != nil {
		backupError(w, err)
		return
	}
	send(w, 200, map[string]any{"enabled": true, "policies": policies, "targets": s.Backups.PublicTargets(), "jobs": jobs, "artifacts": artifacts, "has_more": len(jobs) == 50 || len(artifacts) == 50})
}
func (s *Server) prepareBackup(w http.ResponseWriter, r *http.Request, _ string) {
	if s.Backups == nil {
		problem(w, 503, "backups_disabled")
		return
	}
	var req backup.Request
	if !body(w, r, &req) {
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if !keyPattern.MatchString(key) {
		problem(w, 400, "idempotency_key_required")
		return
	}
	j, err := s.Backups.Prepare(r.Context(), req, key)
	if err != nil {
		backupError(w, err)
		return
	}
	send(w, 201, map[string]any{"job": j, "confirmation": backup.Confirmation(j)})
}
func (s *Server) backupDetail(w http.ResponseWriter, r *http.Request, _ string) {
	if s.Backups == nil {
		problem(w, 503, "backups_disabled")
		return
	}
	j, err := s.Backups.Job(r.Context(), r.PathValue("job"))
	if err != nil {
		backupError(w, err)
		return
	}
	plan := map[string]any{}
	policies, err := s.Backups.Policies(r.Context())
	if err != nil {
		backupError(w, err)
		return
	}
	for _, p := range policies {
		if p["id"] == j.PolicyID {
			plan["policy"] = p
		}
	}
	for _, t := range s.Backups.PublicTargets() {
		if t["id"] == j.TargetID {
			plan["target"] = t
		}
	}
	if j.ArtifactID != "" {
		a, err := s.Backups.Artifact(r.Context(), j.ArtifactID)
		if err != nil {
			backupError(w, err)
			return
		}
		plan["artifact"] = a
	}
	rows, err := s.DB.QueryContext(r.Context(), `SELECT step,at FROM backup_events WHERE job_id=$1 ORDER BY id LIMIT 200`, j.ID)
	if err != nil {
		backupError(w, err)
		return
	}
	defer rows.Close()
	events := []any{}
	for rows.Next() {
		var step string
		var at time.Time
		if err = rows.Scan(&step, &at); err != nil {
			backupError(w, err)
			return
		}
		events = append(events, map[string]any{"step": step, "at": at})
	}
	if err = rows.Err(); err != nil {
		backupError(w, err)
		return
	}
	send(w, 200, map[string]any{"job": j, "confirmation": backup.Confirmation(j), "events": events, "plan": plan})
}
func (s *Server) backupDecision(w http.ResponseWriter, r *http.Request, _ string) {
	if s.Backups == nil {
		problem(w, 503, "backups_disabled")
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
	j, err := s.Backups.Decide(r.Context(), r.PathValue("job"), req.Action, req.Phrase, req.Revision, func() error {
		if s.Operations == nil {
			return operations.ErrDenied
		}
		return s.Operations.AuthorizeIntent(r, req.Proof, operations.Intent{Action: "backup_approve", Target: r.PathValue("job"), Payload: map[string]any{"revision": req.Revision}})
	})
	if err != nil {
		backupError(w, err)
		return
	}
	send(w, 200, map[string]any{"job": j, "confirmation": backup.Confirmation(j)})
}
