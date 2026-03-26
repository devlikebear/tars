# Step 2. 세션과 Transcript

> 학습 목표: JSONL 기반으로 대화 이력을 영속화하는 패턴 이해

## 원본 코드 분석 (TARS)

TARS의 `internal/session/` 패키지는 4개 파일로 구성됩니다:

```
internal/session/
├── message.go      ← Message 구조체 (role, content, timestamp)
├── locks.go        ← 파일 경로별 mutex (동시 쓰기 보호)
├── session.go      ← Store: 세션 생성/조회/삭제, index 관리
└── transcript.go   ← JSONL append/read/rewrite
```

### 핵심 설계 포인트

**1. 세션 = 메타데이터, Transcript = 대화 내용 (분리)**
- `sessions/sessions.json` — 모든 세션의 메타 정보 (ID, 제목, 생성일)
- `sessions/{id}.jsonl` — 해당 세션의 메시지들 (한 줄에 하나씩)

**2. JSONL (JSON Lines) 패턴**
```
{"role":"user","content":"안녕","timestamp":"2026-03-22T10:00:00Z"}
{"role":"assistant","content":"반가워요","timestamp":"2026-03-22T10:00:01Z"}
```
- append가 빠름 (파일 끝에 한 줄 추가)
- 부분 읽기가 가능
- 파일이 중간에 깨져도 나머지 줄은 살릴 수 있음

**3. 파일 잠금** — `sync.Map`으로 경로별 mutex
- 같은 파일에 동시 쓰기를 막는 가벼운 in-process 락

## 실습

### 2-1. 메시지 타입

**`internal/session/message.go`**

```go
package session

import "time"

type Message struct {
    Role      string    `json:"role"`
    Content   string    `json:"content"`
    Timestamp time.Time `json:"timestamp"`
}
```

### 2-2. 세션 Store

세션 메타데이터를 JSON 인덱스 파일로 관리합니다.

**`internal/session/session.go`** 핵심 구조:

```go
type Session struct {
    ID        string    `json:"id"`
    Title     string    `json:"title"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

type Store struct {
    dir string  // workspace/sessions/
}

func NewStore(dir string) *Store {
    return &Store{dir: filepath.Join(dir, "sessions")}
}
```

**인덱스 관리:**
- `loadIndex()` — `sessions.json` 파일을 `map[string]Session`으로 읽기
- `saveIndex()` — map을 JSON으로 직렬화해서 저장
- 파일이 없으면 빈 map 반환 (에러가 아님)

**ID 생성:**
```go
func generateID() (string, error) {
    raw := make([]byte, 8)
    if _, err := rand.Read(raw); err != nil {
        return "", err
    }
    return hex.EncodeToString(raw), nil  // 16자리 hex
}
```
`crypto/rand`를 사용해 충돌 가능성이 극히 낮은 ID를 만듭니다.

**원본과 비교해서 생략한 것:**
- `lockPath()` — 동시성 보호 (나중에 추가 가능)
- `Kind`, `Hidden`, `ProjectID` — 프로젝트 연동 전까지 불필요
- `EnsureMain()`, `EnsureWorker()` — 단일 세션으로 충분

### 2-3. Transcript (JSONL 읽기/쓰기)

**`internal/session/transcript.go`** 핵심 두 함수:

```go
// 한 줄 추가
func AppendMessage(path string, msg Message) error {
    f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
    // ...
    data, _ := json.Marshal(msg)
    f.Write(append(data, '\n'))
}

// 전체 읽기
func ReadMessages(path string) ([]Message, error) {
    f, err := os.Open(path)
    if os.IsNotExist(err) {
        return nil, nil  // 파일 없으면 빈 결과
    }
    scanner := bufio.NewScanner(f)
    for scanner.Scan() {
        json.Unmarshal(scanner.Bytes(), &msg)
        messages = append(messages, msg)
    }
}
```

**포인트:**
- `O_APPEND` — OS 레벨에서 원자적 append 보장
- 파일이 없으면 `nil, nil` 반환 — 에러가 아니라 "아직 대화 없음"

### 2-4. 테스트

7개 테스트로 검증합니다:

| 테스트 | 검증 내용 |
|--------|-----------|
| `CreateAndGet` | 세션 생성 → ID로 다시 조회 |
| `ListSessions` | 여러 세션 생성 후 목록 조회 |
| `TranscriptAppendAndRead` | JSONL에 쌓고 순서대로 읽기 |
| `FileNotExist` | 없는 파일 읽으면 nil, 에러 없음 |
| `TranscriptPath` | Store와 transcript 연동 |
| `NotFound` | 없는 세션 조회 시 에러 |
| `EmptyFile` | 빈 파일도 안전하게 처리 |

```bash
go test ./internal/session/ -v
```

## 체크포인트

- [x] 새 세션 생성 후 다시 읽을 수 있다
- [x] 사용자/assistant 메시지가 JSONL에 순서대로 쌓인다
- [x] 파일이 없어도 에러 없이 빈 결과를 반환한다

## 배운 패턴

- **인덱스와 데이터 분리** — `sessions.json`(메타) + `{id}.jsonl`(내용)
- **JSONL** — append-friendly, 부분 읽기 가능, 깨져도 복구 쉬움
- **Graceful 빈 상태 처리** — 에러가 아니라 nil 반환
