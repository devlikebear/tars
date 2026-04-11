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
| `gateway` | Agent execution platform: runtime state machine, multi-threaded subagents (max 4), run persistence in `workspace/_shared/gateway/` |
| `session` | File-based chat sessions: index + transcripts in `workspace/sessions/`. Kinds: `main` (user-visible), `worker` (hidden) |
| `cron` | Tick-based scheduler (30s default). Supports `@at` (one-time) and cron expressions. Run history capped at 50/job |
| `pulse` | System-surface watchdog (1-min tick). Scans cron failures, stuck gateway runs, disk pressure, telegram delivery failures, reflection health. LLM classifies via `pulse_decide` tool → ignore / notify / autofix. See **System Surface** below. |
| `reflection` | System-surface nightly batch (sleep-window tick, default 02:00-05:00). Runs memory cleanup (experience extraction + knowledge-base compilation, formerly per-turn) and KB cleanup (remove empty sessions). State satisfies `pulse.ReflectionHealthSource`. See **System Surface** below. |
| `ops` | System health (disk/processes), cleanup planning with approval workflow. Consumed by pulse via narrow Go interfaces — no user-facing LLM tool wrappers (see **System Surface**). |
| `llm` | Provider abstraction (anthropic, openai, openai-codex, gemini, gemini-native, claude-code-cli) + 3-tier `Router`. Router binds heavy/standard/light via `config.ResolveAllLLMTiers`. See **LLM Provider Pool** below. |
| `memory` | Semantic memory: Gemini embeddings, cosine similarity search, experience/compaction indexing, JSONL entries |
| `tool` | Built-in agent tools: file ops, exec, web fetch/search, gateway, telegram, memory. `tool.Registry` now has `RegistryScope` (user/pulse/reflection) — scope-forbidden prefixes panic at Register time. |
| `serverauth` | Bearer token auth with SHA256, three token tiers (legacy/user/admin), loopback bypass |
| `config` | YAML → env var override → defaults. 60+ config fields across Runtime/API/LLM/Memory/Usage sections |
| `mcp` | Model Context Protocol client for external tool servers |
| `skill` | `.md` skill files with YAML frontmatter, loaded from disk |

**System Surface (pulse + reflection) vs User Surface (chat + agents):**
- TARS is split into two isolated tool-registry surfaces. User-facing code (chat sessions, agent runs) uses `RegistryScopeUser`; background maintenance (`pulse`, and later `reflection`) uses its own scope.
- `RegistryScopeUser` forbids tool-name prefixes `ops_`, `pulse_`, `reflection_`. Any attempt to register such a tool on a user registry panics — this is a wiring-time guarantee, not a runtime condition.
- Pulse does **not** consume user-facing tool wrappers. It reads `internal/ops`, `internal/cron`, `internal/gateway`, and the telegram delivery counter directly through narrow Go interfaces (`CronJobLister`, `GatewayRunLister`, `DiskStatProvider`, `DeliveryFailureCounter`).
- Pulse's LLM is a classifier only: it may call exactly one tool (`pulse_decide`) to return `{action, severity, title, summary, autofix_name}`. Actions (notify / autofix) execute deterministically in Go.
- Pulse policy lives in config (`pulse_enabled`, `pulse_interval`, thresholds, autofix allowlist, min severity). There is no `PULSE.md` policy file — "policy is config, mechanism is code".
- Autofix whitelist (Phase 1): `compress_old_logs`, `cleanup_stale_tmp`. New autofixes require a Go implementation in `internal/pulse/autofix/` AND an entry in `pulse_allowed_autofixes` — the intersection is what the decider can invoke.
- **Reflection** (nightly batch) is the second system-surface component. It ticks slowly (default every 5 min) but only actually runs jobs when (1) the current local time is inside `reflection_sleep_window` (default 02:00-05:00) AND (2) today's run has not already happened. Wrap-around windows (22:00-02:00) are supported; the "reflection day" is anchored to the start of the window.
- Reflection Phase 1 ships two jobs, run sequentially: `memory` (batch variant of the old per-turn `chat_memory_hook`: experience derivation from keyword rules + LLM knowledge compilation) and `kb_cleanup` (remove sessions whose transcript is empty and older than `reflection_empty_session_age`). Main sessions are never touched.
- Reflection has **no LLM tool surface** — its jobs call `llm.Client.Chat` directly for knowledge compilation without exposing tools to the model. Cross-surface leakage is still enforced at registry time for forward compatibility.
- The per-turn `chat_memory_hook` has shrunk to the minimum: daily log append + the explicit `remember …` hot path (with inline dedup). All other derivation/compilation runs once nightly.
- Pulse observes reflection health via the narrow `pulse.ReflectionHealthSource` interface (implemented by `reflection.State`). When `pulse_reflection_failure_threshold` consecutive nightly runs fail, pulse emits `SignalKindReflectionFailure` and the normal decider flow handles notification/autofix.
- Heartbeat (the previous name) has been removed entirely. Legacy `--run-once` / `--run-loop` CLI flags and `/console/heartbeat` URLs are kept only as no-op redirects for backward compat.

