# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog and the project follows Semantic Versioning.

## [Unreleased]

## [0.31.8] - 2026-04-26

### Added

- `tasks_changed` SSE event — emitted after every `tasks` tool call with the live plan-goal + per-status counts so the chat pulse-bar Tasks badge stays in sync without polling.
- Initial task counts are fetched on session change so the badge reflects state from prior turns, not just the current chat-stream lifetime.

### Changed

- The chat pulse-bar `Tasks` button now displays an `(in_progress / total)` count when a plan exists (e.g. `Tasks (1/3)`) — restores the live counter that PR #291 promised but had since regressed.

### Tests

- `TestChatAPI_TasksToolEmitsTasksChangedEvent`

### Closed

- Closes #391.

## [0.31.7] - 2026-04-26

### Added

- Hardcoded `## Planning` section in the main-agent system prompt that instructs the LLM to use the `tasks` aggregator (`plan_set` / `add` / `update`) for multi-step requests. Sub-agent prompts skip the section.

### Tests

- `TestBuild_PlanningSectionPresentForMainAgent`
- `TestBuild_PlanningSectionAbsentForSubAgent`
- `TestBuild_PlanningSectionWithinBudget`

### Closed

- Closes #390.

## [0.31.6] - 2026-04-26

### Added

- Workspace-local usage signal counters in `workspace/usage/signals-YYYY-MM-DD.jsonl` for unresolved code-review questions.
- `GET /v1/usage/signals?period={today|week|month}` and `/usage signals {period}` for operator inspection.
- Narrow counters for tool calls, session tool-config updates, agent runtime persistence retries/errors, and consensus activation.
- `docs/usage-signals.md` mapping Q-011 through Q-018 to their runtime evidence source.

### Tests

- `TestTracker_RecordSignalAndSummarize`
- `TestUsageAPI_Signals`

### Closed

- Closes #386.

## [0.31.5] - 2026-04-26

### Changed (BREAKING)

- **ID-005 hard cut**: external gateway naming is now canonical `agentruntime` across HTTP routes, console routes, config/env/YAML keys, persistence paths, CLI commands, tool names, and public client types.
- Runtime persistence now defaults to `workspace/_shared/agentruntime/`, and archive files use the `agentruntime-*.jsonl` prefix.
- The legacy `/v1/gateway/*` routes and `gateway_*` / `gateway.*` config keys are intentionally not kept as compatibility aliases.

### Migration

- Replace `/v1/gateway/*` calls with `/v1/agentruntime/*` and `/console/gateway` bookmarks with `/console/agentruntime`.
- Rename `gateway_*` env/config keys and `gateway.*` YAML sections to `agentruntime_*` / `agentruntime.*`.
- Move retained runtime state from `workspace/_shared/gateway/` to `workspace/_shared/agentruntime/` if old run/channel history must be preserved.

### Tests

- `TestAgentRuntimeAPIHandler_HardCutRoutes`
- `TestLoad_AgentRuntimeHardCutIgnoresLegacyGatewayConfigKeys`

### Closed

- Closes #384.

## [0.31.4] - 2026-04-26

### Changed

- **RF-004**: `runtimeDeps` no longer stores a backward-compat `llmClient`. Server bootstrap keeps only the shared `llmRouter`, and chat/API call sites resolve the chat client through `llm.RoleChatMain` at the boundary where the client is needed.
- Agent Runtime prompt runners now use the router-backed default agent runtime role path instead of inheriting the chat-main client fallback from bootstrap. Session-bound cron and Telegram inbound paths use the same resolved chat client as the normal chat handler.

### Tests

- `TestRuntimeDepsDoesNotExposeLegacyLLMClient`
- `TestChatAPI_ResolvesChatClientFromRouter`

### Closed

- Closes #385.

## [0.31.3] - 2026-04-26

### Changed (BREAKING)

- **ID-005 PR #1 — package rename `internal/gateway` → `internal/agentruntime`**:
  - `git mv internal/gateway internal/agentruntime`
  - 모든 import path `github.com/devlikebear/tars/internal/gateway` → `github.com/devlikebear/tars/internal/agentruntime`
  - 식별자 사용 `gateway.X` → `agentruntime.X` 일괄 변경 (Runtime / SpawnRequest / AgentInfo / Run / RunStatus 등 50+ 위치)
  - 패키지 선언 `package gateway` → `package agentruntime`

### Migration

- 외부 코드가 `internal/gateway` 를 import 하던 경우 (워크스페이스 외부) `internal/agentruntime` 로 변경 필요. 단 `internal/*` 는 정의상 사용자 외부 import 불가.
- **변경 안 됨 (이번 PR)**: HTTP URL prefix (`/v1/gateway/*` 그대로), Config 필드 (`gateway_*` 그대로), workspace persistence dir (`workspace/_shared/gateway/` 그대로), 변수명/파일명/콘솔 UI 라벨. 후속 PR (Phase 2-6) 에서 단계적으로 마이그레이션.

### Why split

5 PR 분할 의 첫 단계. 패키지 이름만 먼저 바꿔 외부 호환성 (HTTP/config/persistence) 은 그대로 유지 — 각 단계에서 e2e 검증 가능. ID-005 옵션 결정은 #367 / #378.

## [0.31.2] - 2026-04-26

### Changed

- **ID-004 wrap-up — RF-049 codex advanced fields + RF-047 capability docs**:
  - `openai_codex_client.go`: `buildOpenAICodexRequestBody` 가 이제 `ClientConfig` 를 받아 `effectiveReasoningEffort` / `effectiveServiceTier` 매핑. Responses API 의 `reasoning: {effort: ...}` 객체와 `service_tier` 필드로 직렬화. 이전엔 `ReasoningEffort` / `ServiceTier` 옵션이 silent 무시 (RF-049).
  - `docs/llm-providers.md` 신규 — ChatOptions × Provider capability 매트릭스 + wire format 디테일 (specific tool / json_schema / reasoning / service_tier 표). 향후 `internal/llm` 의 wire-format converter 변경 시 같은 PR 에서 갱신할 것 (문서에 명시) (RF-047).

### Tests

- `TestOpenAICodexClient_ReasoningEffortAndServiceTier` — `reasoning.effort=high` 객체 + `service_tier=priority` 직렬화 검증.

### Closed (#366 / ID-004 종결)

이 PR 로 ID-004 옵션 (2) Phase 1+2 + 작은 후속 (RF-049/047) 완전 종결. 잔여 항목 (옵션 (3) 의 Gemini-native responseSchema/caching, 옵션 (5) 의 gemini-compat deprecate) 은 별도 RF/issue 로 트래킹하거나 사용자 결정 시 재개.

## [0.31.1] - 2026-04-26

### Changed

- **ID-004 Phase 2 — openai_codex parity + PDF + default model (RF-046 / RF-048 / RF-064)**:
  - `openai_codex_client.go`: 하드코딩된 `tool_choice="auto"` 제거 → `toOpenAIToolChoice(opts.ToolChoice)` 헬퍼 사용 (Phase 1 와 동일 wire format). Caller 가 nil 을 보내면 `ToolChoiceAuto()` 로 fallback 해 종전 행동 유지.
  - `openai_codex_client.go`: 신규 `toCodexTextFormat` — Responses API 의 `text.format` 봉투 (Chat Completions 의 `response_format` 과는 다름). `json_schema` 변형은 봉투 최상위에 `name/schema/strict` 펼침.
  - **PDF placeholder 명시 에러 (RF-046)**: `openai_compat_client.go` / `openai_codex_client.go` 가 메시지에 `ContentBlocks[*].Type == "document"` 가 있으면 build 단계에서 `pdf_unsupported_by_provider` ProviderError 를 반환. 이전엔 `[Attached PDF document]` placeholder 로 조용히 흘려보냈음 — 모델은 그걸 throwaway 노트로 취급해 사용자 의도 손실.
  - **Default Anthropic 모델 갱신 (RF-064)**: `defaultAnthropicModel` `claude-3-5-haiku-latest` → `claude-haiku-4-5-20251001`. Claude 4.x 가 현재 최신 패밀리. `defaultOpenAIModel` (gpt-4o-mini), `defaultGeminiModel` (gemini-2.5-flash) 는 현재 가용 최신으로 유지.

### Tests

- `TestOpenAICodexClient_ToolChoice_Specific` — 객체 wire format 검증
- `TestOpenAICodexClient_ResponseFormat_JSONSchema` — `text.format` 봉투 + strict 검증
- `TestOpenAICodexClient_PDFUnsupportedError` / `TestOpenAICompatibleChat_PDFUnsupportedError` — PDF 차단 에러 검증

### Out of scope (별도 PR)

