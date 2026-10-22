# MODIFIE le VPS (crée puis supprime /etc/prisme/drill) : accord d'Alexy requis.
# Ouvre puis résout l'incident PRISME-DRILL (information, non urgent), sans toucher à aucun service.
# ssh -o BatchMode=yes ovh-mantara 'bash -s' < deploy/production/ops/exercice-drill.sh
cd /opt/projects/prisme
sql() { sudo docker compose --env-file deploy/production/ovh-mantara.env -f deploy/production/compose.yml exec -T postgres psql -U prisme -d prisme -At -c "$1" </dev/null; }
before=$(sql "SELECT coalesce(max(id),0) FROM incidents")
echo "incidents avant : $(sql "SELECT count(*) FROM incidents")"
sudo touch /etc/prisme/drill
echo "$(date -u +%T) drapeau créé"
for i in $(seq 1 30); do
  s=$(sql "SELECT id || ' ' || status || ' ' || to_char(opened_at,'HH24:MI:SS') FROM incidents WHERE rule_id='PRISME-DRILL' AND id > $before ORDER BY id DESC LIMIT 1")
  [ -n "$s" ] && { echo "$(date -u +%T) incident ouvert : $s"; break; }
  sleep 5
done
sudo rm -f /etc/prisme/drill
echo "$(date -u +%T) drapeau supprimé"
for i in $(seq 1 40); do
  s=$(sql "SELECT status || ' ' || coalesce(to_char(resolved_at,'HH24:MI:SS'),'-') FROM incidents WHERE rule_id='PRISME-DRILL' AND id > $before ORDER BY id DESC LIMIT 1")
  case "$s" in resolved*) echo "$(date -u +%T) incident : $s"; break;; esac
  sleep 5
done
echo "événements : $(sql "SELECT string_agg(e.kind, ' ' ORDER BY e.id) FROM incident_events e JOIN incidents i ON i.id=e.incident_id WHERE i.rule_id='PRISME-DRILL' AND i.id > $before")"
ls /etc/prisme
