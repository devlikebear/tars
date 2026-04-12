# 03 — Per-task Provider/Credential Override

**Branch**: `feat/hermes-provider-override`
**Sprint**: S2
**Area**: `area/gateway`, `area/llm`
**Depends on**: —
**Blocks**: #04 (MoA consensus)

## 배경

Hermes Agent는 서브에이전트를 스폰할 때 provider/모델/credential을 덮어쓸 수 있다. 덕분에 "무거운 설계 질의는 Anthropic opus, 파싱은 gemini flash, 테스트 생성은 openai mini"처럼 태스크 성격별 최적 조합이 가능하다.

TARS는 3-tier binding(heavy/standard/light)으로 tier 이동은 되지만, tier 안에서 provider alias까지 태스크 단위로 바꾸는 건 불가능하다. 이건 MoA(#04)의 전제조건이기도 하다.

> **선행 작업**: 이 설계는 PR #323(`refactor(llm): named provider pool + tier binding schema`) 이후 스키마를 전제로 한다. `cfg.LLMProviders`(alias → `LLMProviderSettings`: kind/auth/credential)와 `cfg.LLMTiers`(tier → `LLMTierBinding`: alias + model)가 이미 분리돼 있다. 이 PR은 해당 alias 체계 위에 "태스크 단위 alias 선택"을 덧붙인다.

## 목적

1. `subagents_run`의 **입력 task struct**와 `gateway.SpawnRequest`에 optional `ProviderOverride`를 관통시킨다. Override는 **pool alias**를 참조하며 credential을 새로 받지 않는다.
2. 안전을 위해 config의 alias allowlist와 교차 검증.
3. Credential은 기존 `cfg.LLMProviders[alias]` 경로에서 로드 — API나 tool parameter로 **절대 받지 않는다**.
4. Gateway 실행 감사 로그(`runs.json` 스냅샷의 `Run` 레코드)에 실제 사용된 alias/kind/model + override source 기록.

## 현황 (확인 완료된 사실)

- **해석 단일 경로**: `internal/config/llm_resolve.go:55-108`의 `ResolveLLMTier(cfg, tier)`가 `ResolvedLLMTier{Tier, Kind, AuthMode, OAuthProvider, BaseURL, APIKey, Model, ReasoningEffort, ThinkingBudget, ServiceTier, ProviderAlias}`를 돌려준다. 이 함수가 pool entry(`LLMProviderSettings`) + tier binding(`LLMTierBinding`)을 병합하는 유일한 경로다.
- **공용 task 타입은 없음**: gateway에는 `AgentTask` 같은 public 타입이 존재하지 않는다. `subagents_run`은 `internal/tool/tool_subagents.go:47-55`에서 **inline anonymous struct**로 task를 받아 `gateway.SpawnRequest` (`internal/gateway/types.go:53-68`)로 변환해 실행한다. `SpawnRequest`에는 이미 `Tier string` 필드가 있다.
- **Persistence**: gateway는 per-run 디렉토리가 아니라 `runs.json` 단일 스냅샷(`persistence.go:11-70`)에 `[]Run` 슬라이스를 유지한다. `Run` (`types.go:23-51`)은 이미 `Tier` 필드를 갖고 있고, override 관련 필드는 **여기를 확장**해야 한다.
- **파서**: `agents/*/AGENT.md` frontmatter는 현재 `tier` 필드만 받는다 (`gateway_agents_parse.go`).

## 제안 설계

### 데이터 모델

```go
// internal/gateway/types.go
//
// ProviderOverride는 "어떤 pool alias의 어떤 모델을 쓸지"만 기술한다.
// Credential/base_url/auth_mode는 기존 cfg.LLMProviders[Alias] 항목을
// 그대로 사용하므로 task payload에는 절대 포함되지 않는다.
type ProviderOverride struct {
    Alias string `json:"alias" yaml:"alias"` // cfg.LLMProviders의 키
    Model string `json:"model,omitempty" yaml:"model,omitempty"` // 생략 시 tier binding의 model 사용
}

// SpawnRequest는 이미 존재한다 (gateway/types.go:53-68).
// 이 PR은 ProviderOverride 필드를 추가한다.
type SpawnRequest struct {
    // 기존 필드들...
    Tier             string
    ProviderOverride *ProviderOverride // 신규
}

// Run은 이미 존재한다 (gateway/types.go:23-51).
// 이 PR은 resolved_* 감사 필드를 추가한다.
type Run struct {
    // 기존 필드들...
    Tier             string `json:"tier,omitempty"`
    ResolvedAlias    string `json:"resolved_alias,omitempty"`    // 신규
    ResolvedKind     string `json:"resolved_kind,omitempty"`     // 신규
    ResolvedModel    string `json:"resolved_model,omitempty"`    // 신규
    OverrideSource   string `json:"override_source,omitempty"`   // 신규: "task" | "agent" | "tier"
}
```

**새 공용 `AgentTask` 타입을 만들지 않는다.** `subagents_run`의 inline task struct에 `ProviderOverride` 필드를 JSON으로 받아 `SpawnRequest`에 그대로 실어 넘긴다. 타입을 새로 만들면 기존 call site(tool → runtime)를 공용 타입으로 끌어올리느라 리팩터링이 커지고, 호출 경로 대비 가치가 작다.

`Alias`만 교체하고 `Model`을 비워두면 기존 tier binding의 model이 그대로 쓰인다. 이는 "credential/endpoint만 prod↔dev로 바꾸는" 일반적인 유스케이스를 한 줄로 표현하기 위함.

### 해석 우선순위

```
1. task.ProviderOverride            — 명시적이면 최우선
2. AGENT.md frontmatter의 provider_override
3. task.Tier → cfg.LLMTiers[tier]   — 현재 동작
4. role default → cfg.LLMDefaultTier — 현재 동작
```

각 단계는 최종적으로 `ResolvedLLMTier`를 반환한다. 해석기는 `ResolveLLMTier`를 호출한 뒤 override가 있으면 결과 필드(`ProviderAlias`, `Model`, 그리고 alias에 따라 딸려오는 `Kind/AuthMode/BaseURL/APIKey/ServiceTier`)를 교체하는 방식으로 동작한다. **새 resolver를 만드는 대신 기존 resolver의 후처리**라는 점이 중요하다 — 단일 해석 경로 원칙을 유지한다.

### Allowlist

```yaml
# config/standalone.yaml
gateway_task_override:
  enabled: true                 # 기본 false; 기능 토글
  allowed_aliases: []           # 빈 배열이면 cfg.LLMProviders에 등록된 모든 alias 허용
  allowed_models: []            # 빈 배열이면 alias당 모델 제약 없음
```

- Alias allowlist는 **kind가 아니라 alias 단위**다. `anthropic_prod`와 `anthropic_dev`를 구분할 수 있어야 하기 때문.
- Allowlist 밖이면 Go 단에서 즉시 거절:

```
task override rejected: alias 'anthropic_dev' not in gateway_task_override.allowed_aliases
task override rejected: alias 'anthropic_prod' not registered in llm_providers
```

- `enabled=false`이면 override 필드가 들어와도 무시하지 않고 **loud error**로 거절한다 (silent fallback 금지).

### Credential 처리

- **API나 tool parameter로 key를 받지 않는다.** 이는 hard rule.
- 모든 credential/base_url/auth_mode는 `cfg.LLMProviders[alias]`에서만 읽는다. Override는 alias 포인터일 뿐이다.
- 해당 alias의 credential이 부재하면 (`ResolveLLMTier`가 이미 loud error를 던지므로) 명확한 메시지로 태스크 실패: `override failed: provider alias 'anthropic_dev' missing api_key`.

### `subagents_run` 툴 스키마 확장

```json
{
  "tasks": [
    {
      "prompt": "design migration plan",
      "tier": "heavy",
      "provider_override": {"alias": "anthropic_prod", "model": "claude-opus-4-6"}
    },
    {
      "prompt": "extract API endpoints",
      "tier": "light",
      "provider_override": {"alias": "gemini_flash"}
    }
  ]
}
```

### `AGENT.md` frontmatter 확장

```yaml
---
name: planner
tier: heavy
provider_override:
  alias: anthropic_prod
  model: claude-opus-4-6
---
```

기존 `tier:`만 쓰던 에이전트는 변경 없이 동작한다.

### 감사 로그

Gateway persistence는 `runs.json` 단일 스냅샷이다 (`internal/gateway/persistence.go:11-70`). 이 PR은 위에서 정의한 `Run` 필드 확장을 통해 스냅샷에 직접 기록한다:

```json
{
  "run_id": "r-abc",
  "tier": "heavy",
  "resolved_alias": "anthropic_prod",
  "resolved_kind": "anthropic",
  "resolved_model": "claude-opus-4-6",
  "override_source": "task"
}
```

- JSON `omitempty`라서 override 없이 실행된 기존 run은 추가 필드가 직렬화되지 않는다 → 기존 snapshot 스키마와 backward compatible.
- `resolved_kind`는 alias 뒤에 어떤 실제 provider 구현이 있었는지 운영 가시성용으로 중복 기록한다 (alias를 사후에 리네이밍해도 과거 스냅샷에서 kind를 읽을 수 있음).
- `override_source`는 어느 레이어에서 override가 결정됐는지: `task`(subagents_run 입력), `agent`(frontmatter), `tier`(override 없이 tier binding 그대로).

## 수정 대상

### Backend
- `internal/gateway/types.go` — `ProviderOverride` 타입 신규, `SpawnRequest.ProviderOverride` 필드, `Run`에 `ResolvedAlias/ResolvedKind/ResolvedModel/OverrideSource` 필드
- `internal/gateway/runtime_run_execute.go` — override resolution 진입점 (SpawnRequest의 override를 읽어 resolve helper 호출)
- `internal/gateway/resolve_override.go` — 신규, `ResolveLLMTier` 결과를 후처리하는 얇은 helper + allowlist 검증. 별도 resolver를 만들지 않고 기존 resolver 결과를 교체.
- `internal/gateway/resolve_override_test.go` — 표 기반 테스트 (4단계 우선순위, allowlist 거절, `enabled=false` loud error, 같은 kind의 다른 alias가 각기 다른 credential로 귀결되는지)
- `internal/gateway/persistence_test.go` — `Run`의 신규 필드가 round-trip 되는지
- `internal/config/config.go` + `config_input_fields.go` — 신규 필드 `GatewayTaskOverride{Enabled, AllowedAliases, AllowedModels}`
- `internal/config/config_test.go` — 파싱 기본값/오버라이드
- `internal/tool/tool_subagents.go` — task inline struct에 `ProviderOverride` 필드 추가, tool 스키마(`Parameters` JSON) 확장
- `internal/tool/tool_subagents_test.go` — 스키마 검증, alias 미지정 시 reject
- `internal/tool/tool_subagents_orchestrate.go` — orchestrate 입력에도 동일 필드 관통
- `internal/tarsserver/gateway_agents_parse.go` — frontmatter의 `provider_override: {alias, model}` 파싱 → agent metadata → `SpawnRequest.ProviderOverride` (agent 소스로 표시)

### Frontend (선택, 이 PR에는 최소)
- 현재 콘솔에는 gateway run 전용 화면이 없다 (router.ts 확인). 이 PR에서는 UI를 추가하지 않고, `/v1/gateway/runs/{id}` API 응답에 신규 필드가 포함되도록만 확장한다. 실제 run 뷰는 #04 선행 PR(event surface + run view)에서 추가한다.

## 테스트 계획

### Unit
- `resolve_override_test.go`: 4단계 우선순위 × allowlist 거절 케이스
- Alias 미등록 / credential 부재 시 명확한 에러 메시지 검증
- `enabled=false`에서 override 제공 시 silent ignore가 아니라 loud error임을 표기
- Allowlist 빈 배열(= cfg.LLMProviders 전체 허용)과 명시 배열 동작 차이

### Integration
- 실제 gateway run에서 override가 persistence에 기록되는지
- `subagents_run` 툴 end-to-end로 tier × alias 조합
- `cfg.LLMProviders`에 두 개의 같은 kind alias(`anthropic_prod`, `anthropic_dev`)를 두고 각각 지정됐을 때 올바른 credential이 쓰이는지 확인

## Acceptance Criteria

- [ ] `gateway.ProviderOverride` 타입 신규, `SpawnRequest.ProviderOverride`와 `Run`의 resolved_* 필드 추가 (공용 `AgentTask` 타입 생성하지 않음)
- [ ] 4단계 우선순위로 해석, 표 기반 테스트 통과
- [ ] `gateway_task_override.{enabled, allowed_aliases, allowed_models}` config 필드 동작
- [ ] Credential은 `cfg.LLMProviders[alias]`에서만 로드, API/tool 파라미터로 받지 않음
- [ ] Alias가 `cfg.LLMProviders`에 없거나 allowlist 밖이면 loud error
- [ ] `enabled=false`인데 task에 override가 들어오면 loud error (silent ignore 금지)
- [ ] `runs.json` 스냅샷의 `Run` 레코드에 `resolved_alias`, `resolved_kind`, `resolved_model`, `override_source` 기록 (round-trip 테스트)
- [ ] 같은 kind의 서로 다른 alias(`anthropic_prod`/`anthropic_dev`)가 각각 다른 credential/base_url을 타는 integration test
- [ ] `subagents_run` 툴 스키마 확장 (alias + optional model)
- [ ] `subagents_orchestrate` 입력에도 동일 필드 관통
- [ ] `AGENT.md` frontmatter에서 `provider_override: {alias, model}` 파싱
- [ ] 기존 tier-only 에이전트는 동일 동작 (backward compat)
- [ ] `make test`, `make vet`, `make fmt` 통과

## Identity Check

- **단일 Go 바이너리**: 새 런타임 의존성 없음 ✓
- **File-first**: 감사 로그는 기존 파일 포맷 확장 ✓
- **Scope isolation**: Gateway는 user surface — pulse/reflection 경로 무변경 ✓
- **정책은 config, 메커니즘은 Go**: allowlist가 config, resolution이 Go ✓
- **Credential 안전성**: API 파라미터로 key를 받지 않음이 hard rule로 명시됨 ✓
