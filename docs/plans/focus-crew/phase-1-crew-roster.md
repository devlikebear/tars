# Phase 1 — 크루 모델과 편성 (D2)

> Roadmap: [roadmap.md](./roadmap.md) · 참조: [codebase-analysis.md](./codebase-analysis.md)
> 목표: 새 태스크를 만들면 TARS가 크루를 제안(소환 + 채용)하고, 사용자가 확인하면 파이프라인이 크루를 품은 채 시작한다. **턴 실행은 아직 기존과 동일**하다.

## 완료 조건

- 내장 봇 4종 + 워크스페이스 봇 파일을 읽는 라이브러리가 있다.
- `POST /v1/focus/crew/draft`가 목표·템플릿을 받아 크루 제안을 돌려준다 (LLM 불가 시 결정론적 기본 크루).
- `POST /v1/focus/pipelines`가 `crew`를 받고, 파이프라인에 크루가 복사·저장된다. `crew` 없이 만들면 오늘과 동일.
- 콘솔: New task → "크루 꾸리기" → 편성 화면(D2) → "이 크루로 Plan 시작". Stepper에 단계별 크루 색 점이 보인다.

## 데이터 모델

### Bot spec (`internal/focuspipeline/crew.go`)

```go
type BotRole string // researcher | checker | prototyper | reviewer | builder | scribe

type BotSpec struct {
    ID           string    `json:"id" yaml:"id"`             // [a-z0-9-]{2,32}
    Name         string    `json:"name" yaml:"name"`
    Glyph        string    `json:"glyph" yaml:"glyph"`       // owl | cat | dog | elephant | beaver | … (콘솔 아바타 키)
    Role         BotRole   `json:"role" yaml:"role"`
    Tier         string    `json:"tier,omitempty" yaml:"tier,omitempty"` // heavy|standard|light, 빈 값 = 기본
    Tools        []string  `json:"tools,omitempty" yaml:"tools,omitempty"` // 허용 도구 이름 (예: web_search, web_fetch)
    Kinds        []StageID `json:"kinds" yaml:"kinds"`       // 참여 가능한 stage kind
    Instructions string    `json:"instructions,omitempty" yaml:"instructions,omitempty"` // ≤4000B, <focus-…> 금지
    Builtin      bool      `json:"builtin,omitempty" yaml:"-"`
}

type CrewMember struct {
    Bot       BotSpec   `json:"bot"`              // 복사본 (라이브러리 수정이 진행 중 파이프라인에 영향 없음)
    Origin    string    `json:"origin"`           // "library" | "hired"
    Stages    []StageID `json:"stages"`           // 배치된 stage id (템플릿의 id)
    SessionID string    `json:"session_id,omitempty"` // P2에서 채움 (워커 세션)
    Keep      *bool     `json:"keep,omitempty"`   // P5
}
```

`Pipeline`에 `Crew []CrewMember \`json:"crew,omitempty"\`` 추가 (`model.go`). 빌더·스크라이브는 메인 세션 페르소나라 `SessionID`를 갖지 않는다.

### 내장 봇 (`BuiltinBots()`)

| id | 이름 | glyph | role | 기본 tools | kinds |
|---|---|---|---|---|---|
| `hoot` | Hoot | owl | researcher | web_search, web_fetch | plan, review |
| `miso` | Miso | cat | checker | (읽기 도구) | plan, review, pr_review |
| `pip` | Pip | dog | builder | — (메인 세션) | plan, build, review, pr_review |
| `ellie` | Ellie | elephant | scribe | — (메인 세션) | pr, merge |

TARS 자신은 봇이 아니라 리드(드라이버 + 편성 LLM)다.

## 작업

### A. 라이브러리 (Go, 순수)

- [ ] `internal/focuspipeline/crew.go`: 위 타입, `BuiltinBots()`, `(BotSpec) Validate() error`
  - 규칙: id 형식, 이름 1~24자, role 열거, kinds는 6개 kind 안, instructions ≤4000B·`<focus-` 금지(템플릿 `Validate`의 규칙 재사용), tools는 허용 목록(`web_search`, `web_fetch`, 읽기 계열)만
- [ ] `crew_store.go`: `LoadBots(dir) ([]BotSpec, []TemplateDiagnostic)`, `FindBot(dir, id)`, `SaveBot(dir, spec)`, `DeleteBot(dir, id)` — `template.go`의 `LoadTemplates`/`SaveTemplate`/`removeTemplateFiles` 패턴 그대로. 디렉터리 `<workspace>/focus-crew/`. 내장 id는 덮어쓸 수 없음(`-custom` 복사 규칙도 템플릿과 동일)
- [ ] `ValidateCrew(crew []CrewMember, stages []Stage) error`: 배치 stage가 파이프라인에 존재, member kind 허용, 같은 bot id 중복 금지, 최대 6명, 채용(hired)은 최대 2명
- [ ] 테스트 `crew_test.go`, `crew_store_test.go` (table-driven): 유효/무효 스펙, 내장 덮어쓰기 거부, 깨진 YAML diagnostic, ValidateCrew 경계
- 검증: `make test-one TEST_NAME='Bot|Crew' PKG=./internal/focuspipeline/`

### B. 파이프라인에 크루 싣기

