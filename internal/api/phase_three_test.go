package api_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	gh "prisme.local/platform/internal/github"
	"strings"
	"testing"
	"time"
)

type githubFixtureProvider struct {
	runs           map[string]gh.Run
	manifests      map[string]gh.Manifest
	unavailable    bool
	artifactDigest string
	expired        bool
}

func (p *githubFixtureProvider) JSON(_ context.Context, path string, out any) error {
	if p.unavailable {
		return gh.ErrUnavailable
	}
	var value any
	var name string
	for n := range p.runs {
		if strings.HasPrefix(path, "/repos/"+n+"/") {
			name = n
		}
	}
	if name == "" {
		return gh.ErrInvalid
	}
	run := p.runs[name]
	switch {
	case strings.Contains(path, "/actions/runs?"):
		value = map[string]any{"workflow_runs": []gh.Run{run}}
	case strings.Contains(path, "/artifacts?"):
		value = map[string]any{"artifacts": []gh.Artifact{{ID: 55, Name: fmt.Sprintf("prisme-release-%d", run.Attempt), Size: 500, Digest: p.artifactDigest, Expired: p.expired}}}
	case strings.Contains(path, "/jobs?"):
		value = map[string]any{"jobs": []any{map[string]any{"id": 90, "run_id": run.ID, "name": "Build", "steps": []any{map[string]any{"name": "Publish", "conclusion": "success"}}}}}
	case strings.Contains(path, "/actions/jobs/"):
		value = map[string]any{"run_id": run.ID}
	case strings.Contains(path, "/branches?"):
		value = []any{map[string]any{"name": "main", "commit": map[string]any{"sha": run.SHA}, "secret": "private"}}
	default:
		return gh.ErrInvalid
	}
	b, _ := json.Marshal(value)
	return json.Unmarshal(b, out)
}
func (p *githubFixtureProvider) Download(_ context.Context, path string, _ int64) ([]byte, error) {
	if strings.HasSuffix(path, "/logs") {
		return []byte("build complete password=private-value"), nil
	}
	for name, m := range p.manifests {
		if strings.HasPrefix(path, "/repos/"+name+"/") {
			var b bytes.Buffer
			z := zip.NewWriter(&b)
			file, _ := z.Create("prisme-release.json")
			json.NewEncoder(file).Encode(m)
			z.Close()
			return b.Bytes(), nil
		}
	}
	return nil, gh.ErrInvalid
}
func setupGithub(t *testing.T, f *fixture) (*gh.Service, *githubFixtureProvider) {
	t.Helper()
	repos := []gh.Repository{{ID: "11", Name: "fixture/sounding-lines", Environment: "production", WorkflowID: "70"}, {ID: "12", Name: "fixture/arpent", Environment: "production", WorkflowID: "70"}}
	p := &githubFixtureProvider{runs: map[string]gh.Run{}, manifests: map[string]gh.Manifest{}}
	for i, r := range repos {
		run := gh.Run{ID: int64(100 + i), WorkflowID: 70, Name: "Publish immutable images", SHA: strings.Repeat("a", 40), Branch: "main", Status: "completed", Conclusion: "success", Attempt: 1, CreatedAt: time.Now().UTC().Add(-time.Minute), UpdatedAt: time.Now().UTC()}
		run.Repository.ID = int64(11 + i)
		run.HeadRepository.ID = run.Repository.ID
		run.Event = "push"
		run.Actor.Login = "fixture-builder"
		p.runs[r.Name] = run
		p.manifests[r.Name] = gh.Manifest{Version: 1, Repository: r.Name, Commit: run.SHA, RunID: fmt.Sprint(run.ID), RunAttempt: 1, Environment: r.Environment, Images: []gh.Image{{Service: "web", Reference: "ghcr.io/fixture/web@sha256:" + strings.Repeat("b", 64)}}}
	}
	s := &gh.Service{DB: f.db, Config: gh.Config{InstallationID: "7", Repositories: repos}, Provider: p, WebhookSecret: []byte(strings.Repeat("s", 32))}
	if e := s.Initialize(context.Background()); e != nil {
		t.Fatal(e)
	}
	f.app.GitHub = s
	return s, p
}
func TestPhaseThreeBuildReleaseCorrelation(t *testing.T) {
	f := newFixture(t)
	f.call("GET", "/v1/github", nil, 401)
	f.dev()
	if f.call("GET", "/v1/github", nil, 200)["enabled"] != false {
		t.Fatal("disabled integration is not explicit")
	}
	s, p := setupGithub(t, f)
	s.Poll(context.Background())
	for i, r := range s.Config.Repositories {
		root := "/v1/github/repositories/" + r.ID
		builds := f.call("GET", root+"/builds", nil, 200)["items"].([]any)
		releases := f.call("GET", root+"/releases", nil, 200)["items"].([]any)
		if len(builds) != 1 || len(releases) != 1 {
			t.Fatalf("pilot %s missing build/release", r.Name)
		}
		build := builds[0].(map[string]any)
		release := releases[0].(map[string]any)["manifest"].(map[string]any)
		if build["release_state"] != "linked" || release["commit"] != build["commit"] || release["environment"] != r.Environment || release["run_id"] != build["id"] {
			t.Fatal("incorrect provenance")
		}
		details := f.call("GET", fmt.Sprintf("%s/builds/%d", root, 100+i), nil, 200)["data"].(map[string]any)
		if details["jobs"] == nil || details["artifacts"] == nil {
			t.Fatal("build detail absent")
		}
		logs := f.call("GET", fmt.Sprintf("%s/builds/%d/jobs/90/logs", root, 100+i), nil, 200)["text"].(string)
		if strings.Contains(logs, "private-value") || !strings.Contains(logs, "REDACTED") {
			t.Fatal("logs leaked")
		}
	}
	f.call("GET", "/v1/github/repositories/99/builds", nil, 404)
	f.call("GET", "/v1/github/repositories/11/builds?page=-1", nil, 400)
	f.call("GET", "/v1/github/repositories/11/builds/999", nil, 404)
	f.call("GET", "/v1/github/repositories/11/builds/999/jobs/90/logs", nil, 400)
	f.call("GET", "/v1/github/repositories/11/browse?kind=branches", nil, 200)
	// Restart and missed webhook: polling recovers without duplicating releases/audit.
	s = &gh.Service{DB: f.db, Config: s.Config, Provider: p}
	if s.Initialize(context.Background()) != nil {
		t.Fatal("restart")
	}
	f.db.Exec(`UPDATE github_repositories SET last_sync=now()-interval '1 hour'`)
	s.Poll(context.Background())
	var count int
	f.db.QueryRow(`SELECT count(*) FROM github_releases`).Scan(&count)
	if count != 2 {
		t.Fatal("duplicate release")
	}
	f.db.QueryRow(`SELECT count(*) FROM audit_events WHERE action='release.linked'`).Scan(&count)
	if count != 2 {
		t.Fatal("duplicate audit")
	}
	// A new attempt must never inherit an earlier successful attempt's image.
	name := s.Config.Repositories[0].Name
	run := p.runs[name]
	run.Attempt = 2
	run.UpdatedAt = time.Now().UTC()
	p.runs[name] = run
	f.db.Exec(`UPDATE github_repositories SET dirty=true`)
	s.Poll(context.Background())
	build := f.call("GET", "/v1/github/repositories/11/builds", nil, 200)["items"].([]any)[0].(map[string]any)
	if build["release_state"] != "unavailable" || build["attempt"] != float64(2) {
		t.Fatal("stale release inherited")
	}
	// Old API snapshots cannot regress the run's attempt or status.
	run.Attempt = 1
	run.Status = "queued"
	run.UpdatedAt = run.UpdatedAt.Add(-time.Hour)
	p.runs[name] = run
	f.db.Exec(`UPDATE github_repositories SET dirty=true`)
	s.Poll(context.Background())
	build = f.call("GET", "/v1/github/repositories/11/builds", nil, 200)["items"].([]any)[0].(map[string]any)
	if build["attempt"] != float64(2) || build["status"] != "completed" {
		t.Fatal("out-of-order regression")
	}
	// Timestamp precision may be one second; a terminal attempt cannot become queued.
	run.Attempt = 2
	run.UpdatedAt = run.UpdatedAt.Add(time.Hour)
	p.runs[name] = run
	f.db.Exec(`UPDATE github_repositories SET dirty=true`)
	s.Poll(context.Background())
	build = f.call("GET", "/v1/github/repositories/11/builds", nil, 200)["items"].([]any)[0].(map[string]any)
	if build["status"] != "completed" {
		t.Fatal("equal-timestamp terminal regression")
	}
	p.unavailable = true
	f.db.Exec(`UPDATE github_repositories SET dirty=true`)
	s.Poll(context.Background())
	overview := f.call("GET", "/v1/github", nil, 200)["repositories"].([]any)[0].(map[string]any)
	if overview["error_code"] != "sync_unavailable" {
		t.Fatal("provider outage hidden")
	}
	changed := s.Config
	changed.Repositories = append([]gh.Repository(nil), s.Config.Repositories...)
	changed.Repositories[0].Environment = "other"
	if (&gh.Service{DB: f.db, Config: changed}).Initialize(context.Background()) == nil {
		t.Fatal("historical environment reassigned")
	}
}

