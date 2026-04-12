# 04 — MoA Consensus Executor Mode

**Branches**:
  - `feat/hermes-gateway-event-surface` (선행 PR)
  - `feat/hermes-moa-consensus` (executor PR)

**Sprint**: S2
**Area**: `area/gateway`
**Depends on**: #03 (provider override)

## 배경

Hermes Agent의 **가장 distinctive한 기능**은 MoA — 여러 frontier 모델을 병렬로 돌리고 한 모델이 aggregator 역할을 하여 합의(또는 투표)로 최종 답을 만드는 방식이다. 고비용이지만 critical path 결정에서 정확도·robustness 이득이 유의미하다.

TARS는 gateway에 이미 서브에이전트 fan-out(`gateway_subagents_max_threads`) 이 있지만, (a) 공유 execution semaphore가 없고 (`internal/tool/tool_subagents.go:65-70`의 제한은 **입력 fan-out 상한**일 뿐 실제 concurrency pool이 아니다), (b) 콘솔에 gateway run 전용 화면이 없어 variant 스트림을 시각화할 곳이 없으며 (`frontend/console/src/lib/router.ts`에 gateway/run view 미존재), (c) `/v1/events/stream`은 human-facing `notificationEvent` broker(`internal/tarsserver/notify.go:21-92`)라 variant 단위의 구조화 이벤트 stream으로 쓸 수 없다.

따라서 consensus executor를 단일 PR로 넣으면 "돌지만 볼 방법이 없고 pool 제어도 없는 상태"로 도착한다. 이 문서는 #04를 **두 PR**로 쪼갠다.

## PR 분할

### PR-A: Gateway run event surface + run view groundwork

콘솔에 "gateway run 상세"라는 표면을 처음 만드는 PR. Consensus가 없어도 독립적으로 가치가 있다 (일반 subagents_run 결과도 이 화면에서 볼 수 있게 된다).

**범위**:

1. **Gateway 전용 SSE endpoint** — 기존 `/v1/events/stream`(notification broker)과 **섞지 않는다**. 둘은 목적이 다르다 (notification은 severity/title/open_path 중심, gateway event는 run_id/variant_idx/tokens 중심이고 드롭 전략도 다르다). 신규 endpoint:
   - `GET /v1/gateway/runs/{id}/events` — SSE stream. 특정 run에 대한 구조화 이벤트만 방출.
   - 필요 시 `GET /v1/gateway/events?root_run_id=...`로 tree 단위도 구독 가능.
2. **Gateway event broker** — `internal/gateway/runtime_channels.go` 또는 신규 `internal/gateway/events.go`에 per-run event broker. 구독자가 없어도 publish는 non-blocking.
3. **Runtime execution semaphore** — gateway.Runtime에 최초로 실제 실행 풀 세마포어를 도입한다. 이 PR 시점에는 "일반 subagent pool" 하나만 있고, 기본 크기는 현재 `gateway_subagents_max_threads`와 동일 의미로 연결. 세마포어 추상을 만들어두어야 PR-B에서 "consensus pool"을 그 옆에 추가할 수 있다.
4. **Run event 타입** — 최소한:
   ```json
   { "type": "run_accepted", "run_id": "...", "tier": "heavy", "resolved_alias": "anthropic_prod" }
   { "type": "run_started",  "run_id": "...", "agent": "explorer" }
   { "type": "run_delta",    "run_id": "...", "delta": "..." }
   { "type": "run_finished", "run_id": "...", "status": "completed", "tokens_in": 800, "tokens_out": 512 }
   { "type": "run_failed",   "run_id": "...", "error": "..." }
   ```
   `run_delta`는 현재 llm.Client가 streaming을 노출하지 않는 경우 생략 가능 — 이 경우 `run_started` → `run_finished`만 방출하고 스트리밍은 PR-B 또는 후속에서 확장.
5. **콘솔 라우트 + 뷰** — `router.ts`에 `gateway` / `gateway/runs/:id` 추가, 최소 run detail 페이지(agent, tier, prompt, response, resolved_* 뱃지, status). Consensus 전용 UI 없음 — 그건 PR-B가 덧붙인다.

**왜 굳이 따로?**

이 인프라가 없으면 PR-B(consensus)는 "이벤트가 어디로 흘러가야 하는지", "일반 subagent pool과 어떻게 격리해야 하는지", "어떤 화면에 표시되는지" 세 가지를 동시에 해결해야 한다. 각 결정이 상호의존이어서 PR이 거대해지고 리뷰가 어려워진다. PR-A는 이 셋을 **consensus 없이도 가치 있는 최소 표면**으로 독립시킨다.

