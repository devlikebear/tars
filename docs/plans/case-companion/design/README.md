# CASE 디자인 시안

출처: Design 캔버스 "TARS Console 디자인 고도화" E행 (2026-10-15). 1440px 기준, 2x PNG.

| 파일 | 내용 | 해당 페이즈 |
|---|---|---|
| `e1-character.png` | 캐릭터 후보 3종. **A 당직 미어캣으로 확정** (B, C는 기록용) | Phase 1 |
| `e4-expressions.png` | 미어캣 표정 8종, 발동 조건, 모양 규칙, 동작, 자리·부르기·목소리 | Phase 1 (구현 기준) |
| `e2-bridge.png` | 어디서나 묻기 패널: 지금 보는 곳, 라이브 카드, 본 곳, 후속 질문, 범위 토글, ⌘J | Phase 2 |
| `e3-initiative.png` | 먼저 말 걸기: 돌아왔을 때 요약(greet) / 오래 일할 때 안부(check-in), 말하는 때·말하지 않는 때 | Phase 3 |

## HTML (`html/`)

브라우저로 바로 연다 (빌드 불필요). 폰트·토큰은 `../../../design-system/`을 참조한다.

| 파일 | 용도 |
|---|---|
| `html/case-face.html` | **Phase 1 구현 레퍼런스**: span + CSS만으로 만든 미어캣, `expr-*` 8종, `act-*` 5종(버튼으로 재생), reduced-motion, 배경 3종 대비 확인. 마크업·CSS를 `CompanionPet.svelte`로 옮기는 출발점 |
| `html/e1-character.html` ~ `e4-expressions.html` | 캔버스 아트보드의 정적 사본 (레이아웃·간격·색 수치 확인용, 인라인 스타일 그대로) |

정적 사본은 캔버스 런타임 없이 렌더된 DOM을 저장한 것이라 상호작용이 없다. 얼굴 부분은 SVG이므로 구현에는 `case-face.html`을 기준으로 삼는다.

## 구현 시 주의

- 시안의 얼굴은 SVG로 그렸지만 **구현은 CSS + span** (`frontend/console/DESIGN.md` "CASE's expressions" 규칙). 비율·색·모양만 참고한다.
- 색상값: 머리 `#E3C48F`, 몸통 `#D9B97E`, 귀 `#B8955E`, 눈 패치 `#6B5434`, 눈동자 `#F5EBDD`, 코 `#2A1C14`. 가슴 칩: neutral `#8a877d`, working·wary `#f2a93b`, alert·upset `#ff7a59`, happy·greeting `#9bd77a`, sleepy `#4a4842`.
- 시안 배경은 Focus Crew의 Monolith 테마(어두운 배경)다. CASE는 모든 화면에 뜨므로 기존 Graphite 화면 위에서도 대비를 확인할 것.
- 화면 속 데이터(파이프라인 이름, 시간, 비용)는 예시다.
