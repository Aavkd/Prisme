#!/bin/sh
# ──────────────────────────────────────────────────────────────────
# Installe sur l'hôte les deux collecteurs de Prisme (étape 4).
#
#   cd /opt/projects/prisme && sudo sh deploy/production/install-host.sh
#
# - groupe prisme-observation (GID 10002) et compte système prisme-collector
#   (groupes docker et prisme-observation, sans shell ni dossier personnel) ;
# - /usr/local/bin/prisme-inventory, compilé depuis ce dépôt (image host-tools) ;
# - /usr/local/lib/prisme/host_metrics.py et le dossier /etc/prisme (exercices) ;
# - unités prisme-inventory.{service,timer} (inventaire pour l'agent, 60 s) et
#   prisme-host-metrics.{service,timer} (état des conteneurs pour Alloy, 30 s).
# Le groupe docker est un privilège administratif : il ne sert qu'à deux
# programmes fixes qui n'appellent que `docker ps` et `docker inspect`.
# Relançable : met à jour binaire, script et unités ; refuse un GID différent.
# ──────────────────────────────────────────────────────────────────
set -eu
root=$(cd "$(dirname "$0")/../.." && pwd)
host="$root/deploy/production/host"
[ "$(id -u)" -eq 0 ] || { echo "À lancer avec sudo." >&2; exit 1; }

if getent group prisme-observation >/dev/null; then
	[ "$(getent group prisme-observation | cut -d: -f3)" = 10002 ] || { echo "Refus : prisme-observation existe avec un autre GID." >&2; exit 1; }
else
	groupadd --system --gid 10002 prisme-observation
fi
if ! id prisme-collector >/dev/null 2>&1; then
	useradd --system --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin \
		--gid prisme-observation --groups docker prisme-collector
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
docker build -q --target host-tools --output "type=local,dest=$tmp" -f "$root/deploy/Dockerfile" "$root" >/dev/null
install -m 0755 -o root -g root "$tmp/prisme-inventory" /usr/local/bin/prisme-inventory
install -d -m 0755 -o root -g root /usr/local/lib/prisme /etc/prisme
install -m 0644 -o root -g root "$host/host_metrics.py" /usr/local/lib/prisme/host_metrics.py
for unit in prisme-inventory.service prisme-inventory.timer prisme-host-metrics.service prisme-host-metrics.timer; do
	install -m 0644 -o root -g root "$host/$unit" "/etc/systemd/system/$unit"
done
systemctl daemon-reload
systemctl enable --now prisme-inventory.timer prisme-host-metrics.timer
# First run now, so the files exist before Alloy and the agent read them.
systemctl start prisme-inventory.service prisme-host-metrics.service
echo "Collecteurs installés :"
systemctl --no-pager --plain list-timers 'prisme-*' | head -4
ls -l /var/lib/prisme-observation/inventory.json /var/lib/prisme-metrics/host.prom
