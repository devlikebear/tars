자율 AI 에이전트 시스템 - 기술 리서치 및 개발 계획서 (v2)

작성일: 2026-02-11 | OpenClaw 영감 기반 24/7 자율 AI 에이전트 시스템
변경사항: Golang 기반, OpenClaw 3-Layer 메모리, SQLite+sqlite-vec, Standalone/Service 이중 모드


1. 프로젝트 개요
1.1 비전
OpenClaw에서 영감을 받아, 24/7/365 자율 운영되는 AI 에이전트 시스템을 구축한다.
메인데몬이 LLM과 통신하며 자율적으로 판단하고, 감시&제어데몬이 안정성과 보안을 담보하며,
다양한 클라이언트(CLI, Web, Slack, Telegram 등)를 통해 사용자와 소통하는 구조이다.
1.2 핵심 설계 원칙
원칙설명AI First모든 의사결정의 중심에 LLM이 위치자율성허트비트와 크론을 통한 자율적 업무 판단안정성감시데몬에 의한 자동 복구 및 보안 감사확장성클라이언트와 서브에이전트의 유연한 확장효율성컨텍스트 절약 및 메모리 지속 관리이중 배포개인 PC 단독 실행 + 멀티테넌트 서비스형 동시 지원
1.3 배포 모드
모드대상DB특징Standalone개인 사용자 PCSQLite (로컬 파일)단일 바이너리, 제로 의존성, 로컬 완결Service기업/SaaSSQLite per-tenant 또는 PostgreSQL멀티유저, 멀티테넌트, 중앙 관리

2. 기술 리서치 결과
2.1 Golang을 선택하는 이유
Python 대비 Go의 장점 (24/7 데몬 시스템)
항목GoPython배포단일 바이너리, 의존성 제로venv, pip, 수십 개 패키지메모리 사용~10-30MB (데몬 기준)~100-300MB (인터프리터+패키지)시작 시간밀리초 단위수초 (import 체인)동시성고루틴+채널 (수천 개 경량)asyncio 또는 threading (GIL 제한)크로스 컴파일GOOS=linux go build 한 줄Docker 또는 PyInstaller 필요타입 안전성컴파일 타임 검증런타임 에러장기 운영 안정성GC 효율적, 메모리 누수 적음GC 부담, 메모리 단편화 가능
Go의 동시성 모델이 에이전트에 적합한 이유
Go의 고루틴+채널 모델은 멀티에이전트 오케스트레이션에 자연스럽게 매핑된다. 각 서브에이전트가 독립 고루틴으로 실행되고, 채널을 통해 메인 데몬과 안전하게 통신하며, select 문으로 여러 에이전트의 결과를 동시에 대기할 수 있다. "메모리를 공유하지 말고 통신으로 공유하라(share memory by communicating)"는 Go의 철학이 에이전트 아키텍처와 완벽히 부합한다.
Go AI 에이전트 생태계 현황 (2025-2026)
공식 LLM SDK:

anthropic-sdk-go: Anthropic 공식 Go SDK (Go 1.22+)
openai-go: OpenAI 공식 Go SDK (Go 1.22+)

멀티프로바이더 게이트웨이:

Bifrost (github.com/maximhq/bifrost): Go로 작성된 초고속 LLM 게이트웨이. LiteLLM 대비 50배 빠르며 <100µs 오버헤드. 1000+ 모델 지원 (OpenAI, Anthropic, Bedrock, Vertex, Ollama 등). MCP 지원 포함.

에이전트 프레임워크:

LangChainGo (tmc/langchaingo): Python LangChain의 Go 포트. 에이전트 실행기, 체인, RAG 지원
Eino (cloudwego/eino): ByteDance의 Go AI 프레임워크. ReAct 루프, 도구 호출 내장
Google ADK for Go: Google 공식 에이전트 개발 키트
Uno (curaious/uno): 내구성 있는 에이전트 루프 + LLM 게이트웨이

2.2 OpenClaw 3-Layer 메모리 아키텍처
OpenClaw이 채택한 3계층 메모리는 **마크다운 파일이 곧 진실의 원천(source of truth)**이라는 설계 철학에 기반한다.
Layer 1: Daily Logs (단기 세션 메모리)
위치: workspace/memory/YYYY-MM-DD.md
방식: Append-only (수정 불가, 추가만 가능)
로딩: 세션 시작 시 오늘 + 어제 파일 로드
용도: 당일 메모, 결정 기록, 진행 중인 컨텍스트
색인: BM25(키워드) + sqlite-vec(시맨틱) 하이브리드 검색
에이전트가 추론 후 중요한 정보를 일자별 로그에 기록한다. 덮어쓰기 없이 추가만 가능하여 시간 순서가 자연스럽게 보존된다. Git으로 추적하면 변경 이력도 완전히 관리된다.
Layer 2: MEMORY.md (큐레이션된 장기 메모리)
위치: workspace/MEMORY.md
방식: 읽기/쓰기 (주로 사적 세션에서만 접근)
로딩: 매 세션의 시스템 프롬프트에 주입
용도: 지속적 사실, 사용자 선호, 에이전트 역할 정의, 핵심 정책
관리: 의도적으로 작은 크기 유지 (효율적 컨텍스트 주입)
장기적으로 유지해야 할 핵심 사실만 큐레이션하여 보관한다. "커피를 좋아한다"처럼 반복 확인된 선호는 Daily Log에서 MEMORY.md로 승격된다. 크기를 작게 유지하는 것이 핵심이다.
Layer 3: Shared Knowledge (공유 지식 그래프)
위치: workspace/_shared/ (symlink로 각 에이전트에 연결)
  ├── user-profile.md    # 사용자 프로필
  ├── agent-roster.md    # 에이전트 목록
  └── infrastructure.md  # 인프라 정보
