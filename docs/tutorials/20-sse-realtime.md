# Step 20. SSE 실시간 이벤트

> 학습 목표: Server-Sent Events로 서버 상태를 클라이언트에 실시간 푸시

## 왜 SSE인가

Step 18-19에서 Autopilot이 상태를 바꾸고, 사용자가 승인/거절을 합니다. 하지만 대시보드가 이 변화를 **알 방법이 없습니다.** 폴링(5초마다 API 호출)으로 해결할 수 있지만:

- 불필요한 API 호출이 많음
- 5초 지연이 생김
- 서버 부하 증가

**SSE(Server-Sent Events)**는 HTTP 연결을 열어두고 서버가 **이벤트를 푸시**하는 프로토콜입니다. WebSocket보다 단순하고, 단방향(서버→클라이언트) 통신에 적합합니다.

## SSE 프로토콜

```
GET /api/stream
→ 200 OK
→ Content-Type: text/event-stream

event: phase_changed
data: {"project_id":"abc","phase":"executing"}

event: board_updated
data: {"project_id":"abc"}

event: autopilot_status
data: {"project_id":"abc","status":"running","phase":"executing","message":"Executing: task-1"}
```

규칙:
- `event:` 줄이 이벤트 타입
- `data:` 줄이 JSON 페이로드
- 빈 줄이 이벤트 구분자
- 연결은 클라이언트가 끊을 때까지 유지

## 실습

### 20-1. SSEBroker — Go 서버

**`internal/server/sse_broker.go`**

Pub/Sub 패턴: 클라이언트가 Subscribe, Autopilot이 Broadcast.

```go
type SSEEvent struct {
    Type string `json:"type"`
    Data any    `json:"data"`
}

type SSEBroker struct {
    mu      sync.RWMutex
    clients map[chan SSEEvent]struct{}
}
```

**Subscribe:** HTTP 핸들러가 채널을 등록하고 이벤트를 기다립니다.

```go
func (b *SSEBroker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")

    ch := make(chan SSEEvent, 16)
    b.subscribe(ch)
    defer b.unsubscribe(ch)

    for {
        select {
        case <-r.Context().Done():
            return
        case event := <-ch:
            data, _ := json.Marshal(event.Data)
            fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, data)
            flusher.Flush()
        }
    }
}
```

**Broadcast:** 모든 클라이언트 채널에 이벤트를 보냅니다.

```go
func (b *SSEBroker) Broadcast(event SSEEvent) {
    b.mu.RLock()
    defer b.mu.RUnlock()
    for ch := range b.clients {
        select {
        case ch <- event:
        default: // 채널이 가득 차면 스킵
        }
    }
}
```

채널 버퍼(16)가 가득 차면 이벤트를 버립니다. 느린 클라이언트 때문에 서버가 블록되는 걸 방지합니다.

### 20-2. Autopilot → SSEBroker 연결

```go
autopilot.SetEmitter(func(eventType string, data any) {
    sseBroker.Broadcast(SSEEvent{Type: eventType, Data: data})
})
```

Autopilot은 `EventEmitter` 콜백만 호출합니다. SSE 프로토콜을 알 필요가 없습니다.

### 20-3. 이벤트 종류

| 이벤트 | 발생 시점 | 클라이언트 동작 |
|--------|----------|----------------|
| `phase_changed` | phase 전이 | 프로젝트 목록 새로고침 |
| `board_updated` | 태스크 변경 | 보드 새로고침 |
| `activity` | 태스크 완료 등 | 활동 로그 새로고침 |
| `autopilot_status` | 상태 변경마다 | Autopilot 상태 업데이트 |

### 20-4. Swift SSE 클라이언트

**`dashboard/Sources/APIClient.swift`**

`URLSession`으로 SSE 스트림을 읽습니다.

```swift
private let sseSession: URLSession = {
    let config = URLSessionConfiguration.default
    config.timeoutIntervalForRequest = 3600  // 1시간
    config.timeoutIntervalForResource = 3600
    return URLSession(configuration: config)
}()
```

**핵심:** `URLSession.shared`의 기본 타임아웃은 60초입니다. SSE처럼 데이터가 간헐적으로 오는 long-lived 연결에서는 타임아웃됩니다. 전용 세션을 만들어 1시간으로 설정합니다.

```swift
private func listenSSE() async {
    let (bytes, _) = try await sseSession.bytes(from: url)

    var eventType = ""
    for try await line in bytes.lines {
        if line.hasPrefix("event: ") {
            eventType = String(line.dropFirst(7))
        } else if line.hasPrefix("data: ") {
            let data = String(line.dropFirst(6))
            handleSSEEvent(type: eventType, data: data)
            eventType = ""
        }
    }
}
```

SSE 파싱은 Step 15의 터미널 채팅 클라이언트와 **동일한 패턴**입니다. `event:` → `data:` → 처리.

### 20-5. 이벤트 처리 — 데이터 새로고침

```swift
private func handleSSEEvent(type: String, data: String) {
    switch type {
    case "phase_changed", "board_updated", "activity":
        Task {
            await fetchProjects()
            if let board = currentBoard {
                await fetchBoard(projectID: board.projectID)
            }
        }
    default:
        break
    }
}
```

이벤트를 받으면 관련 데이터를 다시 fetch합니다. 이벤트 페이로드에 전체 데이터를 넣지 않고 **"변경됐다"는 신호만** 보내는 것이 핵심입니다.

### 20-6. 자동 재연결

```swift
} catch {
    if !Task.isCancelled {
        try? await Task.sleep(for: .seconds(3))
        if !Task.isCancelled {
            await listenSSE() // 재귀 호출로 재연결
        }
    }
}
```

연결이 끊기면 3초 후 자동 재연결합니다.

### 20-7. Stale 데이터 문제와 해결

Phase 5에서 겪은 주요 버그: 대시보드의 프로젝트 상세 화면이 **navigation 시점의 스냅샷**을 보여주고, SSE 이벤트로 데이터가 갱신되어도 화면이 바뀌지 않았습니다.

**원인:**
```swift
// App.swift
case .detail(let project):  // ← project는 navigation 시점의 복사본
    ProjectDetailView(project: project, ...)
```

**해결:** project 객체 대신 **ID만 저장**하고, 매번 최신 데이터에서 조회:

```swift
case .detail(let projectID):
    if let project = client.projects.first(where: { $0.id == projectID }) {
        ProjectDetailView(project: project, ...)
    }
```

이렇게 하면 SSE 이벤트로 `client.projects`가 갱신될 때 SwiftUI가 자동으로 뷰를 다시 렌더링합니다.

## 전체 데이터 흐름

```
Autopilot (상태 변경)
    → EventEmitter 콜백
    → SSEBroker.Broadcast()
    → 연결된 모든 클라이언트 채널
    → HTTP text/event-stream 응답
    → Swift URLSession.bytes.lines
    → handleSSEEvent → fetchProjects/fetchBoard
    → SwiftUI @Observable → 뷰 갱신
```

## 체크포인트

- [x] 서버 이벤트가 대시보드에 실시간 반영된다
- [x] 연결 끊김 시 자동 재연결된다
- [x] SSE 연결이 60초 이상 안정적으로 유지된다

## 다음 단계

Phase 6이 완성되었습니다. 프로젝트를 생성하고, AI가 자율적으로 계획/실행하며, 사용자가 대시보드에서 실시간으로 감독할 수 있습니다. Phase 7에서는 인증, 비동기 실행, TUI 클라이언트 등 운영 환경에 필요한 기능을 추가합니다.
