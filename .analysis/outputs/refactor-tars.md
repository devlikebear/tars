# 리팩토링 지시서: TARS
# Refactoring Work Order: TARS

> **생성일 / Created At**: 2026-06-09
> **세션 ID / Session ID**: `refactor-guide-20260609-074851`
> **체크포인트 / Checkpoint**: `checkpoint-003`
> **대상 / Target**: `cmd/tars`, `internal/tarsclient`, `pkg/tarsclient`, `internal/launchagent`, `internal/extensions`, `internal/tarsserver`
> **분석 범위 / Analysis Scope**: 기존 `issue-candidates.md` 후보 기반 BFS + 현재 HEAD 직접 분석
> **우선순위 / Priority**: 중간
> **소스 / Source**: `issue-candidates.md` + 직접 분석

---

## 요약 (Summary) / Summary

이 지시서는 `TARS`에서 발견된 코드 품질/구조 문제를 해결하기 위한 리팩토링 작업을 정의합니다.
총 6개의 이슈가 발견되었으며 (`DUP` 1건 / `SEC` 0건 / `TIDY` 5건), 아래 작업 목록을 순서대로 실행하십시오.
This work order defines the refactoring tasks required to address code quality, security, and structural issues found in `TARS`.
There are 6 issues in total (`DUP` 1 / `SEC` 0 / `TIDY` 5), and the work orders below should be executed in sequence.

기존 후보 중 `#408 CLI/client URL resolution`은 `pkg/tarsclient.ResolveURL`/`ConsoleURL` 및 `internal/tarsclient.resolveURL` 위임으로 핵심 문제가 해소되어 직접 WO에서 제외했습니다. 다만 내부 CLI의 죽은 HTTP transport 잔여물은 별도 `DUP-001`로 남겼습니다. `#417 --serve-api=false`는 `cmd/tars` 레벨에서 unknown flag로 거부하는 테스트가 있어 제외했습니다. `#418 usage-signaled feature decisions`는 `docs/usage-signals.md`와 `docs/code-review/status.md` 기준으로 완료된 제품 의사결정 항목이라 코드 리팩토링 WO로 전환하지 않았습니다.

추가 검토에서 README의 문체가 릴리즈 노트처럼 누적되는 문제가 확인되었습니다. `README.md` 앞부분은 핵심 가치 제안이 먼저 압축되어야 하며, "now" 중심의 점진적 동작 설명과 세부 변경 내역은 별도 docs로 분리하는 문서 리팩토링이 필요합니다.

---

## 발견된 이슈 목록 (Issues Found) / Issues Found

| # | 분류 코드 | 이슈 유형 | 위치 | 근거 | 위험도 | 영향 범위 |
|---|-----------|-----------|------|------|--------|-----------|
| 1 | `DUP-001` | 죽은 중복 전송 계층 | `internal/tarsclient/runtime_transport.go:13`, `pkg/tarsclient/client.go:192` | 내부 runtime client가 이미 `runtimeClient.client()`로 공개 클라이언트를 위임하지만, 별도 `requestJSON/requestText/parseAPIErrorPayload` 구현이 남아 있음 | 낮음 | 내부 CLI client |
| 2 | `TIDY-001` | launchd identity/default 중복 | `cmd/tars/service_main.go:53`, `cmd/tars/init_main.go:318`, `cmd/tars/init_reset_main.go:260`, `internal/tarsserver/handler_config.go:250` | label/domain/plist 기본값 조합이 여러 파일에 반복되어 custom label/domain 유지가 어렵고 테스트 훅도 분산됨 | 중간 | macOS service/init/restart |
| 3 | `TIDY-002` | plist ProgramArguments 수동 파싱 | `cmd/tars/init_reset_main.go:278` | TARS가 직접 생성한 XML plist를 문자열 검색으로 파싱해 `--api-addr`를 복원함 | 낮음 | init reset / workspace move |
| 4 | `TIDY-003` | disabled extension state 비원자 쓰기 | `internal/extensions/disabled.go:60` | 장기 상태 파일인데 `os.WriteFile`을 직접 사용하며, `atomicwrite.Write`를 쓰는 usage/ops persistence와 계약이 다름 | 중간 | skills/plugins/MCP disabled state |
| 5 | `TIDY-004` | 서버 내부 Cobra 부트스트랩 잔여 | `cmd/tars/server_main.go:32`, `internal/tarsserver/main.go:35`, `internal/tarsserver/main_cli.go:55` | 외부 CLI가 이미 `serve` flags를 파싱한 뒤 `tarsserver.Serve`를 호출하지만, 내부에서 다시 Cobra command를 만들어 빈 args로 실행함 | 낮음 | server bootstrap/tests |
| 6 | `TIDY-005` | README changelog형 문체 누적 | `README.md:43`, `README.md:53`, `README.md:107`, `README.md:116` | "TARS can now..." 같은 점진적 변경 문장과 긴 누적 기능 설명이 README 전면에 남아 핵심 가치 제안보다 릴리즈 이력처럼 읽힘 | 중간 | README / product docs |

