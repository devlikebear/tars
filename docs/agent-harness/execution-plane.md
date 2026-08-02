# Execution-plane operations

The execution plane separates durable scheduling from the worker process and
filesystem used for one Step Attempt. It is additive: the Work Ledger remains
the source of durable identity, policy, lease, proof, and audit state, while an
execution adapter owns the shorter provision/run/snapshot/collect/cleanup
lifecycle.

## Lifecycle and durable state

The native `agentruntime` adapter performs these operations in order:

1. provision or recover an environment;
2. issue a task-scoped credential grant when a broker is configured;
3. start or recover the worker;
4. record a worker checkpoint when supported;
5. snapshot the environment;
6. copy configured artifacts through the security filter;
7. revoke the credential grant; and
8. destroy only the environment owned by the provider.

Before each externally visible transition, the adapter atomically writes a
versioned JSON state document under
`work_ledger.scheduler.execution_data_dir/state/`. Files use private directory
and file permissions and contain grant identifiers, never credential values.
A terminal cleanup removes the state document. If TARS stops before cleanup,
the scheduler uses the saved environment and checkpoint to recover the same
Attempt rather than silently provisioning a second environment.

The Work Ledger records the following ordered events:

- `execution.environment_provisioned`
- `execution.credentials_issued`
- `execution.worker_started`
- `execution.checkpoint_recorded`
- `execution.environment_synced`
- `execution.artifacts_collected`
- `execution.credentials_revoked`
- `execution.environment_destroyed`
- `execution.recovery_started`
- `execution.worker_cancelled`

Provider, worker, environment, checkpoint, snapshot digest, and artifact count
are visible in the Console Tasks timeline. Credential values and environment
file contents are never included in event payloads.

## Environment modes

| Mode | Recover | Snapshot | Cleanup | Filesystem isolation | Credential isolation | Egress policy |
| --- | --- | --- | --- | --- | --- | --- |
| `local` | yes | yes | no-op | no | no | no |
| `managed-worktree` | yes | yes | yes | Git worktree | no | no |
| container provider | yes | yes | yes | container + worktree | yes | default deny |

`local` is the default and intentionally retains the existing shared-workspace
semantics. Its destroy operation is a no-op and never deletes user files.

`managed-worktree` requires `workspace_dir` to be a Git repository. It records
the source HEAD and a digest/count of the dirty source state, creates a detached
worktree under the execution data directory, and writes an ownership marker in
that worktree. Recovery validates the marker and Git registration. Cleanup
refuses any path outside the managed root or any worktree whose marker does not
match the durable environment identity. Tracked edits and untracked files in
the source repository are not copied, reset, or deleted.

The container provider contract uses a read-only root, default-deny
`--network none`, CPU, memory, PID, and tmpfs limits, and a disposable workspace
at `/workspace`. It rejects an allowlist it cannot enforce. It is not selectable
for the native Agent Runtime adapter: native tools execute in the host process,
so accepting `container` there would falsely claim container execution.
Instead, configure the internal remote-worker gateway and submit Work to its
explicit adapter (for example `remote-container`). That adapter now runs the
same durable provision/sync/execute/collect/destroy lifecycle and crash-safe
result handoff as remote workers. Startup never falls back to host execution.

## Configuration

Keep the execution data directory outside `workspace_dir`:

```yaml
work_ledger:
  enabled: true
  scheduler:
    enabled: true
    execution_environment: managed-worktree # local | managed-worktree
    execution_data_dir: $HOME/.tars/execution-plane
    artifact_paths:
      - reports
      - "dist/*.json"
```

The equivalent environment variables are:

- `TARS_WORK_SCHEDULER_EXECUTION_ENVIRONMENT`
- `TARS_WORK_SCHEDULER_EXECUTION_DATA_DIR`
- `TARS_WORK_SCHEDULER_ARTIFACT_PATHS_JSON`

Changing these values requires a restart. The data directory is not migrated
automatically. Stop or cancel active Work before changing it, retain the old
directory until every in-flight Attempt is terminal, and then apply the new
configuration.

## Artifact policy

Artifact paths are relative to the execution root and may contain local glob
patterns. Absolute paths and parent traversal are rejected. Collection has
bounded file-count and total-byte limits and skips symlinks, non-regular files,
`.git`, `.tars`, and `node_modules`.

Files with common sensitive names or extensions are excluded, including
`.env*`, credentials, secrets, tokens, private keys, and SSH identity files.
Any values issued by the task credential broker are replaced with
`[REDACTED]` in copied files and transcript artifacts. Originals are never
rewritten. Copies are stored with private permissions and referenced from the
Work Ledger by file URI, SHA-256 digest, media type, and byte count.

Filename filtering and exact-value redaction are defense-in-depth, not a data
classification system. Configure the narrowest possible paths and inspect the
first collected artifacts before relying on a new pattern.

## Rollback and incident handling

Set `work_ledger.scheduler.enabled: false` and restart to stop new execution;
this retains ledger history and execution data. Cancel active Work first when
immediate shutdown is required.

For a stranded managed worktree, do not delete its directory manually. Keep
the state document and ownership marker, restart with the same execution data
directory, and let recovery validate and clean it. If recovery reports an
ownership mismatch, preserve the path for inspection and use ordinary Git
worktree commands only after matching the Attempt ID, environment ID, source
repository, and marker contents.
