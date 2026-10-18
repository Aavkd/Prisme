package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"prisme.local/platform/internal/observability"
	"prisme.local/platform/internal/store"
)

func TestPhaseTwoIncidentLifecycle(t *testing.T) {
	f := newFixture(t)
	f.dev()
	now := time.Now().UTC().Truncate(time.Second)
	rule := observability.Rule{ID: "PILOT-HTTP", ScopeID: "pilot", Title: "Pilot unavailable", Metric: "probe_success", Operator: "below", OpenThreshold: 1, ResolveThreshold: 1, ForSeconds: 10, RecoverySeconds: 10, MinSamples: 3, FreshnessSeconds: 30, Severity: "critical", Enabled: true}
	config := observability.Config{Scopes: []observability.Scope{{ID: "pilot", Server: "local", Project: "pilot", Environment: "development", Service: "web"}}, Rules: []observability.Rule{rule}}
	engine := &observability.Engine{DB: f.db, Config: config, Now: func() time.Time { return now }}
	f.app.Observability = engine
	apply := func(v float64) {
		t.Helper()
		err := engine.Apply(context.Background(), rule, &observability.Point{At: now, Value: &v}, now)
		if err != nil {
			t.Fatal(err)
		}
		now = now.Add(5 * time.Second)
	}
	for range 3 {
		apply(0)
	}
	if err := engine.ValidateStoredScopes(context.Background()); err != nil {
		t.Fatal(err)
	}
	changed := config
	changed.Scopes = append([]observability.Scope(nil), config.Scopes...)
	changed.Scopes[0].Service = "different"
	if (&observability.Engine{DB: f.db, Config: changed}).ValidateStoredScopes(context.Background()) == nil {
		t.Fatal("historical scope reassigned")
	}
	page := f.call("GET", "/v1/incidents", nil, 200)
	items := page["items"].([]any)
	if len(items) != 1 {
		t.Fatal("expected one incident")
	}
	id := items[0].(map[string]any)["id"].(string)
	// Repeated/concurrent evaluation never creates duplicate incidents or events.
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			value := 0.0
			if err := engine.Apply(context.Background(), rule, &observability.Point{At: now, Value: &value}, now); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if err := store.Migrate(context.Background(), f.db); err != nil {
		t.Fatal(err)
	}
	detail := f.call("GET", "/v1/incidents/"+id, nil, 200)
	if len(detail["events"].([]any)) != 1 {
		t.Fatal("duplicate opening")
	}
	action := map[string]any{"action": "acknowledge", "note": "password=private-value", "revision": 1}
	f.call("POST", "/v1/incidents/"+id+"/events", action, 200)
	f.call("POST", "/v1/incidents/"+id+"/events", action, 409)
	action["action"] = "close"
	action["revision"] = 2
	f.call("POST", "/v1/incidents/"+id+"/events", action, 409)
	if err := engine.Apply(context.Background(), rule, nil, now); err != nil {
		t.Fatal(err)
	}
	detail = f.call("GET", "/v1/incidents/"+id, nil, 200)
	if detail["incident"].(map[string]any)["status"] != "acknowledged" {
		t.Fatal("missing telemetry cleared incident")
	}
	// New engine instance simulates restart; pending recovery survives in PostgreSQL.
	engine = &observability.Engine{DB: f.db, Config: config}
	now = now.Add(5 * time.Second)
	for range 3 {
		apply(1)
	}
	detail = f.call("GET", "/v1/incidents/"+id, nil, 200)
	if detail["incident"].(map[string]any)["status"] != "resolved" {
		t.Fatal("not resolved")
	}
	for range 3 {
		apply(0)
	}
	detail = f.call("GET", "/v1/incidents/"+id, nil, 200)
	if detail["incident"].(map[string]any)["status"] != "reopened" {
		t.Fatal("recurrence not reopened")
	}
	raw, _ := json.Marshal(detail)
	if strings.Contains(string(raw), "private-value") {
		t.Fatal("note secret persisted")
	}
	for range 3 {
		apply(1)
	}
	detail = f.call("GET", "/v1/incidents/"+id, nil, 200)
	action["revision"] = detail["incident"].(map[string]any)["revision"]
	f.call("POST", "/v1/incidents/"+id+"/events", action, 200)
	var auditCount int
	if err := f.db.QueryRow("SELECT count(*) FROM audit_events WHERE action LIKE 'incident.%'").Scan(&auditCount); err != nil || auditCount < 6 {
		t.Fatal("missing audit")
	}
}

func TestPhaseTwoQueryBoundaries(t *testing.T) {
	f := newFixture(t)
	f.call("GET", "/v1/observability", nil, 401)
	f.dev()
	if f.call("GET", "/v1/observability", nil, 200)["enabled"] != false {
		t.Fatal("disabled feature advertised")
	}
	f.call("GET", "/v1/observability/metrics", nil, 503)
	telemetry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "storage private failure", 503) }))
	defer telemetry.Close()
	source, _ := observability.NewSource(telemetry.URL, telemetry.URL, true)
	f.app.Observability = &observability.Engine{DB: f.db, Source: source, Config: observability.Config{Scopes: []observability.Scope{{ID: "pilot", Server: "local", Project: "pilot", Environment: "development", Service: "web"}}}}
	now := time.Now().UTC()
	q := url.Values{"scope": {"pilot"}, "metric": {"probe_success"}, "start": {now.Add(-time.Hour).Format(time.RFC3339)}, "end": {now.Format(time.RFC3339)}}
	f.call("GET", "/v1/observability/metrics?"+q.Encode(), nil, 503)
	q.Set("metric", "up{} or vector(1)")
	f.call("GET", "/v1/observability/metrics?"+q.Encode(), nil, 400)
	q.Set("metric", "probe_success")
	q.Set("scope", `web"}`)
	f.call("GET", "/v1/observability/metrics?"+q.Encode(), nil, 400)
	q.Set("scope", "pilot")
	q.Set("start", now.Add(-25*time.Hour).Format(time.RFC3339))
	f.call("GET", "/v1/observability/metrics?"+q.Encode(), nil, 400)
	f.call("GET", "/v1/incidents?cursor=oops", nil, 400)
	f.call("GET", "/v1/incidents/999", nil, 404)
	f.call("POST", "/v1/incidents/999/events", map[string]any{"action": "resolved", "revision": 1, "note": ""}, 400)
	// Pagination is bounded and stable; the cursor is the last returned ID.
	for i := 0; i < 52; i++ {
		_, err := f.db.Exec("INSERT INTO incidents(rule_id,scope_id,title,severity,status,opened_at,updated_at) VALUES($1,'pilot','Fixture','warning','resolved',now(),now())", fmt.Sprint(i))
		if err != nil {
			t.Fatal(err)
		}
	}
	page := f.call("GET", "/v1/incidents", nil, 200)
	if len(page["items"].([]any)) != 50 || page["next_cursor"] == "" {
		t.Fatal("unbounded incident list")
	}
	page = f.call("GET", "/v1/incidents?cursor="+page["next_cursor"].(string), nil, 200)
	if len(page["items"].([]any)) != 2 {
		t.Fatal("cursor skipped rows")
	}
}
