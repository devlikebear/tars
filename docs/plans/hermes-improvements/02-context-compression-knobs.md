# 02 — Context Compression Knobs 노출

**Branch**: `feat/hermes-compression-knobs`
**Sprint**: S1
**Area**: `area/config`
**Depends on**: —

## 배경

Hermes의 `ContextCompressor`는 단순하다. "토큰 사용량이 컨텍스트 한도의 일정 비율을 넘으면 오래된 턴부터 요약하되 최근 N턴은 보호한다." 예측 가능성이 강점이다.

TARS에는 이미 두 층의 compaction 경로가 **현재 코드에 존재**한다:

1. `internal/session/compaction.go`의 `CompactTranscriptWithOptions` — transcript를 최근 구간은 보존하고 오래된 구간은 요약으로 대체하는 핵심 엔트리.
2. `internal/tarsserver/helpers_chat.go`의 `compactWithMemoryFlush` — `CompactOptions.SummaryBuilder`에 `buildLLMCompactionSummary`를 꽂아 `llm.RoleContextCompactor` 기반 요약을 시도하고, 실패·nil client 시 `session.BuildCompactionSummaryWithOptions`(순수 결정론적 요약)로 떨어진다 (`helpers_chat.go:69-80, 159-168`).

따라서 이 PR은 "LLM fallback을 새로 만드는 것"이 **아니라** 이미 있는 deterministic fallback을 (a) config로 제어 가능하게 하고 (b) drop-unit 안전성을 강화하며 (c) 콘솔에 상태를 드러내는 작업이다.

## 현황 (확인 완료된 사실)

- 현재 하드코드 상수 (`internal/tarsserver/main_options.go:38-44`):
  ```go
  chatHistoryMaxTokens     = 120000
  autoCompactTriggerTokens = 100000
  autoCompactKeepRecent    = 0
  autoCompactKeepTokens    = session.DefaultKeepRecentTokens  // 12000
  autoCompactKeepShare     = session.DefaultKeepRecentFraction // 0.30
  ```
- 관련 session-level 상수 (`internal/session/compaction.go:11-21`):
  ```go
  DefaultKeepRecentMessages = 20
  MinKeepRecentMessages     = 5
  MaxKeepRecentMessages     = 200
  DefaultKeepRecentTokens   = 12000
  MinKeepRecentTokens       = 1
  MaxKeepRecentTokens       = 64000
  DefaultKeepRecentFraction = 0.30
  MinKeepRecentFraction     = 0.05
  MaxKeepRecentFraction     = 0.90
  ```
- 트리거는 **비율이 아니라 절대 토큰**(`EstimateTokens >= autoCompactTriggerTokens`). 트리거 초과 시 `compactWithMemoryFlush`가 `KeepRecentTokens=12000 + KeepRecentFraction=0.30` 조합으로 tail을 보존한다. 즉 "절대 토큰으로 발동, tail은 (최소 토큰, 비율) 둘 중 큰 쪽으로 보존"하는 하이브리드 모델이다 — 이것을 단일 ratio로 억지 재표현하면 정보가 깎인다.
- LLM 요약 경로는 `buildLLMCompactionSummary` → `client.Chat(...)`. 에러/빈 응답 시 `session.BuildCompactionSummaryWithOptions`로 폴백 (`helpers_chat.go:159-168`). 이 deterministic 경로는 이미 존재하고 항상 돌아간다.
- (구현 착수 시 추가 확인) `ContextMonitor.svelte`가 현재 드러내는 상태, chat pipeline 레벨에서 `session.CompactResult`가 어디까지 전파되는지.

## 목적

1. 하드코드 상수 3개를 **1:1로** config 필드로 승격한다. 새 추상(ratio/N-turn) 도입 없음.
2. LLM 요약 경로의 예산·타임아웃 knob을 추가해 운영자가 상한을 걸 수 있도록 한다.
3. Deterministic fallback 경로가 **drop-unit 안전**하다는 것을 golden test로 잠가둔다 (tool_use/tool_result 페어 보존).
4. Chat pipeline이 compaction 결과를 SSE 이벤트로 드러내고, 콘솔 `ContextMonitor`가 현재 임계값·보호 구간·마지막 모드를 표시한다.

## 제안 설계

### 신규 config 필드

