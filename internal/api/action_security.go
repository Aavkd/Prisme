package api

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"

	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"prisme.local/platform/internal/deployment"
)

func (s *Server) actionKey(w http.ResponseWriter, r *http.Request, session string) {
	var req struct {
		PublicKey string `json:"public_key"`
	}
	if !body(w, r, &req) {
		return
	}
	raw, err := base64.StdEncoding.DecodeString(req.PublicKey)
	if err != nil {
		problem(w, 400, "invalid_key")
		return
	}
	key, err := x509.ParsePKIXPublicKey(raw)
	if err != nil {
		problem(w, 400, "invalid_key")
		return
	}
	ec, ok := key.(*ecdsa.PublicKey)
	if !ok || ec.Curve != elliptic.P256() {
		problem(w, 400, "invalid_key")
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	var device string
	err = tx.QueryRowContext(r.Context(), `SELECT s.device_id FROM sessions s JOIN devices d ON d.id=s.device_id WHERE s.token_hash=$1 AND s.auth_mode='passkey' AND s.authenticated_at>$2 AND s.expires_at>$3 AND NOT d.revoked FOR UPDATE OF d,s`, session, s.Now().Add(-5*time.Minute), s.Now()).Scan(&device)
	if err != nil {
		problem(w, 403, "fresh_passkey_required")
		return
	}
	// Re-enrollment must use a freshly verified passkey, and invalidates old proofs.
	_, err = tx.ExecContext(r.Context(), `INSERT INTO device_action_keys(device_id,public_key) VALUES($1,$2) ON CONFLICT(device_id) DO UPDATE SET public_key=EXCLUDED.public_key,created_at=now()`, device, raw)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `DELETE FROM action_challenges WHERE device_id=$1`, device)
	}
	if err == nil {
		err = audit(r.Context(), tx, "administrator", "action_key.enrolled", device)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 201, map[string]string{"status": "registered"})
}

