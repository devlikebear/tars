# Ideas — 새 아이디어 / 추가 기능

리뷰 중에 떠오른, 현재 코드의 문제는 아니지만 추가로 만들어볼 만한 것들.

---

## ID-001 — Knowledge feature(KB Wiki) 효용 vs 비용 재평가

- **Status**: open (사용자 결정 필요)
- **Related to**: `internal/memory/knowledge.go`, `internal/reflection/job_memory.go`(compileKnowledge), `internal/tool/knowledge_aggregator.go` + `memory_kb.go`, `internal/tool/memory_search.go`(include_knowledge), `internal/tarsserver/handler_memory.go`(`/v1/memory/kb/*`), `frontend/console/src/components/MemoryCenter.svelte` (Knowledge 탭)
- **Discovered in**: [journal/2026-04-25-10-memory-pt2.md](../journal/2026-04-25-10-memory-pt2.md)

### 무엇

Knowledge 시스템은 frontmatter + sections 마크다운 노트의 wiki-style 저장소 + 그래프(nodes/edges):

- **Storage**: `workspace/memory/wiki/notes/<slug>.md` + 자동 생성 `index.md` + `graph.json`
- **Write 경로 3개**:
  1. nightly reflection의 `compileKnowledge` (LLM Chat → JSON parse → ApplyKnowledgeUpdate)
  2. 콘솔 UI Memory Center → Knowledge Base 탭의 editor
  3. `knowledge` 빌트인 LLM 툴의 `upsert` action
- **Read 경로 3개**:
  1. 콘솔 UI Knowledge 탭의 노트 리스트/에디터
  2. `memory_search` 의 `include_knowledge` 파라미터 (기본 **false**)
  3. `knowledge` 툴의 `list / get` action
- **그래프**: `KnowledgeGraphNode/Edge` 타입 + buildKnowledgeGraph + graph.json — 콘솔 UI는 **`graph.edges.length` 카운트만 표시**(시각화 컴포넌트 없음)

### 왜 (관점 1: 정말 필요한가)

이 기능이 다른 메모리 컴포넌트와 어떻게 차별화되는지:

| 컴포넌트 | 역할 | 구조 | 자동 채움 | 검색 통합 |
|---------|------|------|-----------|----------|
| `MEMORY.md` (DurableKindMemory) | 사용자 영구 메모 | 자유 텍스트 | LLM 시스템 프롬프트가 권유 | 프롬프트 직접 주입 + memory_search |
| Daily logs (DurableKindDaily) | 일별 활동 | 자유 텍스트 | 자동(매 turn) | memory_search |
| Experience (JSONL) | 패턴/선호 사실 | category + summary + tags | 자동 (deriveTurnExperiences 키워드 룰) | semantic.go 인덱스에 IndexExperience |
| **KnowledgeNote** | wiki 노트 | slug + kind + summary + body + tags + aliases + **links** | nightly LLM (compileKnowledge) | memory_search의 **opt-in** (`include_knowledge=true`) |

→ KnowledgeNote = "Experience + structured kind/links + wiki body + 그래프"

차별점: (a) 그래프 구조 (b) 사람이 편집 가능한 wiki body (c) 명시적 links로 노트 간 관계.

### 왜 (관점 2: 정말 유용한가)

**비활용 신호 4개**:

1. **`memory_search` 기본 `include_knowledge=false`** — 도구 description 본인이 자백:
   > "Search knowledge-base notes only when explicitly requested."
   
   → 기본 chat 흐름에서 LLM이 KB를 자발적으로 검색 안 함. 검색 통합이 약함.

2. **`shouldCompileKnowledge` 게이트의 좁은 hint set** (derivation.go:73-92):
   ```go
   hints := []string{"prefer", "preference", "habit", "workflow", "policy", "decision", "owns",
                     "선호", "취향", "습관", "워크플로", "규칙", "정책", "결정", "보유"}
   ```
   → 14개 키워드. nightly 컴파일 trigger 빈도가 매우 낮음 → KB 노트 수 누적이 느림 → 그래프가 빈약하게 유지될 가능성.

3. **그래프 시각화 부재** — 콘솔 UI의 `KnowledgeGraph` 타입은 import되고 데이터는 fetch되지만, **시각화는 `{graph.edges.length} KB relations` 카운트 표시만**. 그래프의 실제 가치(노드 간 연결 시각적 탐색)가 미구현. write side만 있고 read side가 미완.

4. **빌트인 툴 시스템 프롬프트 비용** — CLAUDE.md 가이드라인:
   > Every tool registered in `tool.Registry` emits its description into the chat system prompt at startup — each new tool inflates prompt tokens.
   
   `knowledge` aggregator의 description은 매 chat turn마다 시스템 프롬프트에 박힘 (KB가 비어 있어도). 도구의 가치 ÷ 비용 비율이 낮음.

### 가설 / 대안

**가설 A — 현 설계는 "wiki-style 메모"의 야심찬 버전이지만 활용 측면이 미완성**:

- write side(nightly compileKnowledge + 콘솔 editor + LLM upsert)는 존재
- read side(memory_search 기본값, 그래프 시각화)는 절반 이상 비활성
- 결과: 노트는 쌓이지만 chat 시 활용도 낮고, 콘솔에서 가끔 둘러보는 wiki 정도

**가설 B — 미래 확장 베이스**:

- 그래프 시각화/탐색은 향후 추가될 기능의 인프라
- include_knowledge 기본값은 안전을 위한 opt-in, 활용도가 늘면 default true로 전환

가설 A가 맞으면 **단순화** 방향(Experience + 콘솔 editor + 자유 markdown 만으로 축소). 가설 B가 맞으면 **활용 확장** 방향(그래프 시각화 + memory_search 통합 강화 + compileKnowledge 게이트 확대).

### 정량 데이터 수집 권장

사용자 결정 전에:

1. 현재 사용자 워크스페이스의 `workspace/memory/wiki/notes/*.md` 개수 + 평균 quality 확인
2. `compileKnowledge`가 nightly에 실제로 새 노트 생성하는 비율 (reflection JobResult.Details 통계)
3. `memory_search`가 `include_knowledge=true`로 호출된 횟수 (현재 메트릭이 있는지 확인 필요)
4. `knowledge` 툴이 LLM에 의해 호출된 횟수

### 가능한 행동 갈래

- **(1) 현 상태 유지** — "잠재 가치 있고 비용도 미미하다"
- **(2) 단순화 (가설 A)** — KnowledgeNote 자체 제거, Experience + MEMORY.md + daily logs 만으로 축소. 콘솔 KB 탭 제거 또는 자유 markdown 노트로 대체. 코드 ~849줄 + 콘솔 ~150줄 + 빌트인 툴 ~230줄 + handler ~120줄 ≈ **~1.4k LOC 감소** 가능.
- **(3) 활용 확장 (가설 B)** — `include_knowledge` 기본 true, 그래프 시각화 컴포넌트 추가, compileKnowledge 게이트 확대. 신규 코드 추가 비용.
- **(4) 일부만 정리** — 그래프(읽기 미완)만 제거하고 wiki 노트는 유지. 콘솔 카운트 표시도 제거.

