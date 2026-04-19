# Track 2 — Monitored Ops 로드맵

**Status**: Planning (페이즈별 상세 계획서는 사용자 합의 후 작성)
**Branch (제안)**: `feat/dogfooding-monitored-ops` (페이즈마다 별도 worktree 권장)
**Depends on**: [Track 1 — Core Slim](./track1-core-slim.md)
**Blocks**: 없음

## 목표

가상 사이트 `tars-examples-foo`(Docker로 운영, 의도적 버그 포함)의 서버 로그를 TARS가 주기적으로 감시 → GitHub 이슈 등록 → 픽스 PR 제출까지 자동화한다. 시연용이지만 **실제로 도그푸딩**해서 부족한 부분을 발견한다.

**핵심 제약**: TARS 코어 코드 변경은 최소화. 모든 도메인 기능은 plugin/skill로 구현. Phase D에서 `tars-examples-bar`(다른 스택)를 추가할 때 **코어 코드 변경 0 라인**이어야 진짜 범용 검증 통과.

## 아키텍처 모델 (재확인)

새 코어 추상화 0개. 기존 메커니즘만 사용:

```
[기존 코어 — 변경 거의 없음]
  세션 (per monitored target = per session)
  cron (SessionTarget으로 세션 깨움)
  gateway.Spawn (cron 트리거 → background agent run)
  plugin (도메인 어댑터)
  skill (작업 절차)
  memory_search (이슈 dedup)
  approval (휴먼 게이트)

[Track 2에서 추가하는 것 — 모두 코어 밖]
  plugin: log-watcher       (docker_logs / file_tail / k8s_logs 도구)
  plugin: github-ops        (gh issue/pr 도구, exec gh 래핑)
  skill:  log-anomaly-detect.md
  skill:  fix-and-pr.md     (AutoResearch propose/test/verify 패턴 내장)
  skill:  knowledge-compile.md  (선택, Karpathy LLM-wiki 패턴)

[설정으로 끝]
  foo 세션 (worker kind) + cron 잡 1개 (*/30 * * * *)
  bar 세션 + cron 잡 1개 (Phase D, 같은 plugin/skill 재사용)
```

## Phase 개요

### Phase A — 인프라 (foo 데모 + plugin 2개)

**목표**: foo 가상 사이트가 실제로 Docker로 돌고, log-watcher와 github-ops plugin이 도구로 등록되어 채팅에서 호출 가능.

**산출물:**
- 별도 repo `tars-examples-foo` (사용자 GitHub) — 의도적 버그 시드된 작은 Go HTTP API + sqlite + Dockerfile
- plugin: `log-watcher` (`docker_logs`, `file_tail` 도구)
- plugin: `github-ops` (`gh_issue_create`, `gh_issue_search`, `gh_pr_create_draft`, `gh_worktree_*` 등 도구)
- 두 plugin의 단위 테스트 + manifest

**검증:**
- `docker compose up` 으로 foo 기동 → `/health` OK
- TARS 채팅에서 `docker_logs(container="foo")` 호출 → 로그 200줄 수신
- TARS 채팅에서 `gh_issue_search(repo="me/tars-examples-foo")` 호출 → 0건 응답

**예상 시간**: 1-1.5일

---

### Phase B — 감시→이슈 흐름

**목표**: 30분마다 foo 세션이 자동으로 깨어나 로그를 스캔, anomaly를 분류, 신규면 GitHub 이슈로 등록. 동일 이슈 dedup.

**산출물:**
- skill: `log-anomaly-detect.md` (호출 절차: docker_logs → anomaly 분류 LLM 프롬프트 → memory_search dedup → gh_issue_create)
- foo 운영용 worker 세션 1개 (kind=worker, persistent)
- cron 잡 1개: `*/30 * * * *` → 해당 세션 깨움 + skill 호출 prompt
- 이슈 템플릿 (제목/본문/라벨 컨벤션) — skill 내부 정의

**검증:**
- foo에 의도적 버그 트리거 (예: nil panic 유발 endpoint 호출)
- 30분 내 GitHub 이슈 자동 등록
- 같은 버그 다시 발생 → 새 이슈 생성 X (기존 이슈에 "재발" 코멘트만)
- **2주 운영** 후: 등록 이슈 중 진짜 버그 비율 ≥50%

**예상 시간**: 2-3일 + 2주 관찰 기간

---

### Phase C — fix→PR 흐름 (AutoResearch 패턴)

**목표**: Phase B에서 등록된 이슈에 대해 fix-and-pr skill이 패치를 시도, 테스트가 통과하면 draft PR 제출. 사람 리뷰/머지.

**산출물:**
- skill: `fix-and-pr.md` (AutoResearch propose→test→verify 루프를 skill 내부 절차로)
  - propose: 코드 위치 추정 → 패치 제안
  - act: foo repo의 격리 worktree에서 패치 적용
  - verify: foo의 테스트 실행 (예: `go test ./...`) + 로그 anomaly 재현 시도
  - 통과 시 draft PR, 실패 시 최대 N회 재시도 후 "수동 검토 요청" 코멘트
