# Phase 2 — Bridge: 어디서나 묻기 (E2)

> Roadmap: [roadmap.md](./roadmap.md) · 선행: Phase 1
> 목표: 어느 화면에서든 클릭 또는 ⌘J로 Bridge 패널을 열고 "지금 상황"을 묻는다. CASE는 **전용 읽기 전용 세션**에서 `tars_status` 도구로 TARS 전체를 읽고, 답과 함께 라이브 카드(바로 가기)와 "본 곳"을 보여 준다.

## 완료 조건

- `tars_status` 앱 도구가 파이프라인·세션 활동·승인 대기·cron·비용·pulse를 한 번에 요약해 돌려준다. 쓰기 없음.
- CASE 질문은 숨은 `worker` 세션 `case`에서 실행된다 (permission `plan`, 도구 화이트리스트). **main 세션 transcript에 더 이상 섞이지 않는다.**
- 답 끝의 `<case-cards>` 블록을 서버가 검증해 콘솔이 라이브 카드로 렌더한다 (존재하지 않는 id는 버림).
- Bridge 패널: "지금 보는 곳" 맥락, 범위 토글(이 화면 / TARS 전체), 라이브 카드, 본 곳, 후속 질문 칩, ⌘J.
- 기존 말풍선의 짧은 질문 입력은 Bridge로 연결된다 (말풍선 자체의 라인 표시는 그대로).

## 설계

### `tars_status` 도구 (`internal/apptool/tool_tars_status.go`)

집계형 도구 패턴(`session_aggregator.go`) 그대로, action 하나에 섹션 선택:

```json
{"action": "overview" | "focus" | "sessions" | "approvals" | "cron" | "usage" | "pulse", "since": "2h"?}
```

| 섹션 | 소스 (재사용) | 반환 요약 |
|---|---|---|
| focus | `focuspipeline` store list | id, 제목, 현재 단계, open_gate, 대기 시간, goal mode, (있으면) crew 요약 |
| sessions | chat activity (`GET /v1/chat/activity`와 같은 소스) | 실행 중/최근 완료/실패 세션 id·제목·경과 |
| approvals | ops 승인 큐 | 대기 건수, 가장 오래된 것 |
| cron | cron store + history | 최근 실패, 다음 실행 |
| usage | `usage_report` 집계 함수 | 오늘/이번 주 비용, 모델별 상위 |
| pulse | pulse 최신 신호 | 경고 목록 |

- `overview` = 각 섹션 상위 3개 + 수치. 응답 4KB 상한.
- 툴 정의 위치: core가 app을 import하지 않도록 `apptool`에 두고, 서버 bind 시 의존 함수 주입 (`make arch-check`).
- 테스트: 섹션별 table-driven, 빈 상태, 4KB 절단.

### CASE 세션 (`internal/tarsserver/case_session.go`)

`focus_qa.go qaSession()` 패턴:
- 최초 사용 시 `CreateWithOptions("CASE", "worker", true)`, id를 `<workspace>/case.json`에 기록 (재시작 후 재사용)
- 매 질문 전: `SetPermissionMode(plan)`, `SetTierPin(cfg.CompanionTier 또는 light)`, `SetPromptOverride(casePersona)`, `SetToolConfig(ToolsCustom: true, ToolsEnabled: [tars_status, sessions_history, cron_runs, usage_report])`
- `casePersona`: E4의 목소리 규칙 (결론 먼저, 1~2문장, 정확한 숫자, 다음 행동 하나, 감탄·이모지 없음) + `<case-cards>` 형식 + "쓰기 동작을 제안하지 말고 이동 링크로만 안내"
- 실행: `runServerChatTurnAs(..., chatTurnOrigin{unattended: "case", readOnly: true})`
- API: `POST /v1/case/ask {question, scope: "screen"|"all", view_context}` → 202 `{session_id, turn}`. 답은 기존 `GET /v1/chat/stream`으로 받는다
- 세션이 커지면: 하루 단위로 새 세션 회전 (`case.json`에 날짜) — 문맥 비용 억제

