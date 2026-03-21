# 리팩토링 지시서: 저장소 핵심 운영 경로
# Refactoring Work Order: Repository Core Runtime Paths

> **생성일 / Created At**: 2026-03-15
> **세션 ID / Session ID**: `refactor-guide-20260315-095000`
> **체크포인트 / Checkpoint**: `checkpoint-006`
> **대상 / Target**: `internal/config`, `internal/project`, `internal/tarsserver`, `internal/auth`, `internal/llm`, `internal/browserrelay`
> **분석 범위 / Analysis Scope**: 이슈 후보 역추적 + 관련 파일 직접 검증 / issue-candidate verification with direct source inspection
> **우선순위 / Priority**: 높음 / High
> **소스 / Source**: `issue-candidates.md` 검증 / verified from `issue-candidates.md`

---

## 요약 (Summary) / Summary

이 지시서는 저장소 핵심 운영 경로에서 발견된 코드 품질/보안/구조 문제를 해결하기 위한 리팩토링 작업을 정의합니다.
총 26개의 이슈가 발견되었으며 (`DUP` 7건 / `SEC` 7건 / `TIDY` 12건), 이 중 19건이 완료되었고 7건이 미완료입니다.
This work order defines the refactoring tasks required to address code quality, security, and structural issues found in the repository's core runtime paths.
There are 26 issues in total (`DUP` 7 / `SEC` 7 / `TIDY` 12), with 19 completed and 7 remaining.

---

## 발견된 이슈 목록 (Issues Found) / Issues Found

| # | 분류 코드 | 이슈 유형 | 상태 | 위치 | 근거 | 위험도 | 영향 범위 |
|---|-----------|-----------|------|------|------|--------|-----------|
| 1 | `TIDY-001` | 설정 스키마 매핑 중복 | 완료 | `internal/config/defaults_apply.go:13-144`, `internal/config/env.go:8-320`, `internal/config/yaml.go:12-263`, `internal/config/merge.go:3-280` | 같은 `Config` 필드 집합을 입력원별로 반복 매핑 | 중간 | 설정 로딩 전역 |
| 2 | `TIDY-002` | 프로젝트 workflow 전이 분산 | 완료 | `internal/tarsserver/handler_chat.go:26-45`, `internal/project/brief_state.go:128-280`, `internal/project/orchestrator.go:72-136`, `internal/project/project_runner.go:208-320`, `internal/tarsserver/handler_project.go:460-523` | kickoff, brief/state 저장, dispatch, autopilot 루프가 별도 규칙으로 흩어짐 | 높음 | 프로젝트 시작, 자동 실행, 대시보드 |
| 3 | `SEC-001` | 대시보드 공개 모드 오사용 위험 | 완료 | `internal/tarsserver/middleware.go:26-33` | `dashboard_auth_mode=off` 시 `/dashboards`, `/ui/projects/*` 전체가 인증 skip path가 됨 | 높음 | 프로젝트 메타데이터 노출 |
| 4 | `TIDY-003` | 대시보드 섹션 정의 중복 | 완료 | `internal/tarsserver/dashboard.go:214-406`, `internal/tarsserver/dashboard.go:416-428`, `internal/tarsserver/dashboard.go:573-586` | 섹션 ID, refresh 대상, 서버 데이터 조립이 암묵적으로 묶여 있음 | 중간 | 대시보드 렌더링/추가 개발 |
| 5 | `TIDY-004` | Provider credential lifecycle 분산 | 완료 | `internal/auth/token.go:18-97`, `internal/auth/codex_oauth.go:48-247`, `internal/llm/provider.go:118-140`, `internal/llm/openai_codex_client.go:171-185`, `internal/llm/model_lister.go:165-193` | 토큰 해석, provider 특화 credential, refresh retry가 여러 계층에 분산 | 중간 | LLM provider onboarding, 인증 회복 |
| 6 | `SEC-002` | Browser relay query token 노출 가능성 | 완료 | `internal/browserrelay/server.go:24-25`, `internal/browserrelay/server.go:1456-1484` | opt-in 이지만 query string의 `token` / `relay_token`을 그대로 인증값으로 허용 | 중간 | 로컬 브라우저 relay 인증 |
| 7 | `DUP-001` | Ask() 메서드 중복 | 완료 | `llm/anthropic.go:96`, `openai_compat_client.go:289`, `gemini_native.go:78`, `openai_codex_client.go:143`, `claude_code_cli.go:62` | 동일한 Ask() 구현이 5개 클라이언트에 복사됨 | 낮음 | LLM 클라이언트 |
| 8 | `DUP-002` | Chat() 요청 보일러플레이트 중복 | 완료 | `llm/anthropic.go:56-94`, `openai_compat_client.go:61-103`, `gemini_native.go:86-100` | zlog.Debug → buildRequest → doPreparedRequest → parseResponse 패턴 반복 | 중간 | LLM 클라이언트 |
| 9 | `DUP-003` | 클라이언트 생성자 검증 로직 중복 | 완료 | `llm/anthropic.go:28-33`, `openai_compat_client.go:29-37`, `openai_codex_client.go:95-100` | baseURL/apiKey/model 빈 값 검증 동일 패턴 반복 | 낮음 | LLM 클라이언트 |
| 10 | `DUP-004` | providerAuthConfig TrimSpace 반복 | 완료 | `llm/provider.go:193-224` | `strings.TrimSpace(strings.ToLower(...))` 패턴 10회 이상 반복 | 낮음 | LLM 프로바이더 |
| 11 | `DUP-005` | HTTP method 검증 38회 반복 | 완료 | `tarsserver/handler_*.go` 전반 | `if r.Method != http.MethodGet` 패턴 38회 반복, 기존 `requireMethod()` 미사용 | 중간 | API 핸들러 |
| 12 | `DUP-006` | Tool 파라미터 bounds 체크 6회 반복 | 완료 | `tool/read_file.go:58`, `list_dir.go:55`, `glob.go:57` 등 | default → override → min/max clamp 패턴 6개 tool에서 반복 | 낮음 | 도구 |
| 13 | `DUP-007` | Gateway "disabled" 체크 11회 반복 | 완료 | `gateway/runtime_channels.go`, `runtime_executors.go` 등 | `if r == nil \|\| !r.opts.Enabled` 가드 11곳 반복 | 낮음 | Gateway |
| 14 | `SEC-003` | API 토큰 비교 시 loopback fallback 경로 | 완료 | `serverauth/middleware.go` | 토큰 미설정 시 loopback fallback 경로의 컨테이너/VM 환경 위험도 검토 필요 | 낮음 | 서버 인증 |
| 15 | `SEC-004` | Gateway executor 환경변수 주입 | 완료 | `gateway/executor.go:280-292` | RunID/SessionID/WorkspaceID가 검증 없이 환경변수로 결합 | 높음 | Gateway 실행 |
| 16 | `SEC-005` | HTTP 요청 body 크기 미제한 | 완료 | `tarsserver/handler_project.go:79`, `handler_ops.go:67` 등 | `json.NewDecoder(r.Body).Decode()` 호출 시 `MaxBytesReader` 미적용 | 중간 | API 핸들러 |
| 17 | `SEC-006` | Codex refresh token 평문 디스크 저장 | 완료 | `auth/codex_oauth.go:199-250` | OAuth refresh token이 JSON 파일에 평문 저장 (0o600이지만 암호화 없음) | 중간 | 인증 |
| 18 | `SEC-007` | Telegram 토큰 redaction 불완전 | 완료 | `tarsserver/telegram_redaction.go:10-20` | 단순 `strings.ReplaceAll`만 사용, URL-encoded/JSON-nested 형태 미대응 | 중간 | 텔레그램 |
| 19 | `TIDY-005` | Config 구조체 비대화 (148 필드 flat struct) | 미완료 | `config/types.go:29-148` | 단일 struct에 148개 필드, 관심사 분리 없음 | 높음 | 설정 전체 |
| 20 | `TIDY-006` | dashboard.go 인라인 HTML/CSS/JS (974줄) | 미완료 | `tarsserver/dashboard.go:235-974` | Go 코드 내에 HTML/CSS/JS가 문자열 상수로 혼재 | 중간 | 대시보드 |
| 21 | `TIDY-007` | browserrelay/server.go 단일 파일 비대 (1508줄) | 미완료 | `browserrelay/server.go` | WebSocket, CDP, 인증, 상태관리가 한 파일에 혼재 | 중간 | 브라우저 릴레이 |
| 22 | `TIDY-008` | openai_codex_client.go 복잡도 (824줄) | 미완료 | `llm/openai_codex_client.go` | 인증, HTTP 전송, 응답 파싱, tool 매핑이 한 파일에 혼재 | 중간 | Codex LLM 클라이언트 |
| 23 | `TIDY-009` | helpers_cron.go 복잡도 (807줄) | 미완료 | `tarsserver/helpers_cron.go` | cron 실행, 프로젝트 조립, 텔레그램 알림, 텔레메트리가 한 함수 체인에 혼재 | 중간 | 크론 실행 |
| 24 | `TIDY-010` | orchestrator.dispatchTask() 230줄 | 완료 | `project/orchestrator.go:138-367` | 8개 이상의 책임이 한 함수에 혼재 | 높음 | 프로젝트 오케스트레이터 |
| 25 | `TIDY-011` | Gateway Runtime god object (21 필드) | 미완료 | `gateway/types.go:191-221` | Run lifecycle, Channel, Agent, Browser, Persistence를 단일 struct가 관리 | 중간 | Gateway |
| 26 | `TIDY-012` | project_runner.go Run loop 150줄 | 미완료 | `project/project_runner.go:208-359` | 7개 보드 상태에 대한 중첩 switch문, 각 case에 혼재 | 중간 | 프로젝트 러너 |

