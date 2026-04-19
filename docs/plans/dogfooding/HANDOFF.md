# Dogfooding 작업 핸드오프

**Last Updated**: 2026-04-19 (수동 검증 후 업데이트)
**Updated By**: Phase A 수동 검증 세션

> 이 파일은 매 의사결정/단계 완료/블로커 발생/세션 종료 시 갱신된다. 새 세션은 이 파일부터 읽는다.

---

## 새 세션 진입 가이드 (5분)

1. 아래 **컨텍스트 (한 단락)**을 읽는다.
2. **아키텍처 원칙**을 반드시 확인 — 이 도그푸딩의 모든 결정이 여기서 파생된다.
3. **현재 상태** 섹션에서 활성 트랙/페이즈 확인.
4. 활성 페이즈의 계획서를 읽는다 (각 페이즈 계획서는 self-contained).
5. **미해결 결정**이 있으면 사용자에게 먼저 확인.
6. **다음 액션** 첫 항목부터 시작.

작업 끝(세션 종료 직전 또는 PR 생성 시)에 이 파일의 모든 섹션을 갱신한다.

## 컨텍스트 (한 단락)

TARS 도그푸딩 v1 — TARS를 다중 프로젝트 운영/창작/리서치 자동화 호스트로 쓴다. 첫 시연: 가상 사이트 `tars-examples-foo`(Docker 운영, 의도적 버그 시드)의 서버 로그를 30분마다 감시 → GitHub 이슈 등록 → fix-PR 제출 (리뷰/머지는 사람). 핵심 원칙: **TARS 코어에 새 추상화를 추가하지 않는다** (과거 `internal/project/*` 시스템이 비대화 후 #291~#347에서 전부 철거된 경험). 모든 도메인 기능은 **외부 repo(`devlikebear/tars-skills`)의 skill + CLI**로. Track 1(코어 슬림화) → Track 2 4페이즈(시나리오 구현) 순. 자세한 그림은 [README.md](./README.md).

## 아키텍처 원칙 (가장 중요 — 2026-04-19 재정의)

**"TARS 코어에 새 도메인 기능을 추가하지 않는다. 빌트인 Go 플러그인도, MCP 서버도 아니다. skill + CLI로 만들어 외부 repo에서 설치한다."**

### 왜 빌트인 Go 플러그인을 쓰지 않는가

- Go 빌트인 플러그인으로 도구를 노출하면 TARS 기동 시점에 모든 도구 description이 **시스템 프롬프트에 주입**된다. 도구가 늘어날수록 시스템 프롬프트가 비대해지고, 세션 시작 컨텍스트·응답 지연·비용이 악화된다.
- MCP 서버로 감싸도 동일 문제 — MCP 도구도 모두 system prompt에 기술된다.
- TARS 코어에 도메인 기능(foo 로그 감시, GitHub ops 등)을 추가하면 과거 `internal/project/*` 폐기 사유를 반복한다 (#291~#347).

### 채택하는 아키텍처: skill + CLI + tars-skills 외부 repo

1. **기능은 외부 CLI**로 작성 — Python/TypeScript/shell 등 자유 선택. `gh`, `docker` 같은 기존 CLI를 래핑하거나 조합만 해도 됨.
2. **skill(.md)이 해당 CLI의 디스패처** — skill YAML frontmatter의 `recommended_tools: [bash]`로 TARS의 빌트인 `bash` 툴을 사용해 CLI를 호출. 도구 설명이 시스템 프롬프트에 상주하지 않고, 스킬이 호출될 때만 온디맨드로 컨텍스트에 들어간다.
3. **skill은 `devlikebear/tars-skills` 외부 repo에 저장** — TARS 본 repo에는 일절 추가하지 않는다. `tars skill install <name>` (내부 `internal/skillhub`) 로 설치.
4. **검증된 선례**: `tars-skills` repo의 `daily-briefing` skill이 정확히 이 패턴 (`SKILL.md` + `briefing.sh`, frontmatter에 `recommended_tools: [bash]`).

### 이 원칙이 도그푸딩에 적용되는 방식

| 과거 안 (폐기) | 현재 안 |
|---|---|
| `internal/logwatcher/` 빌트인 Go 플러그인 (docker_logs, file_tail 도구 2개) | `devlikebear/tars-skills` repo의 `log-watcher` skill + 로그 수집 CLI (shell or Python) |
| `internal/githubops/` 빌트인 Go 플러그인 (gh_* 도구 6개) | `devlikebear/tars-skills` repo의 `github-ops` skill + gh 래퍼 CLI |
| `cmd/tars/main.go`에 blank import 등록 | `tars skill install log-watcher` / `tars skill install github-ops` 로 설치 |

**TARS 본 repo에서 도그푸딩 때문에 건드릴 일이 생기면 그건 잘못된 방향이라는 신호다.** 필요하다면 스킬 설치/로딩 자체를 개선하는 별도 트랙은 열 수 있으나 도그푸딩 스코프 아님.

## 현재 상태

- **활성 트랙**: Track 2 — Monitored Ops
- **활성 페이즈**: Phase A (인프라) — **완료. Phase B 진입 준비됨.**
- **활성 단계**:
  1. TARS core: 아키텍처 pivot docs [#362](https://github.com/devlikebear/tars/pull/362) 머지 완료 (`bb09d9c`)
  2. `tars-skills`: [tars-skills#3](https://github.com/devlikebear/tars-skills/pull/3) 머지 완료 (`3ec9525`). 수동 검증 중 발견된 2건을 main에 직접 패치로 반영:
     - `988dc88` fix(registry): 설치기가 요구하는 `SKILL.md` 엔트리 + sha256 체크섬 추가
     - `e81181a` fix(log-watcher): slog `"time"`/`"timestamp"`/`"@timestamp"` ts 별칭 수용 (v0.1.1)
  3. `tars-examples-foo`: 초기 시딩 `3b8fd23`, Dockerfile /data 퍼미션 버그 수정 `b21b9c4` (volume 마운트 시 root 소유 → sqlite open 실패 → 재시작 루프)
- **활성 worktree**: 없음 (정리 완료)
- **블로커**: 없음
- **마지막 빌드 / 검증**:
  - TARS core: `make build` 클린 (`bin/tars 0.28.0`)
  - `tars-skills`: 로컬 테스트 11+14 통과, `tars skill install log-watcher|github-ops` 성공
  - `tars-examples-foo`: `docker compose up --build` → `/health 200`, todo CRUD 정상, `/bug/panic` + `/bug/bad`가 slog `"panic recovered"` + stack trace로 기록됨. 병렬 PUT `/todos/{id}`는 WAL+busy_timeout(2000) 덕에 에러는 안 나지만 잃어버린 업데이트(lost-update)로 관찰됨 — Phase B anomaly-detect가 탐지할 타겟
  - Skill → CLI 왕복: `bash $workspace/skills/log-watcher/log_watcher.sh docker --container tars-examples-foo --tail 5` → `{source,target,lines[{ts,level,msg,raw}],...}` 수신. `issue-search`/`worktree-setup`/`worktree-cleanup` 모두 성공
- **주의**: `tars skill update`는 GitHub raw CDN 캐시 만료 후 v0.1.1 자동 승격 (TTL ≈ 5분)

## 미해결 결정 (사용자 확인 대기)

| 결정 사항 | 선택지 | 비고 |
|---|---|---|
| `tars-examples-bar` 스택 | (a) Node, (b) Python, (c) 기타 | Phase D 시작 전 결정. foo가 Go라 가정. |

### 확정된 결정

- **아키텍처**: skill + CLI + `tars-skills` 외부 repo — 2026-04-19 사용자 확정. 빌트인 Go 플러그인 / MCP 서버 경로 모두 기각.
- **`tars-examples-foo` repo 위치**: `devlikebear/tars-examples-foo` (public) — 2026-04-19 사용자 확인, 초기 커밋 투입 완료.
- **`log-watcher`/`github-ops` 설치 경로**: `devlikebear/tars-skills` repo (skill + 부속 CLI), `tars skill install <name>` 로 설치.
- **CLI 언어**: **shell** (bash 4+) — 2026-04-19. `daily-briefing` 선례와 일치, docker/gh/git 모두 shell-friendly, 런타임 의존 최소.
- **log-watcher 스코프**: Phase A에서는 `docker` + `file` 서브커맨드만. Sentry/Loki/OpenSearch/CloudWatch 등 원격 소스는 실제 시나리오가 요구할 때 추가 서브커맨드 또는 별도 skill로 — 선제 추상화 금지. 출력 JSON은 `source` 필드로 forward-compatible.

(Track 1 진행 중 발생한 결정은 [track1-core-slim.md](./track1-core-slim.md) "결정 기록"에 기록)

## 다음 액션 (구체적)

✅ **Phase A 완료 (코드/커밋 + 수동 검증):**
1. TARS core docs pivot — [#362](https://github.com/devlikebear/tars/pull/362) 머지
2. `tars-skills` log-watcher + github-ops — [#3](https://github.com/devlikebear/tars-skills/pull/3) 머지, 검증 중 발견분 main 직접 패치(`988dc88`, `e81181a`)
3. `tars-examples-foo` 시딩 + Dockerfile 볼륨 권한 수정(`b21b9c4`)
4. `tars skill install {log-watcher,github-ops}` 성공, CLI round-trip 확인

⏭️ **다음 세션 첫 과제 (Phase B 진입):**
1. `docs/plans/dogfooding/track2-phase-b-detect-and-issue.md` 읽고 스코프 재확인
2. `tars-skills`에 `log-anomaly-detect` skill 추가 — log-watcher JSON 엔벨로프를 입력으로 받아 ERROR/`panic recovered` 라인을 식별, 이슈 등록 trigger 여부 판단
3. 같은 repo에 `fix-and-pr` skill 뼈대 — github-ops + gateway agent 조합으로 Phase C 준비
4. Phase B 동작 검증은 다음 방법으로:
   - foo를 기동 → `/bug/panic` 호출 → log-watcher → anomaly-detect → github-ops issue-create 체인을 수동으로 호출
   - TARS 콘솔 `/chat`에서 자연어로 "foo 최근 로그 확인하고 이상 있으면 이슈 등록" 발화 → skill 선택 → 체인 실행 확인

⚠️ **Phase A에서 재확인된 사실**:
- `tars skill install`은 `files` 엔트리 각각에 **sha256 필수** (`SKILL.md` 포함). 레거시 string 배열 형태는 parse는 통과하나 install은 실패 → 신규 skill 추가 시 반드시 sha256 포함
- `tars skill update`는 raw.githubusercontent.com 캐시(≈5분) 만료 후 반영. 급하면 버전 태그를 올려 명시적 무효화
- foo의 PUT race는 busy_timeout(2000) 덕에 에러는 거의 안 남. Phase B anomaly-detect는 ERROR/`panic recovered` 라인만 대상으로 충분 — 동시성 회귀 감지는 별도 테스트 하네스가 필요하면 Phase C에서 결정
- docker `/data` 볼륨 퍼미션 패턴(USER app + named volume)은 foo-bar 템플릿으로 재사용 가능 — `tars-examples-bar` 시드 시 동일 패턴 사용

## 진행 이력 (역시간순)

| 날짜 | 트랙/페이즈 | 작업 | 결과 | PR |
|---|---|---|---|---|
| 2026-04-19 | Track 2 / Phase A | 수동 검증 라운드 — `tars skill install` 실패 원인(sha256 필수) 발견 → tars-skills main `988dc88` 패치. foo `/data` 볼륨 퍼미션 버그 발견(USER app + named volume root-owned) → foo main `b21b9c4` 패치. Go slog `time` 필드 ts 추출 안되는 이슈 → log-watcher v0.1.1 `e81181a`. 최종 검증: CRUD/bug/panic/bug/bad/skill round-trip 모두 통과. | Phase A 종결 | 없음 (main 직접 푸시) |
| 2026-04-19 | Track 2 / Phase A | `tars-examples-foo` 시딩 — Go net/http todo API + modernc.org/sqlite + Dockerfile/docker-compose + 의도된 버그 3종 (/bug/panic, /bug/bad, PUT race) + slog JSON 구조화 로그. 초기 커밋 main 푸시 (PR 없음, 빈 repo 대상). | foo 기동 준비 완료 | 없음 (main 직접 푸시) |
| 2026-04-19 | Track 2 / Phase A | `tars-skills`에 `log-watcher` + `github-ops` skill 추가. SKILL.md + 부속 shell CLI + 단위 테스트 (11+14) + registry.json 엔트리 (0.1.0). log-watcher 스코프는 docker/file만으로 의도적 축소 — 원격 소스는 시나리오 기반 확장. | 25/25 테스트 통과, PR open | [tars-skills#3](https://github.com/devlikebear/tars-skills/pull/3) |
| 2026-04-19 | Track 2 / Phase A | TARS core docs pivot — HANDOFF/README/Phase A 계획서 재작성 + CLAUDE.md "Extension Pattern" 섹션 추가 + README.md Extensibility 재작성. 빌트인 Go 플러그인 시도 코드는 전량 revert. | docs-only diff vs main | [#362](https://github.com/devlikebear/tars/pull/362) |
| 2026-04-19 | Track 2 / Phase A | (폐기) 빌트인 Go 플러그인 (`internal/logwatcher`, `internal/githubops`) 구현 시도. | 시스템 프롬프트 비대 문제로 전면 폐기 | - |
| 2026-04-19 | Track 2 / Phase A | (폐기) log-watcher + github-ops builtin plugin 구현 (8개 도구 + 유닛 테스트), `cmd/tars/main.go`에 blank import 등록, VERSION 0.29.0, CHANGELOG Added 작성 | 시스템 프롬프트 비대 문제로 전면 폐기 | - |
| 2026-04-19 | Track 1 / release | `make test`(pre-existing auth 실패 1건만) + `make vet` 통과, VERSION 0.28.0, CHANGELOG Removed/Migration 작성, PR 생성 | Track 1 종결 | [#361](https://github.com/devlikebear/tars/pull/361) |
| 2026-04-19 | Track 1 / removal | research + schedule 코드 전부 제거, `go build ./...` 통과. | 빌드 클린 | [#361](https://github.com/devlikebear/tars/pull/361) |
| 2026-04-19 | Track 1 / audit | research/schedule/scheduleexpr 3개 모듈 audit | research = 제거, schedule = 제거(콘솔 미사용, CLI도 동시 제거), scheduleexpr = NormalizeExpression+ResolveSchedule+ParseNaturalSchedule 모두 cron이 의존하므로 유지. 상세 [track1-core-slim.md](./track1-core-slim.md) "Audit 상세 결과" | [#361](https://github.com/devlikebear/tars/pull/361) |
| 2026-04-19 | Planning | 도그푸딩 계획 문서 + 핸드오프 시스템 작성 | README/Track1/Track2-roadmap/HANDOFF/Phase A-D 작성 완료 | [#361](https://github.com/devlikebear/tars/pull/361) |

## PR 매핑 계획

| Track / Phase | PR 제목 (예정) | 대상 repo | 상태 |
|---|---|---|---|
| Planning docs | `docs(dogfooding): planning docs and handoff` | tars | 완료 ([#361](https://github.com/devlikebear/tars/pull/361)) |
| Track 1 | `chore(dogfooding): slim core (remove research + schedule)` | tars | 완료 ([#361](https://github.com/devlikebear/tars/pull/361)) |
| Track 2 Phase A (아키텍처 재설계 docs) | `docs(dogfooding): pivot phase A to skill+CLI architecture` | tars | 머지 대기 ([#362](https://github.com/devlikebear/tars/pull/362)) |
| Track 2 Phase A (실제 구현) | `feat: add log-watcher + github-ops skills` | **tars-skills** | 머지 대기 ([tars-skills#3](https://github.com/devlikebear/tars-skills/pull/3)) |
| Track 2 Phase A (foo 시딩) | `chore: seed Go todo API with intentional bugs` | **tars-examples-foo** | 완료 (main 직접 푸시) |
| Track 2 Phase B | `feat: add log-anomaly-detect skill` | **tars-skills** | 미시작 |
| Track 2 Phase C | `feat: add fix-and-pr skill (AutoResearch loop)` | **tars-skills** | 미시작 |
| Track 2 Phase D | `feat: add bar validation skill (+ optional knowledge)` | **tars-skills** | 미시작 |

**중요**: Phase A 이후 모든 구현 PR은 `tars` 본 repo가 아닌 `tars-skills` 또는 `tars-examples-foo`/`tars-examples-bar` repo로 향한다.

## 컨텍스트 보존 체크리스트 (PR 작성 전)

- [ ] HANDOFF.md "현재 상태" 갱신
- [ ] HANDOFF.md "이력" 행 추가
- [ ] 진행 중 페이즈 계획서의 "결정 기록" 갱신
- [ ] 새 미해결 결정이 있으면 HANDOFF "미해결 결정"에 추가
- [ ] PR description에 HANDOFF 링크 + "다음 PR 예고" 포함
- [ ] (TARS 본 repo 한정) CHANGELOG.md 갱신 (사용자 영향 있을 때)
- [ ] (TARS 본 repo 한정) VERSION.txt 범프 (Track 단위 또는 큰 페이즈 완료 시)
- [ ] (tars-skills repo 한정) `registry.json`에 skill/plugin 엔트리 추가 + 버전 명시
