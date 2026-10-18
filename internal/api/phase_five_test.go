package api_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"prisme.local/platform/internal/notifications"
)

type pushRecorder struct {
	messages []notifications.Payload
	err      error
}

func (p *pushRecorder) Send(_ context.Context, _ string, message notifications.Payload) error {
	if p.err != nil {
		return p.err
	}
	p.messages = append(p.messages, message)
	return nil
}
func TestPhaseFiveNotifications(t *testing.T) {
	f := newFixture(t)
	f.dev()
	now := time.Date(2026, 9, 8, 23, 0, 0, 0, time.UTC)
	sender := &pushRecorder{}
	engine := &notifications.Engine{DB: f.db, Sender: sender, Now: func() time.Time { return now }, Config: notifications.Config{GroupSeconds: 60, Rules: []notifications.RulePolicy{{RuleID: "site", Urgent: true, EscalateSeconds: 60, MaxEscalations: 2}, {RuleID: "worker"}}}}
	f.app.Notifications = engine
	settings := map[string]any{"token": "fixture-registration-token", "enabled": true, "timezone": "Europe/Paris", "quiet_start": 1320, "quiet_end": 420}
	f.call("PUT", "/v1/notifications", settings, 200)
	settings["timezone"] = "not/a/timezone"
	f.call("PUT", "/v1/notifications", settings, 400)
	var id string
	if err := f.db.QueryRow(`INSERT INTO incidents(rule_id,scope_id,title,severity,status,opened_at,updated_at) VALUES('site','local','secret-host-name','critical','detected',$1,$1) RETURNING id::text`, now).Scan(&id); err != nil {
		t.Fatal(err)
	}
	tick := func() {
		t.Helper()
		if err := engine.Tick(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	tick()
	tick()
	if len(sender.messages) != 1 {
		t.Fatalf("duplicate or missing push: %d", len(sender.messages))
	}
	p := sender.messages[0]
	raw, _ := json.Marshal(p)
	if !p.Urgent || len(p.IncidentID) != 64 || strings.Contains(string(raw), "secret-host-name") {
		t.Fatal("unsafe payload")
	}
	f.callAs(f.client, "", "GET", "/v1/notifications/incidents/"+p.IncidentID, nil, 401)
	if f.call("GET", "/v1/notifications/incidents/"+p.IncidentID, nil, 200)["incident"].(map[string]any)["id"] != id {
		t.Fatal("wrong incident")
	}
	now = now.Add(61 * time.Second)
	tick()
	if len(sender.messages) != 2 {
		t.Fatal("escalation missing")
	}
	f.call("POST", "/v1/incidents/"+id+"/events", map[string]any{"action": "acknowledge", "note": "", "revision": 1}, 200)
	now = now.Add(61 * time.Second)
	tick()
	if len(sender.messages) != 2 {
		t.Fatal("ack did not stop escalation")
	}
	if _, err := f.db.Exec(`INSERT INTO incidents(rule_id,scope_id,title,severity,status,opened_at,updated_at) VALUES('worker','local','worker idle is not urgent','critical','detected',$1,$1)`, now); err != nil {
		t.Fatal(err)
	}
	tick()
	if len(sender.messages) != 2 {
		t.Fatal("nonurgent bypassed quiet hours")
	}
	f.call("DELETE", "/v1/devices/"+strings.Repeat("d", 64), nil, 204)
	now = now.Add(12 * time.Hour)
	tick()
	if len(sender.messages) != 2 {
		t.Fatal("revoked device received push")
	}
}
func TestPhaseFiveDeliveryRetryAndTokenExpiry(t *testing.T) {
	f := newFixture(t)
	f.dev()
	now := time.Now()
	f.call("PUT", "/v1/notifications", map[string]any{"token": "token", "enabled": true, "timezone": "UTC", "quiet_start": 0, "quiet_end": 0}, 200)
	_, err := f.db.Exec(`INSERT INTO incidents(rule_id,scope_id,title,severity,status,opened_at,updated_at) VALUES('local','local','fixture','warning','detected',$1,$1)`, now)
	if err != nil {
		t.Fatal(err)
	}
	sender := &pushRecorder{err: notifications.ErrDelivery}
	e := &notifications.Engine{DB: f.db, Sender: sender, Now: func() time.Time { return now }, Config: notifications.Config{Rules: []notifications.RulePolicy{{RuleID: "local"}}}}
	if err = e.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	var attempts int
	var state string
	f.db.QueryRow(`SELECT attempts,state FROM notification_deliveries`).Scan(&attempts, &state)
	if attempts != 1 || state != "pending" {
		t.Fatal("failed delivery not durable")
	}
	now = now.Add(2 * time.Minute)
	sender.err = notifications.ErrUnregistered
	if err = e.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	var enabled bool
	f.db.QueryRow(`SELECT enabled FROM notification_devices`).Scan(&enabled)
	if enabled {
		t.Fatal("expired token remains active")
	}
}
func TestPhaseFiveActionProofBinding(t *testing.T) {
	f, e, _ := deploymentFixture(t)
	key := newVirtualKey(t)
	flow, begin := f.begin(strings.Repeat("c", 64), "register", f.app.IdentityBootstrap)
	f.finish(flow, begin, key, true)
	native, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub, _ := x509.MarshalPKIXPublicKey(&native.PublicKey)
	f.call("POST", "/v1/action-key", map[string]string{"public_key": base64.StdEncoding.EncodeToString(pub)}, 201)
	j := prepareJob(t, e, "release-1", "phase-five-proof", false)
	challenge := f.call("POST", "/v1/action-challenges", map[string]any{"job_id": j.ID, "revision": 1, "method": "android_key"}, 201)
	id := challenge["id"].(string)
	path := "/v1/action-challenges/" + id
	digest := sha256.Sum256([]byte(challenge["challenge"].(string)))
	sig, _ := ecdsa.SignASN1(rand.Reader, native, digest[:])
	f.call("POST", path+"/sign", map[string]string{"signature": base64.StdEncoding.EncodeToString([]byte("forged"))}, 403)
	f.call("POST", path+"/sign", map[string]string{"signature": base64.StdEncoding.EncodeToString(sig)}, 200)
	f.call("POST", path+"/sign", map[string]string{"signature": base64.StdEncoding.EncodeToString(sig)}, 403)
	// A rejected target phrase must not burn a valid proof.
	decision := map[string]any{"action": "approve", "revision": 1, "phrase": "wrong", "proof": id}
	f.call("POST", "/v1/deployments/"+j.ID+"/decision", decision, 409)
	var used bool
	f.db.QueryRow(`SELECT consumed FROM action_challenges WHERE id=$1`, id).Scan(&used)
	if used {
		t.Fatal("proof consumed on failed decision")
	}
	decision["phrase"] = j.TargetID
	f.call("POST", "/v1/deployments/"+j.ID+"/decision", decision, 200)
	f.db.QueryRow(`SELECT consumed FROM action_challenges WHERE id=$1`, id).Scan(&used)
	if !used {
		t.Fatal("proof was not consumed")
	}
	other := prepareJob(t, e, "release-2", "phase-five-other", false)
	f.call("POST", "/v1/deployments/"+other.ID+"/decision", decision, 409)
	f.call("GET", path, nil, 404)
}
func TestPhaseFivePasskeyUVAndExpiration(t *testing.T) {
	f, e, _ := deploymentFixture(t)
	key := newVirtualKey(t)
	flow, begin := f.begin(strings.Repeat("c", 64), "register", f.app.IdentityBootstrap)
	f.finish(flow, begin, key, true)
	j := prepareJob(t, e, "release-1", "phase-five-passkey", false)
	for _, uv := range []bool{false, true} {
		challenge := f.call("POST", "/v1/action-challenges", map[string]any{"job_id": j.ID, "revision": 1, "method": "passkey"}, 201)
		path := "/v1/action-challenges/" + challenge["id"].(string)
		options := f.call("GET", path+"/options", nil, 200)
		want := 403
		if uv {
			want = 200
		}
		f.callAs(f.client, "", "POST", path+"/passkey", map[string]any{"credential": key.credential(t, options, false, "https://localhost:8443", uv)}, want)
		if uv {
			f.call("GET", path, nil, 200)
			_, err := f.db.Exec(`UPDATE action_challenges SET expires_at=now()-interval '1 second' WHERE id=$1`, challenge["id"])
			if err != nil {
				t.Fatal(err)
			}
			f.call("POST", "/v1/deployments/"+j.ID+"/decision", map[string]any{"action": "approve", "revision": 1, "phrase": j.TargetID, "proof": challenge["id"]}, 409)
		}
	}
	f.dev()
	f.call("POST", "/v1/action-challenges", map[string]any{"job_id": j.ID, "revision": 1, "method": "passkey"}, 403)
}
