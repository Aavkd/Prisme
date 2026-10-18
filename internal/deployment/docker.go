package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Docker struct {
	Root   string
	Client *http.Client
}

var localHTTP = &http.Client{Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{Proxy: nil, IdleConnTimeout: 30 * time.Second, MaxIdleConns: 20, MaxIdleConnsPerHost: 2}}

func (d *Docker) client() *http.Client {
	if d.Client != nil {
		return d.Client
	}
	return localHTTP
}
func project(p Plan) string { return "prisme-" + p.Manifest.ID + "-" + p.Slot }
func port(s Service, slot string) int {
	if slot == "green" {
		return s.GreenPort
	}
	return s.BluePort
}

// Commands have fixed verbs and argument boundaries. Only bounded, selected
// inspect fields are returned; raw stdout/stderr never enters the job journal.
type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 1024*1024 {
		return 0, ErrPrecondition
	}
	return b.Buffer.Write(p)
}
func command(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	configureProcess(cmd)
	var out boundedBuffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	// A shell and inherited Compose options are never involved. Docker credentials
	// remain in the worker's existing private Docker config.
	err := cmd.Run()
	if err != nil {
		return nil, ErrUnknown
	}
	return out.Bytes(), nil
}
func (d *Docker) Compose(p Plan) ([]byte, error) {
	services := map[string]any{}
	for _, s := range p.Manifest.Services {
		service := map[string]any{"image": p.Images[s.Name], "restart": "unless-stopped", "labels": map[string]string{"dev.mantara.prisme.target": p.Manifest.ID}, "mem_limit": "512m", "cpus": 1.0}
		if s.Port > 0 {
			service["ports"] = []string{fmt.Sprintf("127.0.0.1:%d:%d", port(s, p.Slot), s.Port)}
		}
		if s.EnvFile != "" {
			service["env_file"] = []map[string]any{{"path": filepath.Join(d.Root, "secrets", s.EnvFile), "format": "raw"}}
		}
		if p.Manifest.Network != "" {
			service["networks"] = []string{"default", "dependencies"}
		}
		services[s.Name] = service
	}
	doc := map[string]any{"services": services}
	if p.Manifest.Network != "" {
		doc["networks"] = map[string]any{"default": map[string]any{}, "dependencies": map[string]any{"external": true, "name": p.Manifest.Network}}
	}
	return json.MarshalIndent(doc, "", "  ")
}
func (d *Docker) file(p Plan) (string, error) {
	root := filepath.Join(d.Root, "plans", Hash(p))
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", ErrPrecondition
	}
	path := filepath.Join(root, "compose.json")
	b, err := d.Compose(p)
	if err != nil {
		return "", err
	}
	// Exclusive creation: a prepared file cannot silently change between steps.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if os.IsExist(err) {
		old, e := os.ReadFile(path)
		if e != nil || !bytes.Equal(old, b) {
			return "", ErrPrecondition
		}
		return path, nil
	}
	if err != nil {
		return "", ErrPrecondition
	}
	_, err = f.Write(b)
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return "", ErrPrecondition
	}
	return path, nil
}
func (d *Docker) compose(ctx context.Context, p Plan, args ...string) ([]byte, error) {
	file, err := d.file(p)
	if err != nil {
		return nil, err
	}
	all := []string{"compose", "--project-name", project(p), "--env-file", filepath.Join(d.Root, "empty.env"), "-f", file}
	return command(ctx, append(all, args...)...)
}
func (d *Docker) Execute(ctx context.Context, p Plan, step string) (map[string]any, error) {
	proof := map[string]any{"project": project(p)}
	switch step {
	case "preconditions":
		if p.Manifest.Validate() != nil || p.ManifestHash != Hash(p.Manifest) || len(p.Manifest.Blockers()) != 0 {
			return nil, ErrPrecondition
		}
		if _, err := p.Manifest.Images(p.Release); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(d.Root, 0700); err != nil {
			return nil, ErrPrecondition
		}
		free, err := freeBytes(d.Root)
		if err != nil || free < p.Manifest.MinimumFreeBytes {
			return nil, ErrPrecondition
		}
		proof["workspace_free_bytes"] = free
		// No environment interpolation from a developer's .env file.
		f, err := os.OpenFile(filepath.Join(d.Root, "empty.env"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			_ = f.Close()
		} else if !os.IsExist(err) {
			return nil, ErrPrecondition
		}
		b, err := os.ReadFile(filepath.Join(d.Root, "empty.env"))
		if err != nil || len(b) != 0 {
			return nil, ErrPrecondition
		}
		if !d.secretsMatch(p) {
			return nil, ErrPrecondition
		}
		// Require a reachable engine before obtaining any image or changing runtime.
		dockerRoot, infoErr := command(ctx, "info", "--format", "{{.DockerRootDir}}")
		if infoErr != nil {
			return nil, ErrPrecondition
		}
		engineFree, diskErr := freeBytes(strings.TrimSpace(string(dockerRoot)))
		if diskErr != nil || engineFree < p.Manifest.MinimumFreeBytes {
			return nil, ErrPrecondition
		}
		proof["docker_free_bytes"] = engineFree
		ids, err := command(ctx, "ps", "-aq", "--filter", "label=com.docker.compose.project="+project(p))
		if err != nil {
			return nil, ErrPrecondition
		}
		for _, id := range strings.Fields(string(ids)) {
			owner, err := command(ctx, "inspect", "--format", `{{index .Config.Labels "dev.mantara.prisme.target"}}`, id)
			if err != nil || strings.TrimSpace(string(owner)) != p.Manifest.ID {
				return nil, ErrPrecondition
			}
		}
		if p.Manifest.Strategy == "blue-green" {
			if _, _, err = d.route(ctx, p, p.PreviousSlot); err != nil {
				return nil, ErrPrecondition
			}
		}
	case "configuration":
		if _, err := d.compose(ctx, p, "config", "--quiet"); err != nil {
			return nil, ErrPrecondition
		}
		proof["compose_hash"] = Hash(p)
	case "pull":
		if _, err := d.compose(ctx, p, "pull", "--policy", "always"); err != nil {
			return nil, ErrPrecondition
		}
		proof["images"] = p.Images
	case "start":
		if !d.secretsMatch(p) {
			return nil, ErrPrecondition
		}
		if _, err := d.compose(ctx, p, "up", "--detach", "--no-build", "--pull", "never", "--wait", "--wait-timeout", "60"); err != nil {
			return nil, err
		}
	case "health":
		if err := d.health(ctx, p); err != nil {
			return nil, err
		}
		proof["images"] = p.Images
		proof["checks"] = "all_services"
	case "traffic":
		if p.Manifest.Strategy == "blue-green" {
			if err := d.switchTraffic(ctx, p); err != nil {
				return nil, err
			}
			proof["route"] = p.Manifest.CaddyID
			proof["slot"] = p.Slot
		} else {
			proof["mode"] = "compose_recreation"
		}
	default:
		return nil, ErrInvalid
	}
	return proof, nil
}
func (d *Docker) health(ctx context.Context, p Plan) error {
	for _, s := range p.Manifest.Services {
		ids, err := command(ctx, "ps", "-a", "--filter", "label=com.docker.compose.project="+project(p), "--filter", "label=com.docker.compose.service="+s.Name, "--format", "{{.ID}}")
		if err != nil {
			return ErrUnknown
		}
		id := strings.TrimSpace(string(ids))
		if id == "" || strings.ContainsAny(id, "\r\n ") {
			return ErrHealth
		}
		b, err := command(ctx, "inspect", "--format", `{"image":{{json .Config.Image}},"state":{{json .State.Status}},"health":{{if .State.Health}}{{json .State.Health.Status}}{{else}}"absent"{{end}}}`, id)
		if err != nil {
			return ErrUnknown
		}
		var actual struct{ Image, State, Health string }
		if json.Unmarshal(b, &actual) != nil {
			return ErrUnknown
		}
		if actual.Image != p.Images[s.Name] || actual.State != "running" || (s.DockerHealth && actual.Health != "healthy") || actual.Health == "unhealthy" {
			return ErrHealth
		}
		if s.Port > 0 {
			if err = d.httpHealth(ctx, fmt.Sprintf("http://127.0.0.1:%d%s", port(s, p.Slot), s.HealthPath)); err != nil {
				return err
			}
		}
	}
	return nil
}
func (d *Docker) httpHealth(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return ErrPrecondition
	}
	resp, err := d.client().Do(req)
	if err != nil {
		return ErrHealth
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ErrHealth
	}
	return nil
}
func (d *Docker) Verify(ctx context.Context, p Plan) error {
	if !d.secretsMatch(p) {
		return ErrPrecondition
	}
	if err := d.health(ctx, p); err != nil {
		return err
	}
	if p.Manifest.Strategy == "blue-green" {
		if _, _, err := d.route(ctx, p, p.Slot); err != nil {
			return ErrUnknown
		}
		return d.httpHealth(ctx, p.Manifest.PublicHealthURL)
	}
	return nil
}

