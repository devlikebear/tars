# TARS

[![CI](https://github.com/devlikebear/tars/actions/workflows/ci.yml/badge.svg)](https://github.com/devlikebear/tars/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/devlikebear/tars/graph/badge.svg)](https://codecov.io/gh/devlikebear/tars)
[![Go](https://img.shields.io/github/go-mod/go-version/devlikebear/tars)](go.mod)
[![Release](https://img.shields.io/github/v/release/devlikebear/tars)](https://github.com/devlikebear/tars/releases)

**TARS is a self-hosted AI agent runtime.**

A single Go binary that runs on your machine and gives you: an interactive chat with durable memory, parallel sub-agents with model tier routing, background watchdog and nightly maintenance, scheduled jobs, and multi-channel I/O (console, Telegram, webhooks) — all configurable via YAML and extensible via skills, plugins, and MCP servers.

The name is an homage to TARS from *Interstellar*: practical, direct, and built to work beside a human operator under pressure. This project is not affiliated with the film; it borrows the name as a north star for the kind of local agent runtime it wants to be.

## Comparison

| | OpenClaw | Hermes Agent | TARS |
|---|---|---|---|
| **Language** | TypeScript | Python | Go (single binary) |
| **Sub-agents** | ACP + subagent runtimes, push-based completion, Docker sandbox | ThreadPoolExecutor (max 3), ephemeral prompt, credential override | Agent Runtime executor with per-task model tier, allowlist policy, depth control |
| **Model routing** | Per-agent model override | Per-child provider/model override, MoA (4 frontier models) | 3-tier named bundles (heavy/standard/light) with role→tier config mapping |
| **Memory** | Session transcripts | Honcho/Holographic plugin hooks | Durable Markdown memory + semantic search + experience extraction + nightly reflection |
| **Background** | None | None | Pulse watchdog (1-min) + Reflection nightly batch |
| **Scheduling** | None | None | Session-bound cron jobs with audit logs |
| **Channels** | CLI | CLI + Agent Runtime API | Console + Telegram + webhooks |
| **Context mgmt** | Per-session | ContextCompressor (50% threshold, protect-last-N) | Structured compaction with identifier preservation + light-tier LLM summary |
| **Extensibility** | Built-in tools | Toolsets (terminal, file, web, delegation) | Skills + companion CLIs first, plus gated plugins/MCP from Skill Hub |

## Key Features

### Chat + Memory

The primary interface. Browser-based console at `http://127.0.0.1:43180/console`.

- Multi-session chat with full LLM tool-calling loops and transcript snippet matches in session search
- Tool calls render as collapsible rows with live elapsed time, compact argument previews, and automatic error expansion
- `@` file and directory mentions from the session Files roots for explicit context injection
- `/` command autocomplete for built-in chat actions and explicit user-invocable skill selection
- `/config` opens advanced per-session tool and skill policy controls when a selected session needs explicit overrides
- Prior Context preview panel showing the exact memory section, source badges, and token budget for the current draft
- Chat panels share a Dock Manager: Sessions and tool panels, including the read-only Git Inspector, can move between left, right, bottom, and fullscreen zones, with drag-resized dimensions persisted in the browser
- Files workspaces launch the integrated shell in the bottom dock at the selected root or browsed subdirectory, while keeping a macOS Terminal fallback available
- Workspace file previews use workspace storage by default, while selected filesystem roots are served through the explicit filesystem files API boundary
- Durable memory: `MEMORY.md`, experiences, daily logs, semantic embeddings
- Fresh workspaces omit legacy KB Wiki scaffolding while preserving any existing `memory/wiki` files
- Editable memory assets plus Tool path and Prefetch path recall tests through the console/API
- Structured transcript compaction preserving identifiers and recent context
- System prompt customization via `USER.md`, `IDENTITY.md`, `AGENTS.md`, `TOOLS.md`

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

In Console Chat, `subagents_run` renders as a parallel progress card with running/completed/failed counts, elapsed time, compact task titles, and direct links to each Agent Runtime run once they are available.

Advanced staged-flow tools are available only when explicitly allowed for a session: `subagents_orchestrate` runs dependency-aware `parallel` / `sequential` steps, and `subagents_plan` uses the heavy-tier planner model to draft such a flow.
When staged-flow tools run from chat, TARS mirrors the generated plan and live step lifecycle into the session Tasks panel so the right rail shows pending, in-progress, completed, and cancelled subagent work.

Experimental consensus mode remains hidden from the default `subagents_run` schema unless `agentruntime.consensus.enabled` is explicitly set.

Tier resolution priority: task `tier` > agent YAML `tier` > config default.

The Console Agent Runtime page exposes `Runs | Subagents` tabs. Use `Runs` to filter execution history by status, time range, and prompt text, switch between list/tree/Gantt/Flow run views, pan and zoom an interactive Svelte Flow graph, jump back to the originating chat session, scan today/7d/plan cost totals, scrub timestamped run events with Replay, inspect each run's cost/token flow, review file attention for frequently read or edited workspace paths, and open the git diff timeline that attributes workspace file changes to the run, session, agent, and plan step that produced them. Use `Subagents` to inspect the active catalog, default/effective LLM tier, resolved provider/model, source file or command entry, tool policy, and recent run links. Workspace `AGENT.md` profiles can update their default tier, draft new subagents with an LLM-assisted builder, preview and approve LLM edits, and archive inactive workspace profiles from this detail view.

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

Each system role (chat, pulse, reflection, compaction, agent runtime agents) maps to a tier. Background surfaces default to `light`, keeping costs low. If advanced staged subagent planning is explicitly enabled for a session, `llm_role_agentruntime_planner` is exercised by `subagents_plan`; TARS logs the resolved `role`, `tier`, `provider`, `model`, and `source` for chat and agent runtime LLM calls so tier selection is traceable in runtime logs. The Console Settings page includes a typed `llm.tiers` editor for adding, renaming, editing, and removing tier bindings without hand-editing JSON.

### Background Surfaces

Two isolated surfaces run independently from user chat:

- **Pulse** — 1-minute watchdog scanning cron failures, stuck runs, disk pressure, Telegram delivery health, and reflection status. LLM classifier picks `ignore` / `notify` / `autofix`. Autofixes are whitelisted in config.
- **Reflection** — Nightly batch (default 02:00–05:00) running memory reflection (experience extraction) and stale empty-session pruning.

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

### Extensibility

TARS favors **on-demand extension** over always-resident tool registrations. Domain-specific capabilities are shipped as skills (plus optional companion CLIs) from the [Skill Hub](https://github.com/devlikebear/tars-skills) rather than compiled into the TARS binary — this keeps the chat system prompt small no matter how many capabilities a user installs.

- **[Skill Hub](https://github.com/devlikebear/tars-skills)** — Public registry of skills, plugins, and MCP servers. Install with `tars skill install <name>`, `tars plugin install <name>`, `tars mcp install <name>`. Skill installs and updates are first materialized in a temporary workspace and smoke-tested before the real `workspace/skills/<name>/` directory or `skillhub.json` changes. Update commands report updated, skipped, and failed entries so failed package refreshes are visible. The hub is the first place to look before writing a new capability, and the only place to publish one.
- **Skills** — Markdown instruction files (YAML frontmatter + body) with optional companion scripts. A skill's frontmatter can set `recommended_tools: [bash]`, `slash: /name`, `aliases: [...]`, and `smoke_tests: [...]`; users can invoke eligible skills directly from chat via `/name` autocomplete. Companion CLIs keep their interface out of the system prompt until the skill itself is picked. The Extensions console can draft, sandbox-test, and save local `workspace/skills/<name>/` skills before you publish them to the hub, and shows sandbox pass/fail reports for hub skill installs. See `daily-briefing` in the hub for the canonical pattern.
- **Plugins** — Advanced packages that bundle skill directories and optional MCP server declarations with manifest metadata and runtime gating. Plugin-declared MCP servers are disabled by default and require `extensions.plugins.allow_mcp_servers: true` before they can launch. Built-in Go plugins and plugin HTTP routes are not an active extension surface.
- **MCP** — Configured, hub-installed, or plugin-declared local stdio and remote HTTP/SSE/WebSocket servers with bearer or OAuth auth. Use MCP for third-party integrations that cannot be expressed as a CLI the bash tool can call. The Extensions console can draft, stdio-test, and save local `workspace/mcp-servers/<name>/` MCP packages before you publish them to the hub.
- **Browser** — Playwright-based automation for web interaction (shipped as a hub plugin).

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

# Set your provider credentials
export ANTHROPIC_API_KEY="your-key"
# Or: export OPENAI_API_KEY="your-key"
# Then edit ~/.tars/config/config.yaml under llm.providers / llm.tiers if needed

# Validate setup
tars doctor --fix

# Validate server bootstrap without binding the API
tars serve --config-check

# Start the server
tars serve

# Open the web console
tars
```

On macOS, `tars service install && tars service start` manages `tars serve` as a LaunchAgent. Once a plist exists, `tars service start`, `stop`, and `status` keep working from the plist and `launchctl` state even if the config needs repair.

Open `http://127.0.0.1:43180/console` and start chatting.

When TARS is exposed behind a reverse proxy path, pass that base path in `--server-url`; CLI API calls and the console opener resolve `/v1/*` and `/console` from the same base.

For local console development, set `TARS_CONSOLE_DEV_URL=http://127.0.0.1:5173` while Vite is running. The dev proxy keeps Vite assets, HMR, and favicon requests mounted under `/console`.

## Console Pages

The sidebar keeps Home on the TARS logo and groups the working pages into Work, Operate, and Setup.

The sidebar footer keeps server, Pulse, Reflection, and active session status visible with 30-second refreshes and direct jumps to each detail page.

Frontend API response types are maintained in the console source with the current contract policy documented in `docs/frontend-api-types.md`.

The console header includes an EN/KO language toggle. The selected locale is stored in browser localStorage as `tars_console_locale`; first load falls back to `navigator.language` and then English.

Set `usage.limits.daily_tokens` (or `usage_daily_token_budget`) to show a daily token budget chip in the console header. The chip uses UTC day boundaries, counts input plus output tokens, hides when set to `0`, and links to today's analytics focus once usage reaches the error threshold.

The Chat header shows the active plan goal and completed/total task count without opening the Tasks panel. The dockable Chat Contract panel keeps the active work contract explicit with goal, scope, done criteria, verification commands, expected artifacts, approval status, and attached verification evidence. The Chat Tasks panel keeps full plan progress visible, lets tests/logs/screenshots/PRs/releases be attached as task evidence, and includes a collapsible past-plan archive for the active session. The Chat Git Inspector panel detects the active session repository and shows branch, remotes, staged/unstaged files, log, branches, and read-only file diffs without leaving the console. The global Plans page lists active plans across sessions, while global archive data is available through `/v1/admin/plans/archive` for future planning views.

| Group | Page | Path | Purpose |
|-------|------|------|---------|
| Home | Mission Control | `/console` | Live overview for Pulse, Reflection, active plans, Agent Runtime runs, Cron jobs, disk pressure, active sessions, recent notifications, recommended setup actions, and release/PR shortcuts |
| Work | Chat | `/console/chat` | Interactive agent chat with tool calling, dockable Sessions and tool panels, task contract review/approval, task evidence attachments, read-only Git Inspector, `@` file/directory/subagent mentions, `/` command popover for client commands and skills, transcript-snippet session search, parallel subagent progress cards, Files workspace shell, Prior Context preview, and advanced `/config` session policy overrides |
| Work | Plans | `/console/tasks` | Review active plans across sessions with progress cards and jump directly into the owning chat session |
| Work | Memory | `/console/memory` | Edit stored knowledge assets with inline guidance, inspect fill/read metadata, and compare Tool path vs Prefetch path recall |
| Work | System Prompt | `/console/sysprompt` | Edit USER.md, IDENTITY.md, AGENTS.md, TOOLS.md with starter templates, prompt impact metadata, preview, and a technical details toggle |
| Work | Extensions | `/console/extensions` | Skills, sandbox-tested hub skill installs, local Skill Creator drafts/tests, local MCP Server Creator drafts/tests, plugins, MCP servers |
| Operate | Agent Runtime | `/console/agentruntime` | Inspect subagent run history with filters, list/tree/Gantt/Flow views, originating session links, cost summaries, replay scrubber, per-run cost/token flow, file attention, git diff timeline, and subagent tier management |
| Operate | Approvals | `/console/approvals` | Review risky cleanup plans before TARS applies them |
| Operate | Cron | `/console/cron` | Manage global scheduled jobs with delivery targets, pause/resume, run-now, delete, and run history |
| Operate | Logs | `/console/logs` | Tail configured runtime logs with file, level, component, line-count, refresh, and auto-refresh controls |
| Operate | Analytics | `/console/analytics` | Visualize usage totals, daily token bars, model cost rows, and tool or skill call counts |
| Operate | Pulse | `/console/pulse` | Watchdog status and run-now trigger |
| Operate | Reflection | `/console/reflection` | Nightly batch status and run-now trigger |
| Setup | Settings | `/console/config` | Quick Start onboarding plus structured object/array editing, typed LLM tier editing, pending-change impact previews, and field metadata badges |

Settings field metadata, preferred YAML patch paths, and compatible nested YAML parsing are kept together in the config field registry so console schema and config file updates stay aligned.

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

## Documentation

- [Getting Started](GETTING_STARTED.md)
- [Plugin and MCP Packaging Guide](docs/plugins.md)
- [Contributing](CONTRIBUTING.md)
- [Changelog](CHANGELOG.md)

## Status

Pre-1.0.0 — Module path: `github.com/devlikebear/tars`
