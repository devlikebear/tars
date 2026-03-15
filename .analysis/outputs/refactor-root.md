# 리팩토링 지시서: 저장소 핵심 운영 경로
# Refactoring Work Order: Repository Core Runtime Paths

> **생성일 / Created At**: 2026-03-15
> **세션 ID / Session ID**: `refactor-guide-20260315-095000`
> **체크포인트 / Checkpoint**: `checkpoint-001`
> **대상 / Target**: `internal/config`, `internal/project`, `internal/tarsserver`, `internal/auth`, `internal/llm`, `internal/browserrelay`
> **분석 범위 / Analysis Scope**: 이슈 후보 역추적 + 관련 파일 직접 검증 / issue-candidate verification with direct source inspection
> **우선순위 / Priority**: 높음 / High
> **소스 / Source**: `issue-candidates.md` 검증 / verified from `issue-candidates.md`

---

## 요약 (Summary) / Summary

이 지시서는 저장소 핵심 운영 경로에서 발견된 코드 품질/보안/구조 문제를 해결하기 위한 리팩토링 작업을 정의합니다.
총 6개의 이슈가 발견되었으며 (`DUP` 0건 / `SEC` 2건 / `TIDY` 4건), 아래 작업 목록을 순서대로 실행하십시오.
This work order defines the refactoring tasks required to address code quality, security, and structural issues found in the repository's core runtime paths.
There are 6 issues in total (`DUP` 0 / `SEC` 2 / `TIDY` 4), and the work orders below should be executed in sequence.

---

## 발견된 이슈 목록 (Issues Found) / Issues Found

| # | 분류 코드 | 이슈 유형 | 위치 | 근거 | 위험도 | 영향 범위 |
|---|-----------|-----------|------|------|--------|-----------|
| 1 | `TIDY-001` | 설정 스키마 매핑 중복 | `internal/config/defaults_apply.go:13-144`, `internal/config/env.go:8-320`, `internal/config/yaml.go:12-263`, `internal/config/merge.go:3-280` | 같은 `Config` 필드 집합을 입력원별로 반복 매핑 | 중간 | 설정 로딩 전역 |
| 2 | `TIDY-002` | 프로젝트 workflow 전이 분산 | `internal/tarsserver/handler_chat.go:26-45`, `internal/project/brief_state.go:128-280`, `internal/project/orchestrator.go:72-136`, `internal/project/project_runner.go:208-320`, `internal/tarsserver/handler_project.go:460-523` | kickoff, brief/state 저장, dispatch, autopilot 루프가 별도 규칙으로 흩어짐 | 높음 | 프로젝트 시작, 자동 실행, 대시보드 |
| 3 | `SEC-001` | 대시보드 공개 모드 오사용 위험 | `internal/tarsserver/middleware.go:26-33` | `dashboard_auth_mode=off` 시 `/dashboards`, `/ui/projects/*` 전체가 인증 skip path가 됨 | 높음 | 프로젝트 메타데이터 노출 |
| 4 | `TIDY-003` | 대시보드 섹션 정의 중복 | `internal/tarsserver/dashboard.go:214-406`, `internal/tarsserver/dashboard.go:416-428`, `internal/tarsserver/dashboard.go:573-586` | 섹션 ID, refresh 대상, 서버 데이터 조립이 암묵적으로 묶여 있음 | 중간 | 대시보드 렌더링/추가 개발 |
| 5 | `TIDY-004` | Provider credential lifecycle 분산 | `internal/auth/token.go:18-97`, `internal/auth/codex_oauth.go:48-247`, `internal/llm/provider.go:118-140`, `internal/llm/openai_codex_client.go:171-185`, `internal/llm/model_lister.go:165-193` | 토큰 해석, provider 특화 credential, refresh retry가 여러 계층에 분산 | 중간 | LLM provider onboarding, 인증 회복 |
| 6 | `SEC-002` | Browser relay query token 노출 가능성 | `internal/browserrelay/server.go:24-25`, `internal/browserrelay/server.go:1456-1484` | opt-in 이지만 query string의 `token` / `relay_token`을 그대로 인증값으로 허용 | 중간 | 로컬 브라우저 relay 인증 |

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
- [ ] 동일 설정 필드 매핑 로직이 입력원별 switch/if 중복 없이 공통 메타데이터를 통해 정의됨
- [ ] YAML/env/default/merge 경로가 같은 정규화 규칙을 재사용함
- [ ] 새 설정 키 추가 절차가 기존보다 명확하고 짧아짐

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: 대표 설정 키에 대해 default, YAML, env, merge precedence 검증
- [ ] 통합 테스트: `Load` 경로에서 기존 설정 파일과 env override가 동일하게 동작하는지 검증
- [ ] 회귀 테스트: bool/list/int/string 타입별 기존 파싱 결과 유지 확인

