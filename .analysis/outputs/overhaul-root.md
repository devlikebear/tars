# 시스템 오버홀 지시서: TARS 저장소 전체
# System Overhaul Work Order: TARS Repository

> **생성일 / Created At**: 2026-03-15
> **세션 ID / Session ID**: `refactor-guide-20260315-084826`
> **체크포인트 / Checkpoint**: `checkpoint-001`
> **대상 / Target**: `internal/` 전체 (gateway, tarsserver, llm, agent, tool, project, config, browser, extensions, 주변 모듈)
> **분석 범위 / Analysis Scope**: 아키텍처 수준 BFS — ARCH/DEAD/OVER/DEBT 이슈 코드 (기존 DUP/SEC/TIDY 리팩터와 상호 보완)
> **우선순위 / Priority**: 높음
> **소스 / Source**: 5개 병렬 에이전트 심층 분석 결과 종합

---

## 요약 (Summary) / Summary

이 지시서는 기존 리팩터 지시서(DUP/SEC/TIDY 26건)와 **상호 보완** 관계에 있는 **아키텍처 수준** 오버홀 작업을 정의합니다.
코드 수준 중복·보안·정리는 기존 `refactor-root.md`가 담당하고, 이 문서는 **구조적 설계 결함(ARCH)**, **사장 코드(DEAD)**, **과잉 설계(OVER)**, **기술 부채(DEBT)** 를 다룹니다.

총 **30개 이슈**가 발견되었습니다 (`ARCH` 10건 / `DEAD` 5건 / `OVER` 6건 / `DEBT` 9건).
이 중 가장 영향 범위가 큰 것은 **Gateway Runtime god object**, **tarsserver 핸들러 계층 위반**, **browserrelay 사장 모듈(2,321줄)**, **프로젝트 모듈 에러 무시 14건** 입니다.

This work order complements the existing refactor guide (DUP/SEC/TIDY, 26 issues). It addresses **architectural design flaws (ARCH)**, **dead code (DEAD)**, **over-engineering (OVER)**, and **technical debt (DEBT)**.
30 issues total: `ARCH` 10 / `DEAD` 5 / `OVER` 6 / `DEBT` 9.

---

## 이슈 코드 범례 / Issue Code Legend

| 코드 | 의미 | 설명 |
|------|------|------|
| `ARCH` | Architectural | 계층 위반, 잘못된 추상화, 경계 누락, god object, 순환 의존 |
| `DEAD` | Dead Code | 사장 코드, 미사용 export, 폐기된 모듈/기능 |
| `OVER` | Over-Engineering | 불필요한 복잡도, 미사용 변형 타입, 과잉 추상화 |
| `DEBT` | Technical Debt | 누적된 단축 경로, 매직 넘버, 에러 무시, 문자열 기반 제어 흐름 |

---

## 발견된 이슈 목록 (Issues Found) / Issues Found

