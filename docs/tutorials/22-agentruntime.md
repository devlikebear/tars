# Step 22. Agent Runtime (비동기 실행)

> 학습 목표: 비동기 Run 라이프사이클을 관리하는 Agent Runtime 런타임을 구현

## 왜 Agent Runtime인가

지금까지 TARS의 채팅(`/v1/chat`)은 **동기 SSE** 방식입니다. 클라이언트가 요청을 보내고, LLM 응답이 끝날 때까지 연결을 유지합니다. 이 방식은:

- 긴 작업에서 HTTP 타임아웃 위험
- 클라이언트가 연결을 끊으면 작업이 중단됨
- 여러 작업을 동시에 실행할 수 없음

**Agent Runtime**는 작업을 **비동기 Run**으로 관리합니다:

```
클라이언트                              Agent Runtime
    │ POST /v1/agentruntime/runs        │
    │ {"prompt":"분석해줘"}              │
    │ ──────────────────────────────────→│
    │                                    │ Run 생성 (accepted)
    │ ← 202 {"run_id":"run_1", ...}     │
    │                                    │ goroutine에서 실행 (running)
    │ GET /v1/agentruntime/runs/run_1   │
    │ ──────────────────────────────────→│
    │ ← 200 {"status":"completed",...}  │
```

클라이언트가 연결을 끊어도 Run은 서버에서 계속 실행됩니다.

## 핵심 개념

### Run 라이프사이클

```
accepted → running → completed
                   → failed
                   → canceled
```

- **accepted**: Spawn 직후, goroutine 시작 전
- **running**: executor 실행 중
- **completed**: 성공 (response 포함)
- **failed**: 에러 발생 (error 포함)
- **canceled**: Cancel API 또는 context 취소

### 구성 요소

```
Runtime (상태 관리)
    ├── runs map[string]*runState     ← 모든 Run 추적
    ├── executors map[string]Executor ← 실행기 등록
    └── channelMsgs                   ← 채널 메시지

AgentExecutor (인터페이스)
    └── PromptExecutor               ← agent.Loop 기반

Handler (HTTP API)
    ├── GET  /v1/agentruntime/agents     ← Agent list
    ├── POST /v1/agentruntime/runs       ← Spawn
    ├── GET  /v1/agentruntime/runs       ← List
    ├── GET  /v1/agentruntime/runs/{id}  ← Get
    ├── GET  /v1/agentruntime/runs/{id}/events ← Run event stream
    └── POST /v1/agentruntime/runs/{id}/cancel ← Cancel
```

`/v1/agent/agents`, `/v1/agent/runs`, `/v1/agent/runs/{id}`는 호환성을 위한 legacy alias입니다. 신규 문서와 외부 클라이언트는 `/v1/agentruntime/*`를 우선 사용합니다.

## 실습

### 22-1. Run 타입

```go
type Run struct {
    ID              string    `json:"run_id"`
    WorkspaceID     string    `json:"-"`
    SessionID       string    `json:"session_id,omitempty"`
    SessionKind     string    `json:"session_kind,omitempty"`
    Agent           string    `json:"agent,omitempty"`
    Prompt          string    `json:"prompt,omitempty"`
    ParentRunID     string    `json:"parent_run_id,omitempty"`
    ParentSessionID string    `json:"parent_session_id,omitempty"`
    Depth           int       `json:"depth,omitempty"`
    Status          RunStatus `json:"status"`
    Accepted        bool      `json:"accepted"`
    Response        string    `json:"response,omitempty"`
    Error           string    `json:"error,omitempty"`
    CreatedAt       string    `json:"created_at"`
    StartedAt       string    `json:"started_at,omitempty"`
    CompletedAt     string    `json:"completed_at,omitempty"`
    UpdatedAt       string    `json:"updated_at"`
}
```

현재 TARS의 run timestamp는 API/영속화 호환성을 위해 RFC3339 문자열로 저장합니다. `WorkspaceID`는 내부 라우팅용이라 JSON에는 노출하지 않습니다.

### 22-2. 런타임 내부 상태

```go
type runState struct {
    run    Run
    cancel context.CancelFunc  // 취소용
    done   chan struct{}        // 완료 신호
    closed bool
}
```

각 Run은 세 가지 동기화 수단을 가집니다:
- **cancel**: `context.WithCancel`에서 받은 취소 함수
- **done**: 실행 완료 시 닫히는 채널 (Wait에서 사용)
- **closed**: done 중복 close 방지 플래그

