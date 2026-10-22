# Lecture seule : faux positifs des règles SSH depuis une date (défaut : 24 h).
# ssh -o BatchMode=yes ovh-mantara 'bash -s' < deploy/production/ops/observation-ssh.sh
# ssh -o BatchMode=yes ovh-mantara 'bash -s -- "2026-10-22 16:00"' < deploy/production/ops/observation-ssh.sh
cd /opt/projects/prisme
since=${1:-$(python3 -c 'import datetime;print((datetime.datetime.now(datetime.timezone.utc)-datetime.timedelta(days=1)).strftime("%Y-%m-%d %H:%M"))')}
sql() { sudo docker compose --env-file deploy/production/ovh-mantara.env -f deploy/production/compose.yml exec -T postgres psql -U prisme -d prisme -At -c "$1" </dev/null; }
echo "== Depuis le $since UTC"
echo "== Incidents des règles SSH et fail2ban"
sql "SELECT rule_id || ' ' || status || ' ouvert ' || to_char(opened_at,'DD/MM HH24:MI') || coalesce(' résolu ' || to_char(resolved_at,'HH24:MI'),'') FROM incidents WHERE (rule_id LIKE 'VPS-SSH%' OR rule_id LIKE 'VPS-FAIL2BAN%') AND opened_at > '$since+00' ORDER BY id"
echo "total : $(sql "SELECT count(*) FROM incidents WHERE (rule_id LIKE 'VPS-SSH%' OR rule_id LIKE 'VPS-FAIL2BAN%') AND opened_at > '$since+00'")"
echo "== Connexions décrites comme inconnues par le collecteur (adresse, compte, clé)"
u=$(sudo journalctl -u prisme-ssh-metrics.service --since "$since" --no-pager -q -o short-iso | grep 'unknown SSH login' | cut -c1-200); echo "${u:-aucune}"
echo "== Erreurs de collecte"
sudo journalctl -u prisme-ssh-metrics.service --since "$since" --no-pager -q -p err | tail -5; echo "(fin)"
echo "== Échecs SSH sur 10 min : maximum hors exercices (24 h ; seuil d'alerte 150)"
docker run --rm --network prisme_control curlimages/curl:8.17.0 -s -G 'http://victoriametrics:8428/api/v1/query' --data-urlencode 'query=max_over_time((prisme_ssh_failures_10m{server="ovh-mantara",service="host"} unless on(server,service) (prisme_ssh_drill{server="ovh-mantara",service="host"} > 0))[24h:30s])' | python3 -c "import json,sys; print([x['value'][1] for x in json.load(sys.stdin)['data']['result']])"
echo "== fail2ban"; sudo fail2ban-client status sshd | grep -E 'Currently|Total'
echo "== Règles non saines"; sql "SELECT rule_id || ' ' || (document->>'signal') FROM observation_rule_state WHERE document->>'signal' <> 'healthy' ORDER BY 1"
free -m | awk '/^Mem:/{print "RAM disponible", $7, "Mo"} /^Swap:/{print "Swap utilisé", $3, "Mo"}'
