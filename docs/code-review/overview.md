# Code Review Overview — TARS 코어 코드베이스

**검토 기간**: 2026-04-25 (s01 ~ s28, 28 세션)
**검토 범위**: ~50,000+ 줄 (Go backend + Svelte 5 frontend)
**작성일**: 2026-04-25

이 문서는 28 세션의 코드 리뷰 결과를 종합한 **우선순위 백로그 + 의사결정 가이드**다. 새로운 사용자나 다음 검토 세션이 이 문서 한 장으로 컨텍스트를 복원할 수 있게 작성한다.

---

## 1. 결과 요약

### 발견사항 통계

| 카테고리 | 개수 | 비고 |
|---------|------|------|
| **RF** (refactor candidate) | 73 | 코드 동작 유지하면서 구조 개선 |
| **ID** (idea / decision) | 5 | 사용자 결정 필요한 큰 의사결정 |
| **Q** (open question) | 10 | 정량 데이터 또는 추가 조사 필요 |
| **TN** (tension) | 3 | 가이드라인 vs 코드 충돌 |
| **Total open** | **91** | |

### 검토 진척

```
Backend 인프라  ✅ s01-s23 (23 세션)
Backend 도메인  ✅ s24-s27 (4 세션, session/config/cron)
Frontend        ✅ s28    (1 세션, 핵심)
Overview        ✅ 본 문서
```

---

## 2. 큰 결정 백로그 (5 ID)

각 ID 는 사용자 결정이 필요한 의사결정. 진행 순서 권장:

### ID-005 — `internal/gateway` → `internal/agentruntime` (이름 변경)

**상태**: open (사용자 결정 — *"비용 크더라도 깔끔하게"* hard cut 방향)

**왜**: CLAUDE.md description (*"Agent execution platform"*) 와 패키지명 mismatch. 5 책임 (lifecycle/executor/channels/consensus/persistence) 모두 *"실행+조정"* 성격, *"라우팅+변환"* 이 아님.

**작업 추정**: 7-10일, 자동화 70-80% (sed)

