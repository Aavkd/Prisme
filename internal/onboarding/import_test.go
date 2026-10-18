package onboarding

import (
	"encoding/json"
	"prisme.local/platform/internal/inventory"
	"strings"
	"testing"
	"time"
)

func TestArpentDriftPreservesSharedCaddy(t *testing.T) {
	now := time.Now().UTC()
	snapshot := inventory.Snapshot{Protocol: "1", Source: "ssh-readonly", ObservedAt: now, Host: inventory.Host{Hostname: "fixture", CPUs: 4}, DockerAvailable: true}
	services := map[string]any{}
	for _, name := range []string{"web", "pipeline", "ai", "postgres"} {
		services[name] = map[string]any{"image": "ghcr.io/fixture/" + name + "@sha256:" + strings.Repeat("a", 64), "environment": map[string]string{"DATABASE_PASSWORD": "never-in-report"}}
		snapshot.Containers = append(snapshot.Containers, inventory.Container{ID: name, Name: name, Project: "arpent", Service: name, State: "running", Image: "ghcr.io/fixture/" + name + "@sha256:" + strings.Repeat("a", 64)})
	}
	snapshot.Normalize()
	host, _ := json.Marshal(map[string]any{"services": services})
	services["caddy"] = map[string]any{"image": "caddy:2", "ports": []any{map[string]any{"published": "80", "target": 80}}}
	declared, _ := json.Marshal(map[string]any{"services": services})
	r, err := Compare("arpent", declared, host, snapshot, now)
	if err != nil || r.Ready {
		t.Fatal("historical Arpent drift accepted", err)
	}
	text, _ := json.Marshal(r)
	if !strings.Contains(string(text), "shared_edge_must_remain_separate:caddy") || !strings.Contains(string(text), "declared_host_drift") {
		t.Fatal("edge conflict not reported")
	}
	if strings.Contains(string(text), "never-in-report") || strings.Contains(string(text), "DATABASE_PASSWORD") {
		t.Fatal("source secret leaked")
	}
	r, err = Compare("arpent", host, host, snapshot, now)
	if err != nil || !r.Ready {
		t.Fatal("aligned immutable import blocked", r.Blockers, err)
	}
	snapshot.ObservedAt = now.Add(-6 * time.Minute)
	r, err = Compare("arpent", host, host, snapshot, now)
	if err != nil || r.Ready {
		t.Fatal("stale evidence accepted")
	}
}

func TestImportFailsClosedOnPortsAndPartialEvidence(t *testing.T) {
	if _, err := summarize([]byte(`{"services":{"web":{"ports":["80:80"]}}}`)); err == nil {
		t.Fatal("non-normalized ports accepted")
	}
	if _, err := summarize([]byte(`{"services":{"web":{"ports":[{"published":"80-90"}]}}}`)); err == nil {
		t.Fatal("ambiguous ports accepted")
	}
	if _, err := summarize([]byte(`{"services":{"$(command)":{"image":"image"}}}`)); err == nil {
		t.Fatal("unsafe name accepted")
	}
}
