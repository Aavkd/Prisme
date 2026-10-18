package operations

import (
	"bytes"
	"context"
	"sync"
	"time"

	"prisme.local/platform/internal/observability"
)

type boundedOutput struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.Len()+len(p) > 256*1024 {
		return 0, ErrUnavailable
	}
	return b.Buffer.Write(p)
}
func (s *Service) diagnostics(ctx context.Context, p Principal, t Target, container string) (map[string]any, error) {
	if !t.Diagnostics {
		return nil, ErrDenied
	}
	command := "/usr/bin/uptime; /bin/df -Pk; /usr/bin/docker ps --format '{{json .}}'"
	if container != "" {
		allowed := false
		for _, c := range t.Containers {
			if c == container {
				allowed = true
			}
		}
		if !allowed {
			return nil, ErrDenied
		}
		command = "/usr/bin/docker logs --tail 100 --timestamps " + quote(container)
	}
	if e := s.audit(ctx, p, "diagnostics", t.ID, "", "", "requested"); e != nil {
		return nil, e
	}
	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	c, e := Dial(cctx, t)
	if e != nil {
		return nil, e
	}
	defer c.Close()
	stop := context.AfterFunc(cctx, func() { _ = c.Close() })
	defer stop()
	session, e := c.NewSession()
	if e != nil {
		return nil, ErrUnavailable
	}
	defer session.Close()
	var out boundedOutput
	session.Stdout = &out
	session.Stderr = &out
	e = session.Run(command)
	state := "observed"
	if e != nil {
		state = "partial"
	}
	if cctx.Err() != nil {
		return nil, ErrUnavailable
	}
	return map[string]any{"source": "ssh", "target": t.ID, "observed_at": time.Now().UTC(), "state": state, "text": observability.Redact(out.String())}, nil
}
