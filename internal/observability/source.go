package observability

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ErrUnavailable = errors.New("telemetry unavailable")

type Point struct {
	At    time.Time `json:"at"`
	Value *float64  `json:"value"`
}
type Series struct {
	Labels map[string]string `json:"labels"`
	Points []Point           `json:"points"`
}
type LogLine struct {
	At     time.Time         `json:"at"`
	Line   string            `json:"line"`
	Labels map[string]string `json:"labels"`
}
type Source struct {
	MetricsURL, LogsURL string
	Client              *http.Client
}

func NewSource(metrics, logs string, allowLocalHTTP bool) (*Source, error) {
	for _, v := range []string{metrics, logs} {
		u, e := url.Parse(v)
		if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" || u.Scheme != "https" && !(allowLocalHTTP && u.Scheme == "http") {
			return nil, errors.New("invalid telemetry URL (HTTPS required outside development)")
		}
	}
	return &Source{MetricsURL: strings.TrimRight(metrics, "/"), LogsURL: strings.TrimRight(logs, "/"), Client: &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// A single-label Docker service name: never an IP address or a dotted DNS name.
var internalService = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// NewInternalSource allows cleartext HTTP outside development only towards
// Compose service names such as victoriametrics:8428, which resolve on the
// stack's internal network alone. HTTPS URLs keep the usual checks.
func NewInternalSource(metrics, logs string) (*Source, error) {
	for _, v := range []string{metrics, logs} {
		if u, err := url.Parse(v); err == nil && u.Scheme == "http" && !internalService.MatchString(u.Hostname()) {
			return nil, errors.New("invalid telemetry URL (cleartext only to an internal service name)")
		}
	}
	return NewSource(metrics, logs, true)
}

type resultEnvelope struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string   `json:"metric"`
			Stream map[string]string   `json:"stream"`
			Values [][]json.RawMessage `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

func (s *Source) get(ctx context.Context, base, path string, q url.Values) (resultEnvelope, error) {
	var out resultEnvelope
	req, err := http.NewRequestWithContext(ctx, "GET", base+path+"?"+q.Encode(), nil)
	if err != nil {
		return out, ErrUnavailable
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return out, ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return out, ErrUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
	if err != nil || len(data) > 2*1024*1024 {
		return out, ErrUnavailable
	}
	if json.Unmarshal(data, &out) != nil || out.Status != "success" || len(out.Data.Result) > 100 {
		return out, ErrUnavailable
	}
	return out, nil
}
func parsePoint(v []json.RawMessage) (Point, bool) {
	if len(v) != 2 {
		return Point{}, false
	}
	var ts float64
	var str string
	if json.Unmarshal(v[0], &ts) != nil || math.IsNaN(ts) || math.IsInf(ts, 0) || json.Unmarshal(v[1], &str) != nil {
		return Point{}, false
	}
	value, err := strconv.ParseFloat(str, 64)
	p := Point{At: time.UnixMilli(int64(ts * 1000)).UTC()}
	if err == nil && !math.IsNaN(value) && !math.IsInf(value, 0) {
		p.Value = &value
	}
	return p, true
}
func (s *Source) Latest(ctx context.Context, scope Scope, metric string, now time.Time) (*Point, error) {
	raw, transform, ok := ruleSeries(metric)
	if !ok {
		return nil, ErrUnavailable
	}
	// A range selector retains original sample timestamps; repeated evaluations of
	// the same scrape must never satisfy the minimum-sample requirement.
	out, err := s.get(ctx, s.MetricsURL, "/api/v1/query", url.Values{"query": {raw + scope.Selector() + "[5m]"}, "time": {strconv.FormatInt(now.Unix(), 10)}})
	if err != nil {
		return nil, err
	}
	if out.Data.ResultType != "matrix" || len(out.Data.Result) != 1 {
		return nil, nil
	}
	values := out.Data.Result[0].Values
	if len(values) == 0 {
		return nil, nil
	}
	p, ok := parsePoint(values[len(values)-1])
	if !ok {
		return nil, ErrUnavailable
	}
	if transform != nil && p.Value != nil {
		v := transform(p.At, *p.Value)
		p.Value = &v
	}
	return &p, nil
}
func (s *Source) Metrics(ctx context.Context, scope Scope, metric string, start, end time.Time) ([]Series, error) {
	expression, ok := Expression(metric, scope)
	if !ok {
		return nil, ErrUnavailable
	}
	step := int(end.Sub(start).Seconds() / 300)
	if step < 5 {
		step = 5
	}
	// Bound implicit lookback so a last known value isn't drawn through a collector
	// outage. Rules use original timestamps and their own freshness contract.
	reference := metric + scope.Selector()
	switch metric {
	case "host_cpu_percent":
		reference = "node_cpu_seconds_total" + scope.Selector()
	case "host_memory_percent":
		reference = "node_memory_MemAvailable_bytes" + scope.Selector()
	case "host_disk_percent":
		reference = "node_filesystem_avail_bytes" + scope.Selector()
	case "container_cpu_percent":
		reference = "container_cpu_usage_seconds_total" + scope.Selector()
	case "container_network_receive_bytes":
		reference = "container_network_receive_bytes_total" + scope.Selector()
	case "probe_ssl_expiry_days":
		reference = "probe_ssl_earliest_cert_expiry" + scope.Selector()
	case "prisme_host_metrics_age_seconds":
		reference = "prisme_host_metrics_generated_seconds" + scope.Selector()
	case "prisme_ssh_metrics_age_seconds":
		reference = "prisme_ssh_metrics_generated_seconds" + scope.Selector()
	}
	expression = "(" + expression + ") and on() (min(timestamp(" + reference + ")) > (time() - 90))"
	out, err := s.get(ctx, s.MetricsURL, "/api/v1/query_range", url.Values{"query": {expression}, "start": {strconv.FormatInt(start.Unix(), 10)}, "end": {strconv.FormatInt(end.Unix(), 10)}, "step": {strconv.Itoa(step)}})
	if err != nil {
		return nil, err
	}
	if out.Data.ResultType != "matrix" {
		return nil, ErrUnavailable
	}
	series := []Series{}
	for _, r := range out.Data.Result {
		points := []Point{}
		if len(r.Values) > 1000 {
			return nil, ErrUnavailable
		}
		for _, v := range r.Values {
			p, ok := parsePoint(v)
			if !ok {
				return nil, ErrUnavailable
			}
			points = append(points, p)
		}
		series = append(series, Series{Labels: safeLabels(r.Metric), Points: points})
	}
	return series, nil
}
func safeLabels(labels map[string]string) map[string]string {
	out := map[string]string{}
	for _, k := range []string{"server", "project", "environment", "service", "level", "name", "cpu", "device", "mountpoint"} {
		if v, ok := labels[k]; ok && len(v) <= 160 {
			out[k] = Redact(v)
		}
	}
	return out
}

var secretAssignment = regexp.MustCompile(`(?i)("?(?:password|passwd|secret|token|api[_-]?key|authorization|cookie|set-cookie)"?\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;]+)`)
var bearerSecret = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
var urlCredential = regexp.MustCompile(`(https?://)[^\s/@:]+:[^\s/@]+@`)
var keyMaterial = regexp.MustCompile(`(?s)-----BEGIN [^-]*PRIVATE KEY-----.*?(-----END [^-]*PRIVATE KEY-----|$)`)

func Redact(s string) string {
	s = keyMaterial.ReplaceAllString(s, "[REDACTED PRIVATE KEY]")
	s = bearerSecret.ReplaceAllString(s, "Bearer [REDACTED]")
	s = secretAssignment.ReplaceAllString(s, "${1}[REDACTED]")
	s = urlCredential.ReplaceAllString(s, "${1}[REDACTED]@")
	return strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
}
func (s *Source) Logs(ctx context.Context, scope Scope, start, end time.Time, search, level string) ([]LogLine, bool, error) {
	q := scope.Selector()
	if level != "" {
		q = strings.TrimSuffix(q, "}") + ",level=" + strconv.Quote(level) + "}"
	}
	if search != "" {
		q += " |= " + strconv.Quote(search)
	}
	out, err := s.get(ctx, s.LogsURL, "/loki/api/v1/query_range", url.Values{"query": {q}, "start": {strconv.FormatInt(start.UnixNano(), 10)}, "end": {strconv.FormatInt(end.UnixNano(), 10)}, "direction": {"backward"}, "limit": {"201"}})
	if err != nil {
		return nil, false, err
	}
	if out.Data.ResultType != "streams" {
		return nil, false, ErrUnavailable
	}
	lines := []LogLine{}
	for _, r := range out.Data.Result {
		for _, v := range r.Values {
			if len(v) != 2 {
				return nil, false, ErrUnavailable
			}
			var ts, line string
			if json.Unmarshal(v[0], &ts) != nil || json.Unmarshal(v[1], &line) != nil {
				return nil, false, ErrUnavailable
			}
			n, e := strconv.ParseInt(ts, 10, 64)
			if e != nil {
				return nil, false, ErrUnavailable
			}
			if len(line) > 8192 {
				line = line[:8192] + " [truncated]"
			}
			lines = append(lines, LogLine{At: time.Unix(0, n).UTC(), Line: Redact(line), Labels: safeLabels(r.Stream)})
		}
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].At.After(lines[j].At) })
	more := len(lines) > 200
	if more {
		lines = lines[:200]
	}
	return lines, more, nil
}