### PR-B: Consensus executor on top

PR-A가 제공한 이벤트 표면과 실행 세마포어 위에 "같은 질문을 여러 alias로 병렬 실행 → aggregator로 합성"하는 레이어를 얹는다.

**범위**:

1. `SpawnRequest`에 `ConsensusSpec` 필드 추가 (아래 데이터 모델).
2. Runtime에 `consensusPool` 세마포어 추가 (PR-A에서 만든 세마포어 추상을 재사용). 기본 크기 `gateway_consensus_max_fanout × gateway_consensus_concurrent_runs = 3 × 1 = 3`.
3. Aggregator는 `synthesize`만 ship. `vote`는 `ErrStrategyNotImplemented`.
4. `consensus_planned` / `consensus_variant_*` / `consensus_aggregating` / `consensus_finished` 이벤트를 PR-A의 gateway event broker에 방출.
5. 콘솔 gateway run view에 "consensus" 배지 + variant 스트림 병렬 표시 + USD 예상 비용 배지.
6. Budget enforcement (token + USD + timeout + fanout + alias allowlist)를 Go에서 hard stop.

## 목적 (PR-B 기준)

1. `subagents_run` 툴에 `mode=consensus` 추가 (opt-in).
2. 하나의 태스크를 N개 provider alias/model 조합으로 병렬 실행 (`consensusPool` 위에서).
3. Light-tier LLM을 aggregator로 사용해 결과를 합성 (`synthesize`만).
4. Token·USD·시간·팬아웃 예산을 config로 엄격히 제한.
5. PR-A의 gateway SSE로 병렬 스트림과 **USD 예상 비용 배지**를 시각화.

## 데이터 모델 (PR-B)

`#03`에서 추가되는 alias 기반 `ProviderOverride`를 그대로 재사용한다.

```go
// internal/gateway/types.go
type ConsensusSpec struct {
    Variants   []ProviderOverride `json:"variants"`             // 2..N개의 참여 alias
    Aggregator *ProviderOverride  `json:"aggregator,omitempty"` // 기본: light tier (cfg.LLMTiers["light"])
    Strategy   string             `json:"strategy,omitempty"`   // 이 PR에서는 "synthesize"만 유효
}

// SpawnRequest는 #03에서 ProviderOverride가 이미 추가된 상태.
// 여기서는 ConsensusSpec만 더한다.
type SpawnRequest struct {
    // 기존 필드 + #03의 ProviderOverride
    Consensus *ConsensusSpec
}

// Run의 감사 필드 확장 — runs.json 스냅샷에 기록됨
type Run struct {
    // #03에서 추가된 Resolved* 필드 + 아래
    ConsensusMode      string         `json:"consensus_mode,omitempty"`       // "synthesize"
    ConsensusVariants  []ConsensusVariantRecord `json:"consensus_variants,omitempty"`
    ConsensusCostUSD   float64        `json:"consensus_cost_usd,omitempty"`   // actual
    ConsensusBudgetUSD float64        `json:"consensus_budget_usd,omitempty"` // estimated before run
}

type ConsensusVariantRecord struct {
    Alias      string `json:"alias"`
    Kind       string `json:"kind"`
    Model      string `json:"model"`
    Status     string `json:"status"` // completed | failed | canceled
    TokensIn   int    `json:"tokens_in,omitempty"`
    TokensOut  int    `json:"tokens_out,omitempty"`
    DurationMS int    `json:"duration_ms,omitempty"`
    Error      string `json:"error,omitempty"`
}
```

Consensus가 없는 run은 이 필드가 전부 omitempty로 직렬화되지 않는다 → backward compat.

## Aggregator 전략

