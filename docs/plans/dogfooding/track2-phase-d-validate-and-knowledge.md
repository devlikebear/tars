# Track 2 — Phase D: 검증 게이트 (bar 추가) + 지식 축적 (선택)

**Status**: Planning
**Branch (제안)**: `feat/dogfooding-phase-d-validate`
**Depends on**: Track 2 Phase C
**Blocks**: 없음 (도그푸딩 v1 마지막 페이즈)

---

## 단독 진입 시 컨텍스트 (5분)

> 이 페이즈만 보고 작업 시작 가능하도록 정리됨. 더 자세한 그림은 [README.md](./README.md), 진행 상태는 [HANDOFF.md](./HANDOFF.md).

**도그푸딩 본질**: TARS = 다중 프로젝트 자동화 호스트. 코어에 새 추상화 추가 X. 이 페이즈가 그 원칙의 **결정적 검증 게이트**다.

**이 페이즈가 하는 일**:
1. `tars-examples-bar` repo 생성 — foo와 다른 스택 (HANDOFF "미해결 결정"의 bar 스택 결정 따름. 예: foo가 Go면 bar는 Node 또는 Python)
2. bar 운영용 worker 세션 + cron 잡 등록 — **Phase A~C에서 만든 plugin/skill 그대로 재사용**
3. **검증 게이트**: TARS 코어 코드 변경 0 라인이어야 통과 (`git diff main..HEAD -- internal/ cmd/` → 0 lines)
4. (선택) Karpathy LLM-wiki 패턴 적용 — `knowledge-compile.md` skill 작성. 야간 cron으로 호출. 프로젝트별 `project-knowledge.md` 자가 유지. **reflection 코드 변경 X** — skill로 동일 효과
5. wiki 적용 전후 fix 정확도 비교 (선택)

**직전 페이즈 산출물 (Phase C)**:
- `fix-and-pr` skill (AutoResearch propose/test/verify 루프 내장)
- worktree 격리 정책 + approval 게이트
- foo의 자동 fix→draft PR 흐름 동작

**이 페이즈 이후**: 도그푸딩 v1 완료. 다음은 다른 use case (리서치/창작) 검토 또는 v2 개선.

---

## 목표

- bar repo 등록 시 TARS 코어 코드 변경 0 라인 (도메인 종속성 게이트)
- bar의 의도적 버그 → Phase B/C 흐름이 그대로 작동
- (선택) knowledge wiki가 다음 fix 정확도를 측정 가능하게 향상

## Out of Scope (Phase D)

- 3번째 프로젝트 추가 — bar로 충분 (2개 → 다중 검증)
- wiki를 reflection에 통합 — skill로만 (코어 변경 X)
- 클라우드 로그 어댑터 — 후속 v2

## 작업 체크리스트

### 1. tars-examples-bar repo 생성 (별도 repo)

- [ ] 스택 결정 (HANDOFF "미해결 결정" 참조). foo가 Go라 가정 시:
  - 옵션 A: Node + express + sqlite (다른 언어 + 같은 DB)
  - 옵션 B: Python + FastAPI + PostgreSQL (다른 언어 + 다른 DB)
  - **추천: 옵션 A (Node)** — 다른 언어, 도커 친숙, 의도적 버그 시드 쉬움
- [ ] 새 repo 생성 (`gh repo create devlikebear/tars-examples-bar --public ...`)
- [ ] 작은 HTTP API 작성 (선택한 스택)
- [ ] Dockerfile + docker-compose.yml
- [ ] 의도적 버그 시드 — foo와 다른 종류 3개 이상:
  - [ ] uncaught promise rejection (Node) / unhandled exception (Python)
  - [ ] race condition on counter
  - [ ] memory leak (closures, timers)
  - [ ] connection pool exhaustion
- [ ] 로그 포맷: JSON structured (Node면 pino, Python이면 structlog 등)
- [ ] README에 "TARS 도그푸딩 시연용 (bar 인스턴스)" 명시
- [ ] `docker compose up` → `/health` OK 검증

### 2. bar 등록 (코드 변경 0)

- [ ] bar 운영용 worker 세션 1개 생성 — Phase B의 foo 세션 생성 절차 그대로
- [ ] cron 잡 추가 — Phase B의 foo-monitor와 같은 형태:
  - `name: bar-monitor`
  - `schedule: */30 * * * *` (foo와 시간 겹치지 않게 offset 권장)
  - `prompt: log-anomaly-detect skill을 호출해서 bar 컨테이너 로그를 스캔해`
  - `session_target: <bar worker session id>`
- [ ] 라벨 트리거 cron 잡도 동일하게 (Phase C 트리거 정책에 따라)
- [ ] **검증 게이트**: `git diff $(git merge-base HEAD main)..HEAD -- internal/ cmd/ frontend/console/src/` → 0 lines
  - 만약 0이 아니면 → bar 등록을 위해 코어를 만진 것 → **회귀** → 원인 분석 + 도메인 종속 부분을 plugin/skill로 빼냄

### 3. 통합 검증 (수동)