---

## 작업 지시서 (Work Orders) / Work Orders

각 작업은 독립적으로 실행 가능해야 합니다.
의존 관계가 있는 경우 "선행 작업" 필드를 채웁니다.
Each work order should be executable independently.
If there is a dependency, fill in the "Prerequisite" field.

---

### WO-001: 내부 CLI의 죽은 HTTP transport 제거

**분류 코드 / Classification Code**: `DUP-001`
**유형 / Type**: Delete Dead Code / Consolidate Transport
**심각도 / Severity**: 낮음
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `internal/tarsclient/runtime.go:16`은 이미 `pkg/tarsclient.Client`를 생성하고, runtime session/agentruntime/ops/usage 경로가 `c.client()`를 호출합니다. 반면 `internal/tarsclient/runtime_transport.go:13-115`의 `requestJSON`, `requestText`, `parseAPIErrorPayload`는 현재 직접 호출 지점이 없고 공개 클라이언트의 `doText`/`parseAPIErrorPayload`와 중복됩니다.
**소스 이슈 / Source Issue**: `#408 CLI/client URL resolution` 인접 잔여물
**영향 범위 / Impact Area**: 내부 CLI client, 공개 client 오류 처리

**문제 설명 / Problem**
현재 `internal/tarsclient/runtime_transport.go:13-115`에 공개 client와 같은 역할을 하는 HTTP request/JSON/error parsing 코드가 남아 있습니다.
문제가 유지되면 인증 토큰 선택, response body 제한, API error 포맷이 공개 client와 다시 갈라질 수 있습니다.
At `internal/tarsclient/runtime_transport.go:13-115`, duplicate transport and error parsing code remains even though the runtime client delegates to `pkg/tarsclient.Client`.
If left unchanged, internal and public client behavior can drift again.

**현재 코드 위치 / Current Code Location**
- `internal/tarsclient/runtime_transport.go` 라인 13-115
- `internal/tarsclient/runtime.go` 라인 16-23
- `pkg/tarsclient/client.go` 라인 192-283

**SEC 상세 / SEC Details (`SEC-*`인 경우 필수 / required for `SEC-*`)**
- 해당 없음

**TIDY 상세 / TIDY Details (`TIDY-*`인 경우 필수 / required for `TIDY-*`)**
- 해당 없음

**지시 사항 / Instructions**
1. 실패 테스트를 먼저 추가하거나 기존 테스트를 확장해 내부 runtime command가 `pkg/tarsclient.APIError`를 그대로 받아 `formatRuntimeError` hint를 유지하는지 확인하십시오.
2. `internal/tarsclient/runtime_transport.go`를 삭제하십시오.
3. 삭제 후 `apiHTTPError` alias와 `formatRuntimeError` 테스트가 공개 `pkg/tarsclient.APIError`만 바라보는지 확인하십시오.
4. `rg "requestJSON\\(|requestText\\(|parseAPIErrorPayload\\(" internal/tarsclient` 결과가 죽은 정의 없이 정리되도록 하십시오.

