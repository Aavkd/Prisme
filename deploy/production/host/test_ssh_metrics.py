import ipaddress
import os
import sys
import tempfile
import time
import unittest

sys.path.insert(0, os.path.dirname(__file__))
import ssh_metrics  # noqa: E402

NOW = 1_800_000_000.0
KNOWN = [ipaddress.ip_network('203.0.113.7/32'), ipaddress.ip_network('198.51.100.0/24'), ipaddress.ip_network('2001:db8::/32')]


def stamp(seconds_ago):
    return time.strftime('%Y-%m-%d %H:%M:%S', time.localtime(NOW - seconds_ago)) + ',123'


class SSHJournalTest(unittest.TestCase):
    # Message shapes copied from the ovh-mantara journal (addresses replaced by documentation ranges).
    def test_failures_counted_once_per_connection_within_ten_minutes(self):
        entries = [
            (NOW - 30, 'Invalid user admin from 192.0.2.10 port 40100'),
            (NOW - 31, 'Connection closed by invalid user admin 192.0.2.10 port 40100 [preauth]'),
            (NOW - 40, 'Invalid user  from 192.0.2.11 port 40101'),
            (NOW - 50, 'Connection closed by authenticating user root 192.0.2.12 port 40102 [preauth]'),
            (NOW - 60, 'Disconnected from authenticating user ubuntu 192.0.2.13 port 40103 [preauth]'),
            (NOW - 70, 'Connection reset by authenticating user root 192.0.2.14 port 40104 [preauth]'),
            (NOW - 71, 'Received disconnect from 192.0.2.14 port 40104:11: Bye Bye [preauth]'),
            (NOW - 80, 'Timeout before authentication for 192.0.2.15 port 40105'),
            (NOW - 90, 'Connection closed by 192.0.2.16 port 40106 [preauth]'),
            (NOW - 700, 'Invalid user old from 192.0.2.17 port 40107'),
        ]
        self.assertEqual(ssh_metrics.ssh_counts(entries, NOW, KNOWN), (5, 0, []))

    def test_logins_from_known_and_unknown_addresses(self):
        entries = [
            (NOW - 10, 'Accepted publickey for ubuntu from 203.0.113.7 port 50000 ssh2: ED25519 SHA256:known'),
            (NOW - 20, 'Accepted publickey for ubuntu from 198.51.100.42 port 50001 ssh2: ED25519 SHA256:known'),
            (NOW - 30, 'Accepted publickey for ubuntu from 2001:db8::5 port 50002 ssh2: ED25519 SHA256:known'),
            (NOW - 40, 'Accepted publickey for ubuntu from 192.0.2.50 port 50003 ssh2: ED25519 SHA256:other'),
            (NOW - 50, 'Accepted password for ke2c-copie from ::ffff:192.0.2.51 port 50004 ssh2'),
            (NOW - 1900, 'Accepted publickey for ubuntu from 192.0.2.52 port 50005 ssh2: ED25519 SHA256:old'),
        ]
        failures, logins, unknown = ssh_metrics.ssh_counts(entries, NOW, KNOWN)
        self.assertEqual((failures, logins), (0, 5))
        self.assertEqual(unknown, [(NOW - 40, 'ubuntu', '192.0.2.50', 'ED25519 SHA256:other'),
                                   (NOW - 50, 'ke2c-copie', '::ffff:192.0.2.51', 'password')])

    def test_ipv4_mapped_address_matches_its_ipv4_entry(self):
        self.assertTrue(ssh_metrics.known('::ffff:203.0.113.7', KNOWN))
        self.assertFalse(ssh_metrics.known('not-an-address', KNOWN))

    def test_known_list_refuses_anything_but_addresses(self):
        with tempfile.TemporaryDirectory() as d:
            path = os.path.join(d, 'known')
            with open(path, 'w', encoding='utf-8') as f:
                f.write('# Alexy\n203.0.113.7  # tour\n\n198.51.100.0/24\n')
            self.assertEqual(len(ssh_metrics.load_known(path)), 2)
            with open(path, 'a', encoding='utf-8') as f:
                f.write('portable.example\n')
            with self.assertRaises(ValueError):
                ssh_metrics.load_known(path)


