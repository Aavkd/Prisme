package api

import (
	_ "embed"
	"net/http"
)

//go:embed auth.html
var authHTML string

//go:embed auth.js
var authJS string

// RegisterAuthAssets shares only static browser presentation with recovery.
// The recovery service owns its routes, credentials and independent authority.
func RegisterAuthAssets(m *http.ServeMux) {
	m.HandleFunc("GET /auth", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(authHTML))
	})
	m.HandleFunc("GET /auth.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte(authJS))
	})
}

func (s *Server) authPage(w http.ResponseWriter, r *http.Request) {
	if s.WebAuthn == nil {
		problem(w, 404, "passkeys_unconfigured")
		return
	}
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(authHTML))
}
func (s *Server) authScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = w.Write([]byte(authJS))
}
