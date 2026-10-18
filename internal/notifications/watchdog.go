package notifications

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

type WatchState struct {
	IncidentID   string          `json:"incident_id"`
	Failures     int             `json:"failures"`
	Successes    int             `json:"successes"`
	Alerted      bool            `json:"alerted"`
	PendingEvent string          `json:"pending_event,omitempty"`
	LastProbe    time.Time       `json:"last_probe"`
	Delivered    map[string]bool `json:"delivered"`
}
type Watchdog struct {
	URL, StateFile string
	Tokens         []string
	Threshold      int
	HTTP           *http.Client
	Sender         Sender
	Now            func() time.Time
	Preferences    Preferences
}

func (w *Watchdog) Tick(ctx context.Context) error {
	u, err := url.Parse(w.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || w.Threshold < 2 || w.Threshold > 60 || len(w.Tokens) == 0 || len(w.Tokens) > 20 {
		return errors.New("invalid watchdog configuration")
	}
	state := WatchState{Delivered: map[string]bool{}}
	b, err := os.ReadFile(w.StateFile)
	if err == nil {
		if json.Unmarshal(b, &state) != nil {
			return errors.New("invalid watchdog state")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if state.Delivered == nil {
		state.Delivered = map[string]bool{}
	}
	// Upgrade old state, where Alerted became true only after every recipient succeeded.
	if state.PendingEvent == "" && state.IncidentID != "" {
		state.Alerted = true
		state.PendingEvent = "unreachable"
	}
	now := w.Now()
	prefs := w.Preferences
	if prefs.Timezone == "" {
		prefs.Timezone = "UTC"
	}
	if err := prefs.Validate(); err != nil {
		return errors.New("invalid watchdog quiet hours")
	}
	if !state.LastProbe.IsZero() && now.Sub(state.LastProbe) < 15*time.Second {
		return nil
	}
	// A long gap is loss of evidence; do not count it as continuous failure.
	if now.Sub(state.LastProbe) > 2*time.Minute {
		state.Failures = 0
		state.Successes = 0
	}
	state.LastProbe = now
	req, _ := http.NewRequestWithContext(ctx, "GET", w.URL, nil)
	res, err := w.HTTP.Do(req)
	healthy := err == nil && res.StatusCode == 200
	if res != nil {
		res.Body.Close()
	}
	if healthy {
		state.Successes = min(state.Successes+1, w.Threshold)
		state.Failures = 0
	} else {
		state.Failures = min(state.Failures+1, w.Threshold)
		state.Successes = 0
	}
	if !state.Alerted && state.Failures >= w.Threshold {
		b := make([]byte, 32)
		if _, err = rand.Read(b); err != nil {
			return err
		}
		state.IncidentID = hex.EncodeToString(b)
		state.Alerted = true
		state.PendingEvent = "unreachable"
		state.Delivered = map[string]bool{}
	}
	if state.Alerted && state.Successes >= w.Threshold {
		state.Alerted = false
		state.PendingEvent = "reachable"
		state.Delivered = map[string]bool{}
	}
	// Commit the observed transition and stable event ID before any external send.
	// A new confirmed state supersedes undelivered older notifications.
	if err = w.save(state); err != nil {
		return err
	}
	event := state.PendingEvent
	confirmed := event == "unreachable" && state.Failures >= w.Threshold || event == "reachable" && state.Successes >= w.Threshold
	if confirmed && prefs.Enabled && !prefs.Due(now, false, 0).After(now) {
		p := Payload{IncidentID: state.IncidentID, Severity: "warning", Title: "Prisme", State: event, Route: "/watchdog", Urgent: false}
		p.EventID = watchTokenKey(state.IncidentID + ":" + event)
		var deliveryErr error
		for _, token := range w.Tokens {
			// Opaque local token fingerprints; never duplicate registration tokens in state.
			key := watchTokenKey(token) + ":" + event
			if state.Delivered[key] {
				continue
			}
			if err = w.Sender.Send(ctx, token, p); err != nil {
				// A broken recipient must not starve the others, including on recovery.
				deliveryErr = errors.Join(deliveryErr, err)
				continue
			}
			state.Delivered[key] = true
			if err = w.save(state); err != nil {
				return err
			}
		}
		if deliveryErr != nil {
			return deliveryErr
		}
		if event == "reachable" {
			state.IncidentID = ""
			state.PendingEvent = ""
			state.Delivered = map[string]bool{}
		}
	}
	return w.save(state)
}
func (w *Watchdog) save(s WatchState) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(w.StateFile), ".watchdog-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), w.StateFile)
}

func watchTokenKey(v string) string { h := sha256.Sum256([]byte(v)); return hex.EncodeToString(h[:]) }