### 22-3. Spawn — 비동기 실행

```go
func (rt *Runtime) Spawn(ctx context.Context, req SpawnRequest) (*Run, error) {
    // 1. 동시 실행 제한 확인
    // 2. executor 선택
    // 3. Run 생성 (accepted)
    // 4. goroutine 시작
    // 5. Run 즉시 반환
}
```

**동시 실행 제한**은 서버 리소스 보호를 위해 필수입니다:

```go
active := 0
for _, rs := range rt.runs {
    if rs.run.Status == RunStatusAccepted || rs.run.Status == RunStatusRunning {
        active++
    }
}
if active >= maxConcurrentRuns {
    return nil, fmt.Errorf("too many concurrent runs")
}
```

**Run ID는 atomic counter**로 생성합니다:

```go
seq := rt.runSeq.Add(1)  // lock-free
runID := fmt.Sprintf("run_%d", seq)
```

`sync/atomic`은 mutex보다 가볍고, ID 생성처럼 단순한 카운터에 적합합니다.

### 22-4. 실행 goroutine

```go
func (rt *Runtime) executeRun(ctx context.Context, rs *runState, exec AgentExecutor) {
    defer func() {
        if !rs.closed {
            rs.closed = true
            close(rs.done)  // Wait() 해제
        }
    }()

    // running 상태로 전이
    rt.mu.Lock()
    rs.run.Status = RunStatusRunning
    rs.run.StartedAt = time.Now().UTC()
    rt.mu.Unlock()

    // 실행
    resp, err := exec.Execute(ctx, ...)

    // 결과에 따라 completed/failed/canceled
    rt.mu.Lock()
    if ctx.Err() != nil {
        rs.run.Status = RunStatusCanceled
    } else if err != nil {
        rs.run.Status = RunStatusFailed
    } else {
        rs.run.Status = RunStatusCompleted
        rs.run.Response = resp
    }
    rt.mu.Unlock()
}
```

**핵심**: `ctx.Err()`를 먼저 체크합니다. Cancel이 호출되면 executor도 에러를 반환하지만, 이 경우 `failed`가 아니라 `canceled`로 분류해야 합니다.

### 22-5. Cancel과 Wait

```go
func (rt *Runtime) Cancel(runID string) error {
    rs.cancel()  // context 취소 → executor에 전파
    return nil
}

func (rt *Runtime) Wait(runID string) (*Run, error) {
    <-rs.done    // goroutine이 close(done)할 때까지 블록
    return &rs.run, nil
}
```

Go의 context 취소가 goroutine까지 전파되는 흐름:

```
Cancel() → rs.cancel() → ctx 취소
                            ↓
                    executor의 select {
                    case <-ctx.Done(): return ctx.Err()
                    }
                            ↓
                    executeRun에서 ctx.Err() != nil
                            ↓
                    status = RunStatusCanceled
                            ↓
                    close(rs.done)
                            ↓
                    Wait()의 <-rs.done 해제
```

### 22-6. AgentExecutor 인터페이스

```go
type AgentExecutor interface {
    Name() string
    Execute(ctx context.Context, req ExecuteRequest) (string, error)
}
```

**PromptExecutor**는 기존 `agent.Loop`를 래핑합니다:

```go
func (e *PromptExecutor) Execute(ctx context.Context, req ExecuteRequest) (string, error) {
    messages := []llm.ChatMessage{
        {Role: "system", Content: systemPrompt},
        {Role: "user", Content: req.Prompt},
    }
    loop := agent.NewLoop(e.client, e.registry)
    resp, err := loop.Run(ctx, messages, agent.RunOptions{Tools: e.registry.Schemas()})
    return resp.Message.Content, err
}
```

인터페이스를 사용하면 향후 다른 실행기(외부 명령어, HTTP 프록시 등)를 쉽게 추가할 수 있습니다.

### 22-7. 채널 메시지

```go
func (rt *Runtime) MessageSend(channelID, text string) (*ChannelMessage, error) {
    msg := ChannelMessage{
        ID:        fmt.Sprintf("msg_%d", seq),
        ChannelID: channelID,
        Direction: "outbound",
        Source:    "local",
        Text:      text,
        Timestamp: time.Now().UTC(),
    }
    // 채널당 최대 100개, 초과 시 오래된 것 제거
    msgs = append(msgs, msg)
    if len(msgs) > maxChannelMessages {
        msgs = msgs[len(msgs)-maxChannelMessages:]
    }
}
```

