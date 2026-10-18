package operations

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"prisme.local/platform/internal/coordination"
)

type AuditFunc func(context.Context, Principal, string, string, string, string, string) error

// AuthorizeIntent lets another module consume the same fresh, request-bound
// passkey/Android proof without granting access to its execution dispatcher.
func (s *Service) AuthorizeIntent(r *http.Request, proof string, intent Intent) error {
	p, err := s.Identity.Authenticate(r)
	if err != nil || s.Revalidate == nil || !s.Revalidate(r.Context(), p) {
		return ErrDenied
	}
	return s.proofs.Consume(p, proof, intent)
}

type Service struct {
	Config     Config
	Origin     string
	Identity   Identity
	Vault      *Vault
	Audit      AuditFunc
	Revalidate func(context.Context, Principal) bool
	mu         sync.Mutex
	terminals  map[string]*Terminal
	uploads    map[string]*upload
	proofs     Proofs
	ctx        context.Context
}
type upload struct {
	Download                              bool
	Owner, Target, Path, Revision, Digest string
	Size                                  int
	Data                                  []byte
	Expires                               time.Time
}

func New(ctx context.Context, c Config, identity Identity, origin string) *Service {
	s := &Service{ctx: ctx, Config: c, Identity: identity, Origin: origin, terminals: map[string]*Terminal{}, uploads: map[string]*upload{}, proofs: Proofs{Identity: identity}}
	go func() {
		tick := time.NewTicker(time.Minute)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				s.mu.Lock()
				for id, u := range s.uploads {
					wipe(u.Data)
					delete(s.uploads, id)
				}
				s.mu.Unlock()
				return
			case <-tick.C:
				s.mu.Lock()
				for id, u := range s.uploads {
					if time.Now().After(u.Expires) {
						wipe(u.Data)
						delete(s.uploads, id)
					}
				}
				s.mu.Unlock()
			}
		}
	}()
	return s
}
func (s *Service) audit(ctx context.Context, p Principal, action, target, id, incident, result string) error {
	if s.Audit == nil {
		return ErrUnavailable
	}
	if s.Audit(ctx, p, action, target, id, incident, result) != nil {
		return ErrUnavailable
	}
	return nil
}
func send(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, e error) {
	code, status := "operation_unavailable", 503
	switch {
	case errors.Is(e, ErrInvalid):
		code, status = "invalid_operation", 400
	case errors.Is(e, ErrDenied):
		code, status = "operation_denied", 403
	case errors.Is(e, ErrConflict):
		code, status = "revision_conflict", 409
	case errors.Is(e, coordination.ErrBlocked):
		code, status = "host_reconciliation_required", 409
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code})
}
func read(w http.ResponseWriter, r *http.Request, out any) error {
	if r.Header.Get("Content-Type") != "application/json" {
		return ErrInvalid
	}
	b, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 2*MaxFile))
	if e != nil {
		return ErrInvalid
	}
	defer wipe(b)
	return Decode(b, out)
}
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/operations/verify", verifyPage)
	mux.HandleFunc("GET /v1/operations/verify.js", verifyScript)
	mux.HandleFunc("GET /v1/operations/challenges/{id}/options", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.proofs.Options(r.PathValue("id"))
		if e != nil {
			fail(w, e)
			return
		}
		send(w, v)
	})
	mux.HandleFunc("POST /v1/operations/challenges/{id}/passkey", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Credential json.RawMessage `json:"credential"`
		}
		if read(w, r, &q) != nil {
			fail(w, ErrInvalid)
			return
		}
		if e := s.proofs.Passkey(r.Context(), r.PathValue("id"), q.Credential); e != nil {
			fail(w, e)
			return
		}
		send(w, map[string]bool{"approved": true})
	})
	auth := func(fn func(http.ResponseWriter, *http.Request, Principal)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			p, e := s.Identity.Authenticate(r)
			if e != nil {
				w.WriteHeader(401)
				return
			}
			fn(w, r, p)
		}
	}
	mux.HandleFunc("GET /v1/operations/overview", auth(s.overview))
	mux.HandleFunc("POST /v1/operations/challenges", auth(func(w http.ResponseWriter, r *http.Request, p Principal) {
		var q struct {
			Intent Intent `json:"intent"`
			Method string `json:"method"`
		}
		if read(w, r, &q) != nil {
			fail(w, ErrInvalid)
			return
		}
		v, e := s.proofs.Begin(r.Context(), p, q.Intent, q.Method)
		if e != nil {
			fail(w, e)
			return
		}
		send(w, v)
	}))
	mux.HandleFunc("GET /v1/operations/challenges/{id}", auth(func(w http.ResponseWriter, r *http.Request, p Principal) {
		v, e := s.proofs.Status(p, r.PathValue("id"))
		if e != nil {
			fail(w, e)
			return
		}
		send(w, map[string]bool{"approved": v})
	}))
	mux.HandleFunc("POST /v1/operations/challenges/{id}/sign", auth(func(w http.ResponseWriter, r *http.Request, p Principal) {
		var q struct {
			Signature string `json:"signature"`
		}
		if read(w, r, &q) != nil {
			fail(w, ErrInvalid)
			return
		}
		if e := s.proofs.Sign(r.Context(), p, r.PathValue("id"), q.Signature); e != nil {
			fail(w, e)
			return
		}
		send(w, map[string]bool{"approved": true})
	}))
	mux.HandleFunc("POST /v1/operations/execute", auth(func(w http.ResponseWriter, r *http.Request, p Principal) {
		var q struct {
			Intent Intent `json:"intent"`
			Proof  string `json:"proof"`
		}
		if read(w, r, &q) != nil {
			fail(w, ErrInvalid)
			return
		}
		if !withoutProof(q.Intent.Action) {
			if e := s.proofs.Consume(p, q.Proof, q.Intent); e != nil {
				fail(w, e)
				return
			}
		}
		v, e := s.execute(r.Context(), p, q.Intent)
		if e != nil {
			fail(w, e)
			return
		}
		send(w, v)
	}))
	mux.HandleFunc("GET /v1/operations/sessions/{id}/stream", auth(s.stream))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		mux.ServeHTTP(w, r)
	})
}
func withoutProof(a string) bool {
	switch a {
	case "file_list", "file_stat", "upload_chunk", "upload_status", "upload_cancel", "download_chunk", "terminal_close", "diagnostics":
		return true
	}
	return false
}
func (s *Service) overview(w http.ResponseWriter, r *http.Request, p Principal) {
	targets := []map[string]any{}
	for _, t := range s.Config.Targets {
		targets = append(targets, map[string]any{"id": t.ID, "name": t.Name, "user": t.User, "environment": t.Environment, "terminal": t.Terminal, "files": t.Files, "diagnostics": t.Diagnostics, "containers": t.Containers, "roots": t.Roots})
	}
	sessions := []map[string]any{}
	s.mu.Lock()
	for _, t := range s.terminals {
		if t.Owner == p.Session {
			sessions = append(sessions, t.metadata())
		}
	}
	s.mu.Unlock()
	secrets := []Secret{}
	if s.Vault != nil {
		var e error
		secrets, e = s.Vault.List(r.Context())
		if e != nil {
			fail(w, e)
			return
		}
	}
	send(w, map[string]any{"origin": s.Origin, "observed_at": time.Now().UTC(), "targets": targets, "sessions": sessions, "secrets": secrets, "vault": s.Vault != nil})
}
func (s *Service) target(id string) (Target, error) {
	for _, t := range s.Config.Targets {
		if t.ID == id {
			return t, nil
		}
	}
	return Target{}, ErrDenied
}
func (s *Service) execute(ctx context.Context, p Principal, i Intent) (map[string]any, error) {
	if i.Action == "secret_put" {
		if s.Vault == nil {
			return nil, ErrDenied
		}
		q, e := payload[struct {
			Secret Secret `json:"secret"`
			Value  string `json:"value"`
		}](i.Payload)
		if e != nil {
			return nil, e
		}
		if i.Target != q.Secret.Destination {
			return nil, ErrDenied
		}
		if _, e = s.target(q.Secret.Destination); e != nil {
			return nil, e
		}
		secret, e := s.Vault.Put(ctx, p, q.Secret, []byte(q.Value))
		return map[string]any{"secret": secret}, e
	}
	if i.Action == "secret_reveal" {
		if s.Vault == nil {
			return nil, ErrDenied
		}
		q, e := payload[struct {
			ID       string `json:"id"`
			Revision int    `json:"revision"`
		}](i.Payload)
		if e != nil {
			return nil, e
		}
		plain, e := s.Vault.Reveal(ctx, p, q.ID, i.Target, q.Revision)
		if e != nil {
			return nil, e
		}
		defer wipe(plain)
		return map[string]any{"value": string(plain), "hide_after_seconds": 30}, nil
	}
	t, e := s.target(i.Target)
	if e != nil {
		return nil, e
	}
	switch i.Action {
	case "diagnostics":
		q, e := payload[struct {
			Container string `json:"container"`
		}](i.Payload)
		if e != nil {
			return nil, e
		}
		return s.diagnostics(ctx, p, t, q.Container)
	case "terminal_open":
		q, e := payload[TerminalRequest](i.Payload)
		if e != nil {
			return nil, e
		}
		return s.openTerminal(ctx, p, t, q)
	case "terminal_attach":
		q, e := payload[TerminalRequest](i.Payload)
		if e != nil {
			return nil, e
		}
		return s.attach(p, q.ID, t.ID)
	case "terminal_close":
		q, e := payload[TerminalRequest](i.Payload)
		if e != nil {
			return nil, e
		}
		s.mu.Lock()
		terminal := s.terminals[q.ID]
		s.mu.Unlock()
		if terminal == nil || terminal.Owner != p.Session || terminal.Target != i.Target {
			return nil, ErrDenied
		}
		_ = terminal.session.Signal(sshSignalTerm)
		go func() {
			select {
			case <-terminal.exited:
			case <-time.After(2 * time.Second):
				_ = terminal.session.Close()
			}
		}()
		return map[string]any{"state": "close_requested"}, nil
	case "file_list", "file_read", "file_download", "file_stat":
		q, e := payload[FileRequest](i.Payload)
		if e != nil {
			return nil, e
		}
		if e = s.audit(ctx, p, i.Action, t.ID, "", "", "requested"); e != nil {
			return nil, e
		}
		return s.fileRead(ctx, t, i.Action, q)
	case "file_save":
		q, e := payload[FileRequest](i.Payload)
		if e != nil {
			return nil, e
		}
		return s.save(ctx, p, t, q.Path, q.Revision, []byte(q.Content), false)
	case "upload_begin", "upload_chunk", "upload_status", "upload_cancel", "upload_commit", "download_begin", "download_chunk":
		return s.transfer(ctx, p, t, i)
	case "env_apply":
		return s.applyEnv(ctx, p, t, i.Payload)
	default:
		return nil, ErrInvalid
	}
}

