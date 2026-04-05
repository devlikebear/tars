# PR1 — Replace Heartbeat with Pulse Surface

**Branch**: `feat/replace-heartbeat-with-pulse-surface`
**Status**: Planning → Implementation

## 목적

Heartbeat 개념 폐기. 두 개의 시스템 표면(surface)으로 분리:

- **User surface**: 사용자 대화/autopilot/gateway — 기존 도구/스킬 사용
- **System surface**: pulse(1분 감시), reflection(야간 배치) — 전용 최소 도구만 사용

PR1은 pulse 신설과 사용자 표면에서 ops 도구 완전 제거를 담당. Reflection은 PR2, 설정 UI 고도화는 PR3.

## 설계 원칙

1. **Hard isolation**: 사용자 세션 tool registry는 `ops_/pulse_/reflection_` 접두어 도구를 panic으로 차단.
2. **LLM은 판단기, Go는 실행기**: pulse LLM은 `pulse_decide` 단일 도구만 호출. 실제 실행(notify/autofix)은 Go 런타임이 결정론적으로 수행.
3. **정책은 config 단일 소스**: PULSE.md 같은 파일 없음. 모든 정책은 config에서 관리.
4. **Autofix는 화이트리스트**: Go 코드에 구현된 autofix만 실행 가능. LLM이 임의 명령 실행 불가.
5. **Phase 1 YAGNI**: autopilot signal 없음(구현체 부재), reflection 필드 placeholder 없음, delivery failure는 얇은 counter만.

## 삭제 대상

### 코드
- `internal/heartbeat/` 디렉터리 전체
- `internal/tool/automation_heartbeat.go`
- `internal/tool/tool_ops.go`
- `internal/tool/ops_aggregator.go`
- `internal/tarsserver/helpers_heartbeat_ticker.go`
- `internal/tarsserver/handler_ops.go` (API handler — pulse로 대체)
- `plugins/ops-service/` 전체 (6개 스킬)

### Workspace 템플릿
- `workspace/HEARTBEAT.md`
- 기본 workspace 생성 시 HEARTBEAT.md 생성 로직 제거

## 수정 대상

### Backend
- `internal/tarsserver/main_bootstrap.go` — heartbeat 기동 → pulse 기동
- `internal/tarsserver/helpers_runtime.go` — heartbeat runtime state → pulse runtime state
- `internal/tarsserver/main_serve_api.go` — `/v1/heartbeat/*` → `/v1/pulse/*`, ops manager 주입 경로 유지
- `internal/tarsserver/handler_chat_policy.go` — `NewOpsTool`, `NewHeartbeatTool` 등록 제거
- `internal/tarsserver/helpers_agent.go` — 동일
- `internal/tarsserver/helpers_build_tools.go` — automation tools에서 heartbeat 제거
- `internal/tool/tool.go` — `Registry.Register()`에 hard isolation assertion + `RegistryScope` 추가
- `internal/tool/tool_name.go` — ops_* name mapping 제거
- `internal/config/config.go` + `config_input_fields.go` — `heartbeat_*` 필드 → `pulse_*` 필드

### Frontend
- `frontend/console/src/components/Heartbeat.svelte` → `Pulse.svelte` (rename + API 경로 수정)
- `frontend/console/src/components/Nav.svelte` — heartbeat 탭 → pulse 탭
- `frontend/console/src/lib/api.ts` — heartbeat 함수 → pulse 함수
- `frontend/console/src/lib/router.ts` — 라우트 경로 수정

> NOTE: PR1 Frontend 변경은 **최소한의 rename**만. UI 고도화(설정 편집 UI 등)는 PR3에서.

## 신규 파일

