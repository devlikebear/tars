# Dogfooding 로드맵 — TARS를 실제 운영 도구로 쓰기

**Status**: Planning
**Author**: jeidee
**Started**: 2026-04-19

## 목표

TARS를 **실제 다중 프로젝트 운영/창작/리서치 자동화 호스트**로 쓰면서 부족한 부분을 도그푸딩으로 메운다. 첫 시연은 가상 사이트 `tars-examples-foo`(Docker 운영)의 서버 로그를 주기적으로 감시 → 이슈 등록 → 픽스 PR 제출까지. 단, 이 시나리오는 **여러 use case 중 하나**이므로 TARS 코어가 foo 도메인에 종속되어선 안 된다. 추후 `tars-examples-bar` 등 다른 프로젝트(스택/도메인 다름)도 같은 인프라로 붙을 수 있어야 한다.

## 핵심 원칙 — "코어에 새 추상화를 추가하지 않는다"

과거 `internal/project/*` 시스템(#15 부터 #259까지 누적)이 코어를 비대화시키며 의도대로 동작하지 못해 #291 (`replace project system with session-scoped tasks`) ~ #347 (`remove project/TUI/dashboard leftovers`)에서 모두 철거된 경험이 있다. 같은 함정을 반복하지 않는다.

**원칙:**
1. **코어 추상화는 현재 뼈대 그대로** — 채팅 세션 + 게이트웨이 + cron + plugin + skill + memory + approval. 새 도메인 개념(예: ProjectRegistry, MonitoredTarget) 도입 금지.
2. **세션 = 자연스러운 격리 단위** — foo 운영용 세션 1개, bar 운영용 세션 1개. cron이 깨움.
3. **도메인 종속 기능은 plugin/skill로** — log-watcher / github-ops 등은 별도 plugin. 작업 절차는 skill(.md).
4. **오히려 슬림화 우선** — 새 기능 추가 전에 의심 모듈(중복/사용 빈도 낮음) 정리.
5. **검증 가능한 metric으로 묶음** — Karpathy AutoResearch 패턴(propose→act→verify)을 fix-and-pr skill 내부 절차로 흡수.
6. **지식 축적은 야간 batch** — Karpathy LLM-wiki 패턴을 `knowledge-compile` skill로. reflection 코드 변경 없음.

## 두 트랙

### Track 1 — Core Slim (선행, 작은 트랙)

목표: 의심 모듈 실태 조사 → 죽은/중복 코드 제거 → plugin/skill로 시나리오 올릴 깨끗한 토대 확보.

**의심 후보** (확정 아님 — Track 1에서 결정):
- `internal/research/` (2 files) — `tool_research.go`로 도구 노출. 실사용 빈도 확인.
- `internal/schedule/` (2 files) + `internal/scheduleexpr/` (2 files) — cron과 중복 의심. handler/도구 사용처 있음.

**제거 후보 아님** (조사 결과 핵심 추상화):
- `internal/agent/` (4 files, 5+ 사용처) — 채팅 실행 핵심.
- `internal/extensions/` (5 files, 15+ 사용처) — 깊이 박힌 인프라.

세부 계획: [track1-core-slim.md](./track1-core-slim.md)

### Track 2 — Monitored Ops (시나리오 구현, 4 페이즈)

목표: foo 가상 사이트 운영 자동화. 코어 변경 거의 0, 모든 기능을 plugin/skill로 구현.

| Phase | 산출물 | 주요 검증 |
|---|---|---|
| **A. 인프라** | foo Docker 데모 + `log-watcher` plugin + `github-ops` plugin | 두 plugin 단위 테스트, foo 컨테이너 실행 |
| **B. 감시→이슈** | `log-anomaly-detect` skill + foo 세션 + cron 잡 + memory_search dedup | 2주 운영 후 진짜 버그 비율 ≥50% |
| **C. fix→PR** | `fix-and-pr` skill (AutoResearch propose/test/verify 루프) + worktree 격리 + approval | draft PR 머지율 추적 |
| **D. 검증 + 지식** | `tars-examples-bar` 등록(코어 변경 0 검증) + (선택) `knowledge-compile` skill | bar 등록 시 코어 코드 변경 0 라인 |

세부 계획: [track2-monitored-ops-roadmap.md](./track2-monitored-ops-roadmap.md)

## 진행 순서

1. **Track 1 완료** → 코어 슬림화 마무리 + 머지.
2. **Track 2 Phase A부터 순서대로** → 각 phase 완료 후 사용자 검증 게이트.
3. Phase D 통과 = 도그푸딩 v1 마일스톤. 이후 다른 use case (리서치/창작) 검토.

## 핸드오프 시스템

이 도그푸딩은 여러 PR/세션에 걸친다. 컨텍스트 손실 없이 이어가도록 다음 규칙을 따른다.

### 새 세션이 들어오면

1. **[HANDOFF.md](./HANDOFF.md)를 가장 먼저 읽는다** — 현재 어디까지 했고 다음 액션이 뭔지 한 곳에 정리되어 있다.
2. HANDOFF의 "활성 페이즈"가 가리키는 페이즈 계획서를 읽는다 (각 페이즈 계획서는 self-contained — 단독으로도 진입 가능).
3. HANDOFF의 "다음 액션" 첫 항목부터 시작.

### 갱신 규칙 (작업하는 세션의 책임)

매번 갱신:
- 의사결정이 발생할 때 → HANDOFF "이력" + 해당 페이즈 계획서 "결정 기록"
- 단계가 완료될 때 → HANDOFF "현재 상태" + "다음 액션"
- 블로커가 생길 때 → HANDOFF "블로커"
- 세션을 종료할 때(또는 PR 생성 시) → 위 모든 항목 최신 상태로

### PR 단위

- Track 1 = 1 PR (`chore: slim core for dogfooding`)
- Track 2 각 페이즈 = 1 PR (`feat(dogfooding): phase A — infra plugins` 등)
- 각 PR description에 HANDOFF.md 링크 + "이 PR 머지 후 다음 PR 예고" 필수.

### 결정은 메모리에 두지 않는다

세션 메모리 시스템(`/Users/.claude/projects/.../memory/`)이 아니라 plan 문서 안에 기록한다. 메모리는 휘발 + 다른 컨텍스트와 섞일 수 있어 핸드오프에 부적합.

## Out of Scope (이번 도그푸딩 v1)

- ProjectRegistry / ProjectContext / MonitoredTarget 같은 코어 추상화 신규 도입.
- 다중 platform gateway (Slack/Discord/Email) — Telegram만 유지.
- pulse/reflection 자체 변경 — 둘 다 TARS 내부 시스템 surface로 유지.
- 자동 머지 — PR 리뷰/머지는 사람이 직접.
- TARS 자기 코드 자동 수정 (자가 개선 루프) — 이 도그푸딩의 대상이 아님.

## 참고 자료

- 과거 `project` 시스템 폐기 흐름: 커밋 #291, #300, #301, #347, #348
- Karpathy AutoResearch: <https://github.com/karpathy/autoresearch>
- Karpathy LLM-wiki: <https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f>
- Hermes Agent (참고만): 로컬 체크아웃 (`opensources/hermes-agent`)
- TARS CLAUDE.md (System Surface 원칙)
