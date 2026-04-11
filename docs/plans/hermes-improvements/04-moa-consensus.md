# 04 — MoA Consensus Executor Mode

**Branch**: `feat/hermes-moa-consensus`
**Sprint**: S2
**Area**: `area/gateway`
**Depends on**: #03 (provider override)

## 배경

Hermes Agent의 **가장 distinctive한 기능**은 MoA — 여러 frontier 모델을 병렬로 돌리고 한 모델이 aggregator 역할을 하여 합의(또는 투표)로 최종 답을 만드는 방식이다. 고비용이지만 critical path 결정에서 정확도·robustness 이득이 유의미하다.

TARS는 gateway에 이미 멀티스레드 서브에이전트 실행기가 있지만, "여러 모델이 같은 질문에 병렬 답변 → aggregator가 합성"하는 consensus 모드는 없다.

## 목적

1. `subagents_run` / gateway executor에 `mode=consensus` 추가 (opt-in).
2. 하나의 태스크를 N개 provider·model 조합으로 병렬 실행.
3. Light-tier LLM을 aggregator로 사용해 결과를 합성.
4. Token·시간·팬아웃 예산을 config로 엄격히 제한.
5. 콘솔 SSE로 병렬 스트림을 시각화.

## 현황 조사

(구현 착수 시 확인할 것)

- `internal/gateway/executor.go` / `runtime_run_execute.go`의 현재 병렬 실행 경로 (max 4 subagent)
- `internal/gateway/runtime_channels.go`의 SSE 이벤트 포맷
- `frontend/console/src/components/` 중 gateway run 뷰가 어디인지
- 현재 `llm.Client`가 스트리밍 응답을 어떤 인터페이스로 내보내는지

## 제안 설계

### 실행 모델

```
[Task with mode=consensus]
        │
        ├── [Provider A / Model A]  ──┐
        ├── [Provider B / Model B]  ──┼──► [Aggregator (light tier)] ──► Final
        └── [Provider C / Model C]  ──┘
```

### 데이터 모델

```go
// internal/gateway/types.go
type ConsensusSpec struct {
    Variants  []ProviderOverride `json:"variants"`   // 2..N개의 참여 모델
    Aggregator *ProviderOverride `json:"aggregator,omitempty"` // 기본: light tier
    Strategy  string             `json:"strategy,omitempty"`   // "synthesize" | "vote" (기본 synthesize)
}

type AgentTask struct {
    // 기존 필드...
    Mode      string         `json:"mode,omitempty"`      // "single" | "consensus" (기본 single)
    Consensus *ConsensusSpec `json:"consensus,omitempty"`
}
```

### Aggregator 전략

- **synthesize**: aggregator가 모든 variant 응답을 받아 "가장 정확하고 완결한 단일 답"을 작성. 기본.
- **vote**: aggregator가 variant들을 1~5점으로 평가하고 최고점 variant를 그대로 선택. 합성 없음, 응답을 바꾸지 않음.

### Aggregator 프롬프트 (synthesize)

```
You are an aggregator. You will receive {N} candidate answers to the same
question from different models. Your job is to produce the single best answer
by combining correct information, resolving contradictions, and discarding
errors. Preserve all specific identifiers, code, and numbers that appear in
any candidate. Do not add facts that no candidate supports.

Question:
<original prompt>

Candidate answers:
1. [model=<name>] <answer 1>
2. [model=<name>] <answer 2>
...
```

### 예산 제한 (hard stops, config)

| 필드 | 기본값 | 의미 |
|---|---|---|
| `gateway_consensus_enabled` | `false` | 전체 기능 토글. 기본 꺼짐 |
| `gateway_consensus_max_fanout` | `4` | 한 태스크당 variant 최대 수 |
| `gateway_consensus_budget_tokens` | `20000` | 한 consensus 실행의 누적 토큰 상한 |
| `gateway_consensus_timeout_seconds` | `120` | 한 consensus 실행 타임아웃 |
| `gateway_consensus_allowed_providers` | `[anthropic, openai, gemini]` | variant에 허용되는 provider |

초과 시 `consensus_budget_exceeded` 에러로 태스크 실패. Go가 결정론적으로 차단.

### 실행 알고리즘

```go
func RunConsensus(ctx context.Context, task AgentTask) (Result, error) {
    if !cfg.ConsensusEnabled {
        return Result{}, ErrConsensusDisabled
    }
    spec := task.Consensus
    if err := validateSpec(spec, cfg); err != nil { return Result{}, err }

    ctx, cancel := context.WithTimeout(ctx, cfg.ConsensusTimeout)
    defer cancel()

    results := make([]Variant, len(spec.Variants))
    var wg sync.WaitGroup
    for i, v := range spec.Variants {
        wg.Add(1)
        go func(i int, v ProviderOverride) {
            defer wg.Done()
            results[i] = runVariant(ctx, task.Prompt, v)
        }(i, v)
    }
    wg.Wait()

    if allFailed(results) { return Result{}, ErrAllVariantsFailed }

    agg := spec.Aggregator
    if agg == nil { agg = defaultLightTier(cfg) }
    return aggregate(ctx, task.Prompt, results, agg, spec.Strategy)
}
```

