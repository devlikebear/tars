# 아키텍처

## 1. 진입 레이어

루트 명령은 `cmd/tars/main.go` 한 파일에 모인다. 여기서 실제 비즈니스 로직을 거의 수행하지 않고, 환경 변수 로딩과 Cobra 명령 연결만 처리한다. 다만 현재 헤드에서는 기본 UX가 더 이상 TUI가 아니라 웹 콘솔이다.

- 콘솔 경로: `cmd/tars/console_main.go` -> 브라우저에서 `/console` 열기
- one-shot CLI 경로: `cmd/tars/client_main.go` -> `internal/tarsclient/client_main.go`
- 서버 경로: `cmd/tars/server_main.go` -> `internal/tarsserver/main.go`
- 보조 런타임 경로: `cmd/tars/assistant_main.go` -> `internal/assistant/runtime.go`
- 스타터 워크스페이스 경로: `cmd/tars/init_main.go`
- 진단 경로: `cmd/tars/doctor_main.go`
- 프로젝트/운영 경로: `cmd/tars/project_main.go`, `cmd/tars/cron_main.go`, `cmd/tars/approval_main.go`
- 서비스 운영 경로: `cmd/tars/service_main.go`
- 허브 관리 경로: `cmd/tars/skill_main.go`, `cmd/tars/plugin_main.go`, `cmd/tars/mcp_main.go`
- 메인 스레드 shim: `cmd/tars/mainthread_darwin.go`, `cmd/tars/mainthread_other.go`

핵심 포인트는 엔트리포인트가 계속 얇다는 점이다. 새 명령이 늘어났어도 실제 로직은 `internal/*`로 바로 내려간다. `main.go`가 `internal/browserplugin`을 blank import해 built-in browser plugin을 등록한다는 점도 현재 구조에서 중요하다.

## 2. 설정 및 스타터 환경 레이어

`internal/config/load.go`의 `Load`는 설정 병합의 유일한 공용 진입점이다. 순서는 `defaults < YAML < env`인데, 마지막에 `applyDefaults`를 한 번 더 돌려 invalid/empty 값을 정규화하고 workspace 기반 경로를 다시 계산한다.

- `internal/config/config_input_fields.go`: YAML key, env alias, normalize 함수, merge 규칙을 한 테이블로 모은다.
- `internal/config/defaults.go`: 안전한 기본값 집합을 제공한다.
- `internal/config/defaults_apply.go`: provider별 auth mode, base URL, model, workspace 파생 경로를 보정한다.
- `internal/config/yaml.go`: flat key 형태 YAML을 읽고 field metadata를 통해 적용한다.
- `internal/config/env.go`: 같은 field metadata를 이용해 환경 변수 override를 적용한다.
- `cmd/tars/doctor_main.go`: config file, workspace skeleton, bundled plugin manifest, gateway default agent, LLM credential, Claude CLI를 검증한다.

이제 "어떤 입력 키가 어떤 필드로 가는가"는 테이블에서 한 번에 볼 수 있다. 다만 merge semantics는 여전히 zero value를 "명시적 override"로 구분하지 않으므로, 이 레이어를 읽을 때는 apply 단계와 merge 단계를 같이 봐야 한다.

## 3. 서버 부트스트랩 레이어

`internal/tarsserver/main.go`의 `Serve`는 서버 실행의 단일 진입점이다. 실제 의존성 조립은 `internal/tarsserver/main_cli.go`, `main_bootstrap.go`, `main_serve_api.go`에 나뉜다.

- `main.go`: 초기 logger를 세우고 `newRootCmd`를 호출한다.
- `main_cli.go`: config를 읽은 뒤 실제 설정값으로 logger를 재구성하고 실행 모드를 고른다.
- `main_bootstrap.go`: workspace, session store, usage tracker, llm client, semantic memory-aware prompt runner를 만든다.
- `main_serve_api.go`: HTTP mux와 background runtime을 붙인다.

서버는 "로컬 상태 저장소 준비 -> LLM/provider 초기화 -> background runtime 연결 -> HTTP 노출" 순서를 따른다.

## 4. HTTP API 및 요청 보호 레이어

