# Go AI 에이전트 런타임 만들기 — 로드맵

> TARS 프로젝트를 기반으로 단계적으로 기능을 확장하는 전체 학습 계획

---

## 전체 페이즈 구조

```
Phase 1 ✅  최소 동작 버전 (MVP)
Phase 2 ✅  실전 LLM 연동 + 실용 도구
Phase 3 ✅  메모리와 컨텍스트 관리
Phase 4 ✅  확장 시스템 (Skill/Plugin/MCP)
Phase 5 ✅  채팅 인터페이스 + 프로젝트 기반
Phase 6 ✅  자율 실행 + 감독 대시보드
Phase 7 ✅  외부 경계와 배포
```

각 페이즈는 독립적으로 학습 가능하지만, 순서대로 진행하는 것을 권장합니다.

---

## Phase 1 — 최소 동작 버전 (MVP) ✅ 완료

> 목표: Mock LLM으로 전체 파이프라인이 동작하는 최소 서버

| Step | 주제 | 상태 |
|------|------|------|
| 1 | 얇은 CLI (Cobra) | ✅ |
| 2 | 세션 + Transcript (JSONL) | ✅ |
| 3 | 프롬프트 빌더 + 도구 Registry | ✅ |
| 4 | HTTP/SSE 채팅 + Agent Loop | ✅ |

**결과물:** `go run ./cmd/tars/ serve`로 Mock 채팅 서버가 동작

---

## Phase 2 — 실전 LLM 연동 + 실용 도구 ✅ 완료

> 목표: 실제 LLM API를 연결하고, 실용적인 도구를 추가

| Step | 주제 | 상태 |
|------|------|------|
| 5 | OpenAI 호환 Provider Adapter | ✅ |
| 6 | 설정 시스템 (Config) | ✅ |
| 7 | 실용 도구 (read_file, write_file, exec) | ✅ |
| 8 | Anthropic Provider Adapter | ✅ |

**결과물:** OpenAI/Anthropic 실제 LLM으로 채팅 + 파일/명령 도구 사용 가능

---

## Phase 3 — 메모리와 컨텍스트 관리 ✅ 완료

> 목표: 대화가 길어져도 맥락을 유지하는 메모리 시스템

| Step | 주제 | 상태 |
|------|------|------|
| 9 | Transcript Compaction | ✅ |
| 10 | 파일 기반 Memory (save/search) | ✅ |
| 11 | Semantic Memory (Embedding) | ✅ |

**결과물:** 토큰 예산 기반 자동 압축 + Markdown 메모리 저장/검색 + Embedding 유사도 검색 (optional)

---

## Phase 4 — 확장 시스템 (Skill/Plugin/MCP) ✅ 완료

> 목표: 외부 기능을 동적으로 로딩하는 확장 구조

| Step | 주제 | 상태 |
|------|------|------|
| 12 | Skill 로더 (SKILL.md 파싱 + 프롬프트 주입) | ✅ |
| 13 | Plugin 로더 + MCP 클라이언트 (JSON-RPC stdio) | ✅ |
| 14 | Skill Hub (원격 검색/설치/업데이트) | ✅ |

**결과물:** SKILL.md 자동 인식 + MCP 서버 도구 연동 + 원격 skill 설치 CLI

---

## Phase 5 — 채팅 인터페이스 + 프로젝트 기반

> 목표: **사용자가 자연어로 AI와 대화하고, 대화를 통해 프로젝트를 생성/관리할 수 있다**
>
> 종료 조건: 터미널에서 "소나기 스타일 단편소설 프로젝트 만들어줘"라고 치면 프로젝트가 생성되고, 대시보드에서 확인된다.

### Step 15. 터미널 채팅 클라이언트

서버의 `/v1/chat` 엔드포인트에 연결하는 대화형 터미널 클라이언트.

- stdin/stdout 기반 REPL (최소 버전)
- SSE 스트리밍 응답 표시
- 세션 유지 (세션 ID 기반)

**범위 제한:** Bubble Tea 같은 TUI 프레임워크는 사용하지 않음. 순수 Go `fmt` + `bufio`로 충분.

