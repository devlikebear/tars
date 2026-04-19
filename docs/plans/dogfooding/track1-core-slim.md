# Track 1 — Core Slim (코어 슬림화)

**Status**: Planning
**Branch (제안)**: `chore/core-slim`
**Depends on**: 없음
**Blocks**: Track 2 (시나리오 구현 시작 전 완료 권장)

## 배경

도그푸딩 시나리오 구현 전에 의심 모듈을 정리해 plugin/skill로 깔끔히 올릴 토대를 만든다. 새 기능을 더하는 게 아니라 **빼는 작업**이라 위험도는 낮지만, 잘못 빼면 버그가 나니 "조사 → 결정 게이트 → 제거" 흐름으로 진행한다.

## 현황 (Audit 결과 요약)

| 모듈 | 파일수 | 외부 import 수 | 1차 진단 | 결정 게이트 |
|---|---|---|---|---|
| `internal/research` | 2 | 4 (`tool_research.go`, `handler_chat.go`, `helpers_agent.go`, `main_serve_api.go`, `helpers_cron_test.go`) | 도구로 LLM에 노출 — 실사용 빈도/대안 확인 필요 | 사용 빈도 + 대안(web 도구) 평가 후 결정 |
| `internal/schedule` | 2 | 4 (HTTP handler 포함) | cron과 별도 surface. 콘솔에 schedule UI 있는지 확인 필요 | UI/사용자 워크플로 확인 후 결정 |
| `internal/scheduleexpr` | 2 | 2 (`cron/validation.go`, `schedule/store.go`) | schedule 제거 시 cron 단독 의존만 남음 → cron이 cronexpr 라이브러리 직접 쓰면 통합 가능 | schedule 결정에 종속 |
| `internal/agent` | 4 | 5 | 채팅 실행 핵심 — **유지** | (제거 안 함) |
| `internal/extensions` | 5 | 15 | 깊이 박힌 인프라 — **유지** | (제거 안 함) |

## 목표

1. `research`, `schedule`, `scheduleexpr` 3개 모듈에 대해 **유지/제거** 결정을 내린다.
2. 제거 결정된 것은 **dead code 0 라인**으로 정리한다 (handler/route/도구/테스트/콘솔/문서까지).
3. 변경 후 `make test` + `make vet` + `make console-build` 모두 통과.
4. CHANGELOG에 제거 사실과 영향(있다면 마이그레이션) 기록.

## 작업 체크리스트

### 1. Audit — research 모듈

- [ ] `internal/research/` 구현 읽기 — 정확히 무엇을 제공하는지 (web search? 별도 LLM 호출? 메모 정리?)
  - 검증: 한 줄 요약 가능
- [ ] `tool_research.go` 도구 스키마 확인 — LLM에게 노출되는 입출력
- [ ] 최근 30일 도구 호출 로그(있으면)에서 `research_*` 호출 빈도 확인 (`grep -i research .logs/tars-debug.log` 등)
- [ ] `internal/tool/`에 web_fetch, web_search 등 대안 도구가 있는지 확인 — 있으면 research가 중복일 가능성
- [ ] **결정**: 유지 / 제거 (사용자 확인)
  - 유지면: 결정 사유 한 줄 + Track 1 작업에서 제외
  - 제거면: 작업 항목 5번으로 이동

### 2. Audit — schedule + scheduleexpr 모듈

- [ ] `internal/schedule/store.go` 읽기 — cron의 `internal/cron/store.go`와 데이터 모델 차이 비교
  - 차이 항목 표로 정리 (status 필드, NL 파싱, timezone 등)
- [ ] `handler_schedule.go` HTTP 라우트 — 콘솔에서 어떤 화면이 호출하는가? (`frontend/console/src/` grep)
- [ ] `tool_schedule.go` 도구 — cron 도구(`tool_cron.go` 추정)와 기능 중복도
- [ ] 콘솔 UI에서 schedule 화면 사용성/필요성 검토 (cron만으로 충분한가?)
- [ ] **결정**: (a) 유지, (b) cron으로 통합 후 schedule/scheduleexpr 제거, (c) 보류
  - 사용자 확인 필수

