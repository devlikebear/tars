# Usage Signals

TARS records lightweight, workspace-local usage signals in `workspace/usage/signals-YYYY-MM-DD.jsonl`.
These signals are counters only: they avoid prompts, tool arguments, command text, file contents, and model outputs.

## Inspect

- API: `GET /v1/usage/signals?period=today`
- CLI: `/usage signals today`
- Periods: `today`, `week`, `month`

## Review Questions Covered

| Question | Signal |
| --- | --- |
| Q-011 `process` tool frequency | `tool_call` rows with `tool=process` |
| Q-012 subagent planning/orchestration frequency | `tool_call` rows with `tool=subagents_plan`, `tool=subagents_orchestrate`, or `tool=subagents_run` |
| Q-013 `claude-code-cli` provider frequency | Existing `GET /v1/usage/summary?group_by=provider` rows with `key=claude-code-cli` |
| Q-014 persistence retry/mismatch frequency | `agentruntime.persist_snapshot.retry`, `.error`, and `.mismatch_final` |
| Q-015 consensus activation frequency | `agentruntime.consensus.started` |
| Q-016 nodes replacement signal | No replacement signal is needed; the nodes tool was removed rather than replaced |
| Q-017 `SessionToolConfig` frequency | `session.tool_config.updated` |
| Q-018 plan/task frequency | `tool_call` rows with `tool=tasks`, grouped by `action` |

Rows may include sanitized dimensions such as `tool`, `action`, `mode`, `task_count`, `step_count`, `variant_count`, and `strategy`.

## Decision Snapshot - 2026-04-26 to 2026-04-30

Input files:

- `workspace/usage/signals-2026-04-26.jsonl`
- `workspace/usage/signals-2026-04-27.jsonl`
- `workspace/usage/signals-2026-04-28.jsonl`
- `workspace/usage/signals-2026-04-30.jsonl`

The review window contains 428 sanitized signal rows across 15 sessions. It does not include prompt text, command text, file contents, or model output.