type upstream struct {
	Dial string `json:"dial"`
}

func dial(p Plan, slot string) string {
	for _, s := range p.Manifest.Services {
		if s.Name == p.Manifest.IngressService {
			return "127.0.0.1:" + strconv.Itoa(port(s, slot))
		}
	}
	return ""
}

// Only the upstreams of a pre-existing, explicitly owned reverse_proxy handler
// can be patched. No whole-config loads, Caddyfile edits, or Cloudflare writes.
func (d *Docker) route(ctx context.Context, p Plan, slot string) ([]byte, string, error) {
	endpoint := strings.TrimRight(p.Manifest.CaddyURL, "/") + "/id/" + p.Manifest.CaddyID
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, "", ErrPrecondition
	}
	resp, err := d.client().Do(req)
	if err != nil {
		return nil, "", ErrUnknown
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 65537))
	if err != nil || len(b) > 65536 || resp.StatusCode != 200 || resp.Header.Get("Etag") == "" {
		return nil, "", ErrUnknown
	}
	var handler struct {
		ID        string     `json:"@id"`
		Handler   string     `json:"handler"`
		Upstreams []upstream `json:"upstreams"`
	}
	if json.Unmarshal(b, &handler) != nil || handler.ID != p.Manifest.CaddyID || handler.Handler != "reverse_proxy" || len(handler.Upstreams) != 1 {
		return nil, "", ErrPrecondition
	}
	// Initial import requires a sentinel backend. It cannot adopt historical routes.
	expected := "127.0.0.1:1"
	if slot != "" {
		expected = dial(p, slot)
	}
	if handler.Upstreams[0].Dial != expected {
		return nil, "", ErrConflict
	}
	return b, resp.Header.Get("Etag"), nil
}
func (d *Docker) switchTraffic(ctx context.Context, p Plan) error {
	_, etag, err := d.route(ctx, p, p.PreviousSlot)
	if err != nil {
		return err
	}
	b, _ := json.Marshal([]upstream{{Dial: dial(p, p.Slot)}})
	endpoint := strings.TrimRight(p.Manifest.CaddyURL, "/") + "/id/" + p.Manifest.CaddyID + "/upstreams"
	req, err := http.NewRequestWithContext(ctx, "PATCH", endpoint, bytes.NewReader(b))
	if err != nil {
		return ErrPrecondition
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", etag)
	resp, err := d.client().Do(req)
	if err != nil {
		return ErrUnknown
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return ErrUnknown
	}
	_, _, err = d.route(ctx, p, p.Slot)
	return err
}
func SecretHashes(root string, m Manifest) (map[string]string, error) {
	out := map[string]string{}
	for _, s := range m.Services {
		if s.EnvFile == "" {
			continue
		}
		path := filepath.Join(root, "secrets", s.EnvFile)
		actual, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil, ErrPrecondition
		}
		base, err := filepath.EvalSymlinks(filepath.Join(root, "secrets"))
		if err != nil || filepath.Dir(actual) != base {
			return nil, ErrPrecondition
		}
		b, err := os.ReadFile(path)
		if err != nil || len(b) > 65536 {
			return nil, ErrPrecondition
		}
		out[s.EnvFile] = Hash(string(b))
	}
	return out, nil
}
func (d *Docker) secretsMatch(p Plan) bool {
	h, err := SecretHashes(d.Root, p.Manifest)
	return err == nil && Hash(h) == Hash(p.SecretHashes)
}
