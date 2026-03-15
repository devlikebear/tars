# 구현 체크리스트

## 저장소 이해 순서

- [ ] `cmd/tars/main.go`에서 엔트리포인트 구조를 확인한다.
- [ ] `internal/config/load.go`와 `cmd/tars/doctor_main.go`에서 설정 병합과 스타터 진단 흐름을 확인한다.
- [ ] `internal/tarsserver/main.go`와 `internal/tarsserver/main_bootstrap.go`에서 서버 부트스트랩 흐름을 확인한다.
- [ ] `internal/tarsserver/handler_chat_context.go`와 `internal/tarsserver/handler_chat_execution.go`에서 채팅 실행 흐름을 확인한다.
- [ ] `internal/agent/loop.go`와 `internal/tool/tool.go`에서 tool-calling 중심부를 확인한다.
- [ ] `internal/session/*`와 `internal/memory/workspace.go`에서 상태 저장 구조를 확인한다.
- [ ] `internal/project/project_runner.go`와 `internal/tarsserver/dashboard.go`에서 프로젝트 workflow와 운영 UI를 확인한다.
- [ ] `internal/serverauth/middleware.go`에서 auth mode 와 admin path 규칙을 확인한다.
- [ ] `internal/gateway/runtime_run_bootstrap.go`와 `internal/gateway/runtime_run_execute.go`에서 비동기 run lifecycle 을 확인한다.
- [ ] `cmd/tars/service_main.go`와 `internal/browserrelay/server.go`에서 host service / relay 경계를 확인한다.
- [ ] `internal/auth/token.go`와 `internal/llm/openai_codex_client.go`에서 auth mode 별 token source 와 refresh 경계를 확인한다.
- [ ] `internal/llm/openai_compat_client.go`, `internal/llm/anthropic.go`, `internal/llm/gemini_native.go`에서 provider별 transport 차이를 확인한다.

## 로컬 검증 순서

- [ ] `make dev-serve`로 로컬 서버를 실행한다.
- [ ] `make dev-tars`로 TUI를 실행한다.
- [ ] 새 세션으로 메시지를 보내고 transcript 파일이 생성되는지 확인한다.
- [ ] skill 또는 plugin 변경 후 reload 경로가 필요한지 확인한다.
- [ ] `tars doctor`로 config/BYOK/gateway executor 준비 상태를 먼저 점검한다.
- [ ] OAuth 기반 provider를 쓰는 경우 실제 env/file token source 와 refresh 가능 여부를 확인한다.
- [ ] provider 변경 시 `/chat/completions`, `/v1/messages`, `generateContent`, `responses` 중 어느 transport 를 타는지 확인한다.
- [ ] schedule 생성 후 cron job payload 에 `_tars_schedule` 메타데이터가 들어가는지 확인한다.
- [ ] gateway/browser 기능을 쓰는 경우 persistence 와 Node runner 전제가 있는지 확인한다.
- [ ] macOS 서비스나 브라우저 relay를 만지는 경우 launchctl 제약, loopback token, origin allowlist 전제를 확인한다.
- [ ] 프로젝트 workflow를 쓰는 경우 `ACTIVITY.jsonl`, `KANBAN.md`, `AUTOPILOT.json`이 함께 갱신되는지 확인한다.

## 기능 추가 전 질문

- [ ] 이 변경이 클라이언트 표면인지, 서버 파이프라인인지, workspace 저장 구조인지 구분했다.
- [ ] 프로젝트 정책, tool allowlist, auth role 영향이 있는지 확인했다.
- [ ] `dashboard_auth_mode`나 `api_allow_insecure_local_auth` 같은 운영 노출 설정이 바뀌는지 확인했다.
- [ ] transcript, memory, usage tracking 중 어떤 영속 상태를 건드리는지 확인했다.
- [ ] project board/activity/state/dashboard 중 어떤 운영 문서를 건드리는지 확인했다.
- [ ] background manager나 gateway/browser runtime과 연결되는지 확인했다.
- [ ] 외부 채널 입력이나 host OS integration 이면 in-memory OTP / relay token / launchctl 제약을 고려했다.
- [ ] provider auth mode 가 `api-key`인지 `oauth`인지, refresh 가능한 credential인지 구분했다.
- [ ] approval 기반 삭제나 운영 작업이면 즉시 실행이 아니라 ops approval 흐름을 따라야 하는지 확인했다.

## 리팩터링 전 질문

- [ ] 이 패키지가 HTTP, LLM, workspace 파일 중 무엇에 가장 강하게 결합되어 있는지 확인했다.
- [ ] 테스트가 entrypoint 레벨인지 package 레벨인지 먼저 확인했다.
- [ ] 변경 후에도 local-first 파일 저장 규칙이 유지되는지 점검했다.
