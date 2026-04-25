---
date: 2026-04-25
session: 17
scope: internal/tool/{tool_subagents_plan,tool_subagents_orchestrate}.go (~1170줄) — subagent plan/orchestrate. **tool 패키지 종료**
next: internal/cron 또는 internal/session 또는 internal/gateway (s18) — tool layer 위에 있는 백엔드 인프라
findings: [RF-041, RF-042, Q-012, ID-003 보강]
---

# Session 17 — `internal/tool/` subagent plan/orchestrate (마지막 단계)

이번 세션은 tool 패키지의 마지막. 3 subagent 빌트인 툴 중 본문 (plan/orchestrate) 분석.

## 다룬 파일

- `tool_subagents_plan.go` (629) — `subagents_plan` (LLM-as-planner)
- `tool_subagents_orchestrate.go` (539) — `subagents_orchestrate` (단계별 실행)

총 ~1170줄.

## 한눈에

### plan→orchestrate→run 워크플로

3 단계 명확한 책임 분리:

```
[plan]        자연어 goal + constraints → 구조화 flow JSON (LLM-as-planner)
                     ↓
              subagentFlowInput { steps: [{mode, tasks: [{id, prompt, depends_on, tier}]}] }
                     ↓
[orchestrate] flow JSON 받아 실행 (parallel + sequential, placeholder rendering)
                     ↓
[run]         (별도 단순 fan-out 툴, s16)
```

**핵심**: plan 의 출력 schema = orchestrate 의 입력 schema (같은 `subagentFlowInput`). **의미적 통합 자연스러움** [RF-041].

### subagents_plan 의 복잡도 분포

629줄 = ~250줄 (planner LLM 호출 + 프롬프트 + 입력 검증) + **~300줄 normalization layer** + ~80줄 헬퍼.

normalization layer (s17 핵심 분석 영역):
- `normalizeSubagentFlowPlan` — step ID + task ID sanitize + 충돌 시 numbering
- `rewritePlannerDependsOn` / `rewritePlannerPromptReferences` — ID 변경 시 reference 도 같이 rewrite
- `resolvePlannerReference` / `registerPlannerReference` — 매핑 (raw / lower / sanitized 3 변종)
- `collectPlannerTargets` / `ensurePlannerTargetsInPlan` / `plannerTaskForTarget` — required path verbatim 보존 강제
- `decodeSubagentFlowInput` — JSON parsing 폴백 (raw → fenced strip → regex first {} chunk)

→ LLM 출력 noise 흡수에 ~300줄. RF-042 후보.

### subagents_orchestrate 의 핵심 디자인

- **placeholder system** (`{{task.X.summary|response|error}}`):
  - 단계 간 결과 전달
  - regex 기반 substitution
  - missing reference 시 명확한 에러
- **mock-able functions** (orchestrate.go:19-29):
  ```go
  var (
      subagentFlowSpawn = func(...) { ... }
      subagentFlowWait = func(...) { ... }
      subagentFlowCancel = func(...) { ... }
  )
  ```
  → 패키지 레벨 변수로 spawn/wait/cancel 추상화. 테스트에서 override 가능. **모범 패턴**.
- **validateSubagentFlow** — 입력 검증:
  - mode 필수 (parallel | sequential)
  - duplicate task ID 거부
  - parallel step 내 task 가 같은 step task에 depend 거부
  - 미래 task depend 거부
  - parallel step 의 task 수가 maxThreads 초과 거부
- **depth/threads cap** — gateway config 의 max_threads / max_depth 강제

## 좋은 패턴 ⭐

### 1. plan/orchestrate 책임 분리

LLM 은 plan 만 만들고, deterministic 코드가 실행. **에이전트 안전성의 핵심 패턴**:
- LLM 가 직접 spawn/cancel 못함
- depth/threads cap 강제
- subagent safety guard (s16 triple guard) + flow validation 합쳐서 4 layer 방어

### 2. mock-able function variables

`var subagentFlowSpawn = func(...) { ... }` 패턴:
- 테스트에서 `subagentFlowSpawn = mockFn` 으로 override
- production code 에 mock 의존성 (test fixtures) 누수 없음
- → 다른 영역 (memory, cron) 테스트 패턴의 reference 가치

### 3. Required targets verbatim 보존 (ensurePlannerTargetsInPlan)

