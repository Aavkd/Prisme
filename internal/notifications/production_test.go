package notifications

import (
	"regexp"
	"testing"

	"prisme.local/platform/internal/observability"
)

// Decided by Alexy on 19/10/2026: an unknown SSH login, fail2ban down and any site
// unreachable or failing cross quiet hours; certificate warnings and the rest wait.
var siteHTTP = regexp.MustCompile(`^SITE-.+-HTTP$`)

func productionUrgent(id string) bool {
	return id == "VPS-SSH-UNKNOWN-LOGIN" || id == "VPS-FAIL2BAN-DOWN" || siteHTTP.MatchString(id)
}

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
	for _, p := range c.Rules {
		r, ok := rules[p.RuleID]
		if !ok {
			t.Fatalf("policy for unknown rule %s", p.RuleID)
		}
		if p.Urgent != productionUrgent(p.RuleID) {
			t.Fatalf("%s urgent=%v, decided %v", p.RuleID, p.Urgent, productionUrgent(p.RuleID))
		}
		// The engine only lets critical incidents cross quiet hours.
		if p.Urgent && r.Severity != "critical" {
			t.Fatalf("%s is urgent but %s", p.RuleID, r.Severity)
		}
		if p.MaxEscalations != 0 {
			t.Fatalf("%s escalates without a decision", p.RuleID)
		}
	}
}
