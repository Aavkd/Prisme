// Package automation runs bounded, versioned procedures against explicit targets.
// It never interprets remote text or accepts executable commands from clients.
package automation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"time"
)

var ErrInvalid = errors.New("automation_invalid")
var ErrConflict = errors.New("automation_conflict")
var ErrBlocked = errors.New("automation_blocked")
var ErrUncertain = errors.New("automation_uncertain")
var ErrHealth = errors.New("automation_health_failed")
var name = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,79}$`)

type Target struct {
	ID          string `json:"id"`
	Deployment  string `json:"deployment"`
	Server      string `json:"server"`
	Project     string `json:"project"`
	Environment string `json:"environment"`
	Service     string `json:"service"`
	Rule        string `json:"rule"`
}
type Config struct {
	Version int      `json:"version"`
	Targets []Target `json:"targets"`
}

func Load(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil || d.Decode(new(any)) != io.EOF {
		return c, ErrInvalid
	}
	return c, c.Validate()
}
func strictDecode(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		return ErrInvalid
	}
	return nil
}
func (c Config) Validate() error {
	if c.Version != 1 || len(c.Targets) > 100 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, t := range c.Targets {
		if !name.MatchString(t.ID) || !name.MatchString(t.Server) || !name.MatchString(t.Deployment) || !name.MatchString(t.Service) || !name.MatchString(t.Project) || !name.MatchString(t.Rule) || seen[t.ID] || (t.Environment != "production" && t.Environment != "staging" && t.Environment != "test") {
			return ErrInvalid
		}
		// A deployment can have only one automation owner: rollback affects its stack.
		if seen["deployment:"+t.Deployment] {
			return ErrInvalid
		}
		seen[t.ID] = true
		seen["deployment:"+t.Deployment] = true
	}
	return nil
}
func (c Config) Target(id string) (Target, error) {
	for _, t := range c.Targets {
		if t.ID == id {
			return t, nil
		}
	}
	return Target{}, ErrInvalid
}
func Hash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type Policy struct {
	ID                 string     `json:"id"`
	Version            int        `json:"version"`
	Target             string     `json:"target"`
	Mode               string     `json:"mode"`
	Failures           int        `json:"failures"`
	WindowSeconds      int        `json:"window_seconds"`
	FreshnessSeconds   int        `json:"freshness_seconds"`
	Actions            []string   `json:"actions"`
	MaxAttempts        int        `json:"max_attempts"`
	BackoffSeconds     int        `json:"backoff_seconds"`
	ObservationSeconds int        `json:"observation_seconds"`
	TimeoutSeconds     int        `json:"timeout_seconds"`
	CooldownSeconds    int        `json:"cooldown_seconds"`
	MaintenanceUntil   *time.Time `json:"maintenance_until"`
	Dependencies       []string   `json:"dependencies"`
	Exclusions         []string   `json:"exclusions"`
	HealthyRelease     string     `json:"healthy_release"`
	Migration          string     `json:"migration"`
	Notify             bool       `json:"notify"`
}

func (p Policy) Validate(c Config) error {
	if !name.MatchString(p.ID) || p.Version < 1 {
		return ErrInvalid
	}
	if _, err := c.Target(p.Target); err != nil {
		return err
	}
	if p.Mode != "supervised" && p.Mode != "assisted" && p.Mode != "autonomous" {
		return ErrInvalid
	}
	if p.Failures < 2 || p.Failures > 100 || p.WindowSeconds < 5 || p.WindowSeconds > 3600 || p.FreshnessSeconds < 5 || p.FreshnessSeconds > 300 || p.MaxAttempts < 1 || p.MaxAttempts > 5 || p.BackoffSeconds < 1 || p.BackoffSeconds > 3600 || p.ObservationSeconds < 1 || p.ObservationSeconds > 3600 || p.TimeoutSeconds <= p.ObservationSeconds || p.TimeoutSeconds > 86400 || p.CooldownSeconds < 60 || p.CooldownSeconds > 604800 || !p.Notify {
		return ErrInvalid
	}
	if len(p.Actions) == 0 || len(p.Actions) > 2 || len(p.Dependencies) > 20 || len(p.Exclusions) > 20 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, a := range p.Actions {
		if (a != "restart" && a != "rollback") || seen[a] {
			return ErrInvalid
		}
		seen[a] = true
	}
	if p.Migration != "none" && p.Migration != "backward-compatible" {
		return ErrInvalid
	}
	if seen["rollback"] && (p.HealthyRelease == "" || len(p.HealthyRelease) > 160) {
		return ErrInvalid
	}
	for _, r := range append(append([]string{}, p.Dependencies...), p.Exclusions...) {
		if !name.MatchString(r) {
			return ErrInvalid
		}
	}
	return nil
}

type Parameter struct {
	Type    string   `json:"type"`
	Min     int      `json:"min"`
	Max     int      `json:"max"`
	Choices []string `json:"choices"`
}
type Step struct {
	Action         string `json:"action"`
	When           string `json:"when"`
	Seconds        int    `json:"seconds"`
	Parameter      string `json:"parameter"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	Retries        int    `json:"retries"`
	Release        string `json:"release"`
}
type Runbook struct {
	ID               string               `json:"id"`
	Version          int                  `json:"version"`
	Target           string               `json:"target"`
	Parameters       map[string]Parameter `json:"parameters"`
	Steps            []Step               `json:"steps"`
	TimeoutSeconds   int                  `json:"timeout_seconds"`
	MaintenanceUntil *time.Time           `json:"maintenance_until"`
}

