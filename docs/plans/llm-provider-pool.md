# LLM Provider Pool — Named Provider 풀 + Tier Binding 리팩터

**Branch**: `feat/llm-provider-pool`
**Status**: Planning
**Area**: `area/llm`, `area/config`, `area/server`
**Depends on**: 3-tier model routing (이미 머지됨)
**Blocks**: docs/plans/hermes-improvements/03-provider-override.md (task-level override)

## 배경

3-tier model routing이 최근 도입되면서 `llm_tier_{heavy,standard,light}_*` 형태의 플랫 필드 27개가 추가되었다. 레거시 `llm_*` 9개와 합쳐 LLM 관련 config 필드는 **36개 + 환경변수 alias 72개**에 이른다.

운영하면서 다음 문제가 드러났다:

### 1. 자격증명 중복

같은 provider(예: 동일한 Anthropic 직접 키)를 두 개 이상의 티어가 공유해도 `base_url`, `api_key`, `auth_mode`, `oauth_provider`를 티어마다 한 벌씩 적어야 한다. 키 로테이션 시 수정 지점이 N배로 늘어나고, "내가 가진 계정이 몇 개인가"라는 사용자의 멘탈 모델과 어긋난다.

### 2. 오타에 무방비

실제 사용자가 작성한 다음 config에 silent bug가 있었다:

```yaml
llm_tier_light_provider: anthropic
llm_tier_light_light_base_url: https://api.minimax.io/anthropic   # ← "light" 두 번
llm_tier_light_model: MiniMax-M2.7
```

`llm_tier_light_light_base_url`는 `configInputFields`에 등록되지 않은 키라 **silently ignored**되고, light 티어가 minimax 모델을 codex 엔드포인트로 호출하려다 실패한다. 27개 짜리 긴 플랫 이름은 구조적으로 이 종류 오타에 취약하다.

### 3. 스케일 한계

티어를 4개로 늘리거나 role별 credential override를 지원하는 순간 필드 수가 선형으로 폭발한다. 환경변수 alias도 동반 폭발해 12-factor 환경에서 다루기 어려워진다.

### 4. Concept mismatch

사용자는 "**codex 계정 1개, anthropic 직접키 1개, minimax 키 1개**를 가지고 있고, 각 티어에 그중 하나를 배정한다"고 사고한다. 현재 스키마는 "각 티어가 독립적인 9-필드 묶음을 갖는다"는 모델이라 개념과 어긋난다.

### 5. 외부 레퍼런스가 같은 결론

- **OpenClaw** (`src/config/types.models.ts`): `models.providers: Record<string, ModelProviderConfig>` — provider를 alias 키로 한 번만 정의, agent는 `"provider/model"` 문자열로 참조. credential은 provider당 한 번만.
- **Hermes Agent** (`hermes_cli/config.py`): 플랫 per-context 필드를 쓰지만 별도로 `fallback_providers` 배열로 cross-provider fallback을 표현. credential은 env var 키 이름으로만 참조.

OpenClaw가 우리가 원하는 그림에 정확히 부합한다.

## 목적

1. **Named provider pool**을 도입한다: `llm_providers: {alias: {...}}` 맵을 config에 추가하고, credential/base_url/auth_mode를 그 alias 단위로 한 번만 정의한다.
2. **Tier binding**을 분리한다: `llm_tiers: {heavy: {provider: <alias>, model: <name>, ...}}` — 티어는 "어느 alias + 어느 모델 + knobs"만 지정한다.
3. **Backward compat**: 기존 `llm_*` 플랫 필드와 `llm_tier_<x>_*` 플랫 필드를 한 동안 그대로 동작시킨다. 사용자가 한 글자도 안 바꿔도 동일하게 작동.
4. **Single resolution path**: router builder는 `ResolveLLMTier(cfg, tier)` 한 함수만 호출. 신/구 경로 분기는 resolver 내부에 갇힌다.
5. **Phase 1 YAGNI**: provider fallback chain, per-task override, dashboard UI 변경, migration CLI는 모두 후속 PR.

## 설계 원칙

