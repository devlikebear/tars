# 모듈: 엔트리포인트

## 핵심 파일

- `cmd/tars/main.go`
- `cmd/tars/console_main.go`
- `cmd/tars/mainthread_darwin.go`
- `cmd/tars/mainthread_other.go`
- `cmd/tars/client_main.go`
- `cmd/tars/server_main.go`
- `cmd/tars/assistant_main.go`
- `cmd/tars/doctor_main.go`
- `cmd/tars/project_main.go`
- `cmd/tars/cron_main.go`
- `cmd/tars/approval_main.go`
- `cmd/tars/service_main.go`
- `cmd/tars/skill_main.go`
- `cmd/tars/plugin_main.go`
- `cmd/tars/mcp_main.go`

## 역할

이 영역은 입력 플래그와 환경 변수를 런타임 옵션으로 바꾸는 얇은 레이어다. 실제 기능 구현은 대부분 `internal/*`로 내려간다. 현재 헤드에서는 루트 명령의 기본 동작이 더 이상 TUI 실행이 아니라 웹 콘솔 열기라는 점이 중요하다.

## 읽는 순서

1. `cmd/tars/main.go`에서 루트 명령 구조를 본다.
2. `cmd/tars/console_main.go`, `cmd/tars/client_main.go`에서 기본 콘솔 경로와 one-shot CLI 채팅 경로를 확인한다.
3. `cmd/tars/mainthread_*.go`에서 왜 macOS만 메인 스레드 래퍼가 필요한지 확인한다.
4. `cmd/tars/server_main.go`, `cmd/tars/assistant_main.go`, `cmd/tars/service_main.go`에서 런타임 분기를 확인한다.
5. `cmd/tars/project_main.go`, `cmd/tars/cron_main.go`, `cmd/tars/approval_main.go`에서 운영용 CLI surface를 본다.
6. `cmd/tars/skill_main.go`, `cmd/tars/plugin_main.go`, `cmd/tars/mcp_main.go`에서 허브 관리 명령이 어디로 내려가는지 확인한다.

## 중요한 관찰

- `.env`와 `.env.secret` 로딩이 가장 먼저 일어난다.
- 루트 명령은 여전히 "비즈니스 로직 실행"보다 "적절한 내부 패키지로 위임"에 집중한다.
- `main.go`는 `internal/browserplugin`을 blank import해 built-in browser plugin 등록을 부트 시점에 끝낸다.
- 기본 동작은 브라우저에서 `/console`을 여는 것이고, `--message`가 있을 때만 one-shot CLI 채팅으로 내려간다.
- `tars tui`는 hidden/deprecated 호환성 명령으로만 남아 있다.
- `skill`, `plugin`, `mcp`는 runtime loader가 아니라 Hub installer/registry를 호출하는 배포용 진입점이다.
- hotkey 제약은 `internal/assistant/*`가 아니라 루트 명령 진입 시점에서 먼저 처리한다.
