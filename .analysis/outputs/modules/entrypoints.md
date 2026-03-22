# 모듈: 엔트리포인트

## 핵심 파일

- `cmd/tars/main.go`
- `cmd/tars/mainthread_darwin.go`
- `cmd/tars/mainthread_other.go`
- `cmd/tars/client_main.go`
- `cmd/tars/server_main.go`
- `cmd/tars/assistant_main.go`
- `cmd/tars/doctor_main.go`
- `cmd/tars/service_main.go`
- `cmd/tars/skill_main.go`
- `cmd/tars/plugin_main.go`

## 역할

이 영역은 입력 플래그와 환경 변수를 런타임 옵션으로 바꾸는 얇은 레이어다. 실제 기능 구현은 대부분 `internal/*`로 내려간다. 이번 증분에서는 허브 배포 명령(`skill`, `plugin`)과 macOS 메인 스레드 shim이 추가돼 루트 명령의 역할이 조금 넓어졌다.

## 읽는 순서

1. `cmd/tars/main.go`에서 루트 명령 구조를 본다.
2. `cmd/tars/mainthread_*.go`에서 왜 macOS만 메인 스레드 래퍼가 필요한지 확인한다.
3. `cmd/tars/server_main.go`, `cmd/tars/assistant_main.go`에서 런타임 분기를 확인한다.
4. `cmd/tars/skill_main.go`, `cmd/tars/plugin_main.go`에서 허브 관리 명령이 어디로 내려가는지 확인한다.

## 중요한 관찰

- `.env`와 `.env.secret` 로딩이 가장 먼저 일어난다.
- 루트 명령은 여전히 "비즈니스 로직 실행"보다 "적절한 내부 패키지로 위임"에 집중한다.
- `skill`과 `plugin`은 runtime loader가 아니라 Skill Hub installer를 호출하는 배포용 진입점이다.
- hotkey 제약은 `internal/assistant/*`가 아니라 루트 명령 진입 시점에서 먼저 처리한다.