- **synthesize** *(이 PR의 유일한 ship 전략)*: aggregator가 모든 variant 응답을 받아 "가장 정확하고 완결한 단일 답"을 작성. 기본값이자 현재 유일하게 유효한 값.
- **vote** *(후속 PR)*: 이 PR에서는 spec만 받아들이고 해석기에서 `ErrStrategyNotImplemented`를 반환.

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
1. [alias=<name> model=<name>] <answer 1>
2. [alias=<name> model=<name>] <answer 2>
...
```

## 예산 제한 (hard stops, config)

| 필드 | 기본값 | 의미 |
|---|---|---|
| `gateway_consensus_enabled` | `false` | 전체 기능 토글. 기본 꺼짐 |
| `gateway_consensus_max_fanout` | `3` | 한 태스크당 variant 최대 수 |
| `gateway_consensus_budget_tokens` | `20000` | 한 consensus 실행의 누적 입출력 토큰 상한 |
| `gateway_consensus_budget_usd` | `0.50` | 한 consensus 실행의 USD 상한 추정치. 초과 예상 시 실행 전 거절 |
| `gateway_consensus_timeout_seconds` | `120` | 한 consensus 실행 타임아웃 |
| `gateway_consensus_allowed_aliases` | `[]` *(빈 배열 = `#03`의 override allowlist에 위임)* | variant로 허용되는 provider alias |
| `gateway_consensus_concurrent_runs` | `1` | 전체 서버에서 동시에 진행 가능한 consensus run 수 |

초과 시 `consensus_budget_exceeded` / `consensus_usd_budget_exceeded` / `consensus_alias_not_allowed` 에러로 태스크 실패. Go가 결정론적으로 차단.

## 기존 subagent pool과의 격리

PR-A가 도입하는 실행 세마포어 추상 위에, PR-B는 **별도 `consensusPool`**을 추가한다:

- Consensus variant는 `consensusPool`을 점유한다. 일반 subagent pool을 점유하지 않는다.
- `consensusPool` 크기 = `gateway_consensus_max_fanout × gateway_consensus_concurrent_runs` (기본 `3 × 1 = 3`).
- Aggregator 호출은 consensus pool을 빠져나온 뒤 기존 light-tier 경로로 직렬 실행.
- 이 분리를 integration test로 잠근다: "consensus 진행 중에 `subagents_run`이 일반 pool에서 정상 전진하는가".

## USD 추정 산식

`internal/usage/`의 provider × model 단가표를 재사용한다. 각 variant의 `(input_tokens × input_price + output_tokens × output_price)`를 합산하고 aggregator 호출분을 더한다. 단가표에 없는 alias는 **안전 쪽으로 과대 추정**(해당 kind의 가장 비싼 등재 모델 가격)하거나 실행 거절.

## 실행 알고리즘

```go
func RunConsensus(ctx context.Context, req SpawnRequest) (Run, error) {
    if !cfg.ConsensusEnabled {
        return Run{}, ErrConsensusDisabled
    }
    spec := req.Consensus
    if err := validateSpec(spec, cfg); err != nil { return Run{}, err }

    // 실행 전 USD 추정 → budget 체크
    estUSD := estimateConsensusCost(spec, req.Prompt, cfg)
    if estUSD > cfg.ConsensusBudgetUSD {
        return Run{}, fmt.Errorf("consensus_usd_budget_exceeded: %.2f > %.2f", estUSD, cfg.ConsensusBudgetUSD)
    }
    publishPlanned(req, spec, estUSD) // consensus_planned 이벤트

    ctx, cancel := context.WithTimeout(ctx, cfg.ConsensusTimeout)
    defer cancel()

    // consensusPool 세마포어 획득
    results := make([]Variant, len(spec.Variants))
    var wg sync.WaitGroup
    for i, v := range spec.Variants {
        consensusPool.Acquire(ctx)
        wg.Add(1)
        go func(i int, v ProviderOverride) {
            defer wg.Done()
            defer consensusPool.Release()
            results[i] = runVariant(ctx, req.Prompt, v)
        }(i, v)
    }
    wg.Wait()

    if allFailed(results) { return Run{}, ErrAllVariantsFailed }

    return aggregate(ctx, req.Prompt, results, spec.Aggregator, spec.Strategy)
}
```

## SSE 이벤트 (PR-A broker에 방출)

```json
{ "type": "consensus_planned",          "run_id": "...", "variant_count": 3, "cost_usd_estimate": 0.12, "token_budget": 20000 }
{ "type": "consensus_variant_started",  "run_id": "...", "variant_idx": 0, "alias": "anthropic_prod", "kind": "anthropic", "model": "claude-opus-4-6" }
{ "type": "consensus_variant_stream",   "run_id": "...", "variant_idx": 0, "delta": "..." }
{ "type": "consensus_variant_finished", "run_id": "...", "variant_idx": 0, "tokens_in": 800, "tokens_out": 512 }
{ "type": "consensus_aggregating",      "run_id": "...", "strategy": "synthesize" }
{ "type": "consensus_finished",         "run_id": "...", "final_tokens": 700, "cost_usd_actual": 0.14 }
```