백엔드: SQLite + sqlite-vec (BM25 + 벡터 하이브리드 검색)
범위: 멀티에이전트 간 지식 공유, 시간적 팩트 검색
여러 에이전트가 공유하는 지식 기반이다. 프라이버시 경계를 유지하면서도 필요한 정보를 에이전트 간에 전달한다.
계층 간 통합: Pre-Compaction Flush
세션 컨텍스트 한계 접근
  ↓
토큰 카운트가 임계값 돌파:
  contextWindow - reserveTokensFloor - softThresholdTokens
  ↓
[사일런트 메모리 플러시 트리거]
  (사용자에게 보이지 않는 내부 턴)
  ↓
시스템 프롬프트: "세션이 압축 직전입니다. 지속 가능한 메모리를 지금 저장하세요."
  ↓
모델 판단:
  → Daily Log에 당일 메모 기록
  → MEMORY.md에 새 장기 사실 추가
  → 또는 NO_REPLY (저장할 것 없음)
  ↓
[컨텍스트 압축 실행]
  오래된 메시지 잘라내기, 핵심 사실은 이미 마크다운에 안전하게 보관됨
검색 플로우 (하이브리드)
memory_search("프로젝트 마감일")
  ↓
SQLite FTS5 (BM25 키워드 매칭)
  + sqlite-vec (시맨틱 유사도)
  → Reciprocal Rank Fusion으로 결과 합산
  ↓
MEMORY.md (장기 사실) → 높은 순위
memory/2026-02-11.md (최근 업데이트) → 시간적 맥락
_shared/user-profile.md (공유 정보) → 배경 정보
  ↓
상위 N개 결과 반환
2.3 SQLite + sqlite-vec 통합 저장소
왜 SQLite + sqlite-vec인가?
항목SQLite + sqlite-vecPostgreSQL + 별도 벡터DB배포단일 파일, 의존성 제로서버 2개 운영 필요Standalone 적합성완벽 (로컬 파일 하나)과도함멀티테넌트DB 파일 per-tenant스키마 per-tenant벡터 검색BM25 + 시맨틱 하이브리드전문 벡터DB 수준운영 비용$0$15~50/월데이터 프라이버시로컬 완결네트워크 노출백업파일 복사pg_dump + 벡터DB 별도
sqlite-vec 기술 스펙

벡터 타입: Float32, Float16, BFloat16, Int8, UInt8, 1Bit 양자화
거리 메트릭: Euclidean(L2), Cosine, Hamming, Dot Product
검증 차원: 최대 1536-D (OpenAI text-embedding-3 수준)
벡터 수: 100만 벡터까지 실용적 (brute-force)
성능: 100만 1536-D 벡터 검색 ~100-500ms
구현: Pure C, SIMD 가속, 외부 의존성 제로
플랫폼: Linux, macOS, Windows, iOS, Android, WASM

Go에서의 sqlite-vec 사용
go// sqlite-vec-go-bindings 사용
import (
    sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
    "github.com/mattn/go-sqlite3"
)

func init() {
    sqlite_vec.Auto() // 확장 자동 등록
}

// 벡터 검색 쿼리
query := `SELECT chunk_id, file_path, snippet, distance
           FROM vec_memory
           WHERE embedding MATCH ?1 AND k = 10
           ORDER BY distance ASC`
제한사항 및 대응

ANN 인덱싱 미지원: 현재 brute-force만 지원. 10M+ 벡터에서는 느림
대응: Standalone에서는 충분. Service 모드에서 대규모 테넌트는 Qdrant 또는 Weaviate(Go 네이티브)로 선택적 업그레이드

2.4 기타 기술 요소 (v1 리서치 유지)
이전 v1 문서의 허트비트, 크론매니저, 감시데몬, 멀티클라이언트, LLM 비용 최적화, MCP 관련 리서치 결과는 여전히 유효하며 Go 기반으로 동일하게 적용 가능하다.

3. 시스템 아키텍처
3.1 이중 배포 모드 아키텍처
┌──────────────────────────────────────────────────────────────────┐
│                      배포 모드 스위칭                              │
│  ┌─────────────────────────┐  ┌─────────────────────────────┐   │
│  │     Standalone Mode     │  │       Service Mode          │   │
│  │  (개인 PC, 단일 바이너리)  │  │  (서버, 멀티유저/멀티테넌트) │   │
│  │                         │  │                             │   │
│  │  config.yaml:           │  │  config.yaml:               │   │
│  │    mode: standalone     │  │    mode: service            │   │
│  │    db: ./aegis.db       │  │    db: ./tenants/{id}.db    │   │
│  │    tenant: default      │  │    auth: jwt/oauth          │   │
│  │    auth: local          │  │    broker: nats             │   │
│  │    broker: embedded     │  │    metrics: prometheus      │   │
│  └────────────┬────────────┘  └──────────────┬──────────────┘   │
│               │                              │                   │
│               └──────────┬───────────────────┘                   │
│                          │                                       │
│                    공통 코어 엔진                                  │
└──────────────────────────┼───────────────────────────────────────┘
                           │
