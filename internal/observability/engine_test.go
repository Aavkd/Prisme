package observability

import (
	"testing"
	"time"
)

func testRule() Rule {
	return Rule{ID: "test", ScopeID: "pilot", Title: "Pilot", Metric: "probe_success", Operator: "below", OpenThreshold: 1, ResolveThreshold: 1, ForSeconds: 10, RecoverySeconds: 10, MinSamples: 3, FreshnessSeconds: 30, Severity: "critical", Enabled: true}
}
func sample(now time.Time, v float64) *Point { return &Point{At: now, Value: &v} }
func TestAlertEvidenceAndRecovery(t *testing.T) {
	now := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	r := testRule()
	var state RuleState
	var event string
	state, event = Advance(state, r, sample(now, 0), now, "")
	if event != "" {
		t.Fatal("opened on first sample")
	}
	for range 10 {
		state, event = Advance(state, r, sample(now, 0), now.Add(time.Second), "")
		if event != "" || state.FailSamples != 1 {
			t.Fatal("duplicate scrape counted")
		}
	}
	state, event = Advance(state, r, sample(now.Add(5*time.Second), 0), now.Add(5*time.Second), "")
	if event != "" {
		t.Fatal("too early")
	}
	state, event = Advance(state, r, sample(now.Add(10*time.Second), 0), now.Add(10*time.Second), "")
	if event != "detected" {
		t.Fatal("not detected")
	}
	state.IncidentID = 42
	state, event = Advance(state, r, nil, now.Add(15*time.Second), "")
	if event != "" || state.Signal != "unknown" || state.IncidentID != 42 {
		t.Fatal("missing data resolved incident")
	}
	for i := 0; i < 3; i++ {
		at := now.Add(time.Duration(20+i*5) * time.Second)
		state, event = Advance(state, r, sample(at, 1), at, "")
		if i < 2 && event != "" {
			t.Fatal("early recovery")
		}
	}
	if event != "resolved" {
		t.Fatal("did not resolve")
	}
}
func TestGapsMaintenanceDependenciesAndHysteresis(t *testing.T) {
	now := time.Now().UTC()
	r := testRule()
	r.Operator = "above"
	r.OpenThreshold = 90
	r.ResolveThreshold = 75
	state, _ := Advance(RuleState{}, r, sample(now, 95), now, "")
	state, event := Advance(state, r, sample(now.Add(time.Minute), 95), now.Add(time.Minute), "")
	if event != "" || state.FailSamples != 1 {
		t.Fatal("gap bridged")
	}
	state.IncidentID = 1
	state, event = Advance(state, r, sample(now.Add(65*time.Second), 80), now.Add(65*time.Second), "")
	if event != "" || state.Signal != "unstable" {
		t.Fatal("hysteresis ignored")
	}
	state, event = Advance(state, r, sample(now.Add(70*time.Second), 0), now.Add(70*time.Second), "dependency:host")
	if event != "" || state.Signal != "inhibited" || state.IncidentID != 1 {
		t.Fatal("dependency resolved incident")
	}
	until := now.Add(time.Hour)
	r.MaintenanceUntil = &until
	state, event = Advance(state, r, sample(now, 95), now, "")
	if state.Signal != "inhibited" || state.Reason != "maintenance" || event != "" {
		t.Fatal("maintenance ignored")
	}
	r.MaintenanceUntil = nil
	r.Enabled = false
	state, event = Advance(state, r, sample(now, 95), now, "")
	if state.Reason != "disabled" || event != "" {
		t.Fatal("disabled rule opened")
	}
}
func TestInvalidFutureStaleAndChangedRule(t *testing.T) {
	now := time.Now().UTC()
	r := testRule()
	for _, p := range []*Point{nil, {At: now}, sample(now.Add(time.Minute), 0), sample(now.Add(-time.Minute), 1)} {
		state, event := Advance(RuleState{}, r, p, now, "")
		if state.Signal != "unknown" || event != "" {
			t.Fatal("unknown evidence accepted")
		}
	}
	state, _ := Advance(RuleState{}, r, sample(now, 0), now, "")
	r.ForSeconds = 20
	state, event := Advance(state, r, sample(now.Add(5*time.Second), 0), now.Add(5*time.Second), "")
	if state.FailSamples != 1 || event != "" {
		t.Fatal("changed rule reused evidence")
	}
}
func TestConfigValidation(t *testing.T) {
	c := Config{Scopes: []Scope{{ID: "pilot", Server: "local", Project: "pilot", Environment: "development", Service: "web"}}, Rules: []Rule{testRule()}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.Rules[0].InhibitedBy = []string{"test"}
	if c.Validate() == nil {
		t.Fatal("cycle accepted")
	}
	c.Rules[0].InhibitedBy = nil
	c.Scopes[0].Service = `web"} or up`
	if c.Validate() == nil {
		t.Fatal("query injection accepted")
	}
	if _, ok := Expression("up{} or vector(1)", Scope{}); ok {
		t.Fatal("arbitrary expression accepted")
	}
}
