package automation

import (
	"encoding/json"
	"testing"
)

func TestTypedParametersRejectCoercionAndUnknownActions(t *testing.T) {
	c := Config{Version: 1, Targets: []Target{{ID: "pilot", Deployment: "pilot", Server: "local", Project: "pilot", Environment: "test", Service: "web", Rule: "health"}}}
	b := Runbook{ID: "inspect", Version: 1, Target: "pilot", TimeoutSeconds: 60, Parameters: map[string]Parameter{"delay": {Type: "integer", Min: 1, Max: 5}, "enabled": {Type: "boolean"}, "option": {Type: "string", Choices: []string{"a", "b"}}}, Steps: []Step{{Action: "wait", When: "always", Parameter: "delay", TimeoutSeconds: 5}}}
	if b.Validate(c) != nil {
		t.Fatal("valid runbook rejected")
	}
	good := map[string]any{"delay": float64(2), "enabled": true, "option": "a"}
	bound, err := b.Bind(good)
	if err != nil || bound.Steps[0].Seconds != 2 || b.Steps[0].Seconds != 0 {
		t.Fatal("binding changed source", err)
	}
	for _, value := range []any{"2", float64(2.5), float64(-1), float64(6), nil} {
		v := map[string]any{"delay": value, "enabled": true, "option": "a"}
		if _, err = b.Bind(v); err == nil {
			t.Fatal("invalid integer accepted", value)
		}
	}
	good["option"] = "shell"
	if _, err = b.Bind(good); err == nil {
		t.Fatal("enum bypass")
	}
	b.Steps[0].Action = "shell"
	if b.Validate(c) == nil {
		t.Fatal("unknown action accepted")
	}
	b.Steps[0] = Step{Action: "restart", When: "always", TimeoutSeconds: 5, Retries: 1}
	if b.Validate(c) == nil {
		t.Fatal("mutation retry accepted")
	}
	raw, _ := json.Marshal(b)
	var decoded Runbook
	if strictDecode(append(raw, []byte(` {}`)...), &decoded) == nil {
		t.Fatal("trailing JSON accepted")
	}
}
func TestPolicyPlanBoundsEveryMutationAndBackoff(t *testing.T) {
	p := Policy{ID: "repair", Version: 1, Target: "pilot", Actions: []string{"restart", "rollback"}, MaxAttempts: 3, BackoffSeconds: 10, ObservationSeconds: 5, TimeoutSeconds: 300, HealthyRelease: "healthy"}
	b := policyBook(p)
	actions := []string{}
	delays := []int{}
	for _, s := range b.Steps {
		if mutation(s) {
			actions = append(actions, s.Action)
			if s.When != "failed" {
				t.Fatal("unconditional remediation")
			}
		}
		if s.Action == "wait" {
			delays = append(delays, s.Seconds)
		}
	}
	if len(actions) != 3 || actions[2] != "rollback" || len(delays) != 3 || delays[0] != 10 || delays[1] != 20 || delays[2] != 40 {
		t.Fatal(actions, delays)
	}
}