Standalone 모드 상세
사용자 PC
├── aegis (단일 바이너리, ~20-30MB)
├── aegis.db (SQLite + sqlite-vec, 모든 데이터)
├── workspace/
│   ├── MEMORY.md           ← Layer 2: 장기 메모리
│   ├── memory/
│   │   ├── 2026-02-11.md   ← Layer 1: Daily Log
│   │   └── 2026-02-10.md
│   └── _shared/            ← Layer 3: 공유 지식
│       └── user-profile.md
├── config.yaml
└── logs/

설치: 바이너리 다운로드 → 실행. 끝.
의존성: 없음 (Go 단일 바이너리 + SQLite 내장)
업데이트: 감시데몬이 새 바이너리 다운로드 → 교체 → 재시작
네트워크: LLM API 호출만 필요 (나머지 모두 로컬)

Service 모드 상세
서버/클러스터
├── aegis-service (메인 서비스 바이너리)
├── aegis-sentinel (감시 서비스 바이너리)
├── tenants/
│   ├── tenant-001/
│   │   ├── aegis.db         ← 테넌트별 독립 DB
│   │   └── workspace/       ← 테넌트별 독립 워크스페이스
│   │       ├── MEMORY.md
│   │       ├── memory/
│   │       └── _shared/
│   ├── tenant-002/
│   │   ├── aegis.db
│   │   └── workspace/
│   └── ...
├── shared.db                ← 공유 데이터 (사용자 인증, 과금 등)
├── config.yaml
└── nats-server              ← 메시지 브로커 (외부 또는 임베디드)

테넌트 격리: DB-per-tenant (SQLite 파일 분리)로 완벽한 데이터 격리
인증: JWT/OAuth2 기반, 테넌트 컨텍스트 미들웨어
스케일링: 수평 확장 시 테넌트 단위 샤딩
모니터링: Prometheus + Grafana 대시보드

