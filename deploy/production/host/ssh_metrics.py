"""SSH threat gauges for Alloy's textfile collector, read-only.

Run by prisme-ssh-metrics.timer as prisme-collector with the adm group for
this unit only (journal ACL and /var/log/fail2ban.log) and no Docker access:
`python3 -I ssh_metrics.py /etc/prisme/ssh-known-ips /var/lib/prisme-metrics`.
An output directory of `-` prints the gauges instead (dry run).

Reads the ssh.service journal, `systemctl is-active fail2ban` and the fail2ban
log; nothing is ever changed. No address reaches the gauges: an unknown login
is described only in this unit's own journal. On failure (journal or log
unreadable, invalid address list) the file is left as is: its generation time
then ages and the Prisme rule on prisme_ssh_metrics_age_seconds reports it.
"""
import datetime
import ipaddress
import json
import os
import re
import subprocess
import sys
import tempfile
import time

FAILURE_WINDOW = 600  # seconds: failures counted over the last 10 minutes
LOGIN_WINDOW = 1800  # an unknown login keeps its gauge up 30 minutes: the incident opens, then resolves
REPORT_WINDOW = 60  # unknown logins this recent are described in the journal (each run is 30 s apart)
BLIND_MINIMUM = 20  # this many failures without a single fail2ban detection means it does not see them
FAIL2BAN_LOGS = ('/var/log/fail2ban.log.1', '/var/log/fail2ban.log')  # oldest first; weekly rotation, bans last 1 week at most
# Present only during a Prisme drill (sudo touch / rm): each forces one alarm,
# none can hide a real one.
DRILL_DIR = '/etc/prisme'
DRILLS = ('drill-ssh-failures', 'drill-ssh-unknown-login', 'drill-fail2ban-down', 'drill-fail2ban-blind')

# One line per refused connection, as measured on ovh-mantara (OpenSSH >= 9.8 writes
# them from sshd-session, still under ssh.service).
FAILURE = re.compile(r'^(?:Invalid user \S* from|(?:Connection closed by|Disconnected from|Connection reset by) authenticating user \S+) (\S+) port \d+')
ACCEPTED = re.compile(r'^Accepted (\S+) for (\S+) from (\S+) port \d+ \S+?(?:: (\S+ \S+))?$')
F2B_LINE = re.compile(r'^(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d),\d+ .*?(?:\[sshd\] (Found|Ban|Restore Ban|Unban) (\S+)|(Starting Fail2ban|Exiting Fail2ban))')


def load_known(path):
    networks = []
    with open(path, encoding='utf-8') as f:
        for line in f:
            line = line.split('#', 1)[0].strip()
            if line:
                # Raises on anything but an address or network: no silent gap in the list.
                networks.append(ipaddress.ip_network(line, strict=False))
    return networks


def known(address, networks):
    try:
        ip = ipaddress.ip_address(address)
    except ValueError:
        return False
    if ip.version == 6 and ip.ipv4_mapped:
        ip = ip.ipv4_mapped
    return any(ip in n for n in networks if n.version == ip.version)


def journal(since):
    done = subprocess.run(['journalctl', '-u', 'ssh.service', '--since', '@%d' % int(since), '--no-pager', '-q',
                           '-o', 'json', '--output-fields=MESSAGE'], capture_output=True, text=True, timeout=20)
    if done.returncode != 0:
        raise RuntimeError('journal unreadable')
    entries = []
    for line in done.stdout.splitlines():
        try:
            e = json.loads(line)
            stamp = int(e['__REALTIME_TIMESTAMP']) / 1e6
        except (ValueError, KeyError, TypeError):
            continue
        if isinstance(e.get('MESSAGE'), str):
            entries.append((stamp, e['MESSAGE']))
    return entries


def ssh_counts(entries, now, networks):
    failures, logins, unknown = 0, 0, []
    for stamp, message in entries:
        m = FAILURE.match(message)
        if m and now - stamp <= FAILURE_WINDOW:
            failures += 1
            continue
        m = ACCEPTED.match(message)
        if m and now - stamp <= LOGIN_WINDOW:
            logins += 1
            if not known(m.group(3), networks):
                unknown.append((stamp, m.group(2), m.group(3), m.group(4) or m.group(1)))
    return failures, logins, unknown


def fail2ban_active():
    done = subprocess.run(['systemctl', 'is-active', '--quiet', 'fail2ban.service'], capture_output=True, timeout=10)
    return done.returncode == 0


