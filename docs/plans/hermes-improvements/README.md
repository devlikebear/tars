# Hermes-inspired Improvements

**Milestone**: `v0.25.0-hermes`
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

| #  | 제목                                 | Sprint | Area            | 난이도 | 의존성 |
|----|--------------------------------------|--------|-----------------|--------|--------|
| 01 | Toolset groups 정책 표면 정리        | S1     | area/tool       | Low    | -      |
| 02 | Context compression knobs 노출       | S1     | area/config     | Low    | -      |
| 03 | Per-task provider/credential override | S2     | area/gateway, area/llm | Medium | -      |
| 04 | MoA consensus executor mode          | S2     | area/gateway    | High   | #03    |
| 05 | Memory backend interface 추출        | S3     | area/memory     | Medium | -      |

### Sprint 1 — 자체 완성도 (정체성 안전, 즉시 이득)

- [01 Toolset groups](01-toolset-groups.md)
- [02 Context compression knobs](02-context-compression-knobs.md)

hermes 모방이라기보다 TARS 자체의 예측 가능성·정책 표면 완성도를 끌어올리는 두 작업. PR 크기가 작고 리스크가 낮아 가장 먼저 흡수한다.

### Sprint 2 — 핵심 차별화 (opt-in)

- [03 Provider override](03-provider-override.md)
- [04 MoA consensus](04-moa-consensus.md)

MoA의 전제조건이 per-task provider override이므로 03을 먼저 머지하고 04를 그 위에 얹는다. 두 기능 모두 opt-in으로 설계되며, 기본 동작은 변하지 않는다.

### Sprint 3 — 미래 준비

- [05 Memory backend interface](05-memory-backend-interface.md)

외부 메모리 어댑터(Mem0, Zep, Honcho 등)를 당장 붙이지는 않지만, 나중에 붙일 수 있도록 Go interface만 추출한다. 파일 기반 구현은 `FileBackend`로 wrap되며 기본값은 그대로 유지된다.

## Rollback Strategy

각 PR은 독립적으로 되돌릴 수 있어야 한다. 구체적으로:

- **01, 02**: 새 config 필드는 전부 zero-value가 기존 동작과 동일. 필드를 무시하면 롤백과 동일.
- **03**: `provider_override` 필드는 optional. 사용하지 않으면 기존 tier 라우팅 그대로.
- **04**: `mode=consensus`를 지정하지 않으면 기존 `subagents_run` 동작 그대로. `gateway_consensus_enabled=false`(기본)면 코드 경로 자체가 dormant.
- **05**: `FileBackend`가 기본값이며, `memory_backend` config가 없으면 기존 동작과 바이너리 동등.

## 이슈 트래킹

- GitHub milestone: `v0.25.0-hermes`
- 메타 트래커 이슈: "Hermes-inspired improvements tracker"
- 각 항목당 이슈 1개 (총 5 + 메타 1 = 6개)
- 라벨: `enhancement` + `area/<pkg>`
- 생성 스크립트: [`scripts/create-hermes-improvement-issues.sh`](../../../scripts/create-hermes-improvement-issues.sh)

## 진행 방식

- 각 항목은 `feat/hermes-<short-name>` 브랜치 + worktree에서 작업
- PR 본문 상단에 해당 설계 문서 경로 링크 필수
- Acceptance criteria 체크박스가 전부 채워질 때까지 merge 금지
- 각 PR은 Identity Check 절을 리뷰어가 명시적으로 확인
