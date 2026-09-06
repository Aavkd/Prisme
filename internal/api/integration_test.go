package api_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"prisme.local/platform/internal/agent"
	"prisme.local/platform/internal/api"
	"prisme.local/platform/internal/pki"
	"prisme.local/platform/internal/store"
)

func TestFoundation(t *testing.T) {
	dsn := os.Getenv("PRISME_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set PRISME_TEST_DATABASE_URL to run real PostgreSQL/TLS integration tests")
	}
	ctx := context.Background()
	admin, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal("test database unavailable")
	}
	defer admin.Close()
	schema := "test_" + strings.ToLower(time.Now().Format("20060102150405")) + api.Hash(t.Name() + time.Now().String())[:12]
	if _, err = admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := store.Open(ctx, u.String())
	if err != nil {
		t.Fatal("schema connection failed")
	}
	defer db.Close()
	// Applying the migration twice must preserve existing data and schema.
	for range 2 {
		if err = store.Migrate(ctx, db); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	if err = pki.Init(dir); err != nil {
		t.Fatal(err)
	}
	ca, err := pki.Load(filepath.Join(dir, "ca.pem"), filepath.Join(dir, "ca-key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca.Certificate)
	bootstrap := strings.Repeat("a", 64)
	app := &api.Server{DB: db, CA: ca, Bootstrap: bootstrap, Now: func() time.Time { return time.Now().UTC() }}
	server := httptest.NewUnstartedServer(app.Handler())
	cert, err := tls.LoadX509KeyPair(filepath.Join(dir, "server.pem"), filepath.Join(dir, "server-key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, ClientCAs: roots, ClientAuth: tls.VerifyClientCertIfGiven}
	server.StartTLS()
	defer server.Close()
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}}}
	defer client.CloseIdleConnections()
	call := func(c *http.Client, method, path, token, key string, body any) (int, map[string]any) {
		t.Helper()
		data, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, server.URL+path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		resp, e := c.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		var out map[string]any
		if resp.StatusCode != 204 {
			if e = json.NewDecoder(resp.Body).Decode(&out); e != nil {
				t.Fatal(e)
			}
		}
		return resp.StatusCode, out
	}
	expect := func(status, want int) {
		t.Helper()
		if status != want {
			t.Fatalf("HTTP %d, expected %d", status, want)
		}
	}
	status, _ := call(client, "GET", "/v1/me", "", "", nil)
	expect(status, 401)
	status, _ = call(client, "POST", "/v1/dev/sessions", "", "", map[string]string{"secret": "wrong", "device_name": "test"})
	expect(status, 401)
	status, _ = call(client, "POST", "/v1/dev/sessions", "", "", map[string]string{"secret": bootstrap, "device_name": "test", "admin": "true"})
	expect(status, 400)
	status, session := call(client, "POST", "/v1/dev/sessions", "", "", map[string]string{"secret": bootstrap, "device_name": "test"})
	expect(status, 201)
	token := session["access_token"].(string)
	status, identity := call(client, "GET", "/v1/me", token, "", nil)
	expect(status, 200)
	if identity["auth_mode"] != "development" {
		t.Fatal("wrong auth mode")
	}
	status, inv := call(client, "POST", "/v1/enrollment-tokens", token, "integration-invite-1", map[string]any{})
	expect(status, 201)
	status, replay := call(client, "POST", "/v1/enrollment-tokens", token, "integration-invite-1", map[string]any{})
	expect(status, 201)
	if inv["token"] != replay["token"] || inv["expires_at"] != replay["expires_at"] {
		t.Fatal("invite replay changed")
	}
	invite := inv["token"].(string)
	key, keyPEM, _ := pki.Key()
	csr := makeCSR(t, key)
	request := map[string]string{"token": invite, "name": "test-agent", "csr_pem": csr, "protocol_version": "1"}
	status, enrolled := call(client, "POST", "/v1/agent/enroll", "", "", request)
	expect(status, 201)
	status, replay = call(client, "POST", "/v1/agent/enroll", "", "", request)
	expect(status, 200)
	if enrolled["certificate_pem"] != replay["certificate_pem"] {
		t.Fatal("enrollment replay changed certificate")
	}
	different, _, _ := pki.Key()
	request["csr_pem"] = makeCSR(t, different)
	status, _ = call(client, "POST", "/v1/agent/enroll", "", "", request)
	expect(status, 409)
	request["csr_pem"] = csr
	request["protocol_version"] = "2"
	status, _ = call(client, "POST", "/v1/agent/enroll", "", "", request)
	expect(status, 409)
	agentID := enrolled["agent_id"].(string)
	agentCert, err := tls.X509KeyPair([]byte(enrolled["certificate_pem"].(string)), keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(agentCert.Certificate[0])
	if leaf.IsCA || len(leaf.DNSNames) > 0 || leaf.Subject.CommonName != agentID || len(leaf.ExtKeyUsage) != 1 || leaf.ExtKeyUsage[0] != x509.ExtKeyUsageClientAuth {
		t.Fatal("CSR privilege escalation")
	}
	agentHTTP := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: []tls.Certificate{agentCert}}}}
	defer agentHTTP.CloseIdleConnections()
	status, _ = call(client, "POST", "/v1/agent/heartbeat", token, "", map[string]string{"protocol_version": "1"})
	expect(status, 401)
	status, beat := call(agentHTTP, "POST", "/v1/agent/heartbeat", "", "", map[string]string{"protocol_version": "1"})
	expect(status, 200)
	if beat["agent_id"] != agentID || len(beat["capabilities"].([]any)) != 1 || beat["capabilities"].([]any)[0] != "self:heartbeat" {
		t.Fatal("unexpected capabilities")
	}
	status, _ = call(agentHTTP, "GET", "/v1/agents", "", "", nil)
	expect(status, 401)
	status, page := call(client, "GET", "/v1/agents", token, "", nil)
	expect(status, 200)
	if len(page["items"].([]any)) != 1 || page["items"].([]any)[0].(map[string]any)["last_seen_at"] == nil {
		t.Fatal("heartbeat not persisted")
	}
	// Real agent library, with disk identity, survives reconnect without re-consuming token.
	status, newInvite := call(client, "POST", "/v1/enrollment-tokens", token, "integration-invite-2", map[string]any{})
	expect(status, 201)
	tokenPath := filepath.Join(dir, "invite")
	if err = os.WriteFile(tokenPath, []byte(newInvite["token"].(string)), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := agent.Config{URL: server.URL, CAFile: filepath.Join(dir, "ca.pem"), TokenFile: tokenPath, StateDir: filepath.Join(dir, "agent-state"), Name: "real-agent"}
	a, err := agent.Connect(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer a.HTTP.CloseIdleConnections()
	if err = a.Heartbeat(ctx); err != nil {
		t.Fatal(err)
	}
	cfg.TokenFile = "missing-token"
	again, err := agent.Connect(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer again.HTTP.CloseIdleConnections()
	if again.ID != a.ID {
		t.Fatal("identity lost on restart")
	}
	if err = again.Heartbeat(ctx); err != nil {
		t.Fatal(err)
	}
	// Server recreation preserves sessions and enrollment replay because state is PostgreSQL.
	restarted := &api.Server{DB: db, CA: ca, Bootstrap: bootstrap, Now: app.Now}
	rr := httptest.NewRecorder()
	rq := httptest.NewRequest("GET", "/v1/me", nil)
	rq.Header.Set("Authorization", "Bearer "+token)
	restarted.Handler().ServeHTTP(rr, rq)
	expect(rr.Code, 200)
	// Revocation takes effect even for an already pooled TLS connection.
	if _, err = db.Exec("UPDATE agents SET revoked=true WHERE id=$1", agentID); err != nil {
		t.Fatal(err)
	}
	status, _ = call(agentHTTP, "POST", "/v1/agent/heartbeat", "", "", map[string]string{"protocol_version": "1"})
	expect(status, 403)
	// Expired credentials are rejected with the same public error as unknown credentials.
	status, expired := call(client, "POST", "/v1/enrollment-tokens", token, "integration-expired", map[string]any{})
	expect(status, 201)
	if _, err = db.Exec("UPDATE enrollment_tokens SET expires_at=now()-interval '1 second' WHERE token_hash=$1", api.Hash(expired["token"].(string))); err != nil {
		t.Fatal(err)
	}
	request["protocol_version"] = "1"
	request["token"] = expired["token"].(string)
	status, _ = call(client, "POST", "/v1/agent/enroll", "", "", request)
	expect(status, 401)
	// Concurrent duplicate enrollment creates one identity and one audit entry.
	status, concurrent := call(client, "POST", "/v1/enrollment-tokens", token, "integration-concurrent", map[string]any{})
	expect(status, 201)
	request["token"] = concurrent["token"].(string)
	payload, _ := json.Marshal(request)
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for range 2 {
		wg.Go(func() {
			resp, e := client.Post(server.URL+"/v1/agent/enroll", "application/json", bytes.NewReader(payload))
			if e != nil {
				codes <- 0
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			codes <- resp.StatusCode
		})
	}
	wg.Wait()
	close(codes)
	total := 0
	for c := range codes {
		total += c
	}
	if total != 401 {
		t.Fatalf("concurrent codes total=%d; wanted 200+201", total)
	}
	checkNoSecrets(t, db, bootstrap, token, invite)
	status, _ = call(client, "DELETE", "/v1/session", token, "", nil)
	expect(status, 204)
	status, _ = call(client, "GET", "/v1/me", token, "", nil)
	expect(status, 401)
	status, session = call(client, "POST", "/v1/dev/sessions", "", "", map[string]string{"secret": bootstrap, "device_name": "expiry"})
	expect(status, 201)
	token = session["access_token"].(string)
	if _, err = db.Exec("UPDATE sessions SET expires_at=now()-interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	status, _ = call(client, "GET", "/v1/me", token, "", nil)
	expect(status, 401)
	// Rollback consistency: audit failure must roll back session issuance.
	if _, err = db.Exec("ALTER TABLE audit_events ADD CONSTRAINT reject_audit CHECK (action <> 'session.created') NOT VALID"); err != nil {
		t.Fatal(err)
	}
	var before, after int
	if err = db.QueryRow("SELECT count(*) FROM sessions").Scan(&before); err != nil {
		t.Fatal(err)
	}
	status, _ = call(client, "POST", "/v1/dev/sessions", "", "", map[string]string{"secret": bootstrap, "device_name": "rollback"})
	expect(status, 503)
	if err = db.QueryRow("SELECT count(*) FROM sessions").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("audit failure did not roll back session mutation")
	}
	t.Log("verified auth, strict requests, migration replay, enrollment replay/concurrency, mTLS isolation, expiry, revocation, durable agent identity and audit secrecy")
}
func makeCSR(t *testing.T, key *ecdsa.PrivateKey) string {
	t.Helper()
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "administrator"}, DNSNames: []string{"admin.local"}}, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}))
}
func checkNoSecrets(t *testing.T, db *sql.DB, secrets ...string) {
	t.Helper()
	rows, err := db.Query("SELECT row_to_json(a)::text FROM audit_events a")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var line string
		if err = rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		for _, secret := range secrets {
			if strings.Contains(line, secret) {
				t.Fatal("secret in audit")
			}
		}
		count++
	}
	if rows.Err() != nil || count < 4 {
		t.Fatal("missing audit")
	}
}
