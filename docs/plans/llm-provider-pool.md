# LLM Provider Pool — Named Provider 풀 + Tier Binding 스키마 재작성

**Branch**: `feat/llm-provider-pool`
**Status**: Planning → Implementation
**Area**: `area/llm`, `area/config`, `area/server`
**Depends on**: 3-tier model routing (이미 머지됨)
**Blocks**: docs/plans/hermes-improvements/03-provider-override.md (task-level override)

## 배경

3-tier model routing이 최근 도입되면서 `llm_tier_{heavy,standard,light}_*` 형태의 플랫 필드 27개가 추가되었다. 레거시 `llm_*` 9개와 합쳐 LLM 관련 config 필드는 **36개 + 환경변수 alias 72개**에 이른다.

운영하면서 다음 문제가 드러났다:

### 1. 자격증명 중복

같은 provider(예: 동일한 Anthropic 직접 키)를 두 개 이상의 티어가 공유해도 `base_url`, `api_key`, `auth_mode`, `oauth_provider`를 티어마다 한 벌씩 적어야 한다. 키 로테이션 시 수정 지점이 N배로 늘어나고, "내가 가진 계정이 몇 개인가"라는 사용자의 멘탈 모델과 어긋난다.

### 2. 오타에 무방비

실제 사용자 config에 다음 silent bug가 있었다:

```yaml
llm_tier_light_provider: anthropic
llm_tier_light_light_base_url: https://api.minimax.io/anthropic   # ← "light" 두 번
llm_tier_light_model: MiniMax-M2.7
```

`llm_tier_light_light_base_url`는 등록되지 않은 키라 **silently ignored**되고, light 티어가 minimax 모델을 codex 엔드포인트로 호출하려다 실패한다. 27개 짜리 긴 플랫 이름은 구조적으로 이 종류 오타에 취약하다.

### 3. 스케일 한계

티어를 4개로 늘리거나 role별 credential override를 지원하는 순간 필드 수가 선형 폭발한다. 환경변수 alias도 동반 폭발해 12-factor 환경에서 다루기 어려워진다.

### 4. Concept mismatch

사용자는 "**codex 계정 1개, anthropic 직접키 1개, minimax 키 1개**를 가지고 있고, 각 티어에 그중 하나를 배정한다"고 사고한다. 현재 스키마는 "각 티어가 독립적인 9-필드 묶음을 갖는다"는 모델이라 개념과 어긋난다.

### 5. 외부 레퍼런스가 같은 결론

- **OpenClaw** (`src/config/types.models.ts`): `models.providers: Record<string, ModelProviderConfig>` — provider를 alias 키로 한 번만 정의, agent는 `"provider/model"` 문자열로 참조. credential은 provider당 한 번만.
- **Hermes Agent** (`hermes_cli/config.py`): 플랫 per-context 필드를 쓰지만 별도로 `fallback_providers` 배열로 cross-provider fallback을 표현. credential은 env var 키 이름으로만 참조.

OpenClaw가 우리가 원하는 그림에 정확히 부합한다.

## 목적

1. **Named provider pool**을 도입: `llm_providers: {alias: {...}}` 맵으로 credential/base_url/auth_mode를 alias 단위로 한 번만 정의한다.
2. **Tier binding** 분리: `llm_tiers: {heavy: {provider: <alias>, model: <name>, ...}}` — 티어는 "어느 alias + 어느 모델 + knobs"만 지정한다.
3. **레거시 스키마 완전 제거**: 혼자 쓰는 프로젝트이므로 backward compat을 과감히 버린다. `LLMConfig`의 flat 9필드, `LLMTier{Heavy,Standard,Light}`, `llm_tier_*_*` 27개 YAML 키, `llm_role_*` 8개 flat role 필드, 관련 env var 72개 전부 삭제.
4. **Single resolution path**: `ResolveLLMTier(cfg, tier)` 하나가 진입점. 신/구 분기 없음 — `LLMProviders` + `LLMTiers`만 본다.
5. **Phase 1 YAGNI**: provider fallback chain, per-task override, dashboard UI 변경, migration CLI, memory_embed 통합은 모두 후속 작업.

## 설계 원칙

