# Step 11. Semantic Memory (Embedding 기반)

> 학습 목표: 텍스트 검색을 벡터 유사도 검색으로 업그레이드하고, optional layer 설계 이해

## 원본 코드 분석 (TARS)

TARS의 `internal/memory/` 패키지:

```
semantic.go       ← Service (인덱싱, 검색, 코사인 유사도)
gemini_embed.go   ← Gemini embedding API adapter
experience.go     ← 경험 구조체, JSONL 저장
workspace.go      ← 워크스페이스 초기화, 일별 로그
```

### 핵심: 텍스트 검색 vs Semantic 검색

| | 텍스트 검색 | Semantic 검색 |
|--|------------|--------------|
| "Go 개발자" 검색 | "Go"가 포함된 줄만 매칭 | "Golang", "프로그래밍 언어" 등도 매칭 |
| 원리 | 문자열 substring 비교 | 벡터 공간에서 코사인 유사도 |
| 외부 의존 | 없음 | Embedding API 필요 |
| 비용 | 무료 | API 호출당 비용 발생 |

### Embedding이란?

텍스트를 고차원 벡터(숫자 배열)로 변환하는 것입니다:

```
"Go 개발자" → [0.12, -0.45, 0.78, ..., 0.33]  (768차원)
"Golang 프로그래머" → [0.11, -0.44, 0.79, ..., 0.32]  (비슷한 벡터!)
"오늘 날씨" → [0.89, 0.23, -0.11, ..., -0.67]  (전혀 다른 벡터)
```

의미가 비슷한 텍스트는 벡터도 비슷합니다. 두 벡터의 유사도를 **코사인 유사도**로 측정합니다.

### 코사인 유사도

```
similarity = dot(a, b) / (|a| × |b|)
```

- 1.0 = 완전히 같은 방향 (의미 일치)
- 0.0 = 직교 (무관)
- -1.0 = 반대 방향

## 실습

### 11-1. Embedder 인터페이스

**`internal/memory/semantic.go`**

```go
type Embedder interface {
    Embed(ctx context.Context, text string) ([]float64, error)
}
```

이 인터페이스 하나로 Gemini, OpenAI, 로컬 모델 등 어떤 embedding 서비스든 갈아끼울 수 있습니다. Phase 2에서 배운 `Client` 인터페이스와 같은 adapter 패턴이에요.

### 11-2. Service — 인덱싱과 검색

```go
type Service struct {
    embedder Embedder
    entries  []Entry  // 인메모리 저장
}

// 텍스트를 임베딩해서 메모리에 추가
func (s *Service) Index(ctx, id, content, source) error {
    vec := s.embedder.Embed(ctx, content)
    s.entries = append(s.entries, Entry{..., Embedding: vec})
}

// 쿼리와 유사한 메모리를 찾음
func (s *Service) Search(ctx, query, limit) ([]SearchHit, error) {
    queryVec := s.embedder.Embed(ctx, query)
    // 모든 entry와 코사인 유사도 계산
    // 점수 내림차순 정렬
    // 상위 N개 반환
}
```

최소 버전에서는 **인메모리 저장**입니다. TARS 원본은 `entries.jsonl` 파일에 영속화하고, SHA256 해시로 중복 인덱싱을 방지합니다.

### 11-3. 코사인 유사도 구현

```go
func cosineSimilarity(a, b []float64) float64 {
    if len(a) != len(b) || len(a) == 0 {
        return 0
    }
    var dot, normA, normB float64
    for i := range a {
        dot += a[i] * b[i]
        normA += a[i] * a[i]
        normB += b[i] * b[i]
    }
    denom := math.Sqrt(normA) * math.Sqrt(normB)
    if denom == 0 {
        return 0
    }
    return dot / denom
}
```

길이가 다르거나 비어있으면 0을 반환합니다. 0으로 나누는 것도 방어합니다.

### 11-4. Gemini Embedding Adapter

**`internal/memory/gemini.go`**

```go
type GeminiEmbedder struct {
    baseURL, apiKey, model string
    client *http.Client
}

func (g *GeminiEmbedder) Embed(ctx, text) ([]float64, error) {
    // POST {baseURL}/{model}:embedContent?key={apiKey}
    // body: { model, content: { parts: [{ text }] } }
    // response: { embedding: { values: [float64...] } }
}
```

포인트:
- `models/` 접두사 자동 추가 — 사용자가 `text-embedding-004`만 쓰면 `models/text-embedding-004`로 변환
- 타임아웃 20초 — embedding 호출은 빠르지만 안전장치
- API 키를 URL 파라미터로 전달 (`?key=`) — Gemini API 컨벤션

### 11-5. Optional Layer 연결

**`internal/server/server.go`:**

```go
var semanticSvc *memory.Service  // nil = embedding 미설정

registry.Register(tool.NewMemorySearchTool(cfg.WorkspaceDir, semanticSvc))
```

**`internal/tool/memory_search.go`:**

```go
if semantic != nil {
    hits, err := semantic.Search(ctx, query, 10)
    if err == nil && len(hits) > 0 {
        return hits  // semantic 결과 사용
    }
}
// fallback: 텍스트 검색
```

`semanticSvc`가 `nil`이면 텍스트 검색만 동작합니다. **core chat path를 깨지 않는 optional layer** — Phase 1이 항상 동작해야 한다는 원칙을 지킵니다.

## 체크포인트

- [x] "비슷한 의미"의 기억을 찾을 수 있다 (embedding 서비스 활성 시)
- [x] embedding 서비스가 비활성화돼도 기존 파일 검색이 동작한다
- [x] `Embedder` 인터페이스로 provider를 교체할 수 있다

## 최종 구조 (Phase 3 완료)

```
tars/
├── internal/
│   ├── memory/                     ← Phase 3 신규 패키지
│   │   ├── semantic.go             ← Service, Entry, SearchHit, cosineSimilarity
│   │   └── gemini.go              ← GeminiEmbedder (Gemini API adapter)
│   ├── session/
│   │   ├── message.go
│   │   ├── session.go
│   │   ├── transcript.go          ← + RewriteMessages, LoadHistory
│   │   ├── compaction.go          ← CompactTranscript, BuildCompactionSummary
│   │   └── session_test.go
│   ├── tool/
│   │   ├── memory_save.go         ← MEMORY.md에 기억 저장
│   │   ├── memory_search.go       ← semantic-first + text fallback 검색
│   │   └── ...
│   └── server/
│       ├── server.go              ← semantic service optional 연결
│       └── handler_chat.go        ← compaction 체크 추가
└── docs/
    └── lessons/
        ├── 09-transcript-compaction.md
        ├── 10-file-memory.md
        └── 11-semantic-memory.md
```

## 배운 패턴

- **Embedder 인터페이스** — `Client` 인터페이스와 같은 adapter 패턴을 embedding에도 적용
- **코사인 유사도** — 벡터 공간에서 의미적 유사성 측정, 구현은 10줄
- **Optional layer** — `nil` 체크로 없는 서비스를 graceful하게 처리, core를 깨지 않음
- **Semantic-first fallback** — 고급 기능이 실패하면 기본 기능으로 자동 전환
- **인메모리 → 영속화 경로** — 먼저 인메모리로 동작을 확인하고, 나중에 파일 저장 추가
