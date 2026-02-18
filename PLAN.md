# TARS 개발 계획서 (v4)

> 최종 갱신: 2026-02-18
> 모듈: `github.com/devlikebear/tarsncase`
> 바이너리: `tarsd` (메인 데몬), `tars` (Go CLI/TUI 클라이언트, 도입 시작)

## 1. 현재 구현 현황

### 완료된 기능 (Phase 0)
- [x] Go 프로젝트 스켈레톤 (`cmd/tarsd`, `cmd/cased`)
- [x] Makefile + GitHub Actions CI (`make test`)
- [x] 런타임 설정 로더 (`internal/config`) — YAML + 환경변수 + `${ENV_VAR}` 확장
- [x] zerolog 구조화 로깅
- [x] cobra CLI 프레임워크 (tarsd, tars 모두)
- [x] 3-Layer 메모리 기본 구조 (`internal/memory`)
  - `EnsureWorkspace()`: HEARTBEAT.md, MEMORY.md, `_shared/`, `memory/` 생성
  - `AppendDailyLog()`: `memory/YYYY-MM-DD.md` append
- [x] 멀티 LLM 프로바이더 (`internal/llm`)
  - bifrost, openai (OpenAI-compatible 통합), anthropic, gemini(OpenAI-compatible), gemini-native
  - 공통 인터페이스: `Client.Ask(ctx, prompt) (string, error)`
- [x] OAuth 토큰 해석 (`internal/auth`) — claude-code(anthropic), google-antigravity(gemini/gemini-native)
- [x] provider 정책 정리 — `codex-cli`, `openai-codex` 제거
- [x] 기본 허트비트 (`internal/heartbeat`)
  - `RunOnce`: HEARTBEAT.md 읽기 → daily log 기록
  - `RunOnceWithLLM`: HEARTBEAT.md + MEMORY.md + daily log → LLM 호출 → 응답 기록
  - `RunLoop` / `RunLoopWithLLM`: ticker 기반 반복 실행
- [x] tarsd HTTP API: `POST /v1/heartbeat/run-once` (LLM 응답 반환)
- [x] tarsd graceful shutdown (signal.NotifyContext)
- [x] heartbeat API: `POST /v1/heartbeat/run-once`
- [x] `--run-once` / `--run-loop` 상호 배타 검증
- [x] `internal/cli` 공통 에러 처리 (ExitError, IsFlagError)

### 완료된 기능 (Phase 1 진행분)
- [x] 워크스페이스 부트스트랩 파일 확장 (`AGENTS.md`, `SOUL.md`, `USER.md`, `IDENTITY.md`, `TOOLS.md`)
- [x] 시스템 프롬프트 조립 (`internal/prompt`) — 부트스트랩 파일 주입, 파일당 20000자 제한, sub-agent 모드 지원
- [x] 세션 관리 (`internal/session`) — sessions.json + JSONL transcript, CRUD, history/search/export, 토큰 기반 동적 로딩
- [x] LLM Chat API (`internal/llm`) — `Client.Chat`, `OnDelta` 스트리밍 콜백
- [x] tarsd 채팅 API (`POST /v1/chat`) — SSE(delta/done), 세션 자동 생성/지정, transcript 저장
- [x] `tars-ui` 채팅 (`tars-ui`) — 입력창 + SSE 스트리밍 + 세션 유지
- [x] 디버그 로깅 (`--verbose`) — `tars↔tarsd` 및 `tarsd↔LLM` 상세 로그
- [x] non-streaming provider fallback — `OnDelta` 미호출 시 최종 응답을 `delta`로 1회 전송
- [x] `tars-ui` 초기 골격 추가 (`tars-ui/`) — React/TypeScript + Ink 기반 TUI, `/v1/chat` SSE 직결, Chat/Status 패널 분리
- [x] `cmd/tars` 제거 완료 — 클라이언트는 `tars-ui` 단일화, 자동화는 `Make + curl`로 통일

### 미구현 (Phase 1~6에서 개발)
- [x] 컨텍스트 압축 고도화 (LLM 요약 품질 향상 + 로딩 경계 정교화, 토큰 예산 기반 최소 최근 2메시지 유지 안정화)
- [x] 채팅 결과의 메모리 계층 자동 반영 (`MEMORY.md`, `memory/YYYY-MM-DD.md`)
- [x] 빌트인 도구 + Agent Loop (LLM → tool_calls → 실행 → 반복)
- [x] 허트비트 Agent Loop 통합 (도구 자율 실행)
- [x] 크론잡 매니저 (AI 판단 기반 자율 실행)
- [x] 스킬 시스템 (SKILL.md, 레지스트리, 시스템 프롬프트 주입)
- [x] 플러그인 시스템 (매니페스트, 로더, 도구 등록)
- [x] MCP 클라이언트 (stdio/SSE, 도구 어댑터)
- [x] in-process gateway runtime + run registry (`internal/gateway`)
- [x] OpenClaw core action 도구 확장 (`sessions_*`, `agents_list`, `message`, `browser`, `nodes`, `gateway`)
- [x] cased 감시 데몬

### 최근 반영

#### 2026-02-18
- [x] `cased` 실구현(Phase 8-A)
  - `internal/sentinel` supervisor 추가: child process 실행/감시, backoff/cooldown 재시작 정책, health probe 실패 임계치 재시작
  - cased API 추가: `GET /v1/sentinel/status`, `GET /v1/sentinel/events?limit=N`, `POST /v1/sentinel/restart|pause|resume`
  - cased 설정 로더 추가: `target_command` 필수, `target_args_json`, `target_env_json`, probe/restart/event buffer/autostart
  - `tarsd` 헬스체크 endpoint 추가: `GET /v1/healthz`
  - `tars-ui` 명령 추가: `/sentinel`, `/sentinel restart`, `/sentinel pause`, `/sentinel resume`, `/sentinel events [limit]`
  - `tars-ui` 설정 확장: `cased_server_url`, CLI 플래그 `--cased-url`
