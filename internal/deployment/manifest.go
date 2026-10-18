// Package deployment implements explicitly approved, immutable Compose jobs.
package deployment

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	gh "prisme.local/platform/internal/github"
)

var ErrConflict = errors.New("deployment_conflict")
var ErrInvalid = errors.New("invalid_deployment")
var name = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)
var digest = regexp.MustCompile(`^[a-z0-9][a-z0-9./:_-]*@sha256:[a-f0-9]{64}$`)

type Service struct {
	Name         string `json:"name"`
	Port         int    `json:"port"`
	BluePort     int    `json:"blue_port"`
	GreenPort    int    `json:"green_port"`
	HealthPath   string `json:"health_path"`
	DockerHealth bool   `json:"docker_health"`
	EnvFile      string `json:"env_file"`
	Stateless    bool   `json:"stateless"`
}

// The manifest intentionally cannot express shell commands, builds, privileged
// mounts or database migrations. Stateful dependencies stay in their own stack.
type Manifest struct {
	Version             int       `json:"version"`
	ID                  string    `json:"id"`
	Project             string    `json:"project"`
	Environment         string    `json:"environment"`
	Server              string    `json:"server"`
	RepositoryID        string    `json:"repository_id"`
	Repository          string    `json:"repository"`
	Branch              string    `json:"branch"`
	Enabled             bool      `json:"enabled"`
	Legacy              bool      `json:"legacy"`
	Strategy            string    `json:"strategy"`
	Migration           string    `json:"migration"`
	BackupRequired      bool      `json:"backup_required"`
	BackupPolicy        string    `json:"backup_policy,omitempty"`
	BackupMaxAgeSeconds int       `json:"backup_max_age_seconds,omitempty"`
	ObservationSeconds  int       `json:"observation_seconds"`
	TimeoutSeconds      int       `json:"timeout_seconds"`
	MinimumFreeBytes    uint64    `json:"minimum_free_bytes"`
	Network             string    `json:"network"`
	Services            []Service `json:"services"`
	CaddyURL            string    `json:"caddy_url"`
	CaddyID             string    `json:"caddy_id"`
	IngressService      string    `json:"ingress_service"`
	PublicHealthURL     string    `json:"public_health_url"`
}
type Config struct {
	CoordinationRoot string     `json:"coordination_root,omitempty"`
	Version          int        `json:"version"`
	Root             string     `json:"root"`
	Targets          []Manifest `json:"targets"`
}

