# Focus × Crew 디자인 시안

> **Crew 계획은 철회됐다 (2026-10-17, [../roadmap.md](../roadmap.md)).** 이 시안은 기록으로만 남긴다.

출처: Design 캔버스 "TARS Console 디자인 고도화" D행 (2026-10-15). 1440px 기준, 2x PNG. 화면 속 데이터는 예시다.

| 파일 | 내용 | 해당 페이즈 |
|---|---|---|
| `d1-home.png` | Focus 홈: New task 슬랩, 진행 중 파이프라인(크루 아바타·말풍선 알림), 크루 라이브러리, 릴리스 열차 | Phase 4 (라이브러리 패널은 Phase 1 데이터) |
| `d2-assemble.png` | 크루 편성: 소환 카드, 채용 공고, 단계 × 봇 배치표 | Phase 1 |
| `d3-plan-huddle.png` | Plan 허들: 크루 무대(다이어그램·패킷 애니메이션·말풍선), 주목할 것 레일, 결정 카드, G1 예고 | Phase 2 (데이터) · Phase 4 (시각화) |
| `d4-cross-review.png` | Review 상호검토: 리뷰어 2명 교차 판정, 사실 줄, G2 finding 카드(합의 배지), Q&A, 이견 카드 | Phase 3 · Phase 4 |
| `d5-wrap.png` | G4 머지 + 크루 해산(보관/은퇴), 기억 후보 | Phase 5 |
| `d6-detail.png` | 상세보기: 단계별 스윔레인(봇 간 메시지), 원문 패널 | Phase 2 |

## HTML (`html/`)

`html/d1-home.html` ~ `d6-detail.html`: 캔버스 아트보드를 렌더해 저장한 정적 HTML이다. 브라우저로 바로 열 수 있고, 인라인 스타일과 `<style>`(애니메이션 포함)이 그대로 들어 있어 간격·크기·색 수치를 확인하는 용도다. 폰트·토큰은 `../../../design-system/`을 참조한다.

- 화면 폭 1440px 기준 (d1은 높이 960). 반응형 동작은 일부만 들어 있다.
- 봇 아바타는 SVG다. Crew Phase 1의 `CrewAvatar.svelte`는 이 SVG path를 그대로 옮겨도 된다 (크루 아바타는 CASE 얼굴 규칙의 적용 대상이 아님).
- 상호작용(버튼·탭)은 동작하지 않는다.

## 구현 시 주의

- 테마 토큰(D안 Monolith): ground `#0f0f0e`, surface `#161614`/`#1d1d1a`, line `#2a2a26`, text `#eceae4`/`#b0ada3`/`#8a877d`, accent `#f2a93b`, alert `#ff7a59`, ok `#9bd77a`. 폰트 Geist / Geist Mono / Martian Mono(라벨). **focus 라우트에만 스코프** (phase-4 문서 참조).
- 봇 색: Hoot `#C9A7FF`, Miso `#9BD77A`, Pip `#7FB2FF`, Ellie `#B8C4D0`, Bea `#E0A070`.
- 애니메이션은 `prefers-reduced-motion`에서 정지.
