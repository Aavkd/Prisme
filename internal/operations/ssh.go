package operations

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"prisme.local/platform/internal/coordination"
)

func Dial(ctx context.Context, t Target) (*ssh.Client, error) {
	raw, e := os.ReadFile(t.KeyFile)
	if e != nil {
		return nil, ErrUnavailable
	}
	signer, e := ssh.ParsePrivateKey(raw)
	wipe(raw)
	if e != nil {
		return nil, ErrUnavailable
	}
	cfg := &ssh.ClientConfig{User: t.User, Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)}, HostKeyCallback: func(_ string, _ net.Addr, k ssh.PublicKey) error {
		if ssh.FingerprintSHA256(k) != t.HostKey {
			return ErrDenied
		}
		return nil
	}, Timeout: 5 * time.Second}
	conn, e := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", t.Address)
	if e != nil {
		return nil, ErrUnavailable
	}
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	c, channels, requests, e := ssh.NewClientConn(conn, t.Address, cfg)
	if e != nil {
		_ = conn.Close()
		return nil, ErrUnavailable
	}
	_ = conn.SetDeadline(time.Time{})
	return ssh.NewClient(c, channels, requests), nil
}

type RemoteGuard struct {
	session *ssh.Session
	in      io.WriteCloser
	scan    *bufio.Scanner
	Epoch   uint64
}

func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
func Guard(client *ssh.Client, t Target, origin, actor string) (*RemoteGuard, error) {
	s, e := client.NewSession()
	if e != nil {
		return nil, ErrUnavailable
	}
	in, e := s.StdinPipe()
	if e != nil {
		_ = s.Close()
		return nil, ErrUnavailable
	}
	out, e := s.StdoutPipe()
	if e != nil {
		_ = s.Close()
		return nil, ErrUnavailable
	}
	command := quote(t.GuardPath) + " -root " + quote(t.CoordinationRoot) + " -target " + quote(t.LockTarget) + " -origin " + quote(origin) + " -actor " + quote(actor)
	if e = s.Start(command); e != nil {
		_ = s.Close()
		return nil, ErrUnavailable
	}
	scan := bufio.NewScanner(out)
	scan.Buffer(make([]byte, 256), 1024)
	if !scan.Scan() {
		_ = s.Close()
		return nil, coordination.ErrBlocked
	}
	var state coordination.State
	if json.Unmarshal(scan.Bytes(), &state) != nil || !state.Active {
		_ = s.Close()
		return nil, coordination.ErrBlocked
	}
	return &RemoteGuard{session: s, in: in, scan: scan, Epoch: state.Epoch}, nil
}
func (g *RemoteGuard) Check() error {
	if _, e := io.WriteString(g.in, "check\n"); e != nil {
		return ErrUnavailable
	}
	if !g.scan.Scan() || g.scan.Text() != "ok" {
		return ErrUnavailable
	}
	return nil
}
func (g *RemoteGuard) Close(stopped bool) error {
	defer g.session.Close()
	if stopped {
		if _, e := io.WriteString(g.in, "stopped\n"); e != nil {
			return ErrUnavailable
		}
	}
	_ = g.in.Close()
	if g.session.Wait() != nil {
		return ErrUnavailable
	}
	return nil
}
func FileClient(ctx context.Context, t Target) (*ssh.Client, *sftp.Client, error) {
	c, e := Dial(ctx, t)
	if e != nil {
		return nil, nil, e
	}
	stop := context.AfterFunc(ctx, func() { _ = c.Close() })
	defer stop()
	f, e := sftp.NewClient(c)
	if e != nil {
		_ = c.Close()
		return nil, nil, ErrUnavailable
	}
	return c, f, nil
}