---

## 작업 지시서 (Work Orders) / Work Orders

각 작업은 독립적으로 실행 가능해야 합니다.
의존 관계가 있는 경우 "선행 작업" 필드를 채웁니다.
Each work order should be executable independently.
If there is a dependency, fill in the "Prerequisite" field.

---

### WO-001: 설정 입력원 매핑을 메타데이터 중심으로 통합

**분류 코드 / Classification Code**: `TIDY-001`
**유형 / Type**: Extract Table / Split Responsibility / Centralize Mapping
**심각도 / Severity**: 중간
**상태 / Status**: 완료, PR [#64](https://github.com/devlikebear/tars/pull/64)
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `defaults_apply.go`, `env.go`, `yaml.go`, `merge.go`가 같은 `Config` 필드 집합을 서로 다른 분기문으로 반복 처리합니다. 새 설정 키를 추가하면 입력원별 규칙을 각각 수정해야 해서 누락 회귀가 쉽게 발생합니다.
**소스 이슈 / Source Issue**: `TIDY-001`
**영향 범위 / Impact Area**: 설정 로딩, 기본값 적용, YAML/env 우선순위

**문제 설명 / Problem**
현재 `internal/config`에서는 같은 설정 모델을 입력원별 함수가 각각 알고 있습니다.
구조가 유지되면 설정 키 추가, 이름 변경, validation 보강 시 수정 지점이 계속 늘어나고 입력원 간 동작 차이가 누적됩니다.
At the current config layer, each input source knows the same configuration model independently.
If left unchanged, each new setting or validation rule multiplies edit points and increases precedence drift.

**현재 코드 위치 / Current Code Location**
- `internal/config/defaults_apply.go` 라인 13-260
- `internal/config/env.go` 라인 8-320
- `internal/config/yaml.go` 라인 12-263
- `internal/config/merge.go` 라인 3-280

**TIDY 상세 / TIDY Details (`TIDY-*`인 경우 필수 / required for `TIDY-*`)**
- **적용 룰**: `중복을 통제 가능한 단위로 모은다`, `구조 변경과 동작 변경을 분리한다`
- **구조 변경 단계**: 설정 필드 메타데이터 테이블을 도입해 키 이름, 정규화 함수, merge 정책, default 정책을 한 구조로 모읍니다.
- **동작 변경 단계**: 구조 정리 후 필요할 때만 입력원 우선순위나 bool merge semantics 같은 실제 동작 차이를 명시적으로 보정합니다.

**지시 사항 / Instructions**
1. `Config` 필드별 메타데이터를 정의하는 registry 또는 descriptor 테이블을 추가하십시오.
2. YAML 키 파싱, env key 매핑, default 적용, merge 조건을 descriptor 기반 helper로 재구성하십시오.
3. bool/int/string/list 타입별 공통 setter를 만들어 `env.go`, `yaml.go`, `merge.go`의 반복 분기를 제거하십시오.
4. 기존 precedence를 바꾸지 않는 구조 변경을 먼저 완료하고, 동작 보정이 필요하면 별도 커밋 또는 WO로 분리하십시오.
5. 새 설정 키 추가 시 수정 지점이 descriptor 한 곳과 필요한 validator 정도로 제한되도록 만드십시오.

**완료 기준 / Completion Criteria**
- [x] 동일 설정 필드 매핑 로직이 입력원별 switch/if 중복 없이 공통 메타데이터를 통해 정의됨
- [x] YAML/env/default/merge 경로가 같은 정규화 규칙을 재사용함
- [x] 새 설정 키 추가 절차가 기존보다 명확하고 짧아짐

**테스트 기준 / Test Criteria**
- [x] 단위 테스트: 대표 설정 키에 대해 default, YAML, env, merge precedence 검증
- [x] 통합 테스트: `Load` 경로에서 기존 설정 파일과 env override가 동일하게 동작하는지 검증
- [x] 회귀 테스트: bool/list/int/string 타입별 기존 파싱 결과 유지 확인

---

### WO-002: 프로젝트 workflow 전이를 명시적 정책/상태기계로 분리

**분류 코드 / Classification Code**: `TIDY-002`
**유형 / Type**: Extract Policy / State Machine / Move Logic
**심각도 / Severity**: 높음
**상태 / Status**: 완료, PR [#70](https://github.com/devlikebear/tars/pull/70), PR [#74](https://github.com/devlikebear/tars/pull/74)
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: kickoff 감지는 `handler_chat.go`의 `resolveChatSession`/`resolveSkillForMessage`, brief/state 초기화는 `brief_state.go`, stage dispatch는 `orchestrator.go`, 자동 전이는 `project_runner.go`, API 트리거는 `handler_project.go`에 분산돼 있습니다. 상태 전이를 선언적으로 설명하는 단일 모델이 없어 채널별 동작 차이를 만들기 쉽습니다.
**소스 이슈 / Source Issue**: `TIDY-002`
**영향 범위 / Impact Area**: 프로젝트 시작, brief finalize, dispatch, autopilot, dashboard state 표시

**문제 설명 / Problem**
현재 프로젝트 workflow는 여러 계층이 암묵적으로 같은 상태 전이를 공유한다고 가정합니다.
이 구조에서는 새 phase 추가, blocked recovery 조정, 새로운 kickoff 경로 추가 시 chat/API/autopilot 사이의 불일치가 생기기 쉽습니다.
The current project workflow assumes multiple layers share the same transitions implicitly.
That makes it easy for chat, API, and autopilot behavior to drift when phases or recovery rules evolve.

**현재 코드 위치 / Current Code Location**
- `internal/tarsserver/handler_chat.go` 라인 26-45, 215-229
- `internal/project/brief_state.go` 라인 128-280
- `internal/project/orchestrator.go` 라인 72-136
- `internal/project/project_runner.go` 라인 208-320
- `internal/tarsserver/handler_project.go` 라인 460-523

**TIDY 상세 / TIDY Details (`TIDY-*`인 경우 필수 / required for `TIDY-*`)**
- **적용 룰**: `구조 변경과 동작 변경을 분리한다`, `작고 안전한 단계로 진행한다`, `동작 보존을 먼저 보장한다`
- **구조 변경 단계**: workflow transition table 또는 policy 객체를 도입해 kickoff, state normalization, dispatchable stage, autopilot retry 조건을 한 모델로 추출합니다.
- **동작 변경 단계**: 구조 정리 후에만 새로운 phase 추가, blocked/review 정책 조정, dashboard 표기 개선 같은 실제 행동 변경을 적용합니다.

**지시 사항 / Instructions**
1. 프로젝트 상태/전이 규칙을 표현하는 전용 타입을 `internal/project`에 추가하십시오.
2. chat kickoff 판정, brief finalize 초기 상태, todo/review dispatch 조건, autopilot retry 조건을 그 타입으로 역참조하도록 바꾸십시오.
3. API handler와 dashboard는 raw 상태 필드를 직접 해석하지 말고 공통 policy 결과를 사용하도록 조정하십시오.
4. `policy.go`가 현재 tool policy만 담당하므로, workflow policy를 별도 파일 또는 하위 패키지로 분리해 책임을 명확히 하십시오.
5. 구조 정리 이후에만 phase 추가나 자동 복구 동작 변경을 진행하십시오.

**완료 기준 / Completion Criteria**
- [x] 프로젝트 workflow 전이가 선언적 정책 또는 상태기계로 한 곳에 모임
- [x] chat/API/autopilot이 같은 전이 규칙을 재사용함
- [x] 새 phase 또는 retry rule 변경 시 수정 지점이 명확히 줄어듦

**테스트 기준 / Test Criteria**
- [x] 단위 테스트: kickoff 판정, state normalization, dispatchable stage, retry rule 검증
- [x] 통합 테스트: brief finalize -> dispatch -> review -> done 흐름 검증
- [x] 회귀 테스트: stalled task recovery, empty board seed, review-required task 경로 검증

---

### WO-003: 대시보드 공개 모드에 방어적 가드를 추가

**분류 코드 / Classification Code**: `SEC-001`
**유형 / Type**: Harden Default / Add Guard / Add Warning
**심각도 / Severity**: 높음
**상태 / Status**: 완료, PR [#66](https://github.com/devlikebear/tars/pull/66)
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `apiAuthSkipPaths`는 `dashboard_auth_mode=off`일 때 `/dashboards`, `/dashboards/`, `/ui/projects/*`를 인증 예외로 추가합니다. 해당 화면에는 objective, board, activity, worker report, blocker 같은 내부 운영 데이터가 포함됩니다.
**소스 이슈 / Source Issue**: `SEC-001`
**영향 범위 / Impact Area**: HTTP dashboard 접근 제어

**문제 설명 / Problem**
현재 구현은 운영자가 `dashboard_auth_mode=off`를 설정하면 대시보드 전체가 즉시 공개됩니다.
실수로 외부 바인딩 환경에서 이 설정을 켜면 프로젝트 운영 정보가 별도 경고 없이 노출될 수 있습니다.
At the current middleware layer, disabling dashboard auth immediately exposes all dashboard routes.
If this is enabled on a non-loopback deployment by mistake, internal project data can be disclosed without a second guardrail.

**현재 코드 위치 / Current Code Location**
- `internal/tarsserver/middleware.go` 라인 26-33

**SEC 상세 / SEC Details (`SEC-*`인 경우 필수 / required for `SEC-*`)**
- **취약 시나리오**: 운영자가 인증을 끄고 서버를 외부 인터페이스에 바인딩하면 임의의 네트워크 사용자가 `/dashboards`와 개별 프로젝트 UI를 조회합니다.
- **악용 전제**: `dashboard_auth_mode=off`가 설정되어 있고, 프로세스가 loopback 외 주소에서 접근 가능해야 합니다.
- **완화 방향**: loopback-only 제한, 시작 시 강한 경고 로그, 명시적 `unsafe` 플래그, 또는 별도 public dashboard 허용 설정을 추가합니다.

**지시 사항 / Instructions**
1. `dashboard_auth_mode=off`가 설정되더라도 loopback 외 바인딩에서는 기본적으로 공개를 거부하거나 별도 `unsafe` 승인 플래그를 요구하십시오.
2. 대시보드 공개 모드 활성화 시 프로세스 시작 로그와 `/v1/healthz` 또는 관리자 API에 경고 상태를 노출하십시오.
3. 설정 이름만으로는 위험성이 약하므로 `off` 외에 의도를 더 드러내는 opt-in 표현 또는 보조 설정을 설계하십시오.
4. 운영자가 현재 공개 상태를 쉽게 확인할 수 있도록 상태 요약을 추가하십시오.
5. 기존 `APIAuthMode`와의 상호작용을 문서와 테스트에 반영하십시오.

**완료 기준 / Completion Criteria**
- [x] 인증 비활성화만으로 외부 공개가 즉시 성립하지 않음
- [x] 위험한 공개 모드 활성화 시 운영 경고가 명확히 남음
- [x] dashboard 접근 정책이 테스트와 문서에서 재현 가능하게 설명됨

**테스트 기준 / Test Criteria**
- [x] 단위 테스트: loopback/non-loopback 환경에서 skip path 정책 검증
- [x] 통합 테스트: `dashboard_auth_mode=off`와 보호 플래그 조합별 HTTP status 검증
- [x] 회귀 테스트: 기본 설정과 기존 인증 on 경로가 변하지 않음 확인

---

### WO-004: 대시보드 섹션 레지스트리를 도입해 서버/클라이언트 정의를 단일화

**분류 코드 / Classification Code**: `TIDY-003`
**유형 / Type**: Extract Constant / Introduce Registry / Split Template Data
**심각도 / Severity**: 중간
**상태 / Status**: 완료, PR [#72](https://github.com/devlikebear/tars/pull/72)
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: 템플릿 본문은 `autopilot-section`, `board-section`, `activity-section`, `github-flow-section`, `reports-section`, `blockers-section`, `decisions-section`, `replans-section`를 직접 선언하고, refresh 스크립트는 같은 목록을 다시 문자열 배열로 들고 있습니다. 서버는 별도의 builder 호출로 같은 섹션 데이터를 수동 조립합니다.
**소스 이슈 / Source Issue**: `TIDY-003`
**영향 범위 / Impact Area**: dashboard HTML, SSE refresh, 섹션 추가/삭제

**문제 설명 / Problem**
현재 대시보드 섹션 정의는 템플릿, refresh 스크립트, 서버 데이터 조립 코드가 암묵적으로 같은 순서와 ID를 공유합니다.
이 구조에서는 섹션 추가나 이름 변경 시 한 위치라도 빠지면 refresh 누락이나 빈 화면 같은 회귀가 발생합니다.
The dashboard section contract is currently duplicated across the template, refresh script, and server-side data assembly.
That duplication makes section additions and renames fragile.

**현재 코드 위치 / Current Code Location**
- `internal/tarsserver/dashboard.go` 라인 214-406
- `internal/tarsserver/dashboard.go` 라인 416-428
- `internal/tarsserver/dashboard.go` 라인 573-586

**TIDY 상세 / TIDY Details (`TIDY-*`인 경우 필수 / required for `TIDY-*`)**
- **적용 룰**: `의도를 드러내는 이름을 사용한다`, `국소 정리(local tidy)를 우선한다`, `작고 안전한 단계로 진행한다`
- **구조 변경 단계**: 섹션 ID, 제목, 데이터 builder, refresh 대상 여부를 담는 registry를 추가합니다.
- **동작 변경 단계**: 구조 정리 후 필요하면 섹션 가시성 조건이나 ordering 정책을 변경합니다.

**지시 사항 / Instructions**
1. 대시보드 섹션 메타데이터를 표현하는 registry 또는 render spec 타입을 추가하십시오.
2. HTML 템플릿이 registry에서 파생된 데이터만 소비하도록 page data 구조를 단순화하십시오.
3. refresh 스크립트의 대상 ID 목록도 registry에서 생성된 값을 사용하게 하십시오.
4. 서버의 `buildProjectDashboard*` 함수 호출 순서를 registry 기반으로 정리해 섹션 추가 시 한 곳만 수정하면 되게 하십시오.
5. 섹션별 테스트는 hard-coded ID 목록 대신 registry 결과를 검증하도록 바꾸십시오.

**완료 기준 / Completion Criteria**
- [x] 섹션 ID와 refresh 대상 목록이 단일 레지스트리에서 관리됨
- [x] 새 섹션 추가 시 템플릿/스크립트/서버를 각각 따로 수정하지 않아도 됨
- [x] 기존 대시보드 렌더 결과와 refresh 동작이 유지됨

**테스트 기준 / Test Criteria**
- [x] 단위 테스트: registry 기반 섹션 목록과 data builder 매핑 검증
- [x] 통합 테스트: dashboard HTML 렌더와 refresh fetch가 같은 섹션 ID 집합을 사용하는지 검증
- [x] 회귀 테스트: 기존 8개 섹션이 동일 순서 또는 승인된 새 순서로 출력됨을 확인

---

### WO-005: Provider credential resolution과 refresh를 전략 레지스트리로 통합

**분류 코드 / Classification Code**: `TIDY-004`
**유형 / Type**: Extract Strategy / Introduce Registry / Move Logic
**심각도 / Severity**: 중간
**상태 / Status**: 완료, PR [#78](https://github.com/devlikebear/tars/pull/78)
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: 일반 OAuth token 해석은 `internal/auth/token.go`, Codex 전용 credential 파일/refresh/persist는 `internal/auth/codex_oauth.go`, provider 생성은 `internal/llm/provider.go`, Codex chat retry는 `internal/llm/openai_codex_client.go`, live model fetch retry는 `internal/llm/model_lister.go`에 분산되어 있습니다. provider 추가 시 인증 lifecycle를 여러 파일에서 다시 조합해야 합니다.
**소스 이슈 / Source Issue**: `TIDY-004`
**영향 범위 / Impact Area**: provider 생성, OAuth/API-key 해석, token refresh, model listing

**문제 설명 / Problem**
현재 credential lifecycle은 일반 provider 경로와 `openai-codex` 특화 경로가 분리된 채 여러 계층에 흩어져 있습니다.
이 구조에서는 새 provider 또는 새로운 OAuth 저장소를 추가할 때 resolve, refresh, persist, retry를 한 번에 검토하기 어렵습니다.
The credential lifecycle is currently split between generic token resolution and provider-specific Codex flows.
That makes onboarding or evolving providers harder because resolution, refresh, persistence, and retry policy are not modeled together.

**현재 코드 위치 / Current Code Location**
- `internal/auth/token.go` 라인 18-97
- `internal/auth/codex_oauth.go` 라인 48-247
- `internal/llm/provider.go` 라인 118-140
- `internal/llm/openai_codex_client.go` 라인 171-185, 261-276
- `internal/llm/model_lister.go` 라인 165-193

**TIDY 상세 / TIDY Details (`TIDY-*`인 경우 필수 / required for `TIDY-*`)**
- **적용 룰**: `구조 변경과 동작 변경을 분리한다`, `중복을 통제 가능한 단위로 모은다`
- **구조 변경 단계**: provider별 credential strategy를 정의해 resolve, refresh, persist, header shaping을 한 인터페이스로 묶습니다.
- **동작 변경 단계**: 구조 정리 후 필요하면 refresh backoff, proactive refresh, provider-specific fallback 정책을 조정합니다.

**지시 사항 / Instructions**
1. provider별 credential 전략을 등록하는 registry를 도입하십시오.
2. `ResolveToken`, `ResolveCodexCredential`, `RefreshCodexCredential`의 호출 지점을 registry 기반으로 재배치하십시오.
3. `openai-codex` chat client와 model fetcher가 같은 refresh 계약을 재사용하도록 공통 helper 또는 strategy를 추출하십시오.
4. provider 생성 시 auth mode, oauth provider, persisted source를 한 객체로 전달하도록 정리하십시오.
5. 구조 정리 이후에만 refresh 시점 조정 같은 행동 변경을 적용하십시오.

**완료 기준 / Completion Criteria**
- [x] provider별 인증 lifecycle이 명시적 전략/레지스트리로 표현됨
- [x] chat client와 model fetcher가 같은 refresh/persist 규약을 재사용함
- [x] 새 provider 추가 시 인증 관련 수정 지점이 예측 가능하게 줄어듦

**테스트 기준 / Test Criteria**
- [x] 단위 테스트: provider별 resolve/refresh/persist 전략 검증
- [x] 통합 테스트: `openai-codex`의 chat unauthorized retry와 model list retry가 동일 계약으로 동작하는지 검증
- [x] 회귀 테스트: `anthropic`, `gemini`, `openai`, `openai-codex` 기존 인증 경로 유지 확인

---

### WO-006: Browser relay query token 경로를 축소하고 운영 경고를 추가

**분류 코드 / Classification Code**: `SEC-002`
**유형 / Type**: Reduce Attack Surface / Add Guard / Add Warning
**심각도 / Severity**: 중간
**상태 / Status**: 완료, PR [#68](https://github.com/devlikebear/tars/pull/68)
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `relayTokenFromRequest`는 header가 없을 때 `AllowQueryToken`이 켜져 있으면 `token`과 `relay_token` query parameter를 인증값으로 사용합니다. loopback 제한은 있지만 URL 기반 토큰은 브라우저 기록, 프록시 로그, 디버그 출력에 남기 쉽습니다.
**소스 이슈 / Source Issue**: `SEC-002`
**영향 범위 / Impact Area**: browser relay 인증, 로컬 개발/운영 로그

**문제 설명 / Problem**
현재 relay는 opt-in 설정으로 query token을 허용하지만, 토큰이 URL에 남는 순간 보안 경계가 약해집니다.
특히 브라우저 확장, 디버그 복사, 프록시 기록, crash report가 개입하면 토큰이 의도치 않게 노출될 수 있습니다.
The relay currently allows query-based tokens when enabled, which weakens the boundary because URLs are routinely copied and logged.
Even with loopback-only access, token exposure becomes easier once it enters browser history or request logs.

**현재 코드 위치 / Current Code Location**
- `internal/browserrelay/server.go` 라인 24-25
- `internal/browserrelay/server.go` 라인 1456-1484

**SEC 상세 / SEC Details (`SEC-*`인 경우 필수 / required for `SEC-*`)**
- **취약 시나리오**: 사용자가 query token이 포함된 relay URL을 복사하거나 브라우저/프록시 로그가 URL을 저장해 토큰이 유출됩니다.
- **악용 전제**: `AllowQueryToken=true`가 설정되어 있고, 유출된 URL 또는 로그에 접근 가능한 로컬 사용자/프로세스가 있어야 합니다.
- **완화 방향**: header-only 기본 정책 유지, query token 사용 시 강한 경고와 짧은 TTL 또는 one-time token 전략 도입, 사용 흔적 최소화가 필요합니다.

**지시 사항 / Instructions**
1. query token 허용을 더 명시적인 개발 전용 모드로 제한하거나 기본 비활성 상태를 강제하십시오.
2. query token이 실제로 사용되면 경고 로그와 메트릭을 남겨 운영자가 감지할 수 있게 하십시오.
3. 가능하면 header-only 인증으로 마이그레이션하고, 브라우저 확장에는 안전한 header 주입 경로를 제공하십시오.
4. query token이 꼭 필요하다면 짧은 TTL 또는 일회용 token으로 제한하십시오.
5. URL에 토큰이 포함된 요청이 로그/에러 메시지에 남지 않도록 검토하십시오.

**완료 기준 / Completion Criteria**
- [x] query string 기반 relay 인증 사용 면적이 줄어듦
- [x] query token 사용 시 운영자가 즉시 인지할 수 있는 경고 경로가 추가됨
- [x] header-only 경로가 기본이 되고 테스트로 고정됨

**테스트 기준 / Test Criteria**
- [x] 단위 테스트: header token, query token, disabled query token 조합별 인증 결과 검증
- [x] 통합 테스트: loopback 제한과 token 전달 경로가 함께 검증됨
- [x] 회귀 테스트: 기존 브라우저 확장 연결이 승인된 방식으로 계속 동작함을 확인

---

### WO-007: LLM 클라이언트 Ask() 중복 제거

**분류 코드 / Classification Code**: `DUP-001`
**유형 / Type**: Extract Function / Embed via Composition
**심각도 / Severity**: 낮음
**상태 / Status**: 완료, PR [#80](https://github.com/devlikebear/tars/pull/80)
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `Ask()` 메서드가 5개 LLM 클라이언트(`AnthropicClient`, `OpenAICompatibleClient`, `GeminiNativeClient`, `OpenAICodexClient`, `ClaudeCodeCLIClient`)에서 완전히 동일한 구현으로 복사되어 있음.
**소스 이슈 / Source Issue**: `DUP-001`
**영향 범위 / Impact Area**: `internal/llm/`

**문제 설명 / Problem**
모든 클라이언트의 `Ask()` 구현이 동일:
```go
func (c *XClient) Ask(ctx context.Context, prompt string) (string, error) {
    resp, err := c.Chat(ctx, []ChatMessage{{Role: "user", Content: prompt}}, ChatOptions{})
    if err != nil { return "", err }
    return resp.Message.Content, nil
}
```

**현재 코드 위치 / Current Code Location**
- `internal/llm/anthropic.go` 라인 96
- `internal/llm/openai_compat_client.go` 라인 289
- `internal/llm/gemini_native.go` 라인 78
- `internal/llm/openai_codex_client.go` 라인 143
- `internal/llm/claude_code_cli.go` 라인 62

**지시 사항 / Instructions**
1. `provider.go`에 `askViaChat` 헬퍼 함수를 추가하십시오:
   ```go
   func askViaChat(c Client, ctx context.Context, prompt string) (string, error) { ... }
   ```
2. 각 클라이언트의 `Ask()` 메서드를 이 헬퍼 호출로 교체하십시오.
3. 또는 `Chat`을 구현하는 인터페이스를 embedding하는 base struct를 도입하되, 기존 `Client` 인터페이스 계약을 깨뜨리지 않아야 합니다.

**완료 기준 / Completion Criteria**
- [x] `Ask()` 로직이 한 곳에만 존재
- [x] 기존 테스트 모두 통과
- [x] `Client` 인터페이스 계약 유지

**테스트 기준 / Test Criteria**
- [x] 단위 테스트: 각 클라이언트의 Ask 동작 검증
- [x] 회귀 테스트: 기존 fallback_client_test.go 통과

---

### WO-008: LLM 클라이언트 생성자 검증 로직 통합

**분류 코드 / Classification Code**: `DUP-003`
**유형 / Type**: Extract Function
**심각도 / Severity**: 낮음
**상태 / Status**: 완료, PR [#80](https://github.com/devlikebear/tars/pull/80)
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `baseURL`, `apiKey`, `model`의 빈 값 검증이 Anthropic, OpenAI, Codex 클라이언트에서 동일 패턴으로 반복됨.
**소스 이슈 / Source Issue**: `DUP-003`
**영향 범위 / Impact Area**: `internal/llm/`

**문제 설명 / Problem**
```go
if strings.TrimSpace(baseURL) == "" { return nil, fmt.Errorf("X base url is required") }
if strings.TrimSpace(apiKey) == "" { return nil, fmt.Errorf("X api key is required") }
if strings.TrimSpace(model) == "" { return nil, fmt.Errorf("X model is required") }
```
이 패턴이 3곳에서 반복.

**현재 코드 위치 / Current Code Location**
- `internal/llm/anthropic.go` 라인 28-33
- `internal/llm/openai_compat_client.go` 라인 29-37
- `internal/llm/openai_codex_client.go` 라인 95-100

**지시 사항 / Instructions**
1. `validateProviderFields(label, baseURL, apiKey, model string) error` 함수를 `provider.go` 또는 `http_utils.go`에 추가하십시오.
2. 각 클라이언트 생성자에서 이 함수를 호출하도록 변경하십시오.

**완료 기준 / Completion Criteria**
- [x] 검증 로직이 한 곳에만 존재
- [x] 에러 메시지 형식 일관성 유지

**테스트 기준 / Test Criteria**
- [x] 단위 테스트: 빈 값 입력 시 적절한 에러 반환 검증
- [x] 회귀 테스트: 기존 provider_test.go, client_config_test.go 통과

---

### WO-009: Config 구조체 관심사 분리

**분류 코드 / Classification Code**: `TIDY-005`
**유형 / Type**: Split Struct / Extract Sub-config
**심각도 / Severity**: 높음 (수정 범위가 넓어 단계적 실행 필요)
**상태 / Status**: 완료, PR [#86](https://github.com/devlikebear/tars/pull/86)
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `Config` struct가 148개 필드의 flat struct로 되어 있어 관련 없는 설정끼리 결합도가 높음. 새 기능 추가 시 `types.go`, `defaults.go`, `config_input_fields.go`를 반드시 동시 수정해야 함.
**소스 이슈 / Source Issue**: `TIDY-005`
**영향 범위 / Impact Area**: `internal/config/` 및 이를 참조하는 모든 모듈

**문제 설명 / Problem**
단일 struct에 148개 필드가 나열되어 LLM, Browser, Gateway, Vault, Tools, Usage 등 관련 없는 관심사가 혼재합니다.
새 설정 추가 시 `types.go`, `defaults.go`, `config_input_fields.go`를 동시에 수정해야 하며, 어떤 필드가 어떤 기능과 관련되는지 파악하기 어렵습니다.

**현재 코드 위치 / Current Code Location**
- `internal/config/types.go` 라인 29-148
- `internal/config/defaults.go`
- `internal/config/config_input_fields.go`

**TIDY 상세 / TIDY Details**
- **적용 룰**: 구조 변경과 동작 변경 분리 / 국소 정리 우선
- **구조 변경 단계**: `LLMConfig`, `BrowserConfig`, `GatewayConfig`, `VaultConfig`, `ToolsConfig`, `UsageConfig`, `APIConfig`, `AssistantConfig` sub-struct를 정의하고 flat 필드를 이동
- **동작 변경 단계**: 없음 (순수 구조 정리)

**지시 사항 / Instructions**
1. **1단계** (구조): sub-struct들을 `types.go`에 정의하십시오.
2. **2단계** (구조): `Config`의 flat 필드를 sub-struct로 이동하고, 기존 필드 접근 경로를 업데이트하십시오.
3. **3단계** (구조): `defaults.go`와 `config_input_fields.go`의 참조를 sub-struct 경유로 변경하십시오.
4. 한 번에 한 그룹씩 이동하여 각 단계가 되돌리기 쉬운 크기로 유지하십시오.

**완료 기준 / Completion Criteria**
- [ ] Config struct의 직접 필드 수가 20개 이하로 감소
- [ ] 모든 테스트 통과
- [ ] YAML/env 설정 로드 동작 변화 없음

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: defaults_test.go, assistant_test.go 통과
- [ ] 통합 테스트: 설정 파일 로드 및 env 오버라이드 검증
- [ ] 회귀 테스트: 기존 동작 유지 확인

---

### WO-010: dashboard.go에서 HTML/CSS/JS 템플릿 분리

**분류 코드 / Classification Code**: `TIDY-006`
**유형 / Type**: Extract File / Move
**심각도 / Severity**: 중간
**상태 / Status**: 완료, PR [#82](https://github.com/devlikebear/tars/pull/82)
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `tarsserver/dashboard.go`(974줄)에 HTML 템플릿, CSS, 자동 새로고침 JS가 Go 문자열 상수로 인라인되어 있어 프론트엔드와 백엔드 코드가 혼재됨.
**소스 이슈 / Source Issue**: `TIDY-006`
**영향 범위 / Impact Area**: `internal/tarsserver/`

**문제 설명 / Problem**
Go 코드 내에 HTML 템플릿, CSS 스타일, JavaScript 자동 새로고침 코드가 문자열 상수로 혼재되어 프론트엔드 수정 시 Go 코드를 편집해야 합니다.

**현재 코드 위치 / Current Code Location**
- `internal/tarsserver/dashboard.go` 라인 235-974

**TIDY 상세 / TIDY Details**
- **적용 룰**: 구조 변경과 동작 변경 분리
- **구조 변경 단계**: CSS, HTML 템플릿, JS를 별도 파일로 분리하고 `go:embed` 사용
- **동작 변경 단계**: 없음 (순수 파일 분리)

**지시 사항 / Instructions**
1. `internal/tarsserver/templates/` 디렉토리를 생성하십시오.
2. CSS, HTML 템플릿, JS를 각각 별도 파일로 분리하십시오.
3. `go:embed` 지시자를 사용하여 런타임에 로드하십시오.
4. 기존 `template.Must(template.New(...).Parse(...))` 호출을 `embed.FS` 기반으로 변경하십시오.

**완료 기준 / Completion Criteria**
- [ ] dashboard.go가 300줄 이하로 감소
- [ ] 대시보드 렌더링 결과 변화 없음

**테스트 기준 / Test Criteria**
- [ ] 통합 테스트: 대시보드 페이지 렌더링 검증
- [ ] 회귀 테스트: SSE 스트림 동작 검증

---

### WO-011: browserrelay/server.go 파일 분할

**분류 코드 / Classification Code**: `TIDY-007`
**유형 / Type**: Split File
**심각도 / Severity**: 중간
**상태 / Status**: 완료, PR [#92](https://github.com/devlikebear/tars/pull/92)
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `browserrelay/server.go`(1508줄)에 WebSocket 핸들링, CDP 프로토콜 변환, 인증, 상태관리가 혼재됨.
**소스 이슈 / Source Issue**: `TIDY-007`
**영향 범위 / Impact Area**: `internal/browserrelay/`

**문제 설명 / Problem**
단일 파일에 WebSocket 연결 관리, CDP 프로토콜 변환, 인증 로직, 서버 상태관리가 모두 포함되어 있어 탐색과 유지보수가 어렵습니다.

**현재 코드 위치 / Current Code Location**
- `internal/browserrelay/server.go` (1508줄 전체)

**TIDY 상세 / TIDY Details**
- **적용 룰**: 국소 정리 우선 / 작은 단계로 이동
- **구조 변경 단계**: 타입 정의 → 인증 로직 → CDP 변환 → WebSocket 관리 순으로 분리
- **동작 변경 단계**: 없음

**지시 사항 / Instructions**
1. 타입 정의와 생성자를 `types.go`로 분리하십시오.
2. 인증 로직(`relayTokenFromRequest`, `checkOrigin`)을 `auth.go`로 분리하십시오.
3. CDP 프로토콜 변환을 `cdp.go`로 분리하십시오.
4. 한 번에 한 관심사씩 분리하여 각 단계가 `go build` 통과하는 크기로 유지하십시오.

**완료 기준 / Completion Criteria**
- [ ] server.go가 400줄 이하로 감소
- [ ] 모든 파일이 300줄 이하

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: server_test.go 통과
- [ ] 회귀 테스트: WebSocket 연결 및 CDP 메시지 라우팅 동작 유지

---

### WO-012: openai_codex_client.go 관심사 분리

**분류 코드 / Classification Code**: `TIDY-008`
**유형 / Type**: Split File / Extract
**심각도 / Severity**: 중간
**상태 / Status**: 완료, PR [#90](https://github.com/devlikebear/tars/pull/90)
**선행 작업 / Prerequisite**: WO-007 (Ask 중복 제거 후)
**근거 / Evidence**: `openai_codex_client.go`(824줄)에 OAuth 인증/갱신, HTTP 전송, 응답 파싱, tool name 매핑이 혼재됨.
**소스 이슈 / Source Issue**: `TIDY-008`
**영향 범위 / Impact Area**: `internal/llm/`

**문제 설명 / Problem**
단일 파일에 OAuth credential 관리(resolve/refresh/persist), HTTP 요청 조립/전송, 응답 파싱, tool name 매핑 등 4개 이상의 관심사가 혼재합니다.

**현재 코드 위치 / Current Code Location**
- `internal/llm/openai_codex_client.go` (824줄 전체)

**TIDY 상세 / TIDY Details**
- **적용 룰**: 구조 변경과 동작 변경 분리
- **구조 변경 단계**: tool name 매핑 → credential 관리 → HTTP 전송 순으로 분리
- **동작 변경 단계**: 없음

**지시 사항 / Instructions**
1. tool name 매핑 로직을 `codex_tool_map.go`로 이동하십시오.
2. credential 관련 메서드를 `codex_auth.go`로 이동하십시오.
3. 원래 파일은 Chat/Ask 메서드와 생성자만 남기십시오.

**완료 기준 / Completion Criteria**
- [ ] openai_codex_client.go가 300줄 이하로 감소
- [ ] 기존 테스트 통과

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: openai_codex_client_test.go 통과
- [ ] 회귀 테스트: OAuth refresh 흐름 동작 유지

---

### WO-013: helpers_cron.go 함수 분할

**분류 코드 / Classification Code**: `TIDY-009`
**유형 / Type**: Extract Function
**심각도 / Severity**: 중간
**상태 / Status**: 완료, PR [#96](https://github.com/devlikebear/tars/pull/96)
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `helpers_cron.go`(807줄)의 `newCronJobRunnerWithNotify`가 반환하는 클로저가 cron 실행, 프로젝트 컨텍스트 조립, 텔레그램 알림, 텔레메트리 수집, 세션 저장 등 다중 책임을 수행.
**소스 이슈 / Source Issue**: `TIDY-009`
**영향 범위 / Impact Area**: `internal/tarsserver/`

**문제 설명 / Problem**
단일 함수 체인에 cron 실행, 프로젝트 컨텍스트 조립, 텔레그램 프롬프트 조립, 텔레메트리 수집이 혼재되어 개별 관심사를 테스트하거나 수정하기 어렵습니다.

**현재 코드 위치 / Current Code Location**
- `internal/tarsserver/helpers_cron.go` (807줄 전체)

**TIDY 상세 / TIDY Details**
- **적용 룰**: 작고 안전한 단계로 진행
- **구조 변경 단계**: 프로젝트 컨텍스트 → 텔레그램 프롬프트 → 텔레메트리 순으로 분리
- **동작 변경 단계**: 없음

**지시 사항 / Instructions**
1. 프로젝트 관련 함수를 `cron_project.go`로 이동하십시오.
2. 텔레그램 관련 함수를 `cron_telegram.go`로 이동하십시오.
3. 핵심 러너 로직만 `helpers_cron.go`에 남기십시오.

**완료 기준 / Completion Criteria**
- [ ] helpers_cron.go가 300줄 이하로 감소
- [ ] 각 분리 파일이 단일 관심사만 포함

**테스트 기준 / Test Criteria**
- [ ] 회귀 테스트: 기존 크론 실행 흐름 동작 유지

---

### WO-014: Chat() 요청 보일러플레이트 통합

**분류 코드 / Classification Code**: `DUP-002`
**유형 / Type**: Extract Function / Template Method
**심각도 / Severity**: 중간
**상태 / Status**: 미완료
**선행 작업 / Prerequisite**: WO-007, WO-008 (기초 중복 제거 후)
**근거 / Evidence**: 모든 LLM 클라이언트의 `Chat()` 메서드가 동일한 보일러플레이트를 따름: zlog.Debug → request body 생성 → buildRequest → doPreparedRequest → response 파싱.
**소스 이슈 / Source Issue**: `DUP-002`
**영향 범위 / Impact Area**: `internal/llm/`

**문제 설명 / Problem**
`anthropic.go:56-94`, `openai_compat_client.go:61-103`, `gemini_native.go:86-100`에서 로깅, 요청 생성, 전송, 응답 처리의 골격이 반복됩니다.

**현재 코드 위치 / Current Code Location**
- `internal/llm/anthropic.go` 라인 56-94
- `internal/llm/openai_compat_client.go` 라인 61-103
- `internal/llm/gemini_native.go` 라인 86-100

**지시 사항 / Instructions**
1. `chatRoundTrip(ctx, provider, url, headers, body, httpClient, streaming) (*http.Response, error)` 헬퍼를 `http_utils.go`에 추가하십시오.
2. 각 클라이언트의 Chat 메서드에서 공통 부분을 이 헬퍼로 교체하십시오.
3. 프로바이더별 차이(헤더, 요청 구조)는 매개변수로 주입하십시오.

**완료 기준 / Completion Criteria**
- [x] 요청 전송 보일러플레이트가 한 곳에만 존재
- [x] 각 Chat 메서드가 프로바이더별 차이만 포함

**테스트 기준 / Test Criteria**
- [x] 단위 테스트: 각 프로바이더 Chat 동작 검증
- [x] 통합 테스트: 스트리밍/논스트리밍 모두 동작 확인

---

### WO-015: providerAuthConfig TrimSpace 반복 정리

**분류 코드 / Classification Code**: `DUP-004`
**유형 / Type**: Extract Function
**심각도 / Severity**: 낮음
**상태 / Status**: 완료, PR [#80](https://github.com/devlikebear/tars/pull/80)
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `provider.go:193-224`에서 `strings.TrimSpace(strings.ToLower(...))` 패턴이 10회 이상 반복됨.
**소스 이슈 / Source Issue**: `DUP-004`
**영향 범위 / Impact Area**: `internal/llm/`

**문제 설명 / Problem**
`providerAuthConfig` 함수 내에서 `strings.TrimSpace(strings.ToLower(...))` 조합이 10회 이상 반복되어 가독성이 떨어지고 실수 가능성이 높습니다.

**현재 코드 위치 / Current Code Location**
- `internal/llm/provider.go` 라인 193-224

**지시 사항 / Instructions**
1. `lowerTrimmed(s string) string` 헬퍼를 추가하십시오.
2. `providerAuthConfig` 내 반복 호출을 이 헬퍼로 교체하십시오.

**완료 기준 / Completion Criteria**
- [x] 반복 패턴 제거
- [x] 동작 변화 없음

**테스트 기준 / Test Criteria**
- [x] 단위 테스트: provider_test.go 통과

---

### WO-016: Loopback 인증 fallback 경로 검토

**분류 코드 / Classification Code**: `SEC-003`
**유형 / Type**: Review / Add Guard
**심각도 / Severity**: 낮음
**상태 / Status**: 완료, PR [#88](https://github.com/devlikebear/tars/pull/88)
**선행 작업 / Prerequisite**: WO-003 (완료됨)
**근거 / Evidence**: `serverauth/middleware.go`에서 토큰 미설정 + loopback 요청 시 인증을 skip하는 경로가 존재할 가능성. 컨테이너/VM 환경에서 loopback 정의가 달라질 수 있음.
**소스 이슈 / Source Issue**: `SEC-003`
**영향 범위 / Impact Area**: `internal/serverauth/`

**SEC 상세 / SEC Details**
- **취약 시나리오**: Docker/Kubernetes 환경에서 127.0.0.1이 아닌 loopback 주소가 사용될 때 인증 bypass가 의도치 않게 동작.
- **악용 전제**: 컨테이너 네트워크 설정에 따라 다양.
- **완화 방향**: loopback 판별 로직이 Go 표준 `net.IP.IsLoopback()`을 사용하는지 확인하고, 부족하면 보강.

**지시 사항 / Instructions**
1. `middleware.go`의 loopback 판별 로직을 검토하십시오.
2. `net.IP.IsLoopback()` 사용 여부를 확인하십시오.
3. 필요 시 IPv6 loopback(`::1`) 지원도 확인하십시오.

**완료 기준 / Completion Criteria**
- [x] loopback 판별이 표준 Go API 기반으로 동작
- [x] IPv4/IPv6 모두 올바르게 처리

**테스트 기준 / Test Criteria**
- [x] 단위 테스트: 다양한 loopback 주소(127.0.0.1, ::1, 127.0.0.2)에 대한 판별 검증

---

### WO-017: Gateway executor 환경변수 주입 방어

**분류 코드 / Classification Code**: `SEC-004`
**유형 / Type**: Add Validation
**심각도 / Severity**: 높음
**상태 / Status**: 완료, PR [#90](https://github.com/devlikebear/tars/pull/90)
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `gateway/executor.go:280-292`에서 `req.RunID`, `req.SessionID`, `req.WorkspaceID` 값이 검증 없이 `key+"="+value` 형태로 환경변수에 삽입됨. 개행이나 특수문자 포함 시 환경변수 주입 가능.
**소스 이슈 / Source Issue**: `SEC-004`
**영향 범위 / Impact Area**: `internal/gateway/`

**SEC 상세 / SEC Details**
- **취약 시나리오**: 공격자가 RunID에 `\nMALICIOUS_VAR=payload` 형태의 값을 전달하여 프로세스 환경변수를 오염시킴.
- **악용 전제**: Gateway API를 통한 run 요청 권한 보유.
- **완화 방향**: ID 값에 대해 `[a-zA-Z0-9_-]` 정규식 검증 적용, `=`, `\n`, null 바이트 포함 시 거부.

**지시 사항 / Instructions**
1. `executor.go`에 `validateEnvSafeValue(value string) error` 헬퍼를 추가하십시오.
2. `RunID`, `SessionID`, `WorkspaceID`를 환경변수에 추가하기 전에 이 헬퍼로 검증하십시오.
3. 검증 실패 시 해당 환경변수를 skip하고 로그를 남기십시오.

**완료 기준 / Completion Criteria**
- [x] 특수문자 포함 ID가 환경변수에 삽입되지 않음
- [x] 정상 ID는 기존과 동일하게 동작

**테스트 기준 / Test Criteria**
- [x] 단위 테스트: 개행, `=`, null 포함 값 검증
- [x] 회귀 테스트: executor_test.go 통과

---

### WO-018: HTTP 요청 body 크기 제한 적용

**분류 코드 / Classification Code**: `SEC-005`
**유형 / Type**: Add Guard
**심각도 / Severity**: 중간
**상태 / Status**: 미완료
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `handler_project.go:79`, `handler_ops.go:67` 등 5개 이상의 핸들러에서 `json.NewDecoder(r.Body).Decode()`를 `http.MaxBytesReader` 없이 호출. 대용량 요청으로 DoS 가능.
**소스 이슈 / Source Issue**: `SEC-005`
**영향 범위 / Impact Area**: `internal/tarsserver/` 전체 API 핸들러

**SEC 상세 / SEC Details**
- **취약 시나리오**: 공격자가 수 GB JSON body를 전송하여 서버 메모리 고갈.
- **악용 전제**: API 엔드포인트 접근 가능 (인증된 상태).
- **완화 방향**: 모든 JSON 디코딩 전에 `http.MaxBytesReader(w, r.Body, maxSize)` 적용.

**지시 사항 / Instructions**
1. `handler_transport_helpers.go`에 `limitedBodyDecoder(w, r, maxBytes, dest) error` 헬퍼를 추가하십시오.
2. 모든 `json.NewDecoder(r.Body).Decode()` 호출을 이 헬퍼로 교체하십시오.
3. 기본 제한을 10MB로 설정하십시오.

**완료 기준 / Completion Criteria**
- [x] 모든 JSON 디코딩 경로에 body 크기 제한 적용
- [x] 대용량 요청 시 적절한 413 응답 반환

**테스트 기준 / Test Criteria**
- [x] 단위 테스트: 크기 초과 요청 시 에러 반환 확인
- [x] 회귀 테스트: 정상 크기 요청 동작 확인

---

### WO-019: HTTP method 검증 패턴 38회 반복 제거

**분류 코드 / Classification Code**: `DUP-005`
**유형 / Type**: Replace with Existing Helper
**심각도 / Severity**: 중간
**상태 / Status**: 미완료
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `if r.Method != http.MethodGet { http.Error(w, "method not allowed", 405); return }` 패턴이 38회 반복되지만, `handler_transport_helpers.go`에 이미 `requireMethod()` 헬퍼가 존재함.
**소스 이슈 / Source Issue**: `DUP-005`
**영향 범위 / Impact Area**: `internal/tarsserver/handler_*.go`

**문제 설명 / Problem**
기존에 존재하는 `requireMethod()` 헬퍼를 사용하지 않고 인라인 method 검증을 38회 반복하고 있습니다.

**현재 코드 위치 / Current Code Location**
- `internal/tarsserver/handler_auth.go`
- `internal/tarsserver/handler_extensions.go`
- `internal/tarsserver/handler_ops.go`
- `internal/tarsserver/handler_project.go`
- `internal/tarsserver/handler_session.go`

**지시 사항 / Instructions**
1. 인라인 method 검증을 `requireMethod(w, r, http.MethodGet)` 호출로 교체하십시오.
2. POST/PUT/DELETE에 대해서도 동일하게 적용하십시오.

**완료 기준 / Completion Criteria**
- [x] 인라인 method 검증 코드 0건
- [x] 모든 핸들러가 `requireMethod()` 사용

**테스트 기준 / Test Criteria**
- [x] 회귀 테스트: 기존 main_test.go, handler_*_test.go 통과

---

### WO-020: orchestrator.dispatchTask() 함수 분할

**분류 코드 / Classification Code**: `TIDY-010`
**유형 / Type**: Extract Function
**심각도 / Severity**: 높음
**상태 / Status**: 미완료
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `project/orchestrator.go:138-367`의 `dispatchTask()`가 230줄로 8개 이상의 책임을 수행: 워커 프로필 해석, GitHub 인증, 보드 상태 업데이트, 태스크 실행, 대기, 리포트 파싱, 게이트 검증, 활동 기록.
**소스 이슈 / Source Issue**: `TIDY-010`
**영향 범위 / Impact Area**: `internal/project/`

**문제 설명 / Problem**
단일 함수에 8개 이상의 책임이 혼재되어 이해, 테스트, 수정이 모두 어렵습니다.

**현재 코드 위치 / Current Code Location**
- `internal/project/orchestrator.go` 라인 138-367

**TIDY 상세 / TIDY Details**
- **적용 룰**: 작고 안전한 단계로 진행 / 구조 변경과 동작 변경 분리
- **구조 변경 단계**: 게이트 검증 → 리포트 파싱 → 활동 기록 순으로 함수 추출
- **동작 변경 단계**: 없음

**지시 사항 / Instructions**
1. 게이트 검증(line 255-330)을 `verifyDispatchGates()` 메서드로 추출하십시오.
2. 리포트 결과 처리(line 220-250)를 `resolveTaskFinalStatus()` 메서드로 추출하십시오.
3. 각 추출 함수는 20-50줄 범위로 유지하십시오.

**완료 기준 / Completion Criteria**
- [ ] dispatchTask()가 80줄 이하로 감소
- [ ] 추출된 함수가 각각 단일 책임만 수행

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: orchestrator_test.go 통과
- [ ] 회귀 테스트: dispatch 흐름 동작 유지

---

### WO-021: Gateway Runtime god object 분해

**분류 코드 / Classification Code**: `TIDY-011`
**유형 / Type**: Extract Struct / Composition
**심각도 / Severity**: 중간
**상태 / Status**: 미완료
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `gateway/types.go:191-221`의 `Runtime` struct가 21개 필드로 Run lifecycle, Channel messaging, Agent 관리, Browser 상태, Persistence를 모두 관리하는 god object.
**소스 이슈 / Source Issue**: `TIDY-011`
**영향 범위 / Impact Area**: `internal/gateway/`

**문제 설명 / Problem**
단일 struct가 5개 이상의 관심사를 관리하여 필드 간 의존 관계 파악이 어렵고, mutex 범위가 불명확합니다.

**현재 코드 위치 / Current Code Location**
- `internal/gateway/types.go` 라인 191-221

**TIDY 상세 / TIDY Details**
- **적용 룰**: 국소 정리 우선 / 작은 단계로 이동
- **구조 변경 단계**: Browser → Persistence → Channel 순으로 sub-struct 추출
- **동작 변경 단계**: 없음

**지시 사항 / Instructions**
1. Browser 관련 필드를 `runtimeBrowserState` sub-struct로 추출하십시오.
2. Persistence 관련 필드를 `runtimePersistence` sub-struct로 추출하십시오.
3. Channel 관련 필드를 `runtimeChannelState` sub-struct로 추출하십시오.
4. mutex 범위가 변경되지 않도록 주의하십시오.

**완료 기준 / Completion Criteria**
- [ ] Runtime struct의 직접 필드가 12개 이하로 감소
- [ ] 각 sub-struct가 단일 관심사만 포함

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: runtime_test.go 통과
- [ ] 회귀 테스트: 전체 gateway 동작 유지

---

### WO-022: Tool 파라미터 bounds 체크 헬퍼 추출

**분류 코드 / Classification Code**: `DUP-006`
**유형 / Type**: Extract Function
**심각도 / Severity**: 낮음
**상태 / Status**: 미완료
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `read_file.go:58-67`, `list_dir.go:55-64`, `glob.go:57-66`, `memory_search.go:64-73`, `web_fetch.go:80-89`, `exec.go:87-92`에서 동일한 default→override→min/max clamp 패턴이 반복됨.
**소스 이슈 / Source Issue**: `DUP-006`
**영향 범위 / Impact Area**: `internal/tool/`

**문제 설명 / Problem**
6개 tool에서 파라미터 범위 제한 로직이 동일한 패턴으로 반복됩니다.

**현재 코드 위치 / Current Code Location**
- `internal/tool/read_file.go` 라인 58-67
- `internal/tool/list_dir.go` 라인 55-64
- `internal/tool/glob.go` 라인 57-66
- `internal/tool/memory_search.go` 라인 64-73
- `internal/tool/web_fetch.go` 라인 80-89
- `internal/tool/exec.go` 라인 87-92

**지시 사항 / Instructions**
1. `tool.go`에 `boundedInt(val *int, defaultVal, minVal, maxVal int) int` 헬퍼를 추가하십시오.
2. 6개 tool의 파라미터 bounds 체크를 이 헬퍼로 교체하십시오.

**완료 기준 / Completion Criteria**
- [x] bounds 체크 로직이 한 곳에만 존재
- [x] 모든 tool의 기존 동작 유지

**테스트 기준 / Test Criteria**
- [x] 단위 테스트: 헬퍼 함수의 경계값 테스트
- [x] 회귀 테스트: 각 tool의 기존 테스트 통과

---

## 켄트 백(Tidy First) 룰 매핑 테이블 / Kent Beck (Tidy First) Rule Mapping

| WO ID | 적용 룰 | 기대 효과 |
|-------|---------|-----------|
| WO-001 | 중복을 통제 가능한 단위로 모은다 | 설정 키 추가 시 수정 지점 축소 |
| WO-002 | 구조 변경과 동작 변경을 분리한다 | workflow 회귀와 리뷰 난이도 감소 |
| WO-004 | 국소 정리(local tidy)를 우선한다 | 대시보드 섹션 추가 비용 절감 |
| WO-005 | 구조 변경과 동작 변경을 분리한다 | provider 인증 진화 시 영향 반경 축소 |
| WO-007 | 중복을 통제 가능한 단위로 모음 | LLM 클라이언트 유지보수 비용 감소 |
| WO-008 | 중복을 통제 가능한 단위로 모음 | 새 프로바이더 추가 시 보일러플레이트 제거 |
| WO-009 | 구조 변경과 동작 변경 분리 | 설정 관련 변경 범위 축소 |
| WO-010 | 구조 변경과 동작 변경 분리 | 프론트엔드/백엔드 독립 수정 가능 |
| WO-011 | 작은 단계로 이동 | 코드 탐색 시간 감소 |
| WO-012 | 국소 정리 우선 | Codex 클라이언트 이해도 향상 |
| WO-013 | 작은 단계로 이동 | 크론 관련 코드 탐색 용이 |
| WO-014 | 중복을 통제 가능한 단위로 모음 | 프로바이더 추가 비용 감소 |
| WO-015 | 중복을 통제 가능한 단위로 모음 | 반복 패턴 제거, 가독성 향상 |
| WO-019 | 중복을 통제 가능한 단위로 모음 | 핸들러 일관성 확보 |
| WO-020 | 작고 안전한 단계로 진행 | 오케스트레이터 이해도/테스트 용이성 향상 |
| WO-021 | 국소 정리 우선 | Gateway 모듈 관심사 명확화 |
| WO-022 | 중복을 통제 가능한 단위로 모음 | Tool 코드 일관성 확보 |

---

## 리팩토링 순서 (Recommended Order) / Recommended Order

의존 관계 없는 작업은 병렬 실행 가능합니다. 보안 이슈를 최우선으로 처리합니다.
Independent tasks can run in parallel. Security issues take highest priority.

```text
━━ Phase 1: 보안 (완료) ━━
SEC-006 (Codex refresh token 저장)     ← 완료

━━ Phase 2: 중복 제거 (완료) ━━
WO-019 (DUP-005: HTTP method)          ← 완료

━━ Phase 3: 구조 정리 (중기) ━━
WO-020 (TIDY-010: dispatchTask 분할)   ← 독립, 높음
WO-009 (TIDY-005: Config 분리)         ← 독립, 큰 범위
WO-010 (TIDY-006: dashboard 분리)      ← 독립
WO-011 (TIDY-007: relay 분할)          ← 독립
WO-012 (TIDY-008: codex 분할)          ← 독립
WO-013 (TIDY-009: cron 분할)           ← 독립
WO-021 (TIDY-011: Runtime 분해)        ← 독립
TIDY-012 (project_runner run loop)     ← 별도 WO 없이 이슈 단위 추적
```

---

## 체크포인트 진행 로그 (Checkpoint Progress Log) / Checkpoint Progress Log

중단/재개 가능하도록 아래 표를 반드시 유지합니다.
Keep this table updated so the work can be paused and resumed safely.

| Checkpoint | 상태 | 변경된 WO | DUP/SEC/TIDY 진행 현황 | 메모 |
|------------|------|-----------|-------------------------|------|
| checkpoint-001 | completed | WO-001 ~ WO-006 초안 확정 | DUP 0/0, SEC 2/2, TIDY 4/4 | `issue-candidates.md` 6건을 committed source 기준으로 재검증 완료 |
| checkpoint-002 | completed | WO-007 ~ WO-022 추가 | DUP 0/7, SEC 0/5, TIDY 0/8 | `internal/` 전체 BFS 분석, 20개 신규 이슈 추가 |
| checkpoint-003 | completed | WO-007, WO-008, WO-014, WO-015, WO-017, WO-022 및 DUP-007/SEC-007 반영 | DUP 6/7, SEC 4/7, TIDY 4/12 | PR #80, #82, #84, #86, #88 merged 기준으로 복원본 문서 상태 동기화 |
| checkpoint-004 | completed | WO-016, WO-018 반영 | DUP 6/7, SEC 6/7, TIDY 4/12 | PR #90, #92 merged 기준으로 body limit/loopback 검토 상태 동기화 |
| checkpoint-005 | completed | SEC-006, WO-019 반영 | DUP 7/7, SEC 7/7, TIDY 4/12 | PR #94, #96 merged 기준으로 Codex refresh token 저장/handler method 중복 상태 동기화 |
| checkpoint-006 | completed | WO-020 반영 | DUP 7/7, SEC 7/7, TIDY 5/12 | PR #98 merged 기준으로 `dispatchTask()` helper 추출 상태 동기화 |

---

## 다음 세션 재개 지점 (Resume Point) / Resume Point

- **다음 시작 WO**: `TIDY-012` (`project_runner.go` run loop 분할) — 다음 구조 정리 작업
- **남은 선행 작업**: `없음`
- **우선 재검토 파일**: `internal/project/project_runner.go`, `internal/project/project_runner_test.go`
- **재개 순서 / Resume Steps**
  1. 마지막 체크포인트 로그 행 확인
  2. `project_runner.go`의 run loop에서 board 상태별 분기와 retry/wait 로직을 분리할 helper 후보를 식별
  3. `internal/project/project_runner_test.go`에 상태 전이 회귀를 먼저 고정
  4. 실제 구현 세션에서는 helper 추출 후 `go test ./internal/project`로 동작 보존 확인

---

## 완료 검증 체크리스트 / Completion Verification Checklist

- [ ] `make test` 전체 통과
- [ ] `make lint` 경고 없음 (새로 추가된 파일 포함)
- [ ] 변경된 파일의 최대 줄 수가 300줄 이하
- [x] 이슈 목록의 모든 항목이 `DUP-*`, `SEC-*`, `TIDY-*` 중 하나로 분류됨
- [x] 주요 구현 항목은 WO로 정리됨 (SEC-006, TIDY-012는 별도 WO 없이 이슈 단위로 추적)
- [x] `SEC-*` 항목마다 취약 시나리오/악용 전제/완화 방향이 기재됨
- [x] `TIDY-*` 항목마다 적용 룰과 단계 분리가 기재됨
