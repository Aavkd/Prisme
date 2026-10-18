package api

import (
	"encoding/json"
	"net/http"
	"time"

	"prisme.local/platform/internal/deployment"
)

func (s *Server) onboardingRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /v1/onboarding", s.auth(s.onboardingOverview))
	m.HandleFunc("POST /v1/onboarding", s.auth(s.onboardProject))
}

// Projects are created within operator-provisioned hosting targets. The client
// cannot choose a filesystem path, Docker socket, arbitrary repository or port.
func (s *Server) onboardingOverview(w http.ResponseWriter, r *http.Request, _ string) {
	if s.Deployments == nil || s.GitHub == nil {
		send(w, 200, map[string]any{"enabled": false, "targets": []any{}, "projects": []any{}})
		return
	}
	targets, err := s.Deployments.Targets(r.Context())
	if err != nil {
		deploymentError(w, err)
		return
	}
	items := []any{}
	for _, target := range targets {
		m := target.Manifest
		repo, ok := s.GitHub.Repository(m.RepositoryID)
		if !ok || repo.Name != m.Repository || repo.Environment != m.Environment {
			continue
		}
		items = append(items, target)
	}
	rows, err := s.DB.QueryContext(r.Context(), `SELECT p.target_id,p.job_id,p.created_at,j.state FROM project_onboarding p JOIN deployment_jobs j ON j.id=p.job_id ORDER BY p.created_at DESC LIMIT 200`)
	if err != nil {
		deploymentError(w, err)
		return
	}
	defer rows.Close()
	projects := []any{}
	for rows.Next() {
		var target, job, state string
		var at time.Time
		if err = rows.Scan(&target, &job, &at, &state); err != nil {
			deploymentError(w, err)
			return
		}
		projects = append(projects, map[string]any{"target": target, "job": job, "state": state, "created_at": at})
	}
	if err = rows.Err(); err != nil {
		deploymentError(w, err)
		return
	}
	send(w, 200, map[string]any{"enabled": true, "targets": items, "projects": projects})
}

func (s *Server) onboardProject(w http.ResponseWriter, r *http.Request, session string) {
	if s.Deployments == nil || s.GitHub == nil {
		problem(w, 503, "onboarding_disabled")
		return
	}
	if !s.recent(w, r, session) {
		return
	}
	var req struct {
		Target  string `json:"target"`
		Release string `json:"release"`
	}
	if !body(w, r, &req) {
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if !keyPattern.MatchString(key) {
		problem(w, 400, "idempotency_key_required")
		return
	}
	var manifest *deployment.Manifest
	for _, m := range s.Deployments.Config.Targets {
		if m.ID == req.Target {
			manifest = &m
			break
		}
	}
	if manifest == nil {
		problem(w, 404, "target_not_found")
		return
	}
	repo, ok := s.GitHub.Repository(manifest.RepositoryID)
	if !ok || repo.Name != manifest.Repository || repo.Environment != manifest.Environment {
		problem(w, 409, "repository_mapping_required")
		return
	}
	// A plan is durable before project registration. If the response or this
	// transaction is lost, retrying the same key recovers the same plan. Neither
	// endpoint approves or executes it, including when the target is production.
	job, err := s.Deployments.Prepare(r.Context(), deployment.Request{Target: req.Target, Release: req.Release}, "onboarding-"+key)
	if err != nil {
		deploymentError(w, err)
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		deploymentError(w, err)
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), `INSERT INTO project_onboarding(target_id,job_id,request_key) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, req.Target, job.ID, key)
	if err != nil {
		deploymentError(w, err)
		return
	}
	var stored string
	if err = tx.QueryRowContext(r.Context(), `SELECT job_id FROM project_onboarding WHERE target_id=$1`, req.Target).Scan(&stored); err != nil {
		deploymentError(w, err)
		return
	}
	if stored != job.ID {
		problem(w, 409, "project_already_registered")
		return
	}
	if n, _ := result.RowsAffected(); n == 1 {
		if err = audit(r.Context(), tx, "administrator", "project.onboarded", req.Target); err != nil {
			deploymentError(w, err)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		deploymentError(w, err)
		return
	}
	// Generated files are previews, never committed to GitHub or copied remotely.
	compose, err := (&deployment.Docker{Root: "/etc/prisme/deployment"}).Compose(job.Plan)
	if err != nil {
		deploymentError(w, err)
		return
	}
	definition, _ := json.MarshalIndent(job.Plan.Manifest, "", "  ")
	send(w, 201, map[string]any{"job": job, "files": map[string]string{"compose.json": string(compose), "prisme-project.json": string(definition)}})
}
