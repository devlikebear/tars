# Step 18. 상태 머신 + Autopilot

> 학습 목표: `PhaseEngine` 중심의 phase 상태 머신으로 자율 실행 루프를 구현

## 왜 상태 머신인가

Step 16에서 프로젝트와 태스크를 저장하는 Store를 만들었습니다. 하지만 태스크를 사람이 직접 만들고, 직접 상태를 바꿔야 합니다. AI 에이전트라면 이 과정을 자동화해야 합니다:

```
목표 입력 → 태스크 자동 생성 → 하나씩 실행 → 목표 달성 평가 → 완료 or 재계획
```

이걸 무작정 루프로 돌리면 상태 관리가 엉망이 됩니다. "지금 뭘 하고 있는지"를 명확히 하려면 **상태 머신**이 필요합니다.

## Phase 상태 전이도

```
planning ──→ executing ──→ reviewing ──→ done
    ↑             │              │
    └─ re-plan ───┴──── blocked ─┘

필요 시 → blocked (human decision / retry / re-plan)
```

각 phase에서 Autopilot이 하는 일:

| Phase | 동작 |
|-------|------|
| `planning` | 다음 phase 또는 backlog가 필요한지 판단 |
| `executing` | backlog item을 실행하거나 dispatch |
| `reviewing` | 결과를 평가하고 keep / re-plan / block 결정 |
| `blocked` | 사용자 승인, 추가 정보, 복구 대기 |
| `done` | 현재 phase 또는 run 종료 |

## 핵심 설계: 단일 PhaseEngine 의존점

현재 구현에서 외부 코드가 의존해야 할 것은 planner/selector/executor/evaluator 각각이 아니라 **`PhaseEngine` 하나**입니다:

```go
type PhaseSnapshot struct {
    ProjectID  string
    Phase      PhaseName
    Status     PhaseStatus
    Message    string
    NextAction string
}

type PhaseEngine interface {
    Start(ctx context.Context, projectID string) (AutopilotRun, error)
    Status(projectID string) (AutopilotRun, bool)
    Current(projectID string) (PhaseSnapshot, bool)
    Advance(ctx context.Context, projectID string) (PhaseSnapshot, error)
    Escalate(projectID, reason string) error
}
```

planner / selector / executor / evaluator는 `PhaseEngine` **내부 전략**입니다. heartbeat, API handler, dashboard는 이 내부 전략을 알지 못해야 coupling이 작아집니다.

## 실습

### 18-1. AutopilotRun 영속화

```go
type AutopilotRun struct {
    ProjectID  string `json:"project_id"`
    Status     string `json:"status"`     // running, blocked, done, failed
    Phase      string `json:"phase"`
    Message    string `json:"message,omitempty"`
    Iterations int    `json:"iterations"`
    StartedAt  string `json:"started_at,omitempty"`
    UpdatedAt  string `json:"updated_at,omitempty"`
}
```

`AUTOPILOT.json`에 매 상태 변경마다 저장합니다. 서버가 재시작해도 마지막 상태를 확인할 수 있습니다.

### 18-2. 메인 루프와 한-step 전진

```go
func (m *AutopilotManager) Advance(ctx context.Context, projectID string) (PhaseSnapshot, error) {
    // background loop를 무조건 띄우지 않고 현재 프로젝트를 한 번만 전진시킴
    // blocked 경로에서는 sleep 없이 즉시 snapshot 반환
}
```

`Advance()`를 따로 두면 dashboard, API, 테스트에서 phase 전이를 동기적으로 검증하기 쉽고, background heartbeat는 필요할 때만 별도로 보강할 수 있습니다.

### 18-3. step 함수 — phase별 디스패치

```go
func (m *AutopilotManager) runIteration(ctx context.Context, projectID string, iteration int, waitForRetry bool) autopilotStepResult {
    state, _ := m.store.GetState(projectID)
    switch state.Phase {
    case "planning":
        return m.handlePlanningPhase(...)
    case "executing":
        return m.handleDispatchableTasks(...)
    case "reviewing":
        return m.handleReviewPhase(...)
    case "blocked", "done":
        return autopilotStepResult{Stop: true}
    }
}
```