func TestPhaseThreeWebhookAuthenticationReplayAndScope(t *testing.T) {
	f := newFixture(t)
	s, _ := setupGithub(t, f)
	call := func(id string, installation, repository int, signature bool, want int) {
		t.Helper()
		b, _ := json.Marshal(map[string]any{"installation": map[string]int{"id": installation}, "repository": map[string]int{"id": repository}})
		req, _ := http.NewRequest("POST", f.server.URL+"/v1/github/webhook", bytes.NewReader(b))
		req.Header.Set("X-GitHub-Delivery", id)
		req.Header.Set("X-GitHub-Event", "workflow_run")
		if signature {
			mac := hmac.New(sha256.New, s.WebhookSecret)
			mac.Write(b)
			req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		}
		resp, e := f.client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("webhook %d, want %d", resp.StatusCode, want)
		}
	}
	id := "12345678-abcd-1234-abcd-123456789abc"
	call(id, 7, 11, false, 401)
	call(id, 8, 11, true, 403)
	call(id, 7, 99, true, 403)
	call(id, 7, 11, true, 202)
	call(id, 7, 11, true, 202)
	call(id, 7, 12, true, 409)
	var count int
	f.db.QueryRow(`SELECT count(*) FROM github_deliveries`).Scan(&count)
	if count != 1 {
		t.Fatal("delivery not deduplicated")
	}
	f.db.QueryRow(`SELECT count(*) FROM github_builds`).Scan(&count)
	if count != 0 {
		t.Fatal("webhook treated as canonical state")
	}
}

