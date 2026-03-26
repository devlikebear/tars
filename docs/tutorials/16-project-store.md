# Step 16. 프로젝트 Store + Kanban

> 학습 목표: 파일 기반 영속화와 YAML frontmatter 패턴으로 프로젝트/태스크를 관리하는 저장소 구현

## 왜 프로젝트 Store인가

Step 15에서 AI와 대화할 수 있게 되었습니다. 하지만 대화는 흘러가는 것이고, **작업을 구조화**하려면 프로젝트와 태스크라는 단위가 필요합니다.

```
"소나기 스타일 단편소설 써줘"
    → 프로젝트 생성 (이름, 목표)
    → 태스크 분해 (자료조사, 개요 작성, 본문 집필, 퇴고)
    → 진행 상태 추적 (todo → in_progress → done)
    → 활동 로그 기록
```

이 Step에서는 **순수 CRUD를 중심으로** 구현합니다. 자동 실행(Autopilot)은 Phase 6 범위입니다.

> 현재 코드베이스에는 이후 Step에서 재사용할 `Phase` 필드와 project tool 브리지가 일부 먼저 들어와 있습니다. 이 문서에서는 **저장소 구조와 CRUD, REST API**에만 집중합니다.

## 설계: 왜 파일 기반인가

데이터베이스 대신 **Markdown + JSON 파일**을 사용합니다:

| 파일 | 역할 | 형식 |
|------|------|------|
| `PROJECT.md` | 프로젝트 메타데이터 | YAML frontmatter + Markdown 본문 |
| `KANBAN.md` | 태스크 보드 | YAML frontmatter + 구조화된 목록 |
| `ACTIVITY.jsonl` | 활동 로그 | JSON Lines (한 줄에 하나씩) |

```
.workspace/projects/
├── 단편-소설-a1b2c3/
│   ├── PROJECT.md
│   ├── KANBAN.md
│   └── ACTIVITY.jsonl
└── api-서버-d4e5f6/
    ├── PROJECT.md
    ├── KANBAN.md
    └── ACTIVITY.jsonl
```

**이유:**
- git으로 버전 관리 가능
- 사람이 직접 읽고 편집 가능
- 외부 의존성(DB) 불필요
- TARS 원본도 동일한 패턴

## 원본 코드 분석 (TARS)

TARS의 `internal/project/` 패키지:

```
store.go           ← 프로젝트 CRUD + 보드 + 활동 로그
kanban.go          ← 칸반 보드 파싱/직렬화
activity.go        ← JSONL 활동 로그
workflow_policy.go ← 상태 전이 규칙
```

TARS에서는 이를 **`store.go` 하나로 통합**합니다. 학습 단계에서는 파일을 나누는 것보다 전체 흐름을 한 눈에 보는 것이 더 중요합니다.

현재 저장소의 `store.go`에는 Phase 6에서 사용할 `SetPhase`, `Deliverable` 관련 코드도 같이 들어 있습니다. 하지만 Step 16에서 꼭 이해해야 하는 핵심은 다음 3가지입니다:

1. `PROJECT.md` 파싱/직렬화
2. `KANBAN.md` 읽기/쓰기
3. `ACTIVITY.jsonl` append

## 실습

### 16-1. 데이터 모델

**`internal/project/store.go`**

```go
type Project struct {
    ID        string
    Name      string
    Status    string // active, paused, completed, archived, cancelled
    Phase     string // planning, awaiting_approval, executing, reviewing, ...
    Objective string
    Body      string // markdown 본문
    CreatedAt time.Time
    UpdatedAt time.Time
    Path      string
}

type BoardTask struct {
    ID     string
    Title  string
    Status string // todo, in_progress, review, done
}

type Board struct {
    ProjectID string
    UpdatedAt time.Time
    Columns   []string // ["todo","in_progress","review","done"]
    Tasks     []BoardTask
    Path      string
}

type Activity struct {
    ID        string
    ProjectID string
    Kind      string // project_created, board_task_updated, ...
    Message   string
    Timestamp time.Time
}
```

**설계 포인트:**
- `Project.Status`는 사용자 설정값 (active/archived)
- `Project.Phase`는 이후 상태 머신 확장을 위한 필드지만, Step 16에서는 기본값 `planning`만 사용
- `BoardTask.Status`는 고정 4단계: todo → in_progress → review → done

### 16-2. YAML Frontmatter 파싱

PROJECT.md의 형식:

```markdown
---
id: 단편-소설-a1b2c3
name: 단편 소설
status: active
phase: planning
objective: 소나기 스타일 단편소설 작성
created_at: 2026-03-25T10:00:00Z
updated_at: 2026-03-25T10:00:00Z
---

프로젝트 설명이나 메모를 여기에 작성합니다.
```

파싱 함수:

