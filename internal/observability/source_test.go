package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSourcesBoundedAndRedacted(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	scope := Scope{ID: "pilot", Server: "local", Project: "pilot", Environment: "development", Service: "web"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/loki/api/v1/query_range" {
			if !strings.Contains(r.URL.Query().Get("query"), `|= "\"} |= \"injection"`) {
				t.Error("search not quoted")
			}
			json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "streams", "result": []any{map[string]any{"stream": map[string]string{"service": "web", "secret": "must-not-leak"}, "values": [][]string{{fmt.Sprint(now.UnixNano()), `password=do-not-print Authorization: Bearer abc123 token="other-secret"`}}}}}})
		} else {
			if !strings.HasSuffix(r.URL.Query().Get("query"), "[5m]") {
				t.Error("lost sample timestamp selector")
			}
			fmt.Fprintf(w, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[[%d,"0"]]}]}}`, now.Unix())
		}
	}))
	defer server.Close()
	source, _ := NewSource(server.URL, server.URL, true)
	p, err := source.Latest(context.Background(), scope, "probe_success", now)
	if err != nil || p == nil || !p.At.Equal(now) || *p.Value != 0 {
		t.Fatal("lost sample")
	}
	logs, more, err := source.Logs(context.Background(), scope, now.Add(-time.Minute), now, `"} |= "injection`, "")
	if err != nil || more || len(logs) != 1 {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(logs)
	for _, secret := range []string{"do-not-print", "abc123", "other-secret", "must-not-leak"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("secret escaped redaction")
		}
	}
}
func TestStorageFailureAndMissingData(t *testing.T) {
	now := time.Now()
	scope := Scope{Server: "a", Project: "b", Environment: "c", Service: "d"}
	for _, body := range []string{`{"status":"success","data":{"resultType":"matrix","result":[]}}`, `{"status":"success","data":{"resultType":"matrix","result":[{"values":[[1,"NaN"]]}]}}`, `{"status":"success","data":{"resultType":"matrix","result":[{},{}]}}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		source, _ := NewSource(server.URL, server.URL, true)
		p, err := source.Latest(context.Background(), scope, "up", now)
		server.Close()
		if err != nil || p != nil && p.Value != nil {
			t.Fatal("missing sample became zero")
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "private backend error", 503) }))
	defer server.Close()
	source, _ := NewSource(server.URL, server.URL, true)
	_, err := source.Latest(context.Background(), scope, "up", now)
	if err != ErrUnavailable {
		t.Fatal("backend failure not sanitized")
	}
	if _, err = NewSource(server.URL, server.URL, false); err == nil {
		t.Fatal("cleartext allowed outside development")
	}
}
func TestRedaction(t *testing.T) {
	for _, v := range []string{`{"api_key":"sensitive"}`, `cookie=sensitive`, "Bearer sensitive", `https://user:sensitive@example.org`, "-----BEGIN PRIVATE KEY-----\nsensitive\n-----END PRIVATE KEY-----"} {
		if strings.Contains(Redact(v), "sensitive") {
			t.Fatalf("redaction failed for category")
		}
	}
}
