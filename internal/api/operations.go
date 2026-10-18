package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/go-webauthn/webauthn/webauthn"
	"prisme.local/platform/internal/operations"
)

func (s *Server) ConfigureOperations(ctx context.Context, c operations.Config) error {
	valid := func(ctx context.Context, p operations.Principal) bool {
		var ok bool
		e := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sessions s JOIN devices d ON d.id=s.device_id WHERE s.token_hash=$1 AND s.expires_at>now() AND NOT d.revoked AND s.auth_mode='passkey')`, p.Session).Scan(&ok)
		return e == nil && ok
	}
	identity := operations.Identity{WebAuthn: s.WebAuthn}
	identity.Authenticate = func(r *http.Request) (operations.Principal, error) {
		var p operations.Principal
		token := bearer(r)
		if !opaque.MatchString(token) {
			return p, operations.ErrDenied
		}
		p.Session = Hash(token)
		e := s.DB.QueryRowContext(r.Context(), `SELECT s.device_id,s.auth_mode,s.expires_at FROM sessions s JOIN devices d ON d.id=s.device_id WHERE s.token_hash=$1 AND s.expires_at>now() AND NOT d.revoked`, p.Session).Scan(&p.Device, &p.Method, &p.ExpiresAt)
		p.Actor = "administrator"
		if e != nil || (s.Mode == "identity" && p.Method != "passkey") {
			return p, operations.ErrDenied
		}
		return p, nil
	}
	identity.User = func(ctx context.Context, p operations.Principal) (webauthn.User, error) {
		if !valid(ctx, p) {
			return nil, operations.ErrDenied
		}
		tx, e := s.DB.BeginTx(ctx, nil)
		if e != nil {
			return nil, e
		}
		defer tx.Rollback()
		return loadUser(ctx, tx)
	}
	identity.UpdateCredential = func(ctx context.Context, p operations.Principal, c *webauthn.Credential) error {
		if !valid(ctx, p) {
			return operations.ErrDenied
		}
		raw, e := json.Marshal(c)
		if e != nil {
			return e
		}
		_, e = s.DB.ExecContext(ctx, `UPDATE passkeys SET credential=$1 WHERE id=$2`, string(raw), base64.RawURLEncoding.EncodeToString(c.ID))
		return e
	}
	identity.AndroidKey = func(ctx context.Context, p operations.Principal) ([]byte, error) {
		if !valid(ctx, p) {
			return nil, operations.ErrDenied
		}
		var raw []byte
		e := s.DB.QueryRowContext(ctx, `SELECT public_key FROM device_action_keys WHERE device_id=$1`, p.Device).Scan(&raw)
		return raw, e
	}
	svc := operations.New(ctx, c, identity, "normal")
	svc.Revalidate = valid
	svc.Audit = func(ctx context.Context, p operations.Principal, action, target, id, incident, result string) error {
		_, e := s.DB.ExecContext(ctx, `INSERT INTO audit_events(actor,action,target,device_id,origin,correlation_id,result) VALUES($1,$2,$3,$4,'application',$5,$6)`, p.Actor, "operations."+action, target, p.Device, id, result)
		if e == nil && incident != "" {
			_, e = s.DB.ExecContext(ctx, `INSERT INTO incident_events(incident_id,occurred_at,kind,actor,message) SELECT id,now(),'terminal',$2,$3 FROM incidents WHERE id::text=$1`, incident, p.Actor, "Terminal session "+id)
		}
		return e
	}
	if c.MasterKeyFile != "" {
		v, e := operations.NewVault(s.DB, c.MasterKeyFile)
		if e != nil {
			return e
		}
		svc.Vault = v
	}
	s.Operations = svc
	return nil
}