- [x] Phase 9-Lite 경량 안정화
  - sentinel startup grace 추가: `probe_start_grace_ms`(기본 15000ms)
  - sentinel 안정성 telemetry 추가: `start_grace_until`, `consecutive_failures`, `last_probe_duration_ms`
  - sentinel 이벤트 영속화 추가: `event_persistence_enabled`, `event_store_path`, `event_store_max_records`
  - sub-agent 정책 V2 추가: `tools_allow_groups`, `tools_allow_patterns`, `session_routing_mode`, `session_fixed_id`
  - gateway 리포트 API 추가: `/v1/gateway/reports/summary`, `/v1/gateway/reports/runs`, `/v1/gateway/reports/channels`
  - gateway 경량 기본값 추가: `gateway_report_summary_enabled=true`, `gateway_archive_enabled=false`
  - 단일 대상 운영 템플릿 추가: `config/ops/cased.systemd.service.example`, `config/ops/cased.launchd.plist.example`, `config/ops/cased-runbook.md`
- [x] 프로젝트 간소화 전환 시작(공개 릴리즈 준비)
  - `cmd/cased`, `internal/sentinel`, `internal/config/cased*`, `config/cased.config.example.yaml` 제거
  - `cmd/tars` 재도입(MVP): `/v1/chat` SSE 클라이언트 + 기본 REPL(`/new`, `/session`, `/quit`)
  - `cmd/tars` 2차: 세션/상태/확장 명령 + runtime 명령 이식
  - `cmd/tars` 3차: `/cron {list|get|runs|add|run|delete|enable|disable}`, `/channels`, `/resume`, `/agents --detail` 추가
  - `cmd/tars` 4차: `/notify {list|filter|open|clear}` 추가, `/v1/events/stream` 백그라운드 구독으로 알림 로컬 버퍼 유지
  - Make 타깃 단순화: `dev-cased`/`run-cased` 제거, `dev-tars` 추가

#### 2026-02-17
- [x] gateway/agent/channels API 추가
  - `GET /v1/agent/agents`
  - `GET/POST /v1/agent/runs`
  - `GET /v1/agent/runs/{run_id}`
  - `POST /v1/agent/runs/{run_id}/cancel`
  - `GET /v1/gateway/status`, `POST /v1/gateway/reload`, `POST /v1/gateway/restart`
  - `POST /v1/channels/webhook/inbound/{channel_id}`, `POST /v1/channels/telegram/webhook/{bot_id}`
- [x] Gateway executor metadata(`source`, `entry`) 노출 + `/agents --detail` 지원
- [x] `workspace/agents/*/AGENT.md` watcher 자동 반영
  - 설정: `gateway_agents_watch`, `gateway_agents_watch_debounce_ms`
  - 상태: `/v1/gateway/status`에 `agents_count`, `agents_watch_enabled`, `agents_reload_version`, `agents_last_reload_at`
- [x] `POST /v1/runtime/extensions/reload` 시 gateway executor 자동 refresh 연동
  - 응답 확장: `gateway_refreshed`, `gateway_agents` (additive)
- [x] markdown 서브에이전트 `tools_allow` 정책 MVP
  - AGENT frontmatter `tools_allow`(YAML list) 파싱 + canonicalization
  - 허용 도구만 runner schema 주입, 허용 외 호출 하드 차단
  - invalid-only allowlist는 agent 로드 제외 + diagnostics
  - `/v1/agent/agents` 정책 메타데이터(`policy_mode`, `tools_allow_count`, `tools_allow`) 노출
  - `tars-ui /agents --detail` 정책 컬럼(`POLICY`, `ALLOW`) 표시
- [x] `tars-ui` runtime 명령 확장
  - `/agents`, `/runs`, `/spawn`, `/run`, `/cancel-run`, `/gateway`, `/channels`
  - `/spawn` 옵션 자동완성(`--agent`, `--title`, `--session`, `--wait`)
- [x] web 도구 강화
  - `web_search`: Brave/Perplexity provider + cache TTL
  - `web_fetch`: SSRF 차단 + private host allowlist/옵션
- [x] Gateway 영속화/복구(Phase 7-C)
  - run/channel snapshot 저장: `${gateway_persistence_dir}/runs.json`, `${gateway_persistence_dir}/channels.json`
  - 재시작 복구: `accepted|running` run -> `canceled by restart recovery`
  - 보존 정책: `gateway_runs_max_records`, `gateway_channels_max_messages_per_channel`
  - 상태 telemetry 확장: `/v1/gateway/status`에 persistence/restore 메타데이터 노출
  - `tars-ui /gateway`에 persistence telemetry 표시

#### 2026-02-16
- [x] 기본 개발 포트를 `127.0.0.1:43180`으로 통일 (`tarsd`/`tars-ui`/예제 설정/README)
- [x] 런타임 확장 명령 `/reload` 추가 (`POST /v1/runtime/extensions/reload` 호출)
- [x] SSE/네트워크 진단 강화 (endpoint 포함 에러, keepalive 개선, 재연결 백오프)
- [x] MCP 런타임 안정화
  - `sequential-thinking` 대응 jsonline/content-length 이중 전송 지원
  - 타임아웃 시 세션 abort + 안전한 재시도
