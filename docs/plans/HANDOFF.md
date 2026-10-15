# Handoff — Focus × Crew · CASE 리디자인

> Claude Code 세션 시작용. 이 문서를 먼저 읽고, 아래 "첫 작업"부터 진행한다.
> 작성: 2026-10-15 (Cowork 기획 세션) · 기준 커밋: `ece82981` (main)

## 1. 무엇을 하려는가

두 개의 독립 계획이 확정되어 있다.

| 계획 | 한 줄 요약 | 문서 |
|---|---|---|
| **CASE Companion** | 이니셔티브 컴패니언 CASE를 "당직 미어캣"으로 리디자인하고, 어디서나 TARS 전체 상황을 묻는 Bridge와 돌아왔을 때 요약을 붙인다 | `docs/plans/case-companion/` (3 phase) |
| **Focus × Crew** | 기존 Focus mode에 봇 크루(소환·채용·허들·상호검토·해산)를 얹는다. 파이프라인·게이트·사실 기반 전이는 그대로 | `docs/plans/focus-crew/` (5 phase) |

공통 디자인 자산: `docs/plans/design-system/` (tokens.css, fonts.css, fonts/)

## 2. 확정된 결정 (다시 묻지 말 것)

- CASE 캐릭터는 **A안 당직 미어캣**, 이름은 CASE 유지. 대상은 기존 `CompanionPet.svelte` + `internal/initiative`.
- CASE 얼굴은 **CSS + span만** (`frontend/console/DESIGN.md` "CASE's expressions" 규칙 유지). 레퍼런스: `case-companion/design/html/case-face.html`.
- 크루 봇은 동물(Hoot 부엉이, Miso 고양이, Pip 강아지, Ellie 코끼리, 채용 예시 Bea 비버). TARS 자신은 리드(드라이버)이며 봇이 아니다. 크루 아바타는 SVG 허용.
- 크루 실행 방식은 **서브턴**: 봇 = 숨은 `worker` 세션(Q&A 세션 패턴), 허들·교차검토는 드라이버가 메인 턴 전후에 실행. `agentruntime`은 쓰지 않는다.
- Focus 화면 테마는 D안 **Monolith**, **focus 라우트에만 스코프**. 다른 화면은 바꾸지 않는다.
- 단계 전이는 **사실로만** 결정한다 (봇 의견·투표는 카드 근거 표시일 뿐). 크루 없는 파이프라인은 오늘과 동일해야 한다.
- CASE 질문은 Phase 2부터 main 세션이 아닌 **전용 읽기 전용 세션**으로 간다 (tars#1236 대체, ADR 작성).

## 3. 문서 지도

```
docs/plans/
├── HANDOFF.md                     ← 지금 이 문서
├── design-system/                 tokens.css · fonts.css · fonts/ (OFL) · README
├── case-companion/
│   ├── roadmap.md                 범위·불변 조건·Out of Scope
│   ├── phase-1-meerkat-face.md
│   ├── phase-2-bridge.md
│   ├── phase-3-arrival-brief.md
│   └── design/                    README · PNG(e1~e4) · html/(case-face.html = Phase 1 구현 기준, e1~e4 정적 사본)
└── focus-crew/
    ├── codebase-analysis.md       Focus 코드 지도·컨벤션·재사용 자산 (먼저 읽기)
    ├── roadmap.md
    ├── phase-1-crew-roster.md … phase-5-wrap-lifecycle.md
    └── design/                    README · PNG(d1~d6) · html/(정적 사본)
```

각 phase 문서는 작업 체크리스트(파일·함수·테스트) + 끝의 **Checkpoint**(자동/수동 확인)로 되어 있다.

## 4. 진행 방식

1. **레포 규칙 우선**: `CLAUDE.md`, `AGENTS.md`, `tars-github-flow` 스킬(worktree → TDD → 커밋 → PR → CI → 머지 → 정리).
2. phase 하나 = 브랜치 하나(필요하면 PR 2개). phase 문서의 작업을 **위에서부터 순서대로**, 작업 단위마다 사용자에게 짧게 보고.
3. phase 끝에서 Checkpoint의 자동 확인을 모두 실행하고 결과를 보고한 뒤, **수동 확인 항목은 사용자에게 요청하고 멈춘다.** 승인 전에는 다음 phase로 가지 않는다.
4. 계획과 코드가 어긋나면(파일·함수가 문서와 다름) 코드가 맞다. 어긋난 지점과 조정안을 보고하고 진행한다. 범위를 넓히는 변경은 묻고 한다.
5. Out of Scope 항목(각 roadmap)은 하지 않는다.

## 5. 첫 작업

### 5-1. 계획 문서 PR (먼저)

`docs/plans/`의 다음 경로는 main 작업 트리에 **커밋되지 않은 상태로** 놓여 있다: `HANDOFF.md`, `design-system/`, `case-companion/`, `focus-crew/`.

- 브랜치 `docs/focus-crew-case-plans`로 옮겨 커밋 → PR → 머지.
- `make security-scan` 통과 확인 (폰트 바이너리·PNG 포함, 절대경로 없음은 확인됨).
- `fonts/`는 시안 확인용 사본이다. 콘솔 구현 때는 `@fontsource/*` npm 패키지를 쓴다 (`design-system/README.md`).

### 5-2. CASE Phase 1 — 미어캣 얼굴 (권장 첫 구현)

위험이 작고 결과가 바로 보인다.

- 읽을 것: `case-companion/roadmap.md` → `phase-1-meerkat-face.md` → `design/html/case-face.html`(브라우저로 열어 보기) → `frontend/console/DESIGN.md` "CASE's expressions" → `src/components/CompanionPet.svelte`, `src/lib/companion.ts`, `tests/companionPet.test.ts`
- 핵심: `case-face.html`의 마크업과 CSS를 `CompanionPet.svelte`로 옮긴다. `expr-*` 결정 로직(`companionExpression`)은 건드리지 않는다.
- 주의: `CompanionPet`은 모든 화면에 마운트된다 → **`make console-e2e` 전체 실행 필수** (CLAUDE.md, #1194).

이후 순서 제안: CASE P2 → CASE P3 → Crew P1 → P2 → P3 → P4 → P5. Crew를 먼저 해도 된다 (두 계획은 독립).

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

- 전역 CSS(`app.css`)에 Monolith 토큰이나 `--case-*`를 넣지 말 것. Focus는 `.focus-monolith` 아래, CASE는 컴포넌트 `<style>` 안.
- reduced-motion 블록은 애니메이션 셀렉터를 **하나씩 나열** (테스트가 검사).
- 새 문구는 i18n en·ko 둘 다.
- 모델이 출력하는 블록(`<focus-findings>`, `<crew-note>`, `<case-cards>`)의 `source`·링크·id는 서버가 검증·생성한다. 모델 값을 그대로 믿지 않는다.
- API를 바꾸면 `docs/decisions/` ADR과 `docs/feature-map.md`를 같은 PR에서 갱신.
- pkg/* export를 바꾸면 `make api-snapshot`.

## 8. 열린 질문 (해당 phase에서 사용자에게 확인)

- CASE P2: CASE 전용 세션의 tier 기본값 (light 제안). 하루 단위 세션 회전으로 충분한지.
- Crew P1: 크루 편성 LLM 호출 tier (`chat_main` 제안), 채용 봇 최대 2명 제한이 적절한지.
- Crew P2: 허들 타임아웃 기본값(6분)과 웹 검색 키가 없는 환경의 기본 동작.
