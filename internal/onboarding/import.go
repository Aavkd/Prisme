// Package onboarding compares bounded, sanitized project import evidence.
package onboarding

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"prisme.local/platform/internal/deployment"
	"prisme.local/platform/internal/inventory"
)

var ErrEvidence = errors.New("invalid project import evidence")
var identifier = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,79}$`)
var immutable = regexp.MustCompile(`^[a-z0-9][a-z0-9./:_-]*@sha256:[a-f0-9]{64}$`)
var edgePort = regexp.MustCompile(`(^|[^0-9])(80|443)->`)

type Service struct {
	Name       string `json:"name"`
	ImageHash  string `json:"image_hash"`
	Immutable  bool   `json:"immutable"`
	Build      bool   `json:"build"`
	Published  []int  `json:"published"`
	SharedEdge bool   `json:"shared_edge"`
}
type Report struct {
	Version       int       `json:"version"`
	Project       string    `json:"project"`
	DeclaredHash  string    `json:"declared_hash"`
	HostHash      string    `json:"host_hash"`
	InventoryHash string    `json:"inventory_hash"`
	ObservedAt    time.Time `json:"observed_at"`
	Declared      []Service `json:"declared"`
	Host          []Service `json:"host"`
	Running       []string  `json:"running"`
	Blockers      []string  `json:"blockers"`
	Ready         bool      `json:"ready"`
}

// Read docker compose config --format json from a private file. Environment,
// commands, labels, volumes and literal image names never enter the report.
func summarize(raw []byte) ([]Service, error) {
	if len(raw) > inventory.MaxBytes {
		return nil, ErrEvidence
	}
	var doc struct {
		Services map[string]struct {
			Image string          `json:"image"`
			Build json.RawMessage `json:"build"`
			Ports []struct {
				Published json.RawMessage `json:"published"`
			} `json:"ports"`
		} `json:"services"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if d.Decode(&doc) != nil || d.Decode(new(any)) != io.EOF || len(doc.Services) == 0 || len(doc.Services) > 100 {
		return nil, ErrEvidence
	}
	out := []Service{}
	for name, s := range doc.Services {
		if !identifier.MatchString(name) || len(s.Image) > 1024 {
			return nil, ErrEvidence
		}
		x := Service{Name: name, ImageHash: deployment.Hash(s.Image), Immutable: immutable.MatchString(s.Image), Build: len(s.Build) > 0 && string(s.Build) != "null", Published: []int{}}
		imageName := strings.Split(strings.Split(s.Image, "@")[0], ":")[0]
		x.SharedEdge = strings.EqualFold(name, "caddy") || imageName == "caddy" || strings.HasSuffix(imageName, "/caddy")
		for _, port := range s.Ports {
			if len(port.Published) == 0 {
				continue
			}
			var str string
			if json.Unmarshal(port.Published, &str) != nil {
				str = string(port.Published)
			}
			p, err := strconv.Atoi(str)
			if err != nil || p < 1 || p > 65535 {
				return nil, ErrEvidence
			}
			x.Published = append(x.Published, p)
			if p == 80 || p == 443 {
				x.SharedEdge = true
			}
		}
		sort.Ints(x.Published)
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func Compare(project string, declared, host []byte, snapshot inventory.Snapshot, now time.Time) (Report, error) {
	r := Report{Version: 1, Project: project, DeclaredHash: deployment.Hash(json.RawMessage(declared)), HostHash: deployment.Hash(json.RawMessage(host)), InventoryHash: deployment.Hash(snapshot), ObservedAt: snapshot.ObservedAt, Running: []string{}, Blockers: []string{}}
	if !identifier.MatchString(project) || snapshot.Validate(now) != nil {
		return r, ErrEvidence
	}
	var err error
	if r.Declared, err = summarize(declared); err != nil {
		return r, err
	}
	if r.Host, err = summarize(host); err != nil {
		return r, err
	}
	block := func(code string) { r.Blockers = append(r.Blockers, code) }
	if !snapshot.DockerAvailable || len(snapshot.Warnings) > 0 {
		block("inventory_incomplete")
	}
	if now.Sub(snapshot.ObservedAt) > 5*time.Minute {
		block("inventory_stale")
	}
	if deployment.Hash(r.Declared) != deployment.Hash(r.Host) {
		block("declared_host_drift")
	}
	for _, s := range r.Declared {
		if s.SharedEdge {
			block("shared_edge_must_remain_separate:" + s.Name)
		}
		if s.Build || !s.Immutable {
			block("immutable_image_required:" + s.Name)
		}
	}
	for _, s := range r.Host {
		if s.SharedEdge {
			block("host_shared_edge_conflict:" + s.Name)
		}
	}
	seen := map[string]bool{}
	for _, c := range snapshot.Containers {
		if c.Project != project {
			continue
		}
		if !identifier.MatchString(c.Service) {
			return r, ErrEvidence
		}
		if !seen[c.Service] {
			r.Running = append(r.Running, c.Service)
			seen[c.Service] = true
		}
		if c.State != "running" {
			block("service_not_running:" + c.Service)
		}
		if edgePort.MatchString(c.Ports) {
			block("running_shared_edge_conflict:" + c.Service)
		}
		for _, s := range r.Host {
			if s.Name != c.Service {
				continue
			}
			matches := s.ImageHash == deployment.Hash(c.Image)
			for _, image := range snapshot.Images {
				if image.ID == c.ImageID {
					for _, digest := range image.Digests {
						matches = matches || s.ImageHash == deployment.Hash(digest)
					}
				}
			}
			if !matches {
				block("image_inventory_drift:" + c.Service)
			}
		}
	}
	sort.Strings(r.Running)
	hostNames := []string{}
	for _, s := range r.Host {
		hostNames = append(hostNames, s.Name)
	}
	if deployment.Hash(hostNames) != deployment.Hash(r.Running) {
		block("host_inventory_service_drift")
	}
	sort.Strings(r.Blockers)
	r.Ready = len(r.Blockers) == 0
	return r, nil
}
