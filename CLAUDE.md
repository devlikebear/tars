# CLAUDE.md

Behavioral guidelines to reduce common LLM coding mistakes. Merge with project-specific instructions as needed.

Tradeoff: These guidelines bias toward caution over speed. For trivial tasks, use judgment.

## 1. Think Before Coding

Don't assume. Don't hide confusion. Surface tradeoffs.

Before implementing:

- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

## 2. Simplicity First

Minimum code that solves the problem. Nothing speculative.

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.
- Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

## 3. Surgical Changes

Touch only what you must. Clean up only your own mess.

When editing existing code:

- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:

- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.
- The test: Every changed line should trace directly to the user's request.

## 4. Goal-Driven Execution

Define success criteria. Loop until verified.

Transform tasks into verifiable goals:

- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:

1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]

Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.

These guidelines are working if: fewer unnecessary changes in diffs, fewer rewrites due to overcomplication, and clarifying questions come before implementation rather than after mistakes.

---

## 개발 규칙

1. MVP 중심으로 실행 가능한 단위로 개발한다.
2. TDD를 따른다. 실패하는 테스트를 먼저 작성한 뒤 구현한다.
3. 기능 단위마다 다음 사이클을 반복한다: 계획 수립 -> 개발 -> 버전업 -> 문서화 -> 기능 단위 커밋 -> 다음 개발.
4. 불필요한 문서 파일을 생성하지 않는다.
5. 오버엔지니어링 없이 최소한의 간결한 코드로 구현한다.
6. 미래에 사용할 가능성만으로 지금 쓰지 않을 코드를 미리 만들지 않는다.
7. 실제 필요가 생기기 전에는 인터페이스를 과도하게 설계하지 않는다.
8. 개발이 끝난 뒤 문서화 단계에서 이번 턴에 변경되었거나 새로 추가된 개발 규칙, 또는 코드 구조 변경 사항이 있으면 `CLAUDE.md`를 반드시 갱신하고 함께 커밋한다.
9. `~/workspace/opensources/openclaw`의 주요 기능을 개발할 때는 OpenClaw 소스를 분석해 참고한다.
10. `~/workspace/opensources/openclaw` 디렉터리에 접근할 수 없으면 `https://github.com/openclaw/openclaw`를 분석 대상으로 사용하고, 소스 분석은 Repomix MCP를 사용해 진행한다.

## 워크플로우

1. 사용자가 요구사항을 Claude Code에 전달
2. `/plan` — 요구사항을 작업지시서(Work Order)로 분해
3. `/implement` — Work Order를 `codex-implementer` 서브에이전트에 위임하여 코드 구현, Claude Code가 결과 검수
4. `/review` — 변경사항의 품질/보안/회귀 위험 점검, 문제 발견 시 수정 Work Order를 생성해 `/implement`로 재위임
5. 테스트 실행(`make test`)은 Claude Code가 직접 수행
6. 커밋 및 `CLAUDE.md` 갱신은 Claude Code가 수행

## 바이너리 역할 정의

- `tars` (단일 바이너리): `tars serve` 모드에서 서버 런타임(LLM 호출, 허트비트/크론 실행, 메모리/게이트웨이 오케스트레이션)을 수행하고, 기본 모드에서 CLI/TUI 클라이언트 UX를 제공한다.

## tars 내부 프로토콜 규칙

- `tars serve`가 HTTP API 서버를 서빙하고, `tars`(클라이언트 모드)는 해당 API를 호출하는 HTTP 클라이언트로 동작한다.
- LLM 실행, OAuth 토큰 교환/저장, heartbeat/cron 실행 같은 서버 책임 로직은 반드시 `tars serve`에서 수행한다.
- 클라이언트 모드는 사용자 입력 수집, API 요청/응답 렌더링 같은 UX만 담당한다.
- 인증 토큰(특히 OAuth access/refresh token)은 서버 런타임에서만 저장/관리하고, 클라이언트는 직접 저장하지 않는다.

## 코드 구조 변경 기록

### 현재 아키텍처 구조

**바이너리**
- `tars`: 단일 바이너리 (`tars serve` 서버 모드 + CLI/TUI 클라이언트 모드)

