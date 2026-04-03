# TARS 저장소 개요

TARS는 Go로 작성된 local-first AI project autopilot 런타임이다. 지금 헤드 기준 저장소는 더 이상 "터미널 TUI 중심 도구"라기보다, 웹 콘솔을 기본 UX로 두고 로컬 HTTP 서버, one-shot CLI, 에이전트 루프, 워크스페이스 문서 저장소, 프로젝트 오토파일럿, skill/plugin/MCP 확장, Skill Hub 배포, semantic memory, 브라우저 자동화, 외부 provider 경계를 하나의 바이너리 안에 묶은 제품형 모놀리스에 가깝다.

처음 읽을 때는 진입점을 아홉 묶음으로 잡으면 빠르다.

- `cmd/tars/main.go`: 루트 Cobra 명령이다. 기본 동작은 `/console` 열기이고, `--message`가 있으면 one-shot CLI 채팅으로 전환된다. 이 파일이 `internal/browserplugin`을 blank import해 내장 브라우저 플러그인을 프로세스 시작 시 등록한다.
- `cmd/tars/console_main.go`, `cmd/tars/client_main.go`: 브라우저 콘솔 오프너와 one-shot CLI 채팅 진입점이다. 예전 TUI는 hidden/deprecated 커맨드로만 남아 있다.
- `cmd/tars/server_main.go`: 서버 플래그를 `internal/tarsserver.Serve`에 넘긴다.
- `cmd/tars/assistant_main.go`: 음성 보조 기능과 LaunchAgent 설치를 위한 보조 진입점이다.
- `cmd/tars/doctor_main.go`: starter workspace, 설정 파일, BYOK, gateway executor, 콘솔/브라우저 런타임 준비 상태를 진단한다.
- `cmd/tars/project_main.go`, `cmd/tars/cron_main.go`, `cmd/tars/approval_main.go`: 프로젝트 운영, 일정, 승인 관련 one-shot CLI 표면이다.
- `cmd/tars/skill_main.go`, `cmd/tars/plugin_main.go`, `cmd/tars/mcp_main.go`: Hub 검색/설치/업데이트와 MCP 관리 표면이다.
- `cmd/tars/service_main.go`: macOS 서비스 설치와 `launchctl` 연동 진입점이다.
- `cmd/tars/mainthread_darwin.go`, `cmd/tars/mainthread_other.go`: macOS 전역 hotkey 제약 때문에 루트 명령을 메인 스레드에서 실행할지 결정한다.

코어 사용자 흐름은 여전히 "입력 -> 프롬프트 조립 -> 툴 주입 -> LLM 호출 -> 결과 저장"이지만, 지금은 중간에 per-request tool policy, runtime skill mirror, semantic recall, SSE 상태 스트림, 프로젝트 workflow 훅이 더 적극적으로 끼어든다.

1. 사용자가 웹 콘솔 또는 one-shot CLI에서 `/v1/chat` 요청을 보낸다.
2. 서버가 세션과 프로젝트 문맥을 해석하고 transcript snapshot을 읽는다.
3. `internal/prompt/builder.go`와 `internal/prompt/memory_retrieval.go`가 워크스페이스 문서와 relevant memory를 읽어 시스템 프롬프트를 만든다.
4. `internal/extensions/manager.go`가 skill/plugin/MCP snapshot을 만들고, `internal/skill/mirror.go`가 runtime skill 파일을 `workspace/_shared/skills_runtime` 아래에 미러링한다.
5. `internal/tarsserver/handler_chat_policy.go`가 tool registry와 tool schema를 구성하고, 프로젝트 tool policy와 auth role 기반의 high-risk 필터를 적용한다.
6. `internal/agent/loop.go`가 LLM 응답과 tool-calling을 반복 실행한다.
7. 응답은 `internal/session/transcript.go`를 통해 JSONL transcript에 저장되고, compaction/semantic memory 훅이 후처리를 수행한다.
8. `internal/tarsserver/handler_chat_stream.go`가 status/delta/done 이벤트를 SSE로 흘려 보낸다.

LLM provider 레이어는 더 이상 단순 adapter 모음이 아니다.

- `internal/auth/provider_credentials.go`: provider별 auth mode, OAuth source, refresh 가능 여부를 strategy처럼 정리한다.
- `internal/llm/provider.go`: 공급자 선택, credential 해석, client 초기화를 담당한다.
- `internal/llm/model_lister.go`: provider live API를 직접 호출해 model id를 조회한다.
- `internal/llm/openai_compat_client.go`: OpenAI 계열 `/chat/completions` 공용 adapter
- `internal/llm/anthropic.go`: Anthropic `/v1/messages` 전용 adapter
- `internal/llm/gemini_native.go`: Gemini Native REST `generateContent` adapter
- `internal/llm/openai_codex_client.go`: ChatGPT backend `responses` API + OAuth refresh adapter

현재 헤드에서 눈에 띄는 축은 여섯 개다.