func (b Runbook) Validate(c Config) error {
	if !name.MatchString(b.ID) || b.Version < 1 || len(b.Steps) < 1 || len(b.Steps) > 30 || b.TimeoutSeconds < 1 || b.TimeoutSeconds > 86400 || len(b.Parameters) > 10 {
		return ErrInvalid
	}
	if _, err := c.Target(b.Target); err != nil {
		return err
	}
	for k, p := range b.Parameters {
		if !name.MatchString(k) {
			return ErrInvalid
		}
		switch p.Type {
		case "integer":
			if p.Min < 0 || p.Max < p.Min || p.Max > 3600 {
				return ErrInvalid
			}
		case "boolean":
		case "string":
			if len(p.Choices) < 1 || len(p.Choices) > 20 {
				return ErrInvalid
			}
			for _, v := range p.Choices {
				if len(v) > 80 {
					return ErrInvalid
				}
			}
		default:
			return ErrInvalid
		}
	}
	for _, s := range b.Steps {
		switch s.Action {
		case "check", "restart", "rollback", "wait", "approval", "notify":
		default:
			return ErrInvalid
		}
		if s.When != "always" && s.When != "failed" && s.When != "succeeded" {
			return ErrInvalid
		}
		if s.TimeoutSeconds < 1 || s.TimeoutSeconds > 3600 || s.Retries < 0 || s.Retries > 3 || s.Seconds < 0 || s.Seconds > 3600 {
			return ErrInvalid
		}
		if s.Retries > 0 && s.Action != "check" {
			return ErrInvalid
		}
		if s.Action == "rollback" && (s.Release == "" || len(s.Release) > 160) {
			return ErrInvalid
		}
		if s.Parameter != "" {
			p, ok := b.Parameters[s.Parameter]
			if !ok || p.Type != "integer" || s.Action != "wait" {
				return ErrInvalid
			}
		}
	}
	return nil
}
func (b Runbook) Bind(values map[string]any) (Runbook, error) {
	if len(values) != len(b.Parameters) {
		return b, ErrInvalid
	}
	for k, p := range b.Parameters {
		v, ok := values[k]
		if !ok {
			return b, ErrInvalid
		}
		switch p.Type {
		case "integer":
			n, ok := v.(float64)
			if !ok || n != float64(int(n)) || n < float64(p.Min) || n > float64(p.Max) {
				return b, ErrInvalid
			}
		case "boolean":
			if _, ok := v.(bool); !ok {
				return b, ErrInvalid
			}
		case "string":
			s, ok := v.(string)
			found := false
			for _, c := range p.Choices {
				found = found || s == c
			}
			if !ok || !found {
				return b, ErrInvalid
			}
		}
	}
	b.Steps = append([]Step{}, b.Steps...)
	for i, s := range b.Steps {
		if s.Parameter != "" {
			b.Steps[i].Seconds = int(values[s.Parameter].(float64))
		}
	}
	return b, nil
}

type Run struct {
	ID            string         `json:"id"`
	Revision      int            `json:"revision"`
	Target        string         `json:"target"`
	TargetHash    string         `json:"target_hash"`
	State         string         `json:"state"`
	Origin        string         `json:"origin"`
	Book          Runbook        `json:"book"`
	Policy        *Policy        `json:"policy"`
	Parameters    map[string]any `json:"parameters"`
	Incident      int64          `json:"incident"`
	Step          int            `json:"step"`
	Attempts      int            `json:"attempts"`
	Retry         int            `json:"retry"`
	LastFailed    bool           `json:"last_failed"`
	ApprovedStep  int            `json:"approved_step"`
	ApprovedUntil *time.Time     `json:"approved_until"`
	Simulation    bool           `json:"simulation"`
	DueAt         time.Time      `json:"due_at"`
	Deadline      time.Time      `json:"deadline"`
	HealthySince  *time.Time     `json:"healthy_since"`
	LastCheckAt   *time.Time     `json:"last_check_at"`
	ErrorCode     string         `json:"error_code"`
}
type Request struct {
	Runbook    string         `json:"runbook"`
	Version    int            `json:"version"`
	Parameters map[string]any `json:"parameters"`
	Simulation bool           `json:"simulation"`
}
type Schedule struct {
	ID           string    `json:"id"`
	Revision     int       `json:"revision"`
	Request      Request   `json:"request"`
	EverySeconds int       `json:"every_seconds"`
	NextAt       time.Time `json:"next_at"`
	Enabled      bool      `json:"enabled"`
}
type Control struct {
	Stopped          bool       `json:"stopped"`
	Revision         int        `json:"revision"`
	MaintenanceUntil *time.Time `json:"maintenance_until"`
}