- [x] `tars-ui` 입력 UX 고도화
  - bracketed paste
  - undo/kill/yank/yank-pop
  - 명령 히스토리(↑/↓)
  - 명령 자동완성(Tab)
  - `Esc` 입력 초기화 + in-flight LLM 스트림 중단

---

## 2. 핵심 설계 원칙

| 원칙 | 설명 |
|------|------|
| AI First | 허트비트, 크론, 채팅 모두 HEARTBEAT.md 같은 자연어 지시서를 AI가 읽고 자율 판단/실행 |
| tarsd API → tars CLI | 모든 기능은 tarsd HTTP API로 먼저 구현, tars는 해당 API의 클라이언트 |
| SSE 스트리밍 | 채팅 응답은 Phase 1부터 SSE 스트리밍으로 제공 |
| 토큰 기반 동적 히스토리 | 세션 로드 시 context_window - reserve_tokens 범위 내에서 역순 로딩 |
| 마크다운이 진실의 원천 | 3-Layer 메모리는 마크다운 파일 기반, SQLite는 나중에 검색 인덱스로 추가 |
| UI/로직 분리 | `tarsd`는 실행 로직, `tars`는 API 클라이언트 UX 담당 |

---

## 2-A. LLM Provider 운영 가이드 (2026-02-15)

- `codex-cli` provider는 제거되었다.
- `openai-codex` provider는 제거되었다.
- `gemini` provider는 OpenAI-compatible API 경로로 지원된다.
- `gemini-native` provider는 Gemini native API(`generateContent`, `streamGenerateContent`)로 지원된다.

권장 환경변수 예시(안정 경로):
```bash
export LLM_PROVIDER=openai
export LLM_AUTH_MODE=api-key
export OPENAI_API_KEY=...
```

권장 환경변수 예시(대체 경로):
```bash
export LLM_PROVIDER=anthropic
export LLM_AUTH_MODE=api-key
export ANTHROPIC_API_KEY=...
```

권장 환경변수 예시(gemini 경로):
```bash
export LLM_PROVIDER=gemini
export LLM_AUTH_MODE=api-key
export GEMINI_API_KEY=...
```

권장 환경변수 예시(gemini-native 경로):
```bash
export LLM_PROVIDER=gemini-native
export LLM_AUTH_MODE=api-key
export GEMINI_API_KEY=...
```

---

## 3. Phase별 상세 개발 계획

### Phase 1: LLM 대화형 채팅

**목표**: tarsd가 SSE 스트리밍으로 멀티턴 채팅을 제공하고, tars CLI에서 대화형 REPL로 사용

**마일스톤**: `tarsd --serve-api` 실행 후 `tars-ui`로 멀티턴 대화가 가능한 상태 (달성)

#### 1-A. 워크스페이스 부트스트랩 파일 확장

상태: 완료

기존 `internal/memory/workspace.go`의 `EnsureWorkspace()` 확장:

| 파일 | 역할 | OpenClaw 참고 |
|------|------|--------------|
| `AGENTS.md` | 에이전트 운영 지침, 메모리 사용법 | `docs/reference/templates/AGENTS.md` |
| `SOUL.md` | 페르소나, 톤, 경계 | `docs/reference/templates/SOUL.md` |
| `USER.md` | 사용자 프로필 | `docs/reference/templates/USER.dev.md` |
| `IDENTITY.md` | 에이전트 이름, 성격 | `docs/reference/templates/IDENTITY.dev.md` |
| `TOOLS.md` | 사용자 환경별 도구 메모 | `docs/reference/templates/TOOLS.md` |
| `HEARTBEAT.md` | 허트비트 체크리스트 (기존) | `docs/reference/templates/HEARTBEAT.md` |
| `MEMORY.md` | 큐레이션된 장기 메모리 (기존) | `docs/concepts/memory.md` |

**구현 파일**: `internal/memory/workspace.go` 수정

#### 1-B. 세션 관리

상태: 완료

**새 패키지**: `internal/session/`

| 파일 | 역할 |
|------|------|
| `session.go` | `Session` 구조체, `Store` (세션 목록 관리), 생성/전환/리줌 |
| `transcript.go` | JSONL transcript append/read, 토큰 기반 동적 로딩 |
| `message.go` | `Message` 타입 (role, content, tool_calls, tool_results, ts) |

**세션 저장 구조:**
```
{workspace}/sessions/
  sessions.json          # sessionKey → {sessionId, updatedAt, ...}
  {sessionId}.jsonl      # append-only transcript
```

**토큰 기반 동적 히스토리 로딩:**
- JSONL을 역순으로 읽으며 `len(content)/4` heuristic으로 토큰 추정
- `contextWindow - reserveTokens` 초과 시 중단 (기본 128K - 4096)
- 설정: `config.yaml`의 `context_window`, `reserve_tokens`

**OpenClaw 참고:**
- `docs/concepts/session.md` — 세션 키, 라우팅, DM 스코프
- `docs/reference/session-management-compaction.md` — 세션 저장소 2계층 구조 (sessions.json + JSONL)

#### 1-C. 시스템 프롬프트 조립

상태: 완료

**새 패키지**: `internal/prompt/`

| 파일 | 역할 |
|------|------|
| `builder.go` | 시스템 프롬프트 조립 (워크스페이스 파일 주입) |

**프롬프트 구조** (OpenClaw `docs/concepts/system-prompt.md` 참고):
1. 기본 역할 정의 + 안전 가이드라인
2. **워크스페이스 파일 주입**: AGENTS.md, SOUL.md, USER.md, IDENTITY.md, TOOLS.md, HEARTBEAT.md, BOOTSTRAP.md(신규 워크스페이스만), MEMORY.md(존재 시)
3. 현재 시간 (UTC + 사용자 시간대), 런타임 정보 (OS, 모델명 등)
4. (Phase 2 이후) 도구 목록 + 스킬 목록

