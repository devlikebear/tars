# Track 2 — Phase A: 인프라 (foo 데모 + plugin 2개)

**Status**: Planning
**Branch (제안)**: `feat/dogfooding-phase-a-infra`
**Depends on**: Track 1 완료
**Blocks**: Track 2 Phase B

---

## 단독 진입 시 컨텍스트 (5분)

> 이 페이즈만 보고 작업 시작 가능하도록 정리됨. 더 자세한 그림은 [README.md](./README.md), 진행 상태는 [HANDOFF.md](./HANDOFF.md).

**도그푸딩 본질**: TARS를 다중 프로젝트 운영 자동화 호스트로 쓰면서 부족한 부분을 메운다. **코어에 새 추상화를 추가하지 않는다** (과거 project 시스템 폐기 교훈). 모든 도메인 기능은 plugin/skill로.

**이 페이즈가 하는 일**: 
1. 가상 사이트 `tars-examples-foo`를 Docker로 띄울 수 있는 형태로 만든다 (별도 repo).
2. TARS에 두 plugin을 추가한다:
   - `log-watcher` — Docker/파일 로그를 LLM 도구로 노출
   - `github-ops` — `gh` CLI 래핑 (이슈/PR/worktree 도구)
3. 각 plugin은 단위 테스트와 manifest 포함.
4. **이 페이즈에선 자동화 안 함** — 채팅에서 도구를 직접 호출해 동작만 검증.

**직전 페이즈 산출물**: Track 1에서 코어 슬림화 완료. `internal/research`, `internal/schedule`, `internal/scheduleexpr` 처리 결정 끝남 (HANDOFF.md 이력 참조).

**이 페이즈가 다음으로 넘기는 것**: Phase B는 이 plugin 두 개를 사용하는 `log-anomaly-detect` skill + cron 잡을 만든다.

---

## 목표

- foo 가상 사이트가 `docker compose up` 으로 기동
- TARS 채팅에서 `docker_logs(container="...")` 호출 가능
- TARS 채팅에서 `gh_issue_search(repo="...")` 호출 가능
- 두 plugin 모두 manifest + 단위 테스트 포함

## Out of Scope (Phase A)

- 자동 스캔/cron — Phase B
- 이슈 자동 등록 흐름 — Phase B
- fix PR — Phase C
- bar repo — Phase D
- log-watcher의 k8s/CloudWatch 어댑터 — 후속

## 작업 체크리스트

### 1. tars-examples-foo repo 생성 (별도 repo)

위치는 [HANDOFF.md](./HANDOFF.md) "미해결 결정"에서 확인 (TBD: `devlikebear/tars-examples-foo` 추정).

- [ ] 새 repo 생성 (`gh repo create <위치> --public --description "TARS dogfooding demo target"`)
- [ ] Go 프로젝트 초기화 (TARS와 같은 스택 — Go 1.22+)
- [ ] 작은 HTTP API 작성 (Echo 또는 net/http) — 기능: todo CRUD + sqlite
- [ ] Dockerfile + docker-compose.yml 작성
- [ ] 의도적 버그 시드 — 다음 중 3개 이상:
  - [ ] nil pointer dereference (특정 endpoint 호출 시)
  - [ ] sqlite lock race (동시 PUT 시)
  - [ ] panic on malformed input (특정 payload)
  - [ ] memory leak (긴 슬라이스 append, 회수 없음)
  - [ ] goroutine leak (close 안 되는 채널)
- [ ] 로그 포맷: JSON structured (slog 또는 zerolog) — TARS와 동일 컨벤션
- [ ] README에 "이 repo는 TARS 도그푸딩 시연용 의도적 버그 사이트"임을 명시
- [ ] `docker compose up` → `/health` OK 검증
- [ ] 검증: 의도적 버그 endpoint 1개를 curl로 호출 → 로그에 stack trace 출력 확인

**산출물**: foo repo의 첫 commit + push.

### 2. log-watcher plugin 만들기

TARS 코드베이스 안에서 작업.

- [ ] `internal/plugin/builtin/log-watcher/` 디렉토리 생성 (또는 TARS의 기존 plugin 위치 확인 후 따름 — `internal/plugin/types.go` 참고)
- [ ] manifest 작성 — id, name, version, tools_provider 필드. 기존 builtin plugin이 있으면 그 패턴 모방
- [ ] 도구 1: `docker_logs`
  - 입력: `container_name` (string, required), `since` (duration string, optional, default "1h"), `tail` (int, optional, default 200)
  - 동작: `docker logs <container> --since <since> --tail <tail>` 실행 (`exec.Command`)
  - 출력: `{lines: [{ts, level, message, raw}], truncated: bool}`. JSON 라인 파싱 시도, 실패하면 raw로 둠
  - 에러: 컨테이너 없음 / Docker 데몬 미동작 시 명확한 에러
- [ ] 도구 2: `file_tail`
  - 입력: `path` (string, required), `tail` (int, optional, default 200), `grep` (string, optional)
  - 동작: 파일 읽고 마지막 N줄 + 옵셔널 grep 필터
  - 출력: `{lines: [string], truncated: bool}`
  - 에러: 파일 없음 / 권한 없음
- [ ] 도구 등록: TARS의 RegistryScope 모델 따름 — user scope에 등록 (도구명에 `pulse_`, `reflection_`, `ops_` 접두 금지)
- [ ] 단위 테스트 — 각 도구별 정상/에러 3-4 케이스. Docker 도구는 `mock` 또는 `--dry-run` 모드 분리 필요
- [ ] 검증: `make test ./internal/plugin/builtin/log-watcher/...` 통과

