# TARS 저장소 개요

TARS는 Go로 작성된 local-first 자동화 런타임이다. 저장소 하나 안에 CLI/TUI, 로컬 HTTP 서버, 에이전트 루프, 워크스페이스 파일 저장소, 프로젝트 오토파일럿, skill/plugin/MCP 확장, Skill Hub 배포, semantic memory, 브라우저 자동화, host OS 및 외부 provider 경계가 같이 들어 있다.

처음 읽을 때는 진입점을 여덟 묶음으로 잡으면 빠르다.

- `cmd/tars/main.go`: 루트 Cobra 명령이다. 기본 동작은 클라이언트 실행이고 `serve`, `assistant`, `doctor`, `init`, `service`, `skill`, `plugin`, `version` 서브커맨드를 붙인다.
- `cmd/tars/mainthread_darwin.go`, `cmd/tars/mainthread_other.go`: macOS 전역 hotkey 제약 때문에 루트 명령을 메인 스레드에서 실행할지 결정한다.
- `cmd/tars/client_main.go`: CLI 플래그를 `internal/tarsclient.Run` 옵션으로 변환한다.
- `cmd/tars/server_main.go`: 서버 플래그를 `internal/tarsserver.Serve`에 넘긴다.
- `cmd/tars/assistant_main.go`: 음성 보조 기능과 LaunchAgent 설치를 위한 보조 진입점이다.
- `cmd/tars/doctor_main.go`: starter workspace, 설정 파일, BYOK, gateway executor 준비 상태를 진단한다.
- `cmd/tars/skill_main.go`: Skill Hub 검색, 설치, 업데이트, 제거, 정보 조회를 담당한다.
- `cmd/tars/plugin_main.go`: Plugin Hub 검색, 설치, 업데이트, 제거, 정보 조회를 담당한다.

코어 사용자 흐름은 여전히 "입력 -> 프롬프트 조립 -> 툴 주입 -> LLM 호출 -> 결과 저장"이지만, 이제 중간에 semantic recall과 runtime skill mirror가 끼어든다.

1. 클라이언트 또는 SDK가 `/v1/chat`으로 메시지를 보낸다.
2. 서버가 세션과 프로젝트 문맥을 해석하고 transcript를 읽는다.
3. `internal/prompt/builder.go`와 `internal/prompt/memory_retrieval.go`가 워크스페이스 문서와 relevant memory를 읽어 시스템 프롬프트를 만든다.
4. `internal/extensions/manager.go`가 skill/plugin/MCP snapshot을 만들고, `internal/skill/mirror.go`가 runtime skill 파일을 `workspace/_shared/skills_runtime` 아래에 미러링한다.
5. `internal/tarsserver/helpers_build_tools.go`와 `internal/tool/*`가 툴을 준비한다.
6. `internal/agent/loop.go`가 LLM 응답과 tool-calling을 반복 실행한다.
7. 응답은 `internal/session/transcript.go`를 통해 JSONL transcript에 저장되고, compaction/semantic memory 훅이 후처리를 수행한다.

LLM provider 레이어는 더 이상 단순 adapter 모음이 아니다.

- `internal/auth/provider_credentials.go`: provider별 auth mode, OAuth source, refresh 가능 여부를 strategy로 정리한다.
- `internal/llm/provider.go`: 공급자 선택, credential 해석, client 초기화를 담당한다.
- `internal/llm/model_lister.go`: provider live API를 직접 호출해 model id를 조회한다.
- `internal/llm/openai_compat_client.go`: OpenAI 계열 `/chat/completions` 공용 adapter
- `internal/llm/anthropic.go`: Anthropic `/v1/messages` 전용 adapter
- `internal/llm/gemini_native.go`: Gemini Native REST `generateContent` adapter
- `internal/llm/openai_codex_client.go`: ChatGPT backend `responses` API + OAuth refresh adapter

이번 증분 기준으로 새로 눈에 띄는 축은 다섯 개다.

- 설정/진단 축: `internal/config/config_input_fields.go`가 YAML/env/merge 규칙을 하나의 필드 테이블로 모으고, `cmd/tars/doctor_main.go`가 실제 실행 전 위험한 기본값과 누락된 실행기를 잡아낸다.
- semantic memory 축: `internal/memory/semantic.go`, `internal/prompt/memory_retrieval.go`, `internal/tool/memory_search.go`, `internal/tool/memory_save.go`가 explicit memory, compaction summary, project 문서를 embedding 기반으로 다시 찾게 만든다.
- 허브 배포 축: `cmd/tars/skill_main.go`, `cmd/tars/plugin_main.go`, `internal/skillhub/*`가 raw GitHub registry를 읽어 skill/plugin을 workspace에 설치하고 `skillhub.json`에 상태를 남긴다.
- 프로젝트 운영 축: `internal/project/workflow_policy.go`, `internal/project/orchestrator.go`, `internal/project/project_runner.go`, `internal/tarsserver/dashboard.go`가 workflow 규칙, dispatch, supervision, dashboard projection을 조금 더 명시적으로 나눈다.
- 외부 경계 축: `cmd/tars/service_main.go`, `internal/browserrelay/server.go`, `internal/approval/otp.go`, `internal/auth/*`, `internal/llm/*`가 launchd, 브라우저 확장 relay, 인간 승인 입력, provider credential refresh 경계를 별도 adapter로 감싼다.

