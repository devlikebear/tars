# Agent and Harness Market Scan — 2026-08-02

## Scope and status rules

This scan uses official repositories, release pages, and product documentation checked on 2026-08-02. It distinguishes three evidence levels:

- **Stable** — present in a non-prerelease tagged release or current stable documentation.
- **Pre-release** — present only in a release explicitly marked beta/prerelease.
- **Proposal / inference** — a TARS issue or a directional inference from shipped work; not a competitor commitment.

No official future-dated product roadmap was found for OpenClaw or Hermes Agent. Their “direction” sections below are therefore clearly labeled inferences, not vendor promises.

Implementation status was refreshed on 2026-08-03 for TARS `v0.35.0`. The
competitor evidence and release snapshot remain fixed at the original
2026-08-02 verification date; the gap table below is the pre-implementation
baseline that drove the delivered roadmap.

## Release snapshot

| Project | Latest stable used | Pre-release observed | Official evidence |
| --- | --- | --- | --- |
| OpenClaw | `v2026.7.1`, published 2026-07-13 | `v2026.7.2-beta.6`, published 2026-08-01 | [Stable release](https://github.com/openclaw/openclaw/releases/tag/v2026.7.1), [beta release](https://github.com/openclaw/openclaw/releases/tag/v2026.7.2-beta.6) |
| Hermes Agent | `v0.19.1` / `v2026.7.30`, published 2026-07-30 | None newer in GitHub Releases at verification time | [Latest stable](https://github.com/NousResearch/hermes-agent/releases/tag/v2026.7.30), [feature release v0.19.0](https://github.com/NousResearch/hermes-agent/releases/tag/v2026.7.20) |
| TARS | `v0.34.3`, published 2026-06-23 | None | [TARS releases](https://github.com/devlikebear/tars/releases), [control-plane epic #902](https://github.com/devlikebear/tars/issues/902) |

## OpenClaw: current state and direction

### Stable evidence

OpenClaw is now a broad multi-surface control plane rather than a CLI-only agent:

- Native subagents are isolated background runs with configurable nesting, model overrides, sandbox policy, completion events, and a `sessions_yield` wait primitive. [Official subagent docs](https://docs.openclaw.ai/tools/subagents)
- ACP sessions attach external coding harnesses such as Claude Code, Gemini CLI, OpenCode, and explicit Codex ACP, with persistent/thread-bound modes and runtime controls. ACP currently runs on the host rather than inside the OpenClaw sandbox, which is an important trust-boundary caveat. [Official ACP docs](https://docs.openclaw.ai/tools/acp-agents)
- Detached subagents, ACP runs, automations, CLI operations, and background execution appear in a shared task ledger with queued/running/terminal states, notification policies, audit, cancellation, and retention. [Background task docs](https://docs.openclaw.ai/automation/tasks)
- Automations persist job definitions, runtime state, and run history in shared SQLite state so schedules survive restarts. [Automation docs](https://docs.openclaw.ai/automation/cron-jobs)
- The stable `v2026.7.1` release emphasizes the Control UI, connected coding agents, resumable goals/sessions, approvals, scheduled work, remote browser control, terminals, and recovery. [Stable release](https://github.com/openclaw/openclaw/releases/tag/v2026.7.1)

### Pre-release evidence

`v2026.7.2-beta.6` is not treated as stable. It nevertheless shows where OpenClaw engineering effort is moving:

- crash-recoverable SQLite snapshots and quarantine/rollback state;
- durable ingress and dead-letter recovery across channels;
- message-level session rewind and branching;
- broader approval/question surfaces;
- MCP Apps and durable dashboards;
- remote coding sessions and managed worktrees;
- more durable scheduling and task-ledger integration.

Source: [OpenClaw `v2026.7.2-beta.6`](https://github.com/openclaw/openclaw/releases/tag/v2026.7.2-beta.6).

### Directional inference

OpenClaw's likely near-term direction is a distributed, multi-surface agent operating system: one task/session control plane coordinating native agents, external harnesses, paired nodes, channels, and durable automation. This is an inference from its stable and beta release themes, not a published roadmap.

The TARS lesson is not to copy OpenClaw's surface breadth. It is to copy the separation of concerns: durable task identity, execution-runtime adapters, observable lifecycle, and recovery semantics.

## Hermes Agent: current state and direction

### Stable evidence

Hermes has moved well beyond ephemeral `ThreadPoolExecutor` delegation:

- `delegate_task` runs isolated children with restricted tools and separate terminals; current docs state three concurrent subagents by default. [Feature overview](https://hermes-agent.nousresearch.com/docs/user-guide/features/overview/)
- `v0.19.0` ships live subagent transcripts, durable background-delegation completions, a `state.db` delivery-obligation ledger, default smart approvals, explicit deny rules, durable cron audit history, and a performance harness that measures cold start and first token. [v0.19.0 release](https://github.com/NousResearch/hermes-agent/releases/tag/v2026.7.20)
- `v0.18.1` adds goal completion contracts with verification evidence, first-class coding projects/worktrees, cheaper auxiliary-model self-improvement, `/learn`, and scale-to-zero/drain coordination. [v0.18.1 release](https://github.com/NousResearch/hermes-agent/releases/tag/v2026.7.7)
- Agent-managed skills and background review can stage procedural knowledge, while a write-approval gate can require human review. [Skills docs](https://hermes-agent.nousresearch.com/docs/user-guide/features/skills)
- `v0.19.1` is a stabilization tag rolling the post-0.19.0 changes into a downstream-consumable release; its notes defer the full new feature narrative to v0.20.0. [v0.19.1 release](https://github.com/NousResearch/hermes-agent/releases/tag/v2026.7.30)

### Directional inference

Hermes's likely direction is “verified, self-improving personal agent”: persistent goals, proof-backed completion, durable delivery/delegation, lower-latency interaction, reviewed procedural learning, and richer coding-project workflows. This is an inference from successive stable releases, not a published v0.20+ commitment.

The strongest TARS lessons are completion contracts, effect/delivery durability, a measured latency budget, and learning that is proposed and reviewed rather than silently mutating the agent.

## TARS position

### Defensible strengths

- A small local-first Go deployment with one binary, an embedded browser console, and inspectable filesystem state.
- Clear heavy/standard/light model-tier routing with per-run traceability.
- A native Agent Runtime with tool allowlists, depth control, checkpoint metadata, run topology, file attention, and diff attribution.
- Durable Markdown memory, semantic retrieval, reviewed extraction, and progressive-disclosure skills.
- Public Go packages for building smaller embedded agents without importing server internals.

### Baseline gaps confirmed before v0.35.0

| Gap | Evidence | Consequence |
| --- | --- | --- |
| No unified durable unit of work | Tasks, agent runs, cron, approvals, and delivery each have separate records | Operators cannot reason about one lifecycle across triggers and runtimes. |
| Active runs do not resume | `active-run-restart-gap` restores accepted/running work as canceled | A gateway restart creates manual recovery work. |
| No effect receipt / idempotency boundary | `duplicate-side-effect-gap` repeats the fake external effect on checkpoint replay | “Retry” can duplicate a real-world action. |
| Proof is optional and surface-specific | `false-success-detected` shows claimed success can differ from artifact truth | The agent can stop on assertion rather than independently checked evidence. |
| External harness integration is provider-shaped | TARS can call CLI/provider backends but lacks a common session/run adapter contract | Resume, cancel, approvals, streaming, and usage differ by backend. |
| Learning is not a complete governed loop | Skills and reviewed memory exist, but there is no eval-backed candidate→review→promote→rollback lifecycle | Self-improvement cannot safely optimize against measured outcomes. |
| No remote execution plane | All work assumes the local runtime | TARS cannot place work by capability, isolation, or availability. |

The deterministic baseline recorded 12/12 expectation matches, but only 66.7% task success, 75.0% verifier pass, a 16.7% restart-recovery signal across the full pack, one duplicate side effect, and seven operator interventions. “Expectation match” means the baseline is reproducible; it does not hide these quality gaps.

## Product strategy

TARS should become a **durable local agent control plane**, not compete on the number of channels, providers, or built-in tools.

The target architecture is:

```text
Trigger (chat / cron / API / webhook)
  -> Durable Work Ledger (identity, state, dependencies, policy, budget)
  -> Scheduler and recovery loop (leases, retries, wakeups)
  -> Execution adapter (native / CLI harness / remote worker / A2A)
  -> Effect receipts and approvals
  -> Independent proof verifier
  -> Reviewed learning proposals
```

Every phase must improve or protect a metric in the evaluation pack. New surfaces should not become default-visible merely because they exist.

## Roadmap and release gates

| Phase | Status | Deliverable | GitHub issue | Exit evidence |
| --- | --- | --- | --- | --- |
| 0 | Delivered in `v0.35.0` | Evaluation pack, official-source comparison, usage decision, storage ADR | [#906](https://github.com/devlikebear/tars/issues/906) | 10+ deterministic CI scenarios and versioned baseline |
| 1 | Delivered in `v0.35.0` | Durable Work Ledger | [#905](https://github.com/devlikebear/tars/issues/905) | Atomic claim/state transitions, migrations, queryable lifecycle |
| 2A | Delivered in `v0.35.0` | Durable scheduler and restart recovery | [#904](https://github.com/devlikebear/tars/issues/904) | Active work survives restart without duplicate execution |
| 2B | Delivered in `v0.35.0` | Checkpoints and effect receipts | [#907](https://github.com/devlikebear/tars/issues/907) | Idempotency keys and receipt-backed replay |
| 2C | Delivered in `v0.35.0` | Proof verifier and completion gates | [#909](https://github.com/devlikebear/tars/issues/909) | “Done” requires independent evidence when a contract exists |
| 3 | Delivered in `v0.35.0` | Execution-plane / Claude Code harness adapter | [#910](https://github.com/devlikebear/tars/issues/910) | Native and external harnesses share lifecycle, cancellation, usage, transcript, and artifact semantics; unsupported resume is explicit |
| 4 | Delivered in `v0.35.0` | Reviewed self-improvement | [#908](https://github.com/devlikebear/tars/issues/908) | Eval-backed proposals, human review, promotion, rollback |
| 5 | Delivered in `v0.35.0` | Remote workers and A2A gateway | [#903](https://github.com/devlikebear/tars/issues/903) | Capability placement, authenticated transport, leases, failure recovery |

The umbrella is [Epic #902](https://github.com/devlikebear/tars/issues/902). The
implementation preserved the intended dependency order: phases 1–2 establish
the reliability core used by execution adapters, reviewed improvement, remote
workers, and A2A.

## Default-visible surface decision

The latest available local usage data was re-read on 2026-08-02. The post-snapshot window from 2026-05-01 through 2026-05-20 contains 1,144 sanitized rows across 22 sessions and records zero calls to `subagents_plan`, `subagents_orchestrate`, or `subagents_run`, plus zero consensus starts. The earlier 2026-04-26 through 2026-04-30 window recorded four `subagents_run` calls, all parallel, and zero plan/orchestrate/consensus use.

The new deterministic pack proves that direct parallel fan-out, dependency handoff, and partial-failure reporting can be evaluated without exposing a separate planner or consensus surface by default. Therefore:

- keep `subagents_run` as the default delegated-work primitive;
- keep `subagents_plan` and `subagents_orchestrate` behind explicit session opt-in: the Work Ledger now makes their plans durable, but the available usage/evaluation evidence still does not justify default exposure;
- keep consensus disabled and hidden by default until it beats a single strong model on verifier pass rate within a declared token/cost budget;
- preserve the actively used session Tasks surface and evolve it into the Work Ledger view rather than adding another competing planner UI.

The available signal stream ends on 2026-05-20, so this is a current review of the latest available local evidence, not a claim of continuous telemetry through August. See [usage signals](../usage-signals.md).