1. **Credential은 provider 단위**: model/reasoning_effort 같은 호출 파라미터와 분리.
2. **Tier는 binding 1줄**: provider alias + model + 옵셔널 knobs.
3. **Resolver 계층 단일화**: 신구 경로의 분기는 `internal/config/llm_resolve.go` 하나에 격리. 다른 코드는 `ResolvedLLMTier` 구조체만 본다.
4. **Hard backward compat**: 기존 config 파일/환경변수는 무수정 동작. 새 스키마는 opt-in.
5. **Forward-rename 안전**: `kind`(provider 종류) 필드는 의도적으로 `provider`라 부르지 않는다. 그래야 tier binding의 `provider`(=alias 참조)와 혼동되지 않는다.
6. **Identity check 통과**: 단일 Go 바이너리, file-first, scope isolation 영향 없음, 정책은 config / 메커니즘은 Go.

## 현황 진단

### LLM config가 흐르는 경로

```
YAML/Env → applyDefaults() → EnsureLLMTierDefaults() → cfg.LLMTier{Heavy,Standard,Light}
        → buildLLMRouter() in helpers_llm_router.go
        → llm.NewProvider() per tier
        → usage.NewTrackedClient
        → llm.NewRouter(Tiers, DefaultTier, RoleDefaults)
```

### `cfg.LLM*` 플랫 필드 사용처 (변경 영향 범위)

| 파일 | 사용 | 변경 필요? |
|---|---|---|
| `internal/config/defaults_apply.go` | `applyProviderDefaults`, `applyCoreDefaults` | 신 스키마 통합 |
| `internal/config/config_input_fields.go` | YAML/env 파싱 등록 | 신 필드 추가 |
| `internal/config/schema.go` | `getValue` switch | 신 필드 case 추가 |
| `internal/config/llm_tiers.go` | `seedTier`, `EnsureLLMTierDefaults` | resolver로 흡수 |
| `internal/tarsserver/helpers_llm_router.go` | `buildLLMRouter` | resolver 호출로 교체 |
| `internal/tarsserver/main_bootstrap.go` | router 기동 | 변화 없음 (router 인터페이스 유지) |
| `internal/tarsserver/handler_providers_models.go` | `/v1/providers`, `/v1/models` 응답 | "current provider"가 무엇인지 재정의 (Phase 1: 기본 티어의 resolved provider) |
| `cmd/tars/doctor_main.go` | `tars doctor` 출력 | 신 스키마 인지 (Phase 1: 레거시 필드 위주 유지, doctor는 resolved view 출력) |
| `internal/config/defaults_test.go` | 회귀 테스트 | 신 케이스 추가, 기존 통과 |

### Router 측은 변화 거의 없음

`internal/llm/router.go`의 `RouterConfig.Tiers map[Tier]TierEntry` 인터페이스는 그대로 둔다. resolver의 출력이 그대로 `TierEntry`로 흘러들어가므로 router 패키지는 한 줄도 안 바뀐다.

## 제안 스키마

### Before (현재)

질문에 등장한 사용자 config:

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
llm_tier_light_base_url: https://api.minimax.io/anthropic
llm_tier_light_model: MiniMax-M2.7
llm_tier_light_auth_mode: api-key
llm_tier_light_api_key: ${MINIMAX_API_KEY}
```

### After (제안)

```yaml
# 1) Named provider pool — credential은 alias당 한 번
llm_providers:
  codex:
    kind: openai-codex
    auth_mode: oauth
    oauth_provider: openai-codex
    base_url: https://chatgpt.com/backend-api
    service_tier: priority

  minimax:
    kind: anthropic                 # API shape
    auth_mode: api-key
    api_key: ${MINIMAX_API_KEY}
    base_url: https://api.minimax.io/anthropic

llm_default_provider: codex          # alias 한 줄로 전체 기본값 지정

# 2) Tier binding — provider alias 참조 + model + knobs
llm_tiers:
  heavy:
    provider: codex                  # alias 참조
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

# 3) Role → tier 매핑은 그대로
llm_role_defaults:
  chat_main: standard
  pulse_decider: light
  gateway_planner: heavy
