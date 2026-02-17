# AGENTS.md

## 목적
- 이 저장소에서 Codex/에이전트가 일관된 방식으로 개발을 진행하도록 운영 기준을 정의한다.
- 현재 목표는 gateway 기반 서브에이전트 런타임을 고도화하고, 남은 `cased` 안정성 기능을 완성하는 것이다.

## 개발 원칙
- MVP 중심으로 작은 단위로 구현한다.
- TDD를 따른다. 실패 테스트를 먼저 추가하고 구현으로 통과시킨다.
- 오버엔지니어링을 피하고 지금 필요한 코드만 작성한다.
- 서버 책임 로직은 `tarsd`, 클라이언트 UX는 `tars`에 둔다.
- OpenClaw는 개념/패턴 참고용으로만 사용하고 Go 관용구로 독립 구현한다.

## 코딩 가이드라인 (상세)

상세한 가이드라인은 `CLAUDE.md`를 참조. 핵심 원칙:

### 1. 구현 전 사고 (Think Before Coding)

- 가정을 명시적으로 표현한다. 불확실하면 질문한다.
- 여러 해석이 가능하면 모두 제시하고, 임의로 선택하지 않는다.
- 더 간단한 방법이 있으면 제안한다.
- 불명확한 부분이 있으면 멈추고 질문한다.

### 2. 단순함 우선 (Simplicity First)

- 문제를 해결하는 최소한의 코드만 작성한다.
- 요청되지 않은 기능, 추상화, 유연성, 설정 가능성을 추가하지 않는다.
- 일회용 코드에 추상화를 만들지 않는다.
- 불가능한 시나리오에 대한 에러 처리를 하지 않는다.
- 200줄을 50줄로 줄일 수 있으면 다시 작성한다.

### 3. 외과적 변경 (Surgical Changes)

- 반드시 필요한 부분만 수정한다. 본인이 만든 문제만 정리한다.
- 인접 코드, 주석, 포맷팅을 "개선"하지 않는다.
- 기존 스타일을 유지한다.
- 무관한 dead code를 발견하면 언급만 하고 삭제하지 않는다.
- 본인의 변경으로 생긴 미사용 import/변수/함수만 제거한다.
- 모든 변경 라인이 사용자 요청과 직접 연결되어야 한다.

### 4. 목표 주도 실행 (Goal-Driven Execution)

- 검증 가능한 성공 기준을 정의한다.
- 작업을 검증 가능한 목표로 변환한다:
  - "검증 추가" → "잘못된 입력에 대한 테스트 작성 후 통과시키기"
  - "버그 수정" → "재현 테스트 작성 후 통과시키기"
  - "리팩터링 X" → "리팩터링 전후 테스트 통과 확인"
- 다단계 작업은 간단한 계획을 먼저 작성한다:
  1. [단계] → 검증: [확인사항]
  2. [단계] → 검증: [확인사항]
  3. [단계] → 검증: [확인사항]

## 현재 구현 상태 (2026-02-17 기준)