| Question | Observed signal | Decision | Follow-up |
| --- | --- | --- | --- |
| Q-011 `process` tool frequency | 0 `tool_call` rows with `tool=process` | Externalize or deprecate the LLM-facing `process` surface if one more fresh window stays at zero. | [#510](https://github.com/devlikebear/tars/issues/510) |
| Q-012 subagent planning/orchestration frequency | 0 `subagents_plan`, 0 `subagents_orchestrate`, and 4 `subagents_run` calls across 3 sessions; all observed runs used `mode=parallel`. | Simplify in place: keep strengthening `subagents_run`, and stop treating plan/orchestrate as routine default surfaces unless fresh data changes. | [#509](https://github.com/devlikebear/tars/issues/509); keep [#396](https://github.com/devlikebear/tars/issues/396) aligned if plan/orchestrate remains. |
| Q-014 persistence retry/mismatch frequency | 2 `agentruntime.persist_snapshot.retry` rows in 1 agent run session; 0 `.error` and 0 `.mismatch_final` rows. | Keep the 2-attempt retry for now; the retry path is used, and the window does not show final mismatch or write errors. | No implementation issue from this window. Revisit only if `.error` or `.mismatch_final` appears. |
| Q-015 consensus activation frequency | 0 `agentruntime.consensus.started` rows and 0 `subagents_run` calls with `mode=consensus`. | Externalize or deprecate consensus mode if one more fresh window stays at zero. | [#507](https://github.com/devlikebear/tars/issues/507) |
| Q-017 `SessionToolConfig` frequency | 0 `session.tool_config.updated` rows. | Simplify in place: keep underlying safety policy only if needed, but reduce always-visible Console/prompt surface unless fresh data shows use. | [#508](https://github.com/devlikebear/tars/issues/508) |
| Q-018 plan/task frequency | 185 `tool_call` rows with `tool=tasks` across 9 sessions: 77 `add`, 67 `update`, 17 `plan_set`, 12 `plan_propose`, 8 `list`, and 4 `plan_approve`. | Keep and strengthen. Unlike the low-use candidates above, plan/task behavior is actively used. | Continue the existing Console plan UX issues [#393](https://github.com/devlikebear/tars/issues/393), [#394](https://github.com/devlikebear/tars/issues/394), [#395](https://github.com/devlikebear/tars/issues/395), and [#396](https://github.com/devlikebear/tars/issues/396). |

## Follow-up Notes

- 2026-05-01 Q-011 / [#510](https://github.com/devlikebear/tars/issues/510): a fresh check of the available `workspace/usage/signals-*.jsonl` files still showed 0 `tool_call` rows with `tool=process`. TARS now gates `process` out of the default chat tool schema; explicit session tool allowlists can still opt in, and background `exec` keeps using the shared process manager.
- 2026-05-01 Q-012 / [#509](https://github.com/devlikebear/tars/issues/509): a fresh check still showed 0 `subagents_plan`, 0 `subagents_orchestrate`, and 4 `subagents_run` calls, all `mode=parallel`. TARS now keeps `subagents_run` as the default delegated-work tool and gates `subagents_plan` / `subagents_orchestrate` behind explicit session tool opt-in.
- 2026-05-01 Q-017 / [#508](https://github.com/devlikebear/tars/issues/508): a fresh check still showed 0 `session.tool_config.updated` rows. TARS now removes the always-visible Chat toolbar `Config` button, keeps explicit `/config` access for selected sessions, and preserves backend `SessionToolConfig` filtering plus telemetry for diagnostics.
- 2026-05-01 Q-015 / [#507](https://github.com/devlikebear/tars/issues/507): a fresh check still showed 0 `agentruntime.consensus.started` rows and 0 `subagents_run` calls with `mode=consensus`. TARS now hides consensus from the default `subagents_run` schema unless `agentruntime.consensus.enabled=true`, and disabled calls are rejected before a run is spawned.

## Decision Refresh - reviewed 2026-08-02

The latest available workspace signal files were re-read for [the agent-control-plane baseline](agent-harness/market-scan-2026-08-02.md). The additional window from 2026-05-01 through the latest recorded event on 2026-05-20 contains 1,144 sanitized rows across 22 sessions:

| Surface | Additional-window count | Combined interpretation |
| --- | ---: | --- |
| `subagents_plan` | 0 | Keep behind explicit session opt-in. |
| `subagents_orchestrate` | 0 | Keep behind explicit session opt-in until durable plans improve evaluation outcomes. |
| `subagents_run` | 0 | Retain as the single default delegation primitive; the preceding window had four calls, all parallel. |
| `agentruntime.consensus.started` | 0 | Keep disabled and hidden by default until verifier/cost evaluation demonstrates value. |

The deterministic Agent Harness Evaluation Pack independently exercises parallel fan-out, dependency handoff, and partial-child failure without requiring a default-visible planner or consensus surface. Session Tasks remain the high-signal planning surface and should evolve into the Durable Work Ledger UI.

The signal stream available for this review ends on 2026-05-20. These counts are a current review of the latest available local telemetry, not a claim of data collection through August.

## Anthropic cache-breakpoint measurement (#921)

`GET /v1/usage/summary?period=today&group_by=shape` splits recorded calls into
`with-tools` and `no-tools` rows.

The split exists because Anthropic renders `tools` → `system` → `messages` into
one prefix-matched cache key. A request that omits `tools` therefore cannot hit
an entry written by a tool-bearing request, even inside the same agent turn.
`agent.Loop` ends every turn with one tools-absent call (`ToolChoice: none`),
so each turn produces both shapes.

**What the rows mean**

| Row | Expected if placement is working | Suspected regression looks like |
| --- | --- | --- |
| `with-tools` | `cache_read_tokens` grows with conversation length; `cache_write_tokens` stays near one prefix per turn | reads flat at 0 — a silent invalidator upstream |
| `no-tools` | reads roughly match the previous turn's writes | writes each turn with reads at 0 — paying the 1.25x write premium for an entry nothing reads back |

If `no-tools` shows sustained writes with near-zero reads, the tools-absent
final call is a net loss and the fix is to keep `tools` on that call and rely
on `tool_choice: none` (already supported by `toAnthropicToolChoice`) so it
shares the turn's cache lineage.

**Caveats**

- `tool_count` is recorded per call from this change onward. Entries written
  before it default to 0 and land in `no-tools`, so restrict any comparison to
  a window that starts after the upgrade.
- Anthropic's minimum cacheable prefix is ~1024 tokens; below that nothing
  caches and both rows read 0 regardless of placement.
- Ephemeral entries expire after 5 minutes, so a turn that takes longer than
  that between calls will miss for reasons unrelated to placement.
