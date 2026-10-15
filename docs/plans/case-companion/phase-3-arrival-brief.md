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

- [ ] `brief.go` + `brief_test.go`
- [ ] presence away 시 스냅샷 저장 (`presence.go` 훅 지점) + 테스트
- [ ] greet 발화 시 brief 첨부, check-in 시 actions 첨부 (`helpers_initiative_speak.go`) + 테스트
- [ ] `POST /v1/initiative/snooze` + 테스트
- [ ] 콘솔 파싱·말풍선 UI + `tests/companionPet.test.ts` 케이스 + `e2e/companion-events.spec.ts`에 brief 시나리오 (e2e 이벤트 훅 `e2e_events.go` 사용)
- [ ] feature-map "Arrival brief"

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
