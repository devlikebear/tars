# Tensions — 가이드라인 vs 현재 코드 충돌

새로 정착된 가이드라인(주로 CLAUDE.md)과 기존 코드 사이의 의도된/비의도된 불일치를 추적합니다. "잔재(dead code)"와는 다릅니다 — tension은 살아 있지만 방향이 어긋난 코드입니다.

---

## TN-001 — `browserplugin`이 "skill+CLI 우선" 가이드라인과 충돌

- **Status**: open
- **Location**: `internal/browserplugin/` (4 files, ~250 LOC) + 의존하는 `internal/browser/`, `/v1/browser/*` HTTP 라우트, 콘솔 UI
- **Discovered in**: [journal/2026-04-25-01-entrypoint.md](../journal/2026-04-25-01-entrypoint.md)
- **Recommendation**: 현 시점에는 **유지** — 의도된 "정당한 예외(legacy)"로 명시 분류. 다만 향후 브라우저 자동화 기능 확장 시 (1) 새 기능은 skill+CLI 패턴으로 추가하거나, (2) browserplugin 전체를 외부 스킬로 분리하는 마이그레이션 계획 수립 검토.

### 컨텍스트

PR #190 (2026-03-29) — "extract browser into built-in plugin system (Phase 2)"으로 도입. 그 전에는 `internal/browser` 코드가 코어에 박혀 있었고, 이를 "빌트인 플러그인" 형태로 분리한 것이 이 패키지.

이후 CLAUDE.md에 다음 가이드라인이 추가됨:

> **Do not add domain features as builtin Go plugins or MCP servers.** Every tool registered in `tool.Registry` emits its description into the chat system prompt at startup ...
>
> **Default pattern: skill (`.md`) + companion CLI, invoked via the builtin `bash` tool.**
>
> **When a builtin Go plugin IS appropriate**: infrastructure that every session needs regardless of skill choice (filesystem ops, web search, memory, gateway, telegram delivery)

### 충돌 지점

브라우저 자동화는 "every session needs"가 아니라 **도메인 기능**에 가깝다. 가이드라인 기준으로는 빌트인이 아니라 외부 skill+CLI로 빠져야 할 후보.

### 즉시 떼어내지 않는 이유

1. **결합 비용**: HTTP API(`/v1/browser/*`) + 콘솔 UI와 결합된 상태. 단순 CLI로 분리하려면 콘솔 페이지부터 외부화 필요.
2. **보안 의존성**: vault, OTP 등 시크릿 의존성을 갖고 있어 외부 CLI로 노출하기 위해서는 추가 IPC/권한 설계가 필요.
3. **시기**: 가이드라인이 PR #190 이후에 정착됨. 회고적으로 적용하기에 ROI가 낮음.

### 후속 행동 트리거

다음 중 하나가 발생하면 본격 마이그레이션 검토:

- 브라우저 자동화에 새 기능을 크게 추가해야 할 때
- `tool.Registry`에 등록된 browser 관련 도구 수가 늘어 시스템 프롬프트 무게가 체감 증가할 때
- 외부 `tars-skills` 레포가 안정화되어 보안 의존성을 안전하게 위임할 수 있는 인프라가 마련될 때

### 라이프사이클 종착점 (session 04에서 추적 완료)

플러그인의 등록 → init → start → close 흐름이 명확해짐:

```
[시점 1: import]      cmd/tars/main.go의 _ "internal/browserplugin"
                      → register.go의 init() → plugin.RegisterBuiltin(&Plugin{})
                      → 슬라이스에 zero-value 추가만

[시점 2: Init]        buildAPIMux L266의 buildExtensionsManager(...)
                      → 내부에서 plugin.BuiltinPlugins() 순회 + 각 plugin.Init(ctx) 호출
                      → (extensions/manager.go:106 initBuiltinPlugins)

[시점 3: HTTP 등록]   buildAPIMux L586의 extensionsManager.CollectHTTPHandlers()
                      → /v1/browser/*, /v1/vault/status 등 mux에 부착

[시점 4: Start]       startBackgrounds → L729 runtime.extensionsManager.Start(ctx)

[시점 5: Close]       shutdownRuntime → L777 runtime.extensionsManager.Close()
```

함정: 시점 1과 시점 2 사이에 `buildRuntimeDeps`(9단계) + `buildAPIMux` 초반(~250줄)이 끼어 있음. **"등록됐지만 비활성"인 시간 윈도우가 길다** — 디버깅 시 함정. 마이그레이션 시점에 윈도우를 좁히는 방안(예: 등록 시점에 lazy init 콜백 등록) 검토 가치 있음.

### 사용자 결정 (2026-04-25 session 05)

빌트인 플러그인 시스템 자체를 제거하는 방향으로 결정.

> "빌트인 툴은 성능과 보안, 그리고 유용성 측면에서 필요하지만 빌트인 플러그인은 너무 과하다."

