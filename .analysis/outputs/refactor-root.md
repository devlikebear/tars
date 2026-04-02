# 리팩토링 지시서: 현재 헤드 핵심 리팩토링 경로
# Refactoring Work Order: Current-Head Core Refactoring Paths

> **생성일 / Created At**: 2026-04-02
> **세션 ID / Session ID**: `refactor-guide-20260402-131107`
> **체크포인트 / Checkpoint**: `checkpoint-001`
> **대상 / Target**: `internal/project`, `internal/memory`, `internal/skillhub`, `internal/tarsserver`
> **분석 범위 / Analysis Scope**: `issue-candidates.md` 역추적 + 관련 파일 직접 검증 / issue-candidate verification with direct source inspection
> **우선순위 / Priority**: 높음 / High
> **소스 / Source**: `issue-candidates.md` 검증 / verified from `issue-candidates.md`

---

## 요약 (Summary) / Summary

이 지시서는 현재 헤드에서 확인된 구조/보안/운영 신뢰성 문제를 해결하기 위한 리팩토링 작업을 정의합니다.
총 4개의 유효 이슈가 발견되었으며 (`DUP` 0건 / `SEC` 1건 / `TIDY` 3건), 아래 작업 목록을 순서대로 실행하십시오.
This work order defines the refactoring tasks required to address structural, security, and runtime reliability issues verified on the current HEAD.
There are 4 validated issues in total (`DUP` 0 / `SEC` 1 / `TIDY` 3), and the work orders below should be executed in sequence.

