package api

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"prisme.local/platform/internal/inventory"
	"prisme.local/platform/internal/pki"
)

func (s *Server) fleetRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /v1/servers", s.auth(s.servers))
	m.HandleFunc("GET /v1/servers/{id}", s.auth(s.serverDetail))
	m.HandleFunc("PUT /v1/servers/{id}/capabilities", s.auth(s.setCapabilities))
	m.HandleFunc("DELETE /v1/servers/{id}", s.auth(s.revokeAgent))
	m.HandleFunc("POST /v1/agent/renew", s.renewAgent)
	m.HandleFunc("POST /v1/agent/inventory", s.agentInventory)
	m.HandleFunc("POST /v1/inventory-imports", s.auth(s.importInventory))
	m.HandleFunc("GET /v1/audit", s.auth(s.auditEvents))
}
func validCapabilities(caps []string) bool {
	if len(caps) < 1 || len(caps) > 3 {
		return false
	}
	seen := map[string]bool{}
	for _, c := range caps {
		if seen[c] || (c != "self:heartbeat" && c != "inventory:host" && c != "inventory:docker") {
			return false
		}
		seen[c] = true
	}
	return seen["self:heartbeat"] && (!seen["inventory:docker"] || seen["inventory:host"])
}
func contains(caps []string, cap string) bool {
	for _, v := range caps {
		if v == cap {
			return true
		}
	}
	return false
}

func (s *Server) servers(w http.ResponseWriter, r *http.Request, _ string) { s.queryServers(w, r, "") }
func (s *Server) serverDetail(w http.ResponseWriter, r *http.Request, _ string) {
	id := r.PathValue("id")
	if !opaque.MatchString(id) {
		problem(w, 400, "invalid_id")
		return
	}
	s.queryServers(w, r, id)
}
func (s *Server) queryServers(w http.ResponseWriter, r *http.Request, id string) {
	cursor := r.URL.Query().Get("cursor")
	if cursor != "" && !opaque.MatchString(cursor) {
		problem(w, 400, "invalid_cursor")
		return
	}
	rows, err := s.DB.QueryContext(r.Context(), `SELECT a.id,a.name,a.last_seen_at,a.expires_at,a.revoked,a.capabilities,i.document
 FROM agents a LEFT JOIN server_inventory i ON i.agent_id=a.id WHERE a.id>$1 AND ($2='' OR a.id=$2) ORDER BY a.id LIMIT 51`, cursor, id)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	next := ""
	bytesInPage := 0
	for rows.Next() {
		var sid, name string
		var seen sql.NullTime
		var expires time.Time
		var revoked bool
		var caps, doc []byte
		if err = rows.Scan(&sid, &name, &seen, &expires, &revoked, &caps, &doc); err != nil {
			problem(w, 503, "unavailable")
			return
		}
		if len(items) == 50 || len(items) > 0 && bytesInPage+len(doc) > 4*1024*1024 {
			next = items[len(items)-1]["id"].(string)
			break
		}
		bytesInPage += len(doc)
		state := "unknown"
		var last any
		if seen.Valid {
			last = seen.Time
			state = "online"
			if s.Now().Sub(seen.Time) > 90*time.Second {
				state = "offline"
			}
		}
		if !s.Now().Before(expires) {
			state = "expired"
		}
		if revoked {
			state = "revoked"
		}
		var document any
		if doc != nil {
			document = json.RawMessage(doc)
		}
		items = append(items, map[string]any{"id": sid, "name": name, "last_seen_at": last, "expires_at": expires, "revoked": revoked, "capabilities": json.RawMessage(caps), "connection_state": state, "inventory": document})
	}
	if rows.Err() != nil {
		problem(w, 503, "unavailable")
		return
	}
	if id != "" {
		if len(items) == 0 {
			problem(w, 404, "server_not_found")
			return
		}
		send(w, 200, items[0])
		return
	}
	send(w, 200, map[string]any{"items": items, "next_cursor": next, "observed_at": s.Now()})
}
func (s *Server) setCapabilities(w http.ResponseWriter, r *http.Request, hash string) {
	if !s.recent(w, r, hash) {
		return
	}
	var req struct {
		Capabilities []string `json:"capabilities"`
	}
	if !body(w, r, &req) {
		return
	}
	if !opaque.MatchString(r.PathValue("id")) || !validCapabilities(req.Capabilities) {
		problem(w, 400, "invalid_capabilities")
		return
	}
	raw, _ := json.Marshal(req.Capabilities)
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), "UPDATE agents SET capabilities=$1 WHERE id=$2 AND NOT revoked", string(raw), r.PathValue("id"))
	if err == nil {
		n, _ := result.RowsAffected()
		if n != 1 {
			problem(w, 404, "server_not_found")
			return
		}
		err = audit(r.Context(), tx, "administrator", "agent.capabilities_changed", r.PathValue("id"))
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 200, req)
}
func (s *Server) revokeAgent(w http.ResponseWriter, r *http.Request, hash string) {
	if !s.recent(w, r, hash) {
		return
	}
	id := r.PathValue("id")
	if !opaque.MatchString(id) {
		problem(w, 400, "invalid_id")
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), "UPDATE agents SET revoked=true,previous_serial=NULL,previous_until=NULL WHERE id=$1", id)
	if err == nil {
		n, _ := result.RowsAffected()
		if n != 1 {
			problem(w, 404, "server_not_found")
			return
		}
		err = audit(r.Context(), tx, "administrator", "agent.revoked", id)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	w.WriteHeader(204)
}