1. **Credential은 provider 단위**: model/reasoning_effort 같은 호출 파라미터와 분리.
2. **Tier는 binding 1줄**: provider alias + model + 옵셔널 knobs.
3. **레거시 zero**: 하나의 코드 경로만 존재한다. fallback도, 자동 마이그레이션도 없다.
4. **Fail loud**: 기본 config 파일이 유효하지 않으면 startup error. silent fallback 금지.
5. **Forward-rename 안전**: `kind`(provider 종류) 필드는 의도적으로 `provider`라 부르지 않는다. 그래야 tier binding의 `provider`(=alias 참조)와 혼동되지 않는다.
6. **Identity check 통과**: 단일 Go 바이너리, file-first, scope isolation 영향 없음, 정책은 config / 메커니즘은 Go.

## 현황 진단 (삭제 대상)

### `cfg.LLM*` 플랫 필드 사용처

레거시 LLM 필드를 참조하는 파일 7개. 이 PR에서 전부 새 스키마로 전환.

| 파일 | 현재 | 전환 후 |
|---|---|---|
| `internal/config/types.go` | `LLMConfig`의 9 flat + 3 tier 필드 | `LLMProviders`, `LLMTiers`, `LLMDefaultTier`, `LLMRoleDefaults`만 |
| `internal/config/config_input_fields.go` | 36 LLM 엔트리 + 8 role 엔트리 | 4 엔트리 (providers/tiers/default_tier/role_defaults) |
| `internal/config/defaults_apply.go` | `applyCoreDefaults` LLM 섹션 + `applyProviderDefaults` | `applyLLMPoolDefaults` (신규) |
| `internal/config/llm_tiers.go` | `seedTier`, `EnsureLLMTierDefaults`, tierSpec* | **파일 자체 삭제** |
| `internal/config/schema.go` | `getValue` LLM case 9개 + role case 8개 | `llm_providers`, `llm_tiers`, `llm_default_tier`, `llm_role_defaults` case |
| `internal/tarsserver/helpers_llm_router.go` | `cfg.LLMTier*` 직접 접근 | `config.ResolveAllLLMTiers(cfg)` 호출 |
| `internal/tarsserver/handler_providers_models.go` | `cfg.LLMProvider`, `cfg.LLMModel` 직접 접근 | default tier의 resolved view 사용 |
| `cmd/tars/doctor_main.go` | `cfg.LLMProvider`, `cfg.LLMAPIKey` 체크 | resolver 출력 체크 |

### 테스트 파일

| 파일 | 처리 |
|---|---|
| `internal/config/llm_tiers_test.go` | **파일 자체 삭제** |
| `internal/config/defaults_test.go` | LLM 섹션 rewrite |
| `internal/tarsserver/handler_providers_models_test.go` | 신 스키마 기반 재작성 |

### Router는 변경 없음

`internal/llm/router.go`의 `RouterConfig.Tiers map[Tier]TierEntry` 인터페이스는 그대로 둔다. resolver 결과가 그대로 `TierEntry`로 흘러들어가므로 router 패키지는 한 줄도 안 바뀐다.

## 새 스키마

### `config/standalone.yaml` (checked-in default)

```yaml
mode: standalone
workspace_dir: ./workspace

# ---- LLM provider pool ----
# Each alias defines "where to call + how to authenticate" — no model here.
# At minimum one provider must be defined. Credentials come from env vars.
llm_providers:
  default:
    kind: anthropic
    auth_mode: api-key
    api_key: ${ANTHROPIC_API_KEY}
    base_url: https://api.anthropic.com

# ---- Tier bindings ----
# Each tier binds to one provider alias + model + optional knobs.
# All three tiers (heavy/standard/light) must be configured.
llm_tiers:
  heavy:
    provider: default
    model: claude-opus-4-6
    reasoning_effort: high
  standard:
    provider: default
    model: claude-sonnet-4-6
    reasoning_effort: medium
  light:
    provider: default
    model: claude-haiku-4-5-20251001
    reasoning_effort: minimal

llm_default_tier: standard

# ---- Role → tier mapping (optional) ----
# Roles absent from this map fall back to llm_default_tier.
llm_role_defaults:
  chat_main: standard
  context_compactor: light
  memory_hook: light
  reflection_memory: light
  reflection_kb: light
  pulse_decider: light
  gateway_default: standard
  gateway_planner: heavy

agent_max_iterations: 8
```

