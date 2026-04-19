# Track 2 — Phase A: 인프라 (foo 데모 + skill/CLI 2개)

**Status**: Planning (아키텍처 재설계 완료 2026-04-19)
**Branch (제안)**: `feat/phase-a-skills` (tars-skills repo), `chore/seed-foo` (tars-examples-foo repo)
**Depends on**: Track 1 완료
**Blocks**: Track 2 Phase B

---

## 단독 진입 시 컨텍스트 (5분)

> 이 페이즈만 보고 작업 시작 가능하도록 정리됨. 더 자세한 그림은 [README.md](./README.md), 진행 상태는 [HANDOFF.md](./HANDOFF.md).

**도그푸딩 본질**: TARS를 다중 프로젝트 운영 자동화 호스트로 쓰면서 부족한 부분을 메운다. **TARS 코어에 새 도메인 기능(빌트인 Go 플러그인/MCP 서버 포함)을 추가하지 않는다** — 시스템 프롬프트 비대화 + 과거 project 시스템 폐기 교훈. 모든 도메인 기능은 **`devlikebear/tars-skills` 외부 repo의 skill + CLI**로 만들고 `tars skill install`로 배포한다.

**이 페이즈가 하는 일**:
1. 가상 사이트 `tars-examples-foo`를 Docker로 띄울 수 있는 형태로 만든다 (**별도 repo**: `devlikebear/tars-examples-foo`).
2. **`devlikebear/tars-skills` repo에 두 skill을 추가한다**:
   - `log-watcher` — Docker/파일 로그 수집 CLI + SKILL.md 디스패처
   - `github-ops` — `gh` CLI 래퍼 (이슈/PR/worktree 조작) + SKILL.md 디스패처
3. 각 skill은 `recommended_tools: [bash]` frontmatter로 CLI를 호출한다. CLI는 단위 테스트 포함.
4. `registry.json`에 두 skill 엔트리 등록 + 버전 지정.
5. **이 페이즈에선 자동화 안 함** — 채팅에서 skill을 직접 호출(`/log-watcher …` 또는 "log-watcher skill을 써서 …") 해서 동작만 검증.

