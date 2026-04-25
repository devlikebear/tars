# Questions — 미해결 질문

리뷰 중에 떠올랐지만 즉시 답하지 않은 질문들. 답이 나오면 본문에 답을 추가하고 Status를 `resolved`로.

---

## Q-001 — `envloader.Load(".env", ".env.secret")` 가 두 곳에서 호출됨

- **Status**: open
- **Location**: `cmd/tars/main.go:27` ↔ `internal/tarsserver/main.go:25`
- **Discovered in**: [journal/2026-04-25-02-server-bootstrap.md](../journal/2026-04-25-02-server-bootstrap.md)

### 질문

`bootstrapEnv()`가 이미 `cmd/tars/main.go:14-15`에서 호출되어 `.env`/`.env.secret`을 로드함. 그런데 `tarsserver.Serve` (main.go:25)도 같은 인자로 한 번 더 호출함.

가설:

- (a) **의도된 안전장치**: `tarsserver.Serve`를 외부에서 직접 import해 호출하는 케이스 대비. `main_test.go:37`의 `run()` 헬퍼도 `envloader.Load`를 직접 호출하므로 이 가설에 부합.
- (b) **잔재**: `tarsserver`가 독립 바이너리였던 시절의 흔적.

`envloader.Load`가 멱등(idempotent)인지, 즉 두 번 호출돼도 안전한지도 확인 필요.

### 후속 행동

`envloader` 패키지 리뷰 시점에 함께 답변. (a)이고 멱등하다면 의도된 안전장치로 분류 후 close. (b)이거나 비멱등하다면 RF로 승격.

---

## Q-002 — `--serve-api=false` 모드의 의도

- **Status**: open
- **Location**: `internal/tarsserver/main_bootstrap.go:108-111`, `main_cli.go:110-120`
- **Discovered in**: [journal/2026-04-25-03-runtime-deps.md](../journal/2026-04-25-03-runtime-deps.md)

### 질문

`opts.ServeAPI=false`이면:

- `buildRuntimeDeps`가 LLM 초기화를 스킵하고 deps만 만들어 조기 반환.
- `newRootCmd`의 RunE는 `"tars starting in <mode> mode"`만 출력하고 종료.

→ 사실상 **dry-run / health-check / config-validation** 모드처럼 동작. 그런데:

- `tars doctor` 명령이 별도로 존재 → 헬스체크 경로가 두 개?
- 사용자가 일부러 `--serve-api=false`로 호출할 시나리오가 있는가?
- `serveAPI` 플래그의 default가 true이므로 명시적 opt-out이 필요한 케이스가 모호.

### 후속 행동

- `tars doctor` 코드 확인 후 비교.
- git blame으로 `--serve-api` 플래그 도입 PR 추적해서 원래 의도 파악.
- 의도 불명이면 RF로 승격 (플래그 제거 또는 의미 명시).

---

## Q-003 — `daily_log` stage 라벨이 정의되지 않은 곳에서 사용됨

- **Status**: **resolved** (2026-04-25 session 04)
- **Location**: `internal/tarsserver/main_cli.go:95-96` (분류만 존재) / 발생 지점 0건
- **Discovered in**: [journal/2026-04-25-03-runtime-deps.md](../journal/2026-04-25-03-runtime-deps.md)

### 질문

`main_cli.go`의 에러 분류 switch에 `case "daily_log":` 가 있는데, `buildRuntimeDeps` 어디에서도 `&runtimeDepsError{stage: "daily_log", ...}` 형태로 에러를 만들지 않는다.

### 답

전체 `internal/` 트리 grep 결과 `stage: "daily_log"` 발생 지점 **0건**. `runServeAPICommand`, `buildAPIMux`, `resolveMainSessionID` 어디에서도 daily log 단계 에러를 생성하지 않음. 가설 (a) **좀비 케이스**로 확정.

