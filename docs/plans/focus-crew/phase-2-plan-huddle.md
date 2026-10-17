# Phase 2 — Plan 허들과 Crew log (D3 핵심 · D6)

> **철회됨 (2026-10-17).** 진행하지 않는다. 이유는 [roadmap.md](./roadmap.md)의 "철회 결정".

> Roadmap: [roadmap.md](./roadmap.md) · 선행: Phase 1
> 목표: Plan 단계의 첫 메인 턴 전에 봇들이 **조사 → 팩트체크 → 시안/추정** 라운드를 돌고, 결과 다이제스트가 plan 턴에 들어가 G1으로 이어진다. 모든 봇 메시지는 crew log에 남고 상세보기(스윔레인)에서 본다.

## 완료 조건

- 크루에 plan 참여 봇이 있으면 허들이 돈다. 없으면 기존과 동일.
- 허들은 최대 2라운드, 봇 서브턴은 읽기 전용, 연구원 봇만 웹 도구를 쓴다.
- 메인 plan 턴의 `<focus-stage>`에 `<focus-crew-digest>`가 붙고, 결과는 기존 `<focus-plan>` → G1. **전이 규칙 변화 없음.**
- 허들 결과 요약이 report 카드 1장으로 덱에 들어간다 (acknowledge 가능).
- `GET /v1/focus/pipelines/{id}/crew/log`와 상세보기 화면(D6).

## 설계

### 봇 세션 (`internal/tarsserver/focus_crew_session.go`)

`qaSession()`을 일반화한 `crewSession(pipelineID, member, parent)`:

- 없으면 `CreateWithOptions("<봇이름> · <세션 제목>", "worker", true)`, `SetIsolation(off)`
- 매 사용 전: `SetPermissionMode(plan)`, `SetTierPin(member.Bot.Tier)`(빈 값이면 생략), `SetPromptOverride(botPrompt(member))`, `SetToolConfig(&SessionToolConfig{ToolsCustom: true, ToolsEnabled: readTools ∪ member.Bot.Tools})`, workdir를 부모와 동기화
- 세션 id는 `Pipeline.Crew[i].SessionID`에 기록 (store.Update 한 번, qaSession과 같은 락 규칙)
- 턴 실행: `runServerChatTurnAs(..., chatTurnOrigin{unattended: "focus_crew", readOnly: true})` — `focusDriver.bind()`에 `crewTurn` 러너 추가

### 봇 출력 블록 (`internal/focuspipeline/crew_blocks.go`)

봇 서브턴은 끝에 블록 하나를 낸다. 파서는 `blocks.go` 스타일로, 누락·불량은 재요청 1회 후 "응답 없음" 처리 (허들을 멈추지 않음).

```
<crew-note>{"summary":"…","claims":[{"id":"c1","text":"…","sources":["https://…"]}],"proposal":"…","questions":["…"]}</crew-note>
<crew-check>{"verdicts":[{"claim":"c1","verdict":"supported|unsupported|unclear","why":"…"}]}</crew-check>
```

### 허들 오케스트레이션 (`internal/tarsserver/focus_huddle.go`)

드라이버 `loop()`에서 **plan stage의 첫 `send_turn` 직전**(`p.Plan == nil && stage.Iteration == 1 && 허들 미완`)에 `runHuddle(run)`:

1. Round 1 (병렬, 최대 3): researcher·prototyper·builder 역할 봇에게 목표 + 역할 지시 → `<crew-note>`
2. Round 2: checker 봇에게 Round 1의 claims 전체 → `<crew-check>`. researcher에게 unsupported 판정을 돌려줘 정정 노트 1회(선택, 예산 내)
3. `HuddleDigest(notes, checks) string` (순수, `focuspipeline`) — 판정 반영(unsupported는 "기각됨"으로 표기), 출처 목록, 질문 목록, 4000B 이하
4. `Pipeline.Huddle = &HuddleState{Done: true, Rounds, Digest, Stats}` 저장 → 이후 `Guidance`가 plan stage에서 `<focus-crew-digest>`로 포함
5. report 카드 1장: 제목 "크루 허들 요약", payload `{claims_total, rejected, sources, proposals, questions}`

제약:
- 예산: `focus.crew.huddle_timeout`(기본 6분), 봇 턴당 2분. 초과 시 모인 것만으로 다이제스트 (부분 허들도 정상)
- 봇의 `questions`는 다이제스트에 넣기만 한다. **사용자에게 물을지는 메인 plan 턴이 `<focus-report>` decisions로 결정** (기존 decision card 경로)
- Goal mode에서도 동일하게 돈다 (결정은 기존 정책이 처리)
- 재시작: `HuddleState`가 미완이면 허들을 처음부터 다시 (서브턴은 읽기 전용이라 안전), interrupted gate 규칙은 메인 턴에만 적용

