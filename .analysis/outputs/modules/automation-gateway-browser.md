# 모듈: 자동화, 게이트웨이, 브라우저 실행

## 핵심 파일

- `internal/schedule/store.go`
- `internal/ops/manager.go`
- `internal/ops/manager_cleanup.go`
- `internal/ops/manager_approvals.go`
- `internal/gateway/executor.go`
- `internal/gateway/runtime_run_bootstrap.go`
- `internal/gateway/runtime_runs.go`
- `internal/gateway/runtime_channels.go`
- `internal/gateway/runtime_reports.go`
- `internal/browser/service.go`
- `internal/browser/playwright_flow_runner.go`
- `internal/browser/playwright_exec.go`
- `internal/browser/flow.go`

## 역할

이 모듈은 채팅 요청과 분리된 비동기 실행 계층이다. 자연어 일정, 운영 cleanup approval, background agent run, channel surface, report surface, 브라우저 site flow 실행을 각각 파일 기반 상태와 runtime state로 연결한다.

## Gateway run lifecycle

gateway run은 네 단계를 같이 봐야 보인다.

1. `executor.go`: in-process prompt executor 와 command executor, tool allow/deny, session routing 정책 정의
2. `runtime_run_bootstrap.go`: accepted run 생성, session/project 결정
3. `runtime_runs.go`: lifecycle 조회, wait, cancel, trim
4. `runtime_reports.go`: summary/archive 리포트 생성

즉, 이제 단순 run 저장소가 아니라 "실행 표면 + 채널 표면 + 리포트 표면"으로 분리돼 있다.

## Channel surface

`internal/gateway/runtime_channels.go`는 local, webhook, telegram 메시지를 workspace-aware channel key로 저장한다.

- inbound/outbound 방향을 모두 기록한다.
- telegram tool이 보낸 메시지도 gateway channel history로 다시 남길 수 있다.
- channel별 메시지 수 제한과 persistence 옵션이 있다.

## Browser 실행 구조

브라우저 계층은 2단 구조다.

- `internal/browser/service.go`: 사이트 flow 로딩, login/check/run API, credential/OTP 처리
- `internal/browser/playwright_exec.go`: Node subprocess 로 Playwright 실행기 호출

즉, Go는 orchestration 과 상태 관리를 하고, 실제 브라우저 조작은 `scripts/playwright_browser_runner.mjs`가 수행한다.

## 디버깅 포인트

- schedule 이상: `resolveSchedule`, `_tars_schedule` payload, legacy migration
- ops 이상: `scanCandidates`, `isSafeCleanupPath`, approval status 전이
- gateway 이상: `newAcceptedRunState`, `finalizeRunLocked`, `ReportsSummaryByWorkspace`
- channel 이상: `appendChannelMessage`, `workspaceChannelKey`
- browser 이상: `LoadSiteFlows`, `runPlaywrightRequest`, Node stderr 메시지
