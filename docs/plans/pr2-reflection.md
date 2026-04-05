# PR2 — Nightly Reflection

**Branch**: `feat/nightly-reflection`
**Depends on**: PR1 (pulse surface) — merged

## Purpose

Introduce the second half of the system surface: **reflection**, a once-per-day batch runner that handles memory and knowledge-base maintenance during a configurable sleep window. This removes expensive LLM-driven work from the per-turn chat hot path and consolidates it into a nightly pass where latency doesn't matter.

## Scope (fixed by prior discussion)

Reflection is scoped to exactly two responsibilities:

1. **Memory cleanup** — experience extraction and knowledge-base compilation moved out of per-turn `chat_memory_hook.go` into a nightly batch job. Daily log consolidation (optional).
2. **KB cleanup** — hygiene operations on the knowledge base and session transcript store. PR2 Phase 1 ships with the safest operation: **remove empty sessions** (sessions with zero messages older than a configurable age). Session compression is a deliberate follow-up.

**Not in scope** (intentionally):
- Session transcript compression (requires read-side decompression changes — large blast radius)
- Memory decay / aging (future)
- Self-memory / TARS identity expansion (permanently excluded per prior decision)
- Reflection GUI beyond a minimal status endpoint

## Design

### Package layout

```
internal/reflection/
├── reflection.go         # Runtime (Start, Stop, RunOnce), Config
├── scheduler.go          # sleep window + once-per-day tick gate
├── jobs.go               # Job interface + result types
├── job_memory.go         # memoryCompactionJob — moves chat_memory_hook logic to batch
├── job_kb_cleanup.go     # kbCleanupJob — removes empty sessions
├── state.go              # runtime state: last run, last result, job history ring
├── types.go              # shared types (JobResult, RunSummary, Severity alias)
└── *_test.go             # tests for each
```

### Runtime model

- **One tick per minute** (cheap — just checks sleep window + "already run today" flag).
- If inside sleep window AND today's run has not happened yet AND enabled → run all jobs sequentially.
- Each job returns a `JobResult { Name, Success, Changed, Details, Err }`.
- Results aggregate into a `RunSummary` stored in state and exposed via HTTP.

### Sleep window

- Default `02:00-05:00` local time, configurable (`reflection_sleep_window`).
- Wrap-around windows supported (same parser as pulse — reuse it).
- Sleep window is *permissive*: reflection picks any minute inside the window to run, not a specific moment. If the window is skipped (machine asleep), the next in-window tick picks it up.

### Memory cleanup job

Moves these from `internal/tarsserver/chat_memory_hook.go`:
- `maybeCompileKnowledgeBase` (full LLM call)
- `deriveAutoExperiences` / `deriveUserAutoExperience` / `deriveAssistantAutoExperience`
- `appendExperienceIfNew`

Into a nightly batch that:
1. Iterates over all sessions updated in the last 24 hours (via `session.Store.ListAll`).
2. For each session, reads the last N messages and runs the existing derivation/compile logic per turn.
3. Writes experiences and knowledge-base updates in aggregate.

The per-turn hook (`applyPostChatMemoryHooks`) shrinks to only:
- Daily log append (still real-time, cheap)
- Explicit "remember" hot path (user-initiated, must be instant for recognition)

Everything else deferred to reflection.

### KB cleanup job

Phase 1 operation only: **remove empty sessions**.

- Scan `session.Store.ListAll()`
- For each session where `ReadMessages(TranscriptPath)` returns zero messages AND `UpdatedAt` is older than `reflection_empty_session_age` (default 24h)
- Call `session.Store.Delete(id)`
- Main session (`kind=main`) and active sessions are never touched
- Report `{removed_count, skipped_count}` in job result

### Pulse ↔ reflection integration

Pulse's signal scanner gains a `ReflectionHealthSource` interface:

```go
type ReflectionHealthSource interface {
    ConsecutiveFailures() int
    LastRunAt() time.Time
}
```

A new `SignalKindReflectionFailure` is emitted when:
- `ConsecutiveFailures() >= threshold` (default 3, configured via `pulse_reflection_failure_threshold`)

Reflection implements this interface directly from its state. Wiring happens in `buildPulseRuntime` by passing `reflectionRuntime` as one of the scanner sources.

### HTTP API

Minimal — matches pulse's style:

- `GET  /v1/reflection/status`  → JSON state snapshot (last run, recent results, counters)
- `POST /v1/reflection/run-once` → synchronously triggers all jobs; returns summary
- `GET  /v1/reflection/config`   → read-only config view

### Tool registry scope

Reflection uses `RegistryScopeReflection`. Since reflection has **no LLM tool surface** in PR2 (jobs are pure Go, the LLM is called directly for knowledge compilation via `llm.Client.Chat` without tool calls), no tool registration is needed. The scope exists for forward-compat — if reflection later grows a `reflection_decide`-style tool, it'll register there.

## Config schema

New fields in `AutomationConfig`:

| Go field | YAML | Env | Default |
|---|---|---|---|
| `ReflectionEnabled` | `reflection_enabled` | `TARS_REFLECTION_ENABLED` | `true` |
| `ReflectionSleepWindow` | `reflection_sleep_window` | `TARS_REFLECTION_SLEEP_WINDOW` | `02:00-05:00` |
| `ReflectionTimezone` | `reflection_timezone` | `TARS_REFLECTION_TIMEZONE` | `Local` |
| `ReflectionTickInterval` | `reflection_tick_interval` | `TARS_REFLECTION_TICK_INTERVAL` | `5m` |
| `ReflectionEmptySessionAge` | `reflection_empty_session_age` | `TARS_REFLECTION_EMPTY_SESSION_AGE` | `24h` |
| `ReflectionMemoryLookbackHours` | `reflection_memory_lookback_hours` | `TARS_REFLECTION_MEMORY_LOOKBACK_HOURS` | `24` |
| `ReflectionMaxTurnsPerSession` | `reflection_max_turns_per_session` | `TARS_REFLECTION_MAX_TURNS_PER_SESSION` | `20` |
| `PulseReflectionFailureThreshold` | `pulse_reflection_failure_threshold` | `TARS_PULSE_REFLECTION_FAILURE_THRESHOLD` | `3` |

## Chat memory hook changes

`applyPostChatMemoryHooks` → shrinks to:

```go
func applyPostChatMemoryHooks(input chatMemoryHookInput) error {
    // Always: append daily log (cheap, user-visible)
    if err := memory.AppendDailyLog(...); err != nil {
        return err
    }
    // Hot path: user explicitly asked to remember
    if shouldPromoteToMemory(input.UserMessage) {
        // append memory note + one experience, synchronously
    }
    return nil
}
```

Removed from hot path:
- `deriveAutoExperiences` / auto derivation
- `maybeCompileKnowledgeBase` (LLM call)
- All keyword-based auto extraction

These now run inside the nightly reflection job over the last 24h of sessions.

## Commit sequence

Each commit leaves the tree buildable.

1. **chore: add PR2 plan for nightly reflection**
2. **feat: add reflection package skeleton (types, state, config)**
3. **feat: implement reflection scheduler and runtime**
4. **feat: add reflection memory cleanup job** — batch variant of knowledge compile + experience derivation
5. **feat: add reflection KB cleanup job** — remove empty sessions
6. **feat: add reflection HTTP handler (status, run-once, config)**
7. **feat: wire reflection into server lifecycle**
8. **refactor: trim per-turn chat memory hooks** — remove code now owned by reflection
9. **feat: add reflection-failure signal to pulse** — surface via `SignalKindReflectionFailure`
10. **docs: update CLAUDE.md with reflection surface notes**

## Test plan

- Unit tests for each reflection package component
- Sleep window parsing, wrap-around cases, malformed windows
- Once-per-day gate (same day skip, next day run)
- Memory job against a fake llm.Client and session store
- KB cleanup job with mixed empty/non-empty/main sessions
- Pulse reflection-failure signal with a fake health source
- HTTP handler with a fake runtime
- Integration: end-to-end `reflection.Runtime.RunOnce()` exercises scheduler → jobs → state

Full `go test ./...` green after every commit.

## Open decisions resolved

- **Reflection scope**: memory cleanup + KB cleanup only (fixed upstream)
- **Policy location**: config, no REFLECTION.md
- **Session compression**: deferred to a follow-up
- **Self-memory**: permanently excluded
- **Per-turn hook**: shrinks to daily log + "remember" hot path only
- **Batch vs per-turn logic**: Phase 1 does a loop over sessions and calls existing per-turn functions. Full batch-consolidation LLM call is a future optimization.