- [ ] bar 의도적 버그 트리거 → 30분 내 bar repo에 `[auto]` 이슈 등록 확인
- [ ] 같은 버그 재트리거 → 신규 이슈 X, 코멘트만
- [ ] 이슈에 `auto-fix-attempt` 라벨 → fix-and-pr skill 실행 → bar repo의 worktree 격리 → patch → verify (`npm test` 또는 `pytest`) → approval → bar repo에 draft PR
- [ ] **격리 재검증**: foo와 bar의 worktree가 별도 디렉토리, cross-contamination 없음
- [ ] 콘솔에서 두 worker 세션 / 두 cron 잡 / 두 repo PR 모두 분리되어 보임

### 4. (선택) knowledge-compile skill 작성

이 작업은 **선택**. PR 사이즈 + 시간 여유 보고 사용자가 결정. 분리 시 별도 PR(예: `feat(dogfooding): knowledge wiki skill`).

- [ ] YAML frontmatter:
  - `name: knowledge-compile`
  - `description: Compile per-project knowledge wiki from past issues, fixes, and ops notes`
  - `user_invocable: true`
- [ ] 입력 파라미터: `repo`, `output_path` (default `workspace/knowledge/<repo-slug>/wiki.md`), `lookback` (default 30d)
- [ ] 절차 (Karpathy LLM-wiki 패턴):
  1. **소스 수집**: `gh_issue_search` 로 최근 이슈 + 코멘트 / `gh_pr_*` 로 머지된 fix PR / `memory_search` 로 dogfooding scope 메모
  2. **컴파일**: LLM에 "이 자료들을 wiki로 정리해라 — 컴포넌트별 페이지, 자주 발생하는 에러 패턴, 효과적이었던 fix 패턴, cross-reference"
  3. **저장**: `output_path`에 markdown 작성. 기존 wiki 있으면 갱신 (덮어쓰기 X, merge)
  4. **요약**: 새로 추가된 페이지 N개, 갱신된 N개
- [ ] 야간 cron 잡 추가:
  - `name: foo-knowledge-nightly`
  - `schedule: 0 3 * * *` (03:00 KST 등)
  - `prompt: knowledge-compile skill을 호출해서 foo wiki를 갱신해`
  - bar에도 동일 (별도 잡)
- [ ] fix-and-pr skill 절차의 Step 0(컨텍스트 수집)에 wiki 읽기 추가
- [ ] 측정 (선택): wiki 도입 전후 fix 시도 성공률 비교 — 추적 이슈로

### 5. 마무리

- [ ] CHANGELOG.md 갱신 — Phase D + (knowledge 포함 시) wiki 추가
- [ ] HANDOFF.md 갱신 — Track 2 완료, 도그푸딩 v1 마일스톤 도달
- [ ] VERSION.txt 범프 (도그푸딩 v1 완료 마일스톤)
- [ ] **회고 추가**: HANDOFF.md에 "도그푸딩 v1 회고" 섹션 — 잘된 것, 부족한 것, v2 후보
- [ ] PR description: "Phase D 완료 — bar 등록 시 코어 변경 0 라인 검증 통과 / 도그푸딩 v1 마일스톤"

## Checkpoint: Phase D 완료 확인

**구현 확인:**
- [ ] bar repo 별도 존재 + Docker 기동 가능
- [ ] bar worker 세션 + bar-monitor cron 잡 등록
- [ ] (선택) knowledge-compile skill + 야간 cron

**검증 게이트 (가장 중요):**
- [ ] `git diff $(git merge-base HEAD main)..HEAD -- internal/ cmd/ frontend/console/src/` → **0 lines**
- [ ] 만약 0이 아니면 → 도메인 종속이 코어로 새어든 것 → **회귀**, 머지 금지

**실행 확인:**
- [ ] `make test` + `make vet` + `make build` 통과
- [ ] foo와 bar 두 worker 세션 모두 정상 동작
- [ ] 두 cron 잡 모두 정상 실행

**수동 확인:**
- [ ] bar 버그 트리거 → bar repo에 자동 이슈 등록
- [ ] bar 이슈 라벨 → fix 시도 → bar repo에 draft PR
- [ ] foo와 bar의 worktree 격리 (cross-contamination 0)
- [ ] (선택) knowledge wiki 1회 생성 + 다음 fix 시도가 wiki 참조 확인

**통과 후 도그푸딩 v1 완료. Track 2 종료.**

---

## 결정 기록 (작업 진행하며 채워짐)

| 항목 | 결정 | 사유 | 결정일 |
|---|---|---|---|
| bar 스택 | TBD (Node 추천) | 다른 언어 + 도커 친숙 | |
| bar repo 위치 | TBD (`devlikebear/tars-examples-bar` 추정) | | |
| bar의 첫 시드 버그 종류 | TBD | foo와 다른 종류 3+ | |
| knowledge-compile 포함/분리 | TBD | PR 사이즈 + 시간 여유 | |
| wiki 저장 위치 | TBD (`workspace/knowledge/<repo>/wiki.md` 추천) | | |
| knowledge cron 시각 | TBD (`0 3 * * *` 추천 — 03:00 KST) | reflection 시간대와 겹침 주의 | |
