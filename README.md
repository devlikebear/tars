<div align="center">
  <p>
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="docs/brand/tars-readme-header-dark.png" />
      <img src="docs/brand/tars-readme-header.png" alt="TARS — local AI agent runtime" width="560" />
    </picture>
  </p>
  <p>
    <a href="https://tars.marvin-42.com"><img src="https://img.shields.io/badge/website-tars.marvin--42.com-e09145?style=flat" alt="Website" /></a>
    <a href="https://github.com/devlikebear/tars/actions/workflows/ci.yml"><img src="https://github.com/devlikebear/tars/actions/workflows/ci.yml/badge.svg" alt="CI" /></a>
    <a href="https://github.com/devlikebear/tars/actions/workflows/codeql.yml"><img src="https://github.com/devlikebear/tars/actions/workflows/codeql.yml/badge.svg" alt="CodeQL" /></a>
    <a href="https://codecov.io/gh/devlikebear/tars"><img src="https://codecov.io/gh/devlikebear/tars/graph/badge.svg" alt="codecov" /></a>
    <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/devlikebear/tars" alt="Go" /></a>
    <a href="https://github.com/devlikebear/tars/releases"><img src="https://img.shields.io/github/v/release/devlikebear/tars" alt="Release" /></a>
  </p>
  <p><strong>TARS is a local AI agent runtime that runs on your machine, under your control.</strong></p>
  <p><strong>Homepage:</strong> <a href="https://tars.marvin-42.com">tars.marvin-42.com</a> — project overview, features, and quickstart.</p>
</div>

> [!IMPORTANT]
> **Archived.** This repository is read-only and no longer developed. `v0.35.0` (2026-08-03) is the last tagged release, and `main` carries unreleased changes on top of it — including the console narrowing described below, which is in `main` but not in any release. This README describes `main` as archived. It still builds, still installs, and the MIT license still applies, but there will be no further fixes, releases, or answered issues: read it as a record of what was built rather than a roadmap. Fork it if you want to take it further.

TARS is a local agent runtime for people who want an inspectable AI workbench without handing workspace control to a hosted service. It packages a browser console, API server, CLI, background jobs, memory, and extension system into one Go binary.

The core promise is direct control:

- Run agent chat, subagents, cron jobs, watchdog checks, and reflection on your own machine.
- Inspect sessions, tool calls, memory candidates, run history, approvals, usage, logs, and config from the console.
- Route work across heavy, standard, and light model tiers with role-level defaults and traceable runtime logs.
- Extend behavior with skills, companion CLIs, plugins, MCP servers, Telegram, webhooks, and local APIs without bloating the default prompt.

The name comes from the TARS in *Interstellar* — practical, direct, dependable when things get complicated. TARS aims for that. Not affiliated with the film; the name is borrowed.

## Comparison

| | OpenClaw | Hermes Agent | TARS |
|---|---|---|---|
| **Release used** | Stable `v2026.7.1` | Stable `v0.19.1` (`v2026.7.30`) | `v0.35.0` (final release) |
| **Packaging** | TypeScript Gateway plus web/native apps and plugins | Python agent/gateway plus TUI, web, and desktop surfaces | Go single binary with embedded browser console and CLI |
| **Delegation / harnesses** | Native subagents, Codex runtime, and ACP-backed external harness sessions | Isolated `delegate_task` children, live transcripts, MoA, and coding-runtime adapters | Native Agent Runtime plus an opt-in bounded Claude Code execution adapter, model tiers, tool policy, depth limits, and experimental consensus |
| **Durable async work** | Background-task ledger plus SQLite-backed automations | Durable Kanban/goals, delegated-result recovery, and delivery-obligation ledger | SQLite Work Ledger plus an opt-in dependency scheduler with leases, retries, budgets, and operator escalation |
| **Restart behavior** | Persistent automation/task records; the latest beta adds broader crash recovery | Gateway auto-resume and durable delegation/delivery recovery | Opt-in staged flows reconnect or reclaim attempts; standalone runs restore as canceled and then expose capability-bounded Retry, Replay, or provider Resume |
| **Proof of completion** | Completion handoff asks the parent to verify; task audit surfaces unhealthy work | Goal completion contracts and recorded verification evidence | Proof-gated Steps separate worker reports from deterministic independent verification, provenance, and stale evidence |
| **Scheduling** | Persistent Automations, cron alias, heartbeat monitors | Cron, blueprints, watchdog/no-agent jobs | Session cron, Pulse, Reflection, and an opt-in durable dependency scheduler |
| **Skills / learning** | Skills and plugin catalog | On-demand skills, `/learn`, background review, and Curator | On-demand skills, reviewed memory extraction, skill creation, and Skill Hub |