핵심은 **보드 상태가 엔진 그 자체가 아니라 projection**이라는 점입니다. `todo/review/in_progress`는 phase 내부의 작업 표현일 뿐, 외부가 의존해야 할 핵심 상태는 아닙니다.

### 18-4. planning fallback

현재 구현에서는 빈 보드를 즉시 seed하지 않습니다. 대신:

```go
if len(board.Tasks) == 0 {
    return m.planningRequired(projectID, run, "No backlog items remain for the current phase.", "Create or approve the next phase backlog")
}
```

즉 `empty board => auto-seed`가 아니라 `empty board => planning fallback` 입니다.

### 18-5. stepExecuting — 태스크 실행

```go
for i, task := range board.Tasks {
    if task.Status != "todo" { continue }

    board.Tasks[i].Status = "in_progress"
    err := a.runner(ctx, p.ID, task, taskDeliverables)

    if err != nil {
        // 재시도 카운터 증가
        // 3회 실패 시 스킵
        return false
    }

    board.Tasks[i].Status = "done"
    return false // 다음 iteration에서 다음 태스크
}
```

**한 iteration에 작은 단위만 전진**시키는 이유:
- 테스트와 dashboard가 phase 변화를 관찰하기 쉬움
- blocked / retry / human decision을 즉시 surface 가능
- background loop가 없어도 API에서 한 step씩 밀어볼 수 있음

**실패 재시도:** 같은 태스크가 3번 연속 실패하면 "done"으로 스킵 처리하고 `task_skipped` 활동 로그를 남깁니다. 초기 구현에서 API 에러가 나면 무한 재시도하는 문제가 있었습니다.

### 18-6. stepReviewing — 목표 평가

```go
done, reason, err := evaluator(...)
if done {
    // phase -> done
} else {
    // same-phase re-plan or blocked
}
```

평가 결과는 단순 완료/미완료만이 아니라, `same-phase re-plan`, `next-phase planning`, `blocked`를 구분할 수 있어야 합니다.

### 18-7. LLM 콜백 구현 (server.go)

**PlanGenerator:**

```go
func newLLMPlanGenerator(client llm.Client) project.PlanGenerator {
    return func(ctx context.Context, p *project.Project) (*project.PlanResult, error) {
        prompt := "You are a project planner..."
        resp, _ := client.Chat(ctx, messages, llm.ChatOptions{})
        // JSON 파싱 → PlanResult
    }
}
```

**TaskRunner:** agent loop을 사용합니다.

```go
func newLLMTaskRunner(...) project.TaskRunner {
    return func(ctx context.Context, projectID string, task project.BoardTask, deliverables []project.Deliverable) error {
        loop := agent.NewLoop(client, registry)
        resp, err := loop.Run(ctx, messages, agent.RunOptions{Tools: registry.Schemas()})
        // transcript에 결과 저장
    }
}
```

TaskRunner는 단순 Chat이 아니라 **Agent Loop**을 사용합니다. 도구를 호출하면서 태스크를 수행하기 때문입니다.

## 디버깅 팁

Autopilot 디버깅 시 서버 로그의 `[autopilot]` 프리픽스를 grep하면 상태 전이를 추적할 수 있습니다:

```bash
go run ./cmd/tars/ serve --config ... 2>&1 | grep "\[autopilot\]"
```

## 체크포인트

- [x] 외부 서버 코드는 `PhaseEngine` 하나에만 의존한다
- [x] `Advance()`가 현재 프로젝트를 한 step만 전진시킨다
- [x] 빈 보드는 auto-seed가 아니라 planning fallback으로 해석된다
- [x] dashboard가 phase / next action / blocker를 우선 표시한다

## 다음 단계

상태 머신이 돌아가기 시작했지만, 이제 중요한 건 사람 개입 지점을 최소화하면서도 명확히 드러내는 것입니다. Step 19에서는 planning 승인, blocker, 산출물 관점에서 Human-in-the-loop를 정리합니다.