| # | 분류 코드 | 이슈 유형 | 상태 | 위치 | 근거 | 위험도 | 영향 범위 |
|---|-----------|-----------|------|------|------|--------|-----------|
| 1 | `ARCH-001` | Gateway Runtime god object | 미완료 | `gateway/types.go:191`, `runtime.go`, `runtime_*.go` 10개 파일 | 단일 struct에 60+ 메서드, 21 필드, 5개 관심사 혼재 | 높음 | Gateway 전체 |
| 2 | `ARCH-002` | Gateway 이중 API 패턴 | 미완료 | `gateway/runtime_channels.go`, `runtime_runs.go`, `runtime_reports.go` | `Method()` + `MethodByWorkspace()` 쌍으로 API 표면적 2배 | 중간 | Gateway API |
| 3 | `ARCH-003` | tarsserver 핸들러 계층 위반 | 미완료 | `tarsserver/handler_project.go:459`, `handler_chat_context.go:135,249` | 핸들러가 Store/Orchestrator를 직접 생성, 프레젠테이션 계층이 데이터 계층 작업 수행 | 높음 | API 핸들러 전체 |
| 4 | `ARCH-004` | tarsserver buildAPIMux god function | 미완료 | `tarsserver/main_serve_api.go:108-531` | 423줄 단일 함수에 저장소 생성, 핸들러 구축, 미들웨어 등록, 백그라운드 작업 시작 혼재 | 높음 | 서버 부트스트랩 |
| 5 | `ARCH-005` | Orchestrator 에러 14건 무시 | 미완료 | `project/orchestrator.go:146,185,189,203,207,228,260,271,281,290,299,362,397,447` | `appendSystemActivity()`, `appendAgentReport()` 에러 전부 `_ =`으로 무시 | 높음 | 프로젝트 감사 추적 |
| 6 | `ARCH-006` | 핸들러 에러 문자열 매칭 | 미완료 | `tarsserver/handler_project.go:104,309`, `handler_session.go:246` 등 10+ 위치 | `strings.Contains(err.Error(), "not found")` 패턴으로 에러 분류 | 중간 | API 에러 처리 |
| 7 | `ARCH-007` | Agent loop이 tool 인자를 자동 교정 | 미완료 | `agent/loop.go:26,118-122,318-336` | exec tool에 command 인자 없으면 `"pwd"` fallback 삽입 — tool 책임을 agent가 침범 | 중간 | Agent 루프 |
| 8 | `ARCH-008` | Project → Session 계층 역전 | 미완료 | `project/brief_state.go:9,234,273` | 도메인 모듈(project)이 인프라 모듈(session)을 직접 import | 낮음 | 프로젝트 brief |
| 9 | `ARCH-009` | Gateway 영속화 추상화 누락 | 미완료 | `gateway/persistence.go`, `gateway/runtime_persist.go` | snapshotStore가 JSON 전용, 인터페이스 없이 구현에 직결 | 중간 | Gateway 영속화 |
| 10 | `ARCH-010` | Workspace store 패턴 불일치 | 미완료 | `tarsserver/workspace_resolver.go`, `handler_project.go:169`, `handler_chat_context.go:135` | resolver 패턴과 직접 생성 패턴이 공존, 일관성 없음 | 낮음 | 서버 핸들러 |
| 11 | `DEAD-001` | browserrelay 모듈 사장 | 미완료 | `browserrelay/server.go` (1,508줄), `server_test.go` (813줄) | 어디서도 import 안 됨, 총 2,321줄 완전 사장 | 높음 | 저장소 전체 |
| 12 | `DEAD-002` | Gateway 내부 함수 중복 export | 미완료 | `gateway/types.go:223-237` | `DefaultWorkspaceID`/`defaultWorkspaceID`, `NormalizeWorkspaceID`/`normalizeWorkspaceID` 쌍 중복 | 낮음 | Gateway |
| 13 | `DEAD-003` | Agent HookFunc 미사용 | 미완료 | `agent/loop.go:44-48` | `HookFunc` 어댑터 타입 정의됐으나 호출자 없음 | 낮음 | Agent |
| 14 | `DEAD-004` | Project 경로 헬퍼 5개 불필요 export | 미완료 | `project/activity.go:68`, `brief_state.go:90,94,98`, `kanban.go:46` | `ActivityPath()` 등 5개 public 함수가 내부에서만 사용 | 낮음 | Project |
| 15 | `DEAD-005` | handler_chat.go 미사용 핸들러 변형 | 미완료 | `tarsserver/handler_chat.go:377-388` | 여러 `newChatAPIHandler*` 변형 중 일부만 실제 사용 | 낮음 | TarsServer |
| 16 | `OVER-001` | Gateway 브라우저 호환 레이어 (1:1 래핑) | 미완료 | `gateway/runtime_browser.go` | `browser.Service` 메서드를 1:1 래핑, 상태 업데이트 보일러플레이트 6+ 곳 반복 | 중간 | Gateway |
| 17 | `OVER-002` | Tool ContentBlock 변형 타입 미사용 | 미완료 | `tool/tool.go:13-38` | `ContentBlock{Type, Text}` 배열이지만 모든 구현이 `Type="text"`만 사용 | 중간 | Tool 레지스트리 |
| 18 | `OVER-003` | Provider models 커스텀 캐시 | 미완료 | `tarsserver/provider_models_cache.go` (186줄) | 커스텀 키 생성, 수동 JSON 영속화, mutex — 단순 TTL 캐시로 충분 | 낮음 | TarsServer |
| 19 | `OVER-004` | Gemini Native 3파일 분할 (708줄) | 미완료 | `llm/gemini_native.go`, `gemini_native_chat.go`, `gemini_native_convert.go` | OpenAI Compat (384줄 1파일) 대비 과잉 분할 | 낮음 | LLM |
| 20 | `OVER-005` | Gateway 아카이브 1000 파일 회전 | 미완료 | `gateway/runtime_archive.go:53-78` | 파일명 충돌 시 1000번까지 루프 — UUID 또는 타임스탬프로 충분 | 낮음 | Gateway |
| 21 | `OVER-006` | Extensions Manager 단일 파일 과부하 | 미완료 | `extensions/manager.go` (448줄) | 로딩, 감시, MCP 통합, 버전 발행이 한 struct에 혼재 | 낮음 | Extensions |
| 22 | `DEBT-001` | 매직 넘버 산재 (전역) | 미완료 | `gateway/runtime.go:18-36`, `agent/loop.go:24-25`, `prompt/memory_retrieval.go:16-22`, `prompt/bootstrap_sections.go:10-15`, `project/project_runner.go:41,63` | 기본값, 제한값, 토큰 예산이 코드 곳곳에 하드코딩 | 중간 | 전역 |
| 23 | `DEBT-002` | Gateway 세션 에러 무시 | 미완료 | `gateway/runtime_run_execute.go:87,88,216` | `_ = r.appendSessionMessage(...)` — 세션 저장 실패를 무시 | 중간 | Gateway |
| 24 | `DEBT-003` | 프로젝트 상태 매직 문자열 | 미완료 | `project/orchestrator.go:156,186,249-251`, `project/workflow_policy.go:170,176-183` | "todo", "in_progress", "review" 등이 enum 없이 문자열로 산재 | 중간 | Project |
| 25 | `DEBT-004` | 배치 태스크 디스패치 첫 에러만 보고 | 미완료 | `project/orchestrator.go:80-136` | 10개 태스크 실패 시 첫 번째 에러만 반환, 나머지 유실 | 중간 | Project |
| 26 | `DEBT-005` | Gateway 정책 tool 이름 문자열 파싱 | 미완료 | `gateway/runtime_run_execute.go:128-144` | 에러 메시지에서 `"tool not injected for this request:"` 접두사로 tool 이름 추출 | 중간 | Gateway |
| 27 | `DEBT-006` | 토큰 추정 원시적 (len/4) | 미완료 | `prompt/builder.go:161-169` | `tokens := len(content) / 4` — 모델별 토크나이저 미적용 | 낮음 | Prompt |
| 28 | `DEBT-007` | Tool 이름 별칭 하드코딩 맵 | 미완료 | `tool/tool_name.go:5-18` | 9개 별칭이 코드에 하드코딩, 외부 설정 불가 | 낮음 | Tool |
| 29 | `DEBT-008` | Browser 다중 프로필 미완성 | 미완료 | `browser/service_stub_runtime.go:248-278` | `resolveProfile()` 항상 "managed" 반환, `driverForProfile()` 항상 "playwright" — 미완성 아키텍처 | 낮음 | Browser |
| 30 | `DEBT-009` | Gateway 타임스탬프 포맷 반복 | 미완료 | `gateway/` 전체 (20+ 위치) | `r.nowFn().UTC().Format(time.RFC3339)` 헬퍼 없이 반복 | 낮음 | Gateway |

