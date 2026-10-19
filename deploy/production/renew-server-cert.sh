#!/bin/sh
# ──────────────────────────────────────────────────────────────────
# Renouvelle le seul certificat TLS interne de l'API (397 jours) depuis la CA
# existante, puis redémarre l'API. La CA, son empreinte, le bloc Caddy et les
# identités d'agents ne changent pas. Aucune clé n'est affichée.
#
#   cd /opt/projects/prisme && sudo sh deploy/production/renew-server-cert.sh
# ──────────────────────────────────────────────────────────────────
set -eu
umask 077
root=$(cd "$(dirname "$0")/../.." && pwd)
dir="$root/secrets/pki"
compose() {
	docker compose --env-file "$root/deploy/production/ovh-mantara.env" \
		-f "$root/deploy/production/compose.yml" "$@"
}

[ "$(id -u)" -eq 0 ] || { echo "À lancer avec sudo." >&2; exit 1; }
[ -f "$dir/ca-key.pem" ] || { echo "CA absente : lancer d'abord init-secrets.sh." >&2; exit 1; }

compose --profile tools build pki
compose --profile tools run --rm --no-deps pki -dir /secrets renew-server
chown root:10001 "$dir/server.pem" "$dir/server-key.pem"
chmod 0640 "$dir/server.pem" "$dir/server-key.pem"
compose up -d --force-recreate --no-deps --wait api
echo "Certificat serveur renouvelé et API redémarrée."