**완료 기준 / Completion Criteria**
- [ ] 내부 runtime client의 HTTP 전송 경로가 `pkg/tarsclient.Client` 하나로 수렴함
- [ ] 중복 `parseAPIErrorPayload` 구현이 내부 client 패키지에서 제거됨
- [ ] admin/user token hint와 API error redaction 동작이 유지됨

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: `go test ./internal/tarsclient ./pkg/tarsclient`
- [ ] 회귀 테스트: `TestFormatRuntimeError_ProvidesAdminHintOnAdminEndpoint`, `TestChatClientStream_HTTPErrorReturnsAPIError`
- [ ] 정적 확인: `rg "requestJSON\\(|requestText\\(" internal/tarsclient`에서 호출/정의 잔여 없음

---

### WO-002: launchd service target 기본값을 한 경로로 모으기

**분류 코드 / Classification Code**: `TIDY-001`
**유형 / Type**: Extract Function / Consolidate Defaults
**심각도 / Severity**: 중간
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `cmd/tars/service_main.go:53-60`, `cmd/tars/service_main.go:140-147`, `cmd/tars/init_main.go:318-327`, `cmd/tars/init_main.go:640-647`, `cmd/tars/init_reset_main.go:260-270`, `internal/tarsserver/handler_config.go:250-255`가 label/domain/plist path 조합을 각자 만든다.
**소스 이슈 / Source Issue**: `#409 macOS service + LaunchAgent plumbing`
**영향 범위 / Impact Area**: macOS LaunchAgent install/start/stop/status/restart

**문제 설명 / Problem**
현재 macOS service target의 label, domain, plist path 기본값 조립이 CLI service, init, reset, server restart 경로에 흩어져 있습니다.
문제가 유지되면 custom label/domain 또는 환경 변수 기반 launchd identity를 추가/수정할 때 경로별 동작 차이가 생길 수 있습니다.
At the service/init/restart paths, launchd label, domain, and plist defaults are assembled in several places.
If left unchanged, custom launchd identity behavior can drift across service operations.

**현재 코드 위치 / Current Code Location**
- `cmd/tars/service_main.go` 라인 53-60, 140-147, 329-339
- `cmd/tars/init_main.go` 라인 318-327, 640-647
- `cmd/tars/init_reset_main.go` 라인 260-270
- `internal/tarsserver/handler_config.go` 라인 250-255
- `internal/launchagent/launchagent.go` 라인 100-110

**SEC 상세 / SEC Details (`SEC-*`인 경우 필수 / required for `SEC-*`)**
- 해당 없음

**TIDY 상세 / TIDY Details (`TIDY-*`인 경우 필수 / required for `TIDY-*`)**
- **적용 룰**: 중복을 통제 가능한 단위로 모은다 / 구조 변경과 동작 변경을 분리한다
- **구조 변경 단계**: launchd target 조립 helper를 추출하고 기존 호출부는 helper 호출로만 바꾼다.
- **동작 변경 단계**: 없음. custom `--label`, `--domain`, `TARS_LAUNCHD_LABEL`, `TARS_LAUNCHD_DOMAIN` 동작은 유지한다.

**지시 사항 / Instructions**
1. `cmd/tars` 패키지에 테스트 가능한 `resolveServiceTarget(opts serviceOptions) (serviceTarget, error)` 또는 `internal/launchagent`에 `ServerIdentity`/`DefaultDomain` helper를 먼저 추가하십시오.
2. helper는 label, domain, plist path, stdout log, stderr log를 한 번에 resolve하고, 기존 `serviceGetuid`/`serviceUserHomeDir` 테스트 훅과 충돌하지 않게 하십시오.
3. `runServiceCommand`, `startInitService`, `runInitMoveCommand`, `stopExistingService`, `initMoveLaunchAgentPresent`가 같은 helper를 사용하게 바꾸십시오.
4. `internal/tarsserver/handler_config.go`의 restart identity도 같은 domain/label 규칙을 쓰게 정리하되, 서버 패키지에서 `cmd/tars`를 import하지 마십시오.