> **Note**: `memory/*.md` daily log 파일은 시스템 프롬프트에 자동 주입되지 않음. `memory_search`, `memory_get` 도구로 on-demand 접근.
> Sub-agent 세션은 `AGENTS.md` + `TOOLS.md`만 주입 (다른 부트스트랩 파일 제외).

**파일 크기 제한**: 파일당 최대 20000자 (OpenClaw의 `bootstrapMaxChars` 기본값)

#### 1-D. LLM Chat API (messages 배열)

상태: 완료

**수정 패키지**: `internal/llm/`

기존 `Ask(ctx, prompt) (string, error)`에 추가:

```go
type ChatMessage struct {
    Role       string          `json:"role"`       // system, user, assistant, tool
    Content    string          `json:"content"`
    ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
    ToolCallID string          `json:"tool_call_id,omitempty"`
}

type ChatOptions struct {
    Tools    []ToolSchema      // Phase 2에서 사용
    OnDelta  func(text string) // SSE 스트리밍 콜백
}

type ChatResponse struct {
    Message    ChatMessage
    Usage      Usage
    StopReason string
}

// Client 인터페이스에 추가
Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (ChatResponse, error)
```

- `bifrost.go` / `anthropic.go`: 각각 OpenAI / Anthropic messages API에 맞게 구현
- SSE 스트리밍: `OnDelta` 콜백으로 델타 텍스트 전달
- 기존 `Ask()`는 `Chat()`의 단축 래퍼로 유지

#### 1-E. tarsd 채팅 API

상태: 완료

**수정 파일**: `cmd/tarsd/main.go`

```
POST /v1/chat
Body: {"session_id": "main", "message": "안녕"}
Response: Content-Type: text/event-stream
  data: {"type":"delta","text":"안녕하"}
  data: {"type":"delta","text":"세요"}
  data: {"type":"done","session_id":"main","usage":{"input_tokens":100,"output_tokens":50}}
```

#### 1-F. tarsd 세션 관리 API + 슬래시 명령

상태: 완료

```
GET    /v1/sessions                    # 세션 목록
POST   /v1/sessions                    # 새 세션 생성
GET    /v1/sessions/{id}               # 세션 상세 (메타데이터)
GET    /v1/sessions/{id}/history       # 세션 히스토리 (메시지 목록)
DELETE /v1/sessions/{id}               # 세션 삭제
POST   /v1/sessions/{id}/export       # 세션 내보내기 (마크다운)
GET    /v1/sessions/search?q=keyword   # 세션 검색
GET    /v1/status                      # tarsd 상태 (세션 수, 메모리 등)
POST   /v1/compact                     # 컨텍스트 압축 트리거
```

**tars-ui에서 슬래시 명령으로 접근:**
- `/new` → 새 세션 (POST /v1/sessions)
- `/sessions` → 세션 목록 (GET /v1/sessions)
- `/resume {id}` → 세션 전환
- `/history` → 현재 세션 히스토리
- `/export` → 현재 세션 마크다운 내보내기
- `/search {keyword}` → 세션 검색
- `/status` → tarsd 상태
- `/compact` → 컨텍스트 압축

#### 1-G. 컨텍스트 압축 (Compaction)

상태: 완료

**새 파일**: `internal/session/compaction.go`

**동작** (OpenClaw `docs/concepts/compaction.md` 참고):
1. 오래된 메시지들을 LLM에게 요약 요청
2. 요약 결과를 `compaction` 엔트리로 JSONL에 추가
3. 이후 세션 로드 시 compaction 요약 + 그 이후 메시지만 로드
4. **Pre-compaction memory flush**: 압축 직전 MEMORY.md/daily log에 중요 정보 저장

추가 구현 사항:
- token budget로 최근 히스토리만 로딩할 때도 compaction summary 경계를 항상 포함
- auto compact 트리거(`estimated_tokens`) + `/compact` 수동 트리거 둘 다 지원

**트리거 조건:**
- 자동: 토큰 추정치가 `contextWindow - reserveTokensFloor` 초과 시
- 수동: `/compact` 명령

#### 1-H. tars-ui 단일 클라이언트

상태: 완료 (`cmd/tars` 제거, 기능 이전 완료)

**구현 파일**: `tars-ui/src/*`

- `/v1/chat` SSE 수신 (delta/status/error/done)
- 멀티턴 대화, `session_id` 자동 유지
- 슬래시 명령(`/sessions`, `/new`, `/resume`, `/history`, `/export`, `/search`, `/status`, `/compact`, `/heartbeat`, `/quit`)
- Chat/Status/Debug 패널 분리 렌더링

#### 1-I. `tars-ui` (React/TS Ink) 도입

상태: 완료

**새 디렉터리**: `tars-ui/`

| 파일 | 역할 |
|------|------|
| `src/index.tsx` | `/v1/chat` SSE 연결, Chat/Status(및 Debug) 패널 분리, 입력창 처리 |
| `package.json` | Ink/React/TS 실행 스크립트 및 의존성 |
| `tsconfig.json` | TypeScript 빌드 설정 |

**역할 분리 원칙:**
- `tarsd`: LLM/도구/세션/메모리 오케스트레이션
- `tars-ui`: 고급 대화형 UX(레이아웃/스트리밍 렌더링/시각화)
- `tars`: 경량 CLI(운영, 자동화, 파이프라인, fallback 인터페이스)