```go
func splitFrontmatter(raw string) (map[string]string, string) {
    meta := make(map[string]string)
    if !strings.HasPrefix(raw, "---\n") {
        return meta, raw
    }
    rest := raw[4:]
    idx := strings.Index(rest, "\n---")
    if idx < 0 {
        return meta, raw
    }
    header := rest[:idx]
    body := strings.TrimSpace(rest[idx+4:])

    for _, line := range strings.Split(header, "\n") {
        k, v, ok := strings.Cut(line, ":")
        if !ok {
            continue
        }
        meta[strings.TrimSpace(k)] = strings.TrimSpace(v)
    }
    return meta, body
}
```

**왜 YAML 라이브러리를 안 쓰나?** frontmatter는 `key: value` 한 줄씩이라 `strings.Cut`으로 충분합니다. 중첩 구조가 없으므로 외부 의존성을 추가할 필요가 없습니다.

### 16-3. 프로젝트 CRUD

**Create** — ID는 이름 slug + 3바이트 랜덤 hex:

```go
func newProjectID(name string) string {
    slug := strings.ToLower(strings.TrimSpace(name))
    slug = strings.ReplaceAll(slug, " ", "-")
    b := make([]byte, 3)
    rand.Read(b)
    return fmt.Sprintf("%s-%x", slug, b)
}
```

예: `"단편 소설"` → `"단편-소설-a1b2c3"`

프로젝트 생성 시 자동으로:
1. 프로젝트 디렉토리 생성
2. `PROJECT.md` 작성 (frontmatter + 빈 본문)
3. 기본 `KANBAN.md` 생성 (빈 보드, 4개 컬럼)
4. 활동 로그 기록 (`project_created`)

**List** — `projects/` 하위 디렉토리를 순회:

```go
func (s *Store) List() ([]*Project, error) {
    base := filepath.Join(s.workspaceDir, "projects")
    entries, err := os.ReadDir(base)
    // ...
    for _, e := range entries {
        if !e.IsDir() { continue }
        p, err := s.Get(e.Name())
        if err != nil { continue } // 손상된 프로젝트 건너뛰기
        projects = append(projects, p)
    }
    // UpdatedAt 기준 최신순 정렬
    sort.Slice(projects, func(i, j int) bool {
        return projects[i].UpdatedAt.After(projects[j].UpdatedAt)
    })
    return projects, nil
}
```

### 16-4. 칸반 보드

KANBAN.md 형식:

```markdown
---
project_id: 단편-소설-a1b2c3
updated_at: 2026-03-25T10:00:00Z
columns: [todo, in_progress, review, done]
---

- id: task-1
  title: 소나기 원작 분석
  status: done
- id: task-2
  title: 등장인물 설정
  status: in_progress
```

**상태 정규화** — 다양한 입력을 표준 값으로:

```go
func normalizeStatus(s string) string {
    switch strings.ToLower(s) {
    case "backlog":
        return "todo"
    case "doing", "in progress", "in-progress":
        return "in_progress"
    default:
        return strings.ToLower(s)
    }
}
```

사용자가 "doing"이라고 적어도 "in_progress"로 통일됩니다.

### 16-5. 활동 로그 (JSONL)

한 줄에 하나의 JSON 객체를 append하는 패턴:

```go
func (s *Store) AppendActivity(projectID, kind, message string) error {
    now := time.Now().UTC()
    a := Activity{
        ID:        fmt.Sprintf("act_%s_%03d", now.Format("20060102T150405"), now.Nanosecond()%1000),
        ProjectID: projectID,
        Kind:      kind,
        Message:   message,
        Timestamp: now,
    }
    data, _ := json.Marshal(a)
    f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
    if err != nil { return err }
    defer f.Close()
    _, err = fmt.Fprintf(f, "%s\n", data)
    return err
}
```

**왜 JSONL인가:**
- Append-only — 동시성 문제 없음
- 한 줄 손상되어도 나머지 읽기 가능
- `tail -f`로 실시간 모니터링 가능
- DB 없이 시계열 데이터 저장

### 16-6. REST API

**`internal/server/handler_dashboard.go`**

| Method | Path | 동작 |
|--------|------|------|
| GET | `/api/projects` | 프로젝트 목록 |
| POST | `/api/projects` | 프로젝트 생성 |
| GET | `/api/projects/{id}` | 프로젝트 상세 |
| PUT | `/api/projects/{id}` | 프로젝트 수정 |
| DELETE | `/api/projects/{id}` | 프로젝트 아카이브 |
| GET | `/api/projects/{id}/board` | 보드 조회 |
| PUT | `/api/projects/{id}/board` | 보드 업데이트 |
| GET | `/api/projects/{id}/activity` | 활동 로그 |