### Pulse 코어
```
internal/pulse/
├── pulse.go          # Runtime 구조체, Start/Stop
├── ticker.go         # 1분 ticker + hung-task 격리 (cancelable context)
├── signal.go         # Signal 수집기 (cron, gateway, ops, delivery failures)
├── decider.go        # LLM 판단기 호출 + 응답 파싱
├── notify.go         # notification 경로 (session event + telegram)
├── state.go          # runtime state (lastTick, lastDecision, recentSignals)
└── autofix/
    ├── registry.go            # 화이트리스트 + 실행기
    ├── compress_old_logs.go   # autofix #1
    └── cleanup_stale_tmp.go   # autofix #2
```

### Pulse 전용 도구
```
internal/tool/pulse_decide.go   # pulse_decide 단일 도구 (pulse scope 전용)
```

### Telegram delivery counter
```
internal/tarsserver/telegram_delivery_counter.go   # 얇은 wrapper, ring buffer
```

### API handler
```
internal/tarsserver/handler_pulse.go   # /v1/pulse/* endpoints (status, logs, run-once)
```

## Config 신규 필드

| Go 필드 | YAML | Env | 기본값 |
|---|---|---|---|
| `PulseEnabled` | `pulse_enabled` | `TARS_PULSE_ENABLED` | `true` |
| `PulseInterval` | `pulse_interval` | `TARS_PULSE_INTERVAL` | `1m` |
| `PulseTimeout` | `pulse_timeout` | `TARS_PULSE_TIMEOUT` | `2m` |
| `PulseActiveHours` | `pulse_active_hours` | `TARS_PULSE_ACTIVE_HOURS` | `00:00-24:00` |
| `PulseTimezone` | `pulse_timezone` | `TARS_PULSE_TIMEZONE` | `Local` |
| `PulseDiskWarnPct` | `pulse_disk_warn_pct` | `TARS_PULSE_DISK_WARN_PCT` | `0.85` |
| `PulseDiskCriticalPct` | `pulse_disk_critical_pct` | `TARS_PULSE_DISK_CRITICAL_PCT` | `0.95` |
| `PulseStuckRunMinutes` | `pulse_stuck_run_minutes` | `TARS_PULSE_STUCK_RUN_MINUTES` | `60` |
| `PulseCronFailureThreshold` | `pulse_cron_failure_threshold` | `TARS_PULSE_CRON_FAILURE_THRESHOLD` | `3` |
| `PulseDeliveryFailureThreshold` | `pulse_delivery_failure_threshold` | `TARS_PULSE_DELIVERY_FAILURE_THRESHOLD` | `3` |
| `PulseDeliveryFailureWindow` | `pulse_delivery_failure_window` | `TARS_PULSE_DELIVERY_FAILURE_WINDOW` | `10m` |
| `PulseAutofixEnabled` | `pulse_autofix_enabled` | `TARS_PULSE_AUTOFIX_ENABLED` | `true` |
| `PulseAutofixAllowed` | `pulse_autofix_allowed` | `TARS_PULSE_AUTOFIX_ALLOWED` | `compress_old_logs,cleanup_stale_tmp` |
| `PulseNotifyTelegram` | `pulse_notify_telegram` | `TARS_PULSE_NOTIFY_TELEGRAM` | `false` |
| `PulseNotifySessionEvents` | `pulse_notify_session_events` | `TARS_PULSE_NOTIFY_SESSION_EVENTS` | `true` |
| `PulseMinSeverity` | `pulse_min_severity` | `TARS_PULSE_MIN_SEVERITY` | `warn` |

Heartbeat 필드(`HeartbeatInterval`, `HeartbeatActiveHours`, `HeartbeatTimezone`)는 **삭제**. backward-compat 없음.

## Pulse 1턴 동작 시퀀스