**주요 패키지**
- `internal/config`: 설정 로딩 (YAML/ENV 우선순위, 환경변수 확장, 경로 자동 탐지)
- `internal/llm`: LLM provider 추상화 (bifrost, openai, anthropic, gemini, gemini-native)
- `internal/session`: 세션 관리 (JSONL transcript, 토큰 기반 히스토리 로딩, 컴팩션)
- `internal/agent`: Agent Loop (훅 기반 이벤트, 도구 실행 반복, 상태 추적)
- `internal/tool`: 빌트인 도구 (file/web/memory/automation + gateway/sessions/message/browser/nodes 계열)
- `internal/gateway`: in-process gateway 런타임 (run registry, agent executor, channels, browser/nodes 상태, run/channel snapshot 영속화/복구)
- `internal/browser`: 브라우저 profile-aware 서비스 (managed/chrome, site flow login/check/run)
- `internal/browserrelay`: OpenClaw 스타일 로컬 CDP relay (`/json/version`, `/json/list`, `/extension`, `/cdp`)
- `internal/vaultclient`: Vault read-only client (token/approle, secret path allowlist)
- `internal/extensions`: 스킬/플러그인/MCP 통합 스냅샷 + 핫리로드 매니저
- `internal/skill`: SKILL.md frontmatter 파싱/우선순위 머지/available_skills 포맷
- `internal/plugin`: 선언형 매니페스트(`tarsncase.plugin.json`) 로더
- `internal/mcp`: MCP 런타임(지속 세션, 도구 목록 동기화, jsonline/content-length 전송 모드 지원)
- `internal/heartbeat`: 주기 실행 (정책 기반 스케줄, 세션 컨텍스트 연동)
- `internal/cron`: 작업 스케줄러 (interval/at 스케줄, 실행 잠금, 백오프)
- `internal/prompt`: 시스템 프롬프트 빌더 (워크스페이스 파일 조립)
- `internal/memory`: 워크스페이스 부트스트랩 (HEARTBEAT.md, MEMORY.md, daily log)

**LLM Provider**
- `bifrost`, `openai`: OpenAI-compatible API (공통 클라이언트)
- `anthropic`: Messages API (tool_use 지원)
- `gemini`: OpenAI-compatible 경로
- `gemini-native`: Gemini REST API (functionCall/Response 변환)

### Phase별 완성 현황

**Phase 0: 인프라** (완료)
- 설정 로딩 (YAML/ENV 우선순위, 환경변수 확장)
- 구조화 로깅 (zerolog, console/file 분리)
- 워크스페이스 초기화 (부트스트랩 파일 7종)
- CI/CD (Makefile, GitHub Actions)

**Phase 1: 채팅/세션** (완료)
- Chat API (SSE 스트리밍, 시스템 프롬프트 주입)
- 세션 관리 (JSONL transcript, CRUD API, 검색/내보내기)
- 컨텍스트 압축 (LLM 기반 요약, 토큰 예산 기반 유지)
- 메모리 자동화 (daily log, 장기 메모 승격)

**Phase 2: Agent Loop** (완료)
- 도구 레지스트리 (스키마 정의, 동적 등록)
- 빌트인 도구 (파일/디렉터리 읽기, 명령 실행, 경로 가드)
- 훅 시스템 (before/after_llm, before/after_tool_call)
- Status 스트림 (SSE 이벤트, 도구 호출 가시성)

**Phase 3: 자동화** (완료)
- Heartbeat (정책 기반 스케줄, 활성 시간 제한, 세션 연동)
- Cron (interval/at 스케줄, 실행 잠금, 연속 실패 백오프)
- 알림 (SSE 브로커, OS 데스크톱 알림 fallback)
- 자동화 도구 (cron_*/heartbeat_* 빌트인)

**Phase 4: 스킬/플러그인 확장성** (완료)
- 스킬 frontmatter 로더 + `available_skills` 프롬프트 주입
- 선언형 플러그인(`tarsncase.plugin.json`) 로더
- 확장 핫리로드(`POST /v1/runtime/extensions/reload`) + `/skills`/`/plugins`/`/mcp` 명령

**Phase 5: Gateway/Channel 런타임 + Core Action 도구** (진행 중)
- in-process gateway run registry (`accepted|running|completed|failed|canceled`)
- agent/gateway/channels API (`/v1/agent/*`, `/v1/gateway/*`, `/v1/channels/*`)
- OpenClaw core action 대응 도구(`sessions_*`, `agents_list`, `message`, `browser`, `nodes`, `gateway`)
- `cmd/tars` runtime 명령(`/agents`, `/spawn`, `/runs`, `/run`, `/cancel-run`, `/gateway`, `/channels`)

**Phase 6: cased 감시 데몬** (종료)
- 공개 배포 단순화를 위해 `cmd/cased`/`internal/sentinel` 제거
- 프로세스 감시는 systemd/launchd/docker 정책으로 위임
- `tars` 헬스체크 endpoint(`GET /v1/healthz`)는 유지

