# 용어 사전

## Agent Runtime

비동기 run lifecycle을 관리하는 runtime. Run은 `accepted`, `running`, `completed`, `failed`, `canceled` 상태를 가진다.

## Console

`/console` 경로의 Svelte SPA. 최신 TARS의 주 사용자-facing UI다.

## Legacy Alias

과거 public API 호환을 위해 남겨둔 route나 이름. 예: `/v1/agent/runs`.

## Memory

`MEMORY.md`, daily logs, experiences, semantic index를 포함하는 장기 기억 시스템. user-facing KB Wiki/graph surface와는 구분한다.

## Ops Approval

cleanup 같은 위험 작업을 plan/approval/apply 단계로 나누는 Human-in-the-loop 흐름.

## Pulse

1분 단위 watchdog. 시스템 신호를 스캔하고 LLM classifier를 통해 ignore/notify/autofix 결정을 만든다.

## Reflection

nightly batch surface. 현재는 deterministic experience derivation과 empty-session pruning을 수행한다.

## RegistryScope

tool registry의 scope. user/pulse/reflection 등 표면별로 등록 가능한 tool prefix를 제한한다.

## Skill Hub

TARS 외부 기능 배포 경로. domain-specific 기능은 core repo가 아니라 skill/plugin/mcp package로 제공한다.
