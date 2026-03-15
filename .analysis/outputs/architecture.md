# 아키텍처

## 1. 진입 레이어

루트 명령은 `cmd/tars/main.go` 한 파일에 모인다. 여기서 실제 비즈니스 로직을 거의 수행하지 않고, 환경 변수 로딩과 Cobra 명령 연결만 처리한다.

- 클라이언트 경로: `cmd/tars/client_main.go` -> `internal/tarsclient/client_main.go`
- 서버 경로: `cmd/tars/server_main.go` -> `internal/tarsserver/main.go`
- 보조 런타임 경로: `cmd/tars/assistant_main.go` -> `internal/assistant/runtime.go`
- 스타터 워크스페이스 경로: `cmd/tars/init_main.go` -> starter config/workspace 생성
- 진단 경로: `cmd/tars/doctor_main.go` -> starter 상태, BYOK, gateway executor 준비 점검
- 서비스 운영 경로: `cmd/tars/service_main.go` -> macOS LaunchAgent plist 설치/기동/중지/상태 확인

## 2. 설정 및 스타터 환경 레이어

`internal/config/load.go`의 `Load`는 설정 병합의 유일한 공용 진입점이다. 순서는 `defaults < YAML < env`인데, 마지막에 `applyDefaults`를 한 번 더 돌려서 invalid/empty 값을 정규화하고 workspace 기반 경로를 다시 계산한다.

- `internal/config/defaults.go`: 안전한 기본값 집합을 제공한다.
- `internal/config/defaults_apply.go`: provider별 auth mode, base URL, model, workspace 파생 경로를 보정한다.
- `internal/config/yaml.go`: flat key 형태의 YAML을 읽는다.
- `internal/config/env.go`: 같은 필드를 환경 변수로 다시 덮는다.
- `cmd/tars/doctor_main.go`: config file, workspace skeleton, LLM credential, Claude CLI, gateway default agent 경로를 검증한다.

핵심 포인트는 "설정 파싱"과 "실행 가능성 진단"이 분리돼 있다는 점이다. 앱은 `config.Load`만 알면 되고, 설치/운영 가이드는 `doctor`가 맡는다.

## 3. 서버 부트스트랩 레이어

`internal/tarsserver/main.go`의 `Serve`는 서버 실행의 단일 진입점이다. 여기서 로거를 만들고 내부 root command를 실행한다.

실제 의존성 조립은 `internal/tarsserver/main_bootstrap.go`와 `internal/tarsserver/main_serve_api.go`에 있다.

- `internal/memory/workspace.go`: 워크스페이스 기본 파일과 디렉터리를 보장한다.
- `internal/session/session.go`: 세션 인덱스 저장소를 만든다.
- `internal/usage/tracker.go`: 비용 추적기와 제한 정책을 만든다.
- `internal/llm/provider.go`: 공급자별 LLM 클라이언트를 생성한다.
- `internal/project/store.go`: 프로젝트 문서 저장소를 연다.

서버는 이 단계에서 "로컬 상태 저장소"를 먼저 준비하고, 그다음에야 LLM과 외부 확장을 붙인다.

## 4. HTTP API 레이어

`internal/tarsserver/main_serve_api.go`가 API 서버와 백그라운드 런타임을 묶는다.

- 세션, 프로젝트, 사용량, cron, schedule, gateway, browser, events, dashboard 핸들러를 하나의 mux 아래 둔다.
- cron manager, watchdog, telegram poller, gateway runtime 같은 백그라운드 작업도 여기서 시작된다.
- `project.AutopilotManager`를 만들고, heartbeat 이후 `EnsureActiveRuns`를 호출해 프로젝트 supervision을 이어간다.

이 레이어의 진입 직후에는 인증 미들웨어가 붙는다.

- `internal/tarsserver/middleware.go`: API 공통 미들웨어를 조립한다.
- `internal/serverauth/middleware.go`: 인증 모드와 경로별 권한 요구사항을 계산한다.

권한 판단 규칙은 네 가지 축으로 나뉜다.

- auth mode: `off`, `external-required`, `required`
- path policy: skip path, admin path, optional dashboard bypass
- token role: legacy/admin/user bearer token
- workspace scope: 기본 workspace binding

