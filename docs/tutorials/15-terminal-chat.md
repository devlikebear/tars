# Step 15. 터미널 채팅 클라이언트

> 학습 목표: SSE 스트리밍 응답을 파싱하는 대화형 REPL 클라이언트 구현

## 왜 채팅 클라이언트가 필요한가

Phase 1~4까지 우리는 서버를 만들었습니다. `/v1/chat` 엔드포인트가 있고, LLM이 연결되어 있고, 도구도 있습니다. 그런데 **대화할 방법이 없습니다.** curl로 POST를 보낼 수는 있지만, SSE 스트리밍 응답을 실시간으로 읽기엔 불편합니다.

```bash
# 이렇게 해야 했다
curl -N -X POST http://localhost:8080/v1/chat \
  -H "Content-Type: application/json" \
  -d '{"message":"안녕"}'
```

터미널 채팅 클라이언트가 있으면:
- 자연어로 AI와 대화할 수 있다
- 도구 호출 결과를 바로 확인할 수 있다
- 세션을 유지하면서 연속 대화가 가능하다
- 앞으로 "프로젝트 만들어줘"같은 자연어 명령의 기반이 된다

## 서버 API 복습

Step 4에서 구현한 채팅 API:

```
POST /v1/chat
Content-Type: application/json

{"session_id": "abc123", "message": "안녕하세요"}
```

응답은 JSON이 아니라 **SSE(Server-Sent Events)** 스트림:

```
HTTP/1.1 200 OK
Content-Type: text/event-stream
X-Session-ID: abc123

event: status
data: {"session_id":"abc123","status":"stream_open","message":"connected"}

event: delta
data: {"session_id":"abc123","text":"안녕"}

event: delta
data: {"session_id":"abc123","text":"하세요!"}

event: done
data: {"session_id":"abc123","usage":{"input_tokens":15,"output_tokens":8}}
```

핵심:
- `event:` 줄이 이벤트 타입, `data:` 줄이 JSON 페이로드
- `delta` 이벤트가 토큰 단위로 반복 전송됨
- `done` 이벤트로 스트림 종료
- `X-Session-ID` 헤더로 세션 ID 전달

## 실습

### 15-1. Cobra 서브커맨드 등록

**`cmd/tars/main.go`** — `chat` 커맨드 추가:

```go
cmd.AddCommand(newChatCommand(stdout, os.Stderr))
```

실행: `tars chat --server http://localhost:8080`

### 15-2. REPL 루프

**`cmd/tars/chat.go`**

가장 단순한 대화형 루프: stdin에서 한 줄 읽고 → 서버에 전송 → 응답 출력 → 반복.

```go
func runChat(stdout, stderr io.Writer, serverURL, sessionID string) error {
    scanner := bufio.NewScanner(os.Stdin)
    scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

    for {
        fmt.Fprint(stdout, "you> ")
        if !scanner.Scan() {
            break
        }
        input := strings.TrimSpace(scanner.Text())
        if input == "" {
            continue
        }

        newSID, err := sendMessage(stdout, serverURL, sessionID, input)
        if err != nil {
            fmt.Fprintf(stderr, "error: %v\n", err)
            continue
        }
        if newSID != "" {
            sessionID = newSID
        }
    }
    return scanner.Err()
}
```

**설계 포인트:**
- `bufio.Scanner` 기본 버퍼는 64KB. 긴 코드를 붙여넣을 수 있도록 1MB로 확장
- `sessionID`를 루프 밖에서 유지 — 첫 응답의 `X-Session-ID`를 저장하면 이후 대화가 같은 세션에서 이어짐
- 에러가 나도 루프가 끊기지 않음 — 서버 재시작해도 채팅 계속 가능

### 15-3. SSE 스트림 파싱

서버 응답은 일반 JSON이 아니라 SSE 형식입니다. 별도 라이브러리 없이 줄 단위로 파싱합니다:

