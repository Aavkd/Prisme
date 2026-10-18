package operations

import (
	"context"
	"encoding/base64"
	"io"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pkg/sftp"
	"prisme.local/platform/internal/observability"
)

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type FileRequest struct {
	Path     string `json:"path"`
	Offset   int64  `json:"offset"`
	Revision string `json:"revision"`
	Content  string `json:"content"`
	Base64   string `json:"base64"`
}
type Entry struct {
	Name      string    `json:"name"`
	Directory bool      `json:"directory"`
	Size      int64     `json:"size"`
	Mode      string    `json:"mode"`
	UID       uint32    `json:"uid"`
	GID       uint32    `json:"gid"`
	Modified  time.Time `json:"modified_at"`
}

func protected(p string, env bool) bool {
	for _, base := range []string{"/etc", "/proc", "/sys", "/dev", "/root", "/boot", "/run"} {
		if p == base || strings.HasPrefix(p, base+"/") {
			return true
		}
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".ssh" || part == ".git" || strings.HasPrefix(part, ".prisme-") || (!env && (part == ".env" || strings.HasPrefix(part, ".env."))) || strings.HasSuffix(part, ".pem") || strings.HasSuffix(part, ".key") {
			return true
		}
	}
	return false
}
func safePath(f *sftp.Client, t Target, p string, missing, env bool) error {
	if !path.IsAbs(p) || path.Clean(p) != p || strings.ContainsAny(p, "\x00\r\n") || protected(p, env) {
		return ErrDenied
	}
	allowed := false
	for _, root := range t.Roots {
		if p == root || strings.HasPrefix(p, root+"/") {
			allowed = true
		}
	}
	if !allowed {
		return ErrDenied
	}
	current := ""
	parts := strings.Split(strings.TrimPrefix(p, "/"), "/")
	for i, part := range parts {
		current += "/" + part
		s, e := f.Lstat(current)
		if e != nil {
			if missing && i == len(parts)-1 && os.IsNotExist(e) {
				return nil
			}
			return ErrDenied
		}
		if s.Mode()&os.ModeSymlink != 0 {
			return ErrDenied
		}
		if i < len(parts)-1 && !s.IsDir() {
			return ErrDenied
		}
	}
	return nil
}
func readFile(f *sftp.Client, p string) ([]byte, os.FileInfo, error) {
	s, e := f.Lstat(p)
	if e != nil {
		return nil, nil, e
	}
	if !s.Mode().IsRegular() || s.Size() > MaxFile {
		return nil, nil, ErrDenied
	}
	r, e := f.Open(p)
	if e != nil {
		return nil, nil, ErrUnavailable
	}
	defer r.Close()
	b, e := io.ReadAll(io.LimitReader(r, MaxFile+1))
	if e != nil || len(b) > MaxFile {
		return nil, nil, ErrUnavailable
	}
	return b, s, nil
}
func revision(f *sftp.Client, p string) (string, error) {
	b, _, e := readFile(f, p)
	if os.IsNotExist(e) {
		return "absent", nil
	}
	if e != nil {
		return "", e
	}
	defer wipe(b)
	return Hash(b), nil
}
func fileEntry(s os.FileInfo) Entry {
	e := Entry{Name: s.Name(), Directory: s.IsDir(), Size: s.Size(), Mode: s.Mode().String(), Modified: s.ModTime().UTC()}
	if st, ok := s.Sys().(*sftp.FileStat); ok {
		e.UID = st.UID
		e.GID = st.GID
	}
	return e
}
func (s *Service) fileRead(ctx context.Context, t Target, action string, q FileRequest) (map[string]any, error) {
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
	if e = safePath(f, t, q.Path, action == "file_stat", action == "file_stat"); e != nil {
		return nil, e
	}
	if action == "file_list" {
		info, e := f.Stat(q.Path)
		if e != nil || !info.IsDir() {
			return nil, ErrDenied
		}
		list, e := f.ReadDirContext(ctx, q.Path)
		if e != nil || len(list) > 1000 {
			return nil, ErrUnavailable
		}
		out := []Entry{}
		for _, entry := range list {
			if entry.Mode()&os.ModeSymlink == 0 && !protected(path.Join(q.Path, entry.Name()), false) {
				out = append(out, fileEntry(entry))
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return map[string]any{"entries": out, "path": q.Path}, nil
	}
	if action == "file_stat" {
		rev, e := revision(f, q.Path)
		return map[string]any{"revision": rev}, e
	}
	b, info, e := readFile(f, q.Path)
	if e != nil {
		return nil, e
	}
	defer wipe(b)
	rev := Hash(b)
	if action == "file_download" {
		if q.Offset < 0 || q.Offset > int64(len(b)) || (q.Offset > 0 && q.Revision != rev) {
			return nil, ErrConflict
		}
		end := min(q.Offset+ChunkSize, int64(len(b)))
		return map[string]any{"base64": base64.StdEncoding.EncodeToString(b[q.Offset:end]), "offset": end, "size": len(b), "revision": rev}, nil
	}
	if !utf8.Valid(b) || strings.ContainsRune(string(b), 0) {
		return nil, ErrInvalid
	}
	text := string(b)
	redacted := observability.Redact(text)
	if redacted != text {
		return map[string]any{"content": redacted, "revision": rev, "editable": false, "entry": fileEntry(info)}, nil
	}
	return map[string]any{"content": text, "revision": rev, "editable": true, "entry": fileEntry(info)}, nil
}

// atomicSave uses an exclusive temporary file, fsync, backup, and POSIX rename.
// Permission to rename in a directory is broader than permission to write a file;
// require an existing file to be writable by the selected SSH identity as well.
func atomicSave(f *sftp.Client, t Target, p, expected string, data []byte, env bool) (string, error) {
	if len(data) > MaxFile || expected == "" {
		return "", ErrInvalid
	}
	if e := safePath(f, t, p, true, env); e != nil {
		return "", e
	}
	if _, ok := f.HasExtension("posix-rename@openssh.com"); !ok {
		return "", ErrDenied
	}
	if _, ok := f.HasExtension("fsync@openssh.com"); !ok {
		return "", ErrDenied
	}
	old, info, e := readFile(f, p)
	exists := e == nil
	if e != nil && !os.IsNotExist(e) {
		return "", e
	}
	defer wipe(old)
	rev := "absent"
	if exists {
		rev = Hash(old)
	}
	if rev != expected {
		return "", ErrConflict
	}
	if exists {
		w, e := f.OpenFile(p, os.O_WRONLY)
		if e != nil {
			return "", ErrDenied
		}
		_ = w.Close()
	}
	temp := path.Join(path.Dir(p), ".prisme-"+ID())
	out, e := f.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY)
	if e != nil {
		return "", ErrUnavailable
	}
	defer f.Remove(temp)
	mode := os.FileMode(0600)
	if exists && !env {
		mode = info.Mode().Perm()
	}
	if e = out.Chmod(0600); e == nil {
		_, e = out.Write(data)
	}
	if e == nil {
		e = out.Sync()
	}
	ce := out.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return "", ErrUnavailable
	}
	if exists {
		st, ok := info.Sys().(*sftp.FileStat)
		if !ok {
			return "", ErrDenied
		}
		if e = f.Chown(temp, int(st.UID), int(st.GID)); e != nil {
			return "", ErrDenied
		}
	}
	if e = f.Chmod(temp, mode); e != nil {
		return "", ErrUnavailable
	}
	if exists {
		backup := path.Join(path.Dir(p), ".prisme-backup-"+ID())
		b, e := f.OpenFile(backup, os.O_CREATE|os.O_EXCL|os.O_WRONLY)
		if e != nil {
			return "", ErrUnavailable
		}
		if e = b.Chmod(0600); e == nil {
			_, e = b.Write(old)
		}
		if e == nil {
			e = b.Sync()
		}
		ce = b.Close()
		if e == nil {
			e = ce
		}
		if e != nil {
			_ = f.Remove(backup)
			return "", ErrUnavailable
		}
	}
	// Catch cooperating changes after backup and reject changed symlink components.
	if e = safePath(f, t, p, true, env); e != nil {
		return "", e
	}
	current, e := revision(f, p)
	if e != nil || current != expected {
		return "", ErrConflict
	}
	if e = f.PosixRename(temp, p); e != nil {
		return "", ErrUnavailable
	}
	return Hash(data), nil
}