**LLM Provider Pool (`LLMConfig` + `internal/llm`):**
- `LLMConfig` has 4 fields: `LLMProviders` (alias → `LLMProviderSettings`: kind/auth_mode/oauth_provider/base_url/api_key/service_tier), `LLMTiers` (tier name → `LLMTierBinding`: provider alias + model + reasoning_effort + thinking_budget + optional service_tier override), `LLMDefaultTier`, and `LLMRoleDefaults` (role → tier).
- **Credentials live at the provider level, never per-tier.** One provider alias can serve multiple tiers by being referenced from multiple bindings with different models. This is the principle fix vs. the old `llm_tier_*_*` flat-field schema where each tier duplicated its credentials.
- Supported `kind` values match `llm.NewProvider`: `anthropic`, `openai`, `openai-codex`, `gemini`, `gemini-native`, `claude-code-cli`. The `config` package does NOT validate `kind` or role names against a closed list — that validation lives in `internal/llm` so `config` stays import-free of `llm`.
- `config.ResolveLLMTier(cfg, tier)` merges a pool entry + tier binding into a flat `ResolvedLLMTier{Kind, AuthMode, OAuthProvider, BaseURL, APIKey, Model, ReasoningEffort, ThinkingBudget, ServiceTier, ProviderAlias}`. Single resolution path, loud errors on missing alias/tier/model/kind — no silent fallback. `buildLLMRouter` calls `ResolveAllLLMTiers` once at startup and hands each `ResolvedLLMTier` to `llm.NewProvider`.
- Parser supports nested YAML (`llm_providers: { codex: { kind: openai-codex, ... } }`) and single-JSON env override (`TARS_LLM_PROVIDERS_JSON`, `TARS_LLM_TIERS_JSON`, `TARS_LLM_ROLE_DEFAULTS_JSON`). Nested string fields get `os.ExpandEnv` at parse time so `api_key: ${ANTHROPIC_API_KEY}` resolves inside nested maps (the shared YAML loader only expands top-level scalars).
- `applyLLMPoolDefaults` fills base_url/api_key/auth_mode per `kind` (e.g. `kind: anthropic` with empty base_url gets `https://api.anthropic.com`), promotes openai-codex to oauth when api-key mode has no key, and normalizes `reasoning_effort`/`service_tier` per tier. Schema lives in `config/standalone.yaml`; local overrides in `workspace/config/tars.config.yaml` (gitignored). See `docs/plans/llm-provider-pool.md`.

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
- **Router**: vanilla `window.history.pushState()` in `lib/router.ts`. Top-level routes: chat (default), memory, sysprompt, ops, pulse, reflection, extensions, config. Legacy `/console/heartbeat` URLs redirect to `/console/pulse`.
- **System surface views**: `Pulse.svelte` and `Reflection.svelte` both poll their respective `/v1/{pulse,reflection}/status` endpoints every 30s, expose "Run … Now" buttons that bypass the normal gate, and share a matching visual language. Home dashboard surfaces both pulse and reflection health in the top-strip so stalled nightly runs are visible without navigating.
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

Two tracks depending on change size:

### Small changes (docs, config, 1-2 file fixes)

Commit directly to main after `make test` passes locally:

```bash
git add <specific-files>
git commit -m "fix: description"
git push origin main
```

CI runs automatically on push to main (security + test). If it fails, fix forward immediately.

### Feature work (3+ files, new features, refactors)

Use git worktrees and PRs for review and CI gating:

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

**PR flow**: push → `gh pr create` → CI (security + test) → `gh pr merge --squash --admin`

**Note**: Do NOT use `--delete-branch` with worktrees — use `rm -rf` + `git worktree prune` instead.

### Shared rules

**Conventional commits**: `feat:`, `fix:`, `chore:`, `refactor:`. Include `Closes #N` for issue references.

**Main branch protection**: deletion and force-push are blocked by GitHub ruleset. Direct push is allowed.

## Config

- `config/standalone.yaml` — checked-in default config. Ships a minimal anthropic provider pool + 3 tier bindings; new users get a working baseline after setting `ANTHROPIC_API_KEY`.
- `workspace/config/tars.config.yaml` — local override (gitignored). Must define at least one entry under `llm_providers` and the three `llm_tiers` bindings (heavy/standard/light) or startup errors.
- Environment variables override YAML: `TARS_API_AUTH_MODE`, `TARS_LLM_PROVIDERS_JSON`, `TARS_LLM_TIERS_JSON`, etc. Nested pool/tier maps are overridden as a single JSON blob.
- Config field mapping: `internal/config/config_input_fields.go`. LLM pool parsers: `internal/config/llm_providers_field.go`. Resolver: `internal/config/llm_resolve.go`.

## CI

Two jobs in `.github/workflows/ci.yml`:
1. **security** — gitleaks + ripgrep secrets scan
2. **test** — Node 20 setup → Playwright install → Go test with coverage → Codecov upload

Release workflow in `release-on-version-bump.yml` — triggered by `VERSION.txt` change on main. Builds console assets before Go binary.
