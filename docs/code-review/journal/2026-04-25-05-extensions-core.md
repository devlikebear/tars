---
date: 2026-04-25
session: 05
scope: internal/extensions/manager.go (L1-175 + L587-650), internal/extensions/lifecycle.go (전체)
next: internal/extensions/manager.go의 Reload(L175-319) — snapshot 갱신 핵심
findings: [RF-007, RF-008, RF-009, TN-003, TN-001 update]
---

# Session 05 — Extensions 코어: 라이프사이클 + 큰 결정 4개

## 다룬 파일

- `internal/extensions/manager.go` — 상위 175줄(타입+Start/Close) + L587-650(initBuiltinPlugins, collectToolProviderTools, CollectHTTPHandlers)
- `internal/extensions/lifecycle.go` — 56줄 전체

## 흐름 요약

### `extensions.Manager` 구조

- **`Options`** (13필드): workspace, skill/plugin 활성 플래그, 3계층 source dirs (workspace/user/bundled), MCP base servers + Runtime, watch 옵션, **`PluginConfig map[string]map[string]any`** (per-plugin config keyed by plugin ID).
- **`Snapshot`**: 평탄 외부 노출 struct (Version, Skills, Plugins, SkillPrompt, MCPServers, Diagnostics).
- **`Manager`**: snapshot + chatTools + atomic version + disabledStore + fsnotify watcher.

### 라이프사이클 4단계 (`Start(ctx)`, L104-149)

```
1. initBuiltinPlugins(ctx)         ← BuiltinPlugin 인터페이스의 Init(ctx) 호출
2. m.Reload(ctx)                   ← skill/plugin 디스크 로드 + snapshot 갱신
3. runLifecycleHooks(plugins, "on_start", 0)  ← 플러그인 정의의 on_start 훅 (sh -c)
4. (옵션) fsnotify watcher → watchLoop goroutine
```

`Close()`: `on_stop` → 모든 built-in `Close()` → watcher 정리.

### 핵심 사실 — Init vs on_start 분리의 정체

- `Init(ctx)` Go 메서드 = **built-in plugin** (browserplugin 같은 컴파일 인 코드)
- `on_start` shell 훅 = **external plugin** (디스크에 install된 정의 파일)

빌트인은 `plugin.Definition.Lifecycle` 필드를 비워두므로 훅 단계는 자동 스킵. 두 매커니즘이 의도적으로 분리됨.

### `initBuiltinPlugins` (L587-603) — 16줄

```go
for _, bp := range plugin.BuiltinPlugins() {
    cfg := m.opts.PluginConfig[bp.ID()]
    if cfg == nil { cfg = map[string]any{} }
    pctx := plugin.PluginContext{Config: cfg, WorkspaceDir: m.opts.WorkspaceDir}
    if err := bp.Init(pctx); err != nil {
        diagnostics = append(diagnostics, ...)
    }
}
```

browserplugin이 `browser_runtime_enabled`, `vault_status` 등을 받는 정확한 지점.

### `collectToolProviderTools` (L605-641) — 큰 단서

`ToolsProvider.Type` 4분기:

| Type | 처리 |
|------|------|
| `mcp_server` | 스킵 (MCPRuntime 처리) |
| `go_plugin/builtin:` | 빌트인에서 tools 가져옴 |
| `go_plugin/(기타)` | **미지원** (diagnostic) |
| `script` | **미지원** (diagnostic) |

→ `script` 분기 미구현. CLAUDE.md "skill+CLI" 권장 패턴은 builtin `bash` tool로 호출이라 굳이 필요 없음. → [TN-003](../findings/tensions.md#tn-003).

### `CollectHTTPHandlers` (L644-650) — 4줄

빌트인만 HTTP 핸들러 노출. 외부 플러그인은 라우트 못 박음. 의도된 격리인지 불명확. → [RF-009](../findings/refactor.md#rf-009).

### `lifecycle.go` (전체) — 보안 표면 발견

```go
c := exec.CommandContext(hookCtx, "sh", "-c", cmd)
c.Dir = p.RootDir
```

플러그인 정의의 `lifecycle.on_start` / `on_stop` 문자열 = shell 명령. 외부 install 경로의 공급망 위협. → [RF-008](../findings/refactor.md#rf-008).

특이점:

- 30초 default timeout, 실패는 비치명적 (diagnostics 누적)
- `stderr`만 캡처, **`stdout`은 버림** (minor — 디버깅 보강 시 stdout도 포함하면 좋음)

## 사용자 결정 — 4개 큰 finding 등록

이 세션의 핵심은 코드보다 **사용자가 내린 4개 결정**:

### 1. RF-007: 빌트인 플러그인 시스템 자체 제거

> "빌트인 툴은 성능과 보안, 유용성 측면에서 필요하지만 빌트인 플러그인은 너무 과하다."

→ TN-001(browserplugin)을 RF-007로 흡수. browserplugin은 외부 MCP 서버 / sidecar / skill+CLI 중 하나로 마이그레이션. 2단계 작업: (1) browserplugin 재배치 → (2) 시스템 제거.

### 2. RF-008: lifecycle hook의 sh -c 제거

> "플러그인의 훅 명령어를 sh로 실행하는건 너무 치명적이야."

대체 옵션 (사용자 제안):

- (a) TARS 빌트인 툴만 hook으로 호출 가능 (단 `bash` 툴은 훅 화이트리스트에서 제외)
- (b) 빌트인 툴 + MCP tools 허용

→ 사전 마이그레이션 조사 필요(어떤 외부 플러그인이 sh 훅 사용 중인지).

### 3. RF-009: 외부 플러그인 HTTP 핸들러 노출 정책 (보류)

> "보안문제 때문에 고민이되네."

옵션 4개 비교 (격리 vs ergonomics):

- (a) 외부는 HTTP 노출 불가 (현 상태 유지)
- (b) `/v1/plugins/<id>/*` sub-prefix sandboxing **(현실적 절충)**
- (c) manifest 검증 + UAC
- (d) MCP resource serving 우회

→ RF-007 진행 시점에 함께 결정.

### 4. TN-003: `tools_provider.script` 미구현 — 가이드라인 인프라 부재

CLAUDE.md "skill+CLI" 권장의 인프라가 미완. 다만 권장은 builtin `bash` tool로 외부 CLI 호출이라 `script` type 자체가 굳이 필요 없음 → RF-007과 함께 분기 제거 방향.

## 새로 등록된 findings

- [RF-007](../findings/refactor.md#rf-007) — 빌트인 플러그인 시스템 자체 제거 (TN-001 흡수)
- [RF-008](../findings/refactor.md#rf-008) — Lifecycle 훅 sh -c 제거 + 안전 표면 대체
- [RF-009](../findings/refactor.md#rf-009) — 외부 플러그인 HTTP 노출 정책 (결정 보류)
- [TN-003](../findings/tensions.md#tn-003) — `tools_provider.script` 미구현

## 업데이트된 findings

- [TN-001](../findings/tensions.md#tn-001) — "사용자 결정" 섹션 추가, RF-007로 흡수 명시

## 다음 세션 진입점

`internal/extensions/manager.go`의 **`Reload(ctx)`** (L175-319, 144줄) — snapshot 갱신 핵심.

부수 추적:

- skill/plugin 디스크 로드 메커니즘 (workspace/user/bundled 3계층)
- MCP server 머지 (`mergeMCPServers`)
- skill source 정렬 (`sortSkillSources`, `sourceRank`)
- diagnostics 수집 패턴
