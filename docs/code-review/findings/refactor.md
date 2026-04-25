# Refactor Candidates — 리팩토링 후보

코드 동작은 유지하면서 구조·가독성·중복을 개선할 만한 항목들. 각 항목은 후속 PR의 워크오더로 그대로 변환 가능한 수준으로 작성합니다.

---

## RF-001 — Deprecated `--run-once` / `--run-loop` 플래그 완전 제거

- **Status**: open
- **Location**:
  - `cmd/tars/server_main.go:19-20,43-48,57-58,71-72`
  - `internal/tarsserver/main_cli.go:39-41,107-109,131-132`
- **Discovered in**: [journal/2026-04-25-02-server-bootstrap.md](../journal/2026-04-25-02-server-bootstrap.md)
- **Recommendation**: 옵션 필드, 플래그 정의, mutex 체크, deprecation warning을 모두 삭제.
- **Estimated effort**: S
- **Risk**: low

### 현재 상태

`--run-once`/`--run-loop`은 pulse 자동화 도입 후 no-op deprecation 상태:

- `serveOptions`/`options` struct에 필드 보유
- 두 군데(server_main.go, main_cli.go)에서 mutex 체크 + warning 출력
- 두 군데에서 플래그 정의 (TN-002와 같은 중복 패턴)

### 제안하는 변경

CLAUDE.md 가이드라인 적용:
> Avoid backwards-compatibility hacks ... If you are certain that something is unused, you can delete it completely.

옵션 필드, 플래그 정의, mutex 체크, deprecation warning을 모두 삭제.

### 영향 범위

- 변경되는 파일: `cmd/tars/server_main.go`, `internal/tarsserver/main_cli.go`, `internal/tarsserver/main_options.go` (필드 정의 위치 확인 필요), `TestRun_MutuallyExclusiveRunFlags` 삭제
- 외부 호환성: deprecated 상태였음. 외부 자동화 스크립트가 여전히 플래그를 넘기면 cobra가 unknown flag 에러를 던질 것 → CHANGELOG에 명시.
- TN-002와 함께 처리하면 자연스럽게 흡수됨.

---

## RF-002 — `setupRuntimeLogger` 두 번째 호출의 cleanup 누수

- **Status**: open
- **Location**: `internal/tarsserver/main_cli.go:73-78`
- **Discovered in**: [journal/2026-04-25-02-server-bootstrap.md](../journal/2026-04-25-02-server-bootstrap.md)
- **Recommendation**: config 로드를 logger 셋업보다 먼저 수행 → logger를 단 한 번만 셋업.
- **Estimated effort**: S
- **Risk**: low

### 현재 상태

부트스트랩 흐름:

1. `Serve()` (main.go:40) — CLI 옵션만으로 첫 번째 logger + lumberjack 핸들 생성. `defer cleanup()`로 종료 시 닫힘 보장.
2. `newRootCmd`의 RunE (main_cli.go:43) — config 로드 후 config 값이 우선이면 두 번째 `setupRuntimeLogger` 호출. 두 번째 cleanup은 `_ = newCleanup` (line 77)로 의도적으로 버림.
3. 결과: 두 번째 lumberjack 핸들은 영원히 닫히지 않음. 프로세스 종료 시 OS가 회수하므로 실질 피해는 미미하나 명백한 리소스 누수.

코멘트 "previous cleanup runs via deferred Serve()"는 부정확 — `defer cleanup()`이 닫는 건 첫 번째 핸들뿐.

### 제안하는 변경

선택지:

- (a) 두 번째 `setupRuntimeLogger` 호출 시 첫 번째 cleanup을 명시적으로 호출하고 두 번째 cleanup을 새 defer로 등록. `Serve()`에 cleanup pointer를 노출하는 구조 변경 필요.
- (b) **권장**: config 로드를 logger 셋업보다 먼저 수행 → logger를 단 한 번만 셋업. 단 config 로드 자체가 실패하면 그 에러를 어디에 로깅할지 결정 필요(stderr fallback).
- (c) config의 logger 관련 필드를 더 일찍(예: ConfigPath만 받아 가벼운 partial load) 읽어와서 한 번에 셋업.

### 영향 범위

- 변경되는 파일: `internal/tarsserver/main.go`, `main_cli.go`
- 테스트: `TestRun_LogFileWritesJSONLines`, `TestSetupRuntimeLogger_CreatesParentDirForLogFile` 회귀 확인 필요
- TN-002 (방향 2) 진행 시 코드 구조가 바뀌므로 함께 처리하는 게 효율적

---

## RF-003 — `sessionStoreResolver` 의 잉여 1차 초기화

- **Status**: open
- **Location**: `internal/tarsserver/main_bootstrap.go:78-83`
- **Discovered in**: [journal/2026-04-25-03-runtime-deps.md](../journal/2026-04-25-03-runtime-deps.md)
- **Recommendation**: 첫 번째 `nil` 초기화 줄 제거.
- **Estimated effort**: S
- **Risk**: low

### 현재 상태

```go
deps := runtimeDeps{
    cfg:                  cfg,
    sessionStore:         session.NewStore(cfg.WorkspaceDir),
    sessionStoreResolver: newWorkspaceSessionStoreResolver(cfg.WorkspaceDir, nil),  // ← (1) nil로
}
deps.sessionStoreResolver = newWorkspaceSessionStoreResolver(cfg.WorkspaceDir, deps.sessionStore)  // ← (2) 즉시 덮어씀
```

(1)에서 만든 resolver는 어디에서도 사용되지 않은 채 (2)에서 덮어쓰임. 잉여 객체 생성 + 의도 모호.

### 제안하는 변경

(1) 줄 제거. struct 리터럴에서 `sessionStoreResolver` 필드를 빼고, 초기화는 (2) 한 줄로만 수행.

또는 더 깔끔히 `newWorkspaceSessionStoreResolver`가 nil-store fallback 로직을 자체적으로 갖고 있다면, 한 번만 호출하도록 조정.

### 영향 범위

- 변경되는 파일: `internal/tarsserver/main_bootstrap.go`
- 테스트: 없을 가능성 높음. 회귀 위험 거의 없음.

---

## RF-004 — `runtimeDeps.llmClient` backward-compat 필드 마이그레이션 완료

- **Status**: open
- **Location**: `internal/tarsserver/main_bootstrap.go:21-29, 124-134`
- **Discovered in**: [journal/2026-04-25-03-runtime-deps.md](../journal/2026-04-25-03-runtime-deps.md)
- **Recommendation**: pulse / reflection / compaction 호출처를 `deps.llmRouter` 기반으로 마이그레이션 후 `llmClient` 필드 제거.
- **Estimated effort**: M
- **Risk**: medium (호출처 다수, 각각 적절한 Role 결정 필요)

### 현재 상태

코드 자체가 부채를 명시적으로 자백:

> `llmClient is the chat-main tier client, kept for backward compat with call sites that have not yet been migrated to llmRouter. New code should request a client from llmRouter via a Role.`
>
> `chatClient is the tier-resolved client for the main chat role. It is stored on deps.llmClient for backward compatibility ... Non-chat call sites (pulse, reflection, compaction) will migrate in follow-up PRs and start requesting clients from deps.llmRouter directly.`

→ Router 도입은 끝났지만 호출처 마이그레이션이 미완. 두 필드(`llmClient` + `llmRouter`)가 동시에 살아 있음.

### 제안하는 변경

1. `llmClient` 사용처를 grep으로 전수 조사.
2. 각 사용처가 어떤 Role(chat / pulse / reflection / compaction 등)을 요구하는지 파악.
3. 사용처를 `deps.llmRouter.ClientFor(role)`로 전환.
4. `runtimeDeps.llmClient` 필드 + L129-134의 사이드 채우기 코드 제거.

### 영향 범위

- 변경되는 파일: `runtimeDeps`를 받는 모든 호출처. 최소 pulse, reflection, compaction 관련 코드.
- 테스트: 각 마이그레이션 단계에서 회귀 확인.
- TN-002 / RF-001 / RF-002와 독립. 별도 PR로 분리 가능.

---

## RF-005 — `daily_log` 좀비 case 라벨 제거

- **Status**: open
- **Location**: `internal/tarsserver/main_cli.go:95-96`
- **Discovered in**: [journal/2026-04-25-04-plugin-lifecycle.md](../journal/2026-04-25-04-plugin-lifecycle.md)
- **Recommendation**: 해당 case 라벨을 그냥 삭제. (`runtimeDepsError` switch의 default가 일반 에러 처리하므로 영향 없음.)
- **Estimated effort**: S
- **Risk**: trivial

### 현재 상태

`main_cli.go:95-96`:

```go
case "daily_log":
    logger.Error().Err(depErr.err).Msg("failed to write daily log")
```