func TestPhaseThreeRejectsUntrustedBuildOrigins(t *testing.T) {
	f := newFixture(t)
	s, p := setupGithub(t, f)
	name := s.Config.Repositories[0].Name
	for _, kind := range []string{"pull_request", "fork", "workflow"} {
		run := p.runs[name]
		run.Event = "push"
		run.HeadRepository.ID = 11
		run.WorkflowID = 70
		switch kind {
		case "pull_request":
			run.Event = "pull_request"
		case "fork":
			run.HeadRepository.ID = 999
		case "workflow":
			run.WorkflowID = 999
		}
		p.runs[name] = run
		f.db.Exec(`UPDATE github_repositories SET dirty=true`)
		s.Poll(context.Background())
		var count int
		f.db.QueryRow(`SELECT count(*) FROM github_releases WHERE repository_id='11'`).Scan(&count)
		if count != 0 {
			t.Fatalf("untrusted %s created a release", kind)
		}
	}
}

type githubHistoryProvider struct{ *githubFixtureProvider }

func (p githubHistoryProvider) JSON(ctx context.Context, path string, out any) error {
	if strings.Contains(path, "/fixture/sounding-lines/actions/runs?") {
		u, _ := url.Parse(path)
		page := u.Query().Get("page")
		runs := []gh.Run{}
		if page == "1" {
			for i := range 100 {
				r := p.runs["fixture/sounding-lines"]
				r.ID = int64(200 + i)
				r.Status = "queued"
				r.Conclusion = ""
				runs = append(runs, r)
			}
		}
		if page == "2" {
			r := p.runs["fixture/sounding-lines"]
			r.ID = 400
			r.Status = "queued"
			r.Conclusion = ""
			runs = append(runs, r)
		}
		b, _ := json.Marshal(map[string]any{"workflow_runs": runs})
		return json.Unmarshal(b, out)
	}
	return p.githubFixtureProvider.JSON(ctx, path, out)
}
func TestPhaseThreePollingHistoryResumesAfterRestart(t *testing.T) {
	f := newFixture(t)
	s, p := setupGithub(t, f)
	s.Provider = githubHistoryProvider{p}
	s.Poll(context.Background())
	var page int
	f.db.QueryRow(`SELECT next_page FROM github_repositories WHERE id='11'`).Scan(&page)
	if page != 2 {
		t.Fatal("cursor not persisted")
	}
	restarted := &gh.Service{DB: f.db, Config: s.Config, Provider: s.Provider}
	f.db.Exec(`UPDATE github_repositories SET last_sync=now()-interval '1 hour'`)
	restarted.Poll(context.Background())
	var count int
	f.db.QueryRow(`SELECT count(*) FROM github_builds WHERE repository_id='11'`).Scan(&count)
	if count != 101 {
		t.Fatal("history not recovered")
	}
	f.db.QueryRow(`SELECT next_page FROM github_repositories WHERE id='11'`).Scan(&page)
	if page != 1 {
		t.Fatal("completed history did not restart")
	}
}

