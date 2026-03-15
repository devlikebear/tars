# Project Swarm Dogfooding

## 현재 요약

`project-swarm` workflow는 예전 분석 시점보다 실제 런타임에 더 깊게 연결되어 있다. 현재 코드는 backlog seed, autopilot restore, stalled task auto-retry, dashboard 노출, gateway fallback 시 logical worker kind 보존까지 포함한다.

다만 여전히 "skill 문서가 PM 역할을 설명하고, 실제 supervisor는 Go 코드가 수행하는" 이중 구조다. 즉, UX는 자율 프로젝트 운영처럼 보이지만, 내부 모델은 규칙 기반 dispatch/retry loop에 가깝다.

## 이번 기준으로 좋아진 점

### 1. 오토파일럿이 더 오래 살아남는다

- `internal/project/project_runner.go`는 `AUTOPILOT.json`을 저장하고, 서버 시작 시 `RestorePersistedRuns()`로 복구한다.
- heartbeat 후 `EnsureActiveRuns()`를 호출해 살아 있어야 할 프로젝트 loop를 다시 붙인다.

이제 project workflow는 "채팅 한 번으로 끝나는 요청"이 아니라 background supervisor에 가까워졌다.

### 2. 빈 보드를 그냥 막지 않고 seed 한다

- `seedBacklog()`가 보드가 비어 있으면 `pm-seed-bootstrap`, `pm-seed-vertical-slice` 두 개의 MVP task를 만든다.
- bootstrap task는 `filterDispatchableTasksByStatus()` 때문에 먼저 단독 실행된다.

즉, 최소한의 PM 역할은 이제 실제 코드로 들어가 있다.

### 3. gateway fallback이 더 안전해졌다

- `internal/gateway/project_task_runner.go`는 `runKinds` 맵에 요청된 logical worker kind를 저장한다.
- executor alias가 없어서 default agent로 fallback해도 `Wait()` 결과는 원래의 `codex-cli` 또는 `claude-code` kind를 유지한다.

이전 분석에서 지적했던 "fallback executor 이름이 board의 worker_kind를 오염시키는 문제"는 현재 코드 기준으로 완화됐다.

### 4. 실행 전 doctor가 더 많은 실패를 선제 차단한다

- `cmd/tars/doctor_main.go`는 `gateway_default_agent`가 가리키는 command와 args 경로가 실제로 존재하는지 검사한다.
- Claude Code CLI나 LLM credential 누락도 여기서 바로 잡는다.

즉, workspace에서 `./worker_agent.py` 같은 파일이 없는데도 project workflow를 돌리는 상황을 더 일찍 발견할 수 있다.

### 5. 운영 상태가 대시보드에서 보인다

- `internal/tarsserver/dashboard.go`는 board, activity, GitHub Flow metadata, worker report, blocker, decision, replan을 모두 HTML로 보여 준다.
- `/ui/projects/{id}/stream`이 SSE로 section refresh를 트리거한다.

이제 project workflow는 단순 문서 파일 묶음이 아니라 읽기 전용 운영 화면까지 가진다.

## 아직 남아 있는 갭

### 1. PM loop는 여전히 하드코딩된 규칙 기반이다

- `project-autopilot` skill 문서는 고수준 정책을 설명한다.
- 실제 실행은 `AutopilotManager.run()`이 `todo`, `review`, `in_progress`, `done` 상태를 분기하며 처리한다.

즉, PM 판단이 LLM 기반 planner로 일반화된 것은 아니다. 현재는 "정해진 규칙을 가진 supervisor"에 더 가깝다.

### 2. `project-start`는 여전히 brief 수집 품질에 강하게 의존한다

- `handler_chat.go`는 kickoff 문장이나 active brief가 있으면 `project-start`를 자동 선택한다.
- 하지만 실제 finalize 여부는 skill body와 brief 상태 전이에 달려 있다.

사용자가 충분히 상세하게 말하지 않으면, 여전히 한 번 더 질문을 받는 흐름이 자연스럽다.

### 3. GitHub Flow gate가 강하게 묶여 있다

- `orchestrator.go`는 test/build/issue/branch/pr 메타데이터를 모두 task gate로 본다.
- 값이 비면 task를 `done`이나 `review`가 아니라 다시 `in_progress`에 두고 오류를 반환한다.

로컬 MVP나 PoC에는 꽤 강한 정책이다. product intent에는 맞지만, lightweight mode가 따로 있지는 않다.

### 4. dashboard는 읽기 전용이다

- 현재 dashboard는 상태를 보여 주고 자동 새로고침만 한다.
- 승인, replan, retry 같은 조작은 여전히 API나 tool 호출 경로에 있다.

운영 가시성은 좋아졌지만, PM 콘솔로까지 확장된 것은 아니다.

## 현재 코드에서 읽히는 방향성

1. `project-start`가 brief를 만들고,
2. `project_runner.go`가 board를 seed/dispatch/recover 하고,
3. `gateway/project_task_runner.go`가 logical worker role과 실제 executor를 연결하고,
4. `dashboard.go`가 그 상태를 읽기 전용 UI로 노출한다.

즉, 현재 TARS의 project-swarm은 "완전 자율 멀티에이전트 PM"보다는 "문서 기반 workflow 엔진 + supervisor loop + thin dashboard"로 이해하는 편이 정확하다.

## 다음 개선 후보

1. PM 판단 로직을 `AutopilotManager.run()`의 하드코딩 분기에서 더 선언적인 정책 계층으로 분리한다.
2. GitHub Flow gate를 strict mode와 local MVP mode로 나눠 운영 강도를 선택할 수 있게 한다.
3. dashboard에서 retry/approve/replan 같은 제한된 운영 액션을 직접 호출할 수 있게 만든다.
4. `project-start`가 충분히 상세한 kickoff를 받으면 마지막 질문 없이 finalize/start 하도록 heuristics를 더 강화한다.
