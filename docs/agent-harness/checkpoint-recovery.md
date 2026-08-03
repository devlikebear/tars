# Agent Runtime checkpoint recovery

Agent Runtime checkpoints are versioned recovery contracts, not claims that an
arbitrary process can be serialized. Every checkpoint declares its format,
executor capability, supported recovery modes, whether it is resumable, and a
reason when it is not.

## Formats and capabilities

| Format or capability | Meaning |
| --- | --- |
| `prompt_checkpoint_v0` | A legacy prompt snapshot. It remains visible and supports only `retry_from_prompt`. |
| `step_checkpoint_v1` | A boundary snapshot with normalized run/session/workspace references plus tool request, result, effect-receipt, and optional provider-continuation references. |
| `retry_only` | The executor exposes neither replayable tool state nor a continuation handle. Command executors use this capability. |
| `replay` | Receipt-safe recorded tool results can be injected if the recovered model asks for the same canonical tool call. |
| `resumable_step` | Replay is available and a recorded provider session handle can enable continuation. Without a handle, the individual checkpoint degrades to `replay`. |
| `environment_rehydratable` | Reserved capability contract for a future executor that can prove workspace and environment snapshot restoration. Current prompt and command executors do not claim it. |

The optional `workspace_snapshot_refs` and `environment_snapshot_refs` fields
are part of the v1 contract, but current executors leave them empty. A git diff
timeline is observability data, not a restorable workspace image. TARS therefore
does not advertise universal process, shell, worktree, or environment resume.

## Recovery modes

`POST /v1/agentruntime/runs/{run_id}/restart` accepts an explicit `mode`:

```json
{
  "checkpoint_id": "run_7_cp_5",
  "mode": "replay_from_checkpoint",
  "prompt_adjustment": "Continue with the remaining files."
}
```

| Mode | Behavior |
| --- | --- |
| `retry_from_prompt` | Starts a derived run from the saved prompt and recovery context. It does not inject cached tool results, so completed effects can be attempted again. |
| `replay_from_checkpoint` | Starts a derived run and supplies eligible recorded results instead of executing matching tool calls again. Repeated identical calls consume recorded results in their original order. |
| `resume_from_checkpoint` | Seeds the first provider turn with the recorded continuation handle. This is offered only when that checkpoint contains a handle. |

An omitted mode remains `retry_from_prompt` for API compatibility. The Console
uses distinct Retry, Replay, and Resume labels and prefers replay when it is
available. Failed and canceled runs can be recovered; the derived run preserves
the source run, checkpoint, attempt, durable Work/Task/Flow/Step correlation,
and provider override provenance.

## Tool-effect safety boundary

Local tools declare one recovery class:

- `read_only`: a pending call can be safely issued again;
- `idempotent`: a pending call is safe only when its declared downstream
  idempotency-key argument is present; or
- `unsafe`: an ambiguous pending effect requires a human decision.

For a durable scheduler run, Agent Runtime writes a pending effect receipt to
the Work Ledger before invoking a mutating or external local tool and commits
the receipt with the bounded, redacted result afterward. Recovery replays a
mutating result only when its receipt is committed. If a crash happens after
the external action but before receipt commit, TARS deliberately treats the
effect as ambiguous instead of claiming exactly-once execution.

| Crash boundary | Recovery posture |
| --- | --- |
| Receipt persistence fails before local tool invocation | Invocation is stopped. |
| Pending receipt exists, effect not known to have run | Read-only or contractually idempotent calls may retry; unsafe calls require approval. |
| External effect happened, result/receipt commit did not | The same pending ambiguity applies; an operator must decide for unsafe tools. |
| Result and effect receipt committed | Replay returns the recorded result and does not invoke the matching effect again. |

Approval-required recovery returns HTTP `409` with code
`recovery_approval_required`. After inspecting the external system, an operator
can submit the same request with `"confirm_unsafe_recovery": true`. Approval is
a recorded human decision to proceed despite ambiguity; it is not proof that
the previous effect did or did not happen.

Provider-executed tools are different: a CLI-backed provider reports them only
after it has already invoked them, so TARS cannot create a pre-effect receipt.
They are observed as pending unsafe effects and force human review after an
interrupted run.

## Persistence and operational limits

- Durable scheduler runs carry `work_id` and `step_id`, so effect receipts are
  committed to the SQLite Work Ledger and included in Work projections and
  deterministic JSONL exports.
- Standalone Agent Runtime runs have no Work Ledger effect boundary. Their tool
  records survive process restart only when Agent Runtime run persistence is
  enabled and `runs.json` was successfully written.
- `Store.Doctor` verifies effect-receipt JSON, required contract fields, and
  status/timestamp consistency. It reports corruption but never repairs it.
- Tool results are redacted and bounded to 64 KiB. A truncated result makes the
  checkpoint retry-only because replay could change semantics.
- Exactly-once behavior requires an idempotent downstream system. TARS provides
  ordering, receipts, fail-closed ambiguity, and replay suppression; it does
  not turn a non-idempotent external API into an exactly-once API.

During incident recovery, inspect the selected checkpoint's `resume_reason`,
`recovery_modes`, pending tool requests, and effect receipts before approving
an unsafe retry. Keep the Work Ledger backup and legacy `runs.json` snapshot
until the recovered Work reaches a verified terminal state.