---

### WO-002: 프로젝트 workflow 전이를 명시적 정책/상태기계로 분리

**분류 코드 / Classification Code**: `TIDY-002`
**유형 / Type**: Extract Policy / State Machine / Move Logic
**심각도 / Severity**: 높음
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
- [ ] 프로젝트 workflow 전이가 선언적 정책 또는 상태기계로 한 곳에 모임
- [ ] chat/API/autopilot이 같은 전이 규칙을 재사용함
- [ ] 새 phase 또는 retry rule 변경 시 수정 지점이 명확히 줄어듦

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: kickoff 판정, state normalization, dispatchable stage, retry rule 검증
- [ ] 통합 테스트: brief finalize -> dispatch -> review -> done 흐름 검증
- [ ] 회귀 테스트: stalled task recovery, empty board seed, review-required task 경로 검증

---

### WO-003: 대시보드 공개 모드에 방어적 가드를 추가

**분류 코드 / Classification Code**: `SEC-001`
**유형 / Type**: Harden Default / Add Guard / Add Warning
**심각도 / Severity**: 높음
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
- [ ] 인증 비활성화만으로 외부 공개가 즉시 성립하지 않음
- [ ] 위험한 공개 모드 활성화 시 운영 경고가 명확히 남음
- [ ] dashboard 접근 정책이 테스트와 문서에서 재현 가능하게 설명됨

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: loopback/non-loopback 환경에서 skip path 정책 검증
- [ ] 통합 테스트: `dashboard_auth_mode=off`와 보호 플래그 조합별 HTTP status 검증
- [ ] 회귀 테스트: 기본 설정과 기존 인증 on 경로가 변하지 않음 확인

---

### WO-004: 대시보드 섹션 레지스트리를 도입해 서버/클라이언트 정의를 단일화

**분류 코드 / Classification Code**: `TIDY-003`
**유형 / Type**: Extract Constant / Introduce Registry / Split Template Data
**심각도 / Severity**: 중간
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
- [ ] 섹션 ID와 refresh 대상 목록이 단일 레지스트리에서 관리됨
- [ ] 새 섹션 추가 시 템플릿/스크립트/서버를 각각 따로 수정하지 않아도 됨
- [ ] 기존 대시보드 렌더 결과와 refresh 동작이 유지됨

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: registry 기반 섹션 목록과 data builder 매핑 검증
- [ ] 통합 테스트: dashboard HTML 렌더와 refresh fetch가 같은 섹션 ID 집합을 사용하는지 검증
- [ ] 회귀 테스트: 기존 8개 섹션이 동일 순서 또는 승인된 새 순서로 출력됨을 확인

---

### WO-005: Provider credential resolution과 refresh를 전략 레지스트리로 통합

**분류 코드 / Classification Code**: `TIDY-004`
**유형 / Type**: Extract Strategy / Introduce Registry / Move Logic
**심각도 / Severity**: 중간
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
- [ ] provider별 인증 lifecycle이 명시적 전략/레지스트리로 표현됨
- [ ] chat client와 model fetcher가 같은 refresh/persist 규약을 재사용함
- [ ] 새 provider 추가 시 인증 관련 수정 지점이 예측 가능하게 줄어듦

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: provider별 resolve/refresh/persist 전략 검증
- [ ] 통합 테스트: `openai-codex`의 chat unauthorized retry와 model list retry가 동일 계약으로 동작하는지 검증
- [ ] 회귀 테스트: `anthropic`, `gemini`, `openai`, `openai-codex` 기존 인증 경로 유지 확인