---

## 작업 지시서 (Work Orders) / Work Orders

---

### WO-OV-001: browserrelay 사장 모듈 제거

**분류 코드 / Classification Code**: `DEAD-001`
**유형 / Type**: Remove Dead Module
**심각도 / Severity**: 높음
**상태 / Status**: 미완료
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `internal/browserrelay/` 패키지가 저장소 전체에서 단 한 곳도 import되지 않음. `server.go` 1,508줄 + `server_test.go` 813줄 = 총 2,321줄 사장 코드.
**소스 이슈 / Source Issue**: `DEAD-001`
**영향 범위 / Impact Area**: 저장소 전체 (빌드 시간, 유지보수 부담)

**문제 설명 / Problem**
`browserrelay` 패키지는 브라우저 확장 CDP relay 서버를 구현하지만, 어디서도 import되지 않습니다.
코드가 남아 있으면 빌드 크기, 의존성 추적, 보안 리뷰 범위가 불필요하게 넓어집니다.

**지시 사항 / Instructions**
1. `git grep -r "browserrelay"` 로 참조가 없음을 최종 확인하십시오.
2. `internal/browserrelay/` 디렉토리 전체를 삭제하십시오.
3. 기존 리팩터 지시서(`refactor-root.md`)의 SEC-002(WO-006, 완료)와 TIDY-007(WO-011, 미완료)에서 browserrelay 관련 항목을 "삭제로 해소"로 표기하십시오.

**완료 기준 / Completion Criteria**
- [ ] `internal/browserrelay/` 디렉토리 삭제됨
- [ ] `go build ./...` 통과
- [ ] 기존 테스트 통과

**테스트 기준 / Test Criteria**
- [ ] 빌드 테스트: `go build ./...` 성공
- [ ] 회귀 테스트: `make test` 통과

---

### WO-OV-002: Gateway Runtime god object 분해

**분류 코드 / Classification Code**: `ARCH-001`
**유형 / Type**: Extract Struct / Introduce Facade
**심각도 / Severity**: 높음
**상태 / Status**: 미완료
**선행 작업 / Prerequisite**: 없음 (기존 TIDY-011/WO-021과 병합 가능)
**근거 / Evidence**: `Runtime` struct가 21개 필드, 60+ 메서드로 Run lifecycle, Channel messaging, Browser delegation, Persistence, Reporting, Node operations, Executor management를 모두 관리. 10개 `runtime_*.go` 파일에 걸쳐 2,491줄.
**소스 이슈 / Source Issue**: `ARCH-001`, `ARCH-002`
**영향 범위 / Impact Area**: `internal/gateway/` 전체

**문제 설명 / Problem**
`Runtime`은 전형적인 god object입니다. 메서드 수가 60+이며, `MethodByWorkspace()` 변형 때문에 API 표면적이 2배로 늘어납니다.
테스트(`runtime_test.go` 851줄)도 단일 파일에 집중되어 있어 수정 시 영향 범위 파악이 어렵습니다.

**지시 사항 / Instructions**
1. **1단계**: `RunManager` sub-struct를 추출하여 Run 생성/조회/취소/대기 메서드를 이동하십시오 (`runtime_runs.go`, `runtime_run_bootstrap.go`, `runtime_run_execute.go`).
2. **2단계**: `ChannelManager` sub-struct를 추출하여 메시지 송수신 메서드를 이동하십시오 (`runtime_channels.go`).
3. **3단계**: `BrowserDelegate`를 추출하여 1:1 래핑 메서드를 composition으로 교체하십시오 (`runtime_browser.go`).
4. **4단계**: `ByWorkspace` 변형을 workspace ID를 매개변수로 받는 단일 메서드로 통합하십시오 (기본값 fallback 유지).
5. `Runtime`은 이 sub-struct들을 합성하는 facade로 유지하되, 직접 필드는 12개 이하로 줄이십시오.
6. 각 단계가 `go build` 통과하는 크기로 유지하십시오.

