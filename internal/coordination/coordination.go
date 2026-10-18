// Package coordination arbitrates host mutations without a database or network.
// The guard and deployment worker MUST use the same local directory and target.
package coordination

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	"github.com/gofrs/flock"
)

var ErrBlocked = errors.New("host_reconciliation_required")
var valid = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

type State struct {
	Epoch     uint64 `json:"epoch"`
	Active    bool   `json:"active"`
	Suspended bool   `json:"suspended"`
}
type Event struct {
	ID     string    `json:"event_id"`
	Origin string    `json:"origin"`
	Actor  string    `json:"actor"`
	Target string    `json:"target"`
	Epoch  uint64    `json:"epoch"`
	At     time.Time `json:"at"`
	Action string    `json:"action"`
	Result string    `json:"result"`
}
type Lease struct {
	lock                        *flock.Flock
	root, target, origin, actor string
	State                       State
}

func paths(root, target string) (string, error) {
	if !filepath.IsAbs(root) || !valid.MatchString(target) {
		return "", ErrBlocked
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	return filepath.Join(root, target), nil
}
func Read(root, target string) (State, error) {
	base, err := paths(root, target)
	if err != nil {
		return State{}, err
	}
	raw, err := os.ReadFile(base + ".json")
	if errors.Is(err, os.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}
	var s State
	err = json.Unmarshal(raw, &s)
	return s, err
}
func write(base string, s State) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(base), ".state-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(raw)
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
	if err = os.Rename(f.Name(), base+".json"); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		dir, e := os.Open(filepath.Dir(base))
		if e != nil {
			return e
		}
		defer dir.Close()
		return dir.Sync()
	}
	return nil
}
func Acquire(root, target, origin, actor string) (*Lease, error) {
	if (origin != "normal" && origin != "recovery") || !valid.MatchString(actor) {
		return nil, ErrBlocked
	}
	base, err := paths(root, target)
	if err != nil {
		return nil, err
	}
	lock := flock.New(base + ".lock")
	ok, err := lock.TryLock()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrBlocked
	}
	s, err := Read(root, target)
	if err != nil || s.Active || (s.Suspended && origin == "normal") {
		_ = lock.Close()
		return nil, ErrBlocked
	}
	l := &Lease{lock: lock, root: root, target: target, origin: origin, actor: actor, State: s}
	l.State.Epoch++
	l.State.Active = true
	if origin == "recovery" {
		l.State.Suspended = true
	}
	// Audit must be durable BEFORE issuing authority to execute.
	if err = l.Event("acquire", "started"); err == nil {
		err = write(base, l.State)
	}
	if err != nil {
		_ = lock.Close()
		return nil, err
	}
	return l, nil
}
func (l *Lease) Event(action, result string) error {
	if !valid.MatchString(action) || !valid.MatchString(result) {
		return ErrBlocked
	}
	e := Event{ID: fmt.Sprintf("%s-%d-%d", l.target, l.State.Epoch, time.Now().UnixNano()), Origin: l.origin, Actor: l.actor, Target: l.target, Epoch: l.State.Epoch, At: time.Now().UTC(), Action: action, Result: result}
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(l.root, l.target+".audit.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(raw, '\n'))
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err == nil {
		err = ce
	}
	return err
}

// Close(false) retains the active marker: transport loss never proves shell exit.
func (l *Lease) Close(stopped bool) error {
	if l.lock == nil {
		return ErrBlocked
	}
	defer l.lock.Close()
	result := "uncertain"
	if stopped {
		result = "stopped"
	}
	err := l.Event("release", result)
	if err == nil && stopped {
		l.State.Active = false
		err = write(filepath.Join(l.root, l.target), l.State)
	}
	l.lock = nil
	return err
}

// Reconcile is an explicit host operator acknowledgement of stopped processes
// and observed runtime. It never steals a live OS lock; old plans become stale.
func Reconcile(root, target, actor string, epoch uint64) error {
	base, err := paths(root, target)
	if err != nil {
		return err
	}
	lock := flock.New(base + ".lock")
	ok, err := lock.TryLock()
	if err != nil {
		return err
	}
	if !ok {
		return ErrBlocked
	}
	defer lock.Close()
	s, err := Read(root, target)
	if err != nil || s.Epoch != epoch {
		return ErrBlocked
	}
	l := &Lease{root: root, target: target, origin: "normal", actor: actor, State: s}
	l.State.Epoch++
	l.State.Active = false
	l.State.Suspended = false
	if err = l.Event("reconcile", "acknowledged"); err != nil {
		return err
	}
	return write(base, l.State)
}
func Events(root, target string, after int) ([]Event, int, error) {
	base, err := paths(root, target)
	if err != nil || after < 0 {
		return nil, 0, ErrBlocked
	}
	f, err := os.Open(base + ".audit.jsonl")
	if errors.Is(err, os.ErrNotExist) {
		return []Event{}, after, nil
	}
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	out := []Event{}
	scan := bufio.NewScanner(f)
	n := 0
	for scan.Scan() {
		n++
		if n <= after {
			continue
		}
		var e Event
		if json.Unmarshal(scan.Bytes(), &e) != nil {
			return nil, 0, ErrBlocked
		}
		out = append(out, e)
		if len(out) == 200 {
			break
		}
	}
	if err = scan.Err(); err != nil {
		return nil, 0, err
	}
	if n < after {
		return nil, 0, ErrBlocked
	}
	return out, n, nil
}
