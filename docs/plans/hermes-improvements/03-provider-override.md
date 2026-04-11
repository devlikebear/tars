# 03 — Per-task Provider/Credential Override

**Branch**: `feat/hermes-provider-override`
**Sprint**: S2
**Area**: `area/gateway`, `area/llm`
**Depends on**: —
**Blocks**: #04 (MoA consensus)

## 배경

Hermes Agent는 서브에이전트를 스폰할 때 provider/모델/credential을 덮어쓸 수 있다. 덕분에 "무거운 설계 질의는 Anthropic opus, 파싱은 gemini flash, 테스트 생성은 openai mini"처럼 태스크 성격별 최적 조합이 가능하다.

TARS는 3-tier bundle(heavy/standard/light)로 tier 이동은 되지만, tier 안에서 provider까지 태스크 단위로 바꾸는 건 불가능하다. 이건 MoA(#04)의 전제조건이기도 하다.

## 목적

1. `AgentTask`에 optional `provider_override` 필드 추가.
2. 안전을 위해 config의 allowlist와 교차 검증.
3. Credential은 기존 env/config에서 로드 — API나 tool parameter로 **절대 받지 않는다**.
4. Gateway 실행 감사 로그에 실제 사용된 provider/model 기록.

## 현황 조사

(구현 착수 시 확인할 것)

- `internal/llm/` 아래 provider abstraction 진입점 (`Client` interface와 factory 경로)
- `internal/gateway/executor.go` / `runtime_run_execute.go`의 tier 해석 경로
- `subagents_run` 툴 (`internal/tool/tool_subagents.go`)의 현재 스키마
- `agents/*/AGENT.md` frontmatter 파서 (`gateway_agents_parse.go`)

## 제안 설계

### 데이터 모델

```go
// internal/gateway/types.go
type ProviderOverride struct {
    Provider string `json:"provider" yaml:"provider"` // "anthropic" | "openai" | "gemini"
    Model    string `json:"model,omitempty" yaml:"model,omitempty"` // optional; provider 기본 모델 사용
}

type AgentTask struct {
    // 기존 필드들...
    Tier             string            `json:"tier,omitempty"`
    ProviderOverride *ProviderOverride `json:"provider_override,omitempty"`
}
```

### 해석 우선순위

```
1. ProviderOverride (task 수준) — 명시적이면 최우선
2. AGENT.md frontmatter의 provider/model
3. Tier bundle의 기본 provider/model (현재 동작)
4. 전역 llm_provider (현재 fallback)
```

단, **모든 단계에서 allowlist 교차 검증**.

### Allowlist

```yaml
# config/standalone.yaml
gateway_task_override_allowlist:
  providers: [anthropic, openai, gemini]
  models: []  # 빈 배열이면 provider 기본 모델 제한 없음
```

Allowlist 밖의 provider/model이 오면 Go 단에서 즉시 거절:

```
task override rejected: provider 'cohere' not in gateway_task_override_allowlist.providers
```

### Credential 처리

- **API나 tool parameter로 key를 받지 않는다.** 이는 hard rule.
- Provider별 credential은 기존 경로(`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY` env) 그대로.
- 만약 override로 지정된 provider의 credential이 부재하면 "missing credential for provider 'openai'" 에러로 태스크 실패.

### `subagents_run` 툴 스키마 확장

```json
{
  "tasks": [
    {
      "prompt": "design migration plan",
      "tier": "heavy",
      "provider_override": {"provider": "anthropic", "model": "claude-opus-4-6"}
    },
    {
      "prompt": "extract API endpoints",
      "tier": "light",
      "provider_override": {"provider": "gemini"}
    }
  ]
}
```

### `AGENT.md` frontmatter 확장

```yaml
---
name: planner
tier: heavy
provider: anthropic
model: claude-opus-4-6
---
```

### 감사 로그

Gateway run 레코드(`workspace/_shared/gateway/<run_id>/run.json`)에 다음을 기록:

```json
{
  "task_id": "t-01",
  "resolved_provider": "anthropic",
  "resolved_model": "claude-opus-4-6",
  "override_source": "task" | "agent" | "tier" | "global"
}
```

## 수정 대상

### Backend
- `internal/gateway/types.go` — `AgentTask.ProviderOverride` 필드
- `internal/gateway/runtime_run_execute.go` — override resolution 로직 진입
- `internal/gateway/resolve_provider.go` — 신규, 4단계 우선순위 해석
- `internal/gateway/resolve_provider_test.go` — 표 기반 테스트
- `internal/llm/factory.go` (현재 factory 경로) — provider 이름으로 client 생성하는 helper
- `internal/llm/factory_test.go` — allowlist 거절 케이스
- `internal/tool/tool_subagents.go` — 툴 스키마에 `provider_override` 추가
- `internal/tool/tool_subagents_test.go` — 스키마 검증
- `internal/tarsserver/gateway_agents_parse.go` — frontmatter의 `provider`, `model` 필드 파싱
- `internal/gateway/persistence.go` — run.json에 `resolved_provider` 등 기록
- `internal/config/config.go` + `config_input_fields.go` — `gateway_task_override_allowlist`

### Frontend (선택, 이 PR에는 최소)
- Gateway run 상세 뷰에 `resolved_provider` 뱃지만 추가
- 깊은 UI 작업은 #04에서 MoA와 함께

## 테스트 계획

### Unit
- `resolve_provider_test.go`: 4단계 우선순위 × allowlist 거절 케이스
- Credential 부재 시 명확한 에러
- Allowlist 빈 배열(= 전체 허용)과 명시 배열 동작 차이

### Integration
- 실제 gateway run에서 override가 persistence에 기록되는지
- `subagents_run` 툴 end-to-end로 tier/provider 조합

## Acceptance Criteria

- [ ] `AgentTask.ProviderOverride` 필드 추가, optional
- [ ] 4단계 우선순위로 해석, 표 기반 테스트 통과
- [ ] `gateway_task_override_allowlist` config 필드 동작
- [ ] Credential은 env/config에서만 로드, API 파라미터로 받지 않음
- [ ] Gateway run 감사 로그에 `resolved_provider`, `override_source` 기록
- [ ] `subagents_run` 툴 스키마 확장
- [ ] `AGENT.md` frontmatter에서 `provider`, `model` 파싱
- [ ] 기존 tier-only 에이전트는 동일 동작 (backward compat)
- [ ] `make test`, `make vet`, `make fmt` 통과

## Identity Check

- **단일 Go 바이너리**: 새 런타임 의존성 없음 ✓
- **File-first**: 감사 로그는 기존 파일 포맷 확장 ✓
- **Scope isolation**: Gateway는 user surface — pulse/reflection 경로 무변경 ✓
- **정책은 config, 메커니즘은 Go**: allowlist가 config, resolution이 Go ✓
- **Credential 안전성**: API 파라미터로 key를 받지 않음이 hard rule로 명시됨 ✓