**완료 기준 / Completion Criteria**
- [ ] Runtime struct 직접 필드 12개 이하
- [ ] `ByWorkspace` 변형 메서드 제거, workspace ID 매개변수 방식으로 통합
- [ ] 각 sub-struct가 단일 관심사만 포함
- [ ] 기존 테스트 통과

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: 각 sub-struct 독립 테스트
- [ ] 통합 테스트: runtime_test.go 전체 통과
- [ ] 회귀 테스트: gateway handler 호출 경로 동작 유지

---

### WO-OV-003: tarsserver 핸들러 계층 위반 해소

**분류 코드 / Classification Code**: `ARCH-003`, `ARCH-010`
**유형 / Type**: Extract Service / Dependency Injection
**심각도 / Severity**: 높음
**상태 / Status**: 미완료
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `handler_project.go:459`에서 `project.NewOrchestratorWithGitHubAuthChecker()`를 핸들러 내부에서 직접 호출. `handler_chat_context.go:135,249`에서 `project.NewStore()`를 직접 생성. `workspace_resolver.go`의 resolver 패턴과 혼재.
**소스 이슈 / Source Issue**: `ARCH-003`, `ARCH-010`
**영향 범위 / Impact Area**: `internal/tarsserver/` API 핸들러 전체

**문제 설명 / Problem**
HTTP 핸들러(프레젠테이션 계층)가 Store와 Orchestrator를 직접 생성하여 데이터 계층 작업을 수행합니다.
이는 핸들러 테스트를 어렵게 만들고, Store 생성 방식이 변경되면 모든 핸들러를 수정해야 합니다.

**지시 사항 / Instructions**
1. `workspace_resolver.go`의 resolver 패턴을 확장하여 `projectStoreResolver`, `orchestratorResolver`를 추가하십시오.
2. 핸들러가 resolver를 통해 Store/Orchestrator를 받도록 변경하십시오.
3. 핸들러 내부의 `project.NewStore()`, `project.NewOrchestrator*()` 직접 호출을 제거하십시오.
4. `buildAPIMux`에서 resolver를 한 번 구성하고 핸들러에 주입하십시오.

**완료 기준 / Completion Criteria**
- [ ] 핸들러 내부에서 Store/Orchestrator 직접 생성 0건
- [ ] Store 접근이 resolver 패턴으로 통일됨
- [ ] 기존 API 동작 유지

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: resolver 기반 Store/Orchestrator 해석 검증
- [ ] 통합 테스트: 프로젝트 CRUD API 동작 검증
- [ ] 회귀 테스트: main_test.go 통과

---

### WO-OV-004: buildAPIMux 함수 분할

**분류 코드 / Classification Code**: `ARCH-004`
**유형 / Type**: Extract Function / Split Responsibility
**심각도 / Severity**: 높음
**상태 / Status**: 미완료
**선행 작업 / Prerequisite**: WO-OV-003 (핸들러 계층 정리 후)
**근거 / Evidence**: `main_serve_api.go:108-531`의 `buildAPIMux`가 423줄로 저장소 생성, 도구 구축, 핸들러 생성, 미들웨어 등록, 백그라운드 작업 시작을 하나의 함수에서 수행.
**소스 이슈 / Source Issue**: `ARCH-004`
**영향 범위 / Impact Area**: 서버 부트스트랩

**문제 설명 / Problem**
단일 함수가 HTTP 라우팅, 의존성 조립, 백그라운드 작업 시작이라는 세 가지 관심사를 혼재합니다.
수정 시 영향 범위를 예측하기 어렵고, 테스트 격리가 불가능합니다.

**지시 사항 / Instructions**
1. 저장소/관리자 생성 로직을 `buildRuntimeDeps()` 또는 유사 함수로 추출하십시오.
2. 핸들러 등록을 `registerRoutes(mux, deps)` 함수로 분리하십시오.
3. 백그라운드 작업(cron, watchdog, telegram, autopilot) 시작을 `startBackgroundWorkers(deps)` 함수로 분리하십시오.
4. `buildAPIMux`는 이 세 함수를 순서대로 호출하는 100줄 이하 코디네이터로 유지하십시오.

**완료 기준 / Completion Criteria**
- [ ] `buildAPIMux` 100줄 이하
- [ ] 각 분리 함수가 단일 관심사만 수행
- [ ] 기존 서버 부팅 동작 유지

**테스트 기준 / Test Criteria**
- [ ] 통합 테스트: 서버 부팅 및 API 동작 검증
- [ ] 회귀 테스트: main_test.go 통과

---

### WO-OV-005: Orchestrator 에러 무시 14건 해소

