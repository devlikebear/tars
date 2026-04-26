# 모듈: tarsserver

## 역할

`internal/tarsserver`는 TARS runtime의 조립 지점이다. config, LLM router, session store, tool registry, cron, pulse, reflection, agent runtime, ops, console static serving을 하나의 HTTP mux로 묶는다.

## 주요 파일

- `internal/tarsserver/main.go`
- `internal/tarsserver/main_serve_api.go`
- `internal/tarsserver/middleware.go`
- `internal/tarsserver/handler_chat*.go`
- `internal/tarsserver/handler_agentruntime*.go`
- `internal/tarsserver/handler_ops.go`
- `internal/tarsserver/notify.go`

## 관찰

- route registration은 `registerAPIRoutes`에 집중되어 있다.
- auth/admin path는 `serverauth` middleware와 `apiAdminPaths`로 적용된다.
- `/v1/memory/kb/*` residue와 `/v1/agent/*` legacy route는 각각 #410, #414에서 추적한다.