#### 1-테스트

- `internal/session/session_test.go` — JSONL 저장/로드, 토큰 기반 동적 로딩
- `internal/session/compaction_test.go` — 압축 로직
- `internal/prompt/builder_test.go` — 프롬프트 조립 (워크스페이스 파일 주입)
- `internal/llm/*_test.go` — Chat API 추가분 테스트
- `cmd/tarsd/main_test.go` — chat API 핸들러, 세션 API 핸들러
- `tars-ui/src/**/*.test.ts` — parseArgs/chat parser/router/state 테스트

---

### Phase 2: 빌트인 도구 + Agent Loop

**목표**: LLM이 도구를 호출하여 자율적으로 작업을 수행하는 agent loop 완성

**마일스톤**: `tars-ui`에서 "이 파일 읽어줘", "날씨 검색해" 같은 도구 사용 대화가 가능한 상태

#### 2-A. 도구 인터페이스 + 레지스트리

**새 패키지**: `internal/tool/`

```go
// tool.go
type Tool struct {
    Name        string
    Description string
    Parameters  json.RawMessage  // JSON Schema
    Execute     func(ctx context.Context, params json.RawMessage) (Result, error)
}

type Result struct {
    Content []ContentBlock
    IsError bool
}

type ContentBlock struct {
    Type string  // "text", "image"
    Text string
}

type Registry struct { /* name → Tool 매핑 */ }
func (r *Registry) Register(t Tool)
func (r *Registry) Get(name string) (Tool, bool)
func (r *Registry) All() []Tool
func (r *Registry) Schemas() []ToolSchema  // LLM에 전달할 JSON Schema 목록
```

**OpenClaw 참고:**
- `docs/tools/index.md` — 도구 표면, allow/deny, 프로파일
- `docs/plugins/agent-tools.md` — 도구 등록 패턴 (TypeBox schema, execute 시그니처)

#### 2-B. 빌트인 도구 구현

| 파일 | 도구명 | 설명 | OpenClaw 참고 |
|------|--------|------|--------------|
| `exec.go` | `exec` | 셸 명령 실행 (timeout, background) | `docs/tools/exec.md` |
| `readfile.go` | `read_file` | 파일 읽기 (line range 지원) | Pi 기본 도구 |
| `writefile.go` | `write_file` | 파일 쓰기 | Pi 기본 도구 |
| `editfile.go` | `edit_file` | 파일 편집 (old→new 치환) | Pi 기본 도구 |
| `websearch.go` | `web_search` | 웹 검색 (Brave API) | `docs/tools/web.md` |
| `webfetch.go` | `web_fetch` | URL → 텍스트 추출 | `docs/tools/web.md` |
| `memory_search.go` | `memory_search` | daily log + MEMORY.md 키워드 검색 | `docs/concepts/memory.md` |
| `memory_get.go` | `memory_get` | 특정 날짜 daily log 읽기 | `docs/concepts/memory.md` |
| `session_status.go` | `session_status` | 현재 세션 정보 (토큰, 시간 등) | OpenClaw `session_status` 도구 |

#### 2-C. Agent Loop

**새 패키지**: `internal/agent/`

| 파일 | 역할 |
|------|------|
| `loop.go` | Agent Loop 실행기 |

**동작** (OpenClaw `docs/concepts/agent-loop.md` 참고):
1. 시스템 프롬프트 + 메시지 히스토리 + **도구 목록** → LLM `Chat()` 호출
2. LLM 응답에 `tool_calls` 있으면:
   - 각 도구를 Registry에서 찾아 실행
   - 결과를 `tool` role 메시지로 히스토리에 추가
   - LLM 재호출 (tool_calls 없을 때까지 반복, 최대 반복 횟수 제한)
3. 최종 `assistant` 텍스트 응답을 세션에 기록하고 반환
4. SSE 스트리밍: 도구 실행 중/완료 이벤트도 스트리밍

```
data: {"type":"tool_start","name":"exec","id":"call_1"}
data: {"type":"tool_result","id":"call_1","content":"..."}
data: {"type":"delta","text":"실행 결과는..."}
data: {"type":"done","usage":{...}}
```

#### 2-D. tarsd chat API에 agent loop 통합

**수정 파일**: `cmd/tarsd/main.go`
- chat 핸들러가 `agent.Loop`를 사용하도록 변경
- 도구 레지스트리 초기화 시 빌트인 도구 등록

#### 2-테스트

- `internal/tool/*_test.go` — 각 도구 단위 테스트
- `internal/agent/loop_test.go` — mock LLM으로 agent loop 테스트 (tool_calls → 실행 → 재호출)

---

### Phase 3: 허트비트 + 크론잡 자율 실행

**목표**: 허트비트와 크론잡이 agent loop를 사용해 도구를 자율적으로 실행

**마일스톤**: `tarsd --serve-api --run-loop`로 데몬이 주기적으로 HEARTBEAT.md를 읽고, AI가 판단하여 도구를 실행하고, 결과를 daily log에 기록하는 상태

**핵심 개념 (AI First):**
- HEARTBEAT.md는 자연어 지시서 — "이메일 확인", "캘린더 체크", "프로젝트 상태 확인" 같은 체크리스트
- 매 허트비트마다: HEARTBEAT.md 읽기 → agent loop (도구 포함) 실행 → AI가 자율 판단/실행/기록
- 할 일 없으면 `HEARTBEAT_OK` 응답 → 로그에 기록하지 않음
- 크론잡도 동일 방식: 자연어 설명의 작업을 AI가 agent loop로 자율 수행

#### 3-A. 허트비트 Agent Loop 통합

