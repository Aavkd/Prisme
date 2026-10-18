package github

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"prisme.local/platform/internal/observability"
)

var ErrUnavailable = errors.New("GitHub unavailable")
var ErrInvalid = errors.New("invalid GitHub data")
var ErrRateLimit = errors.New("GitHub rate limited")

type Provider interface {
	JSON(context.Context, string, any) error
	Download(context.Context, string, int64) ([]byte, error)
}
type Client struct {
	config           Config
	key              *rsa.PrivateKey
	http             *http.Client
	base             string
	mu               sync.Mutex
	token            string
	expires, retryAt time.Time
}

func NewClient(c Config, key *rsa.PrivateKey) *Client {
	return &Client{config: c, key: key, base: "https://api.github.com", http: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func readBounded(r io.Reader, max int64) ([]byte, error) {
	b, e := io.ReadAll(io.LimitReader(r, max+1))
	if e != nil || int64(len(b)) > max {
		return nil, ErrUnavailable
	}
	return b, nil
}
func (c *Client) request(ctx context.Context, path, token string) (*http.Response, error) {
	r, e := http.NewRequestWithContext(ctx, "GET", c.base+path, nil)
	if e != nil {
		return nil, ErrInvalid
	}
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Accept", "application/vnd.github+json")
	r.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, e := c.http.Do(r)
	if e != nil {
		return nil, ErrUnavailable
	}
	return resp, nil
}
func (c *Client) access(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx.Err() != nil {
		return "", ErrUnavailable
	}
	if time.Now().Before(c.retryAt) {
		return "", ErrRateLimit
	}
	if time.Until(c.expires) > time.Minute {
		return c.token, nil
	}
	now := time.Now()
	// Bound token renewal while holding the cache lock, including background polls.
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	j, e := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.RegisteredClaims{Issuer: c.config.AppID, IssuedAt: jwt.NewNumericDate(now.Add(-time.Minute)), ExpiresAt: jwt.NewNumericDate(now.Add(9 * time.Minute))}).SignedString(c.key)
	if e != nil {
		return "", ErrUnavailable
	}
	ids := []int64{}
	for _, r := range c.config.Repositories {
		id, _ := strconv.ParseInt(r.ID, 10, 64)
		ids = append(ids, id)
	}
	b, _ := json.Marshal(map[string]any{"repository_ids": ids, "permissions": map[string]string{"contents": "read", "actions": "read", "pull_requests": "read", "metadata": "read"}})
	r, e := http.NewRequestWithContext(ctx, "POST", c.base+"/app/installations/"+c.config.InstallationID+"/access_tokens", bytes.NewReader(b))
	if e != nil {
		return "", ErrInvalid
	}
	r.Header.Set("Authorization", "Bearer "+j)
	r.Header.Set("Accept", "application/vnd.github+json")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, e := c.http.Do(r)
	if e != nil {
		return "", ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		c.setBackoff(resp)
		return "", ErrUnavailable
	}
	b, e = readBounded(resp.Body, 65536)
	if e != nil {
		return "", e
	}
	var out struct {
		Token   string    `json:"token"`
		Expires time.Time `json:"expires_at"`
	}
	if json.Unmarshal(b, &out) != nil || out.Token == "" || time.Until(out.Expires) < time.Minute {
		return "", ErrInvalid
	}
	c.token = out.Token
	c.expires = out.Expires
	return c.token, nil
}

// Called with mu held; response bodies and signed URLs never enter errors/logs.
func (c *Client) setBackoff(r *http.Response) {
	d := time.Minute
	if n, e := strconv.Atoi(r.Header.Get("Retry-After")); e == nil && n > 0 && n < 86400 {
		d = time.Duration(n) * time.Second
	}
	if n, e := strconv.ParseInt(r.Header.Get("X-RateLimit-Reset"), 10, 64); e == nil && r.Header.Get("X-RateLimit-Remaining") == "0" {
		if t := time.Until(time.Unix(n, 0)); t > d && t < 24*time.Hour {
			d = t
		}
	}
	c.retryAt = time.Now().Add(d)
}
func (c *Client) response(ctx context.Context, path string) (*http.Response, error) {
	t, e := c.access(ctx)
	if e != nil {
		return nil, e
	}
	r, e := c.request(ctx, path, t)
	if e != nil {
		return nil, e
	}
	if r.StatusCode == 401 || r.StatusCode == 403 || r.StatusCode == 429 || r.StatusCode >= 500 {
		c.mu.Lock()
		if r.StatusCode == 401 {
			c.token = ""
			c.expires = time.Time{}
		}
		c.setBackoff(r)
		c.mu.Unlock()
		r.Body.Close()
		return nil, ErrUnavailable
	}
	return r, nil
}
func (c *Client) JSON(ctx context.Context, path string, out any) error {
	r, e := c.response(ctx, path)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return ErrUnavailable
	}
	b, e := readBounded(r.Body, 4<<20)
	if e != nil {
		return e
	}
	if json.Unmarshal(b, out) != nil {
		return ErrInvalid
	}
	return nil
}
func (c *Client) Download(ctx context.Context, path string, max int64) ([]byte, error) {
	r, e := c.response(ctx, path)
	if e != nil {
		return nil, e
	}
	defer r.Body.Close()
	if r.StatusCode != 302 {
		return nil, ErrUnavailable
	}
	u, e := url.Parse(r.Header.Get("Location"))
	if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || !(strings.HasSuffix(u.Hostname(), ".blob.core.windows.net") || strings.HasSuffix(u.Hostname(), ".actions.githubusercontent.com")) {
		return nil, ErrInvalid
	}
	// Separate unauthenticated request: never forward the installation token.
	q, _ := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	d, e := c.http.Do(q)
	if e != nil {
		return nil, ErrUnavailable
	}
	defer d.Body.Close()
	if d.StatusCode != 200 {
		return nil, ErrUnavailable
	}
	return readBounded(d.Body, max)
}
func ManifestFromZIP(b []byte) ([]byte, error) {
	z, e := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if e != nil || len(z.File) != 1 {
		return nil, ErrInvalid
	}
	f := z.File[0]
	if f.Name != "prisme-release.json" || f.UncompressedSize64 > 65536 {
		return nil, ErrInvalid
	}
	r, e := f.Open()
	if e != nil {
		return nil, ErrInvalid
	}
	defer r.Close()
	return readBounded(r, 65536)
}
func Redact(s string) string { return observability.Redact(s) }
