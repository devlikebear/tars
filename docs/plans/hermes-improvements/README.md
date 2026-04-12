# Hermes-inspired Improvements

**Milestones**: `v0.25.0` (S1) · `v0.26.0` (S2) · `v0.27.0` (S3)
**Status**: Planning

## 배경

Hermes Agent를 실사용하면서 TARS에 접목할 가치가 있는 패턴 몇 가지가 드러났다. 이 디렉터리는 TARS의 정체성을 훼손하지 않는 선에서 hermes 스타일 장점을 흡수하기 위한 설계 문서 묶음이다.

## TARS 정체성 (침범 금지선)

모든 개선안은 아래 다섯 가지를 **훼손하지 않는다**는 전제에서 설계된다. 설계 문서마다 말미에 "Identity Check" 절을 두고 각 항목이 유지되는지 명시한다.

1. **단일 Go 바이너리** — Python/Node 런타임을 런타임 의존성으로 추가하지 않는다.
2. **File-first, DB-less** — 세션, 메모리, KB, 크론 이력 등 영속 상태는 파일로 투명하게 유지한다.
3. **User surface ↔ System surface 컴파일 타임 분리** — `RegistryScope`가 `ops_/pulse_/reflection_` 접두어를 wiring 시점에 panic으로 차단하는 보증을 깨지 않는다.
4. **정책은 config, 메커니즘은 Go** — LLM은 분류·요약·합성까지만 담당하고, 사용자 영향이 있는 액션은 Go가 결정론적으로 실행한다.
5. **Durable semantic memory + 야간 배치 컴파일** — per-turn LLM 부담을 늘리지 않는다. 새 기능은 opt-in이거나 reflection 잡으로 이전한다.

## 개선 항목과 Sprint 계획

| #  | 제목                                  | Sprint | Milestone | Area                     | 난이도 | 의존성 |
|----|---------------------------------------|--------|-----------|--------------------------|--------|--------|
| 01 | Toolset groups 정책 표면 정리          | S1     | v0.25.0   | area/tool                | Low    | -      |
| 02 | Context compression knobs 노출        | S1     | v0.25.0   | area/config              | Low    | -      |
| 03 | Per-task provider/credential override | S2     | v0.26.0   | area/gateway, area/llm   | Medium | -      |
| 04 | MoA consensus (PR-A + PR-B)           | S2     | v0.26.0   | area/gateway             | High   | #03    |
| 05 | Memory backend interface 추출         | S3     | v0.27.0   | area/memory              | Medium | -      |

> **선행 전제**: #03과 #04는 모두 PR #323(`refactor(llm): named provider pool + tier binding schema`)이 main에 머지된 이후에만 설계가 성립한다. Alias 기반 `cfg.LLMProviders`가 전제이므로, 이 refactor 커밋 전 브랜치에서 분기하지 말 것.

> **#04는 두 PR로 분할됨**: gateway run event surface + 최소 run view(PR-A)가 consensus executor(PR-B)보다 먼저 머지된다. PR-A는 consensus 없이도 가치가 있는 인프라(gateway 전용 SSE, 실행 세마포어, run detail 화면)이고, PR-B는 그 위에 consensus 레이어만 얹는다.

### Sprint 1 — 자체 완성도 (`v0.25.0`)

- [01 Toolset groups](01-toolset-groups.md)
- [02 Context compression knobs](02-context-compression-knobs.md)

hermes 모방이라기보다 TARS 자체의 예측 가능성·정책 표면 완성도를 끌어올리는 두 작업. PR 크기가 작고 리스크가 낮아 가장 먼저 흡수한다. **v0.25.0 릴리스의 scope**.

### Sprint 2 — 핵심 차별화 (opt-in, `v0.26.0`)

- [03 Provider override](03-provider-override.md)
- [04 MoA consensus](04-moa-consensus.md) — **PR-A(event surface + run view) → PR-B(consensus executor)** 순서

MoA의 전제조건이 per-task alias override이므로 03을 먼저 머지하고 04를 그 위에 얹는다. 04는 내부에서 다시 두 PR로 쪼개진다 — PR-A가 먼저 gateway 전용 SSE endpoint와 최소 run view를 만들고, PR-B가 그 위에 consensus executor를 올린다. 두 기능 모두 opt-in으로 설계되며, 기본 동작은 변하지 않는다. 비용·예산 이슈가 크기 때문에 S1과 **같은 릴리스에 묶지 않는다** — v0.26.0으로 분리.

