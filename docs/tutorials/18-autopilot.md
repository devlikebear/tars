# Step 18. 상태 머신 + Autopilot

> 학습 목표: phase 기반 상태 머신과 LLM 콜백으로 자율 실행 루프를 구현

## 왜 상태 머신인가

Step 16에서 프로젝트와 태스크를 저장하는 Store를 만들었습니다. 하지만 태스크를 사람이 직접 만들고, 직접 상태를 바꿔야 합니다. AI 에이전트라면 이 과정을 자동화해야 합니다:

```
목표 입력 → 태스크 자동 생성 → 하나씩 실행 → 목표 달성 평가 → 완료 or 재계획
```

이걸 무작정 루프로 돌리면 상태 관리가 엉망이 됩니다. "지금 뭘 하고 있는지"를 명확히 하려면 **상태 머신**이 필요합니다.

## Phase 상태 전이도

```
planning ──→ awaiting_approval ──→ executing ──→ reviewing ──→ completed
    ↑                                               │
    └───────────────── re-planning ←────────────────┘
                    (goal not met)

어디서든 → paused (Resume으로 복귀)
어디서든 → cancelled (종료)
```

각 phase에서 Autopilot이 하는 일:

| Phase | 동작 |
|-------|------|
| `planning` | LLM에게 태스크+산출물 계획 요청 |
| `awaiting_approval` | 대기 (사용자 승인/거절) |
| `executing` | todo 태스크를 하나씩 실행 |
| `reviewing` | LLM에게 목표 달성 여부 평가 |
| `completed` | 루프 종료 |
| `paused` | 대기 (사용자 Resume) |
| `cancelled` | 루프 종료 |

## 핵심 설계: 콜백 패턴

Autopilot은 **LLM을 직접 호출하지 않습니다.** 대신 3개의 콜백을 주입받습니다:

```go
type PlanGenerator func(ctx context.Context, project *Project) (*PlanResult, error)
type TaskRunner    func(ctx context.Context, projectID string, task BoardTask, deliverables []Deliverable) error
type GoalEvaluator func(ctx context.Context, project *Project, completedTasks []BoardTask) (done bool, reason string, err error)
```

**왜 콜백인가?**
- Autopilot은 "상태 전이 규칙"만 알면 됨
- LLM 호출 방법은 서버(`server.go`)가 결정
- 테스트 시 Mock 콜백을 주입할 수 있음
- Provider가 바뀌어도 Autopilot 코드를 수정할 필요 없음

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

### 18-2. 메인 루프

```go
func (a *Autopilot) loop(ctx context.Context, projectID string) {
    for iteration := 1; ; iteration++ {
        if ctx.Err() != nil { return }

        p, _ := a.store.Get(projectID)
        if p.Phase == "awaiting_approval" || p.Phase == "paused" {
            // 대기 상태: 로그 없이 2초마다 체크
            a.step(ctx, projectID, iteration)
            time.Sleep(2 * time.Second)
            continue
        }

        // 활성 상태: 로그 출력 + 10초 간격
        stop := a.step(ctx, projectID, iteration)
        if stop { return }
        time.Sleep(a.interval)
    }
}
```

**대기 상태 최적화:** `awaiting_approval`과 `paused`에서는 iteration 로그를 출력하지 않습니다. 초기 구현에서 이 처리가 없어 "Waiting for human approval..." 로그가 수백 줄 찍히는 문제가 있었습니다.

### 18-3. step 함수 — phase별 디스패치

```go
func (a *Autopilot) step(ctx context.Context, projectID string, iteration int) bool {
    p, _ := a.store.Get(projectID)
    switch p.Phase {
    case "planning":
        return a.stepPlanning(ctx, p, iteration)
    case "awaiting_approval":
        // 상태 메시지 1회만 출력
        return false
    case "executing":
        return a.stepExecuting(ctx, p, iteration)
    case "reviewing":
        return a.stepReviewing(ctx, p, iteration)
    case "completed", "cancelled":
        return true // 루프 종료
    }
}
```

`step`이 `true`를 반환하면 루프가 종료됩니다.

### 18-4. stepPlanning — 계획 생성

```go
plan, err := a.planner(ctx, p)
// plan.Tasks → board에 저장
// plan.Deliverables → DELIVERABLES.json에 저장
// phase → "awaiting_approval"
```

PlanGenerator가 반환하는 `PlanResult`에는 태스크와 산출물 명세가 함께 들어있습니다. (산출물 상세는 Step 19에서)

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

**한 iteration에 한 태스크만 실행합니다.** 이유:
- LLM 호출이 오래 걸릴 수 있음
- 중간에 pause/cancel 체크 가능
- SSE로 진행 상태를 실시간 전파 가능

**실패 재시도:** 같은 태스크가 3번 연속 실패하면 "done"으로 스킵 처리하고 `task_skipped` 활동 로그를 남깁니다. 초기 구현에서 API 에러가 나면 무한 재시도하는 문제가 있었습니다.

### 18-6. stepReviewing — 목표 평가

```go
done, reason, err := a.evaluator(ctx, p, completed)
if done {
    // phase → "completed"
    return true
}
// phase → "planning" (재계획)
return false
```

GoalEvaluator가 `false`를 반환하면 다시 `planning`으로 돌아갑니다. 이것이 **재계획 루프**입니다.

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

- [x] autopilot이 planning → awaiting_approval로 전이한다
- [x] 승인 후 executing에서 태스크를 자동 실행한다
- [x] 3회 연속 실패 시 태스크를 스킵한다
- [x] reviewing에서 목표 미달 시 re-planning이 동작한다

## 다음 단계

상태 머신이 돌아가지만, `awaiting_approval`에서 승인할 방법이 API뿐입니다. Step 19에서 Human-in-the-loop 제어와 산출물 시스템을 만듭니다.
