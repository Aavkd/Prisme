# Project deployment manifest v1

The executable format is strict JSON (`project.json`), separate from the conceptual
YAML example in section 11 of the product spec. `version: 1` has no implicit schema
upgrade. Validate without registering or executing:

```powershell
go run ./cmd/deployer -manifest deploy/deployment/sounding-lines.import.json
```

`deploy/deployment/sounding-lines.import.json` records the known legacy pilot and
proposes a managed application subset. It is disabled. The repository's numeric ID
is intentionally absent. Proposed ports, worker health, dependency network, secret
references, backup verification, and runtime limits need qualification. PostgreSQL
stays in the existing independent stack: importing this file does not adopt, stop,
recreate, copy or migrate its data. The historical VPS Compose file is untouched.

## Fields

| Field | Contract |
|---|---|
| `version` | Exactly 1 |
| `id`, `project`, `environment` | Lowercase `[a-z][a-z0-9-]{0,39}`; immutable target identity |
| `server` | Explicit local executor label; it does not establish a remote connection |
| `repository_id`, `repository`, `branch` | Exact phase-3 repository and allowed successful branch; missing numeric ID permitted only for disabled legacy imports |
| `enabled`, `legacy` | Explicit enablement; legacy builds cannot execute |
| `strategy` | `recreate` or `blue-green`; the latter requires every managed service to be stateless |
| `migration` | `none`, `backward-compatible`, `incompatible`; describes externally qualified schema compatibility, never runs migrations |
| `backup_required` | Blocks approval until a future verified backup integration can satisfy it |
| `observation_seconds` | 1–600, strictly below timeout |
| `timeout_seconds` | 10–1800, covers every execution step |
| `minimum_free_bytes` | At least 1 MiB; checks workspace and Docker data filesystem before pull |
| `network` | Optional existing external dependency network, never created or owned by Prisme |
| `services` | 1–20 distinct names; exactly matches the release image set |
| `caddy_url`, `public_health_url` | Literal loopback HTTP(S) URLs, no credentials, query, fragment or redirects |
| `caddy_id` | Existing owned `reverse_proxy` handler ID beginning `prisme-` |
| `ingress_service` | One managed HTTP service receiving the owned route |

Each service declares `name`, `port`, `blue_port`, `green_port`, `health_path`,
`docker_health`, `env_file`, `stateless`. Container ports are 1–65535 and loopback
host ports are 1024–65535. `green_port` is required for blue-green and zero for
recreation. A service with port zero must have a Docker health check and no HTTP
ports/path. HTTP health requires a 2xx response; Docker health, when requested,
must be `healthy`. A running process alone never proves a worker healthy.

`env_file` is an optional basename ending `.env`, resolved inside `root/secrets`.
Values remain in private files. Plans bind file hashes; runtime refuses a changed
file, and rollback refuses a changed secret configuration. Compose reads files in
raw format. No shell interpolation, arbitrary command, build, bind mount, data
volume, privilege, external Compose include or hook is expressible in this version.
Image-declared volumes still follow Docker behavior; do not mark such a stateful
application stateless. Managed containers currently have a 512 MiB / 1 CPU limit.
Pilot resource requirements must be qualified before enabling their manifests.

## Registration and plans

Worker/API configuration is `{ "version":1, "root":"/absolute/private/path",
"targets":[ /* project manifests */ ] }`. Both processes use identical manifests;
the worker must see the same secret files and the local Docker data filesystem.
Target IDs, host ports and managed Caddy handlers must be unique within the config.
An existing target manifest cannot silently be replaced. Configuration changes
need a separately reviewed target revision and runtime ownership migration; do
not reuse an existing Compose project through an unrelated target ID.

Plans last 15 minutes and record manifest/hash, release/build provenance, prior
healthy release, prior runtime slot, image diff, secret hashes, target revision,
rollback intent, blockers and ordered steps. Approval binds the exact target phrase,
job revision and unchanged target baseline. A target permits one ready, running or
uncertain job. The API never invokes Docker. See `docs/PHASE_4.md` for recovery.

For blue-green, provision a dedicated Caddy reverse-proxy handler with one upstream
`127.0.0.1:1` before the first approval. Prisme only patches that handler's upstream
array using its ETag and `If-Match`; all other routes/settings remain outside its
ownership. Existing historical Caddy routes are never automatically adopted.

References: [Compose up](https://docs.docker.com/reference/cli/docker/compose/up/),
[Compose pull](https://docs.docker.com/reference/cli/docker/compose/pull/),
[Caddy scoped configuration and concurrency](https://caddyserver.com/docs/api).

## Pre-deployment backup (phase 7)

A manifest may provide `backup_policy` and `backup_max_age_seconds` (60–86,400)
when `backup_required` is true. A deployment plan binds an exact retained backup
from the same project, environment and server. Freshness and repository failure
evidence are checked again at approval and execution. Run the backup before
preparing the deployment; an existing plan never silently adopts another copy.
See [backup-v1](backup-v1.md). Without a configured policy the existing
`verified_backup_required` blocker remains in place.
