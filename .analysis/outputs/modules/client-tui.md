# 모듈: 클라이언트와 TUI

## 핵심 파일

- `internal/tarsclient/client_main.go`
- `internal/tarsclient/app_model.go`
- `internal/tarsclient/app_update.go`
- `internal/tarsclient/chat.go`
- `internal/tarsclient/runtime.go`
- `pkg/tarsclient/client.go`

## 역할

이 모듈은 터미널 사용자가 TARS와 상호작용하는 표면이다. 하나는 Bubble Tea 기반 TUI이고, 다른 하나는 다른 Go 코드에서도 재사용할 수 있는 HTTP/SSE SDK다.

## 호출 흐름

```text
cmd/tars/client_main.go
  -> internal/tarsclient.Run
  -> runTUI or one-shot sendMessage
  -> pkg/tarsclient.Client.StreamChat
  -> /v1/chat SSE 수신
```

## 중요한 관찰

- slash command는 로컬에서 먼저 해석하고, 채팅 메시지는 서버로 스트리밍한다.
- 상태 라인과 채팅 라인이 분리되어 있어 tool trace와 assistant 답변이 서로 다른 화면 영역으로 들어간다.
- 세션 ID는 서버가 새로 발급하면 TUI가 즉시 반영한다.

## 읽는 팁

- 화면 상태 변경은 `internal/tarsclient/app_update.go`
- 초기 모델 구조는 `internal/tarsclient/app_model.go`
- HTTP 세부사항은 `pkg/tarsclient/client.go`

UI를 바꾸려면 `internal/tarsclient`를 보면 되고, API 재사용 포인트를 찾는다면 `pkg/tarsclient`를 보면 된다.