```go
// LLM 이 사용자 명시 path 를 prompt 에 안 박았으면 자동 추가
for _, target := range sanitizeStringList(targets) {
    task := plannerTaskForTarget(plan, target)  // basename/dirname matching
    if !strings.Contains(strings.ToLower(task.Prompt), strings.ToLower(target)) {
        task.Prompt += "\n\nRequired exact target path:\n- " + target
    }
}
```

→ LLM 이 *"a/b/c.go 분석해"* 를 *"코드 분석"* 으로 paraphrase 하는 것 방지. 사용자 의도 보존 강제. RF-042 평가 시 단순화 vs 안전성 트레이드오프.

### 4. Reference rewrite chain

ID 정규화 시:
1. `step_1` 같은 base sanitize
2. 충돌 시 `step_1_2` 자동 numbering
3. depends_on 의 옛 ID → 새 ID 매핑
4. prompt 의 `{{task.OLD.summary}}` → `{{task.NEW.summary}}` rewrite

→ **side-effect 없는 rename**. LLM 출력 noise 흡수의 견고한 패턴.

## 새 발견 (RF급)

### RF-041 — 3 subagent 툴 통합

`subagents_run` (s16) + `subagents_plan` + `subagents_orchestrate` = 1512줄, 같은 도메인.

**가장 강한 통합 정당성**: plan 의 출력 = orchestrate 의 입력 (같은 schema). 의미적으로도 묶이는 게 자연스러움.

→ `subagents` 단일 aggregator (action: run / plan / orchestrate). process 패턴 (s15) 적용. 시스템 프롬프트 -300 토큰/turn 추정.

### RF-042 — `subagents_plan` normalization layer 두꺼움

~300줄 의 LLM 출력 정규화. strict tool calling / structured output 모드 활용 시 일부 제거 가능. 단 ID sanitize + reference rewrite 는 유지 필요 (LLM이 한글 ID 만들거나 충돌 ID 만드는 경우).

`collectPlannerTargets` + `ensurePlannerTargetsInPlan` 부분은 prompt 강화 + few-shot 으로 줄일 수 있을 가능성. RF-042 본문에 옵션 정리.

## 새 발견 (Q급)

### Q-012 — plan/orchestrate 의 실제 호출 빈도

3 가설:
- (a) `run` 위주 — plan/orchestrate 거의 dead code 가까움
- (b) `plan→orchestrate` 위주 — LLM-as-planner 패턴 활성
- (c) 혼합 — 복잡도에 따라 분기

usage tracker 데이터로 확인 필요. (a) 라면 RF-041 (통합) 또는 외부화 우선.

## ID-003 보강 — 통합 케이스 4 누적

s13~s17 까지 통합 후보 누적:

| 케이스 | actions | LLM ergonomic | 통합 우선순위 |
|-------|---------|--------------|--------------|
| `web` | search / fetch (2) | 매우 강 | ★★★ (첫 실험 권장) |
| `file` | read/write/edit/list/glob (5-6) | 강 | ★★ |
| `subagents` | run/plan/orchestrate (3) | 강 — 이미 같은 schema | ★★ |
| `gateway-message-nodes` | 9 actions | 중 | ★ (외부화 우선 고려) |

→ **통합 시 시스템 프롬프트 절감 합산** 추정 ~-630 토큰/turn (default 베이스). 100 turn/day 가정 시 63k 토큰/일.

ID-003 결정의 정량 임팩트 큼. 정량 데이터 (사용자 워크스페이스의 turn 수, 토큰 단가) + 사용자 결정 후 진행.

## 새 관점 ("필요한가/유용한가") 평가

| 툴 | 평가 |
|----|------|
| `subagents_run` | ✅ 필수 (multi-agent 인프라) |
| `subagents_plan` | ⚠️ Q-012. LLM-as-planner 패턴 자체의 가치 — chat LLM 이 plan 직접 짜는 게 더 직접적 가능 |
| `subagents_orchestrate` | ⚠️ Q-012. 단계별 실행 + placeholder rendering 의 정당성 — *복잡한 의존성* 이 흔한가? |

→ Q-012 데이터 후 결정. (a) 라면 plan/orchestrate 외부화 또는 통합 우선.

## CLAUDE.md 정렬

