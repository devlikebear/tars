# 모듈: 클라이언트와 TUI

## 핵심 파일

- `internal/tarsclient/client_main.go`
- `internal/tarsclient/chat.go`
- `internal/tarsclient/commands_runtime.go`
- `internal/tarsclient/runtime_ops.go`
- `internal/tarsclient/runtime_usage.go`
- `pkg/tarsclient/client.go`

## 역할

이 모듈은 CLI 사용자가 TARS와 상호작용하는 표면이다. 현재 헤드에서는 interactive Bubble Tea TUI가 제거됐고, one-shot 채팅과 로컬 런타임 제어 커맨드, 그리고 다른 Go 코드에서도 재사용할 수 있는 HTTP/SSE SDK가 중심이다.

## 호출 흐름

```text
cmd/tars/client_main.go
  -> internal/tarsclient.Run
  -> one-shot sendMessage or runtime command helpers
  -> pkg/tarsclient.Client.StreamChat
  -> /v1/chat SSE 수신
```

## 중요한 관찰

- 메시지 모드가 아니면 `Run`은 오류를 반환하고 웹 콘솔 사용을 안내한다.
- one-shot 채팅도 SSE status 이벤트를 읽어 `stderr`에 trace를 찍을 수 있다.
- 런타임 조회/운영 커맨드는 `internal/tarsclient/commands_*`와 `runtime_*` 조합으로 API를 얇게 감싼다.
- SDK 중심 파일은 여전히 `pkg/tarsclient/client.go`다.

## 읽는 팁

- 채팅 스트림 처리와 상태 라벨링은 `internal/tarsclient/chat.go`
- 런타임 커맨드 표면은 `internal/tarsclient/commands_*`
- HTTP 세부사항은 `pkg/tarsclient/client.go`

CLI를 확장하려면 `internal/tarsclient`를 보면 되고, API 재사용 포인트를 찾는다면 `pkg/tarsclient`를 보면 된다.
