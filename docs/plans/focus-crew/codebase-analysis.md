# Codebase Analysis — Focus × Crew

분석일: 2026-10-15 · 기준 커밋: `ece82981` (main) · Focus area: Focus mode 서버/콘솔, 워커 세션, LLM tier, 도구, 알림, 메모리

> 이 문서는 `docs/plans/focus-crew/` 계획서들의 공통 참조 자료다. 각 phase 문서는 여기 적힌 파일·패턴을 전제로 쓰였다.

## 1. 프로젝트 개요

- Go 서버(`cmd/tars`, `internal/tarsserver`) + Svelte 콘솔(`frontend/console`) + macOS 데스크톱 셸(`desktop/`).
- 빌드·검증은 **항상 Makefile** (`CLAUDE.md`): `make build`, `make test`, `make test-one TEST_NAME=… PKG=…`, `make lint-diff`, `make test-diff`, `make arch-check`, `make console-e2e`.
- 콘솔: `cd frontend/console && npm run check` (svelte-check + tsc), `npm run test:ci`, 단위 테스트는 `node --experimental-strip-types --test tests/<name>.test.ts`.
- 작업 방식: `tars-github-flow` 스킬 (worktree → TDD → PR → CI → merge).

## 2. Focus mode 현재 구조 (유지 대상)

결정 문서: `docs/decisions/focus-mode.md` (ADR). 핵심 규칙:

- 1 pipeline = 1 main chat session. 원본 transcript는 접힐 뿐 지워지지 않음.
- 단계: plan → build ↻ → review ↻ → pr → pr_review ↻ → merge (+ build-kind release). 템플릿이 stage 목록을 바꿔도 **kind는 6종 고정**.
- 게이트: G1 plan, G2 triage, G3 PR, G4 merge, blocked.
- **단계 전이는 사실로만** 결정: 블록 파싱, 게이트 액션, verification exit code, `gh pr view` probe. 모델에게 "끝났냐"고 묻지 않는다.
- 카드 우선순위: gate > decision > finding > failure > report > change (+ notice). 한 번에 한 장, ←/→, 1–9, `?`, `o`.
- Q&A: 파이프라인당 숨은 `worker` 세션 1개, permission `plan`(읽기 전용), 카드 맥락을 console context로 전달.
- Goal mode: `focuspipeline.NextGoalStep` 고정 정책이 게이트를 대신 처리. push 예산.

### 서버 파일 지도

| 파일 | 역할 |
|---|---|
| `internal/focuspipeline/model.go` | `Pipeline`, `Stage`, `Card`, `Plan`, 상수(Stage/Gate/Card kind) |
| `internal/focuspipeline/machine.go` | `Apply(p, Event, now) (Pipeline, Action, error)` 순수 상태머신. Action = `send_turn` / `run_verification` / `none` |
| `internal/focuspipeline/blocks.go` | `<focus-plan|report|findings|pr|plan-edit>` 파서, `StripBlocks` |
| `internal/focuspipeline/guidance.go` | 매 턴에 붙는 `<focus-stage>` 지시문 (`Guidance`, `stageInstruction`, `writePlan`) |
| `internal/focuspipeline/finding_add.go` | `AddFinding(p, f, decision, now)` — 외부에서 finding 카드 추가하는 유일한 입구. `FindingSourceDeveloper` |
| `internal/focuspipeline/template.go` | `Template`, `Validate`, `LoadTemplates(dir)`, `SaveTemplate`, `<workspace>/focus-templates/*.yaml` |
| `internal/focuspipeline/store.go` | `<workspace>/sessions/<id>.pipeline.json`, `Update(id, fn)` 락 갱신 |
| `internal/focuspipeline/goal.go` | Goal mode 정책 |
| `internal/tarsserver/focus_pipeline.go` | `/v1/focus/*` 라우트, `focusAPI` |
| `internal/tarsserver/focus_driver.go` | `focusDriver`: `start`, `loop`, `runTurn`, `qaTurn`, `verify`. 턴 실행·검증 실행의 유일한 주체 |
| `internal/tarsserver/focus_qa.go` | **봇 세션의 모방 대상**: `qaSession()`(worker 생성, isolation off, plan 권한, workdir 동기화), `driver.ask()`(고루틴 + release) |
| `internal/tarsserver/focus_template_edit.go` | **크루 편성 LLM 호출의 모방 대상**: decision-only 호출(`ToolChoice: none`, 빈 harness tool list, `permission_mode: plan`), 검증 실패 시 1회 재시도, 실패 2회면 400 |
| `internal/tarsserver/focus_review.go`, `focus_finding.go` | review 라운드, finding 추가 API |
| `internal/tarsserver/focus_goal.go` | goal watcher (2초 주기) |

### 워커 세션으로 "봇"을 만들 수 있는 근거

`pkg/session` Store에 이미 있는 setter:

