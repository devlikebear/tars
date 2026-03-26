# Step 3. 프롬프트 빌더 + 도구 Registry

> 학습 목표: 시스템 프롬프트 조립과 tool schema를 LLM에 넘기는 구조 이해

## 원본 코드 분석 (TARS)

### 프롬프트 빌더 (`internal/prompt/`)

```
builder.go              ← 시스템 프롬프트 조립 엔진
bootstrap_sections.go   ← 워크스페이스에서 읽을 파일 목록 정의
memory_retrieval.go     ← semantic memory 주입 (고급 기능)
```

조립 흐름:
```
"You are TARS..." (고정 인사)
+ 현재 시각
+ PROJECT.md 내용 (있으면)
+ USER.md 내용 (있으면)
+ IDENTITY.md 내용 (있으면)
+ relevant memory (있으면)
→ 하나의 시스템 프롬프트 문자열
```

원본에는 토큰 예산 관리(`trimToBudget`, `estimateTokens`)가 있어서 프롬프트가 무한히 커지지 않습니다. 최소 버전에서는 생략합니다.

### 도구 Registry (`internal/tool/tool.go`)

```go
type Tool struct {
    Name        string
    Description string
    Parameters  json.RawMessage      // JSON Schema
    Execute     func(ctx, params) (Result, error)
}

type Registry struct {
    tools map[string]Tool
}
```

`Schemas()` 메서드가 등록된 도구를 OpenAI tool format으로 변환합니다:
```json
{
  "type": "function",
  "function": {
    "name": "echo",
    "description": "Echoes back the given message",
    "parameters": { "type": "object", "properties": { ... } }
  }
}
```

## 핵심 설계 질문과 답변

### Q: Parameters를 구조체가 아니라 `json.RawMessage`로 한 이유?

**LLM API가 JSON Schema 원문을 그대로 요구하기 때문입니다.**

JSON Schema는 `oneOf`, `anyOf`, `$ref`, `additionalProperties` 등 끝없이 복잡해질 수 있습니다. 이걸 전부 Go 구조체로 모델링하는 것은 과잉 설계입니다.

`json.RawMessage`를 쓰면:
- JSON을 파싱하지 않고 바이트 그대로 보관
- LLM에 보낼 때 그냥 직렬화하면 끝
- 도구 작성자가 자유롭게 schema를 정의할 수 있음

**원칙: "통과시키기만 하면 되는 데이터"는 구조체로 만들 필요가 없다.**

### Q: OpenAI와 Anthropic의 tool 구조는 동일한가?

비슷하지만 다릅니다:

| | OpenAI | Anthropic |
|--|--------|-----------|
| 래핑 | `{"type":"function","function":{...}}` | 없음 (flat) |
| schema 필드명 | `parameters` | `input_schema` |
| schema 내용 | JSON Schema | JSON Schema (동일) |

**JSON Schema 부분은 동일합니다.** 그래서 `json.RawMessage`로 들고 있으면, 보낼 때만 감싸는 형태를 바꿔주면 됩니다. 이것이 provider adapter를 별도로 분리하는 이유입니다.

### Q: 도구 실행 시 `error` 리턴 vs `Result{IsError: true}` 리턴의 차이?

| | `return Result{}, err` | `return Result{IsError: true}, nil` |
|--|--|--|
| 의미 | 시스템 장애 (도구 실행 자체 실패) | 도구는 실행됐지만 결과가 실패 |
| 예시 | DB 연결 끊김, 파일시스템 오류 | 잘못된 파라미터, 검색 결과 없음 |
| LLM에게 | 에러 메시지를 보여주지 않을 수도 있음 | "실패했어, 다시 시도해봐" 라고 알려줌 |

LLM이 잘못된 인자를 보냈을 때는 `IsError: true`로 돌려줘서 수정 기회를 주고, 시스템이 터졌을 때만 `error`를 반환합니다.

## 실습

### 3-1. LLM 타입

도구 registry가 참조하는 공통 타입입니다.

**`internal/llm/types.go`**

```go
type ToolFunctionSchema struct {
    Name        string          `json:"name"`
    Description string          `json:"description"`
    Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type ToolSchema struct {
    Type     string             `json:"type"`
    Function ToolFunctionSchema `json:"function"`
}
```

### 3-2. 도구 Registry

```go
// internal/tool/tool.go
type Registry struct {
    tools map[string]Tool
}

func (r *Registry) Register(t Tool)           // 등록
func (r *Registry) Get(name string) (Tool, bool) // 이름으로 조회
func (r *Registry) Schemas() []llm.ToolSchema    // LLM 포맷 변환
```

`Schemas()`는 이름순으로 정렬해서 반환합니다. 결과가 결정적(deterministic)이어야 테스트와 디버깅이 쉬워지기 때문입니다.

### 3-3. 샘플 도구

도구 하나 = 함수 하나 (`NewXxxTool()`) 패턴:

```go
// internal/tool/echo.go
func NewEchoTool() Tool {
    return Tool{
        Name:        "echo",
        Description: "Echoes back the given message",
        Parameters: json.RawMessage(`{
            "type": "object",
            "properties": {
                "message": { "type": "string", "description": "..." }
            },
            "required": ["message"]
        }`),
        Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
            var args struct { Message string `json:"message"` }
            json.Unmarshal(params, &args)
            return Result{Content: args.Message}, nil
        },
    }
}
```

### 3-4. 프롬프트 빌더

```go
// internal/prompt/builder.go
func Build(opts BuildOptions) string {
    // 1. 고정 인사 + 현재 시각
    // 2. 워크스페이스 파일 읽기 (PROJECT.md, USER.md, IDENTITY.md)
    // 3. 파일이 없으면 조용히 건너뜀
}
```

### 3-5. 테스트

```bash
go test ./internal/tool/ -v     # 5개 PASS
go test ./internal/prompt/ -v   # 3개 PASS
```

## 체크포인트

- [x] 툴 schema를 LLM에 넘길 수 있다
- [x] 툴이 없어도 기본 채팅이 동작한다
- [x] 프롬프트에 워크스페이스 파일이 자동 반영된다

## 배운 패턴

- **`json.RawMessage`로 JSON Schema 통과** — 파싱 불필요한 데이터는 구조체로 만들지 않는다
- **프롬프트 = 고정 텍스트 + 파일 기반 섹션** — 파일이 없으면 조용히 건너뜀
- **도구 = 이름 + 설명 + schema + 실행 함수** — 하나의 구조체에 전부 담김
- **error vs IsError** — 시스템 장애와 도구 실패를 구분
