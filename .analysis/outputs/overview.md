# TARS 저장소 개요

TARS는 Go로 작성된 local-first 자동화 런타임이다. 저장소 하나 안에 CLI/TUI, 로컬 HTTP 서버, LLM 에이전트 루프, 워크스페이스 파일 저장소, 프로젝트 오토파일럿, skill/plugin/MCP 확장, 브라우저 자동화, host OS 및 외부 provider 경계가 같이 들어 있다.

처음 읽을 때는 진입점을 여섯 묶음으로 잡으면 빠르다.

- `cmd/tars/main.go`: 루트 Cobra 명령이다. 기본 동작은 클라이언트 실행이고 `serve`, `assistant`, `doctor`, `init`, `service` 서브커맨드를 붙인다.
- `cmd/tars/client_main.go`: CLI 플래그를 `internal/tarsclient.Run` 옵션으로 변환한다.
- `cmd/tars/server_main.go`: 서버 플래그를 `internal/tarsserver.Serve`에 넘긴다.
- `cmd/tars/assistant_main.go`: 음성 보조 기능과 LaunchAgent 설치를 위한 보조 진입점이다.
- `cmd/tars/doctor_main.go`: starter workspace, 설정 파일, BYOK, gateway executor 준비 상태를 진단한다.
- `cmd/tars/service_main.go`: macOS LaunchAgent plist를 설치하고 `tars serve`를 서비스처럼 기동/중지/조회한다.

코어 사용자 흐름은 여전히 "입력 -> 프롬프트 조립 -> 툴 주입 -> LLM 호출 -> 결과 저장"이다.

1. 클라이언트 또는 SDK가 `/v1/chat`으로 메시지를 보낸다.
2. 서버가 세션과 프로젝트 문맥을 해석하고 transcript를 읽는다.
3. `internal/prompt/builder.go`가 워크스페이스 문서와 메모리를 읽어 시스템 프롬프트를 만든다.
4. `internal/tarsserver/helpers_build_tools.go`와 `internal/extensions/manager.go`가 툴, skill, plugin, MCP 확장을 준비한다.
5. `internal/agent/loop.go`가 LLM 응답과 tool-calling을 반복 실행한다.
6. 응답은 `internal/session/transcript.go`를 통해 JSONL transcript에 저장되고, 메모리 훅이 후처리를 수행한다.

LLM provider 레이어는 하나의 HTTP client로 통일돼 있지 않다.

- `internal/llm/openai_compat_client.go`: OpenAI 계열 `/chat/completions` 공용 adapter
- `internal/llm/anthropic.go`: Anthropic `/v1/messages` 전용 adapter
- `internal/llm/gemini_native.go`: Google GenAI SDK 기반 `generateContent` adapter
- `internal/llm/openai_codex_client.go`: ChatGPT backend `responses` API + OAuth refresh adapter

이번 커밋 기준으로 새로 눈에 띄는 축은 세 개다.

- 설정/진단 축: `internal/config/*`가 flat `Config`를 defaults < YAML < env 순서로 합치고, `cmd/tars/doctor_main.go`가 실제 실행 전 위험한 기본값과 누락된 실행기를 잡아낸다.
- 프로젝트 운영 축: `internal/project/*`, `internal/gateway/project_task_runner.go`, `internal/tarsserver/dashboard.go`가 프로젝트 보드, activity 로그, autopilot 루프, HTML 대시보드를 하나의 workflow로 묶는다.
- 외부 경계 축: `cmd/tars/service_main.go`, `internal/browserrelay/server.go`, `internal/approval/otp.go`, `internal/auth/*`, `internal/llm/openai_codex_client.go`가 launchd, 브라우저 확장 relay, 인간 승인 입력, provider credential refresh 경계를 별도 adapter로 감싼다.

HTTP 요청은 채팅 로직에 들어가기 전에 인증 미들웨어를 지난다.

- `internal/tarsserver/middleware.go`: API 미들웨어 조립 지점이다.
- `internal/serverauth/middleware.go`: auth mode, bearer token, admin path, workspace binding, loopback 예외를 판정한다.
- `dashboard_auth_mode`가 `off`면 `/dashboards`와 `/ui/projects/*`는 인증 없이 볼 수 있다.

확장 계층은 skill만 읽는 단순 구조가 아니다.

- `internal/plugin/loader.go`: plugin manifest를 읽어 skill 디렉터리와 MCP 서버를 추출한다.
- plugin은 경로 탈출을 막기 위해 skill 디렉터리를 plugin root 내부로 제한한다.
- plugin snapshot 결과는 다시 `internal/extensions/manager.go`로 흘러가 skill/plugin/MCP 통합 snapshot이 된다.

저장소를 이해할 때는 코드보다 먼저 "상태가 어디에 쌓이는가"를 같이 봐야 한다.

- `workspace/sessions/*`: 세션 메타데이터와 transcript가 저장된다.
- `workspace/memory/*`, `workspace/MEMORY.md`, `workspace/AGENTS.md`: 프롬프트와 장기 기억 입력원이다.
- `workspace/projects/*`: 프로젝트별 `PROJECT.md`, `STATE.md`, `KANBAN.md`, `ACTIVITY.jsonl`, `AUTOPILOT.json`이 저장된다.
- `workspace/_shared/project_briefs/*`: `/project-start`가 수집하는 brief 상태가 저장된다.
- `workspace/usage/*`: 모델 사용량과 비용 제한이 저장된다.
- `workspace/_shared/*`: gateway, browser, voice 같은 공유 런타임 상태가 저장된다.
- `workspace/ops/*`, `workspace/schedule/*`: cleanup approval 이벤트와 일정 마이그레이션 흔적이 저장된다.

신규 기여자 기준으로 핵심 패키지는 아홉 묶음으로 보면 된다.

- `internal/config` + `cmd/tars/doctor_main.go`: 설정 로딩, 기본값, 스타터 환경 진단.
- `internal/tarsserver`: 서버 부트스트랩, HTTP 핸들러, 채팅 파이프라인, dashboard 노출.
- `internal/tarsclient` + `pkg/tarsclient`: Bubble Tea TUI와 HTTP/SSE 클라이언트 SDK.
- `internal/agent`, `internal/tool`, `internal/llm`, `internal/prompt`: LLM 호출과 tool-calling 중심부.
- `internal/extensions`, `internal/skill`, `internal/plugin`, `internal/mcp`: 동적 확장 로딩 계층.
- `internal/session`, `internal/memory`, `internal/project`, `internal/usage`, `internal/cron`: 워크스페이스 영속화와 프로젝트 문서 계층.
- `internal/gateway`, `internal/browser`, `internal/schedule`, `internal/ops`: 비동기 실행, 브라우저 자동화, 일정/운영 자동화 계층.
- `internal/serverauth`: API 인증, 역할, workspace 문맥 바인딩 계층.
- `cmd/tars/service_main.go` + `internal/browserrelay` + `internal/approval` + `internal/auth`: host service, relay bridge, OTP 승인, provider credential 갱신 같은 외부 경계 adapter 계층.
