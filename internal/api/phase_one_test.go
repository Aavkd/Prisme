package api_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"prisme.local/platform/internal/agent"
	"prisme.local/platform/internal/api"
	"prisme.local/platform/internal/inventory"
	"prisme.local/platform/internal/pki"
	"prisme.local/platform/internal/store"
)

type fixture struct {
	t          *testing.T
	db         *sql.DB
	app        *api.Server
	server     *httptest.Server
	client     *http.Client
	dir, token string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("PRISME_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set PRISME_TEST_DATABASE_URL")
	}
	admin, err := store.Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "phase1_" + api.Hash(t.Name() + time.Now().String())[:20]
	if _, err = admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() })
	u, _ := url.Parse(dsn)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := store.Open(context.Background(), u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	// Upgrade a populated phase-zero database, preserving its session rows.
	initial, err := os.ReadFile("../store/migrations/001_foundation.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(initial)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO sessions(token_hash,device_name,expires_at) VALUES('migration-sentinel','existing',now()+interval '1 hour')"); err != nil {
		t.Fatal(err)
	}
	if err = store.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var preserved int
	if err = db.QueryRow("SELECT count(*) FROM sessions WHERE token_hash='migration-sentinel'").Scan(&preserved); err != nil || preserved != 1 {
		t.Fatal("upgrade lost session")
	}
	if _, err = db.Exec("DELETE FROM sessions WHERE token_hash='migration-sentinel'"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = pki.Init(dir); err != nil {
		t.Fatal(err)
	}
	ca, err := pki.Load(filepath.Join(dir, "ca.pem"), filepath.Join(dir, "ca-key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	wa, err := api.NewWebAuthn("https://localhost:8443")
	if err != nil {
		t.Fatal(err)
	}
	app := &api.Server{DB: db, CA: ca, Bootstrap: strings.Repeat("a", 64), IdentityBootstrap: strings.Repeat("b", 64), WebAuthn: wa, Now: func() time.Time { return time.Now().UTC() }}
	server := httptest.NewUnstartedServer(app.Handler())
	cert, err := tls.LoadX509KeyPair(filepath.Join(dir, "server.pem"), filepath.Join(dir, "server-key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca.Certificate)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, ClientCAs: roots, ClientAuth: tls.VerifyClientCertIfGiven}
	server.StartTLS()
	t.Cleanup(server.Close)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}}, Timeout: 8 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	return &fixture{t: t, db: db, app: app, server: server, client: client, dir: dir}
}
func (f *fixture) call(method, path string, body any, want int) map[string]any {
	return f.callAs(f.client, f.token, method, path, body, want)
}
func (f *fixture) callAs(client *http.Client, token, method, path string, body any, want int) map[string]any {
	f.t.Helper()
	var data io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			f.t.Fatal(err)
		}
		data = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, f.server.URL+path, data)
	if err != nil {
		f.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Idempotency-Key", "phase1-invite-key-123456")
	resp, err := client.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer resp.Body.Close()
	var result map[string]any
	if resp.StatusCode != 204 {
		_ = json.NewDecoder(resp.Body).Decode(&result)
	}
	if resp.StatusCode != want {
		f.t.Fatalf("%s %s: got %d, want %d (code=%v)", method, path, resp.StatusCode, want, result["code"])
	}
	return result
}
func (f *fixture) dev() {
	result := f.call("POST", "/v1/dev/sessions", map[string]string{"secret": f.app.Bootstrap, "device_name": "Windows", "device_id": strings.Repeat("d", 64)}, 201)
	f.token = result["access_token"].(string)
}
func b64(v []byte) string { return base64.RawURLEncoding.EncodeToString(v) }

type virtualKey struct {
	key   *ecdsa.PrivateKey
	id    []byte
	count uint32
}

func newVirtualKey(t *testing.T) *virtualKey {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 32)
	rand.Read(id)
	return &virtualKey{key: key, id: id}
}
func (k *virtualKey) credential(t *testing.T, begin map[string]any, register bool, origin string, uv bool) map[string]any {
	pk := begin["options"].(map[string]any)["publicKey"].(map[string]any)
	kind := "webauthn.get"
	if register {
		kind = "webauthn.create"
	}
	cd, _ := json.Marshal(map[string]any{"type": kind, "challenge": pk["challenge"], "origin": origin, "crossOrigin": false})
	rp := sha256.Sum256([]byte("localhost"))
	auth := append([]byte{}, rp[:]...)
	flags := byte(1)
	if uv {
		flags |= 4
	}
	if register {
		flags |= 64
	}
	auth = append(auth, flags)
	k.count++
	auth = binary.BigEndian.AppendUint32(auth, k.count)
	response := map[string]any{"clientDataJSON": b64(cd)}
	if register {
		auth = append(auth, make([]byte, 16)...)
		auth = binary.BigEndian.AppendUint16(auth, uint16(len(k.id)))
		auth = append(auth, k.id...)
		cose, err := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: k.key.X.FillBytes(make([]byte, 32)), -3: k.key.Y.FillBytes(make([]byte, 32))})
		if err != nil {
			t.Fatal(err)
		}
		auth = append(auth, cose...)
		att, err := cbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": auth})
		if err != nil {
			t.Fatal(err)
		}
		response["attestationObject"] = b64(att)
	} else {
		cdHash := sha256.Sum256(cd)
		signed := sha256.Sum256(append(append([]byte{}, auth...), cdHash[:]...))
		sig, err := ecdsa.SignASN1(rand.Reader, k.key, signed[:])
		if err != nil {
			t.Fatal(err)
		}
		response["authenticatorData"] = b64(auth)
		response["signature"] = b64(sig)
		response["userHandle"] = b64([]byte("prisme-administrator"))
	}
	return map[string]any{"id": b64(k.id), "rawId": b64(k.id), "type": "public-key", "response": response, "clientExtensionResults": map[string]any{}}
}
func (f *fixture) begin(device, kind, authorization string) (map[string]any, map[string]any) {
	flow := f.call("POST", "/v1/auth/browser", map[string]string{"device_id": device, "device_name": "Passkey device"}, 201)
	begin := f.call("POST", "/v1/auth/begin", map[string]string{"flow_id": flow["flow_id"].(string), "kind": kind, "authorization": authorization}, 200)
	return flow, begin
}
func (f *fixture) finish(flow, begin map[string]any, key *virtualKey, register bool) {
	f.call("POST", "/v1/auth/finish", map[string]any{"ceremony_id": begin["ceremony_id"], "credential": key.credential(f.t, begin, register, "https://localhost:8443", true)}, 200)
	result := f.call("POST", "/v1/auth/poll", map[string]any{"flow_id": flow["flow_id"], "poll_token": flow["poll_token"]}, 200)
	f.token = result["access_token"].(string)
}
func TestPhaseOnePasskeysRecoveryAndDevices(t *testing.T) {
	f := newFixture(t)
	f.app.Mode = "identity"
	f.call("POST", "/v1/dev/sessions", map[string]string{"secret": f.app.Bootstrap, "device_name": "bypass"}, 404)
	key := newVirtualKey(t)
	device := strings.Repeat("c", 64)
	flow, begin := f.begin(device, "register", f.app.IdentityBootstrap)
	f.call("POST", "/v1/auth/poll", map[string]any{"flow_id": flow["flow_id"], "poll_token": flow["poll_token"]}, 202)
	f.call("POST", "/v1/auth/poll", map[string]any{"flow_id": flow["flow_id"], "poll_token": "wrong"}, 401)
	f.finish(flow, begin, key, true)
	f.call("POST", "/v1/auth/finish", map[string]any{"ceremony_id": begin["ceremony_id"], "credential": map[string]any{}}, 401)
	f.call("POST", "/v1/auth/poll", map[string]any{"flow_id": flow["flow_id"], "poll_token": flow["poll_token"]}, 401)
	if me := f.call("GET", "/v1/me", nil, 200); me["auth_mode"] != "passkey" {
		t.Fatal("not passkey authenticated")
	}
	oldToken := f.token
	rotated := f.call("POST", "/v1/session/rotate", nil, 200)
	f.token = rotated["access_token"].(string)
	f.callAs(f.client, oldToken, "GET", "/v1/me", nil, 401)
	codes := f.call("POST", "/v1/recovery-codes", nil, 201)["codes"].([]any)
	f.call("PUT", "/v1/preferences", map[string]any{"pinned_servers": []string{strings.Repeat("e", 64)}, "pinned_workers": []string{}}, 200)
	ticket := f.call("POST", "/v1/registration-ticket", nil, 201)["ticket"].(string)
	additional := newVirtualKey(t)
	flow, begin = f.begin(device, "register", ticket)
	f.finish(flow, begin, additional, true)
	// Initial bootstrap can never be reused once a passkey exists.
	another := f.call("POST", "/v1/auth/browser", map[string]string{"device_id": device, "device_name": "retry"}, 201)
	f.call("POST", "/v1/auth/begin", map[string]any{"flow_id": another["flow_id"], "kind": "register", "authorization": f.app.IdentityBootstrap}, 401)
	for _, invalid := range []string{"origin", "uv", "signature"} {
		_, begin = f.begin(device, "login", "")
		origin := "https://localhost:8443"
		if invalid == "origin" {
			origin = "https://evil.example"
		}
		credential := key.credential(t, begin, false, origin, invalid != "uv")
		if invalid == "signature" {
			credential["response"].(map[string]any)["signature"] = b64([]byte("forged"))
		}
		f.call("POST", "/v1/auth/finish", map[string]any{"ceremony_id": begin["ceremony_id"], "credential": credential}, 401)
	}
	second := strings.Repeat("f", 64)
	flow, begin = f.begin(second, "login", "")
	f.finish(flow, begin, key, false)
	prefs := f.call("GET", "/v1/preferences", nil, 200)
	if len(prefs["pinned_servers"].([]any)) != 0 {
		t.Fatal("device preferences leaked")
	}
	secondToken := f.token
	flow, begin = f.begin(device, "login", "")
	f.finish(flow, begin, key, false)
	f.call("DELETE", "/v1/devices/"+second, nil, 204)
	f.callAs(f.client, secondToken, "GET", "/v1/me", nil, 401)
	// Recovery replaces old authenticators and sessions, and consumes the code once.
	oldToken = f.token
	recovered := newVirtualKey(t)
	flow, begin = f.begin(device, "register", codes[0].(string))
	f.finish(flow, begin, recovered, true)
	f.callAs(f.client, oldToken, "GET", "/v1/me", nil, 401)
	another = f.call("POST", "/v1/auth/browser", map[string]string{"device_id": device, "device_name": "retry"}, 201)
	f.call("POST", "/v1/auth/begin", map[string]any{"flow_id": another["flow_id"], "kind": "register", "authorization": codes[0]}, 401)
	flow, begin = f.begin(device, "login", "")
	f.finish(flow, begin, recovered, false)
	if _, err := f.db.Exec("UPDATE sessions SET authenticated_at=now()-interval '6 minutes'"); err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/v1/recovery-codes", nil, 403)
	checkNoSecrets(t, f.db, f.app.IdentityBootstrap, oldToken, codes[0].(string), ticket)
}

