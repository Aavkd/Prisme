# Operations v1 — Phase 6

OpenAPI **0.7.0**, PostgreSQL schema **8**. All endpoints are private TLS APIs.
Targets and SSH identities are configured on the server. Requests cannot supply
SSH addresses, usernames, credentials or executable paths. Every SSH connection
pins an independently verified SHA-256 host-key fingerprint; TOFU is unavailable.

## Authorization

`GET /v1/operations/overview` returns target capabilities, owned sessions and vault
metadata. Values, SSH keys and tickets are absent. Responses are never cacheable.

The native client prepares an `OperationIntent`:

```json
{"action":"file_save","target":"local-admin","payload":{"path":"/srv/prisme-managed/config.txt","revision":"sha256-of-original","content":"new contents"}}
```

`POST /v1/operations/challenges` accepts `{intent,method}` (`passkey` or
`android_key`). It requires a passkey session and creates a two-minute challenge
bound to the session, device and SHA-256 of `action + LF + target + LF + canonical
JSON(payload)`. Canonical JSON is the Go JSON encoding of the parsed payload.
Only that digest and non-sensitive action/target labels enter challenge storage.
Challenges are memory-only, bounded to 100, and disappear on restart.

The browser uses `/v1/operations/verify#<id>`, retrieves `.../challenges/<id>/options`
and posts its UV-required WebAuthn assertion to `.../<id>/passkey`. The browser
never receives the native bearer. Android signs the returned challenge through
the enrolled P-256 key and posts a base64 ASN.1 signature to `.../<id>/sign`.
Re-enrolling the key invalidates outstanding Android operation proofs. A proof
is consumed once by `POST /v1/operations/execute` with `{intent,proof}`.
There is no transparent retry of a mutation or terminal input.

The existing Android Keystore guarantees and attestation limitations from Phase 5
still apply. Recovery enrolls a separate, origin-scoped action key through its own
`/v1/action-key` route after a recent independent passkey login.

## Actions

| Action | Payload | Result / behavior |
|---|---|---|
| `terminal_open` | columns 20–500, rows 5–300, optional incident ID | ID, one-use ticket, input cursor, account, environment |
| `terminal_attach` | id | New ticket for the same owned, detached session |
| `terminal_close` | id | Request termination; force channel closure after two seconds; final state can be uncertain |
| `diagnostics` | optional allowlisted container | Timestamped bounded/redacted uptime, disk, container listing or 100 log lines |
| `file_list` | path | At most 1,000 entries, mode, UID/GID, size, modification time; protected names hidden |
| `file_stat` | path | SHA-256 revision, or `absent` for a new destination |
| `file_read` | path | UTF-8 preview, revision, editable flag; redacted previews are read-only |
| `file_save` | path, revision, content | Verified revision and atomic replacement after private backup |
| `upload_begin` | path, revision, size, digest | Session-bound transfer ID |
| `upload_chunk` | id, offset, base64 | Next offset; an identical chunk retry is idempotent |
| `upload_status` | id | Current offset and size |
| `upload_commit` | id | Verify full digest and original revision, then save atomically |
| `upload_cancel` | id | Remove the transfer and clear the in-memory bytes |
| `download_begin` | path | Immutable, session-bound in-memory snapshot; size and revision |
| `download_chunk` | id, offset | Up to 48 KiB from that snapshot, safe to repeat |
| `secret_put` | secret metadata, value | Create or rotate with optimistic integer revision |
| `secret_reveal` | id, revision | Value for the exact authorized destination; hide after 30 seconds |
| `env_apply` | path, revision, variables, secrets | Replace a private `.env` using literal variables and `{id,revision}` secret references |

`file_list`, `file_stat`, `diagnostics`, `terminal_close`, `upload_chunk`,
`upload_status`, `upload_cancel`, and `download_chunk` require the authenticated
session but no new proof. Transfer continuation and closure enforce ownership.
All other actions require a one-use proof. Legacy chunked `file_download` also
exists; new clients use `download_begin`/`download_chunk` to reauthenticate once.