3.2 전체 아키텍처 개요
┌─────────────────────────────────────────────────────────────┐
│                    클라이언트 레이어                          │
│  ┌─────┐  ┌──────┐  ┌───────┐  ┌──────────┐  ┌──────────┐ │
│  │ CLI │  │ Web  │  │ Slack │  │ Telegram │  │ Discord  │ │
│  └──┬──┘  └──┬───┘  └──┬────┘  └────┬─────┘  └────┬─────┘ │
│     └────────┴─────────┴────────────┴──────────────┘       │
│                         │ (메시지 버스)                      │
├─────────────────────────┼───────────────────────────────────┤
│               메시지 브로커 (모드별 분기)                     │
│  Standalone: 임베디드 채널    Service: NATS JetStream       │
│              ┌──────────┴──────────┐                        │
│              │  통합 메시지 인터페이스 │                        │
│              └──────────┬──────────┘                        │
├─────────────────────────┼───────────────────────────────────┤
│                    코어 레이어                               │
│                                                             │
│  ┌──────────────────────────────────────────────┐           │
│  │          메인 데몬 (Go 바이너리)               │           │
│  │  ┌────────────┐  ┌────────────┐  ┌─────────┐│           │
│  │  │ 허트비트    │  │ 크론매니저  │  │ 3-Layer ││           │
│  │  │ 매니저     │  │(robfig/cron)│ │ 메모리  ││           │
│  │  └─────┬──────┘  └─────┬──────┘  └────┬────┘│           │
│  │        │               │              │     │           │
│  │  ┌─────┴───────────────┴──────────────┴───┐ │           │
│  │  │         메인 컨텍스트 (최소한 유지)       │ │           │
│  │  │    - 서브에이전트 고루틴 제어 & 취합     │ │           │
│  │  │    - 작업 판단 & 우선순위 결정            │ │           │
│  │  └──────────────────┬─────────────────────┘ │           │
│  │                     │ (채널로 태스크 지시)    │           │
│  └─────────────────────┼───────────────────────┘           │
│                        │                                    │
│  ┌─────────────────────┼───────────────────────┐           │
│  │      서브에이전트 풀 (Go 고루틴 기반)         │           │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐    │           │
│  │  │코딩 Agent│ │리서치    │ │모니터링  │    │           │
│  │  │(goroutine)│ │Agent    │ │Agent    │ ...│           │
│  │  └──────────┘ └──────────┘ └──────────┘    │           │
│  └────────────────────────────────────────────┘           │
│                                                             │
├─────────────────────────────────────────────────────────────┤
│            감시 & 제어 데몬 (별도 Go 바이너리)               │
│  ┌──────────────────────────────────────────────┐           │
│  │  ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐│           │
│  │  │프로세스│ │업데이트│ │감사    │ │자가    ││           │
│  │  │감시    │ │매니저  │ │로깅    │ │치유    ││           │
│  │  └────────┘ └────────┘ └────────┘ └────────┘│           │
│  └──────────────────────────────────────────────┘           │
├─────────────────────────────────────────────────────────────┤
│                    LLM 게이트웨이                            │
│  ┌──────────────────────────────────────────────┐           │
│  │  Bifrost 또는 직접 SDK 호출                    │           │
│  │  ┌────────┐  ┌──────┐  ┌────────┐  ┌──────┐│           │
│  │  │Claude  │  │GPT-4 │  │DeepSeek│  │Ollama││           │
│  │  │(Primary)│ │(Fall.)│ │(Fall.) │  │(Local)││           │
│  │  └────────┘  └──────┘  └────────┘  └──────┘│           │
│  └──────────────────────────────────────────────┘           │
├─────────────────────────────────────────────────────────────┤
│              통합 저장소 (SQLite + sqlite-vec)               │
│  ┌──────────────────────────────────────────────┐           │
│  │  aegis.db (단일 SQLite 파일, 테넌트별)         │           │
│  │  ├── memory_chunks (FTS5 BM25 색인)          │           │
│  │  ├── vec_memory (sqlite-vec 벡터 색인)        │           │
│  │  ├── episodic_events (에피소딕 이벤트)         │           │
│  │  ├── cron_schedules (크론 스케줄)             │           │
│  │  ├── agent_states (에이전트 상태)              │           │
│  │  ├── audit_logs (감사 로그)                   │           │
│  │  └── cost_tracking (비용 추적)                │           │
│  │                                               │           │
│  │  + workspace/ (마크다운 파일 = 진실의 원천)     │           │
│  │    ├── MEMORY.md                              │           │
│  │    ├── memory/*.md                            │           │
│  │    └── _shared/*.md                           │           │
│  └──────────────────────────────────────────────┘           │
└─────────────────────────────────────────────────────────────┘
3.3 메인 데몬 상세 설계
허트비트 매니저 (Go 구현)
go// 의사 코드
func (h *HeartbeatManager) Run(ctx context.Context) {
    ticker := time.NewTicker(h.config.Interval) // 기본 30분
    for {
        select {
        case <-ticker.C:
            h.executeHeartbeat(ctx)
        case <-ctx.Done():
            return
        }
    }
}

func (h *HeartbeatManager) executeHeartbeat(ctx context.Context) {
    // 1. HEARTBEAT.md 읽기
    heartbeatContent := h.workspace.ReadFile("HEARTBEAT.md")

    // 2. 3-Layer 메모리에서 현재 상태 로드
    memoryMd := h.workspace.ReadFile("MEMORY.md")
    todayLog := h.workspace.ReadFile("memory/" + today() + ".md")
    recentSearch := h.memory.Search("현재 진행 중인 작업")

    // 3. 컨텍스트 조립 (최소한으로)
    prompt := h.buildHeartbeatPrompt(heartbeatContent, memoryMd, todayLog, recentSearch)

    // 4. LLM 판단 요청
    response := h.llm.Complete(prompt)

    // 5. 판단 결과 처리
    if response.HasTasks() {
        for _, task := range response.Tasks {
            h.dispatcher.Dispatch(ctx, task) // 서브에이전트에 위임
        }
    }

    // 6. 메모리 업데이트
    h.workspace.AppendFile("memory/"+today()+".md", response.Notes)
}
크론 매니저 (robfig/cron 기반)
goimport "github.com/robfig/cron/v3"

func (cm *CronManager) Setup() {
    cm.cron = cron.New(cron.WithSeconds())

    // 등록된 스케줄 로드
    schedules := cm.db.LoadSchedules()
    for _, s := range schedules {
        cm.cron.AddFunc(s.Expression, func() {
            cm.executeWithAIJudgment(s)
        })
    }
    cm.cron.Start()
}

func (cm *CronManager) executeWithAIJudgment(schedule Schedule) {
    // AI에게 실행 판단 요청
    judgment := cm.llm.Complete(
        "이 작업을 지금 실행해야 하는가? 조건 변경이 있는가?",
        schedule.Description,
        cm.memory.GetCurrentState(),
    )

    if judgment.ShouldExecute {
        cm.dispatcher.Dispatch(context.Background(), judgment.Task)
    } else {
        cm.logPostponement(schedule, judgment.Reason)
    }
}
서브에이전트 오케스트레이션 (고루틴 기반)
gofunc (d *Dispatcher) Dispatch(ctx context.Context, task Task) {
    // 동시 실행 제한 체크
    d.semaphore <- struct{}{} // 동시 실행 수 제어

    go func() {
        defer func() { <-d.semaphore }()

        agent := d.pool.Acquire(task.Type)
        defer d.pool.Release(agent)

        // 독립 컨텍스트에서 실행
        resultCh := make(chan AgentResult, 1)
        go agent.Execute(ctx, task, resultCh)

        // 타임아웃과 결과 동시 대기
        select {
        case result := <-resultCh:
            d.handleResult(task, result)
        case <-time.After(task.Timeout):
            agent.Cancel()
            d.handleTimeout(task)
        case <-ctx.Done():
            agent.Cancel()
        }
    }()
}
3.4 이중 모드 DB 추상화
go// 인터페이스로 추상화하여 모드별 구현 전환
type Store interface {
    // 메모리 검색
    SearchMemory(query string, topK int) ([]MemoryChunk, error)
    // 벡터 저장/검색
    StoreEmbedding(chunkID string, embedding []float32) error
    SearchSimilar(embedding []float32, topK int) ([]VectorResult, error)
    // 에피소딕 이벤트
    LogEvent(event EpisodicEvent) error
    // 감사 로그
    WriteAuditLog(entry AuditEntry) error
}

// Standalone: 단일 SQLite 파일
type StandaloneStore struct {
    db *sql.DB // aegis.db
}

// Service: 테넌트별 SQLite 파일 + 공유 DB
type ServiceStore struct {
    tenantDB *sql.DB // tenants/{id}/aegis.db
    sharedDB *sql.DB // shared.db (인증, 과금)
}

// 모드별 팩토리
func NewStore(config Config) Store {
    switch config.Mode {
    case "standalone":
        return NewStandaloneStore(config.DBPath)
    case "service":
        return NewServiceStore(config.TenantDBDir, config.SharedDBPath)
    }
}
3.5 감시 & 제어 데몬 (별도 바이너리)
감시데몬은 메인데몬과 완전히 분리된 Go 바이너리로, 최소 의존성과 극도의 안정성을 목표로 한다.
aegis-sentinel (별도 바이너리, ~5-10MB)
├── LLM 의존성 없음 (규칙 기반 판단만)
├── 외부 의존성 없음 (SQLite만 사용)
└── 역할:
    ├── 메인데몬 프로세스 감시 (PID + 허트비트 응답)
    ├── 메인데몬 자동 재시작 (3회 시도 후 안전 모드)
    ├── 바이너리 업데이트 (체크섬 검증 + 원자적 교체)
    ├── 업데이트 후 헬스체크 (실패 시 즉시 롤백)
    ├── LLM 통신 감사 로그 모니터링
    ├── 호스트 PC 실행 명령 감사
    ├── 불법 행위 패턴 탐지 → 차단/강제종료
    └── 사용자 레포트 생성 (이메일/메시지)

4. 기술 스택
4.1 코어 기술
계층기술선택 이유언어Go 1.22+단일 바이너리, 고루틴 동시성, 메모리 효율동시성goroutine + channel수천 개 경량 에이전트, 자연스러운 오케스트레이션프로세스 관리systemd (Linux) / 자체 관리 (macOS/Windows)OS 네이티브 통합메시지 브로커임베디드 채널 (Standalone) / NATS JetStream (Service)모드별 최적화크론 스케줄러robfig/cron/v3Go 표준 크론 라이브러리LLM 클라이언트anthropic-sdk-go + openai-go (직접) 또는 Bifrost (게이트웨이)공식 SDK + 50x 빠른 게이트웨이
4.2 저장소 (통합 SQLite)
용도기술비고모든 구조화 데이터SQLite (modernc.org/sqlite)Pure Go, CGO 불필요, 크로스 컴파일벡터 검색sqlite-vec (Go 바인딩)하이브리드 BM25+벡터 검색전문 검색SQLite FTS5BM25 키워드 매칭 내장3-Layer 메모리 원본마크다운 파일 (workspace/)진실의 원천, Git 추적 가능Service 모드 공유shared.db (SQLite)인증, 과금, 테넌트 관리
4.3 클라이언트 인터페이스
클라이언트기술우선순위CLIspf13/cobra + ViperPhase 1Web APIGin (Standalone) / Echo (Service)Phase 2Web FrontendReact/Next.js (별도 빌드)Phase 2Slackgo-chat-bot/bot 또는 직접 구현Phase 3Telegramgo-telegram/botPhase 3Discordbwmarrin/discordgoPhase 3
4.4 관측성 & 품질
용도기술비고로깅zerolog (Standalone) / zap (Service)경량 vs 고성능메트릭Prometheus (Service)내장 HTTP 엔드포인트테스트testify + mockery표준 Go 테스트 도구ORMGORM 또는 sqlc복잡한 쿼리=GORM, 성능=sqlc
4.5 MCP (Model Context Protocol)

Go MCP SDK를 사용하여 MCP 서버 구현
모든 외부 도구 연동을 MCP 서버로 표준화
서브에이전트가 MCP를 통해 도구에 안전하게 접근


5. 개발 로드맵
Phase 0: 기반 구축 (2주)
목표: Go 프로젝트 스켈레톤 및 개발 환경

 Go 모노레포 구조 생성 (go.work 멀티모듈)
 개발 환경 설정 (Makefile, golangci-lint, pre-commit)
 CI/CD 파이프라인 (GitHub Actions, 크로스 빌드)
 설정 관리 시스템 (Viper: YAML + 환경변수 + 플래그)
 로깅 프레임워크 (zerolog)
 SQLite + sqlite-vec 기본 통합 및 테스트
 이중 모드(Standalone/Service) 설정 구조 설계

Phase 1: 메인 데몬 MVP (4주)
목표: Standalone 모드에서 최소한의 자율 동작 데몬

 LLM 클라이언트 통합 (anthropic-sdk-go + openai-go)
 3-Layer 메모리 매니저 구현

 Layer 1: Daily Log (append-only 마크다운)
 Layer 2: MEMORY.md (큐레이션된 장기 메모리)
 Layer 3: 공유 지식 (_shared/ 디렉토리)


 하이브리드 검색 (FTS5 BM25 + sqlite-vec 벡터)
 Pre-Compaction Flush 메커니즘
 기본 허트비트 매니저 (HEARTBEAT.md 기반)
 CLI 클라이언트 (cobra 기반)
 단일 서브에이전트 (고루틴) 실행 & 결과 수집

마일스톤: ./aegis로 실행하면 허트비트로 깨어나 자율적으로 판단하고, 서브에이전트에 작업을 위임하며, 3-Layer 메모리에 결과를 저장하는 단일 바이너리 데몬
Phase 2: 감시 데몬 & 안정성 (3주)
목표: aegis-sentinel 바이너리 완성

 감시 데몬 구현 (별도 Go 바이너리)

 메인데몬 프로세스 모니터링
 자동 재시작 & 복구 (3회 시도 → 안전 모드)


 감사 로그 시스템 (LLM 통신 전수 기록)
 불법 행위 패턴 탐지 규칙 엔진
 스냅샷 기반 상태 백업/복원
 바이너리 업데이트 매니저 (체크섬 + 원자적 교체 + 롤백)
 systemd 서비스 파일 생성

마일스톤: ./aegis-sentinel이 메인데몬을 감시하고, 죽으면 살리고, 의심 활동을 탐지하며, 업데이트를 관리하는 상태
Phase 3: 크론 매니저 & 멀티에이전트 (3주)
목표: 주기적 자율 업무 수행 + 복수 에이전트

 크론 매니저 (robfig/cron + AI 판단)
 멀티 서브에이전트 동시 실행 (고루틴 풀)
 서브에이전트 이상 감지 & 강제 종료
 서브에이전트 간 채널 기반 통신
 MCP 서버 기본 통합 (파일시스템, 웹검색)
 임베딩 파이프라인 (워크스페이스 파일 자동 색인)
 비용 추적기 (토큰 사용량 + 비용 집계)

마일스톤: 크론 스케줄에 따라 AI가 자율적으로 판단하여 여러 고루틴 서브에이전트에 작업을 분배하고, 결과를 수집하여 3-Layer 메모리에 통합하는 상태
Phase 4: Service 모드 & 멀티 클라이언트 (4주)
목표: 멀티테넌트 서비스 + 다채널 클라이언트

 Service 모드 구현

 테넌트별 DB 파일 격리
 JWT/OAuth2 인증 미들웨어
 테넌트 컨텍스트 자동 주입
 테넌트 관리 API


 NATS JetStream 메시지 브로커 통합
 Web API (Echo 프레임워크)
 Web Frontend (대시보드 + 실시간 상태)
 Slack 봇 클라이언트
 Telegram 봇 클라이언트
 채널 간 컨텍스트 공유

마일스톤: Standalone과 Service 모드를 설정 하나로 전환할 수 있고, CLI/Web/Slack/Telegram에서 동일한 에이전트에 접근 가능한 상태
Phase 5: 고도화 & 프로덕션 (4주)
목표: 프로덕션 배포 가능 수준

 LLM 비용 최적화 (동적 모델 라우팅, 프롬프트 캐싱, 배치)
 Bifrost 게이트웨이 통합 (멀티프로바이더 부하 분산)
 Bifrost를 LLM 프록시 계층으로 표준화
 단일 API 스키마로 프로바이더별 차이를 흡수하여 API 일관성 확보
 모델별 TPS/QPS 제한, 큐잉, 백프레셔 정책으로 쓰루풋 제어
 요청/응답 단위 토큰 및 비용 메트릭 수집으로 비용 측정 고도화
 장애 시 자동 폴백/재시도/서킷브레이커 적용
 자가 치유 고도화 (에러 원인 분석, 자동 패치)
 Prometheus 메트릭 + Grafana 대시보드 (Service 모드)
 보안 강화 (샌드박싱, 최소 권한)
 대규모 테넌트용 벡터DB 업그레이드 옵션 (Weaviate)
 크로스 플랫폼 빌드 (Linux/macOS/Windows ARM64/AMD64)
 설치 스크립트 & 문서화
 부하 테스트 & 스트레스 테스트

마일스톤: 개인 PC에서 ./aegis로 즉시 실행되거나, 서버에서 멀티테넌트 SaaS로 운영되며, 24/7 안정적으로 자율 동작하는 프로덕션 시스템

Phase 6: 엔터프라이즈 클러스터 확장 (선택, 향후 계획)
목표: 엔터프라이즈 규모 고가용성/확장성 아키텍처 준비
범위: 현재 개발 범위에서 제외 (Future Roadmap)

 노드 간 통신 계층 (gRPC + NATS JetStream) 설계
 공유 메모리/공유 상태 계층 설계 (Redis 또는 NATS KV 기반)
 클러스터 Pub/Sub 토폴로지 및 이벤트 스키마 정의
 백엔드 LLM 라우팅 로드밸런서 (가중치 기반 + 헬스체크) 설계
 사용자 세션 부하 분산 (L7 LB + 스티키 세션 또는 외부 세션 스토어) 설계
 크론잡 분산 실행 (리더 선출 + 분산 락 + 중복 실행 방지) 설계
 멀티노드 장애 복구 시나리오 및 SLO/SLI 정의

마일스톤: 단일 노드 구현을 유지한 상태에서, 클러스터 전환 시 필요한 통신/상태/분산 실행 설계 문서와 PoC 체크리스트가 준비된 상태

6. 리스크 분석 및 대응 전략
6.1 기술 리스크
리스크영향확률대응 전략Go AI 생태계 미성숙중간중간공식 SDK 존재 확인. LangChainGo/Eino 등 프레임워크 성장 중. 필요시 직접 구현sqlite-vec 성능 한계중간낮음100만 벡터까지 충분. Service 모드 대규모는 Weaviate 폴백LLM API 장애높음중간Bifrost 멀티프로바이더 + Ollama 로컬 폴백컨텍스트 오염/드리프트높음높음Pre-Compaction Flush + 3-Layer 메모리 복원비용 폭발높음중간일일 예산 한도 + 동적 모델 라우팅서브에이전트 무한루프중간중간고루틴 타임아웃 + context.WithTimeout + 감시데몬멀티테넌트 데이터 누수높음낮음DB-per-tenant 물리적 격리 + 미들웨어 검증
6.2 Go 전환 관련 리스크
리스크대응Python AI 라이브러리 부재공식 LLM SDK 사용, 필요시 HTTP 직접 호출로 대체Go sqlite-vec 바인딩 미성숙CGO 옵션과 WASM 옵션 모두 존재. 최악의 경우 C FFI 직접 호출Go 개발자 AI 경험 부족LangChainGo 문서 + Google ADK 참조크로스 컴파일 시 CGO 문제modernc.org/sqlite(Pure Go) 사용으로 CGO 회피 가능
6.3 운영/품질 리스크 (v1과 동일)
리스크대응AI의 잘못된 자율 판단중요도 기반 사용자 확인 정책24/7 운영 비용수면 모드 + 허트비트 주기 조정LLM 환각결과 검증 파이프라인 + 팩트체크 에이전트메모리 부정확마크다운 파일 Git 추적 + 주기적 정합성 검사

7. 비용 추정 (월간)
7.1 LLM API 비용 (최적화 적용 시)
시나리오허트비트 주기일일 크론사용자 상호작용예상 월 비용Standalone 경량60분5건20건/일$50~100Standalone 표준30분15건50건/일$150~300Service (테넌트당)30분20건80건/일$200~400
7.2 인프라 비용
항목StandaloneService (10 테넌트)호스팅$0 (개인 PC)$20~50/월 (VPS)SQLite$0$0NATS해당없음$0 (자체 호스팅)모니터링해당없음$0 (Prometheus+Grafana)합계$0$20~50/월

8. 프로젝트 디렉토리 구조
aegis/                             # 프로젝트 루트
├── go.work                        # Go 워크스페이스 (멀티모듈)
├── go.work.sum
├── Makefile                       # 빌드/테스트/린트 자동화
├── docker-compose.yml             # Service 모드 개발환경
├── .golangci.yml                  # 린터 설정
│
├── cmd/                           # 실행 바이너리 엔트리포인트
│   ├── aegis/                     # 메인 데몬
│   │   └── main.go
│   ├── aegis-sentinel/            # 감시 데몬
│   │   └── main.go
│   └── aegis-cli/                 # CLI 클라이언트
│       └── main.go
│
├── internal/                      # 비공개 패키지 (Go 컨벤션)
│   ├── core/                      # 코어 엔진
│   │   ├── daemon.go              # 데몬 메인 루프
│   │   ├── heartbeat/
│   │   │   ├── manager.go         # 허트비트 매니저
│   │   │   └── manager_test.go
│   │   ├── cron/
│   │   │   ├── manager.go         # 크론 매니저
│   │   │   └── manager_test.go
│   │   └── context/
│   │       └── budget.go          # 컨텍스트 예산 관리
│   │
│   ├── memory/                    # 3-Layer 메모리 시스템
│   │   ├── manager.go             # 메모리 통합 매니저
│   │   ├── daily_log.go           # Layer 1: Daily Logs
│   │   ├── long_term.go           # Layer 2: MEMORY.md
│   │   ├── shared.go              # Layer 3: 공유 지식
│   │   ├── search.go              # 하이브리드 검색 (BM25+벡터)
│   │   ├── flush.go               # Pre-Compaction Flush
│   │   ├── embedder.go            # 임베딩 파이프라인
│   │   └── memory_test.go
│   │
│   ├── agent/                     # 서브에이전트 오케스트레이션
│   │   ├── orchestrator.go        # 오케스트레이터
│   │   ├── dispatcher.go          # 태스크 디스패처
│   │   ├── pool.go                # 고루틴 풀
│   │   ├── base.go                # 에이전트 인터페이스
│   │   └── types/                 # 에이전트 유형별 구현
│   │       ├── coding.go
│   │       ├── research.go
│   │       └── monitor.go
│   │
│   ├── llm/                       # LLM 통신 레이어
│   │   ├── client.go              # 통합 LLM 클라이언트 인터페이스
│   │   ├── anthropic.go           # Claude SDK 래퍼
│   │   ├── openai.go              # OpenAI SDK 래퍼
│   │   ├── ollama.go              # 로컬 모델 래퍼
│   │   ├── router.go              # 동적 모델 라우팅
│   │   └── cost.go                # 비용 추적
│   │
│   ├── store/                     # 저장소 추상화
│   │   ├── store.go               # Store 인터페이스
│   │   ├── sqlite.go              # SQLite + sqlite-vec 구현
│   │   ├── standalone.go          # Standalone 모드 Store
│   │   ├── service.go             # Service 모드 Store (per-tenant)
│   │   └── migrations/            # DB 마이그레이션
│   │       ├── 001_init.sql
│   │       └── 002_vec.sql
│   │
│   ├── sentinel/                  # 감시 데몬 내부
│   │   ├── monitor.go             # 프로세스 모니터링
│   │   ├── updater.go             # 업데이트 매니저
│   │   ├── auditor.go             # 감사 로깅
│   │   ├── healer.go              # 자가 치유
│   │   └── rules/
│   │       └── security.go        # 보안 규칙 엔진
│   │
│   ├── tenant/                    # 멀티테넌트 (Service 모드)
│   │   ├── manager.go             # 테넌트 관리
│   │   ├── middleware.go          # 테넌트 컨텍스트 미들웨어
│   │   └── auth.go                # JWT/OAuth2 인증
│   │
│   └── config/                    # 설정 관리
│       ├── config.go              # Viper 기반 설정
│       └── defaults.go
│
├── pkg/                           # 공개 패키지 (재사용 가능)
│   ├── mcp/                       # MCP 서버/클라이언트
│   │   ├── server.go
│   │   └── tools/
│   │       ├── filesystem.go
│   │       └── websearch.go
│   └── broker/                    # 메시지 브로커 추상화
│       ├── broker.go              # 인터페이스
│       ├── embedded.go            # Standalone: Go 채널 기반
│       └── nats.go                # Service: NATS JetStream
│
├── clients/                       # 클라이언트 구현
│   ├── web/
│   │   ├── api/                   # Echo/Gin HTTP API
│   │   └── frontend/              # React/Next.js
│   ├── slack/
│   │   └── bot.go
│   └── telegram/
│       └── bot.go
│
├── workspace/                     # 기본 워크스페이스 템플릿
│   ├── HEARTBEAT.md
│   ├── MEMORY.md
│   └── _shared/
│       └── user-profile.md
│
├── config/
│   ├── standalone.yaml            # Standalone 기본 설정
│   └── service.yaml               # Service 기본 설정
│
├── scripts/
│   ├── build.sh                   # 크로스 플랫폼 빌드
│   ├── install.sh                 # Standalone 설치
│   └── deploy.sh                  # Service 배포
│
└── tests/
    ├── unit/
    ├── integration/
    └── e2e/

9. 결론 및 다음 단계
결론
현재 기술로 충분히 구현 가능하며, Go는 이 프로젝트에 최적의 선택이다.
핵심 근거:

Go의 단일 바이너리 배포는 Standalone 모드의 "다운로드 → 실행" 경험에 완벽
고루틴+채널은 멀티에이전트 오케스트레이션의 자연스러운 구현
SQLite+sqlite-vec은 로컬 완결형 AI 메모리에 검증된 조합 (OpenClaw이 사용 중)
OpenClaw 3-Layer 메모리는 마크다운 기반으로 투명하고, 편집 가능하며, Git 추적 가능
**이중 모드(Standalone/Service)**는 Go의 인터페이스 추상화와 설정 기반 전환으로 깔끔하게 구현 가능
**공식 LLM SDK(anthropic-sdk-go, openai-go)**와 Bifrost 게이트웨이로 LLM 생태계도 충분

Python 대비 트레이드오프
잃는 것얻는 것풍부한 AI/ML 라이브러리10배 적은 메모리, 밀리초 시작LiteLLM 직접 사용Bifrost (50x 빠름) 또는 직접 SDK빠른 프로토타이핑프로덕션 안정성, 타입 안전Django/FastAPI 생태계Gin/Echo + 단일 바이너리 배포
AI/ML 관련 무거운 작업(모델 파인튜닝, 복잡한 데이터 처리)은 MCP 서버를 Python으로 작성하여 연동하는 하이브리드 전략도 가능하다.
제안하는 다음 단계

프로젝트명 확정 (제안: Aegis) 및 GitHub 저장소 생성
Phase 0 시작: Go 모노레포 스켈레톤 + SQLite+sqlite-vec 통합 테스트
HEARTBEAT.md 포맷 설계: 자율 판단의 핵심 입력
3-Layer 메모리 프로토타입: Daily Log + MEMORY.md + 하이브리드 검색
LLM 클라이언트 프로토타입: anthropic-sdk-go 기반 기본 통신
Bifrost 프록시 PoC: API 일관성, 쓰루풋 제어, 비용 측정 검증
핵심 판단 프롬프트 설계: 허트비트와 크론의 자율 판단 프롬프트
클러스터 확장 RFC 작성 (범위 외): 노드 통신/공유 상태/분산 스케줄링 기준선 정의


이 문서는 2026년 2월 기준 기술 조사 결과를 바탕으로 작성되었습니다.
v2 변경: Golang 기반, OpenClaw 3-Layer 메모리, SQLite+sqlite-vec, Standalone/Service 이중 모드
