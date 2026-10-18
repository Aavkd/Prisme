package api

import (
	"bytes"
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// One administrator, with multiple authenticators and independently revocable devices.
type passkeyUser struct{ Credentials []webauthn.Credential }

func (u passkeyUser) WebAuthnID() []byte                         { return []byte("prisme-administrator") }
func (u passkeyUser) WebAuthnName() string                       { return "administrator" }
func (u passkeyUser) WebAuthnDisplayName() string                { return "Prisme administrator" }
func (u passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }

func NewWebAuthn(origin string) (*webauthn.WebAuthn, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return nil, errors.New("passkeys require an explicit HTTPS origin")
	}
	return webauthn.New(&webauthn.Config{RPID: u.Hostname(), RPOrigins: []string{origin}, RPDisplayName: "Prisme", AuthenticatorSelection: protocol.AuthenticatorSelection{ResidentKey: protocol.ResidentKeyRequirementRequired, UserVerification: protocol.VerificationRequired}})
}

func (s *Server) identityRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /v1/auth/browser", s.beginBrowser)
	m.HandleFunc("POST /v1/auth/poll", s.pollBrowser)
	m.HandleFunc("POST /v1/auth/begin", s.beginPasskey)
	m.HandleFunc("POST /v1/auth/finish", s.finishPasskey)
	m.HandleFunc("GET /v1/devices", s.auth(s.devices))
	m.HandleFunc("DELETE /v1/devices/{id}", s.auth(s.revokeDevice))
	m.HandleFunc("POST /v1/recovery-codes", s.auth(s.recoveryCodes))
	m.HandleFunc("POST /v1/registration-ticket", s.auth(s.registrationTicket))
	m.HandleFunc("POST /v1/session/rotate", s.auth(s.rotateSession))
	m.HandleFunc("GET /v1/preferences", s.auth(s.preferences))
	m.HandleFunc("PUT /v1/preferences", s.auth(s.preferences))
	m.HandleFunc("GET /auth", s.authPage)
	m.HandleFunc("GET /auth.js", s.authScript)
}

func loadUser(ctx context.Context, tx *sql.Tx) (passkeyUser, error) {
	u := passkeyUser{}
	rows, err := tx.QueryContext(ctx, "SELECT credential FROM passkeys ORDER BY id")
	if err != nil {
		return u, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var c webauthn.Credential
		if err = rows.Scan(&raw); err != nil {
			return u, err
		}
		if err = json.Unmarshal(raw, &c); err != nil {
			return u, err
		}
		u.Credentials = append(u.Credentials, c)
	}
	return u, rows.Err()
}

func (s *Server) beginBrowser(w http.ResponseWriter, r *http.Request) {
	if s.WebAuthn == nil {
		problem(w, 503, "passkeys_unconfigured")
		return
	}
	var req struct {
		DeviceID   string `json:"device_id"`
		DeviceName string `json:"device_name"`
	}
	if !body(w, r, &req) {
		return
	}
	if !opaque.MatchString(req.DeviceID) || !validName(req.DeviceName) {
		problem(w, 400, "invalid_device")
		return
	}
	id, err := randomID()
	if err != nil {
		problem(w, 500, "internal")
		return
	}
	poll, err := randomID()
	if err != nil {
		problem(w, 500, "internal")
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(r.Context(), "DELETE FROM browser_logins WHERE expires_at <= $1", s.Now())
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "INSERT INTO browser_logins(id,poll_hash,device_id,device_name,expires_at) VALUES($1,$2,$3,$4,$5)", id, Hash(poll), req.DeviceID, req.DeviceName, s.Now().Add(5*time.Minute))
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 201, map[string]any{"flow_id": id, "poll_token": poll, "browser_url": s.WebAuthn.Config.RPOrigins[0] + "/auth#" + id, "expires_at": s.Now().Add(5 * time.Minute)})
}