`dashboard_auth_mode=off`이면 `/dashboards`와 `/ui/projects/*`는 인증 없이 접근 가능하다.

## 5. 채팅 실행 레이어

채팅 파이프라인은 네 파일로 나눠 읽으면 된다.

- `internal/tarsserver/handler_chat.go`: HTTP 요청 수신, SSE 스트림 시작, 최종 저장.
- `internal/tarsserver/handler_chat_context.go`: 세션/프로젝트/프롬프트/툴 주입 상태 조립.
- `internal/tarsserver/handler_chat_execution.go`: `agent.Loop` 실행과 스트리밍 delta 전달.
- `internal/tarsserver/handlers.go`: 공용 helper, prompt 조립, skill 해석, 상태 이벤트 포맷.

실제 호출 흐름은 다음과 같다.

```text
User or TUI
  -> pkg/tarsclient.Client.StreamChat
  -> POST /v1/chat
  -> prepareChatRunState
     -> session resolve
     -> transcript load
     -> project resolve
     -> prompt.BuildResultFor
     -> buildChatToolRegistry
     -> resolveInjectedToolSchemas
  -> agent.Loop.Run
     -> llm.Client.Chat
     -> tool.Registry.Execute
  -> transcript append + memory hook
  -> SSE done
```

추가로 이 레이어는 프로젝트 workflow 진입도 맡는다.

- kickoff 문장이나 활성 brief가 있으면 `handler_chat.go`가 `project-start` skill을 자동 선택한다.
- 즉, 프로젝트 시작은 별도 UI가 아니라 채팅 파이프라인 위에 얹힌 skill-driven flow다.

## 6. LLM + Tooling 레이어

에이전트 중심부는 다음 조합이다.

- `internal/agent/loop.go`: LLM 응답과 tool call을 반복 실행한다.
- `internal/tool/tool.go`: 런타임 툴 레지스트리와 schema 변환을 담당한다.
- `internal/tarsserver/helpers_build_tools.go`: cron, heartbeat, gateway, browser, web, apply_patch 같은 툴을 설정값에 맞춰 켠다.
- `internal/llm/provider.go`: OpenAI 계열, Gemini, Anthropic 같은 공급자를 공통 인터페이스로 감싼다.
- `internal/auth/token.go` + `internal/auth/codex_oauth.go`: provider별 API key/OAuth credential source와 refresh 가능한 Codex credential을 분리한다.
- `internal/llm/openai_compat_client.go`, `internal/llm/anthropic.go`, `internal/llm/gemini_native.go`, `internal/llm/openai_codex_client.go`: transport 형식이 다른 provider client를 각각 구현한다.
- `internal/prompt/builder.go`: 워크스페이스 파일과 관련 메모리를 읽어 시스템 프롬프트를 만든다.

핵심 포인트는 "프롬프트와 툴이 모두 워크스페이스 상태에 영향을 받는다"는 점이다. 이 저장소는 단순 챗봇이 아니라, 워크스페이스 문서와 로컬 파일 구조를 운영 인터페이스로 본다.

추가로 provider client의 공통점과 차이도 중요하다.

- OpenAI-compatible 계열은 `/chat/completions` 와 공통 SSE parser를 공유한다.
- Anthropic은 system/tool/thinking 개념을 자체 wire format으로 다시 매핑한다.
- Gemini Native는 HTTP JSON 수작업보다 SDK 호출과 model preflight 검사 쪽에 무게가 있다.
- Codex는 OAuth refresh 와 Responses API shape 때문에 별도 재시도 로직을 가진다.

## 7. 확장 레이어

`internal/extensions/manager.go`가 확장 통합 허브다.

- skill: `internal/skill/loader.go`
- plugin: `internal/plugin/loader.go`
- MCP: `internal/mcp/client.go`

reload 후에는 하나의 `Snapshot`으로 skill 목록, plugin 목록, MCP 서버, 진단 메시지를 제공한다. 서버는 이 snapshot을 채팅 프롬프트와 tool registry에 반영한다.

plugin 로딩은 단순 manifest 수집이 아니라 보안 제약을 포함한다.