- Provider capability 매트릭스 문서화 (RF-047)
- Gemini-native responseSchema / caching (Phase 3+)
- Codex ChatOptions silent ignore 잔여 (RF-049)
- Provider registry (RF-066) / yaml_paths DRY (RF-065)

## [0.31.0] - 2026-04-26

### Changed (BREAKING)

- **ID-004 Phase 1 — ChatOptions strict tool & response controls (RF-042 / RF-048 partial)**: `llm.ChatOptions.ToolChoice` 가 `string` → `*llm.ToolChoice` 구조체로 변경. 새 헬퍼 `llm.ToolChoiceAuto/None/Required/Specific(name)` 로 모든 호출자 마이그레이션. `ChatOptions.ResponseFormat *llm.ResponseFormat` 신규 필드 (text / json_object / json_schema, OpenAI-style strict 토글).
- **OpenAI compat client**: `tool_choice` 가 mode 별로 정확한 wire format 으로 직렬화. specific tool 은 `{"type":"function","function":{"name":"…"}}` 객체. `response_format` 은 `{"type":"json_schema","json_schema":{"name","schema","strict"}}` 봉투. 이전엔 string 만 그대로 forwarding 했고 specific tool 은 사실상 미지원이었음.
- **Anthropic / Gemini-native**: 새 `*ToolChoice` 받게 변환 함수 시그니처 업데이트. anthropic 은 specific tool 이미 지원 (`tool_choice: {type: tool, name: …}`). gemini-native 는 specific 일 때 `functionCallingConfig.mode=ANY + allowedFunctionNames=[name]` 으로 매핑 (Google Live API 표준).
- **subagents_plan (RF-042 직접 해소)**: planner Chat 호출에 `ResponseFormat: json_schema` 적용. OpenAI-호환 planner 에서는 markdown 펜스/문장 잡음 없이 schema-검증된 JSON 출력. 기존 정규화 레이어 (id 유일성, 참조 재작성) 는 그대로 유지.
- `agent.RunOptions` 에 `ResponseFormat *llm.ResponseFormat` 신규 필드 (loop 전 iteration 으로 전달).

### Migration

`ChatOptions.ToolChoice = "required"` 같은 string 호출 코드는 더 이상 컴파일되지 않음. `llm.ToolChoiceRequired()` / `llm.ToolChoiceAuto()` / `llm.ToolChoiceNone()` / `llm.ToolChoiceSpecific("tool_name")` 헬퍼로 교체. ResponseFormat 사용 시: `&llm.ResponseFormat{Type: llm.ResponseFormatJSONSchema, Name: "…", Schema: rawJSON, Strict: true}`.

### Out of scope (Phase 2 — 별도 PR)

- `openai_codex_client.go` 의 `tool_choice="auto"` 하드코딩 제거 (RF-048 잔여)
- PDF placeholder 명시 에러 (RF-046)
- Default model 갱신 (RF-064)

## [0.30.2] - 2026-04-26

### Reverted

- **PR #377 (ID-003 B web aggregator)** 전체 revert. `web_search` 와 `web_fetch` 가 다시 분리 툴로 복귀. `web` 단일 aggregator + `web_search`/`web_fetch` alias + tool_groups 변경 모두 되돌림.

### Why

`web_search` 와 `web_fetch` 는 LLM workflow 상 성격이 다른 작업 (snippet 탐색 vs URL 본문 가져오기) 이고, 더 큰 정책상 *위험도가 다른 빌트인 툴은 단일 aggregator 로 묶지 않는다* 는 결정 (file aggregator 검토 중 발견된 권한 모델 한계 — `read_file → file` alias 가 `ToolsEnabled` allowlist 정밀도를 깨뜨림 + high-risk 분류 불가능). 같은 사유로 ID-003 issue 자체 폐기.

### Migration