**분류 코드 / Classification Code**: `ARCH-005`
**유형 / Type**: Add Error Handling / Logging
**심각도 / Severity**: 높음
**상태 / Status**: 미완료
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `orchestrator.go`에서 `appendSystemActivity()`와 `appendAgentReport()` 호출 14곳이 모두 `_ =`으로 에러를 무시. 감사 추적 로그가 유실되면 디버깅과 사용자 가시성이 심각하게 저하됨.
**소스 이슈 / Source Issue**: `ARCH-005`
**영향 범위 / Impact Area**: 프로젝트 오케스트레이터, 감사 로그

**문제 설명 / Problem**
태스크 디스패치, 상태 전이, 에이전트 리포트 기록 시 발생하는 activity 저장 에러가 모두 무시됩니다.
사용자는 태스크가 실패한 이유를 알 수 없고, 운영자는 감사 로그 유실을 감지할 수 없습니다.

**지시 사항 / Instructions**
1. `appendSystemActivity()` 에러를 `zlog.Warn`으로 기록하십시오 (워크플로를 중단할 필요는 없음).
2. `appendAgentReport()` 에러도 동일하게 경고 로그로 기록하십시오.
3. 14곳의 `_ =` 패턴을 모두 교체하십시오.
4. 에러가 빈번하게 발생할 수 있는 경로에는 rate-limited 로깅을 고려하십시오.

**완료 기준 / Completion Criteria**
- [ ] `orchestrator.go`에서 activity 관련 `_ =` 패턴 0건
- [ ] 에러 발생 시 경고 로그가 남음
- [ ] 기존 워크플로 중단 없이 동작

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: activity 저장 실패 시 로그 출력 검증
- [ ] 회귀 테스트: orchestrator_test.go 통과

---

### WO-OV-006: 핸들러 에러 분류를 타입 기반으로 전환

**분류 코드 / Classification Code**: `ARCH-006`
**유형 / Type**: Extract Error Type / Replace String Matching
**심각도 / Severity**: 중간
**상태 / Status**: 미완료
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `handler_project.go:104,309`, `handler_session.go:246` 등 10+ 위치에서 `strings.Contains(strings.ToLower(err.Error()), "not found")` 패턴으로 에러를 분류. 에러 메시지가 변경되면 분류가 깨짐.
**소스 이슈 / Source Issue**: `ARCH-006`
**영향 범위 / Impact Area**: `internal/tarsserver/` 에러 처리, `internal/project/`, `internal/session/`

**문제 설명 / Problem**
에러 분류가 메시지 문자열 매칭에 의존하여, 하위 패키지의 에러 메시지가 바뀌면 HTTP 상태 코드가 잘못 반환됩니다.

**지시 사항 / Instructions**
1. `internal/project/`와 `internal/session/`에 sentinel 에러 또는 에러 타입을 정의하십시오 (예: `ErrNotFound`, `ErrRequired`).
2. 핸들러에서 `errors.Is()` 또는 `errors.As()`로 에러를 분류하도록 변경하십시오.
3. 기존 `strings.Contains(err.Error(), ...)` 패턴을 모두 제거하십시오.
4. `handlers.go`의 `writeError()` 함수가 에러 타입에 따라 HTTP 상태 코드를 결정하도록 보강하십시오.

**완료 기준 / Completion Criteria**
- [ ] 핸들러에서 에러 메시지 문자열 매칭 0건
- [ ] 에러 분류가 타입/sentinel 기반으로 동작
- [ ] HTTP 상태 코드 반환 동작 유지

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: 에러 타입별 HTTP 상태 코드 검증
- [ ] 회귀 테스트: handler_*_test.go 통과

---

### WO-OV-007: Agent loop에서 tool 인자 자동 교정 제거

**분류 코드 / Classification Code**: `ARCH-007`
**유형 / Type**: Move Responsibility / Remove Concern Leak
**심각도 / Severity**: 중간
**상태 / Status**: 미완료
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `agent/loop.go:26`에서 `autoExecCommandFallback = "pwd"` 상수 정의. `loop.go:118-122,318-336`에서 exec tool 호출 시 `command` 인자가 없으면 `"pwd"`를 삽입. tool의 기본값 책임을 agent가 침범.
**소스 이슈 / Source Issue**: `ARCH-007`
**영향 범위 / Impact Area**: Agent 루프, exec tool

**문제 설명 / Problem**
Agent loop이 특정 tool(exec)의 인자를 자동 교정하는 것은 관심사 분리 위반입니다.
tool 자체가 기본값을 처리해야 하며, agent loop은 tool 구현 세부사항을 몰라야 합니다.

**지시 사항 / Instructions**
1. `tool/exec.go`의 Execute 함수에서 `command` 인자가 없을 때 기본값을 처리하도록 변경하십시오.
2. `agent/loop.go`에서 `autoExecCommandFallback` 상수와 관련 로직(118-122, 318-336)을 제거하십시오.
3. tool name normalization도 tool 등록 시점으로 이동을 검토하십시오 (`loop.go:161-164`의 이중 조회 제거).