- manifest 파일은 `tars.plugin.json`이 우선이고 legacy 이름도 허용된다.
- plugin이 노출하는 skill 경로는 plugin root 밖으로 escape할 수 없다.
- plugin이 선언한 MCP 서버는 이름 기준으로 dedupe된다.

## 8. 프로젝트 운영 레이어

`internal/project/*`는 단순 메타데이터 저장소가 아니라, brief -> board -> execution -> dashboard 흐름 전체를 관리한다.

- `brief_state.go`: session 기반 brief와 프로젝트 `STATE.md`를 저장한다.
- `kanban.go`: `KANBAN.md` frontmatter를 읽고 canonical column/status로 정규화한다.
- `activity.go` + `activity_auto.go`: 상태 변화와 worker report를 `ACTIVITY.jsonl`에 쌓는다.
- `orchestrator.go`: `todo`와 `review` task dispatch, verification/GitHub Flow gate, agent report 기록을 처리한다.
- `project_runner.go`: autopilot 루프를 돌며 backlog seed, auto-retry, done/block/failed 상태를 관리한다.
- `task_report.go`: worker output을 고정 포맷 `<task-report>`로 파싱한다.

핵심 포인트는 "프로젝트 진행 상태가 코드 메모리가 아니라 파일 문서 집합으로 남는다"는 점이다.

## 9. 비동기 실행과 브라우저 레이어

`internal/gateway/*`와 `internal/browser/*`는 채팅 API와 별도로 오래 사는 실행 경로를 제공한다.

- `internal/gateway/runtime_run_bootstrap.go`: run을 accepted 상태로 등록하고 worker session을 정한다.
- `internal/gateway/runtime_run_execute.go`: accepted -> running -> completed/failed/canceled 전이를 처리한다.
- `internal/gateway/project_task_runner.go`: 프로젝트의 logical worker kind와 실제 gateway executor를 분리한다. alias가 없으면 default agent로 fallback해도 logical kind는 보존한다.
- `internal/gateway/runtime_persist.go`: run/channel snapshot을 저장하고 재시작 시 복구한다.
- `internal/browser/service.go`: 사이트 flow를 읽고 login/check/run API를 제공한다.
- `internal/browser/playwright_exec.go`: 실제 브라우저 제어는 Node + Playwright 스크립트로 위임한다.

즉, 브라우저 기능은 Go 안에 직접 임베드된 엔진이 아니라 "Go 서비스 레이어 + Node 실행기" 2단 구조다.

## 10. 호스트 서비스와 relay 경계

이 저장소는 HTTP API 바깥 운영 경계도 별도 adapter로 둔다.

- `cmd/tars/service_main.go`: 현재 `tars serve` 실행 파일을 macOS LaunchAgent plist로 감싸고 `launchctl`과 연결한다.
- `internal/browserrelay/server.go`: loopback + relay token + origin allowlist 조건을 만족한 브라우저 확장만 CDP relay에 붙인다.
- `internal/approval/otp.go`: Telegram 같은 외부 채널에서 들어오는 OTP 코드를 chat ID 단위로 대기/소비한다.
- `internal/auth/codex_oauth.go` + `internal/llm/openai_codex_client.go`: OpenAI Codex처럼 refresh token 과 account header 가 필요한 provider를 별도 credential lifecycle 로 감싼다.

핵심 포인트는 "운영 경계를 비즈니스 로직에 직접 섞지 않고, 별도 adapter와 in-memory coordinator로 분리한다"는 점이다.

## 11. 대시보드 레이어

`internal/tarsserver/dashboard.go`는 프로젝트 상태를 읽기 전용 HTML 대시보드로 노출한다.

- `/dashboards`: 프로젝트 목록
- `/ui/projects/{id}`: 개별 프로젝트 화면
- `/ui/projects/{id}/stream`: EventSource 갱신 채널

이 대시보드는 API를 별도 SPA로 감싼 것이 아니라, 서버 렌더링된 HTML 조각을 SSE 이벤트가 올 때만 부분 새로고침하는 얇은 구조다. 그래서 워크스페이스 문서와 activity 로그가 곧바로 운영 UI가 된다.
