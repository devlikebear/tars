# 모듈: 프로젝트 오토파일럿과 대시보드

## 핵심 파일

- `internal/project/activity.go`
- `internal/project/activity_auto.go`
- `internal/project/brief_state.go`
- `internal/project/policy.go`
- `internal/project/workflow_policy.go`
- `internal/project/workflow_runtime_policy.go`
- `internal/project/kanban.go`
- `internal/project/orchestrator.go`
- `internal/project/orchestrator_plan.go`
- `internal/project/store_normalize.go`
- `internal/project/task_report.go`
- `internal/project/worker_profiles.go`
- `internal/tarsserver/helpers_project_progress.go`
- `internal/tarsserver/dashboard.go`
- `internal/tarsserver/main_serve_api.go`
- `internal/tarsserver/handler_chat.go`
- `plugins/project-swarm/skills/project-start/SKILL.md`
- `plugins/project-swarm/skills/project-autopilot/SKILL.md`

## 역할

이 모듈은 채팅으로 시작한 프로젝트를 workspace 문서 집합으로 바꾸고, backlog planning과 task dispatch/review를 수행하며, heartbeat 후크가 autonomous project를 진전시키고, 서버가 그 상태를 콘솔/대시보드로 노출하는 계층이다. 한마디로 말하면 "문서 기반 PM workflow 엔진"이다.

## 저장 문서 구조

- `workspace/_shared/project_briefs/<session>/BRIEF.md`
- `workspace/projects/<project>/PROJECT.md`
- `workspace/projects/<project>/STATE.md`
- `workspace/projects/<project>/KANBAN.md`
- `workspace/projects/<project>/ACTIVITY.jsonl`
- `workspace/projects/<project>/AUTOPILOT.json`

즉, 이 모듈의 truth source는 DB가 아니라 파일 문서 묶음이다.

## 정책, dispatch, supervision 분리

이번 증분에서 역할 분리가 조금 더 명확해졌다.

- `workflow_policy.go`: brief/project 상태 normalize, 기본 next action, blocked/done 전이 규칙
- `policy.go`: 프로젝트 tool allow/deny/pattern/risk 정책과 prompt context 렌더링
- `orchestrator.go`: todo/review dispatch, verification gate, activity 기록
- `orchestrator_plan.go`: planner run을 통해 backlog 후보 task를 생성
- `helpers_project_progress.go`: heartbeat 뒤에 manual/autonomous project를 어떻게 전진시킬지 결정
- `dashboard.go`: board/activity/state를 읽기 전용 projection으로 변환

즉, 한 파일에 몰려 있던 규칙이 일부 빠져나왔지만, 전체 workflow 모델은 아직 여러 파일에 걸쳐 있다. 특히 chat kickoff, planner run, autonomous heartbeat progress, dashboard projection이 완전히 단일 상태 머신으로 합쳐진 것은 아니다.

## Worker profile 과 gateway 경계

`internal/project/worker_profiles.go`는 logical worker kind를 정의한다.

- developer류 기본값은 `codex-cli`
- reviewer/pm류 기본값은 `claude-code`
- fallback은 `default`

gateway 기반 dispatch는 여전히 project task runner 경계를 통해 실제 executor와 연결된다. logical worker kind와 실제 executor alias가 달라도 board/activity에는 logical worker kind를 남긴다.

## Dashboard 구조

`internal/tarsserver/dashboard.go`는 project workflow를 읽기 전용 운영 UI로 보여 준다. 다만 현재 기본 사용자 진입은 `/console`이며, 예전 `/dashboards`와 `/ui/projects/*`는 리다이렉트 경로다.

- `/console`: 현재 기본 콘솔
- `/console/projects/{id}`: 프로젝트 상세
- `/dashboards`, `/ui/projects/{id}`: legacy redirect
- `/ui/projects/{id}/stream`: legacy stream route

프로젝트 상태 표시는 서버 API와 콘솔 프런트엔드가 함께 만든다. 예전 HTML 대시보드 URL은 호환성 유지용 진입점으로 보는 편이 맞다.

## 초보자가 놓치기 쉬운 점

- `project-autopilot` skill 문서가 있어도 실제 background loop는 Go 코드가 돈다.
- backlog planning은 하나의 곳에서만 일어나지 않는다. explicit planner run은 `orchestrator_plan.go`, heartbeat 기반 autonomous planning은 `helpers_project_progress.go`가 맡는다.
- review task는 원래 task를 복제해서 `Role=reviewer`로 다시 dispatch하는 방식이다.
- 프로젝트 문서 입력은 `store_normalize.go`에서 profile/rule/sub-agent/tool policy를 정규화한 뒤 저장된다.
- heartbeat가 active project의 autonomous progress 트리거가 될 수 있다.