**완료 기준 / Completion Criteria**
- [ ] Agent loop에서 tool 인자 교정 로직 0건
- [ ] exec tool이 자체적으로 기본값 처리
- [ ] 기존 동작 유지

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: exec tool 기본값 처리 검증
- [ ] 회귀 테스트: loop_test.go 통과

---

### WO-OV-008: 매직 넘버를 구성 가능한 상수/설정으로 통합

**분류 코드 / Classification Code**: `DEBT-001`
**유형 / Type**: Extract Constant / Add Configuration
**심각도 / Severity**: 중간
**상태 / Status**: 미완료
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: 5+ 모듈에 걸쳐 기본값이 하드코딩됨. Gateway: 2000(runs), 500(messages), 30(days). Agent: 8(max iters), 3(repeat limit). Prompt: 7000(static budget), 700(relevant budget). Project: 16(max iterations), 60s(interval).
**소스 이슈 / Source Issue**: `DEBT-001`
**영향 범위 / Impact Area**: 전역

**문제 설명 / Problem**
운영 환경별 조정이 필요한 기본값들이 코드에 산재되어, 변경 시 소스 수정과 재배포가 필요합니다.

**지시 사항 / Instructions**
1. 각 모듈의 패키지 수준 상수를 `const` 블록으로 정리하고 명확한 이름을 부여하십시오.
2. 운영 조정이 필요한 값(gateway 제한, agent 반복 제한, prompt 토큰 예산)은 `Config` struct 또는 모듈별 `Options` struct로 외부화하십시오.
3. 현재 하드코딩된 값을 기본값으로 유지하되, 설정으로 오버라이드 가능하게 하십시오.
4. 한 모듈씩 점진적으로 진행하십시오.

**완료 기준 / Completion Criteria**
- [ ] 운영 조정 대상 매직 넘버가 설정 또는 named constant로 관리됨
- [ ] 기존 기본 동작 유지

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: 기본값과 오버라이드 값 검증
- [ ] 회귀 테스트: 각 모듈 기존 테스트 통과

---

### WO-OV-009: Gateway 세션 에러 무시 해소

**분류 코드 / Classification Code**: `DEBT-002`
**유형 / Type**: Add Error Handling
**심각도 / Severity**: 중간
**상태 / Status**: 미완료
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `runtime_run_execute.go:87,88,216`에서 `_ = r.appendSessionMessage(...)` — 세션 메시지 저장 실패를 무시하여 디버깅 불가.
**소스 이슈 / Source Issue**: `DEBT-002`
**영향 범위 / Impact Area**: Gateway run 실행

**지시 사항 / Instructions**
1. `appendSessionMessage` 에러를 `zlog.Warn`으로 기록하십시오.
2. 세션 저장 실패가 run 실행을 중단할 필요는 없지만, 실패 사실은 반드시 기록되어야 합니다.
3. `_ =` 패턴을 모두 교체하십시오.

**완료 기준 / Completion Criteria**
- [ ] Gateway에서 세션 관련 `_ =` 패턴 0건
- [ ] 에러 발생 시 경고 로그 출력

**테스트 기준 / Test Criteria**
- [ ] 회귀 테스트: runtime_test.go 통과

---

### WO-OV-010: 프로젝트 상태를 enum/상수로 정의

**분류 코드 / Classification Code**: `DEBT-003`
**유형 / Type**: Extract Constant / Define Enum
**심각도 / Severity**: 중간
**상태 / Status**: 미완료
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: "todo", "in_progress", "review", "done", "blocked", "failed", "canceled" 등 상태 문자열이 `orchestrator.go`, `workflow_policy.go`, `project_runner.go`, `kanban.go` 등에 산재. 오타 시 런타임 오류 발생 가능.
**소스 이슈 / Source Issue**: `DEBT-003`
**영향 범위 / Impact Area**: `internal/project/` 전체

**지시 사항 / Instructions**
1. `internal/project/` 에 `TaskStatus` 타입과 상수를 정의하십시오 (예: `TaskStatusTodo`, `TaskStatusInProgress`).
2. `orchestrator.go`, `workflow_policy.go`, `project_runner.go`, `kanban.go`의 문자열 리터럴을 상수로 교체하십시오.
3. `NormalizeTaskStatus()` 등 기존 정규화 함수가 상수를 반환하도록 변경하십시오.

**완료 기준 / Completion Criteria**
- [ ] 프로젝트 상태 문자열 리터럴이 상수로 교체됨
- [ ] 오타 시 컴파일 에러 발생

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: 상태 상수와 정규화 함수 검증
- [ ] 회귀 테스트: project 모듈 전체 테스트 통과

---

### WO-OV-011: 배치 태스크 디스패치 에러 집계

**분류 코드 / Classification Code**: `DEBT-004`
**유형 / Type**: Add Error Aggregation
**심각도 / Severity**: 중간
**상태 / Status**: 미완료
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `orchestrator.go:80-136`의 `dispatchTasksByStatus()`가 병렬 태스크 디스패치 시 첫 번째 에러만 보존. 10개 태스크 실패 시 9개 에러 유실.
**소스 이슈 / Source Issue**: `DEBT-004`
**영향 범위 / Impact Area**: 프로젝트 오케스트레이터

