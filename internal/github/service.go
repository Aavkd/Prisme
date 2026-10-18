package github

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type Build struct {
	ID           string     `json:"id"`
	RepositoryID string     `json:"repository_id"`
	WorkflowID   string     `json:"workflow_id"`
	Name         string     `json:"name"`
	Commit       string     `json:"commit"`
	Branch       string     `json:"branch"`
	Author       string     `json:"author"`
	Status       string     `json:"status"`
	Conclusion   string     `json:"conclusion"`
	Attempt      int        `json:"attempt"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	ReleaseState string     `json:"release_state"`
}
type Run struct {
	Event          string `json:"event"`
	HeadRepository struct {
		ID int64 `json:"id"`
	} `json:"head_repository"`
	ID         int64      `json:"id"`
	WorkflowID int64      `json:"workflow_id"`
	Name       string     `json:"name"`
	SHA        string     `json:"head_sha"`
	Branch     string     `json:"head_branch"`
	Status     string     `json:"status"`
	Conclusion string     `json:"conclusion"`
	Attempt    int        `json:"run_attempt"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"run_started_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	Actor      struct {
		Login string `json:"login"`
	} `json:"actor"`
	Repository struct {
		ID int64 `json:"id"`
	} `json:"repository"`
}
type Image struct {
	Service    string `json:"service"`
	Reference  string `json:"reference"`
	Tag        string `json:"tag"`
	Provenance string `json:"provenance"`
}
type Manifest struct {
	Version     int     `json:"version"`
	Repository  string  `json:"repository"`
	Commit      string  `json:"commit"`
	RunID       string  `json:"run_id"`
	RunAttempt  int     `json:"run_attempt"`
	Environment string  `json:"environment"`
	Images      []Image `json:"images"`
}
type Release struct {
	ID           string   `json:"id"`
	RepositoryID string   `json:"repository_id"`
	ArtifactID   string   `json:"artifact_id"`
	Manifest     Manifest `json:"manifest"`
}
type Artifact struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Size    int64  `json:"size_in_bytes"`
	Expired bool   `json:"expired"`
	Digest  string `json:"digest"`
}
type Service struct {
	DB            *sql.DB
	Config        Config
	Provider      Provider
	WebhookSecret []byte
	mu            sync.Mutex
}

