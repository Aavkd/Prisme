package api_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prisme.local/platform/internal/api"
	"prisme.local/platform/internal/coordination"
	"prisme.local/platform/internal/deployment"
	"prisme.local/platform/internal/operations"
	"prisme.local/platform/internal/recovery"
)

func operationsFixture(t *testing.T) (*fixture, *virtualKey) {
	f := newFixture(t)
	key := newVirtualKey(t)
	flow, begin := f.begin(strings.Repeat("c", 64), "register", f.app.IdentityBootstrap)
	f.finish(flow, begin, key, true)
	master := make([]byte, 32)
	_, _ = rand.Read(master)
	file := filepath.Join(f.dir, "vault-master")
	if e := os.WriteFile(file, master, 0600); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if e := f.app.ConfigureOperations(ctx, operations.Config{MasterKeyFile: file, Targets: []operations.Target{{ID: "local"}}}); e != nil {
		t.Fatal(e)
	}
	return f, key
}
func operationProof(t *testing.T, f *fixture, key *virtualKey, intent map[string]any) string {
	t.Helper()
	challenge := f.call("POST", "/v1/operations/challenges", map[string]any{"intent": intent, "method": "passkey"}, 200)
	id := challenge["id"].(string)
	options := f.call("GET", "/v1/operations/challenges/"+id+"/options", nil, 200)
	f.callAs(f.client, "", "POST", "/v1/operations/challenges/"+id+"/passkey", map[string]any{"credential": key.credential(t, options, false, "https://localhost:8443", true)}, 200)
	return id
}
func TestPhaseSixVaultProofRotationAndRevocation(t *testing.T) {
	f, key := operationsFixture(t)
	put := map[string]any{"action": "secret_put", "target": "local", "payload": map[string]any{"secret": map[string]any{"id": "", "name": "REGISTRY_TOKEN", "scope": "pilot", "kind": "registry", "destination": "local", "revision": 0, "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)}, "value": "fixture-secret-never-in-metadata"}}
	id := operationProof(t, f, key, put)
	created := f.call("POST", "/v1/operations/execute", map[string]any{"intent": put, "proof": id}, 200)
	secret := created["secret"].(map[string]any)
	secretID := secret["id"].(string)
	f.call("POST", "/v1/operations/execute", map[string]any{"intent": put, "proof": id}, 403)
	overview := f.call("GET", "/v1/operations/overview", nil, 200)
	raw, _ := json.Marshal(overview)
	if strings.Contains(string(raw), "fixture-secret-never-in-metadata") {
		t.Fatal("secret in metadata")
	}
	var envelope, audit string
	if e := f.db.QueryRow(`SELECT envelope::text FROM vault_secrets WHERE id=$1`, secretID).Scan(&envelope); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(envelope, "fixture-secret-never-in-metadata") {
		t.Fatal("plaintext at rest")
	}
	reveal := map[string]any{"action": "secret_reveal", "target": "local", "payload": map[string]any{"id": secretID, "revision": 1}}
	proof := operationProof(t, f, key, reveal)
	out := f.call("POST", "/v1/operations/execute", map[string]any{"intent": reveal, "proof": proof}, 200)
	if out["value"] != "fixture-secret-never-in-metadata" {
		t.Fatal("vault decryption failed")
	}
	secret["revision"] = 1
	put["payload"].(map[string]any)["secret"] = secret
	put["payload"].(map[string]any)["value"] = "rotated-fixture-value"
	f.call("POST", "/v1/operations/execute", map[string]any{"intent": put, "proof": operationProof(t, f, key, put)}, 200)
	f.call("POST", "/v1/operations/execute", map[string]any{"intent": reveal, "proof": operationProof(t, f, key, reveal)}, 403)
	reveal["payload"].(map[string]any)["revision"] = 2
	proof = operationProof(t, f, key, reveal)
	if _, e := f.db.Exec(`UPDATE devices SET revoked=true WHERE id=$1`, strings.Repeat("c", 64)); e != nil {
		t.Fatal(e)
	}
	f.call("POST", "/v1/operations/execute", map[string]any{"intent": reveal, "proof": proof}, 401)
	if e := f.db.QueryRow(`SELECT string_agg(row_to_json(a)::text,'') FROM audit_events a`).Scan(&audit); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(audit, "fixture-secret") || strings.Contains(audit, "rotated-fixture-value") {
		t.Fatal("secret entered audit")
	}
	var count int
	if e := f.db.QueryRow(`SELECT count(*) FROM vault_history WHERE secret_id=$1`, secretID).Scan(&count); e != nil || count != 2 {
		t.Fatal("rotation history missing", e)
	}
}
func TestPhaseSixIndependentRecoveryAndSignedAudit(t *testing.T) {
	f, _ := operationsFixture(t)
	root := t.TempDir()
	device := strings.Repeat("d", 64)
	codeFile := filepath.Join(t.TempDir(), "enrollment")
	if e := recovery.Provision(root, device, "Recovery fixture", codeFile, false); e != nil {
		t.Fatal(e)
	}
	code, e := os.ReadFile(codeFile)
	if e != nil {
		t.Fatal(e)
	}
	wa, e := api.NewWebAuthn("https://localhost:9443")
	if e != nil {
		t.Fatal(e)
	}
	identity, e := recovery.New(root, wa)
	if e != nil {
		t.Fatal(e)
	}
	mux := http.NewServeMux()
	identity.Routes(mux)
	svc := operations.New(context.Background(), operations.Config{}, identity.OperationsIdentity(), "recovery")
	svc.Audit = identity.OperationsAudit(operations.Config{})
	svc.Revalidate = identity.Revalidate
	mux.Handle("/v1/operations/", svc.Handler())
	server := httptest.NewTLSServer(mux)
	defer server.Close()
	rf := &fixture{t: t, server: server, client: server.Client()}
	flow := rf.call("POST", "/v1/auth/browser", map[string]any{"device_id": device, "device_name": "Recovery fixture"}, 200)
	begin := rf.call("POST", "/v1/auth/begin", map[string]any{"flow_id": flow["flow_id"], "kind": "register", "authorization": string(code)}, 200)
	key := newVirtualKey(t)
	rf.call("POST", "/v1/auth/finish", map[string]any{"ceremony_id": begin["ceremony_id"], "credential": key.credential(t, begin, true, "https://localhost:9443", true)}, 200)
	session := rf.call("POST", "/v1/auth/poll", map[string]any{"flow_id": flow["flow_id"], "poll_token": flow["poll_token"]}, 200)
	rf.token = session["access_token"].(string)
	// A normal bearer has no authority at the independent recovery listener.
	rf.callAs(rf.client, f.token, "GET", "/v1/operations/overview", nil, 401)
	rf.call("GET", "/v1/operations/overview", nil, 200)
	batch, signature, e := identity.Export(0)
	if e != nil {
		t.Fatal(e)
	}
	f.app.Operations.Config.RecoveryKeys = map[string]string{batch.Origin: identity.PublicKey()}
	input := map[string]any{"batch": batch, "signature": signature}
	f.call("POST", "/v1/recovery/import", input, 200)
	f.call("POST", "/v1/recovery/import", input, 200)
	var n int
	if e = f.db.QueryRow(`SELECT count(*) FROM recovery_audit`).Scan(&n); e != nil || n != len(batch.Events) {
		t.Fatal("audit duplicated", e)
	}
	batch.Events[0].Result = "tampered"
	f.call("POST", "/v1/recovery/import", map[string]any{"batch": batch, "signature": signature}, 403)
	// Central identity expires; recovery still authenticates without querying it.
	if _, e = f.db.Exec(`UPDATE sessions SET expires_at=now()-interval '1 second'`); e != nil {
		t.Fatal(e)
	}
	f.call("GET", "/v1/operations/overview", nil, 401)
	f.server.Close()
	rf.call("GET", "/v1/operations/overview", nil, 200)
	// A fresh recovery login uses only its own passkey and origin.
	flow = rf.call("POST", "/v1/auth/browser", map[string]any{"device_id": device, "device_name": "Recovery fixture"}, 200)
	begin = rf.call("POST", "/v1/auth/begin", map[string]any{"flow_id": flow["flow_id"], "kind": "login", "authorization": ""}, 200)
	credential := key.credential(t, begin, false, "https://localhost:9443", true)
	credential["response"].(map[string]any)["userHandle"] = b64([]byte("prisme-recovery-" + device))
	rf.call("POST", "/v1/auth/finish", map[string]any{"ceremony_id": begin["ceremony_id"], "credential": credential}, 200)
	session = rf.call("POST", "/v1/auth/poll", map[string]any{"flow_id": flow["flow_id"], "poll_token": flow["poll_token"]}, 200)
	rf.token = session["access_token"].(string)
	if e = identity.Revoke(device); e != nil {
		t.Fatal(e)
	}
	rf.call("GET", "/v1/operations/overview", nil, 401)
	restarted, e := recovery.New(root, wa)
	if e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest("GET", "https://localhost", nil)
	r.Header.Set("Authorization", "Bearer "+rf.token)
	if _, e = restarted.OperationsIdentity().Authenticate(r); e == nil {
		t.Fatal("restarted gateway retained session")
	}
}

func TestPhaseSixDeploymentGuardRetainsUncertainExecution(t *testing.T) {
	_, e, d := deploymentFixture(t)
	e.Config.CoordinationRoot = t.TempDir()
	host := e.Config.Targets[0].Server
	j := approveJob(t, e, prepareJob(t, e, "release-1", "phase6-uncertain", false))
	d.fail = "start"
	runJob(t, e, j, "uncertain")
	state, err := coordination.Read(e.Config.CoordinationRoot, host)
	if err != nil || !state.Active {
		t.Fatal("uncertain worker released the host", err)
	}
	if _, err = coordination.Acquire(e.Config.CoordinationRoot, host, "recovery", "device"); err == nil {
		t.Fatal("recovery raced unfinished deployment")
	}
	if err = coordination.Reconcile(e.Config.CoordinationRoot, host, "operator", state.Epoch); err != nil {
		t.Fatal(err)
	}
	if err = e.RunTarget(context.Background(), j.TargetID); err != deployment.ErrPrecondition {
		t.Fatal("old fenced plan resumed", err)
	}
}
func TestPhaseSixDeploymentGuardAcknowledgesSuccess(t *testing.T) {
	_, e, _ := deploymentFixture(t)
	e.Config.CoordinationRoot = t.TempDir()
	host := e.Config.Targets[0].Server
	j := approveJob(t, e, prepareJob(t, e, "release-1", "phase6-success", false))
	runJob(t, e, j, "succeeded")
	state, err := coordination.Read(e.Config.CoordinationRoot, host)
	if err != nil || state.Active {
		t.Fatal("successful execution did not release host", err)
	}
	lease, err := coordination.Acquire(e.Config.CoordinationRoot, host, "recovery", "device")
	if err != nil {
		t.Fatal(err)
	}
	if err = lease.Close(true); err != nil {
		t.Fatal(err)
	}
	planned := prepareJob(t, e, "release-2", "phase6-suspended", false)
	if !strings.Contains(strings.Join(planned.Plan.Blockers, ","), "host_reconciliation_required") {
		t.Fatal("suspended host admitted planning")
	}
}
