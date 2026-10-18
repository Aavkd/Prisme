package operations

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

type Envelope struct {
	WrappedKey []byte `json:"wrapped_key"`
	Ciphertext []byte `json:"ciphertext"`
}
type Vault struct {
	DB     *sql.DB
	master []byte
}
type Secret struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Scope       string    `json:"scope"`
	Kind        string    `json:"kind"`
	Destination string    `json:"destination"`
	Revision    int       `json:"revision"`
	ExpiresAt   time.Time `json:"expires_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func NewVault(db *sql.DB, file string) (*Vault, error) {
	key, e := os.ReadFile(file)
	if e != nil || len(key) != 32 {
		return nil, ErrUnavailable
	}
	return &Vault{DB: db, master: key}, nil
}
func seal(key, plain, aad []byte) ([]byte, error) {
	b, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	g, e := cipher.NewGCM(b)
	if e != nil {
		return nil, e
	}
	n := make([]byte, g.NonceSize())
	if _, e = rand.Read(n); e != nil {
		return nil, e
	}
	return g.Seal(n, n, plain, aad), nil
}
func unseal(key, raw, aad []byte) ([]byte, error) {
	b, e := aes.NewCipher(key)
	if e != nil {
		return nil, ErrUnavailable
	}
	g, e := cipher.NewGCM(b)
	if e != nil || len(raw) < g.NonceSize() {
		return nil, ErrUnavailable
	}
	p, e := g.Open(nil, raw[:g.NonceSize()], raw[g.NonceSize():], aad)
	if e != nil {
		return nil, ErrUnavailable
	}
	return p, nil
}
func wipe(b []byte) { clear(b) }
func (v *Vault) Encrypt(plain, aad []byte) (Envelope, error) {
	key := make([]byte, 32)
	if _, e := rand.Read(key); e != nil {
		return Envelope{}, e
	}
	defer wipe(key)
	w, e := seal(v.master, key, append([]byte("key:"), aad...))
	if e != nil {
		return Envelope{}, e
	}
	c, e := seal(key, plain, aad)
	return Envelope{WrappedKey: w, Ciphertext: c}, e
}
func (v *Vault) Decrypt(env Envelope, aad []byte) ([]byte, error) {
	key, e := unseal(v.master, env.WrappedKey, append([]byte("key:"), aad...))
	if e != nil {
		return nil, e
	}
	defer wipe(key)
	return unseal(key, env.Ciphertext, aad)
}
func aad(s Secret) []byte {
	return []byte(fmt.Sprintf("prisme-vault-v1\n%s\n%d\n%s\n%s\n%s\n%s\n%s", s.ID, s.Revision, s.Name, s.Scope, s.Kind, s.Destination, s.ExpiresAt.UTC().Format(time.RFC3339Nano)))
}
func vaultAudit(ctx context.Context, tx *sql.Tx, p Principal, action, target string) error {
	_, e := tx.ExecContext(ctx, `INSERT INTO audit_events(actor,action,target,device_id,origin) VALUES($1,$2,$3,$4,'application')`, p.Actor, action, target, p.Device)
	return e
}
func (v *Vault) List(ctx context.Context) ([]Secret, error) {
	rows, e := v.DB.QueryContext(ctx, `SELECT id,name,scope,kind,destination,revision,expires_at,updated_at FROM vault_secrets ORDER BY name LIMIT 200`)
	if e != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	out := []Secret{}
	for rows.Next() {
		var s Secret
		if rows.Scan(&s.ID, &s.Name, &s.Scope, &s.Kind, &s.Destination, &s.Revision, &s.ExpiresAt, &s.UpdatedAt) != nil {
			return nil, ErrUnavailable
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
func (v *Vault) Put(ctx context.Context, p Principal, s Secret, value []byte) (Secret, error) {
	if !identifier.MatchString(s.Name) || !identifier.MatchString(s.Scope) || !identifier.MatchString(s.Destination) || !identifier.MatchString(s.Kind) || len(value) == 0 || len(value) > 64*1024 || !s.ExpiresAt.After(time.Now()) || s.ExpiresAt.After(time.Now().Add(366*24*time.Hour)) {
		return s, ErrInvalid
	}
	if s.ID == "" {
		if s.Revision != 0 {
			return s, ErrInvalid
		}
		s.ID = ID()
	} else if !digestPattern.MatchString(s.ID) {
		return s, ErrInvalid
	}
	expected := s.Revision
	s.ExpiresAt = s.ExpiresAt.UTC().Truncate(time.Microsecond)
	s.Revision++
	s.UpdatedAt = time.Now().UTC()
	env, e := v.Encrypt(value, aad(s))
	if e != nil {
		return s, ErrUnavailable
	}
	raw, _ := json.Marshal(env)
	tx, e := v.DB.BeginTx(ctx, nil)
	if e != nil {
		return s, ErrUnavailable
	}
	defer tx.Rollback()
	var result sql.Result
	if expected == 0 {
		result, e = tx.ExecContext(ctx, `INSERT INTO vault_secrets(id,name,scope,kind,destination,revision,expires_at,envelope) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING`, s.ID, s.Name, s.Scope, s.Kind, s.Destination, s.Revision, s.ExpiresAt, string(raw))
	} else {
		result, e = tx.ExecContext(ctx, `UPDATE vault_secrets SET name=$2,scope=$3,kind=$4,destination=$5,revision=$6,expires_at=$7,envelope=$8,updated_at=now() WHERE id=$1 AND revision=$9`, s.ID, s.Name, s.Scope, s.Kind, s.Destination, s.Revision, s.ExpiresAt, string(raw), expected)
	}
	if e != nil {
		return s, ErrUnavailable
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return s, ErrConflict
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO vault_history(secret_id,revision,actor,action) VALUES($1,$2,$3,'rotate')`, s.ID, s.Revision, p.Actor)
	if e == nil {
		e = vaultAudit(ctx, tx, p, "vault.rotated", s.ID)
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		return s, ErrUnavailable
	}
	return s, nil
}
func (v *Vault) Reveal(ctx context.Context, p Principal, id, destination string, revision int) ([]byte, error) {
	tx, e := v.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, ErrUnavailable
	}
	defer tx.Rollback()
	var s Secret
	var raw []byte
	e = tx.QueryRowContext(ctx, `SELECT id,name,scope,kind,destination,revision,expires_at,updated_at,envelope FROM vault_secrets WHERE id=$1 FOR UPDATE`, id).Scan(&s.ID, &s.Name, &s.Scope, &s.Kind, &s.Destination, &s.Revision, &s.ExpiresAt, &s.UpdatedAt, &raw)
	if e != nil || s.Revision != revision || s.Destination != destination || !s.ExpiresAt.After(time.Now()) {
		return nil, ErrDenied
	}
	var env Envelope
	if json.Unmarshal(raw, &env) != nil {
		return nil, ErrUnavailable
	}
	plain, e := v.Decrypt(env, aad(s))
	if e != nil {
		return nil, e
	}
	if e = vaultAudit(ctx, tx, p, "vault.revealed", id); e == nil {
		e = tx.Commit()
	}
	if e != nil {
		wipe(plain)
		return nil, ErrUnavailable
	}
	return plain, nil
}

// Env accepts literal values only. No interpolation, shell execution or export.
func Env(values map[string]string) ([]byte, error) {
	if len(values) > 200 {
		return nil, ErrInvalid
	}
	var b strings.Builder
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := values[k]
		if !envName.MatchString(k) || strings.ContainsAny(v, "\x00\r\n") {
			return nil, ErrInvalid
		}
		b.WriteString(k + "='" + strings.ReplaceAll(v, "'", "\\'") + "'\n")
	}
	if b.Len() > 64*1024 {
		return nil, ErrInvalid
	}
	return []byte(b.String()), nil
}
