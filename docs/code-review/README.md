# Code Review — TARS 코어 코드베이스

이 디렉토리는 TARS 프로젝트의 **단계별 코드리뷰 기록**과 거기서 파생된 **후속작업 백로그**를 보관합니다.

## 목적

1. **세션 간 연속성** — 다른 날, 다른 Claude Code 세션에서 이어서 진행해도 컨텍스트가 끊기지 않게.
2. **후속작업 인풋** — 리뷰에서 나온 리팩토링 후보·아이디어·tension을 그대로 작업지시서로 변환할 수 있게.

## 디렉토리 구조

```
docs/code-review/
├── README.md           ← 이 파일. 대시보드 + 사용 가이드
├── code-map.md         ← 패키지/파일별 검토 진척표
├── overview.md         ← (마지막에 작성) 종합 분석 + 우선순위 백로그
├── journal/            ← 시간순 리뷰 스토리 (세션별 1파일 이상)
│   └── YYYY-MM-DD-NN-<topic>.md
└── findings/           ← 분류된 발견사항 (참조용)
    ├── questions.md    ← 미해결 질문 (Q-NNN)
    ├── refactor.md     ← 리팩토링 후보 (RF-NNN)
    ├── ideas.md        ← 새 아이디어/기능 (ID-NNN)
    └── tensions.md     ← 가이드라인 vs 현재 코드 충돌 (TN-NNN)
```

## ID 체계

모든 finding은 고유 ID를 가집니다. journal에서 `[TN-001]` 식으로 인라인 링크하면 양방향 추적이 됩니다. 후속 PR 제목에도 ID를 포함하면 추적이 쉬워집니다 (예: `refactor(plugin): TN-001 browserplugin을 skill+CLI로 분리`).

| Prefix | 카테고리   | 파일                  |
|--------|-----------|----------------------|
| Q-     | 질문      | findings/questions.md |
| RF-    | 리팩토링   | findings/refactor.md  |
| ID-    | 아이디어   | findings/ideas.md     |
| TN-    | Tension   | findings/tensions.md  |

## Finding 항목 표준 포맷

각 항목은 아래 4가지를 반드시 포함합니다:

```markdown
### TN-001 — <한 줄 제목>

- **Status**: open | resolved | deferred
- **Location**: `path/to/file.go:LL-LL` (또는 패키지)
- **Discovered in**: [journal/2026-04-25-01-entrypoint.md](../journal/2026-04-25-01-entrypoint.md)
- **Recommendation**: <권장 조치 1~2줄>

본문 — 컨텍스트, 근거, 대안 등.
```

## Journal 엔트리 표준 포맷

```markdown
---
date: 2026-04-25
session: 01
scope: <다룬 파일/패키지 목록>
next: <다음 세션에서 다룰 곳>
findings: [TN-001, Q-001]
---

# <제목>

본문 — 대화형 스토리. 발견 시점에 finding ID 인라인 링크.
```

## 다음 세션 시작 시 컨텍스트 복원 순서

1. **이 README** 의 "현재 상태" 섹션
2. **`code-map.md`** — 어디까지 봤는지
3. **마지막 journal 엔트리**의 `next:` 필드
4. (필요시) `findings/*.md` 의 open 항목들

## 운영 규칙

- 의미 있는 덩어리(파일·패키지 하나, 또는 세션 끝)마다 journal 새 엔트리 추가.
- 발견사항이 나오면 즉시 해당 카테고리 finding 파일에 추가 + journal에서 인라인 링크.
- 사용자가 finding을 "해결됐다 / 덮자 / PR로 분리하자" 결정하면 Status 업데이트.
- 모든 리뷰가 끝나면 `overview.md` 작성 — 우선순위 매겨진 백로그가 핵심 산출물.

---

## 현재 상태 (Live)