- 서버 측 채팅 API `POST /v1/chat`는 구현되어 있다.
- 세션 관리 API(`GET/POST/DELETE /v1/sessions`, history/export/search)와 상태 API(`GET /v1/status`)가 구현되어 있다.
- LLM Chat 인터페이스(`Client.Chat`)와 스트리밍 콜백(`OnDelta`)이 구현되어 있다.
- 워크스페이스 부트스트랩 파일(AGENTS/SOUL/USER/IDENTITY/TOOLS/HEARTBEAT/MEMORY) 생성과 시스템 프롬프트 조립이 구현되어 있다.
- `/compact` 요약 저장 + 로딩 경계(Compaction summary boundary 포함)가 구현되어 있다.
- `tars-ui` 슬래시 명령(`/new`, `/sessions`, `/resume`, `/history`, `/export`, `/search`, `/status`, `/compact`)이 연결되어 있다.
- 채팅 루프는 요청마다 등록된 전체 도구 스키마를 주입한다(OpenClaw parity).
- 미주입 도구/selector 기반 정책 주입 경로는 제거되어 설정 항목도 더 이상 사용하지 않는다.
- 확장 빌트인 도구(`read/write/edit/glob`, `process`, `apply_patch`, `web_fetch`, `web_search`, `cron`, `heartbeat`)가 구현되어 있다.
- 크론잡 상세/실행 이력 조회(`cron_get`, `cron_runs`)와 관련 API가 구현되어 있다.
- `tars-ui`에 `/cron get`, `/cron runs`, `/notify` 명령 및 알림 프리뷰/필터/미읽음 카운트가 구현되어 있다.
- 런타임 알림은 세션 연결 시 SSE로 전달되고, 비연결 시 OS 알림 커맨드 폴백이 동작한다.
- Agent loop에서 `exec` 도구의 누락 인자 패턴은 1회 자동 보정하고, 반복 패턴은 가드로 차단한다.
- 스킬 로더(`internal/skill`)가 frontmatter 파싱/우선순위 병합/`available_skills` 프롬프트 포맷/워크스페이스 미러링을 지원한다.
- 선언형 플러그인 로더(`internal/plugin`)가 `tarsncase.plugin.json`을 로드하고 skill dir + MCP server를 병합한다.
- 확장 매니저(`internal/extensions`)가 스킬/플러그인/MCP를 통합 스냅샷으로 관리하고 fsnotify 기반 핫리로드를 지원한다.
- API `GET /v1/skills`, `GET /v1/skills/{name}`, `GET /v1/plugins`, `POST /v1/runtime/extensions/reload`가 구현되어 있다.
- `tars-ui` 명령 `/skills`, `/plugins`, `/mcp`가 추가되었고, 미지의 `/{skill}` 입력은 채팅 경로로 전달된다.
- `tars-ui` 입력 엔진이 `CustomTextInput`으로 교체되어 bracketed paste(`\x1b[200~...\x1b[201~`)를 안정 처리한다.
- `tars-ui` 입력창이 Undo/Kill-Ring/히스토리/자동완성을 지원한다.
  - Undo: `Ctrl+Z`
  - Kill: `Ctrl+U`, `Ctrl+K`
  - Yank: `Ctrl+Y`, Yank-pop: `Alt+Y`
  - History: `↑/↓`
  - Command completion: `Tab` (`/`, `/cron`, `/notify`)
- `Esc`로 입력을 즉시 초기화하고, 진행 중 LLM 스트리밍은 abort로 중단할 수 있다.
- `tarsd`/`tars-ui` 기본 개발 포트가 `127.0.0.1:43180`으로 통일되었다.
- MCP 런타임이 JSON line 전송 방식 서버(`sequential-thinking`)를 자동 감지/폴백하여 연결한다.
- in-process gateway 런타임(`internal/gateway`)이 추가되어 run registry, 채널, browser/nodes 상태를 함께 관리한다.
- agent/gateway/channels API가 구현되어 비동기 run 제어가 가능하다.
  - `GET /v1/agent/agents`
  - `GET/POST /v1/agent/runs`
  - `GET /v1/agent/runs/{run_id}`
  - `POST /v1/agent/runs/{run_id}/cancel`
  - `GET /v1/gateway/status`
  - `POST /v1/gateway/reload`
  - `POST /v1/gateway/restart`
  - `POST /v1/channels/webhook/inbound/{channel_id}`
  - `POST /v1/channels/telegram/webhook/{bot_id}`
- OpenClaw core action 기준 built-in 도구가 확장되었다.
  - 세션/런: `sessions_list`, `sessions_history`, `sessions_send`, `sessions_spawn`, `sessions_runs`, `agents_list`
  - 게이트웨이 계열: `message`, `browser`, `nodes`, `gateway`
