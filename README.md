# TARS

TARS is a local-first automation runtime written in Go.

It combines a terminal client, a local HTTP runtime, agent tools, sessions, scheduling, and optional browser automation in a single repository.

## Status

- Pre-`1.0.0`
- Primary module path: `github.com/devlikebear/tars`
- Version source: [`VERSION.txt`](VERSION.txt)
- Release notes: [`CHANGELOG.md`](CHANGELOG.md)

## Core Capabilities

- Terminal client with a Bubble Tea TUI
- Local HTTP API via `tars serve`
- Project manager workflow with a project board, activity feed, dispatch API, and GitHub Flow status dashboard
- Session lifecycle and transcript storage
- Agent loop with built-in file, process, scheduling, memory, and ops tools
- Semantic memory recall for durable memories, compaction summaries, and active project documents
- Runtime extension loading for skills, plugins, and MCP servers
- Playwright-based browser automation
- Optional macOS assistant workflow

## Requirements

- Go `1.25.6` or newer
- Provider credentials for API-backed models, or a local Claude Code CLI install for `llm_provider: claude-code-cli`
- Optional: a Gemini API key if you enable semantic memory embeddings
- Optional: Node.js for Playwright browser installation

## Install

Homebrew tap:

```bash
brew tap devlikebear/tap
brew install devlikebear/tap/tars
```

Curl installer:

```bash
curl -fsSL https://raw.githubusercontent.com/devlikebear/tars/main/install.sh | sh
```

The installer downloads the latest published GitHub Release by default.

Published installs also include bundled `share/tars/{skills,plugins}` assets so starter workspaces can copy built-in project plugins locally.

Install to a custom path or pin a version:

```bash
curl -fsSL https://raw.githubusercontent.com/devlikebear/tars/main/install.sh | INSTALL_DIR="$HOME/.local/bin" VERSION=0.2.0 sh
```

## Quick Start

1. Initialize a starter workspace and config:

```bash
tars init
```

`tars init` creates the starter config, enables the local gateway path used by project workflows, and copies bundled workspace plugins, including `project-swarm`, into `workspace/plugins/`.

2. Choose a provider for the starter config:

```bash
export OPENAI_API_KEY="your-api-key"
```

Or switch the starter config to Claude Code CLI:

```yaml
llm_provider: claude-code-cli
llm_auth_mode: cli
llm_model: sonnet
```

If you want semantic memory recall, add Gemini embeddings to the starter config:

```yaml
memory_semantic_enabled: true
memory_embed_provider: gemini
memory_embed_api_key: ${GEMINI_API_KEY}
memory_embed_model: gemini-embedding-2-preview
memory_embed_dimensions: 768
```

3. Check or repair the local starter setup:

```bash
tars doctor
tars doctor --fix
```

`--fix` only creates missing local files and directories. API-backed providers still need credentials, while `claude-code-cli` requires a local `claude` install instead.
It also restores missing bundled workspace plugins when the installed assets are available, and `tars doctor` warns if `gateway_enabled=false` would block the bundled project workflow.

4. Install and start the macOS background service:

```bash
tars service install
tars service start
tars service status
```

5. Start the local server manually if you do not want a background service:

```bash
tars serve --config ./workspace/config/tars.config.yaml
```

Open a project dashboard in the browser:

```bash
open http://127.0.0.1:43180/ui/projects/<project-id>
```

Open the workspace-wide dashboard index:

```bash
open http://127.0.0.1:43180/dashboards
```

The dashboard renders the current board, recent activity, autopilot status, worker reports, blocker/decision/replan PM notes, and GitHub Flow task metadata. The `/dashboards` index lists every project and links to each detail page. Live updates stream from:

```bash
curl -N http://127.0.0.1:43180/ui/projects/<project-id>/stream
```

If you want browser dashboards to stay open without bearer tokens during trusted local development, set this in `workspace/config/tars.config.yaml` while leaving API auth enabled:

```yaml
dashboard_auth_mode: off
```

6. Start the client:

```bash
tars
```

Kick off a project from chat:

```text
todo 앱 만드는 프로젝트 시작해줘
```

The bundled `project-start` skill will collect a few brief answers, finalize the project, seed the board, and start background autopilot execution.

When a project kickoff starts without an explicit `session_id`, TARS now creates a fresh chat session so project brief collection and kickoff memory do not leak into the current main session. Project boards also normalize to the canonical runtime statuses `todo`, `in_progress`, `review`, and `done`.

If a chat request carries an explicit stale `session_id`, the server now creates a fresh chat session instead of silently attaching that request to the current main session.

Once a project exists, you can operate the workflow directly from the TUI without dropping to raw HTTP:

```text
/project board <project-id>
/project activity <project-id> 20
/project dispatch <project-id> todo
/project autopilot start <project-id>
/project autopilot status <project-id>
```

7. Run basic checks:

```bash
make api-status
make api-sessions
make smoke-auth
```

## Semantic Memory

When `memory_semantic_enabled: true`, TARS keeps the existing workspace files as the source of truth and builds a derived semantic index under `workspace/memory/index`.