```

**라인 수**: 19줄 → 26줄 (구조가 보이는 만큼 약간 늘지만, 자격증명 중복 제거 + 의미 명료화로 net 가독성은 크게 향상). 4번째 티어/추가 alias 도입 시 신 스키마는 alias 1개 + 라인 5~6줄만 추가, 구 스키마는 9줄.

### 또 다른 사용 패턴 (참고)

같은 풀을 재사용해 multi-vendor mix:

```yaml
llm_providers:
  codex_oauth:
    kind: openai-codex
    auth_mode: oauth
    oauth_provider: openai-codex
    base_url: https://chatgpt.com/backend-api
  anthropic_direct:
    kind: anthropic
    auth_mode: api-key
    api_key: ${ANTHROPIC_API_KEY}
    base_url: https://api.anthropic.com
  gemini_flash:
    kind: gemini
    auth_mode: api-key
    api_key: ${GEMINI_API_KEY}

llm_tiers:
  heavy:    { provider: anthropic_direct, model: claude-opus-4-6,   reasoning_effort: high }
  standard: { provider: codex_oauth,      model: gpt-5.4,            reasoning_effort: medium }
  light:    { provider: gemini_flash,     model: gemini-2.5-flash,   reasoning_effort: minimal }
```

## 새 데이터 모델

### Go 타입

```go
// internal/config/types.go

// LLMProviderSettings is one entry in the named provider pool. It holds
// "where to call + how to authenticate" but NOT "what model to call".
// Models are bound at the tier level (LLMTierBinding.Model) so that one
// provider can serve multiple models.
type LLMProviderSettings struct {
    Kind          string  `json:"kind"             yaml:"kind"`
    AuthMode      string  `json:"auth_mode"        yaml:"auth_mode"`
    OAuthProvider string  `json:"oauth_provider"   yaml:"oauth_provider"`
    BaseURL       string  `json:"base_url"         yaml:"base_url"`
    APIKey        string  `json:"api_key"          yaml:"api_key"`
    ServiceTier   string  `json:"service_tier"     yaml:"service_tier"`
}

// LLMTierBinding binds a tier to a provider alias + model + per-call knobs.
// When Provider is the name of an entry in cfg.LLMProviders, that pool
// entry supplies kind/auth/base_url/api_key. When Provider is a raw kind
// (e.g. "anthropic"), an anonymous provider is synthesized at resolve time.
type LLMTierBinding struct {
    Provider        string `json:"provider"         yaml:"provider"`
    Model           string `json:"model"            yaml:"model"`
    ReasoningEffort string `json:"reasoning_effort" yaml:"reasoning_effort"`
    ThinkingBudget  int    `json:"thinking_budget"  yaml:"thinking_budget"`
    ServiceTier     string `json:"service_tier"     yaml:"service_tier"`
}

// LLMConfig (additions)
type LLMConfig struct {
    // ── legacy flat fields (kept for backward compat) ───────────────────
    LLMProvider        string
    LLMAuthMode        string
    LLMOAuthProvider   string
    LLMBaseURL         string
    LLMAPIKey          string
    LLMModel           string
    LLMReasoningEffort string
    LLMThinkingBudget  int
    LLMServiceTier     string

    // ── legacy per-tier flat fields (kept for backward compat) ──────────
    LLMTierHeavy    LLMTierSettings
    LLMTierStandard LLMTierSettings
    LLMTierLight    LLMTierSettings

    // ── NEW: named provider pool ────────────────────────────────────────
    LLMProviders       map[string]LLMProviderSettings
    LLMDefaultProvider string

    // ── NEW: tier bindings (replaces flat tier fields long-term) ────────
    LLMTiers map[string]LLMTierBinding

    // ── existing ────────────────────────────────────────────────────────
    LLMDefaultTier  string
    LLMRoleDefaults map[string]string
}
```

### Resolver 출력 타입

```go
// internal/config/llm_resolve.go

