# Step 10. 파일 기반 Memory

> 학습 목표: LLM이 장기 기억을 저장하고 검색하는 도구 구현, Markdown 기반 메모리 패턴 이해

## 원본 코드 분석 (TARS)

TARS의 메모리 시스템은 여러 소스로 구성됩니다:

```
memory/MEMORY.md        ← 영구 사실/선호도
memory/YYYY-MM-DD.md    ← 일별 로그
memory/experiences.jsonl ← 구조화된 경험 (카테고리, 태그, 중요도)
```

최소 버전에서는 **단일 `memory/MEMORY.md` 파일**에 append하고 검색하는 것으로 시작합니다.

### 핵심 설계: 왜 Markdown인가

- **사람이 읽을 수 있음** — 에디터로 직접 열어서 편집 가능
- **LLM이 이해하기 쉬움** — 별도 파싱 없이 프롬프트에 주입 가능
- **Append-friendly** — JSONL transcript과 같은 패턴
- OpenClaw도 같은 선택 — `MEMORY.md` Markdown 파일에 메모리 저장

### 두 가지 도구

| 도구 | 역할 | 파라미터 |
|------|------|----------|
| `memory_save` | 기억 저장 | `content` (필수) |
| `memory_search` | 기억 검색 | `query` (필수) |

## 실습

### 10-1. `memory_save` 도구

**`internal/tool/memory_save.go`**

```go
func NewMemorySaveTool(workspaceDir string) Tool {
    Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
        // 1. content 파싱
        // 2. memory/ 디렉터리 생성 (MkdirAll)
        // 3. MEMORY.md에 타임스탬프 + 내용 append
    }
}
```

저장 형식:
```markdown
## 2026-03-22T14:30:00Z

사용자는 Go 언어를 주로 사용합니다.
```

`## 타임스탬프` 형식이라 나중에 날짜별로 구분하기 쉽습니다.

### 10-2. `memory_search` 도구

**`internal/tool/memory_search.go`**

```go
func NewMemorySearchTool(workspaceDir string, semantic *memory.Service) Tool {
    Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
        // 1. semantic 검색 시도 (있으면)
        if semantic != nil {
            hits, _ := semantic.Search(ctx, query, 10)
            if len(hits) > 0 { return hits }
        }
        // 2. fallback: 텍스트 검색
        // - 대소문자 무시 substring 매칭
        // - 최대 10개 결과
    }
}
```

포인트:
- **Semantic-first, text fallback** — semantic service가 있으면 먼저 사용, 없거나 실패하면 텍스트 검색
- `*memory.Service`가 `nil`이면 텍스트 검색만 동작 — core를 깨지 않는 optional layer
- 파일이 없으면 "no memories found" (에러가 아님)

### 10-3. 서버에 등록

**`internal/server/server.go`:**

```go
var semanticSvc *memory.Service  // nil — 아직 embedding 미설정

registry.Register(tool.NewMemorySaveTool(cfg.WorkspaceDir))
registry.Register(tool.NewMemorySearchTool(cfg.WorkspaceDir, semanticSvc))
```

## 테스트

```bash
go run ./cmd/tars/ serve

# 기억 저장 유도
curl -N -X POST http://localhost:8080/v1/chat \
  -H "Content-Type: application/json" \
  -d '{"message":"내가 Go 개발자라는 걸 기억해줘"}'

# 기억 검색 유도
curl -N -X POST http://localhost:8080/v1/chat \
  -H "Content-Type: application/json" \
  -d '{"message":"내가 어떤 언어를 쓰는지 기억나?"}'

# 직접 파일 확인
cat .workspace/memory/MEMORY.md
```

## 체크포인트

- [x] 저장한 기억을 다음 대화에서 검색할 수 있다
- [x] semantic service 없이도 텍스트 검색이 동작한다
- [x] `memory/MEMORY.md` 파일을 사람이 직접 읽고 편집할 수 있다

## 배운 패턴

- **Markdown 메모리** — 사람과 LLM 모두 읽기 쉬운 포맷
- **Optional dependency** — `nil` 체크로 없는 서비스를 graceful하게 처리
- **Semantic-first fallback** — 고급 기능이 없으면 기본 기능으로 자동 전환
- **도구 = 메모리 인터페이스** — LLM이 `memory_save`/`memory_search` 도구를 통해 자율적으로 기억을 관리