`internal/tarsserver/main_serve_api.go`가 API 서버와 백그라운드 런타임을 묶는다.

- 세션, 프로젝트, 사용량, cron, schedule, gateway, events, config, Hub, provider model 핸들러를 하나의 mux 아래 둔다.
- `/console` 정적 자산 또는 dev proxy는 `internal/tarsserver/console.go`가 맡는다.
- 예전 `/dashboards`와 `/ui/projects/*`는 `internal/tarsserver/legacy_dashboard.go`가 `/console`로 리다이렉트한다.
- built-in plugin이 제공하는 HTTP route는 `extensionsManager.CollectHTTPHandlers()`를 통해 mux에 추가된다.
- cron manager, watchdog, telegram poller, gateway runtime, autopilot manager 같은 background 작업도 여기서 시작된다.
- `internal/tarsserver/handler_transport_helpers.go`가 JSON body size 제한과 공통 method guard를 제공한다.

이 레이어의 진입 직후에는 인증 미들웨어가 붙는다.

- `internal/tarsserver/middleware.go`: API 공통 미들웨어를 조립한다.
- `internal/serverauth/middleware.go`: 인증 모드와 경로별 권한 요구사항을 계산한다.

`dashboard_auth_mode=off`는 이제 완전 공개가 아니라 "loopback 요청만 skip"으로 바뀌었다. 즉, 공개 모드의 위험을 줄이되 운영상 편의는 남겨 둔 형태다. 콘솔과 legacy dashboard redirect도 같은 인증 표면 안에서 보호된다.

## 5. 채팅 실행 레이어

채팅 파이프라인은 다섯 파일과 세 helper를 같이 봐야 한다.

- `internal/tarsserver/handler_chat.go`: HTTP 요청 수신, 세션 선택, skill 자동 선택, transcript 저장, compaction 트리거.
- `internal/tarsserver/handler_chat_context.go`: 세션/프로젝트/프롬프트/툴 주입 상태 조립.
- `internal/tarsserver/handler_chat_policy.go`: tool registry 구성, project tool policy 적용, auth role 기반 high-risk 필터를 담당한다.
- `internal/tarsserver/handler_chat_pipeline.go`: 실제 loop 실행 전후를 감싼다.
- `internal/tarsserver/handler_chat_execution.go`: `agent.Loop` 실행과 스트리밍 delta 전달.
- `internal/tarsserver/handler_chat_stream.go`: status/delta/done SSE 포맷을 책임진다.
- `internal/tarsserver/helpers_memory.go`: semantic memory 설정을 runtime 서비스로 바꾼다.
- `internal/prompt/memory_retrieval.go`: prompt 빌드 중 relevant memory를 붙인다.

실제 호출 흐름은 다음과 같다.

```text
User or TUI
  -> POST /v1/chat
  -> resolve session + project
  -> prompt.BuildResultFor + relevant memory retrieval
  -> buildChatToolRegistry + injected tool schemas
  -> agent.Loop.Run
     -> llm.Client.Chat
     -> tool.Registry.Execute
  -> transcript append + auto-compaction + semantic memory flush
  -> SSE done
```

추가로 이 레이어는 프로젝트 workflow 진입도 맡는다. kickoff 문장이나 활성 brief가 있으면 `handler_chat.go`가 `project-start` skill을 자동 선택한다. 첨부파일은 text/image/pdf block으로 변환돼 LLM 메시지에 주입된다.

## 6. Semantic Memory 레이어

이번 증분에서 가장 큰 새 축은 `internal/memory/semantic.go`다.

- `internal/memory/semantic.go`: 경험, compaction summary, project 문서를 embedding 기반 엔트리로 색인하고 검색한다.
- `internal/memory/gemini_embed.go`: Gemini embedding REST adapter다.
- `internal/tool/memory_search.go`: semantic index가 있으면 먼저 사용하고, 없으면 기존 텍스트 파일 검색으로 fallback 한다.
- `internal/tool/memory_save.go`: explicit experience를 파일과 semantic index에 함께 저장한다.
- `internal/tarsserver/helpers_chat.go`: compaction summary에서 durable memory 후보를 뽑아 semantic index에 flush 한다.

