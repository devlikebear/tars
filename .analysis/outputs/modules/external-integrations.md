# 모듈: 호스트 서비스와 외부 경계

## 핵심 파일

- `cmd/tars/service_main.go`
- `internal/browserrelay/server.go`
- `internal/approval/otp.go`
- `internal/auth/codex_oauth.go`
- `internal/auth/token.go`
- `internal/llm/anthropic.go`
- `internal/llm/openai_compat_client.go`
- `internal/llm/openai_codex_client.go`
- `internal/llm/gemini_native.go`

## 역할

이 모듈은 TARS가 로컬 Go 프로세스 바깥 세계와 맞닿는 경계를 정리한다. macOS launchd 서비스 설치, 브라우저 확장과 CDP 클라이언트 사이의 relay, 사람이 입력하는 OTP 대기 흐름, 외부 LLM provider용 OAuth credential과 transport adapter가 여기 묶인다.

## Service 명령 흐름

`cmd/tars/service_main.go`는 `tars serve`를 macOS LaunchAgent로 설치하고 시작/중지/상태 조회하는 얇은 운영 명령이다.

- `install`: 먼저 `buildDoctorReport`로 현재 workspace/config 환경이 실제로 실행 가능한지 검사한다.
- 진단이 통과하면 현재 `tars` 실행 파일 경로, workspace, config, 로그 경로를 LaunchAgent plist로 렌더링한다.
- `start`: `launchctl bootstrap` + `kickstart`로 프로세스를 올린다.
- `stop`: `launchctl bootout`으로 unload 한다.
- `status`: plist 설치 여부와 `launchctl print` 결과를 함께 보여 준다.

핵심은 서비스 관리가 별도 daemon 구현이 아니라 "현재 CLI를 launchd에 다시 연결하는 래퍼"라는 점이다.

## Browser relay 구조

`internal/browserrelay/server.go`는 브라우저 확장과 로컬 CDP 소비자를 이어 주는 relay 서버다.

- 모든 HTTP/WebSocket 진입점은 loopback remote address인지 먼저 확인한다.
- relay token이 헤더 또는 선택적 query parameter로 맞아야 `/json`, `/extension`, `/cdp` 경로를 쓸 수 있다.
- 확장 연결은 추가로 origin allowlist 검사를 통과해야 한다.
- relay는 extension 쪽과 ping/pong keepalive를 유지하고, stale pong이면 연결을 닫는다.
- CDP 요청은 pending map에 넣고, extension 응답이 늦거나 끊기면 timeout/error를 돌려준다.

즉, 브라우저 relay는 단순 프록시가 아니라 "loopback + relay token + origin allowlist + keepalive" 4중 경계 위에서 동작한다.

## OTP 승인 흐름

`internal/approval/otp.go`의 `OTPManager`는 chat ID별로 하나의 OTP 대기 슬롯을 가진다.

- `Request`: timeout 기본값은 300초이며, 결과 채널을 기다린다.
- `Consume`: 입력된 코드를 한 번 전달하고 pending entry를 제거한다.
- `HasPending`: 현재 chat ID에 열린 OTP 요청이 있는지 확인한다.

이 상태는 메모리 전용이라 서버 재시작 후 복구되지 않는다. 대신 만료 시간과 explicit consume으로 오래된 요청을 정리한다.

## OAuth credential 해석

`internal/auth/token.go`와 `internal/auth/codex_oauth.go`는 provider별 credential source를 정리한다.

- 일반 provider 경로에서 `ResolveToken`은 `api-key` 또는 `oauth` 모드를 고르고, Claude Code나 Google Antigravity 토큰을 env 또는 표준 auth 파일에서 읽는다.
- OpenAI Codex는 access token만으로 부족해서 `refresh_token`, `account_id`, source path를 함께 다루는 별도 `CodexCredential` 구조체를 사용한다.
- Codex credential은 env override가 없으면 `CODEX_HOME` 또는 `~/.codex/auth.json`을 읽는다.
- access token에 account ID가 없으면 JWT payload에서 `chatgpt_account_id` claim을 추출한다.
- refresh가 성공하면 auth file을 temp file + rename 방식으로 안전하게 교체한다.

