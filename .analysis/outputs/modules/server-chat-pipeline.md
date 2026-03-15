# 모듈: 서버와 채팅 파이프라인

## 핵심 파일

- `internal/tarsserver/main.go`
- `internal/tarsserver/main_bootstrap.go`
- `internal/tarsserver/main_serve_api.go`
- `internal/tarsserver/handler_chat.go`
- `internal/tarsserver/handler_chat_context.go`
- `internal/tarsserver/handler_chat_execution.go`
- `internal/tarsserver/handlers.go`

## 역할

이 모듈은 TARS 서버의 중심이다. 설정을 읽고, 워크스페이스를 준비하고, HTTP API를 열고, `/v1/chat` 요청을 실제 agent 실행으로 연결한다.

## 내부 구조

- 부트스트랩: 설정, 워크스페이스, 세션 저장소, 사용량 추적기, LLM 클라이언트 생성
- API 구성: 핸들러 등록, cron/watchdog/gateway/background 시작
- 채팅 준비: 세션/프로젝트/프롬프트/툴 스키마 결정
- 채팅 실행: `agent.Loop.Run`으로 LLM + tool-calling 수행
- 후처리: transcript append, session touch, 메모리 훅 실행

## 초보자가 놓치기 쉬운 점

- `/v1/chat`는 단순히 LLM만 부르는 핸들러가 아니다. 세션 저장, project policy, skill prompt, tool allowlist가 한 번에 결정된다.
- tool 주입은 request마다 달라질 수 있다. 기본 툴셋, 프로젝트 제한, auth role이 모두 영향을 준다.
- transcript는 매 요청 전에 읽고, 사용자 메시지는 LLM 호출 전에 먼저 저장한다.

## 디버깅 포인트

- 세션 문제: `prepareChatRunState`와 `internal/session/session.go`
- 프롬프트 문제: `prepareChatContextDetailsWithExtensions`와 `internal/prompt/builder.go`
- 툴 문제: `buildChatToolRegistry`, `resolveInjectedToolSchemas`, `internal/tool/tool.go`
- 스트리밍 문제: `chatStreamWriter`와 `pkg/tarsclient/client.go`