func Hash(v any) string {
	b, _ := json.Marshal(v)
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
func Decode(b []byte, out any) error {
	if len(b) > 256*1024 {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil || d.Decode(new(any)) != io.EOF {
		return ErrInvalid
	}
	return nil
}
func Load(path string) (Config, error) {
	var c Config
	b, e := os.ReadFile(path)
	if e != nil {
		return c, e
	}
	if e = Decode(b, &c); e != nil {
		return c, e
	}
	if c.Version != 1 || !filepath.IsAbs(c.Root) || len(c.Targets) == 0 || len(c.Targets) > 50 {
		return c, ErrInvalid
	}
	if c.CoordinationRoot != "" && !filepath.IsAbs(c.CoordinationRoot) {
		return c, ErrInvalid
	}
	seen := map[string]bool{}
	ports := map[int]bool{}
	routes := map[string]bool{}
	for _, m := range c.Targets {
		if e = m.Validate(); e != nil {
			return c, e
		}
		if seen[m.ID] {
			return c, ErrInvalid
		}
		seen[m.ID] = true
		for _, s := range m.Services {
			for _, p := range []int{s.BluePort, s.GreenPort} {
				if p != 0 {
					if ports[p] {
						return c, ErrInvalid
					}
					ports[p] = true
				}
			}
		}
		if m.Strategy == "blue-green" {
			key := m.CaddyURL + "/" + m.CaddyID
			if routes[key] {
				return c, ErrInvalid
			}
			routes[key] = true
		}
	}
	return c, nil
}
func localURL(raw string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	ip := net.ParseIP(u.Hostname())
	return (u.Scheme == "http" || u.Scheme == "https") && ip != nil && ip.IsLoopback()
}
func (m Manifest) Validate() error {
	if m.BackupPolicy != "" && (!m.BackupRequired || !name.MatchString(m.BackupPolicy) || m.BackupMaxAgeSeconds < 60 || m.BackupMaxAgeSeconds > 86400) {
		return ErrInvalid
	}
	if m.BackupPolicy == "" && m.BackupMaxAgeSeconds != 0 {
		return ErrInvalid
	}
	if m.Version != 1 || !name.MatchString(m.ID) || !name.MatchString(m.Project) || !name.MatchString(m.Environment) || m.Server == "" || len(m.Server) > 80 || (!gh.Number.MatchString(m.RepositoryID) && !(m.RepositoryID == "" && m.Legacy && !m.Enabled)) || len(strings.Split(m.Repository, "/")) != 2 || m.Branch == "" {
		return ErrInvalid
	}
	if m.Strategy != "recreate" && m.Strategy != "blue-green" {
		return ErrInvalid
	}
	if m.Migration != "none" && m.Migration != "backward-compatible" && m.Migration != "incompatible" {
		return ErrInvalid
	}
	if m.TimeoutSeconds < 10 || m.TimeoutSeconds > 1800 || m.ObservationSeconds < 1 || m.ObservationSeconds > 600 || m.ObservationSeconds >= m.TimeoutSeconds || m.MinimumFreeBytes < 1024*1024 || len(m.Services) == 0 || len(m.Services) > 20 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	ingress := false
	for _, s := range m.Services {
		if !name.MatchString(s.Name) || seen[s.Name] {
			return ErrInvalid
		}
		seen[s.Name] = true
		if s.EnvFile != "" && (!name.MatchString(strings.TrimSuffix(s.EnvFile, ".env")) || !strings.HasSuffix(s.EnvFile, ".env")) {
			return ErrInvalid
		}
		if s.Port < 0 || s.Port > 65535 {
			return ErrInvalid
		}
		if s.Port == 0 {
			if s.BluePort != 0 || s.GreenPort != 0 || s.HealthPath != "" || !s.DockerHealth {
				return ErrInvalid
			}
		} else {
			if s.BluePort < 1024 || s.BluePort > 65535 || !strings.HasPrefix(s.HealthPath, "/") || strings.HasPrefix(s.HealthPath, "//") || strings.ContainsAny(s.HealthPath, "\r\n?#") {
				return ErrInvalid
			}
			if m.Strategy == "blue-green" && (s.GreenPort < 1024 || s.GreenPort > 65535 || s.GreenPort == s.BluePort) {
				return ErrInvalid
			}
			if m.Strategy == "recreate" && s.GreenPort != 0 {
				return ErrInvalid
			}
		}
		if m.Strategy == "blue-green" && !s.Stateless {
			return ErrInvalid
		}
		if s.Name == m.IngressService && s.Port > 0 {
			ingress = true
		}
	}
	if m.Network != "" && !name.MatchString(m.Network) {
		return ErrInvalid
	}
	if m.Strategy == "blue-green" && (!localURL(m.CaddyURL) || !strings.HasPrefix(m.CaddyID, "prisme-") || !name.MatchString(m.CaddyID) || !ingress || !localURL(m.PublicHealthURL)) {
		return ErrInvalid
	}
	if m.Strategy == "recreate" && (m.CaddyURL != "" || m.CaddyID != "" || m.PublicHealthURL != "" || m.IngressService != "") {
		return ErrInvalid
	}
	return nil
}
func (m Manifest) Blockers() []string {
	b := []string{}
	if !m.Enabled {
		b = append(b, "target_disabled")
	}
	if m.Legacy {
		b = append(b, "legacy_build_requires_immutable_images")
	}
	if m.Migration == "incompatible" {
		b = append(b, "schema_incompatible")
	}
	if m.BackupRequired && m.BackupPolicy == "" {
		b = append(b, "verified_backup_required")
	}
	return b
}
func (m Manifest) Images(r gh.Release) (map[string]string, error) {
	if r.RepositoryID != m.RepositoryID || r.Manifest.Repository != m.Repository || r.Manifest.Environment != m.Environment {
		return nil, ErrInvalid
	}
	images := map[string]string{}
	for _, i := range r.Manifest.Images {
		if !digest.MatchString(i.Reference) || images[i.Service] != "" {
			return nil, ErrInvalid
		}
		images[i.Service] = i.Reference
	}
	if len(images) != len(m.Services) {
		return nil, ErrInvalid
	}
	for _, s := range m.Services {
		if images[s.Name] == "" {
			return nil, ErrInvalid
		}
	}
	return images, nil
}
