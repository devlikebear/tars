# Go AI 에이전트 런타임 만들기 — 로드맵

> TARS 프로젝트를 기반으로 단계적으로 기능을 확장하는 전체 학습 계획

## 전체 페이즈 구조

```
Phase 1 ✅  최소 동작 버전 (MVP)
Phase 2 ✅  실전 LLM 연동 + 실용 도구
Phase 3 ✅  메모리와 컨텍스트 관리
Phase 4 ✅  확장 시스템 (Skill/Plugin/MCP)
Phase 5 ✅  콘솔/CLI 사용 표면
Phase 6 ✅  운영 자동화와 승인 흐름
Phase 7 ✅  인증과 비동기 Agent Runtime
```

각 페이즈는 독립적으로 학습 가능하지만, 순서대로 진행하는 것을 권장합니다.

## Phase 1 — 최소 동작 버전 (MVP) ✅

> 목표: Mock LLM으로 전체 파이프라인이 동작하는 최소 서버

| Step | 주제 | 상태 |
|------|------|------|
| 1 | 얇은 CLI (Cobra) | ✅ |
| 2 | 세션 + Transcript (JSONL) | ✅ |
| 3 | 프롬프트 빌더 + 도구 Registry | ✅ |
| 4 | HTTP/SSE 채팅 + Agent Loop | ✅ |

**결과물:** `go run ./cmd/tars/ serve`로 채팅 서버가 동작

## Phase 2 — 실전 LLM 연동 + 실용 도구 ✅

> 목표: 실제 LLM API를 연결하고, 실용적인 도구를 추가

| Step | 주제 | 상태 |
|------|------|------|
| 5 | OpenAI 호환 Provider Adapter | ✅ |
| 6 | 설정 시스템 (Config) | ✅ |
| 7 | 실용 도구 (read_file, write_file, exec) | ✅ |
| 8 | Anthropic Provider Adapter | ✅ |

**결과물:** OpenAI/Anthropic/Gemini/Claude Code CLI 계열 provider와 파일/명령 도구 사용 가능

## Phase 3 — 메모리와 컨텍스트 관리 ✅

> 목표: 대화가 길어져도 맥락을 유지하는 메모리 시스템

| Step | 주제 | 상태 |
|------|------|------|
| 9 | Transcript Compaction | ✅ |
| 10 | 파일 기반 Memory (save/search) | ✅ |
| 11 | Semantic Memory (Embedding) | ✅ |

**결과물:** 토큰 예산 기반 자동 압축 + Markdown memory + 경험/일별 로그 + optional embedding 검색

## Phase 4 — 확장 시스템 (Skill/Plugin/MCP) ✅

> 목표: 외부 기능을 동적으로 로딩하는 확장 구조

| Step | 주제 | 상태 |
|------|------|------|
| 12 | Skill 로더 (SKILL.md 파싱 + 프롬프트 주입) | ✅ |
| 13 | Plugin 로더 + MCP 클라이언트 (JSON-RPC stdio) | ✅ |
| 14 | Skill Hub (원격 검색/설치/업데이트) | ✅ |

**결과물:** SKILL.md 자동 인식 + MCP 서버 도구 연동 + 원격 skill/plugin/mcp 설치 CLI

## Phase 5 — 콘솔/CLI 사용 표면 ✅

> 과거 `tars chat` 대화형 TUI, `internal/project`, macOS dashboard는 제거되었습니다. 최신 TARS는 웹 콘솔(`/console`)과 root one-shot CLI를 사용자-facing 표면으로 둡니다.

### Step 15. One-shot CLI와 웹 콘솔

서버의 `/v1/chat` 엔드포인트에 연결하는 최소 CLI 경로와, 실제 일상 사용 표면인 웹 콘솔을 다룹니다.

- `tars` root 명령: 기본 브라우저로 `/console` 열기
- `tars --message "..."`: 한 번 메시지를 보내고 SSE 응답 출력
- `/console`: Work(Chat, Memory, System Prompt, Extensions), Operate(Agent Runtime, Approvals, Pulse, Reflection), Setup(Settings)

**체크포인트:**
- [x] `tars`가 콘솔 URL을 열고 출력한다
- [x] `tars --message`가 `/v1/chat` SSE 응답을 출력한다
- [x] 콘솔 Chat 화면에서 세션 기반 대화가 이어진다

## Phase 6 — 운영 자동화와 승인 흐름 ✅

> 과거 Autopilot/project dashboard 학습 흐름은 제거되었습니다. 현재 운영 자동화는 cron, ops approval, event stream, pulse/reflection으로 구성됩니다.

