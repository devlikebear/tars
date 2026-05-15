# CLAUDE.md

## Build & Development

```bash
make build                # Go binary → bin/tars (ALWAYS Makefile, never go build)
make console-build        # Svelte frontend assets (includes npm install)
make test                 # go test ./...
make lint-diff            # PR preflight: golangci-lint new issues since DIFF_BASE (+errcheck/staticcheck)
make test-diff            # PR preflight: changed Go packages + coverage check
make test-cover-diff      # PR preflight: changed-line coverage >= DIFF_COVER_MIN
make ci-static-analysis-check # PR preflight: CI static-analysis guardrails
make codeql-workflow-check # PR preflight: CodeQL code-scanning workflow guardrails
make sonarcloud-workflow-check # PR preflight: SonarCloud evaluation workflow guardrails
make test-one TEST_NAME=TestFoo PKG=./internal/tarsserver/
make test-race / make test-cover
make fmt / make vet / make lint / make tidy / make security-scan
make dev-serve            # production-like (requires console-build first)
make dev-console          # Vite (5173) + Go API (43180), auth off → http://127.0.0.1:43180/console
cd frontend/console && npm run check   # svelte-check + tsc
cd frontend/console && npm run test:ci # stable frontend CI test slice
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

**claude-code-cli provider** (Epic #857):
- Local `claude` CLI 재사용. 2026-06-15부터 `claude -p` + Agent SDK 사용량이 Anthropic 구독 플랜의 별도 월 크레딧(Pro $20 / Max5x $100 / Max20x $200)에서 차감 — `tars doctor`가 subscription 모드일 때 안내 hint를 출력
- 멀티턴 비용 절감: 응답의 `session_id`를 `session.Session.UpstreamSessionID`에 저장하고 다음 턴 `ChatOptions.ResumeSessionID`로 다시 넘겨 `--resume`로 처리, 시스템 프롬프트/이전 transcript 재과금 회피
- MCP 자동 주입: 세션-effective MCP 서버 셋을 매 호출마다 임시 `--mcp-config` 파일로 마운트 (`internal/tarsserver/claude_code_cli_mcp.go`의 `toClaudeCodeMCPServers` 변환). websocket transport는 Claude Code 미지원이라 silently drop
- Permission mode: `llm.claude_code_cli.permission_mode` (`auto`/`acceptEdits`/`plan`/`bypassPermissions`) → `--permission-mode`. 빈 값/오타는 `auto`로 graceful degrade
- **단일 사용자 전용**: Agent SDK 크레딧이 개인 계정 귀속이라 다중 사용자 서버로는 부적합. claude.ai 로그인을 외부에 *제공*하는 형태로 노출 금지(Anthropic 정책)

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

## Session-scoped overrides (`.tars/` in active cwd)

세션 채팅의 active cwd 아래 `.tars/` 디렉터리가 있으면 그 세션 한정으로 설정·스킬·커맨드를 추가/오버라이드할 수 있다. 모든 세션이 아니라 active cwd로 진입한 세션만 영향받는다.

**Active cwd 모델** (Phase 1)
- 후보 cwd = 세션 아티팩트 디렉터리 ∪ `Session.WorkDirs[]`
- active cwd = `Session.CurrentDir` (없으면 아티팩트로 fallback)
- 채팅 헤더 amber `cwd ~/path` 칩으로 표시·전환 (`/cwd`, `/cwd list`, `/cwd <path>`)
- `GET/PUT /v1/admin/sessions/{id}/cwd` — 후보 외 경로는 400

**디렉터리 구조** (`<active_cwd>/.tars/`)
```
.tars/
  settings.json          # 팀 공유 (커밋 권장)
  settings.local.json    # 개인 (gitignore 권장)
  skills/<name>/SKILL.md # 빌트인과 머지, cwd 우선
  commands/<name>.md     # skill 별칭 (target_skill: <기존스킬>)
