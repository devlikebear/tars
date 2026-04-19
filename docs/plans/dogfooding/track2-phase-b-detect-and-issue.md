# Track 2 — Phase B: 감시→이슈 흐름

**Status**: Skill 머지 완료, CLI 체인 검증 통과 / 실제 이슈 등록(end-to-end)은 TARS 콘솔에서 수동 검증 대기
**실현 위치**: `tars-skills` repo / [tars-skills#4](https://github.com/devlikebear/tars-skills/pull/4) 머지 (`000d5ae`)
**Depends on**: Track 2 Phase A (완료)
**Blocks**: Track 2 Phase C

---

## 단독 진입 시 컨텍스트 (5분)

> 이 페이즈만 보고 작업 시작 가능하도록 정리됨. 더 자세한 그림은 [README.md](./README.md), 진행 상태는 [HANDOFF.md](./HANDOFF.md).

**도그푸딩 본질**: TARS = 다중 프로젝트 자동화 호스트. 코어에 새 추상화 추가 X. 모든 도메인 기능은 plugin/skill.

**이 페이즈가 하는 일**:
1. `log-anomaly-detect` skill 작성 — log-watcher와 github-ops 도구를 호출하는 절차서
2. foo 운영용 worker 세션 1개 생성
3. cron 잡 1개 등록 (`*/30 * * * *` → 그 세션 깨워서 skill 실행)
4. memory_search 기반 dedup — 같은 anomaly 반복 등록 안 함
5. 이슈 템플릿 컨벤션 결정 (제목/본문/라벨)

**직전 페이즈 산출물 (Phase A)**:
- foo Docker로 운영 가능 (별도 repo, 의도적 버그 포함)
- `log-watcher` plugin: `docker_logs`, `file_tail` 도구
- `github-ops` plugin: `gh_issue_*`, `gh_pr_*`, `gh_worktree_*` 도구

**이 페이즈가 다음으로 넘기는 것**: Phase C는 등록된 이슈에 대해 `fix-and-pr` skill로 패치 시도 → draft PR. AutoResearch propose/test/verify 루프.

---

## 목표

- 30분마다 foo 세션이 자동 깨어나 로그 스캔
- LLM이 anomaly 분류 → 신규면 GitHub 이슈 등록 / 재발이면 기존 이슈에 코멘트만
- 이슈 템플릿이 fix 작업에 필요한 정보 포함 (로그 발췌, 발생 시각, 추정 컴포넌트)
- **PR 머지 기준은 "동작 검증"까지** — "2주 운영 후 정확도 검증"은 머지 후 추적 (별도 이슈)

## Out of Scope (Phase B)

- fix PR 자동 생성 — Phase C
- 코드 위치 추정 — Phase C 또는 별도
- bar 등록 — Phase D
- 정확도 ≥50% 보장 — 머지 후 관찰 단계
- 알림 다채널 — Telegram만 (필요 시)

## 작업 체크리스트

### 1. 이슈 템플릿 컨벤션 결정 + 문서화

- [ ] 제목 포맷 결정 — 예: `[auto] <component>: <short anomaly>` (앞에 `[auto]` 라벨 필수 → dedup 검색 키)
- [ ] 본문 섹션 결정 — 예:
  - "발생 시각" (UTC + KST)
  - "감지 패턴" (어느 로그 라인이 트리거)
  - "로그 발췌" (전후 10줄)
  - "추정 컴포넌트/파일" (로그에서 파일/함수 추출 시도)
  - "재발 여부" (memory_search 결과 + 기존 이슈 번호)
  - "신뢰도" (LLM의 self-reported confidence: high/medium/low)
- [ ] 라벨 컨벤션 결정 — `auto-detected`, `severity:critical|warn|info`, `component:*`
- [ ] 이 컨벤션을 skill 본문에 명시 (LLM이 따라 만들도록)

### 2. log-anomaly-detect skill 작성

위치: TARS의 skill 디렉토리 컨벤션 따름 (예: `skills/log-anomaly-detect/SKILL.md` 또는 `internal/skill/builtin/...`. Phase A에서 plugin 위치 결정한 패턴 참고).

- [ ] YAML frontmatter:
  - `name: log-anomaly-detect`
  - `description: Scan recent server logs for anomalies and file GitHub issues with deduplication`
  - `user_invocable: true` (수동 트리거도 가능)
  - 필요한 도구 명시 (`docker_logs`, `file_tail`, `gh_issue_search`, `gh_issue_create`, `gh_issue_comment`, `memory_search`, `memory_save`)
- [ ] skill 본문 (절차):
  1. 입력 파라미터 (예상): `target_container`, `repo`, `since` (default `30m`), `severity_threshold`
  2. **로그 수집**: `docker_logs(container=..., since=..., tail=500)`
  3. **anomaly 분류**: 받은 라인을 LLM이 분석 — error/panic/warn 패턴 추출, 정상 노이즈 제거
  4. **dedup 검사** (각 anomaly마다):
     - `memory_search(query="<anomaly signature>", scope="dogfooding/<repo>")` 로 과거 기록 확인
     - 추가로 `gh_issue_search(repo=..., query="[auto] <component>", state=all)` 로 이슈 트래커 확인
     - 매칭되는 기존 이슈 있으면 → "재발" 판단
  5. **신규 이슈 등록**:
     - `gh_issue_create(...)` 위 템플릿 따름
     - `memory_save` 로 anomaly signature + issue url 영속화 (다음 dedup 위함)
  6. **재발 코멘트**:
     - `gh_issue_comment(...)` — "동일 anomaly 재발: <시각> / 발생 횟수 N" 갱신
     - memory도 갱신
  7. **요약 응답**: 신규 N건, 재발 M건, 무시(노이즈) K건
- [ ] 안전장치:
  - 한 실행당 신규 이슈 생성 최대치 (예: 5건) — LLM 폭주 방지
  - dedup 실패 시 보수적으로 "코멘트만" — 중복 이슈 등록 회피
  - rate limit 감지 시 즉시 중단
- [ ] LLM tier 권장 — `standard` 또는 `light` (heavy는 비용 부담)

### 3. foo 운영용 worker 세션 생성

- [ ] 세션 생성 절차 결정 — TARS의 세션 생성 API/CLI 확인
  - `kind=worker` 사용 (사용자에게 노출 안 됨, cron이 깨우는 용도)
  - 세션 메타데이터: `name=monitor-foo`, `tags=[dogfooding, foo]`
- [ ] 세션 system prompt에 컨텍스트 주입 — "너는 foo 사이트 모니터링을 담당한다. 깨어나면 log-anomaly-detect skill을 호출해라"
- [ ] 세션 ID를 별도 메모/config에 기록 (cron 잡이 참조할 ID)
- [ ] 검증: 세션 목록 API에서 `monitor-foo` worker 보임

### 4. cron 잡 등록

- [ ] cron 잡 정의:
  - `name: foo-monitor`
  - `schedule: */30 * * * *`
  - `prompt: log-anomaly-detect skill을 호출해서 foo 컨테이너 로그를 스캔해`
  - `session_target: <foo worker session id>`
  - (Track 1에서 schedule 흡수 결정에 따라 cron 또는 schedule API 사용)
- [ ] enabled: false로 등록 → 수동 검증 후 enable
- [ ] 검증: cron list API에서 잡 보임

### 5. 통합 검증 (수동)

- [ ] foo의 의도적 버그 endpoint 1개 트리거 (`curl ...`) → 로그에 에러 발생
- [ ] cron 잡 수동 실행 (`tars cron run foo-monitor` 또는 콘솔 트리거)
- [ ] GitHub에서 `[auto]` 라벨 새 이슈 등록 확인 — 본문이 컨벤션 따르는지
- [ ] 같은 endpoint 다시 트리거 → cron 다시 실행 → 새 이슈 X, 기존 이슈에 "재발" 코멘트
- [ ] 다른 종류의 버그 endpoint 트리거 → cron 실행 → 별도 신규 이슈
- [ ] 한 번에 여러 anomaly 발생시킴 → 안전장치 (최대 5건) 동작 확인

### 6. 마무리

- [ ] cron 잡 enabled: true로 변경 (사용자 확인 후)
- [ ] CHANGELOG.md 갱신
- [ ] HANDOFF.md 갱신 (활성 페이즈를 Phase C로)
- [ ] PR description에 "머지 후 2주 관찰: 진짜 버그 비율 ≥50% 추적용 별도 이슈 생성" 명시
- [ ] 추적 이슈 생성 (TARS repo) — 제목: "Phase B 후속: anomaly detect 정확도 2주 관찰"

## Checkpoint: Phase B 완료 확인

**구현 확인:**
- [ ] `log-anomaly-detect.md` skill 존재 + frontmatter + 도구 명시 + 절차 명확
- [ ] foo worker 세션 + foo-monitor cron 잡 등록
- [ ] 이슈 템플릿 컨벤션 문서화

**실행 확인:**
- [ ] `make test` 통과
- [ ] cron 잡 1회 실행 → 정상 종료 (에러 로그 없음)
- [ ] dedup 안전장치 동작 (LLM 폭주 시 5건 cap)

**수동 확인:**
- [ ] 의도적 버그 트리거 → 새 이슈 자동 등록 (본문이 템플릿 따름)
- [ ] 같은 버그 재트리거 → 신규 이슈 X, 기존 이슈 코멘트 갱신
- [ ] 다른 종류 버그 → 별도 신규 이슈

**머지 후 추적 (PR과 분리):**
- [ ] 추적 이슈로 2주간 anomaly detect 정확도 관찰
- [ ] 정확도 ≥50% 미달 시 skill 프롬프트 튜닝 후속 이슈

**통과 후 Phase C로 진행.**

---

## 결정 기록

| 항목 | 결정 | 사유 | 결정일 |
|---|---|---|---|
| 이슈 제목 포맷 | `[auto] <component>: <one-line-summary>` | `[auto]` 접두어가 dedup용 제목 검색 키 | 2026-04-19 |
| 이슈 라벨 컨벤션 | `auto-detected`, `severity:{critical,warn,info}`, `component:<name>` | 트리아지/메트릭 집계 | 2026-04-19 |
| 이슈 본문 템플릿 | skill 리포의 `templates/issue_body.md` | skill과 같이 버전 관리, 변경 추적 | 2026-04-19 |
| 한 실행당 신규 이슈 cap | 5, 초과분은 "multi-anomaly 1건"으로 묶음 | LLM 폭주 방지 + 알림 소음 최소화 | 2026-04-19 |
| dedup 신호 | memory_search AND github-ops issue-search 모두 미매치일 때만 신규 / 하나라도 매치면 재발 | 중복 이슈 회피 우선 | 2026-04-19 |
| LLM tier (skill 실행) | standard 권장 (운영 중 비용 이슈 시 light 하락 허용) | heavy는 과잉, light는 stack trace 해석 약함 | 2026-04-19 |
| 아키텍처 | 새 CLI 없음, log-watcher + github-ops + memory_* 체이닝 | skill+CLI 피벗 원칙 일관 | 2026-04-19 |
| 명시적 중단 키워드 | `rate limit`, `401`, `403` 발견 시 전체 중단 | GitHub/OAuth 장애 조기 종료 | 2026-04-19 |
| cron 인터벌 | 사용자 console 세션 설정에 위임 (기본 `*/30` 권장) | 세션/cron은 TARS core 영역 | 2026-04-19 |