func (s *Server) actionChallenge(w http.ResponseWriter, r *http.Request, session string) {
	var req struct {
		JobID    string `json:"job_id"`
		Revision int    `json:"revision"`
		Method   string `json:"method"`
	}
	if !body(w, r, &req) {
		return
	}
	if s.Deployments == nil {
		problem(w, 503, "deployments_disabled")
		return
	}
	if req.Method != "android_key" && req.Method != "passkey" {
		problem(w, 400, "invalid_method")
		return
	}
	j, err := s.Deployments.Job(r.Context(), req.JobID)
	if err != nil {
		deploymentError(w, err)
		return
	}
	if j.Revision != req.Revision || j.State != "awaiting_approval" || !j.ExpiresAt.After(s.Now()) {
		problem(w, 409, "revision_conflict")
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	var device string
	if err = tx.QueryRowContext(r.Context(), `SELECT s.device_id FROM sessions s JOIN devices d ON d.id=s.device_id WHERE s.token_hash=$1 AND s.auth_mode='passkey' AND s.expires_at>$2 AND NOT d.revoked FOR UPDATE OF d,s`, session, s.Now()).Scan(&device); err != nil {
		problem(w, 403, "passkey_session_required")
		return
	}
	if req.Method == "android_key" {
		var exists bool
		if err = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM device_action_keys WHERE device_id=$1)`, device).Scan(&exists); err != nil || !exists {
			problem(w, 403, "action_key_required")
			return
		}
	}
	id, err := randomID()
	if err != nil {
		problem(w, 500, "internal")
		return
	}
	challenge := fmt.Sprintf("prisme-action-v1\n%s\n%s\n%s\n%d\napprove\n%s", id, device, j.ID, j.Revision, deployment.Hash(j.Plan))
	var data any
	if req.Method == "passkey" {
		if s.WebAuthn == nil {
			problem(w, 503, "passkeys_unconfigured")
			return
		}
		user, err := loadUser(r.Context(), tx)
		if err != nil {
			problem(w, 503, "unavailable")
			return
		}
		options, sessionData, err := s.WebAuthn.BeginLogin(user)
		if err != nil {
			problem(w, 403, "passkey_required")
			return
		}
		// The random WebAuthn challenge is stored with this immutable action binding.
		b, _ := json.Marshal(map[string]any{"options": options, "session": sessionData})
		data = string(b)
	}
	_, err = tx.ExecContext(r.Context(), `DELETE FROM action_challenges WHERE expires_at<=$1 OR session_hash=$2`, s.Now(), session)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO action_challenges(id,session_hash,device_id,job_id,revision,method,challenge,session_data,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, session, device, j.ID, j.Revision, req.Method, challenge, data, s.Now().Add(2*time.Minute))
	}
	if err == nil {
		err = audit(r.Context(), tx, "administrator", "action.challenge", j.ID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	browser := ""
	if req.Method == "passkey" {
		browser = s.WebAuthn.Config.RPOrigins[0] + "/action#" + id
	}
	send(w, 201, map[string]any{"id": id, "challenge": challenge, "browser_url": browser, "expires_at": s.Now().Add(2 * time.Minute)})
}
func (s *Server) actionStatus(w http.ResponseWriter, r *http.Request, session string) {
	var approved bool
	err := s.DB.QueryRowContext(r.Context(), `SELECT approved FROM action_challenges WHERE id=$1 AND session_hash=$2 AND expires_at>$3 AND NOT consumed`, r.PathValue("challenge"), session, s.Now()).Scan(&approved)
	if err != nil {
		problem(w, 404, "challenge_unavailable")
		return
	}
	send(w, 200, map[string]bool{"approved": approved})
}
func (s *Server) actionSign(w http.ResponseWriter, r *http.Request, session string) {
	var req struct {
		Signature string `json:"signature"`
	}
	if !body(w, r, &req) {
		return
	}
	sig, err := base64.StdEncoding.DecodeString(req.Signature)
	if err != nil || len(sig) > 128 {
		problem(w, 400, "invalid_signature")
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	var raw []byte
	var challenge string
	err = tx.QueryRowContext(r.Context(), `SELECT k.public_key,c.challenge FROM action_challenges c JOIN device_action_keys k ON k.device_id=c.device_id WHERE c.id=$1 AND c.session_hash=$2 AND c.method='android_key' AND c.expires_at>$3 AND NOT c.approved AND NOT c.consumed FOR UPDATE OF c,k`, r.PathValue("challenge"), session, s.Now()).Scan(&raw, &challenge)
	if err != nil {
		problem(w, 403, "challenge_unavailable")
		return
	}
	key, err := x509.ParsePKIXPublicKey(raw)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	ec, ok := key.(*ecdsa.PublicKey)
	digest := sha256.Sum256([]byte(challenge))
	if !ok || !ecdsa.VerifyASN1(ec, digest[:], sig) {
		problem(w, 403, "invalid_signature")
		return
	}
	_, err = tx.ExecContext(r.Context(), `UPDATE action_challenges SET approved=true WHERE id=$1`, r.PathValue("challenge"))
	if err == nil {
		err = audit(r.Context(), tx, "administrator", "action.android_key_verified", r.PathValue("challenge"))
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 200, map[string]bool{"approved": true})
}
func (s *Server) actionOptions(w http.ResponseWriter, r *http.Request) {
	var raw []byte
	err := s.DB.QueryRowContext(r.Context(), `SELECT session_data FROM action_challenges WHERE id=$1 AND method='passkey' AND expires_at>$2 AND NOT approved AND NOT consumed`, r.PathValue("challenge"), s.Now()).Scan(&raw)
	if err != nil {
		problem(w, 404, "challenge_unavailable")
		return
	}
	var data struct {
		Options json.RawMessage `json:"options"`
	}
	if json.Unmarshal(raw, &data) != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 200, map[string]any{"options": data.Options})
}
func (s *Server) actionPasskey(w http.ResponseWriter, r *http.Request) {
	if s.WebAuthn == nil {
		problem(w, 503, "passkeys_unconfigured")
		return
	}
	var req struct {
		Credential json.RawMessage `json:"credential"`
	}
	if !body(w, r, &req) {
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	var raw []byte
	err = tx.QueryRowContext(r.Context(), `SELECT c.session_data FROM action_challenges c JOIN sessions s ON s.token_hash=c.session_hash JOIN devices d ON d.id=c.device_id WHERE c.id=$1 AND c.method='passkey' AND c.expires_at>$2 AND s.expires_at>$2 AND s.auth_mode='passkey' AND NOT d.revoked AND NOT c.approved AND NOT c.consumed FOR UPDATE OF c`, r.PathValue("challenge"), s.Now()).Scan(&raw)
	if err != nil {
		problem(w, 403, "challenge_unavailable")
		return
	}
	var data struct {
		Session webauthn.SessionData `json:"session"`
	}
	if json.Unmarshal(raw, &data) != nil {
		problem(w, 503, "unavailable")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `SELECT id FROM identity_state WHERE id=1 FOR UPDATE`); err != nil {
		problem(w, 503, "unavailable")
		return
	}
	user, err := loadUser(r.Context(), tx)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	response, _ := http.NewRequestWithContext(r.Context(), "POST", "https://localhost", bytes.NewReader(req.Credential))
	credential, err := s.WebAuthn.FinishLogin(user, data.Session, response)
	if err != nil || credential.Authenticator.CloneWarning {
		_, _ = tx.ExecContext(r.Context(), `DELETE FROM action_challenges WHERE id=$1`, r.PathValue("challenge"))
		_ = tx.Commit()
		problem(w, 403, "invalid_credential")
		return
	}
	encoded, _ := json.Marshal(credential)
	_, err = tx.ExecContext(r.Context(), `UPDATE passkeys SET credential=$1 WHERE id=$2`, string(encoded), base64.RawURLEncoding.EncodeToString(credential.ID))
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE action_challenges SET approved=true WHERE id=$1`, r.PathValue("challenge"))
	}
	if err == nil {
		err = audit(r.Context(), tx, "administrator", "action.passkey_verified", r.PathValue("challenge"))
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 200, map[string]bool{"approved": true})
}

//go:embed action.html
var actionHTML []byte

//go:embed action.js
var actionJS []byte

func (s *Server) actionPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	_, _ = w.Write(actionHTML)
}
func (s *Server) actionScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	_, _ = w.Write(actionJS)
}