func (s *Server) beginPasskey(w http.ResponseWriter, r *http.Request) {
	if s.WebAuthn == nil {
		problem(w, 503, "passkeys_unconfigured")
		return
	}
	var req struct {
		FlowID        string `json:"flow_id"`
		Kind          string `json:"kind"`
		Authorization string `json:"authorization"`
	}
	if !body(w, r, &req) {
		return
	}
	if !opaque.MatchString(req.FlowID) || (req.Kind != "login" && req.Kind != "register") {
		problem(w, 400, "invalid_request")
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	var device, name string
	err = tx.QueryRowContext(r.Context(), "SELECT device_id,device_name FROM browser_logins WHERE id=$1 AND expires_at>$2 AND NOT approved FOR UPDATE", req.FlowID, s.Now()).Scan(&device, &name)
	if err != nil {
		problem(w, 401, "invalid_flow")
		return
	}
	var initialized bool
	if err = tx.QueryRowContext(r.Context(), "SELECT initialized FROM identity_state WHERE id=1 FOR UPDATE").Scan(&initialized); err != nil {
		problem(w, 503, "unavailable")
		return
	}
	kind := req.Kind
	if kind == "register" {
		if !initialized {
			if s.IdentityBootstrap == "" || subtle.ConstantTimeCompare([]byte(Hash(req.Authorization)), []byte(Hash(s.IdentityBootstrap))) != 1 {
				problem(w, 401, "invalid_authorization")
				return
			}
			kind = "bootstrap"
		} else {
			result, e := tx.ExecContext(r.Context(), "DELETE FROM recovery_codes WHERE code_hash=$1", Hash(req.Authorization))
			if e != nil {
				problem(w, 503, "unavailable")
				return
			}
			n, _ := result.RowsAffected()
			if n == 1 {
				kind = "recovery"
			} else {
				result, e = tx.ExecContext(r.Context(), "DELETE FROM auth_ceremonies WHERE id=$1 AND kind='ticket' AND expires_at>$2 AND (device_id IS NULL OR EXISTS(SELECT 1 FROM devices d WHERE d.id=auth_ceremonies.device_id AND NOT d.revoked))", Hash(req.Authorization), s.Now())
				if e != nil {
					problem(w, 503, "unavailable")
					return
				}
				n, _ = result.RowsAffected()
				if n != 1 {
					problem(w, 401, "invalid_authorization")
					return
				}
				kind = "register"
			}
		}
	}
	user, err := loadUser(r.Context(), tx)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	var options any
	var data *webauthn.SessionData
	if kind == "login" {
		options, data, err = s.WebAuthn.BeginLogin(user, webauthn.WithUserVerification(protocol.VerificationRequired))
	} else {
		options, data, err = s.WebAuthn.BeginRegistration(user, webauthn.WithExclusions(credentialDescriptors(user.Credentials)))
	}
	if err != nil {
		problem(w, 400, "ceremony_unavailable")
		return
	}
	id, err := randomID()
	if err != nil {
		problem(w, 500, "internal")
		return
	}
	raw, err := json.Marshal(data)
	if err != nil {
		problem(w, 500, "internal")
		return
	}
	_, err = tx.ExecContext(r.Context(), "DELETE FROM auth_ceremonies WHERE expires_at<=$1 OR device_id=$2 AND kind<>'ticket'", s.Now(), req.FlowID)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "INSERT INTO auth_ceremonies(id,kind,session_data,device_id,device_name,expires_at) VALUES($1,$2,$3,$4,$5,$6)", id, kind, string(raw), req.FlowID, name, s.Now().Add(2*time.Minute))
	}
	if err == nil && kind != "login" {
		err = audit(r.Context(), tx, "identity-authorization", "passkey.registration_started", device)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 200, map[string]any{"ceremony_id": id, "options": options, "device_name": name})
}
func credentialDescriptors(cs []webauthn.Credential) []protocol.CredentialDescriptor {
	out := make([]protocol.CredentialDescriptor, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Descriptor())
	}
	return out
}

