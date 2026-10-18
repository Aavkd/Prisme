"""Fixed read-only collector; never emit env, commands, arbitrary labels or health logs."""
import datetime
import json
import os
import platform
import shutil
import subprocess

warnings = []
def docker(args, warning):
    try:
        with subprocess.Popen(['docker', *args], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL) as process:
            try:
                # Docker responses are bounded before JSON parsing. The outer Go process has a deadline.
                data = process.stdout.read(2 * 1024 * 1024 + 1)
                if len(data) > 2 * 1024 * 1024:
                    process.kill()
                    raise ValueError('limit')
                if process.wait(timeout=10) != 0:
                    raise ValueError('unavailable')
                return [json.loads(line) for line in data.decode('utf-8').splitlines() if line]
            finally:
                if process.poll() is None:
                    process.kill()
    except (OSError, ValueError, subprocess.SubprocessError):
        warnings.append(warning)
        return None

memory = None
try:
    with open('/proc/meminfo', encoding='utf-8') as f:
        for line in f:
            if line.startswith('MemTotal:'):
                memory = int(line.split()[1]) * 1024
except (OSError, ValueError):
    warnings.append('memory_unavailable')
try:
    disk = shutil.disk_usage(os.path.abspath(os.sep))
    total, free = disk.total, disk.free
except OSError:
    total, free = None, None
    warnings.append('disk_unavailable')

containers = docker(['ps', '-a', '--no-trunc', '--format',
    '{"id":{{json .ID}},"name":{{json .Names}},"image":{{json .Image}},"state":{{json .State}},"ports":{{json .Ports}},"networks":{{json .Networks}},"project":{{json (.Label "com.docker.compose.project")}},"service":{{json (.Label "com.docker.compose.service")}},"config_files":{{json (.Label "com.docker.compose.project.config_files")}}}'], 'docker_unavailable')
available = containers is not None
containers = containers or []
if len(containers) > 1000:
    containers = containers[:1000]
    warnings.append('containers_truncated')
for c in containers:
    c['health'] = 'unknown'
    c['image_id'] = ''
# Inspect only selected fields; health probe output and container config never cross the boundary.
if containers:
    facts = docker(['inspect', '--type', 'container', '--format',
        '{"id":{{json .Id}},"image_id":{{json .Image}},"health":{{if index .State "Health"}}{{json (index (index .State "Health") "Status")}}{{else}}"not_configured"{{end}}}',
        *[c['id'] for c in containers]], 'container_details_partial')
    by_id = {c['id']: c for c in containers}
    for fact in facts or []:
        if fact['id'] in by_id:
            by_id[fact['id']].update(fact)
resources = {}
for kind in ['network', 'volume']:
    resources[kind] = docker([kind, 'ls', '--format',
        '{"name":{{json .Name}},"driver":{{json .Driver}},"project":{{json (.Label "com.docker.compose.project")}}}'], kind + 's_unavailable') if available else []
images = []
image_ids = sorted(set(c['image_id'] for c in containers if c['image_id']))
if image_ids:
    images = docker(['image', 'inspect', '--format',
        '{"id":{{json .Id}},"tags":{{json .RepoTags}},"digests":{{json .RepoDigests}}}', *image_ids], 'images_unavailable') or []
    for image in images:
        image['tags'] = image['tags'] or []
        image['digests'] = image['digests'] or []
print(json.dumps({
    'protocol_version': '1', 'observed_at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
    'source': 'agent', 'host': {'hostname': platform.node(), 'os': platform.platform(),
        'architecture': platform.machine(), 'cpus': os.cpu_count() or 1,
        'memory_bytes': memory, 'disk_total_bytes': total, 'disk_free_bytes': free},
    'docker_available': available, 'containers': containers, 'networks': resources['network'] or [],
    'volumes': resources['volume'] or [], 'images': images, 'warnings': warnings
}, separators=(',', ':')))