| 필드 | 기본값 | 현재 매핑 |
|---|---|---|
| `compaction_trigger_tokens` | `100000` | `autoCompactTriggerTokens` |
| `compaction_keep_recent_tokens` | `12000` | `autoCompactKeepTokens` → `DefaultKeepRecentTokens` |
| `compaction_keep_recent_fraction` | `0.30` | `autoCompactKeepShare` → `DefaultKeepRecentFraction` |
| `compaction_llm_mode` | `"auto"` | `auto`: LLM 우선, 실패 시 deterministic. `deterministic`: LLM 호출 없이 deterministic 경로만. |
| `compaction_llm_timeout_seconds` | `15` | 요약 호출 타임아웃. 초과 시 deterministic 경로로 폴백. |

> **기본값 원칙**: 첫 세 필드는 **현재 하드코드 상수를 숫자 그대로** 승격한다. `main_options.go`의 상수 선언을 지우고 config 로더에서 주입하면 관찰 가능한 동작은 달라지지 않는다. PR 설명에 "기존 동작과 수치 동등: trigger_tokens=100000, keep_recent_tokens=12000, keep_recent_fraction=0.30"을 명시한다.

> **Ratio 모델을 도입하지 않는 이유**: 현재는 "절대 토큰 트리거 + (최소 토큰, 비율) 이중 tail"이라 ratio 하나로는 의미 손실이 발생한다. 이 PR은 *보존 우선*이며, ratio 기반 재설계가 필요하다면 별도 PR에서 근거(benchmark, UX study)와 함께 논의한다.

환경변수 오버라이드는 기존 `TARS_` prefix 규칙을 그대로 따른다 (`TARS_COMPACTION_TRIGGER_TOKENS` 등).

### LLM mode 해석

```go
// internal/tarsserver/helpers_chat.go
func compactionClient(router llm.Router, mode string) llm.Client {
    if router == nil || strings.EqualFold(mode, "deterministic") {
        return nil // SummaryBuilder가 nil client를 받으면 자연스럽게 deterministic 경로
    }
    client, _, err := router.ClientFor(llm.RoleContextCompactor)
    if err != nil {
        return nil
    }
    return client
}
```

`mode=auto`는 **현재 동작**이다. `mode=deterministic`은 router 해석 자체를 건너뛰어 LLM을 **한 번도 호출하지 않는다** — 운영 시나리오(off-net, air-gapped, 비용 제로 요구)에서 강제 보장용.

### Deterministic 경로의 drop-unit 안전성

현재 `session.BuildCompactionSummaryWithOptions` / `CompactTranscriptWithOptions`는 messages를 다룰 때 user/assistant/tool message 경계를 섞지 않는다. 하지만 "tool_use ↔ tool_result 페어가 함께 유지되는가"는 **현재 테스트가 커버하지 않는 회색 지대**다. 이 PR은 이 영역을 명시적으로 잠근다:

- **Drop unit**: 메시지가 아니라 완결된 블록.
  - `user → assistant` 쌍은 한 단위. 중간만 drop 금지.
  - `assistant(tool_use) → tool(tool_result) → assistant(follow-up)` 삼단 블록은 함께 drop하거나 함께 유지. 그 중간만 남기면 Anthropic/OpenAI가 `400 invalid messages`로 거절한다.
  - System prompt는 절대 drop하지 않는다.

- **Golden test** (`compaction_test.go` 신규 케이스):
  - tool_use/tool_result 페어가 있는 전사에서, 각 compaction 결과가 여전히 valid sequence인지 검증 (orphan tool_result 0건, orphan tool_use 0건).
  - 2개 이상의 연속 tool_use 블록이 있을 때 중간에서 끊기지 않는지.
  - User-only / assistant-only 비정상 전사에서도 panic 없이 통과.

### Compaction 이벤트 노출

`CompactResult`는 이미 `Compacted / OriginalCount / FinalCount / CompactedCount / Summary` 필드를 갖고 있다 (`compaction.go:30-36`). 이걸 chat pipeline이 SSE로 방출한다:

```json
{
  "type": "compaction_applied",
  "session_id": "...",
  "mode": "llm" | "deterministic",
  "original_count": 128,
  "final_count": 42,
  "compacted_count": 86,
  "trigger_tokens": 100000,
  "estimated_tokens_before": 112450
}
```

`mode`는 `buildLLMCompactionSummary`가 실제로 LLM을 성공적으로 호출했는지(`llm`) 아니면 deterministic 경로를 탔는지(`deterministic`)를 구분한다. 이 플래그는 `CompactOptions.BeforeRewrite` 콜백 시점에 알 수 있으므로 체인에 담아 이벤트 브로드캐스트까지 내려보낸다.

### ContextMonitor UI