**6 Phase 작업 (ID-005 본문 [findings/ideas.md#id-005])**:
- Phase 0: 사전 준비 (이름 + URL prefix + config 호환성 결정) — 1일
- Phase 1: 패키지 import + 식별자 (50+ 파일) — 1-2일
- Phase 2: 변수명 + 파일명 + handler — 2-3일
- Phase 3: Config 35+ field — 2일
- Phase 4: HTTP URL 7+ + 콘솔 UI fetch URL 35+ — 1-2일
- Phase 5: 빌트인 툴 + 콘솔 UI 페이지 라벨 — 1일
- Phase 6: 잔재 정리 + archive prefix 정책 — 1일

**5 PR 분할 권장** (각 phase 별 e2e 검증).

**진행 우선**: 가장 큰 결정이지만 자동화 비율 높음. 다른 ID 결정 전에 진행하면 base 가 깨끗해짐.

### ID-004 — OpenAI/Gemini provider capability 격차 해소 (OpenAI 1순위)

**상태**: open (사용자 명시 — *"OpenAI Codex 부상, OpenAI 완성도 1순위"*)

**Provider capability 점수** (13점 만점):

| Provider | 점수 | 비고 |
|----------|------|------|
| anthropic | **12/13** ⭐ | 표준 수준 |
| gemini-native | **9/13** | 의외로 풍부 (PDF + Thinking 양차원) |
| openai-compat | **7/13** | reasoning_effort + service_tier |
| **openai-codex** | **4/13** ⚠️ | **사용자 1순위** — 격차 가장 큼 |
| gemini-compat | 4/13 ⚠️ | gemini-native 가 우월 → deprecate 후보 |
| claude-code-cli | 0/13 (special) | CLI 자체 도구 |

**5 Phase 작업 ([findings/ideas.md#id-004])**:
- Phase 1: OpenAI Codex ToolChoice specific tool + ResponseFormat (RF-048 + RF-049) — **사용자 1순위**
- Phase 2: OpenAI multimodal PDF 지원 (RF-046)
- Phase 3: capability 매트릭스 통합 (RF-047)
- Phase 4: Gemini-native responseSchema 추가
- Phase 5: gemini-compat deprecate 결정

**진행 우선**: ID-005 다음. OpenAI Codex 가 캡처 capability 가장 빈약 (사용자 명시).

### ID-003 — 빌트인 툴 통합 vs 분리 정책 명문화

**상태**: open (사용자 결정)

**현재 인벤토리**:
- Aggregator 6개: memory / knowledge / workspace / automation / session / subagents_*
- 분리 8+개: read_file / write_file / edit_file / apply_patch / list_dir / glob / web_search / web_fetch / exec / process / usage_report / telegram

**시스템 프롬프트 비용 추정**:
- Default 베이스 ~750-900 토큰/turn
- Full ~1500-2000 토큰/turn

**4 통합 후보 (절감 추정)**:
| 케이스 | actions | 우선순위 | 절감 토큰 |
|--------|---------|---------|----------|
| `web` (s14) | search / fetch | ★★★ | -30 |
| `file` (s13) | read/write/edit/list/glob | ★★ | -150 |
| `subagents` (s17) | run/plan/orchestrate (이미 같은 schema) | ★★ | -300 |
| `gateway-message-nodes` (s16) | 9 actions | ★ (외부화 우선) | -150 |

**합산 절감**: ~-630 토큰/turn (default). 100 turn/day 가정 시 **63k 토큰/일**.

**4 행동 갈래** ([findings/ideas.md#id-003]):
- (A) 현 상태 유지
- (B) 통합 (web 부터 첫 실험)
- (C) 분리 (기존 aggregator 풀기)
- (D) 부분 통합 + 빈도 낮은 툴 외부화 (knowledge/workspace/usage_report)

**진행 우선**: ID-005 + ID-004 후. 정량 데이터 (usage tracker) 후 결정.

### ID-001 — Knowledge feature (KB Wiki) 효용 vs 비용 재평가

**상태**: open (가설 A 확정 강화)

**가설 A (확정 입증)**: KB 가 *"write 경로는 야심차지만 read/그래프 시각화/chat 통합이 미완"*

**입증 chain (s10/s11/s15)**:
1. KB note 가 semantic 인덱스에 등록 안 됨 (s15)
2. `memory_search.include_knowledge` 기본 false (s11)
3. `prompt/memory_retrieval.go` fallback chain 6 매처에 KB 없음 (s11)
4. 콘솔 UI 의 그래프 시각화 부재 (s11/s28 — 카운트만 표시)

→ **chat 시 KB 사용하려면 두 단계 명시 호출 필요** (memory.search + include_knowledge=true). LLM 자발적 호출 확률 매우 낮음.

**4 행동 갈래** ([findings/ideas.md#id-001]):
- (A) 현 상태 유지
- (B) 단순화 (KnowledgeNote 제거, ~1.4k LOC 감소)
- (C) 활용 확장 (include_knowledge default true + 그래프 시각화 + compileKnowledge 게이트 확대)
- (D) 부분 정리 (그래프 제거, wiki 노트 유지)

**진행 우선**: 정량 데이터 (KB notes 개수, compileKnowledge 빈도) 후 결정. 가설 A 강하므로 (B)/(D) 가능성 높음.

### ID-002 — 시스템 프롬프트 헤더 외부화

**상태**: open

**왜**: `prompt/builder.go:46-55` 의 헤더 ("You are TARS...") + Response Formatting 가이드가 코드 박힘. USER.md/IDENTITY.md 가 자유 편집인데 헤더는 컴파일 타임 고정 — 일관성 결여.

**4 행동 갈래** ([findings/ideas.md#id-002]):
- (a) 헤더 → IDENTITY.md 통합
- (b) Response Formatting → RESPONSE.md 분리
- (c) 현 상태 유지
- (d) 부분 외부화 (정체성만)

**진행 우선**: 마지막. 다른 ID 진행 후 검토.

---

## 3. 보안/Resilience 가드 11종 ⭐⭐ (모범 패턴 catalog)

| # | 가드 | 위치 | 책임 |
|---|------|------|------|
| 1 | RegistryScope panic | s12 `internal/tool/tool.go` | wiring-time 누수 방지 |
| 2 | PathPolicy symlink escape | s13 `internal/tool/workspace_path.go` | read/write 분리 |
| 3 | web_fetch SSRF 다층 | s14 `internal/tool/web_fetch.go` | scheme/IP/DNS/redirect 재검증 |
| 4 | subagent triple guard | s16 `internal/tool/tool_subagents.go` | agent kind + tool blocklist + prefix catch-all |
| 5 | flow validation 5 가드 | s17 `internal/tool/tool_subagents_orchestrate.go` | depth/threads/dup/dep dag/maxThreads |
| 6 | NewRouter 7-validation | s18 `internal/llm/router.go` | boot-time loud failure |
| 7 | OpenAI Codex OAuth 자동 갱신 | s20 `internal/llm/openai_codex_client.go` | 401/403 + refresh + transient retry |
| 8 | CommandExecutor env injection | s21 `internal/gateway/executor.go` | control char 거부 |
| 9 | consensus 7-layer budget guard | s22 `internal/gateway/consensus.go` | flag/fanout/token/USD/timeout/runtime cap/semaphore × 2 |
| 10 | ResolveOverride task override 7-layer | s23 `internal/gateway/resolve_override.go` | enabled + alias + AllowedAliases + LLMProviders + model + AllowedModels + APIKey |
| 11 | ResolveLLMTier 7-validation | s25 `internal/config/llm_resolve.go` | NewRouter 7-validation 의 사전 단계 |

**14-validation 체인** (NewRouter + ResolveLLMTier) = CLAUDE.md *"single resolution path, loud errors"* 의 본거지.

**모두 wiring/input-time 정합성 검증**. 코드베이스의 일관된 *"silent skew 방지"* + *"resilience"* 의식.

→ 새 영역 보안 코드 작성 시 이 11종이 reference 모델.

---

## 4. 사용자 친화 패턴 catalog

| 패턴 | 위치 | 책임 |
|------|------|------|
| 3-mode schedule (at:/every/cron) | `internal/cron/manager.go` | one-shot/interval/cron 다양성 |
| Natural language schedule | `internal/cron/validation.go` (한국어/영어 fallback) | *"내일 9시"* / *"every Monday"* |
| Delete after run for one-shot | `internal/cron/helpers.go` | `at:` 자동 cleanup |
| PayloadMeta `_tars_cron` 격리 | `internal/cron/payload_meta.go` | 사용자 payload 와 충돌 방지 |
| LoadHistorySnapshot compaction 보존 | `internal/session/transcript.go` | token budget 잘라도 boundary 유지 |
| Stacking carry-forward | `internal/session/compaction.go` | 이전 summary 영원히 보존 |
| CJK token 가중치 | `internal/session/compaction.go` | 한국어/일본어/중국어 정량 |
| Legacy URL alias | `frontend/console/src/lib/router.ts` | 옛 bookmark 호환 |
| Legacy migration warning | `internal/config/legacy.go` (`tars doctor`) | removed key 안내 |
| applyLLMPoolDefaults kind-specific | `internal/config/defaults_apply.go` | `kind: anthropic` 한 줄로 동작 |
| OpenAI-codex oauth fallback | `internal/config/defaults_apply.go` | api-key 없으면 oauth 자동 promotion |
| Shared EventSource | `frontend/console/src/lib/api.ts` | 브라우저 connection 한계 회피 |
| Anthropic 자동 prompt caching | `internal/llm/anthropic.go` | system + tools 끝 ephemeral 자동 |
| Subagent → main session forwarding | `internal/gateway/runtime_run_execute.go` | `[RUN SUMMARY]` 자동 |
| migrateLegacyArtifactDir | `internal/session/session.go` | silent migration |
| applySessionDefaults | `internal/session/session.go` | artifact dir + work_dirs 정규화 |

→ 코드베이스 두 축: **보안 가드** + **사용자 친화**. CLAUDE.md *"crash-resistant idempotence"* + *"working baseline"* 정렬.

---

## 5. 누적 안티패턴 4 카테고리 (큰 결정 후보)

s27 backend 종료 시점 정리. **여러 사례가 누적된 코드베이스 안티패턴**.

### A) O(N) read+write per mutation (4 사례 누적)

| RF | 위치 | 패턴 |
|----|------|------|
| RF-017 | `entries.jsonl` (semantic.go) | O(N) load+rewrite |
| RF-021 | `KnowledgeStore.rebuildArtifacts` | O(N×N) batch |
| RF-058 | `sessions.json` (Store) | O(N) full read+write per mutation |
| RF-067 | `cron/jobs.json` | 동일 |

**큰 결정 후보**: SQLite 마이그레이션. 4 패키지의 file-based persistence 통합. 사용자 워크스페이스 100+ 항목 환경에서 hot path 비용 누적.

### B) Atomic write 미적용 (3 사례 누적)

| 위치 | atomic? |
|------|---------|
| `gateway/persistence.go` writeJSONAtomic | ✅ 모범 |
| `tool/write_file.go` writeTextFileAtomic | ✅ 모범 |
| `session/session.go` saveIndex | ❌ [RF-059] |
| `knowledge.go` buildKnowledgeDocument | ❌ |
| `cron/store.go` save | ❌ [RF-068] |

**큰 결정 후보**: 공통 `internal/atomicwrite/` 패키지 추출. session/knowledge/cron 모두 사용. 사용자 데이터 crash safety.

### C) 자체 min/max 함수 (5 사례)

Go 1.25.6 환경에서 built-in `min`/`max` 사용 가능:

| RF | 위치 |
|----|------|
| RF-019 | `semantic.go` (4 dead code 함수) |
| RF-019 | `prompt/builder.go:215-227` |
| RF-019 | `tool/list_dir.go:172-177` |
| RF-019 | `cron/helpers.go:64-69` (RF-069) |

**작은 RF**. 일괄 정리 PR 가능 (5 사례).

### D) YAGNI 위반 stub (4 사례 누적)

| RF | 위치 | 사용 |
|----|------|------|
| RF-044 | `Router.Close` no-op | 호출자 0건 |
| RF-044 | `fallbackClient` | production wiring 0건 (테스트 only) |
| RF-051 | `CommandExecutor` | production wiring 0건 |
| RF-052 | consensus `vote` strategy | schema enum 노출 + 미구현 |
| RF-043 | `provider="codex-cli"` error stub | removed 코멘트 잔재 |
| RF-055 | `runtime_nodes.go` 3 hardcoded node | 사용 빈도 의문 (Q-016) |

**일괄 정리 PR**. CLAUDE.md *"If you are certain that something is unused, you can delete it completely"* 정렬.

---

## 6. 73 RF 우선순위 매트릭스

### Tier 1 — Trivial cleanup (즉시 가능)

| RF | 영역 | 비고 |
|----|------|------|
| RF-019 | min/max 자체 함수 5 사례 | Go built-in |
| RF-027 | IsExecToolName 이중 정규화 | 1 줄 |
| RF-031 | exec command/cmd alias | schema 일치 |
| RF-038 | include_sessions description 자기모순 | trivial |
| RF-043 | codex-cli error stub | 3 줄 삭제 |
| RF-054 | event 중복 publish | 한 번만 |
| RF-060 | Compaction keepRecent 우선순위 docstring | trivial |
| RF-063 | ServiceTier 2-level docstring | trivial |
| RF-069 | cron/helpers.go min 함수 | RF-019 카테고리 |

→ **9개**. 한 PR 또는 *"YAGNI cleanup"* 묶음 PR.

### Tier 2 — 작은 개선 (S, ROI 높음)

| RF | 영역 | 비고 |
|----|------|------|
| RF-001 | Deprecated `--run-once` 플래그 | TN-002 와 함께 |
| RF-002 | setupRuntimeLogger cleanup 누수 | resource leak |
| RF-003 | sessionStoreResolver 잉여 초기화 | dead |
| RF-005 | daily_log 좀비 case | dead branch |
| RF-006 | telegramDeliveryCounter pulse wiring | CLAUDE.md gap |
| RF-013 | Source enum priority | silent skew |
| RF-018 | KR stopwords 추가 | 다국어 |
| RF-020 | KnowledgeStore.nowFn dead | dead capability |
| RF-022 | Graph self-healing 단순화 | magic substring |
| RF-023 | dead project/brief matchers | RF-019 카테고리 |
| RF-026 | Registry.Register silent overwrite | wiring panic |
| RF-029 | read_file 6 parameter alias | LLM 혼란 |
| RF-030 | File 툴 시그니처 3중 | 중복 |
| RF-033 | exec blocklist false sense | 보안 |
| RF-035 | ProcessManager timeout 30초 cap | long-running 충돌 |
| RF-038 | include_sessions 자기모순 | (Tier 1) |
| RF-039 | cron alias 다중 silent | RF-029 카테고리 |
| RF-041 | subagents 3 툴 통합 | ID-003 |
| RF-045 | openai_compat ctx.Value | 비표준 |
| RF-046 | PDF silent placeholder | RF-072 |
| RF-047 | provider capability 매트릭스 docstring | ID-004 |
| RF-048 | openai_codex tool_choice hardcoded | **ID-004 핵심** |
| RF-049 | openai_codex advanced fields | **ID-004 핵심** |
| RF-050 | semaphore 코드 중복 | 통합 |
| RF-053 | finalizeRunLocked 200줄 | 가독성 |
| RF-056 | Telegram channelID fallback | 명시화 |
| RF-057 | archive flag 의미 분리 | 명시화 |
| RF-061 | applySessionDefaults disk IO | hot path |
| RF-064 | default model name outdated | **ID-004 결합** |
| RF-068 | cron save atomic write | RF-059 카테고리 |
| RF-070 | computeBackoffDuration magic | 작음 |
| RF-073 | frontend fetch URL 35+ | **ID-005 결합** |

→ **~30개**. ID-004 + ID-005 진행 시 자연스러운 결합 진행.

### Tier 3 — 큰 결정 (M-L, 사용자 결정 필요)

| RF | 영역 | 카테고리 |
|----|------|---------|
| RF-007 | 빌트인 플러그인 시스템 제거 | **TN-001 흡수** |
| RF-008 | sh 훅 제거 | **보안 critical** |
| RF-009 | 외부 HTTP 정책 결정 | RF-007 종속 |
| RF-014 | Silent error swallowing | logging |
| RF-015 | 공통 timewindow/textutil 패키지 | 중복 정리 |
| RF-017 | entries.jsonl O(N) | **누적 카테고리 A** |
| RF-021 | KnowledgeStore.rebuildArtifacts | **누적 카테고리 A** |
| RF-024 | Bootstrap 메타 3곳 분산 | silent skew |
| RF-025 | fallback chain O(N×lines) | RF-061 카테고리 |
| RF-032 | exec.blockedExecCommands false sense | (Tier 2 보강) |
| RF-034 | web_fetch htmlToText 단순 regex | 출력 품질 |
| RF-037 | memory_search ↔ prompt fallback chain 중복 | 통합 |
| RF-040 | message/nodes/gateway 통합/외부화 | **ID-003 결합** |
| RF-042 | subagents_plan normalization | **ID-004 종속** |
| RF-044 | Router.Close + fallbackClient | **누적 카테고리 D** |
| RF-051 | CommandExecutor wiring 0 | **누적 카테고리 D** |
| RF-052 | consensus vote 미구현 | **누적 카테고리 D** |
| RF-055 | nodes 3 hardcoded | **누적 카테고리 D** |
| RF-058 | sessions.json O(N) | **누적 카테고리 A** |
| RF-059 | session saveIndex atomic | **누적 카테고리 B** |
| RF-062 | Config 350+ field flat | namespaced |
| RF-065 | yaml_paths 200+ switch | DRY |
| RF-066 | applyLLMPoolDefaults kind switch | provider registry |
| RF-067 | jobs.json O(N) | **누적 카테고리 A** |
| RF-071 | frontend 1000+ component 분해 | UI ergonomic |
| RF-072 | types.ts 백엔드 mirror | OpenAPI 검토 |

→ **~26개**. ID-001 / ID-003 / ID-004 / ID-005 와 결합 진행 권장.

### Tier 4 — Tension 흡수 / 의도 결정

| TN | 영역 |
|----|------|
| TN-001 | browserplugin (RF-007 흡수 예정) |
| TN-002 | cmd/tars ↔ tarsserver 플래그 이중화 |
| TN-003 | tools_provider.script 미구현 (RF-007 종속) |

---

## 7. ID-005 마이그레이션 PR 분할 (5 PR 권장)

```
PR #1: Phase 1 (패키지 import + 식별자 sed)
       — 큰 PR, 대부분 자동화. 컴파일+테스트 통과만 검증.
PR #2: Phase 2 (변수명 + 파일명 + handler 정리)
       — 자동화 + 수동 검토.
PR #3: Phase 3 (config 35+ field 마이그레이션)
       — config 호환성 결정 포함 (hard cut vs 1 release alias).
PR #4: Phase 4 + 5 (HTTP URL + 콘솔 UI + 빌트인 툴 동시)
       — UI 동시 마이그레이션 필수.
PR #5: Phase 6 (잔재 정리 + archive prefix 정책 + CLAUDE.md 갱신)
       — 최종 cleanup + 문서.
```

각 PR 마다:
- VERSION.txt bump
- CHANGELOG entry (이름 변경 + 마이그레이션 가이드)
- e2e 테스트 회귀
- 사용자 워크스페이스 마이그레이션 안내

---

## 8. 모범 패키지 catalog

### internal/pulse/ + internal/reflection/

System surface 쌍. nil-tolerant Dependencies + narrow interfaces + panic recovery + crash-resistant idempotence. **다른 영역 리팩토링 베이스로 가치**.

### internal/tool/

`RegistryScope` 의 wiring-time panic 가드. CanonicalToolName 단일 진입점 + 4-rule policy resolution. PathPolicy 추상화. web_fetch SSRF 다층 가드. subagent triple guard.

### internal/llm/

Tier/Role/Router architectural clarity. *"config-only role-tier reassignment"* 약속 + boot-time loud failure. NewRouter 7-validation. **CLAUDE.md 약속 거의 완벽 정렬**.

### internal/llm/anthropic.go

자동 prompt caching (system + tools 끝 ephemeral) + ToolChoice specific tool 강제 지원 (RF-042 의 anthropic 환경 즉시 가능 인풋).

### internal/llm/gemini_native.go

Thinking 양차원 (Level + Budget) + PDF native (inlineData) + preflight check. **anthropic 다음 풍부** (9/13).

### internal/gateway/ (=agentruntime)

5 책임 통합 (lifecycle / executor / channels / consensus / persistence). Run lifecycle 4단계 분리 + non-blocking SSE publish + atomic snapshot + parseIDSequence + active run 보존 trim + workspace 격리. **Resilience 3축** (persistence + restore + archive). **TARS 의 dependency hub**.

### internal/session/

chat 컨텍스트 관리의 4-layer (message/history/compaction/summary 보존) + LoadHistorySnapshot 의 compaction boundary 보존 + stacking carry-forward + CJK token 가중치 + path-level mutex (sync.Map).

### internal/config/

CLAUDE.md *"single resolution path, loud errors"* 의 본거지. LLM 3-layer (provider pool + tier binding + role defaults) + JSON env override + nested expansion + **configInputField DRY 패턴** (180+ field 단일 source) + **applyLLMPoolDefaults kind-specific** (사용자 minimum config).

### internal/cron/

3-mode schedule (`at:`/`every:`/cron) + TryStartRun mutex + exponential backoff + delete_after_run + PayloadMeta `_tars_cron` 격리 + natural language schedule fallback. **사용자 친화 모범**.

### frontend/console/

Svelte 5 runes 일관 + vanilla pushState + Shared EventSource pattern + Legacy URL alias + 9-view discriminated union + requestJSON wrapper.

---

## 9. CLAUDE.md 정렬 정리

### 거의 완벽 정렬 ✅ (9개)

- *"3-tier Router binds heavy/standard/light"*
- *"single resolution path, loud errors"*
- *"Provider alias can serve multiple tiers"*
- *"single-JSON env override"*
- *"60+ config fields"*
- *"new users get a working baseline after setting ANTHROPIC_API_KEY"*
- *"max 4 subagents"* + *"max depth"*
- *"crash-resistant idempotence"* (대부분 영역)
- *"Svelte 5 runes + vanilla pushState"*

### 부분 위반 ⚠️ (RF 카테고리)

| 약속 | 위반 |
|------|------|
| *"Avoid backwards-compatibility hacks"* | RF-001 / RF-043 / RF-052 / RF-064 (default model) |
| *"Don't add features beyond what the task requires"* | RF-044 / RF-051 / RF-052 / RF-055 (YAGNI 누적) |
| *"crash-resistant idempotence"* | RF-059 / RF-068 (atomic write 미적용) |
| *"Most recent Claude model family is Claude 4.X"* | RF-064 (default model outdated) |

### 명세-명명 mismatch ⚠️

| CLAUDE.md description | 패키지명 | 결정 |
|----------------------|---------|------|
| *"Agent execution platform"* | `internal/gateway/` | **ID-005** (rename 결정) |

---

## 10. 다음 단계 권장 순서

### 즉시 가능 (사용자 결정 불필요)

**Tier 1 (trivial cleanup)** — 9 RF 한 PR:
- min/max built-in 사용 (RF-019)
- IsExecToolName 단순화 (RF-027)
- exec cmd alias 제거 (RF-031)
- include_sessions docstring (RF-038)
- codex-cli stub 제거 (RF-043)
- ServiceTier docstring (RF-063)
- 등

→ **반나절 작업**. 코드베이스 정리 출발점.

**Tier 2 일부 (logging + dead code)** — 5 RF:
- RF-002 logger cleanup
- RF-003 sessionStoreResolver
- RF-005 daily_log 좀비
- RF-014 silent error logging
- RF-020 KnowledgeStore.nowFn

→ **하루 작업**. 재발 방지.

### 사용자 결정 필요 — 진행 권장 순서

1. **ID-005** (gateway → agentruntime) — base 정리
2. **ID-004 Phase 1** (OpenAI Codex 격차 해소, RF-048+RF-049+RF-064)
3. **누적 카테고리 D** (YAGNI stub 일괄 — RF-044/051/052/055)
4. **누적 카테고리 C** (atomic write 공통 헬퍼 — RF-059/068)
5. **ID-003** (빌트인 툴 통합 — web 부터 첫 실험)
6. **ID-001** (KB 효용 결정 — 정량 데이터 후)
7. **누적 카테고리 A** (O(N) → SQLite — 큰 결정)
8. **ID-002** (시스템 프롬프트 헤더 외부화)

---

## 11. 정량 데이터 수집 권장 (10 Q resolution)

다음 결정 전에 usage tracker 데이터 필요:

| Q | 영역 | 결정 인풋 |
|---|------|----------|
| Q-011 | process 빈도 | 외부화 후보 |
| Q-012 | subagents plan/orchestrate 빈도 | RF-041 또는 외부화 |
| Q-013 | claude-code-cli 빈도 | RF-047 capability 매트릭스 |
| Q-014 | persistSnapshot retry 효용 | 단순화 가능 |
| Q-015 | consensus 활성 빈도 | 외부화 후보 (~400줄+) |
| Q-016 | nodes 사용 빈도 | RF-055 적용 |
| Q-017 | SessionToolConfig 활용 | 단순화 후보 |
| Q-018 | Plan/Task (tasks.go) 활용 | 외부화 후보 (195줄) |

→ **8 Q + ID-001/003 정량 인풋** = usage tracker 1 주~1 개월 수집. 그 후 결정 가능.

---

## 12. 재방문 가이드 (다음 검토 세션)

이 검토는 28 세션 / 1일에 진행됨. 다음 세션이 컨텍스트 복원할 때 순서:

1. **본 문서 (overview.md)** — 큰 그림
2. **README.md** — 마지막 진척 + 백로그 + 모범 패키지
3. **code-map.md** — 어디까지 봤는지
4. **journal/** — 시간순 스토리, 각 세션의 발견
5. **findings/** — 분류된 finding (RF/Q/ID/TN)

ID 결정 시 본 overview 의 **§2 큰 결정 백로그** 참고. RF 작업 시 **§6 우선순위 매트릭스** 참고.

---

## 13. 마무리

TARS 코드베이스의 **두 축**:

- **보안 가드 11종** — wiring/input-time 정합성 검증, *"silent skew 방지"*
- **사용자 친화 패턴** — Natural language schedule, Stacking carry-forward, Legacy URL alias, kind-specific defaults 등

**기술 부채**:
- 4 누적 안티패턴 (O(N) / atomic write / min-max / YAGNI stub)
- 5 큰 의사결정 (ID-001~005)

**작업 우선순위**:
- 즉시: Tier 1 trivial cleanup (~반나절)
- 사용자 결정: ID-005 → ID-004 → 누적 카테고리 → ID-003 → ID-001 → ID-002

**개선 가능성**:
- ID-005 hard cut 으로 base 정리 (~7-10일)
- ID-004 Phase 1 으로 OpenAI Codex capability 격차 해소
- 4 누적 안티패턴 일괄 정리로 코드베이스 정리

→ 코드베이스가 **잘 설계됐고 일관성 있다**. 다만 *"한 단계 더 정리"* 가 가능한 잔재 + 의사결정 백로그가 있다. 사용자 결정 + 정량 데이터 수집 후 진행 권장.

---

*검토 종료: 2026-04-25 / s28*
*작성자: Claude Code (Sonnet 4.6 1M context)*
*검토 진척: 51 / ~50 항목 (100%)*