실제 핸들러에는 Autopilot/Approve/Reject 같은 후속 Step용 경로도 같이 들어 있지만, Step 16에서는 위 8개만 보면 됩니다.

라우팅은 `net/http`의 `ServeMux`만으로 구현합니다. 경로 파싱:

```go
func (h *DashboardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    path := strings.TrimPrefix(r.URL.Path, "/api/projects")
    path = strings.TrimPrefix(path, "/")

    switch r.Method {
    case http.MethodGet:
        h.handleGet(w, r, path)
    case http.MethodPost:
        h.handlePost(w, r, path)
    // ...
    }
}
```

`/api/projects/abc123/board` → `path = "abc123/board"` → `SplitN("/", 2)` → `["abc123", "board"]`

**JSON null 방어:** Go의 nil slice는 JSON으로 `null`이 됩니다. Swift 등 클라이언트가 decode에 실패하지 않도록 빈 배열로 강제합니다:

```go
if board.Tasks == nil {
    board.Tasks = []project.BoardTask{}
}
```

### 16-7. 채팅과 연결되는 지점

Step 15에서 만든 터미널 채팅 클라이언트가 Step 16의 저장소를 쓰려면, 저장소를 **LLM tool**로 노출해야 합니다.

**`internal/tool/project.go`**

- `project_create`
- `project_list`
- `project_get`
- `project_add_task`

예를 들어 사용자가 이렇게 말하면:

```text
소나기 스타일 단편소설 프로젝트 만들어줘
```

LLM은 `project_create`를 호출할 수 있고, 이 도구는 내부에서 `store.Create()`를 실행합니다.

```
자연어 요청
  → ChatHandler
  → agent loop
  → project_create tool
  → Store.Create()
  → PROJECT.md / KANBAN.md / ACTIVITY.jsonl 생성
```

이 흐름 덕분에 Step 16의 파일 기반 저장소가 Step 15의 채팅 인터페이스와 자연스럽게 이어집니다.

## 전체 구조

```
internal/project/
└── store.go          ← Project, Board, Activity + Store CRUD

internal/tool/
└── project.go        ← project_create / list / get / add_task

internal/server/
├── handler_dashboard.go  ← REST API 핸들러
├── handler_chat.go       ← 채팅 요청 처리
└── server.go             ← Store + Tool + Handler 연결

.workspace/projects/
└── {project-id}/
    ├── PROJECT.md
    ├── KANBAN.md
    └── ACTIVITY.jsonl
```

## 체크포인트

- [x] API로 프로젝트 생성/조회/수정이 가능하다
- [x] 태스크를 추가하고 상태를 변경할 수 있다
- [x] 활동 로그가 JSONL로 누적된다
- [x] `PROJECT.md`를 직접 열어 사람이 읽을 수 있다
- [x] 채팅에서 자연어 요청이 `project_create` tool로 연결된다

### curl로 테스트

```bash
# 프로젝트 생성
curl -s -X POST http://localhost:8080/api/projects \
  -H "Content-Type: application/json" \
  -d '{"name":"단편소설","objective":"소나기 스타일 단편소설 작성"}' | jq

# 프로젝트 목록
curl -s http://localhost:8080/api/projects | jq

# 보드 조회
curl -s http://localhost:8080/api/projects/{id}/board | jq

# 활동 로그
curl -s http://localhost:8080/api/projects/{id}/activity | jq

# 채팅에서 자연어로 프로젝트 생성
curl -N -X POST http://localhost:8080/v1/chat \
  -H "Content-Type: application/json" \
  -d '{"message":"소나기 스타일 단편소설 프로젝트 만들어줘"}'
```

### 자동 테스트

현재 코드베이스에는 Step 16 흐름을 고정하는 테스트가 추가되어 있습니다:

- `internal/project/store_test.go`
  - 프로젝트 생성 시 `PROJECT.md`, `KANBAN.md`, `ACTIVITY.jsonl`이 생성되는지 검증
  - `PROJECT.md`가 사람이 읽을 수 있는 frontmatter 문서인지 검증
- `internal/server/handler_dashboard_test.go`
  - `/api/projects`, `/board`, `/activity` CRUD 경로 검증
  - 빈 slice가 JSON `null`이 아니라 배열로 내려가는지 검증
- `internal/server/handler_chat_test.go`
  - 자연어 요청이 `project_create` tool 호출로 이어지고 실제 프로젝트가 생성되는지 검증

실행:

```bash
go test ./...
```

## 다음 단계

프로젝트와 태스크를 저장하고 조회하는 기반이 만들어졌습니다. 다음 Step에서는 macOS 메뉴바 대시보드를 만들어 이 데이터를 시각적으로 확인하고, 그 다음 Step에서 Autopilot과 승인 흐름이 같은 저장소 위에 올라가게 됩니다.
