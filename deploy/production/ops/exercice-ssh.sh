# MODIFIE le VPS (drapeaux dans /etc/prisme) : accord d'Alexy requis. Exercice SSH, ≈ 25 min :
# quatre incidents ouverts puis résolus. Avec les notifications actives, deux sont URGENTS.
# ssh -o BatchMode=yes ovh-mantara 'bash -s' < deploy/production/ops/exercice-ssh.sh (en arrière-plan)
# Quatre drapeaux qui ne font que lever des alarmes ; SSH et fail2ban ne sont pas touchés.
cd /opt/projects/prisme
sql() { sudo docker compose --env-file deploy/production/ovh-mantara.env -f deploy/production/compose.yml exec -T postgres psql -U prisme -d prisme -At -c "$1" </dev/null; }
now() { python3 -c 'import datetime;print(datetime.datetime.now(datetime.timezone.utc).strftime("%H:%M:%S"))'; }
RULES="'VPS-SSH-UNKNOWN-LOGIN','VPS-FAIL2BAN-DOWN','VPS-SSH-FAILURES','VPS-FAIL2BAN-BLIND'"
FLAGS="/etc/prisme/drill-ssh-unknown-login /etc/prisme/drill-fail2ban-down /etc/prisme/drill-ssh-failures /etc/prisme/drill-fail2ban-blind"
before=$(sql "SELECT coalesce(max(id),0) FROM incidents")
state() { sql "SELECT rule_id || ' ' || status || ' ouvert ' || to_char(opened_at,'HH24:MI:SS') || coalesce(' résolu ' || to_char(resolved_at,'HH24:MI:SS'),'') FROM incidents WHERE rule_id IN ($RULES) AND id > $before ORDER BY rule_id"; }
opened() { sql "SELECT count(DISTINCT rule_id) FROM incidents WHERE rule_id IN ($RULES) AND id > $before"; }
resolved() { sql "SELECT count(DISTINCT rule_id) FROM incidents WHERE rule_id IN ($RULES) AND id > $before AND status = 'resolved'"; }

sudo touch $FLAGS
echo "$(now) drapeaux créés"
for i in $(seq 1 120); do [ "$(opened)" = 4 ] && break; sleep 10; done
echo "$(now) incidents ouverts : $(opened)/4"; state
sudo rm -f $FLAGS
echo "$(now) drapeaux supprimés"
for i in $(seq 1 60); do [ "$(resolved)" = 4 ] && break; sleep 10; done
echo "$(now) incidents résolus : $(resolved)/4"; state
echo "événements :"; sql "SELECT i.rule_id || ' : ' || string_agg(e.kind, ' ' ORDER BY e.id) FROM incident_events e JOIN incidents i ON i.id=e.incident_id WHERE i.rule_id IN ($RULES) AND i.id > $before GROUP BY i.rule_id ORDER BY 1"
echo "reste dans /etc/prisme :"; ls /etc/prisme
free -m | awk '/^Mem:/{print "RAM disponible", $7, "Mo"} /^Swap:/{print "Swap utilisé", $3, "Mo"}'