func TestPhaseThreeRejectsExpiredAndTamperedReleaseArtifacts(t *testing.T) {
	f := newFixture(t)
	s, p := setupGithub(t, f)
	name := s.Config.Repositories[0].Name
	original := p.manifests[name]
	for _, kind := range []string{"digest", "expired", "commit"} {
		p.expired = false
		p.artifactDigest = ""
		p.manifests[name] = original
		switch kind {
		case "digest":
			p.artifactDigest = "sha256:" + strings.Repeat("0", 64)
		case "expired":
			p.expired = true
		case "commit":
			m := original
			m.Commit = strings.Repeat("c", 40)
			p.manifests[name] = m
		}
		f.db.Exec(`UPDATE github_repositories SET dirty=true`)
		s.Poll(context.Background())
		var count int
		f.db.QueryRow(`SELECT count(*) FROM github_releases WHERE repository_id='11'`).Scan(&count)
		if count != 0 {
			t.Fatalf("invalid %s accepted", kind)
		}
	}
	p.expired = false
	p.manifests[name] = original
	b, e := p.Download(context.Background(), "/repos/"+name+"/actions/artifacts/55/zip", 1<<20)
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(b)
	p.artifactDigest = "sha256:" + hex.EncodeToString(h[:])
	f.db.Exec(`UPDATE github_repositories SET dirty=true`)
	s.Poll(context.Background())
	var count int
	f.db.QueryRow(`SELECT count(*) FROM github_releases WHERE repository_id='11'`).Scan(&count)
	if count != 1 {
		t.Fatal("verified artifact not imported on retry")
	}
}
