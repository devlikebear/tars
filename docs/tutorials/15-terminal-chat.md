# Step 15. One-shot CLI와 웹 콘솔

> 학습 목표: `/v1/chat` SSE 응답을 CLI에서 한 번 실행하고, 실제 대화 표면은 웹 콘솔로 분리하는 구조 이해

## 왜 대화형 TUI가 제거되었나

초기 설계에는 `tars chat` 같은 터미널 REPL이 있었습니다. 하지만 최신 TARS는 다음처럼 역할을 나눕니다.

- `tars` root 명령: 기본 브라우저로 `/console` 열기
- `tars --message "..."`: 한 번 메시지를 보내고 SSE 응답을 stdout에 출력
- `/console`: 세션 목록, Chat, Memory, Ops, Pulse, Reflection, Config 등을 다루는 주 UI

이렇게 하면 CLI는 자동화와 스크립트에 집중하고, 반복 대화와 운영 UI는 콘솔로 모을 수 있습니다.

## 서버 API 복습

```
POST /v1/chat
Content-Type: application/json

{"session_id": "abc123", "message": "안녕하세요"}
```

응답은 JSON 한 덩어리가 아니라 SSE(Server-Sent Events) 스트림입니다.

```
data: {"type":"status","session_id":"abc123","message":"connected"}
data: {"type":"delta","session_id":"abc123","text":"안녕"}
data: {"type":"delta","session_id":"abc123","text":"하세요!"}
data: {"type":"done","session_id":"abc123","usage":{"input_tokens":15,"output_tokens":8}}
```

핵심:
- `delta` 이벤트가 토큰 단위로 반복 전송됨
- `status` 이벤트는 도구 호출 전후 상태를 보여줌
- `done` 이벤트가 세션 ID와 사용량 정보를 마무리함

## 실습

### 15-1. Root command에 client flags 붙이기

**`cmd/tars/main.go`**

```go
clientOpts := defaultClientOptions()
cmd := &cobra.Command{
    Use: "tars",
    RunE: func(cmd *cobra.Command, _ []string) error {
        if clientOpts.message != "" {
            return runClientCommand(cmd.Context(), stdin, stdout, stderr, clientOpts)
        }
        return runConsoleCommand(cmd.Context(), stdout, stderr, clientOpts)
    },
}
bindClientFlags(cmd, &clientOpts)
```

root 명령에 `--message`, `--session`, `--server-url`, `--api-token`을 붙이면 별도 `chat` 서브커맨드 없이도 one-shot 채팅을 지원할 수 있습니다.

### 15-2. 콘솔 열기

**`cmd/tars/console_main.go`**

```go
func runConsoleCommand(ctx context.Context, stdout, stderr io.Writer, opts clientOptions) error {
    target := buildConsoleURL(opts.serverURL)
    _ = openConsoleURL(ctx, target)
    _, err := fmt.Fprintf(stdout, "Open the console: %s\n", target)
    return err
}
```

OS별로 `open`, `xdg-open`, `rundll32`를 사용합니다. 브라우저 열기에 실패해도 URL은 stdout에 남기므로 사용자가 직접 열 수 있습니다.

### 15-3. One-shot 메시지 전송

**`internal/tarsclient/client_main.go`**

```go
func Run(ctx context.Context, _ io.Reader, stdout, stderr io.Writer, opts Options) error {
    chat := chatClient{serverURL: opts.ServerURL, apiToken: opts.APIToken}
    session := strings.TrimSpace(opts.SessionID)
    if strings.TrimSpace(opts.Message) != "" {
        res, err := sendMessage(ctx, chat, session, opts.Message, opts.Verbose, opts.Verbose, stdout, stderr)
        if err != nil {
            return err
        }
        if res.SessionID != "" {
            fmt.Fprintf(stderr, "session=%s\n", res.SessionID)
        }
        return nil
    }
    return fmt.Errorf("interactive terminal UI has been removed; use the web console at /console or one-shot CLI commands")
}
```

메시지가 있으면 곧바로 `/v1/chat`에 연결합니다. 메시지가 없으면 대화형 TUI를 시작하지 않고 콘솔 사용을 안내합니다.

### 15-4. SSE 스트림 파싱

**`pkg/tarsclient.Client.StreamChat`**

```go
err := c.StreamSSE(ctx, http.MethodPost, "/v1/chat", req, func(payload []byte) error {
    var evt ChatEvent
    if err := json.Unmarshal(payload, &evt); err != nil {
        return fmt.Errorf("decode sse event: %w", err)
    }
    switch evt.Type {
    case "status":
        onStatus(evt)
    case "delta":
        result.Assistant += evt.Text
        onDelta(evt.Text)
    case "error":
        return errors.New(strings.TrimSpace(evt.Error))
    case "done":
        result.SessionID = strings.TrimSpace(evt.SessionID)
    }
    return nil
})
```

CLI는 `delta`만 stdout에 이어 붙이고, `--verbose`일 때 status 이벤트를 stderr로 출력합니다.

## 실행

```bash
# 서버
tars serve

# 콘솔 열기
tars

# one-shot chat
tars --message "오늘 작업 요약해줘"

# 기존 세션으로 이어서 보내기
tars --session <session_id> --message "방금 내용에서 TODO만 뽑아줘"
```

## 체크포인트

- [x] `tars`가 `/console` URL을 열고 출력한다
- [x] `tars --message`가 SSE delta를 stdout에 출력한다
- [x] `--session`으로 기존 세션을 이어갈 수 있다
- [x] 메시지 없이 client runner를 호출하면 레거시 TUI 제거 안내를 반환한다

## 다음 단계

다음 단계는 운영 작업을 사람이 승인한 뒤 실행하는 흐름입니다. Step 19에서는 cleanup plan과 approval API를 다룹니다.
