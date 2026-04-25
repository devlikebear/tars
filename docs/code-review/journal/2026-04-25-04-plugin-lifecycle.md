---
date: 2026-04-25
session: 04
scope: internal/tarsserver/main_serve_api.go (부분, 핀포인트), main_session.go, internal/plugin/builtin_registry.go (grep), internal/extensions/manager.go (grep)
next: internal/extensions/manager.go — 플러그인 라이프사이클 owner. TN-001 후속 작업의 베이스.
findings: [RF-005, RF-006, Q-004, TN-001 update]
---

# Session 04 — `runServeAPICommand` 핀포인트 탐색 + 플러그인 라이프사이클 종착

## 다룬 파일

- `internal/tarsserver/main_serve_api.go` — `runServeAPICommand`(L74-111) 정밀 read, `buildAPIMux` L113-220 정밀 read, 나머지는 grep으로 핀포인트
- `internal/tarsserver/main_session.go` — `resolveMainSessionID` 확인
- `internal/plugin/builtin_registry.go` — 등록 슬라이스 확인
- `internal/extensions/manager.go` — `initBuiltinPlugins` 위치만 확인 (다음 세션 본격 검토)

## 흐름 요약

### `runServeAPICommand` (L74-111) — 깔끔한 라이프사이클 wrapper

5단계만:

1. `buildAPIMux` → 모든 의존성 짜인 `serveAPIRuntime` 받음
2. `signal.NotifyContext(SIGINT, SIGTERM)` — graceful shutdown 트리거
3. `startBackgrounds` — 백그라운드 러닝 시작
4. `ctx.Done()` 감지 시 5초 timeout으로 `shutdownRuntime` 실행하는 goroutine
5. `server.ListenAndServe()` — blocking, `http.ErrServerClosed`만 정상 종료

깔끔. 짚을 점 없음.

### `serveAPIRuntime` (L32-45) — 11개 필드의 "실행 중인 서버 상태"

```go
cfg, configPath, mainSessionID
server (*http.Server)
extensionsManager
gatewayRuntime
gatewayAgentsWatch    ← 에이전트 정의 파일 감시
cronManager
watchdogManager       ← pulseRuntime과 별개 (→ Q-004)
pulseRuntime
reflectionRuntime
telegramPoller
```

### `apiRouteHandlers` (L47-72) — 23개 핸들러 그룹

console / chat / sessions / memory / ops / status / auth / healthz / providersModels / compact / cron / mcp / extensions / agentRuns / gateway / channels / events / config / skillhub / filesystem / workspaceFiles / pulse / reflection.

### `buildAPIMux` 핀포인트 결과

| 라인 | 컴포넌트 | 비고 |
|------|---------|------|
| 154-158 | `deps.llmClient` 타입 단언 → notifier 박기 | RF-004 첫 사례 |
| 163-168 | `telegramDeliveryCounter` 생성, pulse wiring 미완 | → RF-006 |
| 172-220 | `gatewayRuntimeForTelegram` late-binding 패턴 | 순환 의존, nil-check로 막아둠 |
| 257 | `mcpClient := mcp.NewClient(cfg.MCPServers)` | MCP 클라이언트 init |
| 266 | `extensionsManager, err := buildExtensionsManager(cfg, mcpClient, configMap)` | **플러그인 Init 트리거** |
| 272 | `gatewayRuntime := gateway.NewRuntime(...)` | gateway 본체 init |
| 323 | `gatewayRuntimeForTelegram = gatewayRuntime` | late-binding 해소 |
| 586 | `extensionsManager.CollectHTTPHandlers()` | 플러그인 HTTP 라우트 수집 |
| 728-729 | `runtime.extensionsManager.Start(ctx)` | 백그라운드 작업 시작 |
| 776-777 | `runtime.extensionsManager.Close()` | shutdown 정리 |

## 핵심 분석

### 🎯 TN-001 추적 종착 — 플러그인 라이프사이클 5단계

```
[1] import 시점
    cmd/tars/main.go의 _ "internal/browserplugin"
    → register.go init()
    → plugin.RegisterBuiltin(&Plugin{})  (zero-value 슬라이스 추가)

[2] Init 시점 (buildAPIMux L266)
    buildExtensionsManager(cfg, mcpClient, configMap)
    → plugin.BuiltinPlugins() 순회
    → 각 plugin.Init(ctx) 호출 (extensions/manager.go:106 initBuiltinPlugins)

[3] HTTP 등록 (buildAPIMux L586)
    extensionsManager.CollectHTTPHandlers()
    → /v1/browser/*, /v1/vault/status 등 mux에 부착

[4] Start (startBackgrounds L729)
    runtime.extensionsManager.Start(ctx)

[5] Close (shutdownRuntime L777)
    runtime.extensionsManager.Close()
```

→ TN-001 본문에 "라이프사이클 종착점" 섹션 추가.

핵심 함정: **시점 [1]과 [2] 사이가 매우 길다**. 중간에 `buildRuntimeDeps`(9단계) + `buildAPIMux` 초반(~250줄)이 끼어 있음. "등록됐지만 비활성" 윈도우. 디버깅 시 가시성 부족 가능. 마이그레이션 시 윈도우를 좁히는 방안 검토 가치 있음.

### Q-003 → resolved → RF-005 승격

`stage: "daily_log"`를 만드는 코드 0건 (전체 `internal/` grep). `main_cli.go:95-96`의 case는 dead branch → RF-005로 즉시 제거 가능.

### RF-006 신규 — telegram pulse wiring 미완

L163-168의 `_ = telegramDeliveryCounter // retained for pulse wiring in a later commit` — CLAUDE.md가 약속한 `DeliveryFailureCounter` 인터페이스 wiring이 미완. 가이드라인 vs 구현 gap.

### Q-004 신규 — `pulseRuntime` vs `watchdogManager` 역할 차이

CLAUDE.md는 pulse를 워치독으로 묘사하는데 코드엔 별개 필드가 둘. 가설 3개:

- (a) cron job 실행 단위 워치독 (job-level)
- (b) workspace 별 헬스 체커
- (c) pulse 도입 후 일부 책임만 남은 잔재

→ pulse / `workspaceWatchdogManager` 검토 시 답변.

## 새로 등록된 findings

- [RF-005](../findings/refactor.md#rf-005) — `daily_log` 좀비 case 라벨 제거
- [RF-006](../findings/refactor.md#rf-006) — `telegramDeliveryCounter` pulse wiring 완성
- [Q-004](../findings/questions.md#q-004) — `pulseRuntime` vs `watchdogManager` 역할 차이

## 업데이트된 findings

- [TN-001](../findings/tensions.md#tn-001) — "라이프사이클 종착점" 섹션 추가
- [Q-003](../findings/questions.md#q-003) — Status: resolved + 답변 추가, RF-005로 승격

## 다음 세션 진입점

`internal/extensions/manager.go` — 플러그인 + MCP + skill 통합 owner. TN-001/TN-002 후속 작업의 베이스가 될 코드.

부수 추적:

- `initBuiltinPlugins` 본체 (시점 [2] 정확한 코드)
- `Start(ctx)` 가 무슨 백그라운드 작업을 시작하는지
- `CollectHTTPHandlers` 의 라우트 수집 메커니즘
- MCP 클라이언트와 어떻게 통합되는지 (`buildExtensionsManager`가 받는 `mcpClient`)
