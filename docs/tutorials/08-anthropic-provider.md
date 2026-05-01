# Step 8. Anthropic Provider Adapter

> 학습 목표: 두 번째 provider를 추가하면서, OpenAI와의 구조적 차이를 adapter 패턴으로 흡수

## 원본 코드 분석 (TARS)

TARS의 `internal/llm/anthropic.go`는 Anthropic `/v1/messages` API 전용 adapter입니다. OpenAI adapter와 **같은 `Client` 인터페이스**를 구현하지만, wire format이 크게 다릅니다.

현재 TARS에서 provider는 단일 `provider/model/api_key` flat field가 아니라 Step 6의 LLM provider pool로 선택된다. 이 Step은 Anthropic wire format adapter 자체를 이해하기 위한 구간이다.

### OpenAI vs Anthropic 주요 차이

| 항목 | OpenAI | Anthropic |
|------|--------|-----------|
| 엔드포인트 | `/chat/completions` | `/v1/messages` |
| 인증 헤더 | `Authorization: Bearer KEY` | `x-api-key: KEY` |
| system 메시지 | messages 배열 안에 포함 | 별도 `system` 필드 |
| tool schema | `{"type":"function","function":{...,"parameters":...}}` | flat `{"name":...,"input_schema":...}` |
| tool 결과 | `role: "tool"` | `role: "user"` + `type: "tool_result"` |
| assistant tool call | `tool_calls: [{function:{name,arguments}}]` | `content: [{type:"tool_use",name,input}]` |
| SSE 이벤트 | `data:` 한 줄 | `event:` + `data:` 두 줄 |
| max_tokens | 선택 (기본값 있음) | **필수** |

### 핵심 변환 포인트 3가지

**1. system 메시지 분리**

```go
// OpenAI: system이 messages 안에 있음
messages: [
  {"role": "system", "content": "You are..."},
  {"role": "user", "content": "안녕"}
]

// Anthropic: system을 별도 필드로 분리
system: "You are...",
messages: [
  {"role": "user", "content": "안녕"}
]
```

**2. tool schema 변환**

```go
// OpenAI (내부 저장 포맷)
{"type": "function", "function": {"name": "echo", "parameters": {...}}}

// Anthropic (전송 포맷)
{"name": "echo", "input_schema": {...}}
```

래핑을 벗기고 `parameters` → `input_schema`로 키만 바꿉니다.

**3. tool result 변환**

```go
// OpenAI
{"role": "tool", "tool_call_id": "call_abc", "content": "결과"}

// Anthropic
{"role": "user", "content": [{"type": "tool_result", "tool_use_id": "call_abc", "content": "결과"}]}
```

`role`이 `"user"`로 바뀌고, content가 배열이 됩니다.

## 실습

### 8-1. AnthropicClient 구조체

**`internal/llm/anthropic.go`**

```go
type AnthropicClient struct {
    baseURL    string
    apiKey     string
    model      string
    maxTokens  int        // Anthropic는 필수!
    httpClient *http.Client
}

func NewAnthropicClient(baseURL, apiKey, model string) (*AnthropicClient, error) {
    return &AnthropicClient{
        maxTokens: 4096,   // 기본값 설정
        // ...
    }, nil
}
```

### 8-2. 요청 빌드 — 가장 큰 차이

```go
func (c *AnthropicClient) buildRequest(messages, opts, streaming) map[string]any {
    // 1. system 메시지 분리
    var systemParts []string
    var nonSystem []ChatMessage
    for _, msg := range messages {
        if msg.Role == "system" {
            systemParts = append(systemParts, msg.Content)
        } else {
            nonSystem = append(nonSystem, msg)
        }
    }

    reqBody := map[string]any{
        "model":      c.model,
        "max_tokens": c.maxTokens,            // 필수!
        "messages":   toAnthropicMessages(nonSystem),
    }
    if len(systemParts) > 0 {
        reqBody["system"] = strings.Join(systemParts, "\n")
    }

    // 2. tool 변환
    if len(opts.Tools) > 0 {
        reqBody["tools"] = toAnthropicTools(opts.Tools)
    }
    return reqBody
}
```

### 8-3. 메시지 변환 — role과 content 구조 차이

```go
func toAnthropicMessages(messages []ChatMessage) []map[string]any {
    for _, msg := range messages {
        switch msg.Role {
        case "assistant":
            // tool call이 있으면 content를 블록 배열로 변환
            // [{type:"text",text:"..."}, {type:"tool_use",id:...,name:...,input:...}]
        case "tool":
            // role을 "user"로 바꾸고 tool_result 블록으로 감쌈
            // {role:"user", content:[{type:"tool_result",tool_use_id:...,content:...}]}
        default:
            // user 등은 그대로
        }
    }
}
```