const sshSignalTerm = "TERM"

func (s *Service) save(ctx context.Context, p Principal, t Target, file, rev string, data []byte, env bool) (map[string]any, error) {
	defer wipe(data)
	if !t.Files {
		return nil, ErrDenied
	}
	c, f, e := FileClient(ctx, t)
	if e != nil {
		return nil, e
	}
	defer c.Close()
	defer f.Close()
	stop := context.AfterFunc(ctx, func() { _ = c.Close() })
	defer stop()
	g, e := Guard(c, t, s.Origin, p.Device)
	if e != nil {
		return nil, e
	}
	stopped := false
	defer func() { _ = g.Close(stopped) }()
	if e = s.audit(ctx, p, "file_save", t.ID, "", "", "started"); e != nil {
		return nil, e
	}
	if e = g.Check(); e != nil {
		return nil, e
	}
	if s.Revalidate != nil && !s.Revalidate(ctx, p) {
		stopped = true
		return nil, ErrDenied
	}
	newRev, e := atomicSave(f, t, file, rev, data, env)
	if e != nil {
		if errors.Is(e, ErrConflict) || errors.Is(e, ErrDenied) || errors.Is(e, ErrInvalid) {
			stopped = true
		}
		return nil, e
	}
	stopped = true
	if e = s.audit(ctx, p, "file_save", t.ID, "", "", "succeeded"); e != nil {
		return nil, e
	}
	return map[string]any{"revision": newRev, "state": "saved"}, nil
}
func (s *Service) transfer(ctx context.Context, p Principal, t Target, i Intent) (map[string]any, error) {
	if !t.Files {
		return nil, ErrDenied
	}
	q, e := payload[struct {
		ID       string `json:"id"`
		Path     string `json:"path"`
		Revision string `json:"revision"`
		Digest   string `json:"digest"`
		Size     int    `json:"size"`
		Offset   int    `json:"offset"`
		Base64   string `json:"base64"`
	}](i.Payload)
	if e != nil {
		return nil, e
	}
	if i.Action == "download_begin" {
		c, f, e := FileClient(ctx, t)
		if e != nil {
			return nil, e
		}
		defer c.Close()
		defer f.Close()
		stop := context.AfterFunc(ctx, func() { _ = c.Close() })
		defer stop()
		if e = safePath(f, t, q.Path, false, false); e != nil {
			return nil, e
		}
		data, _, e := readFile(f, q.Path)
		if e != nil {
			return nil, e
		}
		if e = s.audit(ctx, p, "file_download", t.ID, "", "", "requested"); e != nil {
			wipe(data)
			return nil, e
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if len(s.uploads) >= 8 {
			wipe(data)
			return nil, ErrDenied
		}
		id := ID()
		s.uploads[id] = &upload{Download: true, Owner: p.Session, Target: t.ID, Path: q.Path, Digest: Hash(data), Data: data, Size: len(data), Expires: time.Now().Add(10 * time.Minute)}
		return map[string]any{"id": id, "size": len(data), "revision": Hash(data)}, nil
	}
	s.mu.Lock()
	for id, u := range s.uploads {
		if time.Now().After(u.Expires) {
			wipe(u.Data)
			delete(s.uploads, id)
		}
	}
	if i.Action == "upload_begin" {
		defer s.mu.Unlock()
		if len(s.uploads) >= 8 || q.Size < 1 || q.Size > MaxFile || !digestPattern.MatchString(q.Digest) || q.Revision == "" {
			return nil, ErrInvalid
		}
		id := ID()
		s.uploads[id] = &upload{Owner: p.Session, Target: t.ID, Path: q.Path, Revision: q.Revision, Digest: q.Digest, Size: q.Size, Expires: time.Now().Add(10 * time.Minute)}
		return map[string]any{"id": id, "offset": 0}, nil
	}
	u := s.uploads[q.ID]
	if u == nil || u.Owner != p.Session || u.Target != t.ID {
		s.mu.Unlock()
		return nil, ErrDenied
	}
	if i.Action == "download_chunk" {
		defer s.mu.Unlock()
		if !u.Download || q.Offset < 0 || q.Offset > len(u.Data) {
			return nil, ErrDenied
		}
		end := min(q.Offset+ChunkSize, len(u.Data))
		return map[string]any{"base64": base64.StdEncoding.EncodeToString(u.Data[q.Offset:end]), "offset": end, "size": u.Size, "revision": u.Digest}, nil
	}
	if u.Download && i.Action != "upload_cancel" && i.Action != "upload_status" {
		s.mu.Unlock()
		return nil, ErrDenied
	}
	switch i.Action {
	case "upload_cancel":
		wipe(u.Data)
		delete(s.uploads, q.ID)
		s.mu.Unlock()
		return map[string]any{"state": "cancelled"}, nil
	case "upload_status":
		n := len(u.Data)
		s.mu.Unlock()
		return map[string]any{"id": q.ID, "offset": n, "size": u.Size}, nil
	case "upload_chunk":
		defer s.mu.Unlock()
		data, e := base64.StdEncoding.DecodeString(q.Base64)
		if e != nil || len(data) > ChunkSize || len(data) == 0 || q.Offset < 0 || q.Offset+len(data) > u.Size {
			return nil, ErrInvalid
		}
		defer wipe(data)
		if q.Offset == len(u.Data) {
			u.Data = append(u.Data, data...)
		} else if q.Offset+len(data) > len(u.Data) || Hash(u.Data[q.Offset:q.Offset+len(data)]) != Hash(data) {
			return nil, ErrConflict
		}
		return map[string]any{"id": q.ID, "offset": len(u.Data)}, nil
	case "upload_commit":
		if len(u.Data) != u.Size || Hash(u.Data) != u.Digest {
			s.mu.Unlock()
			return nil, ErrConflict
		}
		delete(s.uploads, q.ID)
		s.mu.Unlock()
		return s.save(ctx, p, t, u.Path, u.Revision, u.Data, false)
	}
	s.mu.Unlock()
	return nil, ErrInvalid
}
func (s *Service) applyEnv(ctx context.Context, p Principal, t Target, m map[string]any) (map[string]any, error) {
	if s.Vault == nil {
		return nil, ErrDenied
	}
	q, e := payload[struct {
		Path      string            `json:"path"`
		Revision  string            `json:"revision"`
		Variables map[string]string `json:"variables"`
		Secrets   map[string]struct {
			ID       string `json:"id"`
			Revision int    `json:"revision"`
		} `json:"secrets"`
	}](m)
	if e != nil {
		return nil, e
	}
	if name := path.Base(q.Path); name != ".env" && !strings.HasPrefix(name, ".env.") {
		return nil, ErrDenied
	}
	if q.Variables == nil {
		q.Variables = map[string]string{}
	}
	for name, ref := range q.Secrets {
		if _, ok := q.Variables[name]; ok {
			return nil, ErrInvalid
		}
		plain, e := s.Vault.Reveal(ctx, p, ref.ID, t.ID, ref.Revision)
		if e != nil {
			return nil, e
		}
		q.Variables[name] = string(plain)
		wipe(plain)
	}
	data, e := Env(q.Variables)
	if e != nil {
		return nil, e
	}
	return s.save(ctx, p, t, q.Path, q.Revision, data, true)
}
