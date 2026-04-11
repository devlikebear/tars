# 아키텍처

## 시스템 다이어그램

```
┌─────────────────────────────────────────────────────────────────┐
│                        tars serve (Go 바이너리)                  │
│                                                                  │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────────────┐  │
│  │  Console  │  │ Telegram │  │ Webhooks │  │   CLI (cobra)  │  │
│  │ (Svelte5) │  │  채널    │  │   채널   │  │                │  │
│  └─────┬─────┘  └─────┬────┘  └─────┬────┘  └───────┬────────┘  │
│        │              │              │               │           │
│  ┌─────▼──────────────▼──────────────▼───────────────▼────────┐  │
│  │                   HTTP API (net/http)                       │  │
│  │  /v1/chat  /v1/sessions  /v1/gateway  /v1/cron  /v1/...   │  │
│  └────────────────────────┬───────────────────────────────────┘  │
│                           │                                      │
│  ┌────────────────────────▼───────────────────────────────────┐  │
│  │                    Chat Handler                             │  │
│  │  세션 로드 → 에이전트 루프 → LLM 호출 → 도구 실행 → 저장   │  │
│  └──────┬─────────────────────────────┬───────────────────────┘  │
│         │                             │                          │
│  ┌──────▼──────┐               ┌──────▼──────┐                  │
│  │  LLM Router │               │ Tool Registry│                  │
│  │ heavy/std/  │               │ file, exec,  │                  │
│  │ light 3tier │               │ memory, web, │                  │
│  └──────┬──────┘               │ gateway, mcp │                  │
│         │                      └──────────────┘                  │
│  ┌──────▼──────────────────────────────────────────────────┐    │
│  │  LLM Providers: Anthropic | OpenAI | Gemini | CLI      │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │              백그라운드 런타임                              │   │
│  │  ┌─────────┐  ┌────────────┐  ┌──────┐  ┌───────────┐  │   │
│  │  │  Pulse  │  │ Reflection │  │ Cron │  │  Gateway  │  │   │
│  │  │  (1분)  │  │  (야간)    │  │(30초)│  │ (온디맨드 │  │   │
│  │  └─────────┘  └────────────┘  └──────┘  │  에이전트)│  │   │
│  │                                          └───────────┘  │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │              영속성 (파일 기반, 외부 DB 없음)               │   │
│  │  workspace/sessions/  workspace/memory/  workspace/cron/  │   │
│  │  workspace/_shared/gateway/  workspace/config/            │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## 서피스 격리

컴파일 타임에 두 서피스 간 도구 접근을 격리한다:

| 서피스 | 범위 | LLM 접근 | 도구 접근 |
|--------|------|---------|---------|
| **User** | Chat, Gateway 에이전트 | Router (전 tier) | 전체 도구 레지스트리 (`RegistryScopeUser`) |
| **System** | Pulse, Reflection | Router (light tier 기본) | 자체 도구만; `ops_`/`pulse_`/`reflection_` 접두사는 User 레지스트리에서 panic |

## 3-Tier 모델 라우팅

```
┌────────────┐     ┌──────────┐     ┌───────────────┐
│  Role      │────▶│  Tier    │────▶│  Provider+Model│
│ chat_main  │     │ standard │     │ anthropic/sonnet│
│ pulse_decider│   │ light    │     │ anthropic/haiku │
│ gateway_planner│ │ heavy    │     │ anthropic/opus  │
└────────────┘     └──────────┘     └───────────────┘
```

- **Role**: 코드 상수 (chat_main, pulse_decider 등 8개)
- **Tier**: named bundle (provider + model + reasoning_effort + thinking_budget + service_tier)
- **매핑**: config에서 `llm_role_*: <tier>` 로 지정. 코드는 Role만 참조.
- **서브에이전트**: task 단위로 tier 지정 가능 (task > agent YAML > config default)

## 핵심 데이터 플로우

### 1. 채팅 메시지 처리

```
POST /v1/chat {message, session_id}
  → handleChatRequest()
  → prepareChatRunState()
    → 세션 트랜스크립트 로드 + 히스토리
    → 시스템 프롬프트 빌드 (USER.md + IDENTITY.md + AGENTS.md + 메모리)
    → 도구 레지스트리 구성 (built-in + MCP + skills)
    → 토큰 초과 시 자동 compaction (light tier)
  → executeChatLoop()
    → agent.Loop.Run() 반복:
      → llm.Client.Chat() (메시지 + 도구 스키마)
      → tool_calls 있으면 각 도구 실행 via Registry
      → 도구 결과를 메시지에 추가
      → end_turn 또는 max_iterations까지 반복
  → 어시스턴트 메시지 세션에 저장
  → SSE 스트림: 델타 + 사용량 + 상태 이벤트