`consensus_planned`는 variant 실행을 **시작하기 전에** 발행되며, 콘솔은 이 이벤트를 받는 순간 예상 비용 배지를 노란색으로 즉시 표시한다(사용자가 실행 중간에 중단 결정을 내릴 수 있도록).

## 실패 모드

- Variant N-1개 실패 + 1개 성공: 성공한 걸로 aggregator 호출 (degraded)
- 전부 실패: `ErrAllVariantsFailed` → task fail
- Aggregator 실패: variant 중 첫 성공 응답을 그대로 반환 (`strategy=passthrough` 로 로깅)
- Token budget 초과: 즉시 모든 variant 취소 + 태스크 실패
- USD budget 초과: **실행 전** 거절 (한 variant도 시작하지 않음)

## 콘솔 UI (PR-B가 PR-A의 run view에 덧붙임)

- Gateway run 뷰 상단에 "Consensus run" 배지
- **USD 예상 비용 배지** — `consensus_planned` 이벤트의 `cost_usd_estimate`를 받는 즉시 노란색 배지로 표시. Variant 실행 완료 후 `consensus_finished`의 `cost_usd_actual`로 업데이트
- Variant별 스트리밍 영역을 가로로 나란히 표시 (alias + kind + model 라벨)
- Aggregator 단계로 진입할 때 variant 영역이 회색으로 고정되고 최종 답이 아래에 스트리밍

## 수정 대상

### PR-A (Gateway event surface + run view)

**Backend**
- `internal/gateway/events.go` — 신규, per-run event broker
- `internal/gateway/semaphore.go` — 신규, 실행 세마포어 추상
- `internal/gateway/runtime_run_execute.go` — 세마포어 점유 + 이벤트 발행 (run_started / run_finished / run_failed 최소)
- `internal/gateway/runtime.go` — 런타임에 semaphore + broker 필드 주입
- `internal/tarsserver/handler_gateway_events.go` — 신규, `/v1/gateway/runs/{id}/events` SSE 핸들러
- `internal/tarsserver/handler_gateway_runs.go` — 기존 run detail API 응답에 `#03`의 resolved_* 필드 노출

**Frontend**
- `frontend/console/src/lib/router.ts` — `gateway` / `gateway/runs/:id` 라우트 추가
- `frontend/console/src/components/GatewayRunView.svelte` — 신규, 최소 run 상세 뷰
- `frontend/console/src/lib/api.ts` — gateway event 타입 + SSE 구독 helper

### PR-B (Consensus executor)

**Backend**
- `internal/gateway/types.go` — `ConsensusSpec`, `ConsensusVariantRecord`, `Run`의 consensus_* 필드
- `internal/gateway/consensus.go` — 신규, `RunConsensus` 진입점 + consensusPool 사용
- `internal/gateway/consensus_aggregator.go` — 신규, synthesize 전략 (vote는 `ErrStrategyNotImplemented`)
- `internal/gateway/consensus_cost.go` — 신규, USD 추정 (기존 usage 단가표 재사용)
- `internal/gateway/consensus_test.go` — 신규, 병렬 실행 + 실패 모드 + budget 초과 + pool 격리 표 기반
- `internal/gateway/runtime_run_execute.go` — `req.Consensus != nil` 분기
- `internal/tool/tool_subagents.go` — 스키마에 `mode`, `consensus` 필드 (alias 기반 variant)
- `internal/tool/tool_subagents_test.go`
- `internal/config/config.go` + `config_input_fields.go` — 7개 consensus 필드

**Frontend**
- `frontend/console/src/components/GatewayRunView.svelte` — consensus 배지 + variant stream + USD 배지 렌더
- `frontend/console/src/lib/api.ts` — consensus event 타입 추가

## 테스트 계획

### PR-A
- Runtime semaphore가 실제로 concurrency를 제한하는지 (2개 동시 요청에서 1개는 대기하는지)
- `/v1/gateway/runs/{id}/events` SSE가 구독자 없이도 publish에 블로킹하지 않는지
- run_accepted/started/finished 이벤트 순서가 일관적인지
- 콘솔 run view가 기존 subagents_run 결과를 렌더링하는지 (수동 스크린샷 첨부)