Files are limited to **1 MiB**, secret values to **64 KiB**, transfer chunks to
**48 KiB**, concurrent transfer buffers to **8**, expiring after ten minutes.
Plaintext transfer bytes stay only in memory and are cleared on expiry/cancel.
Transfers do not survive a gateway restart. The client retains an interrupted
transfer only in the current screen/session and offers explicit resume.

SFTP edits reject symlinks, traversal, paths outside configured roots, `.ssh`,
`.git`, key material and system paths. Generic file actions reject `.env`;
`env_apply` resolves its secrets server-side. POSIX rename and fsync SFTP
extensions are mandatory. Existing file permissions/ownership are preserved;
backups and `.env` files use mode 0600. An SSH identity with restricted filesystem
permissions is essential: client-side SFTP checks cannot eliminate races caused
by a separate privileged process changing the remote filesystem.

The UI masks diffs containing recognized credentials. This is best-effort
detection; file contents and terminal output are never written to platform logs,
audit events, notification payloads or offline cache. Full session recording is
disabled. Protected credential files are intentionally handled by the vault flow.

## Terminal transport

Connect to `wss://<origin>/v1/operations/sessions/<id>/stream` with `Authorization:
Bearer ...` and `X-Prisme-Terminal-Ticket`. Tickets expire after 30 seconds and
are consumed on attachment. No credential goes in the URL. One socket can attach
at a time; a reconnect needs a new proof and explicit attach.

Input frames: `{type:"input",seq:1,data:"base64-bytes"}` with a strictly increasing
per-session sequence (max 8 KiB decoded per frame). Resize frames:
`{type:"resize",columns:100,rows:35}`. Duplicate or skipped input sequence closes
the socket; the client never automatically replays uncertain input.

Output: `{frames:[{seq,data}],gap,state,input_seq}`. The server retains up to 64
4-KiB frames in memory. A reconnect may replay output, which the client deduplicates;
`gap` explicitly reports missing output. Sessions last at most 30 minutes, with
a ten-minute input idle limit and authorization checks during input and every two
seconds. Up to eight sessions can exist across independent host targets. One
interactive terminal holds the host mutation guard, so additional terminals on
the same coordinated host are deliberately refused.

## Independent recovery and host coordination

`cmd/recovery` uses its own HTTPS origin, TLS identity, local passkey/device
registry, one-use enrollment/recovery codes, sessions, biometric keys and audit
signing key. It neither calls the central database nor loads the central vault.
The static browser presentation and operations implementation are shared; identity
and transport state are separate. Restart invalidates in-memory sessions/proofs.

All mutating access uses `cmd/host-guard` on the execution host, through SSH.
Configure **the same `coordination_root`, `lock_target`, and effective Unix UID**
for normal/recovery access. The deployment worker's `coordination_root` and
manifest `server` must refer to that same directory/target. The legacy local
deployment mode without a coordination directory must not run alongside terminal
or recovery operations on the same host.

The guard holds an OS file lock and durably records a monotonic epoch and active
marker. EOF, process crash or missing exit acknowledgement leaves the marker set.
Recovery also sets `suspended`, which persists after a clean session close.
`host-guard -reconcile -epoch <observed>` is an explicit host operator assertion
that processes have stopped and runtime state was inspected. It refuses a live
lock and increments the epoch; previously prepared deployment plans become stale.
Expiry is never treated as proof that a remote command stopped.

Recovery audit uses append-only JSONL with event ID, origin, actor, target, epoch,
timestamp, action and result. The service fails closed on audit errors.
`GET /v1/recovery/audit?after=N` exports at most 200 events in an Ed25519-signed
batch. The normal API verifies a pinned key in `recovery_keys` before accepting
`POST /v1/recovery/import`. PostgreSQL enforces `(origin,event_id)` uniqueness and
acknowledges a cursor. Retries cannot duplicate events or advance across gaps.
Audit import does not clear host suspension or resume automation.
