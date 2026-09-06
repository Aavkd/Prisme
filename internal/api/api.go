// Package api exposes only the local foundation API; production auth is not enabled.
package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"prisme.local/platform/internal/pki"
)

type Server struct {
	DB        *sql.DB
	CA        *pki.Authority
	Bootstrap string
	Now       func() time.Time
	mu        sync.Mutex
	window    time.Time
	requests  int
}

func Hash(v string) string { sum := sha256.Sum256([]byte(v)); return hex.EncodeToString(sum[:]) }
func randomID() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}

var opaque = regexp.MustCompile(`^[a-f0-9]{64}$`)
var keyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { send(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.DB.PingContext(r.Context()); err != nil {
			problem(w, 503, "unavailable")
			return
		}
		send(w, 200, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("POST /v1/dev/sessions", s.login)
	mux.HandleFunc("GET /v1/me", s.auth(s.me))
	mux.HandleFunc("DELETE /v1/session", s.auth(s.logout))
	mux.HandleFunc("GET /v1/agents", s.auth(s.agents))
	mux.HandleFunc("POST /v1/enrollment-tokens", s.auth(s.invite))
	mux.HandleFunc("POST /v1/agent/enroll", s.enroll)
	mux.HandleFunc("POST /v1/agent/heartbeat", s.heartbeat)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Global bounded limiter for a single-user local foundation. Never trust proxy headers.
		if r.URL.Path != "/healthz" && r.URL.Path != "/readyz" {
			s.mu.Lock()
			now := s.Now()
			if now.Sub(s.window) >= time.Minute {
				s.window = now
				s.requests = 0
			}
			s.requests++
			allowed := s.requests <= 120
			s.mu.Unlock()
			if !allowed {
				w.Header().Set("Retry-After", "60")
				problem(w, 429, "rate_limited")
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}

func send(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, status int, code string) {
	send(w, status, map[string]any{"code": code, "message": http.StatusText(status), "details": map[string]string{}, "recommended_action": "Check your connection, credentials and request; retry only when appropriate."})
}
func body(w http.ResponseWriter, r *http.Request, v any) bool {
	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		problem(w, 415, "content_type")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		problem(w, 400, "invalid_request")
		return false
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		problem(w, 400, "invalid_request")
		return false
	}
	return true
}
func validName(s string) bool {
	return len(s) > 0 && len(s) <= 80 && !strings.ContainsAny(s, "\r\n\x00") && strings.TrimSpace(s) == s
}
func bearer(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if !strings.HasPrefix(v, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(v, "Bearer ")
}

type authenticated func(http.ResponseWriter, *http.Request, string)

func (s *Server) auth(next authenticated) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearer(r)
		if !opaque.MatchString(token) {
			problem(w, 401, "unauthorized")
			return
		}
		hash := Hash(token)
		var active bool
		err := s.DB.QueryRowContext(r.Context(), "SELECT EXISTS(SELECT 1 FROM sessions WHERE token_hash=$1 AND expires_at>$2)", hash, s.Now()).Scan(&active)
		if err != nil {
			problem(w, 503, "unavailable")
			return
		}
		if !active {
			problem(w, 401, "unauthorized")
			return
		}
		next(w, r, hash)
	}
}

func audit(ctx context.Context, tx *sql.Tx, actor, action, target string) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO audit_events(actor,action,target) VALUES($1,$2,$3)", actor, action, target)
	return err
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Secret     string `json:"secret"`
		DeviceName string `json:"device_name"`
	}
	if !body(w, r, &req) {
		return
	}
	actual, expected := Hash(req.Secret), Hash(s.Bootstrap)
	if subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) != 1 {
		problem(w, 401, "unauthorized")
		return
	}
	if !validName(req.DeviceName) {
		problem(w, 400, "invalid_device_name")
		return
	}
	token, err := randomID()
	if err != nil {
		problem(w, 500, "internal")
		return
	}
	expires := s.Now().Add(time.Hour)
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	// Expired ephemeral credentials are bounded without touching the audit.
	if _, err = tx.ExecContext(r.Context(), "DELETE FROM sessions WHERE expires_at <= $1", s.Now()); err == nil {
		_, err = tx.ExecContext(r.Context(), "INSERT INTO sessions(token_hash,device_name,expires_at) VALUES($1,$2,$3)", Hash(token), req.DeviceName, expires)
	}
	if err == nil {
		err = audit(r.Context(), tx, "development-user", "session.created", "device")
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 201, map[string]any{"access_token": token, "expires_at": expires, "auth_mode": "development"})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request, hash string) {
	var name string
	var expires time.Time
	if err := s.DB.QueryRowContext(r.Context(), "SELECT device_name,expires_at FROM sessions WHERE token_hash=$1", hash).Scan(&name, &expires); err != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 200, map[string]any{"user_id": "development-user", "device_name": name, "expires_at": expires, "auth_mode": "development"})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request, hash string) {
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(r.Context(), "DELETE FROM sessions WHERE token_hash=$1", hash)
	if err == nil {
		err = audit(r.Context(), tx, "development-user", "session.revoked", "session")
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

func (s *Server) invite(w http.ResponseWriter, r *http.Request, sessionHash string) {
	key := r.Header.Get("Idempotency-Key")
	if !keyPattern.MatchString(key) {
		problem(w, 400, "invalid_idempotency_key")
		return
	}
	var req struct{}
	if !body(w, r, &req) {
		return
	}
	mac := hmac.New(sha256.New, []byte(s.Bootstrap))
	mac.Write([]byte("invite:" + sessionHash + ":" + key))
	token := hex.EncodeToString(mac.Sum(nil))
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), "INSERT INTO enrollment_tokens(token_hash,expires_at) VALUES($1,$2) ON CONFLICT DO NOTHING", Hash(token), s.Now().Add(10*time.Minute))
	var expires time.Time
	if err == nil {
		err = tx.QueryRowContext(r.Context(), "SELECT expires_at FROM enrollment_tokens WHERE token_hash=$1", Hash(token)).Scan(&expires)
	}
	if err == nil {
		n, _ := result.RowsAffected()
		if n == 1 {
			err = audit(r.Context(), tx, "development-user", "enrollment.issued", "test-agent")
		}
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 201, map[string]any{"token": token, "expires_at": expires, "capabilities": []string{"self:heartbeat"}})
}