기존 `web` 호출 LLM 코드/스킬은 다시 `web_search` / `web_fetch` 분리 호출로 돌아가야 함. (PR #377 머지 직후 한 세션 분량이라 외부 영향 거의 없음.)

## [0.30.0] - 2026-04-26

### Removed (BREAKING)

- **ID-001**: Knowledge Base (KB Wiki) 시스템 전체 제거. chat path 통합 0% 정량 증거 + KB note semantic 인덱스 등록 0% + read 통합 ~0% → 사용자 결정 *"완전 제거. 필요할 때 다시 구현하겠음"*.
  - `internal/memory/knowledge.go` (840 줄) + `internal/memory/knowledge_test.go` (137 줄) 삭제. `KnowledgeStore`, `KnowledgeNote`, `KnowledgeUpdate`, `KnowledgeListOptions`, `KnowledgeNotePatch`, `KnowledgeGraph`, `KnowledgeGraphNode`, `KnowledgeGraphEdge`, `KnowledgeLink` 모두 삭제.
  - `internal/tool/knowledge_aggregator.go` + `internal/tool/memory_kb.go` + `internal/tool/memory_kb_test.go` 삭제. `knowledge` aggregator 빌트인 툴 + `memory_kb_*` 4 alias 모두 제거.
  - `internal/tarsserver/handler_memory.go` 의 `/v1/memory/kb/graph` + `/v1/memory/kb/notes` (POST/GET) + `/v1/memory/kb/notes/{slug}` (GET/PATCH/DELETE) HTTP 라우트 + `decodeKnowledgePatchRequest` 헬퍼 모두 제거. tests 도 같이 정리.
  - `internal/reflection/job_memory.go` 의 `compileKnowledge` 함수 (nightly LLM KB 컴파일) + `derivation.go` 의 `shouldCompileKnowledge` 게이트 제거. nightly memory 작업은 experience derivation 만 남음.
  - `internal/tool/memory_search.go` 의 `include_knowledge` 파라미터 + `searchKnowledgeNotes` 헬퍼 제거.
  - `internal/memory/Backend` 인터페이스에서 6 KB 메서드 (`ListKnowledgeNotes`/`GetKnowledgeNote`/`ApplyKnowledgePatch`/`ApplyKnowledgeUpdate`/`DeleteKnowledgeNote`/`KnowledgeGraph`) 제거.
  - `internal/tool/tool_groups.go` 의 `knowledge` → `memory` group 매핑 제거.
  - **Frontend**: `MemoryCenter.svelte` 의 Knowledge 탭 + 관련 state/handlers 제거 (909 → 551 줄, **-358 줄**). `lib/api.ts` 의 6 KB 함수 + `KnowledgeGraph`/`KnowledgeNote` import 제거. `lib/types.ts` 의 KB 타입 5종 제거. `lib/router.ts` 의 `/console/knowledge` alias 제거.
  - `tarsserver/main_options.go` chat 시스템 프롬프트의 `include_knowledge`/`knowledge(action=...)` 가이드 제거.

### Migration

KB 가 필요해지면 미래 PR 에서 재구현. 현재 사용자 워크스페이스의 `workspace/memory/wiki/notes/*.md` 파일은 read-only 자료로 남아있음 (TARS 가 더 이상 read/write 안 함). 사용자가 직접 보존하거나 삭제 가능.

### Net diff

총 ~2.2k 줄 감소 (Go ~1.5k + Svelte 358 + TS ~50 + tests 정리).

## [0.29.2] - 2026-04-26

### Changed

- **ID-002 (a)**: 시스템 프롬프트의 정체성 헤더 (`You are TARS, a personal AI assistant.`) 가 코드에서 IDENTITY.md 의 default content 로 이동. 사용자가 워크스페이스의 IDENTITY.md 를 편집해 어시스턴트 정체성을 자유롭게 재정의 가능. `Current time` 동적 line 과 `## Response Formatting` 가이드라인은 그대로 builder.go 에 유지 (runtime 제약 + 출력 품질 일관성 보장).

## [0.29.1] - 2026-04-26

### Changed

- **RF-053**: `gateway.Runtime.finalizeRunLocked` (200줄) 분할 — `applyFailedFinalState`(실패 상태 + run_failed 이벤트), `applyCompletedFinalState`(성공 상태 + run_finished 이벤트), `commitRunFinalization`(공통 tail: history trim + state version bump + run summary append + 단일 publish) 3 함수로 분리. 동작 동일, 향후 동시성 invariant 변경이 한 곳에 집중되도록 정리.

### Removed

- **RF-055**: `gateway.Runtime` 의 dead/demo 노드 시스템 전체 제거. `Runtime.Nodes()` / `NodeDescribe` / `NodeInvoke` 메서드 + `defaultNodes()` (`echo`/`clock.now`/`sessions.latest` 데모 노드) + `internal/gateway/runtime_nodes.go` 파일 + `gateway.NodeInfo` 타입 + `GatewayStatus.Nodes` 필드 모두 삭제.
- **RF-055**: `tool.NewNodesTool` 빌트인 툴 + `cfg.ToolsNodesEnabled` config 필드 + 관련 schema/input/yaml/test 항목 삭제. 데모 외 사용 사례 0건이었음.

## [0.29.0] - 2026-04-26

### Removed (BREAKING)

- **RF-007**: 빌트인 Go 플러그인 시스템 자체 제거. `plugin.BuiltinPlugin` 인터페이스 + `RegisterBuiltin` / `BuiltinPlugins` + `extensions.Manager.initBuiltinPlugins` + `tools_provider: builtin:<id>` 분기 모두 삭제.
- **RF-007**: `internal/browserplugin` (브라우저 자동화 빌트인 플러그인 — 유일 사용자) 디렉토리 전체 삭제. 의존했던 `internal/browser` (Chrome/CDP/Playwright runtime), `internal/vaultclient` (HashiCorp Vault SDK 클라이언트), `internal/approval` (OTP manager) 패키지도 함께 삭제.
- **RF-007**: HTTP API `/v1/browser/*` (status/profiles/login/check/run) 6 엔드포인트 + `/v1/vault/status` 제거.
- **RF-007**: tarsclient 의 `/browser` / `/vault` REPL 명령 + `pkg/tarsclient` 의 `BrowserState` / `BrowserProfile` / `BrowserLoginResult` / `BrowserCheckResult` / `BrowserRunResult` / `VaultStatusInfo` 타입 + `BrowserStatus` / `BrowserProfiles` / `BrowserLogin` / `BrowserCheck` / `BrowserRun` / `VaultStatus` 클라이언트 메서드 모두 삭제.
- **RF-007**: config 의 `VaultConfig` + `BrowserConfig` 임베디드 그룹 전체 + `ToolsBrowserEnabled` 필드 제거 (총 20+ 필드, env var 매핑, schema 메타, defaults, yaml path 매핑까지). 사용자 워크스페이스의 `tars.config.yaml` 에 `vault:` 또는 `browser:` 블록이 있으면 silently 무시됨.
- **RF-009**: 외부 플러그인의 HTTP 라우트 등록 경로 폐쇄. `extensions.Manager.CollectHTTPHandlers` 함수 + `plugin.HTTPHandlerEntry` 타입 + 매니페스트 `http_routes` 처리 모두 제거. 외부 플러그인은 더 이상 HTTP 라우트를 노출할 수 없음 — RF-009 옵션 (a) 적용. 라우트가 필요한 도메인 기능은 sidecar 프로세스 + 자체 포트로 운영해야 함.

### Migration

브라우저 자동화 / vault auto-login 기능을 사용하던 사용자는 외부 도구로 마이그레이션 필요:
- **브라우저 자동화**: Chrome DevTools MCP server, Playwright MCP server, 또는 사용자 정의 skill+CLI (CLAUDE.md 의 *"Default pattern: skill (.md) + companion CLI"*).
- **OTP / vault**: 별도 secrets manager + skill+CLI 호출. TARS 코어는 더 이상 vault SDK 통합을 제공하지 않음.

## [0.28.5] - 2026-04-26

### Security

- **RF-008**: 플러그인 lifecycle 훅의 임의 shell 명령 실행 (`sh -c`) 완전 제거. 이전엔 플러그인 manifest 의 `lifecycle.on_start: "echo ..."` 같은 문자열이 그대로 셸로 실행되어 외부 install 플러그인이 TARS 프로세스 환경 (vault 토큰, `~/.aws`, `~/.kube` 등) 에 임의 접근 가능했음.
  - `Lifecycle.OnStart` / `OnStop` 타입을 `string` → `*LifecycleHook { Tool string, Args json.RawMessage }` 로 변경. 빌트인 툴 호출 디스크립터 형식만 허용.
  - `LifecycleDeniedTools` deny-list (`bash` / `exec` / `shell_exec` / `process`) — manifest 파싱 시 + 훅 실행 시 두 번 검증 (defense-in-depth).
  - 기존 string 형식 manifest 는 명확한 마이그레이션 메시지와 함께 거부 (`"plugin manifest uses removed string form lifecycle.on_start; replace with object {tool, args}"`).
  - `runLifecycleHooks` 가 `LifecycleToolResolver` 를 통해 빌트인 툴 호출. resolver 가 nil 이면 declared 훅마다 "no tool resolver available" diagnostic 1줄 (현재 wiring 미완 — 향후 PR 에서 user-surface tool registry 연결).
- **RF-008 보강**: `extensions.Manager.Reload` 가 `plugins_allow_mcp_servers=true` 활성화 + plugin-declared MCP server 가 있을 때 startup WARN 로그. 플러그인 manifest 의 `mcp_servers` 필드도 외부 프로세스 실행 = sh-c 와 같은 카테고리 위험 표면이므로, 활성화 시 운영자에게 *"verify each plugin source is trusted"* 강조.

### Changed

- `extensions.Options` 에 `LifecycleToolResolver` 필드 추가 (현재 caller 는 nil 전달; 빌트인 툴 호출 wiring 은 후속 PR).
- `internal/plugin/manifest.go` validation 강화 — `rejectLegacyShellLifecycle` + `validateLifecycleHook` 로 양 단계 검증.

### Tests

- `internal/extensions`: lifecycle 훅 6 케이스 (resolved tool 호출 / deny-listed / unknown / tool error / nil resolver / no hooks)
- `internal/plugin`: 2 신규 케이스 (legacy string form 거부 / deny-listed tool 거부)

## [0.28.4] - 2026-04-26

### Added

- `internal/atomicwrite` 패키지 — TARS state file 의 표준 crash-safe write 헬퍼. tmp 파일 생성 → write → fsync → close → rename. 부모 디렉토리 자동 생성. unit test 5개 (new file / parent dir / overwrite / no temp leftover / read-only dir failure preserves original) (RF-059/068)

### Changed

- `cron.Store.save` (jobs.json 저장) 와 `cron.Store.pruneRunFile` (run history 트리밍) 가 `os.WriteFile` 대신 `atomicwrite.Write` 사용 — partial write 가능성 제거 (RF-068)
- `session.Store.saveIndex` (sessions.json) 와 `session.Store.SaveTasks` (tasks per session) 가 `atomicwrite.Write` 사용 (RF-059)
- `memory.KnowledgeStore` 의 노트 마크다운 / `index.md` / `graph.json` 3 write 경로가 `atomicwrite.Write` 사용
- `memory.saveEntries` (semantic 인덱스 entries.jsonl) 가 weak local `writeAtomicFile` (tmp + rename, no fsync) 대신 `atomicwrite.Write` 사용 — fsync 추가
- `gateway.writeJSONAtomic` (runs.json + channels.json) 가 `atomicwrite.Write` 로 위임 — gateway 내부 중복 구현 제거

이번 PR 의 가치: persistence anti-pattern 카테고리 정리 (5 사례 누적 → 0). 향후 SQLite 마이그레이션 (RF-017/021/058/067) 결정 시 첫 단계에서 단일 helper 만 교체하면 되도록 사전 정리.

## [0.28.3] - 2026-04-26

### Changed

- `pulse.Runtime.Start()` 가 `pulse_active_hours` / `pulse_timezone` 를 startup 에 1회 검증. 잘못된 값이면 ERROR 로그 1줄을 남기고 fail-soft (always-active) 로 진행. 운영자가 부팅 직후 로그만 봐도 잘못된 설정을 발견 가능 (RF-014)
- `pulse.Scanner.scanCron` / `scanDisk` 가 source 호출 (`cron.List` / `ops.Status`) 실패 시 silent 가 아닌 WARN 로그 출력. 동작은 그대로 (해당 tick 의 시그널만 skip) (RF-014)
- `pulse.Scanner.scanStuckRuns` 의 `parseRunTimestamp` 실패 시 WARN 로그 (run_id 포함). 손상된 timestamp 가 있는 run 을 stuck-run 검사에서 제외하는 동작은 그대로 (RF-014)
- `memory.FileBackend.AppendExperience` 가 caller 의 context 를 `IndexExperience` 에 그대로 전달 (이전엔 `context.Background()` 를 강제 사용 → caller cancellation 무시). 또한 indexing 실패 시 WARN 로그 (experience 저장은 성공, 검색 인덱싱만 실패) (RF-014)
- `memory.loadEntries` 가 손상된 JSONL line 을 skip 할 때 WARN 로그 (path/line/error). 누적 skip 수가 있으면 함수 종료 시 요약 로그 — "consider rebuilding" 힌트 (RF-014)

## [0.28.2] - 2026-04-26

### Changed

- `tarsserver`: 부트스트랩 순서 재구성 — config 를 cobra.Execute 이전에 로드, 최종 logger config 를 CLI+config 에서 도출, `setupRuntimeLogger` 를 단 한 번만 호출. config 로드 실패 시 panic. `newRootCmd` 가 pre-loaded `cfg` 를 인자로 받음. `buildRuntimeDeps` 도 cfg 를 인자로 받아 두 번 로드하지 않음. 이전 두 단계 logger 셋업 (CLI-only → config 로드 후 reconfigure) 에서 첫 번째 lumberjack handle 이 누수되던 문제 해소 (RF-002)
- `plugin.Source` 에 `Priority()` 메서드 추가 + `Load` 가 sources 를 priority 로 자동 정렬. 호출자 슬라이스 순서와 무관하게 일관된 머지 결과 (workspace > user > bundled). 동일 priority 내 stable sort 유지 (RF-013)
- `reflection.MemoryJob.compileKnowledge` 시그니처 `bool → (bool, []string)`. router/list/chat/json/apply 5 silent failure 경로가 prefix 가 붙은 에러 문자열로 `JobResult.Details["errors"]` 에 누적 (RF-016)
- `memory.normalizeSemanticTerms` + `prompt.normalizeRelevantTerms` stopwords 에 한국어 조사/대명사/지시어 추가 (`나/내/너/그/이/저`, `는/은/이/가/을/를`, `의/도/와/과`, `이거/저거/그거`, `뭐/뭐였지/뭐지/뭐야`, `선호/취향/좋아/좋아요/좋아함`). KR 쿼리 매칭 점수가 조사로 부풀려지던 문제 해소 (RF-018)
- `tool.dispatchAction` 가 `aliasFns ...func(map[string]json.RawMessage)` variadic 옵션을 받도록 확장. `automation.normalizeAutomationActionInput` 가 본문 복제 대신 `dispatchAction(params, aliasAutomationJobID)` 한 줄 호출로 단순화 (RF-028)
- `gateway.Runtime.ReportsRuns/ChannelsByWorkspace` 에 `GatewayArchiveEnabled` 플래그가 *디스크 archive* + *report endpoint 가시성* 두 의미를 겸한다는 docstring 추가. 분리는 ID-005 config 마이그레이션과 결합 (RF-057)

### Removed

- `tarsserver/main_serve_api.go` 의 dead `_ = telegramDeliveryCounter` 라인. pulse wiring 은 helpers_pulse.go 에서 이미 완료된 상태였음 — 잔재 정리 (RF-006)
- `internal/memory/knowledge.go` 의 `KnowledgeStore.nowFn` 필드 + 초기화. `Upsert` 가 `time.Now().UTC()` 를 직접 호출. 외부 주입 경로가 없는 채 stub 만 있던 의존성 제거 (RF-020)
- `internal/llm/router.go` 의 `Router.Close()` interface 메서드 + `multiTierRouter.Close()` no-op 구현. 호출자 0건 + Client interface 에 Close 가 없어 cleanup 할 게 없는 reserved-for-future stub (RF-044)
- `internal/llm/fallback_client.go` + `fallback_client_test.go` 전체. production wiring 0건의 reserved-for-future 구현. 필요 시점에 다시 추가 (RF-044)

### Follow-ups

이번 PR 도 Tier B 의 mechanical/independent 항목만. 결정 의존 항목은 별도:
- ID-001 ~ ID-005: GitHub issues #363-#367
- 70+ RF 우선순위: `docs/code-review/findings/refactor.md`

## [0.28.1] - 2026-04-25

### Changed

- `memory_search` 의 `include_sessions` 기본값 `false` → `true` (description 의도와 일치, RF-038)
- `Compaction.CompactOptions` 에 `keepRecent` 3-strategy 우선순위 docstring 추가 (RF-060)
- `LLMProviderSettings.ServiceTier` 가 provider-level default 임을 docstring 으로 명시 (tier-level 이 우선) (RF-063)
- `gateway/runtime_run_execute.go` `finalizeRunLocked` 가 동일 event 를 두 번 publish 하던 동작을 한 번으로 통합 (RF-054)
- `KnowledgeStore.Graph()` 자기치유 로직 단순화: 누락/손상/legacy 3 경로 → 단일 rebuild fallback (RF-022)
- `tool.Registry.Register` 가 같은 이름 중복 등록 시 silent overwrite 대신 warn 로그 출력 (RF-026)
- `cron.computeBackoffDuration` 의 magic number 를 documented const (`backoffBaseDuration`/`backoffMaxMultiplier`/`backoffMaxDuration`) 로 추출 (RF-070)

### Removed

- 사용되지 않는 deprecated 플래그 `--run-once` / `--run-loop` (`tars serve`) 와 관련 ServeOptions 필드, mutually-exclusive 검증, deprecation warning 모두 제거. pulse 는 서버 시작 시 자동 실행됨. 외부 자동화 스크립트가 이 플래그를 넘기면 `unknown flag` 에러가 발생하므로 호출부 수정 필요 (RF-001)
- `runtimeDepsError` 의 `daily_log` 좀비 case label — 어디서도 생성되지 않는 dead branch (RF-005)
- `internal/memory/semantic.go` 의 dead code 7 블록 (`indexState` 타입, `loadIndexState`/`saveIndexState`, `readDoc`, `firstMeaningfulParagraph`, 자체 `min`) (RF-019)
- `internal/prompt/builder.go` 자체 `max`/`min` 함수 (Go 1.25 built-in 사용) (RF-019)
- `internal/tool/list_dir.go` 자체 `min`/`minInt` 함수 (Go 1.25 built-in 사용) (RF-019)
- `internal/cron/helpers.go` 자체 `min` 함수 (Go 1.25 built-in 사용) (RF-019/RF-069)
- `internal/prompt/memory_retrieval.go` 의 죽은 fallback matcher (`collectProjectDocumentMatches`, `collectBriefMatches`, `classifySourceTag` 의 `projects/` + `_shared/` 분기) — project 패키지 제거 후 잔재 (RF-023)
- `IsExecToolName` 의 이중 정규화 (CanonicalToolName 한 번 호출로 단순화) (RF-027)
- `exec` tool 의 undocumented `cmd` alias (schema 에 `command` 만 정의됨) (RF-031)
- `provider="codex-cli"` removed-alias error stub 3 줄 (RF-043)
- consensus strategy `vote` schema enum (구현 미완 — 사용 시 runtime 에러였음. enum 을 `["synthesize"]` 만으로 축소) (RF-052)
- `runtimeDeps.sessionStoreResolver` 의 잉여 첫 nil 초기화 (RF-003)

### Follow-ups

이번 PR 은 Tier A (mechanical / silent acceptance / docstring) 만 정리. 사용자 결정이 필요한 큰 작업은 별도 추적:

- ID-001 ~ ID-005 의사결정: GitHub issues #363-#367
- 70+ RF 우선순위 매트릭스: `docs/code-review/findings/refactor.md`

## [0.28.0] - 2026-04-19

### Removed

- `research_report` tool and `internal/research` package — the deep-research workflow is no longer part of the supported surface
- `internal/schedule` package, `/v1/schedules` HTTP routes, and `tars schedule` CLI subcommand — one-shot scheduling is replaced by cron entries (cron already accepts natural-language `@at` expressions)
- `schedule_*` tool aliases (e.g. `schedule_create`, `schedule_cancel`) — use cron tools instead

### Migration

- Users who relied on `tars schedule`/`schedule_*` tools should register a one-shot cron job: `cron_create` accepts `@at <natural language time>` expressions via the existing `scheduleexpr` parser (e.g. `@at "tomorrow 9am"`)
- The `/v1/schedules` endpoints return 404 — update any external client to call `/v1/cron` instead

## [0.27.1] - 2026-04-12

### Fixed

- Console no longer hangs when switching chat sessions rapidly; SSE connections are now shared via a singleton EventSource instead of per-component instances that exhausted the browser's HTTP/1.1 connection limit
- Compaction deadlock resolved: auto-compaction no longer re-acquires the transcript file lock by reusing already-read messages via PreloadedMessages
- `--verbose` flag now correctly overrides `log_level` from workspace config, fixing missing HTTP debug logs
- Manual compact via console now uses aggressive thresholds (keepRecent=5, tokens=2000) so it always performs meaningful work

### Added

- HTTP request start logging: "http request started" debug log emitted before handler execution for visibility even when handlers block
- Compact result feedback: API returns detailed token counts and reason; console shows a feedback banner with message count, percentage saved, or skip reason

## [0.27.0] - 2026-04-12

### Changed

- Session compaction now uses rune-based CJK-aware token estimation instead of byte-length heuristic, improving accuracy for Korean/Chinese/Japanese content
- Deterministic summary restructured from 6 overlapping sections to 4 deduplicated sections (Topic, Key Decisions, Active Identifiers, Current State), reducing summary size by 59%
- LLM summary input upgraded from fixed 80-message x 240-char limit to 8K token budget with proportional content allocation

### Added

- Pre-compaction tool result pruning: long tool outputs (>200 chars) replaced with placeholder to prevent code dump pollution in summaries
- Stacking carry-forward: previous compaction summaries are detected and passed as Prior Context, preserving information across re-compactions
- Exported `EstimateMessageTokenCost` function for external use

## [0.26.1] - 2026-04-12

### Fixed

- Console sidebar now displays the server version dynamically from `/v1/status` instead of a hardcoded string
- Zero-time dates (`0001-01-01T00:00:00Z`) now display as "never" instead of absurd relative times like "739717d ago" across all console components
- Console static assets are now accessible in all auth modes; previously `external-required` mode blocked the SPA from loading
- Legacy config key detection no longer false-positives on the valid `llm_role_defaults` key

## [0.26.0] - 2026-04-12

### Added

- Hierarchical YAML config loading and patching across runtime, automation, gateway, tools, browser, vault, channels, and extensions, including migration-safe reads from existing flat keys
- Structured `/console/config` metadata and editing support for provider pools, tier bindings, nested object settings, and list-based settings such as allowlists and extra directories

### Changed

- Starter config generation, checked-in standalone defaults, and the shipped example config now use the hierarchical schema as the canonical layout
- README and Getting Started examples now describe the current console-first flow and nested config model instead of removed flat-key and project-oriented flows

### Fixed

- Settings patches written from the console now preserve the preferred nested YAML layout instead of reintroducing legacy flat keys into updated config files

## [0.25.0] - 2026-04-12

### Added

- Group-based tool policy controls across session config, workspace gateway agents, and the console tool configuration surface, including structured blocked-tool diagnostics and a manual verification guide for the Hermes improvement bundle
- Gateway provider override metadata, run detail APIs, live run events, consensus execution mode, and a dedicated console run view for inspecting multi-agent executions
- A file-backed memory backend interface that now powers memory APIs and tools behind a common abstraction

### Changed

- Chat compaction now exposes configurable trigger and retention knobs, supports deterministic mode and timeout-bounded LLM fallback, and reports compaction telemetry to the console context monitor
- Subagent orchestration can now carry per-task provider override and consensus settings through the agent runtime runtime and persistence layer

### Fixed

- Session tool group allow/deny rules now remain effective even when custom session tool mode is enabled without an explicit tool allowlist
- Chat context previews now persist and report the last applied compaction mode, and gateway agent list responses now include tier and provider override metadata

## [0.24.1] - 2026-04-05

### Fixed

- Cron jobs created from the chat tool inside a regular console chat session are now correctly bound to that session instead of silently becoming global; empty-kind chat sessions are treated as session-bound contexts, matching the behavior already available to the `kind=session` and `kind=main` paths
- Chat page now auto-refreshes when a background cron job delivers a result to the currently open session, and `[CRON]`/`[REMINDER]` transcript entries are no longer hidden from history so users can see why a scheduled run fired

## [0.24.0] - 2026-04-05

### Added

- Chat right panel now includes a dedicated `Cron` tab so main chats can manage global cron jobs and regular chats can manage only their bound session cron jobs in context
- Reminder cron jobs now deliver deterministically: global reminders post into the main chat session and send Telegram notifications when a target chat is available, while session-bound reminders stay inside their bound chat session

### Fixed

- `cron(action=create)` now accepts reminder-style aliases like `task_type`, `message`, and `title`, and can parse natural schedules such as `in 1 minute`
- Cron creation from chat now respects the current session kind by defaulting main chats to global/main-target jobs and regular chats to session-bound jobs

## [0.23.0] - 2026-04-05

### Added

- Session-bound cron jobs with optional `session_id` binding, so scheduled runs can reuse a chat session's tool and skill policy, work dirs, prompt override, and recent history
- User-visible cron audit logs appended to `artifacts/<session_id>/cronjob-log.jsonl` for bound jobs and `artifacts/_global/cronjob-log.jsonl` for global jobs
- Cron API, CLI, and console surfaces now show cron execution scope and session binding metadata

### Fixed

- Tasks panel no longer crashes when empty or legacy session task payloads omit the `tasks` array

## [0.22.0] - 2026-04-05

### Added

- Session-aware Files panel flows for chat: artifact deep links from messages, typed file previews, and workspace folder creation from both the browser and the directory picker
- Rich file preview modes for markdown render/raw text, syntax-highlighted code, zoomable images, and binary-file notices

### Fixed

- Session artifact tracking now keeps canonical paths, avoids duplicate entries, and opens the correct file reliably from chat history and the Files panel
- Session workdirs now always keep the mandatory `artifacts/{sessionId}` directory first, normalize stored paths, and repair misresolved `workspace/workspace/artifacts/...` file writes
- Workspace file APIs now handle absolute and relative artifact paths consistently, preventing transient or persistent 404s in file preview dialogs

## [0.21.0] - 2026-04-04

### Added

- Tasks panel in chat UI — view session plan, task progress bar, and task cards with status badges
- `GET /v1/admin/sessions/{id}/tasks` API endpoint for fetching session tasks
- Workspace file browser API: `GET /v1/workspace/files?path=` for directory listing and file content preview
- Tasks toggle button in chat pulse bar with live task count

## [0.20.0] - 2026-04-04

### Added

- Session-scoped `tasks` tool with plan + task management (actions: plan_set, plan_get, add, update, remove, list, clear)
- Tasks are stored per-session in `{sessionID}.tasks.json`, archived to memory when replaced
- Tool group utilities (`tool.KnownToolGroups`, `tool.ExpandToolGroups`, `tool.ExpandToolPatterns`) for agent policy resolution

### Removed

- **Breaking:** Removed entire project system (`internal/project/` package, ~30 files)
- Removed project tools (`project`, `project_work`, `project_brief` aggregators)
- Removed project API routes (`/v1/projects`, `/v1/project-briefs/`)
- Removed project CLI commands (`tars project list/get/activity/autopilot`)
- Removed project-related gateway integration (`project_task_runner`)
- Removed `Session.ProjectID` field and `SetProjectID()`, `EnsureWorker()` methods
- Removed worker session type (sessions are now `main` or general)
- Removed project frontend pages (`Projects.svelte`, `ProjectView.svelte`)
- Removed `project-swarm` plugin

### Changed

- Session tasks replace project-based task management with a simpler, session-scoped model
- System prompt rules updated to guide LLM on tasks tool usage
- Gateway agent policy resolution simplified (no longer depends on project package)

## [0.19.0] - 2026-04-04

### Changed

- **Tool consolidation: 71 → 27 built-in tools** using aggregator pattern
  - `memory` aggregator (replaces memory_save, memory_search, memory_get)
  - `knowledge` aggregator (replaces memory_kb_list/get/upsert/delete)
  - `workspace` aggregator (replaces workspace_sysprompt_get/set, agent_sysprompt_get/set)
  - `project` aggregator (replaces project_create/list/get/update/delete/activate)
  - `project_work` aggregator (replaces project_board/activity/dispatch/state tools)
  - `project_brief` aggregator (replaces project_brief_get/update/finalize)
  - `session` aggregator (replaces sessions_list/history/send/spawn/runs, agents_list, session_status)
  - `ops` aggregator (replaces ops_status/cleanup_plan/cleanup_apply)
  - `cron`/`heartbeat` aggregators: individual sub-tools removed from registry
  - Schedule tools absorbed into cron; file I/O aliases (read/write/edit) removed
- System prompt tool routing rules now explicitly guide LLM to use `workspace` for user profile updates
- Tool group expansion updated to recognize aggregator names (`memory`, `knowledge`)
- High-risk tool classification updated for aggregator names

### Removed

- SOUL.md removed from sysprompt specs, bootstrap files, and prompt builder (fully absorbed into IDENTITY.md)
- Individual cron/heartbeat sub-tools removed from tool registry (aggregators remain)
- Schedule tools removed (use cron aggregator instead)
- File I/O short aliases (`read`, `write`, `edit`) removed — use `read_file`, `write_file`, `edit_file`

## [0.18.0] - 2026-04-04

### Added

- Dedicated system prompt built-in tools: `workspace_sysprompt_get`, `workspace_sysprompt_set`, `agent_sysprompt_get`, `agent_sysprompt_set`
- Explicit system prompt management API endpoints: `/v1/workspace/sysprompt/files` and `/v1/workspace/sysprompt/file`
- Dedicated System Prompt console page at `/console/sysprompt` for managing `USER.md`, `IDENTITY.md`, `AGENTS.md`, and `TOOLS.md`

### Changed

- Workspace bootstrap metadata now treats `USER.md` as user identity, `IDENTITY.md` as TARS persona, `AGENTS.md` as agent operating rules, and `TOOLS.md` as tool guidance
- Prompt-source files can now be managed through domain-specific sysprompt surfaces instead of relying only on generic file tools

## [0.17.0] - 2026-04-04

### Added

- Memory management API endpoints for durable memory assets and search testing: `/v1/memory/assets`, `/v1/memory/file`, `/v1/memory/search`
- Dedicated Memory console page at `/console/memory` for inspecting and editing `MEMORY.md`, `memory/experiences.jsonl`, daily durable memory files, semantic index artifacts, and the knowledge base in one place
- In-console memory search test harness with toggles for `MEMORY.md`, daily logs, session history, and opt-in knowledge-base lookup

### Changed

- `memory_save` now writes durable memory to both `memory/experiences.jsonl` and `MEMORY.md`
- `memory_search` now searches `experiences.jsonl` with term-based lexical scoring, improving recall for cross-session memory checks without semantic embeddings
- Knowledge-base lookup is no longer part of default `memory_search`; callers must explicitly opt in with `include_knowledge=true`
- Automatic KB compilation is now gated to durable-signal turns instead of every chat turn

### Fixed

- Korean remember requests such as `... 기억해줘` now trigger durable memory promotion
- Cross-session recall no longer depends on KB note creation when only structured durable memory was saved

## [0.16.1] - 2026-04-04

### Fixed

- Empty knowledge bases no longer break `/v1/memory/kb/graph` with a 500 when `graph.json` has a blank `updated_at`
- Existing legacy `memory/wiki/graph.json` artifacts with blank timestamps are now tolerated and automatically repaired on read

## [0.16.0] - 2026-04-04

### Added

- Obsidian-style knowledge base layer under `memory/wiki/`: durable markdown notes, `index.md`, and `graph.json`
- Automatic post-chat knowledge compilation: the LLM can turn each completed chat turn into durable wiki notes and graph links
- Built-in KB CRUD tools: `memory_kb_list`, `memory_kb_get`, `memory_kb_upsert`, `memory_kb_delete`
- Knowledge Base API endpoints: `/v1/memory/kb/notes`, `/v1/memory/kb/notes/{slug}`, `/v1/memory/kb/graph`
- Dedicated console Knowledge page for browsing, editing, creating, and deleting wiki notes plus reviewing graph relations

### Changed

- `memory_search` now searches knowledge-base notes alongside `MEMORY.md`, daily logs, semantic recall, and optional session transcripts
- Workspace init/doctor now provision and validate `memory/raw` plus `memory/wiki/{notes,index.md,graph.json}`

## [0.15.2] - 2026-04-04

### Changed

- Default workspace path changed from `./workspace` to `~/.tars/workspace`
- Config path is now fixed at `~/.tars/config/config.yaml` (not user-overridable)
- `tars service install/start` no longer requires `--workspace-dir` or `--config` flags
- `ResolveConfigPath` fallback chain now includes `~/.tars/config/config.yaml`

### Added

- `tars init move --to <dir>` subcommand to relocate workspace directory (updates config and advises service restart)
- Auto-migration of legacy configs (`./workspace/config/tars.config.yaml`) on `tars init`
- `config.TarsHomeDir()`, `config.FixedConfigPath()`, `config.DefaultWorkspaceDir()` helpers

## [0.15.1] - 2026-04-04

### Added

- Project onboarding flow with planning mode: new projects without `project.md` enter planning phase where AI guides project planning via conversation
- Phase-aware system prompt: planning phase injects structured prompts for collaborative project definition
- Auto-transition from planning to executing phase when `project.md` is created
- Frontend: phase badge display (planning/executing), auto-send onboarding message on project creation

## [0.15.0] - 2026-04-04

### Added

- Proactive memory search: LLM now MUST call memory_search before answering questions about prior conversations, decisions, preferences, or facts
- Session transcript search via `include_sessions` parameter in memory_search tool for conversational continuity
- In-process memory cache with TTL (5 min) — cache-first strategy skips semantic search on cache hit
- Async memory prefetch goroutine for next-turn cache warming (fire-and-forget)
- `memory_recall` SSE event type for frontend memory notification
- 20+ conversational continuity detection patterns (EN/KR): "그거", "지난번", "you mentioned", "last time", etc.
- Deep session content search fallback in relevant memory collection
- Source type tags in Prior Context section: `conversation`, `experience`, `project`, `daily`

### Changed

- Renamed "Relevant Memory" section to "Prior Context" with source-type-tagged format

## [0.14.3] - 2026-03-29

### Added

- Extension detail view: click skill name to expand full SKILL.md content with markdown rendering
- Works for both installed skills and hub skills with full usage/help documentation
- Detail panel shows metadata (source, invocable status) and scrollable content

## [0.14.2] - 2026-03-29

### Fixed

- Extensions Hub tab no longer crashes when registry response has missing or null `plugins`/`skills`/`mcp_servers` arrays

## [0.14.1] - 2026-03-29

### Fixed

- Workspace reset now fully reinitializes to `tars init` state: removes all runtime artifacts (sessions, projects, cron, gateway, skills, plugins, mcp-servers, skillhub.json, ops, memory data) while preserving config/ and .md template files, then re-runs EnsureWorkspace to recreate the pristine directory structure

## [0.14.0] - 2026-03-29

### Added

- **Config Management** — structured Settings UI with field-level editing, select dropdowns for enumerable options, YAML raw editor toggle, server restart (launchd/exec auto-detection), workspace reset, and Danger Zone actions
- **Console CRUD** — project create/edit/delete with physical removal, cron job create/edit/delete/manual-run, session chat with ChatPanel embedding
- **Multimodal Chat** — file upload (image/PDF/text) with base64 encoding, clipboard paste (Ctrl+V), ContentBlock support across all LLM providers (Anthropic, OpenAI Codex, OpenAI Compat, Gemini)
- **Notification Panel** — clickable header badge with dropdown, newest-first sort, All/Unread/Read filter tabs, mark-all-read via events API
- **Projects Page** — dedicated project list separated from Home dashboard, with search, status filter (All/Active/Archived), table view, and Ask AI button for natural language editing
- **Extensions Management** — new Extensions page with Hub tab (browse/install/uninstall from tars-skills registry) and Installed tab with ON/OFF toggle per skill/plugin/MCP server, persistent disable state via `extensions_disabled.json`
- **Skillhub API** — `/v1/hub/registry`, `/v1/hub/installed`, `/v1/hub/install`, `/v1/hub/uninstall`, `/v1/hub/update` endpoints wrapping existing `skillhub.Installer`
- **Ask AI** buttons on Projects and Ops pages that navigate to Home chat with context-prefilled prompts

### Fixed

- Cleanup approval now auto-applies on approve (no separate Apply step), with result stored in Approval.Note and displayed in Ops UI
- Blocked MCP servers no longer cause the entire `ListServers` API to return 500; blocked servers are included with error field set while others return normally
- Project DELETE now physically removes the directory instead of soft-archiving
- `requestJSON` handles 204 No Content responses without JSON parse errors
- `openai-codex` added to LLM provider select options in Settings UI

### Changed

- Home page redesigned with Chat as the primary feature (moved to top), summary widgets below
- Notification section removed from Home (replaced by header notification panel)

## [0.13.5] - 2026-03-28

### Fixed

- Source checkouts now serve an explicit `/console` placeholder page with build and dev-proxy instructions instead of a blank-looking shell when the Svelte console assets have not been built yet
- `tars serve` now logs a startup warning when it falls back to placeholder console assets, and the developer workflow documents the `make console-install` / `make console-build` steps for local source runs

## [0.13.4] - 2026-03-28

### Fixed

- The `ops-service-demo` Docker Compose template no longer pins a global `ops-service-demo` container name, so repeated seed repos do not collide on stale container names during local reruns
- The ops-service example tests now lock in the absence of a fixed container name, and the walkthrough clarifies that Compose names are project-scoped while the host port remains shared

## [0.13.3] - 2026-03-27

### Fixed

- The ops-service example now treats the bootstrapped repository as a seed repo only and moves all runtime `docker compose` and `opsctl` steps to the authoritative project clone under `projects/<project-id>/repo`
- The bootstrap helper output now explains the seed-repo role directly instead of suggesting runtime service commands before the TARS project clone exists

## [0.13.2] - 2026-03-27

### Fixed

- Project-linked cron jobs now inherit the owning project's tool allowlist during background agent runs, so approved shell/file tools are available to workflows such as the ops-service triage example
- The ops-service example walkthrough now switches the running demo service into the project's cloned repo and filters immediate cron runs by `project_id`, avoiding duplicate-job selection and repo-path mismatches

## [0.13.1] - 2026-03-27

### Fixed

- The `ops-service` example template no longer requires a nested Go module inside the TARS repository, so `go test ./examples/ops-service-demo/...` now works from the repo root
- The demo repo bootstrap script now writes a standalone `go.mod`, preserving independent `go test ./...` execution after the template is copied into its own repository

## [0.13.0] - 2026-03-27

### Added

- Bundled `ops-service` plugin with operational planning, log triage, issue creation, remediation, PR, and reporting skills
- `examples/ops-service-demo/` with a bootstrap script, standalone demo repo template, `opsctl` operational CLI, Docker Compose service, and example project/cron payloads

### Changed

- Workspace bootstrap and repair flows now restore the bundled `ops-service` plugin alongside the existing bundled project workflow plugin
- README documentation now includes the new end-to-end ops-service example walkthrough

## [0.12.1] - 2026-03-27

### Added

- Project autopilot status responses now include phase, phase status, summary, and next action metadata for CLI/API clients
- Typed chat events now expose `skill_name` and `skill_reason` when auto skill routing is announced

### Changed

- Planning blockers now age into an explicit timeout/escalation path instead of staying in an unbounded blocked-planning state forever
- Expired terminal `AUTOPILOT.json` snapshots are pruned during status/restore so stale runtime state does not linger indefinitely
- Telegram chat replies now surface auto-selected skill notices for active brief and explicit skill routing
- CI and release workflows now opt into the Node 24 GitHub Actions runtime and use the current checkout/setup action majors to avoid deprecation warnings

## [0.12.0] - 2026-03-27

### Added

- Typed `PhaseEngine` project runtime with a step-wise `advance` flow exposed through chat tools, REST, and TUI project commands
- Project workflow metadata fields `workflow_profile` and `workflow_rules` for per-project worker and verification policy overrides
- Chat status events that surface automatic skill routing decisions before execution starts

### Changed

- Project autopilot now follows a planning-first, phase-centric workflow instead of immediately seeding and cycling a Kanban board from an empty brief
- Empty backlog states now fall back to planning or approval instead of auto-seeding bootstrap tasks
- Dashboard project views now prioritize phase status, run status, pending human decisions, and blockers over raw board columns
- Built-in project-start and project-autopilot skills now align with the phase engine, approval gates, and one-step runtime control
- Non-software workflow profiles can disable software-specific worker defaults and GitHub/test/build gates without changing core code

## [0.11.0] - 2026-03-22

### Added

- Plugin manifest v2 metadata: `schema_version`, `requires`, `supported_os`, `supported_arch`, `default_project_profile`, and `policies`
- Remote MCP transports for `streamable_http`, legacy `sse`, and `websocket`, alongside existing `stdio`
- MCP server auth settings for bearer-token env injection and OAuth-backed bearer headers on remote transports

### Changed

- Plugin loading now applies runtime availability gating, so unavailable plugins no longer contribute skills or MCP servers
- MCP server status APIs now expose transport, source, URL, and auth mode metadata in addition to connectivity state
- Bundled `project-swarm` plugin manifest now declares schema version 2 and its default project profile

## [0.10.3] - 2026-03-22

### Added

- Skill runtime gating for `SKILL.md` frontmatter: `requires_plugin`, `requires_bins`, `requires_env`, `os`, and `arch`

### Changed

- Unavailable skills are now excluded from the runtime snapshot and prompt, with extension diagnostics explaining missing plugins, binaries, environment variables, or platform mismatches
- Plugin source priority documentation now matches runtime behavior: `workspace > user > bundled`

## [0.10.2] - 2026-03-22

### Added

- Manual `/compact [instructions]` now works from the single-main-session TUI/runtime path and forwards custom focus guidance to compaction

### Changed

- Session compaction now writes structured deterministic summaries with preserved identifiers, current-goal/open-state sections, and explicit requested-focus capture
- Auto and default manual compaction now preserve a safer recent tail using a 30% token-share policy with the existing 12K-token floor instead of relying only on a fixed recent-count fallback

## [0.10.1] - 2026-03-22

### Changed

- Built-in `read_file` now uses 2,000-line pagination with continuation guidance, 20MB file-size guards, and long-line shortening instead of raw byte-only truncation
- Built-in `write_file` now resolves create targets against the real workspace path and writes through an atomic temp-file rename to avoid symlink escapes and partial writes

## [0.10.0] - 2026-03-22

### Added

- `subagents_run` chat tool for parallel read-only delegation to gateway-backed explorer subagents
- Built-in `explorer` gateway agent with a read-only allowlist for codebase and project research tasks
- Gateway run metadata for subagent lineage and hidden subagent sessions
- Config knobs `agentruntime_subagents_max_threads` and `agentruntime_subagents_max_depth`

### Changed

- Hidden subagent runs now append compact system summaries back to the parent chat session instead of leaking raw child transcripts into the main conversation context

## [0.9.0] - 2026-03-22

### Added

- Trusted MCP Hub CLI: `tars mcp {search,install,uninstall,list,update,info}` for discovering and managing vetted MCP packages from `devlikebear/tars-skills`
- Registry v3 format with `mcp_servers` section and checksum-verified package files
- Hub-managed MCP runtime source that loads installed MCP manifests alongside base config and plugin-provided MCP servers

### Changed

- Extension reload diagnostics now report MCP source overrides and malformed installed MCP manifests
- Public docs now distinguish plugin-embedded MCP servers from hub-managed MCP packages and document the `mcp_command_allowlist_json` requirement

## [0.8.0] - 2026-03-21

### Changed

- Gemini native provider rewritten to raw HTTP, removing `google.golang.org/genai` SDK and all transitive dependencies (cloud.google.com, grpc, protobuf)
- Reduced binary dependency footprint and build time

### Added

- Plugin interface documentation (`docs/plugins.md`) covering manifest schema, skill directories, MCP servers, plugin sources, and the `project-swarm` reference implementation

## [0.7.1] - 2026-03-21

### Added

- TARS Plugin Hub CLI: `tars plugin {search,install,uninstall,list,update,info}` for managing plugins from the public registry
- Registry v2 format with `plugins` section in `devlikebear/tars-skills`
- Skill install now warns when a `requires_plugin` dependency is missing and suggests the install command
- CI coverage reporting with Codecov upload

### Changed

- README rewritten: repositioned as "local-first AI project autopilot" with badges, three-tier feature structure, and concise quick start
- GitHub repository description and topics updated
- `web/relay-extension/` extracted to standalone `devlikebear/tars-relay-extension` repository
- CI now runs `make test-cover` instead of `make test`

## [0.7.0] - 2026-03-21

### Added

- TARS Skill Hub CLI: `tars skill {search,install,uninstall,list,update,info}` for discovering and installing skills from the public `devlikebear/tars-skills` registry
- Companion file support for skills: scripts (`.sh`, `.py`, `.ts`), templates, and other reference files are installed alongside `SKILL.md` and mirrored to runtime
- `internal/skillhub` package with registry fetch, search, install, list, and update operations
- Skill registry `files` field for declaring companion files in `registry.json`

### Changed

- Skill runtime mirror now copies all companion files from the source skill directory, preserving subdirectory structure and executable permissions

## [0.6.3] - 2026-03-21

### Fixed

- MCP server failures no longer block server startup; continues without MCP tools

## [0.6.2] - 2026-03-21

### Fixed

- Startup LLM traffic storm: `RestorePersistedRuns` no longer auto-starts all project autopilot loops on startup; runs resume on next heartbeat instead
- Session 404 error: translate public session ID `"main"` to internal hash ID in chat handler
- Stale `AUTOPILOT.json` status correction: persisted `running` status with blocked/failed message is fixed on restore
- macOS build warning: suppress `-lobjc` duplicate library linker warning

### Added

- Log rotation config: `log_level`, `log_file`, `log_rotate_max_size_mb`, `log_rotate_max_days`, `log_rotate_max_backups` with lumberjack
- Logger configuration printed as INFO on server startup
- Config `log_file` takes precedence over CLI default; parent directory auto-created
- `make build` outputs binary to `bin/` directory

## [0.6.1] - 2026-03-20

### Changed

- Homebrew release automation now updates the unified `devlikebear/homebrew-tap` repository instead of the dedicated `homebrew-tars` tap
- Public install instructions now use `brew tap devlikebear/tap` and `brew install devlikebear/tap/tars`

## [0.6.0] - 2026-03-20

### Added

- Semantic Memory V2 with local derived indexing under `workspace/memory/index` for durable memories and project documents
- Gemini embedding configuration for semantic retrieval with `memory_semantic_enabled`, `memory_embed_*`, and default `gemini-embedding-2-preview` support

### Changed

- Prompt assembly now prefers semantic memory recall for paraphrases and project-scoped context, with lexical retrieval kept as the fallback path
- `memory_save` now dual-writes to both `experiences.jsonl` and the semantic memory index when semantic memory is enabled
- Session compaction now stores compaction summaries and extracted durable memory candidates in the semantic index without breaking compaction when extraction fails
- `memory_search` now uses semantic recall first and falls back to the existing file-based substring search when embeddings are unavailable

## [0.5.11] - 2026-03-14

### Fixed

- Project autopilot now stays alive in a periodic supervisor loop until the board reaches `done` instead of stopping after one bounded burst of dispatches
- Server startup now recreates autopilot loops for incomplete projects so active work resumes automatically after a TARS restart
- Heartbeat-triggered supervision now force-starts missing autopilot loops for incomplete projects as a safety net when a project is active but no live PM loop is attached

### Changed

- PM supervision now auto-requeues stalled `in_progress` work back to `todo`, records an automatic retry decision/replan, and keeps moving without asking the user for routine retry decisions

## [0.5.10] - 2026-03-14

### Added

- `/dashboards` now renders a workspace-wide project index that links to every project dashboard and summarizes status, phase, next action, and autopilot state

### Changed

- Project dashboard auth can now be disabled independently from API auth with `dashboard_auth_mode: off`, so trusted local browser monitoring can stay open while `/v1/*` routes remain protected

## [0.5.9] - 2026-03-14

### Fixed

- Natural-language project kickoff without an explicit `session_id` now starts in a fresh chat session instead of inheriting the current main session context
- Project board normalization now canonicalizes common Kanban aliases such as `backlog` and `doing` to the runtime statuses `todo` and `in_progress`, so dispatch, activity, and dashboard views stay aligned

### Changed

- The bundled `project-start` skill now explicitly seeds boards with the canonical status set `todo`, `in_progress`, `review`, `done`

## [0.5.7] - 2026-03-14

### Fixed

- Project worker runs now create a distinct hidden session per project run instead of reusing one shared hidden session across subagent work
- PM seed backlog dispatch now stages `pm-seed-bootstrap` ahead of dependent seed tasks so autopilot does not start the first vertical slice before bootstrap is underway
- Chat requests with an explicit stale `session_id` now create a fresh chat session instead of silently attaching to the current main session
- Project autopilot run status now persists to `AUTOPILOT.json`, survives server restart, and no longer disappears from `/v1/projects/{id}/autopilot` after the process restarts
- Persisted `running` autopilot runs now recover as `blocked` with restart guidance and an interrupted PM blocker entry instead of reporting a false in-progress state after restart

### Changed

- API startup now preloads persisted autopilot runs so project state, activity, and dashboard views are already synchronized before the first autopilot status request
- Autopilot persistence now uses atomic file replacement for `AUTOPILOT.json` writes

## [0.5.6] - 2026-03-14

### Fixed

- Project autopilot now preserves the logical worker kind even when task dispatch falls back to the runtime default gateway agent
- Failed worker runs now restore the task to `todo`, record the real worker error, and stop autopilot on the actual blocker instead of corrupting the next dispatch with an executor alias
- Empty project boards now block autopilot for backlog seeding instead of incorrectly marking the project complete
- `tars doctor` now fails fast when `gateway_default_agent` points to an enabled gateway executor with a missing local command or script path
- The flaky browser relay broadcast test now waits for both CDP clients to be fully registered before asserting fan-out delivery

### Changed

- The project dashboard now shows autopilot run status and dedicated worker report entries extracted from structured task reports
- The project dashboard now also shows PM blocker, decision, and replan notes from the supervisor loop
- Project autopilot now behaves more like a PM supervisor by seeding a minimal MVP backlog when a project starts with an empty board
- Bundled `project-start` and `project-autopilot` skill instructions now align with the runtime by defaulting low-risk kickoff decisions and by treating an empty board as blocked work rather than completed work

## [0.5.5] - 2026-03-14

### Added

- `llm_provider: claude-code-cli` to run chat requests through a locally installed Claude Code CLI without API keys

### Changed

- `tars doctor`, starter config comments, and public docs now explain the local Claude Code CLI provider path alongside API-backed providers

## [0.5.4] - 2026-03-14

### Fixed

- Terminal chat now recovers automatically when a stale local `session_id` causes `/v1/chat` to return `404 not_found: session not found`
- TUI and one-shot CLI chat retry once against the current main session, or fall back to creating a fresh session when no main session exists

## [0.5.3] - 2026-03-14

### Changed

- Project task dispatch now falls back to the runtime default gateway agent when a requested worker alias such as `codex-cli` is not explicitly registered
- Starter project autopilot can advance past gateway agent-name mismatches instead of failing immediately on `unknown agent`

## [0.5.2] - 2026-03-14

### Added

- `tars doctor` now warns when `gateway_enabled=false` would disable the bundled project workflow and autopilot

### Changed

- Starter workspaces created by `tars init` now enable the gateway path required by bundled project workflows out of the box

## [0.5.1] - 2026-03-14

### Added

- TUI project workflow commands for board inspection, activity inspection, task dispatch, and autopilot start/status
- `GET` and `POST /v1/projects/{id}/autopilot` so non-chat clients can start and inspect project autopilot runs

### Changed

- Project manager operations no longer require `curl` for common TUI workflows after a project has been created
- Dogfooding documentation now shows both TUI and HTTP routes for project manager operation

## [0.5.0] - 2026-03-14

### Added

- Starter workspace setup now installs bundled plugins such as `project-swarm` into `workspace/plugins`
- `tars doctor --fix` now restores missing bundled workspace plugins in addition to starter files

### Changed

- Bundled skill and plugin directories now resolve from installed package layouts such as `share/tars/{skills,plugins}` as well as repo-local paths
- Release archives, the curl installer, and the Homebrew formula now install bundled `share/tars` assets alongside the `tars` binary

## [0.4.0] - 2026-03-14

### Added

- Bundled `project-swarm` plugin with `project-start` and `project-autopilot` skills for workspace project kickoff and autonomous follow-through
- Built-in project runtime tools for board read/write, activity read/append, task dispatch, and background autopilot start
- Natural-language project kickoff routing for chat and Telegram when a project brief is being collected or a project start request is detected
- Background project autopilot loop that can keep dispatching `todo` and `review` stages while updating project state for the dashboard

### Changed

- Minimal chat tool injection now includes safe project runtime tools needed by the bundled project skills
- Project kickoff can proceed from a brief-driven interview instead of requiring only manual API calls
- Test chat helpers are synchronized for concurrent inflight chat coverage

## [0.3.0] - 2026-03-14

### Added

- Project manager workflow primitives: project activity log, Kanban board storage, and a server-rendered dashboard with live updates
- Project task orchestration with built-in `codex-cli` and `claude-code` worker profiles plus a gateway-backed task runner
- Review gate and GitHub Flow metadata tracking for project tasks, including issue/branch/PR and verification status
- `POST /v1/projects/{id}/dispatch` to run `todo` or `review` project task dispatch stages through the orchestrator

### Changed

- The project dashboard now renders board state, recent activity, and a dedicated GitHub Flow status block in one page
- Review-required tasks now stop at `review` until a reviewer run approves them
- Test/build and GitHub Flow metadata now gate task promotion to `review` or `done`

## [0.2.0] - 2026-03-11

### Added

- `tars init` to create a starter workspace plus minimal `workspace/config/tars.config.yaml`
- `tars doctor` and `tars doctor --fix` to validate or repair local starter files before first run
- `tars service install/start/stop/status` to manage `tars serve` as a macOS LaunchAgent

### Changed

- Quick start documentation now prefers `init -> doctor -> service` before manual `tars serve`
- The public example config comments now point packaged installs to the starter onboarding flow

## [0.1.2] - 2026-03-10

### Changed

- Release assets now build both macOS archives on a single `macos-14` runner so GitHub Release publishing is not blocked by a second runner matrix leg

## [0.1.1] - 2026-03-10

### Added

- Automated release workflow driven by `VERSION.txt` changes on `main`, including tag/release publishing and Homebrew tap updates
- Public `install.sh` for curl-based macOS installs from GitHub Releases
- Homebrew tap formula generation for `devlikebear/homebrew-tap`

### Changed

- Public documentation is maintained in English for the published repository surface
- `install.sh` now installs the latest published GitHub Release by default
- Release PRs must update `VERSION.txt` and `CHANGELOG.md` together

## [0.1.0] - 2026-03-08

### Added

- Initial public release of the local-first TARS runtime
- Embedded build metadata via `VERSION.txt`, Git commit, and build date
- `tars version` and `tars --version`

### Changed

- Primary Go module path is `github.com/devlikebear/tars`
- Primary plugin manifest filename is `tars.plugin.json`
- Primary user extension directories use `~/.tars`

### Security

- Repository publishing flow includes `make security-scan`
- Gitleaks false-positive handling is documented via repository ignore metadata