### 사용자 `workspace/config/tars.config.yaml` 마이그레이션 예시

**Before (현재)**:

```yaml
llm_auth_mode: oauth
llm_base_url: https://chatgpt.com/backend-api
llm_model: gpt-5.4
llm_oauth_provider: openai-codex
llm_provider: openai-codex
llm_reasoning_effort: medium
llm_service_tier: priority

llm_default_tier: standard
llm_tier_heavy_model: gpt-5.4
llm_tier_heavy_reasoning_effort: high
llm_tier_standard_model: gpt-5.4
llm_tier_standard_reasoning_effort: medium
llm_tier_light_provider: anthropic
llm_tier_light_light_base_url: https://api.minimax.io/anthropic   # ← 버그
llm_tier_light_model: MiniMax-M2.7
llm_tier_light_auth_mode: api-key
```

**After (신 스키마)**:

```yaml
llm_providers:
  codex:
    kind: openai-codex
    auth_mode: oauth
    oauth_provider: openai-codex
    base_url: https://chatgpt.com/backend-api
    service_tier: priority

  minimax:
    kind: anthropic                 # Anthropic 호환 API
    auth_mode: api-key
    api_key: ${MINIMAX_API_KEY}
    base_url: https://api.minimax.io/anthropic

llm_tiers:
  heavy:
    provider: codex
    model: gpt-5.4
    reasoning_effort: high
  standard:
    provider: codex
    model: gpt-5.4
    reasoning_effort: medium
  light:
    provider: minimax
    model: MiniMax-M2.7
    reasoning_effort: minimal

llm_default_tier: standard
```

오타 버그(`light_light`)는 구조적으로 사라진다. 자격증명 중복도 사라진다.

## 새 데이터 모델

### Go 타입

```go
// internal/config/types.go

// LLMProviderSettings is one entry in the named provider pool. It holds
// "where to call + how to authenticate" but NOT "what model to call".
// Models are bound at the tier level (LLMTierBinding.Model) so that one
// provider can serve multiple models.
type LLMProviderSettings struct {
    Kind          string `yaml:"kind"           json:"kind"`
    AuthMode      string `yaml:"auth_mode"      json:"auth_mode"`
    OAuthProvider string `yaml:"oauth_provider" json:"oauth_provider"`
    BaseURL       string `yaml:"base_url"       json:"base_url"`
    APIKey        string `yaml:"api_key"        json:"api_key"`
    ServiceTier   string `yaml:"service_tier"   json:"service_tier"`
}

// LLMTierBinding binds a tier to a provider alias + model + per-call knobs.
// Provider must be a key in cfg.LLMProviders.
type LLMTierBinding struct {
    Provider        string `yaml:"provider"         json:"provider"`
    Model           string `yaml:"model"            json:"model"`
    ReasoningEffort string `yaml:"reasoning_effort" json:"reasoning_effort"`
    ThinkingBudget  int    `yaml:"thinking_budget"  json:"thinking_budget"`
    ServiceTier     string `yaml:"service_tier"     json:"service_tier"`
}

// LLMConfig — all legacy fields deleted. Only pool + tiers remain.
type LLMConfig struct {
    LLMProviders    map[string]LLMProviderSettings
    LLMTiers        map[string]LLMTierBinding
    LLMDefaultTier  string
    LLMRoleDefaults map[string]string
}
```

### Resolver 출력

