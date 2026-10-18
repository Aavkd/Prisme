package notifications

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestQuietHoursDSTAndUrgency(t *testing.T) {
	p := Preferences{Enabled: true, Timezone: "Europe/Paris", QuietStart: 22 * 60, QuietEnd: 7 * 60}
	for _, v := range []struct{ now, want string }{
		{"2026-09-08T21:00:00Z", "2026-09-09T05:00:00Z"},
		{"2026-03-28T23:00:00Z", "2026-03-29T05:00:00Z"},
		{"2026-10-24T23:00:00Z", "2026-10-25T06:00:00Z"},
	} {
		now, _ := time.Parse(time.RFC3339, v.now)
		if got := p.Due(now, false, 0).Format(time.RFC3339); got != v.want {
			t.Fatalf("%s != %s", got, v.want)
		}
		if !p.Due(now, true, time.Minute).Equal(now) {
			t.Fatal("urgent delayed")
		}
	}
	p.QuietStart = p.QuietEnd
	now := time.Now()
	if !p.Due(now, false, time.Minute).Equal(now.Add(time.Minute)) {
		t.Fatal("equal hours should disable quiet window")
	}
	p.Timezone = "invalid"
	if p.Validate() == nil {
		t.Fatal("invalid timezone accepted")
	}
}

type recordingSender struct {
	calls []Payload
	fail  bool
}

func (s *recordingSender) Send(_ context.Context, _ string, p Payload) error {
	if s.fail {
		return ErrDelivery
	}
	s.calls = append(s.calls, p)
	return nil
}
func TestIndependentWatchdogPersistsTransitions(t *testing.T) {
	healthy := false
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !healthy {
			w.WriteHeader(503)
		}
	}))
	defer server.Close()
	now := time.Now()
	sender := &recordingSender{}
	w := Watchdog{URL: server.URL, StateFile: filepath.Join(t.TempDir(), "state.json"), Tokens: []string{"fixture-token"}, Threshold: 3, HTTP: server.Client(), Sender: sender, Now: func() time.Time { return now }, Preferences: Preferences{Enabled: true}}
	for i := 0; i < 3; i++ {
		if err := w.Tick(context.Background()); err != nil {
			t.Fatal(err)
		}
		now = now.Add(30 * time.Second)
	}
	if len(sender.calls) != 1 || sender.calls[0].Urgent || sender.calls[0].State != "unreachable" {
		t.Fatal(sender.calls)
	}
	restarted := w
	if err := restarted.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(30 * time.Second)
	if len(sender.calls) != 1 {
		t.Fatal("duplicate after restart")
	}
	healthy = true
	for i := 0; i < 3; i++ {
		if err := restarted.Tick(context.Background()); err != nil {
			t.Fatal(err)
		}
		now = now.Add(30 * time.Second)
	}
	if len(sender.calls) != 2 || sender.calls[1].State != "reachable" {
		t.Fatal(sender.calls)
	}
}

func TestWatchdogRespectsIndependentQuietHours(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer server.Close()
	now := time.Date(2026, 9, 8, 23, 0, 0, 0, time.UTC)
	sender := &recordingSender{}
	w := Watchdog{URL: server.URL, StateFile: filepath.Join(t.TempDir(), "state.json"), Tokens: []string{"fixture"}, Threshold: 2, HTTP: server.Client(), Sender: sender, Now: func() time.Time { return now }, Preferences: Preferences{Enabled: true, Timezone: "Europe/Paris", QuietStart: 1320, QuietEnd: 420}}
	for range 3 {
		if err := w.Tick(context.Background()); err != nil {
			t.Fatal(err)
		}
		now = now.Add(30 * time.Second)
	}
	if len(sender.calls) != 0 {
		t.Fatal("watchdog interrupted quiet hours")
	}
	now = time.Date(2026, 9, 9, 6, 0, 0, 0, time.UTC)
	for range 2 {
		if err := w.Tick(context.Background()); err != nil {
			t.Fatal(err)
		}
		now = now.Add(30 * time.Second)
	}
	if len(sender.calls) != 1 {
		t.Fatal("confirmed outage not delivered after quiet hours")
	}
}
