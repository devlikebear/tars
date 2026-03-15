# 튜토리얼: TARS 로컬 채팅 런타임

> 대상 독자: 이 저장소에 처음 들어오는 Go 개발자
> 목표 시간: 90분
> 기준 세션: `analyze-20260310-140305` / `checkpoint-011`

---

## 학습 목표

- TARS가 `cmd/tars/main.go`에서 `internal/tarsserver`와 `internal/tarsclient`로 갈라지는 구조를 설명할 수 있다.
- `internal/config/*`와 `cmd/tars/doctor_main.go`가 왜 별도 진단 계층으로 존재하는지 설명할 수 있다.
- `/v1/chat` 요청이 세션 저장, 프롬프트 조립, 툴 주입, LLM 실행으로 이어지는 흐름을 추적할 수 있다.
- 워크스페이스 기반 상태 저장이 왜 이 저장소의 핵심인지 설명할 수 있다.
- project autopilot, gateway run, schedule, browser 자동화가 메인 채팅 파이프라인과 어떻게 연결되는지 설명할 수 있다.
- host service, browser relay, provider adapter가 왜 별도 경계 계층으로 분리돼 있는지 설명할 수 있다.

---

## 사전지식과 실행환경

- 필수 사전지식: Go 모듈, HTTP, JSON, 기본 CLI 사용법
- 런타임: Go `1.25.6`
- 선택 런타임: Node.js (Playwright 설치 시)
- 권장 명령:
  - `make dev-serve`
  - `make dev-tars`
  - `go test ./...`
- 시작 파일:
  - `cmd/tars/main.go`
  - `internal/tarsserver/main.go`

---

## 아키텍처 한눈에 보기

| 레이어 | 경로 | 책임 |
|--------|------|------|
| CLI | `cmd/tars/*` | 사용자 입력을 내부 런타임 옵션으로 변환 |
| Client | `internal/tarsclient/*`, `pkg/tarsclient/*` | TUI와 HTTP/SSE SDK |
| Server | `internal/tarsserver/*` | 설정 로딩, API 핸들러, 채팅 실행 |
| Agent Core | `internal/agent/*`, `internal/tool/*`, `internal/llm/*` | LLM 호출과 tool-calling |
| Workspace | `internal/session/*`, `internal/memory/*`, `internal/project/*` | 로컬 상태 저장 |
| Automation | `internal/gateway/*`, `internal/browser/*`, `internal/schedule/*` | 비동기 실행, 브라우저, 일정 |
| Boundary Adapters | `cmd/tars/service_main.go`, `internal/auth/*`, `internal/browserrelay/*` | host OS, OAuth, 브라우저 relay 경계 |

```text
TUI/SDK -> /v1/chat -> chat context 준비 -> agent loop -> tool 실행 -> transcript 저장
```

---

## 단계별 따라 읽기

### Step 1. 진입점 지도 만들기

읽을 파일:

- `cmd/tars/main.go`
- `cmd/tars/client_main.go`
- `cmd/tars/server_main.go`
- `cmd/tars/assistant_main.go`

확인 포인트:

- 루트 명령의 기본 동작이 클라이언트 실행인지 확인한다.
- `serve`와 `assistant`가 별도 런타임 진입점인지 확인한다.

### Step 2. 서버 부트스트랩 이해하기

읽을 파일:

- `internal/tarsserver/main.go`
- `internal/tarsserver/main_bootstrap.go`
- `internal/config/load.go`
- `internal/config/defaults_apply.go`
- `cmd/tars/doctor_main.go`
- `internal/memory/workspace.go`

확인 포인트:

- 설정 병합 순서가 defaults < YAML < env 인지 확인한다.
- `applyDefaults`가 마지막에 다시 돌면서 provider/auth/workspace 파생값을 보정하는지 확인한다.
- 서버 시작 시 워크스페이스가 자동으로 준비되는지 확인한다.
- `doctor`가 config 파일 생성과 실제 실행 가능성 진단을 분리하는지 확인한다.
- 세션 저장소, 사용량 추적기, LLM 클라이언트가 어디서 생성되는지 찾는다.

### Step 3. 채팅 파이프라인 추적하기

읽을 파일:

- `internal/tarsserver/handler_chat.go`
- `internal/tarsserver/handler_chat_context.go`
- `internal/tarsserver/handler_chat_execution.go`
- `internal/agent/loop.go`
- `internal/tool/tool.go`

확인 포인트:

