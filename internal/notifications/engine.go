package notifications

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type Engine struct {
	DB     *sql.DB
	Config Config
	Sender Sender
	Now    func() time.Time
}

func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		_ = e.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// A transaction-wide lock serializes scheduling across API replicas. The durable
// uniqueness key survives restarts; note-only revisions do not produce pushes.
func (e *Engine) Schedule(ctx context.Context) error {
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(731507)"); err != nil {
		return err
	}
	now := e.Now()
	// Retain the most recent week of completed attempts; pending retries are bounded.
	if _, err = tx.ExecContext(ctx, `DELETE FROM notification_deliveries WHERE state<>'pending' AND created_at<$1`, now.Add(-7*24*time.Hour)); err != nil {
		return err
	}
	for _, policy := range e.Config.Rules {
		rows, err := tx.QueryContext(ctx, `SELECT i.id,i.severity,i.status,COALESCE((SELECT max(occurred_at) FROM incident_events WHERE incident_id=i.id AND kind IN ('detected','reopened')),i.opened_at),COALESCE(n.public_id,''),COALESCE(n.state_key,''),COALESCE(n.escalation,0) FROM incidents i LEFT JOIN notification_incidents n ON n.incident_id=i.id WHERE i.rule_id=$1 AND (i.status NOT IN ('resolved','closed') OR i.updated_at>$2) ORDER BY i.id DESC LIMIT 100`, policy.RuleID, now.Add(-24*time.Hour))
		if err != nil {
			return err
		}
		type incident struct {
			id               int64
			severity, status string
			opened           time.Time
			public, key      string
			escalation       int
		}
		all := []incident{}
		for rows.Next() {
			var i incident
			if err = rows.Scan(&i.id, &i.severity, &i.status, &i.opened, &i.public, &i.key, &i.escalation); err != nil {
				rows.Close()
				return err
			}
			all = append(all, i)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, i := range all {
			if i.public == "" {
				b := make([]byte, 32)
				if _, err = rand.Read(b); err != nil {
					return err
				}
				i.public = hex.EncodeToString(b)
				if _, err = tx.ExecContext(ctx, `INSERT INTO notification_incidents(incident_id,public_id) VALUES($1,$2)`, i.id, i.public); err != nil {
					return err
				}
			}
			active := i.status == "detected" || i.status == "reopened"
			resolved := i.status == "resolved" || i.status == "closed"
			urgent := policy.Urgent && i.severity == "critical" && active
			state := "attention"
			if resolved {
				state = "resolved"
			} else if !active {
				state = "acknowledged"
			}
			escalation := 0
			if urgent && policy.MaxEscalations > 0 {
				escalation = int(now.Sub(i.opened).Seconds()) / policy.EscalateSeconds
				if escalation > policy.MaxEscalations {
					escalation = policy.MaxEscalations
				}
			}
			key := fmt.Sprintf("%s:%s:%d", state, i.severity, escalation)
			// A circuit opening is a distinct, bounded request for intervention,
			// even if the original incident was already critical. Ordinary step
			// logs do not generate additional pushes.
			var circuit int64
			if err = tx.QueryRowContext(ctx, `SELECT COALESCE(max(a.id),0) FROM automation_events a JOIN automation_runs r ON r.id=a.run_id WHERE a.kind='circuit_open' AND (r.document->>'incident')::bigint=$1`, i.id).Scan(&circuit); err != nil {
				return err
			}
			if circuit > 0 {
				key += fmt.Sprintf(":circuit:%d", circuit)
			}
			if key == i.key {
				continue
			}
			if _, err = tx.ExecContext(ctx, `UPDATE notification_deliveries SET state='cancelled' WHERE incident_id=$1 AND state='pending'`, i.id); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `UPDATE notification_incidents SET state_key=$2,escalation=$3 WHERE incident_id=$1`, i.id, key, escalation); err != nil {
				return err
			}
			if state == "acknowledged" {
				continue
			}
			p := Payload{IncidentID: i.public, Severity: i.severity, Title: "Prisme", State: state, Route: "/incidents/" + i.public, Urgent: urgent}
			eventHash := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", i.public, key, now.UnixNano())))
			p.EventID = hex.EncodeToString(eventHash[:])
			raw, _ := json.Marshal(p)
			dr, err := tx.QueryContext(ctx, `SELECT n.device_id,n.enabled,n.timezone,n.quiet_start,n.quiet_end FROM notification_devices n JOIN devices d ON d.id=n.device_id WHERE n.enabled AND NOT d.revoked`)
			if err != nil {
				return err
			}
			type device struct {
				id string
				p  Preferences
			}
			devices := []device{}
			for dr.Next() {
				var d device
				if err = dr.Scan(&d.id, &d.p.Enabled, &d.p.Timezone, &d.p.QuietStart, &d.p.QuietEnd); err != nil {
					dr.Close()
					return err
				}
				devices = append(devices, d)
			}
			err = dr.Err()
			dr.Close()
			if err != nil {
				return err
			}
			for _, d := range devices {
				due := d.p.Due(now, urgent, time.Duration(e.Config.GroupSeconds)*time.Second)
				// Include the previous state so a resolved->reopened incident can notify again.
				event := fmt.Sprintf("%s:%d", key, now.UnixNano())
				if _, err = tx.ExecContext(ctx, `INSERT INTO notification_deliveries(device_id,incident_id,event_key,payload,due_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, d.id, i.id, event, string(raw), due); err != nil {
					return err
				}
			}
		}
	}
	return tx.Commit()
}
func (e *Engine) Tick(ctx context.Context) error {
	if err := e.Schedule(ctx); err != nil {
		return err
	}
	if e.Sender == nil {
		return nil
	}
	for i := 0; i < 20; i++ {
		found, err := e.DeliverOne(ctx)
		if err != nil {
			return err
		}
		if !found {
			break
		}
	}
	return nil
}

// Hold device and delivery locks during a bounded send: a completed revocation
// prevents further sends. Provider acceptance is not proof the phone displayed it.
func (e *Engine) DeliverOne(ctx context.Context) (bool, error) {
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var id, incident int64
	var device, token string
	var raw []byte
	var attempts int
	var p Preferences
	var revoked bool
	err = tx.QueryRowContext(ctx, `SELECT q.id,q.incident_id,q.device_id,q.payload,q.attempts FROM notification_deliveries q WHERE q.state='pending' AND q.due_at<=$1 ORDER BY q.due_at,q.id LIMIT 1`, e.Now()).Scan(&id, &incident, &device, &raw, &attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	err = tx.QueryRowContext(ctx, `SELECT n.token,n.enabled,n.timezone,n.quiet_start,n.quiet_end,d.revoked FROM notification_devices n JOIN devices d ON d.id=n.device_id WHERE d.id=$1 FOR UPDATE OF d,n`, device).Scan(&token, &p.Enabled, &p.Timezone, &p.QuietStart, &p.QuietEnd, &revoked)
	deviceErr := err
	// Match the settings transaction's device-before-delivery lock order, then
	// re-read the row so another scheduler/sender cannot replay a stale candidate.
	err = tx.QueryRowContext(ctx, `SELECT payload,attempts FROM notification_deliveries WHERE id=$1 AND state='pending' AND due_at<=$2 FOR UPDATE SKIP LOCKED`, id, e.Now()).Scan(&raw, &attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	err = deviceErr
	state := "pending"
	code := ""
	due := e.Now()
	var payload Payload
	if errors.Is(err, sql.ErrNoRows) || revoked || !p.Enabled {
		state = "cancelled"
	} else if err != nil {
		return false, err
	} else if json.Unmarshal(raw, &payload) != nil {
		state = "failed"
		code = "invalid_payload"
	} else {
		var status, severity, rule string
		if err = tx.QueryRowContext(ctx, `SELECT status,severity,rule_id FROM incidents WHERE id=$1`, incident).Scan(&status, &severity, &rule); err != nil {
			return false, err
		}
		active := status == "detected" || status == "reopened"
		configured := false
		for _, p := range e.Config.Rules {
			if p.RuleID == rule {
				configured = true
				payload.Urgent = payload.Urgent && p.Urgent
			}
		}
		if !configured || payload.State == "attention" && !active || payload.State == "resolved" && status != "resolved" && status != "closed" || payload.Severity != severity {
			state = "cancelled"
		} else {
			due = p.Due(e.Now(), payload.Urgent, 0)
			if !due.After(e.Now()) {
				sendCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
				err = e.Sender.Send(sendCtx, token, payload)
				cancel()
				attempts++
				switch {
				case err == nil:
					state = "sent"
				case errors.Is(err, ErrUnregistered):
					state = "failed"
					code = "unregistered"
					if _, err = tx.ExecContext(ctx, `UPDATE notification_devices SET enabled=false WHERE device_id=$1`, device); err != nil {
						return false, err
					}
				default:
					code = "delivery_failed"
					if attempts >= 8 {
						state = "failed"
					}
					due = e.Now().Add(time.Duration(1<<min(attempts, 10)) * 30 * time.Second)
					var retry *RetryError
					if errors.As(err, &retry) && e.Now().Add(retry.After).After(due) {
						due = e.Now().Add(retry.After)
					}
				}
			}
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE notification_deliveries SET state=$2,error_code=$3,attempts=$4,due_at=$5 WHERE id=$1`, id, state, code, attempts, due); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
