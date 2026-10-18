package github

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"github.com/golang-jwt/jwt/v5"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAppTokenScopeCacheAndBackoff(t *testing.T) {
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	issued, calls := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app/installations/7/access_tokens" {
			issued++
			token, e := jwt.Parse(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), func(t *jwt.Token) (any, error) { return &key.PublicKey, nil }, jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer("8"), jwt.WithExpirationRequired())
			if e != nil || !token.Valid {
				t.Error("invalid App JWT")
			}
			var in struct {
				Repositories []int64           `json:"repository_ids"`
				Permissions  map[string]string `json:"permissions"`
			}
			if json.NewDecoder(r.Body).Decode(&in) != nil || len(in.Repositories) != 1 || in.Repositories[0] != 9 || in.Permissions["actions"] != "read" {
				t.Error("token not scoped")
			}
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(map[string]any{"token": "fixture-installation-token", "expires_at": time.Now().Add(time.Hour)})
		} else {
			calls++
			if r.Header.Get("Authorization") != "Bearer fixture-installation-token" {
				t.Error("wrong token")
			}
			if calls == 3 {
				w.Header().Set("Retry-After", "90")
				w.WriteHeader(429)
				return
			}
			w.Write([]byte(`{"ok":true}`))
		}
	}))
	defer server.Close()
	c := NewClient(Config{AppID: "8", InstallationID: "7", Repositories: []Repository{{ID: "9"}}}, key)
	c.base = server.URL
	for range 2 {
		var out map[string]any
		if c.JSON(context.Background(), "/test", &out) != nil {
			t.Fatal("request failed")
		}
	}
	var out map[string]any
	if c.JSON(context.Background(), "/test", &out) == nil || c.JSON(context.Background(), "/test", &out) != ErrRateLimit {
		t.Fatal("rate limit ignored")
	}
	if issued != 1 || calls != 3 {
		t.Fatal("token cache/backoff failed")
	}
}

func TestDownloadDoesNotForwardCredentialsOrFollowUntrustedRedirects(t *testing.T) {
	for _, location := range []string{"http://example.blob.core.windows.net/log", "https://127.0.0.1/log", "https://evil.example/log", "https://user:pass@x.blob.core.windows.net/log", "https://x.blob.core.windows.net/log"} {
		t.Run(location, func(t *testing.T) {
			c := NewClient(Config{}, nil)
			c.token = "private"
			c.expires = time.Now().Add(time.Hour)
			downloads := 0
			c.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "api.github.com" {
					return &http.Response{StatusCode: 302, Header: http.Header{"Location": {location}}, Body: io.NopCloser(strings.NewReader(""))}, nil
				}
				downloads++
				if r.Header.Get("Authorization") != "" {
					t.Error("credential forwarded")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("logs"))}, nil
			})
			b, e := c.Download(context.Background(), "/logs", 100)
			if location == "https://x.blob.core.windows.net/log" {
				if e != nil || string(b) != "logs" || downloads != 1 {
					t.Fatal("valid download failed")
				}
			} else if e == nil || downloads != 0 {
				t.Fatal("untrusted redirect followed")
			}
		})
	}
}

func TestManifestAndArchiveBoundaries(t *testing.T) {
	repo := Repository{Name: "org/pilot", Environment: "production"}
	build := Build{ID: "10", Attempt: 2, Commit: strings.Repeat("a", 40)}
	m := Manifest{Version: 1, Repository: repo.Name, Environment: repo.Environment, Commit: build.Commit, RunID: build.ID, RunAttempt: 2, Images: []Image{{Service: "web", Reference: "ghcr.io/org/web@sha256:" + strings.Repeat("b", 64)}}}
	if ValidateManifest(m, repo, build) != nil {
		t.Fatal("valid manifest rejected")
	}
	for _, reference := range []string{"ghcr.io/org//web@sha256:", "ghcr.io/org/web/@sha256:"} {
		copy := m
		copy.Images = append([]Image(nil), m.Images...)
		copy.Images[0].Reference = reference + strings.Repeat("b", 64)
		if ValidateManifest(copy, repo, build) == nil {
			t.Fatal("malformed image repository accepted")
		}
	}
	for _, change := range []func(*Manifest){func(x *Manifest) { x.Commit = strings.Repeat("c", 40) }, func(x *Manifest) { x.Environment = "staging" }, func(x *Manifest) { x.RunAttempt = 1 }, func(x *Manifest) { x.Repository = "org/other" }, func(x *Manifest) { x.Images[0].Reference = "ghcr.io/org/web:latest" }, func(x *Manifest) { x.Images = append(x.Images, x.Images[0]) }} {
		copy := m
		copy.Images = append([]Image(nil), m.Images...)
		change(&copy)
		if ValidateManifest(copy, repo, build) == nil {
			t.Fatal("invalid provenance accepted")
		}
	}
	for _, name := range []string{"prisme-release.json", "../prisme-release.json", "other.json"} {
		var buf bytes.Buffer
		z := zip.NewWriter(&buf)
		f, _ := z.Create(name)
		f.Write([]byte(`{}`))
		z.Close()
		_, e := ManifestFromZIP(buf.Bytes())
		if (e == nil) != (name == "prisme-release.json") {
			t.Fatal("archive path guard failed")
		}
	}
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	f, _ := z.Create("prisme-release.json")
	f.Write(bytes.Repeat([]byte("a"), 65537))
	z.Close()
	if _, e := ManifestFromZIP(buf.Bytes()); e == nil {
		t.Fatal("decompression bound ignored")
	}
	if _, e := readBounded(strings.NewReader("oversized"), 2); e == nil {
		t.Fatal("response bound ignored")
	}
}

type fakeProvider struct {
	path string
	out  any
}

func (f *fakeProvider) JSON(_ context.Context, p string, out any) error {
	f.path = p
	b, _ := json.Marshal(f.out)
	return json.Unmarshal(b, out)
}
func (f *fakeProvider) Download(context.Context, string, int64) ([]byte, error) {
	return nil, ErrUnavailable
}
func TestBrowseEncodesRefsAndRedactsDecodedFiles(t *testing.T) {
	p := &fakeProvider{out: map[string]any{"type": "file", "encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte("password=private-value")), "download_url": "https://secret-url"}}
	s := Service{Provider: p}
	r := Repository{Name: "org/repo"}
	out, e := s.Browse(context.Background(), r, "files", "feature/a&other=1", "folder/a b.txt", "1")
	if e != nil || !strings.Contains(p.path, "ref=feature%2Fa%26other%3D1") || !strings.Contains(p.path, "a%20b.txt") {
		t.Fatal("unsafe URL construction")
	}
	b, _ := json.Marshal(out)
	if strings.Contains(string(b), "private-value") || strings.Contains(string(b), "secret-url") || !strings.Contains(string(b), "REDACTED") {
		t.Fatal("file not redacted")
	}
	for _, path := range []string{"../secret", "a/../secret", "/root", "a\\b"} {
		if _, e = s.Browse(context.Background(), r, "files", "", path, "1"); e == nil {
			t.Fatal("unsafe path accepted")
		}
	}
	if _, e = s.Browse(context.Background(), r, "arbitrary", "", "", "1"); e == nil {
		t.Fatal("unrestricted API proxy")
	}
}
