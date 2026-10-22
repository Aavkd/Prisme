# Lecture seule : séries de VictoriaMetrics (sites, disque, swap, conteneurs, collecte hôte).
# ssh -o BatchMode=yes ovh-mantara 'bash -s' < deploy/production/ops/verif-collecte.sh
q() { docker run --rm --network prisme_control curlimages/curl:8.17.0 -s -G 'http://victoriametrics:8428/api/v1/query' --data-urlencode "query=$1"; }
show() { printf '%-58s ' "$1"; q "$2" | python3 -c "import json,sys; r=json.load(sys.stdin)['data']['result']; print(len(r), 'série(s)', sorted(set(round(float(x['value'][1]),1) for x in r))[:6])"; }
show 'probe_success (28 sites)' 'probe_success{environment="production",service="web"}'
show 'sites en échec' 'probe_success{service="web"} == 0'
show 'jours avant expiration TLS (min)' 'min((probe_ssl_earliest_cert_expiry{service="web"} - time()) / 86400)'
show 'node_filesystem_avail_bytes{service="host"}' 'node_filesystem_avail_bytes{server="ovh-mantara",service="host"}'
show 'node_memory_SwapFree_bytes' 'node_memory_SwapFree_bytes{server="ovh-mantara",service="host"}'
show 'prisme_containers_problem' 'prisme_containers_problem{server="ovh-mantara",service="host"}'
show 'âge collecte hôte (s)' 'time() - prisme_host_metrics_generated_seconds{service="host"}'
show 'prisme_drill' 'prisme_drill{service="host"}'
show 'container_memory_usage_bytes (conteneurs)' 'count by (name) (container_memory_usage_bytes{server="ovh-mantara"})'
show 'doublons cAdvisor (même nom)' 'count by (name) (container_memory_usage_bytes{server="ovh-mantara"}) > 1'
docker run --rm --network prisme_control curlimages/curl:8.17.0 -s -G 'http://victoriametrics:8428/api/v1/query' --data-urlencode 'query=probe_success{service="web"} == 0' | python3 -c "import json,sys; [print('  échec:', x['metric'].get('project')) for x in json.load(sys.stdin)['data']['result']]"