### Sprint 3 — 미래 준비 (`v0.27.0`)

- [05 Memory backend interface](05-memory-backend-interface.md)

외부 메모리 어댑터(Mem0, Zep, Honcho 등)를 당장 붙이지는 않지만, 나중에 붙일 수 있도록 Go interface만 추출한다. 파일 기반 구현은 `FileBackend`로 wrap되며 기본값은 그대로 유지된다. Interface는 "지금 실제로 쓰이는 primitive"만 노출하고, `KBCleanupJob`은 session 도메인이므로 backend와 분리되어 유지된다.

## PR 슬라이스 (머지 순서)

각 슬라이스는 독립 리뷰 가능한 최소 단위로 설계됐다. 아래 순서로 쌓으면 뒤 PR이 앞 PR의 인프라를 그대로 재사용한다.

1. **`feat(tool): unify group-based tool policy across agent and session`** — #01
2. **`feat(chat): expose compaction controls and deterministic fallback`** — #02
3. **`feat(gateway): add alias override plumbing and run metadata`** — #03
4. **`feat(gateway): add gateway run event surface and console view`** — #04 PR-A (consensus 없이도 값이 있다)
5. **`feat(gateway): add consensus executor mode`** — #04 PR-B (PR-A 위에 올라간다)
6. **`refactor(memory): extract file-backed backend interface`** — #05

## Rollback Strategy

각 PR은 독립적으로 되돌릴 수 있어야 한다. 구체적으로:

- **01**: 새 config 필드 zero-value = 기존 동작. `SessionToolConfig`의 신규 슬라이스는 `omitempty`라 `sessions.json` round-trip에 영향 없음. Revert 시 기존 세션 파일 그대로 로드됨.
- **02**: 신규 config 3개(`compaction_trigger_tokens`, `compaction_keep_recent_tokens`, `compaction_keep_recent_fraction`)가 현재 하드코드(`100000`, `12000`, `0.30`)와 수치 동등. `compaction_llm_mode=auto`(기본)는 현재 LLM 폴백 동작 그대로. `deterministic` 모드만 새로 추가되는 선택지.
- **03**: `gateway_task_override.enabled=false`(기본)이면 필드를 넣어도 loud error. 기능 비활성화 상태면 기존 tier 라우팅 그대로. `Run` 스냅샷 필드는 omitempty라 backward compatible.
- **04 PR-A**: `/v1/gateway/runs/{id}/events` endpoint는 신규이므로 기존 경로에 영향 없음. Runtime 세마포어는 기본 크기를 현재 `gateway_subagents_max_threads`에 맞춰 기동하므로 관찰 가능한 throttling 동작 변화 없음. Revert는 endpoint + 세마포어 코드 삭제로 안전.
- **04 PR-B**: `mode=consensus`를 지정하지 않으면 기존 `subagents_run` 동작 그대로. `gateway_consensus_enabled=false`(기본)면 코드 경로 자체가 dormant. Variant는 별도 `consensusPool`에서 실행되므로 기존 subagent pool은 영향받지 않음.
- **05**: `FileBackend`가 기본값이며, `memory_backend` config가 없으면 기존 동작과 바이너리 동등. Interface 도입은 refactor이므로 관찰 가능한 동작 변화 없음. `KBCleanupJob`은 이 PR에서 건드리지 않아 session cleanup 동작 무변경.

## 이슈 트래킹

- GitHub milestones: `v0.25.0`, `v0.26.0`, `v0.27.0` (Sprint별 1개)
- 메타 트래커 이슈: "Hermes-inspired improvements tracker" — 세 마일스톤 전체를 묶는 추적용
- 이슈는 문서 번호 단위로 생성한다 (01/02/03/04/05). `#04`는 하나의 이슈를 PR-A/PR-B 두 개의 체크박스로 분리해 추적한다.
- 라벨: `enhancement` + `area/<pkg>`
- 생성 스크립트: [`scripts/create-hermes-improvement-issues.sh`](../../../scripts/create-hermes-improvement-issues.sh) — 기존 이슈 존재 시 skip 로직 유지

## 진행 방식

- 각 항목은 `feat/hermes-<short-name>` 브랜치 + worktree에서 작업
- PR 본문 상단에 해당 설계 문서 경로 링크 필수
- Acceptance criteria 체크박스가 전부 채워질 때까지 merge 금지
- 각 PR은 Identity Check 절을 리뷰어가 명시적으로 확인
