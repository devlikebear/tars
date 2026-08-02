# Independent proof verification

TARS treats a worker result and proof of that result as different records. A
successful executor attempt can finish with `succeeded` while its Step and Work
remain in `review` or `blocked` until the declared proof policy is satisfied.

## Proof lifecycle and authority

Proof state is explicit and durable:

| State | Meaning |
| --- | --- |
| `reported` | A worker, operator, or legacy import reported evidence. It has no completion authority. |
| `pending` | An independent verifier has claimed the check but has not produced a terminal decision. |
| `passed` | An independent verifier completed the check with valid provenance. |
| `failed` | The independent check disproved or could not establish the required outcome. |
| `stale` | The files, commit, artifact, or URL represented by the saved subject digest changed. |

Every proof records its reporter, verifier identity and implementation, verifier
environment, command or structured input, artifact digests, aggregate subject
digest, timestamps, and rationale. A `passed` record satisfies a scheduler gate
only when the verifier and reporter IDs differ, the verifier environment is
non-empty, and the subject digest and rationale are present. `Store.Doctor`
reports invalid JSON, lifecycle timestamps, authority, and pass provenance.

Schema migration v6 deliberately converts every proof written by the older
three-state schema into `legacy/reported`, even if it previously said `passed`.
Session evidence without the new provenance contract is imported the same way.
An upgrade therefore cannot grant completion authority to an old assertion.

## Declaring a gate

Durable Steps carry their policy in `step_schedules.policy_json`:

```json
{
  "max_attempts": 1,
  "escalation_state": "review",
  "proof": {
    "required": true,
    "failure_state": "review",
    "requirements": [
      {
        "kind": "test",
        "verifier": "deterministic",
        "command": "go test ./internal/workstore",
        "paths": ["internal/workstore"]
      }
    ]
  }
}
```

All declared kinds must have a non-stale independent pass for the active
Attempt. A worker report, a self-verification using the worker identity, a
different verifier implementation than the one declared, or proof attached to
another Attempt cannot satisfy the gate. Missing, pending, failed, or stale
proof moves the Step to the policy's `review` or `blocked` state and requires an
operator decision before resume.

Session Task Contracts expose the same operator intent through
`proof_policy.required`, `verifier`, and `failure_state`. The Console labels
manual and old evidence as **Reported only** and verified evidence as
**Independently verified**, and its durable timeline shows reporter, verifier,
rationale, and subject digest.

## Deterministic verifier

The built-in verifier runs as a dedicated subprocess role with provider keys,
tokens, passwords, and credential variables removed from its environment. It
supports:

- approved commands, including exit code, bounded output excerpts, output
  hashes, duration, and before/after subject snapshots;
- files, directories, logs, screenshots, and other artifacts with SHA-256
  digests and optional expected-digest matching; and
- PR, release, and other HTTPS URLs with status, redirect, ETag,
  Last-Modified, and bounded body hashes.

Paths are confined to the verifier root. URL checks default to HTTPS, reject
private, loopback, link-local, multicast, and unspecified addresses, recheck
redirects, cap redirect count, and bound response bodies. A caller must opt in
explicitly to HTTP or private-network verification.

Call `Engine.SubjectDigest` without rerunning a command to recalculate a file,
workspace, artifact, commit, or URL snapshot, then pass that digest to
`Store.DetectStaleProof`. A mismatch atomically transitions a terminal proof to
`stale` and emits a ledger event.

## LLM judge boundary

An LLM judge is optional and is considered only when the requirement declares
no command, path, artifact, or URL verifier. The Step policy must explicitly
allow it and set positive token and USD ceilings. Missing approval, missing
budgets, invalid usage, or observed usage beyond either ceiling leaves proof
`pending` for operator review; it never becomes a silent pass. Deterministic
checks always take precedence and do not invoke the judge.

## Fan-out policy

Current consensus remains advanced and opt-in. A future policy-selected
(`automatic=true`) fan-out is rejected unless it records an OH-001 baseline ID,
a positive expected quality delta, and a decision reason. The durable Run then
retains expected tokens/cost, configured budgets, fan-out, completed/failed
variants, observed tokens/cost, and the observed outcome. This prevents an
automatic strategy from spending more merely because parallel execution is
available.

## Operational limits

- A digest proves that the observed bytes or response matched the verifier's
  snapshot; it does not prove the artifact is semantically correct unless the
  deterministic command establishes that property.
- A URL can change immediately after verification. Recalculate it at the final
  delivery gate when freshness matters.
- Command verification is intended for commands approved in a Task Contract;
  it is not a sandbox for untrusted shell text.
- Container and managed-worktree verifier environments are execution-plane
  concerns tracked by Phase 3. This phase establishes the identity,
  environment, provenance, and completion-gate contracts they must implement.