func (s *Server) finishPasskey(w http.ResponseWriter, r *http.Request) {
	if s.WebAuthn == nil {
		problem(w, 503, "passkeys_unconfigured")
		return
	}
	var req struct {
		CeremonyID string          `json:"ceremony_id"`
		Credential json.RawMessage `json:"credential"`
	}
	if !body(w, r, &req) {
		return
	}
	// Consume even an invalid response; retries require a new challenge.
	var raw []byte
	var kind, flow, name string
	err := s.DB.QueryRowContext(r.Context(), "DELETE FROM auth_ceremonies WHERE id=$1 AND kind<>'ticket' AND expires_at>$2 RETURNING kind,session_data,device_id,device_name", req.CeremonyID, s.Now()).Scan(&kind, &raw, &flow, &name)
	if err != nil {
		problem(w, 401, "invalid_ceremony")
		return
	}
	var session webauthn.SessionData
	if json.Unmarshal(raw, &session) != nil {
		problem(w, 503, "unavailable")
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	var initialized bool
	if err = tx.QueryRowContext(r.Context(), "SELECT initialized FROM identity_state WHERE id=1 FOR UPDATE").Scan(&initialized); err != nil {
		problem(w, 503, "unavailable")
		return
	}
	if kind == "bootstrap" && initialized {
		problem(w, 409, "already_initialized")
		return
	}
	user, err := loadUser(r.Context(), tx)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	response, err := http.NewRequestWithContext(r.Context(), "POST", "https://localhost", bytes.NewReader(req.Credential))
	if err != nil {
		problem(w, 400, "invalid_credential")
		return
	}
	var credential *webauthn.Credential
	if kind == "login" {
		credential, err = s.WebAuthn.FinishLogin(user, session, response)
	} else {
		credential, err = s.WebAuthn.FinishRegistration(user, session, response)
	}
	if err != nil || credential.Authenticator.CloneWarning {
		problem(w, 401, "invalid_credential")
		return
	}
	encoded, err := json.Marshal(credential)
	if err != nil {
		problem(w, 500, "internal")
		return
	}
	id := base64.RawURLEncoding.EncodeToString(credential.ID)
	if kind == "login" {
		_, err = tx.ExecContext(r.Context(), "UPDATE passkeys SET credential=$1 WHERE id=$2", string(encoded), id)
	} else {
		_, err = tx.ExecContext(r.Context(), "INSERT INTO passkeys(id,credential,name) VALUES($1,$2,$3)", id, string(encoded), name)
		if err == nil {
			_, err = tx.ExecContext(r.Context(), "UPDATE identity_state SET initialized=true WHERE id=1")
		}
	}
	if err == nil && kind == "recovery" {
		_, err = tx.ExecContext(r.Context(), "DELETE FROM sessions")
		if err == nil {
			_, err = tx.ExecContext(r.Context(), "DELETE FROM passkeys WHERE id<>$1", id)
		}
		if err == nil {
			_, err = tx.ExecContext(r.Context(), "DELETE FROM recovery_codes")
		}
		if err == nil {
			_, err = tx.ExecContext(r.Context(), "DELETE FROM auth_ceremonies")
		}
		if err == nil {
			_, err = tx.ExecContext(r.Context(), "DELETE FROM browser_logins WHERE id<>$1", flow)
		}
	}
	if err == nil {
		var result sql.Result
		result, err = tx.ExecContext(r.Context(), "UPDATE browser_logins SET approved=true WHERE id=$1 AND expires_at>$2", flow, s.Now())
		if err == nil {
			n, _ := result.RowsAffected()
			if n != 1 {
				problem(w, 401, "invalid_flow")
				return
			}
		}
	}
	if err == nil {
		err = audit(r.Context(), tx, "administrator", "passkey."+kind, id)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 200, map[string]string{"status": "approved"})
}

func (s *Server) pollBrowser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FlowID    string `json:"flow_id"`
		PollToken string `json:"poll_token"`
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
	var id, name string
	var approved bool
	err = tx.QueryRowContext(r.Context(), "SELECT device_id,device_name,approved FROM browser_logins WHERE id=$1 AND poll_hash=$2 AND expires_at>$3 FOR UPDATE", req.FlowID, Hash(req.PollToken), s.Now()).Scan(&id, &name, &approved)
	if err != nil {
		problem(w, 401, "invalid_flow")
		return
	}
	if !approved {
		send(w, 202, map[string]string{"status": "pending", "access_token": "", "expires_at": "", "auth_mode": ""})
		return
	}
	_, err = tx.ExecContext(r.Context(), "INSERT INTO devices(id,name) VALUES($1,$2) ON CONFLICT(id) DO NOTHING", id, name)
	var revoked bool
	if err == nil {
		err = tx.QueryRowContext(r.Context(), "SELECT revoked FROM devices WHERE id=$1 FOR UPDATE", id).Scan(&revoked)
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	if revoked {
		problem(w, 403, "device_revoked")
		return
	}
	token, err := randomID()
	if err != nil {
		problem(w, 500, "internal")
		return
	}
	expires := s.Now().Add(time.Hour)
	_, err = tx.ExecContext(r.Context(), "INSERT INTO sessions(token_hash,device_name,device_id,auth_mode,authenticated_at,expires_at) VALUES($1,$2,$3,'passkey',$4,$5)", Hash(token), name, id, s.Now(), expires)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "DELETE FROM browser_logins WHERE id=$1", req.FlowID)
	}
	if err == nil {
		err = audit(r.Context(), tx, "administrator", "session.created", id)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 200, map[string]any{"status": "approved", "access_token": token, "expires_at": expires, "auth_mode": "passkey"})
}

