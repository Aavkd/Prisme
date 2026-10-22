# Lecture seule : collecteur SSH, ssh.prom, séries SSH/fail2ban, état des règles, incidents ouverts.
# ssh -o BatchMode=yes ovh-mantara 'bash -s' < deploy/production/ops/verif-ssh.sh
cd /opt/projects/prisme
sql() { sudo docker compose --env-file deploy/production/ovh-mantara.env -f deploy/production/compose.yml exec -T postgres psql -U prisme -d prisme -At -c "$1" </dev/null; }
Q() { docker run --rm --network prisme_control curlimages/curl:8.17.0 -s -G 'http://victoriametrics:8428/api/v1/query' --data-urlencode "query=$1"; }
echo "== Minuteurs"; systemctl --no-pager --plain list-timers 'prisme-*' | head -5
echo "== Dernières exécutions du collecteur SSH"; sudo journalctl -u prisme-ssh-metrics.service -n 6 --no-pager -q -o short-iso | cut -c1-200
echo "== ssh.prom"; ls -l /var/lib/prisme-metrics/; grep -v '^#' /var/lib/prisme-metrics/ssh.prom
echo "== VictoriaMetrics (portée hôte)"
for m in prisme_ssh_failures_10m prisme_ssh_logins_30m prisme_ssh_unknown_logins_30m prisme_fail2ban_up prisme_fail2ban_banned prisme_fail2ban_found_10m prisme_fail2ban_blind prisme_ssh_drill prisme_ssh_metrics_generated_seconds; do
  printf '%-40s ' "$m"; Q "$m{server=\"ovh-mantara\",service=\"host\"}" | python3 -c "import json,sys; r=json.load(sys.stdin)['data']['result']; print(len(r), 'série(s)', [x['value'][1] for x in r])"
done
echo "== Règles"
echo "nombre : $(sql "SELECT count(*) FROM observation_rule_state")"
sql "SELECT document->>'signal', count(*) FROM observation_rule_state GROUP BY 1 ORDER BY 1"
sql "SELECT rule_id || ' ' || coalesce(document->>'signal','?') || ' ' || coalesce(document->>'value','-') FROM observation_rule_state WHERE rule_id LIKE 'VPS-SSH%' OR rule_id LIKE 'VPS-FAIL2BAN%' ORDER BY 1"
echo "règles non saines :"; sql "SELECT rule_id || ' ' || (document->>'signal') FROM observation_rule_state WHERE document->>'signal' <> 'healthy' ORDER BY 1"
echo "incidents ouverts : $(sql "SELECT count(*) FROM incidents WHERE status <> 'resolved'")"
free -m | awk '/^Mem:/{print "RAM disponible", $7, "Mo"} /^Swap:/{print "Swap utilisé", $3, "Mo"}'
