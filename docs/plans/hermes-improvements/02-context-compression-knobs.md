# 02 — Context Compression Knobs 노출

**Branch**: `feat/hermes-compression-knobs`
**Sprint**: S1
**Area**: `area/config`
**Depends on**: —

## 배경

Hermes의 `ContextCompressor`는 단순하다. "토큰 사용량이 컨텍스트 한도의 50%를 넘으면 오래된 턴부터 요약하되 최근 N턴은 보호한다." 예측 가능성이 강점이다.

TARS의 구조화 compaction(identifier 보존 + light-tier LLM 요약)은 품질은 더 좋지만 (1) 임계값·보호 턴 수 같은 튜닝 knob이 config에 노출돼 있지 않고, (2) LLM 호출이 실패하거나 예산이 부족할 때의 fallback 경로가 사용자에게 보이지 않는다.

## 목적

1. Compaction 튜닝 knob을 config에 노출.
2. LLM 호출 실패·예산 초과 시 결정론적 fallback 모드 도입.
3. 콘솔 `ContextMonitor.svelte`에 현재 임계값과 보호 구간 시각화.

## 현황 조사

(구현 착수 시 확인할 것)

- Compaction 로직 위치: `internal/session/` 또는 `internal/prompt/` 하위. 구체 함수 식별.
- 현재 하드코드된 임계값, 보호 턴 수
- Light-tier LLM 호출이 실패할 때 현재 어떤 fallback이 있는지 (있긴 한지)
- `ContextMonitor.svelte`가 현재 드러내는 상태

## 제안 설계

### 신규 config 필드

| 필드 | 기본값 | 의미 |
|---|---|---|
| `compaction_threshold_ratio` | `0.75` | 전체 컨텍스트 한도 대비 이 비율을 넘으면 compaction 시작 |
| `compaction_protect_last_n_turns` | `5` | 아무리 컨텍스트가 커져도 최근 N턴은 건드리지 않음 |
| `compaction_fallback_mode` | `"structured"` | `structured` \| `simple`. `simple`은 LLM 호출 없이 오래된 턴을 단순 drop |
| `compaction_llm_budget_tokens` | `2000` | 요약용 LLM 호출 1회당 상한. 초과 예상 시 자동 `simple` fallback |
| `compaction_llm_timeout_seconds` | `15` | 요약 호출 타임아웃. 초과 시 자동 `simple` fallback |

환경변수 오버라이드 전부 지원 (`TARS_COMPACTION_THRESHOLD_RATIO` 등).

### Simple fallback 알고리즘

```
1. 최근 compaction_protect_last_n_turns 턴을 별도로 떼어둠
2. 남은 턴을 오래된 순으로 drop하여 토큰이 threshold * 0.85 아래로 떨어질 때까지 반복
3. System/tool 메시지는 유지, 사용자/assistant 본문만 drop
4. 각 drop마다 `{"type":"compaction_drop", "range":[i,j]}` 트레이스 기록
```

LLM이 전혀 관여하지 않으므로 예산·네트워크·장애와 무관하게 동작. 품질은 떨어지지만 **결코 실패하지 않는다**.

### Structured → Simple fallback 경로

```go
func Compact(ctx context.Context, turns []Turn, cfg Config) (Result, error) {
    switch cfg.FallbackMode {
    case "simple":
        return compactSimple(turns, cfg)
    default:
        res, err := compactStructured(ctx, turns, cfg)
        if err != nil || res.BudgetExceeded {
            return compactSimple(turns, cfg), nil
        }
        return res, nil
    }
}
```

`BudgetExceeded`는 호출 전 예상 토큰이 `compaction_llm_budget_tokens`보다 크면 true.

### ContextMonitor UI

- 현재 사용 중인 토큰 / 한도 / threshold 라인을 표시
- 보호 구간을 다른 색으로 표시
- Compaction 발생 시 마지막 이벤트의 `structured` vs `simple` 표시

## 수정 대상

### Backend
- `internal/config/config.go` — 5개 필드 추가
- `internal/config/config_input_fields.go` — 매핑과 env var 해석
- `internal/config/config_test.go` — 기본값·오버라이드 테스트
- `internal/session/compaction.go` (또는 현재 compaction이 사는 파일) — `Compact` 엔트리 분기
- `internal/session/compaction_simple.go` — 신규, drop 알고리즘
- `internal/session/compaction_test.go` — structured/simple 두 경로 모두 표 기반 테스트
- `internal/tarsserver/handler_chat_pipeline.go` — compaction 결과의 이벤트 방출(`compaction_applied` SSE 이벤트)

### Frontend
- `frontend/console/src/components/ContextMonitor.svelte` — 임계값·보호 구간·마지막 모드 표시
- `frontend/console/src/lib/api.ts` — 신규 SSE 이벤트 타입

### Docs
- `docs/` 아래 compaction 문서가 있다면 신규 knob 설명 추가
- `config/standalone.yaml`에 주석과 함께 신규 필드 샘플

## 테스트 계획

### Unit
- `compaction_simple`: protect-last-N, threshold, budget 상한 각각 표 기반
- `Compact` 분기: LLM 실패 주입 시 simple로 fallback 되는지
- Config 파싱: 기본값, YAML 오버라이드, env var 오버라이드

### Integration
- `handler_chat_pipeline` 레벨에서 긴 전사를 넣고 compaction이 실제 적용되는지
- `compaction_applied` SSE 이벤트가 방출되는지

## Acceptance Criteria

- [ ] 5개 config 필드가 YAML, env var, `config_input_fields` 매핑으로 전부 접근 가능
- [ ] `compaction_fallback_mode=simple`에서 LLM 호출 없이 동작
- [ ] Structured compaction 실패/예산 초과 시 자동 simple fallback
- [ ] `compaction_applied` SSE 이벤트 방출 (`mode`, `dropped_turns`, `kept_turns` 필드)
- [ ] ContextMonitor에 threshold·보호 구간 시각화
- [ ] 기본값(`0.75`, `5`, `structured`)이 기존 동작과 사실상 동일
- [ ] `make test`, `make vet`, `make fmt` 통과

## Identity Check

- **단일 Go 바이너리** ✓
- **File-first**: 세션 파일 형식 변경 없음, 트레이스만 추가 ✓
- **Scope isolation**: User surface 한정 ✓
- **정책은 config, 메커니즘은 Go**: 모든 knob이 config 필드로 표현됨 ✓
- **Memory 영향**: 요약이 memory에 저장되는 경로는 **변경하지 않음** ✓