```
1. Ticker fires (1분 주기)
2. Active hours 체크 → out-of-window면 skip (silent)
3. Signal scan (Go, 결정론적):
   a. cron.Store — 최근 job들의 ConsecutiveFailures 합산
   b. gateway.Runtime — Running 상태 >N분 run 집계 (stuck runs)
   c. ops.Manager.Status() — disk usage pct
   d. telegram_delivery_counter — 최근 N분 실패 집계
4. 임계값 체크:
   - 모든 signal 임계 미만 → silent return + state update
5. LLM 판단 호출 (별도 goroutine + cancelable context + timeout):
   - Prompt: signal 요약 + config 정책(임계값, 허용 autofix)
   - Tool: pulse_decide 단 하나
   - 반환: {action, severity, title, summary, details, autofix_name?}
6. Action 실행:
   - ignore → state만 기록
   - notify → session event 발행 + (config 허용 시) telegram 전달
   - autofix → Go 화이트리스트 조회 → 함수 호출 → 결과 이벤트
7. State 업데이트: lastTick, lastDecision, recentSignals, recentDecisions ring
```

## Hard Isolation 메커니즘

```go
// internal/tool/tool.go

type RegistryScope int

const (
    RegistryScopeUser RegistryScope = iota  // chat, agent
    RegistryScopePulse                       // pulse runtime
    RegistryScopeReflection                  // reflection runtime (PR2)
)

var forbiddenPrefixes = map[RegistryScope][]string{
    RegistryScopeUser: {"pulse_", "reflection_", "ops_"},
    RegistryScopePulse: {"ops_", "reflection_"},  // pulse도 reflection 도구 접근 금지
    RegistryScopeReflection: {"ops_", "pulse_"},
}

func (r *Registry) Register(t Tool) {
    for _, p := range forbiddenPrefixes[r.scope] {
        if strings.HasPrefix(t.Name(), p) {
            panic(fmt.Sprintf(
                "tool %q cannot be registered in scope %v (forbidden prefix %q)",
                t.Name(), r.scope, p))
        }
    }
    // ... existing registration logic
}
```

- User registry는 ops/pulse/reflection 전부 차단
- Pulse registry는 ops/reflection 차단 (pulse는 `internal/ops`를 Go API로 직접 호출)
- Reflection도 동일

## Telegram Delivery Counter

얇은 wrapper + ring buffer로 **최근 N분 실패 개수**만 카운트. 외부 API 없음.

```go
// internal/tarsserver/telegram_delivery_counter.go

type DeliveryCounter struct {
    mu      sync.Mutex
    attempts []DeliveryAttempt  // ring, cap 100
}

type DeliveryAttempt struct {
    At      time.Time
    Success bool
    Error   string
}

func (c *DeliveryCounter) Record(success bool, err error)
func (c *DeliveryCounter) FailuresWithin(window time.Duration) int
```

Telegram sender가 송신 시 `Record()` 호출. pulse signal scanner가 `FailuresWithin()` 쿼리.

## Autofix Phase 1 목록

### `compress_old_logs`
- **조건**: `workspace/logs/` 내 7일 이상 된 `.log` 파일 ≥ 10개 존재
- **동작**: 각 파일 gzip 압축 후 원본 삭제
- **안전성**: idempotent, dry-run 지원, 실패 시 원본 보존

### `cleanup_stale_tmp`
- **조건**: `workspace/tmp/` 또는 `workspace/_shared/tmp/` 내 7일 이상 된 파일 존재
- **동작**: 해당 파일 삭제
- **안전성**: 심볼릭 링크 제외, 디렉터리 제외 (파일만)

두 autofix 모두 config `PulseAutofixAllowed`에서 제거하면 비활성화.

## 테스트 계획

### Unit
- `internal/pulse/signal_test.go` — 각 signal source mock, 임계값 분기
- `internal/pulse/decider_test.go` — LLM mock, 정상 응답 파싱, malformed 응답 처리, timeout
- `internal/pulse/ticker_test.go` — 1분 간격, active hours 필터, hung task 격리, graceful cancel
- `internal/pulse/state_test.go` — ring buffer, 동시성
- `internal/pulse/notify_test.go` — severity routing, telegram on/off
- `internal/pulse/autofix/compress_old_logs_test.go` — dry-run, 실제 압축, 실패 시 rollback
- `internal/pulse/autofix/cleanup_stale_tmp_test.go` — 심링크 제외, age 필터
- `internal/pulse/autofix/registry_test.go` — 화이트리스트 조회, 미허용 autofix 거부
- `internal/tool/tool_test.go` — Registry scope별 forbidden prefix panic
- `internal/tool/pulse_decide_test.go` — 도구 스키마 validation
- `internal/tarsserver/telegram_delivery_counter_test.go` — ring buffer, window 계산
- `internal/tarsserver/handler_pulse_test.go` — `/v1/pulse/*` endpoint

