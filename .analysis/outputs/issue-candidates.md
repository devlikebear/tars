### TIDY-002: 프로젝트 workflow 모델 단일화 추가 정리

- Module: `internal/project/ + internal/tarsserver/`
- Type: `TIDY`
- Evidence: `workflow_policy.go`와 `policy.go`가 normalize/tool-policy 규칙을 모으긴 했지만 kickoff trigger는 `handler_chat.go`, planner run은 `orchestrator_plan.go`, dispatch gate는 `orchestrator.go`, autonomous progress는 `helpers_project_progress.go`, dashboard projection은 `dashboard.go`에 여전히 나뉘어 있다.
- Suggested action: chat/autopilot/dashboard가 공통으로 쓰는 stage/event 모델을 두고 state transition과 projection 규칙을 한 레이어로 더 끌어올린다.

### SEC-002: Browser relay query token 노출 위험은 완전히 제거되지 않음

- Module: `internal/browserrelay/`
- Type: `SEC`
- Evidence: `internal/browserrelay/server.go`는 `AllowQueryToken`이 켜지면 여전히 query string의 relay token을 허용하고, 이번 변경은 status payload에 warning을 추가하는 수준이다.
- Suggested action: 운영 기본값을 header-only로 더 강하게 고정하거나, query token 허용 시 TTL/one-shot token 같은 추가 제약을 둔다.

### TIDY-005: Semantic embed provider surface와 실제 구현 범위가 어긋남

- Module: `internal/memory/`
- Type: `TIDY`
- Evidence: `SemanticConfig`와 config 키는 generic provider interface처럼 보이지만 `memory.NewService`는 현재 `gemini`일 때만 embedder를 생성한다. 다른 provider 값은 설정은 받아도 실제 서비스가 활성화되지 않는다.
- Suggested action: embed provider registry를 도입하거나, 지원하지 않는 provider 설정 시 startup/doctor 단계에서 명시적 오류를 내도록 바꾼다.

### SEC-003: Skill Hub 원격 설치 무결성 검증 부재

- Module: `internal/skillhub/`
- Type: `SEC`
- Evidence: `internal/skillhub/registry.go`는 `raw.githubusercontent.com`의 registry와 파일을 그대로 내려받고, 설치 시 checksum/signature/lockfile 검증을 하지 않는다.
- Suggested action: signed registry, checksum manifest, 또는 최소한 버전별 해시 검증을 도입해 supply-chain 신뢰 경계를 명확히 한다.

### TIDY-006: Skill Hub 설치가 부분 성공 상태를 정상 설치처럼 기록할 수 있음

- Module: `internal/skillhub/`
- Type: `TIDY`
- Evidence: `internal/skillhub/install.go`는 companion file과 plugin file fetch/write 실패를 `continue`로 무시하는 best-effort 전략을 사용하고, 최종적으로는 `skillhub.json`에 설치 완료를 기록한다.
- Suggested action: 임시 디렉터리에 전체 payload를 검증한 뒤 원자적으로 교체하고, 누락 파일이 있으면 설치를 실패로 처리한다.
