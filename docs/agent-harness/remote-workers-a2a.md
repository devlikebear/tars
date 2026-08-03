# Remote worker and A2A operations

TARS has two deliberately separate remote-execution boundaries:

- the internal worker protocol places a durable Step Attempt in a TARS-controlled
  environment; and
- the A2A adapter delegates a Step to an independently hosted agent through the
  public A2A HTTP+JSON protocol.

Neither path replaces the Work Ledger scheduler. MCP remains the tool and data
integration boundary. Both remote paths are disabled by default, so an existing
local-only installation continues to use the native Agent Runtime execution
plane.

## Internal worker protocol v1

Protocol `1.0` defines register, heartbeat, provision, workspace sync, lease,
execute, stream, checkpoint, collect, destroy, loss, reclaim, and rehydrate
messages. Every envelope has a message ID, idempotency key, worker/placement
identity, monotonically increasing sequence, and send time. Duplicate messages
return the previously committed result; conflicting duplicates and reordered
messages fail without mutating controller state.

The gateway controller persists atomic state at:

```text
<work_ledger.scheduler.execution_data_dir>/remote-workers/controller.json
```

Work-scoped placement transitions are also projected into the Work Ledger.
Worker registration and idle heartbeats remain in the controller audit log.
The Console **Approvals → Remote Execution** card reads a sanitized view from
`GET /v1/admin/workers`: it shows worker health, placement state, sync size,
resource/egress policy, checkpoint and recovery counts, and recent transitions.
It intentionally omits worker endpoints and event payloads.

Enable only the controller and observability surface with:

```yaml
work_ledger:
  enabled: true
  scheduler:
    enabled: true
    execution_data_dir: $HOME/.tars/execution-plane
    remote_workers:
      enabled: true
```

The equivalent environment switch is
`TARS_WORK_SCHEDULER_REMOTE_WORKERS_ENABLED=true`. Restart TARS after changing
the setting. Enabling the controller alone does not invent or auto-discover a
fleet and does not register a scheduler adapter.

## Bind one worker to the durable scheduler

Set `gateway_config_path` to register one explicitly configured container or
SSH worker as a Work Scheduler adapter:

```yaml
work_ledger:
  enabled: true
  scheduler:
    enabled: true
    execution_data_dir: $HOME/.tars/execution-plane
    remote_workers:
      enabled: true
      gateway_config_path: /var/lib/tars-gateway/remote-gateway.json
```

The equivalent environment setting is
`TARS_WORK_SCHEDULER_REMOTE_WORKERS_GATEWAY_CONFIG_PATH`. The JSON file must be
absolute, regular, non-symlinked, outside the source workspace, and owner-only
(`0600`). It contains the gateway's raw 64-byte Ed25519 private key encoded with
unpadded standard base64, so do not commit it or place it in the workspace.

An in-process container worker uses the same serialized protocol and lifecycle
as SSH:

```json
{
  "schema_version": 1,
  "adapter": "remote-container",
  "worker_id": "worker-preview-1",
  "transport": "in-process",
  "private_key": "BASE64_RAW_64_BYTE_ED25519_PRIVATE_KEY",
  "lease_ttl_seconds": 120,
  "token_ttl_seconds": 60,
  "sync_mode": "directory",
  "policy": {
    "egress": {"mode": "deny"},
    "limits": {
      "cpu_seconds": 3600,
      "memory_mb": 2048,
      "disk_mb": 4096,
      "max_output_bytes": 67108864
    }
  },
  "in_process": {
    "worker_config_path": "/var/lib/tars-gateway/worker.json"
  }
}
```

Submit Work with the exact adapter name, here `remote-container`. The gateway
ships only the bounded workspace plus Work title/objective and Step
title/instructions; Work contract and metadata blobs remain in the ledger.
Configured scheduler workers require default-deny egress in protocol v1.
Set `sync_mode` to `git` only with an explicit absolute `git_path`; directory
mode is the portable default.

The scheduler writes prepared inputs and verified results below
`execution_data_dir/remote-workers/scheduler/runs`. A result is persisted before
the placement is completed, then the journal is removed only after the Work
Ledger commits the Attempt and remote cleanup succeeds. After a gateway crash,
TARS replays the protocol message with fresh task authorization, retrieves the
worker's persisted result, and does not invoke the container task twice.

## Reference worker service

The reference worker accepts exactly one bounded JSONL request over stdio:

```bash
tars worker serve --stdio --protocol 1.0
```

Its default configuration path is `~/.tars/worker.json`. `--config` accepts an
absolute override. The file must be a regular file that is not group- or
world-writable, and unknown JSON fields are rejected.

```json
{
  "schema_version": 1,
  "worker_id": "worker-preview-1",
  "root_dir": "/var/lib/tars-worker/workspaces",
  "state_path": "/var/lib/tars-worker/state.json",
  "public_key": "BASE64_RAW_32_BYTE_ED25519_PUBLIC_KEY",
  "max_token_ttl_seconds": 60,
  "wire_limits": {
    "max_request_bytes": 268435456,
    "max_response_bytes": 67108864
  },
  "container": {
    "runtime_path": "/usr/bin/docker",
    "image": "registry.example/tars-worker@sha256:REPLACE_WITH_64_HEX_DIGEST",
    "command": ["/usr/local/bin/tars-task"],
    "cpus": "1",
    "pids_limit": 128,
    "supports_resume": true
  }
}
```

Replace the placeholders before use. The image must be digest-pinned. The
configured public key is the raw 32-byte Ed25519 verification key encoded with
unpadded standard base64 and must match the gateway's task-token issuer.