**완료 기준 / Completion Criteria**
- [ ] `gui/<uid>` domain 문자열 조립이 한 helper 또는 한 패키지 책임으로 수렴함
- [ ] 기본 server label/plist/log path 정책이 service/init/reset에서 동일함
- [ ] custom label/domain 테스트가 모든 restart/start/stop 경로에서 유지됨

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: `go test ./cmd/tars ./internal/launchagent ./internal/tarsserver`
- [ ] 회귀 테스트: `TestRootCommand_ServiceInstallWritesLaunchdIdentityEnvironment`, `TestDetectRunModeUsesLaunchdIdentityFromEnvironment`
- [ ] 새 테스트: init reset/move 경로가 같은 custom domain/label helper를 사용하는지 확인

---

### WO-003: LaunchAgent plist의 ProgramArguments 파서를 XML 기반 helper로 분리

**분류 코드 / Classification Code**: `TIDY-002`
**유형 / Type**: Extract Function / Replace String Parsing
**심각도 / Severity**: 낮음
**선행 작업 / Prerequisite**: WO-002 완료 후 권장
**근거 / Evidence**: `cmd/tars/init_reset_main.go:278-310`은 plist XML에서 `<string>--api-addr</string>`를 문자열 검색으로 찾아 다음 `<string>` 값을 잘라낸다. 같은 저장 형식은 `internal/launchagent.BuildPlist`가 생성한다.
**소스 이슈 / Source Issue**: `#409 macOS service + LaunchAgent plumbing`
**영향 범위 / Impact Area**: `tars init reset`, workspace move 후 service restart

**문제 설명 / Problem**
현재 `readExistingAPIAddrFromPlist`가 plist XML을 구조적으로 읽지 않고 문자열 위치로 파싱합니다.
문제가 유지되면 whitespace, XML escaping, ProgramArguments 재정렬 같은 작은 형식 변화에도 포트 복원이 실패할 수 있습니다.
At `cmd/tars/init_reset_main.go:278-310`, plist XML is parsed with substring searches.
If left unchanged, small plist formatting changes can break API address restoration.

**현재 코드 위치 / Current Code Location**
- `cmd/tars/init_reset_main.go` 라인 278-310
- `internal/launchagent/launchagent.go` 라인 30-75
- `cmd/tars/init_reset_main_test.go`의 plist read tests

**SEC 상세 / SEC Details (`SEC-*`인 경우 필수 / required for `SEC-*`)**
- 해당 없음

**TIDY 상세 / TIDY Details (`TIDY-*`인 경우 필수 / required for `TIDY-*`)**
- **적용 룰**: 작고 안전한 단계로 진행한다 / 의도를 드러내는 이름을 사용한다
- **구조 변경 단계**: plist ProgramArguments 추출 함수를 `internal/launchagent` 또는 `cmd/tars` local helper로 분리한다.
- **동작 변경 단계**: 없음. `--api-addr`가 없으면 `(string, false)`를 반환하는 기존 계약을 유지한다.

**지시 사항 / Instructions**
1. 실패 테스트를 먼저 작성해 ProgramArguments에 줄바꿈/공백/escaped string이 있어도 `--api-addr` 다음 값을 읽는지 확인하십시오.
2. `encoding/xml` 기반으로 ProgramArguments 배열의 string 값들을 순서대로 추출하는 helper를 추가하십시오.
3. `readExistingAPIAddrFromPlist`는 파일 읽기와 helper 호출만 남기십시오.
4. 기존 malformed/no-flag 테스트를 유지하고, helper 단위 테스트를 추가하십시오.

**완료 기준 / Completion Criteria**
- [ ] `readExistingAPIAddrFromPlist`에서 직접 `<string>` substring parsing이 제거됨
- [ ] ProgramArguments 추출 로직이 테스트 가능한 작은 함수로 분리됨
- [ ] 기존 reset 포트 보존 동작이 유지됨

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: `go test ./cmd/tars ./internal/launchagent`
- [ ] 회귀 테스트: 기존 `readExistingAPIAddrFromPlist` 관련 테스트
- [ ] 새 테스트: escaped value, no flag, flag without value, non-ProgramArguments string 처리

---

### WO-004: disabled extension state 저장을 atomicwrite 계약으로 맞추기

