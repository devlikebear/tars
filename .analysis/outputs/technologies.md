# 기술 스택

## 언어와 빌드

- Go `1.25.6`: `go.mod` 기준 단일 모듈 저장소다.
- Makefile 기반 실행 흐름: `make dev-serve`, `make dev-tars`, `make build-bins` 같은 개발 커맨드를 제공한다.

## CLI와 UI

- `spf13/cobra`: `cmd/tars/*.go`에서 CLI 명령 트리를 만든다.
- `charmbracelet/bubbletea`, `bubbles`, `lipgloss`: `internal/tarsclient` TUI를 구성한다.

## HTTP와 스트리밍

- `net/http`: 서버와 SDK가 모두 표준 라이브러리 위에 있다.
- SSE: `pkg/tarsclient/client.go`의 `StreamSSE`와 `StreamChat`이 스트리밍 채팅과 이벤트 구독을 담당한다.
- `html/template`: `internal/tarsserver/dashboard.go`가 프로젝트 dashboard를 서버 렌더링한다.
- `gorilla/websocket`: `internal/browserrelay/server.go`가 브라우저 확장과 CDP relay를 유지한다.

## 인증과 보안 경계

- SHA-256 + constant-time compare: `internal/serverauth/middleware.go`에서 bearer token 비교에 사용한다.
- 경로 기반 권한 분기: admin path, skip path, loopback 판정을 별도 matcher로 관리한다.

## LLM 계층

- 공통 인터페이스: `internal/llm/provider.go`
- 공급자 지원: OpenAI 호환, Gemini OpenAI 호환, Gemini Native, Anthropic, OpenAI Codex OAuth
- 인증 보조: `internal/auth/*`
- OpenAI-compatible Chat Completions: `internal/llm/openai_compat_client.go`
- Anthropic Messages API: `internal/llm/anthropic.go`
- Google GenAI SDK: `google.golang.org/genai`, `internal/llm/gemini_native.go`
- OpenAI Codex Responses API: `internal/llm/openai_codex_client.go`가 `/codex/responses` 형식, tool name 변환, refresh retry를 처리한다.
- 실행 전 진단: `cmd/tars/doctor_main.go`가 API key, Claude CLI, gateway executor 경로를 점검한다.

## 자동화와 스케줄링

- `robfig/cron/v3`: cron 표현식 파싱과 스케줄 실행
- 자체 heartbeat, schedule, ops, gateway 런타임이 Go 루프로 구현되어 있다.
- 자연어 일정 해석: `internal/scheduleexpr/*`
- 프로젝트 오토파일럿: `internal/project/project_runner.go`가 board/activity/state 문서를 기반으로 주기 루프를 돈다.

## 확장 시스템

- skill 로딩: `internal/skill/*`
- plugin 로딩: `internal/plugin/*`
- 파일 감시: `fsnotify`
- MCP 서버 통신: `internal/mcp/client.go`
- plugin manifest: JSON 기반 `tars.plugin.json`

## 브라우저 자동화

- Playwright 기반 런타임: `internal/browser/service.go`
- Node subprocess bridge: `internal/browser/playwright_exec.go`가 `scripts/playwright_browser_runner.mjs`를 호출한다.
- 브라우저 확장 relay: `internal/browserrelay/server.go`가 loopback token + WebSocket relay를 제공한다.
- Node.js는 브라우저 런타임 설치 시에만 추가로 필요하다.

## 호스트 운영 통합

- `launchctl`: `cmd/tars/service_main.go`가 macOS LaunchAgent plist를 설치하고 관리한다.
- 표준 라이브러리 `os/exec`: `launchctl` 같은 host command를 직접 호출한다.
- in-memory OTP coordinator: `internal/approval/otp.go`가 외부 채널 입력을 브라우저 로그인 흐름과 연결한다.
- credential file adapter: `internal/auth/codex_oauth.go`가 `~/.codex/auth.json` 류의 파일을 읽고 atomic rename 으로 갱신한다.

## 로깅과 설정

- `rs/zerolog`: 구조화 로그
- `gopkg.in/yaml.v3`: 설정 파일 로딩
- `.env` + `.env.secret` + YAML + 환경 변수 병합
- `gh` CLI: 프로젝트 task dispatch 전에 GitHub Flow 전제조건으로 인증 상태를 확인한다.

## 저장 방식

- 세션 transcript: JSONL
- 프로젝트 문서: Markdown frontmatter 스타일 문서
- 프로젝트 활동 로그: `ACTIVITY.jsonl`
- 오토파일럿 상태: `AUTOPILOT.json`
- 사용량/제한: JSON
- 연구 보고서: Markdown + `summary.jsonl`

## 이 저장소의 기술적 성격

TARS는 프레임워크 중심 앱이라기보다 "Go 표준 라이브러리 + 작은 라이브러리 조합"에 가깝다. 핵심 복잡도는 프레임워크가 아니라 로컬 워크스페이스 파일, 프롬프트 조립, 툴 주입, 백그라운드 런타임 관리에서 나온다.
