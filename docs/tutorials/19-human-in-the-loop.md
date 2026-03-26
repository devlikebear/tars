# Step 19. Human-in-the-loop + 산출물

> 학습 목표: 사용자 승인 게이트와 LLM 기반 산출물 명세 시스템을 구현

## 왜 Human-in-the-loop인가

Step 18에서 Autopilot이 자동으로 계획하고 실행하는 루프를 만들었습니다. 하지만 AI가 만든 계획을 **검토 없이 바로 실행하면 위험**합니다:

- 엉뚱한 태스크를 만들 수 있음
- 의도하지 않은 파일을 생성할 수 있음
- API 비용이 낭비될 수 있음

그래서 `planning → executing` 사이에 **`awaiting_approval` 게이트**를 둡니다. 사용자가 계획을 확인하고 Approve/Reject를 결정합니다.

## 제어 API

5개의 POST 엔드포인트:

| 엔드포인트 | 전이 | 조건 |
|-----------|------|------|
| `POST /api/projects/{id}/approve` | awaiting_approval → executing | phase == awaiting_approval |
| `POST /api/projects/{id}/reject` | awaiting_approval → planning | phase == awaiting_approval |
| `POST /api/projects/{id}/pause` | * → paused | phase != completed/cancelled |
| `POST /api/projects/{id}/resume` | paused → executing | phase == paused |
| `POST /api/projects/{id}/cancel` | * → cancelled | 언제든 |

### Approve

```go
func (a *Autopilot) Approve(projectID string) error {
    p, _ := a.store.Get(projectID)
    if p.Phase != "awaiting_approval" {
        return fmt.Errorf("cannot approve: phase is %s", p.Phase)
    }
    a.store.SetPhase(projectID, "executing")
    a.emit("phase_changed", ...)
    return nil
}
```

### Reject

```go
func (a *Autopilot) Reject(projectID string) error {
    // 보드 초기화 (기존 태스크 삭제)
    a.store.UpdateBoard(projectID, []BoardTask{})
    // planning으로 되돌림 → LLM이 다시 계획 생성
    a.store.SetPhase(projectID, "planning")
    return nil
}
```

Reject 시 보드를 **완전히 초기화**합니다. LLM이 처음부터 다시 계획을 세우게 됩니다.

## 산출물 (Deliverables)

### 문제: 프로젝트마다 산출물이 다르다

- 소설 프로젝트 → `story.md`, `outline.md`
- 코딩 프로젝트 → `main.go`, `handler.go`, `README.md`
- 이미지 프로젝트 → `concept.md`, `prompt.txt`

산출물의 개수, 파일명, 포맷을 하드코딩할 수 없습니다. **LLM이 계획 단계에서 결정**하게 합니다.

### Deliverable 타입

```go
type Deliverable struct {
    ID          string `json:"id"`
    Path        string `json:"path"`        // output/ 기준 상대경로
    Format      string `json:"format"`      // md, go, py, png, ...
    Description string `json:"description"` // 설명
    TaskID      string `json:"task_id"`     // 어떤 태스크가 생성하는지
}
```

### 계획 단계에서 생성

PlanGenerator가 태스크와 산출물을 **함께** 생성합니다:

```go
type PlanResult struct {
    Tasks        []BoardTask   `json:"tasks"`
    Deliverables []Deliverable `json:"deliverables"`
}
```

LLM 프롬프트:

```
Generate 3-7 actionable tasks AND a list of deliverables.
Respond in JSON:
{
  "tasks": [...],
  "deliverables": [
    {"id":"d-1","path":"outline.md","format":"md",
     "description":"소설 구성 개요","task_id":"task-2"}
  ]
}
```

### 승인 화면에서 확인

사용자는 `awaiting_approval` 상태에서 태스크 목록 **+ 산출물 명세**를 함께 봅니다:

```
Tasks:
  - task-1: 소나기 원작 분석
  - task-2: 개요 작성
  - task-3: 본문 집필

Deliverables:
  📄 outline.md — 소설 구성 개요 (md)
  📄 story.md — 완성된 단편소설 (md)
```

이 정보를 보고 Approve/Reject를 결정합니다.

### 실행 시 산출물 경로 전달

TaskRunner에 해당 태스크의 산출물 목록을 전달합니다:

```go
// stepExecuting에서
deliverables, _ := a.store.GetDeliverables(p.ID)
var taskDeliverables []Deliverable
for _, d := range deliverables {
    if d.TaskID == task.ID {
        taskDeliverables = append(taskDeliverables, d)
    }
}
a.runner(ctx, p.ID, task, taskDeliverables)
```

TaskRunner의 시스템 프롬프트에 산출물 경로가 포함됩니다:

```
Deliverables to produce:
- d-2: 소설 구성 개요 (format: md)
  → write to: .workspace/projects/{id}/output/outline.md

IMPORTANT: Use the write_file tool to save each deliverable.
```

### 저장 구조

```
.workspace/projects/{id}/
├── PROJECT.md
├── KANBAN.md
├── ACTIVITY.jsonl
├── DELIVERABLES.json      ← 산출물 명세
├── AUTOPILOT.json         ← 실행 상태
└── output/                ← 산출물 파일
    ├── outline.md
    └── story.md
```

## 전체 흐름

```
1. 사용자가 Autopilot Start
2. planning: LLM이 tasks + deliverables 생성
3. awaiting_approval: 대시보드에서 확인
   ├─ Approve → executing
   └─ Reject → planning (보드 초기화, 재계획)
4. executing: 태스크별로 실행, deliverable 경로를 프롬프트에 포함
5. reviewing: 목표 달성 평가
   ├─ done → completed
   └─ not done → planning (재계획)
```

## 체크포인트

- [x] 대시보드에서 Approve/Reject 버튼이 표시된다
- [x] 승인 화면에서 산출물 명세를 확인할 수 있다
- [x] Reject 시 태스크가 초기화되고 re-planning이 진행된다
- [ ] 실행 완료 후 output/ 디렉토리에 산출물 파일이 존재한다

> output/ 파일 생성은 LLM이 `write_file` 도구를 올바르게 호출하는지에 의존합니다. 경로를 프롬프트에 명시했지만, 모델에 따라 도구 호출을 건너뛸 수 있습니다.

## 다음 단계

승인/거절과 산출물 시스템이 완성되었습니다. Step 20에서는 이 모든 상태 변경을 대시보드에 **실시간으로** 전달하는 SSE 이벤트 시스템을 만듭니다.