### Crew log (`internal/focuspipeline/crew_log.go`)

`<workspace>/sessions/<id>.crew.jsonl`, append-only:

```json
{"ts":"…","stage":"plan","round":1,"from":"tars","to":"hoot","kind":"command|note|check|verdict|finding|handoff|digest","text":"한 줄 요약","ref":{"session_id":"…","turn":3},"payload":{}}
```

- 드라이버가 서브턴 전(command)·후(note/check)에 기록. `text`는 서버가 블록에서 만든 한 줄 요약 (모델 원문 아님)
- `GET /v1/focus/pipelines/{id}/crew/log?stage=plan` → `{entries}`; 원문은 `ref`로 해당 워커 세션 history에서 읽음 (기존 session history API)

## 작업

### A. 순수 로직
- [ ] `crew_blocks.go` + 테스트: 정상/누락/깨진 JSON/과대 크기
- [ ] `HuddleState`, `HuddleDigest()` + 테스트: 기각 표기, 4000B 절단, 출처 중복 제거
- [ ] `guidance.go`: plan stage이고 `p.Huddle.Done`이면 다이제스트 블록 추가 + "다이제스트의 기각된 주장을 근거로 쓰지 말 것" 지시. 테스트: 크루 없는 파이프라인 guidance가 기존과 바이트 동일
- [ ] `crew_log.go`: `AppendCrewLog`, `ReadCrewLog(stage)` + 테스트

### B. 서버
- [ ] `focus_crew_session.go` (+ 테스트: 생성, 재사용, 권한·도구·tier 적용)
- [ ] `focus_huddle.go`: `runHuddle` (주입 가능한 `crewTurn` 러너로 단위 테스트: 라운드 순서, 타임아웃 시 부분 다이제스트, 블록 누락 1회 재요청)
- [ ] `focus_driver.go loop()`에 허들 훅 1곳 + 테스트 (크루 없음 → 호출 안 됨)
- [ ] 라우트 `GET /v1/focus/pipelines/{id}/crew/log`
- [ ] config `focus.crew.huddle_timeout`, `focus.crew.max_parallel` (기존 config 필드 추가 패턴, `internal/config`)
- [ ] mock LLM `[e2e:focus-huddle]`: 워커 세션 프롬프트에 역할별 `<crew-note>`/`<crew-check>` 응답, 하나는 unsupported

### C. 콘솔
- [ ] `lib/focusCrew.ts`: `swimlane(entries, crew)` → 레인 목록 + 메시지(from/to 인덱스, 화살표 방향, 강조 여부); `huddleSummary(card)`
- [ ] `FocusCrewDetail.svelte` (D6): 단계 탭, 레인 헤더(아바타), 메시지 화살표(줄 단위 grid), 선택 시 오른쪽 원문 패널(`FocusRawSlice`의 렌더러 재사용), "← 주목할 것만 보기"
- [ ] `FocusView` 헤더에 "상세보기" 버튼 → 상세보기 토글 (라우트 `?view=crew` 쿼리, 뒤로가기 지원)
- [ ] Plan 단계 진행 중 패널에 허들 상태 한 줄 (`progressLine` 확장: "크루 허들 · 2라운드 · 출처 12")
- [ ] 테스트: `tests/focusCrew.test.ts`에 swimlane 케이스, `e2e/focusCrew.spec.ts`에 "허들 → 요약 카드 → 상세보기에서 기각 메시지 확인"

### D. 문서
- [ ] ADR §11.1 Huddle: 실행 시점, 블록, 예산, 실패 시 동작, 전이 불변
- [ ] feature-map "Plan huddle", "Crew log / detail view"

---

### ✅ Checkpoint: Phase 2

**자동 확인**
- [ ] `make test` · `make test-one TEST_NAME='Huddle|CrewLog|CrewBlock' PKG=./internal/...`
- [ ] 크루 없는 파이프라인 golden guidance 테스트 통과 (바이트 동일)
- [ ] `npm run check`, `tests/focusCrew.test.ts`, `npx playwright test e2e/focusCrew.spec.ts e2e/focus.spec.ts`
- [ ] `make lint-diff && make test-diff`

**수동 확인** (실제 엔진 + 웹 검색 켠 샌드박스)
- [ ] 보안 정책류 목표로 시작 → 허들 중 progress line 갱신 → G1 계획에 출처가 반영되고 기각된 주장은 근거로 안 쓰임
- [ ] 웹 검색 키 없는 config → researcher가 "검색 불가"를 노트로 남기고 허들은 계속
- [ ] 허들 중 서버 재시작 → 허들 재실행 후 정상 진행
- [ ] 상세보기에서 봇 간 메시지 순서와 원문 확인

**통과하면 사용자 확인 후 Phase 3로.**
