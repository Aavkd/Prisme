package edge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
)

type limitedOutput struct{ bytes.Buffer }

func (b *limitedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 2*1024*1024 {
		return 0, ErrUnavailable
	}
	return b.Buffer.Write(p)
}
func caddyCommand(ctx context.Context, c *Caddy, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Binary, args...)
	cmd.Dir = filepath.Dir(c.SourceFile)
	// Provider output can include secrets from historical configuration. Never log it.
	var out limitedOutput
	cmd.Stdout = &out
	if cmd.Run() != nil {
		return nil, ErrUnavailable
	}
	return out.Bytes(), nil
}
func importLine(t Target) string { return "import \"" + filepath.ToSlash(t.Caddy.ManagedFile) + "\"" }
func managed(t Target, up string) string {
	domain := t.Domain
	if t.Caddy.HTTPOnly {
		domain = "http://" + domain
	}
	imports := ""
	for _, snippet := range t.Caddy.Snippets {
		imports += "\timport " + snippet + "\n"
	}
	return "# prisme-managed: " + t.ID + "\n" + domain + " {\n" + imports + "\treverse_proxy " + up + "\n}\n"
}
func managedOwned(t Target, v string) bool {
	if v == "" {
		return true
	}
	for _, up := range t.Caddy.Upstreams {
		if v == managed(t, up) {
			return true
		}
	}
	return false
}
func readConfig(path string, optional bool) (string, error) {
	info, e := os.Lstat(path)
	if optional && errors.Is(e, os.ErrNotExist) {
		return "", nil
	}
	if e != nil || !info.Mode().IsRegular() || info.Size() > 1024*1024 {
		return "", ErrInvalid
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return "", ErrUnavailable
	}
	return string(b), nil
}
func (d *Native) active(ctx context.Context, t Target) (json.RawMessage, string, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(t.Caddy.AdminURL, "/")+"/config/", nil)
	resp, e := d.client().Do(req)
	if e != nil {
		return nil, "", ErrUnavailable
	}
	defer resp.Body.Close()
	b, e := bounded(resp.Body)
	if e != nil || resp.StatusCode != 200 || !json.Valid(b) || resp.Header.Get("Etag") == "" {
		return nil, "", ErrUnavailable
	}
	return b, resp.Header.Get("Etag"), nil
}
func (d *Native) caddyState(ctx context.Context, t Target) (State, string, error) {
	var s State
	var e error
	s.Source, e = readConfig(t.Caddy.SourceFile, false)
	if e != nil {
		return s, "", e
	}
	s.Managed, e = readConfig(t.Caddy.ManagedFile, true)
	if e != nil || !managedOwned(t, s.Managed) {
		return s, "", ErrConflict
	}
	a, tag, e := d.active(ctx, t)
	s.Active = a
	return s, tag, e
}

