# TARS

[![CI](https://github.com/devlikebear/tars/actions/workflows/ci.yml/badge.svg)](https://github.com/devlikebear/tars/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/devlikebear/tars/graph/badge.svg)](https://codecov.io/gh/devlikebear/tars)
[![Go](https://img.shields.io/github/go-mod/go-version/devlikebear/tars)](go.mod)
[![Release](https://img.shields.io/github/v/release/devlikebear/tars)](https://github.com/devlikebear/tars/releases)

**TARS is a self-hosted AI agent runtime.**

A single Go binary that runs on your machine and gives you: an interactive chat with durable memory, parallel sub-agents with model tier routing, background watchdog and nightly maintenance, scheduled jobs, and multi-channel I/O (console, Telegram, webhooks) — all configurable via YAML and extensible via skills, plugins, and MCP servers.

## Comparison

| | OpenClaw | Hermes Agent | TARS |
|---|---|---|---|
| **Language** | TypeScript | Python | Go (single binary) |
| **Sub-agents** | ACP + subagent runtimes, push-based completion, Docker sandbox | ThreadPoolExecutor (max 3), ephemeral prompt, credential override | Gateway executor with per-task model tier, allowlist policy, depth control |
| **Model routing** | Per-agent model override | Per-child provider/model override, MoA (4 frontier models) | 3-tier named bundles (heavy/standard/light) with role→tier config mapping |
| **Memory** | Session transcripts | Honcho/Holographic plugin hooks | Durable KB + semantic search + experience extraction + nightly compilation |
| **Background** | None | None | Pulse watchdog (1-min) + Reflection nightly batch |
| **Scheduling** | None | None | Session-bound cron jobs with audit logs |
| **Channels** | CLI | CLI + Gateway API | Console + Telegram + webhooks |
| **Context mgmt** | Per-session | ContextCompressor (50% threshold, protect-last-N) | Structured compaction with identifier preservation + light-tier LLM summary |
| **Extensibility** | Built-in tools | Toolsets (terminal, file, web, delegation) | Skills + Plugins + MCP servers + Skill Hub registry |

## Key Features

### Chat + Memory

The primary interface. Browser-based console at `http://127.0.0.1:43180/console`.

- Multi-session chat with full LLM tool-calling loops
- Durable memory: `MEMORY.md`, experiences, daily logs, semantic embeddings
- Obsidian-style knowledge base: wiki notes with graph metadata and KB CRUD tools
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

Use `subagents_orchestrate` when later tasks depend on earlier subagent results. It executes staged `parallel` and `sequential` steps and supports placeholders such as `{{task.backend.summary}}`.

Use `subagents_plan` before `subagents_orchestrate` when the main agent needs the heavy-tier planner model to decide which tasks should run in parallel versus sequence. The planner returns a validated staged flow that can be executed directly.

Tier resolution priority: task `tier` > agent YAML `tier` > config default.

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
    gateway_planner: heavy
```

Each system role (chat, pulse, reflection, compaction, gateway agents) maps to a tier. Background surfaces default to `light`, keeping costs low. `llm_role_gateway_planner` is now exercised by `subagents_plan`, and TARS logs the resolved `role`, `tier`, `provider`, `model`, and `source` for chat and gateway LLM calls so tier selection is traceable in runtime logs.

### Background Surfaces

Two isolated surfaces run independently from user chat:

- **Pulse** — 1-minute watchdog scanning cron failures, stuck runs, disk pressure, Telegram delivery health, and reflection status. LLM classifier picks `ignore` / `notify` / `autofix`. Autofixes are whitelisted in config.
- **Reflection** — Nightly batch (default 02:00–05:00) running memory cleanup (experience extraction + knowledge-base compilation) and empty-session pruning.

Both use the `light` tier by default and have no access to user-facing tools (enforced at compile time via `RegistryScope`).

### Scheduling

Native cron with session binding:

- Cron expressions and one-shot `@at` schedules
- Session-bound jobs inherit the session's tool policy, work dirs, and prompt override
- Audit logs: `artifacts/<session_id>/cronjob-log.jsonl`
- Console Cron tab for per-session job management

### Channels

Multi-channel I/O beyond the web console:

- **Telegram** — Bidirectional messaging with pairing-based access control
- **Webhooks** — Inbound HTTP triggers for external integrations
- **Local** — Direct API calls for scripts and automation

### Extensibility

- **[Skill Hub](https://github.com/devlikebear/tars-skills)** — `tars skill search`, `tars plugin install`, `tars mcp install`
- **Plugins** — Bundle skills + MCP servers with manifest metadata and runtime gating
- **MCP** — Local stdio and remote HTTP/WebSocket servers with bearer or OAuth auth
- **Skills** — Markdown instruction files with companion scripts and platform requirements
- **Browser** — Playwright-based automation for web interaction

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

# Start the server
tars serve

# Open the web console
tars
```

Open `http://127.0.0.1:43180/console` and start chatting.

## Console Pages

| Page | Path | Purpose |
|------|------|---------|
| Chat | `/console` | Interactive agent chat with tool calling |
| Memory | `/console/memory` | Edit durable memory, test semantic search, browse KB |
| System Prompt | `/console/sysprompt` | Edit USER.md, IDENTITY.md, AGENTS.md, TOOLS.md |
| Ops | `/console/ops` | System health and cleanup operations |
| Pulse | `/console/pulse` | Watchdog status and run-now trigger |
| Reflection | `/console/reflection` | Nightly batch status and run-now trigger |
| Extensions | `/console/extensions` | Skills, plugins, MCP servers |
| Config | `/console/config` | Workspace configuration |

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
