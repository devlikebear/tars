# 세컨드 오피니언 리뷰 — 계획

> 상태: 구현 PR 리뷰 중 (2026-10-17, 브랜치 `feat/second-opinion-review`) · 기준 커밋: `9907a083` (main) · 계획과 달라진 점은 §9
> 진행 관리: #1281 (체크리스트·PR·확인 결과·측정은 이슈에, 설계는 이 문서에. 구현 PR 본문에 `Closes #1281`)
> 출처: 철회한 Focus × Crew의 Phase 3([`../focus-crew/phase-3-cross-review.md`](../focus-crew/phase-3-cross-review.md))를 크루 없이 줄인 것. 철회 이유는 [`../focus-crew/roadmap.md`](../focus-crew/roadmap.md)의 "철회 결정".
> 결정 문서: `docs/decisions/focus-mode.md` (구현 PR에서 §5.1을 새로 쓴다)

## 1. 왜

Focus의 review 단계는 한 모델이 직접 만든 변경을 그 모델이 리뷰한다. 같은 모델은 자기가 놓친 것을 다시 놓치기 쉽다. 지금 focus 코드에는 두 번째 모델이 리뷰하는 경로가 없다 (`review_tier` 같은 설정도 없다).

**파이프라인 근거 (`docs/decisions/product-focus.md` 규칙 1):** 한 모델이 놓친 결함을 머지 전에 다른 모델이 잡는다.

## 2. 무엇이 달라지는가

옵트인한 파이프라인에서만, review kind 단계의 첫 라운드에 다음이 일어난다.

1. 메인 review 턴이 평소처럼 끝나고, 그 턴의 지적이 평소처럼 finding 카드가 된다.
2. 서버가 지정된 tier로 숨은 worker 세션에서 **읽기 전용 리뷰 턴을 한 번** 돌린다. 이 턴은 같은 변경(diff)을 보지만, 메인 리뷰가 무엇을 지적했는지는 보지 않는다(독립성).
3. 그 지적이 같은 라운드의 finding 카드로 들어간다. 메인 리뷰가 이미 지적한 것과 겹치면 새 카드를 만들지 않고, 기존 카드에 "두 리뷰 모두 지적" 표시를 단다.
4. 그 뒤로는 오늘과 같다: 카드가 있으면 triage(G2), 없으면 검증.

옵트인하지 않은 파이프라인은 바이트 단위로 오늘과 같다.

## 3. 불변 조건

