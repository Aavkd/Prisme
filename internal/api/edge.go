package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"prisme.local/platform/internal/edge"
	"prisme.local/platform/internal/operations"
)

func (s *Server) ConfigureEdge(ctx context.Context, c edge.Config) error {
	e := &edge.Engine{DB: s.DB, Config: c, Audit: func(ctx context.Context, tx *sql.Tx, action, target string) error {
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
	s.Edge = e
	return nil
}
func (s *Server) edgeRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /v1/edge", s.auth(s.edgeOverview))
	m.HandleFunc("POST /v1/edge/jobs", s.auth(s.prepareEdge))
	m.HandleFunc("GET /v1/edge/jobs/{job}", s.auth(s.edgeDetail))
	m.HandleFunc("POST /v1/edge/jobs/{job}/decision", s.auth(s.edgeDecision))
}
func edgeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		problem(w, 404, "edge_not_found")
	case errors.Is(err, edge.ErrInvalid):
		problem(w, 400, "edge_invalid")
	case errors.Is(err, edge.ErrConflict):
		problem(w, 409, "edge_conflict")
	case errors.Is(err, operations.ErrDenied):
		problem(w, 403, "fresh_proof_required")
	default:
		problem(w, 503, "edge_unavailable")
	}
}
func (s *Server) edgeOverview(w http.ResponseWriter, r *http.Request, _ string) {
	if s.Edge == nil {
		send(w, 200, map[string]any{"enabled": false, "targets": []any{}, "observations": []any{}, "jobs": []any{}, "has_more": false})
		return
	}
	page, ok := githubPage(w, r)
	if !ok {
		return
	}
	jobs, err := s.Edge.Jobs(r.Context(), page)
	if err != nil {
		edgeError(w, err)
		return
	}
	obs, err := s.Edge.Observations(r.Context())
	if err != nil {
		edgeError(w, err)
		return
	}
	send(w, 200, map[string]any{"enabled": true, "targets": s.Edge.Config.PublicTargets(), "observations": obs, "jobs": jobs, "has_more": len(jobs) == 50})
}
func (s *Server) prepareEdge(w http.ResponseWriter, r *http.Request, _ string) {
	if s.Edge == nil {
		problem(w, 503, "edge_disabled")
		return
	}
	var req edge.Request
	if !body(w, r, &req) {
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if !keyPattern.MatchString(key) {
		problem(w, 400, "idempotency_key_required")
		return
	}
	j, err := s.Edge.Prepare(r.Context(), req, key)
	if err != nil {
		edgeError(w, err)
		return
	}
	send(w, 201, map[string]any{"job": j, "confirmation": edge.Confirmation(j)})
}
func (s *Server) edgeDetail(w http.ResponseWriter, r *http.Request, _ string) {
	if s.Edge == nil {
		problem(w, 503, "edge_disabled")
		return
	}
	j, err := s.Edge.Job(r.Context(), r.PathValue("job"))
	if err != nil {
		edgeError(w, err)
		return
	}
	send(w, 200, map[string]any{"job": j, "confirmation": edge.Confirmation(j)})
}
func (s *Server) edgeDecision(w http.ResponseWriter, r *http.Request, _ string) {
	if s.Edge == nil {
		problem(w, 503, "edge_disabled")
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
	j, err := s.Edge.Decide(r.Context(), r.PathValue("job"), req.Action, req.Phrase, req.Revision, func() error {
		if s.Operations == nil {
			return operations.ErrDenied
		}
		return s.Operations.AuthorizeIntent(r, req.Proof, operations.Intent{Action: "edge_approve", Target: r.PathValue("job"), Payload: map[string]any{"revision": req.Revision}})
	})
	if err != nil {
		edgeError(w, err)
		return
	}
	send(w, 200, map[string]any{"job": j, "confirmation": edge.Confirmation(j)})
}