```go
// internal/config/llm_resolve.go

// ResolvedLLMTier is the flat, final view of one tier's effective LLM
// configuration after merging the named provider pool and tier binding.
// The router builder consumes this struct directly.
type ResolvedLLMTier struct {
    Tier string // "heavy" | "standard" | "light"

    // From the referenced provider pool entry
    Kind          string
    AuthMode      string
    OAuthProvider string
    BaseURL       string
    APIKey        string

    // From the tier binding
    Model           string
    ReasoningEffort string
    ThinkingBudget  int

    // ServiceTier: binding override > provider default
    ServiceTier string

    // Provenance — alias of the provider that served this tier.
    // Used by tars doctor and startup logs.
    ProviderAlias string
}

// ResolveLLMTier returns the effective settings for the given tier.
// Errors when:
//   - tier name is not in cfg.LLMTiers
//   - binding.Provider is not a key in cfg.LLMProviders
//   - resolved Kind is empty (provider pool entry has no kind)
//   - binding.Model is empty
func ResolveLLMTier(cfg *Config, tier string) (ResolvedLLMTier, error)

// ResolveAllLLMTiers returns resolved settings for every tier present in
// cfg.LLMTiers. The router builder uses this single entry point.
func ResolveAllLLMTiers(cfg *Config) (map[string]ResolvedLLMTier, error)
```

## 해석 로직

```
Input: cfg, tierName

1. binding := cfg.LLMTiers[tierName]
   if !exists → error "tier %q not configured in llm_tiers"

2. provider := cfg.LLMProviders[binding.Provider]
   if !exists → error "tier %q references unknown provider alias %q"

3. Resolved.Tier            = tierName
   Resolved.Kind            = provider.Kind
   Resolved.AuthMode        = provider.AuthMode
   Resolved.OAuthProvider   = provider.OAuthProvider
   Resolved.BaseURL         = provider.BaseURL
   Resolved.APIKey          = provider.APIKey
   Resolved.Model           = binding.Model
   Resolved.ReasoningEffort = binding.ReasoningEffort
   Resolved.ThinkingBudget  = binding.ThinkingBudget
   Resolved.ServiceTier     = first non-empty of (binding.ServiceTier, provider.ServiceTier)
   Resolved.ProviderAlias   = binding.Provider

4. Validation:
   if Resolved.Kind == "" → error "provider %q has empty kind"
   if Resolved.Model == "" → error "tier %q binding has empty model"

5. return Resolved, nil
```

**핵심**: 레거시 경로 없음. 한 방향 해석. fallback 없음.

## Defaults 로직

```go
// internal/config/defaults_apply.go

func applyLLMPoolDefaults(cfg *Config) {
    // Per-provider defaults: fill base_url, api_key, auth_mode per kind.
    for alias, p := range cfg.LLMProviders {
        p.Kind     = strings.ToLower(strings.TrimSpace(p.Kind))
        p.AuthMode = strings.ToLower(strings.TrimSpace(p.AuthMode))

        if p.AuthMode == "" {
            switch p.Kind {
            case "openai-codex":   p.AuthMode = "oauth"
            case "claude-code-cli": p.AuthMode = "cli"
            default:                p.AuthMode = "api-key"
            }
        }
        switch p.Kind {
        case "anthropic":
            if p.BaseURL == "" { p.BaseURL = defaultAnthropicBaseURL }
            if p.APIKey  == "" { p.APIKey  = os.Getenv("ANTHROPIC_API_KEY") }
        case "openai":
            if p.BaseURL == "" { p.BaseURL = defaultOpenAIBaseURL }
            if p.APIKey  == "" { p.APIKey  = os.Getenv("OPENAI_API_KEY") }
        case "openai-codex":
            if p.BaseURL == "" { p.BaseURL = defaultOpenAICodexBaseURL }
            if p.APIKey  == "" { p.APIKey  = firstNonEmpty(os.Getenv("OPENAI_CODEX_OAUTH_TOKEN"), os.Getenv("TARS_OPENAI_CODEX_OAUTH_TOKEN")) }
        case "gemini", "gemini-native":
            if p.BaseURL == "" {
                if p.Kind == "gemini-native" {
                    p.BaseURL = defaultGeminiNativeBaseURL
                } else {
                    p.BaseURL = defaultGeminiBaseURL
                }
            }
            if p.APIKey == "" { p.APIKey = os.Getenv("GEMINI_API_KEY") }
        }
        cfg.LLMProviders[alias] = p   // write back (map entries are copies)
    }

    // Default tier
    if strings.TrimSpace(cfg.LLMDefaultTier) == "" {
        cfg.LLMDefaultTier = "standard"
    }

    // Normalize role defaults
    cfg.LLMRoleDefaults = normalizeRoleDefaults(cfg.LLMRoleDefaults)
}
```

