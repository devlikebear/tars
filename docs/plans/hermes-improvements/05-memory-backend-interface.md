# 05 — Memory Backend Interface 추출

**Branch**: `feat/hermes-memory-backend-interface`
**Sprint**: S3
**Area**: `area/memory`
**Depends on**: —

## 배경

Hermes Agent는 Honcho / Holographic 같은 외부 메모리 플랫폼을 **플러그인 훅**으로 꽂는다. 덕분에 사용자가 메모리 백엔드를 실험·교체할 수 있다.

TARS는 현재 `internal/memory/`에 Gemini 임베딩 + 파일 기반 KB가 단일 구현으로 묶여 있다. "외부 메모리 플랫폼을 강제 의존으로 붙이기"는 TARS의 file-first 정체성을 훼손하므로 절대 하지 않는다. **그러나** Go interface만 추출해두면:

- 미래에 Mem0/Zep/Honcho 어댑터를 플러그인/MCP로 얹을 수 있는 "빈 콘센트"
- 파일 기반 구현을 테스트에서 mock으로 교체 가능
- `tool/memory_*` 계열 코드가 interface에 의존하게 되면서 구현 세부에서 분리됨

이 PR은 **interface 추출 + 기존 구현을 `FileBackend`로 wrap**하는 refactor이며, 기능은 하나도 바뀌지 않는다.

## 목적

1. `internal/memory/`에 `Backend` interface 신설.
2. 기존 `workspace.go`, `semantic.go`, `knowledge.go`, `experience.go`를 호출하는 adapter를 `FileBackend`로 구현.
3. `internal/tool/memory_*.go`가 구체 함수 대신 interface를 통해 메모리에 접근.
4. Config에 `memory_backend: file` 필드 추가. 현재 유효값은 `file`만 (향후 확장 자리).
5. 모든 기존 테스트가 **변경 없이** 통과.

## 현황 조사

(구현 착수 시 확인할 것)

- `internal/memory/` 파일 간 의존 그래프 (`workspace.go`, `semantic.go`, `knowledge.go`, `experience.go`)
- `internal/tool/memory_get.go`, `memory_save.go`, `memory_search.go`, `memory_kb.go`, `memory_aggregator.go`가 구체 패키지 함수를 어떻게 호출하고 있는지
- `internal/reflection/`이 `memory` 패키지를 어떻게 사용하는지 (야간 배치 경로 건드리면 안 됨)
- `tarsserver/handler_memory.go`의 호출 지점

## 제안 설계

### Interface

```go
// internal/memory/backend.go
package memory

import "context"

type Backend interface {
    // MEMORY.md 같은 durable prose
    LoadDurable(ctx context.Context, kind DurableKind) (string, error)
    SaveDurable(ctx context.Context, kind DurableKind, body string) error

    // Semantic search (Gemini 임베딩 기반)
    Search(ctx context.Context, query string, opts SearchOptions) ([]Match, error)

    // KB CRUD
    GetNote(ctx context.Context, id string) (Note, error)
    PutNote(ctx context.Context, note Note) error
    DeleteNote(ctx context.Context, id string) error
    ListNotes(ctx context.Context, filter NoteFilter) ([]NoteSummary, error)

    // Experience / daily logs
    AppendDailyLog(ctx context.Context, entry LogEntry) error
    AppendExperience(ctx context.Context, exp Experience) error
    ListExperiences(ctx context.Context, filter ExperienceFilter) ([]Experience, error)

    // Housekeeping (reflection 경로가 호출)
    Compile(ctx context.Context, scope CompileScope) (CompileResult, error)
    CleanupEmptySessions(ctx context.Context, olderThan time.Duration) (int, error)
}
```

**중요**: interface는 **기존 호출자가 실제로 쓰는 오퍼레이션만** 노출한다. 즉 추상화를 미리 만들지 않는다. 먼저 호출 그래프를 전수 조사해서 쓰이는 함수만 interface로 올린다.

### Adapter

```go
// internal/memory/file_backend.go
type FileBackend struct {
    rootDir string
    embed   EmbeddingClient
    // 기존 workspace/semantic/knowledge 관련 필드 흡수
}

func NewFileBackend(rootDir string, embed EmbeddingClient) *FileBackend { ... }

func (b *FileBackend) Search(ctx context.Context, q string, opts SearchOptions) ([]Match, error) {
    // 기존 semantic.go의 Search 로직을 그대로 호출
    return semanticSearch(b.rootDir, b.embed, q, opts)
}
// ... 나머지 오퍼레이션 동일
```

기존 `workspace.go`, `semantic.go` 등의 로직은 **한 줄도 바꾸지 않는다**. `FileBackend`는 얇은 래퍼.

