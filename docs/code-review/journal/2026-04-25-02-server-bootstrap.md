---
date: 2026-04-25
session: 02
scope: cmd/tars/server_main.go, internal/tarsserver/main.go, main_cli.go, main_test.go(부분)
next: internal/tarsserver/main_bootstrap.go (buildRuntimeDeps) — 진짜 부트스트랩
findings: [TN-002, RF-001, RF-002, Q-001]
---

# Session 02 — `tars serve` 명령의 실제 부트스트랩 추적

## 다룬 파일

- `cmd/tars/server_main.go` (77줄)
- `internal/tarsserver/main.go` (150줄)
- `internal/tarsserver/main_cli.go` (138줄)
- `internal/tarsserver/main_test.go` 일부 (`run` 헬퍼 36-63줄)

## 흐름 요약

### 1. `cmd/tars/server_main.go` — 얇은 위임 wrapper

`newServeCommand`는 9개 플래그 정의 + `serveOptions` 구조체 채움 → `tarsserver.Serve(ctx, ServeOptions{...})` 호출이 전부.

특이점:

- `serveRunner = runServeCommand` (var) — 테스트가 함수 주입으로 가로챌 수 있게 한 hook.
- `--run-once`/`--run-loop` mutex 체크 + deprecation warning 존재 → [RF-001](../findings/refactor.md#rf-001).

### 2. `internal/tarsserver/main.go` — 두 번째 cobra 시작점

`Serve(ctx, ServeOptions, stdout, stderr)`:

1. `envloader.Load(".env", ".env.secret")` — **중복 호출** → [Q-001](../findings/questions.md#q-001).
2. `ServeOptions` → 내부 `*options` 변환 + `applyOptionDefaults`.
3. `setupRuntimeLogger` 첫 번째 호출 + `defer cleanup()`.
4. `newRootCmd(opts, ...)` 생성 → `cmd.SetArgs([]string{}); cmd.Execute()` — Cobra를 함수처럼 사용.

특이점:

- `applyOptionDefaults`가 매우 빈약 (`APIAddr`만 처리) — 다른 default는 어디서 적용되는지 추적 필요 (다음 세션 과제).
- `setupRuntimeLogger`는 lumberjack으로 회전 로그 처리. 기본값 100MB / 30days / 5backups.

### 3. `internal/tarsserver/main_cli.go` — 두 번째 cobra의 본체

`newRootCmd` RunE:

1. `--run-once`/`--run-loop` mutex 체크 (server_main.go에서 이미 했는데 또 함 — RF-001).
2. **`buildRuntimeDeps(opts, nowFn, logger)`** ← **다음 세션의 핵심**.
3. config 값으로 logger 재구성 — 두 번째 `setupRuntimeLogger` 호출, cleanup 누수 → [RF-002](../findings/refactor.md#rf-002).
4. `runtimeDepsError` 분류로 stage별 에러 로깅 (`load_config`, `ensure_workspace`, `daily_log`, `init_llm` 등).
5. `opts.ServeAPI`이면 `runServeAPICommand` 호출.

플래그 정의 9개가 server_main.go와 **완전 중복** → [TN-002](../findings/tensions.md#tn-002).

## 핵심 분석 — 이중 Cobra의 정체

추적 결과:

- `tarsserver.Serve` 호출처: `cmd/tars/server_main.go` 단 1곳 (worktree들은 사본).
- `newRootCmd` 호출처: `Serve` (main.go:46) + `main_test.go:47` (테스트의 `run()` 헬퍼).

`run()` 헬퍼는 `args []string`을 받아 `cmd.SetArgs(args); cmd.Execute()` 형태로 CLI 시나리오를 직접 시뮬레이션함:

- `TestRun_FlagOverridesEnvAndYAML` — 플래그가 env/YAML보다 우선하는지
- `TestRun_HelpReturnsZero` — `--help` 처리 (exit 0)
- `TestRun_MutuallyExclusiveRunFlags` — `--run-once --run-loop` mutex
- `TestRun_InvalidConfigPathReturnsError` — 잘못된 플래그 값
- `TestRun_UsesEnvConfigPathWhenFlagIsEmpty` — env vs 플래그 폴백

**결론**: 이중 cobra 자체는 잔재가 아니라 **테스트 가능성을 위한 의도된 분리**. 진짜 문제는 **플래그 정의가 두 곳에서 갈라지는 것** (silent skew 위험).

### 두 갈래 방향 + 결정

- **방향 1**: 플래그 정의 함수 `bindServeFlags`를 한 곳에 두고 양쪽 cobra가 공유.
- **방향 2 (선택)**: `tarsserver`에서 cobra 제거 → `Serve`를 순수 라이브러리 함수로. args 파싱 테스트는 `cmd/tars/`로 이전.

방향 2가 책임 분리 측면에서 더 클린. 비용은 테스트 ~6개 이전, 다만 `cmd/tars/main_test.go`가 이미 존재하므로 인프라 일부 마련됨. → TN-002에 권장 방향으로 명시.

## 새로 등록된 findings

- [TN-002](../findings/tensions.md#tn-002) — 플래그 정의 이중화 (방향 2 권장)
- [RF-001](../findings/refactor.md#rf-001) — deprecated `--run-once` / `--run-loop` 완전 제거
- [RF-002](../findings/refactor.md#rf-002) — `setupRuntimeLogger` 이중 호출의 cleanup 누수
- [Q-001](../findings/questions.md#q-001) — `envloader.Load` 중복 호출 의도 (envloader 리뷰 시 해소)

## 다음 세션 진입점

`internal/tarsserver/main_bootstrap.go` — `buildRuntimeDeps` 함수.

config 로드, workspace 보장, daily log, llm 초기화 등 **진짜 부트스트랩**이 여기서 일어남. **플러그인 init 시점도 여기서 만날 가능성 높음** (TN-001 추적의 출발점이었음).

부수적으로 추적할 것:

- `applyOptionDefaults`가 빈약한 이유 — 나머지 default가 config 단에서 적용되는지, buildRuntimeDeps 단에서 적용되는지.
- `runtimeDepsError`의 stage 문자열 카탈로그.