**분류 코드 / Classification Code**: `TIDY-003`
**유형 / Type**: Add Persistence Helper / Replace Direct Write
**심각도 / Severity**: 중간
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `internal/extensions/disabled.go:60-68`은 `extensions_disabled.json`을 `os.WriteFile`로 직접 쓴다. 반면 `internal/atomicwrite/atomicwrite.go:1-12`는 장기 상태 파일의 canonical persistence helper를 정의하고, usage/ops는 이미 이를 사용한다.
**소스 이슈 / Source Issue**: `#415 State persistence/default contracts`
**영향 범위 / Impact Area**: Skill/Plugin/MCP disabled state

**문제 설명 / Problem**
현재 disabled extension state는 workspace-local 장기 상태 파일이지만 원자적 쓰기 helper를 쓰지 않습니다.
문제가 유지되면 프로세스 종료나 디스크 오류 타이밍에 `extensions_disabled.json`이 반쯤 쓰일 수 있고, 다음 reload에서 disabled state 전체가 로드 실패할 수 있습니다.
At `internal/extensions/disabled.go:60-68`, long-lived extension state is written directly with `os.WriteFile`.
If left unchanged, partial writes can corrupt disabled state across restarts.

**현재 코드 위치 / Current Code Location**
- `internal/extensions/disabled.go` 라인 60-68
- `internal/atomicwrite/atomicwrite.go` 라인 1-12
- `internal/usage/tracker_limits.go` 라인 70-76
- `internal/ops/manager_approvals.go` 라인 111-122

**SEC 상세 / SEC Details (`SEC-*`인 경우 필수 / required for `SEC-*`)**
- 해당 없음

**TIDY 상세 / TIDY Details (`TIDY-*`인 경우 필수 / required for `TIDY-*`)**
- **적용 룰**: 동작 보존을 먼저 보장한다 / 중복을 통제 가능한 단위로 모은다
- **구조 변경 단계**: `saveLocked`에서 `atomicwrite.Write`를 사용하고, 기존 파일 mode 계약을 명시한다.
- **동작 변경 단계**: 없음. corrupt file을 덮어쓰지 않고 에러를 반환하는 기존 load contract를 유지한다.

**지시 사항 / Instructions**
1. 실패 테스트를 먼저 작성해 temp file 생성 실패 시 기존 `extensions_disabled.json`과 in-memory 변경이 보존되는지 확인하십시오.
2. `internal/extensions/disabled.go`에서 `os.WriteFile`을 `atomicwrite.Write`로 교체하십시오.
3. 현재 파일 권한을 유지할지 `0o600`으로 바꿀지 결정하고 테스트에 명시하십시오. 동작 변경을 피하려면 write 후 `os.Chmod(path, 0o644)`로 기존 mode를 유지하십시오.
4. corrupt disabled state 보존 테스트와 concurrent update 테스트를 유지하십시오.

**완료 기준 / Completion Criteria**
- [ ] disabled state 저장이 canonical `atomicwrite.Write` 경로를 사용함
- [ ] write 실패 시 기존 파일이 손상되지 않음
- [ ] corrupt state를 자동 덮어쓰지 않는 기존 안전장치가 유지됨

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: `go test ./internal/extensions`
- [ ] 회귀 테스트: `TestDisabledStoreSetDisabledReturnsLoadErrorAndPreservesFile`, `TestDisabledStoreSetDisabledKeepsConcurrentUpdates`
- [ ] 새 테스트: atomic temp 생성 실패 시 기존 파일 보존

---

### WO-005: `tarsserver.Serve` 내부 Cobra 부트스트랩 제거

**분류 코드 / Classification Code**: `TIDY-004`
**유형 / Type**: Simplify Control Flow / Extract Function
**심각도 / Severity**: 낮음
**선행 작업 / Prerequisite**: WO-001과 독립 실행 가능
**근거 / Evidence**: `cmd/tars/server_main.go:32-58`이 외부 `serve` command flags를 파싱해 `tarsserver.Serve`로 넘긴다. 그런데 `internal/tarsserver/main.go:35-57`은 다시 `newRootCmd`를 만들고 빈 args로 실행하며, `internal/tarsserver/main_cli.go:55-124`는 같은 성격의 flags를 내부 Cobra command에 정의한다.
**소스 이슈 / Source Issue**: `#417 --serve-api=false` 검토 중 확인한 잔여 구조
**영향 범위 / Impact Area**: server bootstrap, config-check, setup-only mode tests

