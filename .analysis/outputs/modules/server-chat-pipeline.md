# 모듈: 서버와 채팅 파이프라인

## 핵심 파일

- `internal/tarsserver/main.go`
- `internal/tarsserver/main_cli.go`
- `internal/tarsserver/main_bootstrap.go`
- `internal/tarsserver/main_serve_api.go`
- `internal/tarsserver/console.go`
- `internal/tarsserver/handler_chat.go`
- `internal/tarsserver/handler_chat_context.go`
- `internal/tarsserver/handler_chat_policy.go`
- `internal/tarsserver/handler_chat_execution.go`
- `internal/tarsserver/handler_chat_stream.go`
- `internal/tarsserver/handler_transport_helpers.go`
- `internal/tarsserver/helpers_memory.go`
- `internal/tarsserver/handlers.go`

## 역할

이 모듈은 TARS 서버의 중심이다. 설정을 읽고, 워크스페이스를 준비하고, HTTP API와 웹 콘솔을 열고, `/v1/chat` 요청을 실제 agent 실행으로 연결한다. 현재 헤드에서는 chat policy와 SSE streaming이 별도 파일로 분리됐고, built-in plugin HTTP route도 이 레이어를 통해 서버에 붙는다.

## 내부 구조

- 부트스트랩: 설정, 워크스페이스, 세션 저장소, 사용량 추적기, LLM 클라이언트 생성
- API 구성: 핸들러 등록, 콘솔 핸들러/dev proxy, cron/watchdog/gateway/background 시작
- 채팅 준비: 세션/프로젝트/프롬프트/툴 레지스트리/툴 스키마 결정
- 채팅 실행: `agent.Loop.Run`으로 LLM + tool-calling 수행
- 스트리밍: status/delta/done SSE 이벤트 직렬화
- 후처리: transcript append, session touch, compaction, semantic memory 훅 실행

## 초보자가 놓치기 쉬운 점

- `/v1/chat`는 단순히 LLM만 부르는 핸들러가 아니다. 세션 저장, relevant memory, project policy, skill prompt, tool allowlist가 한 번에 결정된다.
- tool 주입은 request마다 달라질 수 있다. 기본 툴셋, 프로젝트 제한, auth role, high-risk user 허용 여부가 모두 영향을 준다.
- semantic memory는 prompt build와 tool registry 양쪽에 동시에 개입한다.
- 첨부파일은 MIME type에 따라 text/image/pdf block으로 변환돼 LLM 입력에 섞인다.
- `/console`은 별도 프런트엔드 프로젝트지만, 서버는 임베드 자산 서빙과 dev proxy를 둘 다 지원한다.
- `handler_transport_helpers.go`의 공통 JSON body 제한이 API surface 전체의 입력 안전장치 역할을 한다.

## 디버깅 포인트

- 세션 문제: `prepareChatRunState`와 `internal/session/session.go`
- 프롬프트 문제: `prepareChatContextDetailsWithExtensions`, `internal/prompt/builder.go`, `internal/prompt/memory_retrieval.go`
- 툴 문제: `buildChatToolRegistry`, `resolveInjectedToolSchemas`, `internal/tool/tool.go`
- semantic memory 문제: `helpers_memory.go`, `buildSemanticMemoryService`
- 스트리밍 문제: `handler_chat_stream.go`와 `pkg/tarsclient/client.go`
- 콘솔 문제: `console.go`, `legacy_dashboard.go`, `frontend/console/*`
