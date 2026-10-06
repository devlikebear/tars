# CLAUDE.md

## Build & Development

```bash
make build                # Go binary → bin/tars (ALWAYS Makefile, never go build)
make console-build        # Svelte frontend assets (includes npm install)
make test                 # go test ./...
make lint-diff            # PR preflight: golangci-lint new issues since DIFF_BASE (+errcheck/staticcheck)
make test-diff            # PR preflight: changed Go packages + coverage check
make test-cover-diff      # PR preflight: changed-line coverage >= DIFF_COVER_MIN
make arch-check           # PR preflight: core must not import app (see Layering below)
make api-check            # PR preflight: fail when the checked-in pkg/* API snapshot (docs/public-api-surface.txt) is stale — after changing exported pkg/ types/funcs, regenerate with `make api-snapshot` and commit it
make security-scan        # PR preflight: scan tracked files/history for secrets and local-path leaks (gitleaks + absolute home-dir paths + private key blocks) — no machine-local absolute paths or key-shaped strings in tests/fixtures
make ci-static-analysis-check # PR preflight: CI static-analysis guardrails
make codeql-workflow-check # PR preflight: CodeQL code-scanning workflow guardrails
make sonarcloud-workflow-check # PR preflight: SonarCloud evaluation workflow guardrails
make test-one TEST_NAME=TestFoo PKG=./internal/tarsserver/
make test-race / make test-cover
make fmt / make fmt-check / make vet / make lint / make tidy / make security-scan
make dev-serve            # production-like (requires console-build first)
make dev-console          # Vite (5173) + Go API (43180), auth off → http://127.0.0.1:43180/console
cd frontend/console && npm run check   # svelte-check + tsc
cd frontend/console && npm run test:ci # stable frontend CI test slice
make console-e2e          # Playwright: rebuilt console + tars serve + mock LLM (browser once: cd frontend/console && ./node_modules/.bin/playwright install chromium)
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
| `initiative` | Speak-first loop (1-min, off by default, shadow only): Go signals + System One text signals → pure policy → ledger |
| `ops` | System health, cleanup planning + approval workflow |
| `llm` | Provider abstraction (anthropic/openai/openai-codex/gemini/gemini-native/claude-code-cli/antigravity-cli) + 3-tier Router |
| `memory` | Semantic: Gemini embeddings, cosine similarity, JSONL entries |
| `tool` | Core tool primitives: registry, policy, file ops, exec, web fetch/search, memory |
| `apptool` | TARS app tools: agent runtime, sessions, subagents, cron, tasks, telegram, usage |
| `serverauth` | Bearer token auth, SHA256, three tiers (legacy/user/admin), loopback bypass |
| `config` | YAML → env override → defaults. 60+ fields |
| `mcp` | Model Context Protocol client |
| `jev` | `/v1/systemone` client for System One servers (hosted Jev or local Kev); shared by initiative and computer use |
| `computeruse` | GUI loop behind the `computer_use` tool: cua-driver accessibility snapshots + light LLM decisions (optional Jev) |
| `skill` | `.md` skill files with YAML frontmatter |

**Layering (enforced — `make arch-check`):**
```
cmd/  →  app layer  →  core layer  →  pkg/
```
- **core**: llm, tool, session, memory, skill, mcp, prompt, agent, config, auth, git, secrets, …
- **app**: tarsserver, apptool, agentruntime, cron, pulse, reflection, ops, workstore, workscheduler, …
- **A core package must never import an app package**, and neither may anything under `pkg/`. Both are test-enforced.
- Membership lives in `internal/architecture/layers.go`. **A new `internal/` package fails the test until it is classified** there — that is deliberate, not an obstacle to route around.
- TARS-specific tools go in `internal/apptool`; generic primitives stay in `internal/tool`. Putting an app-coupled tool in core puts its dependencies into every consumer of `pkg/agentloop`.
- Decision and re-evaluation criteria: `docs/decisions/repository-layering.md`

**System Surface constraints:**
- Two isolated registries: `RegistryScopeUser` (chat/agents) vs system (pulse/reflection)
- `RegistryScopeUser` forbids `ops_`, `pulse_`, `reflection_` prefixes — panics at register time
- Pulse uses narrow Go interfaces only; LLM calls only `pulse_decide`
- Reflection has **no LLM tool surface** — deterministic Go only

**Initiative (Epic #997):**
- 먼저 말을 거는 루프. `initiative.enabled` 기본 false, P1은 `mode: shadow`만 — 판단만 기록하고 말하지 않는다. 실제 행동은 `body_provider`가 있을 때의 무음 표정(`body_only`)뿐
- 판단 구조: Go가 정확한 신호(시각·조용한 시간, 입력 중, 콘솔 도착, 긴 세션·부재, 쿨다운·하루 한도)를 계산 → System One이 사용자 원문을 읽어 원자 신호 3개(`quiet_requested`/`user_strained`/`special_day`, 기준값 config) → 순수 함수 `initiative.Decide`가 intent 결정. P0(#998)에서 "말 걸까"를 System One에 통째로 묻는 방식은 AUC≈0.5로 실패했다 — **합친 판단을 System One에 묻지 말 것, 정형 사실(개수·시각)도 묻지 말 것**
- **사용자 원문(최근 메시지·USER.md)은 `jev.base_url`이 loopback일 때만** state에 들어간다(`jev.Client.IsLoopback`). 원격이면 메타데이터만 보내고 텍스트 신호는 꺼진다. ledger(`workspace/_shared/initiative/ledger.jsonl`)는 텍스트를 저장하지 않는다
- 로컬 백엔드 권고는 Kev-0.8B(한국어 원문 AUC 0.92, 약 3.1GB). TARS는 모델 프로세스를 띄우지 않고 `tars doctor`가 도달 여부만 점검
- 상태: `GET /v1/initiative/status`. 새 LLM 도구 없음, pulse와 분리

**Computer use (Epic #973):**
- One built-in chat tool: `computer_use(goal, app?, inputs?, max_steps?, resume?, confirm?)`. Enabled by default; `tools.computer_use.backend: llm` uses the `computer_use` role, mapped to light by default. Existing provider authentication and tracked router clients are reused. Explicit `enabled: false` disables it. `backend: jev` selects the optional System One endpoint under `jev.*`; initiative remains unchanged.
- `internal/computeruse/engine.go`: observe (cua-driver accessibility tree) -> decide -> validate/gate -> act -> observe again. LLM responses are strict JSON with only supplied op/target/input_key choices and boolean risky/done. No generated confidence numbers are used. Jev retains its op/target probability thresholds and target margin gate.
- Both paths preserve risky and secure-field confirmation (`needs_confirmation`, one-use in-memory resume token, 10-minute TTL). The LLM path also confirms Return and known send/delete/purchase/permission controls even if the model calls them safe. Caller-supplied input values are read only during driver execution; requests expose only input key names. `expose_values: false` withholds field contents and read-only text; secure values are always withheld.
- Screen text is sent to the selected backend (configured LLM provider by default; `jev.base_url` for Jev). Doctor reports the chosen backend and driver availability. Missing backend or driver returns `unavailable` with setup guidance. Driver lookup is per call: configured path -> `CUA_DRIVER_PATH` -> PATH -> known install locations.
- LLM decisions cannot execute tools: ToolChoiceNone, Claude Code empty harness tool list (`--tools ""`), strict MCP and no Chrome. antigravity-cli cannot enforce decision-only calls and is unavailable for this role. All actions run through the existing driver and chat permission gates.
- Budgets: default 25 steps, hard cap 50, 300s total; repeated stuck/no_change/look loops stop. Backend/model/input/output tokens and cost are returned; LLM calls go through the existing usage tracker. Unknown LLM pricing never uses Jev's unit price.
- cua-driver 0.32+ snapshots are session-scoped. Every call shares `tars-computer-use`; old drivers retry without the session argument, ended sessions revive with `start_session`. Read-only `tree_markdown` lines populate screen text/hash (menu bar excluded, 80 lines, 160 chars per line).
- Unit tests use FakeDriver/FakeAsker and recorded fixtures. `internal/tarsserver/computer_use*_e2e_test.go` covers both backends through a real subprocess + HTTP provider. Live LLM tool test: `TARS_COMPUTER_USE_LIVE_CONFIG=<config> CUA_DRIVER_PATH=<driver> go test -tags integration ./internal/tarsserver -run TestLiveComputerUseLight -v`. Jev live test remains `CU_APP=Calculator go test -tags integration ./internal/computeruse -run TestLive_Loop -v`. Native driver verification is macOS-only. See `docs/feature-map.md` for public entry points.

**LLM Provider Pool:**
- `LLMConfig`: `LLMProviders` (alias → settings), `LLMTiers` (name → binding), `LLMDefaultTier`, `LLMRoleDefaults`
- Credentials at provider level, never per-tier
- `config.ResolveLLMTier` → flat `ResolvedLLMTier`. Loud errors on missing alias/tier/model/kind
- JSON env overrides: `TARS_LLM_PROVIDERS_JSON`, `TARS_LLM_TIERS_JSON`, `TARS_LLM_ROLE_DEFAULTS_JSON`

**claude-code-cli provider** (Epic #857):
- Local `claude` CLI 재사용. `claude -p` / Agent SDK 사용량은 구독 플랜의 **기존 usage limit에서 차감**된다. Anthropic이 2026-05-13에 예고했던 별도 월 크레딧(Pro $20 / Max5x $100 / Max20x $200) 분리는 시행일인 2026-06-15에 보류됐고, 재개 시 사전 공지하겠다고만 밝혔다 — 실제로 시행되기 전까지 코드·문서에 크레딧 관련 안내를 넣지 말 것
- 멀티턴 비용 절감: 응답의 `session_id`를 `session.Session.UpstreamSessionID`에 저장하고 다음 턴 `ChatOptions.ResumeSessionID`로 다시 넘겨 `--resume`로 처리, 시스템 프롬프트/이전 transcript 재과금 회피. 채팅 턴은 `ChatOptions.PersistSession`으로 첫 호출부터 세션을 저장한다. 이 옵션이 없으면 `--no-session-persistence`로 실행되어 다음 턴 resume이 'No conversation found'로 실패했다. resume이 그래도 실패하면 재시도하지 않고 전체 transcript로 새 세션을 한 번 시작한다. 호출이 타임아웃·취소·에러로 끝나도 스트림이 이미 session_id를 냈고 그 세션이 저장되는 호출(resume 또는 `PersistSession`)이었다면, provider가 에러를 `llm.UpstreamSessionError`로 감싸 ID를 싣고(`llm.UpstreamSessionIDFromError`) 채팅 서버가 그걸 `UpstreamSessionID`에 저장해 다음 턴이 이어서 resume한다. agentloop도 앞선 반복이 받은 세션을 실패한 반복의 에러에 싣는다
- 실패한 호출의 사용량: result 이벤트 없이 끝난 호출(타임아웃·취소·에러)은 assistant 이벤트의 `message.usage`를 `message.id`별로 한 번씩(같은 id는 필드별 최댓값) 합산해 `llm.PartialUsageError{Usage, ByModel}`로 에러에 싣는다(is_error result가 왔으면 그 usage·`total_cost_usd`). `usage.TrackedClient`가 이를 기록하고, 보고된 비용이 없으면 `Tracker.PartialCallCost`가 upstream 모델 id(`message.model`)별로 anthropic 단가표로 추정한다(tier의 `sonnet` 같은 별칭으로는 가격을 못 매기므로). 정상 경로는 그대로 result 값을 쓴다
- 작업 디렉터리: 채팅 턴은 세션 cwd를 `ChatOptions.WorkDir`로 넘겨 CLI를 그 폴더에서 실행한다. 그래서 그 프로젝트의 CLAUDE.md·`.claude` 설정(훅 포함)·`.mcp.json`을 읽는다. 작업 공간은 `--add-dir`로 계속 접근한다. 다른 폴더에서도 resume된다(Claude Code 2.1.270에서 확인)
- 실시간 도구 카드: stream-json의 `tool_use`(시작)·`user` 이벤트 `tool_result`(완료, `is_error`)를 provider 중립 `ChatOptions.OnProviderTool`로 즉시 알린다 → agentloop이 `EventProviderTool`/`EventProviderToolResult`로 바로 내고(끝난 뒤 `ProviderExecutedTools`에서는 같은 ID를 다시 내지 않음) → 서버가 SSE `provider_tool`/`provider_tool_result`로 보내며 → 콘솔(`lib/providerToolCards.ts`)이 네이티브와 같은 카드로 그린다. `before/after_tool_call`과 분리한 건 TARS 도구 사용량에 세지 않기 위해서다. 타임아웃·에러·취소된 턴도 upstream 도구 기록은 transcript에 남고, 결과를 못 받은 것은 `(no result: …)`로 저장된다. 인자 미리보기는 `providerToolArgsPreview`(`internal/tarsserver/provider_tool_preview.go`)가 JSON 텍스트를 자르지 않고 값을 줄여 만든다 — 라벨 인자(file_path·command·pattern 등)를 앞에 두고 항상 파싱되는 JSON으로 180/500자에 맞춘다. 카드 라벨은 콘솔 `lib/cliToolLabels.ts`가 도구별로 만든다(Bash는 command · description, 파일 도구는 세션 폴더·worktree 기준 상대 경로, 모르는 도구는 일반 표시). 메인 스레드·턴 피드 재생·옆 세션 패널이 같은 함수를 쓰고, 이전에 잘려 저장된 transcript도 관대하게 읽는다
- 세션 health(콘솔 `lib/sessionHealth.ts`, 서버 아님): 다음 턴의 provider kind(고정 티어 → 마지막 턴 `llm_provider` → `default_tier`, `chatSessionStore.healthProvider`)로 판단한다. CLI provider(claude-code-cli/antigravity-cli)가 `upstream_session_id`로 resume 중이면 transcript 길이 기반 압축 경고를 내지 않고 "CLI가 컨텍스트를 관리함" 메모를 보여준다(TARS 압축은 CLI 컨텍스트를 줄이지 못함). CLI 세션은 TARS 고위험 도구 수 대신 CLI 권한 모드를 지표로 쓰고, `bypassPermissions`일 때만 경고한다
- MCP 자동 주입: 세션-effective MCP 서버 셋을 매 호출마다 임시 `--mcp-config` 파일로 마운트 (`internal/tarsserver/claude_code_cli_mcp.go`의 `toClaudeCodeMCPServers` 변환). websocket transport는 Claude Code 미지원이라 silently drop
- Permission mode: `llm.claude_code_cli.permission_mode` (`auto`/`default`/`acceptEdits`/`plan`/`dontAsk`/`bypassPermissions`) → `--permission-mode`. 빈 값/오타는 `auto`로 graceful degrade
- 세션 권한 모드 (#970): `Session.PermissionMode`(`manual`/`accept_edits`/`plan`/`auto`, 빈 값=상속)를 `GET/PUT /v1/admin/sessions/{id}/permission-mode`로 바꾼다(`internal/tarsserver/chat_permission_mode.go`). 세션 값이 있으면 `.tars` override와 config보다 우선해 `--permission-mode`(`default`/`acceptEdits`/`plan`/`auto`)가 되고, 네이티브 provider 게이트에도 적용된다: high-risk 도구를 manual은 묻고, accept_edits는 파일 편집만 통과, plan은 카드 없이 거부, auto는 통과. 네이티브의 상속 기본값은 manual이다. plan 승인은 Claude Code의 `ExitPlanMode` 권한 질문으로 오고, 허용하면 세션 모드를 저장하고 `setMode` 업데이트로 실행 중인 CLI를 바로 전환한다. 모드 변경·승인 결정은 ops automation audit(`chat_permission_mode`, `chat_tool_permission`)에 남는다
- 세션 goal과 사전 승인: `/goal [--auto|--accept-edits] <설명>`(`PUT /v1/admin/sessions/{id}/goal`의 `permission_mode`)은 goal과 함께 권한 모드를 한 번에 승인받는다. goal이 active인 동안 그 모드가 세션 모드가 되고, goal이 끝나면(달성·소진·해제·교체) 이전 모드로 되돌린다 — 그 사이 사용자가 모드를 직접 바꿨으면 건드리지 않는다(`pkg/session` `replaceGoal`). goal이 있으면 계획 게이트(plan_propose 뒤 STOP)도 통과한다. goal은 main 세션과 일반 채팅에만 걸 수 있다(worker·subagent 거절)
- 무인 실행 승인 (#970, `internal/tarsserver/chat_unattended.go`): 세션 모드가 Ask/Accept edits/Plan이면 그 세션의 cron(세션 컨텍스트)·텔레그램 턴과, 그 세션이 띄운 subagent(`agentruntime.ParentSessionFromContext`)도 같은 모드를 따른다. 물어야 할 호출은 ops 승인 `tool_permission`(`ops.CreateToolPermissionApproval`)으로 큐에 올리고 `notification`(ops/warning "Needs input")을 보낸 뒤 답을 폴링해 기다린다 — 30분 무응답은 `expired`로 닫고 호출을 건너뛰며, 턴 취소는 `withdrawn`. 무인 턴은 plan을 승인할 사람이 없으므로 `ExitPlanMode`는 거절한다. 모드가 없거나 `auto`인 세션은 이전처럼 게이트 없이 돈다(`claude-code-cli`는 `auto`). 서버 시작 시 남은 대기 질문은 `ExpirePendingToolPermissions`로 닫는다. 답은 Ops 페이지나 채팅의 needs-input 바(`ChatUnattendedApprovals.svelte`)에서 `POST /v1/ops/approvals/{id}/approve|reject`로 하며, 이미 닫힌 질문은 409
- 인라인 권한 (#996 스파이크 → #970 채팅 연결): `ChatOptions.ClaudeCodePermissionHandler`가 있으면 `-p <prompt>` 대신 `--input-format stream-json --permission-prompt-tool stdio`로 실행하고, stdio 제어 프로토콜(`pkg/llm/internal/ccproto`, Python Agent SDK 기준 포팅)로 `can_use_tool`을 핸들러에 넘긴다. 읽기 전용 명령처럼 CLI가 스스로 허용하는 호출은 핸들러에 오지 않는다. CLI는 stdin EOF에만 종료하므로 stdin은 결과가 오고 진행 중인 subagent 작업(`local_agent`/`local_workflow`)이 없을 때 닫는다 — 결과마다 닫으면 부모 결과 뒤에 오는 subagent의 권한 질문을 놓친다. 핸들러가 없으면 기존 `-p` 경로 그대로. 채팅 서버는 콘솔이 요청에 `interactive_permissions: true`를 실을 때만 핸들러를 붙인다(`internal/tarsserver/chat_permissions.go`) — 질문을 SSE `permission_request`로 보내고 `POST /v1/chat/permissions/{request_id}`(`allow_once`/`allow_session`/`deny`)를 기다린다. cron·텔레그램처럼 답할 사람이 없는 호출은 이 콘솔 핸들러가 없다 — 세션 모드가 없으면 기존 동작 그대로이고, 있으면 위의 무인 실행 승인(ops 큐)을 쓴다. "이 세션 동안 허용" 규칙은 서버가 만든다: CLI 제안은 정확한 명령 + `localSettings`(프로젝트 설정 파일에 기록)라 쓰지 않고, `destination: session`의 prefix 규칙을 보내며 복합 명령·`rm`·`sudo`에는 규칙을 제안하지 않는다. `CLAUDE_CODE_CLI_TIMEOUT`(기본 15분)은 호출 전체가 아니라 **무응답 한도**다: CLI stdout에서 줄이 올 때마다 다시 채워지고, 사람이 권한 질문에 답하길 기다리는 동안은 멈춘다(`claude_code_cli_clock.go`). 만료 에러는 `cli timed out: no output for 15m0s`. 기본값은 Bash 도구 하나가 출력 없이 돌 수 있는 최대 10분보다 길게 잡았다. "이 폴더에서 항상 허용"은 **TARS 자체 저장소**(`<workspace>/_shared/permissions/always-allow.json`, 폴더별)에만 저장되고 매 턴 `ChatOptions.ClaudeCodePermissionAllow` → `--settings`의 `permissions.allow`로 전달된다. allow 규칙은 권한을 넓히므로 저장소가 실어 올 수 있는 `.tars` 파일에서는 절대 읽지 않는다(아래 Session-scoped overrides의 deny 전용 원칙 유지). 라이브 테스트: `go test -tags integration ./pkg/llm/ -run TestClaudeCodeCLIControlLive -v` (haiku, safe-mode로 1회 약 $0.03)
- **단일 사용자 전용**: 구독 usage limit이 개인 계정 귀속이라 다중 사용자 서버로는 부적합. claude.ai 로그인을 외부에 *제공*하는 형태로 노출 금지(Anthropic 정책)

**antigravity-cli provider:**
- 로컬 `agy` CLI 재사용. `agy --output-format stream-json --print <text>`로 헤드리스 호출하고 중첩 NDJSON 이벤트(`init`/`step_update`/`result`)를 파싱한다. **최소 CLI 버전은 1.1.12** — `stream-json` 자체는 1.1.8에 나왔지만 도구 감사에 쓰는 `tool_info`와 usage의 `cache_read_tokens`가 1.1.12 추가라 그 이전에서는 턴은 성공해도 두 값이 조용히 비어 온다. 1.1.13으로 실제 왕복 검증함
- **자격증명이 TARS를 통과하지 않는다**: CLI가 시스템 키링의 자기 Google 로그인을 쓰므로 `api_key`/`base_url`이 없다. 사용자는 먼저 대화형 `agy`에서 인증해야 한다
- 시스템 프롬프트 플래그가 없어서 system 메시지를 프롬프트 앞 블록으로 접어 넣는다
- **세션 재개 지원**: stream의 `conversation_id`를 `ChatResponse.SessionID`에 싣고 다음 턴 `--conversation <id>`로 넘긴다. agy는 대화를 항상 저장하므로 실패한 호출도 받은 `conversation_id`를 `llm.UpstreamSessionError`로 돌려준다. 재개 턴에는 저장된 과거 transcript/system block을 다시 보내지 않는다
- `ReasoningEffort` → `--effort low|medium|high`, JSON schema 응답 → `--json-schema`. 모델은 `agy models`가 보여주는 slug를 tier에 명시하고 TARS는 변동 가능한 기본 모델을 하드코딩하지 않는다
- `AGY_CLI_MODE`는 `accept-edits`/`plan`만 허용한다. 오타·미인식 값은 flag를 생략해 CLI의 기본 permission policy로 떨어지고, `--dangerously-skip-permissions`로 승격하는 경로는 의도적으로 없다
- 도구는 CLI 것이며 TARS tool registry로 재실행하지 않는다. 실행 기록은 `ProviderExecutedTools`에 observation-only로 올린다. 실제 권한은 사용자의 Antigravity 설정이 결정한다
- `AGY_CLI_PATH` / `AGY_CLI_TIMEOUT` / `AGY_CLI_MODE`이 provider 환경 override다
- 테스트는 stream 파서가 순수 함수라 서브프로세스 없이 검증한다 — POSIX 셸 스텁이 필요한 claude-code-cli 테스트와 달리 Windows에서도 돌아간다
- 실제 `agy`를 띄우는 라이브 테스트는 `//go:build integration`: `go test -tags integration ./internal/llm/ -run TestAntigravityCLILive -v` (CLI 미설치 시 skip)

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
- Router: `lib/router.ts` (vanilla pushState). Routes: board (`/console`, home), home (`/console/system`, overview), chat, memory, sysprompt, ops, pulse, reflection, extensions, config
- API: `lib/api.ts` — `requestJSON<T>()`, SSE via EventSource + ReadableStream
- Chat workbench: `Chat.svelte` (route + slash commands) composes `ChatSessionHeader`, `ChatDockHost`, `ChatPanel`, `ChatStatusBar` (tier pin — saved on the session as `tier_pin` via `PATCH /v1/admin/sessions/{id}`, and a tier accepted on the first-turn card is pinned the same way (`tier_recommendation.pin`); the server uses it for every turn that sends no tier, permission mode, cwd, session cost via `/v1/usage/summary?session_id=`), and `ChatRail` (dock panel icons + ⌘K). Shared state lives in runes stores, not callback props: `lib/stores/chatSession.ts` (sessions, active session, per-session slices) and `lib/stores/chatDockStore.svelte.ts` (dock layout; each zone stacks its panels as tabs, model in `lib/dock/layout.ts`). Stores take their API by injection and are behavior-tested under Node via `tests/helpers/compileSvelteModule.ts`
- Hidden turn guidance: text the user didn't type (the companion handoff, `lib/companion.ts` `companionHandoffForAsk`) goes as the chat request's `console_context`, not in `message`. The server appends it as a `<console-context>` block after the message (`chat_console_context.go`, like `<review-notes>`), so the model and transcript keep it; the console strips it (`lib/consoleContext.ts` `userVisibleText`) from bubbles, the side panel and auto titles, so they show the user's own words
- i18n (en/ko): components read `$t.<namespace>.<key>`; strings live in `src/i18n/{en,ko,types}.ts`, and the chat workbench's panels keep theirs in `src/i18n/sections/<area>.ts` (the English object is the section's type, so Korean must match key for key). `src/lib` builders take their section's strings as a parameter instead of importing the store. New UI text goes through `$t`: `e2e/workbench-ko.spec.ts` fails on English phrases left in the Korean workbench
- Design tokens: `app.css` — "Graphite Signal": dark graphite, signal green `#3ee07f` (`--primary-rgb` for tints), IBM Plex Sans/Mono. Canvas/Mermaid colors live in `lib/themeColors.ts`
- **Design source of truth**: `frontend/console/DESIGN.md` — consult before any visual change; update it in same PR if deviating

