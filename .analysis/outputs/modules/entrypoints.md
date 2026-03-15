# 모듈: 엔트리포인트

## 핵심 파일

- `cmd/tars/main.go`
- `cmd/tars/client_main.go`
- `cmd/tars/server_main.go`
- `cmd/tars/assistant_main.go`

## 역할

이 영역은 입력 플래그와 환경 변수를 런타임 옵션으로 바꾸는 얇은 레이어다. 실제 기능 구현은 대부분 `internal/*`로 내려간다.

## 읽는 순서

1. `cmd/tars/main.go`에서 루트 명령 구조를 본다.
2. `cmd/tars/client_main.go`에서 기본 실행 경로가 `internal/tarsclient.Run`임을 확인한다.
3. `cmd/tars/server_main.go`에서 서버 옵션이 `internal/tarsserver.ServeOptions`로 정규화되는 흐름을 본다.
4. `cmd/tars/assistant_main.go`에서 음성/LaunchAgent 경로를 확인한다.

## 중요한 관찰

- `.env`와 `.env.secret` 로딩이 가장 먼저 일어난다.
- 루트 명령은 "버전 출력", "클라이언트", "서버", "assistant" 네 흐름으로 갈린다.
- 이 레이어는 비즈니스 로직을 거의 갖지 않으므로, 버그를 찾을 때는 보통 이 아래 패키지로 바로 내려가야 한다.
