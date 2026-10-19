import datetime
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(__file__))
import host_metrics  # noqa: E402

NOW = 1_800_000_000.0


def container(status='running', policy='unless-stopped', code=0, health=None, restarts=0, started=NOW - 86400):
    # Docker prints nanoseconds; fromisoformat must accept them.
    stamp = datetime.datetime.fromtimestamp(started, datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%S.123456789Z')
    state = {'Status': status, 'ExitCode': code, 'StartedAt': stamp}
    if health:
        state['Health'] = {'Status': health}
    return {'State': state, 'HostConfig': {'RestartPolicy': {'Name': policy}}, 'RestartCount': restarts}


class ProblemTest(unittest.TestCase):
    def test_healthy_fleet_and_finished_one_shot_job(self):
        items = [container(), container(health='healthy'), container('exited', policy='no', code=0),
                 container('exited', policy='no', code=0, health='unhealthy'), container('created')]
        self.assertFalse(any(host_metrics.problem(c, NOW) for c in items))

    def test_unexpected_states(self):
        for c in [container('exited', code=0), container('exited', code=137), container('exited', policy='no', code=1),
                  container('dead'), container('restarting'), container(health='unhealthy'),
                  container(restarts=3, started=NOW - 120)]:
            self.assertTrue(host_metrics.problem(c, NOW), c)

    def test_old_restart_is_no_longer_a_problem(self):
        self.assertFalse(host_metrics.problem(container(restarts=3, started=NOW - 3600), NOW))

    def test_render_is_a_valid_textfile(self):
        text = host_metrics.render([container(), container('exited')], NOW)
        values = dict(line.split() for line in text.splitlines() if not line.startswith('#'))
        self.assertEqual(values, {'prisme_containers_problem': '1', 'prisme_containers_running': '1',
                                  'prisme_containers_total': '2', 'prisme_drill': '0',
                                  'prisme_host_metrics_generated_seconds': str(int(NOW))})
        self.assertIn('prisme_drill 1', host_metrics.render([], NOW, drill=True))


if __name__ == '__main__':
    unittest.main()
