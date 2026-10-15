# Focus × Crew — Roadmap

> 기존 Focus mode를 **크루(봇 협업) 컨셉**으로 고도화한다. 파이프라인·게이트·사실 기반 전이·카드 덱·Q&A·Goal mode·템플릿·릴리스 열차는 **그대로 둔다**.
> 디자인 시안: Design 캔버스 "TARS Console 디자인 고도화"의 D행 (D1 홈 · D2 크루 편성 · D3 Plan 허들 · D4 Review 상호검토 · D5 G4+해산 · D6 상세보기).
> 레포 사본: [`design/`](./design/README.md) (PNG + 색상·토큰 메모)
> 공통 참조: [`codebase-analysis.md`](./codebase-analysis.md)

## 왜

Focus mode는 "지금 어느 단계이고 무엇을 결정해야 하는지"를 이미 잘 보여 준다. 부족한 것은 두 가지다.

1. **한 모델의 맹점**: plan과 review를 한 에이전트가 혼자 한다. 근거 조사·팩트체크·다른 엔진의 교차 검토가 없다.
2. **협업의 가시성**: 여러 관점이 오가는 과정이 보이지 않고, 보이게 하면 채팅 스트림이 다시 시끄러워진다.

Crew는 TARS가 작업마다 봇 팀을 꾸리고(소환/채용), 봇끼리 토론·검증하게 하되, **사용자에게는 주목할 것만** 올린다. 대화 기록은 상세보기에서만 본다.

## 지켜야 할 불변 조건

- 단계 전이는 사실로만 결정한다. 봇의 의견·투표는 **카드의 근거 표시**일 뿐 전이 조건이 아니다.
- 크루가 없는 파이프라인은 오늘과 동일하게 동작한다 (모든 기존 테스트 통과).
- 봇 서브턴은 기본 **읽기 전용**(`plan` 권한)이다. 코드를 바꾸는 건 메인 세션 턴뿐이다.
- 사용자 개입 지점은 기존 게이트 + decision card 그대로다. 새 게이트를 만들지 않는다.
- Goal mode의 고정 정책(`NextGoalStep`)은 바꾸지 않는다.

## 용어

| 용어 | 뜻 |
|---|---|
| Bot spec | 봇 정의: id, 이름, 동물(glyph), 역할, 엔진(tier), 도구, 지시문. `<workspace>/focus-crew/*.yaml` 또는 내장 |
| Crew | 파이프라인에 복사된 봇 목록 + 단계 배치 (`Pipeline.Crew`) |
| 소환 / 채용 | 라이브러리의 봇을 쓰기 / 이번 미션용 봇 스펙을 새로 만들기(일회성) |
| 서브턴 | 봇 워커 세션에서 드라이버가 실행하는 턴. 메인 세션 transcript에 들어가지 않음 |
| 허들 | Plan 메인 턴 전에 도는 봇 토론 라운드 |
| Crew log | 봇 간 메시지 기록 `<workspace>/sessions/<id>.crew.jsonl` |
| 주목할 것 | 사용자가 봐야 할 것만 추린 목록 (must-handle 카드 + 새 정보 요약 + 예고) |

## 페이즈 (수직 슬라이스)

| # | 문서 | 끝나면 동작하는 것 | 예상 |
|---|---|---|---|
| 1 | [phase-1-crew-roster.md](./phase-1-crew-roster.md) | 새 태스크 → TARS가 크루 제안(소환+채용) → 확인 후 시작. 단계마다 크루가 표시됨. 턴 동작은 기존과 동일 | 2~3일 |
| 2 | [phase-2-plan-huddle.md](./phase-2-plan-huddle.md) | Plan 단계에서 봇들이 조사·팩트체크·시안 토론 → 다이제스트가 plan 턴에 들어감 → G1. 상세보기 스윔레인 | 3~4일 |
| 3 | [phase-3-cross-review.md](./phase-3-cross-review.md) | Review에서 다른 엔진 리뷰어가 독립 리뷰 + 상호 판정 → finding 카드에 합의/이견 배지 | 2일 |
| 4 | [phase-4-attention.md](./phase-4-attention.md) | "주목할 것" 레일, 말풍선 알림, 홈 리디자인, 허들 다이어그램 애니메이션, B Monolith 테마(focus 한정) | 2~3일 |
| 5 | [phase-5-wrap-lifecycle.md](./phase-5-wrap-lifecycle.md) | 미션 종료 시 크루 해산(보관/은퇴), 워커 세션 정리, 기억 후보 제안, 라이브러리 관리 | 1~2일 |

합계 약 10~14일(1인). 각 페이즈는 `tars-github-flow`로 PR 1~2개.

## 의존 관계

```
P1 크루 모델·편성 ──► P2 허들(서브턴 엔진 + crew log) ──► P3 교차검토(같은 엔진 재사용)
                                     │
                                     └──► P4 주목할 것·시각화 (log를 읽음)
P1 ───────────────────────────────────────► P5 해산·라이브러리
```

P3와 P4는 P2 이후 순서 바꿔도 된다.

## Out of Scope (이번 로드맵에서 하지 않음)

- `agentruntime` consensus/override 연동 (결정: 서브턴 방식)
- 봇이 코드를 직접 수정 (빌드는 메인 세션만)
- 지시 입력의 `@봇이름` 라우팅 → 다음 단계 후보 (P4에서 입력란 placeholder만)
- 프로토타이퍼가 실행 가능한 코드 생성 — 텍스트/마크다운 시안만
- Goal mode 정책 변경 (이견 finding 자동 처리 등)
- 모바일 화면, 봇 아바타 커스터마이징 UI
- 봇별 비용 청구 상세 화면 (합계만 표시)

## 문서 갱신 책임

각 페이즈 PR은 다음을 함께 갱신한다.

- `docs/decisions/focus-mode.md` — 새 절 "§11 Crew" (P1에서 생성, 이후 페이즈가 하위 절 추가)
- `docs/feature-map.md` — Focus mode 아래 해당 기능 항목
- `docs/focus-templates.md` — 사용자용 설명 (P1, P5)
