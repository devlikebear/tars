# Phase 3 — Review 상호검토 (D4)

> Roadmap: [roadmap.md](./roadmap.md) · 선행: Phase 2 (봇 세션·crew log 재사용)
> 목표: review 단계에서 **다른 엔진의 리뷰어 봇**이 독립 리뷰를 하고, 리뷰어끼리 서로의 발견을 판정한다. 결과는 기존 finding 카드에 합의/이견 배지로 붙고, G2 트리아지는 기존 그대로.

## 완료 조건

- 크루에 review 배치된 reviewer/checker 봇이 있으면, 메인 review 턴 후 봇 리뷰 + 상호 판정이 돈다.
- 봇이 찾은 finding은 `AddFinding` 입구로 들어오고 `source: "crew:<bot_id>"`로 표시된다. 모델이 source를 주장할 수는 없다(기존 규칙 유지).
- 같은 file+title(정규화) finding은 새 카드 대신 기존 카드의 `agreed_by`에 합쳐진다.
- 각 finding 카드 payload에 `crew: {found_by, agreed_by[], disputed_by[{bot, why}]}`.
- 콘솔: 카드에 "합의 2/2" / "이견" 배지, 이견 카드는 양쪽 근거를 함께 보여 줌.
- 트리아지 진행 중 봇 판정이 늦게 도착해도 카드만 갱신되고 게이트 상태는 변하지 않는다.

## 설계

### 실행 시점 (`internal/tarsserver/focus_crossreview.go`)

`Apply`가 review report 턴 완료 후 triage를 연(또는 finding 0개로 다음으로 가려는) **직후**, 드라이버가 `runCrossReview(run)`:

1. **독립 리뷰**: review 배치 봇마다 서브턴 — 입력은 기존 review guidance와 같은 diff 범위(`git diff <BaseCommit>...HEAD` + 미커밋, `focus_review.go`의 diff 생성 함수 재사용), 출력은 기존 `<focus-findings>` 블록(같은 파서 재사용)
2. **병합**: `MergeCrewFindings(p, botID, findings)` (순수) — 같은 키면 `agreed_by` 추가, 새 것이면 `AddFinding(..., source=crew:<id>)`. dismissed 목록(이전 라운드)과 같은 키는 버림 (기존 규칙 재사용)
3. **상호 판정**: 각 리뷰어(메인 리뷰 결과 포함)에게 *자기가 찾지 않은* 카드 목록을 주고 `<crew-check>`로 agree/dispute 판정 → `ApplyVerdicts(p, botID, verdicts)` (순수)가 payload의 `agreed_by`/`disputed_by` 갱신
4. crew log에 finding/verdict 항목 기록

주의:
- 메인 review 턴의 finding이 0개였고 봇도 0개면 기존대로 stage 종료. 봇이 1개 이상 찾았으면 **triage를 연다** — 이것은 `AddFinding`의 기존 규칙("triage가 열릴 때 합류")에 따른 결과이며 새 전이가 아니다. 이 경로를 테스트로 고정한다.
- `AddFinding`이 409를 줄 상황(blocked gate 등)이면 봇 finding은 crew log에만 남기고 카드로 만들지 않는다.
- pr_review 단계는 이번 페이즈에서 하지 않는다 (CI·리뷰 코멘트가 이미 사실 소스).

### 순수 로직 변경 (`internal/focuspipeline`)

- [ ] `finding_add.go`: `FindingSourceCrew = "crew:"` prefix 허용 경로 `crewFinding(f, botID)` 추가. 파서가 모델 블록의 source를 버리는 기존 동작 유지 테스트
- [ ] `crew_review.go`: `findingKey(f)`(file + 정규화 title), `MergeCrewFindings`, `ApplyVerdicts`, 카드 payload `crew` 필드 타입
- [ ] 테스트: 중복 병합, 자기 finding 판정 무시, 이견 누적, dismissed 재등장 차단, triage 미오픈 상태에서 추가 시 기존 규칙대로 대기

## 작업

### A. 서버
- [ ] `focus_crossreview.go` + 단위 테스트 (주입 러너): 순서, 타임아웃 시 부분 결과, 블록 누락
- [ ] `focus_driver.go`에 훅 1곳 (review report 턴 처리 직후), 크루 없음 → 미호출 테스트
- [ ] 봇 리뷰 프롬프트: 기존 review stage instruction(`stageInstruction`) + 봇 instructions + "편집하지 말 것"
- [ ] mock LLM `[e2e:focus-crossreview]`: 봇 리뷰가 메인과 같은 finding 1개(합의) + 새 finding 1개, 판정에서 1개 dispute

### B. 콘솔
- [ ] `lib/focusCrew.ts`: `consensusBadge(card, crew)` → `{kind: 'agreed'|'disputed'|'single', text, bots}`
- [ ] `FocusCard.svelte` finding 렌더에 배지 + 이견이면 "양쪽 근거" 접이식 블록 (봇 아바타 + why)
- [ ] `FocusView` review 단계 패널: 상호검토 상태 한 줄 ("리뷰어 2 · 합의 2 · 이견 1")
- [ ] i18n en/ko, `tests/focusCrew.test.ts` 배지 케이스, e2e 시나리오 1개

### C. 문서
- [ ] ADR §11.2 Cross review: 시점, 병합 키, source 규칙, 전이 불변의 근거
- [ ] feature-map "Crew cross review"

---

### ✅ Checkpoint: Phase 3

**자동 확인**
- [ ] `make test`, `make test-one TEST_NAME='CrewFinding|Verdict|CrossReview' PKG=./internal/...`
- [ ] 기존 review/finding 테스트 전부 통과 (`TestFocusDeveloperFinding` 포함)
- [ ] `npm run check`, 콘솔 단위·e2e (`e2e/focusCrew.spec.ts`, `e2e/focus.spec.ts`)
- [ ] `make lint-diff && make test-diff`

**수동 확인**
- [ ] 메인 Claude Code + 봇 Codex 조합으로 review → 같은 문제를 둘이 찾으면 카드 1장 "합의 2/2"
- [ ] 이견 카드에서 양쪽 근거 확인 후 수정/기각 → fix 턴 프롬프트에 기존 형식대로 포함
- [ ] Goal mode에서 같은 시나리오 → 기존 정책대로 처리되고 멈춤 없음

**통과하면 사용자 확인 후 Phase 4로.**