### 3. 결정 게이트 (사용자 확인)

- [ ] research 모듈: 유지 / 제거 결정
- [ ] schedule + scheduleexpr 모듈: 유지 / 통합제거 / 보류 결정
- [ ] 결정 사유를 본 문서 "결정 기록" 섹션에 추가

### 4. 제거 작업 — research (제거 결정 시)

조건부 작업 — 결정 게이트 결과에 따라 실행.

- [ ] `internal/research/` 디렉토리 삭제
- [ ] `internal/tool/tool_research.go` + 테스트 삭제
- [ ] `internal/tarsserver/handler_chat.go` import + 사용처 제거
- [ ] `internal/tarsserver/helpers_agent.go` import + 사용처 제거
- [ ] `internal/tarsserver/main_serve_api.go` import + 라우트 등록 제거
- [ ] `internal/tarsserver/helpers_cron_test.go` import + 테스트 정리
- [ ] config 필드(있으면) 제거 + `config/standalone.yaml` 갱신
- [ ] 콘솔에 research 관련 UI 있으면 제거
- [ ] system prompt(`internal/sysprompt/`)에서 research 도구 언급 제거
- [ ] `make test`, `make vet`, `make console-build` 통과
- [ ] 검증: `grep -ri research internal/ cmd/ frontend/console/src/` → 의도된 잔존만 남음

### 5. 제거 작업 — schedule + scheduleexpr (통합제거 결정 시)

조건부 작업 — 결정 게이트 결과에 따라 실행.

- [ ] cron이 cronexpr 라이브러리(`github.com/robfig/cron/v3` 등 추정) 직접 사용하도록 `internal/cron/validation.go` 리팩터링
- [ ] schedule이 cron에 추가하던 기능(NL 파싱, status 등) 중 살릴 것 결정 → cron으로 이전 또는 제거
- [ ] `internal/schedule/` 디렉토리 삭제
- [ ] `internal/scheduleexpr/` 디렉토리 삭제
- [ ] `internal/tool/tool_schedule.go` + 테스트 삭제 (또는 `tool_cron.go`로 흡수)
- [ ] HTTP 라우트(`handler_schedule.go`) 삭제 + `main_serve_api.go` 등록 제거
- [ ] 콘솔에서 schedule 화면 제거 + 라우터 정리
- [ ] config 필드(있으면) 제거
- [ ] `make test`, `make vet`, `make console-build` 통과
- [ ] 검증: `grep -ri "internal/schedule" .` → 0건

### 6. 마무리

- [ ] CHANGELOG.md에 제거 항목 추가 (사용자 영향 명시)
- [ ] CLAUDE.md의 모듈 목록(있으면) 갱신
- [ ] PR 작성 — 제목: `chore: slim core for dogfooding (remove <modules>)`
- [ ] PR description에 결정 사유 + 검증 결과 포함

## Checkpoint: Track 1 완료 확인

**구현 확인:**
- [ ] research / schedule+scheduleexpr 결정이 본 문서에 기록됨
- [ ] 제거 결정된 모듈은 디렉토리 + 모든 import + 테스트 + UI 흔적이 0
- [ ] `internal/agent`, `internal/extensions`는 손대지 않음 (의도적 유지)

**실행 확인:**
- [ ] `make test` → 0 failure
- [ ] `make vet` → 0 warning
- [ ] `make console-build` → 성공
- [ ] `make build` → `bin/tars` 생성
- [ ] `./bin/tars serve` 정상 기동 + `/console` 정상 렌더

**수동 확인:**
- [ ] 콘솔에서 채팅 1회 실행 → 정상 응답
- [ ] cron 잡 1개 생성/실행/삭제 → 정상 동작 (schedule 통합제거 시 회귀 없음 확인)

**통과 후 Track 2 Phase A로 진행.**
실패 시: 어느 항목이 실패했는지 보고 → 사용자와 함께 원인 파악 → 수정 또는 결정 재검토.

---

## 결정 기록