// ResolvedLLMTier is the final, flat view of one tier's effective LLM
// configuration after merging the named provider pool, tier bindings,
// and legacy flat fields. The router builder consumes this struct
// directly — it never reads cfg.LLMTier* or cfg.LLMProviders.
type ResolvedLLMTier struct {
    Tier            string  // "heavy" | "standard" | "light"

    // From provider pool / synthesized
    Kind            string  // "anthropic" | "openai" | "openai-codex" | ...
    AuthMode        string
    OAuthProvider   string
    BaseURL         string
    APIKey          string

    // From tier binding
    Model           string
    ReasoningEffort string
    ThinkingBudget  int
    ServiceTier     string  // tier override > provider default > legacy

    // Provenance — for tars doctor and logs
    ProviderSource  string  // "pool:<alias>" | "tier_flat" | "legacy"
    BindingSource   string  // "llm_tiers"     | "tier_flat" | "legacy"
}

// ResolveLLMTier returns the effective settings for the given tier.
// Resolution order:
//
//   1. cfg.LLMTiers[tier] exists?
//      a. binding.Provider is a key in cfg.LLMProviders → use that pool entry
//      b. binding.Provider is a known kind → synthesize an anonymous pool entry
//      c. provenance: ProviderSource = "pool:<name>" | "synth:<kind>"
//                     BindingSource  = "llm_tiers"
//   2. else cfg.LLMTier{Heavy,Standard,Light} has any non-empty field?
//      → existing seedTier behavior (legacy tier flat fields)
//      → ProviderSource = "tier_flat", BindingSource = "tier_flat"
//   3. else → fall back to cfg.LLM* legacy flat fields
//      → ProviderSource = "legacy",   BindingSource = "legacy"
//
// Errors when:
//   - tier name is unknown
//   - cfg.LLMTiers[tier].Provider references a missing alias AND is not a known kind
//   - resolved Kind is empty (no provider configured anywhere)
func ResolveLLMTier(cfg *Config, tier string) (ResolvedLLMTier, error)

// ResolveAllLLMTiers returns resolved settings for heavy/standard/light.
// Used by buildLLMRouter as a single entry point.
func ResolveAllLLMTiers(cfg *Config) (map[string]ResolvedLLMTier, error)
```

## 해석 우선순위

```
For each tier in {heavy, standard, light}:

  IF cfg.LLMTiers[tier] is set (new schema):
    binding := cfg.LLMTiers[tier]
    IF binding.Provider in cfg.LLMProviders:
      base := cfg.LLMProviders[binding.Provider]
      ProviderSource = "pool:<alias>"
    ELIF binding.Provider in known kinds (anthropic|openai|openai-codex|gemini|gemini-native|claude-code-cli):
      base := synthesize from binding.Provider + legacy fields
      ProviderSource = "synth:<kind>"
    ELSE:
      ERROR: "tier %s: provider %q not found in llm_providers and is not a known kind"

    Resolved.Kind            = base.Kind
    Resolved.AuthMode        = base.AuthMode
    Resolved.OAuthProvider   = base.OAuthProvider
    Resolved.BaseURL         = base.BaseURL
    Resolved.APIKey          = base.APIKey
    Resolved.Model           = binding.Model
    Resolved.ReasoningEffort = binding.ReasoningEffort
    Resolved.ThinkingBudget  = binding.ThinkingBudget
    Resolved.ServiceTier     = first non-empty of (binding.ServiceTier, base.ServiceTier)
    BindingSource            = "llm_tiers"

  ELIF cfg.LLMTier<X> has any non-empty field (current legacy):
    Resolved = cfg.LLMTier<X>  (already seeded by legacy seedTier)
    ProviderSource = "tier_flat"
    BindingSource  = "tier_flat"

  ELSE:
    Resolved = cfg.LLM* legacy flat fields
    ProviderSource = "legacy"
    BindingSource  = "legacy"