**수정 파일**: `internal/heartbeat/heartbeat.go`

- `RunOnceWithLLM` → `agent.Loop` 사용하도록 변경
- 도구 레지스트리를 허트비트에 주입
- `HEARTBEAT_OK` 응답 계약:
  - 응답 시작/끝에 `HEARTBEAT_OK` → ack로 처리, daily log 기록 스킵
  - 알림이 필요한 경우 → `HEARTBEAT_OK` 없이 텍스트만 반환
- `activeHours` 지원: 설정된 시간대 밖에서는 허트비트 스킵

**OpenClaw 참고:**
- `docs/gateway/heartbeat.md` — 허트비트 설정, 응답 계약, activeHours
- `docs/automation/cron-vs-heartbeat.md` — 허트비트 vs 크론 선택 기준

#### 3-B. 크론잡 매니저

**새 패키지**: `internal/cron/`

| 파일 | 역할 |
|------|------|
| `manager.go` | 크론잡 스케줄러 (robfig/cron 기반) |
| `job.go` | Job 구조체, 저장/로드 |
| `store.go` | `{workspace}/cron/jobs.json` 관리 |

**Job 구조:**
```go
type Job struct {
    ID          string
    Name        string
    Schedule    Schedule     // at(one-shot), every(interval), cron(expression)
    Prompt      string       // AI에게 전달할 자연어 지시
    Session     string       // "main" 또는 "isolated"
    AgentID     string
    Enabled     bool
    DeleteAfterRun bool
}
```

**실행 방식** (AI First):
1. 스케줄 시각 도달 → Job의 `Prompt`를 agent loop에 전달
2. AI가 도구를 사용해 자율적으로 작업 수행
3. 결과를 daily log에 기록
4. `session: "isolated"` → 별도 세션, `session: "main"` → 메인 세션의 허트비트에 시스템 이벤트로 주입

**tarsd API:**
```
GET    /v1/cron/jobs           # 크론잡 목록
POST   /v1/cron/jobs           # 크론잡 추가
PUT    /v1/cron/jobs/{id}      # 크론잡 수정
DELETE /v1/cron/jobs/{id}      # 크론잡 삭제
POST   /v1/cron/jobs/{id}/run  # 크론잡 즉시 실행
```

**tars CLI 슬래시 명령:**
- `/cron list` → 크론잡 목록
- `/cron add "매일 아침 9시 뉴스 요약" --cron "0 9 * * *"` → 크론잡 추가
- `/cron run {id}` → 즉시 실행

**OpenClaw 참고:**
- `docs/automation/cron-jobs.md` — 크론잡 개념, 스케줄 종류, 실행 방식
- `docs/automation/cron-vs-heartbeat.md` — 사용 시나리오 선택 가이드

#### 3-C. 데몬 모드 통합

**수정 파일**: `cmd/tarsd/main.go`

- `--serve-api` + `--run-loop` 동시 실행 지원 (현재는 상호 배타적)
- HTTP API 서버 + 허트비트 루프 + 크론잡 스케줄러를 고루틴으로 병렬 실행
- graceful shutdown 시 모두 정리

#### 3-테스트

- `internal/heartbeat/heartbeat_test.go` — agent loop 통합, HEARTBEAT_OK 테스트
- `internal/cron/manager_test.go` — 스케줄 파싱, 실행 트리거
- `cmd/tarsd/main_test.go` — 데몬 모드 통합 테스트

---

### Phase 4: 스킬 시스템

**목표**: SKILL.md 기반 스킬 로드 → 시스템 프롬프트 주입 → 슬래시 명령 지원

**마일스톤**: `{workspace}/skills/weather/SKILL.md` 생성 후 `/weather` 슬래시 명령으로 스킬 활성화

#### 4-A. 스킬 로더 + 레지스트리

**새 패키지**: `internal/skill/`

| 파일 | 역할 |
|------|------|
| `loader.go` | SKILL.md YAML frontmatter 파싱 |
| `registry.go` | 스킬 레지스트리 (로드 우선순위 적용) |
| `prompt.go` | 스킬 목록을 시스템 프롬프트용 XML로 포맷 |

**SKILL.md 형식** (OpenClaw AgentSkills 호환):
```markdown
---
name: weather
description: 날씨 정보를 조회합니다
user-invocable: true
---
# Weather Skill
[스킬 사용 가이드...]
```

**로드 우선순위** (OpenClaw 동일):
1. `{workspace}/skills/` (최고)
2. `~/.tarsncase/skills/` (사용자 글로벌)
3. 번들 스킬 (내장)

**시스템 프롬프트 주입**: 로드된 스킬 목록을 XML로 시스템 프롬프트에 추가:
```xml
<skills>
  <skill><name>weather</name><description>날씨 조회</description></skill>
</skills>
```

**OpenClaw 참고:**
- `docs/tools/skills.md` — 스킬 형식, 로드 위치, 우선순위
- `docs/tools/creating-skills.md` — 스킬 작성법
- `docs/tools/skills-config.md` — 스킬 설정
- `docs/tools/slash-commands.md` — 슬래시 명령 라우팅

#### 4-B. tarsd API + tars CLI

```
GET /v1/skills              # 스킬 목록
GET /v1/skills/{name}       # 스킬 상세
```

- `/skills` → 스킬 목록
- `/{skill-name}` → 스킬 내용을 채팅 컨텍스트에 주입 후 실행

#### 4-테스트

- `internal/skill/loader_test.go` — YAML frontmatter 파싱
- `internal/skill/registry_test.go` — 로드 우선순위, 충돌 해결

---

### Phase 5: 플러그인 + MCP

