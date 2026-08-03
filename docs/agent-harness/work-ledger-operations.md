# Work Ledger operations

The Work Ledger is a local SQLite database opened through `internal/workstore`.
It uses WAL mode, foreign-key enforcement, full synchronous writes, versioned
migrations, and `0600` database permissions.

The default path is `<workspace>/_shared/work-ledger/work-ledger.db`. On startup,
TARS imports the current `sessions/sessions.json`, per-session `*.tasks.json`,
and Agent Runtime `runs.json` documents as checksum-addressed, append-only
revisions. Import never deletes or rewrites a source file. Session task saves
and Agent Runtime snapshots continue to update their compatibility stores and
then synchronize the new revision to the ledger.

## Safety rules

- Keep the active database and every backup on a trusted local filesystem.
- Backup, restore, JSONL export, and quarantine operations never overwrite an
  existing destination. Choose a new path or move the existing file yourself.
- A restore target must not be the active database path.
- Large outputs remain external artifacts referenced by URI and digest; they
  are not copied into SQLite unless an operator explicitly archives them.
- A quarantined source is copied, not moved. The original remains in place for
  forensic review and manual recovery.

## Capture a preview pre-migration snapshot

Before enabling a preview release against an existing workspace:

1. Stop TARS so legacy files are not changing during the copy.
2. Copy `<workspace>/sessions/` and
   `<workspace>/_shared/agentruntime/` to a new timestamped backup directory.
3. Record a SHA-256 digest and byte count for every copied file.
4. Keep the snapshot until at least one release after the Work Ledger is stable.
5. Start TARS, confirm `work ledger bootstrap completed` appears without a
   startup failure, and run `Store.Doctor` before relying on the projections.

The importer itself leaves these legacy inputs in place, but an independent
snapshot protects the exact pre-upgrade point from later ordinary session or
Agent Runtime writes.

## Create and verify a backup

Call `Store.Backup(ctx, destination)`. The operation serializes local writers
and uses SQLite `VACUUM INTO`, so committed records still present in the WAL are
included. It validates `quick_check`, foreign keys, and migration checksums
before publishing the backup atomically with `0600` permissions.

Persist the returned SHA-256 digest with the backup. A destination collision is
reported as `ErrDestinationExists`; it is never treated as permission to
replace the existing backup.

## Restore a backup

Call `workstore.RestoreBackup(ctx, source, newTarget, options)` while the target
path does not exist. Restore performs these checks before publishing the new
database:

1. SQLite can open the source read-only.
2. `PRAGMA quick_check` reports `ok`.
3. `PRAGMA foreign_key_check` reports no violations.
4. Known migration checksums match and no unknown future migration exists.

The restored copy is opened once through the normal Work Ledger migration path
and then closed. Start TARS with the new target only after comparing the
reported digest and running `Store.Doctor`.

## Export audit data as JSONL

Call `Store.ExportJSONL(ctx, workspaceID, destination)`. Records are written in
a deterministic order as a header followed by Work, Step, dependency, Attempt,
Event, Approval, Artifact, Proof, effect-receipt, CapabilityVersion,
EvaluationRun, CapabilityOutcome, and import-marker envelopes.
The final line
contains a SHA-256 digest over every preceding line plus per-type record counts.

The export is a portable audit artifact, not an in-place database replacement.
Use a SQLite backup for operational restore. Verify a JSONL file by hashing all
bytes before the `checksum` footer and comparing the hexadecimal digest.

## Run doctor

Call `Store.Doctor(ctx, workspaceID)` before and after migrations, restores, or
manual incident recovery. A healthy report requires:

- SQLite quick-check and foreign-key checks to pass;
- every applied migration checksum to match the binary;
- stored JSON documents to decode;
- terminal Work, Step, and Attempt records to have terminal timestamps;
- the Step dependency graph to be acyclic; and
- effect receipts to contain valid outcome JSON, required idempotency contract
  fields, and timestamps consistent with pending or committed status; and
- capability snapshots, provenance, permissions, rollout policy, metrics,
  deltas, and reports to contain valid JSON, with required passed evaluation
  stages and human approval for reviewed canary/promoted versions; and
- every Work ID referenced by an import marker to exist in that workspace.

`Doctor` is read-only. It reports issue codes and record IDs but does not repair
or delete data automatically.

## Quarantine a corrupt legacy source

Call `Store.QuarantineSource` with the source kind, stable source ID, original
path, quarantine directory, reason, actor, and workspace. The operation writes:

- an exact `0600` copy named with timestamp and SHA-256 prefix;
- a JSON manifest containing provenance, digest, size, reason, and marker; and
- a `quarantined` import marker in the ledger.