```

`buildLLMRouter`는 위 결과를 그대로 `llm.NewProvider`로 흘려보낸다.

## 마이그레이션 전략

### Phase 1 (이 PR): 신구 병존

- 신 스키마(`llm_providers`, `llm_tiers`, `llm_default_provider`)가 우선 적용된다.
- 기존 플랫 필드(`llm_tier_*_*`, `llm_*`)는 모두 그대로 동작한다.
- `EnsureLLMTierDefaults`는 호출이 유지되지만, 내부적으로 새 resolver와 일치하는 결과를 내도록 정리(또는 그대로 두고 resolver가 두 경로를 모두 확인).
- 새 schema의 default tier는 두 가지 경로:
  1. `llm_default_tier`가 명시되면 그 값
  2. 없으면 `cfg.LLMTiers`에 정의된 tier 중 첫 번째 (heavy/standard/light 순)
  3. 둘 다 없으면 기존 fallback("standard")
- **비파괴 보장**: 기존 모든 회귀 테스트 그대로 통과.

### Phase 2 (후속 PR): 권장 경로 전환

- `config/standalone.yaml`의 티어 주석 템플릿을 신 스키마로 교체.
- `tars doctor`가 레거시 플랫 티어 필드 사용 시 hint 출력 ("consider migrating to llm_providers / llm_tiers").
- README / GETTING_STARTED 신 스키마 우선 설명.

### Phase 3 (후속 PR): Deprecation 경고

- 서버 부팅 시 `llm_tier_*_provider` 같은 플랫 티어 필드 검출되면 `zlog.Warn` 1회 출력.

### Phase 4 (장기): 플랫 티어 필드 제거

- `llm_tier_<x>_*` 27개 필드 + 환경변수 alias 54개 삭제.
- 레거시 `llm_*` 9개 단일 필드는 **유지**한다 (가장 단순한 single-provider 운영자에게 가치 있음).

## 신규 / 수정 / 삭제 파일

### 신규
- `internal/config/llm_resolve.go` — `ResolvedLLMTier`, `ResolveLLMTier`, `ResolveAllLLMTiers`
- `internal/config/llm_resolve_test.go` — 표 기반 케이스: pool / pool+synth / legacy tier flat / pure legacy / 우선순위 / 에러 경로
- `internal/config/llm_providers_field.go` — `llmProvidersField()`, `llmTiersField()` YAML/env 파서 (구조는 기존 `usagePriceOverridesField` 참조)
- `internal/config/llm_providers_field_test.go` — JSON env override, nested YAML 라운드트립

### 수정
- `internal/config/types.go` — `LLMProviderSettings`, `LLMTierBinding` 추가; `LLMConfig`에 `LLMProviders`, `LLMDefaultProvider`, `LLMTiers` 추가
- `internal/config/config_input_fields.go` — `llm_providers_json`, `llm_tiers_json`, `llm_default_provider` 등록
- `internal/config/llm_tiers.go` — `EnsureLLMTierDefaults` 호출 흐름 유지하되 resolver에 위임 (또는 unchanged + resolver가 양쪽 경로 모두 검사)
- `internal/config/defaults_apply.go` — `applyDefaults` 마지막에 `EnsureLLMConfigResolved` (resolver invariant 검증)
- `internal/config/schema.go` — `getValue` switch에 `llm_providers`, `llm_default_provider`, `llm_tiers` 추가
- `internal/tarsserver/helpers_llm_router.go` — `cfg.LLMTier*` 직접 접근 → `config.ResolveAllLLMTiers(cfg)` 호출. usage tracker는 `resolved.Kind`, `resolved.Model` 사용
- `internal/tarsserver/handler_providers_models.go` — `currentProvider`/`currentModel`을 `ResolveLLMTier(cfg, defaultTier)` 결과로 계산 (현재는 `cfg.LLMProvider`/`cfg.LLMModel` 직접 사용)
- `internal/config/defaults_test.go` — 신 스키마 기본값/병존 케이스 추가 (기존 케이스는 그대로 통과)
- `cmd/tars/doctor_main.go` — `cfg.LLMProvider` 출력 → resolved view 우선 출력 (legacy fall-through 유지)

### 변경 없음 (의도적)
- `internal/llm/router.go`, `tier.go`, `role.go`, `provider.go` — router 인터페이스 유지
- `internal/llm/anthropic.go`, `openai_*.go`, `gemini_*.go` — provider client 코드
- `frontend/console/**` — Phase 1 UI 변경 없음 (Phase 2에서 다룸)

### 삭제
- 없음. 모든 기존 필드는 backward compat을 위해 유지.

## Config 신규 필드

| Go 필드 | YAML | Env | 기본값 | 비고 |
|---|---|---|---|---|
| `LLMProviders` | `llm_providers` | `LLM_PROVIDERS_JSON`, `TARS_LLM_PROVIDERS_JSON` | `nil` | nested YAML 또는 JSON 문자열 |
| `LLMDefaultProvider` | `llm_default_provider` | `LLM_DEFAULT_PROVIDER`, `TARS_LLM_DEFAULT_PROVIDER` | `""` | alias 이름; 없으면 첫 alias 사용 |
| `LLMTiers` | `llm_tiers` | `LLM_TIERS_JSON`, `TARS_LLM_TIERS_JSON` | `nil` | nested YAML 또는 JSON 문자열 |

`llm_providers`와 `llm_tiers`는 nested map이므로 단일 환경변수로 override할 때는 JSON 문자열을 받는다 (`usage_price_overrides_json`과 동일 패턴). 개별 필드 단위 env override(`TARS_LLM_PROVIDER_CODEX_BASE_URL` 같은)는 **Phase 1에서는 추가하지 않는다** — alias 이름이 동적이라 환경변수 이름을 정적으로 등록할 수 없고, dynamic env scan은 별도 메커니즘이 필요하다. 필요하면 후속 PR에서 도입.

### YAML 파싱 노트

`configInputFields`는 현재 평면 키 → setter 구조다. nested map은 다음 두 가지 방식 중 하나로 처리한다:

**옵션 A**: `usagePriceOverridesField`처럼 별도 파서 함수를 등록 — YAML 로더가 raw `interface{}` 값을 받아 직접 unmarshal한다.

**옵션 B**: `gopkg.in/yaml.v3`의 `Node` API로 raw 노드를 받아 `yaml.Unmarshal`로 struct에 매핑한다.

A안이 기존 패턴과 정합. **결정**: 옵션 A 채택. 구현 시 `usagePriceOverridesField` 코드를 직접 참조한다 (`internal/config/usage_price_overrides_field.go` — 존재 확인 필요).

## Env override 전략

1. **Full pool replacement**: `TARS_LLM_PROVIDERS_JSON='{"codex":{"kind":"openai-codex","auth_mode":"oauth"},"minimax":{...}}'`
2. **Default provider**: `TARS_LLM_DEFAULT_PROVIDER=codex`
3. **Full tier bindings**: `TARS_LLM_TIERS_JSON='{"heavy":{"provider":"codex","model":"gpt-5.4"},...}'`
4. **레거시 env 그대로**: `TARS_LLM_PROVIDER`, `TARS_LLM_API_KEY`, ... 모두 동작.

12-factor 환경에서 secret 주입은:
- `${MINIMAX_API_KEY}` 같은 yaml-time placeholder는 현재 config 로더가 지원하면 그대로, 아니면 운영자가 `TARS_LLM_PROVIDERS_JSON`을 secret store에서 만들어 주입.
- (Phase 1에서는 placeholder 동작 여부를 별도 확인하고, 필요 시 별 PR로 처리)

## 테스트 계획

### Unit — `internal/config/llm_resolve_test.go`

표 기반 케이스로 모든 우선순위 분기를 커버:

| 케이스 | LLM* | LLMTier<X> | LLMProviders | LLMTiers | 기대 결과 |
|---|---|---|---|---|---|
| 1. 순수 레거시 | set | empty | nil | nil | legacy로 해석, ProviderSource="legacy" |
| 2. 레거시 + 티어 플랫 (현 동작) | set | partial | nil | nil | seedTier 결과, ProviderSource="tier_flat" |
| 3. 신 스키마 (pool + binding) | empty | empty | set | set | pool에서 해석, ProviderSource="pool:<alias>" |
| 4. 신 스키마, alias 미정의 | set | empty | nil | provider="anthropic" | synth, ProviderSource="synth:anthropic" |
| 5. 신 스키마, alias 잘못 | empty | empty | {codex:...} | provider="cdex" (오타) | error: not found and not known kind |
| 6. 신 스키마 + 레거시 티어 동시 존재 | set | set | set | set | 신 스키마 우선 (binding 있는 티어는 신, 없는 티어는 legacy fallback) |
| 7. 신 스키마, default_provider만 지정 | set | empty | {codex:...} | nil | tiers 미정의 → 레거시 경로로 떨어짐 (default_provider는 무시되거나 별도 동작) |
| 8. ServiceTier override | - | - | {codex: service_tier=priority} | {heavy: service_tier=flex} | 결과 ServiceTier=flex (binding 우선) |
| 9. 모든 게 비어 있음 | empty | empty | nil | nil | 기본 anthropic + 기본 모델 (applyDefaults 결과) |

각 케이스에 대해 `ResolveLLMTier(cfg, "heavy")`, `"standard"`, `"light"` 모두 검증.

### Unit — `internal/config/llm_providers_field_test.go`

- nested YAML 정상 파싱
- JSON env override (`TARS_LLM_PROVIDERS_JSON`)
- 잘못된 JSON → graceful (기존값 유지 + 로그)
- merge 동작 (하나는 YAML, 하나는 env)

### Unit — `internal/config/defaults_test.go` (확장)

- 신 스키마 단독 사용 시 `applyDefaults` 결과
- 레거시 단독 사용 시 동일 (회귀)
- 두 스키마 동시 사용 시 우선순위

### Integration — `internal/tarsserver/helpers_llm_router_test.go` (신규 또는 확장)

- 신 스키마로 구성된 cfg → `buildLLMRouter` → 3개 tier 모두 client 생성 성공
- 레거시 cfg → 동일 (회귀)
- 잘못된 alias 참조 → 에러 메시지에 alias 이름 포함

### Integration — `internal/tarsserver/handler_providers_models_test.go` (확장)

- `/v1/providers` 응답이 신 스키마에서도 올바른 current_provider를 반환

### 회귀 — 모든 기존 LLM 관련 테스트

`make test` 한 줄도 깨지면 안 된다. 이게 backward compat의 정의다.

## 커밋 분할 (작업 순서)

각 커밋 후 `make test && make vet` 녹색 유지. 빌드 깨지는 중간 커밋 금지.

1. **chore: add llm-provider-pool plan doc** — 이 파일 커밋
2. **feat(config): add LLMProviderSettings and LLMTierBinding types** — 타입만 추가. `LLMConfig`에 빈 필드 추가. 컴파일은 통과하지만 어디서도 안 읽음. 테스트 없음.
3. **feat(config): add llm_providers / llm_tiers / llm_default_provider yaml parsers** — `llmProvidersField`, `llmTiersField`, `stringField("llm_default_provider", ...)` 등록. YAML/env 모두 동작. 파싱 라운드트립 테스트.
4. **feat(config): add ResolveLLMTier resolver** — `internal/config/llm_resolve.go` 신규. `ResolvedLLMTier`, `ResolveLLMTier`, `ResolveAllLLMTiers`. 표 기반 unit 테스트 (위 9개 케이스 전부). 아직 호출하는 곳 없음.
5. **refactor(server): switch buildLLMRouter to use ResolveAllLLMTiers** — `helpers_llm_router.go`에서 `cfg.LLMTier*` 직접 접근 제거, resolver 호출. 기존 회귀 테스트 그대로 통과 확인.
6. **refactor(server): switch handler_providers_models to resolved view** — `currentProvider`/`currentModel`이 default tier의 resolved 결과를 사용. 응답 필드는 그대로지만 의미가 "현재 활성 default tier"로 명료화. handler 테스트 추가/수정.
7. **refactor(cli): tars doctor uses resolved view** — `doctor_main.go` LLM 섹션이 resolved view 출력. 레거시 fall-through 유지. doctor 테스트 추가.
8. **test(config): add integration cases for new schema in defaults_test.go** — 신 스키마 단독, 두 스키마 동시 케이스. 전부 녹색.
9. **docs: update CLAUDE.md with provider pool architecture** — `## Architecture` 의 `llm` 항목과 `Config` 항목에 신 스키마 설명 한 단락 추가.

총 9개 커밋. PR1 pulse 작업과 비슷한 규모.

> NOTE: standalone.yaml 템플릿 교체와 README 갱신은 **이 PR에 포함하지 않는다**. Phase 2 별도 PR에서 다룬다 — 템플릿 교체는 사용자가 보는 첫 인상을 바꾸므로 burn-in 후 진행이 안전.

## 체크리스트

- [ ] Plan 파일 커밋 (this commit)
- [ ] `LLMProviderSettings`, `LLMTierBinding` 타입 추가
- [ ] `llm_providers` / `llm_tiers` / `llm_default_provider` YAML/env 파싱
- [ ] `ResolveLLMTier` resolver + 9개 표 기반 테스트
- [ ] `buildLLMRouter` resolver 사용으로 전환
- [ ] `handler_providers_models` resolved view 사용
- [ ] `tars doctor` resolved view 출력
- [ ] `defaults_test.go` 신 스키마 케이스 추가
- [ ] CLAUDE.md 업데이트
- [ ] `make test` 녹색
- [ ] `make vet` 녹색
- [ ] PR 생성 + CI 통과

## Acceptance Criteria

- [ ] 사용자가 위 "After" 예시 YAML로 config를 작성하면 그대로 동작
- [ ] 기존 사용자가 한 글자도 안 바꿔도 동일하게 동작 (회귀 0)
- [ ] 잘못된 alias 참조 시 명확한 에러 메시지 (alias 이름 포함)
- [ ] `ResolvedLLMTier`의 `ProviderSource`/`BindingSource`가 `tars doctor`와 부팅 로그에 노출되어 운영자가 어떤 경로로 해석되었는지 추적 가능
- [ ] 9개 resolver 테스트 케이스 전부 통과
- [ ] `make test`, `make vet`, `make fmt` 통과
- [ ] PR description에 before/after YAML 비교 포함

## Identity Check

- **단일 Go 바이너리**: 새 런타임 의존성 없음, YAML 파서는 기존 것 재사용 ✓
- **File-first**: config는 YAML 그대로, 새로운 외부 저장소 없음 ✓
- **Scope isolation**: pulse/reflection 경로 무변경, llm 패키지 인터페이스 무변경 ✓
- **정책은 config / 메커니즘은 Go**: 풀 정의는 config, 해석 로직은 Go ✓
- **Backward compat hard rule**: 기존 config는 무수정 동작 ✓
- **YAGNI**: fallback chain, per-task override, dashboard UI는 명시적으로 후속 PR로 분리 ✓

## Phase 1 YAGNI / 명시적 배제

다음 항목은 **이번 PR에 포함하지 않는다**. 별도 PR에서 다룬다.

1. **Provider fallback chain** (`llm_fallback_order: [codex, anthropic_direct]`) — 흥미롭지만 router 패키지에 retry 로직을 추가해야 함. 별도 설계 필요.
2. **Per-task / per-agent provider override** — `docs/plans/hermes-improvements/03-provider-override.md`에서 다룸. 이 PR이 그 작업의 전제 조건임.
3. **Dashboard config UI** — `frontend/console/src/components/Config.svelte` 수정. 신 스키마가 burn-in 된 후 진행.
4. **Migration CLI** (`tars config migrate`) — 자동 변환 CLI. 우선 사용자가 직접 옮기게 두고, 충분한 사용자가 새 스키마로 옮기지 않으면 도입.
5. **Memory embed provider 통합** (`memory_embed_*` 5개 필드를 같은 풀에 통합) — concept 적합, 구현 분량 큼. 별도 PR.
6. **Provider 이름 단위 환경변수 alias** (`TARS_LLM_PROVIDER_CODEX_BASE_URL`) — alias가 동적이라 env 키 정적 등록 불가. 별도 메커니즘 필요.
7. **Standalone.yaml 템플릿 교체** — Phase 2 별도 PR.
8. **Deprecation warning** — Phase 3 별도 PR.
9. **플랫 티어 필드 제거** — Phase 4, 충분한 burn-in 후.

## 의도적으로 유지하는 것들

- **레거시 `llm_*` 9개 필드**: 가장 단순한 single-provider 운영자에게 여전히 가장 짧은 경로. 영구 유지.
- **`llm_role_defaults` 매핑**: 신/구 스키마와 직교. 그대로 사용.
- **`llm.Router` 인터페이스**: tier-based 추상화는 충분히 좋음. 변경 없음.
- **3-tier 고정 (heavy/standard/light)**: tier 개수 일반화는 별 가치 없고 Phase 1 범위 밖.