### Step 19. Ops Approval

위험하거나 파괴적인 운영 작업은 계획을 먼저 만들고, approval ID를 통해 명시적으로 승인합니다.
CON-041은 Approvals를 유지하는 것으로 결정했으며, Pulse의 안전한 allowlist autofix와 더 위험한 approval queue 라우팅을 구분합니다. 자세한 결정 기록은 `docs/decisions/approvals-workflow.md`를 참조하세요.

- `POST /v1/ops/cleanup/plan`: 삭제 후보와 approval ID 생성
- `GET /v1/ops/approvals`: pending approval 목록
- `POST /v1/ops/approvals/{id}/approve`: 승인 후 cleanup 적용
- `POST /v1/ops/approvals/{id}/reject`: 거절
- `tars approve list|run|reject`: CLI에서 같은 흐름 실행

**체크포인트:**
- [x] cleanup plan이 바로 삭제하지 않고 approval을 요구한다
- [x] 승인/거절 결과가 ops event로 남는다
- [x] CLI와 콘솔이 같은 approval API를 사용한다

### Step 20. Runtime Event Stream

콘솔은 폴링만으로 운영 상태를 맞추지 않고, `/v1/events/stream`으로 runtime notification을 받습니다.

- `GET /v1/events/stream`: SSE 실시간 이벤트
- `GET /v1/events/history?limit=N`: 최근 이벤트와 unread count
- `POST /v1/events/read`: 읽음 cursor 갱신
- 이벤트 category: `cron`, `ops`, `pulse`, `watchdog`, `usage`, `system`

**체크포인트:**
- [x] 이벤트 스트림이 keepalive와 notification payload를 전송한다
- [x] 이벤트 히스토리가 role별 read cursor를 유지한다
- [x] cron/ops/pulse가 같은 notification pipeline으로 합류한다

## Phase 7 — 인증과 비동기 Agent Runtime ✅

### Step 21. 인증 미들웨어

- Bearer token 인증
- user/admin role 분리
- `off`, `required`, `external-required` 모드
- loopback/dev surface 예외와 admin path 강제 보호

### Step 22. Agent Runtime

- Run lifecycle: `accepted → running → completed/failed/canceled`
- PromptExecutor + CommandExecutor + workspace agent executor
- run/channel persistence under `workspace/_shared/agentruntime/`
- `/v1/agentruntime/runs` API와 `/v1/agentruntime/status|reload|restart|reports/*`
- `/v1/agent/runs`는 legacy alias, `/v1/agent/agents`는 현재 agent list endpoint

## 페이즈별 난이도와 예상 학습 시간

| Phase | 난이도 | 예상 시간 | 핵심 키워드 |
|-------|--------|-----------|-------------|
| 1 | ★☆☆☆☆ | 2-3시간 | CLI, JSONL, SSE, Agent Loop |
| 2 | ★★☆☆☆ | 4-6시간 | API 연동, Config, Provider Adapter |
| 3 | ★★★☆☆ | 4-6시간 | Compaction, Embedding, Memory |
| 4 | ★★★☆☆ | 6-8시간 | Skill/Plugin/MCP, 동적 로딩 |
| 5 | ★★☆☆☆ | 2-4시간 | Console, one-shot CLI |
| 6 | ★★★☆☆ | 4-6시간 | Ops approval, Event stream, Pulse, Reflection |
| 7 | ★★★★☆ | 6-8시간 | Auth, async runs, Agent Runtime |

## 원칙

1. **각 페이즈가 끝나면 동작하는 상태여야 한다** — 중간에 깨지지 않게
2. **확장 레이어는 core를 깨지 않고 optional로 붙인다** — Phase 1이 항상 동작
3. **원본 코드를 먼저 읽고, 최소 버전을 구현한 뒤, 점진적으로 원본 수준으로 올린다**
4. **테스트를 먼저 작성하고 구현한다** — 각 Step마다 검증 포인트 확인
5. **종료 조건을 먼저 확인하고 작업한다** — 범위를 벗어나는 작업은 다음 Phase로 미룬다

## 제거된 레거시 흐름

다음 항목은 과거 문서/브랜치의 학습 흔적이며 최신 TARS 본체 기준으로는 활성 기능이 아닙니다.

- `internal/project/*`, project briefs, Kanban dashboard
- macOS Swift dashboard
- Autopilot 상태 머신과 deliverables executor
- Bubble Tea 기반 `tars chat` TUI
- user-facing KB Wiki/graph surface
