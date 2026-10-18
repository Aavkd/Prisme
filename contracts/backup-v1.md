# Backup protocol v1

OpenAPI 0.8.0, PostgreSQL migration 009. Configuration is opt-in and validated
strictly. The API stores policy hashes, job state and snapshot metadata; only the
separate Linux worker reads repository and PostgreSQL credentials or invokes CLIs.

## Requests and plans

All routes require an active application session. JSON bodies are limited to
16 KiB. Provider paths, SQL, repository passwords and commands are not accepted
from a client.

- `GET /v1/backups?page=1`: configured policy summaries, test targets, 50 jobs and
  50 artifacts per page; `has_more` covers either history.
- `POST /v1/backups/jobs`: `{policy,kind,artifact,target}` plus a 16–128 character
  `Idempotency-Key`. `kind` is `backup`, `check`, `retention` or `restore`.
  `artifact` and `target` are required only for restores and otherwise empty.
  Reusing a key with another request returns 409.
- `GET /v1/backups/jobs/{job}`: job, chronological bounded step events and exact
  confirmation phrase.
- `POST /v1/backups/jobs/{job}/decision`: `{action,revision,phrase,proof}`. Approval
  applies to a planned restore/retention operation; cancellation applies to a
  planned or queued job. Running work cannot be cancelled through this endpoint.

Backup/check requests queue immediately. Manual restore/retention requests create
a 15-minute plan. `RESTORE <target>` or `RETENTION <policy>` is required, along
with a one-use phase-6 operation proof for:

```json
{"action":"backup_approve","target":"<job-id>","payload":{"revision":1}}
```

Proofs use the existing `/v1/operations/challenges` flow. They bind the exact job,
revision, device and session, require WebAuthn UV or Android signature and are
consumed with the plan locked. Development sessions cannot approve. The server
revalidates identity and key revocation. A lost approval response is resolved by
reading the job, not replaying a proof. A failed transaction may consume a proof
conservatively without queueing work.

## Execution and evidence

Jobs progress `planned → queued → running → succeeded | failed | uncertain`.
Events identify snapshot capture, full integrity read, extraction, database
creation/restore, SQL validation, retention and scheduled-test cleanup.

Only a complete backup followed by `restic check --read-data` creates an artifact.
The snapshot ID is exact, bytes are logical source bytes, and timestamps are UTC.
No password or dump content appears in history or audit. A subsequent failed
check does not erase historical evidence of an earlier verification; every
restore performs a new full check and verified extraction.

PostgreSQL uses `pg_dump --format=custom` through Restic's
`--stdin-from-command`. There is no plaintext dump staging during backup. A failed
producer or partial file backup cannot be recorded as successful. File sources
are archived relative to an explicit directory; Linux stays on one filesystem.
Volumes must be provided as an operator-managed stopped tree or consistent
snapshot. Live database data directories are rejected.

Restore targets are preconfigured test destinations. File restores create a new
private `restore-<job-id>` directory. PostgreSQL restores create a fresh
`prisme_restore_<24-hex>` database using a non-superuser CREATEDB role, then run
`pg_restore --single-transaction --no-owner --no-acl --exit-on-error`. A private,
operator-controlled SQL check must return exactly `t` in a read-only session.
This validates the application's data invariants as well as successful schema
creation. At most ten test databases may exist for that role before another
restore is refused. Manual copies remain for inspection; successful scheduled
tests drop only their own database. A failed restore retains its database name
for operator inspection. Plaintext archive directories are removed after the run.

Retention first reads all repository data, then obtains Restic's dry-run policy
decision (`keep_last`, daily, weekly, grouped by host and filtered by policy tag).
Only registered artifacts are removable, and the newest `keep_last` verified
artifacts are protected even when newer partial/unrecorded snapshots exist.
Each cycle removes at most 100 snapshots and prunes unreferenced data. Other
policies' snapshots remain referenced. Metadata marks removed artifacts expired;
an expired artifact cannot be planned or executed. Unregistered snapshots require
operator review and are never automatically deleted.

## Scheduling, coordination and failure

Intervals are UTC seconds: `0` disables an automatic backup or test. Due times
advance transactionally with one queued job; missed slots coalesce into one run.
A successful scheduled backup queues retention. Due PostgreSQL test restores
take priority when a verified copy exists. Scheduling does not create an external
Codex automation. Stop the worker to stop its schedules.

A dedicated PostgreSQL advisory lock serializes backup workers and repository
maintenance. Source/test host locks use phase 6's coordination directory and
operational UID. Every cooperating worker on a physical host must use the same
directory and target identifier. A remote database's configured server label is
not a remote distributed lock: if other mutators execute on another machine,
deploy the worker there or arrange a genuinely shared guard before relying on
host exclusion. Restores create independent databases and never switch traffic.

Interrupted running work becomes `uncertain` and is never replayed. Process groups
are cancelled together on Linux. Timeouts retain the host's active marker until
the operator checks processes and reconciles the guard. After inspecting Restic
locks and partial outputs, the local `-acknowledge <job>` action can mark an
uncertain job failed once the host guard permits it. No automatic `restic unlock`,
repository initialization, production overwrite or target destruction is exposed.

Failures open/update an incident with rule `backup-<policy-id>-<kind>`. Add that
exact rule to the existing FCM configuration to deliver notifications. Successful
operations of the same kind resolve their failure incident. Command stderr and
database records are excluded from incidents and notification payloads.

## Deployment integration

A Compose manifest may set `backup_required: true`, `backup_policy` and
`backup_max_age_seconds` (60–86,400). The plan pins one retained artifact with
matching project, environment and source host. Approval and execution require
that exact artifact to remain fresh and retained. A repository integrity,
retention or uncertain execution failure since its verification blocks the gate
until a new verified backup exists. Run a backup before preparing a sensitive
deployment; an old plan is never silently rebound to a newer snapshot.

## Boundaries

Repository locations support local/mounted storage and S3 over HTTPS. A mounted
NAS/SFTP destination requires external mount and host-key configuration. Native
SFTP, WAL/PITR, physical database backup, production overwrite with maintenance
or traffic reopening, automatic Docker quiescence and credential rotation are
not implemented. The `offsite` property and storage-host identity are operator
declarations. `offsite_verified` means a retained, previously verified backup
exists at that declared destination; it is not a physical-topology attestation.

Policies and targets are immutable after registration; changing their fields
requires a new policy ID and associated targets. Keep old configuration available
to restore historical artifacts. Stop the worker/API configuration to disable
execution; do not destructively edit the migration or repository.

Implementation references: [Restic backup semantics](https://restic.readthedocs.io/en/stable/040_backup.html),
[retention](https://restic.readthedocs.io/en/stable/060_forget.html),
[PostgreSQL restore](https://www.postgresql.org/docs/17/app-pgrestore.html).
