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

- `tarsd` (메인 데몬/서버): LLM 호출, 허트비트/크론 실행, 3-Layer 메모리 처리, 작업 판단과 실행 오케스트레이션을 담당한다.
- `cased` (감시 데몬): `tarsd` 프로세스 감시, 자동 재시작/복구, 감사/보안 모니터링, 업데이트/롤백 같은 안정성 제어를 담당한다.
- `tars-ui` (React/TS Ink TUI 클라이언트): 고급 대화형 UX(패널 레이아웃, 스트리밍 렌더링, 상태/디버그 시각화)를 담당한다.

## tars-tarsd 통신 프로토콜 규칙

- `tarsd`가 HTTP API 서버를 서빙하고, `tars-ui`는 해당 API를 호출하는 HTTP 클라이언트로 구현한다.
- LLM 실행, OAuth 토큰 교환/저장, heartbeat/cron 실행 같은 서버 책임 로직은 반드시 `tarsd`에서 수행한다.
- `tars-ui`는 사용자 입력 수집, API 요청/응답 렌더링 같은 클라이언트 UX만 담당한다.
- LLM 응답은 `tarsd`의 REST API로 제공하고, `tars-ui`는 해당 API의 클라이언트로 구현한다.
- 인증 토큰(특히 OAuth access/refresh token)은 서버(`tarsd`)에서만 저장/관리하고, `tars-ui`는 직접 저장하지 않는다.

## 코드 구조 변경 기록

### 현재 아키텍처 구조

**바이너리**
- `tarsd`: 메인 데몬/서버 (HTTP API, LLM 호출, heartbeat/cron 실행, 메모리 관리)
- `cased`: 감시 데몬 (프로세스 감시, 자동 재시작/복구, 모니터링)
- `tars-ui`: React/TypeScript Ink 기반 TUI 클라이언트 (대화형 UX, 패널 렌더링)

**주요 패키지**
- `internal/config`: 설정 로딩 (YAML/ENV 우선순위, 환경변수 확장, 경로 자동 탐지)
- `internal/llm`: LLM provider 추상화 (bifrost, openai, anthropic, gemini, gemini-native)
- `internal/session`: 세션 관리 (JSONL transcript, 토큰 기반 히스토리 로딩, 컴팩션)
- `internal/agent`: Agent Loop (훅 기반 이벤트, 도구 실행 반복, 상태 추적)
- `internal/tool`: 빌트인 도구 (file/web/memory/automation + gateway/sessions/message/browser/nodes 계열)
- `internal/gateway`: in-process gateway 런타임 (run registry, agent executor, channels, browser/nodes 상태, run/channel snapshot 영속화/복구)
- `internal/sentinel`: cased supervisor 런타임 (child process 관리, 헬스 probe, 재시작/backoff/cooldown, 상태/이벤트 API)
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
- `tars-ui` runtime 명령(`/agents`, `/spawn`, `/runs`, `/run`, `/cancel-run`, `/gateway`, `/channels`)

**Phase 6: cased 감시 데몬** (진행 중)
- `cmd/cased` 실구현: target child 실행/감시/재시작
- `internal/sentinel` 상태머신: `starting|running|paused|cooldown|stopped|error`
- cased API: `/v1/sentinel/status`, `/v1/sentinel/events`, `/v1/sentinel/restart`, `/v1/sentinel/pause`, `/v1/sentinel/resume`
- tarsd 헬스체크 endpoint: `GET /v1/healthz`
- tars-ui 제어 명령: `/sentinel ...` + `cased_server_url`/`--cased-url`

### 최근 주요 변경

**2026-02-15**
- `tarsd`/`tars-ui` 리팩토링: Extract Function 패턴으로 파일 크기 축소 (main.go 76%↓, index.tsx 59%↓)
- `gemini`/`gemini-native` provider 추가: OpenAI-compatible 및 native API 지원
- Cron 확장: session_target, delivery_mode, per-job 실행 기록, 동시 실행 잠금
- Agent Loop 설정화: `agent_max_iterations` ENV/YAML 제어

**2026-02-16**
- 설정 파일 경로 자동 탐지: `config/standalone.yaml` 존재 시 자동 로드
- `tars-ui` 설정 파일 지원: CLI 플래그 우선순위 정리
- 기본 개발 포트 통일: `tarsd`/`tars-ui` 기본값을 `127.0.0.1:43180`으로 변경
- `tars-ui` 입력 엔진 고도화:
  - `ink-text-input` 의존 경로를 `CustomTextInput`으로 교체
  - bracketed paste 파서(`src/ui/paste.ts`) 추가
  - 편집 코어(`src/ui/inputEdit.ts`), undo stack(`src/ui/undoStack.ts`), kill ring(`src/ui/killRing.ts`) 추가
  - 입력 히스토리(`src/ui/inputHistory.ts`) + 명령 자동완성(`src/commands/complete.ts`) 추가
  - `Esc`로 입력 초기화 및 진행 중 LLM 스트림 취소(abort) 지원
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
  - tarsd 재시작 시 snapshot 자동 복구, 실행 중 run은 `canceled by restart recovery`로 정리
  - 보존 정책(`gateway_runs_max_records`, `gateway_channels_max_messages_per_channel`) 적용
  - `/v1/gateway/status` telemetry 확장(`persistence_*`, `runs_restored`, `channels_restored`, `last_persist_at`, `last_restore_at`, `last_restore_error`)
- Web 도구 강화:
  - `web_search`: Brave/Perplexity provider 선택 + cache TTL
  - `web_fetch`: SSRF 가드 + private host allowlist
- `tars-ui` 명령 확장:
  - `/agents --detail` (source/entry 포함)
  - `/agents --detail` 정책 컬럼(`POLICY`, `ALLOW`) 추가
  - `/spawn` 옵션 자동완성(`--agent`, `--title`, `--session`, `--wait`)
  - `/gateway`에 persistence/restore telemetry 출력

**2026-02-18**
- `cased` 설정/부트스트랩 구현:
  - `internal/config`에 `LoadCased`/`ResolveCasedConfigPath` 추가
  - `target_command` 필수 검증 + YAML/ENV(`target_args_json`, `target_env_json`) 파싱
- `internal/sentinel` 신규 구현:
  - supervisor lifecycle(autostart, pause/resume, manual restart)
  - restart 정책(지수 backoff + max attempts 초과 cool-down)
  - probe 기반 health 연속 실패 재시작
  - in-memory ring buffer events
- `cased` HTTP API 노출 + `tars-ui` `/sentinel` 명령 연동
- `tarsd` `GET /v1/healthz` 추가 및 cased 기본 probe 경로로 사용

**상세 이력**
- 일일 개발 이력은 `git log` 참조
- Phase 4-7 계획은 `PLAN.md` 참조