Verified on **2026-08-02** from official project documentation and release pages. OpenClaw `v2026.7.2-beta.6` capabilities are treated as pre-release, not stable. See the [status-separated market scan](docs/agent-harness/market-scan-2026-08-02.md) and [reproducible evaluation baseline](docs/agent-harness/README.md) for sources, limitations, and the TARS evolution roadmap.

## Key Features

### Public Agent Packages

Public packages under `pkg/` turn the runtime primitives into a small Go agent-building kit. Lightweight apps can reuse provider-normalized LLM contracts, the tool registry, the iterative agent loop, memory helpers, skill loading, and MCP adaptation without copying `internal/` code.

- `pkg/llm`, `pkg/tools`, and `pkg/agentloop` are the core loop: provider client, registered tools, and tool-calling iterations.
- `pkg/memory`, `pkg/skill`, and `pkg/mcp` expose the first helper surfaces for durable memory, `SKILL.md` loading, and MCP tool adaptation.
- `examples/min-agent` shows the minimal no-network composition, while [docs/public-agent-packages.md](docs/public-agent-packages.md) documents package boundaries and what still intentionally remains internal.
- That page also carries the [stability policy](docs/public-agent-packages.md#stability-policy): which packages are stable versus experimental, what deprecation means here, and how a breaking change is caught. The surface itself is checked in at [docs/public-api-surface.txt](docs/public-api-surface.txt) and diffed in CI, so a change to what consumers can rely on shows up in review.

### Chat + Memory

The primary interface is the browser console at `http://127.0.0.1:43180/console`.

- Multi-session chat with tool-calling loops, session search, pins, archives, cleanup review, and message-level forks.
- Dockable panels for sessions, files, terminal, Git inspection, tool calls, prior context, Tasks, Session Health, and session-specific policy.
- Durable memory through `MEMORY.md`, reviewed experiences, daily logs, semantic search, structured compaction, and a Memory Inbox review queue.
- Explicit context injection through `@` file/directory mentions, `/` command autocomplete, and user-invocable skills.
- Configurable system prompts through `USER.md`, `IDENTITY.md`, `AGENTS.md`, and `TOOLS.md`.
- A locale-aware companion surface for Poke, Suggest, Feedback, runtime signals, and bounded handoff into Chat.

See [docs/console.md](docs/console.md) for the detailed console page and panel inventory.

### Durable Work Ledger

TARS opens a local SQLite Work Ledger at `workspace/_shared/work-ledger/work-ledger.db`. Versioned Work, Step, Attempt, Event, Approval, Artifact, Proof, CapabilityVersion, EvaluationRun, and CapabilityOutcome records use workspace-scoped idempotency keys and transactional state transitions. SQLite runs in WAL mode with foreign keys, full synchronous writes, checksummed migrations, and indexed read projections.

During the compatibility window, startup imports existing Session Goal/Plan/Task/Contract/Evidence and Agent Runtime `runs.json` records without deleting or rewriting those files. Session task saves and Agent Runtime snapshots append new ledger revisions; existing task/run API shapes read the latest projection with a legacy fallback.

The durable dependency scheduler is deliberately opt-in. Set `work_ledger.scheduler.enabled: true` only after Agent Runtime is configured, then restart. New `subagents_orchestrate` calls submit the complete DAG, return a `work_id`, and continue independently from the originating request. Atomic leases and heartbeats prevent duplicate valid claims; recovery reconnects supported Agent Runtime attempts or deterministically reclaims them into the configured retry, replan, decompose, review, or blocked policy. Every scheduled attempt now passes through a common execution-plane lifecycle. `local` preserves the shared workspace behavior, while opt-in `managed-worktree` provisions a detached disposable Git worktree, routes native Agent Runtime tools and diff capture into that root, snapshots and collects configured non-sensitive artifacts, and then removes only the owned worktree. An owner-only config can register the single `claude-code` external coding pilot: it reuses the local Claude login, runs only in a disposable worktree with safe mode, strict MCP isolation, scoped coding tools, and time/turn/cost limits, and retains a redacted patch, transcript, and usage record rather than applying changes to the source checkout. The optional internal remote-worker protocol adds versioned placement, sync, scoped authorization, SSH transport, artifact quarantine, and reclaim/rehydrate state; an owner-only gateway config can register one container or SSH worker as an explicit durable scheduler adapter with crash-safe result handoff. The optional A2A 1.0 adapter delegates only explicitly selected Steps to an independent external agent. Every optional external or remote path is disabled by default and none replaces MCP or Work Ledger scheduling. Mutating local tools write pending and committed effect receipts around invocation, so checkpoint replay can suppress a completed effect and require a human decision for an ambiguous unsafe effect. Proof-gated Steps record worker output as `reported`, run declared command/artifact/HTTPS verification under a separate identity and environment, and stop in review or blocked when evidence is missing, failed, or stale. The Tasks timeline follows scheduler, execution-environment, worker, and A2A events live and allows an operator to cancel work or resume a reviewed/blocked step with a durable reason. The Approvals page adds a sanitized Remote Execution control-plane view for worker health, placements, sync, checkpoints, and recovery.

Set `work_ledger.enabled: false` (or `TARS_WORK_LEDGER_ENABLED=false`) and restart to stop ledger writes and return readers to the untouched legacy stores. The SQLite database is retained for inspection or recovery. See [Work Ledger operations](docs/agent-harness/work-ledger-operations.md) for backup, JSONL export, restore, doctor, quarantine, and preview rollback procedures, [execution-plane operations](docs/agent-harness/execution-plane.md) for environment capabilities, artifact policy, and cleanup/recovery behavior, [remote worker and A2A operations](docs/agent-harness/remote-workers-a2a.md) for worker service, SSH, sync, recovery, and external-agent configuration, [checkpoint recovery](docs/agent-harness/checkpoint-recovery.md) for retry/replay/resume semantics, [independent proof verification](docs/agent-harness/proof-verification.md) for proof policies, provenance, freshness, and deterministic verifier limits, and [reviewed self-improvement](docs/agent-harness/reviewed-self-improvement.md) for capability evaluation, approval, rollout, regression review, and rollback.

### Sub-Agent Orchestration

Spawn read-only agents for research, planning, and specialized tasks:

```yaml
# workspace/agents/explorer/AGENT.md
---
name: explorer
tier: light
tools_allow: [read_file, list_dir, glob, memory_search]
---
```

Use `subagents_run` when tasks are independent and can fan out in parallel:

```json
{"tasks": [
  {"prompt": "find all API endpoints", "tier": "light"},
  {"prompt": "design the migration plan", "tier": "heavy"}
]}
```

Use compare mode when 2-3 safe subagents should inspect the same prompt independently and TARS should keep their outputs side-by-side:

```json
{"mode":"compare","tasks": [
  {"agent": "explorer", "title": "Explorer pass", "prompt": "find the root cause"},
  {"agent": "reviewer", "title": "Reviewer pass", "prompt": "find the root cause"}
]}
```

In Console Chat, `subagents_run` renders as a progress card with running/completed/failed counts, elapsed time, compact task titles, and direct links to each Agent Runtime run once they are available. Compare-mode results add common findings, conflict candidates, sourced evidence snippets, and side-by-side output panes.

Advanced staged-flow tools are available only when explicitly allowed for a session: `subagents_orchestrate` runs dependency-aware `parallel` / `sequential` steps, and `subagents_plan` uses the heavy-tier planner model to draft such a flow. With the durable scheduler enabled, orchestration is projected directly from the Work Ledger instead of independently mutating the legacy session task mirror. With it disabled, the existing request-bound implementation and task mirror remain available as the rollback path.

Experimental consensus mode remains hidden from the default `subagents_run` schema unless `agentruntime.consensus.enabled` is explicitly set.

Tier resolution priority: task `tier` > agent YAML `tier` > config default.

The Console Agent Runtime page keeps run history, run topology, event replay, versioned recovery checkpoints, explicit Retry/Replay/Resume actions, checkpoint safety decisions, cost/token flow, file attention, git diff attribution, and subagent profile management in one operational surface. See [docs/console.md](docs/console.md), [checkpoint recovery](docs/agent-harness/checkpoint-recovery.md), and [docs/tutorials/22-agentruntime.md](docs/tutorials/22-agentruntime.md) for details.

### 3-Tier Model Routing

Route workloads to different models for cost and quality optimization:

| Tier | Purpose | Example |
|------|---------|---------|
| **heavy** | Planning, complex reasoning, architecture | claude-opus-4-6, gpt-5.4 |
| **standard** | General chat, agent loops, tool calling | claude-sonnet-4-6, gpt-5.4 |
| **light** | Summarization, classification, pulse, reflection | claude-haiku-4-5, gpt-4o-mini |

```yaml
# tars.config.yaml
llm:
  providers:
    default:
      kind: anthropic
      auth_mode: api-key
      api_key: ${ANTHROPIC_API_KEY}
  tiers:
    heavy:
      provider: default
      model: claude-opus-4-6
    standard:
      provider: default
      model: claude-sonnet-4-6
    light:
      provider: default
      model: claude-haiku-4-5
  default_tier: standard
  role_defaults:
    pulse_decider: light
    agentruntime_planner: heavy
```

System roles such as chat, pulse, reflection, compaction, cleanup, and agent runtime agents map to tiers. Background work defaults to `light`, Chat can recommend a tier before the first expensive turn, and runtime logs record the resolved `role`, `tier`, `provider`, `model`, and `source` for traceability. Tier bindings are edited through the onboarding wizard — the Settings Quick Start card for `llm.tiers` reports readiness and hands off to it with "Edit in wizard".

### Background Surfaces

Two isolated surfaces run independently from user chat:

- **Pulse** — 1-minute watchdog scanning cron failures, stuck runs, stalled chats, disk pressure, Telegram delivery health, and reflection status. LLM classifier picks `ignore` / `notify` / `autofix`. The Console renders recent signals as incident cards with likely cause, evidence, recommended action, safe navigation, and re-check controls, while repeated chat-attention notifications are grouped with occurrence counts instead of inflating unread rows. Autofixes are whitelisted in config, cleanup-like autofixes are opt-in, and stalled-chat continuation requires per-session auto-resume consent.
- **Reflection** — Nightly batch (default 02:00–05:00) running memory reflection (Memory Inbox candidate extraction) and stale empty-session pruning.

Both use the `light` tier by default and have no access to user-facing tools (enforced at compile time via `RegistryScope`).

### Scheduling

Native cron with session binding:

- Cron expressions and one-shot `at:` schedules
- Session-bound jobs inherit the session's tool policy, work dirs, and prompt override
- Audit logs: `artifacts/<session_id>/cronjob-log.jsonl`
- Console Cron page for global job management plus per-session Cron tabs in chat context

### Channels

Multi-channel I/O beyond the web console:

- **Telegram** — Bidirectional messaging with pairing-based access control
- **Webhooks** — Inbound HTTP triggers for external integrations
- **Assistant** — macOS popup and voice helpers that share the core `~/.tars/workspace` default unless overridden
- **Local** — Direct API calls for scripts and automation
- **Remote Access** — Tailscale Serve can publish the loopback-only console over tailnet HTTPS with a TARS-owned target, while browser sessions use admin/user password auth and remote/mobile logins stay user-scoped.

### Embodiment

TARS has a core `embodiment` subsystem for body providers. The shipped default is `enabled: false` with no providers, so existing chat/runtime behavior is unchanged and no LLM tools are registered. When enabled, known providers can post Percepts; owner audio becomes an autonomous directive turn, while ambient/stranger observations stay non-triggering by default. Cognition turns can emit structured `tars-body-action` blocks, which TARS routes back to the bound provider only when the provider declares the matching capability.

In the Console, open Settings and use the Quick Start `Embodied Bot` and `Embodied Bot providers` cards to edit the same settings without switching to raw YAML. The providers card opens a structured form with `Mac Host`, `StackChan`, and `Custom` presets, field hints, and grouped capability toggles. For MCP providers, `endpoint` must match an `mcp.servers` key from the companion server config.

```yaml
embodiment:
  enabled: true
  providers:
    - name: stackchan
      enabled: true
      transport: mcp
      endpoint: tars-stackchan
      capabilities: [vision, hearing, speech, expression, motion, led]
      session_id: sess_main
      owner_only_directive: true
      min_trigger_interval: 30s
      max_triggers_per_hour: 60
    - name: host
      enabled: true
      transport: mcp
      endpoint: tars-stackchan-host
      capabilities: [hearing, speech]
      session_id: sess_main
      owner_only_directive: false
      trigger_observations: true
      min_trigger_interval: 30s
      max_triggers_per_hour: 60
```

Providers may also use the existing webhook inbox path `/v1/channels/webhook/inbound/{provider}` or the direct percept path `/v1/embodiment/percept/{provider}`. TARS persists the Percept in the channel inbox first, then routes self-sensory Percepts through the embodiment gate and agent runtime. Successful percept intake also publishes an ephemeral live Console companion signal so camera and microphone providers can make the pet react without creating desktop notifications. Body actions map to provider capabilities (`speak` → `speech`, `express` → `expression`, `move` → `motion`, `led` → `led`); unsupported capabilities are dropped gracefully instead of failing the cognition loop. MCP providers receive actions through their existing MCP server, so this does not add any built-in LLM tool surface.

### Remote Access

TARS can expose the local console to your phone or another trusted device through Tailscale without binding the server to `0.0.0.0`.

```bash
# Create or rotate console passwords
tars auth init
tars auth passwd admin
tars auth passwd user

# Publish the loopback console through Tailscale Serve
tars remote status
tars remote enable
tars remote url
```

Remote Access requires `api_auth_mode: required`, configured admin/user passwords, and a logged-in Tailscale client. The Settings page shows both the saved YAML value and the effective runtime value when environment variables override config, so a local dev launch such as `TARS_API_AUTH_MODE=off` is visible before enabling remote access.

### Extensibility

TARS favors **on-demand extension** over always-resident tool registrations. Domain-specific capabilities are shipped as skills (plus optional companion CLIs) from the [Skill Hub](https://github.com/devlikebear/tars-skills) rather than compiled into the TARS binary — this keeps the chat system prompt small no matter how many capabilities a user installs.

- **[Skill Hub](https://github.com/devlikebear/tars-skills)** — Public registry of skills, plugins, MCP servers, and domain packs. Install individual packages with `tars skill install <name>`, `tars plugin install <name>`, `tars mcp install <name>`, or install a reviewed bundle with `tars pack install <name>`. Skill, plugin, MCP, and pack member installs/updates are first materialized in a temporary workspace and sandbox-validated before the real `workspace/skills/<name>/`, `workspace/plugins/<name>/`, `workspace/mcp-servers/<name>/`, or `skillhub.json` changes. Hub entries can publish quality metadata such as score, last update, passing tests, required tools, permissions, companion CLI presence, and install count; the Extensions console renders these trust signals before install. Update commands report updated, skipped, and failed entries so failed package refreshes are visible. The hub is the first place to look before writing a new capability, and the only place to publish one.
- **External skill hubs (federation)** — `tars skill` also federates across external MIT-compatible hubs. Pass `--from <hub>` to install from `openclaw` ([steipete/openclaw](https://github.com/steipete/openclaw)), `hermes` ([NousResearch/hermes-agent](https://github.com/NousResearch/hermes-agent)), or `anthropic` ([anthropics/skills](https://github.com/anthropics/skills)). External installs always preview first (`--dry-run` or the console hub-selector + dry-run modal showing per-file sha256, adapter warnings, and the ATTRIBUTION notice), generate an `ATTRIBUTION.md` carrying the source's license body and copyright, and refuse to materialize source-available content (Anthropic's docx/pdf/pptx/xlsx skills are blocked at the attribution layer). Both `tars skill search` (no `--from`) and the console Hub Skills list span every registered source. See `docs/tutorials/14-skill-hub.md` for the federation architecture.
- **Skills** — Markdown instruction files (YAML frontmatter + body) with optional companion scripts. A skill's frontmatter can set `recommended_tools: [bash]`, `slash: /name`, `aliases: [...]`, and `smoke_tests: [...]`; users can invoke eligible skills directly from chat via `/name` autocomplete. Companion CLIs keep their interface out of the system prompt until the skill itself is picked. The Chat Skill Inbox extracts reusable candidates and preserves provenance, evidence, and success/failure/dead-end/correction signals, but it no longer writes an active skill on approval. Candidates advance through sandbox, offline, shadow, human approval, and canary gates; only an explicit promotion atomically activates `workspace/skills/<name>`, and one action restores the previous known-good version. The Extensions console can also draft, sandbox-test, and save explicitly authored local skills before publishing to the hub, and shows sandbox pass/fail reports for hub skill installs. See `daily-briefing` in the hub for the canonical pattern.
- **Plugins** — Advanced packages that bundle skill directories and optional MCP server declarations with manifest metadata and runtime gating. Plugin-declared MCP servers are disabled by default and require `extensions.plugins.allow_mcp_servers: true` before they can launch. Built-in Go plugins and plugin HTTP routes are not an active extension surface.
- **MCP** — Configured, hub-installed, or plugin-declared local stdio and remote HTTP/SSE/WebSocket servers with bearer or OAuth auth. Use MCP for third-party integrations that cannot be expressed as a CLI the bash tool can call. The Extensions console can draft, stdio-test, save, diagnose, and repair local `workspace/mcp-servers/<name>/` MCP packages before you publish them to the hub.
- **Browser** — Playwright-based automation for web interaction (shipped as a hub plugin).
- **Per-project overrides (`.tars/`)** — Run `tars init local --cwd <project>` or drop a `.tars/settings.json` (team-shared, commit it) plus `.tars/settings.local.json` (per-user, gitignore it) under the chat session's active working directory and TARS will fold those values into the session's `tool_config` / `prompt_override` for chats run in that directory. A sibling `.tars/skills/<name>/SKILL.md` adds full project-only skills that the LLM can select automatically, while `.tars/commands/<name>.md` adds standalone slash-command prompts that only run when the user explicitly invokes `/name`. The `project_skill` built-in tool can create both forms in the active cwd, and the Session Config panel reloads, enables, disables, and persists session-local choices to `.tars/settings.local.json`; custom local tool, skill, command, and MCP lists replace inherited project allowlists so per-user narrowing is effective. The active cwd is shown as an amber chip in the chat header (or via `/cwd`); merged values render with a `shared` / `local` source badge in the Session Config panel. The whitelist is narrow on purpose — credentials, hooks, and arbitrary binary paths are blocked at load time. See `CLAUDE.md` for the full schema.

The Extensions console validates MCP draft names before requesting generated files, so incomplete drafts stay local to the form instead of sending avoidable server requests.

**When to build a hub skill vs. a core feature**: if the capability is domain-specific (one site's logs, one vendor's API, one workflow), it belongs in `tars-skills` as a skill + CLI. Builtin tools inside this repo are reserved for universal surfaces (file ops, memory, agent runtime, channels) that every session uses.

## Install

**Homebrew:**

```bash
brew tap devlikebear/tap
brew install devlikebear/tap/tars
```

**Curl:**

```bash
curl -fsSL https://raw.githubusercontent.com/devlikebear/tars/main/install.sh | sh
```

## Quick Start

```bash
# Initialize workspace and config
tars init

# Start the server (it boots in setup-only mode if no LLM is configured yet)
tars serve

# Open the web console — the onboarding wizard runs automatically when needed
tars
```

On first run, if `~/.tars/config/config.yaml` has no `llm_providers` / `llm_tiers`, the server boots in **setup-only mode**: only the wizard endpoints + `/console` are active and `tars api serving on … (setup-only mode)` is printed to stdout. Open `http://127.0.0.1:43180/console`, the SPA detects `needs_setup=true` from `/v1/healthz`, and walks you through provider setup, heavy/standard/light tier binding, optional Tailscale Remote Access, then save & restart. The wizard is also re-runnable later from the Settings page (`/console/config` → "Re-run setup wizard") if you need to swap providers, tweak tier bindings, or enable remote access later.

To skip the wizard and edit by hand, set credentials directly and validate:

```bash
export ANTHROPIC_API_KEY="your-key"   # or OPENAI_API_KEY, etc.
# Edit ~/.tars/config/config.yaml under llm.providers / llm.tiers
tars doctor --fix
tars serve --config-check              # exits 1 in setup-only mode
```

On macOS, `tars service install && tars service start` manages `tars serve` as a LaunchAgent. Once a plist exists, `tars service start`, `stop`, and `status` keep working from the plist and `launchctl` state even if the config needs repair.

When TARS is exposed behind a reverse proxy path, pass that base path in `--server-url`; CLI API calls and the console opener resolve `/v1/*` and `/console` from the same base.

For local console development, set `TARS_CONSOLE_DEV_URL=http://127.0.0.1:5173` while Vite is running. The dev proxy keeps Vite assets, HMR, and favicon requests mounted under `/console`.

## Console Pages

The console runs at `http://127.0.0.1:43180/console`. The sidebar is deliberately short: the last changes before the archive trimmed it to the pages a single operator opens daily, grouped under Work, Operate, and Setup. That narrowing is in `main` only — a `v0.35.0` build still shows the wider nav. The footer keeps server, Pulse, Reflection, and active session status visible with direct jumps to each detail page.

| Group | Page | Path | Purpose |
|-------|------|------|---------|
| — | Mission Control | `/console` | The landing screen: health, plans, runs, jobs, sessions, notifications, and recommended setup actions |
| Work | Chat | `/console/chat` | Agent chat, tool calls, files, terminal, Git, tasks, session policy, and memory context |
| Operate | Approvals | `/console/approvals` | Review cleanup plans and approved Git mutations before apply, plus the sanitized Remote Execution view |
| Operate | Logs | `/console/logs` | Tail configured runtime logs with file, level, component, line-count, refresh, and auto-refresh controls |
| Operate | Pulse | `/console/pulse` | Watchdog status, incident cards, and run-now trigger |
| Setup | Settings | `/console/config` | Quick Start onboarding gates, wizard reentry, credential entry, remote access, restart, and workspace reset |

These routes are **hidden from the nav, not removed** — components and backend packages are untouched and each still opens by URL:

| Page | Path | Purpose |
|------|------|---------|
| Lineage | `/console/sessions/graph` | Root/forked session tree with fork previews and chat navigation |
| Plans | `/console/tasks` | Active plans across sessions with progress and chat links |
| Memory | `/console/memory` | Review memory candidates, edit stored knowledge, and test recall paths |
| System Prompt | `/console/sysprompt` | Edit USER.md, IDENTITY.md, AGENTS.md, and TOOLS.md |
| Extensions | `/console/extensions` | Skills, plugins, MCP packages, hub installs, diagnostics, and local drafts |
| Agent Runtime | `/console/agentruntime` | Run history, topology views, replay, restart, costs, file attention, and subagent profiles |
| Channels | `/console/channels` | Telegram pairing requests: approve, revoke, and review the DM policy |
| Cron | `/console/cron` | Manage global scheduled jobs with delivery targets, pause/resume, run-now, delete, and run history |
| Analytics | `/console/analytics` | Visualize usage totals, daily token bars, model cost rows, and tool or skill call counts |
| Reflection | `/console/reflection` | Nightly batch status and run-now trigger |

The narrowing is recorded as normative policy in [`frontend/console/DESIGN.md`](frontend/console/DESIGN.md) (Console Purpose & Surface Policy). Detailed console behavior, panel inventory, localization, usage budget chips, and frontend API type policy live in [docs/console.md](docs/console.md) and [docs/frontend-api-types.md](docs/frontend-api-types.md).

### Screenshots

Captured from a running `tars serve` built from `main` at the archive point — one per sidebar page, in the order the sidebar lists them. The workspace is a throwaway one, so the counters are small. (The sidebar footer reads `v0.35.0` because `VERSION.txt` was never bumped past the last release.)

**Chat** — `/console/chat`

![TARS console Chat page: session list on the left, a two-turn conversation in the middle, dockable panel tabs across the top](docs/screenshots/console-chat.webp)

Panel tabs across the top dock Sessions, Files, Git, Tasks, Health and the rest beside the transcript; the header carries session health and the active working directory.

**Approvals** — `/console/approvals`

![TARS console Approvals page showing the review queue, what triggers an approval, and the Remote Execution panel](docs/screenshots/console-approvals.webp)

The review queue, with what puts something in it and what each decision does. Remote execution sits on the same page, reporting disabled because it is off by default.

**Logs** — `/console/logs`

![TARS console Logs page tailing the runtime log filtered to INFO level](docs/screenshots/console-logs.webp)

File, level, component, and line count are all filters; any line expands to the raw JSON record behind it.

**Pulse** — `/console/pulse`

![TARS console Pulse page listing watch targets, the ignore/notify/autofix actions, and current watchdog status](docs/screenshots/console-pulse.webp)

What the watchdog watches and what it is allowed to do about it. The counters are live: this run classified a disk-pressure signal as `notify` rather than `autofix`.

**Settings** — `/console/config`

![TARS console Settings page showing the Quick Start readiness cards, 9 of 10 ready](docs/screenshots/console-settings.webp)

Quick Start, and only Quick Start. Each card is one gate between you and a working install, with a readiness badge and a note when the change needs a restart.

## Requirements

- Go 1.25.6+ (for building from source)
- LLM provider credentials (Anthropic, OpenAI, Gemini, or Claude Code CLI)
- Optional: Gemini API key for semantic memory embeddings
- Optional: Node.js for Playwright browser automation

## Build

```bash
make build-bins
bin/tars version
```

For development with hot-reload:

```bash
make dev-console    # Vite (5173) + Go API (43180), open http://127.0.0.1:43180/console
```

For pull-request preflight checks:

```bash
make ci-static-analysis-check
make lint-diff
make test-cover-diff
cd frontend/console && npm run check && npm run test:ci
```

## Documentation

- [Project homepage](https://tars.marvin-42.com)
- [Getting Started](GETTING_STARTED.md)
- [Console Guide](docs/console.md)
- [Plugin and MCP Packaging Guide](docs/plugins.md)
- [Contributing](CONTRIBUTING.md)
- [Changelog](CHANGELOG.md)

## Status

**Archived.** `v0.35.0` is the last tagged release and `main` ends a few unreleased changes later; the repository is read-only and there will be no further work on it. It never reached 1.0.0, so treat the API surface as the pre-1.0 one it was — the [stability policy](docs/public-agent-packages.md#stability-policy) describes what `pkg/` promised, and nothing beyond that was settled.

Module path: `github.com/devlikebear/tars`. MIT licensed — fork it if you want to carry it further.