즉, memory는 이제 단순 Markdown 검색이 아니라 "파일 기반 truth source + embedding index" 2층 구조가 됐다.

## 7. LLM + Credential 레이어

에이전트 중심부는 다음 조합이다.

- `internal/agent/loop.go`: LLM 응답과 tool call을 반복 실행한다.
- `internal/tool/tool.go`: 런타임 툴 레지스트리와 schema 변환을 담당한다.
- `internal/tarsserver/helpers_build_tools.go`: cron, heartbeat, gateway, browser, web, apply_patch 같은 툴을 설정값에 맞춰 켠다.
- `internal/auth/provider_credentials.go`: provider별 auth mode와 refresh 전략을 registry처럼 관리한다.
- `internal/llm/provider.go`: provider client를 생성한다.
- `internal/llm/model_lister.go`: provider live API에서 model id를 조회한다.
- `internal/llm/transport.go`: JSON request/response 공통 transport helper다.

provider client의 차이도 중요하다.

- OpenAI-compatible 계열은 `/chat/completions` 와 공통 SSE parser를 공유한다.
- Anthropic은 system/tool/thinking 개념을 자체 wire format으로 다시 매핑한다.
- Gemini Native는 SDK가 아니라 REST `generateContent`와 `/models` 조회를 직접 구현한다.
- Codex는 OAuth refresh, account header, Responses API shape 때문에 별도 재시도 로직을 가진다.

## 8. 확장 런타임 레이어

`internal/extensions/manager.go`가 확장 통합 허브다.

- skill: `internal/skill/loader.go`
- runtime mirror: `internal/skill/mirror.go`
- plugin: `internal/plugin/loader.go`
- built-in plugin registry: `internal/plugin/builtin.go`, `internal/plugin/builtin_registry.go`
- MCP: `internal/mcp/client.go`

reload 후에는 하나의 `Snapshot`으로 skill 목록, plugin 목록, MCP 서버, 진단 메시지를 제공한다. MCP server build가 실패해도 서버 startup 전체를 막지 않고, diagnostic만 남긴 채 계속 진행할 수 있다.

현재 구조에서 새로 중요한 점은 built-in plugin도 일반 manifest plugin과 같은 스냅샷 표면으로 합쳐진다는 것이다. `manager.Start()`는 built-in plugin을 초기화하고, `runLifecycleHooks()`는 plugin manifest의 on_start/on_stop 훅을 best-effort로 실행하며, `CollectHTTPHandlers()`는 plugin이 제공하는 HTTP route를 서버 mux에 붙인다.

skill mirror가 계속 중요한 이유는 agent가 source path가 아니라 workspace 내부의 안정된 runtime path를 읽기 때문이다. companion file도 같이 복사되므로 skill이 참조하는 script/config를 runtime 경로에서 그대로 찾을 수 있다.

## 9. Skill Hub 배포 레이어

`cmd/tars/skill_main.go`, `cmd/tars/plugin_main.go`, `internal/skillhub/*`는 runtime 로딩 레이어와 별도로 "배포"를 담당한다.

- `internal/skillhub/registry.go`: raw GitHub registry를 읽고 search/info/download를 제공한다.
- `internal/skillhub/install.go`: workspace `skills/`, `plugins/` 디렉터리에 파일을 설치하고 `skillhub.json`에 설치 상태를 기록한다.
- skill은 optional `RequiresPlugin` metadata를 통해 companion plugin 의존성을 알려 줄 수 있다.

즉, extension runtime은 local snapshot을 읽고, Skill Hub는 그 snapshot의 입력 재료를 원격에서 가져오는 역할을 한다.

## 10. 프로젝트 운영 레이어

`internal/project/*`는 단순 메타데이터 저장소가 아니라 brief -> board -> planning -> dispatch/review -> progress -> dashboard 흐름 전체를 관리한다.