- 사용자 메시지가 transcript에 언제 저장되는지 확인한다.
- tool schema가 어떤 조건에서 주입되는지 본다.
- LLM 응답에 tool call이 있을 때 어떤 루프로 실행되는지 확인한다.

### Step 4. 클라이언트 스트리밍 이해하기

읽을 파일:

- `internal/tarsclient/app_model.go`
- `internal/tarsclient/app_update.go`
- `pkg/tarsclient/client.go`

확인 포인트:

- SSE delta와 status 이벤트가 화면에 어떻게 반영되는지 확인한다.
- 세션 ID가 서버에서 발급될 때 클라이언트 상태가 어떻게 갱신되는지 확인한다.

### Step 5. 확장과 워크스페이스 연결 보기

읽을 파일:

- `internal/extensions/manager.go`
- `internal/skill/loader.go`
- `internal/mcp/client.go`
- `internal/project/store.go`
- `internal/session/compaction.go`

확인 포인트:

- skill/plugin/MCP가 하나의 snapshot으로 합쳐지는지 확인한다.
- project policy가 프롬프트와 툴 허용 범위에 어떤 영향을 주는지 본다.
- 긴 대화가 compaction summary로 줄어드는 방식을 이해한다.

### Step 6. 자동화 런타임 읽기

읽을 파일:

- `internal/schedule/store.go`
- `internal/ops/manager.go`
- `internal/gateway/runtime_run_bootstrap.go`
- `internal/gateway/runtime_run_execute.go`
- `internal/project/project_runner.go`
- `internal/tarsserver/dashboard.go`
- `internal/browser/playwright_exec.go`

확인 포인트:

- 자연어 일정이 cron payload로 어떻게 바뀌는지 확인한다.
- cleanup이 즉시 삭제가 아니라 approval 기반인지 확인한다.
- gateway run 상태가 accepted -> running -> completed/failed/canceled 로 어떻게 바뀌는지 본다.
- project autopilot이 board/state/activity를 읽어 todo -> review -> done 순서로 진행하는지 확인한다.
- dashboard가 별도 SPA가 아니라 서버 렌더링 HTML + SSE refresh 구조인지 확인한다.
- 브라우저 자동화가 Go 내부가 아니라 Node Playwright 실행기로 위임되는지 확인한다.

### Step 7. 운영 경계와 provider adapter 읽기

읽을 파일:

- `cmd/tars/service_main.go`
- `internal/browserrelay/server.go`
- `internal/auth/token.go`
- `internal/auth/codex_oauth.go`
- `internal/llm/openai_compat_client.go`
- `internal/llm/anthropic.go`
- `internal/llm/gemini_native.go`
- `internal/llm/openai_codex_client.go`

확인 포인트:

- `service` 명령이 자체 daemon 이 아니라 현재 CLI를 LaunchAgent로 감싸는 래퍼인지 확인한다.
- browser relay가 API bearer token이 아니라 별도 relay token과 loopback/origin 정책으로 보호되는지 본다.
- 일반 OAuth token 해석과 Codex credential refresh가 왜 분리돼 있는지 이해한다.
- provider마다 transport shape가 `/chat/completions`, `/v1/messages`, `generateContent`, `responses`로 다르다는 점을 확인한다.

---

## 자가검증 체크리스트

- [ ] `cmd/tars/main.go`에서 실제 비즈니스 로직이 거의 없다는 점을 설명할 수 있다.
- [ ] `/v1/chat` 요청이 지나가는 핵심 함수 3개 이상을 파일 경로와 함께 말할 수 있다.
- [ ] `doctor`가 단순 lint가 아니라 starter workspace/BYOK/gateway executor를 함께 보는 이유를 설명할 수 있다.
- [ ] 워크스페이스가 단순 캐시가 아니라 프롬프트 입력원이라는 점을 설명할 수 있다.
- [ ] skill, plugin, MCP가 어디서 묶이는지 설명할 수 있다.
- [ ] project autopilot, schedule, gateway run, browser 실행이 어떤 파일 경로를 통해 이어지는지 설명할 수 있다.
- [ ] provider adapter와 host boundary adapter를 왜 별도 계층으로 분리했는지 설명할 수 있다.

---

## 확장 미션

1. `internal/tarsserver/main_serve_api.go`에서 cron, gateway, telegram 관련 백그라운드 런타임이 어떻게 시작되는지 추가로 정리한다.
2. `internal/tool/*`를 읽고 기본 툴셋과 고위험 툴셋의 경계를 표로 정리한다.
3. `internal/tarsclient/commands_*`를 읽고 slash command를 기능별로 분류한다.
