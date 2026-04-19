# Dogfooding 작업 핸드오프

**Last Updated**: 2026-04-19
**Updated By**: planning session (worktree: `unruffled-moore-2fc3c3`)

> 이 파일은 매 의사결정/단계 완료/블로커 발생/세션 종료 시 갱신된다. 새 세션은 이 파일부터 읽는다.

---

## 새 세션 진입 가이드 (5분)

1. 아래 **컨텍스트 (한 단락)**을 읽는다.
2. **현재 상태** 섹션에서 활성 트랙/페이즈 확인.
3. 활성 페이즈의 계획서를 읽는다 (각 페이즈 계획서는 self-contained).
4. **미해결 결정**이 있으면 사용자에게 먼저 확인.
5. **다음 액션** 첫 항목부터 시작.

작업 끝(세션 종료 직전 또는 PR 생성 시)에 이 파일의 모든 섹션을 갱신한다.

## 컨텍스트 (한 단락)

TARS 도그푸딩 v1 — TARS를 다중 프로젝트 운영/창작/리서치 자동화 호스트로 쓴다. 첫 시연: 가상 사이트 `tars-examples-foo`(Docker 운영, 의도적 버그 시드)의 서버 로그를 30분마다 감시 → GitHub 이슈 등록 → fix-PR 제출 (리뷰/머지는 사람). 핵심 원칙: **TARS 코어에 새 추상화를 추가하지 않는다** (과거 `internal/project/*` 시스템이 비대화 후 #291~#347에서 전부 철거된 경험). 모든 도메인 기능은 plugin/skill로. Track 1(코어 슬림화) → Track 2 4페이즈(시나리오 구현) 순. 자세한 그림은 [README.md](./README.md).

## 현재 상태

- **활성 트랙**: Track 1 — Core Slim (PR 제출 단계)
- **활성 페이즈**: (Track 2 미시작)
- **활성 단계**: `make test`/`make vet` 통과, VERSION/CHANGELOG 갱신, PR 생성
- **활성 worktree**: `.claude/worktrees/unruffled-moore-2fc3c3` (`claude/unruffled-moore-2fc3c3` branch)
- **블로커**: `internal/auth/TestResolveToken_OAuthClaudeFromFile` 1건 실패 — main 기준 사전 존재 (환경의 `CLAUDE_CODE_OAUTH_TOKEN` env 누수), 본 PR과 무관
- **마지막 빌드**: `go build ./...` + `go vet ./...` 통과 (2026-04-19)

## 미해결 결정 (사용자 확인 대기)

| 결정 사항 | 선택지 | 비고 |
|---|---|---|
| `tars-examples-foo` repo 위치 | (a) `devlikebear/tars-examples-foo` GitHub, (b) 다른 위치 | Phase A 시작 전 결정 필요 |
| `tars-examples-bar` 스택 | (a) Node, (b) Python, (c) 기타 | Phase D 시작 전 결정. foo가 Go라 가정. |

(Track 1 진행 중 발생할 결정은 [track1-core-slim.md](./track1-core-slim.md) "결정 기록"에 기록)

## 다음 액션 (구체적)

✅ **완료**: research 제거, schedule 제거 (코드 + 핸들러 + 라우트 + CLI + 타입 모두), `make test` / `make vet` 통과, VERSION 0.28.0 범프, CHANGELOG Removed 섹션 작성, PR 생성.

다음 세션 시작점: **Track 2 Phase A** (`track2-phase-a-infra.md`). Phase A 시작 전에 HANDOFF "미해결 결정"의 `tars-examples-foo` repo 위치를 사용자에게 확정 받을 것.

## 진행 이력 (역시간순)

| 날짜 | 트랙/페이즈 | 작업 | 결과 | PR |
|---|---|---|---|---|
| 2026-04-19 | Track 1 / release | `make test`(pre-existing auth 실패 1건만) + `make vet` 통과, VERSION 0.28.0, CHANGELOG Removed/Migration 작성, PR 생성 | Track 1 종결 | (이 PR) |
| 2026-04-19 | Track 1 / removal | research + schedule 코드 전부 제거, `go build ./...` 통과. 다음 세션은 test/vet/version/CHANGELOG/PR만 처리하면 됨. | 빌드 클린 | (이 PR에 포함) |
| 2026-04-19 | Track 1 / audit | research/schedule/scheduleexpr 3개 모듈 audit | research = 제거, schedule = 제거(콘솔 미사용, CLI도 동시 제거), scheduleexpr = NormalizeExpression+ResolveSchedule+ParseNaturalSchedule 모두 cron이 의존하므로 유지. 상세 [track1-core-slim.md](./track1-core-slim.md) "Audit 상세 결과" | (이 PR에 포함) |
| 2026-04-19 | Planning | 도그푸딩 계획 문서 + 핸드오프 시스템 작성 | README/Track1/Track2-roadmap/HANDOFF/Phase A-D 작성 완료 | (이 PR에 포함) |

## PR 매핑 계획

| Track / Phase | PR 제목 (예정) | 상태 |
|---|---|---|
| Planning docs | `docs(dogfooding): planning docs and handoff` | Track 1 PR에 병합 |
| Track 1 | `chore(dogfooding): slim core (remove research + schedule)` | 진행 중 (이 worktree) |
| Track 2 Phase A | `feat(dogfooding): phase A — log-watcher + github-ops plugins` | 미시작 |
| Track 2 Phase B | `feat(dogfooding): phase B — log anomaly detect skill + cron` | 미시작 |
| Track 2 Phase C | `feat(dogfooding): phase C — fix-and-pr skill (AutoResearch loop)` | 미시작 |
| Track 2 Phase D | `feat(dogfooding): phase D — bar validation gate (+ knowledge)` | 미시작 |

## 컨텍스트 보존 체크리스트 (PR 작성 전)

- [ ] HANDOFF.md "현재 상태" 갱신
- [ ] HANDOFF.md "이력" 행 추가
- [ ] 진행 중 페이즈 계획서의 "결정 기록" 갱신
- [ ] 새 미해결 결정이 있으면 HANDOFF "미해결 결정"에 추가
- [ ] PR description에 HANDOFF 링크 + "다음 PR 예고" 포함
- [ ] CHANGELOG.md 갱신 (사용자 영향 있을 때)
- [ ] VERSION.txt 범프 (Track 단위 또는 큰 페이즈 완료 시)
