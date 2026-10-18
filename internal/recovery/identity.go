// Package recovery provides a standalone, locally provisioned passkey registry.
// Its sessions, revocations, recovery codes and audit do not use the control plane.
package recovery

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/gofrs/flock"
	"prisme.local/platform/internal/operations"
)

type Device struct {
	ActionKey   []byte                `json:"action_key,omitempty"`
	Name        string                `json:"name"`
	Revoked     bool                  `json:"revoked"`
	Credentials []webauthn.Credential `json:"credentials"`
}
type Registry struct {
	Devices  map[string]Device `json:"devices"`
	Tickets  map[string]string `json:"tickets"`
	Recovery map[string]bool   `json:"recovery"`
}
type User struct {
	ID          string
	Credentials []webauthn.Credential
}

func (u User) WebAuthnID() []byte                         { return []byte("prisme-recovery-" + u.ID) }
func (u User) WebAuthnName() string                       { return "recovery-administrator" }
func (u User) WebAuthnDisplayName() string                { return "Prisme recovery" }
func (u User) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }

type flow struct {
	Device, Name, PollHash, Kind string
	Expires                      time.Time
	Approved                     bool
	Session                      *webauthn.SessionData
	Recovery                     bool
}
type Identity struct {
	Root     string
	WebAuthn *webauthn.WebAuthn
	mu       sync.Mutex
	flows    map[string]*flow
	sessions map[string]operations.Principal
	key      ed25519.PrivateKey
}

func New(root string, w *webauthn.WebAuthn) (*Identity, error) {
	if !filepath.IsAbs(root) || w == nil {
		return nil, operations.ErrInvalid
	}
	if e := os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	i := &Identity{Root: root, WebAuthn: w, flows: map[string]*flow{}, sessions: map[string]operations.Principal{}}
	raw, e := os.ReadFile(filepath.Join(root, "audit-key"))
	if e != nil || len(raw) != ed25519.PrivateKeySize {
		return nil, errors.New("run recovery provisioning before starting")
	}
	i.key = ed25519.PrivateKey(raw)
	if e = i.transaction(func(*Registry) error { return nil }); e != nil {
		return nil, e
	}
	return i, nil
}
func atomic(file string, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(file), ".registry-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = os.Rename(f.Name(), file); e != nil {
		return e
	}
	if runtime.GOOS != "windows" {
		dir, e := os.Open(filepath.Dir(file))
		if e != nil {
			return e
		}
		defer dir.Close()
		return dir.Sync()
	}
	return nil
}
func (i *Identity) transaction(fn func(*Registry) error) error {
	lock := flock.New(filepath.Join(i.Root, "registry.lock"))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ok, e := lock.TryLockContext(ctx, 20*time.Millisecond)
	if e != nil || !ok {
		return operations.ErrUnavailable
	}
	defer lock.Close()
	var reg Registry
	raw, e := os.ReadFile(filepath.Join(i.Root, "registry.json"))
	if e != nil || json.Unmarshal(raw, &reg) != nil || reg.Devices == nil || reg.Tickets == nil || reg.Recovery == nil {
		return operations.ErrUnavailable
	}
	if e = fn(&reg); e != nil {
		return e
	}
	return atomic(filepath.Join(i.Root, "registry.json"), reg)
}