- [ ] `NewFromTemplate`는 그대로 두고 `WithCrew(p Pipeline, crew []CrewMember) (Pipeline, error)` 추가 (검증 후 복사)
- [ ] `focus_pipeline.go` `api.create`: 요청에 `crew` 필드 추가 → `WithCrew`. 없으면 기존 경로 그대로
- [ ] 테스트: crew 없는 생성이 기존 golden과 동일한 `pipeline.json`을 만드는지 (`focus_pipeline_test.go`에 케이스 추가)

### C. 편성 API (`internal/tarsserver/focus_crew.go`)

- [ ] `GET /v1/focus/crew/bots` → `{bots, diagnostics}` (내장 + 워크스페이스)
- [ ] `POST /v1/focus/crew/draft` `{goal, template, cwd?}` → `{summon: [bot_id], hire: [BotSpec], assignment: {bot_id: [stage_id]}, reasons: {bot_id: string}, summary}`
  - 구현은 `focus_template_edit.go` 복제: `chat_main` tier, decision-only (`ToolChoice: none`, harness tool list 비움, permission plan), 응답 JSON 1개, `ValidateCrew` 실패 시 오류를 프롬프트에 넣어 1회 재시도, 2회 실패 400, tool-call 시도 거부
  - LLM/라우터 없음 → 503 대신 `defaultCrew(template)`(내장 4종, kind 매칭 배치)를 `fallback: true`로 반환
  - 프롬프트에 넣을 것: 목표, 템플릿 stage 목록(id/kind/label), 라이브러리 봇 요약(id, role, kinds, 최근 사용 횟수는 P5), 채용 규칙("라이브러리로 부족한 역할만, 최대 2명, 읽기 전용")
- [ ] admin 토큰 필요 (파이프라인 생성과 같은 규칙)
- [ ] 테스트 `focus_crew_test.go`: 정상 draft, 재시도 후 성공, 2회 실패 400, tool-call 거부, 폴백
- [ ] mock LLM: `e2e/mock-llm.mjs`에 `[e2e:focus-crew]` 마커 — draft 요청에 hoot/miso/pip/ellie 소환 + `bea`(prototyper) 채용 JSON 응답

### D. 콘솔 — 편성 화면(D2)과 표시

- [ ] `src/lib/focusCrew.ts` (순수): 타입 `BotSpec`, `CrewMember`, `CrewDraft`; `assignmentMatrix(stages, crew)` → 행=stage, 열=bot, 셀=역할 라벨/게이트("당신"); `crewDots(stage, crew)` → 색 배열; `botColor(glyph)`
- [ ] `src/components/focus/CrewAvatar.svelte`: glyph → SVG (D안 각진 동물 5종 + TARS 4슬랩, 크기 prop). 장식이므로 `aria-hidden`, 이름은 옆 텍스트로
- [ ] `src/components/focus/FocusCrewAssemble.svelte` (D2): 소환 카드(빼기 ×), 구인 공고 카드(채용 봇 스펙 요약, "미션 후 보관" 체크 — 값은 P5에서 사용), 단계×봇 배치 표, "크루 바꾸기"(봇 추가/제거 드롭다운), "이 크루로 Plan 시작"
- [ ] `FocusNewTask.svelte`: 시작 버튼을 "크루 꾸리기 →"로 바꾸고 draft 호출 → Assemble 단계로 전환. "크루 없이 시작" 보조 링크 유지(기존 경로)
- [ ] `FocusStepper.svelte`: `pipeline.crew`가 있으면 단계 아래 크루 색 점(8px)
- [ ] i18n en/ko: `focus.crew.*`
- [ ] 테스트: `tests/focusCrew.test.ts` (assignmentMatrix, crewDots), `e2e/focusCrew.spec.ts` ("크루 꾸리기 → 편성 화면 → 시작 → stepper에 점" 한 시나리오, en/ko)

### E. 문서

- [ ] `docs/decisions/focus-mode.md` §11 Crew: 봇 스펙, 복사 원칙, 편성 API, 불변 조건(roadmap의 목록)
- [ ] `docs/feature-map.md`: "Crew assembly" 항목
- [ ] `docs/focus-templates.md`: `focus-crew/*.yaml` 형식 예시

---

### ✅ Checkpoint: Phase 1

**자동 확인**
- [ ] `make test` 통과 (기존 focus 테스트 전부 포함)
- [ ] `make test-one TEST_NAME='Crew|Bot' PKG=./internal/focuspipeline/` · `PKG=./internal/tarsserver/` 통과
- [ ] `cd frontend/console && npm run check && node --experimental-strip-types --test tests/focusCrew.test.ts`
- [ ] `cd frontend/console && npx playwright test e2e/focusCrew.spec.ts e2e/focus.spec.ts` (기존 focus e2e 회귀 없음)
- [ ] `make lint-diff && make test-diff && make arch-check`

**수동 확인** (`make dev-console`, 샌드박스 config)
- [ ] 새 태스크 "로그인 5번 실패하면 10분 잠가줘" → 편성 화면에 소환 4 + 채용 1, 배치 표가 템플릿 단계와 일치
- [ ] LLM을 끈 config에서 같은 동작 → 기본 크루 + "자동 제안 불가" 안내
- [ ] "크루 없이 시작" → 기존과 똑같은 파이프라인 (`pipeline.json`에 `crew` 없음)

**통과하면 사용자에게 확인 요청 후 Phase 2로.** 실패하면 실패 항목과 로그를 보고하고 원인 합의 후 수정 → 재검증.