**Note**: 이 함수는 **defaults만 채운다**. 완전히 빈 config에서 자동으로 pool을 생성하지 않는다. 사용자는 최소 하나의 provider + 세 개의 tier(`heavy`/`standard`/`light`)를 정의해야 한다. 누락 시 `buildLLMRouter`가 `RouterConfig` 검증에서 loud하게 실패. `config/standalone.yaml`에 checked-in 스타터가 있어 새 사용자도 바로 동작.

## YAML/Env 파싱

### YAML nested map 처리

`usagePriceOverridesField`와 동일 패턴으로 별도 파서 함수 작성. `gopkg.in/yaml.v3`가 이미 nested map을 지원하므로 struct-to-yaml 자동 라운드트립 가능.

```go
// internal/config/llm_providers_field.go

// llmProvidersField parses the `llm_providers` YAML key into
// cfg.LLMProviders. Value is a map alias→LLMProviderSettings.
func llmProvidersField() configInputField

// llmTiersField parses the `llm_tiers` YAML key into cfg.LLMTiers.
func llmTiersField() configInputField

// llmRoleDefaultsField parses the `llm_role_defaults` YAML key into
// cfg.LLMRoleDefaults. Value is a map role→tier name.
func llmRoleDefaultsField() configInputField
```

### Env override

단일 JSON 문자열로 전체 맵 교체:

```
TARS_LLM_PROVIDERS_JSON='{"codex":{"kind":"openai-codex","auth_mode":"oauth"},"minimax":{"kind":"anthropic","api_key":"sk-..."}}'
TARS_LLM_TIERS_JSON='{"heavy":{"provider":"codex","model":"gpt-5.4"},...}'
TARS_LLM_DEFAULT_TIER=standard
TARS_LLM_ROLE_DEFAULTS_JSON='{"chat_main":"standard","pulse_decider":"light"}'
```

**Phase 1 YAGNI**: alias-level env override(`TARS_LLM_PROVIDER_CODEX_BASE_URL` 같은)는 alias가 동적이라 정적 env 키 등록 불가. 별도 메커니즘 필요. 후속 PR.

## Config 신규 필드 전체

기존 36개 LLM 필드 + 8개 role 필드 = **총 44개 삭제**. 다음 4개로 대체:

| Go 필드 | YAML | Env | 기본값 |
|---|---|---|---|
| `LLMProviders` | `llm_providers` | `LLM_PROVIDERS_JSON`, `TARS_LLM_PROVIDERS_JSON` | `{}` (standalone.yaml에서 seed) |
| `LLMTiers` | `llm_tiers` | `LLM_TIERS_JSON`, `TARS_LLM_TIERS_JSON` | `{}` (standalone.yaml에서 seed) |
| `LLMDefaultTier` | `llm_default_tier` | `LLM_DEFAULT_TIER`, `TARS_LLM_DEFAULT_TIER` | `"standard"` |
| `LLMRoleDefaults` | `llm_role_defaults` | `LLM_ROLE_DEFAULTS_JSON`, `TARS_LLM_ROLE_DEFAULTS_JSON` | `nil` |

## 소비자 쪽 변경

### `internal/tarsserver/helpers_llm_router.go`