func (s *Server) enroll(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Name     string `json:"name"`
		CSR      string `json:"csr_pem"`
		Protocol string `json:"protocol_version"`
	}
	if !body(w, r, &req) {
		return
	}
	if req.Protocol != "1" {
		problem(w, 409, "unsupported_protocol")
		return
	}
	if !validName(req.Name) || !opaque.MatchString(req.Token) {
		problem(w, 400, "invalid_request")
		return
	}
	csr, err := pki.ParseCSR(req.CSR)
	if err != nil {
		problem(w, 400, "invalid_csr")
		return
	}
	fingerprint := Hash(string(csr.Raw))
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	var expires time.Time
	var existing sql.NullString
	err = tx.QueryRowContext(r.Context(), "SELECT expires_at,agent_id FROM enrollment_tokens WHERE token_hash=$1 FOR UPDATE", Hash(req.Token)).Scan(&expires, &existing)
	if errors.Is(err, sql.ErrNoRows) {
		problem(w, 401, "invalid_enrollment")
		return
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	if existing.Valid {
		var name, hash, cert string
		var end time.Time
		var revoked bool
		err = tx.QueryRowContext(r.Context(), "SELECT name,csr_hash,certificate_pem,expires_at,revoked FROM agents WHERE id=$1", existing.String).Scan(&name, &hash, &cert, &end, &revoked)
		if err != nil {
			problem(w, 503, "unavailable")
			return
		}
		if hash != fingerprint || name != req.Name || revoked || !s.Now().Before(end) {
			problem(w, 409, "enrollment_consumed")
			return
		}
		send(w, 200, enrollment(existing.String, cert, end))
		return
	}
	if !s.Now().Before(expires) {
		problem(w, 401, "invalid_enrollment")
		return
	}
	id, err := randomID()
	if err != nil {
		problem(w, 500, "internal")
		return
	}
	cert, serial, end, err := s.CA.Issue(id, csr, s.Now())
	if err != nil {
		problem(w, 500, "internal")
		return
	}
	_, err = tx.ExecContext(r.Context(), "INSERT INTO agents(id,name,csr_hash,certificate_pem,serial,enrolled_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)", id, req.Name, fingerprint, cert, serial, s.Now(), end)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "UPDATE enrollment_tokens SET agent_id=$1 WHERE token_hash=$2", id, Hash(req.Token))
	}
	if err == nil {
		err = audit(r.Context(), tx, "enrollment-token", "agent.enrolled", id)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 201, enrollment(id, cert, end))
}
func enrollment(id, cert string, end time.Time) map[string]any {
	return map[string]any{"agent_id": id, "certificate_pem": cert, "expires_at": end, "protocol_version": "1", "capabilities": []string{"self:heartbeat"}}
}

func (s *Server) heartbeat(w http.ResponseWriter, r *http.Request) {
	if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) == 0 {
		problem(w, 401, "agent_certificate_required")
		return
	}
	cert := r.TLS.PeerCertificates[0]
	var req struct {
		Protocol string `json:"protocol_version"`
	}
	if !body(w, r, &req) {
		return
	}
	if req.Protocol != "1" {
		problem(w, 409, "unsupported_protocol")
		return
	}
	now := s.Now()
	result, err := s.DB.ExecContext(r.Context(), "UPDATE agents SET last_seen_at=$1 WHERE id=$2 AND serial=$3 AND revoked=false AND expires_at>$1", now, cert.Subject.CommonName, cert.SerialNumber.String())
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		problem(w, 403, "agent_not_authorized")
		return
	}
	send(w, 200, map[string]any{"agent_id": cert.Subject.CommonName, "observed_at": now, "capabilities": []string{"self:heartbeat"}})
}
func (s *Server) agents(w http.ResponseWriter, r *http.Request, _ string) {
	cursor := r.URL.Query().Get("cursor")
	if cursor != "" && !opaque.MatchString(cursor) {
		problem(w, 400, "invalid_cursor")
		return
	}
	rows, err := s.DB.QueryContext(r.Context(), "SELECT id,name,enrolled_at,last_seen_at,expires_at,revoked FROM agents WHERE id>$1 ORDER BY id LIMIT 51", cursor)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	next := ""
	for rows.Next() {
		var id, name string
		var enrolled, expires time.Time
		var seen sql.NullTime
		var revoked bool
		if err = rows.Scan(&id, &name, &enrolled, &seen, &expires, &revoked); err != nil {
			problem(w, 503, "unavailable")
			return
		}
		if len(items) == 50 {
			next = items[49]["id"].(string)
			break
		}
		var last any
		if seen.Valid {
			last = seen.Time
		}
		items = append(items, map[string]any{"id": id, "name": name, "enrolled_at": enrolled, "last_seen_at": last, "expires_at": expires, "revoked": revoked, "capabilities": []string{"self:heartbeat"}})
	}
	if rows.Err() != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 200, map[string]any{"items": items, "next_cursor": next, "observed_at": s.Now()})
}
