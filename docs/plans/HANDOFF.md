# Handoff — 세컨드 오피니언 리뷰 · CASE 리디자인

> Claude Code 세션 시작용. 이 문서를 먼저 읽고, 아래 "다음 작업"부터 진행한다.
> 작성: 2026-10-15 (Cowork 기획 세션) · 개정: 2026-10-17 (Focus × Crew 철회, 세컨드 오피니언 리뷰 채택) · 기준 커밋: `9907a083` (main)

## 1. 무엇을 하려는가

| 계획 | 한 줄 요약 | 문서 | 상태 |
|---|---|---|---|
| **세컨드 오피니언 리뷰** | Focus review 단계에서 **다른 모델**이 같은 변경을 읽기 전용으로 한 번 더 리뷰하고, 그 지적이 기존 finding 카드로 들어간다. 파이프라인별 옵트인, 기본 꺼짐 | [`second-opinion-review/plan.md`](./second-opinion-review/plan.md) | 계획 확정, 미착수 |
| **CASE Companion** | 이니셔티브 컴패니언 CASE를 "당직 미어캣"으로 리디자인하고, 어디서나 TARS 전체 상황을 묻는 Bridge와 돌아왔을 때 요약을 붙인다 | `docs/plans/case-companion/` (3 phase) | P1 머지(#1265), 수동 확인 대기 · P2·P3 미착수 |
| ~~Focus × Crew~~ | 봇 크루(소환·채용·허들·상호검토·해산) | `docs/plans/focus-crew/` | **철회 (2026-10-17)** — 이유는 [`focus-crew/roadmap.md`](./focus-crew/roadmap.md)의 "철회 결정". 문서는 기록으로만 남는다 |

공통 디자인 자산: `docs/plans/design-system/` (tokens.css, fonts.css, fonts/)

## 2. 확정된 결정 (다시 묻지 말 것)

세컨드 오피니언 리뷰:
- Focus × Crew는 **철회**했다. 크루·봇 라이브러리·허들·crew log·해산·Monolith 테마·Focus 화면 개편은 하지 않는다. 지금 Focus 화면과 시작 흐름은 그대로다.
- Crew Phase 3의 독립 리뷰만 **세컨드 오피니언 리뷰**로 줄여 채택한다. 파이프라인별 옵트인이고 **기본은 꺼짐**이다 (e2e와 같은 원칙).
- 단계 전이는 여전히 사실로만 결정한다. 세컨드 오피니언의 지적은 기존 finding 카드와 triage를 그대로 탄다. 목표 모드의 고정 정책(`NextGoalStep`)은 바꾸지 않는다.

CASE:
- CASE 캐릭터는 **A안 당직 미어캣**, 이름은 CASE 유지. 대상은 기존 `CompanionPet.svelte` + `internal/initiative`.
- CASE 얼굴은 **CSS + span만** (`frontend/console/DESIGN.md` "CASE's expressions" 규칙 유지). 레퍼런스: `case-companion/design/html/case-face.html`.
- CASE 질문은 Phase 2부터 main 세션이 아닌 **전용 읽기 전용 세션**으로 간다 (tars#1236 대체, ADR 작성).

## 3. 문서 지도

```
docs/plans/
├── HANDOFF.md                     ← 지금 이 문서
├── second-opinion-review/
│   └── plan.md                    범위·설계·작업·Checkpoint (먼저 읽기)
├── design-system/                 tokens.css · fonts.css · fonts/ (OFL) · README
├── case-companion/
│   ├── roadmap.md                 범위·불변 조건·Out of Scope
│   ├── phase-1-meerkat-face.md    (머지됨, #1265)
│   ├── phase-2-bridge.md
│   ├── phase-3-arrival-brief.md
│   └── design/
└── focus-crew/                    철회됨 — 결정 기록과 시안만 남김
```

계획 문서는 작업 체크리스트(파일·함수·테스트) + 끝의 **Checkpoint**(자동/수동 확인)로 되어 있다.

## 4. 진행 방식

1. **레포 규칙 우선**: `CLAUDE.md`, `AGENTS.md`, `tars-github-flow` 스킬(worktree → TDD → 커밋 → PR → CI → 머지 → 정리).
2. phase 하나 = 브랜치 하나(필요하면 PR 2개). 문서의 작업을 **위에서부터 순서대로**, 작업 단위마다 사용자에게 짧게 보고.
3. 끝에서 Checkpoint의 자동 확인을 모두 실행하고 결과를 보고한 뒤, **수동 확인 항목은 사용자에게 요청하고 멈춘다.** 승인 전에는 다음으로 가지 않는다.
4. 계획과 코드가 어긋나면(파일·함수가 문서와 다름) 코드가 맞다. 어긋난 지점과 조정안을 보고하고 진행한다. 범위를 넓히는 변경은 묻고 한다.
5. Out of Scope 항목은 하지 않는다.

## 5. 진척과 다음 작업

### 끝난 것
- 계획 문서 PR — #1262 (2026-10-15)
- CASE Phase 1 미어캣 얼굴 — #1265. 자동 확인 통과(`npm run test:ci` 533, `make console-e2e` 143). **수동 확인 5개가 사용자 확인 대기**다(승인 대기 alert, 실패 upset, 5분 sleepy, 동작 줄이기, Chat 레일 비가림 — `case-companion/phase-1-meerkat-face.md` Checkpoint)

### 다음 작업: 세컨드 오피니언 리뷰 (권장)
- 읽을 것: [`second-opinion-review/plan.md`](./second-opinion-review/plan.md) → `docs/decisions/focus-mode.md` §4(파이프라인·게이트)·§4.2(목표 모드)·§5(사실)·§8(Q&A) → `internal/focuspipeline/review.go`, `finding_add.go`, `verification.go`(`Interrupt`) → `internal/tarsserver/focus_qa.go`, `focus_driver.go`
- Focus는 `docs/decisions/product-focus.md`의 Core 영역이다. PR 본문에 파이프라인 근거 한 문장을 쓴다: "한 모델이 놓친 결함을 머지 전에 다른 모델이 잡는다."

### CASE Phase 2·3 — 착수 전 확인 필요
`docs/decisions/product-focus.md`는 Companion을 "#1191–#1193을 이미 정한 범위대로 끝내고, 에픽이 닫히면 동결"로 정했고 에픽 #1188은 닫혔다. Bridge(P2)와 돌아왔을 때 요약(P3)은 그 뒤에 범위를 넓히는 일이라, 진행하려면 ADR의 Companion 항목을 먼저 고쳐야 한다. 또 P2는 채팅 도구 `tars_status`를 새로 추가하므로 CLAUDE.md의 Extension pattern("builtin 도구를 늘리지 말 것")과도 맞춰야 한다. 아래 8절의 열린 질문으로 사용자에게 확인한다.

## 6. 자주 쓰는 검증 명령

```bash
make test                         # Go 전체
make test-one TEST_NAME=… PKG=./internal/…/
make lint-diff && make test-diff && make arch-check
cd frontend/console && npm run check && npm run test:ci
cd frontend/console && node --experimental-strip-types --test tests/<name>.test.ts
make console-e2e                  # 전역 컴포넌트·CSS를 건드렸으면 전체
make console-screenshots          # UI 변경 후 docs/screenshots 갱신
```

## 7. 함정

- 전역 CSS(`app.css`)에 `--case-*`를 넣지 말 것. CASE는 컴포넌트 `<style>` 안.
- reduced-motion 블록은 애니메이션 셀렉터를 **하나씩 나열** (테스트가 검사).
- 새 문구는 i18n en·ko 둘 다.
- 모델이 출력하는 블록(`<focus-findings>`, `<case-cards>`)의 `source`·링크·id는 서버가 검증·생성한다. 모델 값을 그대로 믿지 않는다.
- API를 바꾸면 `docs/decisions/` ADR과 `docs/feature-map.md`를 같은 PR에서 갱신.
- pkg/* export를 바꾸면 `make api-snapshot`.

## 8. 열린 질문 (해당 작업에서 사용자에게 확인)

- 세컨드 오피니언: 첫 라운드에만 돌릴지(계획의 기본값), 수정 뒤 라운드마다 돌릴지. 몇 주 사용 뒤 `plan.md`의 측정 결과로 정한다.
- CASE P2·P3: ADR 동결 범위를 고쳐 진행할지, 2026-12-31 재평가까지 미룰지.
- CASE P2: CASE 전용 세션의 tier 기본값 (light 제안). 하루 단위 세션 회전으로 충분한지.
