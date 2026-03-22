# TARS

[![CI](https://github.com/devlikebear/tars/actions/workflows/ci.yml/badge.svg)](https://github.com/devlikebear/tars/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/devlikebear/tars/graph/badge.svg)](https://codecov.io/gh/devlikebear/tars)
[![Go](https://img.shields.io/github/go-mod/go-version/devlikebear/tars)](go.mod)
[![Release](https://img.shields.io/github/v/release/devlikebear/tars)](https://github.com/devlikebear/tars/releases)

**TARS is a local-first AI project autopilot.**

Unlike Claude Code, Aider, or Cursor — which operate at the file and conversation level — TARS manages entire projects autonomously: it seeds a Kanban board from a natural-language brief, dispatches tasks to AI worker agents, reviews their output, retries failures, and keeps going until the project is done. All in a single Go binary running on your machine.

## Key Features

### Project Autopilot

The killer feature. Describe what you want to build, and TARS handles the rest:

1. **Brief** — Collects requirements through a short interview
2. **Board** — Seeds a Kanban board with `todo` / `in_progress` / `review` / `done` stages
3. **Dispatch** — Assigns tasks to AI worker agents (Claude Code CLI, Codex, or any gateway agent)
4. **Review** — Validates test/build results and GitHub Flow metadata before promotion
5. **Retry** — Auto-recovers stalled work without asking you to intervene
6. **Dashboard** — Live project status, worker reports, and PM notes in a browser

```
tars init && tars serve
# In the TUI:
> todo 앱 만드는 프로젝트 시작해줘
```

### Agent Runtime

- Terminal client with a Bubble Tea TUI + local HTTP API (`tars serve`)
- Session lifecycle, transcript storage, and context compaction
- Agent loop with built-in file, process, scheduling, memory, and ops tools
- Built-in file tools with 2,000-line read pagination, continuation hints, and safe atomic writes
- Parallel read-only chat subagents through the built-in `explorer` gateway agent
- Semantic memory recall with Gemini embeddings (optional)
- Playwright-based browser automation

### Extensibility

- **[Skill Hub](https://github.com/devlikebear/tars-skills)** — `tars skill search`, `tars plugin install`, and `tars mcp install` from a vetted registry
- **Plugins** — Bundle MCP servers, tools, and skills into installable packages
- **Managed MCP Hub** — Install checksum-verified MCP packages hosted in `tars-skills`
- **Skills** — LLM instruction files (SKILL.md) with companion scripts

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

# 5. Launch the TUI client
tars
```

Kick off a project from chat, or use the TUI commands directly:

```text
/project board <project-id>
/project dispatch <project-id> todo
/project autopilot start <project-id>
```

For read-heavy codebase research in chat, TARS can now fan out parallel `explorer` subagents and merge back compact summaries. The runtime defaults are:

```yaml
gateway_subagents_max_threads: 4
gateway_subagents_max_depth: 1
```

Open the dashboard: `http://127.0.0.1:43180/dashboards`

Install trusted MCP packages from the hub:

```bash
tars mcp search
tars mcp install safe-time
```

Hub-managed MCP packages still respect `mcp_command_allowlist_json`. For example, a Node-based MCP package requires a config allowlist such as:

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

## Documentation

- [Getting Started](GETTING_STARTED.md)
- [Project Workflow Example](examples/project/README.md)
- [Plugin and MCP Packaging Guide](docs/plugins.md)
- [Contributing](CONTRIBUTING.md)
- [Changelog](CHANGELOG.md)

## Status

Pre-1.0.0 — Module path: `github.com/devlikebear/tars`
