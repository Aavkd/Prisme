"""Exercise only the named local pilot through the real TLS API and Alloy stores.

Requires the local observability Compose overlay. Always recovers its fixture.
No production host, pilot repository, or cloud service is modified.
"""
import datetime as dt
import json
from pathlib import Path
import ssl
import subprocess
import time
import urllib.parse
import urllib.request

ROOT = Path(__file__).resolve().parent.parent
BASE = 'https://localhost:8443'
COMPOSE = ['docker','compose','-f','deploy/compose.dev.yml','-f','deploy/compose.observability.yml']
# Match the Go/Flutter trust policy for the existing development CA, whose older
# certificates omit AKI. Chain, hostname, expiry and TLS verification stay enabled.
context = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
context.load_verify_locations(cafile=str(ROOT / '.dev/pki/ca.pem'))
token = None

def api(path, body=None, method=None):
    headers = {'Content-Type':'application/json'}
    if token:
        headers['Authorization'] = 'Bearer ' + token
    request = urllib.request.Request(BASE+path, data=None if body is None else json.dumps(body).encode(), headers=headers, method=method)
    with urllib.request.urlopen(request, context=context, timeout=8) as response:
        if response.status == 204:
            return None
        return json.load(response)

def wait_for(label, predicate, seconds=120):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        value = predicate()
        if value:
            print(label, flush=True)
            return value
        time.sleep(3)
    raise RuntimeError('Timed out: ' + label)

def pilot(flag):
    subprocess.run(COMPOSE+['exec','-T','pilot','/pilot',flag], cwd=ROOT, check=True, capture_output=True)

def incident(statuses):
    for item in api('/v1/incidents')['items']:
        if item['rule_id'] == 'LOCAL-PILOT-HTTP' and item['status'] in statuses:
            return item
    return None

def main():
    global token
    token = api('/v1/dev/sessions', {'secret':(ROOT/'.dev/bootstrap-secret').read_text().strip(),'device_name':'Phase 2 local exercise'})['access_token']
    pilot('-recover')
    try:
        wait_for('Fresh HTTP samples received through Alloy', lambda: any(r['definition']['id']=='LOCAL-PILOT-HTTP' and r['state']['signal']=='healthy' for r in api('/v1/observability')['rules']))
        # Finish any earlier exercise before inducing a new recurrence.
        wait_for('No unresolved local exercise', lambda: not incident({'detected','reopened','acknowledged','diagnosing','monitoring'}))
        pilot('-fail')
        opened = wait_for('Controlled outage opened a correlated incident', lambda: incident({'detected','reopened'}))
        # VictoriaMetrics hides the latest 30 s of range queries by default and the
        # incident can open sooner: retry instead of reading the graph only once.
        def window():
            now = dt.datetime.now(dt.timezone.utc)
            return {'scope':'local-pilot','start':(now-dt.timedelta(minutes=10)).isoformat(),'end':now.isoformat(),'metric':'probe_success'}
        def failing_metric():
            query = window()
            metrics = api('/v1/observability/metrics?'+urllib.parse.urlencode(query))
            return query if any(p['value']==0 for series in metrics['series'] for p in series['points']) else None
        query = wait_for('Failing samples visible in VictoriaMetrics', failing_metric, 90)
        def outage_logs():
            logs = api('/v1/observability/logs?'+urllib.parse.urlencode({**window(),'search':'controlled local pilot outage'}))
            return logs if logs['lines'] else None
        logs = wait_for('Correlated outage logs visible in Loki', outage_logs, 90)
        # Reload the revision: the incident may have moved while waiting for the stores.
        opened = incident({'detected','reopened'}) or opened
        api('/v1/incidents/'+opened['id']+'/events', {'action':'acknowledge','note':'Local controlled failure verified with Alloy, VictoriaMetrics and Loki.','revision':opened['revision']})
        pilot('-recover')
        resolved = wait_for('Stable recovery automatically resolved the incident', lambda: incident({'resolved'}))
        assert resolved['id']==opened['id'], 'Recovery targeted a different incident'
        detail = api('/v1/incidents/'+resolved['id'])
        kinds = [event['kind'] for event in detail['events']]
        assert 'acknowledge' in kinds and 'resolved' in kinds
        # Verify collection is attributable to the Linux Docker host and selected container.
        coverage = {}
        for scope,metric in [('local-host','host_memory_percent'),('local-host','host_cpu_percent'),('local-pilot','container_memory_usage_bytes')]:
            result = api('/v1/observability/metrics?'+urllib.parse.urlencode({**query,'scope':scope,'metric':metric}))
            coverage[metric] = any(p['value'] is not None for s in result['series'] for p in s['points'])
        if not all(coverage.values()):
            raise RuntimeError('Missing actual telemetry: '+str(coverage))
        report = {'verified_at':dt.datetime.now(dt.timezone.utc).isoformat(),'incident_id':resolved['id'],'status':resolved['status'],'logs':len(logs['lines']),'coverage':coverage,'timeline':kinds,'target':'local isolated fixture; no production outage'}
        (ROOT/'.dev/phase2-drill.json').write_text(json.dumps(report,indent=2)+'\n')
        print(json.dumps(report), flush=True)
    finally:
        pilot('-recover')
        if token:
            api('/v1/session',method='DELETE')

if __name__ == '__main__':
    main()
