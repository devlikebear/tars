# CLAUDE.md

## Build & Development

```bash
make build                # Go binary → bin/tars (ALWAYS Makefile, never go build)
make console-build        # Svelte frontend assets (includes npm install)
make test                 # go test ./...
make test-one TEST_NAME=TestFoo PKG=./internal/tarsserver/
make test-race / make test-cover
make fmt / make vet / make tidy / make security-scan
make dev-serve            # production-like (requires console-build first)
make dev-console          # Vite (5173) + Go API (43180), auth off → http://127.0.0.1:43180/console
cd frontend/console && npm run check   # svelte-check + tsc
```

## Architecture

**Go CLI** (`cmd/tars`) → Cobra: `serve`, `service`, `init`, `doctor`, `status`, `health`, `cron`, `approve`, `assistant`, `skill`, `plugin`, `mcp`, `version`. Root opens `/console`; `tars --message` = one-shot chat.

**Server** (`internal/tarsserver`) — `127.0.0.1:43180`
- Routes: `main_serve_api.go` → `registerAPIRoutes()`
- Auth: `middleware.go` → `off` / `required` / `external-required`. Admin paths always require admin token.
- Console at `/console/*` — embedded assets or Vite dev proxy (`TARS_CONSOLE_DEV_URL`)

**Key packages:**

| Package | Purpose |
|---------|---------|
| `agentruntime` | Agent execution: state machine, max 4 subagents, `workspace/_shared/agentruntime/` |
| `session` | File-based chat in `workspace/sessions/`. Kinds: `main` (visible), `worker` (hidden) |
| `cron` | Tick-based scheduler (30s). `@at` one-time + cron exprs. History capped 50/job |
| `pulse` | Watchdog (1-min): cron failures, stuck runs, disk, telegram, reflection → `pulse_decide` |
| `reflection` | Nightly (02:00-05:00): experience extraction + empty session cleanup |
| `ops` | System health, cleanup planning + approval workflow |
| `llm` | Provider abstraction (anthropic/openai/openai-codex/gemini/gemini-native/claude-code-cli) + 3-tier Router |
| `memory` | Semantic: Gemini embeddings, cosine similarity, JSONL entries |
| `tool` | Built-in tools: file ops, exec, web fetch/search, agent runtime, telegram, memory |
| `serverauth` | Bearer token auth, SHA256, three tiers (legacy/user/admin), loopback bypass |
| `config` | YAML → env override → defaults. 60+ fields |
| `mcp` | Model Context Protocol client |
| `skill` | `.md` skill files with YAML frontmatter |

**System Surface constraints:**
- Two isolated registries: `RegistryScopeUser` (chat/agents) vs system (pulse/reflection)
- `RegistryScopeUser` forbids `ops_`, `pulse_`, `reflection_` prefixes — panics at register time
- Pulse uses narrow Go interfaces only; LLM calls only `pulse_decide`
- Reflection has **no LLM tool surface** — deterministic Go only

**LLM Provider Pool:**
- `LLMConfig`: `LLMProviders` (alias → settings), `LLMTiers` (name → binding), `LLMDefaultTier`, `LLMRoleDefaults`
- Credentials at provider level, never per-tier
- `config.ResolveLLMTier` → flat `ResolvedLLMTier`. Loud errors on missing alias/tier/model/kind
- JSON env overrides: `TARS_LLM_PROVIDERS_JSON`, `TARS_LLM_TIERS_JSON`, `TARS_LLM_ROLE_DEFAULTS_JSON`

**Extension pattern — IMPORTANT:**
- **Do NOT add domain features as builtin Go tools or MCP tools** — every registered tool inflates system prompt on every chat turn
- **Default: skill (`.md`) + companion CLI via `bash` tool** — only loaded when skill is invoked
- New skills/plugins belong in external [`devlikebear/tars-skills`](https://github.com/devlikebear/tars-skills), NOT this repo

**Chat Memory:**
- Cache-first (5min TTL) → semantic search; system prompt instructs LLM to call `memory_search`
- `memory_search` supports `include_sessions=true`; async prefetch after each response
- Post-chat: daily log + explicit `remember …` hot path; experience derivation runs nightly

**Frontend** (`frontend/console/`) — Svelte 5 SPA embedded via `go:embed`
- Svelte 5 runes: `$state()`, `$props()`, `Snippet`
- Router: `lib/router.ts` (vanilla pushState). Routes: chat, memory, sysprompt, ops, pulse, reflection, extensions, config
- API: `lib/api.ts` — `requestJSON<T>()`, SSE via EventSource + ReadableStream
- Design tokens: `app.css` — dark theme, amber `#e09145`, Outfit/DM Sans/JetBrains Mono
- **Design source of truth**: `frontend/console/DESIGN.md` — consult before any visual change; update it in same PR if deviating

**SSE:** `/v1/events/stream` — `{type,category,severity,title,message,timestamp}`; `/v1/events/history?limit=N`

## Git Workflow

**Small changes** (1-2 files): commit directly to main after `make test`, then push.

**Feature work** (3+ files/new features/refactors): worktree + PR:
```bash
git fetch origin && git switch main && git pull --rebase
# Use EnterWorktree tool — do NOT use git worktree add + -C manually
git worktree add .claude/worktrees/<branch> -b <branch> main
# work → make test → commit → push
gh pr create → CI → gh pr merge --squash --admin
rm -rf .claude/worktrees/<branch> && git worktree prune
```
Branch naming: `feat/`, `fix/`, `chore/` (kebab-case). **Conventional commits**. `Closes #N` for issues.
Do NOT use `--delete-branch` with worktrees.

## Config

- `config/default.yaml` — checked-in defaults
- `config/tars.config.example.yaml` — annotated reference (all fields)
- `workspace/config/tars.config.yaml` — local override (gitignored). Missing/invalid → setup-only mode
- Field mapping: `internal/config/config_input_fields.go`; LLM resolvers: `internal/config/llm_resolve.go`

## Onboarding (setup-only mode)

Missing/invalid `llm_providers` or tiers → boots in setup-only mode. Detection: `config.NeedsSetup`.
Setup-only mux: only wizard routes; all other `/v1/*` returns JSON 503.

Frontend wizard (`Onboarding.svelte`): 4 steps — Provider → Tiers → Review → Restarting.
`App.svelte` force-pushes `/console/onboarding` when `needs_setup=true`.
Re-entry via `?reentry=1`: prefills form, masks api_key, adds [Save only] option.

## Marketing Site

별도 레포 `tars-site` → Cloudflare Pages → https://tars.marvin-42.com/
TARS 기능 변경 시 홈페이지 콘텐츠도 갱신 필요 (매 변경마다 X, 사용자 판단으로 주기적 갱신).

## CI

`.github/workflows/ci.yml`:
1. **security** — gitleaks + ripgrep secrets scan
2. **test** — Node 20 → Playwright → Go test + coverage → Codecov

`release-on-version-bump.yml` — triggered by `VERSION.txt` change on main. Builds console before binary.