- worktree 격리: TARS workspace 안의 별도 디렉토리(`workspace/managed-repos/foo/<branch>/`)에서 작업, main 브랜치 절대 안 건드림
- approval OTP 게이트: PR 생성 직전 사용자 승인 (Telegram or 콘솔)
- cron 잡 1개 추가: 이슈 등록 후 1시간 뒤에 fix 시도 (또는 사용자 트리거)

**검증:**
- Phase B에서 등록된 이슈 1개를 수동 트리거 → fix-and-pr skill 실행 → draft PR 생성됨
- worktree 격리 확인 — TARS 자기 repo와 foo repo가 분리되어 있고 cross-contamination 없음
- approval 미승인 시 PR 생성 안 됨
- fix 실패 케이스 — 제대로 "수동 검토 요청" 코멘트 + 재시도 안 함

**예상 시간**: 3-4일

---

### Phase D — 검증 게이트 (bar 추가) + 지식 축적 (선택)

**목표**: 다른 스택의 가상 사이트 `tars-examples-bar`를 추가. **TARS 코어 코드 변경 0 라인**으로 동작해야 진짜 범용. 추가로 Karpathy LLM-wiki 패턴으로 운영 지식 자가 축적 (선택).

**산출물:**
- 별도 repo `tars-examples-bar` — foo와 다른 스택 (예: foo가 Go면 bar는 Node/Python, 또는 다른 DB)
- bar 운영용 worker 세션 + cron 잡 (foo와 같은 skill/plugin 재사용)
- (선택) skill: `knowledge-compile.md` — 야간 cron으로 호출. 프로젝트별 `project-knowledge.md` 자가 유지 (이슈 패턴, fix 히스토리, 운영 노트)
  - reflection 코드 변경 X — skill로 동일 효과
- 다음 fix 작업이 wiki를 먼저 읽어 정확도 향상되는지 측정

**검증 (도메인 종속성 게이트):**
- bar 등록 시 코어 코드 변경 0 라인 (`git diff main..HEAD -- internal/ cmd/` → 0 lines)
- bar에 의도적 버그 트리거 → Phase B/C 흐름이 그대로 작동 → 이슈 + PR 생성됨
- (선택) wiki 적용 전후 fix 정확도 비교

**예상 시간**: 1.5-2일 (wiki 포함 시 +1일)

---

## 전체 예상 시간

| Phase | 시간 |
|---|---|
| A. 인프라 | 1-1.5일 |
| B. 감시→이슈 | 2-3일 + 2주 관찰 |
| C. fix→PR | 3-4일 |
| D. 검증 게이트 (+ wiki) | 1.5-3일 |
| **합계** | **8-11.5일 코딩 + 2주 관찰 기간** |

규모상 페이즈별 worktree + PR 분리 권장. 각 Phase는 독립적으로 동작하는 무언가를 산출 (수직 슬라이스 원칙).

## 위험 / 미해결 결정

각 페이즈 디테일 계획서 작성 시 다시 짚을 사항:

1. **fix-and-pr skill의 LLM 비용** — 매 시도마다 코드 분석 + 패치 + 테스트 결과 분석. heavy tier 사용 시 비용 폭증 가능. budget 게이트 필요할지.
2. **worktree 정리 정책** — 실패한 fix 시도의 worktree 누적 시 디스크 압박. 자동 cleanup or pulse autofix 후보?
3. **GitHub rate limit** — gh CLI 통한 호출이 빈번하면 API 한도. cron 간격 + dedup이 충분한지.
4. **Telegram approval UX** — Phase C 휴먼 게이트가 매번 Telegram 알림이면 피로. 일별 한도 또는 dashboard 통한 승인 옵션.
5. **bar의 "다른 스택" 구체화** — Phase D에서 결정. foo가 Go라 가정하면 bar는 Node 또는 Python 권장.

## Phase 디테일 계획서 (작성 예정)

**아직 작성 안 함**. Track 1 완료 + Phase A 직전에 디테일 계획서를 만든다 (계획 변경 여지 큼).

작성 시 파일명:
- `track2-phase-a-infra.md`
- `track2-phase-b-detect-and-issue.md`
- `track2-phase-c-fix-and-pr.md`
- `track2-phase-d-validate-and-knowledge.md`

각 파일은 작업 체크리스트 + Checkpoint 형식 (Track 1과 동일 구조).

## Out of Scope (Track 2 v1)

- 자동 머지 — PR 머지는 항상 사람.
- TARS 자기 자신 fix — 도그푸딩 대상 아님.
- 다중 platform 알림 — Telegram만.
- 클라우드 로그 어댑터 (CloudWatch 등) — Phase A는 docker logs만.
- Web UI에서 monitored target 관리 — CLI/cron만으로 시작.
- Multi-tenant — 사용자 본인 운영 시나리오만.
