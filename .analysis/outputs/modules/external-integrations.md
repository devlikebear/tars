# 모듈: 호스트 서비스와 외부 경계

## 핵심 파일

- `cmd/tars/service_main.go`
- `internal/browserrelay/server.go`
- `internal/approval/otp.go`
- `internal/auth/provider_credentials.go`
- `internal/auth/codex_oauth.go`
- `internal/auth/codex_refresh_store.go`
- `internal/auth/token.go`
- `internal/llm/provider.go`
- `internal/llm/model_lister.go`
- `internal/llm/openai_compat_client.go`
- `internal/llm/openai_codex_client.go`
- `internal/llm/gemini_native.go`
- `internal/llm/anthropic.go`

## 역할

이 모듈은 TARS가 로컬 Go 프로세스 바깥 세계와 맞닿는 경계를 정리한다. macOS launchd 서비스 설치, 브라우저 확장과 CDP 클라이언트 사이의 relay, 사람이 입력하는 OTP 대기 흐름, 외부 LLM provider용 credential/transport adapter가 여기 묶인다.

## Service 명령 흐름

`cmd/tars/service_main.go`는 `tars serve`를 macOS LaunchAgent로 설치하고 시작/중지/상태 조회하는 얇은 운영 명령이다.

- `install`: 먼저 `buildDoctorReport`로 현재 workspace/config 환경이 실제로 실행 가능한지 검사한다.
- 진단이 통과하면 현재 `tars` 실행 파일 경로, workspace, config, 로그 경로를 LaunchAgent plist로 렌더링한다.
- `start`: `launchctl bootstrap` + `kickstart`로 프로세스를 올린다.
- `stop`: `launchctl bootout`으로 unload 한다.
- `status`: plist 설치 여부와 `launchctl print` 결과를 함께 보여 준다.

## Browser relay 구조

`internal/browserrelay/server.go`는 브라우저 확장과 로컬 CDP 소비자를 이어 주는 relay 서버다.

- 모든 HTTP/WebSocket 진입점은 loopback remote address인지 먼저 확인한다.
- relay token이 헤더 또는 선택적 query parameter로 맞아야 `/json`, `/extension`, `/cdp` 경로를 쓸 수 있다.
- query token 허용 시 status payload에 warning을 같이 싣는다.
- 확장 연결은 추가로 origin allowlist 검사를 통과해야 한다.
- relay는 extension 쪽과 ping/pong keepalive를 유지하고 stale pong이면 연결을 닫는다.

## Provider credential 구조

이번 증분의 핵심은 `internal/auth/provider_credentials.go`다.

- generic provider는 `api-key` 또는 `oauth` 모드로 token source를 고른다.
- `openai-codex`는 strategy table에서 별도 resolver/refresh handler를 갖는다.
- `codex_oauth.go`는 access token, refresh token, account id, source path를 함께 다루는 credential 저장소에 가깝다.
- `codex_refresh_store.go`는 macOS에서 refresh token을 keychain에 저장할 수 있다.

## Provider transport adapter 비교

- `internal/llm/openai_compat_client.go`: `/chat/completions` 계열 공용 adapter
- `internal/llm/anthropic.go`: `/v1/messages` 전용 adapter
- `internal/llm/gemini_native.go`: Gemini Native REST adapter
- `internal/llm/openai_codex_client.go`: Responses API + refresh retry adapter
- `internal/llm/model_lister.go`: provider live API로 model id를 가져오는 조회 전용 adapter

## 초보자가 놓치기 쉬운 점

- `service` 명령은 macOS 전용이며, 설치 전에 `doctor` 통과가 강제된다.
- 브라우저 relay의 인증은 API 서버 bearer token과 별개로 relay token을 한 번 더 사용한다.
- query token 허용은 여전히 옵션이며, warning 추가가 위험 자체를 없애 주지는 않는다.
- OAuth provider마다 token source가 다르며, OpenAI Codex는 refresh token 과 account ID까지 함께 관리한다.
- provider들은 같은 인터페이스를 공유하지만 request URL, streaming parser, model listing 방식은 제각각이다.
