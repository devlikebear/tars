# Phase 3 — 돌아왔을 때 요약과 안부 (E3)

> Roadmap: [roadmap.md](./roadmap.md) · 선행: Phase 2 (`tars_status` 재사용)
> 목표: 이니셔티브의 greet에 **"그동안 바뀐 것" 요약**을 붙이고, check-in에 "20분 뒤 다시"와 "오늘은 조용히" 동작을 붙인다. 말을 걸지 결정하는 정책(`initiative.Decide`)은 바꾸지 않는다.

## 완료 조건

- 사용자가 떠날 때(presence away) 상태 스냅샷을 저장하고, 돌아왔을 때 greet가 나가면 스냅샷과 현재의 **차이**를 최대 3줄로 말풍선에 붙인다.
- 차이 계산은 결정론적(순수 함수)이다. LLM은 기존처럼 첫 문장만 만든다.
- 각 줄은 클릭하면 해당 화면으로 이동한다 (Phase 2의 링크 규칙).
- check-in 말풍선 버튼: "괜찮아"(닫기), "20분 뒤 다시"(해당 시각까지 check-in 쿨다운), "오늘은 조용히"(기존 `StoppedToday`).
- 침묵 조건은 모두 그대로 동작한다.

## 설계

### 스냅샷과 차이 (`internal/initiative/brief.go`, 순수)

```go
type StatusSnapshot struct {
    At        time.Time
    Pipelines map[string]PipelinePoint // id → {stage, open_gate, finished}
    Failed    []string                 // 실패한 세션/cron id
    Approvals int
}

type BriefItem struct {
    Kind     string // advanced | waiting | finished | failed | recovered
    Severity string // block | info
    Text     string // 서버가 만든 한국어/영어 한 줄 (모델 아님)
    Ref      CardRef
}

func Brief(prev, now StatusSnapshot, locale string) []BriefItem // block 먼저, 최대 3
```

- 스냅샷 소스: Phase 2의 `tars_status` 집계 함수(도구가 아닌 내부 함수)를 그대로 호출
- 저장: presence가 away로 바뀔 때 `<workspace>/initiative/snapshot.json` (기존 ledger 옆, atomicwrite)
- 테스트: 단계 진행, 새 게이트 대기, 완료, 실패→복구, 변화 없음(빈 목록 → 요약 없이 인사만), 정렬·3개 제한

### 이벤트 확장

- `helpers_initiative_speak.go sendCompanionEvent`: `category: "companion"` 이벤트에 `brief: []BriefItem`, `actions: ["dismiss","snooze_20m","quiet_today"]` 필드 추가 (없으면 기존과 동일)
- `POST /v1/initiative/snooze {minutes}`: check-in 쿨다운 설정 (기존 cooldown 저장소 재사용)
- "오늘은 조용히": 기존 stopped-today 경로 호출 (새 저장소 만들지 않음)

### 콘솔

- `lib/companion.ts`: `CompanionServerCue`에 `brief`, `actions` 파싱 (검증 실패 시 무시 — 기존 #1192 검증 규칙)
- `CompanionPet.svelte` 말풍선: 서버 라인 아래 brief 목록(상태 칩 + 한 줄, 링크), 버튼 행(E3). greeting 표정 + wave 동작은 기존 cue 머신 그대로
- i18n en/ko

## 작업

- [x] `brief.go` + `brief_test.go`
- [x] presence away 시 스냅샷 저장 (`presence.go` 훅 지점) + 테스트 — 구현: 존재 신호마다 `BriefStore.Seen`
- [x] greet 발화 시 brief 첨부, check-in 시 actions 첨부 (`helpers_initiative_speak.go`) + 테스트
- [x] `POST /v1/initiative/snooze` + 테스트 — 구현: `POST /v1/initiative/checkin/answer`
- [x] 콘솔 파싱·말풍선 UI + `tests/companionPet.test.ts` 케이스 + `e2e/companion-events.spec.ts`에 brief 시나리오 (e2e 이벤트 훅 `e2e_events.go` 사용), `e2e/initiative-live.spec.ts`에 실제 check-in의 snooze
- [x] feature-map "Arrival brief"

### 구현하면서 바뀐 점 (현재 코드 기준)

- **스냅샷 소스**: `tars_status` 도구는 없다(Phase 2는 도구가 아닌 `internal/casebridge` 스냅샷 + `<tars-status>` 턴 맥락). 그 집계(`caseBridge.pipelinesLocked`/`activityLocked`)를 재사용하는 `caseBridge.briefSnapshot`이 `initiative.StatusSnapshot`을 만든다. cron은 Bridge의 "최근 24시간 실패"가 아니라 모든 작업의 마지막 실행 실패 여부를 읽는다 — 24시간 창에서 빠진 것을 "복구"로 읽지 않기 위해서. usage·pulse는 읽지 않는다.
- **저장 시점·위치**: away로 바뀌는 순간은 이벤트가 없다(신호가 10분 끊겨야 안다). 대신 존재 신호마다 "마지막으로 본 상태"(Seen)를 분당 최대 1회 갱신하고, 새 도착의 첫 신호에서 그것을 Away로 얼린다 — 기준이 떠난 순간이 된다. `<workspace>/_shared/initiative/snapshot.json`(ledger 옆, tmp+rename).
- **BriefItem**: `Ref CardRef` 대신 서버가 만든 `Href`. 이벤트에는 `text: {en, ko}`로 두 언어를 싣고 콘솔이 고른다(서버는 콘솔 언어를 모른다). "승인 N건이 기다려요" 줄을 더했다(승인 수가 늘었을 때).
- **버튼 엔드포인트**: `POST /v1/initiative/checkin/answer {entry_id, action: ok|snooze|quiet_today}` — confirm_memory의 `/v1/initiative/memory/answer` 모양. 답은 ledger `answer` 줄로 남아 재시작 후 복원된다. "괜찮아"도 서버에 보낸다(답장으로 세어 ignored 연속에 들어가지 않도록).
- **"20분 뒤 다시"의 뜻**: 전달된 check-in 뒤에는 원래 3시간 간격(`CheckedInRecently`)과 쿨다운이 걸려 있어 "20분 동안 없음"만으로는 아무것도 바뀌지 않는다. 그래서 snooze는 그 check-in의 3시간 간격을 snooze 끝(20분)으로 바꾼다 — 20분 동안은 없고, 그 뒤 조건이 맞으면 일찍 다시 물을 수 있다. 쿨다운(기본 45분)과 다른 침묵 조건은 그대로다. `Decide`는 바꾸지 않았다(신호 파생만).
- **설정**: `initiative.arrival_brief`, `initiative.check_in_buttons`(live 모드, 기본 true).

---

### ✅ Checkpoint: Phase 3

**자동 확인**
- [ ] `make test-one TEST_NAME='Brief|Snapshot|Snooze' PKG=./internal/...`, `make test`
- [ ] `make console-e2e` 전체 (CompanionPet 변경)
- [ ] `make lint-diff && make test-diff`

**수동 확인**
- [ ] 콘솔 탭을 10분 이상 숨김 → 그동안 파이프라인 하나가 게이트에 도착 → 돌아오면 인사 + "G2에서 기다려요" 줄, 클릭 시 이동
- [ ] 변화 없음 → 인사만, 요약 없음
- [ ] check-in에서 "20분 뒤 다시" → 20분 동안 check-in 없음, greet는 영향 없음
- [ ] 조용한 시간대·오늘 멈춤에서는 아무 말도 안 함

**통과하면 로드맵 완료. 사용자와 회고.**
