#!/bin/sh
set -eu
su-exec postgres initdb -D /fixture/pg -U postgres -A trust --encoding=UTF8 --locale=C >/dev/null
su-exec postgres pg_ctl -D /fixture/pg -l /fixture/postgres.log -o '-h 127.0.0.1 -k /fixture/socket' -w start >/dev/null
trap 'su-exec postgres pg_ctl -D /fixture/pg -m fast -w stop >/dev/null' EXIT
/backup.test -test.v
/api.test -test.v -test.run=TestPhaseSeven -test.timeout=8m
