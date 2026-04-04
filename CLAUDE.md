# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Development Commands

```bash
# Build
make build                # Go binary → bin/tars (ALWAYS use Makefile, never go build to project root)
make console-build        # Build Svelte frontend assets (includes npm install)

# Test
make test                 # go test ./...
make test-one TEST_NAME=TestFoo PKG=./internal/project/  # single test
make test-race            # race detector
make test-cover           # coverage → coverage.out

# Lint / Quality
make fmt                  # go fmt
make vet                  # go vet (aliased as make lint)
make tidy                 # go mod tidy
make security-scan        # gitleaks + ripgrep secrets scan

# Dev server (production-like, requires make console-build first)
make dev-serve

# Dev server with hot-reload (no build needed, frontend auto-reloads)
make dev-console          # Vite (5173) + Go API (43180), auth disabled, open http://127.0.0.1:43180/console

# Frontend type checking
cd frontend/console && npm run check   # svelte-check + tsc
```

## Architecture

**Go CLI** (`cmd/tars`) → Cobra subcommands: `serve`, `project`, `cron`, `skill`, `assistant`, `approval`, `mcp`

**Server** (`internal/tarsserver`) — HTTP API on configurable address (default `127.0.0.1:43180`)
- Routes registered in `main_serve_api.go` → `registerAPIRoutes()`
- Auth middleware in `middleware.go` → modes: `off`, `required`, `external-required`
- Admin paths (`/v1/admin/*`) require admin token even when auth mode is not `off`
- When `api_auth_mode=off`, admin role is auto-granted for all requests
- Console served at `/console/*` — either embedded static assets or Vite dev proxy (`TARS_CONSOLE_DEV_URL`)

**Key internal packages:**

| Package | Purpose |
|---------|---------|
| `project` | Autopilot lifecycle: `AutopilotManager` → `runIteration()` loop → `Orchestrator` dispatches tasks. States: running/blocked/done/failed. Persisted as `AUTOPILOT.json`. Phase-aware onboarding (planning → executing) |
| `gateway` | Agent execution platform: runtime state machine, multi-threaded subagents (max 4), run persistence in `workspace/_shared/gateway/` |
| `session` | File-based chat sessions: index + transcripts in `workspace/sessions/`. Kinds: `main` (user-visible), `worker` (hidden) |
| `cron` | Tick-based scheduler (30s default). Supports `@at` (one-time) and cron expressions. Run history capped at 50/job |
| `ops` | System health (disk/processes), cleanup planning with approval workflow |
| `llm` | Provider abstraction: Anthropic, OpenAI-compat, Gemini |
| `memory` | Semantic memory: Gemini embeddings, cosine similarity search, experience/compaction indexing, JSONL entries |
| `tool` | Built-in agent tools: file ops, exec, web fetch/search, project, gateway, telegram, memory |
| `serverauth` | Bearer token auth with SHA256, three token tiers (legacy/user/admin), loopback bypass |
| `config` | YAML → env var override → defaults. 60+ config fields across Runtime/API/LLM/Memory/Usage sections |
| `mcp` | Model Context Protocol client for external tool servers |
| `skill` | `.md` skill files with YAML frontmatter, loaded from disk |

**Chat Memory System** (`internal/tarsserver` + `internal/memory` + `internal/prompt`):
- **Cache-first strategy**: In-process `memoryCache` (TTL 5min) checked before every semantic search
- **Proactive search**: System prompt instructs LLM to MUST call `memory_search` for prior context
- **Session transcript search**: `memory_search` tool supports `include_sessions=true` for cross-session lookup
- **Async prefetch**: Goroutine warms cache for next turn after each response (`startMemoryPrefetchForNextTurn`)
- **Prior Context section**: System prompt includes `## Prior Context` with source-tagged matches (`conversation|experience|project|daily`)
- **Continuity detection**: `shouldForceMemoryToolCall` detects 30+ patterns (EN/KR) like "그거", "지난번", "you mentioned"
- **Post-chat hooks**: Auto-extract experiences, daily log, compaction memories

**Frontend** (`frontend/console/`) — Svelte 5 SPA embedded via `go:embed`

- **Svelte 5 runes**: `$state()` for reactivity, `$props()` for component props, `Snippet` type for slots
- **Router**: vanilla `window.history.pushState()` in `lib/router.ts`, routes: home / projects / project/:id / sessions / ops
- **API client**: `lib/api.ts` — `requestJSON<T>()` wrapper, SSE streaming via `EventSource` and `fetch` + `ReadableStream`
- **Design tokens**: `app.css` — dark theme, amber accent (`#e09145`), Outfit/DM Sans/JetBrains Mono fonts
- **Badge/button classes**: `.badge-{default,accent,success,warning,error,info}`, `.btn-{primary,ghost,danger,sm}`
- **Markdown**: custom lightweight renderer in `lib/markdown.ts` (no external deps)

**SSE Event System:**
- `/v1/events/stream` — global event stream, optional `?project_id=` filter
- Events: `{ type, category, severity, title, message, timestamp }`, keepalive filtered client-side
- `/v1/events/history?limit=N` — recent events with `unread_count`
- `memory_recall` event — signals async memory prefetch results to frontend

## Git Workflow

**All work in git worktrees** — never commit directly to main.

```bash
# Create worktree
git fetch origin && git switch main && git pull --rebase
git worktree add .claude/worktrees/<branch-name> -b <branch-name> main

# Branch naming: feat/<name>, fix/<name>, chore/<name> (lowercase kebab-case)

# After PR merge, cleanup
rm -rf .claude/worktrees/<branch-name>  # --force if dirty
git worktree prune
git fetch origin && git switch main && git pull --rebase
```

**Conventional commits**: `feat:`, `fix:`, `chore:`, `refactor:`. Include `Closes #N` for issue references.

**PR flow**: push → `gh pr create` → CI (security + test) → `gh pr merge --squash --admin`

**Note**: Do NOT use `--delete-branch` with worktrees — use `rm -rf` + `git worktree prune` instead.

## Config

- `config/standalone.yaml` — checked-in default config
- `workspace/config/tars.config.yaml` — local override (gitignored)
- Environment variables override YAML: `TARS_API_AUTH_MODE`, `TARS_LLM_PROVIDER`, etc.
- Config field mapping: `internal/config/config_input_fields.go`

## CI

Two jobs in `.github/workflows/ci.yml`:
1. **security** — gitleaks + ripgrep secrets scan
2. **test** — Node 20 setup → Playwright install → Go test with coverage → Codecov upload

Release workflow in `release-on-version-bump.yml` — triggered by `VERSION.txt` change on main. Builds console assets before Go binary.