채널 메시지는 에이전트가 외부 시스템에 메시지를 보낼 때 사용합니다. 현재는 `local` 소스만 지원하며, 향후 webhook/Telegram 등을 추가할 수 있습니다.

### 22-8. HTTP 핸들러

```go
// POST /v1/agentruntime/runs — 새 Run 생성
func (h *Handler) spawnRun(w http.ResponseWriter, r *http.Request) {
    var req SpawnRequest
    json.NewDecoder(r.Body).Decode(&req)
    run, err := h.rt.Spawn(r.Context(), req)
    writeJSON(w, http.StatusAccepted, run)  // 202 Accepted
}
```

**202 Accepted**를 반환합니다. 200이 아닌 이유: 요청을 수락했지만 아직 처리가 완료되지 않았기 때문입니다. REST API에서 비동기 작업의 표준 응답 코드입니다.

### 22-9. 메모리 관리 — Run 트리밍

```go
func (rt *Runtime) trimRunsLocked() {
    if len(rt.runOrder) <= maxRuns {
        return
    }
    // 완료된 Run부터 삭제 (활성 Run은 유지)
    for _, id := range rt.runOrder {
        if removed < target && !isActive(rs) {
            delete(rt.runs, id)
            removed++
        }
    }
}
```

메모리에만 상태를 유지하므로, maxRuns(200)을 초과하면 완료된 오래된 Run부터 제거합니다. 활성(accepted/running) Run은 절대 제거하지 않습니다.

### 22-10. Subagent compare mode

최신 TARS의 `subagents_run`은 기본 병렬 실행 외에 `mode: "compare"`를 지원합니다. 이 모드는 2-3개의 read-only subagent task가 같은 prompt를 독립적으로 실행하도록 강제하고, 도구 결과에 다음 비교 섹션을 함께 반환합니다.

- `common_findings`: 둘 이상의 subagent 출력에 함께 등장한 핵심 문장
- `conflicts`: 부정/긍정 문장이 같은 주제를 다르게 말하는 후보
- `evidence`: 각 subagent 출력에서 뽑은 run-id가 붙은 근거 조각
- `side_by_side`: 콘솔이 나란히 렌더링할 원문 출력

```json
{
  "mode": "compare",
  "tasks": [
    {"agent": "explorer", "title": "Explorer pass", "prompt": "원인 분석"},
    {"agent": "reviewer", "title": "Reviewer pass", "prompt": "원인 분석"}
  ]
}
```

Console Chat은 이 payload를 progress card로 렌더링하고 각 개별 run을 `/console/agentruntime/runs/{run_id}`로 연결합니다. 그래서 설계 검토나 root-cause analysis에서 "공통 결론", "충돌 후보", "근거", "원문"을 한 화면에서 비교할 수 있습니다.

## TARS 원본과의 차이

| 항목 | TARS 원본 | 최소 구현 |
|------|------|--------|
| 실행기 | PromptExecutor + CommandExecutor + workspace agent executor | PromptExecutor만 |
| 채널 | local + webhook + Telegram | local만 |
| 영속화 | runs.json + channels.json 파일 (`workspace/_shared/agentruntime/`) | 인메모리만 |
| 세션 라우팅 | caller/new/fixed 모드 | 미구현 |
| Sub-agent | 계층 구조 (parent/root/depth) | 미구현 |
| 도구 정책 | executor + agent frontmatter 정책 병합 | 미구현 |
| 아카이브 | JSONL 일별 로테이션 | 미구현 |

TARS는 핵심 패턴(비동기 Spawn/Wait, Run 라이프사이클, executor 인터페이스)만 구현합니다.

## 체크포인트

- [x] Run을 비동기로 생성하고 완료를 기다릴 수 있다
- [x] 동시 실행 제한이 동작한다 (4개 초과 시 거부)
- [x] Cancel API로 실행 중인 Run을 취소할 수 있다
- [x] 14개 테스트 케이스가 모두 통과한다

## 다음 단계

Step 23은 더 이상 별도 TUI로 이어지지 않습니다. 최신 TARS는 Agent Runtime API와 운영 기능을 `/console` 기반 웹 콘솔, one-shot CLI, cron/pulse/reflection surface로 노출합니다.
