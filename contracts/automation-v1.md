# Automation v1

Phase 9 uses migration 011, `internal/automation` and the separate Linux worker
`cmd/automation`. The API owns configuration versions, approvals and history;
the worker owns the existing, explicitly configured deployment credentials.
No client-supplied text is interpreted as a shell command.

## Targets and policies

Each configured target binds a deployment manifest, host, project, environment,
stateless service and observation rule. The native driver verifies that binding.
One automation target owns one deployment stack, because a rollback can affect
all services in that stack. Restart addresses only the selected, owned service.

Policies are append-only `(id, version)` records; publishing requires exactly the
next version and an action-bound proof. No policy is installed or activated by
loading the example configuration. The global control starts **stopped**.

| Mode | Behavior |
|---|---|
| `supervised` | Record the incident and proposed procedure, execute no steps. |
| `assisted` | Require a fresh proof for each individual mutation. |
| `autonomous` | Execute only the approved policy's bounded action sequence. |

The trigger consumes the existing consecutive-failure evidence: at least
`failures` distinct samples, all within `window_seconds` from the beginning of
that consecutive failure sequence, with the last sample no older than
`freshness_seconds`. An active incident and the exact host/project/environment/
service scope are mandatory. Missing, inhibited, stale or ambiguous evidence
never authorizes a mutation. Dependencies and exclusions are rule IDs: every
referenced rule must have fresh `healthy` evidence. Unknown blocks execution.

`max_attempts` is a **total mutation limit** (1–5), not a retry count per action.
When rollback is allowed, one attempt is reserved for it after allowed restarts.
The delay doubles between attempts, capped at 3,600 seconds. Each mutation is
followed by that delay and a health observation window; healthy recovery skips
remaining mutations. Only a previously successful release of the exact manifest
is eligible for rollback. Image digests, secret hashes, backup requirements,
migration compatibility, target revision and host fencing still apply.

Failure, uncertainty or timeout opens a persistent circuit breaker. Cooldown
does not reset it automatically: an explicit fresh proof is required. Reset
retains the last incident ID, so the same unresolved incident cannot restart the
loop. Policy changes and worker restarts cannot reset this bound. Switching a
supervised incident to autonomous does not retroactively execute that incident.

## Runbooks

A run stores the exact version, resolved step parameters, target fingerprint,
origin, incident, attempts, step cursor, deadline and audit events. Parameters
are integer ranges, booleans or enumerated strings; no implicit string/number
coercion. Integer parameters can supply a wait duration. Other typed values are
retained as input context; they cannot inject targets, release IDs or commands.

Supported actions: `check`, `restart`, `rollback`, `wait`, `approval`, `notify`.
Conditions are `always`, `failed`, `succeeded`, evaluated against the previous
health result. A successful command requires a later check before the run can
finish successfully. Read checks can retry up to three times with bounded
backoff; mutations are never blindly retried. `seconds` on a check requires
continuous successful samples; a gap over ten seconds restarts that observation.
Each provider call has a step timeout, and the whole run has an absolute deadline
including waits, approvals and maintenance. An interrupted read can resume;
an interrupted mutation becomes `uncertain` and occupies its target.

Simulation validates and records the planned path without provider calls or
approvals. It is a dry plan, not a prediction of live health or migration safety.
Simulations use the same global stop/maintenance controls. Manual and scheduled
procedures require individual proofs for sensitive steps. An approval must reach
its step within two minutes, or a new proof is required.

`notify` records a durable timeline notification. Incident-triggered executions
correlate all events with the incident. A circuit opening changes the existing
notification worker's deduplication key, preserving device revocation, quiet
hours and FCM delivery rules. A manual run without an incident has an in-app
timeline notification only. External providers for arbitrary messages are not
part of this contract.

## Scheduling, maintenance and stop

Schedules pin a runbook version, typed parameters, UTC start time and interval
(60 seconds to 31 days). Updates require the next schedule revision and a proof.
Missed intervals coalesce into at most one run; busy targets are skipped and
audited. Schedule persistence and run creation share a transaction.

Global, policy and runbook maintenance use an absolute `maintenance_until` time.
The global stop cancels pending runs and prevents scheduling. The worker checks
control state during calls and before/after dispatch. Cancellation during a
mutation is uncertain; it cannot undo an already accepted Docker command.
Linked rollback jobs retain `automation_run_id` so other deployment workers
must honor stop and policy revocation too.

After uncertainty: stop automation, inspect processes/runtime, explicitly
reconcile the independent host guard at its observed epoch, then request run
reconciliation with a fresh proof. The automation worker waits for a clear host
guard and free deployment lock before retiring linked jobs as failed. No command
is replayed and no version is promoted. Finally reset the breaker after cooldown
and enable automation when appropriate. The old incident remains consumed.

## API and proof bindings

All routes require an application session. `GET /v1/automation` returns latest
definitions, targets, control, circuits, schedules and paginated runs (50/page).
`GET /v1/automation/runs/{run}` includes the exact plan and up to 1,000 events.

| Mutation | Proof action / target / payload |
|---|---|
| `POST /definitions` | `automation_publish` / definition ID / `{kind,document}` |
| `POST /control` | `automation_control` / `global` / exact supplied control object |
| `POST /runs` | Plan creation; requires `Idempotency-Key`, no provider mutation |
| `POST /runs/{run}/decision` | `automation_approve` or `automation_reconcile` / run ID / `{revision}` |
| `POST /schedules` | `automation_schedule` / schedule ID / exact supplied schedule object |
| `POST /policies/{policy}/reset` | `automation_reset` / policy ID / `{}` |

The prefix is `/v1/automation`. `cancel` and setting `stopped:true` need no
biometric proof, but remain authenticated and revision-checked. Proofs use the
existing one-use WebAuthn/Android operation-intent mechanism. Timestamp spelling
is preserved for binding, then parsed for scheduling. Reject unknown fields and
unsupported actions; structured responses do not return raw provider stderr.

Arbitrary commands, build/migration/backup/edge step adapters, cron expressions
and recurring weekly maintenance calendars are deliberately outside this first
bounded contract; the existing modules retain their own approved APIs.