### Integration
- `internal/tarsserver/pulse_e2e_test.go` — 임계값 초과 시 LLM mock 호출 → notify 경로 end-to-end

## 커밋 분할 (작업 순서)

모든 커밋 후 `make test && make vet` 녹색 유지(또는 명시적 사유).

1. **chore: add PR1 plan doc** — 이 파일 커밋 ✅
2. **refactor: introduce RegistryScope with hard isolation** — `tool.go` 수정 + 테스트. 아직 prefix 비어있어 기존 동작 변화 없음.
3. **feat: add telegram delivery counter** — wrapper + 테스트. 기존 sender에 instrumentation.
4. **feat: add pulse package skeleton** — 빈 패키지 + 컴파일 가능한 stub. 테스트 최소.
5. **feat: implement pulse signal scanner** — cron/gateway/ops/delivery signal 수집 + 테스트.
6. **feat: implement pulse decider and notify** — LLM 판단 + notification 경로 + 테스트.
7. **feat: implement pulse autofix whitelist** — registry + 2개 autofix + 테스트.
8. **feat: implement pulse ticker and runtime** — 1분 ticker, hung 격리, state, start/stop + 테스트.
9. **feat: add pulse_decide tool and pulse HTTP API** — 전용 도구 + handler + 테스트.
10. **refactor: wire pulse into server runtime** — main_bootstrap, main_serve_api, helpers_runtime 수정. pulse 활성화.
11. **refactor: remove ops_* tool wrappers from user surface** — tool_ops.go, ops_aggregator.go 삭제 + chat/agent 등록 제거 + Registry scope 강제로 panic 차단 검증.
12. **refactor: remove heartbeat package and wiring** — internal/heartbeat/ 전체 삭제 + HEARTBEAT.md + helpers_heartbeat_ticker.go + automation_heartbeat.go + config 필드 제거.
13. **chore: delete plugins/ops-service** — 6개 스킬 삭제.
14. **feat(frontend): rename Heartbeat to Pulse** — Svelte 컴포넌트 rename, Nav, api.ts, router.ts.
15. **docs: update CLAUDE.md with pulse/reflection architecture notes** — CLAUDE.md 내 heartbeat 섹션 업데이트. AutopilotManager 문서/실제 불일치도 정리(Phase 1에선 "문서상 존재, 구현 예정" 문구로).

## 체크리스트

- [ ] Plan 파일 커밋 (this commit)
- [ ] RegistryScope + hard isolation
- [ ] Telegram delivery counter
- [ ] Pulse 패키지 전체
- [ ] Pulse HTTP API
- [ ] Server wiring
- [ ] ops_* 도구 제거
- [ ] Heartbeat 제거
- [ ] plugins/ops-service 삭제
- [ ] Frontend rename
- [ ] CLAUDE.md 업데이트
- [ ] `make test` 녹색
- [ ] `make vet` 녹색
- [ ] `make console-build` 성공
- [ ] PR 생성 + CI 통과

## 의도적으로 배제된 것들 (Phase 1 YAGNI)

- Autopilot signal (구현체 부재)
- Delivery failure의 자세한 failure 로그 (카운터만 있음)
- PULSE.md / REFLECTION.md 파일 (정책은 config로)
- Reflection 관련 config 필드 (PR2에서)
- Frontend 설정 편집 UI 고도화 (PR3에서)
- Pulse autofix 3번째 이상 (후속 PR)
- Pulse self-memory / 정체성 확장 (영구 제외)
- User 세션에서 ops 도구 접근 (영구 제외)
