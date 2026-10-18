package backup

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Artifact struct {
	ID         string     `json:"id"`
	PolicyID   string     `json:"policy_id"`
	JobID      string     `json:"job_id"`
	SnapshotID string     `json:"snapshot_id"`
	Bytes      int64      `json:"bytes"`
	CreatedAt  time.Time  `json:"created_at"`
	VerifiedAt time.Time  `json:"verified_at"`
	ExpiredAt  *time.Time `json:"expired_at"`
}
type Result struct {
	Snapshot string
	Bytes    int64
	Retained []string
	Evidence map[string]any
}
type Driver interface {
	Execute(context.Context, Config, Policy, Job, *Artifact, func(string) error) (Result, error)
}
type Restic struct{}

// Child output is bounded and never surfaced in errors, logs, audit or the API.
type bounded struct {
	bytes.Buffer
	limit    int
	overflow bool
}

// A successful backup ends with a summary. Keep only a bounded tail so hours
// of JSON progress do not turn a complete backup into an output-limit failure.
type tail struct {
	data  []byte
	limit int
}

func (b *tail) Write(p []byte) (int, error) {
	n := len(p)
	if n >= b.limit {
		b.data = append(b.data[:0], p[n-b.limit:]...)
		return n, nil
	}
	if len(b.data)+n > b.limit {
		b.data = append(b.data[:0], b.data[len(b.data)+n-b.limit:]...)
	}
	b.data = append(b.data, p...)
	return n, nil
}

func (b *bounded) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len()+n > b.limit {
		b.overflow = true
		p = p[:max(0, b.limit-b.Len())]
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}
func command(ctx context.Context, env []string, dir, bin string, args ...string) ([]byte, error) {
	c := exec.CommandContext(ctx, bin, args...)
	c.Env = env
	c.Dir = dir
	c.WaitDelay = 3 * time.Second
	configureProcess(c)
	out := &bounded{limit: 4 * 1024 * 1024}
	c.Stdout = out
	last := &tail{limit: 64 * 1024}
	backupJSON := false
	if bin == "restic" {
		for _, a := range args {
			if a == "backup" {
				backupJSON = true
				break
			}
		}
	}
	if backupJSON {
		c.Stdout = last
	}
	c.Stderr = io.Discard
	if c.Run() != nil || out.overflow {
		return nil, ErrCommand
	}
	if backupJSON {
		return last.data, nil
	}
	return out.Bytes(), nil
}
func baseEnv() []string {
	out := []string{"GOMAXPROCS=2"}
	for _, k := range []string{"PATH", "HOME", "USERPROFILE", "SYSTEMROOT", "TMPDIR", "TEMP", "TMP"} {
		if v := os.Getenv(k); v != "" {
			out = append(out, k+"="+v)
		}
	}
	return out
}
func privateFile(path string) ([]byte, error) {
	s, e := os.Lstat(path)
	if e != nil || !s.Mode().IsRegular() || s.Size() > 1024*1024 || s.Size() == 0 {
		return nil, ErrInvalid
	}
	if e = checkPrivate(s); e != nil {
		return nil, e
	}
	return os.ReadFile(path)
}
func repositoryEnv(r Repository) ([]string, error) {
	location, e := privateFile(r.LocationFile)
	if e != nil {
		return nil, e
	}
	password, e := privateFile(r.PasswordFile)
	if e != nil {
		return nil, e
	}
	location = bytes.TrimSpace(location)
	if len(bytes.TrimSpace(password)) < 16 || len(location) == 0 || bytes.ContainsAny(location, "\r\n\x00") {
		return nil, ErrInvalid
	}
	loc := string(location)
	// No executable backends or shell transport supplied by configuration. SFTP
	// can be mounted by the operator; native remote support here is HTTPS S3.
	if !filepath.IsAbs(loc) && !strings.HasPrefix(loc, "s3:https://") {
		return nil, ErrInvalid
	}
	if strings.HasPrefix(loc, "s3:") {
		u, err := url.Parse(strings.TrimPrefix(loc, "s3:"))
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, ErrInvalid
		}
		if r.CredentialsFile == "" {
			return nil, ErrInvalid
		}
	}
	env := append(baseEnv(), "RESTIC_REPOSITORY_FILE="+r.LocationFile, "RESTIC_PASSWORD_FILE="+r.PasswordFile)
	if r.CredentialsFile != "" {
		b, e := privateFile(r.CredentialsFile)
		if e != nil {
			return nil, e
		}
		var v map[string]string
		if json.Unmarshal(b, &v) != nil {
			return nil, ErrInvalid
		}
		if strings.HasPrefix(loc, "s3:") && (v["AWS_ACCESS_KEY_ID"] == "" || v["AWS_SECRET_ACCESS_KEY"] == "") {
			return nil, ErrInvalid
		}
		for k, value := range v {
			switch k {
			case "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_DEFAULT_REGION":
			default:
				return nil, ErrInvalid
			}
			if strings.ContainsAny(value, "\x00\r\n") {
				return nil, ErrInvalid
			}
			env = append(env, k+"="+value)
		}
	}
	return env, nil
}
func pgEnv(file, service string) ([]string, error) {
	if _, e := privateFile(file); e != nil {
		return nil, e
	}
	return append(baseEnv(), "PGSERVICEFILE="+file, "PGSERVICE="+service, "PGCONNECT_TIMEOUT=10"), nil
}
func resticCommand(ctx context.Context, env []string, p Policy, dir string, args ...string) ([]byte, error) {
	prefix := []string{"--no-cache", "--limit-upload", strconv.Itoa(p.UploadKiB), "--limit-download", strconv.Itoa(p.DownloadKiB)}
	return command(ctx, env, dir, "restic", append(prefix, args...)...)
}
func safeDirectory(path string) error {
	if !filepath.IsAbs(path) {
		return ErrInvalid
	}
	resolved, e := filepath.EvalSymlinks(path)
	if e != nil || resolved != filepath.Clean(path) {
		return ErrInvalid
	}
	s, e := os.Stat(path)
	if e != nil || !s.IsDir() {
		return ErrInvalid
	}
	return nil
}

