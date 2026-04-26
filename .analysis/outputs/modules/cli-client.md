# 모듈: CLI와 Client

## 역할

`cmd/tars`는 Cobra 기반 얇은 진입점이다. root 명령은 콘솔을 열거나 `--message` one-shot chat을 실행하고, subcommand들은 대부분 `pkg/tarsclient`/`internal/tarsclient`를 통해 서버 API를 호출한다.

## 주요 파일

- `cmd/tars/main.go`
- `cmd/tars/server_main.go`
- `cmd/tars/client_main.go`
- `cmd/tars/console_main.go`
- `cmd/tars/approval_main.go`
- `pkg/tarsclient/*`
- `internal/tarsclient/*`

## 관찰

- root `tars`와 `tars --message`는 최신 사용자-facing CLI 경로다.
- 레거시 interactive terminal UI는 제거됐다.
- URL 조립/default 중복은 #408에서 추적한다.
