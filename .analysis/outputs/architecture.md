# 아키텍처

## 상위 레이어

```
cmd/tars
  -> internal/tarsserver
      -> config / llm / prompt / session / tool
      -> agentruntime / cron / pulse / reflection / ops
      -> frontend/console embedded assets
```

## 요청 흐름: Chat

1. `cmd/tars/server_main.go`가 `tarsserver.Serve`에 옵션을 넘긴다.
2. `internal/tarsserver/main_serve_api.go`가 runtime dependencies와 HTTP mux를 만든다.
3. `POST /v1/chat`은 session을 resolve하고 transcript/history/prior context를 모은다.
4. `prompt`가 system prompt와 memory context를 조립한다.
5. `agent.Loop`가 LLM 호출과 tool call을 반복한다.
6. delta/status/done payload가 SSE로 전송된다.
7. assistant 응답은 transcript에 저장되고, daily log/explicit remember hook이 실행된다.

## 요청 흐름: Agent Runtime

1. `POST /v1/agentruntime/runs`가 run을 accepted 상태로 만든다.
2. runtime은 executor를 선택한다.
3. run goroutine이 `running -> completed|failed|canceled` 상태를 기록한다.
4. run/channel state는 workspace별 persistence path에 저장된다.
5. `/v1/agentruntime/status|reload|restart|reports/*`가 운영 상태를 노출한다.

`/v1/agent/runs`는 legacy alias이며, agent list는 현재 `/v1/agent/agents`에 남아 있다.

## System Surface

TARS는 user surface와 system surface를 분리한다.

- User surface: chat, agent runtime, console.
- Pulse surface: cron failure, stuck run, disk pressure, Telegram delivery, reflection health를 스캔한다.
- Reflection surface: nightly memory experience derivation과 legacy-named `kb_cleanup` empty-session pruning을 실행한다.
- Ops surface: cleanup plan과 approval workflow를 제공한다.

`tool.Registry`는 scope별 금지 prefix를 통해 user registry에 system tool이 섞이는 것을 막는다.

## Extension Boundary

도메인 기능은 core Go tool이 아니라 Skill Hub의 skill + companion CLI로 분리하는 것이 기본 패턴이다. Builtin plugin HTTP surface와 `internal/browserplugin`은 제거됐다.