- `CreateWithOptions(title, "worker", hidden)` — 숨은 세션
- `SetTierPin(id, tier)` — 엔진 고정 (tier → `llm_providers` alias + model, `docs/plans/llm-provider-pool.md`)
- `SetPromptOverride(id, text)` — 봇 성격/역할 지시
- `SetToolConfig(id, *SessionToolConfig)` — `ToolsEnabled`/`ToolsDisabled`/allow·deny groups
- `SetPermissionMode(id, chatPermissionModePlan)` — 읽기 전용
- `SetWorkDirs`, `SetIsolation(IsolationOff)`

턴 실행: `runServerChatTurnAs(ctx, deps, sessionID, message, consoleContext, chatTurnOrigin{unattended: <source>, readOnly: true})` (`focus_driver.go bind()`의 `qaTurn` 참고).

### 재사용할 기존 기능

| 필요 | 기존 자산 |
|---|---|
| 웹 검색/페치 | `tool.NewWebSearchToolWithOptions` (Brave/Perplexity, `cfg.ToolsWebSearchEnabled`), web fetch (`internal/tool`) — `helpers_build_tools.go` |
| 엔진(프로바이더) | 3-tier router + provider pool. 프로바이더: anthropic / openai / openai-codex / gemini / claude-code-cli / antigravity-cli |
| 알림 | `internal/notification`: `NewEvent(category, severity, title, message)`, `Dispatcher.Emit` → SSE `/v1/events/*` + 데스크톱 알림 |
| 메모리 제안 | `pkg/memory`: `AppendInboxCandidateIfNew`, `ListMemoryCandidates`, `ReviewMemoryCandidate` (inbox 후보 → 사람이 승인) |
| 다중 에이전트 | `internal/agentruntime` (consensus, provider override) — **이번 계획에서는 쓰지 않음** (결합도 큼, 결정 사항) |

## 3. 콘솔 구조

- 라우트/셸: `src/App.svelte`, `lib/router`, `focusChromeHidden(route)` — focus 화면은 사이드바 숨김.
- Focus 컴포넌트: `src/components/focus/` (약 4,600줄) — `FocusView`, `FocusHome`, `FocusNewTask`, `FocusStepper`, `FocusDeck`, `FocusCard`, `FocusPlanGate`, `FocusPRGate`, `FocusMergeSummary`, `FocusQADrawer`, `FocusAddFinding`, `FocusGraph`, `FocusReleaseTrain`, `FocusTemplates`, `FocusRawSlice`, `FocusCIChips`.
- 순수 로직: `src/lib/focus.ts` (`orderCards`, `mustHandle`, `deckFor`, `stepperItems`, `progressLine`, `qaThreads`, `triageProgress`, `findingEntry` …) + `focusGraph.ts`, `focusPR.ts`, `focusRelease.ts`, `focusTemplateEdit.ts`, `focusKickoff.ts`.
- i18n: `src/i18n/` (en/ko, `focus.*` 섹션). 새 문구는 en·ko 둘 다.
- 테스트: `tests/focus*.test.ts` (node test runner, 순수 함수), `e2e/focus*.spec.ts` (Playwright + `e2e/mock-llm.mjs`의 `[e2e:…]` 마커).
- 디자인 토큰: `src/app.css` "Graphite Signal". **D안(B Monolith)은 focus 라우트에만 스코프**해서 적용한다 (전역 변경 시 `make console-e2e` 전체 필요 — CLAUDE.md 경고, #1194).

## 4. 컨벤션

- Go: 패키지 주석에 파일 지도. 에러는 `fmt.Errorf("…: %w")`, 센티널 에러 `var errX = errors.New`. 로깅 zerolog (`d.logger.Info().Err(err).Str("session_id", id).Msg("focus: …")`, 메시지 prefix `focus:`).
- 순수 로직은 `focuspipeline`, I/O·LLM·세션은 `tarsserver`. **새 패키지는 `httpapi` 헬퍼를 직접 사용** (#1204).
- 테스트: 같은 디렉터리 `_test.go`, table-driven. 시간은 `now` 주입.
- 워크스페이스 파일: 텍스트만, 명령 없음 (템플릿 원칙). 검증 실패 파일은 diagnostic과 함께 skip.
- API 변경 시: `docs/decisions/focus-mode.md` 갱신 + `docs/feature-map.md` 항목(Reach / Drive / Healthy result / Evidence) 추가.
- pkg/* export 변경 시 `make api-snapshot`.

## 5. 설계 결정 (이번 계획)

1. **봇 = 워커 세션 + 봇 스펙**. 실행은 기존 Q&A 패턴, 읽기 전용이 기본.
2. **허들·교차검토 = 드라이버가 메인 턴 전/후에 돌리는 서브턴**. 상태머신(`Apply`)은 거의 건드리지 않는다. 서브턴 결과는 (a) 메인 턴 guidance에 다이제스트로 들어가거나 (b) 기존 `AddFinding` 입구로 카드가 된다.
3. 빌더·스크라이브는 **메인 세션의 페르소나**(라벨 + tier). 메인 세션 턴 구조는 그대로.
4. 크루가 없는 파이프라인은 **오늘과 완전히 동일**하게 동작 (하위 호환).
5. 크루 스펙은 파이프라인 시작 시 **복사**된다 (템플릿과 같은 원칙). 채용 봇은 파이프라인 안에만 존재하고, "보관"할 때만 라이브러리 파일이 된다.