즉, `internal/auth/token.go`는 "문자열 토큰 반환기"이고, `internal/auth/codex_oauth.go`는 "갱신 가능한 credential 저장소"에 더 가깝다.

## OpenAI Codex client 특이점

`internal/llm/openai_codex_client.go`는 다른 provider보다 더 두꺼운 adapter다.

- 클라이언트 생성 시 credential 해석이 즉시 수행돼 시작 단계에서 설정 오류를 빨리 드러낸다.
- 요청은 `/codex/responses` endpoint로 가고 `OpenAI-Beta`, `originator`, `chatgpt-account-id` 헤더를 추가한다.
- Responses API 요구사항에 맞게 tool 이름을 sanitize/dedupe 하고, assistant/tool turn을 별도 item 형식으로 변환한다.
- 401/403이 오면 refresh token으로 credential 갱신을 시도하고, 성공하면 메모리 override credential도 교체한다.
- non-streaming 호출이 거부되면 streaming fallback을 한 번 더 시도한다.

즉, Codex 경로는 단순 bearer forwarding이 아니라 "credential refresh + request shape translation + fallback retry"를 함께 가진다.

## Provider transport adapter 비교

`internal/llm/*`의 provider client는 같은 `llm.Client` 인터페이스를 구현하지만, wire format은 세 갈래로 나뉜다.

- `internal/llm/openai_compat_client.go`: Bifrost, OpenAI, Gemini OpenAI-compatible endpoint를 `/chat/completions` 한 가지 패턴으로 감싼다. SSE/JSON 파서도 이 형식 하나를 기준으로 재사용한다.
- `internal/llm/anthropic.go`: `/v1/messages` 전용 client다. system message를 별도 field로 올리고, prompt caching beta header와 thinking budget, tool_use content block parsing을 함께 처리한다.
- `internal/llm/gemini_native.go`: `google.golang.org/genai` SDK 위에서 동작한다. base URL에서 API version을 분리하고, 첫 호출 전에 model이 `generateContent`를 지원하는지 preflight 검사한다.

즉, provider 레이어는 한 구현을 if/switch 로 늘리는 구조가 아니라 "공통 인터페이스 + provider별 transport adapter" 집합에 가깝다.

## 초보자가 놓치기 쉬운 점

- `service` 명령은 macOS 전용이며, 설치 전에 `doctor` 통과가 강제된다.
- 브라우저 relay의 인증은 API 서버 bearer token과 별개로 relay token을 한 번 더 사용한다.
- OTP 승인은 파일 저장이 아니라 프로세스 메모리에서만 대기한다.
- OAuth provider마다 token source가 다르며, OpenAI Codex는 refresh token 과 account ID까지 함께 관리한다.
- LLM provider들은 같은 인터페이스를 공유하지만 request URL, streaming parser, tool-call shape는 provider마다 다르다.

## 디버깅 포인트

- 서비스 설치/기동 이상: `buildDoctorReport`, `buildServiceLaunchAgentPlist`, `serviceStatus`
- relay 인증 실패: `authorizeRelayRequest`, `relayTokenFromRequest`, `originAllowed`
- relay 끊김/응답 누락: `extensionKeepalive`, `pending`, `sendPendingError`
- OTP 응답 누락: `Request`, `Consume`, `HasPending`
- Codex 인증 이상: `ResolveCodexCredential`, `RefreshCodexCredential`, `ParseCodexAccountIDFromJWT`
- Codex 호출 이상: `resolveOpenAICodexResponsesURL`, `buildOpenAICodexRequestBody`, `parseOpenAICodexSSE`
- Anthropic 호출 이상: `buildChatRequest`, `chatStreamingResponse`, `parseAnthropicContentBlocks`
- OpenAI-compatible 호출 이상: `buildChatRequest`, `chatStreaming`, `chatNonStreaming`
- Gemini Native 호출 이상: `ensureModelSupportsGenerateContent`, `buildGenerateContentConfig`, `chatStreamingResponse`