assistant의 tool call에서 `Arguments`(문자열)를 `input`(객체)으로 변환할 때 `json.Unmarshal`이 필요합니다. OpenAI는 arguments가 문자열이지만 Anthropic의 input은 JSON 객체입니다.

### 8-4. SSE 파싱 — event + data 조합

OpenAI와 가장 다른 부분:

```
event: content_block_start
data: {"index":0,"content_block":{"type":"tool_use","id":"toolu_abc","name":"echo"}}

event: content_block_delta
data: {"index":0,"delta":{"partial_json":"{\"message\":"}}

event: content_block_delta
data: {"index":0,"delta":{"partial_json":"\"hello\"}"}}

event: message_delta
data: {"delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":15}}
```

파싱 전략:
1. `event:` 줄이 오면 이벤트 타입을 저장
2. `data:` 줄이 오면 저장된 이벤트 타입에 따라 처리
3. `content_block_start` — tool call의 ID와 이름 기록
4. `content_block_delta` — 텍스트 또는 `partial_json` 누적
5. `message_start` — input_tokens 기록
6. `message_delta` — output_tokens, stop_reason 기록

### 8-5. provider pool에 Anthropic 연결

```go
client, err := llm.NewProvider(llm.ProviderOptions{
    Provider: "anthropic",
    BaseURL:  "https://api.anthropic.com",
    APIKey:   os.Getenv("ANTHROPIC_API_KEY"),
    Model:    "claude-sonnet-4-6",
})
if err != nil {
    return err
}
```

실제 서버에서는 이 생성이 `config.ResolveAllLLMTiers`로 해석된 tier별 provider 설정을 통해 일어난다.

## 테스트

```bash
# Anthropic API로 테스트
cat > config.yaml <<'EOF'
llm:
  providers:
    default:
      kind: anthropic
      auth_mode: api-key
      api_key: ${ANTHROPIC_API_KEY}
  tiers:
    heavy: { provider: default, model: claude-opus-4-6 }
    standard: { provider: default, model: claude-sonnet-4-6 }
    light: { provider: default, model: claude-haiku-4-5 }
  default_tier: standard
EOF

ANTHROPIC_API_KEY=sk-ant-... go run ./cmd/tars/ serve --config config.yaml

curl -N -X POST http://127.0.0.1:43180/v1/chat \
  -H "Content-Type: application/json" \
  -d '{"message":"안녕하세요"}'
```

## 체크포인트

- [x] provider별 wire format 차이를 별도 구현체로 분리했다
- [x] config에서 provider를 바꾸면 다른 LLM이 사용된다
- [x] `Client` 인터페이스 덕분에 나머지 코드는 변경 없음

## 최종 구조 (Phase 2 완료)

```
tars/
├── cmd/tars/
│   ├── main.go
│   └── server_main.go          ← config.Load() + tarsserver.Serve()
├── internal/
│   ├── buildinfo/
│   │   └── buildinfo.go
│   ├── config/
│   │   ├── defaults.go
│   │   ├── load.go
│   │   └── llm_resolve.go      ← provider pool/tier 해석
│   ├── llm/
│   │   ├── provider.go         ← Client interface, ChatMessage, ...
│   │   ├── router.go           ← heavy/standard/light tier routing
│   │   ├── openai_compat_client.go
│   │   ├── openai_codex_client.go
│   │   └── anthropic.go        ← Anthropic /v1/messages adapter
│   ├── agent/                  ← LLM call + tool call loop
│   ├── prompt/
│   │   └── builder.go
│   ├── session/
│   │   ├── session.go
│   │   └── transcript.go
│   ├── tarsserver/
│   │   ├── handler_chat.go
│   │   ├── helpers_llm_router.go
│   │   └── main_serve_api.go
│   └── tool/
│       ├── tool.go
│       ├── read_file.go
│       ├── write_file.go
│       └── exec.go
├── docs/
│   └── tutorials/
│       ├── 00-overview.md
│       ├── 01-thin-cli.md
│       ├── 02-session-transcript.md
│       ├── 03-prompt-tool-registry.md
│       ├── 04-http-sse-chat.md
│       ├── 05-openai-provider.md
│       ├── 06-config-system.md
│       ├── 07-practical-tools.md
│       ├── 08-anthropic-provider.md
│       └── 99-roadmap.md
└── go.mod
```

## 배운 패턴

- **Adapter 패턴** — 같은 인터페이스 뒤에 다른 wire format을 숨김
- **system 메시지 분리** — provider에 따라 system 메시지 위치가 다름
- **`parameters` ↔ `input_schema`** — JSON Schema는 동일하고, 키 이름만 다름
- **event + data SSE** — Anthropic은 OpenAI와 달리 event 줄이 별도로 옴
- **`partial_json` 누적** — tool call arguments가 JSON 조각으로 올 때 문자열 연결
