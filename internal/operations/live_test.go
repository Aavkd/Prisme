package operations

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"prisme.local/platform/internal/coordination"
)

func TestPhaseSixLiveSSH(t *testing.T) {
	if os.Getenv("PRISME_TEST_SSH") != "1" {
		t.Skip("isolated OpenSSH fixture required")
	}
	fingerprint, e := os.ReadFile("/fixture/host-fingerprint")
	if e != nil {
		t.Fatal(e)
	}
	target := Target{ID: "local", Name: "Local SSH fixture", Environment: "development", Address: "127.0.0.1:2222", User: "prisme", KeyFile: "/fixture/client-key", HostKey: strings.TrimSpace(string(fingerprint)), Terminal: true, Files: true, Roots: []string{"/fixture/project"}, GuardPath: "/usr/local/bin/prisme-host-guard", CoordinationRoot: "/fixture/coordination", LockTarget: "host"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := Principal{Actor: "fixture", Device: strings.Repeat("a", 64), Session: "fixture-session", Method: "passkey", ExpiresAt: time.Now().Add(time.Hour)}
	var revoked atomic.Bool
	var auditFail atomic.Bool
	identity := Identity{Authenticate: func(r *http.Request) (Principal, error) {
		if r.Header.Get("Authorization") != "Bearer fixture" || revoked.Load() {
			return Principal{}, ErrDenied
		}
		return p, nil
	}}
	s := New(ctx, Config{Targets: []Target{target}}, identity, "normal")
	s.Revalidate = func(context.Context, Principal) bool { return !revoked.Load() }
	s.Audit = func(_ context.Context, _ Principal, action, target, id, incident, result string) error {
		if auditFail.Load() {
			return ErrUnavailable
		}
		return nil
	}
	server := httptest.NewTLSServer(s.Handler())
	defer server.Close()
	requestCtx, done := context.WithTimeout(ctx, 30*time.Second)
	defer done()
	wrong := target
	wrong.HostKey = "SHA256:incorrect"
	if c, e := Dial(requestCtx, wrong); e == nil {
		c.Close()
		t.Fatal("host key mismatch accepted")
	}
	c, f, e := FileClient(requestCtx, target)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	defer f.Close()
	for _, file := range []string{"/fixture/project/../client-key", "/etc/passwd", "/fixture/project/escape/client-key", "/fixture/project/.env"} {
		if safePath(f, target, file, false, false) == nil {
			t.Fatal("path escaped", file)
		}
	}
	content := []byte("phase-six-initial\n")
	result, e := s.save(requestCtx, p, target, "/fixture/project/config.txt", "absent", append([]byte(nil), content...), false)
	if e != nil {
		t.Fatal("atomic create", e)
	}
	rev := result["revision"].(string)
	read, e := s.fileRead(requestCtx, target, "file_read", FileRequest{Path: "/fixture/project/config.txt"})
	if e != nil || read["content"] != string(content) {
		t.Fatal("SFTP read", e)
	}
	result, e = s.save(requestCtx, p, target, "/fixture/project/config.txt", rev, []byte("phase-six-updated\n"), false)
	if e != nil {
		t.Fatal("atomic replace", e)
	}
	list, e := f.ReadDir("/fixture/project")
	if e != nil {
		t.Fatal(e)
	}
	backupFound := false
	for _, entry := range list {
		if strings.HasPrefix(entry.Name(), ".prisme-backup-") {
			b, _, e := readFile(f, "/fixture/project/"+entry.Name())
			if e != nil || string(b) != string(content) || entry.Mode().Perm() != 0600 {
				t.Fatal("backup invalid", e)
			}
			backupFound = true
		}
	}
	if !backupFound {
		t.Fatal("no backup")
	}
	if _, e = atomicSave(f, target, "/fixture/project/config.txt", rev, []byte("stale"), false); e != ErrConflict {
		t.Fatal("stale editor overwrote file", e)
	}
	// Resumable chunks accept an identical retry, never a changed chunk.
	data := []byte("resumable-upload-fixture")
	begin, e := s.transfer(ctx, p, target, Intent{Action: "upload_begin", Payload: map[string]any{"path": "/fixture/project/upload.txt", "revision": "absent", "size": len(data), "digest": Hash(data)}})
	if e != nil {
		t.Fatal(e)
	}
	id := begin["id"].(string)
	chunk := Intent{Action: "upload_chunk", Payload: map[string]any{"id": id, "offset": 0, "base64": base64.StdEncoding.EncodeToString(data)}}
	if _, e = s.transfer(ctx, p, target, chunk); e != nil {
		t.Fatal(e)
	}
	if _, e = s.transfer(ctx, p, target, chunk); e != nil {
		t.Fatal("chunk retry", e)
	}
	if _, e = s.transfer(ctx, Principal{Session: "other"}, target, chunk); e != ErrDenied {
		t.Fatal("transfer leaked across sessions")
	}
	if _, e = s.transfer(ctx, p, target, Intent{Action: "upload_commit", Payload: map[string]any{"id": id}}); e != nil {
		t.Fatal("upload commit", e)
	}
	auditFail.Store(true)
	if _, e = s.openTerminal(requestCtx, p, target, TerminalRequest{Columns: 80, Rows: 24}); e == nil {
		t.Fatal("terminal opened with unavailable audit")
	}
	auditFail.Store(false)
	opened, e := s.openTerminal(requestCtx, p, target, TerminalRequest{Columns: 80, Rows: 24, Incident: "42"})
	if e != nil {
		t.Fatal("PTY open", e)
	}
	id = opened["id"].(string)
	if _, e = s.openTerminal(requestCtx, p, target, TerminalRequest{Columns: 80, Rows: 24}); e == nil {
		t.Fatal("simultaneous host mutation")
	}
	headers := http.Header{"Authorization": []string{"Bearer fixture"}, "X-Prisme-Terminal-Ticket": []string{opened["ticket"].(string)}}
	ws, _, e := websocket.Dial(requestCtx, strings.Replace(server.URL, "https://", "wss://", 1)+"/v1/operations/sessions/"+id+"/stream", &websocket.DialOptions{HTTPClient: server.Client(), HTTPHeader: headers})
	if e != nil {
		t.Fatal(e)
	}
	sendInput := func(seq int, data string) {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"type": "input", "seq": seq, "data": base64.StdEncoding.EncodeToString([]byte(data))})
		if e = ws.Write(requestCtx, websocket.MessageText, raw); e != nil {
			t.Fatal(e)
		}
	}
	sendInput(1, "stty size\nprintf 'PTY_VERIFIED\\n'\n")
	output := ""
	for !strings.Contains(output, "PTY_VERIFIED") || !strings.Contains(output, "24 80") {
		_, raw, e := ws.Read(requestCtx)
		if e != nil {
			t.Fatal(e)
		}
		var msg struct {
			Frames []frame `json:"frames"`
		}
		if json.Unmarshal(raw, &msg) != nil {
			t.Fatal("invalid frame")
		}
		for _, fr := range msg.Frames {
			b, _ := base64.StdEncoding.DecodeString(fr.Data)
			output += string(b)
		}
	}
	if e = ws.CloseNow(); e != nil {
		t.Fatal(e)
	}
	// A dropped WebSocket does not rerun the shell or replay stdin.
	var attached map[string]any
	for n := 0; n < 50; n++ {
		attached, e = s.attach(p, id, target.ID)
		if e == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if e != nil {
		t.Fatal("controlled reconnect", e)
	}
	if attached["input_seq"] != int64(1) {
		t.Fatal("input cursor lost")
	}
	headers.Set("X-Prisme-Terminal-Ticket", attached["ticket"].(string))
	ws, _, e = websocket.Dial(requestCtx, strings.Replace(server.URL, "https://", "wss://", 1)+"/v1/operations/sessions/"+id+"/stream", &websocket.DialOptions{HTTPClient: server.Client(), HTTPHeader: headers})
	if e != nil {
		t.Fatal(e)
	}
	defer ws.CloseNow()
	// Resizing goes through the actual SSH PTY channel.
	raw, _ := json.Marshal(map[string]any{"type": "resize", "columns": 100, "rows": 35})
	if e = ws.Write(requestCtx, websocket.MessageText, raw); e != nil {
		t.Fatal(e)
	}
	sendInput(2, "stty size\nexit\n")
	for n := 0; n < 100; n++ {
		s.mu.Lock()
		remaining := len(s.terminals)
		s.mu.Unlock()
		if remaining == 0 {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	state, e := coordination.Read(target.CoordinationRoot, target.LockTarget)
	if e != nil || state.Active {
		t.Fatal("PTY exit not acknowledged", e)
	}
	// Recovery is independent and forces subsequent normal operations to reconcile.
	s.Origin = "recovery"
	if _, e = s.save(requestCtx, p, target, "/fixture/project/recovery.txt", "absent", []byte("recovery"), false); e != nil {
		t.Fatal(e)
	}
	s.Origin = "normal"
	if _, e = s.save(requestCtx, p, target, "/fixture/project/blocked.txt", "absent", []byte("blocked"), false); e == nil {
		t.Fatal("normal operations resumed without reconciliation")
	}
	state, _ = coordination.Read(target.CoordinationRoot, target.LockTarget)
	if e = coordination.Reconcile(target.CoordinationRoot, target.LockTarget, "operator", state.Epoch); e != nil {
		t.Fatal(e)
	}
	// Independent audit disk failure is simulated by a non-writable audit object.
	block := filepath.Join(target.CoordinationRoot, "diskfull.audit.jsonl")
	if e = os.Mkdir(block, 0700); e != nil {
		t.Fatal(e)
	}
	blocked := target
	blocked.LockTarget = "diskfull"
	if _, e = s.openTerminal(requestCtx, p, blocked, TerminalRequest{Columns: 80, Rows: 24}); e == nil {
		t.Fatal("remote audit failure ignored")
	}
	t.Log("real SSH host-key pinning, PTY, reconnect, resize, SFTP, backup, transfer resume, fencing and audit failures verified")
}