- `memory_save` still appends to `memory/experiences.jsonl`, then also indexes the saved memory for semantic recall
- prompt assembly searches project docs, saved experiences, and compaction-derived memories with embeddings before falling back to lexical matches
- compaction summaries can produce durable memory candidates that get indexed without blocking transcript compaction if extraction fails

If semantic memory is disabled, or the embedding request fails, TARS keeps using the current lexical retrieval path.

## Project Manager

TARS now ships a bundled `project-swarm` plugin under [`plugins/project-swarm`](plugins/project-swarm). Installed builds carry it under `share/tars/plugins`, and `tars init` copies it into `workspace/plugins/project-swarm`. Its skills are mirrored into the workspace runtime and can be invoked explicitly with `/project-start` or selected automatically from natural-language kickoff messages in chat and Telegram.

For day-to-day operation from the terminal client, use:

```text
/project board <project-id>
/project activity <project-id> [limit]
/project dispatch <project-id> {todo|review}
/project autopilot {start|status} <project-id>
```

When dedicated `codex-cli` or `claude-code` gateway agents are not explicitly registered, project dispatch falls back to the runtime default gateway agent instead of failing immediately on an unknown worker alias.
The logical `worker_kind` still remains attached to the task so autopilot can continue to reason about the intended worker profile instead of persisting the fallback executor alias.
If a configured default gateway agent points to a missing local command or script, `tars doctor` reports that configuration as a failing check.

For direct API control, the project manager routes remain available:

Create a project:

```bash
curl -s http://127.0.0.1:43180/v1/projects \
  -H 'Content-Type: application/json' \
  -d '{"name":"Project PM Demo","type":"operations","objective":"Ship the MVP"}'
```

Seed or update the board:

```bash
curl -s http://127.0.0.1:43180/v1/projects/<project-id>/board \
  -X PATCH \
  -H 'Content-Type: application/json' \
  -d '{
    "tasks":[
      {
        "id":"task-1",
        "title":"Implement feature",
        "status":"todo",
        "assignee":"dev-1",
        "role":"developer",
        "review_required":true,
        "test_command":"go test ./internal/project",
        "build_command":"go test ./internal/tarsserver"
      }
    ]
  }'
```

Dispatch developer work and then reviewer work:

```bash
curl -s http://127.0.0.1:43180/v1/projects/<project-id>/dispatch \
  -X POST \
  -H 'Content-Type: application/json' \
  -d '{"stage":"todo"}'

curl -s http://127.0.0.1:43180/v1/projects/<project-id>/dispatch \
  -X POST \
  -H 'Content-Type: application/json' \
  -d '{"stage":"review"}'
```

Start or inspect background autopilot:

```bash
curl -s http://127.0.0.1:43180/v1/projects/<project-id>/autopilot -X POST
curl -s http://127.0.0.1:43180/v1/projects/<project-id>/autopilot
```

Tasks that require review will only move to `done` after the reviewer stage approves them. Developer runs must also report passing test/build results plus issue/branch/PR metadata before the task can advance.
Worker and reviewer runs report back through a structured `<task-report>` block, and the dashboard surfaces those reports in a dedicated worker report section alongside the autopilot status.
When autopilot starts from an empty board, the PM supervisor now seeds a minimal MVP backlog and records blocker, decision, and replan notes for the dashboard instead of immediately treating the project as complete.
Autopilot now runs as a persistent PM loop with a one-minute supervision interval until the project reaches `done`.
If the TARS server restarts, incomplete projects automatically get a fresh autopilot loop on startup instead of waiting for a manual restart.
If heartbeat supervision runs while an incomplete project has no live autopilot loop, heartbeat force-starts the missing PM loop as a safety net.
When a task is left in `in_progress` because verification or review blocked forward progress, the PM loop requeues that work to `todo`, records the auto-retry decision, and continues without asking the user to manually rerun the project.

An end-to-end TUI and curl example is available in [`examples/project/README.md`](examples/project/README.md).

## Build

Build the binary with version metadata from [`VERSION.txt`](VERSION.txt):

```bash
make build-bins
bin/tars version
```

Build a macOS release archive with the same version metadata used by GitHub Releases:

```bash
make release-asset RELEASE_GOOS=darwin RELEASE_GOARCH=arm64
```

## Browser Automation

Playwright runtime is the primary browser automation path.

- Install browser dependencies with `make browser-install`
- Configure browser-related settings in `workspace/config/tars.config.yaml`
- Use the runtime APIs and TUI commands for browser status, profiles, login checks, and runs

The Chrome relay extension in [`web/relay-extension/README.md`](web/relay-extension/README.md) is still available as an experimental legacy workflow for local debugging.

## Security

- Default API auth mode is token-based and role-aware
- High-risk tools are restricted by default on non-admin routes
- Run `make security-scan` before publishing or tagging a release

## Repository Layout

- `cmd/tars`: CLI entrypoint for client, server, and assistant commands
- `internal/*`: runtime packages
- `config/`: example and standalone configuration
- `workspace/`: local runtime state, ignored by Git
- `web/relay-extension/`: optional Chrome extension for the legacy relay flow

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for development workflow, release rules, and PR requirements.

## Getting Started

See [`GETTING_STARTED.md`](GETTING_STARTED.md) for a short setup and operations guide.