**문제 설명 / Problem**
현재 서버 시작 경로는 외부 CLI parsing 이후 내부에서 다시 Cobra command를 통해 runtime bootstrap을 실행합니다.
문제가 유지되면 CLI flag 의미와 server runtime option 의미가 다시 섞이고, 사용자-facing flag 제거 후에도 내부 command 구조가 남아 테스트/문서가 혼동될 수 있습니다.
At `tarsserver.Serve`, runtime bootstrap still goes through an internal Cobra command even though `cmd/tars` already parsed the user-facing CLI.
If left unchanged, command parsing concerns and server runtime concerns remain unnecessarily coupled.

**현재 코드 위치 / Current Code Location**
- `cmd/tars/server_main.go` 라인 32-58
- `internal/tarsserver/main.go` 라인 35-57
- `internal/tarsserver/main_cli.go` 라인 55-124
- `internal/tarsserver/main_options.go`

**SEC 상세 / SEC Details (`SEC-*`인 경우 필수 / required for `SEC-*`)**
- 해당 없음

**TIDY 상세 / TIDY Details (`TIDY-*`인 경우 필수 / required for `TIDY-*`)**
- **적용 룰**: 구조 변경과 동작 변경을 분리한다 / 국소 정리(local tidy)를 우선한다
- **구조 변경 단계**: 내부 `newRootCmd`의 `RunE` 본문을 `runServeRuntime(ctx, opts, cfg, stdout, stderr, nowFn)` 같은 일반 함수로 추출한다.
- **동작 변경 단계**: 없음. `config-check`, setup-only degrade, logger setup, `runServeAPICommand` 호출 결과를 유지한다.

**지시 사항 / Instructions**
1. 실패 테스트를 먼저 추가해 `tarsserver.Serve`가 `ConfigCheck`, `APIAddr`, setup-only downgrade를 내부 Cobra 없이도 유지해야 함을 고정하십시오.
2. `internal/tarsserver/main_cli.go`의 `RunE` 본문을 일반 함수로 추출하십시오.
3. `Serve`는 `cmd.Execute()` 대신 추출 함수 호출로 바꾸십시오.
4. 내부 Cobra command가 외부에서 더 이상 필요 없다면 제거하고, 테스트 helper는 추출 함수 또는 options builder를 직접 호출하게 바꾸십시오.
5. 사용자-facing `cmd/tars serve` flag parsing은 `cmd/tars/server_main.go`에 유지하십시오.

**완료 기준 / Completion Criteria**
- [ ] `tarsserver.Serve`가 내부 Cobra command를 만들지 않음
- [ ] 서버 runtime bootstrap과 CLI flag definition의 책임이 분리됨
- [ ] setup-only mode와 config-check 동작이 기존과 동일함

**테스트 기준 / Test Criteria**
- [ ] 단위 테스트: `go test ./internal/tarsserver ./cmd/tars`
- [ ] 회귀 테스트: config-check success/failure, setup-only e2e, `TestRootCommand_ServeSubcommandRejectsServeAPIFlag`
- [ ] 정적 확인: `internal/tarsserver`에서 server runtime용 flag definition이 제거되거나 test-only helper로 축소됨

---

### WO-006: README 문체를 가치 제안 중심으로 재구성

**분류 코드 / Classification Code**: `TIDY-005`
**유형 / Type**: Documentation Restructure / Copy Editing / Move Detail to Docs
**심각도 / Severity**: 중간
**선행 작업 / Prerequisite**: 없음
**근거 / Evidence**: `README.md:43`이 "TARS can now be used..."로 시작해 최근 변경사항을 전면 가치 제안처럼 소개합니다. `README.md:53-74`, `README.md:107-116`, `README.md:152`에는 기능별 점진 동작 설명이 길게 누적되어 README가 제품 소개보다 changelog처럼 읽힙니다.
**소스 이슈 / Source Issue**: 직접 분석
**영향 범위 / Impact Area**: README, onboarding docs, feature-specific docs

