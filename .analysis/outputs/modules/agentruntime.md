# 모듈: Agent Runtime

## 역할

`internal/agentruntime`은 비동기 run lifecycle, executor registry, workspace agent, channel message, persistence, run event stream을 관리한다.

## 주요 파일

- `internal/agentruntime/types.go`
- `internal/agentruntime/runtime.go`
- `internal/agentruntime/runtime_runs.go`
- `internal/agentruntime/runtime_run_execute.go`
- `internal/agentruntime/executor.go`
- `internal/agentruntime/events.go`

## 관찰

- canonical run route는 `/v1/agentruntime/runs`다.
- `/v1/agent/runs`는 호환 alias다.
- `Run`은 internal `WorkspaceID`와 public `run_id`, `session_id`, `agent`, `status` 등을 분리한다.
- workspace agent frontmatter와 tool policy가 runtime execution에 반영된다.
