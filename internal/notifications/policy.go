// Package notifications transports minimal context; operational data stays private.
package notifications

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"
	_ "time/tzdata"
)

type RulePolicy struct {
	RuleID          string `json:"rule_id"`
	Urgent          bool   `json:"urgent"`
	EscalateSeconds int    `json:"escalate_seconds"`
	MaxEscalations  int    `json:"max_escalations"`
}
type Config struct {
	CredentialsFile string       `json:"credentials_file"`
	GroupSeconds    int          `json:"group_seconds"`
	Rules           []RulePolicy `json:"rules"`
}

func Load(path string) (Config, error) {
	var c Config
	f, err := os.Open(path)
	if err != nil {
		return c, err
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 65536))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil || d.Decode(new(any)) != io.EOF {
		return c, errors.New("invalid notification configuration")
	}
	if c.GroupSeconds < 0 || c.GroupSeconds > 900 || len(c.Rules) > 200 {
		return c, errors.New("invalid notification bounds")
	}
	seen := map[string]bool{}
	for _, r := range c.Rules {
		if r.RuleID == "" || seen[r.RuleID] || r.MaxEscalations < 0 || r.MaxEscalations > 5 || (r.MaxEscalations > 0 && (r.EscalateSeconds < 60 || r.EscalateSeconds > 86400)) {
			return c, errors.New("invalid notification rule")
		}
		seen[r.RuleID] = true
	}
	return c, nil
}

type Preferences struct {
	Enabled    bool   `json:"enabled"`
	Timezone   string `json:"timezone"`
	QuietStart int    `json:"quiet_start"`
	QuietEnd   int    `json:"quiet_end"`
}

func (p Preferences) Validate() error {
	if p.QuietStart < 0 || p.QuietStart > 1439 || p.QuietEnd < 0 || p.QuietEnd > 1439 {
		return errors.New("invalid quiet hours")
	}
	_, err := time.LoadLocation(p.Timezone)
	return err
}

// Advance over real instants to handle missing and repeated wall-clock times at DST.
func (p Preferences) Due(now time.Time, urgent bool, group time.Duration) time.Time {
	due := now.Add(group)
	if urgent {
		return now
	}
	loc, err := time.LoadLocation(p.Timezone)
	if err != nil {
		return now.Add(24 * time.Hour)
	}
	quiet := func(t time.Time) bool {
		v := t.In(loc)
		m := v.Hour()*60 + v.Minute()
		if p.QuietStart == p.QuietEnd {
			return false
		}
		if p.QuietStart < p.QuietEnd {
			return m >= p.QuietStart && m < p.QuietEnd
		}
		return m >= p.QuietStart || m < p.QuietEnd
	}
	for i := 0; i < 1500 && quiet(due); i++ {
		due = due.Truncate(time.Minute).Add(time.Minute)
	}
	return due
}

type Payload struct {
	EventID    string `json:"event_id"`
	IncidentID string `json:"incident_id"`
	Severity   string `json:"severity"`
	Title      string `json:"title"`
	State      string `json:"state"`
	Route      string `json:"route"`
	Urgent     bool   `json:"urgent"`
}