```

### 2. 서브에이전트 실행

```
LLM이 subagents_run 도구 호출
  → tool_subagents.go: 에이전트 검증 (prompt-kind + allowlist 필수)
  → tier 해석: task.tier > agent.tier > config default
  → gateway.Runtime.Spawn() (각 task 병렬 goroutine)
    → executor 해석 (agent 이름으로)
    → 격리 세션 생성 (kind=subagent, hidden=true)
    → executor.Execute() → runPrompt(ctx, label, prompt, tools, tier)
      → router.ClientForTier(tier) 로 모델 선택
      → 에이전트 루프 격리 세션에서 실행
  → Wait() 모든 spawned run 완료 대기
  → 결과 집계: {run_id, status, tier, summary (220자)}
  → 압축 JSON으로 부모 LLM에 반환
```

### 3. Pulse 틱 처리

```
pulse.Runtime.loop() 매 1분 기상
  → Scanner.Scan() 시그널 수집:
    → CronJobLister: 연속 실패 횟수
    → GatewayRunLister: 멈춘 run (>60분)
    → DiskStatProvider: 디스크 사용률 %
    → DeliveryFailureCounter: Telegram 전송 실패
    → ReflectionHealthSource: 야간 실행 건강도
  → 시그널 있으면:
    → Decider.Decide(signals) — light tier LLM 사용
      → 시그널 요약 + 정책으로 프롬프트 빌드
      → LLM이 pulse_decide 도구 호출 (필수)
      → 파싱: action(ignore/notify/autofix), severity, title
    → notify: NotifyRouter → SSE 이벤트 + Telegram (선택)
    → autofix: autofix.Registry에서 Go 코드 실행 (LLM 아님)
  → TickOutcome을 pulse 상태에 기록
```

## 패키지 의존성 레이어

```
cmd/tars (CLI 진입)
  └─ internal/tarsserver (HTTP 서버 와이어링)
       ├─ internal/llm (프로바이더 추상화 + Router)
       ├─ internal/gateway (에이전트 실행 런타임)
       ├─ internal/session (채팅 세션 스토어)
       ├─ internal/memory (시맨틱 메모리 + KB)
       ├─ internal/tool (빌트인 도구)
       ├─ internal/pulse (감시자)
       ├─ internal/reflection (야간 배치)
       ├─ internal/cron (스케줄러)
       ├─ internal/mcp (MCP 클라이언트)
       ├─ internal/agent (에이전트 루프)
       ├─ internal/usage (비용 추적)
       ├─ internal/config (YAML 설정)
       ├─ internal/auth (인증 정보 해석)
       ├─ internal/serverauth (HTTP 미들웨어)
       ├─ internal/extensions (스킬/플러그인 생명주기)
       └─ internal/browser (Playwright 자동화)
```

## 설정 모델

```
config/standalone.yaml              ← 체크인된 기본값
workspace/config/tars.config.yaml   ← 로컬 오버라이드 (gitignored)
환경변수                             ← TARS_* 접두사가 YAML 오버라이드
```

우선순위: 환경변수 > 로컬 YAML > 기본 YAML > 하드코딩 defaults

13개 설정 그룹 (Runtime, API, LLM, Memory, Usage, Automation, Assistant, Tool, Vault, Browser, Gateway, Channel, Extension)에 걸쳐 60+ 필드.