```
스캐폴딩: `tars init local --cwd <active_cwd>`가 위 구조와 `.tars/.gitignore`를 생성한다.

**머지 우선순위 (낮음 → 높음)** — `internal/sessionoverride.Merge`
```
sessions.json (세션 base)
  ← <cwd>/.tars/settings.json       [shared 배지, amber]
  ← <cwd>/.tars/settings.local.json [local 배지, grey]
```
- 슬라이스 필드(`tools_enabled`, `mcp_enabled` 등)는 기본 union+dedup
- 단, 해당 레이어가 `tools_custom`, `skills_custom`, `commands_custom`, `mcp_custom`을 `true`로 명시하면 해당 allowlist는 이전 레이어를 대체한다. allowlist를 비우면 상속 항목도 비워진다.
- 스칼라/맵 필드는 last-write-wins, `mcp_servers_extra`는 Name 키로 머지
- 결과는 `GET /v1/admin/sessions/{id}/effective-config`에서 `{effective, sources, diagnostics}`로 노출, `Service.Resolve`가 (cwd, mtime) 키로 캐시

**허용 필드** — `tool_config`, `prompt_override`, `mcp_servers_extra`, `model_tier_override`, `claude_code_cli_permission_mode`, `claude_code_cli_permission_deny`(Claude Code deny 규칙 리스트, 레이어 union = tightening-only → `--settings` 임시 파일로 마운트). 차단 필드(`llm_providers`, `api_key`, `auth*`, `hooks`, `server_command`)는 로드 시 drop + error diagnostic — 절대 자격증명/임의 바이너리 등록을 settings 파일에 허용하지 않는다.

**적용 지점**
- 채팅 시스템 프롬프트: `effectiveSessionView` 헬퍼가 `prompt_override`를 머지된 값으로 교체 (`handler_chat_context.go`, `handler_chat.go` 양쪽)
- 도구 게이팅: `filterExtensionsSnapshotForSession`에 머지된 `tool_config` 전달
- 스킬: `augmentSnapshotWithCwdSkills`가 `<cwd>/.tars/skills/`와 `<cwd>/.tars/commands/`를 chat snapshot에 추가, cwd 우선
- 콘솔 Session Config 패널의 Tools/Skills 탭: 항목별 `shared`/`local` 배지로 출처 표시

**현재 한계 (follow-up 예정)**
- Automation/Style 탭에는 배지 없음 (Tools/Skills만 1차)

## Marketing Site

별도 레포 `tars-site` → Cloudflare Pages → https://tars.marvin-42.com/
TARS 기능 변경 시 홈페이지 콘텐츠도 갱신 필요 (매 변경마다 X, 사용자 판단으로 주기적 갱신).

## CI

`.github/workflows/ci.yml`:
1. **security** — gitleaks + ripgrep secrets scan
2. **pr-diff** — pull requests run Svelte console checks, `npm run test:ci`, `make lint-diff` (with new-line `errcheck`/`staticcheck`), and `make test-cover-diff` against the PR base SHA
3. **test** — pushes to main run Node 24 → frontend console checks/test slice → Playwright → Go test + coverage threshold → Codecov

`.github/workflows/codeql.yml`:
1. **Analyze (go)** — CodeQL autobuild + analysis for Go source
2. **Analyze (javascript-typescript)** — buildless CodeQL analysis for Svelte/TypeScript/JavaScript
3. **Analyze (actions)** — buildless CodeQL analysis for GitHub Actions workflows

`.github/workflows/sonarcloud.yml`:
1. **Check SonarCloud configuration** — skip cleanly unless `SONAR_TOKEN`, `SONAR_PROJECT_KEY`, and `SONAR_ORGANIZATION` are configured
2. **Generate Go coverage** — when configured, run `make test-cover` and publish `coverage.out`
3. **Run SonarCloud scan** — non-blocking evaluation mode; do not make this a required merge gate until the baseline and quality gate policy are reviewed

See `docs/static-analysis.md` for the static-analysis layering and local workflow guards.

`release-on-version-bump.yml` — triggered by `VERSION.txt` change on main. Builds console before binary.
