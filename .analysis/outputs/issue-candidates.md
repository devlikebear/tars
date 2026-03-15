### TIDY-001: 설정 스키마 매핑 중복 축소

- Module: `internal/config/`
- Type: `TIDY`
- Evidence: `defaults.go`, `defaults_apply.go`, `env.go`, `yaml.go`, `merge.go`가 같은 필드 집합을 서로 다른 규칙으로 반복 매핑한다. 새 설정 키를 추가할 때 여러 switch/if 블록을 동시에 수정해야 한다.
- Suggested action: 필드 메타데이터 테이블이나 declarative schema를 도입해 YAML/env/default merge 규칙을 한 군데로 모은다.

### TIDY-002: 프로젝트 workflow 상태기계 분리

- Module: `internal/project/`
- Type: `TIDY`
- Evidence: kickoff 감지는 `handler_chat.go`, brief/state 저장은 `brief_state.go`, dispatch 규칙은 `orchestrator.go`, supervisor loop는 `project_runner.go`, 정책 설명은 `plugins/project-swarm/skills/*.md`에 흩어져 있다.
- Suggested action: project workflow 전이를 명시하는 상태기계 또는 policy 계층을 두고 chat trigger, autopilot, dashboard가 같은 모델을 바라보게 만든다.

### SEC-001: 대시보드 공개 모드 명시 강화

- Module: `internal/tarsserver/`
- Type: `SEC`
- Evidence: `middleware.go`는 `dashboard_auth_mode=off`일 때 `/dashboards`와 `/ui/projects/*`를 인증 skip path로 추가한다. 프로젝트 objective, board, activity가 그대로 노출되는 화면이라 설정 실수의 영향이 크다.
- Suggested action: loopback-only guard나 시작 시 경고 로그를 추가하고, 공개 모드를 더 명시적으로 선택하게 만든다.

### TIDY-003: 대시보드 섹션 정의 단일화

- Module: `internal/tarsserver/dashboard.go`
- Type: `TIDY`
- Evidence: 대시보드 섹션 ID 목록이 템플릿 HTML, 브라우저 refresh 스크립트, 서버 data builder 함수에 암묵적으로 중복돼 있다. 새 섹션을 추가할 때 여러 위치를 함께 수정해야 한다.
- Suggested action: 섹션 레지스트리나 render spec을 도입해 서버 렌더와 클라이언트 refresh 대상 목록을 한 구조에서 생성한다.

### TIDY-004: Provider credential lifecycle 단일화

- Module: `internal/auth/`
- Type: `TIDY`
- Evidence: 일반 OAuth token 해석은 `internal/auth/token.go`, Codex refresh/persistence 는 `internal/auth/codex_oauth.go`, 실제 refresh retry 는 `internal/llm/openai_codex_client.go`에 나뉘어 있다. provider onboarding 시 token source 와 refresh 위치를 한 번에 파악하기 어렵다.
- Suggested action: provider별 credential metadata와 refresh hook을 한 registry나 strategy table로 모아 `provider.go`와 client 구현이 같은 lifecycle 계약을 보게 만든다.

### SEC-002: Browser relay query token 노출 위험 축소

- Module: `internal/browserrelay/`
- Type: `SEC`
- Evidence: `relayTokenFromRequest`는 `AllowQueryToken`이 켜지면 query string의 `token` 또는 `relay_token`도 허용한다. loopback 제한이 있어도 URL 기반 토큰은 브라우저 기록, 로그, 디버그 출력에 남기 쉽다.
- Suggested action: 기본값은 header-only 로 유지하고, query token 허용 시 시작 로그 경고나 짧은 TTL 전략을 추가해 운영 실수를 줄인다.
