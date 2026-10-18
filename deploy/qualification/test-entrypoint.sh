#!/bin/sh
set -eu
su-exec postgres initdb -D /fixture/pg -U postgres -A trust --encoding=UTF8 --locale=C >/dev/null
su-exec postgres pg_ctl -D /fixture/pg -l /fixture/postgres.log -o '-h 127.0.0.1 -k /fixture/socket' -w start >/dev/null
trap 'su-exec postgres pg_ctl -D /fixture/pg -m fast -w stop >/dev/null' EXIT
/tests/restore.test -test.v -test.timeout=3m
/tests/api.test -test.v -test.run='TestPhaseTen|TestPhaseSixIndependentRecoveryAndSignedAudit|TestPhaseFourRestartNeverReplaysMutation' -test.timeout=5m
/tests/notifications.test -test.v -test.run=TestWatchdog -test.timeout=1m