Repeating the operation for the same source bytes returns the existing marker
and paths. If the source changes, its checksum changes and a separate quarantine
record is created. Never delete the original automatically; decide retention
only after the copy, manifest, and digest have been independently verified.

## Disable the Work Ledger and roll back

The preview rollback is a reader/write-path switch, not a down-migration:

```yaml
work_ledger:
  enabled: false
```

Alternatively set `TARS_WORK_LEDGER_ENABLED=false`. Restart TARS after changing
either value. Disabled startup does not open, migrate, import into, truncate, or
delete the ledger database. Session task reads/writes return to the legacy
session files, Agent Runtime list/detail reads use the live runtime and its
legacy snapshot, and `/v1/work/*` responds with service unavailable.

If the live legacy files must be returned to the exact pre-migration point,
stop TARS and restore the independently captured `sessions/` and Agent Runtime
directories from the snapshot above. This intentionally discards legacy-file
changes made after that snapshot; compare timestamps and digests first. Keep
the disabled SQLite database and any JSONL export until recovery is verified.

To restore a Work Ledger backup instead, restore it to a new path with
`workstore.RestoreBackup`, validate its digest and doctor report, stop TARS,
replace the inactive ledger path only under an explicit operator recovery
procedure, and then re-enable `work_ledger.enabled`. Never copy only a live
`.db` file while its WAL may contain committed data.

## Enable or roll back the durable scheduler

The scheduler is disabled by default because enabling it allows accepted
staged subagent work to continue after the originating chat or HTTP request
ends. Enable it only with Agent Runtime configured:

```yaml
work_ledger:
  enabled: true
  scheduler:
    enabled: true
    max_workers: 4
    lease_seconds: 60
    heartbeat_seconds: 20
    poll_milliseconds: 250
    execution_environment: local
    execution_data_dir: $HOME/.tars/execution-plane
    artifact_paths: []
```

The heartbeat must be shorter than the lease. All numeric values must be
positive. Explicit scheduler enablement fails startup when either Work Ledger
or Agent Runtime is disabled, so the runtime cannot silently fall back while
the operator believes durable execution is active. The equivalent environment
variables are
`TARS_WORK_SCHEDULER_ENABLED`, `TARS_WORK_SCHEDULER_MAX_WORKERS`,
`TARS_WORK_SCHEDULER_LEASE_SECONDS`,
`TARS_WORK_SCHEDULER_HEARTBEAT_SECONDS`, and
`TARS_WORK_SCHEDULER_POLL_MILLISECONDS`. Execution-plane settings use
`TARS_WORK_SCHEDULER_EXECUTION_ENVIRONMENT`,
`TARS_WORK_SCHEDULER_EXECUTION_DATA_DIR`, and
`TARS_WORK_SCHEDULER_ARTIFACT_PATHS_JSON`.

When enabled, `subagents_orchestrate` stores the entire dependency graph before
moving its Work to `running`, returns `work_id` by default, and uses the
request-bound response only when `wait_for_completion` is explicitly set. On
restart, the scheduler reconnects a still-valid Agent Runtime attempt when the
executor can identify it. Otherwise it releases or reclaims the lease and
applies the recorded bounded policy. A Step never implies exactly-once external
side effects; mutating-tool idempotency and effect receipts are a separate
control-plane capability. Every attempt now uses the execution-plane lifecycle;
see [Execution-plane operations](execution-plane.md) for local versus disposable
worktree behavior, artifact filtering, recovery, and cleanup, and see
[Agent Runtime checkpoint recovery](checkpoint-recovery.md) for the crash
boundaries, human-decision path, and executor limitations.

Operator APIs are workspace-scoped:

- `GET /v1/work/works/{work_id}/wait?timeout_ms=30000` waits for a terminal or
  human-attention state and returns `202` with the current projection on timeout.
- `GET /v1/work/works/{work_id}/watch?after_sequence=N` streams ordered
  `work_event` SSE messages after the requested cursor.
- `POST /v1/admin/work/works/{work_id}/cancel` requires JSON
  `{"reason":"..."}` and cancels unfinished steps.
- `POST /v1/admin/work/works/{work_id}/steps/{step_id}/resume` requires JSON
  `{"reason":"..."}` and only resumes a durable human-attention step.

To roll back execution without deleting history, set
`work_ledger.scheduler.enabled: false` (or
`TARS_WORK_SCHEDULER_ENABLED=false`) and restart. New orchestration requests use
the legacy request-bound path; existing ledger records remain inspectable. Stop
or cancel active work before the restart when immediate execution shutdown is
required.