**SSE:** `/v1/events/stream` — `{type,category,severity,title,message,timestamp}`; `/v1/events/history?limit=N`. A chat turn waiting for tool approval publishes `category: "approval"` (with `session_id`, `request_id`, `open_path`). On the chat stream (not this bus), native providers' `write_file`/`edit_file`/`apply_patch` emit `file_change` `{tool_call_id, path, op: create|modify|delete, additions, deletions, hunks?, binary?, truncated?}` as each call finishes (#1032): the tools put it on `tools.Result.FileChanges` (never in the model's text), the agent loop carries it on `Event.ToolFileChanges`, and `handler_chat_execution.go`'s after-tool hook writes it through the stream writer, so the turn feed replays it; the console attaches it to the tool card. CLI providers send none. `GET /v1/chat/activity` lists running turns and pending approvals for clients other than the request running the turn — the desktop tray (#972) or the console showing another session (`chat_activity.go`, fed by watching each turn stream's permission events). Unattended runs' tool calls waiting in the ops queue come in a separate array, `queued_approvals` (`approval_id`, `session_id`, `session_title`, `source`, `tool_name`, `preview`, `requested_at`; #1033, read from `unattendedPermissions.queuedApprovals`, empty when the ops queue is missing or unreadable): they count as needs input in the sidebar and tray but are answered only via `POST /v1/ops/approvals/{id}/approve|reject`, never `/v1/chat/permissions`. Their notification is `category: "ops"` without a `request_id`, so clients re-poll on ops events too. `GET /v1/chat/board` is the console home's session board (#971, `chat_board.go`): visible main sessions with that status (needs input also when an unattended run's tool call waits in the ops queue, `queued_approvals`), repository top level + branch (cached 15s), the latest checkpointed change, and this month's cost in one call; `/console` renders it and the old dashboard moved to `/console/system`

**Desktop shell** (`desktop/`, #972) — Wails v3 app `tars-desktop` that shows the console of a *local* server in a native window. **Separate Go module** (`desktop/go.mod`): root `go test ./...`/`make test` never reach it; use `make desktop-test` / `make desktop-build` / `make desktop-package`. Linux builds need `libgtk-4-dev libwebkitgtk-6.0-dev` (Wails defaults to GTK4; `-tags gtk3` is the old path)
- Adds only what a tab can't: tray state from `GET /v1/chat/activity` (#1014) + SSE, approval notifications whose buttons `POST /v1/chat/permissions/{id}` — and, for unattended runs' `queued_approvals` (#1033), a separate notification category whose approve/reject `POST /v1/ops/approvals/{id}/approve|reject` (not an admin path: the user token suffices), global hotkey, `tars://` links, self-update from the release's `tars-desktop_*` assets verified by `checksums.txt`. On Windows it also keeps the server current (`desktop/internal/serverupdate`): every 6h it runs `tars update --check --json`, then `tars update --yes --json` only while `/v1/chat/activity` shows no running turn and no pending/queued approval (a restart would cut them off); admin token via env, never argv
- `tars update` (`cmd/tars/update_main.go` → `internal/selfupdate`, app layer): latest release → `release.AssetArchiveName` → sha256 from `checksums.txt` → extract only `tars(.exe)` + `share/` (escaping entries refused) → rename the running exe to `.old` (Windows can rename but not overwrite a running exe; removed by the next update) → `POST /v1/admin/restart`, which re-execs the same path, then waits for `/v1/healthz` to report the new version. Refuses dev builds and Homebrew paths. Platform passed in, so Linux CI covers the Windows path
- Never runs the server in-process: macOS `tars service start`, else a detached `tars serve`. Closing the window hides it
- macOS distribution: Homebrew cask `devlikebear/tap/tars-desktop` (`depends_on formula: devlikebear/tap/tars`, rendered by `releasectl homebrew-cask` next to the formula). The release signs with Developer ID + hardened runtime and notarizes/staples (`scripts/desktop_package.sh`, `APPLE_*` secrets, same names as linetta); without the secrets it falls back to ad-hoc with a warning and Gatekeeper refuses the download. `FindTARS` also checks `/opt/homebrew/bin`, `/usr/local/bin`, `~/.local/bin` because a Finder-launched app gets launchd's PATH
- First run & upgrades: the cask's `depends_on formula` never upgrades an installed server, so `/v1/healthz` reports `version` to loopback callers and the shell warns once per server version when it is older (`server.Outdated`; no `version` = before 0.42.2). **Start server** runs `tars service start --install-if-missing` (fallback: plain `service start` on "unknown flag"): no plist → write the starter config/workspace if missing, install, start. `tars service start` and `tars doctor` warn when the plist runs another binary of another version (`cmd/tars/service_binary.go`). A first `tars serve` on loopback with no config file writes `tars init`'s skeleton (`onboarding.WriteSkeletonConfig`, auth off) — otherwise auth is `required` with no token and every wizard save is a 401
- Loopback servers only. Tokens: flag > env (`TARS_API_TOKEN`/`TARS_ADMIN_API_TOKEN`) > `<user config dir>/tars-desktop/config.json` (0600). Admin routes (recent chats, new chat in folder) need the admin token in every auth mode but `off`
- A `tars://` link or dropped folder only navigates or *proposes* (confirm dialog) — never answers an approval or sends a message
- Windows (#1031): the console window's bounds/maximised state and the open chat windows (tray **Recent chats → Open in new window**, `tars://session/<id>?window=new`; each loads `/console/chat/<id>` of the same server, max 8) live in `<user config dir>/tars-desktop/window.json` (0600) and come back on start. Saved off every screen → default place; no screen info → clamped only
- All logic lives in plain-Go `desktop/internal/*` packages with tests; `main.go`/`shell.go` only wire Wails. Details: `desktop/README.md`

**Session worktrees** (#971, `internal/sessionworktree` + `internal/tarsserver/chat_worktree.go`):
- 채팅 턴은 세션 cwd의 git 저장소에 대한 **write lease**(서버 메모리, 턴 + 15분)를 잡는다. 다른 세션이 잡고 있으면 새 턴의 세션은 자동으로 자기 worktree(`<workspace>/_shared/session-worktrees/<repo>/<session>`, 브랜치 `tars/session-<id>`, 체크아웃 HEAD에서 분기)로 옮겨 가고 SSE `worktree` 이벤트를 보낸다. 세션에 묶인 cron 실행은 lease와 상관없이 격리된다. 헤더의 ⑂ 칩으로 직접 격리하거나 끝낸다
- 끝내기: **apply** = base 대비 패치를 체크아웃 작업 트리에 `git apply`(stage/commit/stash 없음, 충돌이면 아무것도 바꾸지 않고 409), **keep** = 남은 변경을 브랜치에 커밋하고 폴더 제거, **discard** = 폴더와 브랜치 삭제. 세션 삭제와 시작 시 sweep은 keep으로 작업을 보존한다. 턴이 도는 동안에는 409
- `GET/POST/PUT /v1/admin/sessions/{id}/worktree` (`{action: isolate|apply|keep|discard}`, `{isolation: ""|"off"}`). 모든 동작은 automation audit `session_worktree`에 남는다
- `.tars/settings*.json`의 `worktree_include`(레이어 union)는 gitignore된 파일과 **폴더**(`.env`, `frontend/console/node_modules` 등)를 worktree로 **복사**한다(`internal/sessionworktree/include.go`). 링크는 쓰지 않는다: node_modules를 원본에 심볼릭 링크했다가 격리 세션의 `npm install`이 원본을 비운 사고가 있었다. 파일마다 copy-on-write 복제(macOS APFS `clonefile`, Linux Btrfs/XFS `FICLONE`)를 먼저 시도해 이 저장소의 node_modules(269MB, 1.5만 항목)가 약 2초·추가 디스크 0으로 들어오고, 복제가 안 되는 파일 시스템은 worktree당 1GiB까지만 일반 복사한다(넘으면 그 항목은 통째로 건너뜀)
  - 경로 규칙: 저장소 루트 기준 상대 경로(glob 허용)만, `..` 탈출·절대 경로·`.git`은 로드 시 warn 진단과 함께 버리고 복사 때 다시 검사한다. 경로의 어느 구성요소든 심볼릭 링크면 따라가지 않는다. 폴더 안 심볼릭 링크는 상대 경로이면서 저장소 안을 가리킬 때만 링크 그대로 다시 만들고(`.bin/*`), 절대 경로나 밖을 가리키면 빼고 개수를 알린다
  - 턴을 오래 막지 않는다: 복사는 worktree 옆 staging 폴더(`<repo>/<session>.include`)에서 만들어 끝나면 rename으로 한 번에 넣는다(반쯤 된 폴더가 보이지 않음). `Create`는 3초만 기다리고 나머지는 백그라운드로 이어가며 SSE `worktree` 이벤트의 `pending`에 싣고, 끝나면 automation audit `include_copied`/`include_skipped`로 남는다. 그 사이 세션이 같은 경로를 만들었으면 덮어쓰지 않는다. apply/keep/discard는 진행 중인 복사를 취소하고 기다린 뒤 지운다
  - 이 저장소는 `.tars/settings.json`(`.gitignore`에서 이 파일만 예외)에 `frontend/console/node_modules`를 넣어 둔다. 설정은 세션 cwd의 `.tars`에서 읽으므로 cwd가 저장소 루트인 세션에 적용된다
- 명령을 실행하는 `worktree_setup`은 차단 필드다(클론한 저장소가 명령을 실행하게 되므로)
- **폴더에서 새 채팅** (`internal/tarsserver/chat_new_session.go`): `POST /v1/admin/sessions`가 `{title, cwd, isolate}`를 받아 한 번에 만든다 — cwd(절대 경로나 `~`, 있는 폴더)를 유일한 work dir·active cwd로 두고, `isolate`면 첫 턴 전에 worktree로 옮긴다(reason `new_chat`). 폴더가 없거나 git 저장소가 아닌데 isolate면 아무것도 만들지 않고 404/400, worktree 생성이 실패하면 세션을 지운다. cwd·isolate가 없으면 기존 핸들러로 그대로 간다. 임의 폴더 지정은 `PUT …/workdirs`와 같은 권한이라 브라우저 user 세션이 아닌 **admin 역할만** 된다(403). `GET /v1/admin/session-folders`는 최근 세션이 작업한 폴더(격리 세션은 원본 폴더, 아티팩트 폴더 제외, 최대 8개)를, `?path=`는 생성과 같은 검증 결과와 `repo_root`를 준다. 콘솔은 보드의 New chat·사이드바 + New Chat 옆 ▾(`NewChatFolderMenu.svelte`)와 `/new [경로] [--isolate]`(경로 없으면 이 채팅의 폴더, 격리 세션이면 원본 체크아웃, `lib/newChat.ts`)로, 데스크톱은 `activity.NewSessionIn`과 `tars://new?cwd=…&isolate=1`로 같은 호출을 쓴다
- 삭제는 manager 루트 아래 `<repo>/<session>` 폴더만 허용한다(세션 기록이 조작돼도 다른 경로를 지우지 않음). Windows는 지원(ADR §6 (a)): 테스트가 windows-test job에서 돈다

**Background turns** (#971, `internal/tarsserver/chat_turn_feed.go`): 채팅 턴의 컨텍스트는 요청에서 분리돼 있어 콘솔이 떠나도(세션 전환·새로고침·연결 끊김) 턴이 계속 돈다. 멈추는 건 `POST /v1/chat/cancel`뿐이다. 취소는 턴이 세션 claim을 놓을 때까지(최대 30초, `chatCancelWait`) 기다렸다가 `{cancelled, ended}`로 답한다 — 취소된 턴도 끝 체크포인트·transcript를 쓰는 동안 claim을 잡고 있어서, 바로 답하면 그 답에 맞춰 보낸 다음 메시지(Stop 뒤 대기열 Resume)가 409를 받았다. 턴이 보내는 모든 SSE 이벤트는 세션별 피드(최대 20,000개, 넘치면 오래된 것부터 버리고 `turn_feed_truncated`)에 남고, `GET /v1/chat/stream?session_id=`가 처음부터 재생한 뒤 이어서 따라간다(턴이 없으면 204). `ChatPanel`은 세션을 열 때 여기에 붙어 진행 중인 답과 대기 중인 승인 카드를 다시 만든다. 떠나는 패널은 자기 fetch만 abort한다. 승인 질문은 이제 연결이 끊겨도 `withdrawn`되지 않고, 다시 붙은 콘솔이 답하거나 취소될 때까지 기다린다. 같은 피드로 도크의 **옆 세션** 패널(`SideSessionPanel.svelte`, 패널 id `side`)이 다른 세션을 나란히 보여주고 답하게 한다

**Focus mode PR 단계** (#1068 P4, `internal/focuspipeline/pr.go` + `internal/tarsserver/focus_gh.go`·`focus_pr_poll.go`): PR·CI·리뷰 상태는 서버가 세션 cwd에서 돌리는 **읽기 전용** `gh pr view --json number,url,state,mergeStateStatus,statusCheckRollup,reviews,comments,headRefOid,headRefName,mergeCommit,author`(20초 타임아웃, 프롬프트 끔, 프로세스 그룹 kill)로만 얻는다 — 모델에게 묻지 않는다. 쓰기(push·`gh pr create`·`gh pr merge --squash`)는 G3/G4 게이트 승인 뒤 에이전트 턴으로만 한다. 세션별 poller가 PR 대기·`pr_review`·머지 대기 동안 60초마다(턴이 도는 중엔 건너뜀) probe한다. **처음 찾는 PR은 열린 PR + 로컬 브랜치 일치만 인정**하고(번호 없는 `gh pr view`는 같은 브랜치 이름의 예전 merged/closed PR로 떨어진다) 그 뒤로는 번호로 고정해 `gh pr view <n>`만 돈다. 실패한 체크(이름+head 커밋)와 리뷰 코멘트(id, 본문 없는 CHANGES_REQUESTED 포함)를 중복 없이 finding 카드로 만든다. 봇 코멘트·리뷰(`[bot]` 로그인, gh가 `[bot]`을 떼고 주는 `sonarqubecloud`·`codecov` 같은 상태 알림 앱 로그인 — 목록은 `pr.go`의 `knownBotApps` 한 곳, 코드 리뷰를 하는 앱은 넣지 않는다)는 변경 요청(CHANGES_REQUESTED)이 아니면 finding이 되지 않는다(#1094): 실패한 품질 신호는 실패한 체크로 온다. 체크가 하나도 없는 probe는 체크를 본 적 없는 PR이고 head가 `PRSettle`(60초) 지난 뒤에만 green, 같은 head에서 dismiss한 실패 체크는 통과로 친다. G4의 request changes 턴 뒤에는 `pr_review`로 돌아가 새 CI 사실로 G4를 다시 연다. 수정 턴에는 PR 작성자·OWNER/MEMBER/COLLABORATOR의 코멘트만 데이터 블록으로 인용하고 나머지는 링크만. `gh`가 없거나 로그인 안 됨·타임아웃이면 `unavailable` → 단계마다 notice 카드 하나 + 수동 통과(`advance`, 턴이 도는 중엔 409); 배너는 마지막 probe(`pr_unavailable`)를 따른다. `TARS_FOCUS_GH_PATH`가 있으면 PATH의 `gh` 대신 그 바이너리를 돈다 — 콘솔 E2E는 `frontend/console/e2e/fake-gh.sh`(시나리오는 저장소의 `.git/e2e-gh`: 없음=unavailable, `open`, `merged`)를 써서 호스트의 `gh`·로그인·네트워크에 기대지 않는다(GitHub 러너에는 인증된 `gh`가 있다). 파이프라인이 끝나면 merge 단계가 있는 계획에 한해(store 락 밖에서) 세션 worktree를 정리하고 결과를 `worktree_end`에 남긴다: `MERGED` + 작업 트리 깨끗 + 로컬 HEAD가 머지된 head(또는 그 조상) + 턴 없음일 때만 discard, 아니면 keep(이유 기록). 머지된 PR의 `mergeCommit`은 `PRInfo.MergeOID`로 남고, release train(`focus_release.go`)은 이 커밋이 최신 `v*` 태그에 들어 있으면(예전 파이프라인은 태그 히스토리의 `(#N)`·`Merge pull request #N` 커밋 제목으로 판단) 그 파이프라인을 목록에서 뺀다 — 포커스 밖에서 만든 릴리스도 반영하기 위해서다

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
- 입력창 아래 상태 바의 강조색(초록) `cwd ~/path` 칩으로 표시·전환 (`/cwd`, `/cwd list`, `/cwd <path>`)
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
  ← <cwd>/.tars/settings.json       [shared 배지, 강조색]
  ← <cwd>/.tars/settings.local.json [local 배지, grey]
```
- 슬라이스 필드(`tools_enabled`, `mcp_enabled` 등)는 기본 union+dedup
- 단, 해당 레이어가 `tools_custom`, `skills_custom`, `commands_custom`, `mcp_custom`을 `true`로 명시하면 해당 allowlist는 이전 레이어를 대체한다. allowlist를 비우면 상속 항목도 비워진다.
- 스칼라/맵 필드는 last-write-wins, `mcp_servers_extra`는 Name 키로 머지
- 결과는 `GET /v1/admin/sessions/{id}/effective-config`에서 `{effective, sources, diagnostics}`로 노출, `Service.Resolve`가 (cwd, mtime) 키로 캐시

**허용 필드** — `tool_config`, `prompt_override`, `mcp_servers_extra`, `model_tier_override`, `claude_code_cli_permission_mode`, `claude_code_cli_permission_deny`(Claude Code deny 규칙 리스트, 레이어 union = tightening-only → `--settings` 임시 파일로 마운트), `worktree_include`(세션 worktree에 복사할 gitignore 파일, 레이어 union). 차단 필드(`llm_providers`, `api_key`, `auth*`, `hooks`, `server_command`, `worktree_setup`)는 로드 시 drop + error diagnostic — 절대 자격증명/임의 바이너리 등록을 settings 파일에 허용하지 않는다.

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
1. **format** — `make fmt-check` (`gofmt -l` over the tree). Fails the build on any unformatted file. `.golangci.yml` enables only `govet`/`ineffassign`/`revive`, so no linter covers formatting — this job is the only guard
2. **security** — gitleaks + ripgrep secrets scan
3. **windows-build** — `make windows-build-check`, cross-compiling the whole module for Windows on ubuntu
4. **windows-test** — `scripts/windows_test.sh` on `windows-latest`. The only job that runs tests on Windows; everything else is Linux
5. **pr-diff** — pull requests run Svelte console checks, `npm run test:ci`, `make console-e2e` (Playwright specs in `frontend/console/e2e/` against the real server and `e2e/mock-llm.mjs`; report uploaded on failure), `make lint-diff` (with new-line `errcheck`/`staticcheck`), and `make test-cover-diff` against the PR base SHA
6. **desktop** / **desktop-macos** — `make desktop-test`, then package the Linux + Windows archives (ubuntu) and the `.app` bundle (macos-14, `codesign --verify`, `--version` check)
7. **test** — pushes to main run Node 24 → frontend console checks/test slice → Playwright → Go test + coverage threshold → Codecov

`scripts/windows_test.sh` carries two lists of Windows-failing tests — packages excluded wholesale, and individual tests skipped in otherwise-green packages. **Both are debt, not policy**: shrink them rather than adding to them. Reproduce the job locally on Windows with `make windows-test`.

Note that the Linux-only test jobs cannot cover `*_windows.go` files at all, so SonarCloud's `new_coverage` gate reads low on PRs that add platform-split code.

`.github/workflows/codeql.yml`:
1. **Analyze (go)** — CodeQL autobuild + analysis for Go source
2. **Analyze (javascript-typescript)** — buildless CodeQL analysis for Svelte/TypeScript/JavaScript
3. **Analyze (actions)** — buildless CodeQL analysis for GitHub Actions workflows

`.github/workflows/sonarcloud.yml`:
1. **Check SonarCloud configuration** — skip cleanly unless `SONAR_TOKEN`, `SONAR_PROJECT_KEY`, and `SONAR_ORGANIZATION` are configured
2. **Generate Go coverage** — when configured, run `make test-cover` and publish `coverage.out`
3. **Run SonarCloud scan** — non-blocking evaluation mode; do not make this a required merge gate until the baseline and quality gate policy are reviewed

See `docs/static-analysis.md` for the static-analysis layering and local workflow guards.

`release-on-version-bump.yml` — triggered by `VERSION.txt` change on main. Builds console before binary. Server archives are darwin arm64/amd64 `.tar.gz` plus windows amd64 `.zip` (`tars.exe` + `share/`, cross-built on macos-14; `verify-windows-asset` installs it with `install.ps1` on windows-latest before publishing). `install.ps1` (repo root, Windows PowerShell 5.1+) is the Windows installer: `%LOCALAPPDATA%\Programs\TARS`, user PATH, `-Desktop`/`-StartAtLogin`, checksum-verified, replaces running executables by renaming them to `.old`. Also builds the desktop archives (`scripts/desktop_package.sh`: darwin arm64/amd64 on macos-14, linux/windows amd64 on ubuntu) and adds them to the release and `checksums.txt` — each archive must hold exactly one top-level entry for the self-updater.

## Codebase Analysis

Architecture and module analysis available at `.analysis/AI_CONTEXT.md`.
Read it first when you need to understand the project structure, dependencies, or key data flows.
