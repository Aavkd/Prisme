// Package backup owns opt-in Restic policies and isolated restore jobs.
package backup

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var ErrInvalid = errors.New("backup_invalid")
var ErrConflict = errors.New("backup_conflict")
var ErrCommand = errors.New("backup_command_failed")
var name = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

func absolute(v string) bool {
	return filepath.IsAbs(v) || (strings.HasPrefix(v, "/") && !strings.HasPrefix(v, "//"))
}

var snapshotID = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Repository struct {
	ID              string `json:"id"`
	LocationFile    string `json:"location_file"`
	PasswordFile    string `json:"password_file"`
	CredentialsFile string `json:"credentials_file"`
	StorageHost     string `json:"storage_host"`
	Offsite         bool   `json:"offsite"`
}
type Retention struct {
	KeepLast   int `json:"keep_last"`
	KeepDaily  int `json:"keep_daily"`
	KeepWeekly int `json:"keep_weekly"`
}
type Policy struct {
	ID                 string    `json:"id"`
	Project            string    `json:"project"`
	Environment        string    `json:"environment"`
	Server             string    `json:"server"`
	Repository         string    `json:"repository"`
	Kind               string    `json:"kind"`
	Path               string    `json:"path"`
	ServiceFile        string    `json:"service_file"`
	Service            string    `json:"service"`
	Consistency        string    `json:"consistency"`
	Enabled            bool      `json:"enabled"`
	IntervalSeconds    int       `json:"interval_seconds"`
	RestoreTestSeconds int       `json:"restore_test_seconds"`
	TestTarget         string    `json:"test_target"`
	TimeoutSeconds     int       `json:"timeout_seconds"`
	UploadKiB          int       `json:"upload_kib"`
	DownloadKiB        int       `json:"download_kib"`
	Retention          Retention `json:"retention"`
}
type Target struct {
	ID             string `json:"id"`
	Policy         string `json:"policy"`
	Server         string `json:"server"`
	Kind           string `json:"kind"`
	TestOnly       bool   `json:"test_only"`
	Root           string `json:"root"`
	ServiceFile    string `json:"service_file"`
	Service        string `json:"service"`
	ValidationFile string `json:"validation_file"`
}
type Config struct {
	Version          int          `json:"version"`
	CoordinationRoot string       `json:"coordination_root"`
	WorkRoot         string       `json:"work_root"`
	Repositories     []Repository `json:"repositories"`
	Policies         []Policy     `json:"policies"`
	Targets          []Target     `json:"targets"`
}

func Hash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func Load(path string) (Config, error) {
	var c Config
	b, e := os.ReadFile(path)
	if e != nil || len(b) > 256*1024 {
		return c, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil || d.Decode(new(any)) != io.EOF {
		return c, ErrInvalid
	}
	return c, c.Validate()
}
func (c Config) Repository(id string) (Repository, error) {
	for _, r := range c.Repositories {
		if r.ID == id {
			return r, nil
		}
	}
	return Repository{}, ErrInvalid
}
func (c Config) Policy(id string) (Policy, error) {
	for _, p := range c.Policies {
		if p.ID == id {
			return p, nil
		}
	}
	return Policy{}, ErrInvalid
}
func (c Config) Target(id string) (Target, error) {
	for _, t := range c.Targets {
		if t.ID == id {
			return t, nil
		}
	}
	return Target{}, ErrInvalid
}
func (c Config) Validate() error {
	if c.Version != 1 || !absolute(c.WorkRoot) || !absolute(c.CoordinationRoot) || len(c.Repositories) == 0 || len(c.Repositories) > 50 || len(c.Policies) == 0 || len(c.Policies) > 100 || len(c.Targets) > 100 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, r := range c.Repositories {
		if !name.MatchString(r.ID) || seen[r.ID] || !name.MatchString(r.StorageHost) || !absolute(r.LocationFile) || !absolute(r.PasswordFile) || (r.CredentialsFile != "" && !absolute(r.CredentialsFile)) {
			return ErrInvalid
		}
		seen[r.ID] = true
	}
	seen = map[string]bool{}
	for _, p := range c.Policies {
		r, e := c.Repository(p.Repository)
		if e != nil || !name.MatchString(p.ID) || seen[p.ID] || !name.MatchString(p.Project) || !name.MatchString(p.Environment) || !name.MatchString(p.Server) || (r.Offsite && r.StorageHost == p.Server) {
			return ErrInvalid
		}
		seen[p.ID] = true
		if p.TimeoutSeconds < 30 || p.TimeoutSeconds > 86400 || p.IntervalSeconds < 0 || (p.IntervalSeconds > 0 && p.IntervalSeconds < 300) || p.IntervalSeconds > 2592000 || p.RestoreTestSeconds < 0 || (p.RestoreTestSeconds > 0 && p.RestoreTestSeconds < 3600) || p.RestoreTestSeconds > 2592000 || p.UploadKiB < 0 || p.DownloadKiB < 0 || p.Retention.KeepLast < 2 || p.Retention.KeepLast > 1000 || p.Retention.KeepDaily < 0 || p.Retention.KeepDaily > 365 || p.Retention.KeepWeekly < 0 || p.Retention.KeepWeekly > 520 {
			return ErrInvalid
		}
		switch p.Kind {
		case "postgres":
			if !absolute(p.ServiceFile) || !name.MatchString(p.Service) || p.Path != "" || p.Consistency != "logical" {
				return ErrInvalid
			}
		case "files", "volume":
			if !absolute(p.Path) || (p.Path == "/" || p.Path == filepath.VolumeName(p.Path)+string(filepath.Separator)) || p.ServiceFile != "" || p.Service != "" || (p.Consistency != "snapshot" && p.Consistency != "stopped" && !(p.Kind == "files" && p.Consistency == "crash")) {
				return ErrInvalid
			}
		default:
			return ErrInvalid
		}
		if p.RestoreTestSeconds > 0 {
			t, e := c.Target(p.TestTarget)
			if e != nil || t.Policy != p.ID || t.Kind != "postgres" {
				return ErrInvalid
			}
		}
	}
	seen = map[string]bool{}
	for _, t := range c.Targets {
		p, e := c.Policy(t.Policy)
		if e != nil || !name.MatchString(t.ID) || seen[t.ID] || !name.MatchString(t.Server) || !t.TestOnly {
			return ErrInvalid
		}
		seen[t.ID] = true
		if t.Kind == "postgres" {
			if p.Kind != "postgres" || !absolute(t.ServiceFile) || !name.MatchString(t.Service) || !absolute(t.ValidationFile) || t.Root != "" || (t.ServiceFile == p.ServiceFile && t.Service == p.Service) {
				return ErrInvalid
			}
		} else if t.Kind == "files" {
			if p.Kind == "postgres" || !absolute(t.Root) || within(p.Path, t.Root) || within(t.Root, p.Path) || t.ServiceFile != "" || t.ValidationFile != "" {
				return ErrInvalid
			}
		} else {
			return ErrInvalid
		}
	}
	return nil
}