**문제 설명 / Problem**
현재 README는 핵심 가치 제안보다 누적된 기능 설명이 먼저 보입니다.
문제가 유지되면 첫 방문자가 TARS의 정체성을 빠르게 이해하기 어렵고, LLM이 커밋 메시지나 릴리즈 노트를 README 본문으로 승격한 듯한 문체 인상을 줍니다.
At `README.md`, incremental release-note phrasing appears in the primary product narrative.
If left unchanged, the README will feel less intentional and harder to scan for newcomers.

**현재 코드 위치 / Current Code Location**
- `README.md` 라인 39-47
- `README.md` 라인 53-74
- `README.md` 라인 107-116
- `README.md` 라인 152
- 세부 이동 후보: `docs/public-agent-packages.md`, `docs/tutorials/22-agentruntime.md`, `docs/frontend-api-types.md`, 필요 시 신규 `docs/console.md`

**SEC 상세 / SEC Details (`SEC-*`인 경우 필수 / required for `SEC-*`)**
- 해당 없음

**TIDY 상세 / TIDY Details (`TIDY-*`인 경우 필수 / required for `TIDY-*`)**
- **적용 룰**: 의도를 드러내는 이름을 사용한다 / 국소 정리(local tidy)를 우선한다 / 구조 변경과 동작 변경을 분리한다
- **구조 변경 단계**: README의 첫 화면을 가치 제안, 핵심 사용 흐름, 주요 차별점으로 압축하고, 세부 동작 설명을 owning docs로 이동한다.
- **동작 변경 단계**: 없음. 문서 표현과 정보 구조만 바꾼다.

**지시 사항 / Instructions**
1. README 리라이트 전에 현재 README 문체 회귀를 고정하는 문서 리뷰 체크를 작성하십시오. 최소한 `README.md`의 첫 120줄에서 `can now`, `now creates`, `now requeues`, `now ...` 같은 릴리즈 노트형 phrasing이 남지 않도록 검사하십시오.
2. README 상단을 다음 순서로 재구성하십시오: 한 문장 가치 제안, 3-5개 핵심 사용 사례, 설치/quickstart로 가는 짧은 경로, 상세 기능 문서 링크.
3. `Public Agent Packages`는 "새로 가능해졌다"가 아니라 "TARS가 제공하는 재사용 가능한 Go agent-building kit"로 현재형 가치 제안 중심으로 다시 쓰십시오.
4. Chat/Agent Runtime/Model Routing 섹션의 긴 누적 동작 설명은 5-7개 핵심 bullet만 남기고, 세부 UI 동작/릴리즈성 기능은 `docs/`의 owning 문서로 이동하거나 링크하십시오.
5. README에서 제거한 세부 정보가 사라지지 않도록 기존 docs에 흡수하고, 링크가 깨지지 않게 확인하십시오.
6. changelog성 문장은 `CHANGELOG.md` 또는 feature docs에만 두고, README에는 "현재 제품이 무엇인지"를 설명하는 현재형 문장만 남기십시오.

**완료 기준 / Completion Criteria**
- [ ] README 첫 화면에서 TARS의 핵심 가치 제안과 주요 사용 흐름이 30초 안에 파악됨
- [ ] `README.md` 상단/Key Features 구간의 "now" 중심 변경 이력 문체가 제거됨
- [ ] 상세 동작 설명은 `docs/`로 이동하거나 링크되어 정보 손실이 없음
- [ ] README가 제품 소개, docs가 세부 동작 설명, CHANGELOG가 릴리즈 이력이라는 역할 분리가 명확함

**테스트 기준 / Test Criteria**
- [ ] 문서 검사: `rg -n "\\b(can now|now creates|now requeues|now [a-z])\\b" README.md` 결과가 의도된 문맥 외 0건
- [ ] 링크 검사: README에서 새로 추가/수정한 상대 링크가 모두 존재함
- [ ] 리뷰 기준: README 첫 120줄을 읽고 "최근 변경사항 묶음"이 아니라 "현재 제품 설명"으로 읽히는지 수동 확인

---

## 켄트 백(Tidy First) 룰 매핑 테이블 / Kent Beck (Tidy First) Rule Mapping