### SSE 이벤트

기존 `/v1/events/stream`에 consensus 전용 이벤트 추가:

```json
{ "type": "consensus_variant_started",  "run_id": "...", "variant_idx": 0, "provider": "anthropic" }
{ "type": "consensus_variant_stream",   "run_id": "...", "variant_idx": 0, "delta": "..." }
{ "type": "consensus_variant_finished", "run_id": "...", "variant_idx": 0, "tokens": 512 }
{ "type": "consensus_aggregating",      "run_id": "...", "strategy": "synthesize" }
{ "type": "consensus_finished",         "run_id": "...", "final_tokens": 700, "cost_usd_estimate": 0.12 }
```

### 실패 모드

- Variant N-1개 실패 + 1개 성공: 성공한 걸로 aggregator 호출 (degraded)
- 전부 실패: `ErrAllVariantsFailed` → task fail
- Aggregator 실패: variant 중 첫 성공 응답을 그대로 반환 (`strategy=passthrough` 로 로깅)
- Budget 초과: 즉시 모든 variant 취소 + 태스크 실패

### 콘솔 UI

- Gateway run 뷰에 "Consensus run" 배지
- Variant별 스트리밍 영역을 가로로 나란히 표시
- Aggregator 단계로 진입할 때 variant 영역이 회색으로 고정되고 최종 답이 아래에 스트리밍

## 수정 대상

### Backend
- `internal/gateway/types.go` — `ConsensusSpec`, `AgentTask.Mode` 필드
- `internal/gateway/consensus.go` — 신규, `RunConsensus` 진입점
- `internal/gateway/consensus_aggregator.go` — 신규, synthesize/vote 전략
- `internal/gateway/consensus_test.go` — 신규, 병렬 실행 + 실패 모드 표 기반
- `internal/gateway/runtime_run_execute.go` — `Mode=consensus` 분기
- `internal/gateway/runtime_channels.go` — SSE 이벤트 발행
- `internal/tool/tool_subagents.go` — 스키마에 `mode`, `consensus` 필드
- `internal/tool/tool_subagents_test.go`
- `internal/config/config.go` + `config_input_fields.go` — 5개 consensus 필드

### Frontend
- `frontend/console/src/components/` 아래 gateway run 뷰 컴포넌트 (파일명 확인 필요) — variant 병렬 스트림 UI
- `frontend/console/src/lib/api.ts` — 신규 SSE 이벤트 타입

## 테스트 계획

### Unit
- `consensus_test.go`: 2/3/4 variant, 전부 성공/일부 실패/전부 실패, strategy synthesize/vote
- Budget 초과 시 즉시 취소 검증
- Timeout 초과 시 context.DeadlineExceeded

### Integration
- `gateway_api_test.go`에 consensus 실행 end-to-end
- SSE 이벤트 순서 검증

### Manual / Dev
- `make dev-console`로 띄우고 `subagents_run mode=consensus`로 실제 호출
- 비용이 크므로 초기에는 `gemini-flash` 같은 저가 모델로만 테스트

## Acceptance Criteria

- [ ] `gateway_consensus_enabled=false` (기본)에서 코드 경로가 완전히 dormant
- [ ] `mode=consensus`로 2~4 variant 병렬 실행, light-tier aggregator로 합성
- [ ] `synthesize` / `vote` 두 전략 모두 동작
- [ ] Budget·timeout·fanout 제한이 Go 단에서 hard stop
- [ ] SSE 이벤트로 variant 스트림 + aggregator 단계 구분 가능
- [ ] 콘솔에서 consensus run이 시각적으로 구분
- [ ] `make test`, `make vet`, `make fmt` 통과
- [ ] README 비교표 업데이트 (TARS 쪽 "MoA" 칸 갱신)

## Identity Check

- **단일 Go 바이너리**: 새 런타임 의존성 없음 ✓
- **File-first**: 감사 로그는 기존 gateway run.json 확장 ✓
- **Scope isolation**: User surface 한정 ✓
- **정책은 config, 메커니즘은 Go**: 예산·fanout·allowlist가 전부 config, 실행이 Go ✓
- **LLM은 판단·합성까지, 액션은 Go**: aggregator LLM도 텍스트 합성만. Variant 실행·취소·budget은 Go가 결정론적으로 제어 ✓
- **Opt-in**: `gateway_consensus_enabled=false` 기본, 기존 사용자 동작 무변경 ✓
- **Memory 영향 없음** ✓

## 비용·위험 경고

- Consensus 1회 = provider N개 × 입력 토큰 + aggregator 입력 토큰. 쉽게 10배 이상 비쌀 수 있음.
- 기본 비활성화, budget config, 콘솔에 예상 비용 배지가 **반드시 함께 제공**되어야 merge.
- 초기에는 `subagents_run` 레벨에서만 사용 가능하게 열고, 일반 chat 응답에 자동 적용되는 경로는 열지 않음.