**지시 사항 / Instructions**
1. `errors.Join()` (Go 1.20+) 또는 커스텀 에러 집계를 사용하여 모든 디스패치 에러를 수집하십시오.
2. 반환된 에러에서 개별 실패를 `errors.Unwrap()` 또는 `errors.As()`로 접근 가능하게 하십시오.
3. 호출자가 전체 실패 현황을 파악할 수 있도록 에러 메시지를 포맷하십시오.

**완료 기준 / Completion Criteria**
- [ ] 모든 디스패치 에러가 집계되어 반환됨
- [ ] 호출자가 개별 에러에 접근 가능

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: 다중 실패 시 모든 에러 포함 검증
- [ ] 회귀 테스트: orchestrator_test.go 통과

---

### WO-OV-012: Gateway 정책 tool 이름 추출을 타입 기반으로 전환

**분류 코드 / Classification Code**: `DEBT-005`
**유형 / Type**: Replace String Parsing / Add Error Type
**심각도 / Severity**: 중간
**상태 / Status**: 미완료
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `runtime_run_execute.go:128-144`에서 에러 메시지의 `"tool not injected for this request:"` 접두사를 파싱하여 tool 이름을 추출. 에러 메시지 변경 시 깨짐.
**소스 이슈 / Source Issue**: `DEBT-005`
**영향 범위 / Impact Area**: Gateway 실행

**지시 사항 / Instructions**
1. `internal/tool/` 또는 `internal/agent/`에 `ToolNotInjectedError` 타입을 정의하고, tool 이름 필드를 포함하십시오.
2. tool 주입 검증 실패 시 이 에러 타입을 반환하도록 변경하십시오.
3. `runtime_run_execute.go`에서 `errors.As()`로 tool 이름을 추출하십시오.

**완료 기준 / Completion Criteria**
- [ ] 에러 메시지 문자열 파싱 0건
- [ ] 타입 기반 tool 이름 추출

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: ToolNotInjectedError 생성 및 추출 검증
- [ ] 회귀 테스트: runtime_run_execute 관련 테스트 통과

---

### WO-OV-013: DEAD 코드 정리 (minor)

**분류 코드 / Classification Code**: `DEAD-002`, `DEAD-003`, `DEAD-004`, `DEAD-005`
**유형 / Type**: Remove / Unexport
**심각도 / Severity**: 낮음
**상태 / Status**: 미완료
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: 4건의 minor dead code: Gateway 내부 함수 중복 export(`types.go:223-237`), Agent HookFunc 미사용(`loop.go:44-48`), Project 경로 헬퍼 불필요 export(5곳), handler_chat.go 미사용 변형.
**소스 이슈 / Source Issue**: `DEAD-002`, `DEAD-003`, `DEAD-004`, `DEAD-005`
**영향 범위 / Impact Area**: Gateway, Agent, Project, TarsServer

**지시 사항 / Instructions**
1. Gateway: `defaultWorkspaceID`, `normalizeWorkspaceID` (private 중복) 제거하고 public 버전만 유지.
2. Agent: `HookFunc` 타입이 실제로 사용되지 않으면 제거.
3. Project: `ActivityPath()`, `BriefPath()`, `StatePath()`, `ProjectFilePath()`, `BoardPath()`를 unexport (소문자로 변경).
4. TarsServer: 미사용 `newChatAPIHandler*` 변형 제거.

**완료 기준 / Completion Criteria**
- [ ] 미사용 export 제거/unexport
- [ ] 빌드 및 테스트 통과

**테스트 기준 / Test Criteria**
- [ ] 빌드 테스트: `go build ./...` 성공
- [ ] 회귀 테스트: 각 모듈 기존 테스트 통과

---

### WO-OV-014: OVER 코드 단순화 (minor)

**분류 코드 / Classification Code**: `OVER-001`, `OVER-002`, `OVER-005`
**유형 / Type**: Simplify / Remove Unused Variant
**심각도 / Severity**: 낮음
**상태 / Status**: 미완료
**선행 작업 / Prerequisite**: WO-OV-002 (Gateway 분해 후)
**근거 / Evidence**: Gateway 브라우저 1:1 래핑(`runtime_browser.go`), Tool ContentBlock 변형 미사용(`tool.go:13-38`), 아카이브 1000 파일 회전(`runtime_archive.go:53-78`).
**소스 이슈 / Source Issue**: `OVER-001`, `OVER-002`, `OVER-005`
**영향 범위 / Impact Area**: Gateway, Tool

**지시 사항 / Instructions**
1. Gateway 브라우저: WO-OV-002의 BrowserDelegate 추출 후, 불필요한 상태 복사를 제거하고 `browserService`에 위임만 하십시오.
2. Tool ContentBlock: `Result.Text() string` 메서드를 유지하되, `ContentBlock` 타입을 `type Result struct { Content string }` 로 단순화를 검토하십시오. 향후 멀티모달 확장이 예정된 경우 현행 유지.
3. 아카이브: 파일명에 타임스탬프(nano 또는 UUID)를 사용하여 1000회 루프를 제거하십시오.

