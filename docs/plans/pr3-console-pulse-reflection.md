# PR3 — Console Pulse & Reflection Settings

**Branch**: `feat/console-pulse-reflection-settings`
**Depends on**: PR1 (pulse), PR2 (reflection) — both merged

## Purpose

Make the pulse and reflection system surfaces first-class citizens in the console UI. PR1 shipped a minimal `Pulse.svelte` runtime view; PR2 shipped the reflection backend + HTTP API but **no** frontend view at all. PR3 closes that gap and makes it easy for operators to inspect status, trigger one-off runs, and find the relevant config fields.

## Key observation — config editing is already free

PR1 + PR2 registered all `pulse_*` and `reflection_*` fields in `internal/config/config_input_fields.go` and `schema.go`, so the existing `Config.svelte` Settings page **already** shows and edits them under the Automation section via `/v1/admin/config/values` PATCH. PR3 therefore does **not** need a custom policy editor — the Settings page works today.

What's missing is runtime visibility: reflection has no status view in the console, and `Pulse.svelte` in PR1 was minimal (run-once button + snapshot; no signals display, no recent-ticks detail).

## Scope

Frontend only. No Go changes except possibly small fixes if I catch anything.

### 1. New Reflection view (primary work)

Mirror of `Pulse.svelte` pointed at the `/v1/reflection/*` endpoints:

- Runtime status card: enabled, sleep window, timezone, tick interval, last run, total runs/successes/failures, consecutive failures, last successful run
- Last run summary card: per-job results with success/changed badges, details dropdown, error pill
- Run tick now button (calls `POST /v1/reflection/run-once`, bypasses sleep-window gate)
- Recent runs ring: reverse chronological list of `RunSummary` entries from `snapshot.recent`

### 2. Nav + router wiring

- Add `Reflection` nav item next to `Pulse` (icon `☾` — moon for nightly)
- Router route `/console/reflection` → `{ view: 'reflection' }`
- `App.svelte` imports + renders the new component
- Existing `/console/heartbeat` legacy redirect already points to pulse; no legacy reflection path to worry about

### 3. API + types

- `frontend/console/src/lib/api.ts`:
  - `getReflectionStatus()` → `/v1/reflection/status`
  - `runReflectionOnce()` → `/v1/reflection/run-once`
  - `getReflectionConfig()` → `/v1/reflection/config`
- `frontend/console/src/lib/types.ts`:
  - `ReflectionJobResult`, `ReflectionRunSummary`, `ReflectionSnapshot`, `ReflectionConfigView`
  - Type shapes mirror the Go structs in `internal/reflection/types.go` and `internal/reflection/state.go`

### 4. Minor Pulse.svelte polish

While in the area, tighten the existing Pulse view:
- Show signals from `last_decision.details` when present (currently shown only as a summary line)
- Add a "Recent decisions" list filtered to ticks where `decider_invoked=true` so the noise of silent ticks doesn't dominate
- Add a "Quick settings" link that jumps to the Settings page filtered to `pulse_*` fields (if the Settings search supports deep-linking; if not, skip)

### 5. Home dashboard hookup

The home "pulse strip" (PR1) currently shows only pulse stats. Extend it to also surface reflection health:
- Add a "Last reflection" cell showing `relativeTime(snapshot.last_successful_run_at)` with a warn color when `consecutive_failures > 0`
- This gives operators a single place to notice that nightly runs have stopped working

### 6. Docs

- Short update to `CLAUDE.md` Frontend section noting the new routes

## Out of scope

- Custom pulse/reflection policy editors (Settings page handles this via schema)
- Writable `/v1/pulse/config` or `/v1/reflection/config` endpoints (writes go through `/v1/admin/config/values`)
- Any Go backend changes
- Translation / i18n (existing frontend is English-only)

## Commit sequence

1. **chore: add PR3 plan for console pulse/reflection settings**
2. **feat(frontend): add reflection API client + types**
3. **feat(frontend): add Reflection.svelte runtime view**
4. **feat(frontend): wire Reflection into nav, router, and App**
5. **feat(frontend): extend Home dashboard with reflection health**
6. **feat(frontend): polish Pulse.svelte with signals and decisions list**
7. **docs: update CLAUDE.md frontend routes**

## Test plan

- `make console-build` (npm install + vite build + svelte-check + tsc) must pass
- `go test ./...` must remain green (no Go changes expected, but worth verifying since `go:embed` picks up built assets)
- Manual smoke list to be verified locally via `make dev-console`:
  - `/console/pulse` renders with live updates
  - `/console/reflection` renders, shows empty-state when no runs yet
  - "Run tick now" / "Run reflection now" buttons trigger the real endpoints
  - Home dashboard shows both pulse and reflection status strips
  - Navigating from Nav works in both directions
  - Pulse/Reflection `*_*` fields editable in `/console/config` under Automation

## Future follow-ups (not PR3)

- Unified "System" nav section with tabs for Pulse / Reflection / Signal history
- Real-time SSE stream for tick outcomes (currently polled at 30s)
- Chart/sparkline of recent tick counts per decision action