- **시작일**: 2026-04-25
- **마지막 세션**: [2026-04-25-28-frontend](journal/2026-04-25-28-frontend.md)
- **검토 종료**: ✅ **`overview.md` 작성 완료** ([overview.md](overview.md))
- **Open findings**: TN: 3 (TN-001 흡수 예정) / Q: 10 open, 4 resolved / **RF: 73** / ID: 5 = **총 91 finding**
- **검토 진척**: ✅ **100% (51/~50)** — backend (llm+gateway+session+config+cron) + frontend 전수 종료
- **무게 큰 결정 백로그**: RF-007(빌트인 플러그인 제거) + RF-008(sh 훅 제거) + RF-009(외부 HTTP 정책) + **ID-001(KB 유지/단순화/확장)** + **ID-002(시스템 프롬프트 헤더 외부화)** + **ID-003(빌트인 툴 통합/분리 정책)** + **ID-004(OpenAI/Gemini provider capability 격차 해소, OpenAI 우선)** + **ID-005(`internal/gateway` → `internal/agentruntime` 패키지명 변경, hard cut)** — 사용자 결정 + 후속 평가 단계
- **사용자 우선순위 (s19 후속)**: OpenAI Codex 부상 → OpenAI 완성도/지원 1순위로 끌어올림. anthropic 수준의 prompt caching / strict tool choice / structured output / multimodal PDF / thinking 격차 해소. ID-004 본문 phase 1-5 참고
- **Provider capability 정량 (s20 완성)**: anthropic 12/13 ⭐ → gemini-native 9/13 → openai-compat 7/13 → **openai-codex 4/13 ⚠️** (사용자 1순위) ≒ gemini-compat 4/13 (deprecate 후보). RF-048 (codex tool_choice hardcoded) + RF-049 (codex advanced fields silent ignore) 가 ID-004 Phase 1 핵심
- **모범 패키지/패턴**:
  - `internal/pulse/`, `internal/reflection/` — system surface 쌍. nil-tolerant Dependencies, narrow interfaces, panic recovery, crash-resistant idempotence
  - `internal/tool/` — `RegistryScope` 의 wiring-time panic 가드 (cross-surface 누수 방지). CanonicalToolName 단일 진입점 + 4-rule policy resolution
  - `internal/tool/workspace_path.go` — PathPolicy 추상화 (read/write 분리 + symlink escape 검증)
  - `internal/tool/web_fetch.go` — SSRF 다층 가드 (scheme/직접 IP/DNS lookup/매 redirect 재검증). 외부 자원 접근 코드의 reference 모델
  - `internal/tool/tool_subagents.go` — subagent triple guard (agent kind + tool blocklist + write_*/edit_* prefix catch-all). read-only 보장
  - `internal/tool/tool_subagents_orchestrate.go` — mock-able function variables (subagentFlowSpawn/Wait/Cancel) — 테스트 패턴 reference + flow validation 5 가드 (depth/threads/dup/dep dag)
  - **wiring-time validation + resilience 가드 11종 누적** ⭐⭐: RegistryScope(s12) + PathPolicy(s13) + web_fetch SSRF(s14) + subagent triple guard(s16) + flow validation(s17) + NewRouter 7-validation(s18) + OpenAI Codex OAuth 자동 갱신(s20) + CommandExecutor env injection 가드(s21) + consensus 7-layer budget guard(s22) + ResolveOverride task override 7-layer 가드(s23) + **ResolveLLMTier 7-validation**(s25) — 코드베이스 전체 *"silent skew 방지"* + *"resilience"* 의식 일관됨. NewRouter + ResolveLLMTier = **14-validation 체인**, CLAUDE.md *"single resolution path, loud errors"* 의 본거지
  - `internal/gateway/` — 5 책임 (lifecycle/executor/channels/consensus/persistence) + Run lifecycle 4단계 분리 + non-blocking SSE publish + atomic snapshot + parseIDSequence (id 충돌 방지) + active run 보존 trim + workspace 격리 (channelKey). **Resilience 3축** (persistence + restore + archive). **TARS 의 dependency hub** — llm.Router + session.Store + usage + serverauth + 외부 callbacks 5개 캡처
  - `internal/session/` — chat 컨텍스트 관리의 4-layer (message/history/compaction/summary 보존) + LoadHistorySnapshot 의 compaction boundary 보존 + stacking carry-forward (이전 summary 영원히 보존) + CJK token 가중치 + path-level mutex (sync.Map). 모범 패턴 다수. 단 **atomic write 미적용** (RF-059) + **O(N) read+write** (RF-058, RF-017/021 카테고리 누적)
  - `internal/config/` — CLAUDE.md *"single resolution path, loud errors"* 의 본거지. LLM 3-layer (provider pool + tier binding + role defaults) — 같은 provider alias 가 여러 tier 매핑 가능. JSON env override + nested expansion (`${KEY}` placeholder 정상). **configInputField DRY 패턴** (180+ field 단일 source). **applyLLMPoolDefaults kind-specific defaults** (사용자 minimum config — `kind: anthropic` 한 줄로 동작). Schema FieldMeta UI 자동 렌더링. Legacy migration warning (`tars doctor` 명령에서 활성). 단 default model name outdated (RF-064) + yaml_paths.go DRY 위반 (RF-065) + provider kind switch 분산 (RF-066)
  - `internal/cron/` — 3-mode schedule (`at:`/`every:`/cron) + TryStartRun mutex (동시 실행 방지) + exponential backoff (30s→12h cap) + delete_after_run for one-shot + PayloadMeta `_tars_cron` 격리 (사용자 payload 와 충돌 방지) + natural language schedule fallback (한국어/영어). **사용자 친화 모범**. 단 RF-067 (load/save O(N), RF-058 카테고리 4 사례 누적) + RF-068 (atomic write 미적용, 3 사례 누적)