- 콘솔 우선 UX 축: `cmd/tars/main.go`, `cmd/tars/console_main.go`, `internal/tarsserver/console.go`, `internal/tarsserver/legacy_dashboard.go`가 기본 사용 경로를 웹 콘솔로 고정하고, 예전 `/dashboards`와 `/ui/projects/*`는 `/console`로 리다이렉트한다.
- 채팅 실행 분리 축: `internal/tarsserver/handler_chat.go`, `handler_chat_context.go`, `handler_chat_policy.go`, `handler_chat_execution.go`, `handler_chat_stream.go`가 세션/프롬프트/툴/실행/스트리밍 관심사를 조금 더 분리한다.
- 내장 플러그인 축: `internal/browserplugin/*`, `internal/plugin/builtin*.go`, `internal/extensions/manager.go`가 compile-time 내장 플러그인을 일반 플러그인 스냅샷과 같은 표면으로 노출한다.
- 프로젝트 운영 축: `internal/project/workflow_policy.go`, `internal/project/policy.go`, `internal/project/orchestrator.go`, `internal/project/orchestrator_plan.go`, `internal/tarsserver/helpers_project_progress.go`, `internal/tarsserver/dashboard.go`가 brief, tool policy, backlog planning, dispatch, heartbeat 기반 autonomous progress, dashboard projection을 나눠 맡는다.
- 설정/진단 축: `internal/config/config_input_fields.go`가 YAML/env/merge 규칙을 하나의 필드 테이블로 모으고, `cmd/tars/doctor_main.go`가 실제 실행 전 위험한 기본값과 누락된 실행기를 잡아낸다.
- 허브 배포 축: `cmd/tars/skill_main.go`, `cmd/tars/plugin_main.go`, `cmd/tars/mcp_main.go`, `internal/skillhub/*`가 raw GitHub registry를 읽어 skill/plugin/MCP package를 workspace에 설치하고 상태를 남긴다.

HTTP 요청은 채팅 로직에 들어가기 전에 인증 미들웨어를 지난다.

- `internal/tarsserver/middleware.go`: API 미들웨어 조립 지점이다.
- `internal/serverauth/middleware.go`: auth mode, bearer token, admin path, workspace binding, loopback 예외를 판정한다.
- `dashboard_auth_mode=off`이어도 콘솔/대시보드 skip은 loopback 요청에서만 허용된다.

확장 계층은 runtime 배포와 실제 실행을 분리한다.

- `internal/plugin/loader.go`: plugin manifest를 읽어 skill 디렉터리와 MCP 서버를 추출한다.
- `internal/plugin/builtin_registry.go`: 내장 Go 플러그인을 전역 registry에 등록한다.
- `internal/extensions/manager.go`: skill/plugin/MCP snapshot을 만들고 built-in plugin init, lifecycle hook, HTTP/tool surface 수집까지 맡는다.
- `internal/skill/mirror.go`: 로딩된 skill과 companion file을 `workspace/_shared/skills_runtime` 아래에 복사해 agent가 안정된 runtime path를 보게 만든다.
- `internal/skillhub/install.go`: Skill Hub에서 받은 skill/plugin 파일을 workspace `skills/`, `plugins/`에 설치한다.

저장소를 이해할 때는 코드보다 먼저 "상태가 어디에 쌓이는가"를 같이 봐야 한다.

- `workspace/sessions/*`: 세션 메타데이터와 transcript가 저장된다.
- `workspace/memory/*`, `workspace/MEMORY.md`, `workspace/AGENTS.md`: 프롬프트와 장기 기억 입력원이다.
- `workspace/memory/semantic/*`: embedding index와 source state가 저장된다.
- `workspace/projects/*`: 프로젝트별 `PROJECT.md`, `STATE.md`, `KANBAN.md`, `ACTIVITY.jsonl`, `AUTOPILOT.json`이 저장된다.
- `workspace/_shared/project_briefs/*`: project-start 인터뷰 상태가 저장된다.
- `workspace/usage/*`: 모델 사용량과 비용 제한이 저장된다.
- `workspace/_shared/*`: gateway, browser, runtime skill mirror 같은 공유 런타임 상태가 저장된다.
- `workspace/ops/*`, `workspace/schedule/*`: cleanup approval 이벤트와 일정 흔적이 저장된다.
- `workspace/skillhub.json`: Hub에서 설치한 skill/plugin/MCP 버전과 경로를 추적한다.

신규 기여자 기준으로 핵심 패키지는 열한 묶음으로 보면 된다.

- `internal/config` + `cmd/tars/doctor_main.go`: 설정 로딩, 기본값, field-table 기반 YAML/env merge, 스타터 환경 진단.
- `internal/tarsserver`: 서버 부트스트랩, HTTP/console 핸들러, 채팅 파이프라인, project/autopilot HTTP surface, provider model/config/runtime 엔드포인트.
- `internal/tarsclient` + `pkg/tarsclient`: one-shot CLI 채팅, 로컬 런타임 제어 커맨드, HTTP/SSE 클라이언트 SDK. 대화형 TUI는 제거됐다.
- `internal/agent`, `internal/tool`, `internal/llm`, `internal/prompt`: LLM 호출과 tool-calling 중심부.
- `internal/extensions`, `internal/skill`, `internal/plugin`, `internal/mcp`: 동적 확장 로딩, built-in plugin lifecycle, runtime mirror 계층.
- `cmd/tars/skill_main.go`, `cmd/tars/plugin_main.go`, `cmd/tars/mcp_main.go`, `internal/skillhub`: 원격 registry 검색과 workspace 설치 계층.
- `internal/session`, `internal/memory`, `internal/project`, `internal/usage`, `internal/cron`: 워크스페이스 영속화와 프로젝트 문서 계층.
- `internal/gateway`, `internal/browser`, `internal/schedule`, `internal/ops`: 비동기 실행, 채널/리포트, 브라우저 자동화, 일정/운영 자동화 계층.
- `internal/serverauth`: API 인증, 역할, workspace 문맥 바인딩 계층.
- `cmd/tars/service_main.go` + `internal/browserrelay` + `internal/approval` + `internal/auth`: host service, relay bridge, OTP 승인, provider credential 갱신 같은 외부 경계 adapter 계층.
- `plugins/project-swarm`: bundled workflow skill 문서와 실제 project/autopilot 구현이 맞물리는 dogfooding 계층.