### 최근 주요 변경

**2026-02-15**
- `tars`/`tars-ui` 리팩토링: Extract Function 패턴으로 파일 크기 축소 (main.go 76%↓, index.tsx 59%↓)
- `gemini`/`gemini-native` provider 추가: OpenAI-compatible 및 native API 지원
- Cron 확장: session_target, delivery_mode, per-job 실행 기록, 동시 실행 잠금
- Agent Loop 설정화: `agent_max_iterations` ENV/YAML 제어

**2026-02-16**
- 설정 파일 경로 자동 탐지: `config/standalone.yaml` 존재 시 자동 로드
- 기본 개발 포트 통일: `tars serve`/`tars` 기본값을 `127.0.0.1:43180`으로 변경
- MCP 안정화:
  - `sequential-thinking` 호환을 위해 jsonline/content-length 이중 전송 모드 지원
  - 타임아웃 시 세션 abort 및 안전한 재시도 경로 추가

**2026-02-17**
- Gateway 런타임 확장:
  - `SetExecutors(...)`로 런타임 executor 동적 교체 지원
  - gateway reload 시 extension refresh hook 선반영 후 재로드
  - `GET /v1/agent/agents` 추가, agent 메타데이터(`source`, `entry`) 노출
  - `workspace/agents/*/AGENT.md` watch 기반 자동 반영(`gateway_agents_watch`, `gateway_agents_watch_debounce_ms`)
  - `GET /v1/gateway/status`에 agent watcher telemetry(`agents_count`, `agents_watch_enabled`, `agents_reload_version`, `agents_last_reload_at`) 추가
  - markdown sub-agent 정책 MVP: `tools_allow`(YAML list) allowlist 적용 + 허용 외 도구 하드 차단
  - `/v1/agent/agents` 정책 메타데이터(`policy_mode`, `tools_allow_count`, `tools_allow`) 노출
- Gateway persistence/recovery:
  - run/channel snapshot JSON 저장(`runs.json`, `channels.json`) + 원자적 write/rename
  - tars 재시작 시 snapshot 자동 복구, 실행 중 run은 `canceled by restart recovery`로 정리
  - 보존 정책(`gateway_runs_max_records`, `gateway_channels_max_messages_per_channel`) 적용
  - `/v1/gateway/status` telemetry 확장(`persistence_*`, `runs_restored`, `channels_restored`, `last_persist_at`, `last_restore_at`, `last_restore_error`)
- Web 도구 강화:
  - `web_search`: Brave/Perplexity provider 선택 + cache TTL
  - `web_fetch`: SSRF 가드 + private host allowlist
- `cmd/tars` 명령 확장:
  - `/agents --detail` (source/entry/policy 포함)
  - `/spawn` 옵션 파싱(`--agent`, `--title`, `--session`, `--wait`)
  - `/gateway`에 persistence/restore telemetry 출력

**2026-02-18**
- 프로젝트 간소화 시작:
  - `cmd/cased`, `internal/sentinel`, `internal/config/cased*` 제거
  - `config/cased.config.example.yaml` 및 cased 운영 템플릿 제거
  - `cmd/tars` 재도입(MVP): `/v1/chat` SSE + 기본 인터랙션(`/new`, `/session`, `/quit`)
  - `cmd/tars` 2차 확장: 세션/상태/확장 명령(`/sessions`, `/history`, `/export`, `/search`, `/status`, `/compact`, `/heartbeat`, `/skills`, `/plugins`, `/mcp`, `/reload`) + runtime 명령(`/agents`, `/spawn`, `/runs`, `/run`, `/cancel-run`, `/gateway`)
  - `cmd/tars` 3차 확장: `/cron {list|get|runs|add|run|delete|enable|disable}`, `/channels`, `/resume`, `/agents --detail`
  - `cmd/tars` 4차 확장: `/notify {list|filter|open|clear}` + `/v1/events/stream` 구독 기반 알림 로컬 버퍼
  - `cmd/tars` 5차 확장: `/gateway summary|runs|channels` + `/gateway report ...`로 gateway report API 조회 지원
  - `cmd/tars` 6차 확장: `/health`로 `tars` `/v1/healthz` 직접 점검 지원
  - `cmd/tars` HTTP 경로 해석 보강: query string이 path로 인코딩되지 않도록 `runtimeClient.resolve` 수정
  - Make 타깃 정리: `dev-cased`/`run-cased`/`dev-tars-ui` 제거, `dev-tars` 추가