**체크포인트:**
- [x] `tars chat`으로 서버에 연결, 대화가 가능하다
- [x] LLM 응답이 스트리밍으로 한 글자씩 출력된다
- [x] 도구 호출 결과가 대화에 반영된다

### Step 16. 프로젝트 Store + Kanban ✅ 구현됨

워크스페이스에 프로젝트를 만들고 태스크를 관리하는 저장소.

- `PROJECT.md` (YAML frontmatter), `KANBAN.md`, `ACTIVITY.jsonl`
- 태스크 상태: `todo → in_progress → review → done`
- REST API: CRUD + board + activity

**범위 제한:** 상태 머신/자율 실행 없음. 순수 CRUD만.

**구현 파일:**
- `internal/project/store.go`
- `internal/server/handler_dashboard.go`

**체크포인트:**
- [x] API로 프로젝트 생성/조회/수정이 가능하다
- [x] 태스크를 추가하고 상태를 변경할 수 있다
- [x] 채팅에서 자연어로 프로젝트를 생성할 수 있다 (← Step 15 이후)

### Step 17. macOS 대시보드 ✅ 구현됨

프로젝트 상태를 확인하는 macOS 메뉴바 앱.

- SwiftUI MenuBarExtra (.window 스타일)
- 서버 연결/해제, 프로젝트 목록, 상세 화면
- 프로젝트 생성/태스크 관리 UI

**범위 제한:** 읽기 + 기본 CRUD만. 채팅 패널과 자율 실행 감독은 Phase 6.

**구현 파일:**
- `dashboard/Sources/*.swift`

**체크포인트:**
- [x] 메뉴바에서 서버 연결 후 프로젝트 목록이 보인다
- [x] 프로젝트 상세에서 보드/활동이 표시된다
- [x] 대시보드에서 프로젝트를 생성하고 태스크를 편집할 수 있다

---

## Phase 6 — 자율 실행 + 감독 대시보드

> 목표: **AI가 프로젝트를 자율적으로 계획/실행하고, 사용자가 대시보드에서 승인/감독한다**
>
> 종료 조건: 프로젝트에 Autopilot Start → LLM이 태스크+산출물 계획 → 사용자 승인 → 자동 실행 → output/ 디렉토리에 산출물 파일이 생성된다.

### Step 18. 상태 머신 + Autopilot ✅ 구현됨

프로젝트 phase 기반 상태 머신과 자율 실행 루프.

- Phase: `planning → awaiting_approval → executing → reviewing → completed`
- LLM 콜백: PlanGenerator (태스크+산출물 생성), TaskRunner (태스크 실행), GoalEvaluator (목표 평가)
- 실패 재시도 (최대 3회), 대기 상태 루프 최적화

**구현 파일:**
- `internal/project/autopilot.go`
- `internal/server/server.go` (LLM 콜백 구현)

**체크포인트:**
- [x] autopilot이 planning → awaiting_approval로 전이한다
- [x] 승인 후 executing에서 태스크를 자동 실행한다
- [x] 3회 연속 실패 시 태스크를 스킵한다
- [x] reviewing에서 목표 미달 시 re-planning이 동작한다

### Step 19. Human-in-the-loop + 산출물 ✅ 구현됨

사용자 승인 게이트와 산출물 명세 시스템.

- Approve/Reject/Pause/Resume/Cancel API
- Deliverables: 계획 단계에서 산출물 파일명/포맷/설명을 정의
- TaskRunner에 산출물 경로를 프롬프트로 전달 → `write_file`로 output/ 에 저장
- `DELIVERABLES.json` 영속화

**구현 파일:**
- `internal/project/store.go` (Deliverable 타입, SaveDeliverables/GetDeliverables)
- `internal/server/handler_dashboard.go` (approve/reject/pause/resume/cancel 엔드포인트)

