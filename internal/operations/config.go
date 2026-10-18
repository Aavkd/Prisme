// Package operations owns bounded SSH, SFTP and vault operations. It never logs
// terminal bytes, file contents, keys, bearer credentials or provider errors.
package operations

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var ErrInvalid = errors.New("invalid_operation")
var ErrDenied = errors.New("operation_denied")
var ErrConflict = errors.New("revision_conflict")
var ErrUnavailable = errors.New("operation_unavailable")
var identifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

const MaxFile = 1024 * 1024
const ChunkSize = 48 * 1024

type Target struct {
	Diagnostics      bool     `json:"diagnostics"`
	Containers       []string `json:"containers,omitempty"`
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Environment      string   `json:"environment"`
	Address          string   `json:"address"`
	User             string   `json:"user"`
	KeyFile          string   `json:"key_file"`
	HostKey          string   `json:"host_key"`
	Terminal         bool     `json:"terminal"`
	Files            bool     `json:"files"`
	Roots            []string `json:"roots"`
	GuardPath        string   `json:"guard_path"`
	CoordinationRoot string   `json:"coordination_root"`
	LockTarget       string   `json:"lock_target"`
}
type Config struct {
	RecoveryKeys  map[string]string `json:"recovery_keys,omitempty"`
	Version       int               `json:"version"`
	StateRoot     string            `json:"state_root"`
	MasterKeyFile string            `json:"master_key_file"`
	Targets       []Target          `json:"targets"`
}

func Decode(b []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil || d.Decode(new(any)) != io.EOF {
		return ErrInvalid
	}
	return nil
}
func Load(file string) (Config, error) {
	var c Config
	b, e := os.ReadFile(file)
	if e != nil || len(b) > 256*1024 {
		return c, ErrInvalid
	}
	if Decode(b, &c) != nil || c.Version != 1 || !filepath.IsAbs(c.StateRoot) || len(c.Targets) > 50 {
		return c, ErrInvalid
	}
	seen := map[string]bool{}
	for _, t := range c.Targets {
		if !identifier.MatchString(t.ID) || seen[t.ID] || !identifier.MatchString(t.User) || !identifier.MatchString(t.LockTarget) || t.Name == "" || len(t.Name) > 100 || strings.ContainsAny(t.Name, "\r\n\x00") || !filepath.IsAbs(t.KeyFile) || !strings.HasPrefix(t.HostKey, "SHA256:") || !path.IsAbs(t.GuardPath) || !path.IsAbs(t.CoordinationRoot) {
			return c, ErrInvalid
		}
		if _, _, e = net.SplitHostPort(t.Address); e != nil {
			return c, ErrInvalid
		}
		if t.Environment != "production" && t.Environment != "staging" && t.Environment != "development" {
			return c, ErrInvalid
		}
		for _, r := range t.Roots {
			if !path.IsAbs(r) || path.Clean(r) != r || r == "/" || strings.ContainsAny(r, "\x00\r\n") {
				return c, ErrInvalid
			}
		}
		if t.Files && len(t.Roots) == 0 {
			return c, ErrInvalid
		}
		seen[t.ID] = true
	}
	return c, nil
}
func ID() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic("secure randomness unavailable")
	}
	return hex.EncodeToString(b)
}
func Hash(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func Binding(action, target string, payload map[string]any) string {
	b, _ := json.Marshal(payload)
	return Hash([]byte(action + "\n" + target + "\n" + string(b)))
}