```go
func buildLLMRouter(cfg config.Config, tracker *usage.Tracker) (llm.Router, error) {
    resolved, err := config.ResolveAllLLMTiers(&cfg)
    if err != nil {
        return nil, fmt.Errorf("resolve llm tiers: %w", err)
    }

    tiers := make(map[llm.Tier]llm.TierEntry, len(resolved))
    for _, r := range resolved {
        tier, err := llm.ParseTier(r.Tier)
        if err != nil {
            continue
        }
        client, err := llm.NewProvider(llm.ProviderOptions{
            Provider:        r.Kind,
            AuthMode:        r.AuthMode,
            OAuthProvider:   r.OAuthProvider,
            BaseURL:         r.BaseURL,
            WorkDir:         cfg.WorkspaceDir,
            Model:           r.Model,
            APIKey:          r.APIKey,
            ReasoningEffort: r.ReasoningEffort,
            ThinkingBudget:  r.ThinkingBudget,
            ServiceTier:     r.ServiceTier,
        })
        if err != nil {
            return nil, fmt.Errorf("tier %s: %w", r.Tier, err)
        }
        tracked := usage.NewTrackedClient(client, tracker, r.Kind, r.Model)
        tiers[tier] = llm.TierEntry{Client: tracked, Provider: r.Kind, Model: r.Model}
    }

    defaultTier, err := llm.ParseTier(cfg.LLMDefaultTier)
    if err != nil {
        return nil, fmt.Errorf("invalid llm_default_tier: %w", err)
    }

    roleDefaults := make(map[llm.Role]llm.Tier, len(cfg.LLMRoleDefaults))
    for name, tierName := range cfg.LLMRoleDefaults {
        role, ok := llm.ParseRole(name)
        if !ok {
            continue
        }
        tier, err := llm.ParseTier(tierName)
        if err != nil {
            continue
        }
        roleDefaults[role] = tier
    }

    return llm.NewRouter(llm.RouterConfig{
        Tiers:        tiers,
        DefaultTier:  defaultTier,
        RoleDefaults: roleDefaults,
    })
}
```

### `internal/tarsserver/handler_providers_models.go`

`/v1/providers`와 `/v1/models` 응답 시맨틱:
- `currentProvider` = default tier의 resolved `ProviderAlias` (또는 `Kind` — UX 결정 필요)
- `currentModel` = default tier의 resolved `Model`

여러 provider를 pool에 가진 사용자를 위해 응답에 `providers: []{alias, kind}` 배열도 추가 (dashboard가 "풀에 정의된 모든 provider 나열"을 렌더링 할 수 있게). UI 변경은 이 PR 범위 밖이지만 백엔드는 준비.

### `cmd/tars/doctor_main.go`

LLM 섹션이 `ResolveAllLLMTiers` 결과를 출력:

```
llm_tiers:
  heavy    → codex / gpt-5.4            (auth=oauth, key=present)
  standard → codex / gpt-5.4            (auth=oauth, key=present)
  light    → minimax / MiniMax-M2.7     (auth=api-key, key=present)
llm_default_tier: standard
```

Credential 체크는 `ResolvedLLMTier.APIKey`가 비어있는지 검사.

## 테스트 계획

### Unit — `internal/config/llm_resolve_test.go` (신규)

| 케이스 | 설정 | 기대 |
|---|---|---|
| 1. 단일 provider, 세 tier 전부 같은 alias | 1 pool, 3 tiers | 세 Resolved 모두 같은 `Kind`/`BaseURL`, 다른 `Model` |
| 2. 멀티 provider mix | 2 pool, tiers가 각자 다른 alias | 티어별 다른 `Kind` |
| 3. Provider가 다른 tier에서 model만 다름 | 1 pool, tiers model만 다름 | 동일 credential 공유 확인 |
| 4. ServiceTier override | provider에 priority, binding에 flex | 결과 `ServiceTier=flex` |
| 5. binding.Provider 오타 | pool={codex:...}, tier.provider="cdex" | error: "unknown provider alias" |
| 6. tier 누락 | pool={...}, tiers에 light 없음 | `ResolveAllLLMTiers`는 있는 tier만 반환. 상위(buildLLMRouter)가 전부 필요성 검증 |
| 7. Kind 빈 값 | pool={codex: {kind: ""}} | error: "provider has empty kind" |
| 8. Model 빈 값 | pool, tier에 model 누락 | error: "tier binding has empty model" |
| 9. Role defaults 파싱 | role_defaults: {chat_main: standard} | 맵이 정규화되어 저장 |

### Unit — `internal/config/llm_providers_field_test.go` (신규)

- YAML round-trip: 신 스키마 파싱 → `Config` → re-marshal → 동일
- Env override: `TARS_LLM_PROVIDERS_JSON` → `cfg.LLMProviders` 채움
- Malformed JSON → graceful (로그 + 기존값 유지)

### Unit — `internal/config/defaults_test.go` (rewrite)

- 기본값 적용: `applyLLMPoolDefaults` 동작
- provider.Kind 기반 base_url/api_key 자동 채움
- 빈 cfg → applyDefaults 후 **LLMProviders/LLMTiers는 여전히 빈 채** (standalone.yaml 없이는 error 유도)
- Role defaults 정규화