### `<case-cards>` 블록

```
<case-cards>[{"kind":"focus|session|cron|approval","id":"…","title":"…","sub":"…","severity":"block|info"}]</case-cards>
```

- 서버가 턴 완료 후 파싱, id를 실제 store에서 확인해 없는 것은 버리고 링크 경로를 서버가 만든다 (`/console/focus/<id>`, `/console/chat/<id>`, `/console/cron`, `/console/approvals`)
- 결과는 세션 메시지 메타로 저장 → `GET /v1/case/turns/{turn}` `{answer, cards, sources}`; sources = 그 턴의 도구 호출 목록(서버 기록)

### 콘솔

- `src/lib/caseBridge.ts` (순수): `bridgeRequest(question, scope, route)`, `cardHref(card)`, `followUps(cards)` (카드 종류별 후속 질문 2~4개), `sourcesLine(sources)`
- `src/components/CaseBridge.svelte` (E2): 오른쪽 슬라이드 패널 500px, 헤더(미어캣·당직 요약), "지금 보는 곳", 대화(질문/답/카드/본 곳), 칩, 범위 토글, 입력. 900px 이하 전체폭
- 단축키: ⌘J (`lib/keymap` 기존 등록 방식, `ShortcutHelp`에 추가). Focus 라우트 단축키와 충돌 없는지 `focusOwnsShortcut` 확인
- `CompanionPet.svelte`: 말풍선의 질문 폼 → Bridge 열기 + 질문 전달. 기존 "Chat 화면에서는 스레드로 전달" 동작은 Bridge의 "이 대화로 보내기" 버튼으로 대체
- `App.svelte`에 Bridge 마운트 (전역 → `make console-e2e` 전체)

## 작업

- [ ] `tool_tars_status.go` + 테스트
- [ ] `case_session.go`, `POST /v1/case/ask`, `GET /v1/case/turns/{turn}` + 테스트 (세션 생성·재사용·권한·도구 제한, 쓰기 도구 호출 불가)
- [ ] `<case-cards>` 파서·검증 + 테스트 (없는 id 제거, 링크 생성)
- [ ] mock LLM `[e2e:case-bridge]`: `tars_status` 호출 1회 + 카드 2장 답
- [ ] `caseBridge.ts` + `tests/caseBridge.test.ts`
- [ ] `CaseBridge.svelte`, ⌘J, CompanionPet 연결, i18n en/ko
- [ ] `e2e/case-bridge.spec.ts`: Focus 화면에서 ⌘J → 질문 → 카드 클릭으로 해당 파이프라인 이동
- [ ] ADR `docs/decisions/case-bridge.md`: 질문 경로를 main 세션 → 전용 세션으로 바꾼 이유(tars#1236 대체), 읽기 전용 경계
- [ ] feature-map "CASE Bridge"

---

### ✅ Checkpoint: Phase 2

**자동 확인**
- [ ] `make test`, `make test-one TEST_NAME='TarsStatus|CaseSession|CaseCards' PKG=./internal/...`
- [ ] `make arch-check` (apptool ↔ tarsserver 경계)
- [ ] `npm run check`, `tests/caseBridge.test.ts`, `make console-e2e` 전체
- [ ] `make lint-diff && make test-diff`

**수동 확인**
- [ ] 파이프라인 2개·cron 실패 1건이 있는 상태에서 "막힌 거 있어?" → 막힌 것부터, 카드 클릭 시 정확한 화면으로 이동
- [ ] main 세션 transcript에 CASE 질문이 남지 않음
- [ ] "cron 지워줘" → 직접 실행하지 않고 cron 화면 링크로 안내
- [ ] 범위 "이 화면"에서 Focus 파이프라인 질문 → 해당 파이프라인만 근거로 답

**통과하면 사용자 확인 후 Phase 3로.**