### Tool 레이어의 interface 의존

```go
// 변경 전
func NewMemorySearchTool(workspace *memory.Workspace, embed *memory.GeminiEmbedder) Tool { ... }

// 변경 후
func NewMemorySearchTool(backend memory.Backend) Tool { ... }
```

`tool/memory_*.go` 전체가 `memory.Backend`를 받도록 시그니처 교체. 호출 측(`tarsserver/handler_chat_policy.go` 등)은 `memory.NewFileBackend(...)`로 생성해서 주입.

### Reflection 경로

`internal/reflection/`의 memory/kb_cleanup 잡도 동일하게 `memory.Backend`를 받도록. **주의**: reflection은 system surface이고 tool registry에 노출되지 않으므로 `RegistryScope` 검사와는 무관. 순수 Go 호출만.

### Config

```yaml
memory_backend: file  # 현재 유효값: file. 향후: mcp, external
```

`file` 이외 값이 오면 기동 시 fatal error. "미래 확장 자리"라는 것을 사용자에게 문서로 명시.

### 외부 어댑터는 이 PR에서 만들지 않는다

- Mem0/Zep/Honcho 어댑터는 실험해보고 싶을 때 별도 PR로 다룬다.
- 이 PR의 목표는 **interface 추출 + refactor + 기능 불변**.

## 수정 대상

### Backend
- `internal/memory/backend.go` — 신규, interface 정의
- `internal/memory/file_backend.go` — 신규, adapter
- `internal/memory/file_backend_test.go` — 신규, interface 준수 + 기존 동작 동등성
- `internal/memory/workspace.go`, `semantic.go`, `knowledge.go`, `experience.go` — 로직 불변, 외부 노출만 package-private로 정리 가능 (선택)
- `internal/tool/memory_get.go`, `memory_save.go`, `memory_search.go`, `memory_kb.go`, `memory_aggregator.go`, `knowledge_aggregator.go` — signature 교체
- `internal/tool/memory_*_test.go` — 테스트에서 `memory.Backend` mock 사용 가능 (기존 테스트는 `FileBackend` 그대로 주입)
- `internal/tarsserver/handler_memory.go` — backend 주입 경로
- `internal/tarsserver/handler_chat_policy.go` — backend 주입 경로
- `internal/tarsserver/main_bootstrap.go` (기동 경로) — `NewFileBackend` 생성 지점
- `internal/reflection/` 하위 파일들 — backend 주입
- `internal/config/config.go` + `config_input_fields.go` — `memory_backend` 필드

## 테스트 계획

### Unit
- `file_backend_test.go`: interface의 각 메서드가 기존 함수와 동일 결과를 내는지 확인하는 golden test
- Mock backend를 만들어 `tool/memory_*` 테스트 하나 정도 interface 교체 입증

### Integration
- 기존 memory 관련 통합 테스트 전부 그대로 통과해야 함 (기능 불변이 이 PR의 핵심)
- Reflection의 memory 잡 스모크 테스트

## Acceptance Criteria

- [ ] `memory.Backend` interface 정의
- [ ] `FileBackend`가 interface 구현, 기존 로직을 한 줄도 바꾸지 않고 래핑
- [ ] `internal/tool/memory_*.go` 전부 interface 의존으로 전환
- [ ] `internal/tarsserver/` 기동 경로에서 `FileBackend` 주입
- [ ] `internal/reflection/`이 backend 주입을 받도록 전환
- [ ] `memory_backend: file` 이외 값은 기동 시 fatal
- [ ] **기존 memory 테스트 전부 변경 없이 통과**
- [ ] `make test`, `make vet`, `make fmt` 통과

## Identity Check

- **단일 Go 바이너리**: 외부 의존성 추가 없음, interface만 추출 ✓
- **File-first**: 기본값 `file`, 기존 파일 구조 그대로 ✓
- **Scope isolation**: `memory.Backend`는 user/system 양쪽에서 쓰이지만 tool registry 경유가 아님. `RegistryScope` 보증 무영향 ✓
- **정책은 config, 메커니즘은 Go**: `memory_backend` 필드가 유일한 스위치 ✓
- **기능 불변**: 이 PR은 refactor. 사용자가 관찰하는 동작은 달라지지 않음 ✓

## 리뷰어 체크리스트

- [ ] 기존 memory 테스트가 한 줄도 수정되지 않고 통과했는지
- [ ] Interface가 "지금 실제로 쓰이는 오퍼레이션만" 노출하는지 (과도 추상화 방지)
- [ ] Reflection 경로가 여전히 system surface로 격리돼 있는지
- [ ] `memory_backend: file`이 유일한 유효값임이 문서화됐는지