def fail2ban_lines():
    lines, read = [], False
    for path in FAIL2BAN_LOGS:
        try:
            with open(path, encoding='utf-8', errors='replace') as f:
                lines += f.read().splitlines()
            read = True
        except FileNotFoundError:
            continue
    if not read:
        raise RuntimeError('fail2ban log unreadable')
    return lines


def fail2ban_state(lines, now):
    """Addresses banned by the sshd jail now, and its detections over the failure window.

    The log is written in the host's local time. A restart clears the firewall
    set; persisted bans come back as `Restore Ban`.
    """
    banned, found = set(), 0
    for line in lines:
        m = F2B_LINE.match(line)
        if not m:
            continue
        if m.group(4):
            banned.clear()
            continue
        event, address = m.group(2), m.group(3)
        if event == 'Found':
            if now - time.mktime(time.strptime(m.group(1), '%Y-%m-%d %H:%M:%S')) <= FAILURE_WINDOW:
                found += 1
        elif event == 'Unban':
            banned.discard(address)
        else:
            banned.add(address)
    return len(banned), found


def drills(directory=DRILL_DIR):
    return {name for name in DRILLS if os.path.exists(os.path.join(directory, name))}


def gauges(failures, logins, unknown, active, banned, found, drill=frozenset(), now=0.0):
    blind = active and failures >= BLIND_MINIMUM and found == 0
    if 'drill-ssh-failures' in drill:
        failures = max(failures, 1000)
    if 'drill-ssh-unknown-login' in drill:
        unknown = max(unknown, 1)
    if 'drill-fail2ban-down' in drill:
        active = False
    if 'drill-fail2ban-blind' in drill:
        blind = True
    return [
        ('prisme_ssh_failures_10m', 'Refused SSH connections (unknown user or rejected key) over the last 10 minutes.', failures),
        ('prisme_ssh_logins_30m', 'Successful SSH logins over the last 30 minutes.', logins),
        ('prisme_ssh_unknown_logins_30m', 'Successful SSH logins from an address outside the known list, last 30 minutes.', unknown),
        ('prisme_fail2ban_up', 'fail2ban.service active (1) or not (0).', 1 if active else 0),
        ('prisme_fail2ban_banned', 'Addresses currently banned by the sshd jail.', banned),
        ('prisme_fail2ban_found_10m', 'SSH failures detected by fail2ban over the last 10 minutes.', found),
        ('prisme_fail2ban_blind', 'fail2ban active but detecting none of at least %d SSH failures in 10 minutes.' % BLIND_MINIMUM, 1 if blind else 0),
        ('prisme_ssh_drill', 'Synthetic SSH drill flags present in ' + DRILL_DIR + '.', len(drill)),
        ('prisme_ssh_metrics_generated_seconds', 'Unix time when this file was generated.', int(now)),
    ]


def render(values):
    lines = []
    for name, help_text, value in values:
        lines += ['# HELP %s %s' % (name, help_text), '# TYPE %s gauge' % name, '%s %d' % (name, value)]
    return '\n'.join(lines) + '\n'


def write(directory, text):
    if directory == '-':
        sys.stdout.write(text)
        return
    fd, tmp = tempfile.mkstemp(prefix='.ssh-', suffix='.tmp', dir=directory)
    try:
        with os.fdopen(fd, 'w', encoding='ascii') as f:
            f.write(text)
        os.chmod(tmp, 0o644)
        os.replace(tmp, os.path.join(directory, 'ssh.prom'))
    finally:
        if os.path.exists(tmp):
            os.unlink(tmp)


def main():
    if len(sys.argv) != 3:
        sys.exit('usage: ssh_metrics.py KNOWN_ADDRESSES_FILE OUTPUT_DIRECTORY|-')
    networks = load_known(sys.argv[1])
    now = time.time()
    failures, logins, unknown = ssh_counts(journal(now - max(FAILURE_WINDOW, LOGIN_WINDOW)), now, networks)
    banned, found = fail2ban_state(fail2ban_lines(), now)
    text = render(gauges(failures, logins, len(unknown), fail2ban_active(), banned, found, drills(), now))
    for stamp, user, address, key in unknown:
        if now - stamp <= REPORT_WINDOW:
            at = datetime.datetime.fromtimestamp(stamp, datetime.timezone.utc).strftime('%Y-%m-%d %H:%M:%S UTC')
            print('unknown SSH login: %s from %s with %s at %s' % (user, address, key, at), file=sys.stderr)
    write(sys.argv[2], text)


if __name__ == '__main__':
    main()
