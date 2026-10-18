package operations

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

type Principal struct {
	Actor, Device, Session, Method string
	ExpiresAt                      time.Time
}
type Identity struct {
	Authenticate     func(*http.Request) (Principal, error)
	User             func(context.Context, Principal) (webauthn.User, error)
	UpdateCredential func(context.Context, Principal, *webauthn.Credential) error
	AndroidKey       func(context.Context, Principal) ([]byte, error)
	WebAuthn         *webauthn.WebAuthn
}
type Intent struct {
	Action  string         `json:"action"`
	Target  string         `json:"target"`
	Payload map[string]any `json:"payload"`
}
type proof struct {
	Principal                 Principal
	Intent                    Intent
	Digest, Method, Challenge string
	KeyHash                   string
	Expires                   time.Time
	Approved                  bool
	Options                   any
	Session                   *webauthn.SessionData
}
type Proofs struct {
	mu       sync.Mutex
	items    map[string]*proof
	Identity Identity
}

func (p *Proofs) Begin(ctx context.Context, actor Principal, intent Intent, method string) (map[string]any, error) {
	if actor.Method != "passkey" || (method != "passkey" && method != "android_key") || !identifier.MatchString(intent.Target) || !identifier.MatchString(intent.Action) {
		return nil, ErrDenied
	}
	item := &proof{Principal: actor, Intent: Intent{Action: intent.Action, Target: intent.Target}, Digest: Binding(intent.Action, intent.Target, intent.Payload), Method: method, Expires: time.Now().Add(2 * time.Minute)}
	id := ID()
	item.Challenge = "prisme-operation-v1\n" + id + "\n" + actor.Session + "\n" + actor.Device + "\n" + item.Digest
	browser := ""
	if method == "passkey" {
		if p.Identity.WebAuthn == nil {
			return nil, ErrUnavailable
		}
		u, e := p.Identity.User(ctx, actor)
		if e != nil {
			return nil, ErrDenied
		}
		opts, session, e := p.Identity.WebAuthn.BeginLogin(u)
		if e != nil {
			return nil, ErrDenied
		}
		item.Options = opts
		item.Session = session
		browser = p.Identity.WebAuthn.Config.RPOrigins[0] + "/v1/operations/verify#" + id
	} else {
		if p.Identity.AndroidKey == nil {
			return nil, ErrDenied
		}
		key, e := p.Identity.AndroidKey(ctx, actor)
		if e != nil {
			return nil, ErrDenied
		}
		item.KeyHash = Hash(key)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.items == nil {
		p.items = map[string]*proof{}
	}
	for k, v := range p.items {
		if time.Now().After(v.Expires) || v.Principal.Session == actor.Session {
			delete(p.items, k)
		}
	}
	if len(p.items) >= 100 {
		return nil, ErrUnavailable
	}
	p.items[id] = item
	return map[string]any{"id": id, "challenge": item.Challenge, "browser_url": browser, "expires_at": item.Expires}, nil
}
func (p *Proofs) Options(id string) (map[string]any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	v := p.items[id]
	if v == nil || time.Now().After(v.Expires) || v.Approved || v.Method != "passkey" {
		return nil, ErrDenied
	}
	return map[string]any{"options": v.Options, "action": v.Intent.Action, "target": v.Intent.Target}, nil
}
func (p *Proofs) Passkey(ctx context.Context, id string, raw []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	v := p.items[id]
	if v == nil || time.Now().After(v.Expires) || v.Approved || v.Method != "passkey" {
		return ErrDenied
	}
	u, e := p.Identity.User(ctx, v.Principal)
	if e != nil {
		return ErrDenied
	}
	r, _ := http.NewRequestWithContext(ctx, "POST", "https://localhost", bytes.NewReader(raw))
	c, e := p.Identity.WebAuthn.FinishLogin(u, *v.Session, r)
	if e != nil || c.Authenticator.CloneWarning || !c.Flags.UserVerified {
		return ErrDenied
	}
	if p.Identity.UpdateCredential(ctx, v.Principal, c) != nil {
		return ErrDenied
	}
	v.Approved = true
	return nil
}
func (p *Proofs) Sign(ctx context.Context, actor Principal, id, signature string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	v := p.items[id]
	if v == nil || time.Now().After(v.Expires) || v.Approved || v.Method != "android_key" || v.Principal.Session != actor.Session {
		return ErrDenied
	}
	raw, e := p.Identity.AndroidKey(ctx, actor)
	if e != nil || Hash(raw) != v.KeyHash {
		return ErrDenied
	}
	parsed, e := x509.ParsePKIXPublicKey(raw)
	if e != nil {
		return ErrDenied
	}
	key, ok := parsed.(*ecdsa.PublicKey)
	if !ok || key.Curve != elliptic.P256() {
		return ErrDenied
	}
	sig, e := base64.StdEncoding.DecodeString(signature)
	d := sha256.Sum256([]byte(v.Challenge))
	if e != nil || len(sig) > 128 || !ecdsa.VerifyASN1(key, d[:], sig) {
		return ErrDenied
	}
	v.Approved = true
	return nil
}
func (p *Proofs) Status(actor Principal, id string) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	v := p.items[id]
	if v == nil || v.Principal.Session != actor.Session || time.Now().After(v.Expires) {
		return false, ErrDenied
	}
	return v.Approved, nil
}
func (p *Proofs) Consume(actor Principal, id string, intent Intent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	v := p.items[id]
	if v == nil || v.Principal.Session != actor.Session || !v.Approved || time.Now().After(v.Expires) || v.Digest != Binding(intent.Action, intent.Target, intent.Payload) {
		return ErrDenied
	}
	if v.Method == "android_key" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		raw, e := p.Identity.AndroidKey(ctx, actor)
		if e != nil || Hash(raw) != v.KeyHash {
			return ErrDenied
		}
	}
	delete(p.items, id)
	return nil
}
func payload[T any](m map[string]any) (T, error) {
	var out T
	b, e := json.Marshal(m)
	if e == nil {
		e = Decode(b, &out)
	}
	return out, e
}
