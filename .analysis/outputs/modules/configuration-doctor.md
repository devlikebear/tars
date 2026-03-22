# 모듈: 설정 로딩과 Doctor 진단

## 핵심 파일

- `internal/config/load.go`
- `internal/config/config_input_fields.go`
- `internal/config/defaults.go`
- `internal/config/defaults_apply.go`
- `internal/config/env.go`
- `internal/config/merge.go`
- `internal/config/types.go`
- `internal/config/yaml.go`
- `cmd/tars/doctor_main.go`
- `cmd/tars/init_main.go`
- `config/tars.config.example.yaml`

## 역할

이 모듈은 "앱이 어떤 설정으로 실행되는가"와 "그 설정이 실제로 실행 가능한가"를 분리해서 다룬다. `internal/config/*`는 값을 해석하고 정규화하며, `tars doctor`는 starter workspace, BYOK, gateway executor, CLI 의존성까지 진단한다.

## 설정 병합 흐름

설정 병합 순서는 `defaults < YAML < environment variables`다.

- `internal/config/load.go`가 기본값으로 시작한다.
- `internal/config/yaml.go`가 flat key 형태 YAML을 읽는다.
- `internal/config/env.go`가 같은 필드를 환경 변수로 다시 덮는다.
- 마지막에 `internal/config/defaults_apply.go`가 invalid/blank 값을 정규화하고 workspace 기반 파생 경로를 보정한다.

이번 증분의 핵심은 `internal/config/config_input_fields.go`다. 이제 YAML key, env alias, normalize 함수, merge 규칙이 한 field table에 모여서 어떤 입력이 어느 필드로 가는지 추적하기 쉬워졌다.

## Provider 와 Memory 기본값

`defaults_apply.go`는 단순 빈 값 채우기가 아니라 provider별 런타임 성격을 반영한다.

- `openai-codex`는 API key가 없으면 `oauth` 모드로 자동 전환된다.
- `claude-code-cli`는 API key가 없으면 `cli` 모드로 전환된다.
- `memory_semantic_enabled`, `memory_embed_*`는 semantic recall의 활성화와 embedding provider 구성을 결정한다.
- workspace 경로가 바뀌면 browser, gateway, archive, relay 관련 파생 경로도 같이 재계산된다.

## 보안 하드닝 기본값

이번 기준 설정은 safe-by-default 성격이 더 강하다.

- `api_auth_mode` 기본값은 `required`
- `dashboard_auth_mode` 기본값은 `inherit`
- `api_max_inflight_chat=2`, `api_max_inflight_agent_runs=4`
- `plugins_allow_mcp_servers=false`
- `browser_relay_enabled=true`지만 relay token 없이는 쓸 수 없다.

## Doctor 흐름

`cmd/tars/doctor_main.go`는 다섯 단계로 읽으면 된다.

1. workspace/config 경로를 해석한다.
2. config 파일이 없으면 `--fix`에서 starter config를 생성한다.
3. workspace skeleton과 bundled plugin manifest가 있는지 확인한다.
4. config를 실제로 로드한다.
5. API auth, gateway enabled 여부, gateway default agent, LLM credential, Claude CLI, Skill/Plugin starter 상태를 검증한다.

## 초보자가 놓치기 쉬운 점

- `Config`는 nested struct처럼 보이지만 입력 표면은 flat key 테이블이 중심이다.
- field table이 중앙화됐어도 merge semantics는 zero value를 "명시적 override"로 구분하지 않는다.
- 예제 설정 파일은 단순 샘플이 아니라 현재 지원 키의 카탈로그 역할도 한다.
- `doctor`는 lint가 아니라 실제 설치 상태 검사 도구다.

## 디버깅 포인트

- 설정이 예상과 다를 때: `Load`, `config_input_fields.go`, `applyEnv`, `applyDefaults`
- 공급자 인증 모드가 헷갈릴 때: `applyCoreDefaults`, `applyProviderDefaults`
- starter 환경이 비어 있을 때: `resolveConfigPath`, `ensureStarterWorkspaceLayout`, `writeStarterConfigFile`
- gateway executor 실패 시: `checkDoctorGatewayAgents`, `validateDoctorGatewayAgent`