// Provision must be invoked by the host administrator with private output paths.
func Provision(root, device, name, output string, recoverCode bool) error {
	if len(device) != 64 || name == "" || len(name) > 80 || !filepath.IsAbs(root) || !filepath.IsAbs(output) {
		return operations.ErrInvalid
	}
	if e := os.MkdirAll(root, 0700); e != nil {
		return e
	}
	regFile := filepath.Join(root, "registry.json")
	if _, e := os.Stat(regFile); errors.Is(e, os.ErrNotExist) {
		if e = atomic(regFile, Registry{Devices: map[string]Device{}, Tickets: map[string]string{}, Recovery: map[string]bool{}}); e != nil {
			return e
		}
	}
	keyFile := filepath.Join(root, "audit-key")
	if _, e := os.Stat(keyFile); errors.Is(e, os.ErrNotExist) {
		_, key, e := ed25519.GenerateKey(rand.Reader)
		if e != nil {
			return e
		}
		f, e := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		_, e = f.Write(key)
		if e == nil {
			e = f.Sync()
		}
		ce := f.Close()
		if e == nil {
			e = ce
		}
		if e != nil {
			return e
		}
	}
	i := &Identity{Root: root}
	code := operations.ID()
	// Create the private output before enabling the one-use code. Never print it.
	f, e := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	_, e = f.WriteString(code)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return i.transaction(func(r *Registry) error {
		if recoverCode {
			r.Recovery[operations.Hash([]byte(code))] = true
			return nil
		}
		if d, ok := r.Devices[device]; ok && (d.Revoked || len(d.Credentials) > 0) {
			return operations.ErrDenied
		}
		r.Devices[device] = Device{Name: name, Credentials: []webauthn.Credential{}}
		r.Tickets[operations.Hash([]byte(code))] = device
		return nil
	})
}
func (i *Identity) Revoke(device string) error {
	return i.transaction(func(r *Registry) error {
		d, ok := r.Devices[device]
		if !ok {
			return operations.ErrDenied
		}
		if e := i.audit("device_revoke", device, "administrator", "revoked"); e != nil {
			return e
		}
		d.Revoked = true
		r.Devices[device] = d
		for k, v := range r.Tickets {
			if v == device {
				delete(r.Tickets, k)
			}
		}
		return nil
	})
}
func (i *Identity) valid(ctx context.Context, p operations.Principal) bool {
	if time.Now().After(p.ExpiresAt) {
		return false
	}
	ok := false
	e := i.transaction(func(r *Registry) error {
		d, exists := r.Devices[p.Device]
		ok = exists && !d.Revoked && len(d.Credentials) > 0
		return nil
	})
	return e == nil && ok
}
func (i *Identity) OperationsIdentity() operations.Identity {
	return operations.Identity{WebAuthn: i.WebAuthn, Authenticate: func(r *http.Request) (operations.Principal, error) {
		h := r.Header.Get("Authorization")
		if len(h) < 8 || h[:7] != "Bearer " {
			return operations.Principal{}, operations.ErrDenied
		}
		i.mu.Lock()
		p, ok := i.sessions[operations.Hash([]byte(h[7:]))]
		i.mu.Unlock()
		if !ok || !i.valid(r.Context(), p) {
			return p, operations.ErrDenied
		}
		return p, nil
	}, AndroidKey: func(ctx context.Context, p operations.Principal) ([]byte, error) {
		if !i.Revalidate(ctx, p) {
			return nil, operations.ErrDenied
		}
		var key []byte
		e := i.transaction(func(r *Registry) error {
			key = r.Devices[p.Device].ActionKey
			if len(key) == 0 {
				return operations.ErrDenied
			}
			return nil
		})
		return key, e
	}, User: func(ctx context.Context, p operations.Principal) (webauthn.User, error) {
		if !i.valid(ctx, p) {
			return nil, operations.ErrDenied
		}
		var u User
		e := i.transaction(func(r *Registry) error {
			u = User{ID: p.Device, Credentials: r.Devices[p.Device].Credentials}
			return nil
		})
		return u, e
	}, UpdateCredential: func(ctx context.Context, p operations.Principal, c *webauthn.Credential) error {
		if !i.valid(ctx, p) {
			return operations.ErrDenied
		}
		return i.transaction(func(r *Registry) error {
			d := r.Devices[p.Device]
			for n, old := range d.Credentials {
				if bytes.Equal(old.ID, c.ID) {
					d.Credentials[n] = *c
					r.Devices[p.Device] = d
					return nil
				}
			}
			return operations.ErrDenied
		})
	}}
}
func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func rejected(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(403)
	reply(w, map[string]string{"code": "recovery_authentication_failed"})
}
func request(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	return r.Header.Get("Content-Type") == "application/json" && d.Decode(v) == nil && d.Decode(new(any)) == io.EOF
}
func (i *Identity) Routes(m *http.ServeMux) {
	m.HandleFunc("POST /v1/action-key", func(w http.ResponseWriter, r *http.Request) {
		p, e := i.OperationsIdentity().Authenticate(r)
		if e != nil || time.Until(p.ExpiresAt) < 25*time.Minute {
			rejected(w)
			return
		}
		var q struct {
			PublicKey string `json:"public_key"`
		}
		if !request(w, r, &q) {
			rejected(w)
			return
		}
		raw, e := base64.StdEncoding.DecodeString(q.PublicKey)
		if e != nil {
			rejected(w)
			return
		}
		parsed, e := x509.ParsePKIXPublicKey(raw)
		if e != nil {
			rejected(w)
			return
		}
		key, ok := parsed.(*ecdsa.PublicKey)
		if !ok || key.Curve != elliptic.P256() {
			rejected(w)
			return
		}
		e = i.transaction(func(reg *Registry) error {
			d := reg.Devices[p.Device]
			if d.Revoked {
				return operations.ErrDenied
			}
			if e := i.audit("action_key_enrolled", p.Device, p.Device, "verified"); e != nil {
				return e
			}
			d.ActionKey = raw
			reg.Devices[p.Device] = d
			return nil
		})
		if e != nil {
			rejected(w)
			return
		}
		reply(w, map[string]string{"status": "registered"})
	})
	m.HandleFunc("POST /v1/auth/browser", i.browser)
	m.HandleFunc("POST /v1/auth/begin", i.begin)
	m.HandleFunc("POST /v1/auth/finish", i.finish)
	m.HandleFunc("POST /v1/auth/poll", i.poll)
	m.HandleFunc("DELETE /v1/session", func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if len(h) > 7 {
			i.mu.Lock()
			delete(i.sessions, operations.Hash([]byte(h[7:])))
			i.mu.Unlock()
		}
		w.WriteHeader(204)
	})
	m.HandleFunc("GET /v1/recovery/audit", i.export)
}
func (i *Identity) browser(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Device string `json:"device_id"`
		Name   string `json:"device_name"`
	}
	if !request(w, r, &q) || len(q.Device) != 64 || len(q.Name) > 80 {
		rejected(w)
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	for id, f := range i.flows {
		if time.Now().After(f.Expires) {
			delete(i.flows, id)
		}
	}
	for id, s := range i.sessions {
		if time.Now().After(s.ExpiresAt) {
			delete(i.sessions, id)
		}
	}
	if len(i.flows) >= 50 {
		rejected(w)
		return
	}
	id, poll := operations.ID(), operations.ID()
	expires := time.Now().Add(3 * time.Minute)
	i.flows[id] = &flow{Device: q.Device, Name: q.Name, PollHash: operations.Hash([]byte(poll)), Expires: expires}
	reply(w, map[string]any{"flow_id": id, "poll_token": poll, "browser_url": i.WebAuthn.Config.RPOrigins[0] + "/auth#" + id, "expires_at": expires})
}
func (i *Identity) begin(w http.ResponseWriter, r *http.Request) {
	var q struct {
		ID            string `json:"flow_id"`
		Kind          string `json:"kind"`
		Authorization string `json:"authorization"`
	}
	if !request(w, r, &q) {
		rejected(w)
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	f := i.flows[q.ID]
	if f == nil || f.Approved || time.Now().After(f.Expires) || f.Session != nil {
		rejected(w)
		return
	}
	var options any
	var session *webauthn.SessionData
	e := i.transaction(func(reg *Registry) error {
		d, exists := reg.Devices[f.Device]
		if d.Revoked {
			return operations.ErrDenied
		}
		if q.Kind == "register" {
			hash := operations.Hash([]byte(q.Authorization))
			recovery := reg.Recovery[hash]
			if reg.Tickets[hash] != f.Device && !recovery {
				return operations.ErrDenied
			}
			if recovery {
				f.Recovery = true
				delete(reg.Recovery, hash)
				d = Device{Name: f.Name}
			} else {
				delete(reg.Tickets, hash)
				if !exists || len(d.Credentials) > 0 {
					return operations.ErrDenied
				}
			}
			if e := i.audit("enrollment_code", f.Device, "administrator", "consumed"); e != nil {
				return e
			}
			opts, data, e := i.WebAuthn.BeginRegistration(User{ID: f.Device, Credentials: d.Credentials})
			options, session = opts, data
			return e
		}
		if q.Kind != "login" || !exists || len(d.Credentials) == 0 {
			return operations.ErrDenied
		}
		opts, data, e := i.WebAuthn.BeginLogin(User{ID: f.Device, Credentials: d.Credentials})
		options, session = opts, data
		return e
	})
	if e != nil {
		rejected(w)
		return
	}
	f.Kind = q.Kind
	f.Session = session
	reply(w, map[string]any{"options": options, "ceremony_id": q.ID, "device_name": f.Name})
}
func (i *Identity) finish(w http.ResponseWriter, r *http.Request) {
	var q struct {
		ID         string          `json:"ceremony_id"`
		Credential json.RawMessage `json:"credential"`
	}
	if !request(w, r, &q) {
		rejected(w)
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	f := i.flows[q.ID]
	if f == nil || f.Approved || time.Now().After(f.Expires) || f.Session == nil {
		rejected(w)
		return
	}
	e := i.transaction(func(reg *Registry) error {
		d := reg.Devices[f.Device]
		if d.Revoked {
			return operations.ErrDenied
		}
		u := User{ID: f.Device, Credentials: d.Credentials}
		req, _ := http.NewRequestWithContext(r.Context(), "POST", "https://localhost", bytes.NewReader(q.Credential))
		var c *webauthn.Credential
		var e error
		if f.Kind == "login" {
			c, e = i.WebAuthn.FinishLogin(u, *f.Session, req)
		} else {
			c, e = i.WebAuthn.FinishRegistration(u, *f.Session, req)
		}
		if e != nil || c.Authenticator.CloneWarning || !c.Flags.UserVerified {
			return operations.ErrDenied
		}
		if e = i.audit("passkey_"+f.Kind, f.Device, "administrator", "verified"); e != nil {
			return e
		}
		if f.Recovery {
			for id, old := range reg.Devices {
				old.Revoked = true
				reg.Devices[id] = old
			}
			reg.Tickets = map[string]string{}
			reg.Recovery = map[string]bool{}
			d = Device{Name: f.Name}
			i.sessions = map[string]operations.Principal{}
		}
		if f.Kind == "login" {
			for n, old := range d.Credentials {
				if bytes.Equal(old.ID, c.ID) {
					d.Credentials[n] = *c
				}
			}
		} else {
			d.Credentials = []webauthn.Credential{*c}
			d.Name = f.Name
		}
		reg.Devices[f.Device] = d
		return nil
	})
	if e != nil {
		delete(i.flows, q.ID)
		rejected(w)
		return
	}
	f.Approved = true
	if f.Recovery {
		for id := range i.flows {
			if id != q.ID {
				delete(i.flows, id)
			}
		}
	}
	reply(w, map[string]string{"status": "approved"})
}
func (i *Identity) poll(w http.ResponseWriter, r *http.Request) {
	var q struct {
		ID    string `json:"flow_id"`
		Token string `json:"poll_token"`
	}
	if !request(w, r, &q) {
		rejected(w)
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	f := i.flows[q.ID]
	if f == nil || time.Now().After(f.Expires) || operations.Hash([]byte(q.Token)) != f.PollHash {
		rejected(w)
		return
	}
	if !f.Approved {
		reply(w, map[string]string{"status": "pending"})
		return
	}
	p := operations.Principal{Actor: "recovery-administrator", Device: f.Device, Method: "passkey", ExpiresAt: time.Now().Add(30 * time.Minute)}
	if !i.valid(r.Context(), p) {
		rejected(w)
		return
	}
	token := operations.ID()
	p.Session = operations.Hash([]byte(token))
	i.sessions[p.Session] = p
	delete(i.flows, q.ID)
	reply(w, map[string]any{"status": "approved", "access_token": token, "expires_at": p.ExpiresAt, "auth_mode": "passkey"})
}
func (i *Identity) PublicKey() string {
	return base64.StdEncoding.EncodeToString(i.key.Public().(ed25519.PublicKey))
}
func (i *Identity) Revalidate(ctx context.Context, p operations.Principal) bool {
	i.mu.Lock()
	_, ok := i.sessions[p.Session]
	i.mu.Unlock()
	return ok && i.valid(ctx, p)
}
