package recovery

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gofrs/flock"
	"prisme.local/platform/internal/coordination"
	"prisme.local/platform/internal/operations"
)

type Batch struct {
	Origin string               `json:"origin"`
	After  int                  `json:"after"`
	Cursor int                  `json:"cursor"`
	Events []coordination.Event `json:"events"`
}

func (i *Identity) audit(action, target, actor, result string) error {
	return i.Append(coordination.Event{ID: operations.ID(), Origin: "recovery", Actor: actor, Target: target, At: time.Now().UTC(), Action: action, Result: result})
}
func (i *Identity) Append(event coordination.Event) error {
	lock := flock.New(filepath.Join(i.Root, "audit.lock"))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ok, e := lock.TryLockContext(ctx, 20*time.Millisecond)
	if e != nil || !ok {
		return operations.ErrUnavailable
	}
	defer lock.Close()
	raw, e := json.Marshal(event)
	if e != nil {
		return e
	}
	f, e := os.OpenFile(filepath.Join(i.Root, "audit.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(append(raw, '\n'))
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	return e
}
func (i *Identity) OperationsAudit(config operations.Config) operations.AuditFunc {
	return func(ctx context.Context, p operations.Principal, action, target, id, incident, result string) error {
		epoch := uint64(0)
		for _, t := range config.Targets {
			if t.ID == target {
				state, e := coordination.Read(t.CoordinationRoot, t.LockTarget)
				if e != nil {
					return e
				}
				epoch = state.Epoch
			}
		}
		return i.Append(coordination.Event{ID: operations.ID(), Origin: "recovery", Actor: p.Device, Target: target, Epoch: epoch, At: time.Now().UTC(), Action: action, Result: result})
	}
}
func (i *Identity) Export(after int) (Batch, string, error) {
	b := Batch{Origin: operations.Hash(i.key.Public().(ed25519.PublicKey)), After: after, Events: []coordination.Event{}}
	if after < 0 {
		return b, "", operations.ErrInvalid
	}
	f, e := os.Open(filepath.Join(i.Root, "audit.jsonl"))
	if e != nil {
		return b, "", e
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	n := 0
	for scan.Scan() {
		n++
		if n <= after {
			continue
		}
		var event coordination.Event
		if json.Unmarshal(scan.Bytes(), &event) != nil {
			return b, "", operations.ErrUnavailable
		}
		b.Events = append(b.Events, event)
		if len(b.Events) == 200 {
			break
		}
	}
	if scan.Err() != nil || n < after {
		return b, "", operations.ErrInvalid
	}
	b.Cursor = n
	raw, _ := json.Marshal(b)
	signature := ed25519.Sign(i.key, raw)
	return b, base64.StdEncoding.EncodeToString(signature), nil
}
func (i *Identity) export(w http.ResponseWriter, r *http.Request) {
	if _, e := i.OperationsIdentity().Authenticate(r); e != nil {
		rejected(w)
		return
	}
	after, e := strconv.Atoi(r.URL.Query().Get("after"))
	if e != nil {
		rejected(w)
		return
	}
	b, signature, e := i.Export(after)
	if e != nil {
		rejected(w)
		return
	}
	reply(w, map[string]any{"batch": b, "signature": signature})
}
