package api_test

import (
	"fmt"
	"net/http/httptest"
	"net/netip"
	"prisme.local/platform/internal/api"
	"strings"
	"testing"
	"time"
)

func TestRequestBoundaries(t *testing.T) {
	now := time.Now()
	s := &api.Server{Bootstrap: strings.Repeat("a", 64), Now: func() time.Time { return now }}
	handler := s.Handler()
	for _, body := range []string{`{"secret":"bad","device_name":"test"} {}`, `{"secret":"bad","device_name":"test","admin":true}`, `{"secret":"` + strings.Repeat("x", 20*1024) + `"}`} {
		req := httptest.NewRequest("POST", "/v1/dev/sessions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != 400 {
			t.Fatalf("invalid body accepted: HTTP %d", rec.Code)
		}
	}
	// Fill the shared local window without ever reaching a database operation.
	for range 117 {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/me", nil))
		if rec.Code != 401 {
			t.Fatal(rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/me", nil))
	if rec.Code != 429 || rec.Header().Get("Retry-After") != "60" {
		t.Fatal("rate limit not enforced")
	}
	now = now.Add(time.Minute)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/me", nil))
	if rec.Code != 401 {
		t.Fatal("rate window did not reset")
	}
}

func TestAdmissionSeparatesClientsOnlyBehindTheTrustedProxy(t *testing.T) {
	now := time.Now()
	s := &api.Server{Now: func() time.Time { return now }, TrustedProxy: netip.MustParsePrefix("172.22.0.0/16")}
	handler := s.Handler()
	call := func(remote, forwarded string) int {
		req := httptest.NewRequest("GET", "/v1/me", nil)
		req.RemoteAddr = remote
		if forwarded != "" {
			req.Header.Set("X-Forwarded-For", forwarded)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}
	// Through the proxy, each client has its own allowance; the last hop counts.
	for range 120 {
		if call("172.22.0.2:41000", "203.0.113.9, 198.51.100.7") != 401 {
			t.Fatal("client limited too early")
		}
	}
	if call("172.22.0.2:41000", "198.51.100.7") != 429 {
		t.Fatal("client behind the proxy not limited")
	}
	if call("172.22.0.2:41000", "198.51.100.8") != 401 {
		t.Fatal("one client exhausted every client behind the proxy")
	}
	// A malformed header falls back to the proxy's own address.
	for range 120 {
		call("172.22.0.3:41000", "not-an-address")
	}
	if call("172.22.0.3:41000", "") != 429 {
		t.Fatal("malformed forwarded address escaped the limit")
	}
	// Outside the proxy network, forwarded headers never create new identities.
	for i := range 120 {
		if call("10.0.0.8:41000", fmt.Sprintf("198.51.100.%d", i)) != 401 {
			t.Fatal("direct caller limited too early")
		}
	}
	if call("10.0.0.8:41000", "192.0.2.1") != 429 {
		t.Fatal("direct caller chose a new identity with X-Forwarded-For")
	}
	// Without a configured proxy, the header is ignored even from that network.
	plain := (&api.Server{Now: func() time.Time { return now }}).Handler()
	for i := range 121 {
		req := httptest.NewRequest("GET", "/v1/me", nil)
		req.RemoteAddr = "172.22.0.2:41000"
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i))
		rec := httptest.NewRecorder()
		plain.ServeHTTP(rec, req)
		if i == 120 && rec.Code != 429 {
			t.Fatal("forwarded header trusted without configuration")
		}
	}
}
