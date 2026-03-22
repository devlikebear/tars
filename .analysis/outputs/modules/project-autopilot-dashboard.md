# 모듈: 프로젝트 오토파일럿과 대시보드

## 핵심 파일

- `internal/project/activity.go`
- `internal/project/activity_auto.go`
- `internal/project/brief_state.go`
- `internal/project/workflow_policy.go`
- `internal/project/kanban.go`
- `internal/project/orchestrator.go`
- `internal/project/project_runner.go`
- `internal/project/task_report.go`
- `internal/project/worker_profiles.go`
- `internal/gateway/project_task_runner.go`
- `internal/tarsserver/dashboard.go`
- `internal/tarsserver/main_serve_api.go`
- `internal/tarsserver/handler_chat.go`
- `plugins/project-swarm/skills/project-start/SKILL.md`
- `plugins/project-swarm/skills/project-autopilot/SKILL.md`

## 역할

이 모듈은 채팅으로 시작한 프로젝트를 workspace 문서 집합으로 바꾸고, background supervisor가 task를 dispatch/recover 하며, 서버가 그 상태를 dashboard로 노출하는 계층이다. 한마디로 말하면 "문서 기반 PM workflow 엔진"이다.

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
- `orchestrator.go`: todo/review dispatch, verification gate, activity 기록
- `project_runner.go`: 장기 루프, stalled task recovery, auto-retry, persisted run 복구
- `dashboard.go`: board/activity/state를 읽기 전용 HTML projection으로 변환

즉, 한 파일에 몰려 있던 규칙이 일부 빠져나왔지만, 전체 workflow 모델은 아직 여러 파일에 걸쳐 있다.

## Worker profile 과 gateway 경계

`internal/project/worker_profiles.go`는 logical worker kind를 정의한다.

- developer류 기본값은 `codex-cli`
- reviewer/pm류 기본값은 `claude-code`
- fallback은 `default`

`internal/gateway/project_task_runner.go`는 이 logical worker kind와 실제 executor를 분리한다. executor alias가 default로 fallback해도 board/activity에는 logical worker kind를 남긴다.

## Dashboard 구조

`internal/tarsserver/dashboard.go`는 project workflow를 읽기 전용 운영 UI로 보여 준다.

- `/dashboards`: 프로젝트 목록
- `/ui/projects/{id}`: 개별 프로젝트 화면
- `/ui/projects/{id}/stream`: EventSource 스트림

이번 증분에서 섹션 정의는 `projectDashboardSectionRegistry`로 중앙화됐다. 서버 렌더, refresh 대상 ID, section별 post-processing이 한 registry에서 같이 정의된다.

## 초보자가 놓치기 쉬운 점

- `project-autopilot` skill 문서가 있어도 실제 background loop는 Go 코드가 돈다.
- review task는 원래 task를 복제해서 `Role=reviewer`로 다시 dispatch하는 방식이다.
- bootstrap seed task는 모든 todo를 병렬 실행하지 않기 위한 예외 규칙을 가진다.
- heartbeat와 autopilot은 별도 루프지만, heartbeat가 active project의 autopilot 재시작 트리거가 될 수 있다.