**목표**: 외부 플러그인과 MCP 서버의 도구를 agent loop에 통합

**마일스톤**: MCP 서버 연결 → 도구 자동 등록 → 채팅에서 MCP 도구 사용 가능

#### 5-A. MCP 클라이언트

**새 패키지**: `internal/mcp/`

| 파일 | 역할 |
|------|------|
| `client.go` | MCP stdio/SSE 클라이언트 (JSON-RPC) |
| `tool_adapter.go` | MCP 도구 → internal Tool 변환 |

**동작:**
1. 설정 파일에서 MCP 서버 목록 로드
2. 각 서버에 stdio 연결 → `tools/list` RPC → 도구 목록
3. MCP 도구를 `internal/tool.Tool`로 변환 → 레지스트리에 등록
4. `tools/call` RPC로 도구 실행

**설정:**
```yaml
mcp:
  servers:
    - name: filesystem
      command: npx
      args: ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
    - name: brave-search
      command: npx
      args: ["-y", "@anthropic/brave-search-mcp"]
      env:
        BRAVE_API_KEY: "${BRAVE_API_KEY}"
```

#### 5-B. 플러그인 시스템

**새 패키지**: `internal/plugin/`

| 파일 | 역할 |
|------|------|
| `loader.go` | 플러그인 매니페스트 파싱, subprocess 실행 |
| `manifest.go` | `tarsncase.plugin.json` 스키마 |

**플러그인 구조:**
```
plugins/my-plugin/
  tarsncase.plugin.json   # {"name":"my-plugin","tools":["my_tool"]}
  main                    # 실행 바이너리 (stdin/stdout JSON-RPC)
  skills/
    my-skill/SKILL.md
```

**OpenClaw 참고:**
- `docs/tools/plugin.md` — 플러그인 발견, 우선순위, 런타임
- `docs/plugins/agent-tools.md` — 플러그인 도구 등록 패턴
- `docs/plugins/manifest.md` — 플러그인 매니페스트 스키마

#### 5-C. tarsd API + tars CLI

```
GET /v1/plugins            # 플러그인 목록
GET /v1/mcp/servers        # MCP 서버 목록 + 상태
GET /v1/mcp/tools          # MCP 도구 목록
```

- `/plugins` → 플러그인 목록
- `/mcp` → MCP 서버/도구 상태

#### 5-테스트

- `internal/mcp/client_test.go` — mock MCP 서버로 연결/도구목록/실행 테스트
- `internal/plugin/loader_test.go` — 매니페스트 파싱

---

### Phase 6: cased 감시 데몬

**목표**: tarsd 프로세스 안정성 보장

**마일스톤**: `cased`가 tarsd를 감시하고 비정상 종료 시 자동 재시작 (달성)

#### 6-A. 프로세스 감시

**새 패키지**: `internal/sentinel/`

| 파일 | 역할 |
|------|------|
| `types.go` | sentinel 상태/이벤트/옵션 타입 |
| `supervisor.go` | 상태머신 + child process 라이프사이클 |
| `process.go` | child start/stop/wait |
| `health.go` | `/v1/healthz` probe |
| `policy.go` | backoff/cooldown 계산 |
| `api.go` | `/v1/sentinel/*` HTTP 핸들러 |

#### 6-B. cased 바이너리

**수정 파일**: `cmd/cased/main.go` (스켈레톤 → 실제 daemon)
- config load (`LoadCased`) + signal graceful shutdown
- sentinel supervisor 시작/종료
- cased API 서버 구동

#### 6-테스트

- `internal/sentinel/supervisor_test.go`
- `internal/sentinel/api_test.go`
- `cmd/cased/main_test.go`

---

### Phase 7: Gateway 기반 서브에이전트 런타임 (진행 중)

**목표**: 기본 in-process agent loop 위에 agent executor를 추가해 비동기 run 기반 멀티에이전트 실행 경로를 완성

**현재 상태 (2026-02-17):**
- `internal/gateway` 런타임과 run registry 구현 (`accepted + run_id` 비동기 계약)
- API 구현: `/v1/agent/*`, `/v1/gateway/*`, `/v1/channels/*`
- 도구 구현: `sessions_*`, `agents_list`, `message`, `browser`, `nodes`, `gateway`
- UI 명령 구현: `/agents`, `/spawn`, `/runs`, `/run`, `/cancel-run`, `/gateway`, `/channels`
- markdown 서브에이전트 `tools_allow` allowlist 정책 적용 완료(하드 차단)

**다음 작업:**
1. ~~서브에이전트 정책을 allowlist MVP에서 그룹/정규식/세션 라우팅 정책으로 확장~~ (2026-02-18 완료)
2. ~~run/channel 영속화 및 재시작 복구 정책 정리~~ (2026-02-17 완료)
3. 인증/권한/테넌트/멀티 워크스페이스의 최소 운영 모델을 설계하고 점진 적용

---

## 아키텍처 결정 사항

### Phase 9-Lite 경량 운영 원칙 (2026-02-18)

**결정**:
- `cased`는 코드베이스에서 제거한다(운영 감시는 systemd/launchd/docker로 위임).
- gateway 리포트는 summary 기본 ON, archive/detail 기본 OFF를 유지한다.
- API/UX는 additive only로 확장하고 기존 `/spawn /runs /gateway` 계약을 유지한다.

**성능 가드레일**:
- summary 조회는 in-memory 상태 기반으로 처리한다.
- archive가 꺼져 있으면 추가 디스크 write를 유발하지 않는다.
- sub-agent 정규식 정책은 에이전트 로드 시 1회 컴파일하고 요청 경로에서 재컴파일하지 않는다.

