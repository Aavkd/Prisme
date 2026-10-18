// Package inventory holds a bounded, secret-excluding observation contract.
package inventory

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"os/exec"
	"regexp"
	"time"
)

const MaxBytes = 2 * 1024 * 1024

type Host struct {
	Hostname       string `json:"hostname"`
	OS             string `json:"os"`
	Architecture   string `json:"architecture"`
	CPUs           int    `json:"cpus"`
	MemoryBytes    *int64 `json:"memory_bytes"`
	DiskTotalBytes *int64 `json:"disk_total_bytes"`
	DiskFreeBytes  *int64 `json:"disk_free_bytes"`
}
type Container struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Image       string `json:"image"`
	ImageID     string `json:"image_id"`
	State       string `json:"state"`
	Health      string `json:"health"`
	Project     string `json:"project"`
	Service     string `json:"service"`
	ConfigFiles string `json:"config_files"`
	Ports       string `json:"ports"`
	Networks    string `json:"networks"`
}
type Resource struct {
	Name    string `json:"name"`
	Driver  string `json:"driver"`
	Project string `json:"project"`
}
type Image struct {
	ID      string   `json:"id"`
	Tags    []string `json:"tags"`
	Digests []string `json:"digests"`
}
type Snapshot struct {
	Protocol        string      `json:"protocol_version"`
	ObservedAt      time.Time   `json:"observed_at"`
	Source          string      `json:"source"`
	Host            Host        `json:"host"`
	DockerAvailable bool        `json:"docker_available"`
	Containers      []Container `json:"containers"`
	Networks        []Resource  `json:"networks"`
	Volumes         []Resource  `json:"volumes"`
	Images          []Image     `json:"images"`
	Warnings        []string    `json:"warnings"`
}

var safeText = regexp.MustCompile(`^[^\x00-\x1f\x7f]*$`)

func (s *Snapshot) Normalize() {
	if s.Containers == nil {
		s.Containers = []Container{}
	}
	if s.Networks == nil {
		s.Networks = []Resource{}
	}
	if s.Volumes == nil {
		s.Volumes = []Resource{}
	}
	if s.Images == nil {
		s.Images = []Image{}
	}
	if s.Warnings == nil {
		s.Warnings = []string{}
	}
	for i := range s.Images {
		if s.Images[i].Tags == nil {
			s.Images[i].Tags = []string{}
		}
		if s.Images[i].Digests == nil {
			s.Images[i].Digests = []string{}
		}
	}
}

func (s Snapshot) Validate(now time.Time) error {
	if s.Protocol != "1" || (s.Source != "agent" && s.Source != "ssh-readonly") || s.ObservedAt.After(now.Add(time.Minute)) || s.ObservedAt.Before(now.Add(-24*time.Hour)) || s.Host.Hostname == "" || s.Host.CPUs < 1 || s.Host.CPUs > 65536 || len(s.Containers) > 1000 || len(s.Networks) > 1000 || len(s.Volumes) > 1000 || len(s.Images) > 1000 || len(s.Warnings) > 20 {
		return errors.New("invalid inventory")
	}
	for _, v := range []*int64{s.Host.MemoryBytes, s.Host.DiskTotalBytes, s.Host.DiskFreeBytes} {
		if v != nil && *v < 0 {
			return errors.New("negative capacity")
		}
	}
	if s.Host.DiskFreeBytes != nil && s.Host.DiskTotalBytes != nil && *s.Host.DiskFreeBytes > *s.Host.DiskTotalBytes {
		return errors.New("invalid disk capacity")
	}
	raw, _ := json.Marshal(s)
	var decoded any
	_ = json.Unmarshal(raw, &decoded)
	var valid func(any) bool
	valid = func(v any) bool {
		switch x := v.(type) {
		case string:
			return len(x) <= 4096 && safeText.MatchString(x)
		case []any:
			for _, a := range x {
				if !valid(a) {
					return false
				}
			}
		case map[string]any:
			for _, a := range x {
				if !valid(a) {
					return false
				}
			}
		}
		return true
	}
	if !valid(decoded) {
		return errors.New("invalid inventory text")
	}
	seen := map[string]bool{}
	for _, c := range s.Containers {
		if c.ID == "" || seen[c.ID] {
			return errors.New("duplicate container")
		}
		seen[c.ID] = true
	}
	return nil
}

//go:embed collect.py
var collector string

type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > MaxBytes {
		return 0, errors.New("inventory too large")
	}
	return b.Buffer.Write(p)
}

// Collect runs a fixed read-only program. An SSH alias is data, never a shell fragment.
func Collect(ctx context.Context, sshAlias, python string) (Snapshot, error) {
	var snapshot Snapshot
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	if sshAlias != "" {
		if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,79}$`).MatchString(sshAlias) {
			return snapshot, errors.New("invalid SSH alias")
		}
		cmd = exec.CommandContext(ctx, "ssh", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=8", sshAlias, "python3 -")
	} else {
		if python == "" {
			python = "python3"
		}
		cmd = exec.CommandContext(ctx, python, "-")
	}
	cmd.Stdin = bytes.NewBufferString(collector)
	var out boundedBuffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return snapshot, errors.New("read-only collector unavailable (check Python, SSH and Docker access)")
	}
	dec := json.NewDecoder(&out)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&snapshot); err != nil {
		return snapshot, errors.New("invalid collector response")
	}
	if sshAlias != "" {
		snapshot.Source = "ssh-readonly"
	}
	return snapshot, snapshot.Validate(time.Now().UTC())
}