// authenticateAgent locks the identity until mutation commit so revocation and capability reduction serialize.
func (s *Server) authenticateAgent(w http.ResponseWriter, r *http.Request, tx *sql.Tx, capability string) (string, []string, bool) {
	if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) == 0 {
		problem(w, 401, "agent_certificate_required")
		return "", nil, false
	}
	cert := r.TLS.PeerCertificates[0]
	var raw []byte
	err := tx.QueryRowContext(r.Context(), `SELECT capabilities FROM agents WHERE id=$1 AND NOT revoked AND expires_at>$2 AND (serial=$3 OR previous_serial=$3 AND previous_until>$2) FOR UPDATE`, cert.Subject.CommonName, s.Now(), cert.SerialNumber.String()).Scan(&raw)
	if err != nil {
		if err == sql.ErrNoRows {
			problem(w, 403, "agent_not_authorized")
		} else {
			problem(w, 503, "unavailable")
		}
		return "", nil, false
	}
	var caps []string
	if json.Unmarshal(raw, &caps) != nil {
		problem(w, 503, "unavailable")
		return "", nil, false
	}
	if !contains(caps, capability) {
		problem(w, 403, "capability_required")
		return "", nil, false
	}
	return cert.Subject.CommonName, caps, true
}
func (s *Server) renewAgent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CSR string `json:"csr_pem"`
	}
	if !body(w, r, &req) {
		return
	}
	csr, err := pki.ParseCSR(req.CSR)
	if err != nil {
		problem(w, 400, "invalid_csr")
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	id, caps, ok := s.authenticateAgent(w, r, tx, "self:heartbeat")
	if !ok {
		return
	}
	var serial, cert string
	var expires time.Time
	if err = tx.QueryRowContext(r.Context(), "SELECT serial,certificate_pem,expires_at FROM agents WHERE id=$1", id).Scan(&serial, &cert, &expires); err != nil {
		problem(w, 503, "unavailable")
		return
	}
	// A previous identity may only recover the already-issued certificate for the same key.
	peer := r.TLS.PeerCertificates[0]
	if !bytesEqualKey(csr.RawSubjectPublicKeyInfo, peer.RawSubjectPublicKeyInfo) {
		problem(w, 400, "renewal_key_mismatch")
		return
	}
	if serial != peer.SerialNumber.String() || expires.Sub(s.Now()) > 8*time.Hour {
		send(w, 200, map[string]any{"agent_id": id, "certificate_pem": cert, "expires_at": expires, "protocol_version": "1", "capabilities": caps})
		return
	}
	cert, newSerial, expires, err := s.CA.Issue(id, csr, s.Now())
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "UPDATE agents SET previous_serial=serial,previous_until=$1,serial=$2,certificate_pem=$3,expires_at=$4 WHERE id=$5", s.Now().Add(10*time.Minute), newSerial, cert, expires, id)
	}
	if err == nil {
		err = audit(r.Context(), tx, id, "agent.certificate_renewed", id)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 200, map[string]any{"agent_id": id, "certificate_pem": cert, "expires_at": expires, "protocol_version": "1", "capabilities": caps})
}
func bytesEqualKey(a, b []byte) bool { return string(a) == string(b) }

func inventoryBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Header.Get("Content-Type") != "application/json" {
		problem(w, 415, "content_type")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, inventory.MaxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(v) != nil {
		problem(w, 400, "invalid_inventory")
		return false
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		problem(w, 400, "invalid_inventory")
		return false
	}
	return true
}
func (s *Server) saveInventory(r *http.Request, tx *sql.Tx, id string, snapshot inventory.Snapshot) error {
	snapshot.Normalize()
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(r.Context(), `INSERT INTO server_inventory(agent_id,observed_at,document) VALUES($1,$2,$3)
 ON CONFLICT(agent_id) DO UPDATE SET observed_at=EXCLUDED.observed_at,received_at=now(),document=EXCLUDED.document WHERE server_inventory.observed_at<EXCLUDED.observed_at`, id, snapshot.ObservedAt, string(raw))
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return nil
	}
	_, err = tx.ExecContext(r.Context(), "UPDATE agents SET last_seen_at=$1 WHERE id=$2", s.Now(), id)
	if err == nil {
		err = audit(r.Context(), tx, id, "inventory.observed", id)
	}
	return err
}
func (s *Server) agentInventory(w http.ResponseWriter, r *http.Request) {
	var snapshot inventory.Snapshot
	if !inventoryBody(w, r, &snapshot) {
		return
	}
	if snapshot.Validate(s.Now()) != nil || snapshot.Source != "agent" {
		problem(w, 400, "invalid_inventory")
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	id, caps, ok := s.authenticateAgent(w, r, tx, "inventory:host")
	if !ok {
		return
	}
	if (snapshot.DockerAvailable || len(snapshot.Containers)+len(snapshot.Images)+len(snapshot.Networks)+len(snapshot.Volumes) > 0) && !contains(caps, "inventory:docker") {
		problem(w, 403, "capability_required")
		return
	}
	err = s.saveInventory(r, tx, id, snapshot)
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 200, map[string]string{"status": "accepted"})
}
func (s *Server) importInventory(w http.ResponseWriter, r *http.Request, hash string) {
	if !s.recent(w, r, hash) {
		return
	}
	var req struct {
		Name     string             `json:"name"`
		Snapshot inventory.Snapshot `json:"snapshot"`
	}
	if !inventoryBody(w, r, &req) {
		return
	}
	if !validName(req.Name) || req.Snapshot.Source != "ssh-readonly" || req.Snapshot.Validate(s.Now()) != nil {
		problem(w, 400, "invalid_inventory")
		return
	}
	id := Hash("ssh-readonly:" + req.Name + ":" + req.Snapshot.Host.Hostname)
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(r.Context(), `INSERT INTO agents(id,name,csr_hash,certificate_pem,serial,enrolled_at,expires_at,capabilities) VALUES($1,$2,'','','import-'||$1,$3,$4,'["inventory:host","inventory:docker"]') ON CONFLICT(id) DO NOTHING`, id, req.Name, s.Now(), s.Now().AddDate(100, 0, 0))
	var revoked bool
	if err == nil {
		err = tx.QueryRowContext(r.Context(), "SELECT revoked FROM agents WHERE id=$1 FOR UPDATE", id).Scan(&revoked)
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	if revoked {
		problem(w, 403, "server_revoked")
		return
	}
	err = s.saveInventory(r, tx, id, req.Snapshot)
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 201, map[string]string{"server_id": id})
}
func (s *Server) auditEvents(w http.ResponseWriter, r *http.Request, _ string) {
	cursor := r.URL.Query().Get("cursor")
	var before int64 = 9223372036854775807
	if cursor != "" {
		var err error
		before, err = strconv.ParseInt(cursor, 10, 64)
		if err != nil || before < 1 {
			problem(w, 400, "invalid_cursor")
			return
		}
	}
	rows, err := s.DB.QueryContext(r.Context(), "SELECT id,occurred_at,actor,action,target,device_id,origin,result,correlation_id,environment,remote_address FROM audit_events WHERE id<$1 ORDER BY id DESC LIMIT 51", before)
	if err != nil {
		problem(w, 503, "unavailable")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	next := ""
	for rows.Next() {
		var id int64
		var at time.Time
		var actor, action, target, device, origin, result, correlation, environment, address string
		if err = rows.Scan(&id, &at, &actor, &action, &target, &device, &origin, &result, &correlation, &environment, &address); err != nil {
			problem(w, 503, "unavailable")
			return
		}
		if len(items) == 50 {
			next = items[49]["id"].(string)
			break
		}
		items = append(items, map[string]any{"id": strconv.FormatInt(id, 10), "occurred_at": at, "actor": actor, "action": action, "target": target, "device_id": device, "origin": origin, "result": result, "correlation_id": correlation, "environment": environment, "remote_address": address})
	}
	if rows.Err() != nil {
		problem(w, 503, "unavailable")
		return
	}
	send(w, 200, map[string]any{"items": items, "next_cursor": next})
}