추가 검증 메모:
기존 후보 `SEC-002`는 현재 헤드에서 false positive로 판단했습니다. `browserrelay` 서버 소스는 저장소에서 제거되었고, `/browser relay` 경로도 [internal/tarsclient/commands_gateway_browser.go](internal/tarsclient/commands_gateway_browser.go#L161)에서 제거 안내만 남아 있어 본 지시서 범위에서 제외합니다.

---

## 발견된 이슈 목록 (Issues Found) / Issues Found

| # | 분류 코드 | 이슈 유형 | 위치 | 근거 | 위험도 | 영향 범위 |
|---|-----------|-----------|------|------|--------|-----------|
| 1 | `TIDY-002` | 프로젝트 workflow 전이 분산 | `internal/tarsserver/handler_chat.go:37-58`, `internal/project/brief_state.go:187-236`, `internal/project/orchestrator.go:79-176`, `internal/project/orchestrator_plan.go:12-49`, `internal/tarsserver/helpers_project_progress.go:18-130`, `internal/tarsserver/handler_project.go:112-141` | kickoff, brief finalize, planning, dispatch, autonomous progress, API state sync가 별도 규칙으로 분산 | 높음 | 프로젝트 시작/실행/자동화 전반 |
| 2 | `TIDY-005` | semantic embed provider 표면-구현 불일치 | `internal/config/schema.go:64-69`, `internal/config/defaults_apply.go:179-196`, `internal/memory/semantic.go:29-57`, `internal/memory/semantic.go:152-175`, `cmd/tars/doctor_main.go:162-175` | 설정 표면은 `openai`까지 허용하지만 실제 embedder 생성은 `gemini`만 지원하고 doctor도 미검증 | 중간 | semantic memory 설정/운영 |
| 3 | `SEC-003` | Skill/Plugin 원격 설치 무결성 검증 부재 | `internal/skillhub/types.go:11-19`, `internal/skillhub/types.go:22-26`, `internal/skillhub/registry.go:185-209`, `internal/skillhub/install.go:45-96`, `internal/skillhub/install.go:229-262` | MCP는 checksum 검증을 하지만 skill/plugin은 raw 다운로드 후 바로 기록 | 높음 | 원격 skill/plugin 설치 경로 |
| 4 | `TIDY-006` | Skill/Plugin 설치가 부분 성공 상태를 기록 | `internal/skillhub/install.go:56-87`, `internal/skillhub/install.go:145-175`, `internal/skillhub/install.go:241-261`, `internal/skillhub/install.go:300-333` | best-effort `continue`가 많고 전체 payload 검증 없이 DB/version이 갱신됨 | 중간 | Hub 설치/업데이트 신뢰성 |

---

## 작업 지시서 (Work Orders) / Work Orders

각 작업은 독립적으로 실행 가능해야 합니다.
의존 관계가 있는 경우 "선행 작업" 필드를 채웁니다.
Each work order should be executable independently.
If there is a dependency, fill in the "Prerequisite" field.

---

### WO-001: 프로젝트 workflow 전이를 단일 runtime/state 모델로 끌어올리기

**분류 코드 / Classification Code**: `TIDY-002`
**유형 / Type**: Extract State Machine / Introduce Runtime Policy / Move Logic
**심각도 / Severity**: 높음 / High
**선행 작업 / Prerequisite**: 없음 / None
**근거 / Evidence**: kickoff 세션 분기와 skill routing은 [internal/tarsserver/handler_chat.go](internal/tarsserver/handler_chat.go#L37), brief/state 초기화는 [internal/project/brief_state.go](internal/project/brief_state.go#L187), planning은 [internal/project/orchestrator_plan.go](internal/project/orchestrator_plan.go#L12), dispatch는 [internal/project/orchestrator.go](internal/project/orchestrator.go#L79), heartbeat 기반 autonomous progress는 [internal/tarsserver/helpers_project_progress.go](internal/tarsserver/helpers_project_progress.go#L18), API state sync는 [internal/tarsserver/handler_project.go](internal/tarsserver/handler_project.go#L112)와 [internal/tarsserver/handler_project.go](internal/tarsserver/handler_project.go#L752)에 흩어져 있습니다.
**소스 이슈 / Source Issue**: `TIDY-002`
**영향 범위 / Impact Area**: 프로젝트 kickoff, brief finalize, planning, dispatch, autonomous progress, project API

**문제 설명 / Problem**
현재 프로젝트 workflow는 여러 계층이 같은 상태 전이를 암묵적으로 공유한다고 가정합니다.
그 결과 새 phase 규칙, blocked/review 처리, project API 상태 동기화가 추가될 때 chat/API/autonomous 루프 사이에 동작 불일치가 생기기 쉽습니다.
The current project workflow assumes multiple layers share the same transitions implicitly.
That makes chat, API, and autonomous execution drift-prone whenever phases, retries, or review rules evolve.

**현재 코드 위치 / Current Code Location**
- `internal/tarsserver/handler_chat.go` 라인 37-58, 353-362
- `internal/project/brief_state.go` 라인 187-236, 239-270
- `internal/project/orchestrator.go` 라인 79-176
- `internal/project/orchestrator_plan.go` 라인 12-49
- `internal/tarsserver/helpers_project_progress.go` 라인 18-130
- `internal/tarsserver/handler_project.go` 라인 112-141, 752-769
- `internal/project/workflow_runtime_policy.go` 라인 18-40

**TIDY 상세 / TIDY Details (`TIDY-*`인 경우 필수 / required for `TIDY-*`)**
- **적용 룰**: `구조 변경과 동작 변경을 분리한다`, `작고 안전한 단계로 진행한다`, `동작 보존을 먼저 보장한다`
- **구조 변경 단계**: project 상태 전이와 runtime event를 표현하는 전용 타입 또는 state machine을 `internal/project` 아래에 추가하고, 현재 각 레이어가 가진 조건식을 여기에 모읍니다.
- **동작 변경 단계**: 구조 통합 후에만 planning timeout, retry 전략, phase naming, 자동 복구 규칙을 조정합니다.

**지시 사항 / Instructions**
1. `kickoff`, `brief_finalized`, `planning_requested`, `dispatch_requested`, `phase_completed`, `project_archived` 같은 이벤트를 처리하는 공통 workflow runtime 타입을 추가하십시오.
2. [internal/tarsserver/handler_chat.go](internal/tarsserver/handler_chat.go), [internal/project/brief_state.go](internal/project/brief_state.go), [internal/project/orchestrator.go](internal/project/orchestrator.go), [internal/project/orchestrator_plan.go](internal/project/orchestrator_plan.go), [internal/tarsserver/helpers_project_progress.go](internal/tarsserver/helpers_project_progress.go), [internal/tarsserver/handler_project.go](internal/tarsserver/handler_project.go)가 그 공통 runtime 결과를 사용하도록 바꾸십시오.
3. `policy.go`의 tool policy와 workflow policy/runtime policy는 별도 타입으로 유지하되, workflow 해석 결과가 API/chat/autonomous 경로에 동일하게 주입되도록 경계를 정리하십시오.
4. `FinalizeBrief`, `PlanTasks`, `DispatchTodo/DispatchReview`, `advanceAutonomousProject`가 raw 문자열 상태를 직접 해석하지 않도록 하십시오.
5. 구조 정리 완료 전에는 phase semantics나 retry 동작을 바꾸지 말고, 먼저 동작 보존 테스트를 고정하십시오.

**완료 기준 / Completion Criteria**
- [ ] 프로젝트 workflow 전이 규칙이 단일 runtime/state 모델로 모임
- [ ] chat/API/autonomous 경로가 동일한 transition 결과를 재사용함
- [ ] 새로운 phase/retry 규칙 추가 시 수정 지점이 명확히 줄어듦

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: kickoff 판정, brief 초기화, planning event, dispatchable stage, archive sync 검증
- [ ] 통합 테스트: brief finalize -> planning -> dispatch -> review -> done 흐름 검증
- [ ] 회귀 테스트: active brief 재개, empty board planning, archived -> done sync, stalled autonomous 경로 검증

---

### WO-002: semantic embed provider 표면과 실제 구현 범위를 일치시키기

**분류 코드 / Classification Code**: `TIDY-005`
**유형 / Type**: Introduce Provider Registry / Add Validation / Fail Fast
**심각도 / Severity**: 중간 / Medium
**선행 작업 / Prerequisite**: 없음 / None
**근거 / Evidence**: [internal/config/schema.go](internal/config/schema.go#L64)는 `memory_embed_provider`에 `gemini`, `openai`를 모두 허용하지만, [internal/memory/semantic.go](internal/memory/semantic.go#L159)의 `NewService`는 `gemini`일 때만 embedder를 생성합니다. [cmd/tars/doctor_main.go](cmd/tars/doctor_main.go#L162)은 LLM 자격증명만 확인하고 semantic embed provider 유효성은 검사하지 않습니다.
**소스 이슈 / Source Issue**: `TIDY-005`
**영향 범위 / Impact Area**: semantic memory 설정, startup validation, 운영 진단

**문제 설명 / Problem**
현재 semantic memory 설정 표면은 generic provider abstraction처럼 보이지만 실제 구현은 단일 provider에 묶여 있습니다.
이 상태에서는 사용자가 지원되지 않는 provider를 설정해도 조용히 semantic memory가 비활성화되어 운영 디버깅 비용이 커집니다.
The semantic memory configuration surface currently looks generic while the implementation is single-provider.
That mismatch allows unsupported configurations to fail silently and increases operational debugging cost.

**현재 코드 위치 / Current Code Location**
- `internal/config/schema.go` 라인 64-69
- `internal/config/defaults_apply.go` 라인 179-196
- `internal/memory/semantic.go` 라인 29-57, 152-175
- `cmd/tars/doctor_main.go` 라인 162-175, 221-231

**TIDY 상세 / TIDY Details (`TIDY-*`인 경우 필수 / required for `TIDY-*`)**
- **적용 룰**: `구조 변경과 동작 변경을 분리한다`, `의도를 드러내는 이름을 사용한다`, `동작 보존을 먼저 보장한다`
- **구조 변경 단계**: semantic embedder constructor registry를 도입하거나, 지원 provider 집합을 명시적으로 정의해 config/schema/doctor가 같은 목록을 보게 만듭니다.
- **동작 변경 단계**: registry 도입 후에만 실제 OpenAI embedder 추가 또는 unsupported provider hard-fail 정책을 적용합니다.

**지시 사항 / Instructions**
1. `memory.SemanticConfig`와 `memory.NewService`가 참조할 embed provider registry를 추가하십시오.
2. 현재 지원하는 provider가 `gemini`뿐이라면 config schema와 doctor가 같은 사실을 명시적으로 드러내고, unsupported provider 설정 시 실패 또는 강한 경고를 내도록 하십시오.
3. `memory_embed_provider` 허용 목록과 runtime constructor 목록이 분리되지 않게 공통 소스를 만드십시오.
4. semantic memory가 `enabled=true`인데 provider가 지원되지 않는 경우, startup 또는 `doctor`에서 원인을 바로 알 수 있게 하십시오.
5. 이후 다른 embed provider를 추가할 때는 registry 확장만으로 끝나도록 인터페이스를 정리하십시오.

**완료 기준 / Completion Criteria**
- [ ] config/schema/doctor/runtime이 동일한 supported provider 집합을 사용함
- [ ] 지원되지 않는 provider 설정 시 조용한 비활성화 대신 명시적 실패 또는 경고가 발생함
- [ ] 새 embed provider 추가 절차가 registry 확장 수준으로 단순화됨

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: supported/unsupported provider 정규화와 registry lookup 검증
- [ ] 통합 테스트: semantic memory enabled + unsupported provider 설정 시 doctor/startup 동작 검증
- [ ] 회귀 테스트: `gemini` 설정에서 기존 semantic indexing/search 동작 유지 확인

---

### WO-003: Skill/Plugin 설치 경로에 무결성 검증을 도입하기

**분류 코드 / Classification Code**: `SEC-003`
**유형 / Type**: Add Integrity Verification / Harden Supply Chain / Fail Closed
**심각도 / Severity**: 높음 / High
**선행 작업 / Prerequisite**: 없음 / None
**근거 / Evidence**: [internal/skillhub/types.go](internal/skillhub/types.go#L12)와 [internal/skillhub/types.go](internal/skillhub/types.go#L29)에서 plugin/skill 파일 목록은 checksum 없는 `[]string`인데, MCP만 [internal/skillhub/types.go](internal/skillhub/types.go#L41)에서 `[]RegistryFile`을 사용합니다. [internal/skillhub/registry.go](internal/skillhub/registry.go#L185)는 raw GitHub 파일을 내려받기만 하고, [internal/skillhub/install.go](internal/skillhub/install.go#L45)와 [internal/skillhub/install.go](internal/skillhub/install.go#L229)는 그대로 기록합니다. 반면 MCP는 [internal/skillhub/mcp.go](internal/skillhub/mcp.go#L103)와 [internal/skillhub/install.go](internal/skillhub/install.go#L499)에서 checksum을 검증합니다.
**소스 이슈 / Source Issue**: `SEC-003`
**영향 범위 / Impact Area**: 원격 Hub skill/plugin 설치 및 업데이트

**문제 설명 / Problem**
현재 skill/plugin 설치는 원격 registry와 파일 내용을 신뢰한 채 바로 workspace에 기록합니다.
이 구조에서는 registry 저장소가 오염되거나 전송 중 변조가 발생했을 때, 이후 runtime이 읽고 실행할 수 있는 악성 payload가 로컬에 설치될 수 있습니다.
Skill/plugin installation currently trusts remote registry content and writes it directly into the workspace.
If the registry content is compromised or tampered with in transit, malicious payloads can be installed into a path later consumed by the runtime.

**현재 코드 위치 / Current Code Location**
- `internal/skillhub/types.go` 라인 11-19, 22-26, 28-50
- `internal/skillhub/registry.go` 라인 185-209
- `internal/skillhub/install.go` 라인 45-96, 229-262
- `internal/skillhub/mcp.go` 라인 103-114
- `internal/skillhub/install.go` 라인 499-516

**SEC 상세 / SEC Details (`SEC-*`인 경우 필수 / required for `SEC-*`)**
- **취약 시나리오**: 공격자가 registry 저장소나 배포 아티팩트를 변조하면 사용자가 `tars skill install` 또는 `tars plugin install` 시 악성 SKILL.md/companion/plugin 파일을 로컬 workspace에 설치합니다.
- **악용 전제**: 사용자가 Hub 설치/업데이트 명령을 실행하고, registry 또는 원격 파일 공급원이 변조 가능해야 합니다.
- **완화 방향**: skill/plugin도 MCP와 같은 checksum manifest 또는 signed registry로 검증하고, mismatch 시 fail closed 하도록 바꿉니다.

**지시 사항 / Instructions**
1. skill/plugin registry schema를 checksum 포함 구조로 승격하십시오. 최소한 `SKILL.md`와 모든 companion/plugin 파일에 대해 `sha256`을 선언하게 하십시오.
2. skill/plugin download 경로도 MCP와 동일한 검증 helper를 재사용하거나 공유하여 파일별 checksum mismatch 시 즉시 실패하도록 하십시오.
3. registry versioning 또는 backward-compat 계획을 명시해 기존 checksum 없는 항목을 단계적으로 제거하십시오.
4. install/update 경로에서 무결성 검증 실패 시 partially written 결과를 남기지 말고 DB 갱신도 차단하십시오.
5. tampered payload, missing checksum, registry version mismatch를 각각 테스트로 고정하십시오.

**완료 기준 / Completion Criteria**
- [ ] skill/plugin 설치와 업데이트가 checksum 또는 동등한 무결성 검증을 통과한 파일만 반영함
- [ ] checksum 누락 또는 mismatch 시 설치/업데이트가 fail closed 됨
- [ ] skill/plugin/MCP 패키지 검증 모델이 한 방향으로 수렴함

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: checksum parser와 verifier 검증
- [ ] 통합 테스트: tampered skill/plugin payload 설치 실패 검증
- [ ] 회귀 테스트: 정상 Hub 항목 설치/업데이트가 기존 사용자 흐름을 깨지 않음 확인

---

### WO-004: Skill/Plugin 설치와 업데이트를 원자적 staged activation으로 전환하기

**분류 코드 / Classification Code**: `TIDY-006`
**유형 / Type**: Stage Package / Atomic Replace / Fail Closed
**심각도 / Severity**: 중간 / Medium
**선행 작업 / Prerequisite**: `WO-003` 완료 후 / after `WO-003`
**근거 / Evidence**: skill 설치는 [internal/skillhub/install.go](internal/skillhub/install.go#L56)에서 디렉터리를 먼저 만들고 companion file 실패를 `continue`로 무시한 뒤 [internal/skillhub/install.go](internal/skillhub/install.go#L80)에서 DB를 갱신합니다. skill/plugin update도 [internal/skillhub/install.go](internal/skillhub/install.go#L145), [internal/skillhub/install.go](internal/skillhub/install.go#L310)에서 best-effort로 버전을 올립니다. 반면 MCP는 [internal/skillhub/mcp.go](internal/skillhub/mcp.go#L116)의 temp dir + rename으로 원자적으로 materialize합니다.
**소스 이슈 / Source Issue**: `TIDY-006`
**영향 범위 / Impact Area**: Hub skill/plugin install/update reliability

**문제 설명 / Problem**
현재 skill/plugin 설치는 "일단 쓰고 가능한 만큼만 계속"하는 전략이라 설치 성공의 의미가 약합니다.
이 구조에서는 companion file 일부가 누락된 skill/plugin이 정상 설치로 기록되고, update 도중 실패해도 DB version이 앞서갈 수 있습니다.
The current skill/plugin installation strategy is best-effort and weakens the meaning of a successful install.
That allows partially materialized packages to be recorded as healthy and makes version metadata drift from what is actually on disk.

**현재 코드 위치 / Current Code Location**
- `internal/skillhub/install.go` 라인 56-87
- `internal/skillhub/install.go` 라인 145-175
- `internal/skillhub/install.go` 라인 241-261
- `internal/skillhub/install.go` 라인 300-333
- `internal/skillhub/mcp.go` 라인 116-142

**TIDY 상세 / TIDY Details (`TIDY-*`인 경우 필수 / required for `TIDY-*`)**
- **적용 룰**: `구조 변경과 동작 변경을 분리한다`, `작고 안전한 단계로 진행한다`, `동작 보존을 먼저 보장한다`
- **구조 변경 단계**: skill/plugin용 `download -> verify -> stage -> activate -> persist-db` 파이프라인을 도입하고, MCP의 `materializePackageFiles` 패턴을 재사용 가능한 helper로 끌어올립니다.
- **동작 변경 단계**: staged activation이 안정화된 뒤에만 partial recovery 정책이나 legacy migration 정책을 조정합니다.

**지시 사항 / Instructions**
1. skill/plugin 설치와 업데이트가 전체 파일 집합을 먼저 메모리 또는 temp dir에 모은 뒤, 검증 통과 후 한 번에 활성화되도록 바꾸십시오.
2. partial download/write failure 시 기존 설치본은 유지하고 DB/version 갱신을 금지하십시오.
3. skill/plugin install/update가 MCP install path와 같은 activation contract를 사용하도록 공통 helper를 만드십시오.
4. 설치 성공의 정의를 "필수 파일 전체 materialize + manifest/metadata 반영 완료"로 명확히 하십시오.
5. 실패 시 temp dir cleanup, old version rollback, DB consistency를 테스트로 고정하십시오.

**완료 기준 / Completion Criteria**
- [ ] skill/plugin install/update가 전체 payload 검증 후 원자적으로 활성화됨
- [ ] partial failure 시 기존 설치본과 DB state가 보존됨
- [ ] MCP와 skill/plugin 패키지 설치 contract가 일관되게 설명 가능해짐

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: staged activation helper와 rollback 경로 검증
- [ ] 통합 테스트: install/update 중 파일 다운로드 실패 시 디스크/DB 상태 보존 검증
- [ ] 회귀 테스트: 정상 skill/plugin 설치와 업데이트가 기존 성공 경로를 유지함 확인

---

## 켄트 백(Tidy First) 룰 매핑 테이블 / Kent Beck (Tidy First) Rule Mapping

| WO ID | 적용 룰 | 기대 효과 |
|-------|---------|-----------|
| WO-001 | 구조 변경과 동작 변경을 분리한다 | workflow 규칙 변경 시 chat/API/autonomous drift 감소 |
| WO-001 | 동작 보존을 먼저 보장한다 | 상태 전이 리팩토링 중 회귀 방지 |
| WO-002 | 의도를 드러내는 이름을 사용한다 | supported provider 범위가 설정 표면에 명확히 드러남 |
| WO-002 | 구조 변경과 동작 변경을 분리한다 | provider registry 도입과 실제 provider 추가를 분리 |
| WO-004 | 작고 안전한 단계로 진행한다 | staged activation 전환 시 설치 경로 회귀 위험 축소 |

---

## 리팩토링 순서 (Recommended Order) / Recommended Order

의존 관계 없는 작업은 병렬 실행 가능합니다.
Work orders without dependencies can be executed in parallel.

```text
WO-003 (skill/plugin integrity verification)
  └- WO-004 (skill/plugin staged activation, WO-003 완료 후)

WO-001 (workflow runtime/state model) <- 독립 실행 가능
WO-002 (semantic provider surface alignment) <- 독립 실행 가능
```

---

## 체크포인트 진행 로그 (Checkpoint Progress Log) / Checkpoint Progress Log

중단/재개 가능하도록 아래 표를 반드시 유지합니다.
Keep this table updated so the work can be paused and resumed safely.

| Checkpoint | 상태 | 변경된 WO | DUP/SEC/TIDY 진행 현황 | 메모 |
|------------|------|-----------|-------------------------|------|
| checkpoint-001 | completed | WO-001 ~ WO-004 초안 | DUP 0/0, SEC 1/1, TIDY 3/3 | `SEC-002` 후보는 browser relay 제거로 false positive 판정 후 제외 |

---

## 다음 세션 재개 지점 (Resume Point) / Resume Point

- **다음 시작 WO**: `WO-001`
- **남은 선행 작업**: `WO-003 완료 후 WO-004 진행`
- **우선 재검토 파일**: `internal/project/*`, `internal/memory/semantic.go`, `internal/skillhub/install.go`, `internal/skillhub/types.go`, `internal/skillhub/mcp.go`
- **재개 순서 / Resume Steps**
  1. 마지막 체크포인트 로그 행 확인
  2. `SEC-002` 제외 판단이 여전히 유효한지 현재 헤드에서 다시 확인
  3. `WO-003 -> WO-004` 또는 독립 경로인 `WO-001`, `WO-002` 중 하나부터 구현 시작

---

## 완료 검증 체크리스트 / Completion Verification Checklist

- [ ] `go test ./...` 전체 통과
- [ ] 저장소에 `make lint`가 있으면 경고 없음, 없으면 대체 정적 검증 명령을 명시하고 통과
- [ ] 변경된 파일의 최대 줄 수가 300줄 이하 또는 초과 이유가 문서화됨
- [ ] 이슈 목록의 모든 항목이 `DUP-*`, `SEC-*`, `TIDY-*` 중 하나로 분류됨
- [ ] 이슈 목록의 모든 항목이 WO로 처리됨
- [ ] `SEC-*` 항목마다 취약 시나리오/악용 전제/완화 방향이 기재됨
- [ ] `TIDY-*` 항목마다 적용 룰과 단계 분리가 기재됨