- **누적 안티패턴 4 카테고리** (s27 backend 종료 시점 정리): **O(N) read+write** (4 사례: RF-017/021/058/067, SQLite 마이그레이션 큰 결정 후보) / **atomic write 미적용** (3 사례: session/knowledge/cron, 공통 `internal/atomicwrite/` 추출 권장) / **자체 min/max 함수** (5 사례: RF-019, Go 1.25 built-in 사용) / **YAGNI stub** (4 사례: RF-044/051/052, 일괄 정리)
  - `internal/llm/` core abstraction — Tier/Role/Router 의 architectural clarity. *"config-only role-tier reassignment"* 약속 + boot-time loud failure. CLAUDE.md 약속 거의 완벽 정렬
  - `internal/llm/anthropic.go` — 자동 prompt caching (system + tools 끝 ephemeral) + ToolChoice specific tool 강제 지원 (RF-042 의 anthropic 환경 즉시 가능 인풋)
  - `internal/llm/gemini_native.go` — Thinking 양차원 (Level + Budget) + PDF native (inlineData) + preflight check. **anthropic 다음 풍부** (9/13 vs anthropic 12/13). responseSchema 1개만 추가하면 동급
- **인사이트**:
  - `internal/memory/` 의 통합 `MemoryEntry` + 하이브리드 scoring — 검색 시스템 설계 모범. KB(knowledge.go)는 *write 경로는 야심차지만 read/그래프 시각화/chat 통합이 미완* → ID-001
  - `internal/prompt/` 의 명시적 토큰 예산 — 좋은 패턴. fallback chain은 semantic 비활성 시 hot path (RF-025). 부트스트랩 메타가 prompt + sysprompt + memory 3곳에 분산 (RF-024)
  - **KB는 chat prompt path에 0% 통합** (s11+s15 확정) — fallback chain 6개 매처에 KnowledgeNote 없음 + KB가 semantic 인덱스에 등록 안 됨 + memory_search 의 include_knowledge 기본 false. ID-001 가설 A("read side 미완성") **확정**
  - **memory 표면 비대칭** (s15) — KB는 read 닫힘 + write 열림 (knowledge.upsert) / MEMORY.md는 read 가능 + write 자동 열림 (memory_save 의 AppendMemoryNote, RF-036). 사용자 영역 vs LLM 영역 경계 흐림
  - **빌트인 툴 인벤토리**: default 등록 ~11개 + cfg-flag optional 6개. 매 chat turn 시스템 프롬프트 베이스 비용 ~750-900 토큰 (default), 1500-2000 토큰 (full). aggregator 6개(memory/knowledge/workspace/automation/session/subagents) + 분리 8+개 혼재. 통합 정책 부재 → ID-003
  - **`process` aggregator 패턴이 schema 모범** (s15) — 모든 sub-param top-level + action enum + additionalProperties=false. memory/knowledge/workspace 의 partial schema (additionalProperties=true) 보다 LLM 정확도 ↑. file/web 통합 시 따라야 할 패턴
  - **ID-003 통합 후보 4 누적** (s13~s17): web (★★★) + file (★★) + subagents (★★, plan 출력=orchestrate 입력) + gateway-message-nodes (★, 외부화 우선). 통합 시 시스템 프롬프트 절감 합산 ~-630 토큰/turn → 100 turn/day 가정 시 63k 토큰/일. ID-003 결정의 정량 임팩트 큼
  - **alias 표면 누적** (RF-029/031/039/038) — codebase-wide alias 정책 필요할 정도 (read_file 6 param + exec command/cmd + cron 4쌍 + memory/knowledge/workspace partial schema)
- **새 분석 관점 (2개)**:
  - 2026-04-25 session 10부터 **"이 기능은 꼭 필요한가/정말 유용한가"** — ID-001(s10), ID-002(s11)
  - 2026-04-25 session 13부터 **"빌트인 툴이 너무 많음 — 통합 가능한가, LLM 입장에서 더 효율적인가"** (사용자 요청) — ID-003(s13)
