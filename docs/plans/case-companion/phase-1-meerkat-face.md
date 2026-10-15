# Phase 1 — 미어캣 얼굴 (E1 · E4)

> Roadmap: [roadmap.md](./roadmap.md)
> 목표: CASE의 몸을 미어캣으로 바꾼다. **동작 로직은 건드리지 않는다** — 같은 `expr-<name>` 클래스, 같은 `act-<action>`, 같은 cue 머신.

> 구현 레퍼런스: [`design/html/case-face.html`](./design/html/case-face.html) — span + CSS 미어캣, 표정 8종·동작 5종·reduced-motion이 이미 들어 있다. 이 마크업과 CSS를 `CompanionPet.svelte`로 옮기는 것이 이 페이즈의 대부분이다. 색은 `../design-system/tokens.css`의 `--case-*`.

## 완료 조건

- 콘솔 모든 화면(Chat 레일 옆, Focus 거터, 그 외)의 CASE가 미어캣으로 보인다.
- 표정 8종이 E4 시트와 같은 모양 규칙으로 구분된다. 상태색은 가슴 칩에만.
- 기존 companion 단위·e2e 테스트가 모두 통과한다 (표정 결정 로직 변경 없음).
- reduced-motion에서 반복 애니메이션 정지 테스트 통과.

## 부위 매핑 (기존 → 미어캣)

| 기존 span | 미어캣 부위 | 비고 |
|---|---|---|
| `.companion-antenna` (+ tip 색) | `.companion-gem` 가슴 칩 | 상태색이 실리는 유일한 곳. 클래스명은 바꾸되 테스트·CSS 셀렉터 함께 수정 |
| `.companion-brows` | 눈썹 (눈 패치 위) | alert/upset/wary에서만 보임 — 기존 규칙 그대로 |
| `.companion-eyes` | 눈 패치(짙은 갈색 타원) + 눈동자 | 패치는 항상, 눈동자 모양이 표정마다 바뀜 |
| `.companion-mouth` | 코 아래 입선 | happy/greeting 미소, upset 처짐 |
| (신규) `.companion-ears` | 작은 귀 2개 | alert에서 살짝 위로 |
| (신규) `.companion-torso` + `.companion-paw` | 몸통, 앞발 | greeting에서 한쪽 발 들기, working에서 노트북 |
| `.companion-shadow` | 그대로 | |

색: 머리 `#E3C48F`, 몸통 `#D9B97E`, 귀 `#B8955E`, 눈 패치 `#6B5434`, 눈동자 `#F5EBDD`, 코 `#2A1C14`. 콘솔 토큰에 `--case-*`로 추가 (`app.css`가 아닌 컴포넌트 `<style>` 안 지역 변수로 — 전역 영향 없음).

## 표정 모양 규칙 (E4 시트 기준)

| 표정 | 눈 | 눈썹 | 입·자세 | 칩 | 동작 |
|---|---|---|---|---|---|
| neutral | 둥근 점 | 없음 | 없음 | 회색 | — |
| working | 아래로 내린 작은 점 | 없음 | 노트북 바 | 앰버 | — |
| alert | 큰 원 | 올림 | 귀 위로 | 주황 | bounce |
| happy | ∩ 아치 | 없음 | 미소 | 녹색 | nod |
| upset | 작은 점 | 안쪽으로 처짐 | 입꼬리 아래 | 주황 | shake |
| wary | 가늘게 뜬 눈 + 곁눈 | 비스듬 | — | 앰버 | tilt |
| sleepy | 감은 선 | 없음 | 몸 살짝 기움 + z | 어두운 회색 | — |
| greeting | 한쪽 윙크 | 없음 | 미소 + 앞발 | 녹색 | wave |

alert↔upset은 칩 색이 같으므로 눈 크기·눈썹 방향·입 3곳이 달라야 한다 (DESIGN.md 규칙).

## 작업

- [ ] `CompanionPet.svelte` 마크업: `.companion-body` 안 span 구조를 위 부위로 교체. `expr-*` 클래스 결정 코드는 그대로
- [ ] 같은 파일 `<style>`: 부위 도형(border-radius·clip-path·transform), 표정별 규칙 8종, 기존 `act-*`/`play-*` 애니메이션을 새 부위에 재연결 (wave → 앞발, nod → 머리, bounce → 전신)
- [ ] reduced-motion 블록: 애니메이션이 붙은 모든 셀렉터를 개별로 나열 (DESIGN.md 규칙, `tests/companionPet.test.ts`가 검사)
- [ ] 크기: 현재 버튼 폭 `--case-button-width`(74px) 유지. 말풍선 위치·`beside-rail` 레이아웃 변경 없음
- [ ] `tests/companionPet.test.ts`: 안테나 셀렉터를 칩으로 바꾼 부분만 수정, 표정별 필수 부위 존재 검사 추가
- [ ] `frontend/console/DESIGN.md` "CASE's expressions" 갱신 (부위 이름, 위 표)
- [ ] 스크린샷: `make console-screenshots`

검증 범위 주의: `CompanionPet.svelte`는 모든 화면에 마운트된다 → **`make console-e2e` 전체 실행 필수** (CLAUDE.md, #1194).

---

### ✅ Checkpoint: Phase 1

**자동 확인**
- [ ] `cd frontend/console && npm run check && npm run test:ci`
- [ ] `node --experimental-strip-types --test tests/companionPet.test.ts tests/companionAsk.test.ts`
- [ ] `make console-e2e` 전체 통과 (`e2e/companion-*.spec.ts` 7개 포함)

**수동 확인** (`make dev-console`)
- [ ] 승인 대기 세션 만들기 → alert + bounce, 칩 주황
- [ ] 턴 실패 → upset + shake, alert와 모양으로 구분됨
- [ ] 5분 무입력 → sleepy, 입력하면 즉시 깸
- [ ] 시스템 "동작 줄이기" → 애니메이션 정지, 표정 모양은 그대로
- [ ] Chat 화면에서 레일 아이콘을 가리지 않음

**통과하면 사용자 확인 후 Phase 2로.**
