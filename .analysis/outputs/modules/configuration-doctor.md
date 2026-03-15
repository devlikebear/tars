# 모듈: 설정 로딩과 Doctor 진단

## 핵심 파일

- `internal/config/load.go`
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

이 모듈은 "앱이 어떤 설정으로 실행되는가"와 "그 설정이 실제로 실행 가능한가"를 분리해서 다룬다. `internal/config/*`는 값을 해석하고 정규화하며, `tars doctor`는 스타터 workspace, BYOK, gateway executor, CLI 의존성까지 진단한다.

## 설정 병합 흐름

설정 병합 순서는 `defaults < YAML < environment variables`다.

- `internal/config/load.go`가 기본값으로 시작한다.
- `internal/config/yaml.go`가 flat key 형태 YAML을 읽는다.
- `internal/config/env.go`가 같은 필드를 환경 변수로 다시 덮는다.
- 마지막에 `internal/config/defaults_apply.go`가 invalid/blank 값을 정규화하고 workspace 기반 파생 경로를 보정한다.

이 마지막 단계가 중요하다. `workspace_dir`가 바뀌면 `browser_site_flows_dir`, `browser_managed_user_data_dir`, `gateway_persistence_dir`, `gateway_archive_dir` 같은 경로도 같이 재계산된다.

## Provider 와 Auth 기본값

`defaults_apply.go`는 단순 빈 값 채우기가 아니라 provider별 런타임 성격을 반영한다.

- `openai-codex`는 API key가 없으면 `oauth` 모드로 자동 전환된다.
- `claude-code-cli`는 API key가 없으면 `cli` 모드로 전환된다.
- `openai`, `anthropic`, `gemini`, `gemini-native`는 base URL, model, API key fallback이 공급자별로 다르다.
- `llm_reasoning_effort`, `llm_thinking_budget`, `llm_service_tier`는 느슨한 문자열 입력을 정규화한다.

즉, config는 "사용자가 적은 값"을 그대로 노출하기보다 "실행 가능한 최종 값"으로 바꿔 준다.

## 보안 하드닝 기본값

이번 기준 설정은 safe-by-default 성격이 강하다.

- `api_auth_mode` 기본값은 `required`
- `dashboard_auth_mode` 기본값은 `inherit`
- `api_allow_insecure_local_auth` 기본값은 `false`
- `api_max_inflight_chat=2`, `api_max_inflight_agent_runs=4`
- `plugins_allow_mcp_servers=false`
- `mcp_command_allowlist` 기본값은 빈 리스트

따라서 개발 편의를 위해 `off`나 `external-required`를 쓰려면 `api_allow_insecure_local_auth=true`를 명시해야 한다.

## Doctor 흐름

`cmd/tars/doctor_main.go`는 다섯 단계로 읽으면 된다.

1. workspace/config 경로를 해석한다.
2. config 파일이 없으면 `--fix`에서 starter config를 생성한다.
3. workspace skeleton과 bundled plugin manifest가 있는지 확인한다.
4. config를 실제로 로드한다.
5. API auth, gateway enabled 여부, gateway default agent, LLM credential, Claude CLI 런타임을 검증한다.

특히 `validateDoctorGatewayAgent`는 `gateway_default_agent`가 참조하는 command와 args 안의 로컬 경로가 실제로 존재하는지 확인한다. 프로젝트 오토파일럿이 "executor는 설정돼 있는데 실행 파일이 없는" 상태로 출발하지 않게 막는 방어선이다.

## 초보자가 놓치기 쉬운 점

- `Config`는 nested struct가 아니라 큰 flat struct라서 YAML/env key 이름을 직접 매핑해야 한다.
- `merge.go`는 zero value를 덮지 않기 때문에, boolean false를 의도적으로 설정하는 필드는 기본값과 parser 동작을 같이 봐야 한다.
- 예제 설정 파일 `config/tars.config.example.yaml`은 단순 샘플이 아니라 현재 지원되는 top-level 키의 카탈로그 역할도 한다.
- `doctor`는 lint가 아니라 실제 설치 상태 검사 도구다. missing plugin manifest, missing CLI, missing worker script도 여기서 잡힌다.

## 디버깅 포인트

- 설정이 예상과 다를 때: `Load`, `applyEnv`, `applyDefaults`
- 공급자 인증 모드가 헷갈릴 때: `applyCoreDefaults`, `applyProviderDefaults`
- starter 환경이 비어 있을 때: `resolveConfigPath`, `ensureStarterWorkspaceLayout`, `writeStarterConfigFile`
- gateway executor 실패 시: `checkDoctorGatewayAgents`, `validateDoctorGatewayAgent`
