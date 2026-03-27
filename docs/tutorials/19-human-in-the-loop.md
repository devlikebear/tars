# Step 19. Human-in-the-loop + 산출물

> 학습 목표: planning 승인, blocker, phase 재계획 관점에서 Human-in-the-loop를 정리

## 왜 Human-in-the-loop인가

Step 18에서 Autopilot이 자동으로 계획하고 실행하는 루프를 만들었습니다. 하지만 AI가 만든 계획을 **검토 없이 바로 실행하면 위험**합니다:

- 엉뚱한 태스크를 만들 수 있음
- 의도하지 않은 파일을 생성할 수 있음
- API 비용이 낭비될 수 있음

그래서 사람 개입은 넓게 퍼뜨리지 않고, 최소한 아래 세 지점으로 모읍니다:

- 첫 phase 계획 승인
- terminal blocker 해소
- 다음 phase로 넘어가기 전 재계획 승인

## 제어 API / 상태 갱신

지금 구현의 핵심은 별도 approve/reject 전용 phase를 늘리는 것이 아니라, `STATE.md`와 `AUTOPILOT.json`에 **사람의 결정이 필요한 상태를 명시**하는 것입니다.

대표적인 상태는 다음과 같습니다.

| 상태 | 의미 |
|------|------|
| `planning + active` | 초안 정리 중, 아직 자동 실행 전 |
| `planning + blocked` | 다음 backlog/phase 승인 필요 |
| `executing + blocked` | 실행 중 blocker 발생 |
| `done` | 현재 phase 또는 run 종료 |

### 예시: planning-ready 응답

```go
POST /v1/project-briefs/{session_id}/finalize

{
  "project": {...},
  "brief": {...},
  "state": {
    "phase": "planning",
    "status": "active",
    "next_action": "Review project instructions and define the first executable milestone in STATE.md."
  },
  "planning_ready": true
}
```

이 응답의 의미는 “프로젝트는 만들어졌고, 실행 전에 planning 검토가 가능하다”입니다. 예전처럼 `seeded=true`로 즉시 backlog 실행을 암시하지 않습니다.

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

사용자는 전용 approve phase보다, dashboard의 `Phase Note`, `Pending Decision`, `Current Blocker`를 통해 현재 어떤 결정을 내려야 하는지 확인합니다:

```
Phase: planning
Status: blocked
Pending Decision: Approve the first phase backlog
Current Blocker: No backlog items remain for the current phase.
```
이 정보를 보고 사용자는 `STATE.md`를 보강하거나, 관련 project tool로 다음 계획을 승인/수정합니다.

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
1. 사용자가 brief를 정리하고 finalize
2. project는 planning-ready 상태로 생성
3. 사람과 TARS가 첫 phase 목표와 backlog를 조율
4. 실행 시작 후 autopilot은 backlog item을 진행
5. backlog가 비거나 blocker가 생기면 planning / blocked로 복귀
6. 사람은 승인 또는 필요한 정보만 제공하고, 다시 자율 실행으로 넘김
```

## 체크포인트

- [x] brief finalize 응답이 planning-ready state를 함께 돌려준다
- [x] dashboard가 pending decision / blocker를 우선 노출한다
- [x] empty board는 auto-seed가 아니라 planning fallback으로 처리된다
- [ ] 각 도메인에서 산출물 평가 규칙을 더 명확히 분리한다

> output/ 파일 생성은 LLM이 `write_file` 도구를 올바르게 호출하는지에 의존합니다. 경로를 프롬프트에 명시했지만, 모델에 따라 도구 호출을 건너뛸 수 있습니다.

## 다음 단계

다음 단계는 이 상태 변경을 실시간으로 더 잘 보이게 만드는 것입니다. Step 20에서는 phase, blocker, run 상태를 SSE로 투영하는 흐름을 다룹니다.
