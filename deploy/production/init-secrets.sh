#!/bin/sh
# ──────────────────────────────────────────────────────────────────
# Crée UNE FOIS les secrets de production de Prisme, sans jamais les afficher.
#
#   cd /opt/projects/prisme && sudo sh deploy/production/init-secrets.sh
#
# Résultat : /opt/projects/prisme/secrets (root:10001, dossiers 0750, fichiers
# 0640), lisible par root et par les conteneurs Prisme (UID/GID 10001) seulement.
#   db-password, identity-secret   64 caractères hexadécimaux aléatoires
#   pki/ca.pem, ca-key.pem          CA privée de Prisme (5 ans)
#   pki/server.pem, server-key.pem  certificat de l'API (prisme-api, localhost ; 397 j)
# Refuse de s'exécuter si le dossier existe : rien n'est jamais remplacé ici.
# Renouveler le seul certificat serveur : voir LISEZMOI.md.
# ──────────────────────────────────────────────────────────────────
set -eu
umask 077
root=$(cd "$(dirname "$0")/../.." && pwd)
dir="$root/secrets"
compose() {
	docker compose --env-file "$root/deploy/production/ovh-mantara.env" \
		-f "$root/deploy/production/compose.yml" --profile tools "$@"
}

[ "$(id -u)" -eq 0 ] || { echo "À lancer avec sudo." >&2; exit 1; }
[ ! -e "$dir" ] || { echo "Refus : $dir existe déjà, rien n'est remplacé." >&2; exit 1; }

mkdir -m 0700 "$dir" "$dir/pki"
for f in db-password identity-secret; do
	od -An -N32 -tx1 /dev/urandom | tr -d ' \n' > "$dir/$f"
done
compose build pki
compose run --rm --no-deps pki -dir /secrets init

chown -R root:10001 "$dir"
find "$dir" -type d -exec chmod 0750 {} +
find "$dir" -type f -exec chmod 0640 {} +
echo "Secrets créés dans $dir (root:10001). Aucun n'a été affiché."