func (s *Service) Repository(id string) (Repository, bool) {
	for _, r := range s.Config.Repositories {
		if r.ID == id {
			return r, true
		}
	}
	return Repository{}, false
}
func (s *Service) Initialize(ctx context.Context) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for _, r := range s.Config.Repositories {
		_, e = tx.ExecContext(ctx, `INSERT INTO github_repositories(id,full_name,environment,workflow_id) VALUES($1,$2,$3,$4) ON CONFLICT(id) DO NOTHING`, r.ID, r.Name, r.Environment, r.WorkflowID)
		if e != nil {
			return e
		}
		var name, env, wf string
		e = tx.QueryRowContext(ctx, `SELECT full_name,environment,workflow_id FROM github_repositories WHERE id=$1`, r.ID).Scan(&name, &env, &wf)
		if e != nil {
			return e
		}
		if name != r.Name || env != r.Environment || wf != r.WorkflowID {
			return fmt.Errorf("GitHub repository mapping cannot be reassigned")
		}
	}
	return tx.Commit()
}
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		s.Poll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
func (s *Service) Poll(ctx context.Context) {
	if !s.mu.TryLock() {
		return
	}
	defer s.mu.Unlock()
	for _, repo := range s.Config.Repositories {
		c, cancel := context.WithTimeout(ctx, 90*time.Second)
		e := s.syncRepository(c, repo)
		cancel()
		if e != nil {
			_, _ = s.DB.ExecContext(ctx, `UPDATE github_repositories SET error_code='sync_unavailable' WHERE id=$1`, repo.ID)
		}
	}
}
func (s *Service) syncRepository(ctx context.Context, repo Repository) error {
	var page int
	var last sql.NullTime
	var dirty bool
	if e := s.DB.QueryRowContext(ctx, `SELECT next_page,last_sync,dirty FROM github_repositories WHERE id=$1`, repo.ID).Scan(&page, &last, &dirty); e != nil {
		return e
	}
	if !dirty && last.Valid && time.Since(last.Time) < 2*time.Minute {
		return nil
	}
	// Page 1 always refreshes active runs; a durable cursor scans older history.
	pages := []int{1}
	if page > 1 {
		pages = append(pages, page)
	}
	_, e := s.DB.ExecContext(ctx, `UPDATE github_repositories SET dirty=false WHERE id=$1`, repo.ID)
	if e != nil {
		return e
	}
	next := 1
	for _, p := range pages {
		var out struct {
			Runs []Run `json:"workflow_runs"`
		}
		e = s.Provider.JSON(ctx, base(repo)+"/actions/runs?per_page=100&page="+strconv.Itoa(p), &out)
		if e != nil {
			return e
		}
		if len(out.Runs) > 100 {
			return ErrInvalid
		}
		for _, run := range out.Runs {
			if e = s.saveRun(ctx, repo, run); e != nil {
				return e
			}
		}
		if len(out.Runs) == 100 {
			next = p + 1
		} else {
			next = 1
		}
	}
	_, e = s.DB.ExecContext(ctx, `UPDATE github_repositories SET next_page=$2,last_sync=now(),error_code='' WHERE id=$1`, repo.ID, next)
	return e
}
func base(r Repository) string { return "/repos/" + r.Name }
func (s *Service) saveRun(ctx context.Context, repo Repository, r Run) error {
	if r.ID <= 0 || r.WorkflowID <= 0 || r.Repository.ID <= 0 || strconv.FormatInt(r.Repository.ID, 10) != repo.ID || !SHA.MatchString(r.SHA) || r.Attempt < 1 || r.UpdatedAt.IsZero() || r.CreatedAt.IsZero() {
		return ErrInvalid
	}
	b := Build{ID: strconv.FormatInt(r.ID, 10), RepositoryID: repo.ID, WorkflowID: strconv.FormatInt(r.WorkflowID, 10), Name: Redact(r.Name), Commit: r.SHA, Branch: Redact(r.Branch), Author: Redact(r.Actor.Login), Status: r.Status, Conclusion: r.Conclusion, Attempt: r.Attempt, CreatedAt: r.CreatedAt, StartedAt: r.StartedAt, UpdatedAt: r.UpdatedAt, ReleaseState: "unavailable"}
	d, _ := json.Marshal(b)
	result, e := s.DB.ExecContext(ctx, `INSERT INTO github_builds(repository_id,id,updated_at,document) VALUES($1,$2,$3,$4)
ON CONFLICT(repository_id,id) DO UPDATE SET updated_at=excluded.updated_at,
document=CASE WHEN github_builds.document->>'attempt'=excluded.document->>'attempt' AND github_builds.document->>'release_state'='linked'
THEN jsonb_set(excluded.document,'{release_state}','"linked"') ELSE excluded.document END
WHERE github_builds.updated_at<=excluded.updated_at
AND github_builds.document->>'commit'=excluded.document->>'commit'
AND (github_builds.document->>'attempt')::int<=(excluded.document->>'attempt')::int
AND NOT (github_builds.document->>'attempt'=excluded.document->>'attempt' AND github_builds.document->>'status'='completed' AND excluded.document->>'status'<>'completed')`, repo.ID, b.ID, b.UpdatedAt, string(d))
	if e != nil {
		return e
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return nil
	}
	if b.Status == "completed" && b.Conclusion == "success" && b.WorkflowID == repo.WorkflowID && strconv.FormatInt(r.HeadRepository.ID, 10) == repo.ID && (r.Event == "push" || r.Event == "workflow_dispatch" || r.Event == "release") {
		// Missing/expired/invalid provenance is a visible build state, never a fabricated release.
		if e = s.ingestRelease(ctx, repo, b); e != nil {
			_, e = s.DB.ExecContext(ctx, `UPDATE github_builds SET document=jsonb_set(document,'{release_state}','"unavailable"') WHERE repository_id=$1 AND id=$2 AND (document->>'attempt')::int=$3 AND NOT EXISTS(SELECT 1 FROM github_releases WHERE repository_id=$1 AND run_id=$2 AND run_attempt=$3)`, repo.ID, b.ID, b.Attempt)
			return e
		}
	}
	return nil
}

var imagePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?(?::[0-9]{1,5})?/[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*(?:/[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*)*@sha256:[a-f0-9]{64}$`)

func ValidateManifest(m Manifest, repo Repository, b Build) error {
	if m.Version != 1 || m.Repository != repo.Name || m.Commit != b.Commit || m.RunID != b.ID || m.RunAttempt != b.Attempt || m.Environment != repo.Environment || len(m.Images) == 0 || len(m.Images) > 30 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, im := range m.Images {
		if !environmentPattern.MatchString(im.Service) || seen[im.Service] || len(im.Reference) > 400 || !imagePattern.MatchString(im.Reference) || strings.Contains(im.Reference, "..") || len(im.Tag) > 128 || len(im.Provenance) > 500 {
			return ErrInvalid
		}
		seen[im.Service] = true
	}
	return nil
}
func (s *Service) ingestRelease(ctx context.Context, repo Repository, b Build) error {
	var existing bool
	if e := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM github_releases WHERE repository_id=$1 AND run_id=$2 AND run_attempt=$3)`, repo.ID, b.ID, b.Attempt).Scan(&existing); e != nil {
		return e
	}
	if !existing {
		var out struct {
			Artifacts []Artifact `json:"artifacts"`
		}
		if e := s.Provider.JSON(ctx, base(repo)+"/actions/runs/"+b.ID+"/artifacts?per_page=100", &out); e != nil {
			return e
		}
		var chosen *Artifact
		for i, a := range out.Artifacts {
			if a.Name == "prisme-release-"+strconv.Itoa(b.Attempt) {
				if chosen != nil {
					return ErrInvalid
				}
				chosen = &out.Artifacts[i]
			}
		}
		if chosen == nil || chosen.ID <= 0 || chosen.Expired || chosen.Size > 1<<20 {
			return ErrInvalid
		}
		data, e := s.Provider.Download(ctx, base(repo)+"/actions/artifacts/"+strconv.FormatInt(chosen.ID, 10)+"/zip", 1<<20)
		if e != nil {
			return e
		}
		if chosen.Digest != "" {
			h := sha256.Sum256(data)
			if chosen.Digest != "sha256:"+hex.EncodeToString(h[:]) {
				return ErrInvalid
			}
		}
		data, e = ManifestFromZIP(data)
		if e != nil {
			return e
		}
		var m Manifest
		d := json.NewDecoder(strings.NewReader(string(data)))
		d.DisallowUnknownFields()
		if d.Decode(&m) != nil || d.Decode(new(any)) != io.EOF || ValidateManifest(m, repo, b) != nil {
			return ErrInvalid
		}
		for i := range m.Images {
			m.Images[i].Tag = Redact(m.Images[i].Tag)
			m.Images[i].Provenance = Redact(m.Images[i].Provenance)
		}
		h := sha256.Sum256([]byte(repo.ID + ":" + b.ID + ":" + strconv.Itoa(b.Attempt)))
		rel := Release{ID: hex.EncodeToString(h[:]), RepositoryID: repo.ID, ArtifactID: strconv.FormatInt(chosen.ID, 10), Manifest: m}
		data, _ = json.Marshal(rel)
		tx, e := s.DB.BeginTx(ctx, nil)
		if e != nil {
			return e
		}
		defer tx.Rollback()
		result, e := tx.ExecContext(ctx, `INSERT INTO github_releases(id,repository_id,run_id,run_attempt,artifact_id,document) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, rel.ID, repo.ID, b.ID, b.Attempt, rel.ArtifactID, string(data))
		if e != nil {
			return e
		}
		if n, _ := result.RowsAffected(); n > 0 {
			if _, e = tx.ExecContext(ctx, `INSERT INTO audit_events(actor,action,target,origin) VALUES('github-app','release.linked',$1,'github')`, rel.ID); e != nil {
				return e
			}
		}
		if e = tx.Commit(); e != nil {
			return e
		}
	}
	_, e := s.DB.ExecContext(ctx, `UPDATE github_builds SET document=jsonb_set(document,'{release_state}','"linked"') WHERE repository_id=$1 AND id=$2 AND (document->>'attempt')::int=$3`, repo.ID, b.ID, b.Attempt)
	return e
}

// Browse exposes a fixed set of read-only resources, never arbitrary provider URLs.
func (s *Service) Browse(ctx context.Context, r Repository, kind, ref, path, page string) (any, error) {
	p, e := strconv.Atoi(page)
	if page == "" {
		p = 1
		e = nil
	}
	if e != nil || p < 1 || p > 1000 || len(ref) > 200 || len(path) > 1000 {
		return nil, ErrInvalid
	}
	q := url.Values{"per_page": {"50"}, "page": {strconv.Itoa(p)}}
	endpoint := ""
	switch kind {
	case "branches", "tags", "commits", "pulls", "releases":
		endpoint = "/" + kind
		if kind == "commits" && ref != "" {
			q.Set("sha", ref)
		}
		if kind == "pulls" {
			q.Set("state", "all")
		}
	case "workflows":
		endpoint = "/actions/workflows"
	case "files":
		if strings.HasPrefix(path, "/") || strings.ContainsAny(path, "\\\x00") {
			return nil, ErrInvalid
		}
		parts := strings.Split(path, "/")
		for _, part := range parts {
			if part == ".." || part == "." {
				return nil, ErrInvalid
			}
		}
		for i := range parts {
			parts[i] = url.PathEscape(parts[i])
		}
		endpoint = "/contents/" + strings.Join(parts, "/")
		q = url.Values{}
		if ref != "" {
			q.Set("ref", ref)
		}
	default:
		return nil, ErrInvalid
	}
	var out any
	if e = s.Provider.JSON(ctx, base(r)+endpoint+"?"+q.Encode(), &out); e != nil {
		return nil, e
	}
	if kind == "files" {
		if file, ok := out.(map[string]any); ok {
			if file["type"] != "file" || file["encoding"] != "base64" {
				return nil, ErrInvalid
			}
			encoded, _ := file["content"].(string)
			decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(encoded, "\n", ""))
			if err != nil || len(decoded) > 256*1024 || !utf8.Valid(decoded) || strings.ContainsRune(string(decoded), 0) {
				return nil, ErrInvalid
			}
			file["content"] = Redact(string(decoded))
			file["encoding"] = "utf-8"
		}
	}
	return sanitize(out), nil
}

// Do not return download URLs, credentials, PR bodies or arbitrary provider fields.
var allowedFields = map[string]bool{"id": true, "number": true, "name": true, "path": true, "type": true, "size": true, "sha": true, "ref": true, "title": true, "state": true, "draft": true, "merged": true, "mergeable": true, "login": true, "user": true, "author": true, "date": true, "message": true, "commit": true, "head": true, "base": true, "tag_name": true, "target_commitish": true, "published_at": true, "created_at": true, "updated_at": true, "encoding": true, "content": true, "workflows": true, "total_count": true, "jobs": true, "steps": true, "status": true, "conclusion": true, "started_at": true, "completed_at": true, "runner_name": true, "labels": true, "artifacts": true, "expired": true, "size_in_bytes": true, "digest": true, "run_id": true, "run_attempt": true, "workflow_id": true, "head_sha": true, "head_branch": true}

func sanitize(v any) any {
	switch x := v.(type) {
	case map[string]any:
		o := map[string]any{}
		for k, v := range x {
			if allowedFields[k] {
				o[k] = sanitize(v)
			}
		}
		return o
	case []any:
		o := []any{}
		for _, v := range x {
			o = append(o, sanitize(v))
		}
		return o
	case string:
		return Redact(x)
	default:
		return v
	}
}
func (s *Service) BuildDetails(ctx context.Context, r Repository, id string, page int) (map[string]any, error) {
	if !Number.MatchString(id) || page < 1 || page > 1000 {
		return nil, ErrInvalid
	}
	var jobs, artifacts any
	if e := s.Provider.JSON(ctx, base(r)+"/actions/runs/"+id+"/jobs?per_page=50&page="+strconv.Itoa(page), &jobs); e != nil {
		return nil, e
	}
	if e := s.Provider.JSON(ctx, base(r)+"/actions/runs/"+id+"/artifacts?per_page=50&page="+strconv.Itoa(page), &artifacts); e != nil {
		return nil, e
	}
	return map[string]any{"jobs": sanitize(jobs), "artifacts": sanitize(artifacts)}, nil
}
func (s *Service) JobLogs(ctx context.Context, r Repository, runID, jobID string) (string, error) {
	if !Number.MatchString(runID) || !Number.MatchString(jobID) {
		return "", ErrInvalid
	}
	var job struct {
		RunID int64 `json:"run_id"`
	}
	if e := s.Provider.JSON(ctx, base(r)+"/actions/jobs/"+jobID, &job); e != nil {
		return "", e
	}
	if strconv.FormatInt(job.RunID, 10) != runID {
		return "", ErrInvalid
	}
	b, e := s.Provider.Download(ctx, base(r)+"/actions/jobs/"+jobID+"/logs", 1<<20)
	if e != nil {
		return "", e
	}
	return Redact(string(b)), nil
}
