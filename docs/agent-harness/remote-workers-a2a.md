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
the setting. The preview SSH pilot is one gateway-to-worker placement built
with `workerprotocol.NewSSHTransport` and `workerprotocol.NewGatewayCoordinator`;
enabling the controller alone does not invent or auto-discover a fleet.

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
tars worker serve --stdio --protocol 1.0
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