class Fail2banTest(unittest.TestCase):
    def test_current_bans_and_recent_detections(self):
        lines = [
            stamp(9000) + ' fail2ban.actions        [10]: NOTICE  [sshd] Ban 192.0.2.1',
            stamp(8000) + ' fail2ban.server         [10]: INFO    Exiting Fail2ban',
            stamp(7990) + ' fail2ban.server         [11]: INFO    Starting Fail2ban v1.1.0',
            stamp(7980) + ' fail2ban.actions        [11]: NOTICE  [sshd] Restore Ban 192.0.2.2',
            stamp(5000) + ' fail2ban.actions        [11]: NOTICE  [sshd] Ban 192.0.2.3',
            stamp(4000) + ' fail2ban.actions        [11]: NOTICE  [sshd] Ban 192.0.2.4',
            stamp(3000) + ' fail2ban.actions        [11]: NOTICE  [sshd] Increase Ban 192.0.2.4 (2 # 2:00:00 -> 2026-10-20 00:00:00)',
            stamp(1000) + ' fail2ban.actions        [11]: NOTICE  [sshd] Unban 192.0.2.3',
            stamp(900) + ' fail2ban.filter         [11]: INFO    [sshd] Found 192.0.2.5 - 2026-10-19 20:30:00',
            stamp(120) + ' fail2ban.filter         [11]: INFO    [sshd] Found 192.0.2.6 - 2026-10-19 20:30:00',
            stamp(60) + ' fail2ban.filter         [11]: INFO    [sshd] Found 192.0.2.6 - 2026-10-19 20:30:00',
            stamp(50) + ' fail2ban.filter         [11]: INFO    [recidive] Found 192.0.2.7 - 2026-10-19 20:30:00',
        ]
        self.assertEqual(ssh_metrics.fail2ban_state(lines, NOW), (2, 2))


class RenderTest(unittest.TestCase):
    def values(self, *args, **kwargs):
        text = ssh_metrics.render(ssh_metrics.gauges(*args, now=NOW, **kwargs))
        return dict(line.split() for line in text.splitlines() if not line.startswith('#'))

    def test_render_is_a_valid_textfile(self):
        self.assertEqual(self.values(12, 3, 0, True, 4, 9), {
            'prisme_ssh_failures_10m': '12', 'prisme_ssh_logins_30m': '3', 'prisme_ssh_unknown_logins_30m': '0',
            'prisme_fail2ban_up': '1', 'prisme_fail2ban_banned': '4', 'prisme_fail2ban_found_10m': '9',
            'prisme_fail2ban_blind': '0', 'prisme_ssh_drill': '0', 'prisme_ssh_metrics_generated_seconds': str(int(NOW))})

    def test_blind_only_when_active_and_failures_go_unseen(self):
        self.assertEqual(self.values(25, 0, 0, True, 0, 0)['prisme_fail2ban_blind'], '1')
        self.assertEqual(self.values(19, 0, 0, True, 0, 0)['prisme_fail2ban_blind'], '0')
        self.assertEqual(self.values(25, 0, 0, True, 0, 1)['prisme_fail2ban_blind'], '0')
        self.assertEqual(self.values(25, 0, 0, False, 0, 0)['prisme_fail2ban_blind'], '0')

    def test_drills_only_raise_alarms(self):
        drill = frozenset(ssh_metrics.DRILLS)
        v = self.values(3, 1, 0, True, 0, 3, drill=drill)
        self.assertEqual((v['prisme_ssh_failures_10m'], v['prisme_ssh_unknown_logins_30m'], v['prisme_fail2ban_up'],
                          v['prisme_fail2ban_blind'], v['prisme_ssh_drill']), ('1000', '1', '0', '1', '4'))
        v = self.values(5000, 1, 2, False, 0, 0, drill=drill)
        self.assertEqual((v['prisme_ssh_failures_10m'], v['prisme_ssh_unknown_logins_30m']), ('5000', '2'))

    def test_drill_flags_are_read_from_the_directory(self):
        with tempfile.TemporaryDirectory() as d:
            open(os.path.join(d, 'drill-ssh-unknown-login'), 'w').close()
            open(os.path.join(d, 'drill'), 'w').close()
            self.assertEqual(ssh_metrics.drills(d), {'drill-ssh-unknown-login'})


if __name__ == '__main__':
    unittest.main()