→ [RF-005](../refactor.md#rf-005)로 승격 (case 라벨 제거).

---

## Q-008 — Plugin Source 우선순위가 슬라이스 순서에 의존

- **Status**: **resolved** (2026-04-25 session 06)
- **Location**: `internal/plugin/loader.go:33-36` (Load의 merge), `internal/tarsserver/helpers_build_extensions.go:62-85` (buildPluginSources)
- **Discovered in**: [journal/2026-04-25-06-plugin-pkg.md](../journal/2026-04-25-06-plugin-pkg.md)

### 질문

`Load(opts)` 가 source 우선순위를 호출자가 넘긴 슬라이스 순서로만 결정. `Source` enum 자체엔 우선순위 메타 없음. 호출자가 잘못 정렬하면 silent skew.

### 답

`buildPluginSources`(helpers_build_extensions.go:62-85)의 호출 순서:

```
[0] SourceBundled    cfg.PluginsBundledDir
[1] SourceUser       ~/.tarsncase/plugins        ← legacy
[2] SourceUser       ~/.tars/plugins
[3] SourceUser       cfg.PluginsExtraDirs
[4] SourceWorkspace  cfg.WorkspaceDir/plugins
```

마지막이 이기는 머지 패턴 → **결과 우선순위: workspace > user(extra) > user(.tars) > user(.tarsncase) > bundled**. 합리적이고 마이그레이션 안전(.tars가 .tarsncase를 덮음).

남은 약점: `Source` enum이 우선순위를 자체적으로 알지 못함 → 새 호출자/source 추가 시 정렬 잊으면 silent skew.

→ [RF-013](../refactor.md#rf-013)으로 승격 (Source.Priority() 메서드 + Load 자체 정렬).

---

## Q-004 — `pulseRuntime` vs `watchdogManager` 의 역할 차이

- **Status**: **resolved** (2026-04-25 session 07)
- **Location**: `internal/tarsserver/main_serve_api.go:41-42` (`serveAPIRuntime` 필드), `internal/tarsserver/helpers_watchdog.go` (workspaceWatchdogManager 정의), `internal/pulse/` (전체)
- **Discovered in**: [journal/2026-04-25-04-plugin-lifecycle.md](../journal/2026-04-25-04-plugin-lifecycle.md)

### 답

두 워치독은 **다른 surface를 책임짐**. 통합 대상 아님 — 분리가 정당.

| 컴포넌트 | Surface | 입력 신호 | 분류 방법 | 결정/실행 |
|---------|---------|----------|----------|----------|
| `pulse.Runtime` | **시스템 (system surface)** | cron failures + stuck gateway runs + disk usage + delivery failures + reflection health (5 도메인) | **LLM 분류기**(`pulse_decide` tool) | ignore / notify / **whitelisted autofix** (3중 방어 게이트) |
| `workspaceWatchdogManager` | **워크스페이스 (workspace surface)** | workspace 내부 cron store 정합성 등 deterministic 체크 | **고정 규칙** (코드) | findings 누적 + emit (LLM 미관여) |

가설 (a)/(b)/(c) 중 **(b)에 가장 가까움** — workspace 별 헬스 체커. 단 cron job 단위는 아니고 workspace 단위 정합성. (c)는 아님 (pulse 도입 후 의도적으로 둘 다 살아 있음).

### 분리가 정당한 이유

- pulse: **사람 판단이 필요한 시스템 신호** (LLM이 "이게 문제인지" 분류)
- workspace watchdog: **deterministic 정합성 검사** (코드가 "이건 명확히 문제"로 판정)

각자 다른 알림 채널/표시 위치를 가짐. 콘솔 UI에도 별도 페이지(`/console/pulse`, `/console/heartbeat→pulse redirect` 등) 또는 별도 카드.

### 잠재 미세 개선 (등록 X)

사용자 입장에서 "두 워치독을 어디서 봐야 하는지" 헷갈릴 여지. UX 통합 (한 페이지에서 둘 다 표시)은 검토 가치 있지만, 책임 분리 자체는 유지 — 코드 RF 아님.

CLAUDE.md의 *"Pulse observes reflection health via the narrow `pulse.ReflectionHealthSource` interface"* 패턴이 워크스페이스 헬스도 같은 식으로 통합 가능 (workspace watchdog → pulse signal source) — 그러나 이는 새 기능이지 RF 아님.

---

## Q-014 — `persistSnapshot` 의 2-attempt retry 효용

- **Status**: open
- **Location**: `internal/gateway/runtime_persist.go:172-206` (persistSnapshot)
- **Discovered in**: [journal/2026-04-25-22-gateway-pt2.md](../journal/2026-04-25-22-gateway-pt2.md)

### 질문

```go
for attempt := 0; attempt < 2; attempt++ {
    runs, channels, snapshotVersion := r.snapshotForPersistence()
    // ... write ...
    if currentVersion == snapshotVersion || attempt == 1 {
        return  // 성공 또는 마지막 시도
    }
    // version mismatch — 재시도
}
```

→ optimistic concurrency. 2회 cap. 의문:

- **재시도 빈도**: snapshot 읽고 write 사이 state 변경이 흔한가? 빈도 낮으면 single-attempt + lock 으로 단순화 가능.
- **실패 동작**: attempt==1 의 마지막 시도 후 version mismatch 면? 그냥 `return` — 즉 새 state 가 디스크에 안 반영. 다음 `persistSnapshot` 호출 때 다시 시도. silent loss 는 아니지만 latency.
- **lock 잡고 한 번에 처리**: read+write 를 lock 안에서 하면 mismatch 0. 다만 write 시간 동안 lock 보유 = chat turn latency 증가.

### 후속 행동

- 운영 데이터로 retry 빈도 측정 (각 `persistSnapshot` 호출의 attempts 카운트)
- 빈도 0 가까우면 single-attempt 로 단순화
- 빈도 의미 있으면 (a) 3+ retry 또는 (b) lock-based 로 정책 결정

### 관련

- snapshot store 의 atomic write (`writeJSONAtomic`) — 디스크 IO 안전성 OK

---

## Q-015 — gateway consensus 모드의 실제 활성 빈도

- **Status**: open
- **Location**: `internal/gateway/consensus.go` (전체), `internal/tool/tool_subagents.go` (`mode: "consensus"` 호출)
- **Discovered in**: [journal/2026-04-25-22-gateway-pt2.md](../journal/2026-04-25-22-gateway-pt2.md)

### 질문

consensus 모드 = 같은 prompt 를 여러 provider variant 로 동시 실행 + aggregate (synthesize 만 구현).

7-layer budget guard:
1. `GatewayConsensusEnabled` flag
2. fanout cap (`GatewayConsensusMaxFanout`)
3. token pre-check (`GatewayConsensusBudgetTokens` default 20000)
4. USD pre-check (`GatewayConsensusBudgetUSD` default $0.50)
5. timeout (`GatewayConsensusTimeoutSeconds` default 120)
6. runtime token cap (atomic counter)
7. 2 semaphore (consensusRuns + consensusPool)

→ 매우 신중한 다층 가드. 의도 강함. 다만 **실제 사용 빈도는?**

가설:
- (a) **거의 사용 안 됨** — `subagents_run` 의 `mode: "consensus"` 호출이 LLM 의 일반 chat 워크플로 에서 흔하지 않음. 사용자가 *"이 답을 합의로 처리해줘"* 같은 명시적 요청 필요. 그런데 budget 가드 때문에 비싼 모드.
- (b) **특정 high-stakes 시나리오 핵심** — 코드 리뷰, 의사결정, critical 데이터 분석 등에서 활성. 신뢰성 ↑.

258줄 + 7-layer guard + ConsensusVariantRecord/ConsensusSpec 같은 타입 → 큰 표면 (~400줄+).

### 후속 행동

- usage tracker 의 mode 별 카운트 (`subagents_run` 의 mode="consensus" vs "parallel")
- 1주/1개월 분포
- (a) 라면 외부화 후보 (별도 plugin 또는 deprecated)
- (b) 라면 RF-052 (vote 미구현) 등 강화

### 관련

- [RF-052](../refactor.md#rf-052) — vote strategy 미구현
- [Q-012](#q-012) — subagents plan/orchestrate 빈도 (관련 카테고리)

---

## Q-017 — `SessionToolConfig` 의 활용 빈도

- **Status**: open
- **Location**: `internal/session/session.go:14-25` (SessionToolConfig)
- **Discovered in**: [journal/2026-04-25-24-session.md](../journal/2026-04-25-24-session.md)

### 질문

세션별 도구 customization (`tools_enabled`/`tools_disabled`/`tools_allow_groups`/`tools_deny_groups`/`skills_enabled`/`mcp_enabled`). 사용자가 콘솔에서 *"이 세션에서만 web 도구 끄기"* 같은 워크플로 빈도?

가설:
- (a) 거의 사용 안 됨 — 시스템 레벨 ToolPolicy 가 충분
- (b) 일부 워크플로 핵심 — 보안/성능 고려 시 필요

### 후속

usage 데이터 후 결정. (a) 라면 단순화 후보.

### 관련

- ID-003 (빌트인 툴 통합) 와 같은 카테고리

---

## Q-018 — `Plan` + `Task` (tasks.go) 시스템의 활용 빈도

- **Status**: open
- **Location**: `internal/session/tasks.go` (195줄), 콘솔 UI task 페이지
- **Discovered in**: [journal/2026-04-25-24-session.md](../journal/2026-04-25-24-session.md)

### 질문

세션별 plan + task list (CRUD). LLM 의 자동 생성? 사용자 수동 입력? 빈도?

가설:
- (a) LLM 이 자체 task tracking — 빈도 있음
- (b) 사용자 수동 — 빈도 매우 낮음
- (c) deprecated 가까움 — 다른 외부 도구 (Notion, Linear) 사용

### 후속

usage 데이터 후 결정. (b)/(c) 라면 195줄 외부화/제거.

### 관련

- ID-003 카테고리 (외부화 후보)

---

## Q-016 — `runtime_nodes.go` + `NodesTool` 사용 빈도

- **Status**: open
- **Location**: `internal/gateway/runtime_nodes.go` (63줄, 3 hardcoded node), `internal/tool/tool_gateway.go` (NodesTool, cfg-flag optional)
- **Discovered in**: [journal/2026-04-25-23-gateway-pt3.md](../journal/2026-04-25-23-gateway-pt3.md)

### 질문

3 hardcoded node — `echo` (dummy) / `clock.now` (system prompt 에 이미 *"Current time"* 주입) / `sessions.latest` (sessions_list 가 더 자연스러움). LLM 워크플로 에서 사용 빈도?

가설:
- (a) 거의 사용 안 됨 — RF-055 적용 후 제거
- (b) demo/development 용도 — 빈도 매우 낮음
- (c) 외부 caller 헬스체크 — 가능성 낮음

### 후속

usage tracker 의 NodesTool 호출 카운트. (a) 라면 RF-055 + RF-040 일괄 정리.

### 관련

- [RF-055](../refactor.md#rf-055) — 3 node 제거
- [RF-040](../refactor.md#rf-040) — NodesTool 외부화/통합

---

## Q-013 — `claude-code-cli` provider 의 실제 사용 빈도

- **Status**: open
- **Location**: `internal/llm/claude_code_cli.go` (296), `internal/llm/provider.go:143-146`
- **Discovered in**: [journal/2026-04-25-19-llm-provider-1.md](../journal/2026-04-25-19-llm-provider-1.md)

### 질문

`claude-code-cli` provider 는 사용자 환경의 `claude` CLI 를 subprocess 로 호출하는 wrapper. 296줄.

가설:
- (a) **드물게 사용**: Claude Code CLI 가 사용자별 별도 도구 — TARS 와 함께 설치한 사용자만 사용. anthropic API 키 있는 사용자는 직접 anthropic 사용이 더 직접적.
- (b) **특정 워크플로 핵심**: Claude Code CLI 의 자체 file/bash tool 활용을 위해 선호. CLAUDE.md *"claude-code-cli"* 가 ProviderKind 명시하므로 의도적.

추가:
- Tool calling 무시 (CLI 자체 tool 사용) [RF-047]
- Streaming 지원하지만 conversation history 를 ROLE: text 형식 string 으로 직렬화 (CLI 특성상)
- session-persistence 비활성 (`--no-session-persistence`) — 매 호출이 stateless

### 후속 행동

- usage tracker 의 provider 별 카운트 (anthropic vs claude-code-cli) 1주/1개월
- (a) 라면 외부 plugin 또는 skill+CLI 로 외부화 후보 (CLAUDE.md *"Default pattern: skill+CLI"* 정렬)
- (b) 라면 RF-047 의 capability 매트릭스 명시 우선

### 관련

- [RF-047](../refactor.md#rf-047) — capability 매트릭스 문서화

---

## Q-012 — `subagents_plan` / `subagents_orchestrate` 의 실제 호출 빈도

- **Status**: open
- **Location**: `internal/tool/tool_subagents_plan.go` (629), `tool_subagents_orchestrate.go` (539)
- **Discovered in**: [journal/2026-04-25-17-tool-subagents.md](../journal/2026-04-25-17-tool-subagents.md)

### 질문

3 subagent 빌트인 툴 (`run` + `plan` + `orchestrate`) 의 호출 분포는?

가설:
- (a) **`run` 위주**: chat LLM 이 *"여러 작업 병렬로 진행해줘"* → subagents_run 만 직접 호출. plan/orchestrate 는 거의 사용 안 됨 → ~1170줄 dead 가까움
- (b) **`plan→orchestrate` 위주**: chat LLM 이 *"계획 짜고 실행해줘"* → plan 이 LLM-as-planner 로 flow 생성 후 orchestrate 가 실행
- (c) **혼합**: 단순 fan-out 은 `run`, 복잡한 dependency-aware 워크플로 는 plan→orchestrate

### 후속 행동

- usage tracker 의 tool name 별 카운트 (`subagents_run` vs `subagents_plan` vs `subagents_orchestrate`) 1주/1개월 분포
- (a) 라면 plan/orchestrate 외부화 또는 [RF-041](../refactor.md#rf-041) (통합) 으로 단순화
- (b) 라면 RF-042 (normalization 단순화) 우선
- (c) 라면 현 상태 유지

### 관련

- [RF-041](../refactor.md#rf-041) — 3 툴 통합
- [RF-042](../refactor.md#rf-042) — plan normalization layer 단순화

---

## Q-011 — `process` 빌트인 툴의 실제 호출 빈도

- **Status**: open
- **Location**: `internal/tool/process.go` (process 빌트인 툴), `internal/tool/process_manager.go`
- **Discovered in**: [journal/2026-04-25-14-tool-exec-web.md](../journal/2026-04-25-14-tool-exec-web.md)

### 질문

`process` 빌트인 툴은 ProcessManager 위에 7 action aggregator (list/poll/log/write/kill/clear/remove) 노출. 사용 빈도가 얼마나 되나?

가설:
- (a) **드물게 사용**: LLM이 long-running process 시작 후 polling 패턴이 흔하지 않음. 콘솔/CLI/외부 모니터링이 충분.
- (b) **일부 워크플로의 핵심**: dev server / 빌드 watcher 등이 자주 사용.

추가로 RF-035 (timeout 30초 cap) 와 결합 평가 필요 — 30초 cap 때문에 process 가 *진짜 long-running* 못 함 → 사실상 exec background 변종 수준 → 사용 빈도 낮을 가능성.

### 후속 행동

- usage tracker가 tool name별 카운트를 갖고 있는지 확인
- 1주/1개월 로그에서 `process` 호출 비율
- 0건이면 [ID-003](../ideas.md#id-003) 의 외부화 후보 (skill+CLI) 로 즉시 이동
- 빈도 있으면 RF-035 (timeout 확장) 우선

---

## Q-009 — `internal/memory/semantic.go` 의 사용처 불명 코드들

- **Status**: **resolved** (2026-04-25 session 10)
- **Location**: `internal/memory/semantic.go`
  - `indexState` + `loadIndexState`/`saveIndexState` (L182-192, 498-522)
  - `readDoc` (L532-546)
  - `firstMeaningfulParagraph` (L688-696)
  - 자체 `min` 함수 (L703-708)
- **Discovered in**: [journal/2026-04-25-09-memory-pt1.md](../journal/2026-04-25-09-memory-pt1.md)

### 답

전체 `internal/` 트리 + `cmd/` + 테스트 파일 grep 결과:

- `indexState` / `loadIndexState` / `saveIndexState` — `semantic.go` 외 호출 **0건**
- `readDoc` — 호출 **0건**
- `firstMeaningfulParagraph` — 호출 **0건**
- 자체 `min` 함수 — `semantic.go` 안에서도 호출 **0건** (Go 1.25.6 기준 built-in `min` 사용)

`go.mod`: `go 1.25.6` → 자체 `min` 도 redundant.

가설 (c) **dead code 확정**.

→ [RF-019](../refactor.md#rf-019)로 승격 (4개 일괄 제거, ~70줄 삭제).

---

<!--
템플릿:

## Q-001 — <한 줄 질문>

- **Status**: open | resolved | deferred
- **Location**: `path/to/file.go:LL-LL` (또는 패키지)
- **Discovered in**: [journal/YYYY-MM-DD-NN-topic.md](../journal/YYYY-MM-DD-NN-topic.md)

### 질문
...

### (답이 나오면) 답
...
-->