**직전 페이즈 산출물**: Track 1에서 코어 슬림화 완료 ([#361](https://github.com/devlikebear/tars/pull/361)). `internal/research`, `internal/schedule`, `internal/scheduleexpr` 처리 결정 끝남 (HANDOFF.md 이력 참조).

**이 페이즈가 다음으로 넘기는 것**: Phase B는 이 두 skill을 조합하는 `log-anomaly-detect` skill + cron 잡을 만든다.

---

## 목표

- foo 가상 사이트가 `docker compose up` 으로 기동
- `tars skill install log-watcher` / `tars skill install github-ops` 성공
- TARS 채팅에서 `log-watcher` skill 호출 → bash 툴이 CLI 실행 → foo 컨테이너 최근 로그 반환
- TARS 채팅에서 `github-ops` skill 호출 → `gh issue list` 결과 반환
- 두 CLI 모두 단위 테스트 통과

## Out of Scope (Phase A)

- TARS 본 repo 코드 변경 (이번 phase에서는 **docs 업데이트만**)
- 빌트인 Go 플러그인 / MCP 서버 경로 (전면 기각)
- 자동 스캔/cron — Phase B
- 이슈 자동 등록 흐름 — Phase B
- fix PR — Phase C
- bar repo — Phase D
- log-watcher의 k8s/CloudWatch 어댑터 — 후속

## 아키텍처 개요

```
[TARS 채팅 세션]
      │
      │ 사용자: "log-watcher skill로 foo 컨테이너 로그 최근 200줄 가져와"
      │
      ▼
[skill 로더] ──── SKILL.md (frontmatter: recommended_tools: [bash])
      │
      │ skill 본문: "bash 툴로 $SKILL_DIR/log_watcher.sh <container> --tail 200 실행"
      ▼
[TARS 빌트인 bash 툴]
      │
      │ exec: log_watcher.sh → docker logs …
      ▼
[표준 출력 → LLM 컨텍스트 → 응답]
```

- 시스템 프롬프트에는 **skill description만** 적재. CLI 도구 설명은 상주하지 않음.
- skill 내부에서 필요한 CLI 이름/인자를 결정.
- 에러 처리·포매팅은 CLI 책임, 컨텍스트 관리는 skill 본문 책임.

## 작업 체크리스트

> 모든 작업은 **TARS 본 repo 바깥**에서 수행한다. TARS 본 repo 변경은 이 플랜 문서와 HANDOFF.md 업데이트 외에는 없다.

### 1. `tars-examples-foo` repo 시딩 (repo: `devlikebear/tars-examples-foo`)

빈 repo 생성 완료. 초기 커밋 투입.

- [ ] Go 프로젝트 초기화 (Go 1.22+)
- [ ] 작은 HTTP API 작성 (net/http) — 기능: todo CRUD + sqlite
- [ ] Dockerfile + docker-compose.yml 작성 (컨테이너 이름 `tars-examples-foo`)
- [ ] 의도적 버그 시드 — 다음 중 3개 이상:
  - [ ] nil pointer dereference (특정 endpoint 호출 시)
  - [ ] sqlite lock race (동시 PUT 시)
  - [ ] panic on malformed input (특정 payload)
  - [ ] memory leak (긴 슬라이스 append, 회수 없음)
  - [ ] goroutine leak (close 안 되는 채널)
- [ ] 로그 포맷: JSON structured (`slog` 권장) — `ts`, `level`, `msg`, `error` 필드 포함
- [ ] README에 "이 repo는 TARS 도그푸딩 시연용 의도적 버그 사이트"임을 명시
- [ ] `docker compose up` → `/health` OK 검증
- [ ] 검증: 의도적 버그 endpoint 1개를 curl로 호출 → 로그에 stack trace 출력 확인

**산출물**: foo repo의 첫 commit + push.

### 2. `log-watcher` skill 제작 (repo: `devlikebear/tars-skills`)

디렉토리: `skills/log-watcher/`.

- [ ] **CLI 언어 결정** (HANDOFF.md "미해결 결정" 참조 — shell/Python/TypeScript 중 선택). 초안 권장: shell (daily-briefing과 동일 스택, docker/file tail은 단순 래핑).
- [ ] `SKILL.md` 작성:
  ```yaml
  ---
  name: log-watcher
  description: "Collect recent container/file logs (docker logs or file tail) via companion CLI."
  user-invocable: true
  recommended_tools:
    - bash
  ---
  ```
  본문에 사용 예시 2가지 (docker, file) + 인자 설명 + 출력 JSON 스키마.
- [ ] `log_watcher.sh` (or `.py`) 작성:
  - 하위 명령 `docker <name> [--since DURATION] [--tail N]` → `docker logs` 래핑
  - 하위 명령 `file <path> [--tail N] [--grep REGEX]` → 파일 tail + 옵셔널 grep
  - 출력: JSON `{source, lines:[{ts, level, msg, raw}], truncated}` (JSON 라인 파싱 실패 시 raw만)
  - 에러: 컨테이너 없음 / docker 데몬 미동작 / 파일 없음 → stderr + non-zero exit
- [ ] 단위 테스트 (shell이면 `bats`, Python이면 `pytest`):
  - docker 분기: fake `docker` binary on `PATH`로 exit code/stdout 시뮬레이션
  - file 분기: 임시 파일 + grep 적용/미적용 케이스
  - 에러 케이스: 존재하지 않는 컨테이너, 권한 없는 파일
- [ ] `registry.json`에 skill 엔트리 추가 (name, version 0.1.0, tags, path, `user_invocable: true`, files).

### 3. `github-ops` skill 제작 (repo: `devlikebear/tars-skills`)

디렉토리: `skills/github-ops/`.

- [ ] CLI 언어 결정 (shell 권장 — `gh` 래퍼가 대부분).
- [ ] `SKILL.md` 작성 (frontmatter 위와 동일 형식).
- [ ] `github_ops.sh` (or `.py`) — 하위 명령:
  - `issue-search --repo R [--query Q] [--state open|closed|all] [--limit N]` → `gh issue list … --json …`
  - `issue-create --repo R --title T --body B [--label L …]` → `gh issue create …` → `{number, url}`
  - `issue-comment --repo R --issue N --body B` → `gh issue comment …`
  - `pr-draft --repo R --head H [--base main] --title T --body B` → `gh pr create … --draft`
  - `worktree-setup --repo-path P --branch B [--base main] [--slug S]` → `git -C <path> worktree add workspace/managed-repos/<slug>/<branch> -b <branch> <base>`
  - `worktree-cleanup --repo-path P --branch B [--slug S]` → `git worktree remove` + `git worktree prune`
- [ ] 입력 검증: repo/branch/slug에 shell 메타문자 차단 (regex 화이트리스트).
- [ ] 에러 처리: `gh` 미설치 / `gh auth status` 실패 → 명확한 에러 메시지.
- [ ] 단위 테스트: fake `gh`/`git` binary on `PATH`로 시뮬레이션.
- [ ] `registry.json`에 skill 엔트리 추가.

### 4. 통합 검증 (로컬)

- [ ] `tars-skills` repo의 `registry.json` 변경본을 로컬 경로 또는 브랜치에서 설치 (hub 설정에 따라 git branch 지정).
- [ ] `./bin/tars skill install log-watcher` → 성공, workspace에 파일 배치 확인.
- [ ] `./bin/tars skill install github-ops` → 성공.
- [ ] `./bin/tars` 콘솔 접속 → "log-watcher로 tars-examples-foo 컨테이너 최근 100줄 가져와" → skill 호출 → bash 툴로 CLI 실행 → 로그 수신.
- [ ] "github-ops로 devlikebear/tars-examples-foo 이슈 목록 보여줘" → 0건 응답 확인.
- [ ] "github-ops로 test 브랜치 워크트리 만들어" → 생성 확인 → cleanup 호출 → 정리 확인.

### 5. 문서화

- [ ] `tars-skills` repo README에 두 skill 추가 항목.
- [ ] TARS 본 repo의 HANDOFF.md 갱신 — 활성 페이즈를 Phase B로.
- [ ] 이 파일의 "결정 기록" 섹션에 발생한 결정 추가.
- [ ] TARS 본 repo CHANGELOG: 코드 변경 없으므로 **갱신 불필요**.
- [ ] TARS 본 repo VERSION: 코드 변경 없으므로 **범프 불필요**.

## Checkpoint: Phase A 완료 확인

**구현 확인:**
- [ ] foo repo: Docker로 기동 가능 + 의도적 버그 3종 이상
- [ ] `tars-skills/skills/log-watcher/`: SKILL.md + CLI + 테스트
- [ ] `tars-skills/skills/github-ops/`: SKILL.md + CLI + 테스트
- [ ] `tars-skills/registry.json`: 두 skill 엔트리

**실행 확인:**
- [ ] `tars-skills` 내 CLI 테스트 전부 통과
- [ ] `tars skill install` 두 건 모두 성공

**수동 확인:**
- [ ] foo `docker compose up` → `/health` OK
- [ ] foo 의도적 버그 endpoint 트리거 → 로그에 에러 출력
- [ ] TARS 콘솔에서 log-watcher skill 호출 → foo의 에러 로그 일부 수신
- [ ] TARS 콘솔에서 github-ops skill 호출 성공 (list + worktree round-trip)

**통과 후 Phase B로 진행.**
실패 시: HANDOFF.md "블로커"에 기록 + 사용자와 논의.

---

## 결정 기록

| 항목 | 결정 | 사유 | 결정일 |
|---|---|---|---|
| Phase A 구현 경로 | 빌트인 Go 플러그인 / MCP 서버 기각, **skill + CLI + `tars-skills` 외부 repo** 채택 | 빌트인 / MCP 모두 시스템 프롬프트 비대화. skill+bash는 온디맨드 로딩이라 프롬프트 부담 없음. `daily-briefing` 선례 존재. | 2026-04-19 |
| foo repo 위치 | `devlikebear/tars-examples-foo` (public) | 사용자 확정, 빈 repo 생성 완료 | 2026-04-19 |
| foo 시딩 범위 | Phase A에서 별도 repo에 push. TARS 본 repo PR과 분리. | 서로 다른 repo | 2026-04-19 |
| skill 설치 경로 | `tars skill install <name>` (internal/skillhub), `registry.json` 등록 | 기존 hub 인프라 그대로 활용 | 2026-04-19 |
| skill frontmatter | `recommended_tools: [bash]` + `user-invocable: true` | `daily-briefing` 선례 | 2026-04-19 |
| `gh` CLI 인증 방식 | 사용자 환경의 기존 `gh auth` 세션 사용 | tars 자체 토큰 관리 회피 | 2026-04-19 |
| worktree 격리 위치 | `workspace/managed-repos/<slug>/<branch>/` | 기존 계획안 유지 | 2026-04-19 |
| log-watcher CLI 언어 | TBD (shell 권장, 대안 Python) | 선택 근거는 HANDOFF.md 참조 | |
| github-ops CLI 언어 | TBD (shell 권장, 대안 Python) | `gh` 래핑 위주 | |
| foo의 첫 시드 버그 종류 | TBD | foo repo 시딩 세션에서 결정 | |
