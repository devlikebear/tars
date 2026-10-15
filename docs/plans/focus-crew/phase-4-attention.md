# Phase 4 — 주목할 것 · 시각화 · Monolith 테마 (D1 · D3 · D4 화면)

> Roadmap: [roadmap.md](./roadmap.md) · 선행: Phase 2 (crew log), Phase 3 권장
> 목표: 사용자는 **주목할 것만** 보고, 봇들의 협업은 다이어그램 애니메이션으로 "느낀다". Focus 화면 전체를 D안(B Monolith) 스타일로 바꾸되 **focus 라우트에만 스코프**한다.

## 완료 조건

- 오른쪽 "주목할 것" 레일: must-handle 카드(게이트·결정·finding) + 새 정보 요약(허들 요약, 기각된 주장, 이견) + 예고(곧 열릴 게이트). 글리프 배지 `!` `?` `✓` `≠`.
- 왼쪽 "크루 무대": 현재 단계의 크루 다이어그램 — 노드(봇), 엣지(최근 crew log 대화), 패킷 애니메이션, 봇별 말풍선(최근 노트 한 줄).
- 게이트·결정 카드가 생기면 알림(`notification.Dispatcher.Emit`) — 데스크톱 알림/SSE.
- Focus 홈(D1): 진행 중 파이프라인 행에 크루 아바타 + 말풍선 알림, 크루 라이브러리 패널(읽기 전용), 릴리스 열차.
- `prefers-reduced-motion`이면 애니메이션 정지.
- focus 외 화면의 스타일 변화 0.

## 작업

### A. 순수 로직 (`src/lib/focusCrew.ts`)
- [ ] `attentionItems(p, log, now)` → `{kind: 'gate'|'decision'|'finding'|'info'|'forecast', glyph, title, sub, cardId?, botGlyph?}[]`
  - must-handle 카드는 기존 `orderCards`/`mustHandle` 순서 그대로 맨 위
  - info: 아직 seen 아닌 허들 요약 카드, unsupported 판정, disputed finding
  - forecast: plan 허들 진행 중이면 "곧 G1", review 상호검토 중이면 "곧 G2"
- [ ] `stageGraph(stage, crew, log, now, windowMs=120000)` → `{nodes:[{id, glyph, x, y, talking}], edges:[{from, to, kind:'peer'|'command', active, label}], bubbles:[{bot, text, glyph}]}`
  - 레이아웃: TARS 중앙 + 봇 최대 5명 고정 좌표 테이블(1~5명별), 900×460 기준
  - edge label: 해당 쌍의 최근 log `kind` 기반 ("팩트체크 3/4", "시안 ↔ 추정")
- [ ] 테스트: 우선순위, 중복 제거, 레이아웃 좌표 결정성, window 밖 엣지 비활성

### B. 컴포넌트
- [ ] `src/components/focus/focus-monolith.css`: D안 토큰(`--fm-ground #0f0f0e` 등), Geist / Geist Mono / Martian Mono. `FocusView`·`FocusHome` 루트 클래스 `.focus-monolith` 아래에만 적용
- [ ] `FocusCrewStage.svelte` (D3 왼쪽): absolute 배치 노드 + rotate 엣지 div + `@keyframes run` 패킷 + 말풍선, `role="img"` + `aria-label` 요약 문장
- [ ] `FocusAttention.svelte` (D3/D4 오른쪽): 알림 목록(pop-in) + 기존 `FocusDeck`의 현재 카드 + 대기 카드. 기존 키보드(←/→, 1–9, `?`, `o`) 유지
- [ ] `FocusView.svelte` 레이아웃 교체: 헤더(Goal mode·상세보기·Advanced·중지) / stepper(크루 점) / [크루 무대 | 주목할 것] / 사실 줄(검증 exit code 등, `progressLine` 재사용) / 단계 지시 입력. 크루 없는 파이프라인은 무대 대신 기존 진행 패널
- [ ] `FocusHome.svelte` (D1): New task 슬랩, 진행 중 행(7칸 진행바·크루 아바타·말풍선), 크루 라이브러리 패널(`GET /v1/focus/crew/bots`), 릴리스 열차(기존 컴포넌트 재배치)
- [ ] i18n en/ko

### C. 서버 알림
- [ ] 게이트 오픈, decision 카드 생성, 이견 finding 생성 시 `Dispatcher.Emit(notification.NewEvent("focus", …))` — 이미 emit하는 곳이 있으면 중복 없이 category/메시지만 확장. 테스트: 이벤트 1회 발행
- [ ] CASE 컴패니언 연동은 하지 않음 (기존 동작 유지)

### D. 검증 범위 주의
- `App.svelte`·`app.css`·상시 마운트 컴포넌트는 건드리지 않는다. 건드려야 하면 **`make console-e2e` 전체** 실행 (CLAUDE.md)
- 스크린샷 갱신: `make console-screenshots` (docs/screenshots/console-focus.webp)

---

### ✅ Checkpoint: Phase 4

**자동 확인**
- [ ] `npm run check`, `npm run test:ci`, `node --experimental-strip-types --test tests/focusCrew.test.ts`
- [ ] `npx playwright test e2e/focus.spec.ts e2e/focusCrew.spec.ts e2e/focusTemplates.spec.ts e2e/focusRelease.spec.ts e2e/companion-focus.spec.ts`
- [ ] `make test` (알림 테스트), `make lint-diff`

**수동 확인**
- [ ] Plan 허들 중 무대에서 패킷이 움직이고 말풍선이 최신 노트로 바뀜, 오른쪽엔 결정·예고만
- [ ] 시스템 설정에서 동작 줄이기 켬 → 애니메이션 정지, 정보는 동일
- [ ] 다른 화면(Chat, Sessions)의 색·폰트 변화 없음
- [ ] 게이트가 열리면 데스크톱 알림 1회

**통과하면 사용자 확인 후 Phase 5로.**
