# Step 4. HTTP/SSE 채팅 엔드포인트

> 학습 목표: 지금까지 만든 세션, 프롬프트, 도구를 전부 연결해서 실제 채팅 서버 구현

## 원본 코드 분석 (TARS)

원본은 채팅 파이프라인을 5개 파일로 나눕니다:

| 파일 | 책임 |
|------|------|
| `handler_chat_pipeline.go` | 전체 흐름 오케스트레이션 |
| `handler_chat_context.go` | 세션 resolve, 프롬프트/도구 조립 |
| `handler_chat.go` | LLM 메시지 빌드, 헬퍼 함수 |
| `handler_chat_execution.go` | agent loop 실행, 결과 저장 |
| (여러 파일) | SSE 스트리밍 |

### 전체 흐름

```
POST /v1/chat { session_id, message }
  │
  ├─ 1. 요청 파싱 + 검증
  ├─ 2. 세션 resolve (없으면 생성)
  ├─ 3. transcript에서 이전 대화 읽기
  ├─ 4. 프롬프트 조립 (시스템 프롬프트 + 워크스페이스 파일)
  ├─ 5. LLM 메시지 구성 (system + history + user)
  ├─ 6. 유저 메시지를 transcript에 저장
  ├─ 7. SSE 스트림 시작
  ├─ 8. Agent loop 실행 (LLM 호출 → tool call → 반복)
  ├─ 9. assistant 응답을 transcript에 저장
  └─ 10. SSE done 이벤트
```

### 핵심 타입 (원본 `internal/llm/provider.go`)

```go
type Client interface {
    Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (ChatResponse, error)
}

type ChatMessage struct {
    Role       string     `json:"role"`      // system, user, assistant, tool
    Content    string     `json:"content"`
    ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
    ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ChatOptions struct {
    OnDelta    func(text string)  // SSE 스트리밍 콜백
    Tools      []ToolSchema
    ToolChoice string
}
```

### Agent Loop 핵심 로직 (원본 `internal/agent/loop.go`)

```go
for i := 0; i < maxIters; i++ {
    resp = client.Chat(messages, tools)     // LLM 호출
    if len(resp.ToolCalls) == 0 {
        return resp                          // tool call 없으면 끝
    }
    for each toolCall {
        result = registry.Execute(toolCall)  // 도구 실행
        messages += tool result              // 결과를 대화에 추가
    }
    // 다음 반복에서 LLM이 결과를 보고 다시 판단
}
```

원본에는 추가로 다음이 있습니다 (최소 버전에서는 생략):
- Hook 시스템 (`EventBeforeLLM`, `EventAfterTool` 등)
- 반복 tool call 감지 (무한 루프 방지)
- `autoCorrectExecArguments` (exec 도구 전용 보정)
- `allowedTools` 검증 (보안 레이어)

## 실습

### 4-1. LLM Client 인터페이스 확장

기존 `internal/llm/types.go`에 전체 타입을 추가합니다:

```go
type ToolCall struct {
    ID        string `json:"id"`
    Name      string `json:"name"`
    Arguments string `json:"arguments"`
}

type ChatMessage struct {
    Role       string     `json:"role"`
    Content    string     `json:"content"`
    ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
    ToolCallID string     `json:"tool_call_id,omitempty"`
}

type Client interface {
    Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (ChatResponse, error)
}
```

### 4-2. Agent Loop

LLM 호출 → tool call 확인 → 도구 실행 → 반복하는 핵심 루프입니다.

**`internal/agent/loop.go`** 핵심:

```go
func (l *Loop) Run(ctx, messages, opts) (ChatResponse, error) {
    for i := 0; i < maxIters; i++ {
        resp := l.client.Chat(ctx, msgs, chatOpts)
        msgs = append(msgs, resp.Message)

        if len(resp.Message.ToolCalls) == 0 {
            return resp, nil  // 완료
        }

        for _, call := range resp.Message.ToolCalls {
            t, _ := l.registry.Get(call.Name)
            result, _ := t.Execute(ctx, call.Arguments)
            msgs = append(msgs, toolResultMessage)
        }
    }
    return error("exceeded max iterations")
}
```

### 4-3. SSE 스트림 작성기

서버가 클라이언트에게 실시간으로 응답을 보내는 SSE(Server-Sent Events)입니다.

**`internal/server/sse.go`**

SSE 프로토콜:
```
event: status
data: {"session_id":"abc","status":"stream_open","message":"connected"}

event: delta
data: {"session_id":"abc","text":"안녕"}

event: delta
data: {"session_id":"abc","text":"하세요"}

event: done
data: {"session_id":"abc","usage":{"input_tokens":50,"output_tokens":10}}
```

핵심 포인트:
- `Content-Type: text/event-stream` 헤더 필수
- `event:` — 이벤트 타입, `data:` — JSON 페이로드
- 빈 줄(`\n\n`) — 이벤트 구분자
- `http.Flusher.Flush()` — 버퍼를 즉시 전송 (이게 없으면 스트리밍 안 됨)

### 4-4. 채팅 핸들러

