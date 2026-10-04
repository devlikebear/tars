# Console

The TARS console is the browser workbench for the local runtime at
`http://127.0.0.1:43180/console`. It is designed for inspection first:
sessions, tool calls, runs, memory candidates, approvals, logs, usage, and
configuration stay close to the work they affect.

## Navigation

The sidebar keeps Home on the TARS logo and groups working pages into Work,
Operate, and Setup. The footer keeps server, Pulse, Reflection, and active
session status visible with periodic refreshes and direct navigation to each
detail page.

The console header includes an EN/KO language toggle. The selected locale is
stored in browser localStorage as `tars_console_locale`; first load falls back
to `navigator.language` and then English.

Set `usage.limits.daily_tokens` or `usage_daily_token_budget` to show a daily
token budget chip in the header. The chip uses UTC day boundaries, counts input
plus output tokens, hides when set to `0`, and links to the analytics view when
usage reaches the error threshold.

## Chat Workbench

Chat is the primary work surface. It combines agent conversation with the
panels needed to understand and control a session:

- Multi-session chat with full LLM tool-calling loops, transcript snippet
  search, pinned sessions, non-destructive archives, rule-based cleanup
  suggestions, and AI-reviewed archive/delete cleanup candidates.
- Stable transcript message IDs and lineage metadata for message-level forks.
  A forked child session reuses setup, plan state, tool policy, prompt
  override, and workspace roots through the selected message.
- Tool calls as collapsible rows with live elapsed time, compact argument
  previews, and automatic error expansion.
- `@` file, directory, and subagent mentions for explicit context injection.
- `/` command autocomplete for client commands and user-invocable skills.
- First-turn cost/quality tier recommendation before the first expensive chat
  call. The Context HUD records the selected tier and whether the
  recommendation was accepted or overridden.
- Session Config for per-session tool, group, skill, MCP, automation consent,
  approved Git mutation, autonomous workspace mutation, and style overrides.
  Permission changes include deterministic risk/impact previews before save.
- Dock Manager support for Sessions, files, terminal, Git Inspector, Tasks,
  Evidence, Agent Runtime, Prior Context, Session Health, Skill Inbox, and
  other side panels.
- Files workspaces with integrated shell launch at the selected root or browsed
  subdirectory, plus a macOS Terminal fallback.
- Workspace file previews through workspace storage by default; selected
  filesystem roots are served through the explicit filesystem files API
  boundary.

## Tasks And Evidence

The Chat header shows the active plan goal, completed/total task count,
workbench actions, and Session Health. The Chat Contract panel keeps goal,
scope, done criteria, verification commands, expected artifacts, approval
status, and evidence together. The Chat Tasks panel keeps full plan progress
visible and accepts tests, logs, screenshots, PRs, releases, and command output
as task evidence. Its Timeline tab prefers current session-owned durable work,
falls back to imported legacy-session history, and subscribes directly to Work
Ledger events. Active work can be cancelled after confirmation. Steps in
`review` or `blocked` appear under Operator attention only when their durable
schedule requires human resume; the entered resume reason is retained in the
event history.

The global Plans page lists active plans across sessions. Archived plan data is
available through `/v1/admin/plans/archive` for future planning views.

## Memory

Durable recall is built from `MEMORY.md`, reviewed experiences, daily logs,
semantic embeddings, and structured transcript compaction. The Memory Inbox
lets users approve, reject, or merge reflection-derived candidates before they
enter durable recall. Stored memory assets can be edited in the console, and
Tool path vs Prefetch path recall tests are available through the console and
API.

Fresh workspaces omit legacy KB Wiki scaffolding while preserving any existing
`memory/wiki` files.

## Agent Runtime

The Agent Runtime page exposes `Runs` and `Subagents` tabs.

`Runs` supports filtering by status, time range, and prompt text; list, tree,
Gantt, and Flow views; originating chat links; today/7d/plan cost totals;
timestamped event replay; failed-run restart from prompt/failure checkpoints;
per-run cost/token flow; file attention for frequently read or edited paths;
and a git diff timeline that attributes workspace file changes to the run,
session, agent, and plan step.

`Subagents` shows the active catalog, default/effective LLM tier, resolved
provider/model, source file or command entry, tool policy, and recent run
links. Workspace `AGENT.md` profiles can update their default tier, draft new
subagents with the assisted builder, generate run-derived profile
recommendations with provenance, preview and approve LLM edits, and archive
inactive workspace profiles.

Parallel and compare-mode subagent calls render progress cards in Chat. Compare
mode keeps outputs side-by-side and highlights common findings, conflicts, and
sourced evidence snippets.

## Operations

Mission Control summarizes Pulse, Reflection, active plans, Agent Runtime runs,
Cron jobs, disk pressure, sessions, notifications, setup actions, and
release/PR shortcuts.

Pulse is the watchdog surface for cron failures, stuck runs, stalled chats,
disk pressure, delivery health, and reflection state. Reflection is the nightly
batch surface for memory extraction and stale empty-session pruning. Cron,
Approvals, Logs, and Analytics provide the rest of the operator loop.

## Setup

Settings contains Quick Start onboarding, structured object and array editing,
typed `llm.tiers` editing, subsystem-aware pending-change impact previews, and
field metadata badges. Settings field metadata, subsystem impact hints,
preferred YAML patch paths, and compatible nested YAML parsing are kept in the
config field registry so console schema and config file updates stay aligned.
Pending changes also get a frontend fallback classifier so new fields still
show the affected subsystem before save.

Frontend API response types are maintained in the console source with the
current contract policy documented in
[frontend-api-types.md](frontend-api-types.md).
