# 모듈: 프로젝트 오토파일럿과 대시보드

## 핵심 파일

- `internal/project/activity.go`
- `internal/project/activity_auto.go`
- `internal/project/brief_state.go`
- `internal/project/github_flow.go`
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

프로젝트 workflow는 여러 파일이 같이 움직인다.

- `workspace/_shared/project_briefs/<session>/BRIEF.md`: kickoff 질문과 답을 모으는 brief
- `workspace/projects/<project>/PROJECT.md`: 프로젝트 정의
- `workspace/projects/<project>/STATE.md`: phase/status/next action
- `workspace/projects/<project>/KANBAN.md`: canonical board
- `workspace/projects/<project>/ACTIVITY.jsonl`: worker/PM/system activity
- `workspace/projects/<project>/AUTOPILOT.json`: background loop 상태

즉, 이 모듈의 truth source는 DB가 아니라 파일 문서 묶음이다.

## 시작 흐름

프로젝트 시작 진입은 두 층으로 나뉜다.

- `internal/tarsserver/handler_chat.go`는 kickoff처럼 보이는 일반 문장이나 active brief가 있으면 `project-start` skill을 자동 선택한다.
- 실제 brief 저장/상태 전이는 `internal/project/brief_state.go`가 담당한다.

따라서 프로젝트 시작은 "특수 API를 직접 두드리는 기능"이 아니라 채팅 파이프라인 위에 얹힌 skill-guided flow다.

## 보드와 활동 로그

`internal/project/kanban.go`와 `internal/project/activity.go`를 같이 봐야 한다.

- board는 `todo`, `in_progress`, `review`, `done` canonical status로 정규화된다.
- task가 생성되거나 바뀌면 `activity_auto.go`가 `board_task_created`, `board_task_updated` 같은 system activity를 남긴다.
- task report, blocker, decision, replan, GitHub Flow metadata도 전부 `ACTIVITY.jsonl`로 흘러간다.

이 덕분에 dashboard와 recovery loop는 별도 메모리 상태 없이 activity 문서를 다시 읽어 현재 상태를 복원할 수 있다.

## Worker profile 과 gateway 경계

`internal/project/worker_profiles.go`는 logical worker kind를 정의한다.

- developer류 기본값은 `codex-cli`
- reviewer/pm류 기본값은 `claude-code`
- fallback은 `default`

`internal/gateway/project_task_runner.go`는 이 logical worker kind와 실제 executor를 분리한다.

- requested alias가 있으면 그 executor로 spawn 한다.
- executor alias가 없어서 default agent로 fallback해도 `runKinds` 맵에 원래 logical worker kind를 보존한다.
- `Wait()`는 실행 결과의 실제 agent 이름과 별개로, board/activity에는 logical worker kind를 돌려준다.

즉, runtime executor 이름이 workflow 모델을 오염시키지 않도록 방어하고 있다.

## Task dispatch 와 verification gate

`internal/project/orchestrator.go`는 task dispatch의 중심이다.

1. board에서 `todo` 또는 `review` task를 고른다.
2. `ResolveWorkerProfile()`로 logical worker profile을 결정한다.
3. `BuildTaskPrompt()`가 `<task-report>` 고정 출력 계약을 넣은 prompt를 만든다.
4. `TaskRunner`를 통해 gateway run을 시작하고 기다린다.
5. `task_report.go`가 worker 응답을 파싱한다.
6. test/build/issue/branch/pr metadata를 activity와 board에 기록한다.

여기서 중요한 점은 GitHub Flow gate가 강하게 묶여 있다는 것이다. test/build/issue/branch/pr 중 하나라도 비면 task는 `done` 대신 `in_progress`로 남고 오류를 반환한다.

## Autopilot loop

`internal/project/project_runner.go`의 `AutopilotManager`는 long-lived supervisor다.

- `RestorePersistedRuns()`로 재시작 후 상태를 읽는다.
- `EnsureActiveRuns()`로 살아 있어야 하는 프로젝트에 loop를 다시 붙인다.
- board가 비어 있으면 `seedBacklog()`가 기본 MVP task 두 개를 만든다.
- `todo`면 dispatch, `review`면 review dispatch, `in_progress`면 stalled task recovery를 시도한다.
- 실패가 routine retry 범주면 `autoRecover()`가 task를 다시 `todo`로 되돌리고 decision/replan activity를 남긴다.

즉, 현재 오토파일럿은 planner라기보다 "규칙 기반 PM supervisor"에 가깝다.

## Dashboard 구조

`internal/tarsserver/dashboard.go`는 project workflow를 읽기 전용 운영 UI로 보여 준다.

- `/dashboards`: 프로젝트 목록
- `/ui/projects/{id}`: 개별 프로젝트 화면
- `/ui/projects/{id}/stream`: EventSource 스트림

화면은 server-rendered HTML이다. 브라우저는 SSE 이벤트를 받으면 현재 페이지 HTML을 다시 fetch해서 `autopilot`, `board`, `activity`, `github-flow`, `reports`, `blockers`, `decisions`, `replans` 섹션만 교체한다.

즉, SPA가 아니라 "HTML 문서 + 부분 refresh" 구조다.

## 초보자가 놓치기 쉬운 점

- `project-autopilot` skill 문서가 있어도 실제 background loop는 Go 코드가 돈다.
- review task는 원래 task를 복제해서 `Role=reviewer`로 다시 dispatch하는 방식이다.
- bootstrap seed task는 모든 todo를 병렬 실행하지 않기 위한 예외 규칙을 가진다.
- dashboard는 상태를 보여 주지만, 승인/재시도 같은 조작은 직접 수행하지 않는다.
- heartbeat와 autopilot은 별도 루프지만, heartbeat가 active project의 autopilot 재시작 트리거가 될 수 있다.

## 디버깅 포인트

- kickoff가 예상과 다를 때: `resolveSkillForMessage`, `hasActiveProjectBrief`
- task가 계속 막힐 때: `verificationStatus`, `task gate failed`, `ACTIVITY.jsonl`
- fallback executor가 의심될 때: `ProjectTaskRunner.Start`, `ProjectTaskRunner.Wait`
- 오토파일럿이 멈췄을 때: `AUTOPILOT.json`, `EnsureActiveRuns`, `autoRecover`
- dashboard 갱신이 안 될 때: `/ui/projects/{id}/stream`, `projectDashboardBroker.publish`