→ TN-001은 [RF-007](../findings/refactor.md#rf-007)(빌트인 플러그인 시스템 전체 제거)로 흡수. browserplugin은 외부 MCP 서버 / sidecar 프로세스 / skill+CLI 중 하나로 마이그레이션. 세부는 RF-007 본문 참고.

---

## TN-003 — `tools_provider.script` 미구현 → "skill+CLI" 가이드라인 인프라 부재

- **Status**: open
- **Location**: `internal/extensions/manager.go:636-637` (collectToolProviderTools의 script 분기)
- **Discovered in**: [journal/2026-04-25-05-extensions-core.md](../journal/2026-04-25-05-extensions-core.md)
- **Recommendation**: RF-007 진행 시 함께 정리. CLAUDE.md 가이드라인이 권장하는 "skill+CLI" 패턴은 builtin `bash` tool이 외부 CLI를 호출하는 형태이므로 `tools_provider.script` 자체가 굳이 필요 없음 → 분기 제거 방향.

### 컨텍스트

`collectToolProviderTools`가 분기하는 4개 타입 중 2개가 미구현:

| Type | 상태 |
|------|------|
| `mcp_server` | 처리됨 (MCPRuntime이 담당) |
| `go_plugin/builtin:` | 처리됨 (빌트인 플러그인) |
| `go_plugin/(기타)` | 미지원 (diagnostic만) |
| `script` | 미지원 (diagnostic만) |

CLAUDE.md의 명확한 권장:
> Default pattern: skill (`.md`) + companion CLI, invoked via the builtin `bash` tool.

즉 새 도구는 `tools_provider`를 등록하지 말고 skill body가 bash로 CLI를 호출하라는 권장. 그렇다면 `script` type은 굳이 구현할 필요 없음.

### RF-007과의 관계

RF-007 진행 시:

- `go_plugin/builtin:` 분기 사라짐 (빌트인 플러그인 자체 제거)
- `script` 분기는 가이드라인상 불필요
- → `collectToolProviderTools`는 사실상 `mcp_server` 한 분기만 남음 → MCPRuntime으로 직접 위임 가능 → 함수 자체 삭제 가능

---

## TN-002 — `cmd/tars` ↔ `tarsserver` 사이의 플래그 정의 이중화

- **Status**: open
- **Location**: `cmd/tars/server_main.go:52-61` ↔ `internal/tarsserver/main_cli.go:126-134`
- **Discovered in**: [journal/2026-04-25-02-server-bootstrap.md](../journal/2026-04-25-02-server-bootstrap.md)
- **Recommendation**: **방향 2** — `tarsserver`에서 cobra 제거, `Serve`를 순수 라이브러리 함수로 단순화. args 파싱 의존 테스트는 `cmd/tars/`로 이동.

### 컨텍스트

`cmd/tars/server_main.go`의 `newServeCommand`와 `internal/tarsserver/main_cli.go`의 `newRootCmd`가 **동일한 9개 플래그를 각자 정의**한다 (`--config`, `--mode`, `--workspace-dir`, `--log-file`, `--verbose`, `--run-once`, `--run-loop`, `--serve-api`, `--api-addr`).

이중 cobra 자체는 잔재가 아니라 **테스트 가능성을 위한 의도된 분리**다 (`main_test.go:36-63`의 `run()` 헬퍼가 `cmd.SetArgs(args); cmd.Execute()` 형태로 CLI 시나리오를 직접 시뮬레이션).

진짜 문제는 플래그 정의가 두 곳에서 갈라진다는 것:

- 프로덕션 경로: `cmd/tars` cobra만 의미 있음 (사용자 args 파싱). `Serve` 진입 후의 두 번째 cobra는 빈 args로 실행 → 거기 정의된 플래그는 죽은 코드.
- 테스트 경로: 두 번째 cobra만 의미 있음.
- 한쪽만 추가/제거하면 silent skew.

### 두 갈래 방향 비교

**방향 1**: 플래그 정의 함수 `bindServeFlags(*cobra.Command, *options)`를 `tarsserver`에 두고 양쪽이 import. 두 cobra 모두 같은 정의 공유.

**방향 2 (선택됨)**: `tarsserver`에서 cobra 자체를 제거. `Serve`는 순수 라이브러리 함수로 단순화. args 파싱 의존 테스트(`TestRun_FlagOverridesEnvAndYAML`, `TestRun_HelpReturnsZero`, `TestRun_MutuallyExclusiveRunFlags`, `TestRun_InvalidConfigPathReturnsError`, `TestRun_UsesEnvConfigPathWhenFlagIsEmpty` 등)는 `cmd/tars/`로 이동.

### 영향 범위

- 변경: `internal/tarsserver/main.go`, `main_cli.go`, `main_test.go` + `cmd/tars/server_main.go`, `cmd/tars/main_test.go`
- `tarsserver.Serve` 시그니처 유지 가능 (이미 `ServeOptions` struct로 받음 → 내부에서 cobra만 제거)
- 테스트 ~6개 이전
- Estimated effort: M
- Risk: low (외부 호환성 영향 없음)
