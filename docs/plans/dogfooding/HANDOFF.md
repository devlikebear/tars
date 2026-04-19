# Dogfooding 작업 핸드오프

**Last Updated**: 2026-04-19
**Updated By**: Phase A 재설계 세션 (worktree: `interesting-herschel-a72794`)

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
- **활성 페이즈**: Phase A (인프라) — **아키텍처 재설계 반영 중** (docs-only 커밋으로 전환)
- **활성 단계**: 
  1. TARS 본 repo에서 잘못된 빌트인 Go 플러그인 코드 삭제 완료
  2. 관련 docs(HANDOFF/README/Phase A 계획서) 재작성 진행 중
  3. PR [#362](https://github.com/devlikebear/tars/pull/362)는 docs-only로 축소하거나 닫고 새 PR 생성 예정
- **활성 worktree**: `.claude/worktrees/interesting-herschel-a72794` (`claude/interesting-herschel-a72794` branch)
- **블로커**: 없음 — 아키텍처 재설계 완료 후 일반 작업 재개
- **마지막 빌드**: 검증 재실행 필요 (코드 revert 후)
- **deferred**: `tars-examples-foo` repo 시딩은 변함없이 Phase B 시작 전에 처리. 대상 repo: `devlikebear/tars-examples-foo` (빈 repo 생성 완료 2026-04-19).

## 미해결 결정 (사용자 확인 대기)

| 결정 사항 | 선택지 | 비고 |
|---|---|---|
| `log-watcher` CLI 언어 | (a) shell, (b) Python, (c) TypeScript/Node | 단순 래핑 위주면 shell이 가장 가볍다. 구조화 파싱이 필요하면 Python. daily-briefing이 shell이라 동 스택으로 가는 것도 일관성 있음. |
| `github-ops` CLI 언어 | (a) shell, (b) Python, (c) TypeScript/Node | 대부분 `gh` CLI 래퍼라 shell이 적합. 단 JSON 가공/에러 분기가 많으면 Python. |
| `tars-examples-bar` 스택 | (a) Node, (b) Python, (c) 기타 | Phase D 시작 전 결정. foo가 Go라 가정. |

### 확정된 결정

- **아키텍처**: skill + CLI + `tars-skills` 외부 repo — 2026-04-19 사용자 확정. 빌트인 Go 플러그인 / MCP 서버 경로 모두 기각.
- **`tars-examples-foo` repo 위치**: `devlikebear/tars-examples-foo` (public) — 2026-04-19 사용자 확인, 빈 repo 생성 완료 (<https://github.com/devlikebear/tars-examples-foo>). Phase A에서 초기 커밋 투입.
- **`log-watcher`/`github-ops` 설치 경로**: `devlikebear/tars-skills` repo (skill + 부속 CLI), `tars skill install <name>` 로 설치.

(Track 1 진행 중 발생한 결정은 [track1-core-slim.md](./track1-core-slim.md) "결정 기록"에 기록)

## 다음 액션 (구체적)

⏳ **즉시 (이 세션에서 마무리)**:
1. docs-only 커밋 — HANDOFF/README/Phase A 계획서 업데이트.
2. PR [#362](https://github.com/devlikebear/tars/pull/362) 처리 결정:
   - (선택1) 본 워크트리의 docs-only 커밋만 남겨 PR 제목/본문을 `docs(dogfooding): pivot phase A to skill+CLI architecture`로 교체.
   - (선택2) 현 PR을 닫고 docs-only 신규 PR 생성.
   - 기본값: (선택1) — 코드 revert + docs 추가가 한 번에 보이는 게 히스토리상 깔끔.
3. VERSION/CHANGELOG는 코드 변경이 없으므로 범프하지 않음 (docs-only).

⏳ **다음 세션 첫 과제 (Phase A 본 작업)**:
1. **CLI 언어 결정** (위 "미해결 결정" 2개).
2. **`devlikebear/tars-skills` repo에서 작업**:
   - `skills/log-watcher/` — SKILL.md + CLI (선택된 언어). CLI는 `docker logs` / 파일 tail 기능.
   - `skills/github-ops/` — SKILL.md + CLI. `gh` 래핑 + git worktree 관리.
   - `registry.json` 에 두 skill 엔트리 추가 (version, path, tags, user_invocable, files 목록).
3. **TARS 본 repo (이번 worktree)가 아니라 tars-skills worktree에서 PR 생성**.
4. **`tars-examples-foo` repo 시딩** — Go net/http 기반 todo CRUD + sqlite, Dockerfile/docker-compose, 의도된 버그 3종 이상, JSON 구조화 로그.
5. **수동 검증**:
   - `tars skill install log-watcher` / `tars skill install github-ops` 성공.
   - TARS 콘솔에서 skill 호출 → bash 툴 통해 CLI 실행 → foo 컨테이너 로그 수신.
   - `github-ops` skill로 `gh issue list --repo devlikebear/tars-examples-foo` 0건 응답 확인.
   - git worktree 셋업/정리 round-trip 확인.
6. 수동 검증 후 **Phase B** (`track2-phase-b-detect-and-issue.md`) 시작.

## 진행 이력 (역시간순)

| 날짜 | 트랙/페이즈 | 작업 | 결과 | PR |
|---|---|---|---|---|
| 2026-04-19 | Track 2 / Phase A | **아키텍처 재설계** — 빌트인 Go 플러그인(`internal/logwatcher`, `internal/githubops`) 코드 전체 삭제, VERSION/CHANGELOG 및 `cmd/tars/main.go` blank import 되돌림. 대신 skill + CLI + `tars-skills` 외부 repo 경로 채택. 본 repo는 docs-only 커밋만 남음. | 코드 revert 완료, docs 업데이트 진행 중 | [#362](https://github.com/devlikebear/tars/pull/362) (처리 대기) |
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
| Track 2 Phase A (아키텍처 재설계 docs) | `docs(dogfooding): pivot phase A to skill+CLI architecture` | tars | 진행 중 (이 worktree) |
| Track 2 Phase A (실제 구현) | `feat: add log-watcher + github-ops skills` | **tars-skills** | 미시작 |
| Track 2 Phase A (foo 시딩) | `chore: seed Go todo API with intentional bugs` | **tars-examples-foo** | 미시작 |
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
