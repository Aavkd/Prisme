package notifications

import (
	"testing"

	"prisme.local/platform/internal/observability"
)

// Decided by Alexy on 19/10/2026: only these two cross quiet hours.
var productionUrgent = map[string]bool{"VPS-SSH-UNKNOWN-LOGIN": true, "VPS-FAIL2BAN-DOWN": true}

func TestProductionNotificationsCoverEveryRule(t *testing.T) {
	c, err := Load("../../deploy/production/notifications.json")
	if err != nil {
		t.Fatal(err)
	}
	obs, err := observability.Load("../../deploy/production/observability.json")
	if err != nil {
		t.Fatal(err)
	}
	if c.CredentialsFile != "/run/secrets/firebase_service_account" {
		t.Fatalf("credentials must come from the Compose secret, got %q", c.CredentialsFile)
	}
	rules := map[string]observability.Rule{}
	for _, r := range obs.Rules {
		rules[r.ID] = r
	}
	// A rule missing here would never notify anyone.
	if len(c.Rules) != len(rules) {
		t.Fatalf("%d notification policies for %d rules", len(c.Rules), len(rules))
	}
	urgent := map[string]bool{}
	for _, p := range c.Rules {
		r, ok := rules[p.RuleID]
		if !ok {
			t.Fatalf("policy for unknown rule %s", p.RuleID)
		}
		if p.Urgent {
			urgent[p.RuleID] = true
			// The engine only lets critical incidents cross quiet hours.
			if r.Severity != "critical" {
				t.Fatalf("%s is urgent but %s", p.RuleID, r.Severity)
			}
		}
		if p.MaxEscalations != 0 {
			t.Fatalf("%s escalates without a decision", p.RuleID)
		}
	}
	if len(urgent) != len(productionUrgent) {
		t.Fatalf("urgent rules %v, decided %v", urgent, productionUrgent)
	}
	for id := range productionUrgent {
		if !urgent[id] {
			t.Fatalf("%s must be urgent", id)
		}
	}
}