→ 결정 보류 추천. 위 정량 데이터 + 사용자 본인의 KB 활용 빈도 회고 후 결정.

### Tension 가능성

CLAUDE.md의 *"Don't add features, refactor, or introduce abstractions beyond what the task requires"* + *"Default pattern: skill+CLI"* 가이드라인과 맞물려 보면 **빌트인 툴(`knowledge` aggregator)** 부분은 [TN-001](tensions.md#tn-001)/[RF-007](refactor.md#rf-007) 문맥의 "도메인 기능을 코어에 박았다"에 약간 들어맞을 수 있음. 단 memory 자체는 가이드라인이 명시적으로 "infrastructure"로 인정한 영역 → tension은 약함.

### s15 추가 입증 — KB read 통합 0% 정량적 증거

session 15 (`memory_search`/`memory_aggregator`/`memory_save`/`memory_get` 검토)에서 **가설 A 확정** 강화:

1. **KB note 가 semantic 인덱스에 등록 안 됨**:
   - `knowledge_aggregator.upsert` → `KnowledgeStore.ApplyPatch` → 마크다운 파일 작성 + `rebuildArtifacts` (index.md + graph.json 만)
   - **`semantic.IndexEntry` / `IndexExperience` 호출 없음** — KB note 는 entries.jsonl 인덱스에 안 들어감
   - 즉 `backend.Search` (semantic) 결과에 KB note 0%

2. **`memory_search` 의 `include_knowledge` 기본 false** + description 자체가 *"Search knowledge-base notes only when explicitly requested."*

3. **`prompt/memory_retrieval.go` 의 fallback chain에 KB 매처 0개** (s11 확인)

→ **chat 시 LLM이 KB 사용하려면 두 단계 명시 필요**:
- (1) `memory.search` action 호출
- (2) `{ include_knowledge: true }` 명시

LLM이 자발적으로 두 단계 모두 충족할 확률 매우 낮음. 실질적으로 chat path에서 KB 사용 0%.

**역방향 비대칭** (s15 발견):
- KB는 read 닫힘 / write 가능 (LLM이 `knowledge.upsert` 호출 가능)
- MEMORY.md는 read 가능 (memory_get + memory_search) / **write 자동 열림** (`memory_save` 의 AppendMemoryNote — RF-036)

→ memory 표면의 통합 정책이 일관되지 않음. 가설 A 강화 + 추가 정책 결정 (RF-036 + 본 ID-001 결정).

### 결정적 데이터 요청 (s15 보강)

ID-001 결정 전 정량 데이터:

1. ~~현재 사용자 워크스페이스의 KB notes 개수~~ → 사용자 직접 확인
2. ~~compileKnowledge nightly 새 노트 생성 비율~~ → reflection JobResult.Details
3. **`memory_search` 호출 중 `include_knowledge=true` 비율** → usage tracker 또는 로그 (s15 추가)
4. **`knowledge` aggregator 의 LLM 호출 빈도 (action별)** → upsert(write) vs list/get(read) 비율 (s15 추가)
5. ~~LLM 정확도 비교 실험~~ → dev 브랜치 실험

---

## ID-002 — 시스템 프롬프트 헤더(고정 텍스트)를 사용자 편집 가능 영역으로 이동

- **Status**: open (사용자 결정)
- **Related to**: `internal/prompt/builder.go:46-55` (헤더 + Response Formatting), `internal/sysprompt/`, `workspace/USER.md`/`IDENTITY.md`
- **Discovered in**: [journal/2026-04-25-11-prompt.md](../journal/2026-04-25-11-prompt.md)

### 무엇

`prompt.BuildResultFor` 가 매 chat turn의 시스템 프롬프트 첫 부분에 **코드에 박힌 텍스트**를 추가:

```go
b.WriteString("You are TARS, a personal AI assistant.\n")
b.WriteString(fmt.Sprintf("Current time: %s\n", time.Now().UTC().Format(time.RFC3339)))
...
b.WriteString("## Response Formatting\n\n")
b.WriteString("Always use rich Markdown in your responses:\n")
b.WriteString("- Use headings, bold, lists, and tables to structure information clearly.\n")
b.WriteString("- Use fenced code blocks with language tags (...) for any code.\n")
b.WriteString("- When explaining architecture, flows, relationships, or processes, use Mermaid diagrams (...) proactively.\n")
b.WriteString("- Prefer visual explanations (diagrams, tables) over long text when possible.\n")
```

USER.md / IDENTITY.md / AGENTS.md / TOOLS.md 4개는 사용자 편집 가능 (콘솔 UI + sysprompt 빌트인 툴). 그러나 **헤더 + Response Formatting 가이드는 컴파일 타임 고정**.

### 왜 (관점: 정말 필요한가/유용한가)

- **헤더 ("You are TARS...")**: TARS의 정체성 한 줄. IDENTITY.md 가 이미 정체성 영역인데 별도로 박혀 있어 *중복 가능*. 사용자가 IDENTITY.md를 다시 적으면 헤더와 어조 충돌 가능.
- **Response Formatting**: Markdown/Mermaid/code block 스타일 강제. 사용자에 따라:
  - 한국어 출력 톤 다르게 원할 수 있음
  - Mermaid 비활성 원할 수 있음 (Telegram 등 일부 출력 채널은 Mermaid 미지원)
  - "long text 회피" 가이드는 채널별로 정반대 가치
- **Current time**: 동적 — 박혀 있는 게 맞음.

→ 헤더와 Response Formatting은 **사용자별로 의미가 다를 수 있는** 텍스트. 코드 박힘은 일관성 결여.

### 가설/대안

- **(a) 헤더 → IDENTITY.md 통합**: "You are TARS" 줄 제거, IDENTITY.md의 default content가 이 줄을 시작점으로 포함. 사용자가 자기 정체성으로 덮어쓰기 가능.
- **(b) Response Formatting → 별도 RESPONSE.md 또는 TOOLS.md 통합**: 출력 스타일 가이드를 부트스트랩 파일로 외부화.
- **(c) 현 상태 유지**: "TARS의 일관된 정체성/품질 보장은 코드 박힘이 안전. 사용자가 잘못 편집하면 LLM 행동이 망가짐."
- **(d) 부분 외부화**: 정체성만 (a) 처리, Response Formatting은 (c) 유지.

→ 사용자 결정. (d)가 가장 실용적일 가능성.

### 비용/효용

- 변경 비용: S-M (default content 갱신 + builder.go 헤더 제거 + 4 부트스트랩 파일 정의 갱신)
- 효용: 사용자 커스터마이즈 자유 ↑ + 일관성 (사용자 편집 가능한 영역과 코드 박힘 영역의 경계 명확화)
- 위험: 사용자가 IDENTITY.md를 비우면 정체성 0 — fallback 정책 필요

---

## ID-003 — 빌트인 툴 통합 vs 분리 정책 명문화 + file/web aggregator 통합 검토

- **Status**: open (사용자 결정 — 사용자 요청으로 시작된 분석 관점)
- **Related to**: `internal/tool/` 전체, `internal/tarsserver/helpers_agent.go`, `helpers_build_tools.go`
- **Discovered in**: [journal/2026-04-25-13-tool-file.md](../journal/2026-04-25-13-tool-file.md)

### 무엇

현재 빌트인 툴 인벤토리는 **aggregator 패턴 6개**와 **분리 8+개**가 혼재:

| Aggregator (통합) | Sub-actions |
|------------------|-------------|
| `memory` | save / search / get |
| `knowledge` | list / get / upsert / delete |
| `workspace` | sysprompt_get / sysprompt_set / agent_sysprompt_get / agent_sysprompt_set |
| `automation` (cron) | list / get / runs / create / update / delete / run |
| `session` | list / history / send / spawn / runs / status |
| `subagents_*` | plan / orchestrate / run |

| 분리 (단독 툴) | 비고 |
|---------------|------|
| `read_file` | file 도메인 |
| `write_file` | file 도메인 |
| `edit_file` | file 도메인 |
| `apply_patch` | file 도메인, **default disabled** (cfg flag) |
| `list_dir` | file 도메인 |
| `glob` | file 도메인 |
| `web_search` | web 도메인, optional (cfg flag) |
| `web_fetch` | web 도메인, optional (cfg flag) |
| `exec` | shell 도메인 |
| `process` | shell 도메인 |
| `usage_report` | 단독 |
| `telegram` | 단독 |

→ **통합 정책 부재**. 새 툴 추가 시 결정 일관성 X. 사용자 요청이 짚은 핵심.

### 왜 (관점 1: 빌트인 툴이 꼭 필요한가)

CLAUDE.md 의 명시적 가이드라인:
> Every tool registered in `tool.Registry` emits its description into the chat system prompt at startup — each new tool inflates prompt tokens, slows first-turn latency, and raises cost for every chat turn regardless of whether the tool is used.
>
> **When a builtin Go plugin IS appropriate**: infrastructure that every session needs regardless of skill choice (filesystem ops, web search, memory, gateway, telegram delivery)

**기준에 부합하는 빌트인 툴**:
- ✅ filesystem ops (read/write/edit/list/glob) — every session needs
- ✅ memory (search/save/get) — every session needs
- ✅ gateway (subagents) — every session needs
- ✅ telegram delivery — every session needs (delivery 채널)
- ✅ web search/fetch — every session needs (검색 능력)
- ✅ exec (bash) — every session needs (skill+CLI 패턴의 dispatch)

**기준에 의문**:
- ⚠️ `knowledge` aggregator — ID-001 본문 ([ideas.md#id-001](ideas.md#id-001)). KB가 chat path 통합 0%인데 매 turn 시스템 프롬프트 비용. 외부 skill로 외부화 후보.
- ⚠️ `workspace` (sysprompt_get/set) — 사용자가 콘솔에서 편집하는 4 파일을 LLM도 편집 가능하게. **얼마나 자주 LLM이 이걸 호출?** 빈도 낮으면 외부화 후보.
- ⚠️ `usage_report` — LLM에게 "내 사용량 알려줘" 가 흔한 패턴? skill+CLI로 충분.
- ⚠️ `apply_patch` — MVP (cfg flag default disabled). edit_file 여러 호출이 더 일반적.
- ⚠️ `automation` (cron) — cron 등록은 콘솔 또는 CLI도 충분. LLM이 직접 cron 만드는 빈도?
- ⚠️ `process` — long-running 프로세스 관리. exec와 별개. 빈도 낮을 가능성.

### 왜 (관점 2: 단일 통합 가능성 — file 케이스)

**file 단일 aggregator 시나리오**:

```yaml
name: file
description: |
  Filesystem operations on workspace files.
  Actions:
    - read   (line-paginated read)
    - write  (atomic full-file write)
    - edit   (exact text replace)
    - list   (directory entries)
    - glob   (pattern match)
parameters:
  type: object
  properties:
    action: { enum: [read, write, edit, list, glob] }
    # action별 다른 sub-fields, oneOf로 표현
```

**비용 비교**:

| 측면 | 6 분리 툴 | 1 aggregator | 차이 |
|------|----------|--------------|------|
| Description 합 | ~298자 | ~150자 | -50% |
| Parameter schema 합 | ~1.2KB | ~1.5KB (oneOf 표현 비용) | +25% |
| 시스템 프롬프트 토큰 | ~400 토큰 | ~250 토큰 | -150 토큰/turn |
| 매일 100 turn 가정 시 | 40k 토큰/일 | 25k 토큰/일 | -15k 토큰/일 |

→ 절대값은 작지만 **누적 + 작은 모델에서는 비례 더 큼**.

### 왜 (관점 3: LLM 입장에서 정확도/효율)

**LLM이 multi-tool 더 잘 사용하는 케이스**:
- 각 툴이 **단순/명확한 schema** — required field 적고 parameter 의미 명확
- LLM이 *툴 선택*만 하면 schema 자동 결정 (action 선택 단계 없음)
- Strict mode tool calling 에서 schema 검증 단순

**LLM이 aggregator 더 잘 사용하는 케이스**:
- 도메인 **CRUD 패턴** — list/get/create/update/delete 가 명확한 의미
- 같은 backend/storage 공유 — action 사이 일관성 강함
- 새 action 추가 시 새 툴 description 안 박아도 됨

**file 도메인 평가**:
- read/list/glob — 모두 *읽기*. 독립적이지만 **read는 file, list는 dir, glob은 pattern** — 입력 도메인 다름
- write/edit — write는 *전체 교체*, edit는 *부분 교체*. action으로 구분 가능
- 위 6개를 single aggregator로 묶으면 *입력 도메인 4종*: file path, dir path, pattern, content. action별 oneOf 필요

→ **schema 복잡도 ↑** vs **description 비용 ↓** 트레이드오프. **결정적 답 없음** — 실험 필요.

### 코드베이스의 기존 aggregator 사례 평가

| Aggregator | 정당성 | 비고 |
|-----------|--------|------|
| `memory` | ✅ 강 | save/search/get 모두 같은 backend |
| `knowledge` | ✅ 강 | CRUD + 같은 storage |
| `workspace` | ✅ 강 | 같은 4 파일의 get/set |
| `automation` | ✅ 강 | cron CRUD + run |
| `session` | ⚠️ 중 | list/history는 read, send/spawn은 새 lifecycle 시작 — 의미 폭 넓음 |
| `subagents_*` | ✅ 강 | plan→orchestrate→run 워크플로 단계 |

→ **공통 패턴**: same domain + CRUD or lifecycle. file 도메인도 부합. web 도메인도 부합 (search/fetch).

### 가설 / 행동 갈래

- **(A) 현 상태 유지** — "분리가 LLM 정확도에 더 좋다는 경험적 판단" + "schema 단순"
- **(B) 통합 정책 명문화 (단순화 방향)** — file/web도 aggregator로 통합. CLAUDE.md 또는 별도 가이드라인:
  > **Default**: same domain + CRUD/lifecycle 패턴이면 aggregator. 도메인 무관 + 단발 호출이면 분리.
  > **Apply now**: file → 단일 aggregator, web → 단일 aggregator.
- **(C) 통합 정책 명문화 (분리 방향)** — 기존 aggregator 6개를 풀어서 모두 분리 툴로. CLAUDE.md *"Every tool inflates prompt"* 정확히 따름. 단 시스템 프롬프트 비용 커짐.
- **(D) 부분 통합 + 일부 외부화** — 통합과 동시에 빈도 낮은 툴 (knowledge/workspace/automation/usage_report) 은 skill+CLI로 외부화. 시스템 프롬프트 비용 가장 크게 감소. CLAUDE.md *"Default pattern: skill+CLI"* 정렬.

### 정량 데이터 수집 권장

사용자 결정 전:

1. **현재 시스템 프롬프트 비용 측정**: helpers_agent.go에서 등록된 툴들 description + parameter schema 합 토큰 수.
2. **툴별 호출 빈도**: usage tracker가 tool name별 카운트를 갖고 있는지 확인. 1주/1개월 데이터.
3. **LLM 정확도 비교** (옵션): file 단일 aggregator를 dev 브랜치에 만들고 동일 프롬프트로 호출 정확도 측정.

### 권장 첫 단계

(B) 또는 (D) 방향이라면 **file aggregator** 가 가장 안전한 첫 실험:
- 명확한 CRUD 패턴
- 6 file 툴 모두 같은 backend (filesystem) + 같은 PathPolicy
- 통합 비용 작음 (기존 aggregator 6개의 패턴 재사용)
- LLM 정확도 회귀 시 롤백 쉬움

→ RF로 분리하지 않고 **이 ID-003 안에서 결정**. 사용자가 (A)~(D) 중 선택 후 RF로 변환.

### web aggregator 시나리오 (s14 추가)

`web_search` + `web_fetch` 통합:

```yaml
name: web
description: |
  Web operations.
  Actions:
    - search (Brave/Perplexity 검색 결과 snippet)
    - fetch  (URL 가져와 텍스트 추출, SSRF 가드)
parameters:
  type: object
  properties:
    action: { enum: [search, fetch] }
    # search: query + count + provider
    # fetch: url + max_chars
```

**왜 web 통합이 file보다 더 명확한 후보인지**:

1. **둘 다 같은 cfg-flag 군집** — `ToolsWebFetchEnabled` + `ToolsWebSearchEnabled` 가 항상 한 묶음으로 켜지는 게 자연스러움 (web 능력 on/off).
2. **LLM 워크플로 일관성** — 검색 후 결과 fetch 패턴이 흔함. `web.search → web.fetch` 가 직관적.
3. **action 수 적음 (2개)** — schema 복잡도 낮음. oneOf 표현 단순.
4. **이미 process 빌트인 툴이 7 actions로 통합** (process.go) — 같은 패턴 재사용.

비용:
- description 합 ~80자 → ~50자 (-37%)
- schema ~500B → ~600B (+20%, oneOf 비용)
- 토큰 ~150 → ~120 (-30 토큰/turn)

**file (6 actions, 4 입력 도메인) 보다 web (2 actions, 2 입력 도메인) 이 schema 복잡도 낮음 → 첫 실험으로 더 안전**. ID-003 결정 시 web 부터 시작 권장.

---

## ID-005 — `internal/gateway` 패키지명 변경 (성격-명명 mismatch 해소)

- **Status**: open (사용자 결정 — 2026-04-25 session 21 진입 직전)
  - **결정**: 비용 크더라도 깔끔하게 변경 (사용자 명시 우선순위)
- **Related to**: `internal/gateway/` 전체, HTTP API `/v1/gateway/*`, config `gateway_*` 35+ 필드, 빌트인 툴 (message/nodes/gateway), 콘솔 UI
- **Discovered in**: [journal/2026-04-25-21-gateway-pt1.md](../journal/2026-04-25-21-gateway-pt1.md)

### 사용자 의도

> "패키지이름 변경하도록 계획 작성해줘. 비용이 크더라도 깔끔하게 가야 잠재문제 해소할 수 있음"

→ **hard cut** 방향. deprecation period 또는 alias 최소화. 한 번에 정리.

### 현재 mismatch

CLAUDE.md description (정확):
> `gateway` | **Agent execution platform**: runtime state machine, multi-threaded subagents (max 4), run persistence

vs 실제 명명:
- 패키지명 `gateway`
- 디렉토리 `internal/gateway/`
- URL prefix `/v1/gateway/*`
- config `gateway_*` 35+ 필드
- 변수명 `gatewayRuntime`/`GatewayRuntime`/`gatewayRuntimeForTelegram` 다수

→ 5 책임 (agent 실행 + run lifecycle + 채널 메시징 + consensus + persistence) 모두 *"실행 + 조정"* 성격, *"라우팅 + 변환"* 이 아님.

### 이름 후보 비교

| 후보 | 적합도 | 길이 | 충돌 가능성 | 메인 타입 정렬 |
|------|--------|------|------------|---------------|
| `internal/runtime/` | ⭐⭐⭐ | 짧음 | ⚠️ 다른 *runtime* (pulse/reflection/cron) 과 헷갈림 | `runtime.Runtime` (어색) |
| **`internal/agentruntime/`** | ⭐⭐⭐ | 중 | ✅ 충돌 없음 | `agentruntime.Runtime` |
| `internal/agent/` | ⭐⭐ | 짧음 | ⚠️ 기존 `internal/agent/` 패키지 존재 (loop_test.go 확인) | 충돌 |
| `internal/agentplatform/` | ⭐⭐ | 김 | ✅ 없음 | `agentplatform.Runtime` |
| `internal/orchestrator/` | ⭐⭐ | 중 | ✅ 없음 | `orchestrator.Runtime` (consensus 강조) |
| `internal/exechub/` | ⭐ | 짧음 | ✅ 없음 | execution + hub |

→ **`internal/agentruntime/` 권장**:
- 핵심 책임 (agent 실행) 정확히 표현
- 메인 타입 `Runtime` 과 정렬 (`agentruntime.Runtime`)
- 다른 패키지와 충돌 없음 (`internal/agent/` 와 다른 이름)
- CLAUDE.md description *"agent execution platform"* 과 어휘 정렬

### 6 Phase 작업 계획

총 ~7-10 일. 사용자 결정 (*"비용 크더라도 깔끔하게"*) → **hard cut**, deprecation alias 최소화.

#### Phase 0 — 사전 준비 (S, 1일)

- [ ] 이름 후보 최종 확정 (권장: `internal/agentruntime/` — 사용자 confirm 필요)
- [ ] HTTP URL prefix 결정:
  - **옵션 A**: `/v1/agentruntime/*` (긴 prefix)
  - **옵션 B**: `/v1/runtime/*` (짧음, 다만 의미 약간 모호)
  - **옵션 C**: hard cut + 1 release 동안 `/v1/gateway/*` alias 유지 (CHANGELOG 명시)
- [ ] config field naming:
  - **옵션 A**: `agent_runtime_*` prefix
  - **옵션 B**: `runtime_*` prefix (짧음)
  - 환경변수 (`TARS_GATEWAY_*`) 도 동일 규칙 적용
- [ ] 의존 grep 전수 → 호출자 인벤토리:
  ```bash
  rg -l 'internal/gateway|gateway\.' --type go > /tmp/gateway_callers.txt
  ```
- [ ] **Branch 명명**: `refactor/rename-gateway-to-agentruntime` (TARS GitHub Flow)

#### Phase 1 — 패키지 이름 변경 (M, 1-2일)

- [ ] `git mv internal/gateway internal/agentruntime`
- [ ] 패키지 선언 일괄 변경: `package gateway` → `package agentruntime`
- [ ] import path 일괄 변경:
  ```bash
  rg -l 'github.com/devlikebear/tars/internal/gateway' --type go | xargs sed -i '' \
    's|github.com/devlikebear/tars/internal/gateway|github.com/devlikebear/tars/internal/agentruntime|g'
  ```
- [ ] 식별자 사용 (`gateway.Runtime`, `gateway.Spawn`, `gateway.SpawnRequest`, etc.) 일괄 변경:
  ```bash
  # 패키지 alias 가 없으면:
  rg -l 'gateway\.' --type go | xargs sed -i '' 's|gateway\.|agentruntime\.|g'
  ```
- [ ] 컴파일 통과 (`make build`) + 단위 테스트 통과 (`make test`)

#### Phase 2 — 식별자 + 변수명 정리 (M, 2-3일)

타입명은 **유지** (Runtime/RuntimeOptions/SpawnRequest/AgentExecutor 등) — 패키지 컨텍스트로 명확. 단:

- [ ] 변수명 `gatewayRuntime` / `GatewayRuntime` / `gatewayRuntimeForTelegram` → `agentRuntime` 등 일괄 변경
- [ ] handler 파일명: `handler_gateway_*.go` → `handler_agentruntime_*.go` (또는 `handler_runtime_*.go`)
- [ ] `tool/tool_gateway.go` → `tool/tool_runtime.go` (RF-040 외부화 후보와 결합 검토)
- [ ] `tool/tool_subagents*` 의 `gatewayRuntime` 인자 변수명
- [ ] `helpers_pulse.go` 의 `GatewayRuntime` 필드명
- [ ] 테스트 파일명 일괄 (`gateway_*_test.go` → `agentruntime_*_test.go`)
- [ ] 컴파일 + 테스트 재통과

#### Phase 3 — Config 필드 마이그레이션 (M, 2일)

`internal/config/` 의 35+ `gateway_*` 필드:

- [ ] `GatewayEnabled` / `GatewayPersistenceEnabled` / `GatewaySubagentsMaxThreads` 등 모두 `AgentRuntime*` 또는 `Runtime*` 으로 변경
- [ ] YAML 키 매핑 (`gateway_enabled` → `agent_runtime_enabled` 등)
- [ ] 환경변수 (`TARS_GATEWAY_*` → `TARS_AGENT_RUNTIME_*`)
- [ ] **호환성 정책 (사용자 결정)**:
  - **옵션 A (hard cut)**: 옛 키 즉시 제거. 사용자 워크스페이스의 `tars.config.yaml` 마이그레이션 필요
  - **옵션 B (1 release alias)**: `gateway_*` 키 deprecation warning 1 릴리스 후 제거
- [ ] 기본 config (`config/standalone.yaml`) 갱신
- [ ] CHANGELOG 항목 + 마이그레이션 가이드 (mv ~/.tars/...)

#### Phase 4 — HTTP API 변경 (M, 1-2일)

`/v1/gateway/*` 7+ 엔드포인트 → 새 prefix:

- [ ] handler_gateway_*.go 의 `mux.HandleFunc("/v1/gateway/...")` 일괄 변경
- [ ] 콘솔 UI (`frontend/console/src/lib/api.ts`) 의 fetch URL 변경
- [ ] **호환성 정책 (사용자 결정)**:
  - **옵션 A**: hard cut, 콘솔 UI 도 동시 마이그레이션
  - **옵션 B**: `/v1/gateway/*` 를 1 릴리스 동안 redirect 또는 alias 유지
- [ ] OpenAPI/문서 갱신 (있다면)
- [ ] 외부 caller (스크립트, 외부 콘솔) 마이그레이션 가이드

#### Phase 5 — 빌트인 툴 + 콘솔 UI 정리 (M, 1일)

- [ ] `tool/tool_gateway.go` 의 3 빌트인 툴 (`message`/`nodes`/`gateway`) — RF-040 외부화 결정과 결합:
  - 외부화면: 별도 skill+CLI 로 분리
  - 통합/유지면: 이름만 변경 (`gateway` → `runtime`)
- [ ] 콘솔 UI 의 페이지명/라벨 (`Gateway Status` → `Agent Runtime` 등)
- [ ] 콘솔 SSE 이벤트 type (`run_started` 등은 변경 없음, prefix 만 변경)
- [ ] `internal/extensions/manager.go` 의 `CollectHTTPHandlers` 가 영향받는지 확인

#### Phase 6 — 잔재 정리 + 검증 (S, 1일)

- [ ] 남은 "gateway" 문자열 grep — 코멘트, 변수명 잔재, 테스트 라벨, 로그 message:
  ```bash
  rg -i 'gateway' --type go | grep -v 'CHANGELOG\|gateway runtime is disabled\|migration'
  ```
- [ ] persistence 디렉토리 (`workspace/_shared/gateway/`) — config 의 `GatewayPersistenceDir` default 변경 + 기존 디렉토리 마이그레이션 권장 (또는 호환성 보존)
- [ ] CLAUDE.md 갱신:
  - "Key internal packages" 표의 `gateway` → `agentruntime`
  - 본문의 `gateway` 모든 언급 정리
- [ ] README + 문서 + journal 업데이트
- [ ] 전체 CI 통과 + e2e 검증

### 주의사항 + 잠재 리스크

1. **`workspace/_shared/gateway/` 영구화 디렉토리** — 사용자 워크스페이스에 이미 데이터 있음. 마이그레이션 전략:
   - **옵션 A**: startup 시 `gateway/` → `agentruntime/` 자동 rename
   - **옵션 B**: deprecation warning + 사용자 수동 mv
   - **옵션 C**: 양쪽 경로 fallback read

2. **`/v1/gateway/*` 외부 caller** — TARS 외부 모니터링/스크립트가 이 URL 을 사용하고 있을 수 있음. CHANGELOG 명시 필요.

3. **Tool name `gateway`** — `tool/knowledge_aggregator.go` 같은 다른 aggregator 가 *"use gateway tool"* 같은 description 에서 언급할 가능성. grep 필요.

4. **테스트 fixture/snapshot** — JSON snapshot 에 `"gateway"` 가 박혀 있을 가능성. e2e 테스트 회귀.

5. **plugin 정의 파일** (`plugin.yaml` 등) — 외부 플러그인이 `gateway_*` config 또는 URL 사용 시.

### 영향 범위 요약

| 영역 | 변경 규모 | 자동화 가능 |
|------|----------|-------------|
| Go 패키지 import | 50+ 파일 | sed 일괄 |
| 식별자 사용 (`gateway.X`) | 100+ 호출 | sed 일괄 |
| 변수명 (`gatewayRuntime` 등) | 50+ 줄 | 부분 자동화 |
| 파일명 (`handler_gateway_*`) | ~10 파일 | git mv |
| Config 필드 35+ | 35+ 필드 | sed + 호환 alias |
| HTTP URL prefix | 7+ 엔드포인트 | sed |
| 콘솔 UI fetch | 7+ 엔드포인트 | sed |
| persistence 디렉토리 | 1 경로 | startup 마이그레이션 |
| CLAUDE.md / README / 문서 | ~50줄 | 수동 |

**총 작업량**: M+ (큰 단일 PR 또는 4-5개 phase 별 PR)

### 추천 작업 순서

PR 분할 권장 (테스트 간격 짧게):

```
PR #1: Phase 1 (패키지 이름 + import path) — 큰 PR, 자동화 위주
PR #2: Phase 2 (식별자 + 파일명) — 자동화 + 수동 검토
PR #3: Phase 3 (config) — config 호환성 결정 포함
PR #4: Phase 4 + 5 (HTTP URL + 콘솔 UI + 빌트인 툴) — UI 동시 마이그레이션
PR #5: Phase 6 (잔재 정리 + CLAUDE.md)
```

각 PR 마다:
- VERSION.txt bump (RF-019 카테고리)
- CHANGELOG entry
- e2e 테스트 회귀

### 결정 인풋 (사용자 confirm 필요)

1. **이름 최종 확정** — `agentruntime` (권장) vs `runtime` vs 기타
2. **HTTP URL prefix** — `/v1/agentruntime/*` vs `/v1/runtime/*` vs alias 기간
3. **Config field 호환성** — hard cut vs 1 release alias
4. **persistence 디렉토리 마이그레이션** — auto-rename vs 수동 가이드
5. **PR 분할 vs 단일 PR** — 분할 추천 (각 phase 별 e2e 검증)

### 관련 finding

- RF-040 — `tool/tool_gateway.go` 의 3 빌트인 툴 외부화/통합 (Phase 5 와 결합)
- RF-007 — 빌트인 플러그인 제거 (gateway 의존하는 browserplugin 정리)
- ID-003 — 빌트인 툴 통합 정책 (Phase 5 의 결정 인풋)

### Phase 0 의 사전 결정 (s21 종료 시점에 추가 정리 필요)

s21~s23 의 gateway 분석 진행하면서 다음을 추가로 정리:
- 변경되지 않을 식별자 vs 변경할 식별자
- migration script 자동화 가능 영역
- 회귀 위험 큰 영역 (consensus / persistence / channel)

---

## ID-004 — OpenAI / Gemini provider capability 를 anthropic 수준으로 끌어올리기 (OpenAI 우선)

- **Status**: open (사용자 요청 — 2026-04-25 session 19 후속)
- **Related to**: `internal/llm/openai_compat_client.go`, `openai_codex_client.go`, `gemini_native*.go`, [RF-042](refactor.md#rf-042) (strict mode), [RF-046](refactor.md#rf-046) (PDF placeholder), [RF-047](refactor.md#rf-047) (capability 매트릭스)
- **Discovered in**: [journal/2026-04-25-19-llm-provider-1.md](../journal/2026-04-25-19-llm-provider-1.md) + 사용자 요청

### 사용자 의도

> "openai gemini도 anthropic 수준으로 끌어올려야 할 것 같아. 특히 요즘은 openai codex가 부상하고 있으니 무엇보다 openai는 완성도와 지원을 높게 끌어올려야 함"

→ **OpenAI > Gemini ≫ claude-code-cli** 우선순위로 capability 격차 해소.

### 현재 격차 (s19 + s20 정량 분석 완료)

| 기능 | anthropic | openai (compat) | openai-codex | gemini (compat) | gemini-native | claude-code-cli |
|------|-----------|-----------------|--------------|-----------------|---------------|----------------|
| Streaming | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Tool calling | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ |
| **ToolChoice (specific tool)** | ✅ `{type:"tool", name:"X"}` | ❌ string only | ❌ **hardcoded "auto"** [RF-048] | ❌ string only | ⚠️ Mode string only | ❌ |
| **Strict structured output** | ✅ (tool 강제) | ❌ | ❌ | ❌ | ❌ (responseSchema 미구현) | ❌ |
| **Reasoning effort** | ❌ | ✅ | ❌ silent ignore [RF-049] | ❌ skip | ✅ ThinkingLevel | ❌ |
| **Thinking budget** | ✅ | ❌ | ❌ silent ignore [RF-049] | ❌ skip | ✅ ThinkingBudget | ❌ |
| Service tier | ❌ | ✅ | ❌ silent ignore [RF-049] | ❌ skip | ❌ | ❌ |
| **Prompt caching** | ✅ 자동 ephemeral | ⚠️ usage 만 노출 | ⚠️ usage 만 노출 | ❌ | ❌ (자동 caching 미명시) | ❌ |
| Multimodal image | ✅ native | ✅ data URL | ✅ input_image | ✅ data URL | ✅ inlineData | ❌ |
| **Multimodal PDF** | ✅ native | ⚠️ placeholder | ⚠️ placeholder | ⚠️ placeholder | ✅ **inlineData** | ❌ |
| **OAuth refresh** | ❌ (api-key) | ❌ | ✅ 자동 갱신 | ❌ | ❌ | N/A |

### s20 핵심 발견 (provider 2차 검토 결과)

#### gemini-native 가 의외로 anthropic 다음으로 풍부

- **PDF 지원** ✅ — inlineData 형식으로 native (anthropic 과 동급)
- **Reasoning + Thinking 양쪽 지원** ✅ — `ThinkingLevel` (effort) + `ThinkingBudget` (budget) 둘 다. **anthropic 보다 더 풍부** (anthropic 은 budget 만)
- 미구현: structured output (responseSchema), 자동 caching 명시화
- → ID-004 의 gemini 작업 우선순위는 **낮음**. responseSchema 1개만 추가하면 거의 anthropic 동급

#### openai-codex 가 가장 빈약 ⚠️

- `tool_choice: "auto"` hardcoded [RF-048]
- ChatOptions 의 ReasoningEffort/ThinkingBudget/ServiceTier 모두 silent 무시 [RF-049]
- structured output 미구현
- PDF placeholder
- → **OpenAI Codex 가 capability 격차 50%+ 가장 큼**. 사용자 ID-004 우선순위 *"OpenAI 1순위"* 정확함 — codex 부터.

#### Provider 별 capability 점수 (10점 만점)

| Provider | Tool capabilities | Reasoning | Multimodal | Caching | 합산 |
|----------|------------------|-----------|------------|---------|------|
| anthropic | 4/4 | 2/3 (no effort) | 3/3 | 3/3 (auto) | **12/13** |
| gemini-native | 2/4 (Mode only) | 3/3 | 3/3 | 1/3 | **9/13** |
| openai-compat | 2/4 (string only) | 2/3 (no thinking) | 2/3 (PDF placeholder) | 1/3 | **7/13** |
| openai-codex | 1/4 (hardcoded) | 0/3 (silent ignore) | 2/3 (PDF placeholder) | 1/3 | **4/13** ⚠️ |
| gemini-compat | 2/4 (string only) | 0/3 (skip) | 2/3 | 0/3 | **4/13** ⚠️ |
| claude-code-cli | 0/4 | 0/3 | 0/3 | 0/3 | **0/13** (CLI 자체 도구) |

→ **openai-codex + gemini-compat 이 가장 빈약**. openai-codex 는 사용자 우선순위 1, gemini-compat 은 gemini-native 가 더 풍부하므로 deprecate 후보 [Phase 5].

### OpenAI 우선순위 — 작업 항목

사용자가 명시적으로 *"OpenAI 는 완성도와 지원을 높게 끌어올려야 함"*. 항목별 작업:

#### (1) OpenAI specific tool name 강제 (ToolChoice 확장) — RF-042 핵심 의존

**현재**:
```go
// ChatOptions
ToolChoice string  // "auto" / "none" / "required" 만
```

**목표**:
```go
type ToolChoice struct {
    Mode string  // "auto" / "none" / "required" / "specific"
    Name string  // Mode == "specific" 시
}
// 또는 string 확장 (e.g. "tool:my_tool_name")
```

OpenAI wire format:
```json
{ "tool_choice": { "type": "function", "function": { "name": "my_tool" } } }
```

→ RF-042 (subagents_plan normalization 단순화) 의 OpenAI 환경 적용 가능. anthropic 은 이미 지원 (s19 확인).

#### (2) OpenAI structured output (`response_format: { type: "json_schema", strict: true }`)

**현재**: 미지원. JSON schema 강제 검증 없음.

**목표**:
```go
// ChatOptions
ResponseFormat *ResponseFormat
type ResponseFormat struct {
    Type   string          // "text" / "json_object" / "json_schema"
    Schema json.RawMessage // strict 검증할 JSON schema
    Strict bool            // OpenAI strict mode
}
```

OpenAI wire format:
```json
{ "response_format": { "type": "json_schema", "json_schema": { "name": "X", "schema": {...}, "strict": true } } }
```

→ RF-042 의 또 다른 단순화 경로. LLM 출력이 schema 강제 → JSON parsing 폴백 + ID sanitize 일부 redundant.

#### (3) OpenAI prompt caching 명시화

**현재**: OpenAI 자체 자동 caching → usage 의 `prompt_tokens_details.cached_tokens` 노출. 다만 anthropic 같은 명시적 `cache_control` 마크 없음.

**확인 필요** (s20):
- OpenAI 가 자동 prefix matching caching 인지 (현재) vs 명시적 cache 마커 지원 여부
- 만약 명시적 마커 없으면 *"already automatic"* 으로 그대로 OK (작업 0)
- 자동 caching 정책에 영향 미치는 caller 패턴 문서화

#### (4) OpenAI thinking budget 또는 reasoning effort 통합

**현재**:
- anthropic: `thinking: { type: "enabled", budget_tokens: N }`
- openai: `reasoning_effort: "low|medium|high"`

→ **다른 메커니즘** 이지만 ChatOptions 의 `ThinkingBudget` (int) 와 `ReasoningEffort` (string) 가 별도 필드. 의미 매핑:
- caller 가 *"reasoning 강도"* 를 표현하는 단일 추상화 + provider 별 wire format 변환
- `ChatOptions.ReasoningStrength` 같은 통합 필드 (`none|low|medium|high|extreme`) → anthropic 은 budget tokens, openai 는 reasoning_effort

→ 또는 둘 다 유지 + provider 가 자기에게 맞는 것 사용. 단순화 vs 명시성 트레이드오프.

#### (5) OpenAI multimodal PDF 지원 (RF-046 해소)

**현재**: silent text placeholder.

**목표**:
- OpenAI Files API 통합: PDF 업로드 후 file_id 참조
- 또는 자체 PDF 텍스트 추출 후 text block 으로 전송 (작은 PDF 만)
- 또는 명시적 에러 (*"PDF unavailable in OpenAI; use anthropic"*)

→ RF-046 와 통합. caller 가 PDF 첨부 시 명시적 fallback 정책.

### Gemini 우선순위 (OpenAI 다음)

`gemini-compat` (openai_compat_client 통과) 는 `c.label != "gemini"` 분기 때문에 reasoning/service_tier 안 보냄. 다만 `gemini-native` 별도 client 가 있음 (s20 검토 예정).

작업:
- **gemini-native 의 자체 capability 확인** (s20):
  - native API 의 strict mode (responseSchema?) 지원 여부
  - prompt caching 지원 여부
  - thinking budget 지원 여부 (Gemini 2.5 thinking)
- **gemini-compat 은 deprecate 후보 가능** — gemini-native 가 우월하면 compat 제거 검토

### OpenAI Codex 특별 고려 (사용자 강조)

`openai_codex_client.go` 858줄 — 가장 큰 provider 파일. ChatGPT backend (OAuth-only) 통합. s20 검토 시:

- Codex API 가 OpenAI 의 일반 chat completions 와 다른 endpoint
- OAuth flow 의 무게 (api-key 가 아님)
- Codex 가 가진 추가 capability (sandbox 환경, web access, etc.) 가 ChatOptions 에 노출 안 될 가능성

**사용자 우선순위 적용**: openai-codex 가 OpenAI 의 advanced 변종 — anthropic 수준에 도달하려면 codex 부터.

### 작업 phase 제안

#### Phase 1 (RF-042 기반, OpenAI strict tool choice + structured output)

- ChatOptions.ToolChoice 를 object 로 확장 (string + specific tool name)
- ChatOptions.ResponseFormat 추가
- openai_compat_client + openai_codex_client 둘 다 wire format 변환
- subagents_plan + 다른 LLM-as-planner 호출자가 활용
- 비용: M, 효용: ↑↑ (RF-042 직접 해소)

#### Phase 2 (multimodal 격차 해소)

- RF-046 해소 (PDF 지원 또는 명시적 에러)
- OpenAI Files API 통합 검토
- 비용: M

#### Phase 3 (capability 매트릭스 통합)

- RF-047 의 매트릭스 정리 + 격차 줄이기
- gemini-native 의 capability 평가 후 gemini-compat deprecate 결정
- 비용: S-M

#### Phase 4 (Gemini-native 강화)

- gemini-native 의 strict / caching / thinking 지원 노출
- 비용: M (gemini-native 가 이미 별도 client 라 단순)

### 정량 데이터 권장

ID-004 결정 전:
1. **provider 별 사용 빈도** (usage tracker) — anthropic vs openai vs openai-codex vs gemini vs gemini-native vs claude-code-cli. 1주/1개월
2. **각 capability 의 caller 호출 빈도** — ToolChoice specific / ResponseFormat / PDF 첨부
3. **OpenAI Codex 의 OAuth flow 안정성** — 토큰 갱신 / 만료 / 에러율

### 우선순위 요약

```
[강제 1순위] OpenAI ToolChoice specific tool + ResponseFormat (RF-042 의존)
[강제 2순위] OpenAI Codex 의 capability 1순위 항목들 적용 (사용자 우선순위)
[2순위]      OpenAI multimodal PDF 지원 (RF-046)
[3순위]      Gemini-native capability 끌어올리기
[4순위]      capability 매트릭스 통합 (RF-047)
[5순위]      gemini-compat deprecate 결정 (gemini-native 우월 시)
```

### 관련 finding

- [RF-042](refactor.md#rf-042) — subagents_plan normalization 단순화 (이 ID 의 직접 의존)
- [RF-046](refactor.md#rf-046) — PDF silent placeholder
- [RF-047](refactor.md#rf-047) — capability 매트릭스 문서화 (이 ID 의 first deliverable)
- [Q-013](questions.md#q-013) — claude-code-cli 빈도 (이 ID 의 외부화 후보 평가)

### Aggregator schema 패턴 비교 (s15 추가)

기존 aggregator 6개의 schema 작성 방식을 비교한 결과 — file/web 통합 시 **`process` 패턴 권장**:

| Aggregator | Properties 명시 | additionalProperties | LLM 호출 정확도 |
|-----------|----------------|---------------------|----------------|
| `memory` | action만 | true (sub-params 추측) | ⚠️ 낮음 — schema 약함 |
| `knowledge` | action만 | true | ⚠️ 동일 |
| `workspace` | action만 | true | ⚠️ 동일 |
| `automation` | action + 일부 | mixed | 중 |
| `session` | action + 일부 | mixed | 중 |
| **`process`** | action + session_id + chars (모든 sub-param 명시) | false | ✅ **높음** |

→ **`memory`/`knowledge`/`workspace` 는 partial schema** — LLM 이 sub-action params 추측. strict tool calling 모드에서 schema validation 약함.

→ **`process` 패턴**: 모든 sub-action param을 top-level properties에 명시 + action enum 으로 분기. schema 살짝 두꺼워지지만 LLM 호출 정확도 ↑.

**file/web aggregator 만들 때 따라야 할 패턴**:
```yaml
name: file
parameters:
  type: object
  properties:
    action: { enum: [read, write, edit, list, glob] }
    path: { type: string }
    content: { type: string }      # write 만
    old_text: { type: string }     # edit 만
    new_text: { type: string }     # edit 만
    replace_all: { type: boolean } # edit 만
    pattern: { type: string }      # glob 만
    offset: { type: integer }      # read 만
    limit: { type: integer }       # read/list/glob 공통
    recursive: { type: boolean }   # list 만
  required: [action]
  additionalProperties: false
```

→ schema 사이즈 ~600B (vs 현재 6 분리 툴의 ~1.2KB) — 50% 감소. LLM 정확도는 process 사례로 입증.

### subagents 통합 케이스 (s17 추가)

`subagents_run` + `subagents_plan` + `subagents_orchestrate` 3 빌트인 툴 — 1512줄, 모두 같은 도메인 (gateway 위 multi-agent 워크플로):

| 툴 | 역할 | 입력 |
|----|------|------|
| `run` | 단순 fan-out (parallel/consensus) | tasks 배열 |
| `plan` | LLM-as-planner — goal → flow JSON | goal + targets + constraints |
| `orchestrate` | plan 실행 (단계별 + placeholder rendering) | subagentFlowInput |

**핵심**: `plan` 의 출력 = `orchestrate` 의 입력 (같은 `subagentFlowInput` schema). 즉 의미적으로도 통합이 정당.

→ **`subagents` 단일 aggregator** (action: run/plan/orchestrate)는 **ID-003 의 4번째 명확한 통합 케이스** [RF-041]. process 패턴 적용.

### 통합 케이스 누적 (s13 + s14 + s16 + s17)

| 케이스 | actions | LLM ergonomic | 통합 우선순위 |
|-------|---------|--------------|--------------|
| `web` (s14) | search / fetch | 매우 강 | ★★★ (첫 실험 권장) |
| `file` (s13) | read / write / edit / list / glob (+ apply_patch) | 강 | ★★ |
| `subagents` (s17) | run / plan / orchestrate | 강 — 이미 같은 schema | ★★ |
| `gateway` (s16) | message_send / message_read / nodes_status / etc (9) | 중 — admin 빈도 낮음 | ★ (외부화 우선 고려) |

→ **통합 시 시스템 프롬프트 절감** 합산 추정:
- web (-30 토큰) + file (-150) + subagents (-300) + gateway (-150) ≈ **-630 토큰/turn**
- 매일 100 turn 가정 시 **63k 토큰/일** (default chat session 베이스)

ID-003 결정의 정량 임팩트 큼.

### 추가 권고 — partial schema aggregator 보강

`memory` / `knowledge` / `workspace` 의 partial schema 도 `process` 패턴으로 마이그레이션 검토. 별도 RF로 분리 가능. 현재 LLM 의 sub-action 추측은 *경험적으로 잘 됨* 일 수 있지만, schema validation 약함은 long-tail 버그.

### Tension 가능성

CLAUDE.md 의 *"Every tool ... emits its description into the chat system prompt"* 비용 의식 + *"infrastructure that every session needs"* 기준이 명시됐지만, 빌트인 툴 추가/통합 정책이 없음. 이 ID-003이 그 gap을 메우는 결정.

<!--
템플릿:

## ID-001 — <한 줄 제목>

- **Status**: open | shipped | rejected | deferred
- **Related to**: <관련 패키지 또는 다른 finding ID>
- **Discovered in**: [journal/YYYY-MM-DD-NN-topic.md](../journal/YYYY-MM-DD-NN-topic.md)

### 무엇
...

### 왜 (가치)
...

### 가설/대안
...
-->
