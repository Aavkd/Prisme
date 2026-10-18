package api_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prisme.local/platform/internal/notifications"
	"prisme.local/platform/internal/observability"
)

// Explicit opt-in: only a disposable SQL schema, a failing local HTTP fixture,
// a native test session and the user's named Android device participate.
func TestPhaseFiveLiveAndroid(t *testing.T) {
	serial := os.Getenv("PRISME_TEST_ANDROID_DEVICE")
	credentials := os.Getenv("PRISME_TEST_FCM_FILE")
	if serial == "" || credentials == "" {
		t.Skip("opt-in physical Android/FCM qualification")
	}
	f, e, _ := deploymentFixture(t)
	ctx := context.Background()
	key := newVirtualKey(t)
	device := strings.Repeat("5", 64)
	flow, begin := f.begin(device, "register", f.app.IdentityBootstrap)
	f.finish(flow, begin, key, true)
	j := prepareJob(t, e, "release-1", "s24-native-qualified-action", false)
	sender, err := notifications.NewFCM(credentials)
	if err != nil {
		t.Fatal(err)
	}
	engine := &notifications.Engine{DB: f.db, Sender: sender, Now: time.Now, Config: notifications.Config{Rules: []notifications.RulePolicy{{RuleID: "S24-LOCAL-HTTP", Urgent: true}}}}
	f.app.Notifications = engine
	u, _ := url.Parse(f.server.URL)
	adb := func(args ...string) *exec.Cmd { return exec.Command("adb", append([]string{"-s", serial}, args...)...) }
	if err = adb("reverse", "tcp:18443", "tcp:"+u.Port()).Run(); err != nil {
		t.Fatal("ADB reverse failed")
	}
	defer adb("reverse", "--remove", "tcp:18443").Run()
	ca, err := os.ReadFile(filepath.Join(f.dir, "ca.pem"))
	if err != nil {
		t.Fatal(err)
	}
	input, _ := json.Marshal(map[string]any{"token": f.token, "device": device, "ca": base64.StdEncoding.EncodeToString(ca), "job": j})
	target := "/data/data/dev.mantara.prisme_client/files/phase5"
	defer adb("shell", "run-as", "dev.mantara.prisme_client", "rm", "-f", target+"/input.json", target+"/ready", target+"/complete").Run()
	t.Log("Waiting for the native phase-five test to request input")
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		if adb("shell", "run-as", "dev.mantara.prisme_client", "test", "-f", target+"/ready").Run() == nil {
			break
		}
		time.Sleep(time.Second)
	}
	// The fixture passkey session is fresh when the native test starts; private
	// inputs are piped directly into its sandbox and never logged or compiled in.
	if _, err = f.db.Exec(`UPDATE sessions SET authenticated_at=now() WHERE device_id=$1`, device); err != nil {
		t.Fatal(err)
	}
	command := adb("shell", "run-as", "dev.mantara.prisme_client", "tee", target+"/input.json")
	command.Stdin = strings.NewReader(string(input))
	if err = command.Run(); err != nil {
		t.Fatal("native input staging failed")
	}
	t.Log("Native input staged; waiting for Android notification permission and registration")
	registered := false
	for time.Now().Before(deadline) {
		if err = f.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM notification_devices WHERE device_id=$1 AND enabled)`, device).Scan(&registered); err != nil {
			t.Fatal(err)
		}
		if registered {
			break
		}
		time.Sleep(time.Second)
	}
	if !registered {
		t.Fatal("phone did not register notifications")
	}
	// Measure a real, deliberately failing disposable HTTP endpoint.
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer service.Close()
	rule := observability.Rule{ID: "S24-LOCAL-HTTP", ScopeID: "s24-local", Title: "S24 · panne HTTP locale de qualification", Metric: "probe_success", Operator: "below", OpenThreshold: 1, ResolveThreshold: 1, ForSeconds: 2, RecoverySeconds: 2, MinSamples: 3, FreshnessSeconds: 30, Severity: "critical", Enabled: true}
	observation := &observability.Engine{DB: f.db, Config: observability.Config{Scopes: []observability.Scope{{ID: "s24-local", Server: "local", Project: "qualification", Environment: "test", Service: "http"}}, Rules: []observability.Rule{rule}}, Now: time.Now}
	for range 3 {
		res, err := service.Client().Get(service.URL)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		value := 0.0
		if res.StatusCode == 200 {
			value = 1
		}
		now := time.Now()
		if err = observation.Apply(ctx, rule, &observability.Point{At: now, Value: &value}, now); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Second)
	}
	if err = engine.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	var sent int
	f.db.QueryRow(`SELECT count(*) FROM notification_deliveries WHERE state='sent'`).Scan(&sent)
	if sent != 1 {
		t.Fatal("FCM did not accept the alert")
	}
	t.Log("FCM accepted the real local HTTP alert; open the notification on S24, then confirm the test approval with a fingerprint")
	complete := false
	for time.Now().Before(deadline) {
		if adb("shell", "run-as", "dev.mantara.prisme_client", "test", "-f", target+"/complete").Run() == nil {
			complete = true
			break
		}
		time.Sleep(time.Second)
	}
	if !complete {
		t.Fatal("native notification/biometric flow did not complete")
	}
	job, err := e.Job(ctx, j.ID)
	if err != nil || job.State != "ready" {
		t.Fatal("native approval was not committed")
	}
	var proofs int
	f.db.QueryRow(`SELECT count(*) FROM action_challenges WHERE job_id=$1 AND method='android_key' AND approved AND consumed`, j.ID).Scan(&proofs)
	if proofs != 1 {
		t.Fatal("native biometric proof was not consumed once")
	}
	t.Log("Verified: HTTP failure -> durable incident -> real FCM -> native incident view -> Android biometric signature -> atomic approval; encrypted native cache checked")
}
