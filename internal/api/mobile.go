package api

import (
	"database/sql"
	"net/http"
	"strings"

	"prisme.local/platform/internal/notifications"
)

func (s *Server) mobileRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /v1/notifications", s.auth(s.notificationSettings))
	m.HandleFunc("PUT /v1/notifications", s.auth(s.notificationSettings))
	m.HandleFunc("DELETE /v1/notifications", s.auth(s.notificationSettings))
	m.HandleFunc("GET /v1/notifications/incidents/{public}", s.auth(s.notificationIncident))
	m.HandleFunc("POST /v1/action-key", s.auth(s.actionKey))
	m.HandleFunc("POST /v1/action-challenges", s.auth(s.actionChallenge))
	m.HandleFunc("GET /v1/action-challenges/{challenge}", s.auth(s.actionStatus))
	m.HandleFunc("POST /v1/action-challenges/{challenge}/sign", s.auth(s.actionSign))
	m.HandleFunc("GET /v1/action-challenges/{challenge}/options", s.actionOptions)
	m.HandleFunc("POST /v1/action-challenges/{challenge}/passkey", s.actionPasskey)
	m.HandleFunc("GET /action", s.actionPage)
	m.HandleFunc("GET /action.js", s.actionScript)
}
func (s *Server) notificationSettings(w http.ResponseWriter, r *http.Request, session string) {
	metadata, _ := r.Context().Value(auditContextKey{}).(auditContext)
	if metadata.Device == "" {
		problem(w, 403, "device_required")
		return
	}
	if r.Method == "GET" {
		p := notifications.Preferences{Timezone: "UTC", QuietStart: 1320, QuietEnd: 420}
		err := s.DB.QueryRowContext(r.Context(), `SELECT enabled,timezone,quiet_start,quiet_end FROM notification_devices WHERE device_id=$1`, metadata.Device).Scan(&p.Enabled, &p.Timezone, &p.QuietStart, &p.QuietEnd)
		if err != nil && err != sql.ErrNoRows {
			problem(w, 503, "unavailable")
			return
		}
		var registered bool
		if err = s.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM device_action_keys WHERE device_id=$1)`, metadata.Device).Scan(&registered); err != nil {
			problem(w, 503, "unavailable")
			return
		}
		send(w, 200, map[string]any{"preferences": p, "configured": s.Notifications != nil && s.Notifications.Sender != nil, "action_key_registered": registered})
		return
	}
	var req struct {
		Token string `json:"token"`
		notifications.Preferences
	}
	if r.Method != "DELETE" {
		if !body(w, r, &req) {
			return
		}
		if req.Validate() != nil || len(req.Token) > 4096 || strings.ContainsAny(req.Token, " \r\n\x00") {
			problem(w, 400, "invalid_notification_settings")
			return
		}
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	var revoked bool
	if err = tx.QueryRowContext(r.Context(), `SELECT revoked FROM devices WHERE id=$1 FOR UPDATE`, metadata.Device).Scan(&revoked); err != nil || revoked {
		problem(w, 401, "unauthorized")
		return
	}
	if r.Method == "DELETE" {
		_, err = tx.ExecContext(r.Context(), `DELETE FROM notification_devices WHERE device_id=$1`, metadata.Device)
	} else if req.Token == "" {
		var result sql.Result
		result, err = tx.ExecContext(r.Context(), `UPDATE notification_devices SET enabled=$2,timezone=$3,quiet_start=$4,quiet_end=$5,updated_at=now() WHERE device_id=$1`, metadata.Device, req.Enabled, req.Timezone, req.QuietStart, req.QuietEnd)
		if err == nil {
			n, _ := result.RowsAffected()
			if n != 1 {
				problem(w, 409, "push_token_required")
				return
			}
		}
	} else {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO notification_devices(device_id,token,enabled,timezone,quiet_start,quiet_end) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(device_id) DO UPDATE SET token=EXCLUDED.token,enabled=EXCLUDED.enabled,timezone=EXCLUDED.timezone,quiet_start=EXCLUDED.quiet_start,quiet_end=EXCLUDED.quiet_end,updated_at=now()`, metadata.Device, req.Token, req.Enabled, req.Timezone, req.QuietStart, req.QuietEnd)
	}
	if err == nil {
		if r.Method == "DELETE" || !req.Enabled {
			_, err = tx.ExecContext(r.Context(), `UPDATE notification_deliveries SET state='cancelled' WHERE device_id=$1 AND state='pending'`, metadata.Device)
		} else {
			_, err = tx.ExecContext(r.Context(), `UPDATE notification_deliveries SET due_at=now() WHERE device_id=$1 AND state='pending'`, metadata.Device)
		}
	}
	if err == nil {
		err = audit(r.Context(), tx, metadata.Actor, "notifications.settings", metadata.Device)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		problem(w, 409, "notification_settings_conflict")
		return
	}
	send(w, 200, map[string]string{"status": "saved"})
}
func (s *Server) notificationIncident(w http.ResponseWriter, r *http.Request, _ string) {
	id := r.PathValue("public")
	if !opaque.MatchString(id) {
		problem(w, 400, "invalid_id")
		return
	}
	var internal string
	if err := s.DB.QueryRowContext(r.Context(), `SELECT incident_id::text FROM notification_incidents WHERE public_id=$1`, id).Scan(&internal); err != nil {
		if err == sql.ErrNoRows {
			problem(w, 404, "incident_not_found")
		} else {
			problem(w, 503, "unavailable")
		}
		return
	}
	r.SetPathValue("id", internal)
	s.incidentDetail(w, r, "")
}
