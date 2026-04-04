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
- Obsidian-style knowledge base: durable markdown wiki notes, graph/index metadata, built-in KB CRUD tools, and a dedicated console page
- Built-in file tools with 2,000-line read pagination, continuation hints, and safe atomic writes
- Structured session compaction with identifier-preserving summaries, a safer recent-tail preserve policy, and manual `/compact [instructions]`
- Parallel read-only chat subagents through the built-in `explorer` gateway agent
- MCP transports for local stdio servers and remote HTTP/WebSocket endpoints, with bearer or OAuth auth for remote servers
- Semantic memory recall with Gemini embeddings (optional)
- Playwright-based browser automation

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

Project-linked cron jobs also inherit the project's tool allowlist now, so background workflows can use approved shell/file tools for examples like the bundled ops-service triage loop.

For read-heavy codebase research in chat, TARS can now fan out parallel `explorer` subagents and merge back compact summaries. The runtime defaults are:

```yaml
gateway_subagents_max_threads: 4
gateway_subagents_max_depth: 1
```

Open the console: `http://127.0.0.1:43180/console`

The console now includes a dedicated Knowledge page at `/console/knowledge` for browsing, editing, and deleting compiled wiki notes plus reviewing graph relationships.

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