**체크포인트:**
- [x] 대시보드에서 Approve/Reject 버튼이 표시된다
- [x] 승인 화면에서 산출물 명세를 확인할 수 있다
- [ ] 실행 완료 후 output/ 디렉토리에 산출물 파일이 존재한다
- [x] Reject 시 태스크가 초기화되고 re-planning이 진행된다

### Step 20. SSE 실시간 이벤트 ✅ 구현됨

서버 → 대시보드 실시간 이벤트 스트리밍.

- Go SSEBroker: Subscribe/Unsubscribe/Broadcast 패턴
- 이벤트: phase_changed, board_updated, activity, autopilot_status
- Swift SSE 클라이언트: 전용 URLSession (긴 타임아웃), 자동 재연결

**구현 파일:**
- `internal/server/sse_broker.go`
- `dashboard/Sources/APIClient.swift` (listenSSE)

**체크포인트:**
- [x] 서버 이벤트가 대시보드에 실시간 반영된다
- [x] 연결 끊김 시 자동 재연결된다
- [x] SSE 연결이 60초 이상 안정적으로 유지된다

---

## Phase 7 — 외부 경계와 배포

> 목표: **실제 운영 환경에서 안전하게 쓸 수 있는 수준으로 강화**
>
> 종료 조건: 인증된 사용자만 API에 접근, 장시간 작업을 비동기로 실행, 터미널 TUI로 서버와 대화가 가능하다.

### Step 21. 인증 미들웨어 ✅ 구현됨

- Bearer token 인증 (SHA-256 + constant-time compare)
- 경로별 권한 분기 (admin path, skip path)

**구현 파일:**
- `internal/auth/middleware.go`
- `internal/config/config.go` (AuthMode, AuthToken)

### Step 22. Gateway (비동기 실행) ✅ 구현됨

- Run lifecycle: `accepted → running → completed/failed/canceled`
- PromptExecutor (agent.Loop 래핑)
- 동시 실행 제한, Cancel/Wait, 채널 메시지 (local)

**구현 파일:**
- `internal/gateway/runtime.go`
- `internal/gateway/executor.go`
- `internal/gateway/handler.go`

### Step 23. TUI 클라이언트 ✅ 구현됨

- Bubble Tea 기반 TUI (Model-Update-View)
- SSE 스트리밍 + 비동기 채널 패턴
- 스크롤, 히스토리, 취소 기능

**구현 파일:**
- `internal/tui/model.go`, `update.go`, `view.go`, `client.go`
- `cmd/tars/chat.go`

---

## 페이즈별 난이도와 예상 학습 시간

| Phase | 난이도 | 예상 시간 | 핵심 키워드 |
|-------|--------|-----------|-------------|
| 1 ✅ | ★☆☆☆☆ | 2-3시간 | CLI, JSONL, SSE, Agent Loop |
| 2 ✅ | ★★☆☆☆ | 4-6시간 | 실제 API 연동, Config, Provider Adapter |
| 3 ✅ | ★★★☆☆ | 4-6시간 | Compaction, Embedding, 토큰 관리 |
| 4 ✅ | ★★★☆☆ | 6-8시간 | Skill/Plugin/MCP, 동적 로딩 |
| 5 ✅ | ★★★☆☆ | 4-6시간 | 터미널 채팅, 프로젝트 CRUD, macOS 대시보드 |
| 6 ✅ | ★★★★☆ | 4-6시간 | 상태 머신, Autopilot, SSE, Human-in-the-loop |
| 7 ✅ | ★★★★★ | 6-8시간 | 인증, 비동기, TUI |

---

## 원칙

1. **각 페이즈가 끝나면 동작하는 상태여야 한다** — 중간에 깨지지 않게
2. **확장 레이어는 core를 깨지 않고 optional로 붙인다** — Phase 1이 항상 동작
3. **원본 코드를 먼저 읽고, 최소 버전을 구현한 뒤, 점진적으로 원본 수준으로 올린다**
4. **테스트를 먼저 작성하고 구현한다** — 각 Step마다 검증 포인트 확인
5. **종료 조건을 먼저 확인하고 작업한다** — 범위를 벗어나는 작업은 다음 Phase로 미룬다
