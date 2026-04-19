# Track 2 — Phase C: fix→PR 흐름 (AutoResearch 패턴)

**Status**: Planning
**Branch (제안)**: `feat/dogfooding-phase-c-fix-pr`
**Depends on**: Track 2 Phase B
**Blocks**: Track 2 Phase D

---

## 단독 진입 시 컨텍스트 (5분)

> 이 페이즈만 보고 작업 시작 가능하도록 정리됨. 더 자세한 그림은 [README.md](./README.md), 진행 상태는 [HANDOFF.md](./HANDOFF.md).

**도그푸딩 본질**: TARS = 다중 프로젝트 자동화 호스트. 코어에 새 추상화 추가 X.

**이 페이즈가 하는 일**:
1. `fix-and-pr` skill 작성 — Phase B에서 등록된 이슈에 대해 fix를 시도하는 절차서
2. **Karpathy AutoResearch 패턴**을 skill 내부 절차로 흡수: propose(패치 제안) → act(worktree에서 적용) → verify(테스트 실행 + 로그 anomaly 재현 시도) → 통과 시 draft PR / 실패 시 최대 N회 재시도 → 실패 시 "수동 검토 요청" 코멘트
3. worktree 격리 정책 — TARS workspace 안 별도 디렉토리. main 브랜치 절대 안 건드림
4. **approval OTP 게이트** — PR 생성 직전 사용자 승인 (Telegram 또는 콘솔)
5. cron/트리거 옵션 — 이슈 등록 1시간 후 자동 시도 또는 사용자 수동 트리거 (결정)

**직전 페이즈 산출물 (Phase B)**:
- `log-anomaly-detect` skill + foo worker 세션 + cron 잡
- 등록된 GitHub 이슈에 anomaly signature, 발생 컨텍스트, 추정 컴포넌트 정보 포함
- dedup 메모리 (`dogfooding/<repo>` scope)

**이 페이즈가 다음으로 넘기는 것**: Phase D는 `tars-examples-bar` 등록으로 코어 변경 0 라인 검증 + (선택) knowledge-compile skill로 운영 지식 축적.

---

## 목표

- 이슈 1개 → 격리 worktree에서 패치 시도 → 테스트 + anomaly 재현 검증 → 통과 시 draft PR
- 실패 시 재시도 (최대 N회), 그래도 실패면 이슈에 "수동 검토 요청" 코멘트
- approval 게이트 — PR 생성 직전 사용자 승인 없이는 push/PR 안 함
- worktree 격리 — TARS repo와 foo repo가 cross-contamination 없음

## Out of Scope (Phase C)

- 자동 머지 — 절대 안 함, 사람이 머지
- 다중 이슈 동시 처리 — 한 번에 한 이슈
- 코드 위치 자동 학습 — 단순 grep/heuristic + LLM 추정만
- bar 적용 — Phase D
- AutoResearch 일반 모듈 추출 — fix-and-pr skill 내부에 한정 (코어 안 건드림)

## 작업 체크리스트

### 1. worktree 격리 정책 결정 + 도구 점검

- [ ] 격리 위치 확정: `workspace/managed-repos/<repo-slug>/<branch-name>/` (Phase A 결정 기록 참조)
- [ ] 격리 보장 — fix-and-pr skill이 절대 TARS 자체 repo의 git 상태에 영향 안 주도록 안전장치
  - skill 본문에 명시 "이 작업은 항상 `workspace/managed-repos/` 내부에서만"
  - github-ops plugin의 `gh_worktree_setup` 도구가 이 위치 강제
- [ ] worktree 누적 시 cleanup 정책 — fix 시도 끝나면 즉시 cleanup (성공/실패 무관)
- [ ] 디스크 압박 시 fallback — `gh_worktree_setup`이 기존 동명 worktree 발견 시 cleanup 후 재생성

### 2. approval 게이트 메커니즘 결정

