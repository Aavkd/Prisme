package observability

import (
	"testing"
	"time"
)

func productionRule(t *testing.T, id string) Rule {
	t.Helper()
	c, err := Load("../../deploy/production/observability.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range c.Rules {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("rule %s missing from the production configuration", id)
	return Rule{}
}

// replay feeds one sample every 15 s (Alloy's host scrape) and returns the
// offsets, in seconds, at which the rule opened or resolved.
func replay(r Rule, values []float64) (opened, resolved []int) {
	start := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	var state RuleState
	for i, v := range values {
		at := start.Add(time.Duration(15*i) * time.Second)
		var event string
		state, event = Advance(state, r, sample(at, v), at, "")
		switch event {
		case "detected":
			opened = append(opened, 15*i)
			state.IncidentID = 1
		case "resolved":
			resolved = append(resolved, 15*i)
			state.IncidentID = 0
		}
	}
	return opened, resolved
}

func repeat(v float64, seconds int) []float64 {
	out := make([]float64, seconds/15)
	for i := range out {
		out[i] = v
	}
	return out
}

func concat(parts ...[]float64) []float64 {
	var out []float64
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// Thresholds chosen by Alexy on 19/10/2026 from 30 days of ovh-mantara journal:
// the worst normal 10-minute window held 236 failures only briefly, never 150 for 10 minutes.
func TestProductionSSHFailureThresholdIgnoresMeasuredNoise(t *testing.T) {
	r := productionRule(t, "VPS-SSH-FAILURES")
	if opened, _ := replay(r, concat(repeat(14, 3600), repeat(141, 1800), repeat(236, 120), repeat(97, 600))); len(opened) != 0 {
		t.Fatalf("normal noise opened an incident at %v", opened)
	}
	if opened, _ := replay(r, concat(repeat(151, 540), repeat(120, 60), repeat(151, 540))); len(opened) != 0 {
		t.Fatalf("interrupted bursts opened an incident at %v", opened)
	}
	opened, resolved := replay(r, concat(repeat(10, 300), repeat(400, 900), repeat(20, 600)))
	if len(opened) != 1 || opened[0] != 300+600 || len(resolved) != 1 || resolved[0] != 300+900+300 {
		t.Fatalf("sustained attack: opened %v resolved %v", opened, resolved)
	}
	if r.Severity != "warning" {
		t.Fatalf("severity %s", r.Severity)
	}
}

func TestProductionSSHSecurityRulesOpenFastAndAreCritical(t *testing.T) {
	login := productionRule(t, "VPS-SSH-UNKNOWN-LOGIN")
	// The collector keeps the gauge at 1 for 30 minutes after the login.
	opened, resolved := replay(login, concat(repeat(0, 300), repeat(1, 1800), repeat(0, 300)))
	if len(opened) != 1 || opened[0] != 300+15 || len(resolved) != 1 || resolved[0] != 300+1800+60 {
		t.Fatalf("unknown login: opened %v resolved %v", opened, resolved)
	}
	down := productionRule(t, "VPS-FAIL2BAN-DOWN")
	if opened, _ := replay(down, concat(repeat(1, 300), repeat(0, 45), repeat(1, 300))); len(opened) != 0 {
		t.Fatalf("fail2ban reload opened an incident at %v", opened)
	}
	if opened, _ := replay(down, concat(repeat(1, 300), repeat(0, 120))); len(opened) != 1 || opened[0] != 300+60 {
		t.Fatalf("fail2ban stopped: opened %v", opened)
	}
	for _, r := range []Rule{login, down} {
		if r.Severity != "critical" || !r.Enabled {
			t.Fatalf("%s must be an enabled critical rule", r.ID)
		}
	}
	for _, id := range []string{"VPS-SSH-COLLECTOR", "VPS-FAIL2BAN-BLIND"} {
		if r := productionRule(t, id); r.Severity != "warning" {
			t.Fatalf("%s severity %s", id, r.Severity)
		}
	}
}
