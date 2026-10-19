package api

import (
	"context"
	"database/sql"
	"net/http"
)

type auditContextKey struct{}
type auditContext struct{ Actor, Device, Origin, Correlation, Address string }

func requestAuditContext(r *http.Request, address string) context.Context {
	correlation, _ := randomID()
	return context.WithValue(r.Context(), auditContextKey{}, auditContext{Origin: "application", Correlation: correlation, Address: address})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *statusWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	return w.ResponseWriter.Write(data)
}
func recordRejection(db *sql.DB, r *http.Request, status int) {
	if db == nil || status < 400 || status >= 500 || status == 429 {
		return
	}
	metadata, _ := r.Context().Value(auditContextKey{}).(auditContext)
	// Registered route patterns only: raw paths, queries and payloads are untrusted.
	target := r.Pattern
	if target == "" {
		target = "unmatched-route"
	}
	_, _ = db.ExecContext(r.Context(), "INSERT INTO audit_events(actor,action,target,origin,result,correlation_id,remote_address) VALUES('unverified','request.rejected',$1,'application','rejected',$2,$3)", target, metadata.Correlation, metadata.Address)
}
