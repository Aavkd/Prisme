package notifications

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type watchDelivery struct {
	token   string
	payload Payload
}

type recipientSender struct {
	beforeSend func(Payload)
	failing    map[string]bool
	attempts   []watchDelivery
	delivered  []watchDelivery
}

func (s *recipientSender) Send(_ context.Context, token string, p Payload) error {
	if s.beforeSend != nil {
		s.beforeSend(p)
	}
	v := watchDelivery{token, p}
	s.attempts = append(s.attempts, v)
	if s.failing[token] {
		return ErrDelivery
	}
	s.delivered = append(s.delivered, v)
	return nil
}

func (s *recipientSender) forToken(token string) []Payload {
	var out []Payload
	for _, d := range s.delivered {
		if d.token == token {
			out = append(out, d.payload)
		}
	}
	return out
}

func watchFixture(t *testing.T, sender Sender) (*Watchdog, *bool, func(bool)) {
	t.Helper()
	healthy := false
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !healthy {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(server.Close)
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	w := &Watchdog{
		URL: server.URL, StateFile: filepath.Join(t.TempDir(), "state.json"),
		Tokens: []string{"first", "second"}, Threshold: 2,
		HTTP: server.Client(), Sender: sender, Now: func() time.Time { return now },
		Preferences: Preferences{Enabled: true},
	}
	tick := func(wantError bool) {
		t.Helper()
		err := w.Tick(context.Background())
		if (err != nil) != wantError {
			t.Fatalf("Tick error = %v, want error %v", err, wantError)
		}
		now = now.Add(30 * time.Second)
	}
	return w, &healthy, tick
}

func TestWatchdogPartialOutageDoesNotLoseRecovery(t *testing.T) {
	for _, broken := range []string{"first", "second"} {
		t.Run(broken, func(t *testing.T) {
			s := &recipientSender{failing: map[string]bool{broken: true}}
			_, healthy, tick := watchFixture(t, s)
			tick(false)
			tick(true)
			if len(s.delivered) != 1 || s.delivered[0].payload.State != "unreachable" {
				t.Fatalf("broken recipient blocked outage: %+v", s.delivered)
			}
			outage := s.delivered[0].payload
			// Each tick reloads state from disk, including partially delivered transitions.
			*healthy = true
			tick(false)
			tick(true)
			if len(s.delivered) != 2 || s.delivered[1].payload.State != "reachable" || s.delivered[1].payload.IncidentID != outage.IncidentID {
				t.Fatalf("recovery lost after partial outage: %+v", s.delivered)
			}
			delete(s.failing, broken)
			tick(false)
			late := s.forToken(broken)
			if len(late) != 1 || late[0].State != "reachable" || late[0].EventID != s.delivered[1].payload.EventID {
				t.Fatalf("retry delivered stale outage or changed event ID: %+v", late)
			}
			*healthy = false
			tick(false)
			tick(false)
			if len(s.delivered) != 5 || s.delivered[3].payload.EventID == outage.EventID || s.delivered[3].payload.State != "unreachable" {
				t.Fatalf("new outage was suppressed: %+v", s.delivered)
			}
		})
	}
}

func TestWatchdogNewOutageSupersedesPartialRecovery(t *testing.T) {
	s := &recipientSender{failing: map[string]bool{}}
	_, healthy, tick := watchFixture(t, s)
	tick(false)
	tick(false)
	old := s.forToken("first")[0]
	s.failing["first"] = true
	*healthy = true
	tick(false)
	tick(true)
	*healthy = false
	tick(false)
	tick(true)
	delete(s.failing, "first")
	tick(false)
	first := s.forToken("first")
	second := s.forToken("second")
	if len(first) != 2 || first[1].State != "unreachable" || first[1].IncidentID == old.IncidentID || len(second) != 3 || first[1].EventID != second[2].EventID {
		t.Fatalf("stale recovery or suppressed new outage: first=%+v second=%+v", first, second)
	}
}

func TestWatchdogRetryKeepsEventIDAndPersistsBeforeSending(t *testing.T) {
	s := &recipientSender{failing: map[string]bool{"first": true}}
	w, _, tick := watchFixture(t, s)
	s.beforeSend = func(p Payload) {
		var persisted WatchState
		b, err := os.ReadFile(w.StateFile)
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(b, &persisted); err != nil {
			t.Fatal(err)
		}
		if persisted.IncidentID != p.IncidentID || persisted.PendingEvent != p.State {
			t.Fatal("notification sent before its stable transition was persisted")
		}
	}
	tick(false)
	tick(true)
	tick(true)
	var state WatchState
	b, err := os.ReadFile(w.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &state); err != nil {
		t.Fatal(err)
	}
	if !state.Alerted || state.PendingEvent != "unreachable" || state.IncidentID == "" {
		t.Fatalf("observed outage not persisted: %+v", state)
	}
	delete(s.failing, "first")
	tick(false)
	first := s.forToken("first")
	second := s.forToken("second")
	if len(first) != 1 || len(second) != 1 || first[0].EventID != second[0].EventID {
		t.Fatalf("retry duplicated successful recipient or changed ID: %+v", s.delivered)
	}
	for _, a := range s.attempts {
		if a.payload.EventID != first[0].EventID {
			t.Fatal("retry changed event ID")
		}
	}
}

func TestWatchdogDisabledWithoutTimezoneDoesNotSend(t *testing.T) {
	s := &recipientSender{}
	w, _, tick := watchFixture(t, s)
	w.Preferences = Preferences{Enabled: false}
	tick(false)
	tick(false)
	tick(false)
	if len(s.attempts) != 0 {
		t.Fatal("disabled watchdog sent notification")
	}
}

func TestWatchdogUpgradesLegacyPartialDelivery(t *testing.T) {
	s := &recipientSender{}
	w, healthy, tick := watchFixture(t, s)
	legacy := WatchState{IncidentID: watchTokenKey("legacy-outage"), Failures: 2, Delivered: map[string]bool{watchTokenKey("first") + ":unreachable": true}}
	if err := w.save(legacy); err != nil {
		t.Fatal(err)
	}
	*healthy = true
	tick(false)
	tick(false)
	if len(s.delivered) != 2 {
		t.Fatal("legacy partial outage lost recovery")
	}
	for _, d := range s.delivered {
		if d.payload.State != "reachable" || d.payload.IncidentID != legacy.IncidentID {
			t.Fatal("invalid migrated recovery")
		}
	}
}
