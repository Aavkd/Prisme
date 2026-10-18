package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"prisme.local/platform/internal/observability"
)

func (s *Server) observabilityRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /v1/observability", s.auth(s.observationOverview))
	m.HandleFunc("GET /v1/observability/metrics", s.auth(s.observationMetrics))
	m.HandleFunc("GET /v1/observability/logs", s.auth(s.observationLogs))
	m.HandleFunc("GET /v1/incidents", s.auth(s.incidentList))
	m.HandleFunc("GET /v1/incidents/{id}", s.auth(s.incidentDetail))
	m.HandleFunc("POST /v1/incidents/{id}/events", s.auth(s.incidentUpdate))
}
func (s *Server) observationOverview(w http.ResponseWriter, r *http.Request, _ string) {
	if s.Observability == nil {
		send(w, 200, map[string]any{"enabled": false, "scopes": []any{}, "rules": []any{}, "observed_at": s.Now()})
		return
	}
	rows, err := s.DB.QueryContext(r.Context(), "SELECT rule_id,document,updated_at FROM observation_rule_state")
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer rows.Close()
	states := map[string]any{}
	for rows.Next() {
		var id string
		var raw []byte
		var updated time.Time
		if rows.Scan(&id, &raw, &updated) != nil {
			problem(w, 503, "unavailable")
			return
		}
		var state observability.RuleState
		if json.Unmarshal(raw, &state) != nil {
			problem(w, 503, "unavailable")
			return
		}
		if s.Now().Sub(updated) > 30*time.Second {
			state.Signal = "unknown"
			state.Reason = "evaluator_stale"
			state.Value = nil
		}
		states[id] = state
	}
	if rows.Err() != nil {
		problem(w, 503, "unavailable")
		return
	}
	rules := []any{}
	for _, rule := range s.Observability.Config.Rules {
		state := states[rule.ID]
		if state == nil {
			state = observability.RuleState{Signal: "unknown", Reason: "not_evaluated"}
		}
		rules = append(rules, map[string]any{"definition": rule, "state": state})
	}
	send(w, 200, map[string]any{"enabled": true, "scopes": s.Observability.Config.Scopes, "rules": rules, "observed_at": s.Now()})
}
func (s *Server) observationRange(w http.ResponseWriter, r *http.Request) (observability.Scope, time.Time, time.Time, bool) {
	if s.Observability == nil {
		problem(w, 503, "observability_disabled")
		return observability.Scope{}, time.Time{}, time.Time{}, false
	}
	scope, ok := s.Observability.Config.Scope(r.URL.Query().Get("scope"))
	if !ok {
		problem(w, 400, "invalid_scope")
		return scope, time.Time{}, time.Time{}, false
	}
	start, e1 := time.Parse(time.RFC3339, r.URL.Query().Get("start"))
	end, e2 := time.Parse(time.RFC3339, r.URL.Query().Get("end"))
	if e1 != nil || e2 != nil || !end.After(start) || end.Sub(start) > 24*time.Hour || end.After(s.Now().Add(time.Minute)) || start.Before(s.Now().Add(-90*24*time.Hour)) {
		problem(w, 400, "invalid_time_range")
		return scope, start, end, false
	}
	return scope, start, end, true
}
func (s *Server) observationMetrics(w http.ResponseWriter, r *http.Request, _ string) {
	scope, start, end, ok := s.observationRange(w, r)
	if !ok {
		return
	}
	metric := r.URL.Query().Get("metric")
	if _, ok = observability.Expression(metric, scope); !ok {
		problem(w, 400, "invalid_metric")
		return
	}
	result, err := s.Observability.Source.Metrics(r.Context(), scope, metric, start, end)
	if err != nil {
		problem(w, 503, "metrics_unavailable")
		return
	}
	send(w, 200, map[string]any{"series": result, "start": start, "end": end, "observed_at": s.Now(), "source": "victoriametrics", "step_seconds": max(5, int(end.Sub(start).Seconds()/300))})
}
func (s *Server) observationLogs(w http.ResponseWriter, r *http.Request, hash string) {
	scope, start, end, ok := s.observationRange(w, r)
	if !ok {
		return
	}
	search := r.URL.Query().Get("search")
	level := r.URL.Query().Get("level")
	if len(search) > 200 || strings.ContainsAny(search, "\x00\r\n") || (level != "" && level != "info" && level != "warning" && level != "error" && level != "debug") {
		problem(w, 400, "invalid_filter")
		return
	}
	lines, more, err := s.Observability.Source.Logs(r.Context(), scope, start, end, search, level)
	if err != nil {
		problem(w, 503, "logs_unavailable")
		return
	}
	// Reading/exporting logs is audited without copying log contents or search text.
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err == nil {
		defer tx.Rollback()
		err = audit(r.Context(), tx, "administrator", "logs.read", scope.ID)
		if err == nil {
			err = tx.Commit()
		}
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 200, map[string]any{"lines": lines, "truncated": more, "start": start, "end": end, "observed_at": s.Now(), "source": "loki"})
}

const incidentColumns = "id,rule_id,scope_id,title,severity,status,opened_at,updated_at,resolved_at,revision"

type incidentScanner interface{ Scan(...any) error }