원본 5개 파일의 핵심을 하나로 합칩니다.

**`internal/server/handler_chat.go`**

10단계 흐름을 하나의 `ServeHTTP` 메서드에서 처리:
1. 요청 파싱
2. 세션 resolve
3. 히스토리 로드
4. 프롬프트 조립
5. LLM 메시지 구성
6. 유저 메시지 저장
7. SSE 스트림 시작
8. Agent loop 실행
9. assistant 응답 저장
10. done 이벤트

### 4-5. 서버 부트스트랩

**`internal/server/server.go`** — 모든 조각을 조립:

```go
func Serve(ctx context.Context, cfg Config) error {
    store := session.NewStore(cfg.WorkspaceDir)

    registry := tool.NewRegistry()
    registry.Register(tool.NewEchoTool())
    registry.Register(tool.NewCurrentTimeTool())

    chatHandler := NewChatHandler(store, cfg.Client, registry, cfg.WorkspaceDir)

    mux := http.NewServeMux()
    mux.Handle("/v1/chat", chatHandler)

    return http.ListenAndServe(addr, mux)
}
```

### 4-6. Mock LLM Client

실제 API 키 없이 동작을 확인할 수 있는 테스트용 클라이언트입니다.

**`internal/llm/mock.go`**

동작:
- "시간" 또는 "time"이 포함되면 → `current_time` tool call 반환
- tool 결과가 있으면 → 결과를 포함한 응답 반환
- 그 외 → 메시지를 echo

**주의: tool 결과 확인을 키워드 체크보다 먼저 해야 합니다!**

```go
func (m *MockClient) Chat(...) {
    // ★ tool 결과 확인을 먼저!
    for _, msg := range messages {
        if msg.Role == "tool" {
            return 응답 with tool 결과
        }
    }
    // 그 다음 키워드 체크
    if contains("시간") {
        return tool call
    }
}
```

이 순서가 잘못되면 agent loop가 무한 반복합니다 — tool call을 반환하고, tool 실행 후 다시 호출되면 또 키워드에 매칭되어 다시 tool call을 반환하기 때문입니다.

## 테스트

```bash
# 서버 시작
go run ./cmd/tars/ serve

# 일반 채팅
curl -N -X POST http://localhost:8080/v1/chat \
  -H "Content-Type: application/json" \
  -d '{"message":"안녕하세요"}'

# tool call 테스트 (current_time 도구 실행됨)
curl -N -X POST http://localhost:8080/v1/chat \
  -H "Content-Type: application/json" \
  -d '{"message":"지금 시간 알려줘"}'
```

**기대 결과 (일반 채팅):**
```
event: status
data: {"session_id":"...","status":"stream_open","message":"connected"}

event: delta
data: {"session_id":"...","text":"echo: 안녕하세요"}

event: done
data: {"session_id":"...","usage":{"input_tokens":20,"output_tokens":5}}
```

**기대 결과 (시간 질문 — tool call 발생):**
```
event: status
data: {"session_id":"...","status":"stream_open","message":"connected"}

event: delta
data: {"session_id":"...","text":"현재 시각은 2026-03-22T... 입니다."}

event: done
data: {"session_id":"...","usage":{"input_tokens":30,"output_tokens":10}}
```

## 체크포인트

- [x] SSE delta 이벤트가 순서대로 온다
- [x] 세션 ID가 응답에 포함된다
- [x] tool call이 있으면 실행 후 결과를 다시 LLM에 넘긴다
- [x] 대화가 transcript에 저장된다

## 최종 구조

```
tars/
├── cmd/tars/
│   ├── main.go
│   └── serve.go              ← server.Serve() 호출
├── internal/
│   ├── buildinfo/
│   │   └── buildinfo.go
│   ├── llm/
│   │   ├── types.go           ← Client interface, ChatMessage, ToolCall, ...
│   │   └── mock.go            ← 테스트용 Mock Client
│   ├── agent/
│   │   └── loop.go            ← LLM + tool call 반복 루프
│   ├── prompt/
│   │   ├── builder.go
│   │   └── builder_test.go
│   ├── session/
│   │   ├── message.go
│   │   ├── session.go
│   │   ├── transcript.go
│   │   └── session_test.go
│   ├── server/
│   │   ├── server.go          ← 부트스트랩 + HTTP mux
│   │   ├── handler_chat.go    ← 채팅 핸들러 (10단계 파이프라인)
│   │   └── sse.go             ← SSE 스트림 작성기
│   └── tool/
│       ├── tool.go
│       ├── echo.go
│       ├── current_time.go
│       └── tool_test.go
└── go.mod
```

## 배운 패턴

- **Agent Loop** — LLM 호출 → tool call 확인 → 실행 → 반복의 핵심 구조
- **SSE 스트리밍** — `text/event-stream` + `Flush()`로 실시간 응답
- **Mock Client** — 외부 의존성 없이 전체 파이프라인 검증
- **10단계 파이프라인** — 요청 → 세션 → 히스토리 → 프롬프트 → LLM → 도구 → 저장 → 스트리밍
