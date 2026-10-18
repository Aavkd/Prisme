package operations

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestVaultEnvelopeAuthenticatesMetadata(t *testing.T) {
	master := make([]byte, 32)
	_, _ = rand.Read(master)
	v := &Vault{master: master}
	value := []byte("private-fixture-value")
	a, e := v.Encrypt(value, []byte("scope-a"))
	if e != nil {
		t.Fatal(e)
	}
	b, e := v.Encrypt(value, []byte("scope-a"))
	if e != nil || string(a.Ciphertext) == string(b.Ciphertext) || string(a.WrappedKey) == string(b.WrappedKey) {
		t.Fatal("envelopes reused key or nonce")
	}
	plain, e := v.Decrypt(a, []byte("scope-a"))
	if e != nil || string(plain) != string(value) {
		t.Fatal("roundtrip failed")
	}
	if _, e = v.Decrypt(a, []byte("scope-b")); e == nil {
		t.Fatal("metadata transplant accepted")
	}
	a.Ciphertext[len(a.Ciphertext)-1] ^= 1
	if _, e = v.Decrypt(a, []byte("scope-a")); e == nil {
		t.Fatal("tampered value accepted")
	}
}

func TestTerminalAttachmentBindsTargetAndOwner(t *testing.T) {
	s := &Service{terminals: map[string]*Terminal{"terminal": {ID: "terminal", Target: "host-a", Owner: "session", State: "connected"}}}
	p := Principal{Session: "session"}
	if _, err := s.attach(p, "terminal", "host-b"); err != ErrDenied {
		t.Fatal("attachment accepted a proof for another target")
	}
	if _, err := s.attach(Principal{Session: "other"}, "terminal", "host-a"); err != ErrDenied {
		t.Fatal("attachment accepted another session")
	}
	if _, err := s.attach(p, "terminal", "host-a"); err != nil {
		t.Fatal(err)
	}
}
func TestOperationProofBindingExpiryReplayAndDevice(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	raw, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	proofs := &Proofs{Identity: Identity{AndroidKey: func(context.Context, Principal) ([]byte, error) { return raw, nil }}}
	actor := Principal{Session: "session", Device: "device", Method: "passkey"}
	intent := Intent{Action: "secret_reveal", Target: "local", Payload: map[string]any{"id": "opaque", "revision": 1}}
	result, e := proofs.Begin(context.Background(), actor, intent, "android_key")
	if e != nil {
		t.Fatal(e)
	}
	id := result["id"].(string)
	hash := sha256.Sum256([]byte(result["challenge"].(string)))
	sig, _ := ecdsa.SignASN1(rand.Reader, key, hash[:])
	if proofs.Consume(actor, id, intent) == nil {
		t.Fatal("unsigned action authorized")
	}
	other := actor
	other.Session = "other"
	if proofs.Sign(context.Background(), other, id, base64.StdEncoding.EncodeToString(sig)) == nil {
		t.Fatal("cross-session proof")
	}
	if e = proofs.Sign(context.Background(), actor, id, base64.StdEncoding.EncodeToString(sig)); e != nil {
		t.Fatal(e)
	}
	changed := intent
	changed.Payload = map[string]any{"id": "other", "revision": 1}
	if proofs.Consume(actor, id, changed) == nil {
		t.Fatal("proof transplanted")
	}
	if e = proofs.Consume(actor, id, intent); e != nil {
		t.Fatal(e)
	}
	if proofs.Consume(actor, id, intent) == nil {
		t.Fatal("proof replayed")
	}
	result, e = proofs.Begin(context.Background(), actor, intent, "android_key")
	if e != nil {
		t.Fatal(e)
	}
	id = result["id"].(string)
	proofs.items[id].Approved = true
	proofs.items[id].Expires = time.Now().Add(-time.Second)
	if proofs.Consume(actor, id, intent) == nil {
		t.Fatal("expired proof accepted")
	}
	actor.Method = "development"
	if _, e = proofs.Begin(context.Background(), actor, intent, "android_key"); e == nil {
		t.Fatal("development session authorized")
	}
}
func TestEnvLiteralAndProtectedPaths(t *testing.T) {
	s := &Service{Vault: &Vault{}}
	if _, err := s.applyEnv(context.Background(), Principal{}, Target{}, map[string]any{"path": "/allowed/config.txt"}); err != ErrDenied {
		t.Fatal("dotenv action accepted a non-dotenv destination")
	}
	a, e := Env(map[string]string{"B": "${do_not_expand}", "A": "quote'back\\slash"})
	b, _ := Env(map[string]string{"A": "quote'back\\slash", "B": "${do_not_expand}"})
	if e != nil || string(a) != string(b) || !strings.Contains(string(a), "B='${do_not_expand}'") {
		t.Fatal("unstable or interpolated env")
	}
	for k, v := range map[string]string{"BAD=KEY": "value", "OK": "value\nINJECT=1"} {
		if _, e = Env(map[string]string{k: v}); e == nil {
			t.Fatal("dotenv injection")
		}
	}
	for _, p := range []string{"/etc/passwd", "/allowed/.ssh/id", "/allowed/.env", "/allowed/server.key", "/allowed/.git/config", "/allowed/.prisme-temp"} {
		if !protected(p, false) {
			t.Fatal("protected path allowed", p)
		}
	}
}
