package notifications

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Sender interface {
	Send(context.Context, string, Payload) error
}

var ErrUnregistered = errors.New("push_token_unregistered")
var ErrDelivery = errors.New("push_delivery_failed")

type RetryError struct{ After time.Duration }

func (e *RetryError) Error() string { return "push_delivery_failed" }
func (e *RetryError) Unwrap() error { return ErrDelivery }

type FCM struct {
	Project, Email string
	Key            *rsa.PrivateKey
	HTTP           *http.Client
	mu             sync.Mutex
	token          string
	expiry         time.Time
}

func NewFCM(path string) (*FCM, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("FCM credentials unavailable")
	}
	var c struct {
		Project string `json:"project_id"`
		Email   string `json:"client_email"`
		Key     string `json:"private_key"`
		Type    string `json:"type"`
	}
	if len(b) > 65536 || json.Unmarshal(b, &c) != nil || c.Type != "service_account" || !regexp.MustCompile(`^[a-z][a-z0-9-]{4,62}$`).MatchString(c.Project) || !strings.HasSuffix(c.Email, ".gserviceaccount.com") {
		return nil, errors.New("invalid FCM credentials")
	}
	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(c.Key))
	if err != nil {
		return nil, errors.New("invalid FCM signing key")
	}
	return &FCM{Project: c.Project, Email: c.Email, Key: key, HTTP: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (f *FCM) access(ctx context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if time.Until(f.expiry) > time.Minute {
		return f.token, nil
	}
	now := time.Now()
	assertion, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": f.Email, "scope": "https://www.googleapis.com/auth/firebase.messaging", "aud": "https://oauth2.googleapis.com/token", "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()}).SignedString(f.Key)
	if err != nil {
		return "", ErrDelivery
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := f.HTTP.Do(req)
	if err != nil {
		return "", ErrDelivery
	}
	defer res.Body.Close()
	var data struct {
		Token   string `json:"access_token"`
		Expires int    `json:"expires_in"`
	}
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 65536)).Decode(&data) != nil || data.Token == "" || data.Expires < 60 || data.Expires > 3600 {
		return "", ErrDelivery
	}
	f.token = data.Token
	f.expiry = now.Add(time.Duration(data.Expires) * time.Second)
	return f.token, nil
}
func (f *FCM) Send(ctx context.Context, token string, p Payload) error {
	access, err := f.access(ctx)
	if err != nil {
		return err
	}
	priority := "normal"
	if p.Urgent {
		priority = "high"
	}
	// Data-only delivery: the app validates, deduplicates and renders fixed text.
	data := map[string]string{"event_id": p.EventID, "incident_id": p.IncidentID, "severity": p.Severity, "title": p.Title, "state": p.State, "route": p.Route, "urgent": strconv.FormatBool(p.Urgent)}
	b, _ := json.Marshal(map[string]any{"message": map[string]any{"token": token, "data": data, "android": map[string]any{"priority": priority, "ttl": "300s"}}})
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://fcm.googleapis.com/v1/projects/"+f.Project+"/messages:send", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Content-Type", "application/json")
	res, err := f.HTTP.Do(req)
	if err != nil {
		return ErrDelivery
	}
	defer res.Body.Close()
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}
	if res.StatusCode == 401 {
		f.mu.Lock()
		f.expiry = time.Time{}
		f.mu.Unlock()
	}
	if res.StatusCode == 429 || res.StatusCode == 503 {
		after := time.Minute
		if seconds, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil {
			after = time.Duration(seconds) * time.Second
		} else if date, err := http.ParseTime(res.Header.Get("Retry-After")); err == nil {
			after = time.Until(date)
		}
		if after < time.Minute {
			after = time.Minute
		}
		if after > time.Hour {
			after = time.Hour
		}
		return &RetryError{After: after}
	}
	var body struct {
		Error struct {
			Details []struct {
				Type string `json:"@type"`
				Code string `json:"errorCode"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 65536)).Decode(&body) == nil {
		for _, d := range body.Error.Details {
			if d.Type == "type.googleapis.com/google.firebase.fcm.v1.FcmError" && d.Code == "UNREGISTERED" {
				return ErrUnregistered
			}
		}
	}
	return ErrDelivery
}
