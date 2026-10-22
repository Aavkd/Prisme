# Lecture seule. Usage : ssh -o BatchMode=yes ovh-mantara 'bash -s' < deploy/production/ops/codes-sites.sh
# Code HTTP de chaque site du Caddy mutualisé, joint en local (sans DNS) : base de comparaison.
grep -E '^[a-z0-9.-]+\.[a-z]+( [a-z0-9., -]+)? *\{$' /opt/projects/edge/Caddyfile | sed 's/ *{$//' | tr ', ' '\n\n' | grep -v '^$' | sort -u | while read -r d; do
  printf '%s %s\n' "$d" "$(curl -s -o /dev/null -m 8 -w '%{http_code}' --resolve "$d:443:127.0.0.1" "https://$d/")"
done