// Security administration requires a recent passkey ceremony, or explicit local development.
func (s *Server) recent(w http.ResponseWriter, r *http.Request, hash string) bool {
	var mode string
	var at time.Time
	if err := s.DB.QueryRowContext(r.Context(), "SELECT auth_mode,authenticated_at FROM sessions WHERE token_hash=$1", hash).Scan(&mode, &at); err != nil {
		problem(w, 503, "unavailable")
		return false
	}
	if mode == "development" && s.Mode != "identity" {
		return true
	}
	if mode != "passkey" || s.Now().Sub(at) > 5*time.Minute {
		problem(w, 403, "reauthentication_required")
		return false
	}
	return true
}
func (s *Server) registrationTicket(w http.ResponseWriter, r *http.Request, hash string) {
	if !s.recent(w, r, hash) {
		return
	}
	token, err := randomID()
	if err != nil {
		problem(w, 500, "internal")
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(r.Context(), "DELETE FROM auth_ceremonies WHERE expires_at<=$1", s.Now())
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "INSERT INTO auth_ceremonies(id,kind,session_data,device_name,device_id,expires_at) SELECT $1,'ticket','{}','additional passkey',device_id,$2 FROM sessions WHERE token_hash=$3", Hash(token), s.Now().Add(5*time.Minute), hash)
	}
	if err == nil {
		err = audit(r.Context(), tx, "administrator", "passkey.ticket_created", "identity")
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 201, map[string]any{"ticket": token, "expires_at": s.Now().Add(5 * time.Minute)})
}
func (s *Server) recoveryCodes(w http.ResponseWriter, r *http.Request, hash string) {
	if !s.recent(w, r, hash) {
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(r.Context(), "SELECT id FROM identity_state WHERE id=1 FOR UPDATE")
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "DELETE FROM recovery_codes")
	}
	codes := []string{}
	for range 10 {
		if err != nil {
			break
		}
		var code string
		code, err = randomID()
		if err == nil {
			_, err = tx.ExecContext(r.Context(), "INSERT INTO recovery_codes(code_hash) VALUES($1)", Hash(code))
			codes = append(codes, code)
		}
	}
	if err == nil {
		err = audit(r.Context(), tx, "administrator", "recovery_codes.rotated", "identity")
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 201, map[string]any{"codes": codes})
}
func (s *Server) devices(w http.ResponseWriter, r *http.Request, hash string) {
	rows, err := s.DB.QueryContext(r.Context(), "SELECT d.id,d.name,d.revoked,d.created_at,d.id=COALESCE((SELECT device_id FROM sessions WHERE token_hash=$1),'') FROM devices d ORDER BY created_at DESC LIMIT 100", hash)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, name string
		var revoked, current bool
		var created time.Time
		if err = rows.Scan(&id, &name, &revoked, &created, &current); err != nil {
			problem(w, 503, "unavailable")
			return
		}
		items = append(items, map[string]any{"id": id, "name": name, "revoked": revoked, "created_at": created, "current": current})
	}
	if rows.Err() != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 200, map[string]any{"items": items})
}
func (s *Server) revokeDevice(w http.ResponseWriter, r *http.Request, hash string) {
	if !s.recent(w, r, hash) {
		return
	}
	id := r.PathValue("id")
	if !opaque.MatchString(id) {
		problem(w, 400, "invalid_id")
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), "UPDATE devices SET revoked=true WHERE id=$1", id)
	if err == nil {
		n, _ := result.RowsAffected()
		if n == 0 {
			problem(w, 404, "device_not_found")
			return
		}
		_, err = tx.ExecContext(r.Context(), "DELETE FROM sessions WHERE device_id=$1", id)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "DELETE FROM device_action_keys WHERE device_id=$1", id)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "DELETE FROM notification_devices WHERE device_id=$1", id)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "DELETE FROM browser_logins WHERE device_id=$1", id)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "DELETE FROM auth_ceremonies WHERE kind='ticket' AND device_id=$1", id)
	}
	if err == nil {
		err = audit(r.Context(), tx, "administrator", "device.revoked", id)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	w.WriteHeader(204)
}
func (s *Server) rotateSession(w http.ResponseWriter, r *http.Request, hash string) {
	token, err := randomID()
	if err != nil {
		problem(w, 500, "internal")
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	var expires time.Time
	var mode string
	err = tx.QueryRowContext(r.Context(), "UPDATE sessions SET token_hash=$1 WHERE token_hash=$2 AND expires_at>$3 RETURNING expires_at,auth_mode", Hash(token), hash, s.Now()).Scan(&expires, &mode)
	if err != nil {
		problem(w, 401, "unauthorized")
		return
	}
	err = audit(r.Context(), tx, "administrator", "session.rotated", "session")
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 200, map[string]any{"access_token": token, "expires_at": expires, "auth_mode": mode})
}
func (s *Server) preferences(w http.ResponseWriter, r *http.Request, hash string) {
	var device sql.NullString
	if err := s.DB.QueryRowContext(r.Context(), "SELECT device_id FROM sessions WHERE token_hash=$1", hash).Scan(&device); err != nil {
		problem(w, 503, "unavailable")
		return
	}
	if !device.Valid {
		problem(w, 409, "device_identity_required")
		return
	}
	if r.Method == "GET" {
		var raw json.RawMessage
		if err := s.DB.QueryRowContext(r.Context(), "SELECT preferences FROM devices WHERE id=$1", device.String).Scan(&raw); err != nil {
			problem(w, 503, "unavailable")
			return
		}
		send(w, 200, raw)
		return
	}
	var req struct {
		Servers []string `json:"pinned_servers"`
		Workers []string `json:"pinned_workers"`
	}
	if !body(w, r, &req) {
		return
	}
	if req.Servers == nil || req.Workers == nil || len(req.Servers) > 100 || len(req.Workers) > 100 {
		problem(w, 400, "invalid_preferences")
		return
	}
	for _, id := range append(append([]string{}, req.Servers...), req.Workers...) {
		if !opaque.MatchString(id) {
			problem(w, 400, "invalid_id")
			return
		}
	}
	raw, err := json.Marshal(req)
	if err == nil {
		_, err = s.DB.ExecContext(r.Context(), "UPDATE devices SET preferences=$1 WHERE id=$2", string(raw), device.String)
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 200, req)
}
