package operations

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/crypto/ssh"
)

type TerminalRequest struct {
	Columns  int    `json:"columns"`
	Rows     int    `json:"rows"`
	Incident string `json:"incident"`
	ID       string `json:"id"`
}
type frame struct {
	Seq  int64  `json:"seq"`
	Data string `json:"data"`
}
type Terminal struct {
	mu                                             sync.Mutex
	ID, Target, User, Environment, Owner, Incident string
	Started                                        time.Time
	LastInput                                      time.Time
	State                                          string
	client                                         *ssh.Client
	session                                        *ssh.Session
	guard                                          *RemoteGuard
	input                                          io.WriteCloser
	output                                         []frame
	seq, inputSeq                                  int64
	ticket                                         string
	ticketExpiry                                   time.Time
	attached                                       bool
	generation                                     uint64
	exited                                         chan struct{}
	stopOnce                                       sync.Once
}

func (t *Terminal) metadata() map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	return map[string]any{"id": t.ID, "target": t.Target, "user": t.User, "environment": t.Environment, "incident": t.Incident, "started_at": t.Started, "state": t.State, "input_seq": t.inputSeq}
}

type terminalWriter struct{ t *Terminal }

func (w terminalWriter) Write(b []byte) (int, error) {
	w.t.mu.Lock()
	defer w.t.mu.Unlock()
	n := len(b)
	for len(b) > 0 {
		size := min(len(b), 4096)
		w.t.seq++
		w.t.output = append(w.t.output, frame{Seq: w.t.seq, Data: base64.StdEncoding.EncodeToString(b[:size])})
		if len(w.t.output) > 64 {
			w.t.output = w.t.output[len(w.t.output)-64:]
		}
		b = b[size:]
	}
	return n, nil
}
func validSize(cols, rows int) bool { return cols >= 20 && cols <= 500 && rows >= 5 && rows <= 300 }
func (s *Service) openTerminal(ctx context.Context, p Principal, target Target, q TerminalRequest) (map[string]any, error) {
	if !target.Terminal || !validSize(q.Columns, q.Rows) || len(q.Incident) > 80 || q.Incident != "" && !identifier.MatchString(q.Incident) {
		return nil, ErrDenied
	}
	s.mu.Lock()
	if len(s.terminals) >= 8 {
		s.mu.Unlock()
		return nil, ErrDenied
	}
	s.mu.Unlock()
	c, e := Dial(ctx, target)
	if e != nil {
		return nil, e
	}
	ok := false
	defer func() {
		if !ok {
			_ = c.Close()
		}
	}()
	stop := context.AfterFunc(ctx, func() { _ = c.Close() })
	defer stop()
	g, e := Guard(c, target, s.Origin, p.Device)
	if e != nil {
		return nil, e
	}
	session, e := c.NewSession()
	if e != nil {
		_ = g.Close(true)
		return nil, ErrUnavailable
	}
	t := &Terminal{ID: ID(), Target: target.ID, User: target.User, Environment: target.Environment, Owner: p.Session, Incident: q.Incident, Started: time.Now().UTC(), LastInput: time.Now(), State: "connected", client: c, session: session, guard: g, exited: make(chan struct{})}
	t.input, e = session.StdinPipe()
	if e != nil {
		_ = session.Close()
		_ = g.Close(true)
		return nil, ErrUnavailable
	}
	session.Stdout = terminalWriter{t}
	session.Stderr = terminalWriter{t}
	if e = session.RequestPty("xterm-256color", q.Rows, q.Columns, ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}); e != nil {
		_ = session.Close()
		_ = g.Close(true)
		return nil, ErrUnavailable
	}
	if e = s.audit(ctx, p, "terminal_open", target.ID, t.ID, q.Incident, "started"); e != nil {
		_ = session.Close()
		_ = g.Close(true)
		return nil, e
	}
	if s.Revalidate != nil && !s.Revalidate(ctx, p) {
		_ = session.Close()
		_ = g.Close(true)
		return nil, ErrDenied
	}
	if e = session.Shell(); e != nil {
		_ = session.Close()
		_ = g.Close(false)
		return nil, ErrUnavailable
	}
	s.mu.Lock()
	if len(s.terminals) >= 8 {
		s.mu.Unlock()
		_ = session.Close()
		_ = g.Close(false)
		return nil, ErrDenied
	}
	s.terminals[t.ID] = t
	s.mu.Unlock()
	ok = true
	go func() {
		e := session.Wait()
		var exit *ssh.ExitError
		stopped := e == nil || errors.As(e, &exit)
		t.mu.Lock()
		t.State = "closed"
		if !stopped {
			t.State = "uncertain"
		}
		t.mu.Unlock()
		close(t.exited)
	}()
	go s.watchTerminal(t, p)
	return s.attach(p, t.ID, target.ID)
}
func (s *Service) attach(p Principal, id, target string) (map[string]any, error) {
	s.mu.Lock()
	t := s.terminals[id]
	s.mu.Unlock()
	if t == nil || t.Owner != p.Session || t.Target != target {
		return nil, ErrDenied
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.State == "closed" || t.State == "uncertain" || t.attached {
		return nil, ErrConflict
	}
	t.ticket = ID()
	t.ticketExpiry = time.Now().Add(30 * time.Second)
	return map[string]any{"id": id, "ticket": t.ticket, "input_seq": t.inputSeq, "state": t.State, "user": t.User, "environment": t.Environment, "started_at": t.Started}, nil
}
func (s *Service) watchTerminal(t *Terminal, p Principal) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	defer func() {
		t.stopOnce.Do(func() {
			_ = t.session.Close()
			select {
			case <-t.exited:
			case <-time.After(2 * time.Second):
			}
			t.mu.Lock()
			stopped := t.State == "closed"
			t.mu.Unlock()
			if t.guard.Close(stopped) != nil {
				stopped = false
			}
			_ = t.client.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			result := "uncertain"
			if stopped {
				result = "closed"
			}
			_ = s.audit(ctx, p, "terminal_close", t.Target, t.ID, t.Incident, result)
			s.mu.Lock()
			delete(s.terminals, t.ID)
			s.mu.Unlock()
			t.mu.Lock()
			t.State = result
			t.mu.Unlock()
			go func() { time.Sleep(5 * time.Second); t.mu.Lock(); t.output = nil; t.mu.Unlock() }()
		})
	}()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-t.exited:
			return
		case <-tick.C:
			t.mu.Lock()
			expired := time.Since(t.LastInput) > 10*time.Minute || time.Since(t.Started) > 30*time.Minute
			t.mu.Unlock()
			if expired || time.Now().After(p.ExpiresAt) {
				return
			}
			if s.Revalidate != nil && !s.Revalidate(s.ctx, p) {
				return
			}
			check := make(chan error, 1)
			go func() { check <- t.guard.Check() }()
			select {
			case err := <-check:
				if err != nil {
					return
				}
			case <-time.After(3 * time.Second):
				_ = t.client.Close()
				return
			case <-s.ctx.Done():
				return
			}
		}
	}
}