**완료 기준 / Completion Criteria**
- [ ] 불필요한 복잡도 제거
- [ ] 기존 동작 유지

**테스트 기준 / Test Criteria**
- [ ] 회귀 테스트: 각 모듈 기존 테스트 통과

---

## 기존 리팩터 지시서와의 관계 / Relationship with Existing Refactor Guide

| 오버홀 WO | 기존 리팩터 WO | 관계 |
|-----------|---------------|------|
| WO-OV-001 (browserrelay 삭제) | WO-006 (SEC-002, 완료), WO-011 (TIDY-007, 미완료) | **WO-011 대체** — 파일 분할 불필요, 전체 삭제 |
| WO-OV-002 (Gateway Runtime) | WO-021 (TIDY-011, 미완료) | **WO-021 흡수** — sub-struct 추출을 포함하는 상위 작업 |
| WO-OV-005 (Orchestrator 에러) | WO-020 (TIDY-010, 미완료) | **병행 가능** — WO-020 함수 분할 시 에러 처리도 함께 개선 |
| WO-OV-010 (상태 enum) | 해당 없음 | 신규 |
| WO-OV-003 (핸들러 계층) | 해당 없음 | 신규 |
| WO-OV-004 (buildAPIMux) | 해당 없음 | 신규 |

---

## 오버홀 순서 (Recommended Order) / Recommended Order

의존 관계 없는 작업은 병렬 실행 가능합니다.
Independent tasks can run in parallel.

```text
━━ Phase 1: 사장 코드 제거 (즉시) ━━
WO-OV-001 (DEAD-001: browserrelay 삭제)   ← 독립, 위험 없음, 2,321줄 즉시 제거

━━ Phase 2: 에러 처리 보강 (단기) ━━
WO-OV-005 (ARCH-005: Orchestrator 에러)   ← 독립
WO-OV-009 (DEBT-002: Gateway 세션 에러)   ← 독립
WO-OV-006 (ARCH-006: 에러 타입 전환)      ← 독립
WO-OV-011 (DEBT-004: 배치 에러 집계)      ← 독립
WO-OV-012 (DEBT-005: tool 이름 타입화)    ← 독립

━━ Phase 3: 아키텍처 정리 (중기) ━━
WO-OV-003 (ARCH-003: 핸들러 계층 정리)    ← 독립
  └─ WO-OV-004 (ARCH-004: buildAPIMux 분할) ← WO-OV-003 완료 후
WO-OV-002 (ARCH-001: Gateway Runtime 분해) ← 독립, 큰 범위
WO-OV-007 (ARCH-007: Agent tool 교정 제거) ← 독립

━━ Phase 4: 코드 품질 (후기) ━━
WO-OV-008 (DEBT-001: 매직 넘버 정리)      ← 독립, 점진적
WO-OV-010 (DEBT-003: 상태 enum)           ← 독립
WO-OV-013 (DEAD minor: 정리)              ← 독립
WO-OV-014 (OVER minor: 단순화)            ← WO-OV-002 완료 후
```

---

## 체크포인트 진행 로그 (Checkpoint Progress Log) / Checkpoint Progress Log

| Checkpoint | 상태 | 변경된 WO | ARCH/DEAD/OVER/DEBT 진행 현황 | 메모 |
|------------|------|-----------|-------------------------------|------|
| checkpoint-001 | completed | WO-OV-001 ~ WO-OV-014 초안 확정 | ARCH 0/10, DEAD 0/5, OVER 0/6, DEBT 0/9 | 5개 병렬 에이전트 분석 종합, 30개 이슈 식별 |

---

## 다음 세션 재개 지점 (Resume Point) / Resume Point

- **다음 시작 WO**: `WO-OV-001` (DEAD-001, browserrelay 삭제) — 가장 안전하고 효과 큰 작업
- **남은 선행 작업**: WO-OV-004는 WO-OV-003 완료 후, WO-OV-014는 WO-OV-002 완료 후 실행
- **우선 재검토 파일**: `internal/browserrelay/server.go`, `internal/project/orchestrator.go`, `internal/gateway/runtime_run_execute.go`
- **재개 순서 / Resume Steps**
  1. 마지막 체크포인트 로그 행 확인
  2. Phase 1부터 순서대로 구현 대상 선택
  3. 구현 대상 WO의 "지시 사항 / Instructions"를 작업 단위로 분해
  4. 실제 구현 세션에서는 선택한 WO에 맞는 실패 테스트를 먼저 추가

---

## 완료 검증 체크리스트 / Completion Verification Checklist

- [ ] `make test` 전체 통과
- [ ] `make lint` 경고 없음 (새로 추가된 파일 포함)
- [ ] 변경된 파일의 최대 줄 수가 300줄 이하
- [x] 이슈 목록의 모든 항목이 `ARCH-*`, `DEAD-*`, `OVER-*`, `DEBT-*` 중 하나로 분류됨
- [x] 이슈 목록의 모든 항목이 WO로 처리됨
- [x] `ARCH-*` 항목마다 계층/경계/책임 분석이 기재됨
- [x] 기존 리팩터 지시서(DUP/SEC/TIDY)와의 관계가 명시됨
