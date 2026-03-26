# Step 5. OpenAI 호환 Provider Adapter

> 학습 목표: 실제 LLM API(OpenAI `/chat/completions`)를 연결하고, SSE 스트리밍을 파싱하는 adapter 작성

## 원본 코드 분석 (TARS)

TARS의 `internal/llm/openai_compat_client.go`는 OpenAI 호환 API를 호출하는 클라이언트입니다. 핵심 구조:

```
openai_compat_client.go   ← Chat() 메서드, SSE 파싱
transport.go              ← 공통 HTTP 유틸
http_utils.go             ← 에러 처리 헬퍼
```

### 핵심 설계 포인트

**1. Wire Format 변환**

내부 `ChatMessage`와 OpenAI API의 JSON 형식은 다릅니다. 보낼 때는 내부 → wire로 변환하고, 받을 때는 wire → 내부로 변환합니다.

```go
// 내부 타입
type ToolCall struct { ID, Name, Arguments string }

// OpenAI wire 타입 — function이 중첩 객체
type wireToolCall struct {
    ID       string `json:"id"`
    Type     string `json:"type"`     // 항상 "function"
    Function struct {
        Name      string `json:"name"`
        Arguments string `json:"arguments"`
    } `json:"function"`
}
```

**2. SSE 스트리밍에서 Tool Call 누적**

OpenAI의 스트리밍 응답에서 tool call은 여러 청크에 걸쳐 옵니다:

```
chunk 1: tool_calls[0] = { index: 0, id: "call_abc", function: { name: "echo" } }
chunk 2: tool_calls[0] = { index: 0, function: { arguments: '{"mes' } }
chunk 3: tool_calls[0] = { index: 0, function: { arguments: 'sage":"hi"}' } }
```

`map[int]ToolCall`로 index별로 누적한 뒤, 마지막에 index 순서로 정렬합니다.

**3. 스트리밍 vs 비스트리밍 분기**

`OnDelta` 콜백이 있으면 스트리밍, 없으면 비스트리밍입니다. 스트리밍 시에는 `http.Client`의 `Timeout`을 제거합니다 — 스트리밍은 응답이 점진적으로 오기 때문에 전체 타임아웃을 걸면 긴 응답이 잘립니다.

## 실습

### 5-1. OpenAI 클라이언트 구조체

**`internal/llm/openai.go`**

```go
type OpenAIClient struct {
    baseURL    string
    apiKey     string
    model      string
    httpClient *http.Client
}

func NewOpenAIClient(baseURL, apiKey, model string) (*OpenAIClient, error) {
    // 빈 값 검증: baseURL, apiKey, model 모두 필수
    return &OpenAIClient{
        baseURL:    strings.TrimRight(baseURL, "/"),
        apiKey:     apiKey,
        model:      model,
        httpClient: &http.Client{Timeout: 120 * time.Second},
    }, nil
}
```

### 5-2. Chat() 메서드 — 요청 빌드와 전송

```go
func (c *OpenAIClient) Chat(ctx, messages, opts) (ChatResponse, error) {
    streaming := opts.OnDelta != nil

    // 1. 요청 바디 구성
    reqBody := map[string]any{
        "model":    c.model,
        "messages": toWireMessages(messages),  // 내부 → wire 변환
    }
    if len(opts.Tools) > 0 {
        reqBody["tools"] = opts.Tools          // ToolSchema는 이미 OpenAI 포맷
    }
    if streaming {
        reqBody["stream"] = true
    }

    // 2. HTTP POST
    req.Header.Set("Authorization", "Bearer "+c.apiKey)

    // 스트리밍 시 전체 타임아웃 제거
    httpClient := c.httpClient
    if streaming {
        httpClient = &http.Client{Transport: c.httpClient.Transport}
    }

    // 3. 응답 분기
    if streaming {
        return c.parseStreaming(resp.Body, opts)
    }
    return c.parseNonStreaming(resp.Body)
}
```

### 5-3. SSE 스트리밍 파싱

OpenAI SSE 프로토콜:
```
data: {"choices":[{"delta":{"content":"안녕"}}]}

data: {"choices":[{"delta":{"content":"하세요"}}]}

data: [DONE]
```

파싱 핵심:
1. `bufio.Scanner`로 한 줄씩 읽기
2. `data:` 접두사만 처리, 나머지 무시
3. `[DONE]`이면 종료
4. `delta.content`가 있으면 `OnDelta` 콜백 호출
5. `delta.tool_calls`는 index로 누적

```go
func (c *OpenAIClient) parseStreaming(body io.Reader, opts ChatOptions) (ChatResponse, error) {
    toolCalls := map[int]ToolCall{}  // ← index별 누적

    scanner := bufio.NewScanner(body)
    for scanner.Scan() {
        line := scanner.Text()
        // "data:" 접두사 처리
        // JSON 파싱
        // delta.content → OnDelta 콜백
        // delta.tool_calls → map에 누적
    }

    return ChatResponse{
        Message: ChatMessage{
            ToolCalls: orderedToolCalls(toolCalls),  // index 순 정렬
        },
    }, nil
}
```

### 5-4. Wire Format 변환 함수들

```go
// 내부 ChatMessage → OpenAI API JSON
func toWireMessages(messages []ChatMessage) []wireMessage { ... }

// map[int]ToolCall → []ToolCall (index 순 정렬)
func orderedToolCalls(m map[int]ToolCall) []ToolCall { ... }
```

`orderedToolCalls`는 `sort.Ints`로 index를 정렬합니다. 빈 이름의 tool call은 제거합니다.

## 테스트

```bash
# 컴파일 확인
go build ./...

# 실제 OpenAI API로 테스트 (API 키 필요)
MYCLAW_PROVIDER=openai MYCLAW_API_KEY=sk-... MYCLAW_MODEL=gpt-4o-mini \
  go run ./cmd/tars/ serve

curl -N -X POST http://localhost:8080/v1/chat \
  -H "Content-Type: application/json" \
  -d '{"message":"안녕하세요"}'
```

## 체크포인트

- [x] `OpenAIClient`가 `Client` 인터페이스를 구현한다
- [x] SSE 스트리밍 delta가 `OnDelta` 콜백으로 전달된다
- [x] tool call이 여러 청크에 걸쳐 올 때 올바르게 누적된다

## 배운 패턴

- **Wire Format 변환** — 내부 타입과 API 타입을 분리해서, API 변경이 내부에 전파되지 않게 함
- **SSE Tool Call 누적** — `map[int]ToolCall`로 index별 누적 후 정렬
- **스트리밍 타임아웃 분리** — 스트리밍 시에는 전체 타임아웃을 제거 (Transport만 복사)
- **Scanner 기반 SSE 파싱** — `bufio.Scanner`로 한 줄씩 읽어 `data:` 접두사 처리