| 항목 | 결정 | 사유 | 결정일 |
|---|---|---|---|
| research 유지/제거 | **제거** | `research_report` 도구는 단순 markdown 파일 쓰기 + JSONL 로그 추가일 뿐. 실제 web search/LLM 호출 없음. `write_file` 도구로 동등 기능 가능. 코어 슬림화 원칙에 부합. | 2026-04-19 |
| schedule + scheduleexpr | **schedule 제거 + scheduleexpr는 NormalizeExpression만 유지** | `internal/schedule`은 cron의 CRUD facade(status field + NL 파싱 추가)일 뿐. 콘솔(`frontend/console/src/`)에서 `/v1/schedules` 호출 0건 — UI 미사용. CLI(`tarsclient/commands_schedule_test.go`)만 사용 → CLI도 동시 제거. `scheduleexpr.NormalizeExpression`은 cron이 직접 의존하므로 유지, `ParseNaturalSchedule`은 schedule 전용이므로 제거. | 2026-04-19 |

## Audit 상세 결과

### research 모듈
- **파일**: `internal/research/service.go` (137 LOC), `service_test.go` (33 LOC)
- **노출 도구**: `research_report` (1개)
- **실제 동작**: `workspace/reports/<날짜>-<slug>.md` 파일 생성 + `summary.jsonl` 로그 추가. LLM 호출 X, 웹 호출 X.
- **호출자**: `tool_research.go`, `helpers_agent.go` (등록), `main_serve_api.go` (인스턴스화), `handler_chat.go` (DI), `helpers_cron_test.go` (테스트), `helpers_build_gateway.go` (default tool 목록), `helpers_agent_tools_test.go` (테스트), `handler_chat_policy.go` (정책 목록)
- **대안 도구**: `internal/tool/web_fetch.go`, `web_search.go` 존재 — 진짜 리서치는 이 도구들이 담당 가능. `research_report`는 출력만 정형화하는 sugar.

### schedule 모듈
- **파일**: `internal/schedule/store.go` (684 LOC), `store_test.go` (211 LOC)
- **노출 도구**: `schedule_create/list/update/delete/complete` (5개)
- **HTTP 라우트**: `/v1/schedules` (GET/POST), `/v1/schedules/<id>` (PATCH/DELETE)
- **실제 동작**: `cron.Store`를 감싸서 `_tars_schedule` payload 키에 `status`(active/paused/completed) + `natural` + `timezone` 메타 추가. `schedule_complete`는 `status=completed` 업데이트 alias.
- **콘솔 사용**: 0건 (`frontend/console/src/`에서 `/v1/schedules` grep → 0)
- **CLI 사용**: `internal/tarsclient/commands_schedule_test.go` — CLI subcommand 존재 → 함께 제거
- **중복도**: cron이 `enabled` 플래그로 active/paused 동등 표현 가능. `completed`는 cron의 `--enabled=false` + UI label로 충분. NL 파싱은 schedule 전용 가치였지만 cron도 `normalizeSchedule`이 fallback으로 `ParseNaturalSchedule` 호출 중 (cron에 흡수됨).

### scheduleexpr 모듈
- **파일**: `internal/scheduleexpr/normalize.go` (196 LOC), `normalize_test.go` (78 LOC)
- **함수**:
  - `NormalizeExpression`: cron 표현식 정규화 (`at:`, `every:`, `@every`, 표준 cron) — **cron이 의존**
  - `ResolveSchedule`: explicit + natural fallback — **cron이 의존** (`normalizeSchedule`)
  - `ParseNaturalSchedule`: 한국어/영어 NL → cron 변환 — **cron이 fallback으로 의존** (validation.go:18)
- **결정**: 모듈 자체는 유지. cron이 모든 함수에 의존하므로 수술 불필요. schedule 제거하면 외부 import는 cron 하나만 남음 → 향후 cron 안으로 흡수 검토는 별도 PR.

## Out of Scope (Track 1)

- `internal/agent`, `internal/extensions` 변경 — 핵심 추상화이므로 손대지 않음
- 새 기능 추가 — Track 1은 순수 슬림화
- pulse/reflection 변경 — 시스템 surface는 그대로
- LLM provider pool / config 스키마 변경 — 별도 트랙
