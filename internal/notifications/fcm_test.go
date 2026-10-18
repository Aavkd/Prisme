package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestFCMMinimalMessageAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		retry  string
		want   error
	}{
		{"accepted", 200, `{"name":"fixture"}`, "", nil},
		{"expired token", 404, `{"error":{"details":[{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"UNREGISTERED"}]}}`, "", ErrUnregistered},
		{"bad payload is not expired token", 400, `{"error":{"status":"INVALID_ARGUMENT"}}`, "", ErrDelivery},
		{"retry-after", 429, `{}`, "600", ErrDelivery},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &FCM{Project: "fixture-project", token: "short-lived-fixture", expiry: time.Now().Add(time.Hour), HTTP: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://fcm.googleapis.com/v1/projects/fixture-project/messages:send" {
					t.Fatal("unexpected provider URL")
				}
				var payload map[string]any
				if json.NewDecoder(r.Body).Decode(&payload) != nil {
					t.Fatal("invalid JSON")
				}
				message := payload["message"].(map[string]any)
				data := message["data"].(map[string]any)
				if len(data) != 7 || data["title"] != "Prisme" || message["notification"] != nil {
					t.Fatal("non-minimal or auto-displayed payload")
				}
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Retry-After": []string{tc.retry}}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}}
			err := f.Send(context.Background(), "fixture-token", Payload{EventID: strings.Repeat("e", 64), IncidentID: strings.Repeat("a", 64), Title: "Prisme", State: "attention", Severity: "critical", Route: "/incidents/" + strings.Repeat("a", 64), Urgent: true})
			if !errors.Is(err, tc.want) {
				t.Fatalf("%v != %v", err, tc.want)
			}
			if tc.retry != "" {
				var retry *RetryError
				if !errors.As(err, &retry) || retry.After != 10*time.Minute {
					t.Fatal("provider Retry-After ignored")
				}
			}
		})
	}
}
