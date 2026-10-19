package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSourcesBoundedAndRedacted(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	scope := Scope{ID: "pilot", Server: "local", Project: "pilot", Environment: "development", Service: "web"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/loki/api/v1/query_range" {
			if !strings.Contains(r.URL.Query().Get("query"), `|= "\"} |= \"injection"`) {
				t.Error("search not quoted")
			}
			json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "streams", "result": []any{map[string]any{"stream": map[string]string{"service": "web", "secret": "must-not-leak"}, "values": [][]string{{fmt.Sprint(now.UnixNano()), `password=do-not-print Authorization: Bearer abc123 token="other-secret"`}}}}}})
		} else {
			if !strings.HasSuffix(r.URL.Query().Get("query"), "[5m]") {
				t.Error("lost sample timestamp selector")
			}
			fmt.Fprintf(w, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[[%d,"0"]]}]}}`, now.Unix())
		}
	}))
	defer server.Close()
	source, _ := NewSource(server.URL, server.URL, true)
	p, err := source.Latest(context.Background(), scope, "probe_success", now)
	if err != nil || p == nil || !p.At.Equal(now) || *p.Value != 0 {
		t.Fatal("lost sample")
	}
	logs, more, err := source.Logs(context.Background(), scope, now.Add(-time.Minute), now, `"} |= "injection`, "")
	if err != nil || more || len(logs) != 1 {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(logs)
	for _, secret := range []string{"do-not-print", "abc123", "other-secret", "must-not-leak"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("secret escaped redaction")
		}
	}
}
func TestStorageFailureAndMissingData(t *testing.T) {
	now := time.Now()
	scope := Scope{Server: "a", Project: "b", Environment: "c", Service: "d"}
	for _, body := range []string{`{"status":"success","data":{"resultType":"matrix","result":[]}}`, `{"status":"success","data":{"resultType":"matrix","result":[{"values":[[1,"NaN"]]}]}}`, `{"status":"success","data":{"resultType":"matrix","result":[{},{}]}}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		source, _ := NewSource(server.URL, server.URL, true)
		p, err := source.Latest(context.Background(), scope, "up", now)
		server.Close()
		if err != nil || p != nil && p.Value != nil {
			t.Fatal("missing sample became zero")
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "private backend error", 503) }))
	defer server.Close()
	source, _ := NewSource(server.URL, server.URL, true)
	_, err := source.Latest(context.Background(), scope, "up", now)
	if err != ErrUnavailable {
		t.Fatal("backend failure not sanitized")
	}
	if _, err = NewSource(server.URL, server.URL, false); err == nil {
		t.Fatal("cleartext allowed outside development")
	}
}
func TestInternalSourceAllowsCleartextOnlyToServiceNames(t *testing.T) {
	if _, err := NewInternalSource("http://victoriametrics:8428", "http://loki:3100/"); err != nil {
		t.Fatal("internal Compose services refused", err)
	}
	if _, err := NewInternalSource("https://metrics.example.com", "http://loki:3100"); err != nil {
		t.Fatal("HTTPS store refused", err)
	}
	for _, v := range []string{"http://127.0.0.1:8428", "http://10.0.0.5:8428", "http://[::1]:8428", "http://metrics.example.com:8428", "http://Loki:3100", "http://user:pass@loki:3100", "http://loki:3100/api", "http://loki:3100?x=1", "ftp://loki:3100", "http://"} {
		if _, err := NewInternalSource("http://victoriametrics:8428", v); err == nil {
			t.Fatalf("unsafe telemetry URL %q accepted", v)
		}
	}
}
func TestRedaction(t *testing.T) {
	for _, v := range []string{`{"api_key":"sensitive"}`, `cookie=sensitive`, "Bearer sensitive", `https://user:sensitive@example.org`, "-----BEGIN PRIVATE KEY-----\nsensitive\n-----END PRIVATE KEY-----"} {
		if strings.Contains(Redact(v), "sensitive") {
			t.Fatalf("redaction failed for category")
		}
	}
}
func TestRuleMetricsKeepOriginalSampleTimestamps(t *testing.T) {
	sampled := time.Now().UTC().Add(-20 * time.Second).Truncate(time.Second)
	expiry := sampled.Add(10*24*time.Hour + 12*time.Hour)
	scope := Scope{ID: "site", Server: "ovh-mantara", Project: "mantara.fr", Environment: "production", Service: "web"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if q := r.URL.Query().Get("query"); q != "probe_ssl_earliest_cert_expiry"+scope.Selector()+"[5m]" {
			t.Errorf("unexpected rule query %q", q)
		}
		fmt.Fprintf(w, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[[%d,"%d"]]}]}}`, sampled.Unix(), expiry.Unix())
	}))
	defer server.Close()
	source, _ := NewSource(server.URL, server.URL, true)
	p, err := source.Latest(context.Background(), scope, "probe_ssl_expiry_days", time.Now())
	if err != nil || p == nil || !p.At.Equal(sampled) || *p.Value != 10.5 {
		t.Fatalf("expiry days not computed from the sample itself: %+v %v", p, err)
	}
	age := ruleTransform(t, "prisme_host_metrics_age_seconds")
	if v := age(sampled, float64(sampled.Unix()-240)); v != 240 {
		t.Fatalf("host collector age %v", v)
	}
	if v := ruleTransform(t, "prisme_ssh_metrics_age_seconds")(sampled, float64(sampled.Unix()-75)); v != 75 {
		t.Fatalf("SSH collector age %v", v)
	}
	for _, m := range []string{"probe_ssl_expiry_days", "prisme_host_metrics_age_seconds", "node_filesystem_avail_bytes", "node_memory_SwapFree_bytes", "prisme_containers_problem", "prisme_drill", "probe_success",
		"prisme_ssh_failures_10m", "prisme_ssh_unknown_logins_30m", "prisme_fail2ban_up", "prisme_fail2ban_blind", "prisme_ssh_metrics_age_seconds"} {
		if !RuleMetric(m) {
			t.Fatalf("%s refused in rules", m)
		}
	}
	// Rates and ratios use synthetic timestamps: charts only, never rules.
	for _, m := range []string{"host_cpu_percent", "host_memory_percent", "host_disk_percent", "container_cpu_percent", "up or vector(1)"} {
		if RuleMetric(m) {
			t.Fatalf("%s accepted in rules", m)
		}
	}
	if e, ok := Expression("probe_ssl_expiry_days", scope); !ok || !strings.Contains(e, "- time()") {
		t.Fatal("expiry days not chartable")
	}
}

func ruleTransform(t *testing.T, metric string) func(time.Time, float64) float64 {
	t.Helper()
	_, transform, ok := ruleSeries(metric)
	if !ok || transform == nil {
		t.Fatalf("%s has no sample transform", metric)
	}
	return transform
}
