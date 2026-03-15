# 모듈: 자동화, 게이트웨이, 브라우저 실행

## 핵심 파일

- `internal/schedule/store.go`
- `internal/ops/manager.go`
- `internal/ops/manager_cleanup.go`
- `internal/ops/manager_approvals.go`
- `internal/gateway/runtime_runs.go`
- `internal/gateway/runtime_run_bootstrap.go`
- `internal/gateway/runtime_run_execute.go`
- `internal/gateway/runtime_persist.go`
- `internal/browser/service.go`
- `internal/browser/playwright_flow_runner.go`
- `internal/browser/playwright_exec.go`
- `internal/browser/flow.go`

## 역할

이 모듈은 채팅 요청과 분리된 비동기 실행 계층이다. 자연어 일정, 운영 cleanup approval, background agent run, 브라우저 site flow 실행을 각각 파일 기반 상태와 runtime state로 연결한다.

## Schedule 흐름

`internal/schedule/store.go`는 독립 스케줄 DB가 아니라 cron store 위에 schedule metadata를 얹는 어댑터다.

- 자연어 입력은 `scheduleexpr`로 해석된다.
- 실제 저장은 cron job으로 이루어진다.
- schedule 전용 메타데이터는 payload의 `_tars_schedule` 키 아래 저장된다.
- legacy `items.jsonl`과 `cron_map.json`이 있으면 첫 접근 시 migration 한다.

## Ops 흐름

`internal/ops/manager.go`는 운영 상태 조회와 cleanup approval을 담당한다.

- 상태 조회는 디스크 사용량과 프로세스 개수를 반환한다.
- cleanup은 바로 실행되지 않고 plan 생성 -> approve/reject -> apply 순서로 진행된다.
- 실제 삭제는 `Downloads`, `Desktop`, `Library/Caches`, `.Trash` 같은 safe root 안에서만 허용된다.
- approval 목록은 JSON, 이벤트는 일별 JSONL로 남는다.

## Gateway run lifecycle

gateway run은 세 단계 파일을 같이 봐야 보인다.

1. `runtime_run_bootstrap.go`: accepted run 생성, session/project 결정
2. `runtime_run_execute.go`: 실행 시작, transcript append, executor 호출, 결과 요약
3. `runtime_runs.go`: 조회, wait, cancel, trim

run은 accepted -> running -> completed/failed/canceled 순으로 전이되고, worker session에서 실행된 결과는 main session에 summary로 복사될 수 있다.

## Persistence 와 복구

`internal/gateway/runtime_persist.go`는 run/channel snapshot을 저장하고 재시작 시 복구한다.

- 진행 중이던 run이 복구되면 completed가 아니라 canceled by restart recovery 로 바뀐다.
- channel 메시지는 workspace/channel key 기준으로 정규화된다.
- persistence 와 archive 는 옵션으로 켜고 끌 수 있다.

## Browser 실행 구조

브라우저 계층은 2단 구조다.

- `internal/browser/service.go`: 사이트 flow 로딩, login/check/run API, credential/OTP 처리
- `internal/browser/playwright_exec.go`: Node subprocess 로 Playwright 실행기 호출

즉, Go는 orchestration 과 상태 관리를 하고, 실제 브라우저 조작은 `scripts/playwright_browser_runner.mjs`가 수행한다.

## 디버깅 포인트

- schedule 이상: `resolveSchedule`, `_tars_schedule` payload, legacy migration
- ops 이상: `scanCandidates`, `isSafeCleanupPath`, approval status 전이
- gateway 이상: `newAcceptedRunState`, `finalizeRunLocked`, `persistSnapshot`
- browser 이상: `LoadSiteFlows`, `runPlaywrightRequest`, Node stderr 메시지