- 현재 사용 중인 토큰 / 한도 / trigger 라인을 표시
- `keep_recent_tokens`/`keep_recent_fraction`으로 산정된 보호 구간을 다른 색으로 표시
- 마지막 compaction 이벤트의 `mode`를 뱃지로 표시 (`LLM` / `Deterministic`)

## 수정 대상

### Backend
- `internal/config/config.go` — 5개 필드 추가 (`CompactionTriggerTokens` 등)
- `internal/config/config_input_fields.go` — 매핑과 env var 해석
- `internal/config/config_test.go` — 기본값·오버라이드 테스트
- `internal/tarsserver/main_options.go` — 하드코드 상수 3개를 config에서 주입받도록 변경 (상수 선언 제거)
- `internal/tarsserver/helpers_chat.go` — `compactionClient`가 `mode` 인자를 받고, `maybeAutoCompactSession`이 config 기반 timeout 적용
- `internal/session/compaction.go` — 필요 시 SummaryBuilder가 실제 LLM 사용 여부를 `CompactResult`에 기록할 수 있도록 작은 확장 (예: `CompactResult.SummaryMode string`)
- `internal/session/compaction_test.go` — **신규 golden test**: tool_use/tool_result 보존, user/assistant 쌍 보존, system 메시지 불변
- `internal/tarsserver/handler_chat_pipeline.go` (또는 compaction을 호출하는 지점) — `compaction_applied` SSE 이벤트 방출

### Frontend
- `frontend/console/src/components/ContextMonitor.svelte` — trigger 라인, 보호 구간, 최종 mode 뱃지
- `frontend/console/src/lib/api.ts` — `compaction_applied` 이벤트 타입 추가

### Docs
- `config/standalone.yaml`에 주석과 함께 신규 필드 샘플
- `docs/` 아래 compaction 관련 문서가 있다면 신규 knob 설명 추가

## 테스트 계획

### Unit
- **Config parsing**: 기본값(`100000/12000/0.30/auto/15`), YAML 오버라이드, env var 오버라이드
- **LLM mode 분기**: `mode=deterministic`일 때 `compactionClient`가 router를 touch조차 하지 않는지 (mock router로 검증)
- **Deterministic drop-unit 보존**: golden test — tool_use/tool_result 페어 분리 0건, user/assistant 쌍 분리 0건
- **Timeout**: `compaction_llm_timeout_seconds` 이하에서 호출이 돌아오지 않으면 deterministic 경로로 폴백

### Integration
- `handler_chat_pipeline` 레벨에서 긴 전사를 넣고 compaction이 실제 적용되는지, 결과 `mode` 필드가 LLM 성공/실패에 따라 올바른지
- `compaction_applied` SSE 이벤트가 실제 방출되는지

## Acceptance Criteria

- [ ] 5개 config 필드가 YAML, env var, `config_input_fields` 매핑으로 접근 가능
- [ ] `compaction_trigger_tokens` / `compaction_keep_recent_tokens` / `compaction_keep_recent_fraction` 기본값이 **현재 상수(`100000`, `12000`, `0.30`)와 수치 동등**
- [ ] `main_options.go`의 하드코드 상수 3개가 config에서 주입되도록 삭제됨
- [ ] `compaction_llm_mode=deterministic`일 때 LLM router를 **단 한 번도** 호출하지 않음 (mock으로 검증)
- [ ] Deterministic 경로가 tool_use/tool_result 페어를 분리하지 않음 (golden test)
- [ ] LLM 호출 실패/타임아웃 시 기존처럼 deterministic 경로로 자동 폴백 (회귀 방지 테스트)
- [ ] `compaction_applied` SSE 이벤트 방출 (`mode`, `original_count`, `final_count`, `compacted_count`, `trigger_tokens`, `estimated_tokens_before` 필드)
- [ ] ContextMonitor에 trigger 라인 + 보호 구간 + 최종 mode 뱃지 표시
- [ ] `make test`, `make vet`, `make fmt` 통과

## Identity Check

- **단일 Go 바이너리** ✓
- **File-first**: transcript 파일 포맷 불변, SSE 이벤트만 추가 ✓
- **Scope isolation**: User surface 한정, `pulse_/reflection_` 경로 무관 ✓
- **정책은 config, 메커니즘은 Go**: 모든 knob이 config 필드로 표현되고, fallback 결정은 Go 분기로 ✓
- **Memory 영향**: compaction 결과가 memory에 저장되는 기존 경로(`IndexCompactionSummary`, `IndexCompactionMemories`)는 **변경하지 않음** ✓
- **기능 보존**: 기본값이 현재 하드코드와 수치 동등이므로 사용자가 관찰하는 동작은 달라지지 않음 ✓