| 약속 | 실제 | 평가 |
|------|------|------|
| *"max 4 subagents"* | gateway_subagents_max_threads cap (validateSubagentFlow) | ✅ |
| *"max depth"* | nextDepth + maxDepth check (resolveSubagentParentContext) | ✅ |
| *"LLM 은 plan 만, deterministic 코드가 실행"* | plan/orchestrate 분리 명시적 | ✅ |
| *"required exact paths verbatim"* | ensurePlannerTargetsInPlan 자동 보강 | ✅ |
| *"Every tool inflates prompt"* | 3 툴 1512줄 — 통합 후보 [RF-041] | ⚠️ |

## 핵심 인사이트 — tool 패키지 9 세션 종합

### 모범 보안 가드 5종 누적

s12~s17 동안 본 보안 패턴 5개 (이전 4개 + s16 의 subagent triple guard):

1. **`RegistryScope` panic 가드** (s12) — wiring time
2. **`PathPolicy` symlink escape** (s13) — read/write 분리
3. **`web_fetch` SSRF 다층 가드** (s14) — scheme/IP/DNS/redirect
4. **`subagent` triple guard** (s16) — agent kind + tool blocklist + prefix catch-all
5. **`subagent flow validation`** (s17) — depth/threads cap + dep dag validation

→ **코드베이스 보안 의식 일관됨**. 다른 영역 보안 코드 작성 시 reference 모델.

### 통합 정책 부재의 비용

ID-003 산출이 s13 부터 누적. s17 시점:
- aggregator 6개 (memory/knowledge/workspace/automation/session/subagents_*) + 분리 8+개 혼재
- 통합 가능 후보 4개 (web/file/subagents/gateway-message-nodes)
- 통합 시 시스템 프롬프트 -630 토큰/turn 추정

→ **사용자 결정 (ID-003) 의 정량적 가치 크다**. 정량 데이터 수집 후 결정 권장.

### KB 기능 의문 (ID-001) 의 누적 증거

s10~s17 동안 KB 가 chat path에 통합 안 된 것 거듭 입증:
- s10: knowledge.go 자체 검토 — write 위주, read 미완성
- s11: prompt/memory_retrieval.go fallback chain 6 매처에 KB 없음
- s15: memory_search 의 include_knowledge default false + KB가 semantic 인덱스 미등록

→ **가설 A 확정**. ID-001 사용자 결정 시 (2) 단순화 (~1.4k LOC 감소) 정당성 강함.

### 코드베이스의 alias 표면 누적

read_file (RF-029) + exec (RF-031) + cron (RF-039) + memory/knowledge/workspace partial schema (RF-038/ID-003 카테고리). codebase-wide alias 정책 필요 수준.

### LLM tool 표면 인벤토리 (s12~s17 종합)

```
[Default registered] ~11개 (s13 분석)
  memory, knowledge, workspace, usage_report, read_file, write_file,
  edit_file, list_dir, glob, process, exec

[Optional via cfg flag] ~6개 (s14/s16 분석)
  message, nodes, gateway, apply_patch, web_fetch, web_search

[Caller-added] ~10개 (s16/s17 분석)
  cron, telegram_send, sessions_list/history/send/spawn/runs,
  agents_list, session_status, subagents_run/plan/orchestrate
```

→ 총 ~27개 빌트인 LLM 툴. CLAUDE.md *"infrastructure that every session needs"* 기준에서 외부화 후보 ~7-8개 (knowledge / workspace / usage_report / apply_patch / process / message / nodes / gateway / automation 의문).

## 다음 세션 진입점 (s18)

tool 패키지 **종료**. 다음:

옵션 평가 후 사용자 결정 권장:
- **(A) `internal/cron`** — Q-011 (process) + cron LLM 툴이 의존하는 백엔드. 작은 패키지 추정.
- **(B) `internal/session`** — chat session store + transcripts. session_aggregator + memory_search/prompt fallback 모두 의존.
- **(C) `internal/gateway`** — agent 실행 플랫폼. subagents/sessions 빌트인 툴 의존. 큰 패키지 가능.
- **(D) `internal/llm`** — Router + provider abstraction. RF-042 (strict mode) 의 결정 인풋. config 와 결합.
- **(E) `frontend/console/`** — Svelte 5 SPA. 콘솔 UI 의 KB/Memory/Pulse/Reflection 페이지가 ID-001/ID-002 결정 인풋.

→ 권장 순서: (D) → (B) → (C) → (A) → (E). LLM router 가 가장 많은 캡쳐 의존 + 작은 패키지부터.

s18 부터 tool 위에 있는 백엔드 (gateway/session/cron) + tool 아래 인프라 (llm router) + 사용자 표면 (frontend).
