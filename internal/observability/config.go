// Package observability correlates bounded telemetry with durable incidents.
package observability

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var Identifier = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,79}$`)

type Scope struct {
	ID          string `json:"id"`
	Server      string `json:"server"`
	Project     string `json:"project"`
	Environment string `json:"environment"`
	Service     string `json:"service"`
}

func (s Scope) Selector() string {
	return `{server=` + strconv.Quote(s.Server) + `,project=` + strconv.Quote(s.Project) + `,environment=` + strconv.Quote(s.Environment) + `,service=` + strconv.Quote(s.Service) + `}`
}

type Rule struct {
	ID               string     `json:"id"`
	ScopeID          string     `json:"scope_id"`
	Title            string     `json:"title"`
	Metric           string     `json:"metric"`
	Operator         string     `json:"operator"`
	OpenThreshold    float64    `json:"open_threshold"`
	ResolveThreshold float64    `json:"resolve_threshold"`
	ForSeconds       int        `json:"for_seconds"`
	RecoverySeconds  int        `json:"recovery_seconds"`
	MinSamples       int        `json:"min_samples"`
	FreshnessSeconds int        `json:"freshness_seconds"`
	Severity         string     `json:"severity"`
	Enabled          bool       `json:"enabled"`
	InhibitedBy      []string   `json:"inhibited_by"`
	MaintenanceUntil *time.Time `json:"maintenance_until,omitempty"`
}
type Config struct {
	Scopes []Scope `json:"scopes"`
	Rules  []Rule  `json:"rules"`
}

func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	var c Config
	d := json.NewDecoder(io.LimitReader(f, 256*1024))
	d.DisallowUnknownFields()
	if err = d.Decode(&c); err != nil {
		return c, errors.New("invalid observability configuration")
	}
	if d.Decode(new(any)) != io.EOF {
		return c, errors.New("trailing configuration")
	}
	return c, c.Validate()
}
func (c Config) Scope(id string) (Scope, bool) {
	for _, s := range c.Scopes {
		if s.ID == id {
			return s, true
		}
	}
	return Scope{}, false
}
func (c Config) Validate() error {
	fail := errors.New("invalid observability configuration")
	if len(c.Scopes) == 0 || len(c.Scopes) > 100 || len(c.Rules) > 100 {
		return fail
	}
	ids := map[string]bool{}
	for _, s := range c.Scopes {
		if ids[s.ID] {
			return fail
		}
		ids[s.ID] = true
		for _, v := range []string{s.ID, s.Server, s.Project, s.Environment, s.Service} {
			if !Identifier.MatchString(v) {
				return fail
			}
		}
	}
	rules := map[string]bool{}
	// Dependencies must precede their dependents: deterministic order, no cycles.
	for _, r := range c.Rules {
		_, scopeOK := c.Scope(r.ScopeID)
		if !Identifier.MatchString(r.ID) || rules[r.ID] || !scopeOK || len(r.Title) == 0 || len(r.Title) > 160 || strings.ContainsAny(r.Title, "\r\n\x00") || !RuleMetric(r.Metric) || r.ForSeconds < 0 || r.ForSeconds > 86400 || r.RecoverySeconds < 0 || r.RecoverySeconds > 86400 || r.MinSamples < 2 || r.MinSamples > 1000 || r.FreshnessSeconds < 10 || r.FreshnessSeconds > 300 {
			return fail
		}
		if math.IsNaN(r.OpenThreshold) || math.IsInf(r.OpenThreshold, 0) || math.IsNaN(r.ResolveThreshold) || math.IsInf(r.ResolveThreshold, 0) {
			return fail
		}
		if r.Operator != "below" && r.Operator != "above" {
			return fail
		}
		if r.Operator == "below" && r.ResolveThreshold < r.OpenThreshold || r.Operator == "above" && r.ResolveThreshold > r.OpenThreshold {
			return fail
		}
		if r.Severity != "information" && r.Severity != "warning" && r.Severity != "degraded" && r.Severity != "critical" {
			return fail
		}
		for _, id := range r.InhibitedBy {
			if !rules[id] {
				return fail
			}
		}
		rules[r.ID] = true
	}
	return nil
}
func RawMetric(metric string) bool {
	switch metric {
	case "probe_success", "probe_duration_seconds", "probe_ssl_earliest_cert_expiry", "up", "node_load1", "node_memory_MemAvailable_bytes", "container_memory_usage_bytes", "prisme_worker_blocked", "prisme_queue_depth",
		// Host disk (root filesystem only) and swap; textfile gauges written on the host.
		"node_filesystem_avail_bytes", "node_memory_SwapFree_bytes", "prisme_containers_problem", "prisme_drill",
		// SSH threats (ssh_metrics.py): counts only, never an address.
		"prisme_ssh_failures_10m", "prisme_ssh_logins_30m", "prisme_ssh_unknown_logins_30m",
		"prisme_fail2ban_up", "prisme_fail2ban_banned", "prisme_fail2ban_found_10m", "prisme_fail2ban_blind":
		return true
	}
	return false
}

// RuleMetric accepts one raw series, optionally transformed sample by sample.
// Both keep original sample timestamps, so freshness and minimum distinct
// samples still hold. Rates and ratios (host_cpu_percent, ...) stay chart-only:
// their evaluation timestamps are synthetic.
func RuleMetric(metric string) bool {
	_, _, ok := ruleSeries(metric)
	return ok
}

func ruleSeries(metric string) (string, func(at time.Time, v float64) float64, bool) {
	if RawMetric(metric) {
		return metric, nil, true
	}
	switch metric {
	case "probe_ssl_expiry_days":
		return "probe_ssl_earliest_cert_expiry", func(at time.Time, v float64) float64 { return (v - float64(at.Unix())) / 86400 }, true
	case "prisme_host_metrics_age_seconds":
		// Textfile samples carry the scrape time: the file's own generation time
		// reveals a stopped host collector.
		return "prisme_host_metrics_generated_seconds", func(at time.Time, v float64) float64 { return float64(at.Unix()) - v }, true
	case "prisme_ssh_metrics_age_seconds":
		return "prisme_ssh_metrics_generated_seconds", func(at time.Time, v float64) float64 { return float64(at.Unix()) - v }, true
	}
	return "", nil, false
}

// Only server-owned expressions can reach the storage engines.
func Expression(metric string, scope Scope) (string, bool) {
	sel := scope.Selector()
	if RawMetric(metric) {
		return metric + sel, true
	}
	switch metric {
	case "host_cpu_percent":
		return `100 * (1 - avg(rate(node_cpu_seconds_total` + strings.TrimSuffix(sel, "}") + `,mode="idle"}[2m])))`, true
	case "host_memory_percent":
		return `100 * (1 - node_memory_MemAvailable_bytes` + sel + ` / node_memory_MemTotal_bytes` + sel + `)`, true
	case "host_disk_percent":
		return `100 * (1 - node_filesystem_avail_bytes` + strings.TrimSuffix(sel, "}") + `,mountpoint="/"} / node_filesystem_size_bytes` + strings.TrimSuffix(sel, "}") + `,mountpoint="/"})`, true
	case "container_cpu_percent":
		return `100 * rate(container_cpu_usage_seconds_total` + sel + `[2m])`, true
	case "container_network_receive_bytes":
		return `rate(container_network_receive_bytes_total` + sel + `[2m])`, true
	case "probe_ssl_expiry_days":
		return `(probe_ssl_earliest_cert_expiry` + sel + ` - time()) / 86400`, true
	case "prisme_host_metrics_age_seconds":
		return `time() - prisme_host_metrics_generated_seconds` + sel, true
	case "prisme_ssh_metrics_age_seconds":
		return `time() - prisme_ssh_metrics_generated_seconds` + sel, true
	}
	return "", false
}
