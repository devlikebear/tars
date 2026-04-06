# TARS

[![CI](https://github.com/devlikebear/tars/actions/workflows/ci.yml/badge.svg)](https://github.com/devlikebear/tars/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/devlikebear/tars/graph/badge.svg)](https://codecov.io/gh/devlikebear/tars)
[![Go](https://img.shields.io/github/go-mod/go-version/devlikebear/tars)](go.mod)
[![Release](https://img.shields.io/github/v/release/devlikebear/tars)](https://github.com/devlikebear/tars/releases)

**TARS is a local-first AI project autopilot.**

Unlike Claude Code, Aider, or Cursor, which mostly operate at the file and conversation level, TARS manages long-running projects autonomously: it plans phases with you, executes backlog work inside each phase, coordinates tools and worker agents, and only brings you back for approvals or real blockers. All in a single Go binary running on your machine.

## Key Features

### Project Autopilot

The killer feature. Describe what you want to build, and TARS handles the rest:

1. **Plan** — Collects requirements through a short interview and turns them into a phase plan
2. **Phase Loop** — Builds a backlog, selects the next task, executes it, evaluates the result, and replans when needed
3. **Capabilities** — Combines built-in tools, skills, MCP servers, web research, and worker agents in one runtime
4. **Human-in-the-Loop** — Escalates at phase approvals and real blockers instead of asking for every routine retry
5. **Dashboard** — Live phase status, run status, pending decisions, blockers, and worker reports in a browser

```bash
tars init && tars serve
tars
```

Open the web console, then start a chat such as `todo 앱 만드는 프로젝트 시작해줘`.

### Agent Runtime

- Browser-based operator console + local HTTP API (`tars serve`)
- Session lifecycle, transcript storage, and structured context compaction
- Agent loop with built-in file, process, scheduling, memory, and ops tools
- Dedicated system prompt tools for explicit control of user identity, TARS persona, agent rules, and tool guidance
- Unified memory console: manage `MEMORY.md`, `memory/experiences.jsonl`, daily durable memory files, semantic memory artifacts, and the knowledge base from one page
- Obsidian-style knowledge base: durable markdown wiki notes, graph/index metadata, built-in KB CRUD tools, and explicit opt-in lookup from memory search
- Built-in file tools with 2,000-line read pagination, continuation hints, and safe atomic writes
- Session-aware Files panel for artifact history and workspace browsing, with typed previews for markdown/code/images and in-panel folder management
- Structured session compaction with identifier-preserving summaries, a safer recent-tail preserve policy, and manual `/compact [instructions]`
- Parallel chat subagents with per-task model tier selection through the gateway agent system
- MCP transports for local stdio servers and remote HTTP/WebSocket endpoints, with bearer or OAuth auth for remote servers
- Semantic memory recall with Gemini embeddings (optional)
- Playwright-based browser automation

### 3-Tier Model Routing

Route different workloads to different models for cost and quality optimization:

| Tier | Purpose | Typical Model |
|------|---------|---------------|
| **heavy** | Planning, complex reasoning, architecture decisions | claude-opus-4-6, gpt-5.4 |
| **standard** | General chat, agent work, tool-calling loops | claude-sonnet-4-6, gpt-5.4 |
| **light** | Summarization, classification, memory hooks, pulse watchdog | claude-haiku-4-5, gpt-4o-mini |

Each system role (chat, pulse watchdog, reflection nightly batch, chat compaction, gateway agents) maps to a tier via config. Sub-agents can override the tier per task:

```yaml
# tars.config.yaml
llm_default_tier: standard
llm_tier_heavy_model: claude-opus-4-6
llm_tier_standard_model: claude-sonnet-4-6
llm_tier_light_model: claude-haiku-4-5-20251001
llm_role_pulse_decider: light
llm_role_gateway_planner: heavy
```

Agent YAML files support an optional `tier:` frontmatter field, and the `subagents_run` tool accepts a per-task `tier` parameter for fine-grained control.

### System Surface

TARS separates background maintenance from user-facing chat into isolated surfaces:

- **Pulse** — 1-minute watchdog that scans cron failures, stuck gateway runs, disk pressure, Telegram delivery failures, and reflection health. An LLM classifier picks `ignore`/`notify`/`autofix` per tick. Autofixes are whitelisted in config.
- **Reflection** — Nightly batch (default 02:00–05:00) that runs memory cleanup (experience extraction + knowledge-base compilation) and KB cleanup. No LLM tool surface — calls `llm.Client.Chat` directly.
- Both surfaces use the **light tier** by default, keeping background costs low while the main chat uses standard or heavy.

### Extensibility

- **[Skill Hub](https://github.com/devlikebear/tars-skills)** — `tars skill search`, `tars plugin install`, and `tars mcp install` from a vetted registry
- **Plugins** — Bundle skills and MCP servers with manifest metadata, runtime gating, and default project profiles
- **Managed MCP Hub** — Install checksum-verified MCP packages hosted in `tars-skills`
- **Skills** — LLM instruction files (SKILL.md) with companion scripts and runtime gating by plugin, binary, env, and platform requirements

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
# 1. Initialize workspace and config
tars init

# 2. Set your LLM provider
export OPENAI_API_KEY="your-api-key"
# Or use Claude Code CLI: set llm_provider: claude-code-cli in config

# 3. Validate setup
tars doctor --fix

# 4. Start the server
tars serve --config ./workspace/config/tars.config.yaml
# Or as a macOS background service:
tars service install && tars service start

# 5. Launch the web console
tars
```

`tars tui` is now a hidden, deprecated escape hatch for legacy debugging only. The supported paths are the web console and one-shot CLI commands.

Kick off a project from chat in the console, or use the CLI commands directly:

```bash
tars project activity <project-id> 20
tars project autopilot start <project-id>
tars project autopilot advance <project-id>
tars project autopilot status <project-id>
```

The recommended path is planning first, then controlled phase advancement. `advance` runs one synchronous autopilot step so you can inspect approvals, blockers, and replans explicitly, and `status` shows the current phase, run status, and next action without switching to the web console.

Cron jobs can now bind directly to a chat session with `session_id`. A bound cron run uses that session's tool and skill configuration, work directories, prompt override, and recent transcript context. In the console chat view, the right panel now includes a `Cron` tab alongside Files, Config, Context, Prompt, and Tasks: the main chat manages global cron jobs, while regular chat sessions manage only their bound session cron jobs. User-visible audit logs are appended to `artifacts/<session_id>/cronjob-log.jsonl`, while global cron jobs continue to run with system defaults and append to `artifacts/_global/cronjob-log.jsonl`.

For read-heavy codebase research in chat, TARS can fan out parallel subagents and merge back compact summaries. Agents are defined as markdown files in `workspace/agents/`:

```yaml
# workspace/agents/explorer/AGENT.md frontmatter
---
name: explorer
tier: light
tools_allow: [read_file, list_dir, glob, memory_search]
---
```

The `subagents_run` tool supports per-task tier selection:

```json
{"tasks": [
  {"prompt": "search for auth middleware", "tier": "light"},
  {"prompt": "design the refactor plan", "tier": "heavy"}
]}
```

Runtime defaults:

```yaml
gateway_subagents_max_threads: 4
gateway_subagents_max_depth: 1
```

Open the console: `http://127.0.0.1:43180/console`

The console now includes a dedicated Memory page at `/console/memory` for editing durable memory files, testing `memory_search`, and browsing or editing compiled knowledge-base notes. Legacy `/console/knowledge` links still open the same page.

The console also includes a dedicated System Prompt page at `/console/sysprompt` for editing `USER.md` (user identity and preferences), `IDENTITY.md` (TARS persona), `AGENTS.md` (agent operating rules), and `TOOLS.md` (tool guidance). These files are also exposed through explicit `workspace_sysprompt_*` and `agent_sysprompt_*` built-in tools.

Install trusted MCP packages from the hub:

```bash
tars mcp search
tars mcp install safe-time
```

Local stdio MCP servers still respect `mcp_command_allowlist_json`. For example, a Node-based MCP package requires a config allowlist such as:

```yaml
mcp_command_allowlist_json: ["node"]
```

## Requirements

- Go 1.25.6+ (for building from source)
- LLM provider credentials, or a local Claude Code CLI install
- Optional: Gemini API key for semantic memory embeddings
- Optional: Node.js for Playwright browser automation

## Build

```bash
make build-bins
bin/tars version
```

When you run TARS directly from a source checkout, build the embedded web console once before opening `/console`:

```bash
make console-install
make console-build
```

For live frontend work, run `npm run dev` inside `frontend/console` and start the Go server with `TARS_CONSOLE_DEV_URL=http://127.0.0.1:5173`.

## Documentation

- [Getting Started](GETTING_STARTED.md)
- [Project Workflow Example](examples/project/README.md)
- [Ops Service Example](examples/ops-service-demo/README.md)
- [Plugin and MCP Packaging Guide](docs/plugins.md)
- [Contributing](CONTRIBUTING.md)
- [Changelog](CHANGELOG.md)

## Status

Pre-1.0.0 — Module path: `github.com/devlikebear/tars`