---

### WO-006: Browser relay query token 경로를 축소하고 운영 경고를 추가

**분류 코드 / Classification Code**: `SEC-002`
**유형 / Type**: Reduce Attack Surface / Add Guard / Add Warning
**심각도 / Severity**: 중간
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
- [ ] query string 기반 relay 인증 사용 면적이 줄어듦
- [ ] query token 사용 시 운영자가 즉시 인지할 수 있는 경고 경로가 추가됨
- [ ] header-only 경로가 기본이 되고 테스트로 고정됨

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: header token, query token, disabled query token 조합별 인증 결과 검증
- [ ] 통합 테스트: loopback 제한과 token 전달 경로가 함께 검증됨
- [ ] 회귀 테스트: 기존 브라우저 확장 연결이 승인된 방식으로 계속 동작함을 확인

---

## 켄트 백(Tidy First) 룰 매핑 테이블 / Kent Beck (Tidy First) Rule Mapping

| WO ID | 적용 룰 | 기대 효과 |
|-------|---------|-----------|
| WO-001 | 중복을 통제 가능한 단위로 모은다 | 설정 키 추가 시 수정 지점 축소 |
| WO-002 | 구조 변경과 동작 변경을 분리한다 | workflow 회귀와 리뷰 난이도 감소 |
| WO-004 | 국소 정리(local tidy)를 우선한다 | 대시보드 섹션 추가 비용 절감 |
| WO-005 | 구조 변경과 동작 변경을 분리한다 | provider 인증 진화 시 영향 반경 축소 |

---

## 리팩토링 순서 (Recommended Order) / Recommended Order

의존 관계 없는 작업은 병렬 실행 가능합니다.
Work orders without dependencies can be executed in parallel.

```text
WO-003 (대시보드 공개 모드 방어)
WO-006 (relay query token 축소)
WO-001 (설정 매핑 메타데이터화)
WO-005 (credential lifecycle registry)
WO-002 (project workflow 상태기계화)
WO-004 (dashboard section registry)
```

---

## 체크포인트 진행 로그 (Checkpoint Progress Log) / Checkpoint Progress Log

중단/재개 가능하도록 아래 표를 반드시 유지합니다.
Keep this table updated so the work can be paused and resumed safely.

| Checkpoint | 상태 | 변경된 WO | DUP/SEC/TIDY 진행 현황 | 메모 |
|------------|------|-----------|-------------------------|------|
| checkpoint-001 | completed | WO-001 ~ WO-006 초안 확정 | DUP 0/0, SEC 2/2, TIDY 4/4 | `issue-candidates.md` 6건을 committed source 기준으로 재검증 완료 |

---

## 다음 세션 재개 지점 (Resume Point) / Resume Point

- **다음 시작 WO**: `없음 (completed)`
- **남은 선행 작업**: `없음`
- **우선 재검토 파일**: `없음`
- **재개 순서 / Resume Steps**
  1. 마지막 체크포인트 로그 행 확인
  2. 구현 대상 WO를 하나 선택하고 "지시 사항 / Instructions"를 작업 단위로 분해
  3. 실제 구현 세션에서는 선택한 WO에 맞는 실패 테스트를 먼저 추가

---

## 완료 검증 체크리스트 / Completion Verification Checklist

- [ ] `make test` 전체 통과
- [ ] `make lint` 경고 없음 (새로 추가된 파일 포함)
- [ ] 변경된 파일의 최대 줄 수가 300줄 이하
- [x] 이슈 목록의 모든 항목이 `DUP-*`, `SEC-*`, `TIDY-*` 중 하나로 분류됨
- [x] 이슈 목록의 모든 항목이 WO로 처리됨
- [x] `SEC-*` 항목마다 취약 시나리오/악용 전제/완화 방향이 기재됨
- [x] `TIDY-*` 항목마다 적용 룰과 단계 분리가 기재됨