func scanIncident(row incidentScanner) (map[string]any, error) {
	var id int64
	var rule, scope, title, severity, status string
	var opened, updated time.Time
	var resolved sql.NullTime
	var revision int
	err := row.Scan(&id, &rule, &scope, &title, &severity, &status, &opened, &updated, &resolved, &revision)
	var resolvedAt any
	if resolved.Valid {
		resolvedAt = resolved.Time
	}
	return map[string]any{"id": strconv.FormatInt(id, 10), "rule_id": rule, "scope_id": scope, "title": title, "severity": severity, "status": status, "opened_at": opened, "updated_at": updated, "resolved_at": resolvedAt, "revision": revision}, err
}
func (s *Server) incidentList(w http.ResponseWriter, r *http.Request, _ string) {
	cursor := int64(0)
	var err error
	if v := r.URL.Query().Get("cursor"); v != "" {
		cursor, err = strconv.ParseInt(v, 10, 64)
		if err != nil || cursor < 1 {
			problem(w, 400, "invalid_cursor")
			return
		}
	}
	scope := r.URL.Query().Get("scope")
	if scope != "" && !observability.Identifier.MatchString(scope) {
		problem(w, 400, "invalid_scope")
		return
	}
	rows, err := s.DB.QueryContext(r.Context(), "SELECT "+incidentColumns+" FROM incidents WHERE ($1::bigint=0 OR id<$1) AND ($2='' OR scope_id=$2) ORDER BY id DESC LIMIT 51", cursor, scope)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer rows.Close()
	items := []any{}
	next := ""
	for rows.Next() {
		item, e := scanIncident(rows)
		if e != nil {
			problem(w, 503, "unavailable")
			return
		}
		if len(items) == 50 {
			next = items[49].(map[string]any)["id"].(string)
			break
		}
		items = append(items, item)
	}
	if rows.Err() != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 200, map[string]any{"items": items, "next_cursor": next, "observed_at": s.Now()})
}
func incidentID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		problem(w, 400, "invalid_id")
		return 0, false
	}
	return id, true
}
func (s *Server) incidentDetail(w http.ResponseWriter, r *http.Request, _ string) {
	id, ok := incidentID(w, r)
	if !ok {
		return
	}
	item, err := scanIncident(s.DB.QueryRowContext(r.Context(), "SELECT "+incidentColumns+" FROM incidents WHERE id=$1", id))
	if err == sql.ErrNoRows {
		problem(w, 404, "incident_not_found")
		return
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	cursor := int64(0)
	if v := r.URL.Query().Get("cursor"); v != "" {
		cursor, err = strconv.ParseInt(v, 10, 64)
		if err != nil || cursor < 1 {
			problem(w, 400, "invalid_cursor")
			return
		}
	}
	rows, err := s.DB.QueryContext(r.Context(), "SELECT id,occurred_at,kind,actor,message FROM incident_events WHERE incident_id=$1 AND id>$2 ORDER BY id LIMIT 201", id, cursor)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer rows.Close()
	events := []any{}
	next := ""
	for rows.Next() {
		var eid int64
		var at time.Time
		var kind, actor, message string
		if rows.Scan(&eid, &at, &kind, &actor, &message) != nil {
			problem(w, 503, "unavailable")
			return
		}
		if len(events) == 200 {
			next = events[199].(map[string]any)["id"].(string)
			break
		}
		events = append(events, map[string]any{"id": strconv.FormatInt(eid, 10), "at": at, "kind": kind, "actor": actor, "message": message})
	}
	if rows.Err() != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 200, map[string]any{"incident": item, "events": events, "next_cursor": next, "observed_at": s.Now()})
}
func (s *Server) incidentUpdate(w http.ResponseWriter, r *http.Request, _ string) {
	id, ok := incidentID(w, r)
	if !ok {
		return
	}
	var req struct {
		Action   string `json:"action"`
		Note     string `json:"note"`
		Revision int    `json:"revision"`
	}
	if !body(w, r, &req) {
		return
	}
	if req.Revision < 1 || len(req.Note) > 2000 || strings.ContainsRune(req.Note, 0) {
		problem(w, 400, "invalid_event")
		return
	}
	if req.Action != "acknowledge" && req.Action != "diagnose" && req.Action != "monitor" && req.Action != "close" && req.Action != "note" || req.Action == "note" && strings.TrimSpace(req.Note) == "" {
		problem(w, 400, "invalid_event")
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	var state string
	var revision int
	err = tx.QueryRowContext(r.Context(), "SELECT status,revision FROM incidents WHERE id=$1 FOR UPDATE", id).Scan(&state, &revision)
	if err == sql.ErrNoRows {
		problem(w, 404, "incident_not_found")
		return
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	if revision != req.Revision {
		problem(w, 409, "revision_conflict")
		return
	}
	target := map[string]string{"acknowledge": "acknowledged", "diagnose": "diagnosing", "monitor": "monitoring", "close": "closed", "note": state}[req.Action]
	if req.Action == "close" && state != "resolved" || req.Action != "note" && req.Action != "close" && (state == "closed" || state == "resolved") {
		problem(w, 409, "invalid_transition")
		return
	}
	if target == state && req.Action != "note" {
		problem(w, 409, "invalid_transition")
		return
	}
	message := observability.Redact(strings.TrimSpace(req.Note))
	if message == "" {
		message = req.Action
	}
	metadata, _ := r.Context().Value(auditContextKey{}).(auditContext)
	actor := metadata.Actor
	if actor == "" {
		actor = "administrator"
	}
	_, err = tx.ExecContext(r.Context(), "UPDATE incidents SET status=$2,updated_at=$3,revision=revision+1 WHERE id=$1", id, target, s.Now())
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "INSERT INTO incident_events(incident_id,occurred_at,kind,actor,message) VALUES($1,$2,$3,$4,$5)", id, s.Now(), req.Action, actor, message)
	}
	if err == nil {
		err = audit(r.Context(), tx, actor, "incident."+req.Action, strconv.FormatInt(id, 10))
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 200, map[string]any{"status": target, "revision": revision + 1})
}