- [ ] 옵션 A: TARS의 기존 `internal/approval/` OTP를 통한 Telegram 승인 (CLAUDE.md 참고 — `Request(chatID, timeout)` API)
- [ ] 옵션 B: 콘솔 UI에 승인 버튼 (이미 있으면 활용 / 없으면 신규)
- [ ] 옵션 C: skill 본문에 "approval 도구 호출 → 승인 받을 때까지 대기" 명시 (도구로 흡수)
- [ ] **결정 권장: 옵션 A** — 기존 인프라 재사용, 새 추상화 X
- [ ] approval 도구 wrapper 필요 시 추가 (예: `request_approval(reason, payload, timeout)` — 기존 OTP API 래핑)
- [ ] approval 미승인/타임아웃 → fix 시도 중단, 이슈에 "승인 미받음으로 보류" 코멘트

### 3. fix-and-pr skill 작성

- [ ] YAML frontmatter:
  - `name: fix-and-pr`
  - `description: Attempt to fix a GitHub issue and submit a draft PR (with verify loop and human approval gate)`
  - `user_invocable: true`
  - 도구: `gh_issue_*`, `gh_pr_create_draft`, `gh_worktree_setup`, `gh_worktree_cleanup`, `read_file`, `edit_file`, `glob`, `exec`, `request_approval`, `memory_search`, `memory_save`
- [ ] 입력 파라미터: `repo`, `issue_number`, `repo_local_path` (해당 repo가 로컬에 clone된 경로), `verify_cmd` (default `make test` 또는 `go test ./...`), `max_attempts` (default 3)
- [ ] 절차 (AutoResearch 패턴):

  **Step 0: 컨텍스트 수집**
  - `gh_issue_search` 또는 직접 issue API로 이슈 본문 + 코멘트 가져옴
  - `memory_search` 로 같은 repo의 과거 fix 히스토리 조회 (Phase D의 knowledge-compile이 있으면 wiki도)
  - 이슈 본문에서 "추정 컴포넌트/파일" 추출

  **Step 1: 격리 worktree 만들기**
  - 브랜치명: `auto-fix/issue-<number>-<short-slug>`
  - `gh_worktree_setup(repo_path=<repo_local_path>, branch_name=<위>, base=main)`

  **Step 2: propose (패치 제안)**
  - `glob` + `read_file` 로 추정 파일 읽기
  - LLM에 "이 코드의 어디가 anomaly 원인이고 어떻게 패치할지" 요청
  - 출력: 변경 의도 + diff 후보

  **Step 3: act (worktree에서 적용)**
  - `edit_file` 로 패치 적용
  - 변경 라인 수 cap (예: 단일 fix 시도당 ≤200 라인) — 폭주 방지

  **Step 4: verify**
  - `exec(cmd=<verify_cmd>, cwd=<worktree>)` — 테스트 실행
  - 통과하면: anomaly 재현 시도 (가능하면) — `exec` 로 컨테이너 재기동 + 트리거 endpoint
  - **통과 기준**: 테스트 통과 AND (anomaly 재현 시도 시 에러 사라짐 OR 재현 불가능)

  **Step 5a: 통과 시**
  - `request_approval(reason="draft PR 생성", payload={branch, diff_summary, verify_log}, timeout=24h)`
  - 승인 → `git push` + `gh_pr_create_draft(repo, head=<branch>, body=<...>)`
  - PR body에 "이슈 #N 자동 fix 시도 / 검증 결과 / 사람 리뷰 필수" 명시
  - `gh_worktree_cleanup`
  - `memory_save` 로 fix 히스토리 기록 (signature + 이슈 + PR + 결과)

  **Step 5b: 실패 시**
  - `attempts < max_attempts` → Step 2부터 재시도 (LLM에 직전 시도 + 실패 로그 컨텍스트로 줌)
  - `attempts == max_attempts` → 이슈에 "자동 fix N회 실패" 코멘트 + 실패 로그 첨부 + `gh_worktree_cleanup`

  **Step 5c: 승인 미받음**
  - 이슈에 "fix 후보 생성됨, 승인 대기 중" 코멘트 (worktree 경로 명시)
  - `gh_worktree_cleanup` (worktree는 정리, diff는 코멘트에 첨부)

