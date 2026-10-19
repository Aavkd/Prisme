"""Host gauges for Alloy's textfile collector, read-only.

Run by prisme-host-metrics.timer as prisme-collector (groups docker and
prisme-observation): `python3 -I host_metrics.py /var/lib/prisme-metrics`.
Only `docker ps` and `docker inspect` are called; nothing is ever changed.
The output file is replaced atomically. On failure it is left as is: its
generation time then ages and the Prisme rule on
prisme_host_metrics_age_seconds reports the stopped collection.
"""
import datetime
import json
import os
import subprocess
import sys
import tempfile
import time

RECENT_RESTART = 600  # seconds: a container restarted by Docker stays a problem this long
# Present only during a Prisme drill (sudo touch / rm): opens then resolves PRISME-DRILL.
DRILL_FLAG = '/etc/prisme/drill'


def docker(*args, timeout=30):
    return subprocess.run(['docker', *args], capture_output=True, text=True, timeout=timeout)


def containers():
    listed = docker('ps', '-aq', '--no-trunc', timeout=20)
    if listed.returncode != 0:
        raise RuntimeError('docker ps unavailable')
    ids = listed.stdout.split()
    if not ids:
        return []
    # A container removed between the two calls makes inspect fail for that ID
    # only; the others are still printed as a JSON array.
    inspected = docker('inspect', *ids)
    try:
        data = json.loads(inspected.stdout or '[]')
    except ValueError:
        raise RuntimeError('docker inspect unreadable')
    if not isinstance(data, list):
        raise RuntimeError('docker inspect unreadable')
    return data


def started_at(value):
    try:
        return datetime.datetime.fromisoformat(value.replace('Z', '+00:00')).timestamp()
    except (AttributeError, ValueError):
        return 0.0


def problem(c, now):
    state = c.get('State') or {}
    status = state.get('Status')
    policy = ((c.get('HostConfig') or {}).get('RestartPolicy') or {}).get('Name') or 'no'
    if status == 'restarting':
        return True
    # One-shot jobs (no restart policy) may exit 0; anything else stopped is unexpected.
    if status in ('exited', 'dead') and (policy != 'no' or state.get('ExitCode') != 0):
        return True
    # Docker keeps the last health status of a stopped container: running only.
    if status == 'running' and (state.get('Health') or {}).get('Status') == 'unhealthy':
        return True
    if status == 'running' and (c.get('RestartCount') or 0) > 0:
        return now - started_at(state.get('StartedAt')) < RECENT_RESTART
    return False


def render(items, now, drill=False):
    running = sum(1 for c in items if (c.get('State') or {}).get('Status') == 'running')
    gauges = [
        ('prisme_containers_problem', 'Containers stopped unexpectedly, restarting, unhealthy or restarted by Docker within 10 minutes.', sum(1 for c in items if problem(c, now))),
        ('prisme_containers_running', 'Running containers.', running),
        ('prisme_containers_total', 'Containers known to Docker.', len(items)),
        ('prisme_drill', 'Synthetic drill signal (1 while ' + DRILL_FLAG + ' exists).', 1 if drill else 0),
        ('prisme_host_metrics_generated_seconds', 'Unix time when this file was generated.', int(now)),
    ]
    lines = []
    for name, help_text, value in gauges:
        lines += ['# HELP %s %s' % (name, help_text), '# TYPE %s gauge' % name, '%s %d' % (name, value)]
    return '\n'.join(lines) + '\n'


def main():
    if len(sys.argv) != 2:
        sys.exit('usage: host_metrics.py OUTPUT_DIRECTORY')
    directory = sys.argv[1]
    now = time.time()
    text = render(containers(), now, os.path.exists(DRILL_FLAG))
    fd, tmp = tempfile.mkstemp(prefix='.host-', suffix='.tmp', dir=directory)
    try:
        with os.fdopen(fd, 'w', encoding='ascii') as f:
            f.write(text)
        os.chmod(tmp, 0o644)
        os.replace(tmp, os.path.join(directory, 'host.prom'))
    finally:
        if os.path.exists(tmp):
            os.unlink(tmp)


if __name__ == '__main__':
    main()
