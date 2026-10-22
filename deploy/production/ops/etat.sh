# Lecture seule : état général de Prisme sur le VPS, sans aucun secret (des nombres seulement).
# ssh -o BatchMode=yes ovh-mantara 'bash -s' < deploy/production/ops/etat.sh
cd /opt/projects/prisme
sql() { sudo docker compose --env-file deploy/production/ovh-mantara.env -f deploy/production/compose.yml exec -T postgres psql -U prisme -d prisme -At -c "$1" </dev/null; }
echo "== $(python3 -c 'import datetime;print(datetime.datetime.now(datetime.timezone.utc).strftime("%d/%m/%Y %H:%M"))') UTC ; VPS démarré depuis $(uptime -p | sed 's/^up //') ; redémarrage en attente : $([ -f /var/run/reboot-required ] && echo OUI || echo non)"
free -m | awk '/^Mem:/{m=$7} /^Swap:/{s=$3} END{print "RAM disponible", m, "Mo ; swap utilisé", s, "Mo"}'
df -h / | awk 'NR==2{print "Disque / :", $5, "utilisé,", $4, "libres"}'
echo "== Clone : $(git log --oneline -1)"
echo "== Pile Prisme"; sudo docker compose --env-file deploy/production/ovh-mantara.env -f deploy/production/compose.yml ps --format '{{.Service}} {{.Status}}'
echo "notifications actives dans l'API : $(sudo docker inspect prisme-api-1 --format '{{range .Config.Env}}{{println .}}{{end}}' | grep -c '^PRISME_NOTIFICATION_CONFIG=')"
echo "clé FCM de l'API présente : $(sudo test -f secrets/fcm-api.json && echo oui || echo non)"
echo "== Conteneurs actifs : $(docker ps -q | wc -l) ; hors service : $(docker ps -a --filter status=exited --filter status=dead --format '{{.Names}}' | grep -v '^prisme-migrate-1$' | tr '\n' ' ')"
echo "== Identité et appareils"
echo "passkeys : $(sql "SELECT count(*) FROM passkeys") ; appareils actifs : $(sql "SELECT count(*) FROM devices WHERE NOT revoked") ; abonnés aux notifications : $(sql "SELECT count(*) FROM notification_devices WHERE enabled") ; agents enrôlés : $(sql "SELECT count(*) FROM agents")"
echo "== Règles : $(sql "SELECT count(*) FROM observation_rule_state") ; non saines : $(sql "SELECT coalesce(string_agg(rule_id || ' ' || (document->>'signal'), ', '), 'aucune') FROM observation_rule_state WHERE document->>'signal' <> 'healthy'")"
echo "== Incidents ouverts : $(sql "SELECT count(*) FROM incidents WHERE status NOT IN ('resolved','closed')") ; ouverts ces 7 derniers jours : $(sql "SELECT count(*) FROM incidents WHERE opened_at > now() - interval '7 days'")"
sql "SELECT '  ' || rule_id || ' ' || status || ' ' || to_char(opened_at,'DD/MM HH24:MI') FROM incidents WHERE opened_at > now() - interval '7 days' ORDER BY id DESC LIMIT 10"
echo "== Notifications envoyées ces 7 derniers jours : $(sql "SELECT coalesce(string_agg(state || ' ' || n, ', '), 'aucune') FROM (SELECT state, count(*) n FROM notification_deliveries WHERE created_at > now() - interval '7 days' GROUP BY 1) x")"
echo "== fail2ban"; sudo fail2ban-client status sshd | grep -E 'Currently banned|Total banned'
echo "== Minuteurs"; systemctl --no-pager --plain list-timers 'prisme-*' | awk '$NF ~ /\.service$/ {print "  " $NF}'