**2026-02-19**
- 단일 워크스페이스 모델로 복귀:
  - 서버는 하나의 `workspace_dir`만 사용하며 `workspace_id` 라우팅을 제거
  - `agent runs`, `gateway reports`, `channels inbound`, `sessions_*` 경로에서 workspace 분기를 제거
  - run/channel payload의 `workspace_id` 제거
- 인증/권한 가시성/경계 강화:
  - `GET /v1/auth/whoami` 추가 (`authenticated`, `auth_role`, `is_admin`, `auth_mode`)
  - `cmd/tars /whoami` 추가
  - admin 경로 매트릭스 고정(`runtime/extensions reload`, `gateway reload/restart`, `channels webhook`) + wildcard 경로 처리
- 저장소/백그라운드 실행 단순화:
  - `sessions`, `chat memory`, `compact`, `cron`, `gateway`가 base workspace 경로만 사용
  - cron/heartbeat background manager가 단일 workspace store만 처리
- `cmd/tars` 운영 출력:
  - `/runs`, `/run`, `/gateway runs`, `/gateway channels`에서 workspace 표시 제거
- 정책 위반 진단 강화:
  - gateway run 실패 시 `policy_blocked_tool`, `policy_allowed_tools`를 함께 기록
  - `cmd/tars /run`과 `/runs`에서 `diag`/`blocked`/`policy_allowed`를 표시
  - `cmd/tars /run`에 `policy_denied`, `policy_risk_max` 표시 추가
- `cmd/tars /gateway status` 가시성 개선:
  - `agents_reload_version`, `last_restore_error`를 출력
- `cmd/tars` UX 전면 개편(Phase TARS-UX-1):
  - Bubble Tea 3패널 TUI(`Chat`/`Status`/`Notifications`)로 단일화
  - 채팅 delta와 status 이벤트 분리 렌더링
  - `/trace [on|off]`, `/trace filter {all|llm|tool|error|system}` 지원
  - 입력 UX: 히스토리(Up/Down), 자동완성(Tab), ESC 클리어/스트림 취소
- browser/vault 운영 경로 추가:
  - 브라우저 API: `/v1/browser/status|profiles|login|check|run`
  - Vault 상태 API: `/v1/vault/status`
  - 브라우저 relay 보안: loopback + origin allowlist + `Tars-Relay-Token`
  - `cmd/tars` 명령: `/browser`, `/vault`
  - site flow 정책: `allowed_hosts` 차단/검증, flow profile 고정 적용
- 채팅 상태 preview 민감정보 마스킹 강화:
  - `password/token/secret/api_key/authorization` key-value 및 bearer 토큰 패턴 redaction
- 운영 스모크:
  - `scripts/smoke_auth_workspace.sh` 추가
  - `make smoke-auth`로 auth/role 경계 기본 점검 자동화

**2026-02-21**
- 리팩토링 마감(기능 변경 없음):
  - `internal/tarsapp` 엔트리/부트스트랩/API 실행 경로 분리(`main_bootstrap.go`, `main_serve_api.go`, `main_cli.go`, `main_options.go`)
  - chat handler 파이프라인 분리(`handler_chat_pipeline.go`) 및 공통 에러 응답 헬퍼 정리
  - `internal/config/defaults.go`를 로딩/병합/ENV/YAML 파서 모듈로 분리, YAML 파서를 `yaml.v3` 기반으로 통일
  - `internal/mcp/client`를 API 계층과 transport/protocol 계층으로 분리
  - `internal/llm/gemini_native` 변환/채팅 계층 분리, `bifrost*` 파일명을 OpenAI-compatible 의미로 정정
- 구조 재편(후속):
  - `internal/tarsapp`를 `internal/tarsserver`로 리네임
  - `cmd/tars`를 엔트리 3파일(`main.go`, `client_main.go`, `server_main.go`)로 축소
  - TUI/명령 로직을 `internal/tarsclient`로 이동
  - 공용 프로토콜 레이어를 `pkg/tarsclient`로 분리(`Do`, `StreamSSE`, `StreamChat`, `StreamEvents`)
  - 공용 env 로더를 `internal/envloader`로 분리해 client/server가 함께 사용
- 개발/검증 경로 정리:
  - `Makefile`에 `lint` 타깃 추가(`lint: vet`)
  - 최종 검증: `make test`, `make lint` 통과

**상세 이력**
- 일일 개발 이력은 `git log` 참조
- Phase 4-7 계획은 `PLAN.md` 참조
