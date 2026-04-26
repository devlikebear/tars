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