// Adapt a temporary source in the same directory so historical relative imports
// retain their meaning. Only the exact owned import is substituted for validation.
func (d *Native) adapt(ctx context.Context, t Target, source, block string) (json.RawMessage, error) {
	lines := strings.Split(source, "\n")
	count := 0
	for i, line := range lines {
		if strings.TrimSpace(line) == importLine(t) {
			lines[i] = block
			count++
		}
	}
	if count > 1 {
		return nil, ErrInvalid
	}
	if count == 0 && block != "" {
		return nil, ErrInvalid
	}
	f, e := os.CreateTemp(filepath.Dir(t.Caddy.SourceFile), ".prisme-validate-*.caddy")
	if e != nil {
		return nil, ErrUnavailable
	}
	defer os.Remove(f.Name())
	_, e = f.WriteString(strings.Join(lines, "\n"))
	ce := f.Close()
	if e != nil || ce != nil {
		return nil, ErrUnavailable
	}
	b, e := caddyCommand(ctx, t.Caddy, "adapt", "--config", f.Name(), "--adapter", "caddyfile", "--validate")
	if e != nil || !json.Valid(b) {
		return nil, ErrInvalid
	}
	return b, nil
}
func (d *Native) prepareCaddy(ctx context.Context, t Target, r Request, rollback *Snapshot) (Snapshot, Plan, error) {
	before, _, e := d.caddyState(ctx, t)
	if e != nil {
		return Snapshot{}, Plan{}, e
	}
	current, e := d.adapt(ctx, t, before.Source, before.Managed)
	if e != nil {
		return Snapshot{}, Plan{}, e
	}
	if Hash(jsonValue(current)) != Hash(jsonValue(before.Active)) {
		return Snapshot{}, Plan{}, ErrConflict
	}
	after := before
	if rollback != nil {
		if Hash(jsonValue(before.Active)) != Hash(jsonValue(rollback.After.Active)) || before.Source != rollback.After.Source || before.Managed != rollback.After.Managed {
			return Snapshot{}, Plan{}, ErrConflict
		}
		after = rollback.Before
	} else {
		if !slices.Contains(t.Caddy.Upstreams, r.Value) {
			return Snapshot{}, Plan{}, ErrInvalid
		}
		after.Managed = managed(t, r.Value)
		found := false
		for _, line := range strings.Split(before.Source, "\n") {
			if strings.TrimSpace(line) == importLine(t) {
				found = true
			}
		}
		if !found {
			after.Source = before.Source + "\n# Prisme owned route: " + t.ID + "\n" + importLine(t) + "\n"
		}
	}
	after.Active, e = d.adapt(ctx, t, after.Source, after.Managed)
	if e != nil {
		return Snapshot{}, Plan{}, e
	}
	if Hash(jsonValue(before.Active)) == Hash(jsonValue(after.Active)) {
		return Snapshot{}, Plan{}, ErrInvalid
	}
	// Baseline must already be healthy; otherwise rollback cannot claim a healthy state.
	if _, e = d.checks(ctx, caddyChecks(t, before.Managed)); e != nil {
		return Snapshot{}, Plan{}, e
	}
	p := Plan{Before: before.Managed, After: after.Managed, Impact: "Reload shared Caddy configuration; preserve historical domains. " + t.Domain, Verification: "Validate Caddy configuration, compare active state, then run every configured HTTP/HTTPS check.", Rollback: "Restore the saved source, managed block and active configuration if checks fail. Concurrent drift blocks rollback.", Checks: t.Checks}
	if before.Source != after.Source {
		// Only the owned append/removal changes. Do not copy unrelated historical
		// credentials into the public plan merely to show an import-line diff.
		p.Before = "# managed file\n" + before.Managed
		p.After = "# managed file\n" + after.Managed
		if before.Managed != "" {
			p.Before += "\n# historical Caddyfile import\n" + importLine(t)
		}
		if after.Managed != "" {
			p.After += "\n# historical Caddyfile import\n" + importLine(t)
		}
	}
	return Snapshot{Before: before, After: after}, p, nil
}
func atomicConfig(path, data string) error {
	mode := os.FileMode(0600)
	if info, e := os.Lstat(path); e == nil {
		if !info.Mode().IsRegular() {
			return ErrConflict
		}
		mode = info.Mode().Perm()
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".prisme-write-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(mode); e == nil {
		_, e = f.WriteString(data)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if e = os.Rename(f.Name(), path); e != nil {
		return e
	}
	if runtime.GOOS != "windows" {
		dir, e := os.Open(filepath.Dir(path))
		if e != nil {
			return e
		}
		defer dir.Close()
		return dir.Sync()
	}
	return nil
}
func (d *Native) load(ctx context.Context, t Target, active json.RawMessage, etag string) error {
	// /config/ supports atomic compare-and-swap, unlike an unguarded /load.
	req, _ := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(t.Caddy.AdminURL, "/")+"/config/", bytes.NewReader(active))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", etag)
	resp, e := d.client().Do(req)
	if e != nil {
		return ErrUncertain
	}
	defer resp.Body.Close()
	if resp.StatusCode == 412 {
		return ErrConflict
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ErrUnavailable
	}
	return nil
}
func (d *Native) applyCaddy(ctx context.Context, t Target, s Snapshot) (string, map[string]any, error) {
	before, tag, e := d.caddyState(ctx, t)
	if e != nil {
		return "failed", nil, e
	}
	if before.Source != s.Before.Source || before.Managed != s.Before.Managed || Hash(jsonValue(before.Active)) != Hash(jsonValue(s.Before.Active)) {
		return "failed", nil, ErrConflict
	}
	adapted, e := d.adapt(ctx, t, s.After.Source, s.After.Managed)
	if e != nil || Hash(jsonValue(adapted)) != Hash(jsonValue(s.After.Active)) {
		return "failed", nil, ErrConflict
	}
	if _, e = d.checks(ctx, caddyChecks(t, before.Managed)); e != nil {
		return "failed", nil, e
	}
	// Save desired source before activating it. Any interruption is retained as uncertain.
	if atomicConfig(t.Caddy.ManagedFile, s.After.Managed) != nil || atomicConfig(t.Caddy.SourceFile, s.After.Source) != nil {
		return "uncertain", nil, ErrUncertain
	}
	e = d.load(ctx, t, s.After.Active, tag)
	if e != nil {
		if errors.Is(e, ErrUncertain) {
			return "uncertain", nil, e
		}
		if atomicConfig(t.Caddy.ManagedFile, s.Before.Managed) != nil || atomicConfig(t.Caddy.SourceFile, s.Before.Source) != nil {
			return "uncertain", nil, ErrUncertain
		}
		return "failed", nil, e
	}
	active, afterTag, e := d.active(ctx, t)
	if e != nil || Hash(jsonValue(active)) != Hash(jsonValue(s.After.Active)) {
		return "uncertain", nil, ErrUncertain
	}
	evidence, e := d.checks(ctx, caddyChecks(t, s.After.Managed))
	if e == nil {
		return "succeeded", evidence, nil
	}
	// Fresh context permits rollback even if the request/check deadline elapsed.
	rollback, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	src, se := readConfig(t.Caddy.SourceFile, false)
	block, be := readConfig(t.Caddy.ManagedFile, false)
	if se != nil || be != nil || src != s.After.Source || block != s.After.Managed {
		return "uncertain", evidence, ErrUncertain
	}
	if d.load(rollback, t, s.Before.Active, afterTag) != nil {
		return "uncertain", evidence, ErrUncertain
	}
	if atomicConfig(t.Caddy.ManagedFile, s.Before.Managed) != nil || atomicConfig(t.Caddy.SourceFile, s.Before.Source) != nil {
		return "uncertain", evidence, ErrUncertain
	}
	restored, _, ae := d.active(rollback, t)
	proof, ce := d.checks(rollback, caddyChecks(t, s.Before.Managed))
	evidence["rollback"] = proof
	if ae != nil || ce != nil || Hash(jsonValue(restored)) != Hash(jsonValue(s.Before.Active)) {
		return "uncertain", evidence, ErrUncertain
	}
	return "rolled_back", evidence, nil
}

// There is no managed target to probe before first import or after its removal.
// Active-config comparison still verifies removal; all existing domains are probed.
func caddyChecks(t Target, block string) []Check {
	if block != "" {
		return t.Checks
	}
	out := []Check{}
	for _, c := range t.Checks {
		u, _ := url.Parse(c.URL)
		if c.Host == t.Domain || (c.Host == "" && u.Hostname() == t.Domain) {
			continue
		}
		out = append(out, c)
	}
	return out
}
