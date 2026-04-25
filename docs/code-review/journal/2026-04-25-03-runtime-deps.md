---
date: 2026-04-25
session: 03
scope: internal/tarsserver/main_bootstrap.go
next: internal/tarsserver/main_serve_api.go (runServeAPICommand) — HTTP 서버 + 라우트 등록 + 플러그인 init
findings: [RF-003, RF-004, Q-002, Q-003]
---

# Session 03 — `buildRuntimeDeps`: 진짜 부트스트랩

## 다룬 파일

- `internal/tarsserver/main_bootstrap.go` (163줄)

## 흐름 요약

`buildRuntimeDeps(opts, nowFn, logger)` — 9단계 부트스트랩이 한 함수에 압축.

| 단계 | 코드 | 비고 |
|------|------|------|
| 1. config 경로 해석 | `config.ResolveConfigPath` | |
| 2. config 로드 | `config.Load` | stage: `load_config` |
| 3. CLI 플래그 우선 적용 | `cfg.Mode`, `cfg.WorkspaceDir` 덮어쓰기 | `--mode`, `--workspace-dir`만 처리 |
| 4. API 보안 검증 | `validateAPIAuthSecurity` | stage: `validate_config` |
| 5. workspace 보장 | `memory.EnsureWorkspace` | stage: `ensure_workspace` |
| 6. session store | `session.NewStore` × 2 | 잉여 초기화 (→ RF-003) |
| 7. usage tracker | 가격 오버라이드 + 한도 적용 | stage: `init_usage` |
| 8. **분기** | `if !opts.ServeAPI { return deps, nil }` | LLM 없이 조기 종료 가능 (→ Q-002) |
| 9. LLM router + memory + agent runner | `buildLLMRouter` 등 | stage: `init_llm`, `init_memory_backend`, `init_semantic_memory` |

## 핵심 관찰

### `validateAPIAuthSecurity` (L150-162) — 좋은 보안 게이트

`api_auth_mode=off` 또는 `external-required` 사용 시 `api_allow_insecure_local_auth=true` 명시 동의 강제. 사고 방지 패턴.

### `runtimeDepsError` 패턴

자체 에러 타입에 `stage` 문자열을 박아 호출자가 분류 가능 (`load_config`, `validate_config`, `ensure_workspace`, `init_usage`, `init_llm`, `init_memory_backend`, `init_semantic_memory`). `Unwrap()` 도 구현되어 `errors.Is/As` 호환.

다만 `main_cli.go`의 에러 분류 switch에 `case "daily_log":` 가 있는데 본 함수에는 해당 stage 발생 지점이 없음 → [Q-003](../findings/questions.md#q-003).

### 함수 책임의 비대화 — RF가 아닌 이유

100줄에 9단계는 큰 함수지만:

- 각 단계가 stage 라벨로 명확히 구분됨
- 의존 순서가 강함 (config → workspace → store → tracker → LLM)
- 결과물이 평탄한 struct (`runtimeDeps`) 하나
- 쪼개도 호출자가 같은 순서로 다시 부르게 될 가능성

→ **의도적으로 한 함수에 모은 패턴**. 인정할 만한 선택, RF 아님.

다만 `runtimeDeps` struct 자체는 8필드 + 일부 함수 포인터로 다소 비대. 향후 더 늘면 분해 후보.

### `llmClient` 필드의 자백 (L21-24)

코드 코멘트가 명시적으로 부채를 인정:

> `llmClient is the chat-main tier client, kept for backward compat with call sites that have not yet been migrated to llmRouter.`

Router 도입은 완료, 호출처 마이그레이션은 미완 → [RF-004](../findings/refactor.md#rf-004).

### TN-001 단서는 여기 없음

원래 흐름을 따라온 이유 중 하나는 "**플러그인 init이 언제 일어나는가**"였는데, `buildRuntimeDeps`엔 플러그인 초기화가 없음. → 다음 단계 `runServeAPICommand`에서 만날 가능성.

## 새로 등록된 findings

- [RF-003](../findings/refactor.md#rf-003) — `sessionStoreResolver` 잉여 1차 초기화
- [RF-004](../findings/refactor.md#rf-004) — `runtimeDeps.llmClient` backward-compat 필드 마이그레이션 완료
- [Q-002](../findings/questions.md#q-002) — `--serve-api=false` 모드의 의도
- [Q-003](../findings/questions.md#q-003) — `daily_log` stage 라벨이 정의되지 않은 곳에서 사용됨

## 다음 세션 진입점

`internal/tarsserver/main_serve_api.go` — `runServeAPICommand`.

- HTTP 서버 시작 + 라우트 등록 (`registerAPIRoutes`)
- **플러그인 init 발생 지점** (TN-001 추적의 종착점)
- 미들웨어 체인 (auth, logging 등)
- 콘솔(SPA) 서빙 분기 (embedded vs Vite proxy)

부수 추적:

- `daily_log` stage 발생 지점 (Q-003 해소)
- `--serve-api=false` 와 `tars doctor`의 책임 비교 (Q-002 해소)