### PR-B (Unit)
- `consensus_test.go`: 2/3 variant, 전부 성공/일부 실패/전부 실패, strategy synthesize only
- `strategy=vote`가 `ErrStrategyNotImplemented`를 반환
- Token budget 초과 시 즉시 취소
- **USD budget 초과** 시 실행 전 거절 (한 variant도 시작하지 않음)
- Timeout 초과 시 context.DeadlineExceeded
- **Pool 격리**: consensus 진행 중 `subagents_run`이 일반 pool에서 정상 진행

### PR-B (Integration)
- `gateway_api_test.go`에 consensus 실행 end-to-end
- SSE 이벤트 순서 검증 (`consensus_planned` → variant_started * N → variant_finished * N → aggregating → finished)
- `runs.json` round-trip에 `consensus_*` 필드 포함

### Manual / Dev
- `make dev-console`로 띄우고 `subagents_run mode=consensus`로 실제 호출
- 비용이 크므로 초기에는 `gemini-flash` 같은 저가 alias로만 테스트

## Acceptance Criteria

### PR-A
- [ ] `/v1/gateway/runs/{id}/events` SSE endpoint 동작 (notification broker와 분리)
- [ ] gateway.Runtime에 실행 세마포어 도입, 기본 subagent pool이 여기서 동작
- [ ] 콘솔에 `gateway/runs/:id` 라우트 + 최소 run 상세 뷰
- [ ] #03의 `resolved_*` 필드가 이 뷰에 표시됨
- [ ] 기존 subagents_run 동작 변화 없음 (semaphore 크기를 현재 max_threads에 맞춤)
- [ ] `make test`, `make vet`, `make fmt` 통과

### PR-B
- [ ] `gateway_consensus_enabled=false` (기본)에서 코드 경로가 완전히 dormant
- [ ] `mode=consensus`로 2~3 variant 병렬 실행, light-tier aggregator로 합성
- [ ] `synthesize` 전략이 동작. `vote`는 `ErrStrategyNotImplemented` (후속 PR)
- [ ] Token budget·USD budget·timeout·fanout 제한이 Go 단에서 hard stop
- [ ] **Consensus variant는 `consensusPool`에서 실행**되어 일반 subagent pool을 블록하지 않음 (integration test로 검증)
- [ ] `consensus_planned`가 variant 실행 **전**에 발행되고 `cost_usd_estimate` 포함
- [ ] `runs.json` 스냅샷에 `consensus_mode`, `consensus_variants`, `consensus_cost_usd`, `consensus_budget_usd` 기록 (round-trip)
- [ ] 콘솔 gateway run view에서 consensus 배지 + USD 배지 + variant stream 표시
- [ ] Variant override는 `#03`의 alias allowlist와 consensus 전용 `allowed_aliases` 양쪽에서 검증됨
- [ ] `make test`, `make vet`, `make fmt` 통과
- [ ] README 비교표 업데이트 (TARS 쪽 "MoA" 칸 갱신)

## Identity Check

- **단일 Go 바이너리**: 새 런타임 의존성 없음 ✓
- **File-first**: 감사 로그는 기존 `runs.json` 스냅샷 필드 확장 ✓
- **Scope isolation**: User surface 한정 ✓
- **정책은 config, 메커니즘은 Go**: 예산·fanout·allowlist가 전부 config, 실행·차단이 Go ✓
- **LLM은 판단·합성까지, 액션은 Go**: aggregator LLM도 텍스트 합성만. Variant 실행·취소·budget은 Go가 결정론적으로 제어 ✓
- **Opt-in**: `gateway_consensus_enabled=false` 기본, 기존 사용자 동작 무변경 ✓
- **Memory 영향 없음** ✓

## 비용·위험 경고

- Consensus 1회 = provider N개 × 입력 토큰 + aggregator 입력 토큰. 쉽게 10배 이상 비쌀 수 있음.
- 기본 비활성화, token/USD budget config, 콘솔에 예상 비용 배지가 **반드시 함께 제공**되어야 merge.
- 초기에는 `subagents_run` 레벨에서만 사용 가능하게 열고, 일반 chat 응답에 자동 적용되는 경로는 열지 않음.
- `vote` 전략은 length bias가 크고 평가 프롬프트 튜닝이 필요하므로 **후속 PR로 분리**한다. 이 PR에서는 데이터 모델만 예약.
- PR-A를 먼저 이동하지 않고 PR-B를 단독 merge하지 않는다 — 리뷰어가 "이벤트가 어디로 가야 하는지" 같은 질문을 PR-B에서 해결하려 하면 scope가 폭발한다.
