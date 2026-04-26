# 모듈: System Surfaces

## 역할

Pulse, Reflection, Ops는 사용자 채팅과 분리된 system surface다. 이들은 LLM/tool registry를 사용자 surface와 섞지 않고, 좁은 Go interface를 통해 runtime 상태를 관찰하거나 운영 작업을 수행한다.

## 주요 파일

- `internal/pulse/*`
- `internal/reflection/*`
- `internal/ops/*`
- `internal/tarsserver/handler_pulse.go`
- `internal/tarsserver/handler_reflection.go`
- `internal/tarsserver/handler_ops.go`

## 관찰

- Pulse는 watchdog classifier이며 `pulse_decide` 한 종류의 tool call만 허용한다.
- Reflection은 nightly batch이며 현재 memory experience derivation과 legacy-named `kb_cleanup` empty-session pruning을 수행한다.
- Ops cleanup은 plan/approval/apply 패턴으로 Human-in-the-loop를 구현한다.
