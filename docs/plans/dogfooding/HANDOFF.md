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
- **활성 페이즈**: Phase A (인프라) — **코드 구현 완료, 수동 검증 대기**
- **활성 단계**: 
  1. TARS core: 아키텍처 pivot docs 커밋 완료 — PR [#362](https://github.com/devlikebear/tars/pull/362) 머지 대기
  2. `tars-skills`: `log-watcher`, `github-ops` skill+CLI 추가 완료 — PR [tars-skills#3](https://github.com/devlikebear/tars-skills/pull/3) 머지 대기
  3. `tars-examples-foo`: Go todo API + Docker + 의도된 버그 3종 + JSON 로그 main에 직접 푸시 완료 (<https://github.com/devlikebear/tars-examples-foo>)
  4. 다음: 수동 검증 (`tars skill install` → foo `docker compose up` → skill 호출)
- **활성 worktree**: `.claude/worktrees/interesting-herschel-a72794` (`claude/interesting-herschel-a72794` branch) — 마무리 후 제거 예정
- **블로커**: 없음
- **마지막 빌드**:
  - TARS core: docs-only 변경이라 빌드 영향 없음
  - `tars-skills`: `bash skills/log-watcher/tests/test_log_watcher.sh` 11/11, `bash skills/github-ops/tests/test_github_ops.sh` 14/14 통과
  - `tars-examples-foo`: `go build ./...` 클린

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

✅ **Phase A 완료 (코드/커밋 분):**
1. TARS core docs pivot → PR [#362](https://github.com/devlikebear/tars/pull/362)
2. `tars-skills` feat/phase-a-skills → PR [tars-skills#3](https://github.com/devlikebear/tars-skills/pull/3) (log-watcher v0.1.0, github-ops v0.1.0, 25개 단위 테스트 통과)
3. `tars-examples-foo` main에 초기 커밋 완료 — Go todo API + Docker + 의도된 버그 3종 (/bug/panic, /bug/bad, PUT race) + slog JSON 로그

⏳ **다음 세션 첫 과제 (수동 검증):**
1. **PR 머지**:
   - 먼저 [tars-skills#3](https://github.com/devlikebear/tars-skills/pull/3) 리뷰·머지 (도그푸딩에서 의존)
   - TARS core [#362](https://github.com/devlikebear/tars/pull/362) 리뷰·머지
2. **설치 검증**:
   - `./bin/tars skill install log-watcher` → 성공, workspace에 파일 배치 확인
   - `./bin/tars skill install github-ops` → 성공
3. **foo 기동 검증**:
   - `tars-examples-foo` clone → `docker compose up --build` → `curl localhost:8080/health` 200
   - `curl -X POST localhost:8080/bug/panic` → 로그에 "panic recovered" stack trace 출력 확인
4. **skill 왕복 검증**:
   - TARS 콘솔에서 "log-watcher로 tars-examples-foo 컨테이너 최근 100줄" → skill 디스패치 → bash로 CLI 실행 → JSON 엔벨로프 수신
   - "github-ops로 devlikebear/tars-examples-foo 이슈 목록" → 0건 응답
   - "github-ops로 test-branch 워크트리 만들어" (foo clone 경로 사용) → worktree 생성 → cleanup → 정리 확인
5. 수동 검증 통과 후 **Phase B** (`track2-phase-b-detect-and-issue.md`) 시작.

⚠️ **검증 중 발견될 가능성 높은 이슈 (사전 점검 포인트)**:
- `tars skill install`이 `tars-skills` registry.json을 어떻게 페치하는지 (캐시 버전 주의). 필요 시 `tars skill update` 또는 캐시 초기화 확인.
- skill 프론트매터의 `recommended_tools: [bash]`가 TARS chat 툴 선택에 잘 반영되는지 — daily-briefing 동작 여부로 baseline 확인.
- `docker logs` 출력이 slog JSON 라인과 `request` INFO 라인 혼합이라 log-watcher의 `level` 필드가 섞여 보일 수 있음. Phase B의 anomaly-detect에서 ERROR만 필터링하면 됨 — Phase A는 수신 여부까지만 검증.

## 진행 이력 (역시간순)

| 날짜 | 트랙/페이즈 | 작업 | 결과 | PR |
|---|---|---|---|---|
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