### Integration — `internal/tarsserver/helpers_llm_router_test.go` (신규 or 확장)

- 신 스키마 cfg로 `buildLLMRouter` 호출 → 3개 tier client 생성
- 잘못된 alias → 에러 메시지에 alias 이름 포함
- tier 누락 → `llm.NewRouter`가 loud 실패

### Integration — `internal/tarsserver/handler_providers_models_test.go` (rewrite)

- `/v1/providers` 응답이 default tier의 resolved kind/model 반영
- pool의 모든 provider가 응답에 나열되는지 (신규 필드)

### 삭제 대상 테스트

- `internal/config/llm_tiers_test.go` — 파일 자체 삭제
- `internal/config/defaults_test.go`의 레거시 LLM* 케이스 — 제거 후 rewrite
- `internal/tarsserver/handler_providers_models_test.go`의 `cfg.LLMProvider` 직접 주입 케이스 — rewrite

## 커밋 분할

각 커밋 후 `make test && make vet` 녹색 유지.

1. **chore: rewrite llm provider pool plan (no backward compat)** — 이 파일 업데이트. 첫 커밋.

2. **feat(config): add LLMProviderSettings, LLMTierBinding, ResolveLLMTier** — 신규 타입 + resolver + unit 테스트 (표 기반 9 케이스). 기존 필드는 그대로 둠 (컴파일 깨지지 않음). 이 시점에서 resolver는 새 필드만 읽는데, 새 필드는 아직 파서가 없어 비어 있음 → 테스트는 직접 `cfg`를 구성해서 수행.

3. **feat(config): add llm_providers / llm_tiers / llm_role_defaults / llm_default_tier parsers** — `llm_providers_field.go` 신규. `config_input_fields.go`에 4개 엔트리 추가. YAML nested map + JSON env 파싱. 테스트.

4. **refactor: cut over to provider pool schema (delete legacy LLMConfig)** — **이 PR의 핵심 대형 커밋**. 원자적으로:
   - `LLMConfig`의 9 flat 필드 + 3 tier 필드 삭제
   - `internal/config/llm_tiers.go` + `llm_tiers_test.go` 삭제
   - `config_input_fields.go`에서 36 LLM 엔트리 + 8 role 엔트리 삭제
   - `schema.go`의 `getValue` LLM/role case 삭제 + 신 case 추가
   - `applyCoreDefaults`의 LLM 섹션 제거, `applyProviderDefaults`를 `applyLLMPoolDefaults`로 재작성
   - `applyDefaults` 호출 순서 정리
   - `helpers_llm_router.go`에서 resolver 호출로 전환
   - `handler_providers_models.go`에서 resolved view 사용
   - `cmd/tars/doctor_main.go`에서 resolved view 출력
   - `config/standalone.yaml` 신 스키마로 교체
   - `defaults_test.go` LLM 섹션 rewrite
   - `handler_providers_models_test.go` 신 스키마 기반 rewrite
   - 이 시점부터 `make test` 전체 녹색 복귀

5. **test(tarsserver): integration tests for buildLLMRouter with new schema** — helpers_llm_router_test.go 신규 또는 확장. 멀티 provider mix, 오류 경로 전부.

6. **docs: update CLAUDE.md with provider pool architecture** — `## Architecture` llm 항목 + Config 항목. `TARS_LLM_PROVIDER` 같은 예시 env var 업데이트.

7. **docs: note user-local config migration in PR description** — (PR description에만, 별도 커밋 아님. workspace/config/tars.config.yaml 직접 수정은 user action.)

총 **6개 커밋**. 4번이 가장 크지만 내부적으로 완전히 일관된 단일 스냅샷.

## Acceptance Criteria

- [ ] `LLMConfig`에 레거시 flat 필드 0개
- [ ] `config_input_fields.go`의 LLM 관련 엔트리 정확히 4개 (providers/tiers/default_tier/role_defaults)
- [ ] `internal/config/llm_tiers.go` 파일 부재
- [ ] 9개 resolver 테스트 전부 녹색
- [ ] `config/standalone.yaml`을 그대로 쓰면 `make test`, `make vet`, `make build` 전부 통과
- [ ] `tars doctor` 출력이 resolved view를 보여줌 (`heavy → codex / gpt-5.4` 형태)
- [ ] 잘못된 alias 참조 시 startup 에러가 alias 이름을 포함
- [ ] CLAUDE.md의 llm 관련 문구가 pool 스키마 반영
- [ ] PR description에 user-local config 마이그레이션 예시 포함
- [ ] `make test && make vet && make fmt` 전부 녹색

