package api_test

import (
	"net/http/httptest"
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
