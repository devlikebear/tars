# CASE Companion — Roadmap

> 이니셔티브 컴패니언 CASE를 **당직 미어캣** 캐릭터로 리디자인하고, 어디서나 TARS 전체 상황을 물어볼 수 있는 어시스턴트로 키운다.
> 디자인 시안: Design 캔버스 E행 — E1 캐릭터(A안 미어캣 확정) · E2 Bridge · E3 먼저 말 걸기 · E4 표정 시트.
> 레포 사본: [`design/`](./design/README.md) (PNG + 색상·토큰 메모)
> 함께 진행 중인 계획: [`../focus-crew/roadmap.md`](../focus-crew/roadmap.md) (크루 동물들과 같은 세계관, CASE는 크루가 아닌 당직자)

## 확정된 결정

1. 캐릭터: **A안 당직 미어캣** (이름 CASE 유지). 서서 망을 보는 자세, 가슴의 상태 칩.
2. 대상: 기존 CASE (`CompanionPet.svelte` + `internal/initiative`)를 리디자인한다. 새 봇을 만들지 않는다.
3. 얼굴 구현: `frontend/console/DESIGN.md` "CASE's expressions" 규칙 **유지** — CSS와 마크업(span)만, 이미지·SVG 에셋 없음. 시안의 SVG는 레퍼런스이고 구현은 span 도형으로 옮긴다.

## 현재 코드 (분석 요약)

| 영역 | 파일 | 현재 동작 |
|---|---|---|
| 얼굴·말풍선 | `frontend/console/src/components/CompanionPet.svelte` (1,623줄) | `.companion-body.expr-<name>`, 안테나·눈썹·눈·입 span, `act-<action>`, idle play, 말풍선(라인·서버 라인·질문) |
| 순수 로직 | `src/lib/companion.ts` | 표정 8종 `companionExpression`, cue 머신, `companionAskContext` |
| 질문 | `src/lib/companionAsk.ts`, `stores/companionAskStore.svelte.ts` | 질문을 **main 세션의 일반 채팅 턴**으로 보냄 (tars#1236), light tier 추천 |
| 먼저 말하기 | `internal/initiative/*` (`policy.go Decide`, `compose.go`, `speak.go`) + `internal/tarsserver/helpers_initiative*.go` | 인사(greet)·안부(check_in)만, 조용한 시간·쿨다운·하루 한도, `category: "companion"` SSE 이벤트로 표정·문장 전달 |
| 앱 도구 | `internal/apptool/*` | `sessions_list`, `sessions_history`, `cron_list`, `cron_runs`, `usage_report`, `agents_list`, 집계형 도구 패턴(`session_aggregator.go`, `tasks_aggregator.go`) |
| 테스트 | `tests/companionPet.test.ts`, `companionAsk*.test.ts`, `e2e/companion-*.spec.ts` (7개) | reduced-motion 셀렉터 검사 포함 |

빠진 것:
- CASE 질문이 main 세션 대화에 섞인다. TARS 전체(파이프라인·cron·승인·비용)를 한 번에 읽는 도구가 없다.
- 돌아왔을 때 "무엇이 바뀌었는지" 요약이 없다 (인사 문장만).

## 페이즈

| # | 문서 | 끝나면 동작하는 것 | 예상 |
|---|---|---|---|
| 1 | [phase-1-meerkat-face.md](./phase-1-meerkat-face.md) | 모든 화면의 CASE가 미어캣으로 바뀜. 표정 8종·동작·cue 동작은 그대로 | 1~2일 |
| 2 | [phase-2-bridge.md](./phase-2-bridge.md) | ⌘J / 클릭 → Bridge 패널. 전용 읽기 전용 세션 + `tars_status` 도구로 TARS 전체 상황 답변, 라이브 카드·출처·후속 질문 | 3~4일 |
| 3 | [phase-3-arrival-brief.md](./phase-3-arrival-brief.md) | 돌아왔을 때 "그동안 바뀐 것" 요약 말풍선, 안부의 "20분 뒤 다시"·"오늘은 조용히" | 2일 |

Focus Crew 로드맵과 독립적이다. Crew Phase 4(주목할 것)가 먼저 끝나 있으면 Bridge 카드가 크루 정보를 함께 보여 줄 수 있지만 필수는 아니다.

## 불변 조건

- 실제 상태(대기·실패·실행 중) 라인이 항상 표정 cue보다 우선한다 (`companionExpression` 규칙 유지).
- 표정은 모양으로 구분하고, 상태색은 가슴 칩에만 쓴다. 이웃 표정과 최소 두 군데가 다르다.
- 모든 반복·장식 애니메이션은 `prefers-reduced-motion: reduce`에서 멈춘다.
- CASE는 **읽기 전용**이다. 무언가를 바꾸는 일은 링크로 해당 화면에 데려가는 것까지만 한다.
- 이니셔티브 정책(`Decide`)의 침묵 조건(조용히 요청, 오늘 멈춤, 조용한 시간, 쿨다운, 하루 한도, 바쁨)은 그대로다.

## Out of Scope

- 텔레그램 쪽 CASE 표현 변경 (텍스트 동작 그대로)
- 음성, 다국어 추가(en/ko 외)
- CASE가 게이트 승인·cron 실행 등 쓰기 동작을 직접 수행
- 크루 동물 아바타와 공용 아바타 컴포넌트 통합 (Crew Phase 1의 `CrewAvatar`는 SVG 허용 여부를 그쪽에서 따로 결정)

## 문서 갱신 책임

- `frontend/console/DESIGN.md` "CASE's expressions" — 미어캣 부위 이름과 표정별 모양 표 (P1)
- `docs/decisions/` — CASE 질문 경로 변경 ADR (P2: main 세션 → 전용 세션)
- `docs/feature-map.md` — Bridge, arrival brief 항목