| WO ID | 적용 룰 | 기대 효과 |
|-------|---------|-----------|
| WO-001 | 중복을 통제 가능한 단위로 모은다 | 내부/공개 client drift 방지 |
| WO-002 | 중복을 통제 가능한 단위로 모은다 | launchd identity 변경 시 경로별 불일치 감소 |
| WO-003 | 작고 안전한 단계로 진행한다 | plist 형식 변화에 대한 회귀 위험 감소 |
| WO-004 | 동작 보존을 먼저 보장한다 | disabled state 손상 가능성 감소 |
| WO-005 | 구조 변경과 동작 변경을 분리한다 | server bootstrap 책임 경계 명확화 |
| WO-006 | 의도를 드러내는 이름을 사용한다 | README가 릴리즈 누적물이 아니라 제품 소개로 읽힘 |

---

## 리팩토링 순서 (Recommended Order) / Recommended Order

의존 관계 없는 작업은 병렬 실행 가능합니다.
Work orders without dependencies can be executed in parallel.

```text
WO-001 (내부 client 죽은 전송 코드 삭제) <- 독립 실행 가능
WO-004 (disabled state atomic write) <- 독립 실행 가능
WO-006 (README 문체/정보구조 정리) <- 독립 실행 가능
WO-002 (launchd target helper 추출)
  └- WO-003 (plist ProgramArguments parser 분리)
WO-005 (server 내부 Cobra 제거) <- 독립 실행 가능, 단 테스트 범위가 넓음
```

---

## 체크포인트 진행 로그 (Checkpoint Progress Log) / Checkpoint Progress Log

중단/재개 가능하도록 아래 표를 반드시 유지합니다.
Keep this table updated so the work can be paused and resumed safely.

| Checkpoint | 상태 | 변경된 WO | DUP/SEC/TIDY 진행 현황 | 메모 |
|------------|------|-----------|-------------------------|------|
| checkpoint-001 | completed | WO-001~WO-005 초안 완료 | DUP 1/1, SEC 0/0, TIDY 4/4 | 기존 후보 재검증 완료. `#408`, `#417`, `#418`은 직접 WO 제외 사유 기록 |
| checkpoint-002 | completed | WO-006 추가 | DUP 1/1, SEC 0/0, TIDY 5/5 | README changelog형 문체 누적 문제를 문서 리팩토링 WO로 추가 |
| checkpoint-003 | completed | WO-001~WO-006 구현 완료 | DUP 1/1, SEC 0/0, TIDY 5/5 | 내부 client 중복 transport 삭제, launchd target/plist parser 정리, disabled state atomic write 및 Windows replace fallback, `Serve` 직접 runtime 호출, README/docs 재구성 완료. `make test`, `make lint`, `make lint-diff`, `make test-cover-diff`, `make build`, `make security-scan`, `git diff --check` 통과 |

---

## 다음 세션 재개 지점 (Resume Point) / Resume Point

- **다음 시작 WO**: 없음. `WO-001`~`WO-006` 구현 완료
- **남은 선행 작업**: 없음
- **우선 재검토 파일**: `README.md`, `docs/console.md`, `internal/launchagent/launchagent.go`, `cmd/tars/service_main.go`, `internal/extensions/disabled.go`, `internal/tarsserver/main.go`
- **재개 순서 / Resume Steps**
  1. 마지막 체크포인트 로그 행 확인
  2. 필요 시 구현 diff와 검증 로그를 재확인
  3. 후속 작업은 새 WO로 분리

---

## 완료 검증 체크리스트 / Completion Verification Checklist

- [x] `make test` 전체 통과
- [x] `make lint` 경고 없음 (새로 추가된 파일 포함)
- [x] 변경된 파일의 최대 줄 수가 300줄 이하
- [x] 이슈 목록의 모든 항목이 `DUP-*`, `SEC-*`, `TIDY-*` 중 하나로 분류됨
- [x] 이슈 목록의 모든 항목이 WO로 처리됨
- [x] `SEC-*` 항목마다 취약 시나리오/악용 전제/완화 방향이 기재됨
- [x] `TIDY-*` 항목마다 적용 룰과 단계 분리가 기재됨