The container is launched read-only with no inherited environment, no network,
bounded CPU/memory/PIDs/output, and tmpfs-backed `/workspace` and `/tmp`.
Provider credentials and long-lived signing material are never sent to the
worker. Planning and model inference stay at the gateway boundary; the worker
receives only a short-lived, placement-bound, scope-bound task token and the
bounded request needed for that Attempt.

## SSH pilot requirements

The SSH transport invokes this fixed remote command:

```text
tars worker serve --stdio --protocol 1.0 [--config /absolute/worker.json]
```

It always enables batch mode, strict host-key checking, identity-only auth,
disables forwarding/local commands/TTY allocation, ignores user SSH config,
and uses explicit absolute identity and `known_hosts` files. A pilot host must
therefore have:

1. the matching TARS binary and locked-down worker configuration;
2. an unprivileged SSH account whose path resolves `tars`;
3. a dedicated identity file on the gateway;
4. a pre-populated, pinned `known_hosts` file; and
5. a digest-pinned container image already available to the worker runtime.

Use this gateway configuration for the single-host SSH pilot:

```json
{
  "schema_version": 1,
  "adapter": "remote-ssh",
  "worker_id": "worker-preview-1",
  "transport": "ssh",
  "private_key": "BASE64_RAW_64_BYTE_ED25519_PRIVATE_KEY",
  "lease_ttl_seconds": 120,
  "token_ttl_seconds": 60,
  "sync_mode": "directory",
  "policy": {
    "egress": {"mode": "deny"},
    "limits": {
      "cpu_seconds": 3600,
      "memory_mb": 2048,
      "disk_mb": 4096,
      "max_output_bytes": 67108864
    }
  },
  "wire_limits": {
    "max_request_bytes": 335544320,
    "max_response_bytes": 67108864
  },
  "capabilities": {
    "resume": true,
    "streaming": false,
    "checkpoints": true,
    "egress_policy": true,
    "resource_limits": true,
    "artifact_scan": true
  },
  "ssh": {
    "ssh_path": "/usr/bin/ssh",
    "host": "worker.example.com",
    "user": "tars-worker",
    "port": 22,
    "identity_file": "/var/lib/tars-gateway/ssh/worker_ed25519",
    "known_hosts_file": "/var/lib/tars-gateway/ssh/known_hosts",
    "worker_config_path": "/var/lib/tars-worker/worker.json"
  }
}
```

The worker registration attests both capabilities and the task-token
verification-key ID. Duplicate execute/collect/destroy messages ignore the old
ephemeral token only for idempotency comparison, then require a newly valid,
correctly bound token before returning a persisted result.

Do not put a password, token, or user-info segment in a persisted worker
endpoint. The controller rejects credential-bearing endpoints, redacts failure
text, and records only an endpoint digest in audit events.

## Workspace sync, policy, and artifacts

Directory and Git-aware bundle modes use sorted manifests with SHA-256 file and
manifest digests. Source, mutable workspace, and released artifact ownership is
explicitly gateway/worker/gateway. Symlinks, `.git`, `.tars`, dependency trees,
sensitive filenames, oversize files, and manifest mismatches fail closed.

The reference container currently accepts only default-deny egress. Resource
limits must be positive. Artifacts return to a gateway-owned quarantine, where
path confinement, media type, size, digest, symlink, executable, and credential
checks run before release. Rejected artifacts make the remote result fail.

If a lease expires or the transport is lost, the controller marks the worker
and placement lost. Recovery first records reclaim, then rehydrates the same
Work/Step/Attempt in a replacement environment with a verified workspace
snapshot and eligible checkpoint. A fresh task token is issued; the old token
is not persisted or reused.

## A2A external-agent adapter

A2A is configured independently from the internal worker controller:

```yaml
work_ledger:
  enabled: true
  scheduler:
    enabled: true
    a2a:
      enabled: true
      discovery_url: "https://agent.example"
      bearer_token: "${TARS_WORK_SCHEDULER_A2A_BEARER_TOKEN}"
      allowed_hosts: ["agent.example"]
      allow_private_hosts: false
      allow_insecure_loopback: false
      poll_milliseconds: 2000
      max_poll_seconds: 1800
```

At startup TARS discovers `/.well-known/agent-card.json`, validates the A2A 1.0
card, and binds the executor adapter name `a2a-http-json`. Scheduler submissions
must explicitly select that adapter. A submitted external task ID and its
observed states are journaled in the Work Ledger, so restart recovery polls the
same task instead of sending the message again. Cancellation calls the remote
task cancel route. Input-required and auth-required states stop for operator
action rather than being treated as success.

Discovery and task endpoints require HTTPS by default. Redirects are blocked,
same-host or explicit host allowlisting is enforced, private hosts are denied
unless explicitly allowed, payloads are bounded, and bearer credentials remain
in memory. URL/file/raw artifact parts are quarantined and their contents are
not persisted. `allow_insecure_loopback` exists only for a local development
agent.

The implementation follows A2A 1.0 HTTP+JSON discovery, message send, task get,
and task cancel routes. See the [A2A protocol specification](https://github.com/a2aproject/A2A/blob/main/docs/specification.md)
and [official documentation](https://a2a-protocol.org/latest/).

## Disable and recover

Set both remote switches to `false` and restart:

```yaml
work_ledger:
  scheduler:
    remote_workers:
      enabled: false
    a2a:
      enabled: false
```

This stops new remote placements/delegations without deleting controller or
Work Ledger records. Keep those records for audit and recovery. If controller
state fails validation, preserve the file, disable remote workers, and inspect
the last valid Work Ledger transition before attempting any manual repair.
Remove `gateway_config_path` as well when rolling back the scheduler adapter;
never delete a surviving run journal until its Attempt and placement state have
been reconciled.
