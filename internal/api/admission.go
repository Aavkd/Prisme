package api

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Admission is bounded even when callers rotate addresses or invalid tokens.
// Only database-verified sessions and mTLS-verified certificates get a separate
// allowance. Forwarded headers are deliberately not an identity boundary.
type admission struct {
	mu                     sync.Mutex
	buckets                map[string]int
	window                 time.Time
	total, active, streams int
}

func (s *Server) admit(w http.ResponseWriter, r *http.Request) (func(), bool) {
	if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
		return func() {}, true
	}
	a := &s.admission
	stream := strings.HasPrefix(r.URL.Path, "/v1/operations/sessions/") && strings.HasSuffix(r.URL.Path, "/stream")
	a.mu.Lock()
	now := s.Now()
	if a.buckets == nil || now.Sub(a.window) >= time.Minute || now.Before(a.window) {
		a.window, a.total, a.buckets = now, 0, make(map[string]int)
	}
	if a.total >= 6000 || (!stream && a.active >= 32) || (stream && a.streams >= 8) {
		a.mu.Unlock()
		w.Header().Set("Retry-After", "60")
		problem(w, 429, "rate_limited")
		return nil, false
	}
	a.total++
	if stream {
		a.streams++
	} else {
		a.active++
	}
	a.mu.Unlock()
	release := func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if stream {
			a.streams--
		} else {
			a.active--
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	key, limit := "ip:"+host, 120
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	if s.DB != nil && opaque.MatchString(bearer(r)) {
		var active bool
		hash := Hash(bearer(r))
		err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sessions s LEFT JOIN devices d ON d.id=s.device_id WHERE token_hash=$1 AND expires_at>$2 AND (s.device_id IS NULL OR NOT d.revoked) AND ($3<>'identity' OR s.auth_mode='passkey'))`, hash, now, s.Mode).Scan(&active)
		if err != nil {
			release()
			problem(w, 503, "unavailable")
			return nil, false
		}
		if active {
			key, limit = "session:"+hash, 1200
		}
	} else if s.DB != nil && r.TLS != nil && len(r.TLS.VerifiedChains) > 0 && len(r.TLS.PeerCertificates) > 0 {
		cert := r.TLS.PeerCertificates[0]
		var active bool
		// Agent handlers still check the registered serial and capability on every call.
		err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agents WHERE id=$1 AND NOT revoked AND expires_at>$2)`, cert.Subject.CommonName, now).Scan(&active)
		if err != nil {
			release()
			problem(w, 503, "unavailable")
			return nil, false
		}
		if active {
			key, limit = "agent:"+cert.Subject.CommonName, 180
		}
	}
	a.mu.Lock()
	_, exists := a.buckets[key]
	allowed := (exists || len(a.buckets) < 1024) && a.buckets[key] < limit
	if allowed {
		a.buckets[key]++
	}
	a.mu.Unlock()
	if !allowed {
		release()
		w.Header().Set("Retry-After", "60")
		problem(w, 429, "rate_limited")
		return nil, false
	}
	return release, true
}
