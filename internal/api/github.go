package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	gh "prisme.local/platform/internal/github"
	"regexp"
	"strconv"
	"time"
)

func (s *Server) githubRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /v1/github", s.auth(s.githubOverview))
	m.HandleFunc("GET /v1/github/repositories/{repository}/builds", s.auth(s.githubBuilds))
	m.HandleFunc("GET /v1/github/repositories/{repository}/releases", s.auth(s.githubReleases))
	m.HandleFunc("GET /v1/github/repositories/{repository}/browse", s.auth(s.githubBrowse))
	m.HandleFunc("GET /v1/github/repositories/{repository}/builds/{run}", s.auth(s.githubBuild))
	m.HandleFunc("GET /v1/github/repositories/{repository}/builds/{run}/jobs/{job}/logs", s.auth(s.githubLogs))
	m.HandleFunc("POST /v1/github/webhook", s.githubWebhook)
}

type githubRepository struct {
	gh.Repository
	LastSync  *time.Time `json:"last_sync"`
	ErrorCode string     `json:"error_code"`
}

func (s *Server) githubOverview(w http.ResponseWriter, r *http.Request, _ string) {
	items := []githubRepository{}
	if s.GitHub != nil {
		for _, repo := range s.GitHub.Config.Repositories {
			item := githubRepository{Repository: repo}
			var last sql.NullTime
			if e := s.DB.QueryRowContext(r.Context(), `SELECT last_sync,error_code FROM github_repositories WHERE id=$1`, repo.ID).Scan(&last, &item.ErrorCode); e != nil {
				problem(w, 503, "unavailable")
				return
			}
			if last.Valid {
				item.LastSync = &last.Time
			}
			items = append(items, item)
		}
	}
	send(w, 200, map[string]any{"enabled": s.GitHub != nil, "repositories": items})
}
func (s *Server) githubRepository(w http.ResponseWriter, r *http.Request) (gh.Repository, bool) {
	if s.GitHub == nil {
		problem(w, 503, "github_disabled")
		return gh.Repository{}, false
	}
	repo, ok := s.GitHub.Repository(r.PathValue("repository"))
	if !ok {
		problem(w, 404, "repository_not_found")
	}
	return repo, ok
}
func githubPage(w http.ResponseWriter, r *http.Request) (int, bool) {
	p := r.URL.Query().Get("page")
	if p == "" {
		return 1, true
	}
	n, e := strconv.Atoi(p)
	if e != nil || n < 1 || n > 1000 {
		problem(w, 400, "invalid_page")
		return 0, false
	}
	return n, true
}
func (s *Server) githubBuilds(w http.ResponseWriter, r *http.Request, _ string) {
	repo, ok := s.githubRepository(w, r)
	if !ok {
		return
	}
	page, ok := githubPage(w, r)
	if !ok {
		return
	}
	rows, e := s.DB.QueryContext(r.Context(), `SELECT document FROM github_builds WHERE repository_id=$1 ORDER BY updated_at DESC,id DESC LIMIT 51 OFFSET $2`, repo.ID, (page-1)*50)
	if e != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer rows.Close()
	items := []gh.Build{}
	for rows.Next() {
		var raw []byte
		var b gh.Build
		if rows.Scan(&raw) != nil || json.Unmarshal(raw, &b) != nil {
			problem(w, 503, "unavailable")
			return
		}
		items = append(items, b)
	}
	if rows.Err() != nil {
		problem(w, 503, "unavailable")
		return
	}
	more := len(items) > 50
	if more {
		items = items[:50]
	}
	send(w, 200, map[string]any{"items": items, "has_more": more})
}
func (s *Server) githubReleases(w http.ResponseWriter, r *http.Request, _ string) {
	repo, ok := s.githubRepository(w, r)
	if !ok {
		return
	}
	page, ok := githubPage(w, r)
	if !ok {
		return
	}
	rows, e := s.DB.QueryContext(r.Context(), `SELECT document FROM github_releases WHERE repository_id=$1 ORDER BY created_at DESC,id DESC LIMIT 51 OFFSET $2`, repo.ID, (page-1)*50)
	if e != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer rows.Close()
	items := []gh.Release{}
	for rows.Next() {
		var raw []byte
		var b gh.Release
		if rows.Scan(&raw) != nil || json.Unmarshal(raw, &b) != nil {
			problem(w, 503, "unavailable")
			return
		}
		items = append(items, b)
	}
	if rows.Err() != nil {
		problem(w, 503, "unavailable")
		return
	}
	more := len(items) > 50
	if more {
		items = items[:50]
	}
	send(w, 200, map[string]any{"items": items, "has_more": more})
}
func githubError(w http.ResponseWriter, e error) {
	if errors.Is(e, gh.ErrInvalid) {
		problem(w, 400, "invalid_github_request")
	} else {
		problem(w, 503, "github_unavailable")
	}
}
func (s *Server) githubBrowse(w http.ResponseWriter, r *http.Request, _ string) {
	repo, ok := s.githubRepository(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	v, e := s.GitHub.Browse(r.Context(), repo, q.Get("kind"), q.Get("ref"), q.Get("path"), q.Get("page"))
	if e != nil {
		githubError(w, e)
		return
	}
	data := map[string]any{}
	switch x := v.(type) {
	case []any:
		data["items"] = x
	case map[string]any:
		data = x
	default:
		problem(w, 503, "github_unavailable")
		return
	}
	send(w, 200, map[string]any{"data": data})
}
func (s *Server) githubBuild(w http.ResponseWriter, r *http.Request, _ string) {
	repo, ok := s.githubRepository(w, r)
	if !ok {
		return
	}
	run := r.PathValue("run")
	var raw []byte
	e := s.DB.QueryRowContext(r.Context(), `SELECT document FROM github_builds WHERE repository_id=$1 AND id=$2`, repo.ID, run).Scan(&raw)
	if errors.Is(e, sql.ErrNoRows) {
		problem(w, 404, "build_not_found")
		return
	}
	if e != nil {
		problem(w, 503, "unavailable")
		return
	}
	page, ok := githubPage(w, r)
	if !ok {
		return
	}
	details, e := s.GitHub.BuildDetails(r.Context(), repo, run, page)
	if e != nil {
		githubError(w, e)
		return
	}
	details["build"] = json.RawMessage(raw)
	send(w, 200, map[string]any{"data": details})
}
func (s *Server) githubLogs(w http.ResponseWriter, r *http.Request, _ string) {
	repo, ok := s.githubRepository(w, r)
	if !ok {
		return
	}
	logs, e := s.GitHub.JobLogs(r.Context(), repo, r.PathValue("run"), r.PathValue("job"))
	if e != nil {
		githubError(w, e)
		return
	}
	send(w, 200, map[string]any{"text": logs})
}

var deliveryPattern = regexp.MustCompile(`^[a-fA-F0-9-]{16,64}$`)

func (s *Server) githubWebhook(w http.ResponseWriter, r *http.Request) {
	if s.GitHub == nil {
		problem(w, 503, "github_disabled")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	data, e := io.ReadAll(r.Body)
	if e != nil {
		problem(w, 413, "webhook_too_large")
		return
	}
	mac := hmac.New(sha256.New, s.GitHub.WebhookSecret)
	mac.Write(data)
	if !hmac.Equal([]byte("sha256="+hex.EncodeToString(mac.Sum(nil))), []byte(r.Header.Get("X-Hub-Signature-256"))) {
		problem(w, 401, "invalid_signature")
		return
	}
	id := r.Header.Get("X-GitHub-Delivery")
	if !deliveryPattern.MatchString(id) {
		problem(w, 400, "invalid_delivery")
		return
	}
	var payload struct {
		Installation struct {
			ID int64 `json:"id"`
		} `json:"installation"`
		Repository struct {
			ID int64 `json:"id"`
		} `json:"repository"`
	}
	if json.Unmarshal(data, &payload) != nil || strconv.FormatInt(payload.Installation.ID, 10) != s.GitHub.Config.InstallationID {
		problem(w, 403, "installation_not_allowed")
		return
	}
	event := r.Header.Get("X-GitHub-Event")
	if event != "ping" && event != "workflow_run" && event != "workflow_job" && event != "push" && event != "pull_request" && event != "release" && event != "installation" && event != "installation_repositories" {
		send(w, 202, map[string]string{"status": "ignored"})
		return
	}
	repoID := strconv.FormatInt(payload.Repository.ID, 10)
	if event != "ping" && event != "installation" && event != "installation_repositories" {
		if _, ok := s.GitHub.Repository(repoID); !ok {
			problem(w, 403, "repository_not_allowed")
			return
		}
	}
	tx, e := s.DB.BeginTx(r.Context(), nil)
	if e != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	hash := Hash(event + ":" + string(data))
	result, e := tx.ExecContext(r.Context(), `INSERT INTO github_deliveries(id,payload_hash) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, hash)
	if e != nil {
		problem(w, 503, "unavailable")
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		var previous string
		if tx.QueryRowContext(r.Context(), `SELECT payload_hash FROM github_deliveries WHERE id=$1`, id).Scan(&previous) != nil {
			problem(w, 503, "unavailable")
			return
		}
		if previous != hash {
			problem(w, 409, "delivery_conflict")
			return
		}
	}
	// Webhooks invalidate snapshots; canonical API state prevents out-of-order regression.
	if _, e = tx.ExecContext(r.Context(), `UPDATE github_repositories SET dirty=true WHERE id=$1 OR $2 IN ('installation','installation_repositories')`, repoID, event); e != nil {
		problem(w, 503, "unavailable")
		return
	}
	if _, e = tx.ExecContext(r.Context(), `DELETE FROM github_deliveries WHERE received_at<now()-interval '30 days'`); e != nil {
		problem(w, 503, "unavailable")
		return
	}
	if tx.Commit() != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 202, map[string]string{"status": "accepted"})
}