전체 `internal/` 트리에 `stage: "daily_log"` 를 만드는 코드가 **0건**. 이 case는 절대 실행되지 않는 dead branch. ([Q-003](../questions.md#q-003) 추적 결과)

### 제안하는 변경

case 라벨 2줄 삭제. 끝.

### 영향 범위

- 변경되는 파일: `internal/tarsserver/main_cli.go` (2줄)
- 테스트: 영향 없음.

---

## RF-006 — `telegramDeliveryCounter`의 pulse wiring 완성

- **Status**: open
- **Location**: `internal/tarsserver/main_serve_api.go:163-168`
- **Discovered in**: [journal/2026-04-25-04-plugin-lifecycle.md](../journal/2026-04-25-04-plugin-lifecycle.md)
- **Recommendation**: `pulse.Runtime` 생성자에 `DeliveryFailureCounter` 인터페이스 인자 추가, 여기서 `telegramDeliveryCounter`를 넘기도록 wire.
- **Estimated effort**: S~M (pulse 생성 위치/시그니처 변경 필요)
- **Risk**: low

### 현재 상태

```go
telegramDeliveryCounter := newTelegramDeliveryCounter(100)
telegramSender := newTelegramCountingSender(
    newTelegramSender(cfg.TelegramBotToken),
    telegramDeliveryCounter,
)
_ = telegramDeliveryCounter // retained for pulse wiring in a later commit
```

코멘트가 미완을 자백. CLAUDE.md는 명시적으로 다음을 약속:

> Pulse ... reads ... the telegram delivery counter directly through narrow Go interfaces (`CronJobLister`, `GatewayRunLister`, `DiskStatProvider`, `DeliveryFailureCounter`).

→ 가이드라인과 구현 사이 gap. pulse 입장에서 telegram 전송 실패를 관찰할 수 있어야 한다는 설계가 코드엔 안 박힘.

### 제안하는 변경

1. `internal/pulse/` 에 `DeliveryFailureCounter` 인터페이스 정의 (또는 이미 있으면 재사용).
2. `pulse.NewRuntime(...)` 생성자에 해당 인터페이스 인자 추가.
3. `buildAPIMux` 의 pulse 생성 시점에 `telegramDeliveryCounter`를 인자로 넘김.
4. `_ = telegramDeliveryCounter` 라인 제거.
5. pulse 신호 분류기(`pulse_decide` 입력)에 telegram 실패 시그널을 포함.

### 영향 범위

- 변경되는 파일: `internal/pulse/`, `internal/tarsserver/main_serve_api.go`
- 테스트: pulse 분류 테스트에 telegram 실패 케이스 추가
- 잠재 의존: Q-004(pulse vs watchdog 역할)가 정리되어야 wiring 위치가 명확. 우선순위 조정 필요.

---

## RF-007 — 빌트인 플러그인 시스템 자체 제거

- **Status**: open
- **Location**:
  - `internal/plugin/builtin_registry.go` (RegisterBuiltin / BuiltinPlugins)
  - `internal/plugin/builtin.go` (BuiltinPlugin 인터페이스)
  - `internal/extensions/manager.go:587-603` (initBuiltinPlugins), L606-641 (`go_plugin/builtin:` 분기), L644-650 (CollectHTTPHandlers — 빌트인만 노출)
  - `internal/browserplugin/` (현재 유일한 사용자)
  - `cmd/tars/main.go:8` (`_ "internal/browserplugin"` import)
- **Discovered in**: [journal/2026-04-25-05-extensions-core.md](../journal/2026-04-25-05-extensions-core.md)
- **Recommendation**: 빌트인 플러그인 시스템 전체 제거. browserplugin은 외부 MCP 서버 / sidecar 프로세스 / skill+CLI 중 하나로 마이그레이션.
- **Estimated effort**: L (browserplugin 재배치 비용 포함)
- **Risk**: medium (HTTP API `/v1/browser/*` 의존하는 콘솔 UI 페이지도 함께 이동/deprecate 필요)

### 사용자 의도

> "빌트인 툴(`tool.Registry`)은 성능과 보안, 그리고 유용성 측면에서 필요하지만 빌트인 플러그인은 너무 과하다."

빌트인 툴(filesystem ops, web search, memory, gateway, telegram 등)은 모든 세션이 사용하는 인프라라 컴파일인이 정당. 그러나 빌트인 플러그인은:

- 도메인 기능을 코어 바이너리에 박아넣음 → 코어 비대화
- 사용자가 끄고 싶어도 컴파일 인 → 비활성만 가능 (런타임 disable)
- 새 도메인 기능 추가 시마다 코어 PR 필요 → 외부 ecosystem 발전 저해
- 결과적으로 [TN-001](../tensions.md#tn-001)(browserplugin이 가이드라인과 충돌)의 구조적 원인

### 제안하는 변경 — 두 단계 작업

**1단계: browserplugin 마이그레이션 (사전 작업, 필수)**

옵션 비교:

| 옵션 | 장점 | 단점 |
|------|------|------|
| (A) 외부 MCP 서버 | 기존 ecosystem(Chrome DevTools MCP) 활용. 표준 프로토콜. | vault/OTP 의존성을 별도 secrets API로 노출 필요 |
| (B) skill + companion CLI | CLAUDE.md 권장 패턴 그대로. 콘솔 UI 부담 줄임. | vault/OTP 통합 까다로움 (CLI에서 비밀 안전 전달) |
| (C) sidecar 프로세스 + 좁은 IPC | 기존 vault 통합 그대로 보존 | 가장 무거움 + 새 IPC 표면 추가 |

콘솔 UI의 `/v1/browser/*`, `/v1/vault/status` 페이지도 함께 이동 또는 deprecate 결정.

**2단계: 빌트인 플러그인 시스템 제거**

- `internal/plugin/builtin_registry.go` 삭제
- `BuiltinPlugin` 인터페이스 삭제
- `extensions.Manager.initBuiltinPlugins` 삭제
- `collectToolProviderTools`의 `go_plugin/builtin:` 분기 삭제 (TN-003과 함께)
- `CollectHTTPHandlers` 정책 결정 (RF-009 참조 — 외부 플러그인 HTTP 노출 정책)
- `cmd/tars/main.go`의 `_ "internal/browserplugin"` import 제거
- `internal/browserplugin/` 디렉토리 삭제
- `extensions/manager_test.go`의 builtin 케이스 삭제
- CLAUDE.md "When a builtin Go plugin IS appropriate" 섹션 업데이트 → "빌트인 도구만 정당, 빌트인 플러그인은 안 됨"

### 영향 범위

- 변경: 위 모든 파일 + 콘솔 UI
- **순서 강제**: 반드시 (1) browserplugin 마이그레이션 → (2) 시스템 제거. 순서 바뀌면 빌드 깨짐.
- 연관 finding: [TN-001](../tensions.md#tn-001) 흡수, [TN-003](../tensions.md#tn-003) 함께 정리, [RF-009](#rf-009)와 결합 결정

---

## RF-008 — Lifecycle 훅의 임의 shell 명령 실행 제거 + 안전한 표면으로 대체

- **Status**: open
- **Location**: `internal/extensions/lifecycle.go:18-56` (`runLifecycleHooks`)
- **Discovered in**: [journal/2026-04-25-05-extensions-core.md](../journal/2026-04-25-05-extensions-core.md)
- **Recommendation**: `sh -c` 임의 명령 실행을 제거하고, 허용된 표면(TARS 빌트인 툴 또는 빌트인 툴 + MCP tools)만 호출 가능하게 변경.
- **Estimated effort**: M
- **Risk**: high — 사용자 결정사항: 보안 위험이 크므로 빠른 cut-over 권장

### 위협 모델 (사용자가 "치명적"이라고 평가)

```go
c := exec.CommandContext(hookCtx, "sh", "-c", cmd)
c.Dir = p.RootDir
```

플러그인 정의 파일의 `lifecycle.on_start` / `on_stop` 문자열이 그대로 sh -c로 실행됨:

- **공급망 위협**: `tars plugin install <외부 source>`로 install되는 플러그인이 정의 파일에 임의 명령을 박을 수 있음. install 시점에 별도 검증/sandbox 없음.
- **워크스페이스 침해**: 플러그인 정의 파일이 어떤 경로로든 workspace에 떨어지면 다음 reload에서 즉시 실행됨 (다른 침해된 도구가 정의 파일을 떨어뜨리는 시나리오).
- **자격 증명 노출**: 훅이 실행되는 컨텍스트는 사용자 셸 환경 — `~/.aws`, `~/.kube`, OAuth 토큰, vault 비밀번호 등에 모두 접근 가능.

### 추가 위협 표면 — 플러그인 정의의 `mcp_servers` 필드 (session 06 보강)

`internal/plugin/types.go:50` 의 `Manifest.MCPServers []config.MCPServer` — 플러그인이 자체 MCP 서버를 declare 가능. `extensions.Manager`의 `PluginsAllowMCPServers` 가드가 활성화되면:

- 외부 install 플러그인이 임의 MCP 서버 추가 가능
- MCP 서버 = 외부 프로세스 실행 (sh 훅과 같은 카테고리 위험)
- 자격 증명 노출 + 임의 명령 실행 가능

→ RF-008 결정과 같은 카테고리의 표면이므로 한꺼번에 정책 정리 필요. "허용된 MCP 서버 목록" + 사용자 명시 동의 같은 게이트가 같이 적용돼야 함.

### 제안하는 변경

`Lifecycle.OnStart` / `OnStop`의 의미를 "shell 명령 문자열"에서 **"허용된 호출 디스크립터"**로 변경.

옵션:

- **(a) TARS 빌트인 툴만 허용**: 훅이 호출 가능한 것은 `tool.Registry`에 등록된 빌트인 툴만. 정의 파일 형식 예:
  ```yaml
  lifecycle:
    on_start:
      tool: fs_read     # 빌트인 툴 이름
      args: { path: "manifest.json" }
  ```
  ⚠️ 단 `bash` 툴 자체가 임의 실행을 허용 → **`bash` 툴은 훅 화이트리스트에서 제외 필수**.

- **(b) 빌트인 툴 + MCP tools 허용**: 더 풍부한 표면. MCP 서버는 외부 프로세스라 격리됨 + 사용자가 명시적으로 install한 것이라 신뢰 가능.

- **(c) 사전 정의된 좁은 액션만**: `register_routes`, `prefetch_resources`, `cleanup_temp` 같이 미리 정의된 안전한 액션 목록. 가장 보수적.

→ 사용자 의견: (a) 또는 (b) 방향 검토. (c)는 미언급. 최종 결정은 RF-008 진행 시점에.

### 마이그레이션 우려

현재 어떤 외부 플러그인이 sh 훅을 사용 중인지 사전 조사 필요:

- `tars-skills` 레포 + 사용자 워크스페이스의 plugin 정의 grep
- `lifecycle.on_start:` / `lifecycle.on_stop:` 등장 빈도 + 패턴 분류
- 마이그레이션 가능성 평가

### 영향 범위

- 변경: `internal/extensions/lifecycle.go`, `internal/plugin/` (정의 스키마), 외부 플러그인 정의 파일 마이그레이션 가이드
- 호환성: 보안 위험이라 빠른 cut-over 권장. deprecation period는 짧게 또는 생략.

---

## RF-009 — 외부 플러그인의 HTTP 핸들러 노출 정책 결정

- **Status**: open (결정 보류 — 보안 트레이드오프 평가 필요)
- **Location**: `internal/extensions/manager.go:644-650` (`CollectHTTPHandlers`), `internal/plugin/` 정의 스키마
- **Discovered in**: [journal/2026-04-25-05-extensions-core.md](../journal/2026-04-25-05-extensions-core.md)
- **Recommendation**: RF-007 진행 시점에 함께 결정. 사용자 의견: 보안 문제 때문에 결정을 못하겠다 → 옵션 비교 후 별도 합의 필요.

### 현재 상태

`CollectHTTPHandlers`는 빌트인 플러그인의 HTTP 핸들러만 모아서 mux에 등록. 외부 플러그인은 HTTP 라우트 노출 불가. 의도된 격리인지 미구현인지 불명확.

RF-007 진행 시 빌트인이 사라지므로 `CollectHTTPHandlers`는 빈 함수가 됨 → 외부 플러그인의 HTTP 노출 정책 결정이 필요해짐.

### 보안 우려 (사용자가 짚어준 부분)

외부 플러그인이 HTTP 라우트를 등록하게 하면:

- **라우트 충돌**: 두 플러그인이 같은 경로 등록 → silent override 또는 panic
- **하이재킹**: 악성 플러그인이 `/v1/auth/login` 같은 코어 경로를 가로채서 자격 증명 탈취
- **인증 우회**: 코어 미들웨어 chain을 우회하는 라우트 등록 가능
- **CORS / 헤더 처리**: 잘못된 CORS 헤더 등록으로 정보 노출

### 옵션 비교

| 옵션 | 격리 강도 | 통합 ergonomics | 비고 |
|------|---------|----------------|------|
| (a) 외부 플러그인 HTTP 노출 불가 (현 상태 유지) | 최강 | 낮음 — 자체 포트로 sidecar 띄워야 함 | 가장 안전. RF-007 진행 시 `CollectHTTPHandlers` 함수 자체 삭제 |
| (b) sub-prefix sandboxing — `/v1/plugins/<id>/*`만 | 강 | 중 — 자기 namespace에서만 자유 | 코어 경로 하이재킹 불가, 충돌은 plugin id로 자동 namespacing, 인증 미들웨어 prefix 단위 강제. **현실적 절충안** |
| (c) manifest 검증 + 사용자 동의 후 자유 라우트 | 중 | 높음 | install 시 라우트 목록 사용자 동의(UAC 모델). manifest 위조 가능성 |
| (d) MCP resource serving으로 우회 | 강 | 중 | HTTP 직접 노출 안 함, MCP server가 resource serve → 코어가 별도 prefix로 proxy |

### 우선순위 종속성

이 결정은 RF-007과 강하게 묶임:

- RF-007 진행 + (a) → `CollectHTTPHandlers` 함수 자체 삭제
- RF-007 진행 + (b)/(c)/(d) → `CollectHTTPHandlers` 재설계

→ RF-007 진행 시점에 함께 결정.

### 영향 범위

- 변경 (옵션에 따라): `internal/extensions/manager.go`, `internal/plugin/` 정의 스키마, 미들웨어 chain
- 사용자 가이드: 플러그인 작성자에게 라우트 등록 규칙 문서화

---

## RF-011 — `Manifest` 와 `Definition` 의 필드 중복 → embed로 단순화

- **Status**: open
- **Location**: `internal/plugin/types.go:43-80`
- **Discovered in**: [journal/2026-04-25-06-plugin-pkg.md](../journal/2026-04-25-06-plugin-pkg.md)
- **Recommendation**: `Definition`이 `Manifest`를 embed하도록 변경. 동기화 부담 제거.
- **Estimated effort**: S
- **Risk**: low (JSON 직렬화 호환성 확인 필요)

### 현재 상태

`Manifest`(L43-59)와 `Definition`(L61-80)이 12개 공통 필드를 각자 정의:

| 그룹 | 필드 |
|------|------|
| 공통 12개 | SchemaVersion, ID, Name, Description, Version, Skills, MCPServers, Requires, SupportedOS, SupportedArch, DefaultProjectProfile, Policies, ToolsProvider, Lifecycle, HTTPRoutes |
| Definition 추가 3개 | Source, RootDir, ManifestPath |

### 문제

매니페스트 스키마에 새 필드 추가 시 `Definition`도 함께 수정해야 동기화 유지. 잊으면 silent skew (필드가 매니페스트에는 있는데 런타임 객체엔 없음).

### 제안하는 변경

```go
type Definition struct {
    Manifest                                    // embed
    Source       Source `json:"source"`
    RootDir      string `json:"root_dir"`
    ManifestPath string `json:"manifest_path"`
}
```

embed 시 JSON 직렬화 결과는 동일(필드가 평탄하게 marshal됨). Go 1.17+에서 안정적. 호출처에서 `def.ID`, `def.Name` 그대로 작동 (Go embed 자동 promotion).

### 영향 범위

- 변경: `internal/plugin/types.go`, 호출처에서 `Definition` 필드 직접 접근(예: `def.ID`, `def.Name`) 그대로 동작
- JSON 호환성 확인: 기존 직렬화/역직렬화 테스트 통과 필수 (`manifest_test.go`, `registry_test.go`)
- extensions/manager.go의 `bp.Definition()` 호출처 회귀 확인

### 가장 큰 수혜처 (session 06 보강)

`internal/plugin/loader.go:87-109` — `loadSourcePlugins`가 Manifest의 12개 필드를 Definition으로 일일이 복사하는 23줄 boilerplate. RF-011 적용 시 ~5줄로 축소:

```go
// 적용 후
definition := Definition{
    Manifest:     manifest,
    Source:       source,
    RootDir:      rootDir,
    ManifestPath: path,
}
```

새 매니페스트 필드 추가 시 이 변환 코드 동기화 부담도 함께 사라짐.

### 잠재 의존

RF-007(빌트인 플러그인 제거) 진행 시 `BuiltinPlugin.Definition()` 메서드 자체가 사라지지만, 외부 플러그인 로딩 경로에서 여전히 `Manifest → Definition` 변환이 일어나므로 본 RF는 독립적으로 가치 있음.

---

## RF-012 — Legacy "tarsncase" 잔재의 단계적 deprecation

- **Status**: open
- **Location**:
  - `internal/tarsserver/helpers_build_extensions.go:52` (`~/.tarsncase/skills` source)
  - `internal/tarsserver/helpers_build_extensions.go:77` (`~/.tarsncase/plugins` source)
  - `internal/plugin/loader.go:15` (`legacyManifestFilename = "tarsncase.plugin.json"`)
  - `internal/plugin/registry_test.go:85` (legacy 호환성 테스트)
- **Discovered in**: [journal/2026-04-25-06-plugin-pkg.md](../journal/2026-04-25-06-plugin-pkg.md)
- **Recommendation**: **즉시 제거 X — 단계적 deprecation 작업.** 옛 사용자 데이터 보존 필요.
- **Estimated effort**: M (다단계, 시간 분산)
- **Risk**: low (단계적 진행 시) / high (즉시 제거 시 사용자 데이터 손실)

### 컨텍스트

"tarsncase"는 옛 프로젝트 이름. 현재 디렉토리/파일명은 "tars"로 변경됐지만 호환을 위해 옛 경로/파일명을 여전히 인식:

- 사용자 데이터 디렉토리: `~/.tarsncase/{plugins,skills}` (현재 `~/.tars/{plugins,skills}`)
- 매니페스트 파일명: `tarsncase.plugin.json` (현재 `tars.plugin.json`)

기존 사용자 워크스페이스에 옛 경로의 데이터가 남아 있을 수 있음. 무작정 제거하면 사용자가 자기 플러그인/스킬을 잃음.

머지 우선순위는 안전 — `~/.tars`가 `~/.tarsncase`보다 나중에 쌓여서 같은 ID가 양쪽에 있으면 신 경로가 이김. 하지만 단방향 마이그레이션 자동화는 없음.

### 단계적 제거 계획

1. **단계 1 (즉시)**: deprecation warning 자동 발견
   - `~/.tarsncase/{plugins,skills}` 디렉토리 발견 시 reload마다 warning 로그
   - `tarsncase.plugin.json` 인식 시 warning + "rename to tars.plugin.json" 메시지
   - 콘솔 UI에도 노란 배지 표시 (정보 제공)

2. **단계 2 (CHANGELOG)**: 마이그레이션 가이드
   ```
   mv ~/.tarsncase/plugins/* ~/.tars/plugins/
   mv ~/.tarsncase/skills/* ~/.tars/skills/
   find ~/.tars -name 'tarsncase.plugin.json' -exec rename ... \;
   ```
   다음 릴리스 노트에 명시.

3. **단계 3 (자동 마이그레이션, 옵션)**: 첫 부팅 시 빈 `~/.tars/`이고 `~/.tarsncase/`가 차 있으면 자동 복사 + 안내 메시지.

4. **단계 4 (제거)**: 다음 메이저 버전 또는 N개월 deprecation period 후:
   - `helpers_build_extensions.go`의 `~/.tarsncase` 두 줄 삭제
   - `loader.go`의 `legacyManifestFilename` 삭제
   - `manifestPriority` 단순화 (priority 분기 제거)
   - `registry_test.go:85` 테스트 갱신

### 영향 범위

- 단계 1만으로 즉시 가치 (사용자 인지 + 마이그레이션 유도)
- 단계 4는 BC 끊는 cut-over → 메이저 버전 정책에 맞춰 조정

---

## RF-013 — `Source` enum에 우선순위 메서드 추가 + `Load`에서 자동 정렬

- **Status**: open
- **Location**: `internal/plugin/types.go:35-41` (Source enum), `internal/plugin/loader.go:18-49` (Load의 merge)
- **Discovered in**: [journal/2026-04-25-06-plugin-pkg.md](../journal/2026-04-25-06-plugin-pkg.md)
- **Recommendation**: `Source.Priority() int` 메서드 정의 + `Load`가 호출자 슬라이스 순서 무시하고 우선순위로 자동 정렬.
- **Estimated effort**: S
- **Risk**: low

### 현재 상태

`Load(opts)` 가 source 우선순위를 호출자 슬라이스 순서로만 결정. 호출자가 정렬을 안 하거나 잘못 정렬하면 silent skew.

[Q-008](../questions.md#q-008) 답: 현재 호출자(`buildPluginSources`)는 의도된 순서로 쌓고 있어서 결과는 합리적(workspace > user > bundled). 그러나:

- `Source` enum 자체엔 우선순위 메타 없음
- 새 호출자/source 추가 시 정렬 잊으면 silent skew
- 코드 리뷰 시 "왜 이 순서인가"가 호출자 코드를 봐야 알 수 있음

### 제안하는 변경

```go
// types.go
func (s Source) Priority() int {
    switch s {
    case SourceWorkspace: return 3
    case SourceUser:      return 2
    case SourceBundled:   return 1
    default:              return 0
    }
}

// loader.go: Load의 merge 직전에 우선순위로 자동 정렬
sort.SliceStable(opts.Sources, func(i, j int) bool {
    return opts.Sources[i].Source.Priority() < opts.Sources[j].Source.Priority()
})
// 그 후 단순 덮어쓰기 머지 → 결과는 항상 high priority가 이김
```

이렇게 하면:

- 우선순위 의도가 코드에 명시됨 (Priority 메서드)
- 호출자 슬라이스 순서 무관 — 항상 일관된 결과
- 새 source 추가 시 Priority만 정의하면 자동 동작

### 영향 범위

- 변경: `internal/plugin/types.go`, `internal/plugin/loader.go`, 테스트 케이스 추가 (순서 셔플해도 결과 동일)
- 호출처: 변경 불필요 (호환). 다만 `helpers_build_extensions.go`에서 의도적으로 같은 SourceUser 안에서 .tarsncase < .tars 순서를 두는데, 이건 RF-013 적용 후에도 같은 SourceUser 내 정렬은 안 바뀜(stable sort) → 안전.

---

## RF-014 — Silent error swallowing 패턴에 최소 로그 또는 startup 검증 추가

- **Status**: open
- **Location**:
  - `internal/pulse/runtime.go:270-272` (withinActiveHours 파싱 fallback)
  - `internal/pulse/signal.go:176-178, 271-274, 232-234` (scan 함수들의 fetch 에러 swallowing)
  - `internal/pulse/signal.go:341-348` (parseRunTimestamp의 무성공 fallback)
- **Discovered in**: [journal/2026-04-25-07-pulse.md](../journal/2026-04-25-07-pulse.md)
- **Recommendation**: silent fallback 자체는 유지(watchdog이 잘못된 입력으로 죽지 않게 보호하는 정당한 설계), 다만 (a) 발생 시 최소 로그 또는 (b) startup 시 1회 검증으로 잘못된 입력을 빨리 발견.
- **Estimated effort**: S
- **Risk**: low

### 현재 상태

pulse 패키지가 **fail-soft** 원칙을 일관되게 적용:

- `withinActiveHours`가 ActiveHours 파싱 실패 시 silently `(true, "")` 반환 → 항상 active. 코멘트는 *"unlike the previous heartbeat implementation, we do not silently fall back to defaults"* 라고 자랑하지만, 실제로는 silent fallback 발생.
- `scanCron`/`scanDisk` 등이 source fetch 에러 시 silent nil 반환 → 시그널 안 만들고 진행.
- `parseRunTimestamp`가 RFC3339Nano + RFC3339 둘 다 실패하면 silent zero time + false 반환.

설계 의도: **한 영역의 실패가 watchdog 전체를 죽이지 않게 격리**. 정당함.

### 문제

운영자가 ActiveHours를 잘못 적었어도, 또는 cron store가 일시적으로 오류 응답을 줘도, **로그 없음 + 알림 없음** → 디버깅 어려움. "왜 pulse가 안 도는지" 확인하기 위해 코드를 따라가야 함.

### 제안하는 변경

옵션 (조합 가능):

- **(a) Startup 1회 검증**: `Runtime.Start(ctx)` 진입 시 `parseActiveHours(cfg.ActiveHours)`를 실행해서 실패하면 logger에 ERROR 1줄 + 그래도 진입(fail-soft 유지). 이렇게 하면 운영자가 부팅 직후 로그만 봐도 잘못된 설정을 발견.
- **(b) Silent fallback 발생 시 logger.Warn**: 첫 발생만 로그(반복 폭발 방지). `sync.Once` 패턴.
- **(c) 메트릭 카운터**: state에 `silent_fallback_count`를 추가해 Snapshot에 노출. 콘솔 UI에서 가시화.

→ (a) 가장 비용 적고 효과 큼. 우선순위.

### 영향 범위

- 변경: `internal/pulse/runtime.go`, `internal/pulse/signal.go`
- 테스트: 잘못된 ActiveHours로 Runtime 생성 시 startup 로그 발생 검증
- 호환성: 동작 자체는 그대로(fail-soft 유지). 로그/메트릭만 추가.

### Reflection의 strict 패턴이 모범 (session 08 보강)

같은 코드베이스의 reflection은 동일한 시간 윈도우 처리에서 다른 정책을 채택:

| 컴포넌트 | 잘못된 윈도우 처리 |
|---------|------------------|
| pulse `withinActiveHours` | silent always-active (코멘트는 "no silent fallback"이지만 호출자가 fallback) |
| reflection `decideTick` | strict skip with `"invalid_sleep_window"` reason |

reflection 쪽이 더 안전한 선택 — 잘못된 설정으로 야간 배치가 안 도는 게 watchdog이 잘못 도는 것보다 낫다. RF-014 진행 시 pulse도 reflection처럼 strict로 통일하는 게 자연스러움.

### memory 패키지 사례 보강 (session 09)

같은 silent failure 카테고리의 추가 사례:

- `internal/memory/file_backend.go:62-64` — `_ = b.semantic.IndexExperience(context.Background(), exp)` — 에러 silent + caller ctx 무시 (`context.Background()` 사용으로 cancellation 안 먹힘)
- `internal/memory/semantic.go:468-470` — `loadEntries` 의 JSON unmarshal 실패 시 silent continue → 손상 엔트리 인지 못함

→ RF-014 적용 시 함께 정리. 특히 `IndexExperience`의 bg ctx는 caller 의도 위반이므로 우선순위 높음.

---

## RF-015 — `internal/timewindow/` 같은 공통 패키지로 system surface 헬퍼 추출

- **Status**: open
- **Location**:
  - `internal/pulse/runtime.go:288-327` (parseActiveHours, parseClockMinutes)
  - `internal/reflection/scheduler.go:90-130` (parseSleepWindow, parseClockMinutes — 거의 동일)
  - `internal/reflection/derivation.go:124-130` (trimText — `internal/tarsserver` 헬퍼와 중복, cycle 회피용)
- **Discovered in**: [journal/2026-04-25-08-reflection.md](../journal/2026-04-25-08-reflection.md)
- **Recommendation**: 의도된 중복은 정당하지만 새 system surface 추가 시 또 복사될 위험. 작은 공통 패키지(`internal/timewindow/` 또는 `internal/textutil/` 활용)로 추출 검토.
- **Estimated effort**: S
- **Risk**: low

### 컨텍스트

같은 코드가 두 개의 system surface 패키지에 의도적으로 복사됨:

`scheduler.go:90-91` 코멘트:
> Matches pulse's parser but lives here to avoid cross-package dependencies in the core types.

`derivation.go:121-123` 코멘트:
> trimText is a local copy of the tarsserver trimForMemory helper. Duplicated rather than imported to avoid a tarsserver→reflection cycle.

→ **cycle 회피가 정당한 이유**. 하지만 새 system surface(예: 향후 `internal/observe/` 같은) 추가 시 또 복사될 위험.

### 제안

작은 leaf 패키지에 추출:

- `internal/timewindow/` — `ParseClockMinutes(s)`, `WindowContains(start, end, now)`, `WrapAroundAnchor(local, start, end)` 등. import 그래프 leaf라 cycle 없음.
- `internal/textutil/` (기존 패키지) — `TrimForMemory(s, max)` 추가 가능.

→ 새 system surface가 같은 헬퍼 필요 시 한 줄 import로 끝.

### 영향 범위

- 변경: 새 leaf 패키지 추가 + pulse/reflection/tarsserver 의 `import` 라인 추가, 기존 함수 제거
- 테스트: 옮긴 함수의 단위 테스트도 함께 이전
- 리스크 낮음 (순수 함수)

### 우선순위

낮음. 두 개의 사례로는 추상화 비용이 미미. 셋째 system surface 추가 시점에 함께 처리하는 게 ROI 높음.

---

## RF-016 — `compileKnowledge` 의 silent failure 모드에 errs 누적 추가

- **Status**: open
- **Location**: `internal/reflection/job_memory.go:198-267` (compileKnowledge)
- **Discovered in**: [journal/2026-04-25-08-reflection.md](../journal/2026-04-25-08-reflection.md)
- **Recommendation**: 5개 silent return false 경로에 errs 누적 추가 → `JobResult.Details["errors"]`에 모임. 같은 패키지의 다른 코드와 일관성 회복.
- **Estimated effort**: S
- **Risk**: low

### 현재 상태

`compileKnowledge` 가 5개 실패 경로에서 silent하게 false 반환:

```go
client, _, err := m.Router.ClientFor(...)
if err != nil { return false }                  // (1) router 실패
existing, err := backend.ListKnowledgeNotes(...)
if err != nil { return false }                  // (2) backend 실패
resp, err := client.Chat(...)
if err != nil { return false }                  // (3) LLM 호출 실패
if err := json.Unmarshal(...); err != nil { return false }  // (4) JSON 형식 위반
if err := backend.ApplyKnowledgeUpdate(...); err != nil { return false }  // (5) DB write 실패
```

같은 패키지의 다른 코드는 errs를 잘 누적:

- `MemoryJob.Run` 의 `errs []string` (L86)
- `KBCleanupJob.Run` 의 `errs []string` (L68)

→ **`compileKnowledge`만 silent**. nightly 마다 LLM 형식 위반이 누적되면 운영자가 모름.

### 제안하는 변경

- `compileKnowledge` 시그니처를 `(updated bool, errs []string)` 또는 `(bool, error)` 로 변경
- 호출처(`Run`)에서 errs를 본인의 errs 슬라이스에 합침
- 결과적으로 `JobResult.Details["errors"]`에 LLM/JSON/DB 실패 누적

각 실패 모드별로 구분 가능한 prefix 권장 (`compile router: ...`, `compile chat: ...`, `compile json: ...`, `compile apply: ...`).

### 영향 범위

- 변경: `internal/reflection/job_memory.go`
- 테스트: silent failure 케이스가 errs에 모이는지 검증 추가
- 리스크 낮음 (순수 가시성 개선, 동작 그대로)

---

## RF-017 — `entries.jsonl` 의 O(N) load+rewrite 패턴 — 인덱싱 구조 개선

- **Status**: open
- **Location**: `internal/memory/semantic.go:401-442` (`upsertEmbeddedEntry`), `internal/memory/semantic.go:452-496` (`loadEntries`/`saveEntries`)
- **Discovered in**: [journal/2026-04-25-09-memory-pt1.md](../journal/2026-04-25-09-memory-pt1.md)
- **Recommendation**: 엔트리 수 증가 시 매 indexing마다 전체 read+write — 확장성 한계. append-only JSONL + 별도 인덱스, 또는 SQLite 같은 경량 DB로 마이그레이션 검토.
- **Estimated effort**: L
- **Risk**: medium (저장 포맷 변경 + 마이그레이션 필요)

### 현재 상태

```go
func (s *Service) upsertEmbeddedEntry(ctx, entry, taskType) error {
    ...
    entries, err := s.LoadEntries()              // ← 전체 JSONL 로드
    for idx := range entries { ... }              // ← 선형 탐색으로 ID 매칭
    if !updated { entries = append(entries, entry) }
    return saveEntries(s.entriesPath(), entries)  // ← 전체 다시 저장 (atomic rename)
}
```

새 entry 추가 시마다 **JSONL 전체 read + 전체 write**. atomic file write라 안전하지만 비쌈.

엔트리 수 N이 1만이면 매 indexing마다 1만 entries 직렬화/IO. 야간 reflection에서 한 번에 다수 entries 인덱싱 시 비용 폭증.

### 옵션

- **(a) Append-only JSONL + 별도 인덱스 파일**: ID→file_offset 매핑 보유. update는 새 entry append + 인덱스 갱신. 주기적 compaction으로 dead 항목 정리. 변경 작음.
- **(b) SQLite (or BoltDB)**: 검증된 ACID + 인덱싱 + 트랜잭션. dependency 추가지만 안정. 마이그레이션 큼.
- **(c) 부분 read/write**: ID 기반 partition (예: 첫 2자리 hash로 16개 파일 분리). 임시 완화책.

→ Phase 1 minimum이라 현재 구조는 합리적. 사용량 늘어나는 시점에 (a) 또는 (b) 권장.

### 영향 범위

- 변경: `internal/memory/semantic.go` (저장/로드/upsert 패턴 전부)
- 마이그레이션: 기존 `entries.jsonl` → 새 포맷 변환 스크립트 또는 첫 부팅 시 자동
- 테스트: 동시 indexing/조회 시나리오 추가
- RF-016/RF-014와 독립

---

## RF-019 — `internal/memory/semantic.go` + `internal/prompt/builder.go` + `internal/tool/list_dir.go` dead code 6건 제거

- **Status**: open
- **Location**:
  - `internal/memory/semantic.go`
    - `indexState` + `loadIndexState`/`saveIndexState` (L182-192, 498-522, ~30줄)
    - `readDoc` (L532-546, ~15줄)
    - `firstMeaningfulParagraph` (L688-696, ~9줄)
    - 자체 `min(left, right int)` (L703-708, ~6줄)
  - `internal/prompt/builder.go`
    - 자체 `max(left, right int)` + `min(left, right int)` (L215-227, ~13줄)
  - `internal/tool/list_dir.go`
    - 자체 `minInt(a, b int)` (L172-177, ~6줄)
- **Discovered in**: [journal/2026-04-25-10-memory-pt2.md](../journal/2026-04-25-10-memory-pt2.md), [journal/2026-04-25-11-prompt.md](../journal/2026-04-25-11-prompt.md), [journal/2026-04-25-13-tool-file.md](../journal/2026-04-25-13-tool-file.md), [Q-009](../questions.md#q-009)
- **Recommendation**: 6개 모두 한꺼번에 제거. `go.mod`가 `go 1.25.6`이므로 built-in `min`/`max` 사용 안전.
- **Estimated effort**: S
- **Risk**: trivial

### 현재 상태

[Q-009](../questions.md#q-009) 답 — 전수 grep 결과 4개 모두 `semantic.go` 외부에서도, 같은 파일 안에서도 호출 0건. 옛 기능의 잔재.

### 제안하는 변경

- `indexState` 타입 + load/save 페어 삭제 (`memory/semantic_index.json` 같은 파일이 디스크에 있다면 cleanup 마이그레이션 필요 여부 확인)
- `readDoc` 삭제
- `firstMeaningfulParagraph` 삭제
- `min` 자체 함수 삭제 → built-in 사용

CLAUDE.md 가이드라인:

> Avoid backwards-compatibility hacks like renaming unused _vars, re-exporting types, adding // removed comments for removed code, etc. If you are certain that something is unused, you can delete it completely.

→ 그대로 적용.

### 영향 범위

- 변경: `internal/memory/semantic.go` (~70줄 감소)
- 테스트: 영향 없음 (dead code)
- `indexState` 의 디스크 흔적: `workspace/memory/index/state.json` 같은 파일이 남아 있을 가능성 — 검토 필요. 남아 있으면 (a) 그대로 두고 무시 (b) cleanup migration 추가. 사용자 데이터 손실 위험 없음.

---

## RF-020 — `KnowledgeStore.nowFn` 외부 주입 경로 부재 → 테스트 시점 제어 불가

- **Status**: open
- **Location**: `internal/memory/knowledge.go:226-240` (struct + NewKnowledgeStore)
- **Discovered in**: [journal/2026-04-25-10-memory-pt2.md](../journal/2026-04-25-10-memory-pt2.md)
- **Recommendation**: (a) `nowFn` 필드 제거하고 `time.Now().UTC()` 직접 호출 또는 (b) 옵션 패턴으로 외부 주입 경로 노출.
- **Estimated effort**: S
- **Risk**: low

### 현재 상태

`KnowledgeStore`는 `nowFn func() time.Time` 필드를 보유하지만:

- `NewKnowledgeStore(root, semantic)` 가 항상 default `time.Now().UTC()` 로 셋업
- 외부에서 `nowFn`을 set할 수 있는 경로 (옵션 인자, setter, exported 필드) **없음**
- 테스트도 (`knowledge_test.go`) 시점 제어 안 함 — 그냥 `time.Now()` 값 받음
- `FileBackend.knowledgeStore()` 가 매번 `NewKnowledgeStore` 호출 → 어차피 매번 default로 reset

→ `nowFn` 필드는 **dead capability** (코드는 있으나 누구도 활용 못 함).

### 제안하는 변경

선택지:

- **(a) 단순화**: `nowFn` 필드 삭제, `ApplyPatch` 안에서 `time.Now().UTC()` 직접 호출. 가장 작음.
- **(b) 외부 주입 경로 추가**: 옵션 패턴으로 `NewKnowledgeStore(root, semantic, opts ...Option)` — 테스트가 결정적인 시점 제어 가능. ApplyPatch 가 patch.UpdatedAt 의 `IsZero()` 분기를 갖고 있어서 호출자 시간 주입은 이미 가능 → (b)는 사실상 redundant.

→ **(a) 권장**.

### 영향 범위

- 변경: `internal/memory/knowledge.go` (3-4줄 감소)
- 테스트: 영향 없음
- 호출처: `FileBackend.knowledgeStore()` 매번 new 호출 비용은 미미 — 별도 RF 아님

### 미세 의문 (session 09 → 10) 해소

`knowledgeStore()` 매번 new — KnowledgeStore가 stateful인지: **stateless** 확정 (root + semantic 포인터 + nowFn 모두 immutable + side-effect는 디스크 IO뿐). 매번 new 패턴 자체는 정당. 다만 `nowFn` 필드 자체가 활용 안 됨이 문제.

---

## RF-021 — `KnowledgeStore.rebuildArtifacts` 의 O(N) 패턴

- **Status**: open
- **Location**: `internal/memory/knowledge.go:521-538` (rebuildArtifacts), L411-461 (List), L588-626 (buildKnowledgeIndex), L540-586 (buildKnowledgeGraph)
- **Discovered in**: [journal/2026-04-25-10-memory-pt2.md](../journal/2026-04-25-10-memory-pt2.md)
- **Recommendation**: 매 upsert/delete 마다 (1) 디스크의 모든 `*.md` 글로브 + 파싱 + (2) 전체 index.md 재작성 + (3) 전체 graph.json 재작성 — 노트 수 N이 증가하면 비용 폭증. 인덱스/그래프를 incremental update 또는 별도 잡으로 분리 검토.
- **Estimated effort**: M
- **Risk**: medium (저장 포맷/공정 변경)

### 현재 상태

```go
func (s *KnowledgeStore) ApplyPatch(patch ...) (KnowledgeNote, error) {
    ...
    if err := os.WriteFile(path, ..., 0o644); err != nil { ... }
    ...
    if err := s.rebuildArtifacts(); err != nil { ... }   // ← 매 upsert
    return note, nil
}

func (s *KnowledgeStore) rebuildArtifacts() error {
    items, err := s.List(KnowledgeListOptions{Limit: 10000})  // ← 모든 .md 파일 글로브 + 파싱
    ...
    if err := os.WriteFile(...index.md, buildKnowledgeIndex(items)); ...   // ← 전체 재작성
    encoded, _ := json.MarshalIndent(buildKnowledgeGraph(items), "", "  ")  // ← 전체 그래프 재구성
    if err := os.WriteFile(...graph.json, encoded); ...
}
```

`List` 가 모든 `*.md` 파일을 read + frontmatter 파싱 + filter sort. `rebuildArtifacts`는 limit=10000으로 호출 → 사실상 모든 노트 로드.

`ApplyUpdate` (nightly compileKnowledge가 호출)는 N개 note + M개 edge를 처리하는데, 각 note마다 Upsert → ApplyPatch → rebuildArtifacts → **N×M회 전체 재빌드 가능성**. 같은 nightly 호출에서 N=5, M=3이면 8번 전체 재빌드.

### 옵션

- **(a) Incremental index/graph**: 노트 ID → graph node/edge 매핑을 메모리 인덱스에 보유. upsert는 자기 노트의 entry만 갱신. 디스크 IO는 atomic write of single file.
- **(b) Batch rebuild**: ApplyUpdate 안에서 마지막 한 번만 rebuildArtifacts. 노트 단위 ApplyPatch를 batch-aware로 변경 (e.g. internal flag `skipRebuild bool`).
- **(c) Lazy rebuild**: rebuildArtifacts를 nightly reflection의 별도 잡으로 분리. write path는 단순 파일 작성만. 그래프는 nightly에 한 번만 재계산.

→ 사용자 KB가 작은 동안은 (b)가 가장 실용적. 노트 수가 100+ 가는 시점에 (a) 또는 (c) 검토.

### 영향 범위

- 변경: `internal/memory/knowledge.go` (`ApplyUpdate`, `ApplyPatch`, `rebuildArtifacts`)
- 테스트: index.md/graph.json 일관성 검증 추가
- RF-017(`entries.jsonl`)과 같은 카테고리 — 둘 다 "small N에선 무해, large N에서 폭증"

---

## RF-022 — `KnowledgeStore.Graph()` 의 자기치유 로직 단순화

- **Status**: open
- **Location**: `internal/memory/knowledge.go:474-515`
- **Discovered in**: [journal/2026-04-25-10-memory-pt2.md](../journal/2026-04-25-10-memory-pt2.md)
- **Recommendation**: graph.json 누락/손상 시 rebuildArtifacts 자동 호출 + `"updated_at": ""` 문자열 매직 검사를 제거하고, "graph.json은 항상 rebuildArtifacts가 만든다"는 invariant를 코드/테스트로 강제.
- **Estimated effort**: S
- **Risk**: low

### 현재 상태

```go
func (s *KnowledgeStore) Graph() (KnowledgeGraph, error) {
    raw, err := os.ReadFile(...graph.json)
    if err != nil {
        if os.IsNotExist(err) {
            if err := s.rebuildArtifacts(); err != nil { ... }   // 자동 재빌드
            raw, err = os.ReadFile(...graph.json)
        }
        if err != nil { return ..., err }
    }
    var graph KnowledgeGraph
    if err := json.Unmarshal(raw, &graph); err != nil {
        if rebuildErr := s.rebuildArtifacts(); rebuildErr != nil { ... }  // unmarshal 실패도 재빌드
        raw, err = os.ReadFile(...graph.json)
        ...
        if err := json.Unmarshal(raw, &graph); err != nil { ... }
    }
    ...
    if strings.Contains(string(raw), `"updated_at": ""`) {                 // 매직 문자열 검사
        if err := s.rebuildArtifacts(); err != nil { ... }                 // 또 재빌드
    }
    return graph, nil
}
```

세 가지 자기치유 경로:
1. 파일 누락 → rebuild
2. JSON unmarshal 실패 → rebuild
3. 빈 `updated_at` 문자열 발견 → rebuild

3번이 특히 문제: 옛 포맷 호환을 위해 raw bytes에 substring 검사. 한 번 정상화되면 다시 발생 안 하는 transient case라면 일회성 마이그레이션 스크립트가 더 깔끔.

### 제안하는 변경

- 누락/손상 모두 동일 경로(rebuildArtifacts → re-read)로 단일화
- `"updated_at": ""` 검사는 startup 시점에 1회 마이그레이션으로 분리 (또는 그냥 제거 — 다음 upsert 때 자동 정상화됨)
- `Graph()` 호출의 hot path를 단순 read+unmarshal 로 축소

### 영향 범위

- 변경: `internal/memory/knowledge.go` (Graph 함수만)
- 테스트: legacy `"updated_at": ""` 케이스를 마이그레이션 1회로 옮길지, 제거할지 결정
- 호환성: 첫 호출에 자기치유가 일어나므로 사용자 영향 없음

---

## RF-023 — `internal/prompt/memory_retrieval.go` 의 죽은 fallback 매처 제거

- **Status**: open
- **Location**: `internal/prompt/memory_retrieval.go:103-104` (caller), L174-182 (zombie functions), L445-448 (classifySourceTag dead branches)
- **Discovered in**: [journal/2026-04-25-11-prompt.md](../journal/2026-04-25-11-prompt.md)
- **Recommendation**: `collectProjectDocumentMatches` + `collectBriefMatches` 함수 + 호출 라인 + classifySourceTag의 `projects/` / `_shared/` 분기 모두 제거.
- **Estimated effort**: S
- **Risk**: trivial

### 현재 상태

```go
// memory_retrieval.go:174-182
func collectProjectDocumentMatches(_ BuildOptions, _ []string) []relevantMemoryMatch {
    // Project documents are no longer available after project package removal.
    return nil
}

func collectBriefMatches(_ BuildOptions, _ []string) []relevantMemoryMatch {
    // Project briefs are no longer available after project package removal.
    return nil
}
```

`collectRelevantMemory` (L102-108)에서 둘 다 호출되지만 항상 빈 nil 반환. 함수 자체가 코멘트로 "제거됐다" 자백.

`classifySourceTag` (L445-448)에 `"projects/"` + `"_shared/"` prefix 분기도 함께 dead — 이 prefix를 만드는 source가 더 이상 없음.

### 제안하는 변경

CLAUDE.md 가이드라인:
> If you are certain that something is unused, you can delete it completely.

- `collectProjectDocumentMatches` + `collectBriefMatches` 함수 삭제
- `collectRelevantMemory`의 호출 라인 2개 삭제
- `classifySourceTag`의 `projects/` + `_shared/` case 삭제

### 영향 범위

- 변경: `internal/prompt/memory_retrieval.go` (~30줄 감소)
- 테스트: `memory_retrieval_test.go` 가 brief/project 케이스 가지고 있을지 확인 필요
- 기능 영향: 없음 (이미 dead)

---

## RF-024 — Bootstrap 4파일 메타가 3곳에 분산 → 단일 소스 통합

- **Status**: open
- **Location**:
  - `internal/prompt/bootstrap_sections.go:16-21` (4 sections + maxChars + subAgent flag)
  - `internal/sysprompt/sysprompt.go:42-53` (4 specs + Scope)
  - `internal/memory/workspace.go` (`WorkspaceBootstrapFileSpecs` — title/description/default content)
- **Discovered in**: [journal/2026-04-25-11-prompt.md](../journal/2026-04-25-11-prompt.md)
- **Recommendation**: 단일 source (memory.WorkspaceBootstrapFileSpecs)에 maxChars + subAgent + Scope 메타를 합쳐서, prompt와 sysprompt가 그것만 import.
- **Estimated effort**: M
- **Risk**: low

### 현재 상태

같은 4 파일 (USER.md, IDENTITY.md, AGENTS.md, TOOLS.md)에 대한 메타가 3곳에 흩어짐:

| 메타 종류 | 위치 |
|----------|------|
| Title + Description + DefaultContent | `memory.WorkspaceBootstrapFileSpecs` |
| MaxChars + SubAgent flag | `prompt.bootstrapSections` |
| Scope (workspace/agent) + Editable + PromptTargets | `sysprompt.specs()` |

3곳이 각자의 reason으로 4 파일을 list. 새 부트스트랩 파일을 추가하려면 **3곳 모두 수정**해야 silent skew 없이 통합. 이미 `sysprompt.workspaceSpec` 가 memory에서 메타를 가져오는 식으로 부분 통합돼 있지만 prompt 패키지는 자체 `bootstrapSections` 정의 유지.

### 제안하는 변경

`memory.WorkspaceBootstrapFileSpec` 에 다음 필드 추가:

```go
type WorkspaceBootstrapFileSpec struct {
    Path           string
    Title          string
    Description    string
    DefaultContent string
    Scope          Scope     // workspace | agent
    MaxChars       int       // prompt section budget
    SubAgent       bool      // include in subagent prompts
    Editable       bool
    PromptTargets  []string  // ["main_agent"] | ["sub_agent"]
}
```

→ `prompt/bootstrap_sections.go` 와 `sysprompt/sysprompt.go` 의 specs() 모두 단일 source 참조. 추가 파일은 `memory.WorkspaceBootstrapFileSpecs`에 한 entry만 추가.

대안: 통합이 layering을 어색하게 만든다면 (e.g. memory가 prompt rendering 메타를 알아야 함), 별도 `bootstrap` 패키지를 leaf로 두고 셋이 모두 import.

### 영향 범위

- 변경: `internal/memory/workspace.go`, `internal/prompt/bootstrap_sections.go`, `internal/sysprompt/sysprompt.go`
- 테스트: `prompt/builder_test.go`, `sysprompt` 테스트 회귀
- 호환성: 내부 변경 — 사용자 영향 없음

---

## RF-025 — `collectRelevantMemory` fallback chain의 매 turn O(N) 비용 — bound 강화 또는 캐시 의존 명시

- **Status**: open
- **Location**: `internal/prompt/memory_retrieval.go`
  - `collectExperienceMatches` (L184-214) — 24개 experiences 전체 read + per-row score
  - `collectFileLineMatches` (L248-276) — MEMORY.md + daily logs 전체 line scan
  - `collectDailyLogMatches` (L225-246) — 모든 `memory/*.md` glob + per-file line scan, 누적 10건 cap
  - `collectSessionMatches` (L278-327) — 모든 sessions list + 각 transcript read + parse + scan
- **Discovered in**: [journal/2026-04-25-11-prompt.md](../journal/2026-04-25-11-prompt.md)
- **Recommendation**: `tarsserver/memory_cache.go`(prompt.BuildResult 캐시, TTL 5min)가 cache hit 시는 무관. 다만 cache miss는 매 turn이 위 4개 fallback 중 하나 이상에 N×lines 비용. semantic이 활성이면 fallback 안 가지만, semantic 비활성/실패 시 fallback이 hot path. (1) 더 엄격한 row/line cap, (2) glob 결과 캐싱, (3) "semantic 필수" 정책으로 fallback 자체 제거 중 결정.
- **Estimated effort**: M
- **Risk**: medium (정책 결정 포함)

### 현재 상태

`collectRelevantMemory` 호출 흐름:

```
semantic.Search → 0건이면 fallback chain:
  1. collectProjectDocumentMatches  (이미 dead — RF-023)
  2. collectBriefMatches            (이미 dead — RF-023)
  3. collectExperienceMatches       (24 rows × per-row scoring)
  4. collectMemoryFileMatches       (MEMORY.md 전체 line × scoring)
  5. collectDailyLogMatches         (모든 memory/*.md × per-file line × scoring, 10 match cap)
  6. collectSessionMatches          (모든 sessions × transcript read × parse × scan)
```

prompt.BuildResultFor 는 매 chat turn의 prefetch + actual call에서 호출됨 (`handler_chat_prefetch.go`). 즉 매 turn 2회 호출 가능. cache hit이면 무관.

### 비용 평가

- semantic 활성 + 6개 hit : fallback 0번 호출. 가장 흔한 경우.
- semantic 비활성 (Gemini 미설정): 매 turn fallback chain 풀 실행.
- 사용자 워크스페이스가 오래 운영되면:
  - daily logs `memory/*.md` 파일 수가 ~1년분
  - sessions 수가 ~수백
  - → fallback이 매 turn O(thousands of lines + hundreds of transcript reads).

### 옵션

- **(a) 엄격한 cap 강화**: 모든 fallback에 `maxItemsToScan` 보장. 현재 cap이 일부만 적용 (`collectDailyLogMatches`만 10건 cap, `collectSessionMatches`는 cap 없음).
- **(b) glob 결과 캐싱**: `filepath.Glob` 결과 + 각 파일의 ModTime 기반 캐시. 변경 안 된 파일은 재파싱 안 함.
- **(c) Semantic 필수 정책**: Gemini 미설정 환경에서는 prior context 섹션 자체 비활성. fallback 코드 전체 제거. Phase 1에서는 (a) 부분만, semantic 사용성 늘면 (c)로 전환.
- **(d) Cache TTL 연장**: 현재 5min → caller 정책. 캐시 의존을 더 명시적으로.

### 영향 범위

- 변경: `internal/prompt/memory_retrieval.go` (또는 정책 결정)
- 테스트: 워크스페이스가 큰 상황의 회귀 시뮬레이션 추가
- 사용자 영향: 작은 워크스페이스에서는 무관. 큰 워크스페이스에서 turn 지연 감소 가능.

### CLAUDE.md 약속과의 관계

CLAUDE.md:
> **Cache-first strategy**: In-process `memoryCache` (TTL 5min) checked before every semantic search

→ `tarsserver/memory_cache.go` 가 이 약속의 위치. **prompt 패키지 본인은 cache를 모름** — 약속 이행은 caller 책임. RF-025 결정은 "캐시 의존이 충분한가" 평가 후.

---

## RF-029 — `read_file` 의 6개 parameter 중복 — canonical 1쌍으로 단순화

- **Status**: open
- **Location**: `internal/tool/read_file.go:57-69` (Schema), L113-203 (resolveReadLineWindow)
- **Discovered in**: [journal/2026-04-25-13-tool-file.md](../journal/2026-04-25-13-tool-file.md)
- **Recommendation**: `offset`+`limit` (canonical) 만 유지. Gemini-style alias `start_line`/`end_line` 와 legacy `max_bytes` 는 deprecate/제거.
- **Estimated effort**: S
- **Risk**: low (외부 caller 마이그레이션 필요 시 medium)

### 현재 상태

read_file 이 받는 parameter 6개:

| 파라미터 | 의미 | 출처 |
|---------|------|------|
| `offset` | 1-based 시작 라인 | canonical |
| `limit` | 최대 라인 수 | canonical |
| `start_line` | 시작 라인 alias | Gemini-style |
| `end_line` | 종료 라인 alias (inclusive) | Gemini-style |
| `max_bytes` | byte cap (라인 선택 후 적용) | legacy |
| `path` | 파일 경로 | required |

`resolveReadLineWindow` 가 셋 다 처리:

```go
if offset != nil { start = *offset }
if startLine != nil {
    if offset != nil && *offset != *startLine { return error }  // 둘 다 주면 일치 강제
    start = *startLine
}
...
if endLine != nil {
    requested := (*endLine - start) + 1
    if requested < lines { lines = requested }
    ...
}
```

→ 같은 의미를 다른 이름으로 표현. **LLM에게 혼란**: 어느 alias가 우선? 둘 다 줘도 되나? `max_bytes`는 line 선택 *후* 적용이라 `limit`과 의미 충돌.

코드 자체도 이중 처리 — alias 매핑이 schema 외부에 분산.

### 제안하는 변경

옵션:
- **(a) canonical만 유지**: `offset` + `limit` + `path`. Gemini-style/legacy 제거. 외부 caller 마이그레이션 필요 — Gemini API 호환이라면 별도 어댑터 레이어로.
- **(b) Gemini-style만 유지**: `start_line` + `end_line` + `path`. Gemini SDK 호환에 우선순위.
- **(c) 분리 함수 + 같은 schema 유지**: 코드 단순화는 안 됨. 비추.

→ (a) 권장. CLAUDE.md 의 *"Avoid backwards-compatibility hacks"* 와 정렬.

`max_bytes`는 line-oriented pagination에서 **redundant** — `limit` × `maxReadFileLineChars` (2000) = 4MB cap이 이미 적용. legacy path 제거.

### 영향 범위

- 변경: `internal/tool/read_file.go` (schema + resolveReadLineWindow + buildReadFileMessage)
- 테스트: `read_file_test.go` 의 alias/legacy 케이스 정리
- 외부 caller: Gemini integration이 있다면 마이그레이션 가이드 필요

---

## RF-031 — `exec` 의 `command` ↔ `cmd` schema-input 불일치

- **Status**: open
- **Location**: `internal/tool/exec.go:59-68` (schema), L207-215 (parseExecInput)
- **Discovered in**: [journal/2026-04-25-14-tool-exec-web.md](../journal/2026-04-25-14-tool-exec-web.md)
- **Recommendation**: `cmd` alias 제거 또는 schema에 명시. silent acceptance 제거.
- **Estimated effort**: trivial
- **Risk**: trivial (외부 caller가 `cmd` 보내고 있는지 grep 후 결정)

### 현재 상태

Schema는 `command` 만 명시:

```json
"properties":{
  "command":{"type":"string", ...},
  "timeout_ms":{...},
  "background":{...}
}
```

그런데 `parseExecInput` (L207-215) 은 `cmd` 도 받음:

```go
if v, ok := payload["command"]; ok { ... }
else if v, ok := payload["cmd"]; ok { ... }
```

→ **silent acceptance**. LLM이 strict tool calling 모드에서 `cmd` 사용 시 schema validation 실패하는데, parseExecInput은 통과 — 일관성 없음. 또 schema에 안 보이는 alias는 LLM에게 잠재 버그 (LLM은 schema 보고 호출).

### 제안하는 변경

옵션:
- **(a)** `cmd` alias 제거 — schema와 parser 일치.
- **(b)** schema 에 `cmd` 명시 (anyOf required: command | cmd) — 일관성 회복하지만 schema 복잡해짐.

→ (a) 권장. `cmd` 사용 caller grep 후 0건이면 즉시 제거.

### 영향 범위

- 변경: `internal/tool/exec.go` (parseExecInput 단순화)
- 테스트: `cmd` alias 케이스가 있다면 정리

---

## RF-032 — `exec.blockedExecCommands` 의 false sense of security

- **Status**: open
- **Location**: `internal/tool/exec.go:21-33`
- **Discovered in**: [journal/2026-04-25-14-tool-exec-web.md](../journal/2026-04-25-14-tool-exec-web.md)
- **Recommendation**: blocklist 제거 또는 더 강력한 가드로 대체. 현재 가드 (multi-line 거부 + workspaceDir 한정 + timeout) 가 실질적으로 더 효과적.
- **Estimated effort**: S
- **Risk**: medium (보안 정책 결정 포함)

### 현재 상태

```go
var blockedExecCommands = map[string]struct{}{
    "sudo": {}, "rm": {}, "shutdown": {}, "reboot": {}, ...
    "kill": {}, "killall": {},
}
```

**막히는 것**: `fields[0]` (첫 토큰) 이 위 11개 중 하나일 때.

**막히지 않는 것**:
- `bash -c "rm -rf /"` — fields[0]이 `bash`
- `mv ~/important /tmp/trash` — `mv`는 blocklist에 없음
- `chmod 000 ~/.ssh` — `chmod` 없음
- `chown root:root ~/.ssh` — `chown` 없음
- `find / -delete` — `find` 없음
- `> ~/.bashrc` — redirect로 파일 truncate (사실 fields parsing 안 됨, 아래 참고)

→ 사실 `strings.Fields(commandLine)` 은 shell metacharacter 인식 안 함. `>`, `|`, `&&` 등은 모두 단순 fields로 parse — 즉 실제로 shell 해석 X. 명령은 `exec.CommandContext(command, fields[1:]...)` 으로 직접 실행 (shell 거치지 않음). 그래서 redirect/pipe/subshell 동작 안 함.

→ **실제 가드는**:
1. shell 거치지 않음 → injection 차단
2. workspaceDir 한정 (`cmd.Dir`)
3. timeout 30초
4. multi-line 거부

이 4개가 **blocklist보다 훨씬 강력**. blocklist는 false sense of security.

### 제안하는 변경

옵션:
- **(a) 완전 제거** — 위 4 가드만으로 충분 명시.
- **(b) 보존하되 의도 코멘트 강화** — "이건 LLM의 기본 실수 방지 (e.g. 무심코 `rm` 호출)" 정도로 의도 명확히.
- **(c) 강화** — chmod/chown/mv 같은 path-modifying 명령 추가. 단 blocklist 자체의 한계 (위 우회 패턴) 는 유지.

→ (a) 또는 (b) 권장. (c) 는 false sense 강화 우려.

### 영향 범위

- 변경: `internal/tool/exec.go` (blockedExecCommands)
- 테스트: `exec_test.go` 의 blocked 케이스 정리
- 보안 평가: 사용자 결정 포함 — RF-008 (sh 훅 제거) 와 같은 카테고리

---

## RF-033 — `web_search` 의 unbounded cache map

- **Status**: open
- **Location**: `internal/tool/web_search.go:82-84` (cache map), L262-265 (Put)
- **Discovered in**: [journal/2026-04-25-14-tool-exec-web.md](../journal/2026-04-25-14-tool-exec-web.md)
- **Recommendation**: TTL 만료된 entry 주기적 evict 또는 size cap.
- **Estimated effort**: S
- **Risk**: low

### 현재 상태

```go
cache := map[string]webSearchCacheEntry{}
var cacheMu sync.Mutex

// Get: TTL 체크 후 expired면 cached=false 리턴, but map에서 제거 X
if ok && time.Now().Before(cached.Expires) {
    payload := cached.Value
    payload.Cached = true
    ...
}

// Put: 단순 추가
cache[cacheKey] = webSearchCacheEntry{Expires: ..., Value: payload}
```

→ **TTL expired entry는 map에 영원히 누적**. 같은 query 재호출하면 새 entry로 덮어쓰지만, 한 번 호출 후 다시 안 부르는 query는 영원히 남음. 장기 운영 시 메모리 누수.

### 제안하는 변경

옵션:
- **(a) Lazy evict on Put**: Put 시 모든 entry 순회하면서 expired 제거. 비용 O(N) per Put.
- **(b) Size cap + LRU**: max 100 entries, LRU. `container/list` + map 조합.
- **(c) Time-based GC goroutine**: 1분마다 evict. lifecycle 관리 필요.

→ (a) 가 가장 단순. cache size 가 보통 작아서 O(N) 무관. (b) 는 over-engineering 가능.

### 영향 범위

- 변경: `internal/tool/web_search.go`
- 테스트: cache evict 검증 추가
- 비교: `tarsserver/memory_cache.go` 가 비슷한 in-process cache — 그쪽 패턴과 통일 검토

---

## RF-034 — `web_fetch.htmlToText` 의 단순 regex 파싱

- **Status**: open
- **Location**: `internal/tool/web_fetch.go:265-272`
- **Discovered in**: [journal/2026-04-25-14-tool-exec-web.md](../journal/2026-04-25-14-tool-exec-web.md)
- **Recommendation**: `golang.org/x/net/html` 의 `Tokenizer` 사용. script/style 본문 제거 + entity 디코딩 강화.
- **Estimated effort**: M
- **Risk**: low (출력 품질 ↑)

### 현재 상태

```go
var htmlTagRE = regexp.MustCompile(`(?s)<[^>]*>`)

func htmlToText(html string) string {
    cleaned := htmlTagRE.ReplaceAllString(html, " ")
    cleaned = strings.ReplaceAll(cleaned, "&nbsp;", " ")
    cleaned = strings.ReplaceAll(cleaned, "&amp;", "&")
    return cleaned
}
```

문제:
- **`<script>` / `<style>` 본문이 보존됨** — 태그만 제거되고 inner JavaScript/CSS 가 텍스트로 LLM에 들어감
- **HTML entity 디코딩 단편적** — `&nbsp;`/`&amp;` 만. `&lt;`/`&gt;`/`&quot;`/`&#39;`/`&copy;` 등 무시
- **comment `<!-- -->` 제거 안 됨** — regex가 일반 태그처럼 처리하지만 inner content는 보존
- 사용자 워크플로: web_fetch 결과를 LLM에 넣음 → JS/CSS noise → 토큰 비용 + 답변 품질 저하

### 제안하는 변경

```go
import "golang.org/x/net/html"

func htmlToText(input string) string {
    tokenizer := html.NewTokenizer(strings.NewReader(input))
    var b strings.Builder
    skip := 0
    for {
        tt := tokenizer.Next()
        switch tt {
        case html.ErrorToken:
            return strings.Join(strings.Fields(b.String()), " ")
        case html.StartTagToken:
            name, _ := tokenizer.TagName()
            if string(name) == "script" || string(name) == "style" {
                skip++
            }
        case html.EndTagToken:
            name, _ := tokenizer.TagName()
            if string(name) == "script" || string(name) == "style" {
                if skip > 0 { skip-- }
            }
        case html.TextToken:
            if skip == 0 {
                b.Write(tokenizer.Text())
            }
        }
    }
}
```

→ script/style 제거 + entity 자동 디코딩 (Tokenizer가 처리).

### 영향 범위

- 변경: `internal/tool/web_fetch.go` + go.mod (`golang.org/x/net` 이미 있는지 확인)
- 테스트: web_fetch_test.go 의 HTML 파싱 케이스 강화
- 출력 품질: LLM에 더 깨끗한 텍스트 전달 → 토큰 비용 ↓, 답변 품질 ↑

---

## RF-035 — `ProcessManager` 의 timeout cap 30초 — long-running 의도와 충돌

- **Status**: open
- **Location**: `internal/tool/process_manager.go:63-68` (Start), `exec.go:14-16` (timeout 상수 재사용)
- **Discovered in**: [journal/2026-04-25-14-tool-exec-web.md](../journal/2026-04-25-14-tool-exec-web.md)
- **Recommendation**: ProcessManager 전용 timeout 상수 (예: 10분 또는 unbounded). 현재 `minExecTimeoutMS`/`maxExecTimeoutMS` (5초/30초) 재사용은 background 의도와 충돌.
- **Estimated effort**: S
- **Risk**: low

### 현재 상태

`exec.go` 가 정의:
```go
const (
    minExecTimeoutMS = 100
    maxExecTimeoutMS = 30000  // 30초
)
```

`process_manager.go:63-68` 의 Start:
```go
if timeoutMS < minExecTimeoutMS { timeoutMS = minExecTimeoutMS }
if timeoutMS > maxExecTimeoutMS { timeoutMS = maxExecTimeoutMS }  // ← 30초 cap
runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
```

→ **background process 가 30초 후 자동 kill됨**. ProcessManager 의 의도 (*"Manage background exec sessions"*) 와 충돌. dev server, 빌드, batch 처리 같은 일반적 long-running 워크로드 모두 30초 후 끊김.

### 제안하는 변경

```go
// process_manager.go 또는 별도 const
const (
    minBackgroundProcessTimeoutMS = 1000          // 1초
    maxBackgroundProcessTimeoutMS = 10 * 60 * 1000 // 10분
)
```

또는 unbounded 로 두고 명시적 Kill 만 의존 (cancel context 만 사용).

### 영향 범위

- 변경: `internal/tool/process_manager.go`, `exec.go` (background branch가 process_manager 호출 시 다른 timeout 사용)
- 테스트: long-running scenario 추가
- 호환성: 외부 caller 영향 없음

---

## RF-030 — File 툴 등록 시그니처 3중 (NewX / NewXFile / NewXFileWithPolicy) 단순화

- **Status**: open
- **Location**:
  - `internal/tool/read_file.go:37-51` (NewReadTool / NewReadFileTool / NewReadFileToolWithPolicy)
  - `internal/tool/write_file.go:20-34` (NewWriteTool / NewWriteFileTool / NewWriteFileToolWithPolicy)
  - `internal/tool/edit_file.go:18-32` (NewEditTool / NewEditFileTool / NewEditFileToolWithPolicy)
  - `internal/tool/list_dir.go:32-34` (NewListDirTool / NewListDirToolWithPolicy)
  - `internal/tool/glob.go:24-28` (NewGlobTool / NewGlobToolWithPolicy)
- **Discovered in**: [journal/2026-04-25-13-tool-file.md](../journal/2026-04-25-13-tool-file.md)
- **Recommendation**: `WithPolicy` 변종만 유지하고, 단순 `workspaceDir` 변종은 제거 또는 `WithPolicy(SingleDirPolicy(workspaceDir))` 호출로 통합.
- **Estimated effort**: S
- **Risk**: low

### 현재 상태

read_file 만 봐도 5 시그니처:
1. `NewReadTool(workspaceDir)` — name="read", workspaceDir 정책
2. `NewReadFileTool(workspaceDir)` — name="read_file", workspaceDir 정책
3. `NewReadFileToolWithPolicy(policy)` — name="read_file", PathPolicy
4. `newReadToolWithName(name, workspaceDir)` — internal helper
5. `newReadToolWithPolicy(name, policy)` — internal helper

실제 등록 호출처(`helpers_agent.go:64`)는 `WithPolicy` 만 사용 — 나머지 4개는 호출 0건일 가능성. grep 필요.

`NewReadTool` 의 name="read" 도 RF-019 카테고리 — alias map이 `"read" → "read_file"` 이라 사실상 dead path.

### 제안하는 변경

```go
// 단일 시그니처
func NewReadFileTool(policy PathPolicy) Tool { ... }

// 호출처에서 단순화
registry.Register(tool.NewReadFileTool(tool.SingleDirPolicy(workspaceDir)))
```

또는 helper 보존 시:
```go
func NewReadFileToolForDir(workspaceDir string) Tool {
    return NewReadFileTool(SingleDirPolicy(workspaceDir))
}
```

→ 5개 file 툴 모두 같은 단순화 적용. 테스트 코드도 정리.

### 영향 범위

- 변경: file 툴 5개 + 호출처 grep + 테스트
- 테스트: file 툴 테스트가 어떤 변종을 사용하는지 확인 후 마이그레이션
- 외부 caller: 패키지 외부에서 `NewReadTool`/`NewReadFileTool` 호출하는 코드가 있는지 grep

---

## RF-026 — `Registry.Register` silent overwrite — 중복 등록 시 panic 또는 warn

- **Status**: open
- **Location**: `internal/tool/tool.go:133-145` (Register)
- **Discovered in**: [journal/2026-04-25-12-tool-core.md](../journal/2026-04-25-12-tool-core.md)
- **Recommendation**: 같은 이름의 툴이 이미 등록돼 있으면 **panic** (또는 최소 logger.Warn). 현재는 silent overwrite — 첫 등록이 덮어씌워지고 흔적 없이 사라짐.
- **Estimated effort**: S
- **Risk**: low

### 현재 상태

```go
func (r *Registry) Register(t Tool) {
    for _, p := range registryForbiddenPrefixes[r.scope] {
        if strings.HasPrefix(t.Name, p) {
            panic(fmt.Sprintf("tool %q cannot be registered in %s scope (forbidden prefix %q)",
                t.Name, r.scope, p))
        }
    }
    r.mu.Lock()
    defer r.mu.Unlock()
    r.tools[t.Name] = t  // ← silent overwrite
}
```

scope 가드는 panic으로 강제하면서 (좋은 패턴 — CLAUDE.md *"compile-time-style guarantee enforced at wiring time"*), 같은 이름의 중복 등록은 silent. 실수 시나리오:

- 빌트인 + 외부 플러그인이 같은 이름 등록 → 외부가 빌트인 덮어씀
- alias가 canonical 이름과 충돌 (e.g. plugin이 `"memory"` 라는 이름으로 등록) → memory aggregator 망가짐
- 같은 helpers_agent.go 안에서 두 번 register (실수) → silent

### 제안하는 변경

scope 가드와 동일한 정책 — wiring 시점 에러는 panic:

```go
if _, exists := r.tools[t.Name]; exists {
    panic(fmt.Sprintf("tool %q already registered in %s scope", t.Name, r.scope))
}
r.tools[t.Name] = t
```

또는 최소 옵션: `RegisterOrReplace` / `RegisterStrict` 분리. 후자가 default.

### 영향 범위

- 변경: `internal/tool/tool.go` (Register)
- 테스트: 중복 등록 panic 검증 추가, 기존 register 동작 회귀 확인 (테스트가 같은 이름 재등록 하지 않는지)
- 호환성: 기존 코드가 의도적으로 덮어쓰는 케이스 있다면 깨짐 — grep 필요

---

## RF-027 — `IsExecToolName` 의 이중 정규화 단순화

- **Status**: open
- **Location**: `internal/tool/tool_name.go:76-82`
- **Discovered in**: [journal/2026-04-25-12-tool-core.md](../journal/2026-04-25-12-tool-core.md)
- **Recommendation**: `canonical == "exec"` 직접 비교로 단순화.
- **Estimated effort**: trivial
- **Risk**: trivial

### 현재 상태

```go
func IsExecToolName(name string) bool {
    canonical := CanonicalToolName(name)
    if canonical == "" {
        return false
    }
    return canonical == CanonicalToolName("shell_exec")
}
```

`CanonicalToolName("shell_exec")` 는 alias 매핑으로 `"exec"`. 즉 `canonical == "exec"`와 동등. 이중 호출 비용 + 의도 불명. 의도가 *"exec의 다른 이름이 추가되면 자동 반영"* 이라면 — 실제로는 alias map 자체가 single source라 그냥 `"exec"` 비교로 충분.

### 제안하는 변경

```go
func IsExecToolName(name string) bool {
    return CanonicalToolName(name) == "exec"
}
```

### 영향 범위

- 변경: `internal/tool/tool_name.go`
- 테스트: `tool_name_test.go` 의 IsExecToolName 케이스가 그대로 통과해야 함

---

## RF-028 — `dispatchAction` ↔ `normalizeAutomationActionInput` 통합

- **Status**: open
- **Location**: `internal/tool/aggregator.go:12-40` (generic), `internal/tool/automation.go:58-92` (cron-specific)
- **Discovered in**: [journal/2026-04-25-12-tool-core.md](../journal/2026-04-25-12-tool-core.md)
- **Recommendation**: `dispatchAction`을 base로 두고, automation 의 `id → job_id` aliasing은 별도 후처리로 분리.
- **Estimated effort**: S
- **Risk**: low

### 현재 상태

`aggregator.go:9-11` 코멘트:
> `dispatchAction` extracts the "action" field from params and returns the remaining payload and action string. This is the generic version of normalizeAutomationActionInput without cron-specific id aliasing.

→ 두 함수 본문이 거의 동일. 차이는 automation 만 `id` → `job_id` 매핑 추가 (L81-86):

```go
if _, ok := payload["job_id"]; !ok {
    if id, ok := payload["id"]; ok {
        payload["job_id"] = id
    }
}
delete(payload, "id")
```

DRY 위반. 새로운 aggregator-specific aliasing 추가 시 본문 복사 풍토 우려.

### 제안하는 변경

```go
// generic
payload, action, err := dispatchAction(params)

// automation 안에서 후처리
payload = aliasJobID(payload)
```

또는 dispatchAction 시그니처에 `aliasFn func(map[string]json.RawMessage)` 옵션 추가.

### 영향 범위

- 변경: `internal/tool/aggregator.go`, `internal/tool/automation.go`
- 테스트: automation aggregator 회귀 (id ↔ job_id 매핑 보존 검증)
- 호환성: 외부 호출자 영향 없음

---

## RF-036 — `memory_save` 의 MEMORY.md 자동 append — 사용자 영역 경계 침범

- **Status**: open
- **Location**: `internal/tool/memory_save.go:62` (AppendMemoryNote)
- **Discovered in**: [journal/2026-04-25-15-tool-memory.md](../journal/2026-04-25-15-tool-memory.md)
- **Recommendation**: 두 번 저장 (Experience JSONL + MEMORY.md) 패턴 재검토. MEMORY.md는 사용자 직접 편집 영역 — LLM 자동 append는 경계 침범 가능.
- **Estimated effort**: S (정책 결정 + 분리)
- **Risk**: medium (사용자 워크플로 영향)

### 현재 상태

```go
// memory_save.go:58-64
err := backend.AppendExperience(context.Background(), exp)
if err != nil {
    return memoryGetErrorResult(err.Error()), nil
}
if err := backend.AppendMemoryNote(context.Background(), exp.Timestamp, summary); err != nil {
    return memoryGetErrorResult(err.Error()), nil
}
```

`memory_save` 가 호출될 때마다 **두 곳에 저장**:
1. `Experience` JSONL (구조화된 LLM-extracted 사실)
2. `MEMORY.md` (사용자 직접 편집하는 영구 메모)

### 문제

CLAUDE.md / memory 패키지 정의 (s09/s10):
> **MEMORY.md** (DurableKindMemory): 사용자 facing 영구 메모, 사람이 직접 편집

→ MEMORY.md 는 **사용자 영역**. LLM이 `memory_save` 호출할 때마다 자동 append 하면:
- 사용자가 안 적은 내용이 영구 메모에 쌓임
- 사용자 큐레이션 의도와 LLM extraction 의도 섞임
- 사용자가 LLM-append 한 내용을 직접 지워야 함 (의도 미스매치)

특히 chat-memory-hook 시스템 (CLAUDE.md *"per-turn `chat_memory_hook`이 daily log append + 명시적 `remember …` 핫패스"*) 가 이미 daily log 에 자동 append. `memory_save` 의 추가 MEMORY.md append 는 **이중 자동 저장** 가능.

### 제안하는 변경

옵션:
- **(a) Experience만 저장**: `memory_save` 가 `AppendMemoryNote` 호출 안 함. MEMORY.md 는 사용자 명시적 동의/요청 시에만 (예: chat_memory_hook 의 `remember …` 패턴).
- **(b) explicit flag**: `memory_save` 에 `update_memory_md: bool` 추가, default false.
- **(c) 사용자 의도 기준 분기**: input.Auto=true (LLM 자동 추출) 면 Experience 만, false 면 둘 다.

→ (a) 가 가장 보수적. MEMORY.md 의 사용자 영역 보장.

### 영향 범위

- 변경: `internal/tool/memory_save.go`
- 테스트: memory_save 의 MEMORY.md 미저장 검증
- 사용자 영향: 기존 워크스페이스의 MEMORY.md 가 LLM-append 로 어지럽혀졌을 가능성 — 별도 cleanup 권장

### 관련 finding

- [ID-001](../ideas.md#id-001) — KB 의 read 통합 0% 인 것과 정반대로, MEMORY.md는 LLM이 자동 write. **read는 닫혀 있고 write는 너무 열려 있음** — memory 표면의 비대칭.

---

## RF-037 — `memory_search` 와 `prompt/memory_retrieval.go` 의 fallback chain 중복

- **Status**: open
- **Location**:
  - `internal/tool/memory_search.go:126-217` (runMemorySearch)
  - `internal/prompt/memory_retrieval.go:93-143` (collectRelevantMemory)
- **Discovered in**: [journal/2026-04-25-15-tool-memory.md](../journal/2026-04-25-15-tool-memory.md)
- **Recommendation**: 두 fallback chain 통합. 같은 source/scoring 패턴 + 같은 backend.Search 우선순위. 공통 헬퍼 패키지로 추출.
- **Estimated effort**: M
- **Risk**: low

### 현재 상태

같은 패턴이 두 곳에 거의 같은 코드:

| 측면 | `tool/memory_search.go` | `prompt/memory_retrieval.go` |
|------|------------------------|----------------------------|
| 우선순위 1 | `backend.Search` (semantic) | `collectSemanticMatches` (semantic) |
| Fallback 시작 | knowledge (opt-in) | (knowledge 없음 — RF-007 카테고리) |
| 다음 | experiences | experiences |
| 다음 | memory files (MEMORY.md + daily) | memory files |
| 다음 | sessions (opt-in) | sessions |
| Score 패턴 | knowledge +120, memory +100, experiences/sessions +80 | experiences +140+recency, memory files +100+recency |
| Stopwords | `memorySearchTerms` (rune-based) | `normalizeRelevantTerms` (EN-only) |

미세 차이:
- prompt 쪽은 `ForceRelevantMemory` 로 score 100 미만도 통과 가능
- tool 쪽은 source 별 boost 다름 (knowledge +120, prompt에는 없음)
- prompt 쪽은 stopwords EN-only [RF-018], tool 쪽은 rune-based (KR 안전)

### 제안하는 변경

`internal/memorysearch/` 같은 공통 패키지로 추출:
```go
type SearchOptions struct {
    Query string
    Limit int
    IncludeKnowledge bool
    IncludeSessions bool
    ForceLowScore    bool
    SourceBoosts     map[string]int  // tunable per caller
}

func Search(ctx, backend, workspaceDir, opts) []Match
```

→ `tool/memory_search.go` 는 thin wrapper. `prompt/memory_retrieval.go` 의 fallback chain 부분도 같은 헬퍼 호출.

### 영향 범위

- 변경: 새 패키지 + 두 caller 수정
- 테스트: 두 caller의 기존 테스트가 같은 결과 검증
- 효용: stopwords 처리 통일 (RF-018 자동 해결), score 정책 한 곳에서 관리, KR 처리 일관성

### 관련 finding

- [RF-015](#rf-015) — system surface 헬퍼 추출 (timewindow/textutil)와 같은 카테고리. 공통 패키지로 추출.
- [RF-018](#rf-018) — KR stopwords 가 두 곳 모두 미흡. RF-037 적용 시 한 번에 정리.

---

## RF-038 — `memory_search` 의 `include_sessions` description 자기모순

- **Status**: open
- **Location**: `internal/tool/memory_search.go:56`
- **Discovered in**: [journal/2026-04-25-15-tool-memory.md](../journal/2026-04-25-15-tool-memory.md)
- **Recommendation**: default 를 true로 변경하거나 description 의 *"Always set to true when called"* 문구 제거.
- **Estimated effort**: trivial
- **Risk**: low

### 현재 상태

```json
"include_sessions":{
  "type":"boolean",
  "default":false,
  "description":"Search past session transcripts for conversational continuity. Always set to true when called."
}
```

→ default=false 인데 description은 *"Always set to true when called"*. LLM에게 모순된 신호.

CLAUDE.md *"Continuity detection: shouldForceMemoryToolCall detects 30+ patterns"* 의 의도는 continuity 패턴 감지 시 LLM이 메모리 검색하라는 것. 그렇다면 `include_sessions` default true 가 의도와 일치.

### 제안하는 변경

옵션:
- **(a) default true** — description의 의도와 일치. 비용은 매 memory_search 호출마다 sessions 검색 (최대 10 sessions).
- **(b) description 정리** — *"Always set to true when called"* 제거. default false 유지. LLM이 명시적으로 true 줘야 함.

→ (a) 권장 — description 의도를 코드가 따라가는 방향. CLAUDE.md continuity detection 시스템과 정렬.

### 영향 범위

- 변경: `internal/tool/memory_search.go` (default true)
- 비용: memory_search 호출 시마다 sessions 검색 (최대 10) — 캐시 없음. 워크스페이스 큰 사용자에게 turn 지연 가능 [RF-025 관련]
- 테스트: include_sessions 미지정 시 sessions 포함 검증

### 관련

- [RF-025](#rf-025) — fallback chain 비용 평가. (a) 채택 시 RF-025 의 cap 강화 우선순위 ↑

---

## RF-039 — `cron` aggregator 의 다중 alias silent acceptance

- **Status**: open
- **Location**:
  - `internal/tool/automation_cron.go:60-93` (CronCreate input)
  - `internal/tool/automation_cron.go:122-200` (CronUpdate input)
  - `internal/tool/automation.go:81-86` (id → job_id alias in normalizer)
- **Discovered in**: [journal/2026-04-25-16-tool-final.md](../journal/2026-04-25-16-tool-final.md)
- **Recommendation**: alias 4쌍 (`name`/`title`, `prompt`/`message`, `id`/`job_id`, `task_type` legacy) 정리. canonical 만 schema 명시 + 외부 alias 사용처 grep 후 deprecation.
- **Estimated effort**: M
- **Risk**: medium (외부 caller 마이그레이션 필요 시)

### 현재 상태

`CronCreate` input struct:
```go
Name           string  // alias of Title
Title          string  // alias of Name
Prompt         string  // alias of Message
Message        string  // alias of Prompt
TaskType       string  // legacy, 내부적으로 task_type 추론에만 사용
```

resolution 4 함수에 분산:
- `resolveCronJobName(name, title, message, prompt)` — 첫 non-empty 사용
- `resolveCronJobPrompt(prompt, message, title, taskType)` — fallback chain
- `inferCronTaskType(taskType, name, prompt, message)` — keyword 매칭
- `inferReminderMessage(taskType, message, title, prompt)` — fallback chain

→ **alias 의도가 4 함수에 흩어짐**. 어떤 alias가 어디로 흘러가나 추적 어려움. RF-029 (read_file 의 5 parameter) + RF-031 (exec command/cmd) 와 같은 카테고리지만 더 큰 표면.

또 schema 에 `task_type`, `title`, `message` 모두 명시 안 됨 — silent acceptance. 외부 caller가 이걸 사용해도 LLM은 schema 보고 호출 안 함.

### 제안하는 변경

옵션:
- **(a) canonical 1개만 유지** — `name` + `prompt` + `job_id`. Message/Title/TaskType 제거. 4 resolution 함수 통합 가능.
- **(b) schema 에 alias 명시** — anyOf required. schema 복잡해짐.
- **(c) 보존하되 의도 코멘트 강화** — alias 매핑을 코드 한 곳에 모음 (지금처럼 4 함수 분산 X).

→ (a) + (c) 조합 권장. canonical 1개 + 명시적 alias map (외부 호환 필요시).

### 영향 범위

- 변경: `internal/tool/automation_cron.go` (4 resolution 함수 통합)
- 테스트: alias 케이스 정리
- 외부 caller: 스킬/콘솔/CLI 가 어떤 alias 사용하는지 grep 후 결정

### 관련 finding

- [RF-029](#rf-029) — read_file 의 6 parameter 중복
- [RF-031](#rf-031) — exec 의 cmd alias
- [ID-003](../ideas.md#id-003) — aggregator schema 패턴 통일 (process 패턴 권장)

---

## RF-040 — `message` / `nodes` / `gateway` 빌트인 툴 통합 또는 외부화

- **Status**: open
- **Location**:
  - `internal/tool/tool_gateway.go` (3 빌트인 툴: 162줄)
  - `internal/tarsserver/helpers_build_tools.go:55-64` (cfg-flag 등록)
- **Discovered in**: [journal/2026-04-25-16-tool-final.md](../journal/2026-04-25-16-tool-final.md)
- **Recommendation**: (a) 단일 `gateway` aggregator 로 통합 (action: status/reload/restart/message_*/nodes_*) 또는 (b) 외부화 (skill+CLI).
- **Estimated effort**: S (a) / M (b)
- **Risk**: low

### 현재 상태

3 빌트인 툴 모두:
- 모두 cfg-flag optional (`ToolsMessageEnabled`, `ToolsNodesEnabled`, `ToolsGatewayEnabled`) — default disabled
- 모두 `gateway.Runtime` 위임
- 모두 action enum 패턴 (각자 3 actions)
- description 합 ~50자 + parameter schema 합 ~600B → ~150 토큰/turn (켜졌을 때)

| 툴 | actions | LLM 호출 빈도 추정 |
|----|---------|------------------|
| `message` | send / read / thread_reply | 채팅 메시징 — 워크플로 따라 |
| `nodes` | status / describe / invoke | gateway nodes admin — 낮음 |
| `gateway` | status / reload / restart | runtime admin — 매우 낮음 |

→ 셋 다 같은 gateway runtime + 같은 cfg-flag 군집 + 모두 admin 성격. **통합 자연스러움**. 또는 모두 외부화.

### 제안하는 변경

옵션:
- **(a) 단일 `gateway` aggregator 통합**:
  ```yaml
  name: gateway
  description: "Gateway operations (admin + messaging + nodes)."
  parameters:
    properties:
      action: { enum: [status, reload, restart, message_send, message_read, message_thread_reply, nodes_status, nodes_describe, nodes_invoke] }
      ...
  ```
  - 9 sub-actions, single tool description
  - cfg-flag 1개 (`ToolsGatewayEnabled`) 로 통합
  - 시스템 프롬프트 비용 -50%

- **(b) 외부화 (skill+CLI)** — 콘솔에 admin UI 가 이미 있을 가능성. CLAUDE.md *"Default pattern: skill+CLI"* 정렬. 빈도 낮으면 더 정당.

- **(c) 현 상태 유지** — 별도 cfg flag로 세밀한 제어. 단 사용자가 일부만 켜는 케이스 있는지 확인 필요.

→ ID-003 본문 결정에 종속. (a) 가 단순 통합, (b) 가 빈도 평가 후 외부화.

### 영향 범위

- 변경: `internal/tool/tool_gateway.go` + `internal/tarsserver/helpers_build_tools.go`
- 테스트: 통합 패턴 테스트 추가
- 사용자 영향: cfg flag 변경 시 호환성 (a)면 1 flag로 통합

### 관련 finding

- [ID-001](../ideas.md#id-001) — 빌트인 툴 인벤토리 슬림화
- [ID-003](../ideas.md#id-003) — aggregator 통합 정책. process 패턴 권장 (s15)

---

## RF-041 — `subagents_run` + `subagents_plan` + `subagents_orchestrate` 3 툴 통합

- **Status**: open
- **Location**: `internal/tool/tool_subagents.go` (344) + `tool_subagents_plan.go` (629) + `tool_subagents_orchestrate.go` (539)
- **Discovered in**: [journal/2026-04-25-17-tool-subagents.md](../journal/2026-04-25-17-tool-subagents.md)
- **Recommendation**: 단일 `subagents` aggregator (action: run / plan / orchestrate). plan 의 출력 = orchestrate 의 입력 (같은 `subagentFlowInput` schema) — 자연스러운 통합 후보.
- **Estimated effort**: M
- **Risk**: low

### 현재 상태

3 빌트인 툴, 총 1512줄, 모두 같은 도메인 (gateway 위에 얹힌 multi-agent 워크플로):

| 툴 | 책임 | 입력 |
|----|------|------|
| `subagents_run` | 단순 fan-out (parallel/consensus) | tasks 배열 |
| `subagents_plan` | LLM-as-planner — goal → 구조화 flow JSON | goal + targets + constraints |
| `subagents_orchestrate` | plan 실행 (parallel + sequential + placeholder rendering) | subagentFlowInput |

**핵심 관찰**: `subagents_plan` 의 출력 schema = `subagents_orchestrate` 의 입력 schema (`subagentFlowInput`). 즉 **plan→orchestrate 가 같은 데이터 구조** — 통합이 자연스럽고 의미적으로 정당.

description 합 ~150자 + parameter schema 3 종 ~3KB → **시스템 프롬프트 ~500-700 토큰/turn** (기능 활성 시 매우 큼).

### 제안하는 변경

`process` 패턴 (s15) 으로 통합:

```yaml
name: subagents
description: "Multi-agent orchestration. Actions: run (simple parallel/consensus fan-out), plan (LLM-as-planner: goal → staged flow JSON), orchestrate (execute staged flow with parallel/sequential steps and {{task.X.summary}} placeholders)."
parameters:
  type: object
  properties:
    action: { enum: [run, plan, orchestrate] }
    agent: { type: string }
    flow_id: { type: string }
    timeout_ms: { type: integer }
    # run 만:
    mode: { enum: [parallel, consensus] }
    consensus: { ... }
    tasks: { ... }
    # plan 만:
    goal: { type: string }
    targets: { ... }
    max_steps: { ... }
    constraints: { ... }
    hints: { ... }
    # orchestrate 만:
    steps: { ... }
  required: [action]
  additionalProperties: false
```

→ 시스템 프롬프트 비용 -50% 이상 가능. LLM 정확도는 process 패턴 사례로 입증 (s15).

### 영향 범위

- 변경: 3 파일 통합 + dispatch — `tool_subagents.go` 메인 + sub-action 파일들
- 테스트: aggregator 진입점 테스트 추가, 기존 sub-action 로직 그대로
- 외부 caller: `subagents_run`/`subagents_plan`/`subagents_orchestrate` alias 유지 (CanonicalToolName map 에 이미 alias 정의됨, s12 확인)

### 관련 finding

- [ID-003](../ideas.md#id-003) — 빌트인 툴 통합 정책. file/web/gateway-message-nodes 와 함께 4 번째 명확한 통합 케이스
- [RF-040](#rf-040) — message/nodes/gateway 통합 (같은 패턴)
- [Q-012](../questions.md#q-012) — plan/orchestrate 의 실제 호출 빈도 (Q 신규)

---

## RF-042 — `subagents_plan` 의 normalization layer 두꺼움 — strict structured output 으로 단순화 검토

- **Status**: open
- **Location**: `internal/tool/tool_subagents_plan.go:321-628` (normalization 영역, ~300줄)
- **Discovered in**: [journal/2026-04-25-17-tool-subagents.md](../journal/2026-04-25-17-tool-subagents.md)
- **Recommendation**: planner LLM 의 strict tool calling / structured output 모드 활성화 후 normalization 일부 제거. 최소한 `normalizeSubagentFlowPlan` (ID 정규화 + reference rewrite) 는 유지하되 `collectPlannerTargets` + `ensurePlannerTargetsInPlan` 부분 단순화 가능성 평가.
- **Estimated effort**: M
- **Risk**: medium (LLM 출력 회귀 위험)

### 현재 상태

`subagents_plan` 의 normalization layer ~300줄:

| 함수 | 책임 |
|------|------|
| `normalizeSubagentFlowPlan` | step ID + task ID 정규화 (sanitize + 충돌 시 numbering) |
| `rewritePlannerDependsOn` / `rewritePlannerPromptReferences` | ID 변경 시 depends_on + `{{task.X.summary}}` placeholder 도 같이 rewrite |
| `resolvePlannerReference` / `registerPlannerReference` | reference 매핑 (raw / lower / sanitized 3 변종 시도) |
| `collectPlannerTargets` | goal/constraints/hints 에서 path auto-extract |
| `ensurePlannerTargetsInPlan` | required targets 가 plan prompt 에 들어갔는지 확인 + 누락 시 자동 추가 |
| `plannerTaskForTarget` | basename/dirname matching 으로 가장 관련 task 찾기 |
| `decodeSubagentFlowInput` | JSON parsing 폴백 (raw → fenced strip → regex first {} chunk) |

→ **LLM 출력 noise 흡수에 ~300줄**. 정당하지만 보수 부담 큼.

### 의문

- 최신 OpenAI/Anthropic API 는 **strict tool calling + structured output** 지원
- RoleGatewayPlanner 의 LLM 이 strict mode 사용하면 schema 위반 자동 거부 → ID 정규화 + JSON parsing 폴백 일부 redundant
- 단 ID sanitize (LLM이 한글 ID 만들어도 sanitize 해야 함) + reference rewrite (변경된 ID 따라가기) 는 strict mode 와 직교 — 유지 필요
- `collectPlannerTargets` + `ensurePlannerTargetsInPlan` 는 LLM이 verbatim 보존 안 했을 때의 보강. strict prompt + few-shot 으로 줄일 수 있을 가능성

### 제안하는 변경

옵션:
- **(a) Strict tool calling 활성화** + JSON parsing 폴백 (`stripFencedJSON`, regex first {} chunk) 제거
- **(b) Required targets 보강을 prompt 강화로 대체** — system prompt 에 *"Required exact paths MUST appear verbatim in task prompts"* 강조 + few-shot example. ensurePlannerTargetsInPlan 제거 또는 안전망으로 축소
- **(c) 현 상태 유지** — 보수성. 회귀 위험 없음

→ (a) 부터 시도 권장. 단 RoleGatewayPlanner tier 가 strict mode 지원하는지 LLM router 확인 후.

### 영향 범위

- 변경: `internal/tool/tool_subagents_plan.go` (normalization 일부)
- 의존: `internal/llm/router.go` 의 strict mode 지원 여부 (s18+에서 검토 예정)
- 테스트: planner 출력 noise 케이스 회귀 검증
- 비용 절감: ~150-200줄 감소 가능

---

## RF-043 — `provider="codex-cli"` removed-alias error stub 제거

- **Status**: open
- **Location**: `internal/llm/provider.go:129-131`
- **Discovered in**: [journal/2026-04-25-18-llm-core.md](../journal/2026-04-25-18-llm-core.md)
- **Recommendation**: 그냥 삭제. default unsupported 분기로 자연스럽게 흐르게.
- **Estimated effort**: trivial
- **Risk**: trivial

### 현재 상태

```go
if provider == "codex-cli" {
    return nil, fmt.Errorf("unsupported llm provider: codex-cli (removed)")
}
```

CLAUDE.md *"Avoid backwards-compatibility hacks like ... // removed comments for removed code"* 와 정면 위반. 사용자가 "codex-cli" 명시하면 default unsupported 분기 (`default: return nil, fmt.Errorf("unsupported llm provider: %s", provider)`) 로 흘러도 동일한 에러 메시지 거의 — *"unsupported llm provider: codex-cli"*.

### 제안하는 변경

3 줄 삭제. default 분기가 처리.

### 영향 범위

- 변경: `internal/llm/provider.go` 3 줄
- 테스트: codex-cli 케이스 검증이 있다면 정리. 에러 메시지 일치.

---

## RF-044 — `Router.Close()` 와 `newFallbackClient` 의 reserved-for-future stub 정리

- **Status**: open
- **Location**:
  - `internal/llm/router.go:181-184` (`Close` no-op)
  - `internal/llm/fallback_client.go` (전체, production 호출 0건)
- **Discovered in**: [journal/2026-04-25-18-llm-core.md](../journal/2026-04-25-18-llm-core.md)
- **Recommendation**: `Close` 는 진짜 cleanup 필요할 때 추가 (Client interface 에 Close 추가 후) 또는 제거. `fallbackClient` 는 production 사용 0건 — 사용 의도가 있다면 wiring 추가, 없다면 제거.
- **Estimated effort**: S
- **Risk**: low

### 현재 상태

#### Router.Close

```go
func (r *multiTierRouter) Close() error {
    // llm.Client currently has no Close method; reserved for future use.
    return nil
}
```

→ 호출자가 `defer router.Close()` 해도 무용. Client interface 에 Close 가 없으므로 cleanup 할 게 없음.

CLAUDE.md *"Don't add features ... beyond what the task requires"* + *"Don't add error handling, fallbacks, or validation for scenarios that can't happen"* 의 정신과 충돌.

#### fallbackClient

`newFallbackClient` 의 caller grep 결과: **테스트 파일에서만** (`fallback_client_test.go:37, 63, 82`). production wiring 0건.

→ 의도된 미래 기능 — primary 실패 시 fallback provider 자동 사용. 하지만 wiring 안 됨 → 사실상 dead code.

### 제안하는 변경

옵션:
- **(a) 둘 다 제거**: `Close()` 메서드 + `Router` interface 의 `Close()` 시그니처 + fallback_client.go 전체 + 테스트.
- **(b) Close 만 제거 + fallback 유지**: fallback 은 의도된 기능이라 wiring 만 추가. 단 wiring 결정은 사용자 정책 (primary/fallback provider 셋업).
- **(c) 둘 다 유지**: 미래 기능 stub 으로. 현재 비용 작음.

→ (a) 권장. CLAUDE.md *"If you are certain that something is unused, you can delete it completely"* 정렬. fallback 이 정말 필요해지면 그때 다시 추가.

### 영향 범위

- 변경: `internal/llm/router.go` (Close 제거 + interface 정리), `fallback_client.go` (전체) + 관련 테스트
- 호출자: `defer router.Close()` 호출자 grep 후 정리 필요

### 관련 finding

- [RF-043](#rf-043) — codex-cli stub. 같은 *"removed/reserved"* 카테고리
- CLAUDE.md *"Avoid backwards-compatibility hacks"* 일관성

---

## RF-045 — `openai_compat_client` 의 context.Value 로 response 전달 패턴 단순화

- **Status**: open
- **Location**: `internal/llm/openai_compat_client.go:81-87, 114-116, 207-209`
- **Discovered in**: [journal/2026-04-25-19-llm-provider-1.md](../journal/2026-04-25-19-llm-provider-1.md)
- **Recommendation**: response 를 직접 인자로 전달. context.Value 패턴 제거.
- **Estimated effort**: trivial
- **Risk**: trivial

### 현재 상태

```go
req = req.WithContext(context.WithValue(req.Context(), openAICompatibleResponseContextKey{}, resp))
if opts.OnDelta != nil {
    return c.chatStreaming(ctx, req, opts)
}
return c.chatNonStreaming(ctx, req)

func (c *OpenAICompatibleClient) chatStreaming(ctx context.Context, req *http.Request, opts ChatOptions) (ChatResponse, error) {
    _ = ctx  // ← 사용 안 함
    resp := req.Context().Value(openAICompatibleResponseContextKey{}).(*http.Response)  // ← context.Value 로 추출
    ...
}
```

→ **비표준 패턴**:
- ctx 가 인자에 있지만 `_ = ctx` 로 사용 안 함 (이미 req.Context() 가 같은 ctx)
- response 를 ctx 의 value 로 전달 — ctx 는 보통 cancelation/메타데이터용
- 추출 시 type assertion `.(*http.Response)` — runtime 안전성 떨어짐 (compile-time 검증 X)
- 단순화: 메서드 시그니처에 `resp *http.Response` 직접 추가

### 제안하는 변경

```go
if opts.OnDelta != nil {
    return c.chatStreaming(resp, opts)
}
return c.chatNonStreaming(resp)

func (c *OpenAICompatibleClient) chatStreaming(resp *http.Response, opts ChatOptions) (ChatResponse, error) {
    ...
}
```

→ ctx 인자 제거 + context.Value 패턴 제거 + key type 정의 (`openAICompatibleResponseContextKey`) 제거. ~10줄 감소.

### 영향 범위

- 변경: `internal/llm/openai_compat_client.go` (3곳 시그니처 + key type 제거)
- 테스트: 영향 없음 (외부 시그니처 안 변함)

---

## RF-046 — `openai_compat_client` 의 PDF document → silent text placeholder

- **Status**: open
- **Location**: `internal/llm/openai_compat_client.go:362-368`
- **Discovered in**: [journal/2026-04-25-19-llm-provider-1.md](../journal/2026-04-25-19-llm-provider-1.md)
- **Recommendation**: PDF 첨부 시 caller 에게 명시적 알림 (에러 또는 메타) — silent dropout 방지.
- **Estimated effort**: S
- **Risk**: low

### 현재 상태

```go
case "document":
    blocks = append(blocks, map[string]any{
        "type": "text",
        "text": "[Attached PDF document]",
    })
```

→ OpenAI/Gemini-compat 이 PDF 직접 지원 안 함. 사용자가 PDF 첨부 → LLM 에 *"[Attached PDF document]"* placeholder 만 전달. **사용자 PDF content 가 silent 으로 사라짐**.

### 문제

CLAUDE.md *"Don't add error handling for scenarios that can't happen"* 정신과 충돌 가능 — *"happen 하는 케이스의 silent failure"*. 사용자가 PDF 분석 요청 → LLM 은 placeholder 만 보고 *"PDF 내용을 알 수 없습니다"* 답변 → 사용자 혼란.

대조: anthropic 은 native PDF 지원 (toAnthropicContentBlocks document 케이스). gemini-native 도 별도 (s20 확인).

### 제안하는 변경

옵션:
- **(a)** 에러 반환 — *"openai-compat client does not support PDF; use anthropic or gemini-native"*. 명확하지만 disruptive.
- **(b)** 텍스트 추출 fallback — `pdftotext` 또는 외부 라이브러리로 본문 추출 후 text block 으로 전송. 의존성 추가.
- **(c)** placeholder 강화 — `[Unavailable PDF content: provider=openai/gemini does not support PDFs. Switch to anthropic or extract text first.]` 메시지로 LLM 이 알 수 있게.

→ (c) 가 가장 단순. (a) 는 caller 가 provider 알아야 함 → 현재 추상화 깨짐.

### 영향 범위

- 변경: `internal/llm/openai_compat_client.go` (placeholder 메시지)
- 테스트: PDF 첨부 시 메시지 검증

---

## RF-047 — Provider 별 ChatOptions 지원 매트릭스 문서화

- **Status**: open
- **Location**: `internal/llm/provider.go:59-71` (ChatOptions struct)
- **Discovered in**: [journal/2026-04-25-19-llm-provider-1.md](../journal/2026-04-25-19-llm-provider-1.md)
- **Recommendation**: ChatOptions 의 각 필드가 어떤 provider 에서 지원되는지 docstring 또는 별도 문서. caller 가 *"이 provider 에서 reasoning_effort 가 무시되는가?"* 추측 안 해도 되게.
- **Estimated effort**: S
- **Risk**: trivial

### 현재 상태

ChatOptions 의 8 필드 (OnDelta/Tools/ToolChoice/ReasoningEffort/ThinkingBudget/ServiceTier + Content/messages 의 ContentBlocks):

| 필드 | anthropic | openai | gemini (compat) | claude-code-cli |
|------|-----------|--------|-----------------|----------------|
| OnDelta (streaming) | ✅ | ✅ | ✅ | ✅ |
| Tools | ✅ | ✅ | ✅ | ❌ (silent ignore) |
| ToolChoice (string) | ✅ (any/auto/none) | ✅ | ✅ | ❌ |
| ToolChoice (specific tool name) | ✅ | ❌ (string only) | ❌ | ❌ |
| ReasoningEffort | ❌ | ✅ | ❌ (skip) | ❌ |
| ThinkingBudget | ✅ | ❌ | ❌ | ❌ |
| ServiceTier | ❌ | ✅ | ❌ (skip) | ❌ |
| ContentBlocks (image) | ✅ | ✅ | ✅ | ❌ |
| ContentBlocks (document/PDF) | ✅ | ⚠️ placeholder [RF-046] | ⚠️ (s20 확인) | ❌ |

→ **caller 가 알 수 없음**. 코드 내부의 각 분기 (`c.label != "gemini"`, `claude-code-cli` 의 tool 무시 등) 는 silent.

### 제안하는 변경

옵션:
- **(a)** ChatOptions struct 의 각 필드 docstring 에 supported providers 명시.
- **(b)** 별도 문서 `docs/llm-providers.md` 또는 CLAUDE.md 추가.
- **(c)** `Client` interface 에 `Capabilities() ProviderCapabilities` 메서드 추가 — runtime introspection 가능. 단 추상화 비용.

→ (a) + (b) 권장. (c) 는 over-engineering.

### 영향 범위

- 변경: `internal/llm/provider.go` 의 ChatOptions docstring + 새 문서
- 테스트: 영향 없음
- RF-042 (strict mode) 의 결정 인풋 — anthropic 만 specific tool name 지원 명시 필요

### 관련 finding

- [RF-042](#rf-042) — subagents_plan normalization 단순화. anthropic 사용 시 즉시 가능, openai 는 추가 작업
- [RF-046](#rf-046) — PDF silent placeholder

---

## RF-048 — `openai_codex` 의 `tool_choice: "auto"` hardcoded — ChatOptions.ToolChoice 무시

- **Status**: open (ID-004 Phase 1 핵심)
- **Location**: `internal/llm/openai_codex_client.go:413`
- **Discovered in**: [journal/2026-04-25-20-llm-provider-2.md](../journal/2026-04-25-20-llm-provider-2.md)
- **Recommendation**: Responses API 의 `tool_choice` object 형태 (`{"type": "function", "name": "X"}`) 지원 추가. caller 의 ChatOptions.ToolChoice 매핑.
- **Estimated effort**: S
- **Risk**: low

### 현재 상태

```go
// buildOpenAICodexRequestBody (line 407-415)
if len(opts.Tools) > 0 {
    tools, err := convertOpenAICodexTools(opts.Tools, nameMap)
    if err != nil {
        return nil, err
    }
    body["tools"] = tools
    body["tool_choice"] = "auto"          // ← hardcoded
    body["parallel_tool_calls"] = true
}
```

→ caller 의 `opts.ToolChoice` 가 무엇이든 silent 으로 `"auto"` 사용. anthropic (s19) 의 풍부한 ToolChoice (specific tool name) 와 정반대 — capability 격차 핵심.

### 제안하는 변경

```go
body["tool_choice"] = toCodexToolChoice(opts.ToolChoice)  // string vs object 분기
```

`toCodexToolChoice`:
- `""` / `"auto"` → `"auto"`
- `"none"` → `"none"`
- `"required"` → `"required"`
- `"some_tool"` → `{"type": "function", "name": "some_tool"}`

→ Responses API 가 OpenAI Chat Completions 와 같은 `tool_choice` schema 를 따른다는 가정. 실제 Responses API 문서 확인 필요 (만약 다르면 별도 wire format).

### 영향 범위

- 변경: `openai_codex_client.go` (buildOpenAICodexRequestBody + 새 헬퍼)
- 테스트: ToolChoice 변종 케이스 추가
- ID-004 의존: Phase 1 의 OpenAI Codex 부분 핵심. RF-042 (subagents_plan) 의 codex 환경 적용 가능

### 관련 finding

- [ID-004](../ideas.md#id-004) — OpenAI capability 격차 해소 (1순위)
- [RF-042](#rf-042) — subagents_plan normalization 단순화 (이 RF 적용 시 codex 환경 가능)

---

## RF-049 — `openai_codex` 의 ChatOptions advanced fields silent 무시

- **Status**: open (ID-004 Phase 1)
- **Location**: `internal/llm/openai_codex_client.go:396-416` (buildOpenAICodexRequestBody)
- **Discovered in**: [journal/2026-04-25-20-llm-provider-2.md](../journal/2026-04-25-20-llm-provider-2.md)
- **Recommendation**: Responses API 의 reasoning/structured output/service_tier 매핑 추가.
- **Estimated effort**: M (Responses API spec 조사 필요)
- **Risk**: low

### 현재 상태

`buildOpenAICodexRequestBody` 가 body 에 채우는 것:

```go
body := map[string]any{
    "model":   model,
    "store":   false,
    "stream":  stream,
    "input":   input,
    "include": []string{"reasoning.encrypted_content"},
}
if instructions := ...; len(instructions) > 0 { body["instructions"] = ... }
if len(opts.Tools) > 0 { body["tools"], body["tool_choice"], body["parallel_tool_calls"] = ... }
// 끝.
```

→ **다음 ChatOptions 필드 모두 silent 무시**:
- `ReasoningEffort` — Responses API 의 `reasoning: { effort: "..." }` 활용 가능?
- `ThinkingBudget` — gemini-native 처럼 thinking budget 매핑 가능?
- `ServiceTier` — `service_tier` 필드?
- `ResponseFormat` (ID-004 Phase 1 신규 필드) — `text: { format: { type: "json_schema", schema, strict } }` 가능?

OpenAI Codex 가 Responses API 활용하면서 advanced features 를 모두 미활용. anthropic 수준 격차의 핵심.

### 제안하는 변경

OpenAI Responses API 문서 조사 후:

1. **Reasoning** (Responses API native):
   ```go
   if effort := effectiveReasoningEffort(c.config, opts); effort != "" {
       body["reasoning"] = map[string]any{"effort": effort}
   }
   ```

2. **Structured output** (ID-004 Phase 1 신규 ChatOptions.ResponseFormat 의존):
   ```go
   if rf := opts.ResponseFormat; rf != nil && rf.Type == "json_schema" {
       body["text"] = map[string]any{
           "format": map[string]any{
               "type":   "json_schema",
               "name":   rf.Name,
               "schema": rf.Schema,
               "strict": rf.Strict,
           },
       }
   }
   ```

3. **Service tier**:
   ```go
   if tier := effectiveServiceTier(c.config, opts); tier != "" {
       body["service_tier"] = tier
   }
   ```

→ 각 항목별로 Responses API 가 실제 지원하는지 검증. **ID-004 결정 후 작업**.

### 영향 범위

- 변경: `openai_codex_client.go`
- 의존: ID-004 Phase 1 의 ChatOptions 확장 (ResponseFormat 신규 필드)
- 테스트: 각 advanced field 의 wire format 검증
- 사용자 영향: openai-codex 가 anthropic 수준 capability 도달 (사용자 우선순위)

### 관련 finding

- [ID-004](../ideas.md#id-004) — OpenAI capability 격차 해소 (1순위)
- [RF-048](#rf-048) — ToolChoice (같은 카테고리)
- [RF-042](#rf-042) — subagents_plan 단순화

---

## RF-050 — `semaphore.go` + `execution_semaphore.go` 코드 중복

- **Status**: open
- **Location**: `internal/gateway/semaphore.go` (36줄, `weightedSemaphore`), `internal/gateway/execution_semaphore.go` (36줄, `executionSemaphore`)
- **Discovered in**: [journal/2026-04-25-21-gateway-pt1.md](../journal/2026-04-25-21-gateway-pt1.md)
- **Recommendation**: 단일 `semaphore` 타입으로 통합. 두 사용처 모두 같은 패턴 (chan struct{} based + Acquire/Release).
- **Estimated effort**: trivial
- **Risk**: trivial

### 현재 상태

두 파일이 완전 동일한 코드 (type name 만 다름):

```go
// semaphore.go
type weightedSemaphore struct { ch chan struct{} }
func newWeightedSemaphore(limit int) *weightedSemaphore { ... }
func (s *weightedSemaphore) Acquire(ctx context.Context) error { ... }
func (s *weightedSemaphore) Release() { ... }

// execution_semaphore.go — 위와 100% 동일, type 이름만 executionSemaphore
```

→ 의도상 둘 다 *"limited concurrent slot"* 패턴. 분리 이유 없음.

사용처:
- `executionSemaphore`: 전체 실행 슬롯 (`r.executionSem`)
- `weightedSemaphore`: subagent pool + consensus runs + consensus pool (3 인스턴스)

→ 같은 기능 다른 instance. 단일 타입으로 충분.

### 제안하는 변경

```go
// semaphore.go (통합)
type semaphore struct { ch chan struct{} }
func newSemaphore(limit int) *semaphore { ... }
func (s *semaphore) Acquire(ctx context.Context) error { ... }
func (s *semaphore) Release() { ... }
```

→ `execution_semaphore.go` 제거. Runtime struct 의 4 필드 (`executionSem`/`subagentPool`/`consensusRuns`/`consensusPool`) 모두 같은 타입.

### 영향 범위

- 변경: `internal/gateway/{semaphore.go, execution_semaphore.go, runtime.go, types.go}` (Runtime field 타입)
- 테스트: `semaphore_test.go` 가 둘 다 검증하는지 확인
- ID-005 (gateway → agentruntime 변경) 와 결합 가능

---

## RF-051 — `CommandExecutor` 가 production wiring 0건 (test-only)

- **Status**: open
- **Location**: `internal/gateway/executor.go:221-352` (~130줄)
- **Discovered in**: [journal/2026-04-25-21-gateway-pt1.md](../journal/2026-04-25-21-gateway-pt1.md)
- **Recommendation**: 사용 의도 확정 후 wiring 추가 또는 제거. CLAUDE.md *"Don't add features beyond what the task requires"* + *"If you are certain that something is unused, you can delete it completely"* 정렬 결정.
- **Estimated effort**: S
- **Risk**: low

### 현재 상태

`CommandExecutor` (executor.go:221-352, 130줄):
- subprocess 기반 agent executor
- env (TARS_RUN_ID/TARS_SESSION_ID/TARS_WORKSPACE_ID 자동 전달)
- workDir + timeout
- stdin/stdout/stderr 처리
- `sanitizeMetadataEnvValue` (control char 거부, injection 방지)

caller grep 결과 (`NewCommandExecutor`):
- production: **0건**
- 테스트: `executor_test.go:39, 45, 52` 만

→ RF-044 카테고리 (Router.Close + fallbackClient 의 reserved-for-future stub) 와 같은 패턴. **production wiring 없음**.

### 의도 추정

`PromptExecutor` 가 in-process LLM 호출 (default). `CommandExecutor` 는 외부 subprocess 호출 — 의도:
- 외부 agent (Python script, Rust binary 등) 통합?
- claude-code-cli 같은 CLI wrapper 의 일반화?

CLAUDE.md 의 *"Default pattern: skill+CLI invoked via the builtin `bash` tool"* 와 부분 중복 — `bash` tool 이 이미 subprocess 호출 가능. CommandExecutor 는 *agent 단위* subprocess 라 다름.

### 제안하는 변경

옵션:
- **(a) 제거**: production wiring 없음 + skill+CLI 패턴이 대안. 130줄 + 테스트 정리.
- **(b) wiring 추가**: 외부 agent 등록 메커니즘 (`internal/extensions/manager.go` + plugin 정의에서 `command:` type) 활성화. 작업 비용 큼.
- **(c) 보존 + 의도 docstring**: *"reserved for external command-based agents (e.g., Python data analysis agents)"* 명시. 현 비용 0.

→ (a) 권장 — CLAUDE.md *"If you are certain that something is unused, you can delete it completely"* 정렬. RF-044 의 Router.Close + fallbackClient 와 함께 묶어 *"YAGNI 위반 stub 일괄 정리"* PR.

### 관련 finding

- [RF-044](#rf-044) — Router.Close + fallbackClient 의 reserved-for-future stub
- [RF-007](#rf-007) — 빌트인 플러그인 시스템 자체 제거 (extensions manager 가 agent registration 의 메인 표면)
- [TN-003](../tensions.md#tn-003) — `tools_provider.script` 미구현

---

## RF-052 — consensus strategy `vote` 미구현 — schema 와 구현 mismatch

- **Status**: open
- **Location**: `internal/gateway/consensus.go:180-183` (aggregateConsensus), `internal/tool/tool_subagents.go:27` (schema enum)
- **Discovered in**: [journal/2026-04-25-22-gateway-pt2.md](../journal/2026-04-25-22-gateway-pt2.md)
- **Recommendation**: schema enum 을 `["synthesize"]` 로 축소 또는 vote 구현 추가.
- **Estimated effort**: S (enum 축소) / M (vote 구현)
- **Risk**: low

### 현재 상태

`tool/tool_subagents.go:27` schema:
```yaml
strategy: { type: "string", enum: ["synthesize", "vote"] }
```

`consensus.go:180-183` 구현:
```go
func (r *Runtime) aggregateConsensus(ctx, state, executor, strategy, successes) (string, error) {
    if strings.ToLower(strings.TrimSpace(strategy)) != "synthesize" {
        return "", fmt.Errorf("consensus strategy %q is not implemented", strategy)
    }
    // ... synthesize prompt 만 구현
}
```

→ LLM 이 schema 보고 `strategy: "vote"` 보내면 **silent runtime error**. CLAUDE.md *"Avoid backwards-compatibility hacks like ... // removed comments for removed code"* + *"Don't add features beyond what the task requires"* 둘 다 반영:
- vote 가 reserved-for-future 라면 schema 에서 제거
- 의도된 미래 기능이라면 작업 계획 우선순위 명확히

### 제안하는 변경

옵션:
- **(a) schema enum 축소** — `["synthesize"]` 만. 사용자 의도 단순. RF-019 카테고리 (Reserved-for-future stub).
- **(b) vote 구현 추가** — 각 variant 의 핵심 답을 추출 + 가장 빈도 높은 답 선택 (또는 변형). 작업 비용 medium.
- **(c) majority/best 같은 더 단순한 strategy 추가** — synthesize 의 가벼운 버전. 토큰 비용 ↓.

→ **(a) 권장** (사용자가 명시적 vote 요청 안 했음). RF-051 (CommandExecutor) + RF-044 (Router.Close + fallbackClient) 와 함께 *"YAGNI 위반 stub 일괄 정리"* PR.

### 영향 범위

- 변경: `internal/tool/tool_subagents.go` (schema enum) + `internal/gateway/consensus.go` (분기 단순화)
- 테스트: vote 케이스 검증이 있다면 정리
- 외부 caller: schema 좁아지지만 silent runtime error 보다 나음

### 관련

- [RF-019](#rf-019), [RF-044](#rf-044), [RF-051](#rf-051) — YAGNI 위반 stub 카테고리

---

## RF-053 — `finalizeRunLocked` 200줄 — 성공/실패 분리 + run summary 위임

- **Status**: open
- **Location**: `internal/gateway/runtime_run_execute.go:128-193` (finalizeRunLocked)
- **Discovered in**: [journal/2026-04-25-22-gateway-pt2.md](../journal/2026-04-25-22-gateway-pt2.md)
- **Recommendation**: `finalizeRunFailed` + `finalizeRunCompleted` 로 분리. policy 진단 추출은 별도 헬퍼.
- **Estimated effort**: S
- **Risk**: low (순수 가독성 개선)

### 현재 상태

`finalizeRunLocked` 가 한 함수에 6 책임:
1. status + completedAt + resolved provider metadata 채움
2. err != nil 분기 → status=Failed + diagnostic 분류 + policy 정보 추출
3. err == nil 분기 → status=Completed + response 채움
4. 4종 event publish (failed/finished 각각 2번 — RF-054 참고)
5. `appendRunSummaryToMain` (worker→parent main session)
6. close done channel + trim history + version increment

→ 200줄. 가독성 ↓. 테스트 분기 복잡.

### 제안하는 변경

```go
func (r *Runtime) finalizeRunLocked(state, resp, metadata, err) {
    r.fillRunCompletionMetadata(state, metadata, time)
    if err != nil {
        r.finalizeRunFailedLocked(state, err)
    } else {
        r.finalizeRunCompletedLocked(state, resp)
    }
    r.appendRunSummaryToMain(state.run, ...)
    r.closeRunDoneLocked(state)
    r.trimRunHistoryLocked()
    r.stateVersion++
}
```

→ 각 분기 ~30-50줄로 축소. 명명 명확.

### 영향 범위

- 변경: `internal/gateway/runtime_run_execute.go`
- 테스트: 같은 동작 검증 — 회귀 위험 작음
- ID-005 와 함께 진행 가능 (Phase 2 식별자 정리)

---

## RF-054 — `finalizeRunLocked` 의 동일 event type 중복 publish

- **Status**: open
- **Location**: `internal/gateway/runtime_run_execute.go:159-189` (run_failed + run_finished 각 2회)
- **Discovered in**: [journal/2026-04-25-22-gateway-pt2.md](../journal/2026-04-25-22-gateway-pt2.md)
- **Recommendation**: 한 번만 publish. subscriber duplicate 처리 부담 제거.
- **Estimated effort**: trivial
- **Risk**: low (subscriber 가 1회 받는다고 가정해도 안전)

### 현재 상태

```go
// failed 케이스
r.publishRunEvent(state.run.ID, RunEvent{
    Type: "run_failed", RunID: ..., Agent: ..., Status: ..., ResolvedAlias: ..., Error: ...,
})  // 첫 번째 publish — 풍부한 metadata
r.closeRunDoneLocked(state)
r.trimRunHistoryLocked()
r.stateVersion++
r.appendRunSummaryToMain(state.run, "")
r.publishRunEvent(state.run.ID, RunEvent{
    Type: "run_failed", RunID: ..., Timestamp: finishedAt, Status: ..., Error: ...,
})  // 두 번째 publish — 단순 metadata + Timestamp

// finished 케이스도 동일 패턴
```

→ **같은 type 의 event 가 2번**. subscriber (콘솔 SSE, 외부 monitoring) 가 둘 다 받음. 의도 불명:
- 첫 번째: agent/policy/resolved 정보 풍부 (publish 시점 기준)
- 두 번째: timestamp + summary (appendRunSummaryToMain 후)

### 의도 추정

(a) 첫 번째는 "run 자체 종료" 알림, 두 번째는 "summary 까지 처리 완료" 알림 — 두 단계 의미? 의도면 type 분리 (`run_failed_pending` + `run_failed_summarized`)
(b) 단순 코드 잔재 — 한 번만 publish 충분

→ (b) 가능성 높음. 두 번째 publish 가 첫 번째와 거의 같은 정보 + timestamp만 추가.

### 제안하는 변경

```go
// failed 케이스
r.fillRunCompletionMetadata(state, metadata, time)
r.recordRunFailureLocked(state, err)  // diagnostic + policy 정보 채움
r.appendRunSummaryToMain(state.run, "")
r.closeRunDoneLocked(state)
r.trimRunHistoryLocked()
r.stateVersion++
r.publishRunEvent(state.run.ID, RunEvent{
    Type: "run_failed", RunID, Timestamp, Agent, Status, ResolvedAlias/Kind/Model,
    Error, Message: ...,  // 모든 정보 한 번에
})
```

→ 한 번만 publish. subscriber duplicate 처리 부담 0.

### 영향 범위

- 변경: `internal/gateway/runtime_run_execute.go`
- 테스트: SSE event 1회 받는지 검증
- 콘솔 UI / 외부 monitoring: duplicate 처리 코드 제거 가능 (있다면)
- RF-053 와 결합 가능

---

## RF-055 — `runtime_nodes.go` 의 3 hardcoded node 가 dead/demo 가까움

- **Status**: open
- **Location**: `internal/gateway/runtime_nodes.go` (63줄), `internal/tool/tool_gateway.go` (`NodesTool`)
- **Discovered in**: [journal/2026-04-25-23-gateway-pt3.md](../journal/2026-04-25-23-gateway-pt3.md)
- **Recommendation**: 3 node 모두 제거 또는 외부화 — RF-040 (message/nodes/gateway 통합) 과 결합. NodesTool 도 동시 정리.
- **Estimated effort**: S
- **Risk**: low

### 현재 상태

3 hardcoded node:
- `echo` — args 그대로 반환 (dummy)
- `clock.now` — 현재 UTC (그러나 LLM system prompt 에 *"Current time"* 이미 주입 — s11)
- `sessions.latest` — 가장 최근 세션 (그러나 `sessions_list` aggregator 가 더 자연스러움)

각 node 가 **이미 다른 곳에서 더 자연스럽게 노출** → dead 가까움.

### 제안하는 변경

옵션 (a) 제거 권장. RF-040 + 빈 NodesTool 일괄 정리. CLAUDE.md *"If you are certain that something is unused, you can delete it completely"* 정렬.

### 영향 범위

- 변경: `internal/gateway/runtime_nodes.go` 전체 + `internal/tool/tool_gateway.go` (NodesTool)
- Runtime API: `Nodes()`/`NodeDescribe()`/`NodeInvoke()` 제거 — `GatewayStatus.Nodes` 필드도 제거 (콘솔 UI 영향)

### 관련

- [Q-016](../questions.md#q-016) — Nodes 사용 빈도
- [RF-040](#rf-040) — message/nodes/gateway 통합/외부화

---

## RF-056 — `Inbound`/`OutboundTelegram` 의 channelID 결정 fallback 불일치

- **Status**: open
- **Location**: `internal/gateway/runtime_channels.go:77-89` (Inbound), `runtime_channels.go:95-110` (Outbound)
- **Discovered in**: [journal/2026-04-25-23-gateway-pt3.md](../journal/2026-04-25-23-gateway-pt3.md)
- **Recommendation**: 의도 명시화 (코멘트 또는 헬퍼 함수 추출).
- **Estimated effort**: trivial
- **Risk**: trivial

### 현재 상태

```go
// Inbound — botID → "telegram"
channelID := strings.TrimSpace(botID)
if channelID == "" { channelID = "telegram" }

// Outbound — chatID → botID → "telegram"
channelID := strings.TrimSpace(chatID)
if channelID == "" { channelID = strings.TrimSpace(botID) }
if channelID == "" { channelID = "telegram" }
```

→ 의도 추정: inbound 는 *"bot-level channel 통합"*, outbound 는 *"chat-level channel 분리"*. 합리적이나 **코멘트 없음** — silent inconsistency.

### 제안하는 변경

(a) 헬퍼 추출 (`inboundTelegramChannelID` / `outboundTelegramChannelID`) + (b) 의도 코멘트. 동작 변화 없음.

---

## RF-057 — `ReportsRuns`/`ReportsChannels` 의 archive flag 의미 모호

- **Status**: open
- **Location**: `internal/gateway/runtime_reports.go:120-184`
- **Discovered in**: [journal/2026-04-25-23-gateway-pt3.md](../journal/2026-04-25-23-gateway-pt3.md)
- **Recommendation**: archive flag 의 의미 분리 — *"디스크 archive"* vs *"report endpoint 활성"*.
- **Estimated effort**: S
- **Risk**: low

### 현재 상태

```go
if !r.opts.GatewayArchiveEnabled {
    return ReportRuns{}, fmt.Errorf("gateway archive report is disabled")
}
// ... in-memory runs 반환 (archive 디스크 무관)
```

→ archive flag 가 **두 의미 중첩**: (1) 디스크 archive 활성 (2) in-memory report 노출 권한. 명시 안 됨.

### 제안하는 변경

옵션:
- **(a)** `GatewayReportEnabled` flag 분리 — archive 와 별개
- **(b)** docstring + config 설명 명시화

→ (a)/(b) 권장. ID-005 (config 마이그레이션) 와 결합 가능.

---

## RF-058 — `Store.loadIndex`/`saveIndex` O(N) full read+write — RF-017/021 카테고리

- **Status**: open
- **Location**: `internal/session/session.go:667-694` + 모든 mutation method
- **Discovered in**: [journal/2026-04-25-24-session.md](../journal/2026-04-25-24-session.md)
- **Recommendation**: incremental update (per-session JSON file) 또는 SQLite 마이그레이션. RF-017/021 와 함께 큰 결정.
- **Estimated effort**: L
- **Risk**: medium

### 현재 상태

매 mutation 마다 전체 sessions.json read + write. N session 큰 환경에서 hot path 비용 폭증.

### 동일 카테고리

- [RF-017](#rf-017) — `entries.jsonl` 의 O(N) load+rewrite
- [RF-021](#rf-021) — `KnowledgeStore.rebuildArtifacts`

→ **3 사례 누적** = 코드베이스 안티패턴. SQLite 또는 per-row file 마이그레이션 큰 결정.

---

## RF-059 — session `saveIndex` atomic write 미적용

- **Status**: open
- **Location**: `internal/session/session.go:688-694` (saveIndex), `internal/session/tasks.go:67-78` (SaveTasks)
- **Discovered in**: [journal/2026-04-25-24-session.md](../journal/2026-04-25-24-session.md)
- **Recommendation**: gateway/persistence.go 의 `writeJSONAtomic` 패턴 적용. 공통 헬퍼 (`internal/atomicwrite/`) 추출 검토.
- **Estimated effort**: S
- **Risk**: low

### 현재 상태

```go
return os.WriteFile(s.indexPath(), data, 0o644)  // ← partial write 가능
```

vs gateway/persistence.go:
```go
// tmp create + sync + close + rename — atomic
```

→ session 이 가장 사용자 facing 한데 atomic write 미적용. crash 시 sessions.json 손상 가능.

### 제안하는 변경

`writeJSONAtomic` 패턴을 공통 헬퍼로 추출 후 session/gateway/knowledge 모두 사용.

### 관련

- gateway/persistence.go의 writeJSONAtomic — reference (s22)
- knowledge.go의 buildKnowledgeDocument — 동일 카테고리

---

## RF-060 — Compaction `keepRecent` 3 전략 우선순위 명시화

- **Status**: open
- **Location**: `internal/session/compaction.go:84-128`
- **Discovered in**: [journal/2026-04-25-24-session.md](../journal/2026-04-25-24-session.md)
- **Recommendation**: docstring 명시 또는 explicit Mode enum.
- **Estimated effort**: trivial
- **Risk**: trivial

### 현재 상태

3 전략 silent priority:
1. `KeepRecentFraction` (default 30%) — 우선
2. `KeepRecentTokens` (절대 budget) — fraction 0 이면
3. `keepRecent` (count, default 20) — 둘 다 0 이면

→ caller 가 셋 다 주면 fraction 우선. **명시 안 됨**. *"왜 내 token budget 무시되는지"* 디버깅 어려움.

### 제안하는 변경

(a) docstring 우선순위 명시 — 작업 0, 의도 명시.

---

## RF-061 — `applySessionDefaults` 의 매 Get 호출 disk IO

- **Status**: open
- **Location**: `internal/session/session.go:168-191`, 호출처 Create/Get/SetWorkDirs/SetCurrentDir/EnsureNamed
- **Discovered in**: [journal/2026-04-25-24-session.md](../journal/2026-04-25-24-session.md)
- **Recommendation**: mutation method 에만 호출. Get 은 in-memory only.
- **Estimated effort**: S
- **Risk**: low

### 현재 상태

`applySessionDefaults` 가 (1) MkdirAll artifact dir + (2) legacy stat+rename + (3) symlink 해석 — **매 Get 마다 disk IO 3종**.

호출 빈도 (chat turn hot path):
- `gateway.Spawn` → sessionStore.Get (매 run)
- `memory_search.collectSessionMatches` → 매 chat turn fallback
- `prompt.collectSessionMatches` → 매 chat turn fallback

→ 큰 워크스페이스에서 latency 누적.

### 제안하는 변경

(a) `applySessionDefaults` 를 mutation method (Create/EnsureMain/SetWorkDirs) 에만 호출 + (c) legacy migration 만 startup 1회.

### 영향 범위

- 변경: `internal/session/session.go`
- 테스트: Get 시 artifact dir 자동 생성 안 되는지
- 회귀: artifact dir 사용하는 caller 가 lazy 생성 또는 명시 호출 필요

---

## RF-062 — `Config` 의 14 embed = ~350 field flat access — namespaced 검토

- **Status**: open
- **Location**: `internal/config/types.go:305-321` (Config struct), 50+ caller
- **Discovered in**: [journal/2026-04-25-25-config-pt1.md](../journal/2026-04-25-25-config-pt1.md)
- **Recommendation**: 큰 마이그레이션 비용. 단기는 docstring, 장기는 namespaced access (`cfg.Gateway.ConsensusEnabled`) 검토.
- **Estimated effort**: L
- **Risk**: medium

### 현재 상태

embed promotion → caller 가 도메인 명시 없이 직접 접근:
- `cfg.GatewayConsensusEnabled` / `cfg.PulseEnabled` / `cfg.ToolsWebSearchEnabled` / `cfg.MemoryEmbedAPIKey` / ...

총 ~350 필드 flat. autocomplete 만 의존. **어떤 도메인인지 caller 가 모름**.

### 제안하는 변경

옵션:
- **(a) namespaced** — embed 제거 + named field. 50+ caller 마이그레이션. 큰 비용
- **(b) docstring 강화** — 현 상태 + sub-config docstring 명시
- **(c) 현 상태 유지** — Go embed promotion 표준 패턴

→ **(c) 권장** — Go ecosystem 표준. (b) 는 보조 가능. (a) 는 ROI 낮음.

### 관련

- ID-005 Phase 3 와 같은 카테고리

---

## RF-063 — `ServiceTier` 2-level 의도 docstring 명시화

- **Status**: open
- **Location**: `internal/config/types.go:108, 123` (ServiceTier 두 위치), `llm_resolve.go:90-93`
- **Discovered in**: [journal/2026-04-25-25-config-pt1.md](../journal/2026-04-25-25-config-pt1.md)
- **Recommendation**: docstring 에 2-level 의도 명시.
- **Estimated effort**: trivial
- **Risk**: trivial

### 현재 상태

```go
type LLMProviderSettings struct { ServiceTier string }  // provider default
type LLMTierBinding struct { ServiceTier string }       // tier override
```

resolve 로직: tier override → provider default fallback.

→ 정당한 2-level 의도. caller 가 헷갈릴 수 있음 — *"왜 두 곳?"*

### 제안하는 변경

각 ServiceTier 의 docstring 에 2-level 명시:
- LLMProviderSettings.ServiceTier — *"provider-level default. tier override 가능"*
- LLMTierBinding.ServiceTier — *"provider default override. 빈 값 = provider default 사용"*

→ 작업 0, 의도 명시.

---

## RF-064 — 하드코딩된 default model 이름 outdated — CLAUDE.md 와 mismatch

- **Status**: open (ID-004 Phase 1 과 결합)
- **Location**: `internal/config/defaults.go:81-90` (5 default model constants)
- **Discovered in**: [journal/2026-04-25-26-config-pt2.md](../journal/2026-04-25-26-config-pt2.md)
- **Recommendation**: 최신 model 이름으로 갱신. ID-004 (provider capability) Phase 1 과 결합.
- **Estimated effort**: trivial
- **Risk**: low (default 만 — 사용자 config 우선)

### 현재 상태

```go
defaultOpenAIModel        = "gpt-4o-mini"            // 옛 모델
defaultOpenAICodexModel   = "gpt-5.3-codex"          // 옛 모델 (5.4 가 최신)
defaultClaudeCodeCLIModel = "sonnet"                 // OK (alias, 자동 latest)
defaultGeminiModel        = "gemini-2.5-flash"       // OK (최신)
defaultAnthropicModel     = "claude-3-5-haiku-latest" // ⚠️ Claude 3.5 — Claude 4.x 가 최신
```

CLAUDE.md (현재 환경 안내):
> Most recent Claude model family is Claude 4.X. Model IDs — Opus 4.7: 'claude-opus-4-7', Sonnet 4.6: 'claude-sonnet-4-6', Haiku 4.5: 'claude-haiku-4-5-20251001'

→ **Anthropic default model 이 거의 1년 outdated**. CLAUDE.md 본인이 *"latest"* 라고 명시한 모델과 mismatch.

### 제안하는 변경

```go
defaultOpenAIModel        = "gpt-5.4-mini"  // 또는 최신 small 모델
defaultOpenAICodexModel   = "gpt-5.4-codex" // 최신 codex
defaultAnthropicModel     = "claude-haiku-4-5-20251001"  // CLAUDE.md 명시
defaultClaudeCodeCLIModel = "sonnet"        // 유지 (alias)
defaultGeminiModel        = "gemini-2.5-flash"  // 유지 (최신)
```

→ ID-004 Phase 1 (OpenAI Codex capability 격차 해소) 와 함께 진행. 사용자 config 미설정 환경에서도 *"current best"* 모델 자동 사용.

### 영향 범위

- 변경: `internal/config/defaults.go` (5 줄)
- 사용자 영향: `tars.config.yaml` 에 명시 안 한 사용자만 영향. 명시한 사용자는 무관
- CHANGELOG: default model 변경 명시

### 관련

- [ID-004](../ideas.md#id-004) — provider capability 격차 해소 Phase 1 핵심 작업

---

## RF-065 — `yaml_paths.go` 200+ switch case — DRY 위반, 5 곳 동기화 부담

- **Status**: open
- **Location**: `internal/config/yaml_paths.go` (317줄, ~200 switch case), `internal/config/config_input_fields.go` (configInputFields slice)
- **Discovered in**: [journal/2026-04-25-26-config-pt2.md](../journal/2026-04-25-26-config-pt2.md)
- **Recommendation**: configInputField 에 `preferredPath string` 필드 추가 → yaml_paths.go 제거. 단일 source of truth.
- **Estimated effort**: M (sed automation)
- **Risk**: low

### 현재 상태

`preferredYAMLPathForKey(flatKey)` 가 200+ switch case. 새 config field 추가 시 **5 곳 동기화**:
1. `types.go` — sub-config field 정의
2. `defaults.go` — default constant
3. `defaults_apply.go` — apply function
4. `config_input_fields.go` — configInputFields entry
5. `yaml_paths.go` — preferredYAMLPathForKey case
6. `schema.go` — FieldMeta entry

→ **silent skew 위험**. 한 곳 빠뜨려도 코드 컴파일 통과 (yaml_paths.go 빠뜨리면 콘솔 UI nested 출력만 안 됨).

### 제안하는 변경

`configInputField` 에 `preferredPath` 필드 추가:

```go
type configInputField struct {
    yamlKey       string
    envKeys       []string
    preferredPath string  // ← 추가
    apply         func(*Config, string)
    merge         func(*Config, Config)
}
```

각 entry 정의 시 한 줄로 추가:
```go
stringField("workspace_dir", []string{"TARS_WORKSPACE_DIR"}, ..., "runtime.workspace_dir"),
```

→ `yaml_paths.go` 제거. `preferredYAMLPathForKey` 는 `configInputFieldByYAMLKey(key).preferredPath` 로 대체.

### 영향 범위

- 변경: `internal/config/config_input_fields.go` (180+ entry 에 path 추가, sed 가능) + `yaml_paths.go` 제거 + `schema.go` 의 `preferredYAMLPathForKey` 호출 갱신
- 동작 변화 없음 (기능 동일)
- 새 field 추가 부담: 5 → 4 곳

### 관련

- ID-005 Phase 3 (config 마이그레이션) 와 같은 카테고리

---

## RF-066 — `applyLLMPoolDefaults` 의 kind switch — 새 provider 추가 시 곳곳 반복

- **Status**: open
- **Location**: `internal/config/defaults_apply.go:318-396` (applyLLMPoolDefaults), `internal/llm/provider.go` (NewProvider switch)
- **Discovered in**: [journal/2026-04-25-26-config-pt2.md](../journal/2026-04-25-26-config-pt2.md)
- **Recommendation**: provider registry pattern — kind 별 metadata (default base URL, default api key env, oauth provider, auth mode default) 를 단일 table 로.
- **Estimated effort**: M
- **Risk**: medium (kind 처리 분산 정리)

### 현재 상태

새 provider 추가 시:
1. `defaults.go` — default constant (5+ entry)
2. `defaults_apply.go:applyLLMPoolDefaults` — kind switch 분기 (이 파일)
3. `defaults_apply.go:defaultOAuthProvider` — kind 매핑
4. `internal/llm/provider.go` — NewProvider switch
5. `internal/llm/<provider>_client.go` — 새 client 구현

→ **5 곳 반복**. silent skew 위험 (한 곳 빠뜨리면 default 못 채움 또는 NewProvider 실패).

### 제안하는 변경

```go
// internal/llm/registry.go 또는 internal/config/llm_registry.go
type providerKind struct {
    Name              string
    DefaultBaseURL    string
    DefaultAPIKeyEnv  []string  // env var fallback
    DefaultAuthMode   string
    DefaultOAuthProvider string
    NewClient         func(opts ProviderOptions) (Client, error)
}

var providerKinds = map[string]providerKind{
    "openai":          { ... },
    "openai-codex":    { ... },
    "anthropic":       { ... },
    "gemini":          { ... },
    "gemini-native":   { ... },
    "claude-code-cli": { ... },
}
```

→ 새 provider 추가 시 **1 entry**. applyLLMPoolDefaults 와 NewProvider 둘 다 이 registry 참조.

### 영향 범위

- 변경: `internal/llm/provider.go` (NewProvider switch → registry lookup) + `internal/config/defaults_apply.go` (applyLLMPoolDefaults 의 kind switch → registry lookup) + 새 registry 파일
- ID-004 와 결합 (새 provider 추가/capability 변경 시 registry 갱신)

### 관련

- ID-004 — OpenAI/Gemini capability 격차 해소 (registry 가 capability 메타도 보유 가능)
- ID-005 Phase 3 와 별개

---

## RF-067 — `cron.Store.load`/`save` O(N) full read+write — RF-058 카테고리 (4 사례 누적)

- **Status**: open
- **Location**: `internal/cron/store.go:447-509` (load + save), 모든 mutation method
- **Discovered in**: [journal/2026-04-25-27-cron.md](../journal/2026-04-25-27-cron.md)
- **Recommendation**: RF-017/021/058 와 같은 카테고리. **4 사례 누적** = 코드베이스 일관 안티패턴. SQLite 또는 per-row file 큰 결정.
- **Estimated effort**: L
- **Risk**: medium

### 현재 상태

매 mutation (Create/Update/Delete/MarkRunResult) 마다 `jobs.json` 전체 read + normalize + write.

### 누적 카테고리

| RF | 위치 | 패턴 |
|----|------|------|
| RF-017 | `entries.jsonl` (semantic.go) | O(N) load+rewrite |
| RF-021 | `KnowledgeStore.rebuildArtifacts` | O(N×N) batch |
| RF-058 | `sessions.json` (Store) | O(N) full read+write per mutation |
| **RF-067** | `cron/jobs.json` | 동일 |

→ **4 사례 누적** = 코드베이스 안티패턴. SQLite 마이그레이션 큰 결정 시점.

### 영향

cron job 수가 100+ 가는 환경 (자동화 사용자) 에서 매 update tick 마다 hot path 비용 누적.

---

## RF-068 — `cron.Store.save` atomic write 미적용 — RF-059 카테고리 (3 사례 누적)

- **Status**: open
- **Location**: `internal/cron/store.go:497-509` (save)
- **Discovered in**: [journal/2026-04-25-27-cron.md](../journal/2026-04-25-27-cron.md)
- **Recommendation**: gateway/persistence.go 의 `writeJSONAtomic` 패턴 적용. **공통 헬퍼** (`internal/atomicwrite/`) 추출 — RF-059 (session) + knowledge 와 함께 일괄.
- **Estimated effort**: S
- **Risk**: low

### 현재 상태

```go
return os.WriteFile(s.path, append(payload, '\n'), 0o644)  // ← partial write 가능
```

vs gateway/persistence.go: tmp + sync + rename.

### 누적 카테고리

| 파일 | atomic? |
|------|---------|
| `gateway/persistence.go` writeJSONAtomic | ✅ 모범 |
| `tool/write_file.go` writeTextFileAtomic | ✅ 모범 |
| `session/session.go` saveIndex | ❌ [RF-059] |
| `knowledge.go` buildKnowledgeDocument | ❌ |
| **`cron/store.go` save** | ❌ |

→ **3 사례 누적** (session/knowledge/cron). 공통 `internal/atomicwrite/` 패키지 추출 필요.

### 제안하는 변경

```go
// internal/atomicwrite/atomicwrite.go
func WriteJSON(path string, payload any) error { /* tmp + sync + rename */ }
```

→ session/knowledge/cron 모두 사용. 새로운 persistence 코드의 default.

---

## RF-069 — `cron/helpers.go` 의 자체 `min` 함수 — RF-019 카테고리

- **Status**: open
- **Location**: `internal/cron/helpers.go:64-69`
- **Discovered in**: [journal/2026-04-25-27-cron.md](../journal/2026-04-25-27-cron.md)
- **Recommendation**: Go 1.25 built-in `min` 사용. RF-019 (semantic.go + builder.go + list_dir.go) 와 함께 누적 정리.
- **Estimated effort**: trivial
- **Risk**: trivial

### 현재 상태

```go
func min(a, b int) int {
    if a < b { return a }
    return b
}
```

→ Go 1.25.6 환경에서 built-in `min` 사용 가능. RF-019 (4 사례) 와 함께 5번째.

### 영향 범위

`internal/cron/helpers.go` (6줄 삭제) + RF-019 본문에 5번째 사례 추가.

---

## RF-070 — `computeBackoffDuration` 의 magic number — config 화 검토

- **Status**: open
- **Location**: `internal/cron/helpers.go:36-54`
- **Discovered in**: [journal/2026-04-25-27-cron.md](../journal/2026-04-25-27-cron.md)
- **Recommendation**: `cron_backoff_base_seconds` + `cron_backoff_max_failures` + `cron_backoff_cap_hours` config 추가 또는 `documented const`.
- **Estimated effort**: S
- **Risk**: low

### 현재 상태

```go
base := 30 * time.Second           // hardcoded
multiplier := min(failures-1, 6)   // cap 6 → 64x
capDur := 12 * time.Hour           // hardcoded
```

→ 사용자가 backoff 정책 조정 불가. 다른 watchdog (pulse) 는 config 화 됐는데 cron 만 magic number.

### 제안하는 변경

옵션:
- **(a) config 추가**: `automation_cron_backoff_*` 3 field
- **(b) documented const**: 코드에 코멘트 + CLAUDE.md 명시. config 변경 불필요로 결정

→ (b) 권장 (현재 사용자 결정 백로그가 큼). cron backoff 가 production 이슈 만들지 않으면 magic number 수용.

---

## RF-071 — frontend component 1000+ 줄 5개 — 분해 후보

- **Status**: open
- **Location**: `frontend/console/src/components/{ChatPanel,ArtifactPanel,Ops,Config,MemoryCenter}.svelte`
- **Discovered in**: [journal/2026-04-25-28-frontend.md](../journal/2026-04-25-28-frontend.md)
- **Recommendation**: 각 component 를 100-300 줄 sub-component 로 분해. Svelte 5 ergonomic 향상.
- **Estimated effort**: M
- **Risk**: low

### 현재 상태

| Component | 줄 수 |
|-----------|------|
| ChatPanel | 1004 |
| ArtifactPanel | 1329 |
| Ops | 1074 |
| Config | 950 |
| MemoryCenter | 908 |

→ Single component 의 책임 폭 큼. 분해 후보:
- ChatPanel: input + history + token monitor
- ArtifactPanel: file upload + preview + download
- MemoryCenter: 3 탭 (durable/search/knowledge)

### 영향 범위

- 변경: 5 component → ~15 sub-component
- 사용자 영향: 없음 (UI 동일)
- ID-005 와 별개

---

## RF-072 — `frontend/console/src/lib/types.ts` 564줄 — 백엔드 mirror silent skew

- **Status**: open
- **Location**: `frontend/console/src/lib/types.ts`
- **Discovered in**: [journal/2026-04-25-28-frontend.md](../journal/2026-04-25-28-frontend.md)
- **Recommendation**: 백엔드 → TS type 자동 생성 (OpenAPI 또는 shared schema). 수동 동기화 부담 + silent skew 위험 제거.
- **Estimated effort**: L
- **Risk**: medium

### 현재 상태

`types.ts` 564줄 = Go struct 의 TypeScript mirror. 백엔드 변경 시 수동 동기화. **silent skew 위험** (백엔드가 새 필드 추가 → frontend 못 받음 → silent runtime error).

### 옵션

- **(a) OpenAPI spec**: 백엔드가 OpenAPI YAML export → frontend 가 자동 TS 생성
- **(b) Shared schema (JSON Schema/protobuf)**: 둘 다 같은 source
- **(c) 현 상태 유지**: 변경 빈도 낮으면 수동 OK

→ ID-005 (gateway→agentruntime) 진행 시 types.ts 도 동기화 필요. (a) 검토 가치.

---

## RF-073 — ID-005 영향 — frontend fetch URL 35+ 위치 마이그레이션

- **Status**: open (ID-005 Phase 4 핵심)
- **Location**: `frontend/console/src/lib/api.ts` 의 모든 `/v1/gateway/*` fetch
- **Discovered in**: [journal/2026-04-25-28-frontend.md](../journal/2026-04-25-28-frontend.md)
- **Recommendation**: ID-005 Phase 4 와 동시 마이그레이션. sed 자동화 가능.
- **Estimated effort**: S (sed)
- **Risk**: low

### 현재 상태

`api.ts` 에 `/v1/gateway/*` URL 35+ 위치 (gateway runs/status/reload/restart/reports + channels). ID-005 의 `/v1/agentruntime/*` 변경 시 frontend 도 동시.

### 자동화

```bash
sed -i '' 's|/v1/gateway/|/v1/agentruntime/|g' frontend/console/src/lib/api.ts
```

→ ID-005 Phase 4 본문에 추가.

### 관련

- [ID-005](../ideas.md#id-005) Phase 4 (HTTP API 변경)
- 콘솔 UI 페이지 라벨 (Gateway Status → Agent Runtime 등) 도 동시

---

## RF-018 — `normalizeSemanticTerms` 에 한국어 stopwords 추가

- **Status**: open
- **Location**: `internal/memory/semantic.go:548-579` (`normalizeSemanticTerms`)
- **Discovered in**: [journal/2026-04-25-09-memory-pt1.md](../journal/2026-04-25-09-memory-pt1.md)
- **Recommendation**: KR stopwords 추가 — 한국어 쿼리에서도 의미 없는 단어 필터링.
- **Estimated effort**: S
- **Risk**: low

### 현재 상태

`normalizeSemanticTerms` 의 stopwords가 EN-only:

```go
stopwords := map[string]struct{}{
    "the": {}, "a": {}, "an": {}, "and": {}, "or": {}, ...
    "prefer": {}, "preference": {}, "like": {}, "likes": {},
}
```

→ KR 쿼리("나의 선호도가 뭐였지?")는 stopword 필터 안 됨. CLAUDE.md *"30+ patterns (EN/KR)"* 다국어 지원 약속과 약간 어긋남.

흥미로운 비대칭:

- write side (`derivation.go`): "prefer/선호" → category 트리거
- read side (`normalizeSemanticTerms`): "prefer"는 stopword로 제외, "선호"는 stopword 아님 → KR 쿼리 시 의미 없는 키워드가 매칭 점수 부풀림

### 제안하는 변경

KR 조사/대명사 stopwords 추가:

```go
"나", "내", "너", "는", "은", "이", "가", "을", "를", "에", "에서",
"의", "도", "와", "과", "뭐", "뭐였지", "그", "이거", "저거",
"선호", "취향", "좋아",  // write 쪽과 대칭으로 stopword
```

추가로 한자/유니코드 punctuation도 normalize 검토.

### 영향 범위

- 변경: `internal/memory/semantic.go` (stopwords 맵 확장)
- 테스트: KR 쿼리 사례 추가
- 리스크 낮음 (검색 품질 개선, 회귀 거의 없음)

### prompt 패키지 사례 보강 (session 11)

`internal/prompt/memory_retrieval.go:373-378` 의 `normalizeRelevantTerms` 도 같은 카테고리 — EN-only stopwords:

```go
stopwords := map[string]struct{}{
    "the": {}, "a": {}, "an": {}, "and": {}, "or": {}, "to": {}, "of": {}, "in": {}, "on": {},
    "what": {}, "do": {}, "i": {}, "you": {}, "me": {}, "my": {}, ...
}
```

`scoreRelevantText`가 이 terms로 fallback chain의 모든 매처에 점수 매김 — **KR 쿼리는 조사("는/은/이/가") 까지 term으로 들어가서 매칭 점수 부풀림**. RF-018 적용 시 두 곳을 같은 stopwords map으로 통일 (또는 공통 헬퍼로 추출 — RF-015와 결합 가능).

---

<!--
템플릿:

---

<!--
템플릿:

## RF-001 — <한 줄 제목>

- **Status**: open | in-progress | resolved | deferred
- **Location**: `path/to/file.go:LL-LL`
- **Discovered in**: [journal/YYYY-MM-DD-NN-topic.md](../journal/YYYY-MM-DD-NN-topic.md)
- **Recommendation**: <권장 조치>
- **Estimated effort**: S | M | L
- **Risk**: low | medium | high

### 현재 상태
...

### 제안하는 변경
...

### 영향 범위
- 변경되는 파일:
- 테스트 영향:
- 외부 호환성:
-->