### 3. github-ops plugin 만들기

- [ ] `internal/plugin/builtin/github-ops/` 디렉토리 생성
- [ ] manifest 작성
- [ ] 도구 1: `gh_issue_search`
  - 입력: `repo` (string, required), `query` (string, optional), `state` (open|closed|all, default open), `limit` (int, default 20)
  - 동작: `gh issue list --repo <repo> --search <query> --state <state> --limit <limit> --json number,title,labels,createdAt,body`
  - 출력: 파싱된 JSON 배열
- [ ] 도구 2: `gh_issue_create`
  - 입력: `repo`, `title`, `body`, `labels` (string array, optional)
  - 동작: `gh issue create --repo <repo> --title <...> --body <...> --label <...>`
  - 출력: `{number, url}`
- [ ] 도구 3: `gh_issue_comment`
  - 입력: `repo`, `issue_number`, `body`
  - 동작: `gh issue comment <number> --repo <repo> --body <body>`
  - 출력: `{ok: bool, url}`
- [ ] 도구 4: `gh_pr_create_draft`
  - 입력: `repo`, `head` (branch), `base` (branch, default main), `title`, `body`
  - 동작: `gh pr create --repo <repo> --head <head> --base <base> --title <...> --body <...> --draft`
  - 출력: `{number, url}`
- [ ] 도구 5: `gh_worktree_setup` — 외부 repo의 격리 worktree 관리
  - 입력: `repo_path` (로컬 경로), `branch_name`, `base` (default main)
  - 동작: `git -C <repo_path> worktree add <managed-repos/.../<branch_name>> -b <branch_name> <base>`
  - 출력: `{worktree_path, branch}`
  - 격리 위치: `workspace/managed-repos/<repo>/<branch>/` 권장 (TARS workspace 안)
- [ ] 도구 6: `gh_worktree_cleanup` — 사용 끝난 worktree 정리
- [ ] **에러 처리**: `gh` CLI 미설치 시 명확한 에러. 인증 안 된 경우(`gh auth status` 실패) 명확한 에러
- [ ] 단위 테스트 — `gh` 호출은 mock 또는 `--dry-run` 모드 (옵션이 있으면)
- [ ] 검증: `make test ./internal/plugin/builtin/github-ops/...` 통과

### 4. plugin 활성화 + 통합 검증

- [ ] 기본 활성화 정책 결정 — 두 plugin이 builtin이면 자동 등록 / 외부 plugin이면 사용자가 명시 활성화. (TARS plugin 시스템의 기본 활성화 동작 확인 후)
- [ ] `make build` → `bin/tars` 정상 빌드
- [ ] `./bin/tars serve` 기동 → 로그에 두 plugin 등록 메시지 확인
- [ ] 콘솔 채팅에서 `docker_logs(container_name="foo")` 호출 → 200줄 수신 (foo가 떠 있다는 가정)
- [ ] 콘솔 채팅에서 `gh_issue_search(repo="<foo repo>")` 호출 → 0건 응답 (아직 이슈 없음)
- [ ] 콘솔 채팅에서 `gh_worktree_setup(repo_path="...", branch_name="test-branch")` 호출 → worktree 생성 확인
- [ ] cleanup 호출 → worktree 제거 확인

### 5. 문서화

- [ ] 두 plugin manifest 안에 description 정확히 (LLM이 도구 선택 시 참고)
- [ ] CHANGELOG.md 추가: "Phase A: log-watcher, github-ops plugins 추가"
- [ ] HANDOFF.md 갱신 — 활성 페이즈를 Phase B로
- [ ] 페이즈 디테일 plan(이 파일)의 "결정 기록" 섹션에 발생한 결정 추가

## Checkpoint: Phase A 완료 확인

**구현 확인:**
- [ ] foo repo가 별도로 존재 + Docker로 기동 가능
- [ ] log-watcher plugin: `docker_logs`, `file_tail` 두 도구 등록
- [ ] github-ops plugin: 6개 도구 (`gh_issue_search`, `gh_issue_create`, `gh_issue_comment`, `gh_pr_create_draft`, `gh_worktree_setup`, `gh_worktree_cleanup`) 등록
- [ ] 두 plugin 모두 단위 테스트 통과

**실행 확인:**
- [ ] `make test` → 0 failure
- [ ] `make vet` → 0 warning
- [ ] `make build` → `bin/tars` 생성
- [ ] `./bin/tars serve` 기동 후 로그에 plugin 등록 메시지

**수동 확인:**
- [ ] foo `docker compose up` → `/health` OK
- [ ] foo의 의도적 버그 endpoint 트리거 → 로그에 에러 출력
- [ ] 콘솔 채팅에서 `docker_logs` 호출 → foo의 에러 로그 일부 수신
- [ ] 콘솔 채팅에서 `gh_issue_search` 호출 성공
- [ ] 콘솔 채팅에서 `gh_worktree_setup` + `gh_worktree_cleanup` 정상 동작

**통과 후 Phase B로 진행.**
실패 시: HANDOFF.md "블로커"에 기록 + 사용자와 논의.

---

## 결정 기록 (작업 진행하며 채워짐)

| 항목 | 결정 | 사유 | 결정일 |
|---|---|---|---|
| foo repo 위치 | TBD | | |
| foo의 첫 시드 버그 종류 | TBD | | |
| plugin 위치 (`internal/plugin/builtin/` vs 다른 곳) | TBD | TARS 기존 plugin 패턴 확인 후 | |
| `gh` CLI 인증 방식 (개인 토큰 vs gh auth) | TBD | | |
| worktree 격리 위치 | TBD (`workspace/managed-repos/` 추천) | | |