- `tars-ui`에 runtime 제어 명령이 추가되었다.
  - `/agents`, `/agents --detail`
  - `/runs`, `/run {id}`, `/cancel-run {id}`
  - `/spawn [--agent] [--title] [--session] [--wait] {message}`
  - `/gateway {status|reload|restart}`, `/channels`
- `POST /v1/runtime/extensions/reload` 호출 시 gateway executor refresh가 자동 반영되며 응답에 `gateway_refreshed`, `gateway_agents`가 포함된다.
- `workspace/agents/*/AGENT.md` 변경은 gateway watcher가 자동 감지해 executor를 갱신한다.
  - 설정: `gateway_agents_watch`, `gateway_agents_watch_debounce_ms`
  - 상태: `GET /v1/gateway/status`의 `agents_count`, `agents_watch_enabled`, `agents_reload_version`, `agents_last_reload_at`
- gateway run/channel 상태가 디스크 스냅샷으로 영속화되고 재시작 시 자동 복구된다.
  - 설정: `gateway_persistence_enabled`, `gateway_runs_persistence_enabled`, `gateway_channels_persistence_enabled`
  - 보존 정책: `gateway_runs_max_records`, `gateway_channels_max_messages_per_channel`
  - 경로/복구: `gateway_persistence_dir`, `gateway_restore_on_startup`
  - 복구 규칙: `accepted|running` run은 재시작 복구 시 `canceled by restart recovery`로 정리
  - 상태 telemetry: `/v1/gateway/status`의 `persistence_*`, `runs_restored`, `channels_restored`, `last_persist_at`, `last_restore_at`, `last_restore_error`
- `tars-ui /gateway`가 gateway persistence/restore telemetry를 함께 표시한다.
- markdown 서브에이전트는 AGENT frontmatter `tools_allow`(YAML list) 정책을 지원한다.
  - 정책 미지정: `full` (기존 동작)
  - 정책 지정: allowlist만 주입
  - 전부 무효한 allowlist: 해당 agent 로드 제외 + diagnostics 로그
  - 정책 메타데이터: `GET /v1/agent/agents`의 `policy_mode`, `tools_allow_count`, `tools_allow`
- `web_search`가 Brave/Perplexity provider 선택 + 캐시 TTL을 지원하고, `web_fetch`는 SSRF 차단 + private host allowlist를 지원한다.

## LLM Provider 운영 정책 (2026-02-16)

- `codex-cli` provider는 제거되었다. `LLM_PROVIDER=codex-cli`는 더 이상 지원하지 않는다.
- `openai-codex` provider는 제거되었다. `LLM_PROVIDER=openai-codex`는 더 이상 지원하지 않는다.
- 현재 지원 provider: `bifrost`, `openai`, `anthropic`, `gemini`, `gemini-native`

권장 설정:
- 안정 운영: `LLM_PROVIDER=openai`, `LLM_AUTH_MODE=api-key`, `OPENAI_API_KEY` 사용
- 대체 운영: `LLM_PROVIDER=anthropic`, `LLM_AUTH_MODE=api-key`, `ANTHROPIC_API_KEY` 사용
- gemini 운영: `LLM_PROVIDER=gemini`, `LLM_AUTH_MODE=api-key`, `GEMINI_API_KEY` 사용
- gemini-native 운영: `LLM_PROVIDER=gemini-native`, `LLM_AUTH_MODE=api-key`, `GEMINI_API_KEY` 사용

## 다음 우선순위

1. `cased` 감시 데몬의 실동작(프로세스 감시/재시작/상태 노출) 구현을 마무리한다.
2. 서브에이전트 정책을 allowlist MVP에서 그룹/정규식/세션 라우팅 정책으로 확장한다.
3. gateway run/channel의 장기 아카이빙(압축/회전)과 운영 리포팅을 추가한다.

## 작업 체크리스트

- 변경 전 현재 구현 상태와 범위를 먼저 요약한다.
- 코드 변경 시 테스트를 함께 추가한다.
- 기능 단위 완료 후 `CLAUDE.md`의 코드 구조 변경 기록을 갱신한다.