- **기본은 꺼짐.** 파이프라인을 만들 때 고른다(e2e와 같은 원칙). 템플릿에 넣지 않고, plan 수정으로도 켜거나 끄지 않는다.
- **단계 전이는 사실로만 결정한다.** 세컨드 오피니언은 카드를 더할 뿐이다. 그 뒤 분기(triage를 열지, 검증을 돌릴지)는 오늘과 같은 규칙(`reviewTurnAction`)이 같은 입력으로 정한다. 모델에게 "합의했냐"고 묻지 않는다.
- **읽기 전용.** 세컨드 오피니언 턴은 plan 권한, worker 세션, isolation off다. 코드를 바꾸는 건 여전히 메인 세션의 수정 턴뿐이다.
- **막지 않는다.** 실패·타임아웃·블록 누락은 notice 카드 한 장을 남기고, 세컨드 오피니언이 없었던 것처럼 진행한다. 자동 재시도는 없다.
- **목표 모드의 고정 정책(`NextGoalStep`)은 바꾸지 않는다.** 세컨드 오피니언 카드도 기존 `goalFindingDecision`으로 처리한다(`low`는 dismiss, 나머지는 fix).
- **새 config 필드 0개, 새 채팅 도구 0개.** 시간 제한 같은 값은 상수다. 설정 필드 분류(#1240)를 늘리지 않고, 다른 채팅의 시스템 프롬프트도 늘리지 않는다.
- **Focus 화면 구조는 그대로.** New task의 체크박스 하나, 카드 배지, 진행 줄 한 줄만 더한다.

## 4. 설계

### 4.1 옵트인과 tier

- `POST /v1/focus/pipelines`에 `second_opinion: {tier: "heavy"|"standard"|"light"}`를 받는다. 없으면 꺼짐이다.
- tier는 세션 tier 고정과 같은 규칙(`llm.ParseTier`)이다. 그래서 v1에서 고를 수 있는 건 heavy/standard/light뿐이다. `handler_session.go`의 `tier_pin` 처리와 같다.
- 생성할 때 `config.ResolveLLMTier`로 풀어 본다:
  - 풀리지 않으면 400.
  - provider kind가 `antigravity-cli`면 400. 읽기 전용을 강제할 수 없다(computer use·initiative와 같은 제외).
  - 메인 채팅 tier(`chat_main` 역할 → 없으면 `llm_default_tier`)와 **provider alias와 model이 둘 다 같으면** 경고를 응답에 싣는다(거절하지 않음). 같은 모델이면 세컨드 오피니언의 의미가 약하다.
- 저장: `Pipeline.SecondOpinion *SecondOpinion` (`omitempty`)

```go
// internal/focuspipeline/model.go
type SecondOpinion struct {
    Tier      string `json:"tier"`                 // heavy | standard | light
    SessionID string `json:"session_id,omitempty"` // 숨은 worker 세션, 처음 돌 때 만든다
    // Done lists the review-kind stage ids whose first round already had
    // its second opinion, so a stage gets one per pipeline run.
    Done []StageID `json:"done,omitempty"`
}
```

- 메인 tier와 다른 **회사의** 모델을 쓰려면, 사용자가 heavy/standard/light 중 하나를 다른 provider에 매핑해 두어야 한다(provider pool). 임의 tier 이름을 받는 건 세션 tier 고정이 아직 못 하는 일이라 v1 범위 밖이다.

### 4.2 언제 도는가 — review 라운드가 세컨드 오피니언을 기다린다

**자리:** `internal/focuspipeline/review.go`의 `reviewTurnAction`. 이 함수는 메인 review 턴(수정 턴이 아닌 것)의 지적을 카드로 만든 뒤, triage를 열거나 검증을 돌린다. 다음 조건이 모두 참이면 그 분기 대신 세컨드 오피니언을 요청한다.

- `p.SecondOpinion != nil`
- 지금 단계가 review kind이고 이 단계 id가 `SecondOpinion.Done`에 없다(= 이 단계의 첫 라운드)
- 이 턴이 수정 턴이 아니다(`!p.Review.Fixing`)

그때:

```go
p.Review.SecondOpinionOwed = true          // ReviewState에 추가, 빚진 단계
return Action{Kind: ActionSecondOpinion}   // machine.go에 추가
```

드라이버가 이 액션을 실행한 뒤 결과를 새 이벤트 `EventSecondOpinion{Findings, Outcome}`로 `Apply`에 넣는다. 이벤트 처리는 다음 순서다.

1. 지적을 카드로 만든다(4.4)
2. 단계 id를 `Done`에 넣는다
3. `SecondOpinionOwed`를 끈다
4. **`reviewTurnAction`의 원래 꼬리를 그대로 탄다**: `Review.Triage`가 비었으면 검증, 아니면 `openTriage()`

새 게이트도, 새 전이도 없다. 같은 분기를 세컨드 오피니언이 끝난 뒤에 탈 뿐이다.

**검토했지만 쓰지 않은 두 자리:**
- `focusAfterTurn`에서 `Apply` 전에 블록을 고치는 자리(`focusEnrichFindings`가 있는 곳): 이 함수는 채팅 턴이 메인 세션 claim을 잡은 채로 부른다(`handler_chat_pipeline.go`의 `endClaim()` 앞). 여기서 몇 분짜리 서브턴을 돌리면 그동안 사용자가 그 세션에 아무것도 보낼 수 없다.
- 다 끝난 뒤 `AddFinding`으로 넣는 방식: 메인 리뷰 지적이 0개면 검증이 바로 돈다. 검증이 세컨드 오피니언보다 먼저 통과하면 단계가 넘어가고, 늦게 온 카드는 `ErrCannotAddFinding`으로 버려진다.

### 4.3 서브턴 실행 (`internal/tarsserver/focus_second_opinion.go`)

**세션:** `focus_qa.go`의 `qaSession()`을 본뜬 `secondOpinionSession(pipelineID, parent)`.
- 없으면 `CreateWithOptions("<세컨드 오피니언> · <세션 제목>", "worker", true)`를 만들고 `SetIsolation(session.IsolationOff)`를 건다.
- 매번 `SetPermissionMode(chatPermissionModePlan)`, `SetTierPin(tier)`, workdir를 부모와 동기화한다.
- 세션 id는 `SecondOpinion.SessionID`에 기록한다(store.Update 한 번, qaSession과 같은 락 규칙).

**턴:** 드라이버 `bind()`에 `secondOpinionTurn` 러너를 더한다(`qaTurn`과 같은 모양).

```go
runServerChatTurnAs(ctx, deps, soSessionID, prompt, "", chatTurnOrigin{unattended: focusSecondOpinionSource, readOnly: true})
```

- `focusSecondOpinionSource = "focus_second_opinion"`
- 드라이버 `loop()`의 `switch act.Kind`에 `ActionSecondOpinion`을 더하고, `stepAllowed`는 `SecondOpinionOwed`일 때만 이 액션을 통과시킨다.

**프롬프트:** 서버가 사실로 채운다. 메인 리뷰의 지적은 넣지 않는다.
- 이 단계의 review 지시문(`stageInstruction`의 review 부분)
- 단계의 이전 dismissed 목록(`dismissedList`)
- **서버가 만든 diff**: 새 함수 `focusprobe.ReviewDiff(ctx, dir, base)`. `git diff <BaseCommit>`(작업 트리 포함) + 추적되지 않는 파일 목록이고, 상한은 60KB다. 넘으면 파일별 `--stat`과 "잘림" 표시를 넣는다. git만 읽는 코드라 CLAUDE.md 규칙대로 `focusprobe`에 둔다.
- "파일을 바꾸지 말 것. 답은 `<focus-findings>` 블록 하나."

**상한:** 턴당 10분(상수 `focusSecondOpinionTimeout`). 블록이 없으면 같은 세션에 "블록으로만 답하라"를 한 번 더 묻는다. 그래도 없으면 실패다.

**git은 락 밖에서:** diff와 카드 excerpt(`focusprobe.FindingExcerpts`)는 `store.Update` 밖에서 만든다. 지금 review 코드와 같은 규칙이다.

### 4.4 결과 병합 (순수, `internal/focuspipeline/second_opinion.go`)

`applySecondOpinion(p, findings, outcome, now)`:

- 파서는 기존 `ParseBlocks(reply).Findings`다. 모델이 보낸 `source`는 파서가 이미 지운다. 서버가 `Source = FindingSourceSecondOpinion`(`"second_opinion"`)을 붙인다.
- `p.Review.dismissed(f)`에 걸리는 지적은 버린다(기존 규칙).
- **겹침 판정(결정론):** 같은 라운드의 메인 리뷰 finding 카드 중에서 다음을 모두 만족하면 겹침이다.
  - 같은 `File`(공백 정리 후 정확히 일치)
  - 제목이 대소문자 무시로 같거나, 둘 다 `Line > 0`이고 줄 차이가 3 이하
  - 겹치면 새 카드를 만들지 않는다. 그 메인 카드의 payload에 `confirmed_by: ["second_opinion"]`을 더한다(`Finding`에 `ConfirmedBy []string`, `omitempty`).
- 겹치지 않는 지적은 `CardFinding` 카드가 되어 `Review.Triage`에 들어간다. 라운드당 최대 10장이고, 넘친 수는 notice 카드 문구에 쓴다.
- 수정 프롬프트(`fixFindingsPrompt`)는 `developerMark`처럼 출처 표시를 붙인다: `(second opinion: <tier>)`.

### 4.5 실패와 중단

**실패:** `Outcome`이 `timeout` / `error` / `no_block` / `unavailable` 중 하나면 다음과 같이 처리한다.
- notice 카드 한 장을 남긴다. 제목은 "세컨드 오피니언을 받지 못했다", payload는 `{reason: <code>, tier}`다. 오류 원문은 넣지 않는다.
- 그 뒤는 지적 0개와 같이 진행하고, `Done`에 단계 id를 넣어 다시 돌리지 않는다.

**재시작:** `verification.go`의 `Interrupt`가 `SecondOpinionOwed`를 `AwaitingVerification`처럼 본다. 그래서 interrupted 게이트가 열리고, `BlockedFact.SecondOpinion: true`가 된다. retry는 세컨드 오피니언을 다시 돌린다(읽기 전용이라 안전하다). `resumable`은 그대로다.

**중지·취소:** `applyBlockedGate`의 stop, 파이프라인 중지, `POST /v1/chat/cancel`은 `SecondOpinionOwed`를 지운다. 실행 중인 서브턴은 run ctx로 취소한다.

**목표 모드:** 정책은 바꾸지 않는다. 빚진 단계가 떨어진 경우를 보는 조건(`p.PendingTurn != "" || p.AwaitingVerification` → `GoalInterrupt`)에 `p.Review.SecondOpinionOwed`를 하나 더한다. 그래야 재시작 뒤 목표 모드가 멈추지 않는다.

**세션 삭제:** `focusPipelineCleanup`이 Q&A 세션처럼 `SecondOpinion.SessionID`도 지운다.

### 4.6 콘솔

- **`FocusNewTask.svelte`:** e2e 체크박스 옆에 "세컨드 오피니언 리뷰" 체크박스를 둔다. 켜면 tier 선택이 나온다. 선택지는 `chatSession.tierOptions` 중 `isPinnableTier`인 것이고, 각 줄에 provider·model을 보여 준다. 생성 응답의 경고(메인과 같은 모델)를 상태 줄에 보여 준다. 기본은 꺼짐이고, 켤 때 기본 tier는 메인과 다른 첫 tier다.
- **`FocusCard.svelte`:** finding 카드에 배지를 단다. 기존 `fromDeveloper` 배지와 같은 자리다.
  - `source === 'second_opinion'` → "세컨드 오피니언"
  - `confirmed_by`에 `second_opinion`이 있으면 → "두 리뷰 모두 지적"
- **`lib/focus.ts` `progressLine`:** `SecondOpinionOwed`일 때 "세컨드 오피니언 리뷰 중 · <tier>"를 보여 준다.
- **notice 카드:** 기존 렌더러를 그대로 쓴다.
- i18n en/ko는 `focus.*` 섹션에 넣는다.
- 전역 컴포넌트·`app.css`는 건드리지 않는다. 그래서 e2e는 focus 스펙 범위로 충분하다.

### 4.7 비용

파이프라인당 review kind 단계 수만큼 서브턴을 한 번씩 더 돈다(`dev` 템플릿이면 1회, 블록 누락 재요청 포함 최대 2회). 사용량은 `runServerChatTurnAs`를 통해 기존 usage tracker에 잡히고, 세션 비용에도 들어간다. 라운드마다 돌리지 않는 이유는 비용이다. ADR의 e2e 교훈(목표 모드에서 사용 한도를 다 씀)을 되풀이하지 않는다.

## 5. 작업

### A. 순수 로직 (`internal/focuspipeline`)
- [ ] `model.go`: `SecondOpinion`, `Pipeline.SecondOpinion`, `ReviewState.SecondOpinionOwed`, `Finding.ConfirmedBy`, `FindingSourceSecondOpinion`, `ActionSecondOpinion`, `EventSecondOpinion`. clone 갱신
- [ ] `review.go` `reviewTurnAction`: 4.2의 조건 분기. 원래 꼬리(검증/`openTriage`)는 함수로 빼서 두 곳이 같은 코드를 탄다
- [ ] `second_opinion.go`: `applySecondOpinion`, 겹침 판정, 10장 상한, 실패 notice
- [ ] `machine.go`: `EventSecondOpinion` 처리(활성 review 단계 + `SecondOpinionOwed`일 때만, 아니면 stale로 무시)
- [ ] `verification.go`: `Interrupt`·`applyBlockedGate`(stop·retry)·`BlockedFact.SecondOpinion`
- [ ] `goal.go`: GoalInterrupt 조건에 `SecondOpinionOwed`
- [ ] 테스트(table-driven):
  - 옵트인 없음 → 기존 golden과 동일(guidance·pipeline JSON 바이트 동일)
  - 첫 라운드만 요청하고 수정 뒤 라운드는 요청하지 않음
  - 메인 0개 + 세컨드 0개 → 검증
  - 메인 0개 + 세컨드 1개 → triage
  - 겹침(제목 일치·줄 ±3·다른 파일은 겹침 아님)
  - dismissed 재등장 차단
  - 10장 상한
  - 실패 4종 → notice + 진행
  - 재시작 → interrupted → retry → 재요청
  - stop → 빚 해제
  - 목표 모드에서 멈춤 없음
- 검증: `make test-one TEST_NAME='SecondOpinion|Review|Interrupt|Goal' PKG=./internal/focuspipeline/`

### B. 서버
- [ ] `focusprobe.ReviewDiff` + 테스트(작업 트리 변경 포함, 추적 안 된 파일, 60KB 절단). Windows에서도 도는지 확인(focusprobe는 windows-test 대상)
- [ ] `focus_pipeline.go` 생성: `second_opinion` 필드, 4.1 검증(400 두 가지, 경고 하나)
- [ ] `focus_second_opinion.go`: 세션 준비, 프롬프트, 10분 상한, 블록 누락 재요청 1회, 결과 `Apply`(git은 락 밖)
- [ ] `focus_driver.go`: `bind()` 러너, `loop()` 분기, `stepAllowed`
- [ ] `focus_pipeline.go` `focusPipelineCleanup`: 세컨드 오피니언 세션 삭제
- [ ] mock LLM `e2e/mock-llm.mjs`에 `[e2e:focus-second-opinion]` 마커를 둔다. worker 프롬프트에 메인과 겹치는 지적 1개 + 새 지적 1개로 답하고, 변형 하나는 블록 없이 답한다
- [ ] 테스트 `focus_second_opinion_test.go`(주입 러너): 정상, 타임아웃, 블록 누락→재요청→성공/실패, tier 검증 400, 같은 모델 경고, 세션 재사용, 세션 삭제

### C. 콘솔
- [ ] `FocusNewTask.svelte` 체크박스 + tier 선택 + 경고
- [ ] `FocusCard.svelte` 배지 두 개
- [ ] `lib/focus.ts` `progressLine` 한 줄
- [ ] i18n en/ko
- [ ] 단위 테스트 `tests/focus.test.ts`(progressLine, 배지 조건)
- [ ] e2e `e2e/focus.spec.ts`에 시나리오 하나(en/ko): "세컨드 오피니언 켜고 시작 → review → 새 카드에 배지, 겹친 카드에 '두 리뷰 모두'"

### D. 문서
- [ ] `docs/decisions/focus-mode.md` §5.1 Second opinion: 옵트인, 시점(첫 라운드), 독립성, 겹침 규칙, 실패 처리, 전이 불변의 근거, 쓰지 않은 두 자리와 이유
- [ ] `docs/feature-map.md` Focus mode 아래 "Second opinion review"(Reach / Drive / Healthy result / Evidence)
- [ ] `CLAUDE.md` Focus 절에 한 단락(기본 꺼짐을 다시 켜지 말 것, 라운드마다 돌리지 말 것 — 측정 전까지)
- [ ] `CHANGELOG.md` Unreleased

## 6. 측정과 재평가

몇 주 쓴 뒤, 세컨드 오피니언 카드가 실제로 고쳐졌는지 본다. 파이프라인 파일은 `<workspace>/sessions/<id>.pipeline.json`이다.

```bash
jq -r '.cards[]? | select(.kind=="finding") | (.payload | fromjson? // .) as $f | select($f.source=="second_opinion" or (($f.confirmed_by // []) | index("second_opinion"))) | [.decision // "undecided", $f.severity, (if $f.source=="second_opinion" then "new" else "confirmed" end)] | @tsv' <workspace>/sessions/*.pipeline.json | sort | uniq -c
```

판단 기준(옵트인 파이프라인 10개 이상 모였을 때):
- 새 카드(`new`)의 fix 비율이 20% 미만 → 세컨드 오피니언이 소음이다. 기본 꺼짐을 유지하고, 제거를 검토한다.
- 50% 이상 → 수정 뒤 라운드에도 돌릴지(8절 질문) 검토한다.
- `confirmed`만 많고 `new`가 거의 없다 → 두 모델이 같은 걸 본다. 다른 provider로 tier를 바꿔 다시 잰다.

## 7. Out of Scope

- 크루·봇 페르소나·봇 라이브러리·crew log·허들·해산(Focus × Crew 철회분)
- 리뷰어끼리 서로의 지적을 판정하는 상호 판정 라운드(Crew P3의 3단계). 모델 호출이 두 배가 되고, 판정 결과가 전이에 쓰이지 않으니 v1에서는 겹침 판정(결정론)으로 대신한다
- pr_review 단계(CI·PR 리뷰 코멘트가 이미 사실 소스)
- heavy/standard/light 밖의 임의 tier, 리뷰어 여러 명
- 템플릿·config에 세컨드 오피니언 설정
- Focus 화면 레이아웃·테마 변경

## 8. 열린 질문 (구현 중 사용자에게 확인)

- 첫 라운드만 돌리는 기본값으로 충분한지, 수정 뒤 라운드에도 돌릴지 — 6절 측정 뒤 정한다.
- 콘솔의 기본 tier: "메인과 다른 첫 tier"가 맞는지, 아니면 매번 고르게 할지.

## 9. 구현에서 계획과 달라진 점

코드가 이긴다는 원칙대로, 구현하면서 바꾼 것과 그 이유다.

- **`focusprobe.ReviewDiff` → `ReviewDiffOf(ctx, dir, base) (ReviewDiff, bool)`.** 문자열 대신 `{Base, Diff, Truncated, Stat, Untracked}`를 돌려주고, 프롬프트 모양(코드펜스·잘림 안내)은 서버(`focusSecondOpinionChange`)가 만든다. focusprobe는 git 사실만 읽는다는 규칙 그대로다. git 호출 하나당 20초 상한(`GitTimeout` 2초는 파일 하나짜리 probe용이라 전체 diff에는 짧다).
- **`SecondOpinion` 타입은 `second_opinion.go`에 있다.** `Pipeline.SecondOpinion` 필드만 `model.go`에 있다. 프롬프트도 순수 함수 `focuspipeline.SecondOpinionPrompt`로 두었다(`stageInstruction`이 비공개라서).
- **`POST /v1/chat/cancel`은 `SecondOpinionOwed`를 지우지 않는다(4.5와 다름).** 실행 중인 서브턴만 취소하고, 빚은 검증(`AwaitingVerification`)처럼 남긴다. 지우면 메인 리뷰 카드가 `Review.Triage`에 든 채 게이트도 빚도 없는 라운드가 되어 아무것도 다시 움직이지 않는다. 남겨 두면 재시작 때 interrupted 게이트가 열리고, 사람이 그 단계에서 턴을 보내도 리뷰 턴이면 다시 요청된다. 파이프라인 중지(`EventStop`)·게이트 stop·blocked stop은 계획대로 지운다.
- **서브턴 단계는 파이프라인 세션을 claim한다.** 계획에 없던 결정이다. 검증 단계와 같이 해야 세션이 "실행 중"으로 보이고, 사람의 턴이 선점하고(드라이버 run 취소), `/v1/chat/cancel`이 찾을 수 있다. 4.2가 피한 것은 메인 채팅 턴의 claim 안에서 도는 것(선점이 없음)이고, 드라이버 run 안의 claim은 선점된다. 턴 자체는 worker 세션에서 돈다.
- **진행 줄은 파이프라인 상태가 아니라 턴 피드로 그린다.** 서버가 `focus_progress` 이벤트(`phase: "second_opinion"`, `command: <tier>`)를 보내고 `progressLine`이 검증 단계와 같은 방식으로 읽는다. `progressLine`은 원래 피드만 보는 함수라 같은 길을 탔다.
- **콘솔의 "같은 모델" 경고는 생성 전에 콘솔이 계산한다.** New task는 생성 응답을 받자마자 파이프라인 화면으로 넘어가서 응답의 `second_opinion_warning`을 보여 줄 자리가 없다. 그래서 tier 목록(`listAgentRuntimeSubagents`)의 provider·model을 `default_tier`와 비교해 고를 때 바로 보여 준다. 서버 경고는 그대로 응답에 있다(API 호출자용). 서버는 `chat_main` 역할 → `llm_default_tier` 순으로 비교하고 콘솔은 `default_tier`만 보므로, `chat_main`을 따로 매핑한 설정에서는 둘이 다를 수 있다.
- **tier는 실행할 때도 다시 확인한다.** 생성 뒤 설정이 바뀌어 tier가 풀리지 않거나 antigravity-cli가 되면 `unavailable` notice로 끝난다. 생성 시 400 검증은 계획대로다(드라이버가 tier 확인기·러너를 갖지 못한 서버는 `second_opinion` 요청에 503).
- **notice 카드의 제목과 한 줄은 콘솔이 현지화한다**(`secondOpinionNotice`). 렌더러는 그대로이고 제목·본문 문자열만 바꾼다. interrupted 게이트도 세컨드 오피니언일 때 따로 문구를 쓴다.
- **세컨드 오피니언 카드의 `turn`은 0이다.** 메인 세션 transcript의 턴에서 온 카드가 아니다.
- **골든 테스트는 새로 만들었다.** 저장소에 없었다. 기능보다 먼저 커밋(`30421b0c`)해서, 옵트인 없는 파이프라인의 JSON·가이던스가 그 시점과 같음을 보인다.

---

### ✅ Checkpoint

**자동 확인**
- [ ] `make test` (기존 focus 테스트 전부 포함)
- [ ] `make test-one TEST_NAME='SecondOpinion' PKG=./internal/focuspipeline/` · `PKG=./internal/tarsserver/` · `PKG=./internal/focusprobe/`
- [ ] 옵트인 없는 파이프라인 golden 테스트 바이트 동일
- [ ] `cd frontend/console && npm run check && npm run test:ci`
- [ ] `cd frontend/console && npx playwright test e2e/focus.spec.ts e2e/focusTemplates.spec.ts`
- [ ] `make lint-diff && make test-diff && make test-cover-diff && make arch-check && make security-scan`

**수동 확인** (`make dev-console`, 메인 tier와 다른 provider/model에 매핑한 tier 하나)
- [ ] 세컨드 오피니언 없이 시작 → 오늘과 같은 흐름, `pipeline.json`에 `second_opinion` 없음
- [ ] 켜고 시작 → review 첫 라운드 뒤 진행 줄에 "세컨드 오피니언 리뷰 중", 끝나면 배지 달린 카드가 triage에 함께 나옴
- [ ] 메인과 같은 모델의 tier를 고르면 경고가 보임
- [ ] 세컨드 오피니언 도중 서버 재시작 → interrupted 게이트 → retry로 다시 돌고 진행
- [ ] 같은 시나리오를 목표 모드로 → 멈춤 없이 G4까지
- [ ] Chat 화면 등 다른 화면 변화 없음

**통과하면 사용자에게 확인을 요청한 뒤 머지한다.** 실패하면 실패 항목과 로그를 보고하고, 원인에 합의한 뒤 수정하고 다시 검증한다.
