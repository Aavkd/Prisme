package api

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"

	"prisme.local/platform/internal/recovery"
)

func (s *Server) importRecovery(w http.ResponseWriter, r *http.Request, session string) {
	if s.Operations == nil {
		problem(w, 503, "operations_disabled")
		return
	}
	var q struct {
		Batch     recovery.Batch `json:"batch"`
		Signature string         `json:"signature"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256*1024)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(&q) != nil {
		problem(w, 400, "invalid_batch")
		return
	}
	key, e := base64.StdEncoding.DecodeString(s.Operations.Config.RecoveryKeys[q.Batch.Origin])
	signature, se := base64.StdEncoding.DecodeString(q.Signature)
	raw, _ := json.Marshal(q.Batch)
	if e != nil || se != nil || len(key) != ed25519.PublicKeySize || Hash(string(key)) != q.Batch.Origin || !ed25519.Verify(key, raw, signature) || len(q.Batch.Events) > 200 || q.Batch.After < 0 || q.Batch.Cursor != q.Batch.After+len(q.Batch.Events) {
		problem(w, 403, "untrusted_recovery_audit")
		return
	}
	tx, e := s.DB.BeginTx(r.Context(), nil)
	if e != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(r.Context(), `INSERT INTO recovery_audit_cursors(origin,target,cursor) VALUES($1,'gateway',0) ON CONFLICT DO NOTHING`, q.Batch.Origin)
	var cursor int
	if e == nil {
		e = tx.QueryRowContext(r.Context(), `SELECT cursor FROM recovery_audit_cursors WHERE origin=$1 AND target='gateway' FOR UPDATE`, q.Batch.Origin).Scan(&cursor)
	}
	if e != nil {
		problem(w, 503, "unavailable")
		return
	}
	if q.Batch.After > cursor {
		problem(w, 409, "audit_gap")
		return
	}
	for _, event := range q.Batch.Events {
		if event.ID == "" || event.Target == "" || event.Origin != "recovery" {
			problem(w, 400, "invalid_batch")
			return
		}
		b, _ := json.Marshal(event)
		_, e = tx.ExecContext(r.Context(), `INSERT INTO recovery_audit(origin,event_id,target,event) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, q.Batch.Origin, event.ID, event.Target, string(b))
		if e != nil {
			break
		}
		var equal bool
		e = tx.QueryRowContext(r.Context(), `SELECT event=$3::jsonb FROM recovery_audit WHERE origin=$1 AND event_id=$2`, q.Batch.Origin, event.ID, string(b)).Scan(&equal)
		if e != nil || !equal {
			problem(w, 409, "audit_conflict")
			return
		}
	}
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `UPDATE recovery_audit_cursors SET cursor=GREATEST(cursor,$2) WHERE origin=$1 AND target='gateway'`, q.Batch.Origin, q.Batch.Cursor)
	}
	if e == nil {
		e = audit(r.Context(), tx, "administrator", "recovery.audit_imported", q.Batch.Origin)
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 200, map[string]int{"cursor": max(cursor, q.Batch.Cursor)})
}