func (s *Service) stream(w http.ResponseWriter, r *http.Request, p Principal) {
	s.mu.Lock()
	t := s.terminals[r.PathValue("id")]
	s.mu.Unlock()
	if t == nil || t.Owner != p.Session {
		fail(w, ErrDenied)
		return
	}
	t.mu.Lock()
	ticket := r.Header.Get("X-Prisme-Terminal-Ticket")
	if t.ticket == "" || ticket != t.ticket || time.Now().After(t.ticketExpiry) || t.attached {
		t.mu.Unlock()
		fail(w, ErrDenied)
		return
	}
	t.ticket = ""
	t.attached = true
	t.generation++
	generation := t.generation
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		if t.generation == generation {
			t.attached = false
		}
		t.mu.Unlock()
	}()
	conn, e := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if e != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(32 * 1024)
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	go func() {
		defer cancel()
		for {
			_, raw, e := conn.Read(ctx)
			if e != nil {
				return
			}
			var msg struct {
				Type    string `json:"type"`
				Seq     int64  `json:"seq"`
				Data    string `json:"data"`
				Columns int    `json:"columns"`
				Rows    int    `json:"rows"`
			}
			if Decode(raw, &msg) != nil {
				return
			}
			if s.Revalidate != nil && !s.Revalidate(ctx, p) {
				return
			}
			t.mu.Lock()
			if t.generation != generation || t.State == "closed" || t.State == "uncertain" {
				t.mu.Unlock()
				return
			}
			switch msg.Type {
			case "input":
				b, e := base64.StdEncoding.DecodeString(msg.Data)
				if e != nil || len(b) > 8192 || msg.Seq != t.inputSeq+1 {
					t.mu.Unlock()
					return
				}
				t.inputSeq = msg.Seq
				t.LastInput = time.Now()
				t.mu.Unlock()
				_, e = t.input.Write(b)
				wipe(b)
				if e != nil {
					return
				}
			case "resize":
				t.mu.Unlock()
				if !validSize(msg.Columns, msg.Rows) || t.session.WindowChange(msg.Rows, msg.Columns) != nil {
					return
				}
			default:
				t.mu.Unlock()
				return
			}
		}
	}()
	last := int64(0)
	tick := time.NewTicker(40 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			t.mu.Lock()
			frames := append([]frame(nil), t.output...)
			state := t.State
			inputSeq := t.inputSeq
			t.mu.Unlock()
			fresh := []frame{}
			for _, f := range frames {
				if f.Seq > last {
					fresh = append(fresh, f)
				}
			}
			gap := len(fresh) > 0 && fresh[0].Seq > last+1
			if len(fresh) > 0 {
				last = fresh[len(fresh)-1].Seq
			}
			if len(fresh) == 0 && state == "connected" {
				continue
			}
			raw, _ := json.Marshal(map[string]any{"frames": fresh, "gap": gap, "state": state, "input_seq": inputSeq})
			writeCtx, done := context.WithTimeout(ctx, 3*time.Second)
			e = conn.Write(writeCtx, websocket.MessageText, raw)
			done()
			if e != nil || state == "closed" || state == "uncertain" {
				return
			}
		}
	}
}