func privateDirectory(path string) error {
	if err := safeDirectory(path); err != nil {
		return err
	}
	s, err := os.Stat(path)
	if err != nil {
		return err
	}
	return checkPrivate(s)
}

// InitializeRepository is an explicit local operator action. It is never called
// by a schedule, API request, failed password attempt or automatic retry.
func InitializeRepository(ctx context.Context, c Config, id string) error {
	r, err := c.Repository(id)
	if err != nil {
		return err
	}
	env, err := repositoryEnv(r)
	if err != nil {
		return err
	}
	_, err = command(ctx, env, "", "restic", "init")
	return err
}
func (d *Restic) Execute(ctx context.Context, c Config, p Policy, j Job, a *Artifact, step func(string) error) (Result, error) {
	result := Result{Evidence: map[string]any{}}
	r, e := c.Repository(p.Repository)
	if e != nil {
		return result, e
	}
	env, e := repositoryEnv(r)
	if e != nil {
		return result, e
	}
	if e = privateDirectory(c.WorkRoot); e != nil {
		return result, e
	}
	directory := c.WorkRoot
	run := func(args ...string) ([]byte, error) { return resticCommand(ctx, env, p, directory, args...) }
	check := func() error {
		if e := step("integrity"); e != nil {
			return e
		}
		_, e := run("check", "--read-data")
		return e
	}
	switch j.Kind {
	case "backup":
		if e = step("snapshot"); e != nil {
			return result, e
		}
		args := []string{"backup", "--json", "--host", p.Server, "--tag", "prisme:" + p.ID}
		if p.Kind == "postgres" {
			pg, e := pgEnv(p.ServiceFile, p.Service)
			if e != nil {
				return result, e
			}
			// Restic waits for pg_dump's exit status; a failed producer is never
			// accepted as a complete encrypted snapshot. No plaintext staging.
			for _, v := range pg {
				if strings.HasPrefix(v, "PG") {
					env = append(env, v)
				}
			}
			args = append(args, "--stdin-from-command", "--stdin-filename", "database.dump", "--", "pg_dump", "--format=custom", "--no-owner", "--no-acl", "--no-password")
		} else {
			if e = safeDirectory(p.Path); e != nil {
				return result, e
			}
			// A stopped/snapshot volume is an explicit operator contract. Live
			// PostgreSQL data directories are never a supported file source.
			if _, e = os.Stat(filepath.Join(p.Path, "PG_VERSION")); e == nil {
				return result, ErrInvalid
			}
			loc, _ := os.ReadFile(r.LocationFile)
			loc = bytes.TrimSpace(loc)
			for _, path := range []string{c.WorkRoot, c.CoordinationRoot, r.PasswordFile, r.LocationFile, r.CredentialsFile, string(loc)} {
				if path != "" && within(p.Path, path) {
					return result, ErrInvalid
				}
			}
			if runtime.GOOS != "windows" {
				args = append(args, "--one-file-system")
			}
			// Store the selected tree relative to its root. Ancestor ACLs and
			// machine-specific drive prefixes must not be recreated at the target.
			directory = p.Path
			args = append(args, "--", ".")
		}
		out, e := run(args...)
		if e != nil {
			return result, e
		}
		scan := bufio.NewScanner(bytes.NewReader(out))
		scan.Buffer(make([]byte, 4096), 4*1024*1024)
		for scan.Scan() {
			var v struct {
				Type     string `json:"message_type"`
				Snapshot string `json:"snapshot_id"`
				Bytes    int64  `json:"total_bytes_processed"`
			}
			if json.Unmarshal(scan.Bytes(), &v) == nil && v.Type == "summary" {
				result.Snapshot = v.Snapshot
				result.Bytes = v.Bytes
			}
		}
		if scan.Err() != nil || !snapshotID.MatchString(result.Snapshot) || result.Bytes < 0 {
			return result, ErrCommand
		}
		if e = check(); e != nil {
			return result, e
		}
		result.Evidence = map[string]any{"encrypted": true, "integrity": "full_read", "consistency": p.Consistency}
	case "check":
		if e = check(); e != nil {
			return result, e
		}
		result.Evidence["integrity"] = "full_read"
	case "retention":
		if e = check(); e != nil {
			return result, e
		}
		if e = step("retention"); e != nil {
			return result, e
		}
		out, e := run("forget", "--dry-run", "--json", "--host", p.Server, "--tag", "prisme:"+p.ID, "--group-by", "host", "--keep-last", strconv.Itoa(p.Retention.KeepLast), "--keep-daily", strconv.Itoa(p.Retention.KeepDaily), "--keep-weekly", strconv.Itoa(p.Retention.KeepWeekly))
		if e != nil {
			return result, e
		}
		var groups []struct {
			Remove []struct {
				ID string `json:"id"`
			} `json:"remove"`
		}
		if json.Unmarshal(out, &groups) != nil {
			return result, ErrCommand
		}
		known, protected := map[string]bool{}, map[string]bool{}
		for _, id := range j.KnownSnapshots {
			known[id] = true
		}
		for _, id := range j.ProtectedSnapshots {
			protected[id] = true
		}
		remove := []string{}
		for _, g := range groups {
			for _, s := range g.Remove {
				if !snapshotID.MatchString(s.ID) {
					return result, ErrCommand
				}
				if known[s.ID] && !protected[s.ID] && len(remove) < 100 {
					remove = append(remove, s.ID)
				}
			}
		}
		// The most recent verified copies survive even if partial/unrecorded
		// snapshots would otherwise fill Restic's keep-last slots. Never delete an
		// unregistered snapshot automatically. Limit each prune to 100 artifacts.
		if len(remove) > 0 {
			if _, e = run(append([]string{"forget", "--prune", "--"}, remove...)...); e != nil {
				return result, e
			}
		}
		out, e = run("snapshots", "--json", "--host", p.Server, "--tag", "prisme:"+p.ID)
		if e != nil {
			return result, e
		}
		var snapshots []struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(out, &snapshots) != nil {
			return result, ErrCommand
		}
		result.Retained = []string{}
		for _, s := range snapshots {
			if !snapshotID.MatchString(s.ID) {
				return result, ErrCommand
			}
			result.Retained = append(result.Retained, s.ID)
		}
		result.Evidence["retained"] = len(result.Retained)
	case "restore":
		if a == nil || !snapshotID.MatchString(a.SnapshotID) {
			return result, ErrInvalid
		}
		t, e := c.Target(j.TargetID)
		if e != nil || !t.TestOnly || t.Policy != p.ID {
			return result, ErrInvalid
		}
		if e = check(); e != nil {
			return result, e
		}
		root := c.WorkRoot
		if t.Kind == "files" {
			root = t.Root
		}
		if e = privateDirectory(root); e != nil {
			return result, e
		}
		dir := filepath.Join(root, "restore-"+j.ID)
		if e = os.Mkdir(dir, 0700); e != nil {
			return result, ErrConflict
		}
		// This directory was created by this job, under an operator-owned root.
		// File restores remain for inspection. Temporary DB archives are removed.
		if t.Kind == "postgres" {
			defer os.RemoveAll(dir)
		}
		if e = step("restore_files"); e != nil {
			return result, e
		}
		if _, e = run("restore", a.SnapshotID, "--target", dir, "--verify"); e != nil {
			return result, e
		}
		result.Evidence["integrity"] = "restore_verify"
		if t.Kind == "files" {
			result.Evidence["output_directory"] = dir
			return result, nil
		}
		pg, e := pgEnv(t.ServiceFile, t.Service)
		if e != nil {
			return result, e
		}
		// Target role must be non-superuser with CREATEDB. Each restore creates a
		// fresh database; no --clean, replacement or production traffic switch.
		out, e := command(ctx, pg, "", "psql", "-X", "-w", "-At", "-v", "ON_ERROR_STOP=1", "-c", "SELECT NOT rolsuper AND rolcreatedb AND (SELECT count(*)<10 FROM pg_database WHERE datdba=pg_roles.oid AND datname LIKE 'prisme_restore_%') FROM pg_roles WHERE rolname=current_user")
		if e != nil || strings.TrimSpace(string(out)) != "t" {
			return result, ErrInvalid
		}
		database := "prisme_restore_" + j.ID[:24]
		result.Evidence["database"] = database
		if e = step("restore_database"); e != nil {
			return result, e
		}
		if _, e = command(ctx, pg, "", "createdb", "--no-password", "--template=template0", database); e != nil {
			return result, e
		}
		if _, e = command(ctx, pg, "", "pg_restore", "--no-password", "--no-owner", "--no-acl", "--exit-on-error", "--single-transaction", "--dbname", database, filepath.Join(dir, "database.dump")); e != nil {
			return result, e
		}
		if e = step("validate_database"); e != nil {
			return result, e
		}
		validation, e := privateFile(t.ValidationFile)
		if e != nil {
			return result, e
		}
		// SQL is operator-controlled, never request data. A read-only session and
		// timeout constrain checks; exactly one true result is required.
		validationEnv := append(append([]string{}, pg...), "PGOPTIONS=-c default_transaction_read_only=on -c statement_timeout=30000")
		out, e = command(ctx, validationEnv, "", "psql", "-X", "-w", "-At", "-v", "ON_ERROR_STOP=1", "--dbname", database, "--command", string(validation))
		if e != nil || strings.TrimSpace(string(out)) != "t" {
			return result, ErrCommand
		}
		result.Evidence["validation"] = "passed"
		result.Evidence["temporary"] = j.Scheduled
		if j.Scheduled {
			if e = step("cleanup"); e != nil {
				return result, e
			}
			// Only the fresh database created by this run is eligible. Cleanup is
			// part of success; failures keep the database name visible for recovery.
			if _, e = command(ctx, pg, "", "dropdb", "--no-password", database); e != nil {
				return result, e
			}
			result.Evidence["cleanup"] = "passed"
		}
	default:
		return result, ErrInvalid
	}
	return result, nil
}
func within(root, path string) bool {
	rel, e := filepath.Rel(root, path)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