func testSnapshot() inventory.Snapshot {
	return inventory.Snapshot{Protocol: "1", ObservedAt: time.Now().UTC(), Source: "agent", Host: inventory.Host{Hostname: "vps-fixture", OS: "Linux", Architecture: "amd64", CPUs: 4}, DockerAvailable: true, Containers: []inventory.Container{{ID: "container-a", Name: "sounding-lines-backend", State: "running", Health: "healthy", Project: "sounding-lines", Service: "backend", ConfigFiles: "/opt/projects/sounding-lines/docker-compose.prod.yml"}, {ID: "container-b", Name: "arpent-web", State: "exited", Health: "not_configured", Project: "arpent", Service: "web"}}, Networks: []inventory.Resource{}, Volumes: []inventory.Resource{}, Images: []inventory.Image{}, Warnings: []string{}}
}
func TestPhaseOneFleetLifecycle(t *testing.T) {
	f := newFixture(t)
	f.dev()
	invite := f.call("POST", "/v1/enrollment-tokens", map[string]any{"capabilities": []string{"self:heartbeat", "inventory:host", "inventory:docker"}}, 201)
	tokenPath := filepath.Join(f.dir, "enroll-token")
	os.WriteFile(tokenPath, []byte(invite["token"].(string)), 0600)
	cfg := agent.Config{URL: f.server.URL, CAFile: filepath.Join(f.dir, "ca.pem"), TokenFile: tokenPath, StateDir: filepath.Join(f.dir, "agent"), Name: "VPS fixture"}
	client, err := agent.Connect(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer client.HTTP.CloseIdleConnections()
	if err = client.Heartbeat(context.Background()); err != nil {
		t.Fatal(err)
	}
	snapshot := testSnapshot()
	if err = client.PublishInventory(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	record := f.call("GET", "/v1/servers/"+client.ID, nil, 200)
	if record["inventory"] == nil {
		t.Fatal("missing inventory")
	}
	f.call("PUT", "/v1/servers/"+client.ID+"/capabilities", map[string]any{"capabilities": []string{"self:heartbeat"}}, 200)
	if err = client.PublishInventory(context.Background(), snapshot); err == nil {
		t.Fatal("capability reduction ignored")
	}
	f.call("PUT", "/v1/servers/"+client.ID+"/capabilities", map[string]any{"capabilities": []string{"self:heartbeat", "shell:root"}}, 400)
	f.call("POST", "/v1/enrollment-tokens", map[string]any{"capabilities": []string{"self:heartbeat"}}, 409)
	// Renew only inside the renewal window, preserving the previous identity for retries.
	if _, err = f.db.Exec("UPDATE agents SET expires_at=now()+interval '2 hours' WHERE id=$1", client.ID); err != nil {
		t.Fatal(err)
	}
	oldHTTP := &http.Client{Transport: client.HTTP.Transport, Timeout: 8 * time.Second}
	defer oldHTTP.CloseIdleConnections()
	if err = client.Renew(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.callAs(oldHTTP, "", "POST", "/v1/agent/heartbeat", map[string]string{"protocol_version": "1"}, 200)
	reconnected, err := agent.Connect(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reconnected.HTTP.CloseIdleConnections()
	if err = reconnected.Heartbeat(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec("UPDATE agents SET previous_until=now()-interval '1 second' WHERE id=$1", client.ID); err != nil {
		t.Fatal(err)
	}
	f.callAs(oldHTTP, "", "POST", "/v1/agent/heartbeat", map[string]string{"protocol_version": "1"}, 403)
	f.call("DELETE", "/v1/servers/"+client.ID, nil, 204)
	if err = reconnected.Heartbeat(context.Background()); err == nil {
		t.Fatal("revoked identity accepted")
	}
	if err = reconnected.Renew(context.Background()); err == nil {
		t.Fatal("revoked renewal accepted")
	}
	snapshot.Source = "ssh-readonly"
	imported := f.call("POST", "/v1/inventory-imports", map[string]any{"name": "OVH fixture", "snapshot": snapshot}, 201)
	importedAgain := f.call("POST", "/v1/inventory-imports", map[string]any{"name": "OVH fixture", "snapshot": snapshot}, 201)
	if imported["server_id"] != importedAgain["server_id"] {
		t.Fatal("import not idempotent")
	}
	id := imported["server_id"].(string)
	if _, err = f.db.Exec("UPDATE agents SET last_seen_at=now()-interval '3 minutes' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	if record = f.call("GET", "/v1/servers/"+id, nil, 200); record["connection_state"] != "offline" {
		t.Fatal("stale connection shown online")
	}
	raw, _ := json.Marshal(snapshot)
	var unsafe map[string]any
	json.Unmarshal(raw, &unsafe)
	unsafe["env"] = map[string]string{"PASSWORD": "must-not-be-stored"}
	f.call("POST", "/v1/inventory-imports", map[string]any{"name": "bad", "snapshot": unsafe}, 400)
	events := f.call("GET", "/v1/audit", nil, 200)
	if len(events["items"].([]any)) < 5 {
		t.Fatal("missing audit")
	}
	if _, err = f.db.Exec("UPDATE audit_events SET actor='rewritten'"); err == nil {
		t.Fatal("audit was editable")
	}
	if _, err = f.db.Exec("DELETE FROM audit_events"); err == nil {
		t.Fatal("audit was deletable")
	}
	checkNoSecrets(t, f.db, f.token, invite["token"].(string), "must-not-be-stored")
}