HTTP 요청은 채팅 로직에 들어가기 전에 인증 미들웨어를 지난다.

- `internal/tarsserver/middleware.go`: API 미들웨어 조립 지점이다.
- `internal/serverauth/middleware.go`: auth mode, bearer token, admin path, workspace binding, loopback 예외를 판정한다.
- `dashboard_auth_mode=off`이면 `/dashboards`와 `/ui/projects/*`는 이제 "무조건 공개"가 아니라 loopback 요청일 때만 인증을 건너뛴다.

확장 계층은 runtime 배포와 실제 실행을 분리한다.

- `internal/plugin/loader.go`: plugin manifest를 읽어 skill 디렉터리와 MCP 서버를 추출한다.
- `internal/extensions/manager.go`: skill/plugin/MCP snapshot을 만들고 MCP 실패를 진단 메시지로만 남긴 채 서버 기동은 계속할 수 있다.
- `internal/skill/mirror.go`: 로딩된 skill과 companion file을 `workspace/_shared/skills_runtime` 아래에 복사해 agent가 안정된 runtime path를 보게 만든다.
- `internal/skillhub/install.go`: Skill Hub에서 받은 skill/plugin 파일을 workspace `skills/`, `plugins/`에 설치한다.

저장소를 이해할 때는 코드보다 먼저 "상태가 어디에 쌓이는가"를 같이 봐야 한다.

- `workspace/sessions/*`: 세션 메타데이터와 transcript가 저장된다.
- `workspace/memory/*`, `workspace/MEMORY.md`, `workspace/AGENTS.md`: 프롬프트와 장기 기억 입력원이다.
- `workspace/memory/semantic/*`: embedding index와 source state가 저장된다.
- `workspace/projects/*`: 프로젝트별 `PROJECT.md`, `STATE.md`, `KANBAN.md`, `ACTIVITY.jsonl`, `AUTOPILOT.json`이 저장된다.
- `workspace/_shared/project_briefs/*`: `/project-start`가 수집하는 brief 상태가 저장된다.
- `workspace/usage/*`: 모델 사용량과 비용 제한이 저장된다.
- `workspace/_shared/*`: gateway, browser, voice, runtime skill mirror 같은 공유 런타임 상태가 저장된다.
- `workspace/ops/*`, `workspace/schedule/*`: cleanup approval 이벤트와 일정 마이그레이션 흔적이 저장된다.
- `workspace/skillhub.json`: Hub에서 설치한 skill/plugin 버전과 경로를 추적한다.

신규 기여자 기준으로 핵심 패키지는 열한 묶음으로 보면 된다.

- `internal/config` + `cmd/tars/doctor_main.go`: 설정 로딩, 기본값, field-table 기반 YAML/env merge, 스타터 환경 진단.
- `internal/tarsserver`: 서버 부트스트랩, HTTP 핸들러, 채팅 파이프라인, provider model 조회, dashboard 노출.
- `internal/tarsclient` + `pkg/tarsclient`: Bubble Tea TUI와 HTTP/SSE 클라이언트 SDK.
- `internal/agent`, `internal/tool`, `internal/llm`, `internal/prompt`: LLM 호출과 tool-calling 중심부.
- `internal/extensions`, `internal/skill`, `internal/plugin`, `internal/mcp`: 동적 확장 로딩과 runtime mirror 계층.
- `cmd/tars/skill_main.go`, `cmd/tars/plugin_main.go`, `internal/skillhub`: 원격 registry 검색과 workspace 설치 계층.
- `internal/session`, `internal/memory`, `internal/project`, `internal/usage`, `internal/cron`: 워크스페이스 영속화와 프로젝트 문서 계층.
- `internal/gateway`, `internal/browser`, `internal/schedule`, `internal/ops`: 비동기 실행, 채널/리포트, 브라우저 자동화, 일정/운영 자동화 계층.
- `internal/serverauth`: API 인증, 역할, workspace 문맥 바인딩 계층.
- `cmd/tars/service_main.go` + `internal/browserrelay` + `internal/approval` + `internal/auth`: host service, relay bridge, OTP 승인, provider credential 갱신 같은 외부 경계 adapter 계층.
- `plugins/project-swarm`: bundled workflow skill 문서와 실제 project/autopilot 구현이 맞물리는 dogfooding 계층.