## Identity Check

- **단일 Go 바이너리**: 새 런타임 의존성 없음, YAML 파서는 기존 것 재사용 ✓
- **File-first, DB-less**: config는 YAML 그대로, 새로운 저장소 없음 ✓
- **Scope isolation**: pulse/reflection 경로 무변경, llm 패키지 인터페이스 무변경 ✓
- **정책은 config / 메커니즘은 Go**: 풀 정의는 config, 해석 로직은 Go ✓
- **Memory/reflection 경로 회귀 없음**: router 인터페이스 유지, role→tier 매핑 동작 동일 ✓

## Phase 1 YAGNI / 명시적 배제

1. **Provider fallback chain** (`llm_fallback_order: [codex, anthropic_direct]`) — router 패키지에 retry 로직 추가 필요. 별도 설계.
2. **Per-task / per-agent provider override** — `docs/plans/hermes-improvements/03-provider-override.md`. 이 PR이 그 작업의 전제 조건.
3. **Dashboard config UI** — `frontend/console/src/components/Config.svelte` 수정. 백엔드 API 준비만 하고 UI는 후속.
4. **Migration CLI** (`tars config migrate`) — 혼자 쓰는 프로젝트이므로 수동 마이그레이션으로 충분.
5. **Memory embed provider 통합** (`memory_embed_*` 5개 필드를 같은 풀에 통합) — concept 적합, 구현 분량 큼. 별도 PR.
6. **Alias 단위 env var alias** (`TARS_LLM_PROVIDER_CODEX_BASE_URL`) — 동적 이름이라 정적 등록 불가. 별도 메커니즘.
7. **`standalone.yaml`에 멀티 provider 예시** — 복잡해지니 주석으로만 언급, 실제 seed는 단일 `default` anthropic.

## 명시적으로 삭제하는 것

- `cfg.LLMProvider`, `cfg.LLMAuthMode`, `cfg.LLMOAuthProvider`, `cfg.LLMBaseURL`, `cfg.LLMAPIKey`, `cfg.LLMModel`, `cfg.LLMReasoningEffort`, `cfg.LLMThinkingBudget`, `cfg.LLMServiceTier` — 9 flat 필드
- `cfg.LLMTierHeavy`, `cfg.LLMTierStandard`, `cfg.LLMTierLight` (`LLMTierSettings` struct 포함) — 3 tier 필드 + 해당 struct
- `internal/config/llm_tiers.go` 파일 전체 — `seedTier`, `EnsureLLMTierDefaults`, `llmTierStringField`, `llmTierIntField`, `llmRoleField`, tierSpec* 전부
- `internal/config/llm_tiers_test.go` 파일 전체
- `config_input_fields.go`의 `llm_provider`/`llm_auth_mode`/.../`llm_tier_*_*`/`llm_role_*` 엔트리 44개
- `schema.go`의 해당 `getValue` case 17개
- `defaults_apply.go`의 `applyProviderDefaults` (새 `applyLLMPoolDefaults`로 교체)
- 환경변수 alias 88개 (36×2 + 8×2)
- `TARS_LLM_PROVIDER`, `TARS_LLM_API_KEY` 같은 모든 레거시 env 이름 (신 이름 `TARS_LLM_PROVIDERS_JSON` 등으로 교체)

## 사용자 액션 필요

이 PR이 머지되는 순간 `workspace/config/tars.config.yaml`이 **구 스키마로 되어 있다면 startup이 실패한다**. PR description에 마이그레이션 예시를 명시하고, 머지 전에 사용자가 로컬 config를 업데이트해야 한다. (`config/standalone.yaml`의 checked-in 버전은 이 PR 안에서 함께 교체되므로 checked-in 경로로 부팅하는 테스트는 자동으로 동작.)
