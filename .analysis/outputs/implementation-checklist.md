# 구현 체크리스트

## 저장소 이해 순서

- [ ] `cmd/tars/main.go`에서 엔트리포인트 구조와 `skill`/`plugin` 서브커맨드를 확인한다.
- [ ] `internal/config/load.go`, `config_input_fields.go`, `cmd/tars/doctor_main.go`에서 설정 병합과 스타터 진단 흐름을 확인한다.
- [ ] `internal/tarsserver/main.go`, `main_cli.go`, `main_bootstrap.go`에서 서버 부트스트랩 흐름을 확인한다.
- [ ] `internal/tarsserver/handler_chat_context.go`, `handler_chat_execution.go`, `internal/prompt/memory_retrieval.go`에서 채팅 실행과 relevant memory 조립을 확인한다.
- [ ] `internal/memory/semantic.go`, `internal/tool/memory_search.go`, `internal/tool/memory_save.go`에서 semantic memory 흐름을 확인한다.
- [ ] `internal/agent/loop.go`와 `internal/tool/tool.go`에서 tool-calling 중심부를 확인한다.
- [ ] `internal/extensions/manager.go`, `internal/skill/mirror.go`, `internal/skillhub/*`에서 확장 로딩과 허브 배포 경계를 확인한다.
- [ ] `internal/session/*`, `internal/memory/workspace.go`, `internal/project/store.go`에서 상태 저장 구조를 확인한다.
- [ ] `internal/project/workflow_policy.go`, `project_runner.go`, `internal/tarsserver/dashboard.go`에서 프로젝트 workflow와 운영 UI를 확인한다.
- [ ] `internal/serverauth/middleware.go`에서 auth mode, admin path, loopback skip 규칙을 확인한다.
- [ ] `internal/gateway/executor.go`, `runtime_run_bootstrap.go`, `runtime_channels.go`, `runtime_reports.go`에서 비동기 run lifecycle 과 채널/리포트 표면을 확인한다.
- [ ] `cmd/tars/service_main.go`, `internal/browserrelay/server.go`, `internal/auth/provider_credentials.go`, `internal/llm/openai_codex_client.go`에서 host service / relay / provider credential 경계를 확인한다.

## 로컬 검증 순서

- [ ] `make dev-serve`로 로컬 서버를 실행한다.
- [ ] `make dev-tars`로 TUI를 실행한다.
- [ ] 새 세션으로 메시지를 보내고 transcript 파일이 생성되는지 확인한다.
- [ ] semantic memory를 켰다면 `memory_save` 후 relevant memory나 `memory_search` hit가 나오는지 확인한다.
- [ ] skill 또는 plugin 변경 후 runtime mirror와 reload 경로가 필요한지 확인한다.
- [ ] `tars doctor`로 config/BYOK/gateway executor 준비 상태를 먼저 점검한다.
- [ ] OAuth 기반 provider를 쓰는 경우 env/file/keychain 중 어떤 token source 가 실제로 선택되는지 확인한다.
- [ ] provider 변경 시 `/chat/completions`, `/v1/messages`, `generateContent`, `responses` 중 어느 transport 를 타는지 확인한다.
- [ ] schedule 생성 후 cron job payload 에 `_tars_schedule` 메타데이터가 들어가는지 확인한다.
- [ ] gateway/browser 기능을 쓰는 경우 persistence 와 Node runner 전제가 있는지 확인한다.
- [ ] macOS 서비스나 브라우저 relay를 만지는 경우 launchctl 제약, loopback token, origin allowlist, query token 경고를 확인한다.
- [ ] 프로젝트 workflow를 쓰는 경우 `ACTIVITY.jsonl`, `KANBAN.md`, `AUTOPILOT.json`이 함께 갱신되는지 확인한다.

## 기능 추가 전 질문

- [ ] 이 변경이 클라이언트 표면인지, 서버 파이프라인인지, memory 계층인지, workspace 저장 구조인지 구분했다.
- [ ] semantic recall, project policy, tool allowlist, auth role 영향이 있는지 확인했다.
- [ ] `dashboard_auth_mode`나 `api_allow_insecure_local_auth` 같은 운영 노출 설정이 바뀌는지 확인했다.
- [ ] transcript, memory, usage tracking 중 어떤 영속 상태를 건드리는지 확인했다.
- [ ] project board/activity/state/dashboard 중 어떤 운영 문서를 건드리는지 확인했다.
- [ ] background manager나 gateway/browser runtime과 연결되는지 확인했다.
- [ ] 외부 채널 입력이나 host OS integration 이면 in-memory OTP / relay token / launchctl 제약을 고려했다.
- [ ] provider auth mode 가 `api-key`인지 `oauth`인지, refresh 가능한 credential인지 구분했다.
- [ ] Skill Hub나 plugin 설치를 건드린다면 registry 신뢰성, partial install, runtime mirror 경계까지 고려했다.
- [ ] approval 기반 삭제나 운영 작업이면 즉시 실행이 아니라 ops approval 흐름을 따라야 하는지 확인했다.

## 리팩터링 전 질문

- [ ] 이 패키지가 HTTP, LLM, memory index, workspace 파일 중 무엇에 가장 강하게 결합되어 있는지 확인했다.
- [ ] 테스트가 entrypoint 레벨인지 package 레벨인지 먼저 확인했다.
- [ ] 변경 후에도 local-first 파일 저장 규칙이 유지되는지 점검했다.
