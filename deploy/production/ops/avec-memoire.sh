# Usage : ssh ovh-mantara 'bash -s -- "<commande>"' < deploy/production/ops/avec-memoire.sh
# Lance la commande passée en argument en relevant la RAM disponible (Mo) et le swap utilisé.
log=$(mktemp)
( while :; do free -m | awk '/^Mem:/{m=$7} /^Swap:/{s=$3} END{print m, s}'; sleep 2; done ) > "$log" &
sampler=$!
sh -c "$1" < /dev/null
rc=$?
kill "$sampler" 2>/dev/null
echo "RAM disponible min: $(awk '{print $1}' "$log" | sort -n | head -1) Mo ; swap max: $(awk '{print $2}' "$log" | sort -n | tail -1) Mo ; échantillons: $(wc -l < "$log")"
rm -f "$log"
exit $rc