```go
func sendMessage(stdout io.Writer, serverURL, sessionID, message string) (string, error) {
    body := map[string]string{"session_id": sessionID, "message": message}
    payload, _ := json.Marshal(body)

    resp, err := http.Post(serverURL+"/v1/chat", "application/json",
        strings.NewReader(string(payload)))
    if err != nil {
        return "", fmt.Errorf("connection failed: %w", err)
    }
    defer resp.Body.Close()

    newSessionID := resp.Header.Get("X-Session-ID")

    // SSE 파싱: "event: xxx\ndata: yyy\n\n" 패턴
    fmt.Fprint(stdout, "ai> ")
    scanner := bufio.NewScanner(resp.Body)
    var eventType string

    for scanner.Scan() {
        line := scanner.Text()
        if strings.HasPrefix(line, "event: ") {
            eventType = strings.TrimPrefix(line, "event: ")
        } else if strings.HasPrefix(line, "data: ") {
            data := strings.TrimPrefix(line, "data: ")
            handleEvent(stdout, eventType, data)
            eventType = ""
        }
    }
    return newSessionID, nil
}
```

**SSE 파싱 규칙:**
1. `event:` 줄 → 이벤트 타입 저장
2. `data:` 줄 → 저장된 타입과 함께 처리
3. 빈 줄 → 이벤트 구분자 (무시해도 됨, 이미 data에서 처리했으므로)

### 15-4. 이벤트별 처리

```go
func handleEvent(stdout io.Writer, eventType, data string) {
    switch eventType {
    case "delta":
        var d struct{ Text string `json:"text"` }
        if json.Unmarshal([]byte(data), &d) == nil {
            fmt.Fprint(stdout, d.Text) // 줄바꿈 없이 이어붙임
        }
    case "error":
        var e struct{ Error string `json:"error"` }
        if json.Unmarshal([]byte(data), &e) == nil {
            fmt.Fprintf(stdout, "\n[error] %s", e.Error)
        }
    case "done":
        // 토큰 사용량 표시 (선택적)
    case "status":
        // stream_open — 무시
    }
}
```

**핵심:** `delta` 이벤트의 텍스트를 `fmt.Fprint`로 출력합니다. `Println`이 아닙니다.
토큰이 하나씩 오기 때문에 줄바꿈 없이 이어붙여야 자연스러운 스트리밍 출력이 됩니다:

```
you> 안녕하세요
ai> 안녕하세요! 무엇을 도와드릴까요?
[tokens: in=15 out=12]
```

### 15-5. 세션 명령어

대화 중 사용할 수 있는 몇 가지 내장 명령어:

```go
if input == "/quit" || input == "/exit" {
    fmt.Fprintln(stdout, "bye!")
    return nil
}
if input == "/session" {
    fmt.Fprintf(stdout, "session: %s\n", sessionID)
    continue
}
if input == "/new" {
    sessionID = ""
    fmt.Fprintln(stdout, "(new session)")
    continue
}
```

- `/session` — 현재 세션 ID 확인
- `/new` — 세션 초기화 (다음 메시지에서 새 세션 생성)
- `/quit` — 종료

## 전체 구조

```
cmd/tars/
├── main.go          ← newChatCommand 등록
├── chat.go          ← REPL + SSE 파싱
├── serve.go
└── skill_main.go
```

데이터 흐름:

```
stdin → bufio.Scanner
    → "you> " 프롬프트
    → http.Post /v1/chat (JSON body)
    → SSE 스트림 수신
        → event: delta → fmt.Fprint (실시간 출력)
        → event: done  → 토큰 표시, 루프 복귀
    → "ai> ..." 출력 완료
    → 다음 "you> " 대기
```

## 체크포인트

- [ ] `tars chat`으로 서버에 연결, 대화가 가능하다
- [ ] LLM 응답이 스트리밍으로 한 글자씩 출력된다
- [ ] 도구 호출 결과가 대화에 반영된다
- [ ] `/session`으로 세션 ID 확인, `/new`로 새 세션 시작

## 다음 단계

터미널에서 AI와 대화할 수 있게 되었습니다. Step 16에서는 프로젝트 Store를 만들고, 이 채팅 인터페이스를 통해 "프로젝트 만들어줘"같은 자연어 명령으로 프로젝트를 관리하는 기반을 만듭니다.