- `brief_state.go`: session 기반 brief와 프로젝트 `STATE.md`를 저장한다.
- `workflow_policy.go`: brief/project status normalize, 기본 next action, blocked/done state 전이 규칙을 모은다.
- `policy.go`: 프로젝트 tool allow/deny/pattern/risk 정책을 정규화하고 prompt context를 렌더한다.
- `orchestrator.go`: todo/review dispatch, verification gate, agent report 기록을 처리한다.
- `orchestrator_plan.go`: brief/state를 바탕으로 planner run을 만들고 backlog JSON을 파싱한다.
- `workflow_runtime_policy.go`: planning timeout, run retention 같은 runtime 규칙을 해석한다.
- `store_normalize.go`: 프로젝트 문서 입력값과 workflow profile/rule/sub-agent를 정규화한다.
- `internal/tarsserver/helpers_project_progress.go`: heartbeat 후크에서 autonomous project를 계획/진행시키는 상위 루프를 담당한다.
- `task_report.go`: worker output을 고정 포맷 `<task-report>`로 파싱한다.

핵심 포인트는 "정책이 조금 더 분리됐지만 아직 chat trigger, planning, dispatch, heartbeat 기반 progress, dashboard projection이 완전히 한 모델로 합쳐지지는 않았다"는 점이다.

## 11. 대시보드 레이어

`internal/tarsserver/dashboard.go`는 project workflow를 읽기 전용 운영 UI로 보여 준다. 다만 현재 사용자 기본 진입은 예전 HTML dashboard가 아니라 `/console`이다.

- `/console`: 현재 기본 웹 콘솔
- `/console/projects/{id}`: 프로젝트 상세 화면
- `/dashboards`, `/ui/projects/{id}`: legacy redirect entry
- `/ui/projects/{id}/stream`: legacy route이지만 실제로는 `/console/projects/{id}`로 redirect 된다.

이제 project 화면은 콘솔 프런트엔드와 서버 API 조합으로 보게 되고, 예전 dashboard URL은 호환성 redirect 역할에 가깝다.

## 12. 비동기 실행과 브라우저 레이어

`internal/gateway/*`와 `internal/browser/*`는 채팅 API와 별도로 오래 사는 실행 경로를 제공한다.

- `internal/gateway/executor.go`: prompt executor, command executor, tool allow/deny, session routing 정책을 정의한다.
- `internal/gateway/runtime_run_bootstrap.go`: run을 accepted 상태로 등록하고 worker session을 정한다.
- `internal/gateway/runtime_runs.go`: lifecycle 조회, wait, cancel을 담당한다.
- `internal/gateway/runtime_channels.go`: local/webhook/telegram 채널 메시지를 workspace-aware key로 저장한다.
- `internal/gateway/runtime_reports.go`: gateway summary, archived runs, channel reports를 만든다.
- `internal/browser/service.go`: 사이트 flow를 읽고 login/check/run API를 제공한다.
- `internal/browser/playwright_exec.go`: 실제 브라우저 제어는 Node + Playwright 스크립트로 위임한다.

즉, 비동기 런타임은 이제 "run lifecycle + channel surface + report surface"가 더 분리돼 있다. 브라우저 API route 자체는 built-in plugin HTTP handler를 통해 서버에 붙는다.

## 13. 호스트 서비스와 외부 경계

이 저장소는 HTTP API 바깥 운영 경계도 별도 adapter로 둔다.

- `cmd/tars/service_main.go`: 현재 `tars serve` 실행 파일을 macOS LaunchAgent plist로 감싸고 `launchctl`과 연결한다.
- `internal/browserrelay/server.go`: loopback + relay token + origin allowlist 조건을 만족한 브라우저 확장만 CDP relay에 붙인다.
- `internal/approval/otp.go`: Telegram 같은 외부 채널에서 들어오는 OTP 코드를 chat ID 단위로 대기/소비한다.
- `internal/auth/codex_oauth.go` + `codex_refresh_store.go`: OpenAI Codex credential file, refresh token, macOS keychain 저장을 관리한다.

browser relay는 query token 허용 모드가 남아 있지만, 이제 `/json/version`과 status payload에 명시적 warning을 함께 싣는다. Codex credential도 파일 저장만이 아니라 macOS keychain을 refresh token 저장소로 사용할 수 있다.