---

### tars-ui → cmd/tars 전환 (2026-02-18)

**현재 결정**: 전환 시작. `cmd/tars` MVP를 추가했고, 향후 `tars-ui`를 단계적으로 제거한다.

**단계**:
- 1단계: `cmd/tars` 채팅/세션 기본 경로 이식
- 2단계: runtime 명령(`/agents`, `/spawn`, `/runs`, `/gateway`) 이식
- 3단계: 기능 parity 달성 후 `tars-ui/` 제거 및 Node 툴체인 제거

---

## 4. OpenClaw 참고 지도 (빠른 색인)

| 기능 | OpenClaw 문서 경로 | 핵심 개념 |
|------|-------------------|-----------|
| 세션 관리 | `docs/concepts/session.md` | sessionKey, sessionId, DM 스코프 |
| 세션 저장 | `docs/reference/session-management-compaction.md` | sessions.json + JSONL 2계층 |
| 시스템 프롬프트 | `docs/concepts/system-prompt.md` | 프롬프트 섹션 구조, 부트스트랩 파일 주입 |
| 에이전트 런타임 | `docs/concepts/agent.md` | 워크스페이스 계약, 부트스트랩 파일 목록 |
| 워크스페이스 | `docs/concepts/agent-workspace.md` | 파일 맵, 각 파일의 역할 |
| Agent Loop | `docs/concepts/agent-loop.md` | 진입점 → 컨텍스트 조립 → 추론 → 도구 실행 → 스트리밍 → 저장 |
| 도구 표면 | `docs/tools/index.md` | allow/deny, 프로파일, 그룹 |
| Exec 도구 | `docs/tools/exec.md` | 셸 실행, 파라미터, 보안 모드 |
| 웹 도구 | `docs/tools/web.md` | web_search (Brave/Perplexity), web_fetch |
| 브라우저 | `docs/tools/browser.md` | 관리형 Chrome, 프로파일, 액션 |
| 메모리 | `docs/concepts/memory.md` | 3계층, 자동 flush, 벡터 검색 |
| 컴팩션 | `docs/concepts/compaction.md` | 자동/수동 압축, pre-compaction flush |
| 허트비트 | `docs/gateway/heartbeat.md` | 주기, HEARTBEAT_OK, activeHours |
| 크론잡 | `docs/automation/cron-jobs.md` | 스케줄 종류, 메인/격리 세션, 페이로드 |
| 크론 vs 허트비트 | `docs/automation/cron-vs-heartbeat.md` | 선택 기준표 |
| 훅 | `docs/automation/hooks.md` | 이벤트 드리븐 자동화 |
| 스킬 | `docs/tools/skills.md` | SKILL.md, 로드 위치, 우선순위 |
| 슬래시 명령 | `docs/tools/slash-commands.md` | 커맨드 vs 디렉티브, 인라인 단축키 |
| 플러그인 | `docs/tools/plugin.md` | 발견, 우선순위, 런타임 |
| 플러그인 도구 | `docs/plugins/agent-tools.md` | 도구 등록 패턴, optional 도구 |
| Pi 통합 | `docs/concepts/pi-integration.md` (영문) | Tool Architecture, SessionManager, Agent Loop 내부 |
| 워크스페이스 템플릿 | `docs/reference/templates/` | SOUL.md, USER.md, IDENTITY.md, TOOLS.md, HEARTBEAT.md 기본 내용 |

> **OpenClaw 소스 분석**: `https://github.com/openclaw/openclaw` — Repomix MCP `pack_remote_repository`로 분석. Go 소스가 아닌 TypeScript이므로 개념/구조만 참고하고 구현은 Go 관용구로 독립 작성.

---

## 5. 구현 순서 요약

```
Phase 1: LLM 채팅 (세션 + 프롬프트 + SSE + 슬래시 명령 + 컴팩션)
  ├── 1-A: 워크스페이스 부트스트랩 파일 확장
  ├── 1-B: 세션 관리 (JSONL, 토큰 기반 로딩)
  ├── 1-C: 시스템 프롬프트 조립
  ├── 1-D: LLM Chat API (messages 배열 + SSE 스트리밍)
  ├── 1-E: tarsd 채팅 API
  ├── 1-F: tarsd 세션 관리 API + 슬래시 명령
  ├── 1-G: 컨텍스트 압축
  └── 1-H: tars-ui 단일 클라이언트
      ↓
Phase 2: 빌트인 도구 + Agent Loop
  ├── 2-A: 도구 인터페이스 + 레지스트리
  ├── 2-B: 빌트인 도구 (exec, read, write, edit, web, memory)
  ├── 2-C: Agent Loop
  └── 2-D: chat API에 agent loop 통합
      ↓
Phase 3: 허트비트 + 크론잡 자율 실행
  ├── 3-A: 허트비트 Agent Loop 통합 (AI First)
  ├── 3-B: 크론잡 매니저
  └── 3-C: 데몬 모드 통합 (API + 루프 + 크론 병렬)
      ↓
Phase 4: 스킬 시스템
  ├── 4-A: 스킬 로더 + 레지스트리
  └── 4-B: tarsd API + tars 슬래시 명령
      ↓
Phase 5: 플러그인 + MCP
  ├── 5-A: MCP 클라이언트
  ├── 5-B: 플러그인 시스템
  └── 5-C: tarsd API + tars CLI
      ↓
Phase 6: cased 감시 데몬
      ↓
Phase 7: Gateway 기반 서브에이전트 런타임
```

각 Phase의 각 서브태스크마다 **TDD 사이클**: 실패 테스트 → 구현 → 통과 → 커밋
