package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"prisme.local/platform/internal/automation"
	"prisme.local/platform/internal/operations"
)

func (s *Server) ConfigureAutomation(ctx context.Context, c automation.Config) error {
	e := &automation.Engine{DB: s.DB, Config: c, Now: s.Now, Audit: func(ctx context.Context, tx *sql.Tx, action, target string) error {
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
	s.Automation = e
	return nil
}
func (s *Server) automationRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /v1/automation", s.auth(s.automationOverview))
	m.HandleFunc("POST /v1/automation/definitions", s.auth(s.automationPublish))
	m.HandleFunc("POST /v1/automation/control", s.auth(s.automationControl))
	m.HandleFunc("POST /v1/automation/runs", s.auth(s.automationPrepare))
	m.HandleFunc("GET /v1/automation/runs/{run}", s.auth(s.automationDetail))
	m.HandleFunc("POST /v1/automation/runs/{run}/decision", s.auth(s.automationDecision))
	m.HandleFunc("POST /v1/automation/schedules", s.auth(s.automationSchedule))
	m.HandleFunc("POST /v1/automation/policies/{policy}/reset", s.auth(s.automationReset))
}
func automationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		problem(w, 404, "automation_not_found")
	case errors.Is(err, automation.ErrInvalid):
		problem(w, 400, "automation_invalid")
	case errors.Is(err, automation.ErrConflict):
		problem(w, 409, "automation_conflict")
	case errors.Is(err, operations.ErrDenied):
		problem(w, 403, "fresh_proof_required")
	case errors.Is(err, automation.ErrBlocked):
		problem(w, 409, "automation_blocked")
	default:
		problem(w, 503, "automation_unavailable")
	}
}
func (s *Server) automationEnabled(w http.ResponseWriter) bool {
	if s.Automation == nil {
		problem(w, 503, "automation_disabled")
		return false
	}
	return true
}
func (s *Server) automationProof(r *http.Request, proof, action, target string, payload map[string]any) func() error {
	return func() error {
		if s.Operations == nil {
			return operations.ErrDenied
		}
		return s.Operations.AuthorizeIntent(r, proof, operations.Intent{Action: action, Target: target, Payload: payload})
	}
}
func (s *Server) automationOverview(w http.ResponseWriter, r *http.Request, _ string) {
	if s.Automation == nil {
		send(w, 200, map[string]any{"enabled": false})
		return
	}
	page, ok := githubPage(w, r)
	if !ok {
		return
	}
	v, err := s.Automation.Overview(r.Context(), page)
	if err != nil {
		automationError(w, err)
		return
	}
	send(w, 200, v)
}
func (s *Server) automationPublish(w http.ResponseWriter, r *http.Request, _ string) {
	if !s.automationEnabled(w) {
		return
	}
	var req struct {
		Kind     string          `json:"kind"`
		Document json.RawMessage `json:"document"`
		Proof    string          `json:"proof"`
	}
	if !body(w, r, &req) {
		return
	}
	var doc map[string]any
	if json.Unmarshal(req.Document, &doc) != nil {
		problem(w, 400, "automation_invalid")
		return
	}
	target, _ := doc["id"].(string)
	err := s.Automation.Publish(r.Context(), req.Kind, req.Document, s.automationProof(r, req.Proof, "automation_publish", target, map[string]any{"kind": req.Kind, "document": doc}))
	if err != nil {
		automationError(w, err)
		return
	}
	send(w, 201, map[string]any{"saved": true})
}
func (s *Server) automationControl(w http.ResponseWriter, r *http.Request, _ string) {
	if !s.automationEnabled(w) {
		return
	}
	var req struct {
		Control json.RawMessage `json:"control"`
		Proof   string          `json:"proof"`
	}
	if !body(w, r, &req) {
		return
	}
	var payload map[string]any
	var desired automation.Control
	d := json.NewDecoder(bytes.NewReader(req.Control))
	d.DisallowUnknownFields()
	if d.Decode(&desired) != nil || json.Unmarshal(req.Control, &payload) != nil {
		problem(w, 400, "automation_invalid")
		return
	}
	c, err := s.Automation.SetControl(r.Context(), desired, s.automationProof(r, req.Proof, "automation_control", "global", payload))
	if err != nil {
		automationError(w, err)
		return
	}
	send(w, 200, map[string]any{"control": c})
}
func (s *Server) automationPrepare(w http.ResponseWriter, r *http.Request, _ string) {
	if !s.automationEnabled(w) {
		return
	}
	var req automation.Request
	if !body(w, r, &req) {
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if !keyPattern.MatchString(key) {
		problem(w, 400, "idempotency_key_required")
		return
	}
	v, err := s.Automation.Prepare(r.Context(), req, key)
	if err != nil {
		automationError(w, err)
		return
	}
	send(w, 201, map[string]any{"run": v})
}
func (s *Server) automationDetail(w http.ResponseWriter, r *http.Request, _ string) {
	if !s.automationEnabled(w) {
		return
	}
	v, err := s.Automation.RunByID(r.Context(), r.PathValue("run"))
	if err != nil {
		automationError(w, err)
		return
	}
	events, err := s.Automation.Events(r.Context(), v.ID)
	if err != nil {
		automationError(w, err)
		return
	}
	send(w, 200, map[string]any{"run": v, "events": events})
}
func (s *Server) automationDecision(w http.ResponseWriter, r *http.Request, _ string) {
	if !s.automationEnabled(w) {
		return
	}
	var req struct {
		Action   string `json:"action"`
		Revision int    `json:"revision"`
		Proof    string `json:"proof"`
	}
	if !body(w, r, &req) {
		return
	}
	v, err := s.Automation.Decide(r.Context(), r.PathValue("run"), req.Action, req.Revision, s.automationProof(r, req.Proof, "automation_"+req.Action, r.PathValue("run"), map[string]any{"revision": req.Revision}))
	if err != nil {
		automationError(w, err)
		return
	}
	send(w, 200, map[string]any{"run": v})
}
func (s *Server) automationSchedule(w http.ResponseWriter, r *http.Request, _ string) {
	if !s.automationEnabled(w) {
		return
	}
	var req struct {
		Schedule json.RawMessage `json:"schedule"`
		Proof    string          `json:"proof"`
	}
	if !body(w, r, &req) {
		return
	}
	var payload map[string]any
	var desired automation.Schedule
	d := json.NewDecoder(bytes.NewReader(req.Schedule))
	d.DisallowUnknownFields()
	if d.Decode(&desired) != nil || json.Unmarshal(req.Schedule, &payload) != nil {
		problem(w, 400, "automation_invalid")
		return
	}
	err := s.Automation.Schedule(r.Context(), desired, s.automationProof(r, req.Proof, "automation_schedule", desired.ID, payload))
	if err != nil {
		automationError(w, err)
		return
	}
	send(w, 200, map[string]any{"saved": true})
}
func (s *Server) automationReset(w http.ResponseWriter, r *http.Request, _ string) {
	if !s.automationEnabled(w) {
		return
	}
	var req struct {
		Proof string `json:"proof"`
	}
	if !body(w, r, &req) {
		return
	}
	err := s.Automation.Reset(r.Context(), r.PathValue("policy"), s.automationProof(r, req.Proof, "automation_reset", r.PathValue("policy"), map[string]any{}))
	if err != nil {
		automationError(w, err)
		return
	}
	send(w, 200, map[string]any{"saved": true})
}