- [ ] 안전장치:
  - 변경 라인 수 cap (200)
  - 재시도 cap (max_attempts)
  - 검증 명령 timeout (예: 5분)
  - LLM 호출 cap (max_attempts × 2 = LLM heavy 호출 최대치)
  - **금지 영역**: TARS 자체 repo 경로(`internal/`, `cmd/`, `frontend/console/src/`) 변경 금지 — skill 본문에 명시 + 도구 레벨 가드 가능하면 추가
- [ ] LLM tier — `heavy` (코드 패치는 가장 정확한 모델)

### 4. 트리거 정책 결정

- [ ] 옵션 A: cron — 이슈 등록 1시간 후 자동 시도 (위험: 폭주)
- [ ] 옵션 B: 사용자 수동 트리거만 (안전, 느림)
- [ ] 옵션 C: 이슈에 라벨(`auto-fix-attempt`)이 붙으면 트리거 (반자동, 권장)
- [ ] **결정 권장: 옵션 C** — 사용자가 라벨로 게이팅
- [ ] 라벨 감지 cron 잡 추가 (Phase B의 foo-monitor cron과 별개) — `*/15 * * * *` 정도

### 5. 통합 검증 (수동)

- [ ] Phase B에서 등록된 이슈 1개 선택
- [ ] `auto-fix-attempt` 라벨 추가 → 트리거 cron 또는 수동 실행
- [ ] worktree 생성 확인 (`workspace/managed-repos/foo/auto-fix/...`)
- [ ] propose/act/verify 로그 콘솔에서 확인
- [ ] approval Telegram 알림 수신 → 승인
- [ ] draft PR 생성 + foo repo의 worktree 정리 확인
- [ ] 다른 이슈 → verify 실패 시나리오 → 재시도 + 결국 "수동 검토 요청" 코멘트 확인
- [ ] approval 미승인(타임아웃 짧게 설정 후 실험) → "보류" 코멘트 확인
- [ ] **격리 검증**: TARS 자체 repo (현재 worktree)의 git 상태가 fix 시도 동안 변동 없음 확인

### 6. 마무리

- [ ] CHANGELOG.md 갱신
- [ ] HANDOFF.md 갱신 (활성 페이즈를 Phase D로)
- [ ] PR description: "Phase C 완료 — fix-and-pr skill + AutoResearch 루프 / 자동 머지 X / approval 필수"

## Checkpoint: Phase C 완료 확인

**구현 확인:**
- [ ] `fix-and-pr.md` skill 존재 + 절차/안전장치 명확
- [ ] approval 게이트 동작 (OTP 또는 동등)
- [ ] 라벨 트리거 cron 잡 (옵션 C 채택 시) 또는 수동 트리거 경로
- [ ] worktree 격리 정책 문서화 + 강제

**실행 확인:**
- [ ] `make test` 통과
- [ ] skill 1회 실행 → 정상 종료 (에러 로그 없음)
- [ ] approval 미승인 → push/PR 안 일어남 확인

**수동 확인:**
- [ ] 이슈 1건 라벨 → 자동 fix 시도 → verify 통과 → approval → draft PR 생성 (전 과정)
- [ ] 다른 이슈 verify 실패 → 재시도 → 한도 초과 → "수동 검토 요청" 코멘트
- [ ] 승인 미받음 → "보류" 코멘트
- [ ] TARS 자체 repo의 git 상태 변동 0

**통과 후 Phase D로 진행.**

---

## 결정 기록 (작업 진행하며 채워짐)

| 항목 | 결정 | 사유 | 결정일 |
|---|---|---|---|
| approval 메커니즘 | TBD (OTP via Telegram 추천) | 기존 인프라 재사용 | |
| 트리거 정책 | TBD (라벨 기반 옵션 C 추천) | 안전 + 사용자 통제 | |
| max_attempts | TBD (3 추천) | LLM 비용 + 무한루프 방지 | |
| 변경 라인 cap | TBD (200 추천) | 한 fix가 너무 큰 변경 안 되도록 | |
| verify_cmd 기본값 | TBD (`make test` 또는 `go test ./...`) | 대상 repo 컨벤션 따름 | |
| LLM tier | TBD (heavy 추천) | 코드 패치 정확도 | |
