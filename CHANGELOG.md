# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog and the project follows Semantic Versioning.

## [Unreleased]

## [0.33.0] - 2026-05-30

### Added

- **Public chat-session package (`pkg/session`)** — Adds an additive public Go entrypoint `pkg/session` that thin-aliases `internal/session` so external agent apps can reuse TARS' file-backed chat-session and transcript persistence without importing `internal/*`. Exposes `Store`/`Session`/`Message`/`HistorySnapshot`, the `NewStore` constructor, the transcript helpers (`AppendMessage`, `ReadMessages`, `RewriteMessages`, `LoadHistory`, `LoadHistorySnapshot`, `EstimateMessageTokenCost`), and the `ErrSessionNotFound`/`ErrCwdNotEligible`/`ErrSessionKindUnsupported` sentinels. The on-disk format (`sessions/sessions.json` index plus one `sessions/{id}.jsonl` transcript per session) is unchanged and `internal/session` is untouched.

## [0.32.72] - 2026-05-21

### Added

- **Public agent-building packages (#884, #885, #886)** — Adds additive public Go package entrypoints under `pkg/llm`, `pkg/tools`, `pkg/agentloop`, `pkg/memory`, `pkg/skill`, and `pkg/mcp` so lightweight agent apps can reuse TARS-tested LLM provider contracts, tool registries, the iterative tool-calling loop, durable memory helpers, `SKILL.md` loading, and MCP tool adaptation without importing `internal/*` directly. The first dogfood example lives in `examples/min-agent`, and `docs/public-agent-packages.md` documents the supported package boundaries plus the server/runtime surfaces that intentionally remain internal.

## [0.32.71] - 2026-05-19

### Added

- **Console companion feedback (#882)** — Adds a default-on floating Console companion that can be toggled with `companion.enabled`, localized through the EN/KO Console locale, and poked for immediate route-aware `Poke`, `Suggest`, and `Feedback` reactions. The companion can hand a bounded context prompt into `/console/chat`, reacts to runtime notifications, and includes visible pressed/action feedback so manual stimuli are not hidden behind existing event bubbles.
- **Embodiment percept companion signals (#882)** — Successful `/v1/embodiment/percept/{provider}` intake now publishes ephemeral live companion feedback events so camera and microphone providers can make the Console pet react to seen/heard body signals without turning those signals into desktop notifications.

### Fixed

- **Companion config toggle semantics (#882)** — Tracks explicit `companion.enabled` boolean values so YAML/env `false` reliably disables the default-on companion. Quick Start treats the companion as optional, so intentionally disabling it does not make onboarding incomplete.

## [0.32.70] - 2026-05-17

### Fixed

- **Console Settings: Embodied provider guidance (#880)** — The Embodied Bot providers editor now offers one-click `Mac Host`, `StackChan`, and `Custom` presets instead of making users guess provider names, MCP endpoints, and capability lists. The capability picker has been replaced with grouped perception/actuation chips with tooltips, and every provider field now exposes a short hint. The recommended defaults now match the `tars-stackchan` companion examples: physical StackChan uses MCP endpoint `tars-stackchan` with full body capabilities, while the Mac host companion uses `tars-stackchan-host` with hearing/speech, observation triggers, and owner gating disabled by default.

## [0.32.69] - 2026-05-17

### Fixed

- **Console Settings: Embodied Bot quick-start controls (#878)** — The backend config schema already exposed `embodiment.enabled` and `embodiment.providers`, but the default Console Settings "Quick Start" view did not show them. Quick Start now includes `Embodied Bot` and `Embodied Bot providers` cards with the same restart/env-override metadata as other core runtime settings. Provider descriptors now open a structured editor for name, transport, endpoint, session, trigger limits, owner gating, and capabilities instead of dropping users into raw JSON.

## [0.32.68] - 2026-05-17

### Added

- **Embodiment core scaffold (Phase 1)** — Adds the dormant `internal/embodiment` subsystem without changing chat/runtime behavior when no body provider is configured. The new provider-neutral contract defines body `Capability` values (`vision`, `hearing`, `speech`, `expression`, `motion`, `led`), `Percept`, `ProviderDescriptor`, and future `BodyAction` types, plus a thread-safe registry with duplicate-name rejection, enabled-provider filtering, and defensive capability copies. Runtime wiring starts no background work when `embodiment.enabled: false` or zero enabled providers are configured, and it registers no LLM tools. Config now accepts `embodiment.enabled` and `embodiment.providers` from YAML or `TARS_EMBODIMENT_ENABLED` / `TARS_EMBODIMENT_PROVIDERS_JSON`, and `tars doctor` reports `[ok] embodiment: disabled` by default or `enabled (providers: ...)` when configured. `config/default.yaml`, `config/tars.config.example.yaml`, and README document the disabled default plus stackchan/host provider examples.
- **Embodiment perception → cognition (Phase 2 / #875)** — Adds typed Percept ingress through both the existing `/v1/channels/webhook/inbound/{provider}` path and the dedicated `/v1/embodiment/percept/{provider}` endpoint. Known enabled body providers still persist inbound Percepts as channel messages, then normalize them into self-sensory `Percept` values with owner, modality, salience, trigger, media reference, session, and raw payload metadata. A salience/owner gate maps owner audio to `directive`, leaves stranger/unknown/ambient inputs as `observation` by default, and applies debounce plus per-hour rate limiting. Directive Percepts trigger one agent runtime turn in the bound/default session with an embodied system prompt append; concurrent provider/session triggers are suppressed while a run is in flight. Non-embodiment webhooks keep their previous inbox-only behavior, and Phase 2 still does not add actuation routing or hardware/provider code.
- **Embodiment actuation egress (Phase 3)** — Closes the mock perception→cognition→actuation loop without adding any built-in LLM tools. Embodied cognition responses may append a fenced `tars-body-action` JSON block, which is parsed into normalized `BodyAction` values (`speak`, `express`, `move`, `led`) and validated before routing. The action router maps actions to provider capabilities (`speech`, `expression`, `motion`, `led`) and gracefully drops unsupported or disabled-provider actions with info logging instead of failing the cognition loop. MCP-backed providers receive supported actions through their existing MCP server tools, with one retry on dispatch failure; speech-only mock coverage proves `speak` is delivered while `express` is dropped.

## [0.32.67] - 2026-05-15

### Added

- **session-scoped Claude Code permission deny 규칙 → `--settings` 마운트 (Epic #857 follow-up #2 / #869 구체화·구현)** — #869는 "구체 use case 부재"로 보류돼 있었으나, claude-code-cli가 Claude Code 내장 툴(Read/Edit/Bash/WebFetch)을 **자기 프로세스 안에서 self-execute**한다(#865에서 확인)는 사실에서 명확한 갭을 도출: 이 내부 툴은 TARS 레지스트리·`tool_config` 게이팅을 거치지 않으므로, 세션이 Claude Code의 내장 Bash로 파괴적 명령 실행을 막을 수단은 coarse `--permission-mode`(plan/bypassPermissions = all-or-nothing)뿐이었다. 이를 메우는 구체 시나리오 — `.tars/settings.json`에 `"claude_code_cli_permission_deny": ["Bash(rm:*)", "Bash(git push:*)", "WebFetch"]` 한 줄로 그 세션 한정 세분화된 deny 규칙 적용 — 을 end-to-end 구현. 위협 모델은 **denylist 런타임 필터가 아니라 schema-level allowlist**로 보장: 신규 `internal/llm.writeClaudeCodeSettingsFile(deny []string)`는 deny 슬라이스만 받아 정확히 `{"permissions":{"deny":[...]}}` 두-키 shape만 marshal — `env`/`hooks`/`apiKeyHelper`/`model` 등을 emit할 코드 경로 자체가 없어 adversarial 세션-override 입력도 권한을 *좁히기만* 할 수 있고 credential exfiltration·임의 바이너리 등록이 구조적으로 불가능. 임시 파일은 `--mcp-config`/`--plugin-dir`와 동일 패턴으로 `Chat`에서 `--settings <path>` 인자에 끼우고 `defer cleanup()`(=`os.Remove`)으로 호출 종료 시 제거 — deny 슬라이스가 비거나 전부 공백이면 플래그 자체 생략. 입력은 trim·blank drop·dedup(first-seen order). `ChatOptions.ClaudeCodePermissionDeny` + `agent.RunOptions.ClaudeCodePermissionDeny`(매 iteration forward, 다른 provider는 silently ignore). 세션-scope: `internal/sessionoverride`의 `Override`/`EffectiveConfig`에 `ClaudeCodeCLIPermissionDeny []string` + `AllowedTopLevelFields`/`AllPaths()`/loader case 등록, `merge.go`의 `applyLayer`는 `unionDedup`으로 shared+local 레이어를 union(**tightening-only**: 깊은 레이어가 규칙을 추가할 수는 있어도 상속된 규칙을 제거할 수 없어 shared 베이스라인을 local이 몰래 완화 불가). 신규 헬퍼 `internal/tarsserver/effective_session.go`의 `effectiveClaudeCodePermissionDeny(svc, sess)`는 머지된 deny 리스트를 resolve(전역 config fallback 의도적으로 없음 — deny는 per-session 가드레일), `handler_chat_execution.go`의 `executeChatLoop`가 resume용으로 이미 로드하는 동일 `priorSess`에서 추가 IO 없이 읽어 `RunOptions`로 전달. CLAUDE.md sessionoverride 허용 필드 목록에 `claude_code_cli_permission_mode`(#868에서 누락됐던 것)와 `claude_code_cli_permission_deny`를 함께 보강. 7개 회귀 케이스 추가 — provider: `TestWriteClaudeCodeSettingsFile`(empty→무파일 / trim·dedup·order 보존 / **adversarial**: `env`·`hooks`·`apiKeyHelper`·JSON-injection 문자열이 deny 배열 안 opaque string으로만 들어가고 top-level 키로 절대 누출 안 됨을 정확한 키 카운트로 잠금), `TestClaudeCodeCLIClientChat_SettingsPassedWhenDenyProvided`(셸 stub이 `--settings` 경로 파일을 cat·검증 + Chat 후 temp 제거 확인), `_SettingsSkippedWhenEmpty`; sessionoverride: loader round-trip, merge union-across-layers; tarsserver: override resolve + nil-service→nil. 남은 Epic #857 follow-up은 #871(콘솔 Agent SDK 크레딧 잔액 pill, Anthropic 잔액 API 미공개로 blocked)뿐.

## [0.32.66] - 2026-05-15

### Added

- **claude-code-cli가 TARS 세션 skill 카탈로그를 `--plugin-dir`로 자동 노출 (Epic #857 follow-up #3)** — 로컬 `claude` 2.1.142로 discovery 동작을 실제 검증(#870 코멘트 기록): `--add-dir`는 file-read 권한만 줄 뿐 skill source가 아니며, 워크스페이스 오염 없는 공식 메커니즘은 **`--plugin-dir <path>`**(세션 한정, 반복 가능). 임시 디렉터리에 `.claude-plugin/plugin.json` + `skills/<name>/SKILL.md` 구조를 만들면 stream-json `system.init`의 `skills`/`slash_commands`에 `tars-skills:<name>`으로 등록되고 description frontmatter도 그대로 읽힘을 확인. 이를 구현으로 옮긴다. 신규: `internal/llm.ClaudeCodeSkill{Name,Description,Content}` 타입 + `ChatOptions.ClaudeCodeSkills` 필드. `internal/llm/claude_code_cli.go`에 `claudeCodeSkillDirName`(소문자화·`[a-z0-9-]`만 허용·대시 collapse·trim, 결과가 비면 skip) + `writeClaudeCodeSkillsPluginDir`(임시 plugin 디렉터리 materialize, name 고정 `tars-skills`, sanitize 후 중복 디렉터리는 first-writer-wins, frontmatter description은 개행을 공백으로 접어 단일 라인 보장, 본문 끝 개행 정규화) 헬퍼 추가. `Chat`은 `--mcp-config`와 동일 패턴으로 `--plugin-dir <tmp>` 인자를 끼우고 `defer cleanup()`(=`os.RemoveAll`)으로 호출 종료 시 디렉터리 제거 — skill 슬라이스가 비거나 전부 unusable이면 플래그 자체를 생략. `agent.RunOptions.ClaudeCodeSkills` + `Loop.Run`이 매 iteration `ChatOptions`로 forward(다른 provider는 silently ignore). 새 파일 `internal/tarsserver/claude_code_cli_skills.go`의 `toClaudeCodeSkills`가 세션-effective `extSnapshot.Skills`(chat 프롬프트와 동일한 snapshot 파이프라인 — `filterExtensionsSnapshotForSession` + `augmentSnapshotWithCwdSkills`로 이미 session tool_config 필터됨)를 변환하며 빈 Name 또는 빈 Content를 drop. `chatRunState.claudeCodeSkills` 필드 + `handler_chat_context.go`에서 채우고 `handler_chat_execution.go`의 `executeChatLoop`가 `RunOptions`로 넘긴다. 결과: claude-code-cli 세션 채팅에서 TARS 스킬을 워크스페이스 오염·잔여 임시파일 없이 호출 가능, 세션별 skill allowlist도 그대로 적용. 9개 회귀 케이스 추가 — provider: `TestClaudeCodeSkillDirName`(7 sanitize 케이스 포함 비ASCII/구분자-only→빈 문자열), `TestWriteClaudeCodeSkillsPluginDir_StructureAndCleanup`(manifest JSON 유효성, frontmatter+body 보존, description 개행 collapse, 빈/비ASCII/중복 skip → 정확히 2개, cleanup 후 디렉터리 부재), `TestWriteClaudeCodeSkillsPluginDir_EmptyReturnsNoFlag`, `TestClaudeCodeCLIClientChat_SkillsPluginDirPassed`(셸 stub이 `--plugin-dir` 경로의 SKILL.md를 cat·검증 + Chat 후 디렉터리 제거 확인); 변환: `TestToClaudeCodeSkills_FiltersAndMaps`/`_NilWhenEmptyOrAllFiltered`. 남은 follow-up: #869(`--settings`, 구체 use case 부재로 보류) / #871(콘솔 크레딧, Anthropic API 대기).

## [0.32.65] - 2026-05-15

### Added

- **session-scoped `claude_code_cli_permission_mode` override (Epic #857 follow-up #4)** — `.tars/settings.json` / `.tars/settings.local.json`이 새 top-level 필드 `claude_code_cli_permission_mode`를 허용한다. 값은 글로벌 설정과 동일한 enum(`auto`/`acceptEdits`/`plan`/`bypassPermissions`)이며, 빈 값/오타는 claude-code-cli provider가 이미 `"auto"`로 안전 degradation. 변경된 4개 파일: (1) `internal/sessionoverride/schema.go`의 `Override` 구조체에 `ClaudeCodeCLIPermissionMode *string`을 추가하고 `AllowedTopLevelFields`/`AllPaths()`/`EffectiveConfig`에 모두 등록해 layer source 추적(`shared`/`local` 배지)이 다른 필드와 동일하게 동작한다. (2) `internal/sessionoverride/loader.go`의 `assignTopLevel`에 새 case가 추가돼 JSON 디코드 + presence 마킹 처리. (3) `internal/sessionoverride/merge.go`의 `applyLayer`에 last-write-wins 적용(shared < local). (4) 신규 헬퍼 `internal/tarsserver/effective_session.go`의 `effectiveClaudeCodePermissionMode(svc, sess, fallback)`이 세션을 resolve해 override가 비어 있지 않으면 그 값을, 아니면 caller가 넘긴 fallback(전역 config)을 반환한다. `internal/tarsserver/handler_chat_execution.go`의 `executeChatLoop`는 이미 `state.store.Get`으로 세션을 한 번 로드해 `UpstreamSessionID`(resume 용)를 읽고 있는데, 그 동일 위치에서 같은 `priorSess`를 이 헬퍼에 넘겨 한 번에 permission mode를 결정 — 추가 IO 없음. `agent.RunOptions.ClaudeCodePermissionMode`로 최종 값 전달, 다른 provider는 silently ignore. 결과: 사용자는 글로벌 `tars.config.yaml`에 `permission_mode: auto`를 두고도 특정 프로젝트의 `.tars/settings.json`에 `"claude_code_cli_permission_mode": "plan"`만 한 줄 추가하면 그 프로젝트 안에서만 plan-only(편집·실행 금지) 모드가 적용된다. 9개 회귀 케이스 추가: loader 1(round-trip + presence), merge 3(local-beats-shared / only-shared / no-override-leaves-blank), handler 4(override-wins / fallback / nil-service / whitespace-only-falls-back). 후속 작업으로 남은 Epic #857 follow-up은 #2(`--settings` 마운트)와 #3(`.claude/skills/` 미러)인데, 둘 다 Claude Code의 실제 discovery 동작 검증이 필요한 실험 단계 — 별도 GitHub 이슈로 분리해 설계 검증 후 진입 예정. #5(콘솔 Agent SDK 크레딧 잔액)는 Anthropic이 잔액 API를 공개할 때 즉시 가능.

## [0.32.64] - 2026-05-14

### Added

- **chat 콘솔이 claude-code-cli의 provider-executed tool을 실시간 surface (Epic #857 follow-up #1 마무리)** — #865로 `agent.EventProviderTool`을 noise 없이 발화하도록 분리했고, 본 PR은 그 신호를 `internal/tarsserver/handler_chat.go`의 `setupAgentLoop` 안 `logHook` switch에 새 case로 연결해 두 가지 결과를 만든다. (1) 채팅 SSE 스트림으로 `sendStatus("provider_tool", "upstream tool executed", evt.ToolName, evt.ToolCallID, statusPreviewForTool(...), "")`를 보내 콘솔이 매 stream-json `tool_use`마다 "Claude Code ran Bash(ls)" / "Claude Code ran Read(/etc/passwd)" 같은 inline 표시를 그릴 수 있게 한다 — 같은 헬퍼 `statusPreviewForTool`을 통해 길이 180으로 잘려서 보내므로 거대한 인자(예: 큰 grep 패턴)도 콘솔 레이아웃을 깨지 않는다. (2) 세션 transcript용 `ToolCallRecord`도 한 줄 append되며 `ToolResult: "(executed by upstream provider)"` placeholder가 들어가 후일 session 재현/감사 시 TARS-side 실행과 명확히 구분된다 — `ToolIsError: false`로 고정해 우발적 에러 카운트 부풀림 방지. EventBeforeTool/EventAfterTool 경로와 달리 `recordToolUsageSignal`(TARS 호출 카운트)나 `afterTool` 훅(예: tasks 패널 refresh)은 의도적으로 호출하지 않는다 — provider가 실행한 일에 TARS-side 카운터를 더하면 router/usage 통계가 더블 카운팅된다. 새 회귀 테스트 `TestSetupAgentLoop_ForwardsProviderToolEventToStreamAndTranscript`(별도 파일 `handler_chat_provider_tool_test.go`)는 미니 `providerToolStubClient`로 `ProviderExecutedTools: [Bash(ls)]`만 들고 한 번 응답하는 시나리오를 `setupAgentLoop` 실제 결과로 돌려 (i) `provider_tool` 스트림 이벤트가 정확한 ToolName/ToolCallID/ArgsPreview로 발화되는지, (ii) `*toolCalls`에 placeholder `(executed by upstream provider)` 레코드가 추가되는지, (iii) 같은 호출에 대해 `before_tool_call` / `after_tool_call`이 같이 발화하지 않는지 세 가지 invariant를 동시에 잠근다. 다른 provider(anthropic, openai 등)의 동작은 `ProviderExecutedTools`가 항상 비어 있어 영향 없음.

## [0.32.63] - 2026-05-14

### Fixed

- **claude-code-cli `tool_use` blocks no longer trigger double-execution via agent.Loop** — Phase 1 (#858)이 `parseClaudeCodeCLIStream`이 stream-json의 `tool_use` 블록을 `ChatResponse.Message.ToolCalls`에 직접 채워 넣도록 했는데, `internal/agent/loop.go`의 본문 루프는 `Message.ToolCalls`가 비어 있지 않으면 그 호출들을 *자기* tool registry로 dispatch하려 시도한다. Claude Code의 빌트인 도구 이름(`Read`/`Edit`/`Bash`/`Glob`)은 TARS registry에 등록된 적이 없어 매 턴 "blocked tool"로 떨어지거나, 운 나쁘게 이름이 겹치는 경우엔 동일 작업이 두 번 실행되는 잠재 회귀. `internal/llm.ChatResponse`에 새 필드 `ProviderExecutedTools []ToolCall`을 추가해 "provider가 이미 자체 실행한 도구의 감사 추적"이라는 의미를 `Message.ToolCalls`("모델이 TARS에게 실행을 요청한 도구")와 분리한다. claude-code-cli provider는 `Message.ToolCalls`를 `nil`로 유지하고 모든 tool_use 블록을 `ProviderExecutedTools`로 라우팅 — 다른 provider(anthropic, openai 등)는 영향 없음(`ProviderExecutedTools`는 항상 빈 값). 기존 Phase 1 회귀 테스트 `TestClaudeCodeCLIClientChat_ParsesToolUse`도 새 필드를 어서트하도록 갱신해 잘못된 contract가 다시 자라나지 않도록 잠갔다.

### Added

- **agent.Loop이 provider-executed tools를 audit 이벤트로 surface (Epic #857 follow-up #1)** — 새 이벤트 타입 `agent.EventProviderTool`이 `internal/agent/loop.go`의 const 블록에 추가됐고, `Loop.Run`이 매 iteration의 `Chat()` 호출 직후 `resp.ProviderExecutedTools` 슬라이스를 순회하며 항목당 한 번씩 `Event{Type: EventProviderTool, Iteration, ToolName, ToolCallID, ToolArgs}`을 emit한다. 이 이벤트는 `EventBeforeTool`/`EventAfterTool`처럼 실행을 trigger하지 않으며 — 이미 Claude Code 측에서 실행됐기 때문에 — 순수 read-only 신호로 콘솔 chat stream(`stream.status`)이나 ops audit 로그가 "Claude Code가 이번 턴에 Read `/etc/passwd` + Bash `ls`를 실행했음" 같은 투명성을 사용자에게 제공할 수 있다. 회귀 테스트 두 가지가 추가됐다. `TestLoop_Run_ProviderExecutedToolsDoNotTriggerLocalExecution`은 `Message.ToolCalls: nil` + `ProviderExecutedTools: [Read, Bash]` 상태의 응답을 빈 `allowedTools`로 Loop에 흘려보내 차단 에러 없이 정상 종료하고 한 번만 호출되는지를 확인한다. `TestLoop_Run_EmitsProviderToolEvent`는 동일 응답에 hook을 달아 `EventProviderTool` 이벤트가 정확히 하나 발화되고 ToolName/ToolCallID/ToolArgs 페이로드가 보존되는지 검증한다. 후속 PR에서 `handler_chat_execution.go`의 `setupAgentLoop`이 이 이벤트를 받아 chat stream으로 흘려 사용자가 콘솔에서 Claude Code의 도구 호출을 실시간 관측 가능하도록 만들 예정 — 이번 PR은 LLM/Loop 레이어의 contract 분리와 audit signal emission에 한정.

## [0.32.62] - 2026-05-14

### Added

- **`tars doctor`: claude-code-cli 인증 모드 표시 + 2026-06-15 cutover 안내 (Epic #857 Phase 5)** — `cmd/tars/doctor_main.go`의 `checkDoctorLLMRuntime`이 claude-code-cli 바이너리 존재 확인에 더해 인증 모드(`subscription` vs `api_key`)를 추론해 함께 표시하고, 구독 모드일 때만 2026-06-15 Anthropic 정책 변경(`claude -p` / Agent SDK 사용량이 별도 월 크레딧으로 이전)을 안내한다. 새 헬퍼 `detectClaudeCodeAuthMode`가 `ANTHROPIC_API_KEY`와 `CLAUDE_API_KEY` 두 환경 변수를 차례로 검사해 어느 하나라도 trimSpace 후 비어 있지 않으면 `"api_key"` + ` (env:<varname>)` detail을 반환하고, 둘 다 없으면 `"subscription"`을 반환한다. 의도적으로 `claude config get` 같은 외부 CLI 호출은 하지 않는다(doctor가 자식 프로세스에서 막히거나 binary가 stdin을 요구할 위험). cutover 안내는 새 상수 `claudeCodeAgentSDKCutoverDate = 2026-06-15 UTC`와 비교해 그 이전에만 출력 — 날짜를 지나면 자동으로 사라져 로그를 어지럽히지 않는다. 안내 본문에는 Pro $20 / Max5x $100 / Max20x $200 크레딧 정보와 [공식 헬프 페이지](https://support.claude.com/en/articles/15036540-use-the-claude-agent-sdk-with-your-claude-plan) 링크를 포함. 기존 doctor 출력 포맷(`claude-code-cli=<path>`)에 ` auth=<mode><detail>`이 덧붙어 한 줄에서 인증 모드를 즉시 확인 가능. `cmd/tars/doctor_main_test.go`의 `clearDoctorEnv`에 `CLAUDE_API_KEY`를 추가해 기존 테스트 환경 격리를 강화하고, 새 `TestDetectClaudeCodeAuthMode`로 네 시나리오(둘 다 unset, ANTHROPIC만, CLAUDE_API_KEY만, 공백만 있는 키 — 후자는 subscription으로 떨어져야 함)를 잠갔다.

- **`tars.config.example.yaml`에 `llm.claude_code_cli.permission_mode` 추가 (Epic #857 Phase 5)** — Phase 4에서 노출한 새 설정 키를 reference config에 문서와 함께 추가한다. 허용되는 값(`auto`, `acceptEdits`, `plan`, `bypassPermissions`)과 빈 값/오타에 대한 graceful degrade 동작을 inline 주석으로 명시.

- **`CLAUDE.md`의 "LLM Provider Pool" 섹션 하단에 claude-code-cli 운영 노트 (Epic #857 완료) 추가** — 5단계의 결과물(`--resume` 멀티턴 / MCP 자동 주입 / `permission_mode` 노출 / doctor cutover 안내)을 향후 작업자가 한눈에 파악할 수 있도록 짧은 불릿으로 요약하고, 다중 사용자 시나리오와 외부 OAuth 노출 금지에 대한 비목표를 다시 한 번 명시한다.

이 PR로 Epic #857의 다섯 페이즈(tool_use 파싱 + session_id, `--resume` 멀티턴, MCP 마운트, permission_mode 설정, doctor + cutover + 문서)가 모두 완료된다. 후속 작업으로 남은 항목: `.claude/skills/` 미러, `--settings` 파일 주입, stream-json `tool_use` 이벤트 기반 동적 approval 게이팅, 콘솔 UI에 Agent SDK 크레딧 잔액 표시 — 모두 별도 이슈로 분기 권장.

## [0.32.61] - 2026-05-14

### Added

- **claude-code-cli `--permission-mode` 설정 노출 (Epic #857 Phase 4)** — 기존에 `internal/llm/claude_code_cli.go`의 `Chat`이 하드코딩하던 `--permission-mode auto`를 사용자 설정에 따라 변경 가능하게 만든다. 네 레이어 변경. (1) `internal/llm/provider.go`의 `ChatOptions`에 새 필드 `ClaudeCodePermissionMode string`를 추가하고, claude-code-cli provider만 그 값을 읽도록 문서화(다른 provider는 silently ignore). (2) `internal/llm/claude_code_cli.go`에 새 헬퍼 `resolveClaudeCodePermissionMode(raw)`가 Claude Code의 공식 enum 값 `acceptEdits`/`plan`/`bypassPermissions`/`auto` 4가지만 통과시키고 그 외(빈 문자열 포함)는 `"auto"`로 graceful degrade한다 — 미래 Claude Code 릴리스가 새 값을 추가하거나 기존 값을 제거해도 provider 호출이 통째로 실패하지 않도록. 케이스 sensitive (`"PLAN"`은 unknown → auto)로 두어 오타/잘못된 환경 변수가 의도와 다른 모드로 잘못 활성화되는 것을 막는다. (3) `internal/config/types.go`의 `LLMConfig`에 새 필드 `ClaudeCodeCLIPermissionMode string`을 추가하고 `internal/config/config_input_fields.go`에 새 input field 등록 — YAML 경로 `llm.claude_code_cli.permission_mode`, env override `CLAUDE_CODE_CLI_PERMISSION_MODE` / `TARS_CLAUDE_CODE_CLI_PERMISSION_MODE`, transform은 `strings.TrimSpace`만 적용해 enum 검증은 provider 레이어로 위임. (4) `internal/tarsserver/handler_chat.go`의 `chatToolingOptions`에 `ClaudeCodeCLIPermissionMode string` 필드를 추가하고, `internal/tarsserver/main_serve_api.go`의 `chatTooling` 설정에서 `cfg.ClaudeCodeCLIPermissionMode`를 trimSpace해 채운다. `internal/agent/loop.go`의 `RunOptions`에 동일 이름 필드를 추가하고 매 iteration `Loop.Run`이 `ChatOptions`로 forward하며, `internal/tarsserver/handler_chat_execution.go`의 `executeChatLoop`가 `deps.tooling.ClaudeCodeCLIPermissionMode`를 `RunOptions`로 넘긴다. 결과적으로 사용자가 `workspace/config/tars.config.yaml`에 `llm.claude_code_cli.permission_mode: plan`을 설정하면 모든 claude-code-cli 호출이 plan-only 모드(편집·실행 금지, 분석만)로 동작한다 — TARS 측 approval/ops 워크플로와 자연스럽게 연동되는 단계 0. 회귀 테스트 두 개를 추가했다: `TestResolveClaudeCodePermissionMode`는 8가지 입력(빈 문자열 → auto, 4가지 valid enum → 통과, 공백 trim, `"unknown"` → auto, 대소문자 다른 `"PLAN"` → auto)을 검증하고, `TestClaudeCodeCLIClientChat_PermissionModePropagatesAndFallsBack`는 다섯 subtests(empty/acceptEdits/plan/bypass/unknown)로 셸 스크립트 stub이 capture한 argv에서 `--permission-mode` 값이 의도대로 변하는지(혹은 fallback되는지)를 확인한다. Epic Phase 4의 후속 작업(stream-json `tool_use` 이벤트를 PreToolUse 훅 지점처럼 활용해 TARS approval/ops가 위험 도구를 turn 단위로 차단하는 동적 게이팅)은 별 PR로 분리되며, 권한 모드를 dynamic하게 결정하려면 먼저 그 게이팅 메커니즘이 필요하므로 본 PR의 정적(static) 설정과 별개로 다룬다. session-scoped permission mode override(`mcp_servers_extra`처럼 `.tars/settings.json`에 per-session 모드를 두는 것)도 follow-up이다.

## [0.32.60] - 2026-05-14

### Added

- **세션 단위 claude-code-cli `--mcp-config` 자동 연결 (Epic #857 Phase 3b)** — Phase 3a의 provider capability(`ChatOptions.ClaudeCodeMCPServers`)를 사용자 채팅 루프에 연결한다. 세 레이어 변경. (1) `internal/agent/loop.go`의 `RunOptions`에 `ClaudeCodeMCPServers []llm.ClaudeCodeMCPServer` 필드를 추가하고 `Loop.Run`이 매 iteration의 `ChatOptions`로 그대로 forward — 다른 provider는 silently ignore. (2) 새 파일 `internal/tarsserver/claude_code_cli_mcp.go`의 `toClaudeCodeMCPServers` 헬퍼가 `[]config.MCPServer` → `[]llm.ClaudeCodeMCPServer` 변환을 수행하며, TARS canonical transport 이름(`stdio` / `streamable_http` / `sse` / `websocket`)을 Claude Code의 `--mcp-config` 스키마의 `type` 값으로 매핑한다: `stdio`는 빈 문자열(provider의 `writeClaudeCodeMCPConfigFile`이 기본값으로 stdio 사용), `streamable_http`→`"http"`, `sse`→`"sse"`, `websocket`은 Claude Code 측 스키마에 해당 형태가 없어 **silently drop**(빈 서버를 emit하면 claude가 로드 실패하므로). 그 외 `config.NormalizeMCPServer` 후 `config.MCPServerEnabled` 검사를 통과하지 못한 항목(빈 Name, stdio without Command, remote without URL)도 drop된다. `Args` 슬라이스와 `Env`/`Headers` 맵은 모두 defensive copy로 보장해 호출자가 원본을 mutate해도 컨버터 결과는 안전하다. (3) `internal/tarsserver/handler_chat_context.go`의 `chatRunState`에 `claudeCodeMCPServers []llm.ClaudeCodeMCPServer` 필드를 추가하고, `prepareChatContext...`가 이미 `filterExtensionsSnapshotForSession`으로 만든 session-effective `extSnapshot.MCPServers`(global extensions ∪ session-scoped `mcp_servers_extra`, session `tools_custom`/`mcp_custom` allowlist 적용 결과)를 `toClaudeCodeMCPServers`에 통과시켜 state에 저장한다. `internal/tarsserver/handler_chat_execution.go`의 `executeChatLoop`가 그 값을 `agent.RunOptions.ClaudeCodeMCPServers`로 넘긴다. 결과적으로 현재 세션의 effective MCP 서버 전체 셋이 매 claude-code-cli 호출마다 임시 `--mcp-config` 파일로 마운트되어 Claude Code가 동일한 도구 풀(파일시스템, 브라우저, 데이터베이스 등)에 직접 접근 가능하다. 다른 provider 사용 시에는 capability를 silently ignore하므로 회귀 없음. `internal/tarsserver/claude_code_cli_mcp_test.go`에 네 회귀 케이스 추가: `TestToClaudeCodeMCPServers_StdioAndRemote`는 stdio + streamable_http + sse + websocket 네 종류 입력에 대한 매핑 결과를 검증(websocket drop 포함), `TestToClaudeCodeMCPServers_FiltersEmptyAndDisabled`는 빈 Name, 공백 Name, command/url 없는 broken 엔트리, 정상 엔트리 한 개를 섞은 입력에서 정상 1개만 남는지 확인, `TestToClaudeCodeMCPServers_NilOnEmptyInput`는 nil/빈 슬라이스/모두 필터된 슬라이스 세 가지 모두에서 nil이 반환되는 invariant를 잠그고, `TestToClaudeCodeMCPServers_DefensiveCopies`는 원본 `Args[0]`와 `Env["K"]`를 mutate한 뒤 컨버터 결과가 변하지 않는지를 확인한다. Phase 3의 남은 항목(`.claude/skills/` 미러, `--settings` 마운트)은 별 PR로 분리되며, 권한 모델 매핑이 핵심인 Phase 4(`permission_mode`)와 후속 운영 표면 Phase 5(`tars doctor` 인증 모드 식별 + 6/15 cutover 안내)가 우선 진행된다.

## [0.32.59] - 2026-05-14

### Added

- **claude-code-cli provider: `--mcp-config` 주입 capability (Epic #857 Phase 3a)** — `internal/llm.ChatOptions`에 새 슬라이스 필드 `ClaudeCodeMCPServers []ClaudeCodeMCPServer`를 추가하고, 새 타입 `ClaudeCodeMCPServer{Name, Transport, Command, Args, Env, URL, Headers}`로 stdio/http/sse MCP 서버를 표현한다. 호출 시 `internal/llm/claude_code_cli.go`의 `Chat`이 이 슬라이스를 받으면 새 헬퍼 `writeClaudeCodeMCPConfigFile`로 Claude Code가 이해하는 포맷 `{"mcpServers": {<name>: {type, command|url, args|headers, env|...}}}`을 임시 파일(`os.CreateTemp("", "tars-claude-mcp-*.json")`)에 직렬화하고, `--mcp-config <path>`를 argv에 끼운 뒤 호출이 끝나면 `defer cleanup()`으로 즉시 삭제한다. transport 분기는 lower-case된 값으로 판단: `"http"` 또는 `"sse"`면 type/url/headers 셋업, 그 외(빈 값 포함)는 stdio로 떨어져 command/args/env를 채운다. 빈 `Name` 엔트리는 silently skip — invalid한 `{"mcpServers": {"": ...}}` JSON이 만들어지는 걸 방지한다. 모든 엔트리가 skip되거나 슬라이스가 nil이면 `--mcp-config` 플래그 자체를 생략해 claude에 빈 config 파일을 넘기지 않는다. 이 변경은 capability surface 추가일 뿐 다른 provider는 슬라이스를 silently ignore하며 기본 호출(nil 슬라이스)은 종전과 100% 동일하게 동작한다. `internal/llm/claude_code_cli_test.go`에 두 회귀 케이스를 추가했다: `TestClaudeCodeCLIClientChat_MCPConfigPathPassedWhenServersProvided`는 stdio(`fs`, command="/usr/bin/mcp-fs") + http(`remote`, url="https://mcp.example.com/sse") 서버를 함께 넘기고, 셸 스크립트 stub이 `--mcp-config` 값으로 가리키는 임시 파일 내용을 별도 캡처 경로로 cat한 뒤 테스트가 그 JSON을 unmarshal해 두 엔트리의 type/command/url을 검증한다. 또한 임시 파일이 Chat 리턴 후 cleanup 되어 `os.Stat`이 `IsNotExist`를 반환하는지 확인해 누수가 없음을 잠근다. `TestClaudeCodeCLIClientChat_MCPConfigSkippedWhenEmpty`는 nil 슬라이스와 모두 빈 이름인 슬라이스 두 케이스 모두에서 `--mcp-config`가 argv에 들어가지 않음을 확인한다. 핸들러 통합(`handler_chat_execution`에서 `cfg.MCPServers` + session-scoped `mcp_servers_extra`를 이 슬라이스로 변환해 `agent.RunOptions`를 통해 `ChatOptions`까지 흘리는 작업) + skills/commands 미러 + `--settings` 마운트는 모두 Phase 3b 별 PR로 분리해 리뷰 단위가 작게 유지된다 — 이 PR은 LLM provider 측 capability 노출만 다룬다.

## [0.32.58] - 2026-05-14

### Added

- **세션 단위 claude-code-cli `--resume` 자동 연결 (Epic #857 Phase 2b)** — Phase 2a에서 노출한 `llm.ChatOptions.ResumeSessionID` capability를 실제 TARS 채팅 루프에 연결한다. 세 레이어가 동시에 변경된다. (1) `internal/session/session.go`의 `Session` 구조체에 새 필드 `UpstreamSessionID string` (`json:"upstream_session_id,omitempty"`)를 추가하고, 이를 `WorkDirs`/`CurrentDir` 다음 자리에 배치해 cwd 모델과 의미적으로 인접하게 둔다. 새 `Store.SetUpstreamSessionID(id, upstreamID)` 헬퍼는 `SetCurrentDir`와 동일한 인덱스 락 패턴을 사용하며, 값이 동일하면 no-op로 종료해 매 턴마다 `UpdatedAt`이 불필요하게 갱신되지 않도록 한다. 빈 문자열을 넘기면 명시적 reset이 되고, 알 수 없는 세션 id는 `ErrSessionNotFound`로 응답한다. (2) `internal/agent/loop.go`의 `RunOptions`에 `ResumeSessionID string`를 추가하고, `Loop.Run`이 그 값을 첫 iteration의 `ChatOptions.ResumeSessionID`로 시드한 뒤 매 이터레이션 응답에서 `resp.SessionID`를 읽어 `activeResumeID`를 갱신한다 — 이 덕분에 fresh 세션으로 시작한 경우(iter 1에서 caller intent가 비어 있고 provider가 새 id를 발급)에도 iter 2 이후로는 같은 업스트림 세션에 부착된다. (3) `internal/tarsserver/handler_chat_execution.go`의 `executeChatLoop`가 `loop.Run` 호출 직전 `state.store.Get`으로 현재 세션의 `UpstreamSessionID`를 읽어 `RunOptions.ResumeSessionID`로 넘기고, 호출 직후 응답의 `chatResp.SessionID`가 비어 있지 않고 직전 값과 다를 때만 `SetUpstreamSessionID`로 다시 디스크에 저장한다 — persist 실패는 debug 레벨로 로깅하고 계속 진행한다 (다음 턴이 fresh로 시작될 뿐 사용자 흐름을 막지 않는다). 결과적으로 사용자가 동일 TARS 세션에서 두 번째 메시지를 보내면 claude-code-cli provider는 `--resume <stored_id>` + 마지막 user 메시지만 들고 호출되어 시스템 프롬프트/이전 트랜스크립트를 재과금하지 않는다. 다른 LLM provider는 `ResumeSessionID`를 silently ignore하므로 무영향. 회귀 픽스처 4종이 추가됐다: `TestStoreSetUpstreamSessionID_PersistsAndRoundTrips`(공백 trim + read-back), `TestStoreSetUpstreamSessionID_NoBumpWhenUnchanged`(동일값 set 시 UpdatedAt 보존), `TestStoreSetUpstreamSessionID_UnknownSession`(sentinel 에러), 그리고 `internal/agent/loop_test.go`의 `scriptedLLMClient`에 `seenResumeIDs` 추적 필드를 더하고 `TestLoop_Run_ThreadsResumeSessionID`(빈 caller intent → iter 1 빈 string → 응답 SessionID 채택 → iter 2 "upstream-fresh-1")와 `TestLoop_Run_HonorsCallerResumeSessionID`(caller가 "carried"를 넘기면 iter 1이 그대로 받음)로 두 갈래 시나리오를 잠갔다. Phase 2의 비용 절감 목표(시스템 프롬프트 재과금 회피)와 Anthropic 6/15 Agent SDK 크레딧 정책 대응이 이 PR로 사용자 표면까지 도달한다.

## [0.32.57] - 2026-05-14

### Added

- **claude-code-cli provider: `--resume <session_id>` 지원 (Epic #857 Phase 2a)** — `internal/llm.ChatOptions`에 새 필드 `ResumeSessionID`를 추가하고, `internal/llm/claude_code_cli.go`의 `Chat`이 이 값을 받으면 두 가지 동작을 동시에 토글한다: (1) 인자에 `--resume <id>`를 끼우고 그 대신 `--no-session-persistence` 플래그를 빼서 Claude Code 측 저장된 세션 트랜스크립트가 실제로 로드될 수 있게 하고, (2) 새 헬퍼 `extractLatestUserMessage`로 마지막 user 메시지의 Content만 prompt 인자로 넘긴다. 풀 transcript 텍스트 빌더(`buildClaudeCodeCLIPrompt`의 "Continue the conversation below…" / `USER:` / `ASSISTANT:` 직렬화)는 resume 모드에서 의도적으로 우회 — 업스트림 세션이 이미 같은 히스토리를 들고 있어 재전송 시 토큰 중복 과금과 컨텍스트 혼선이 발생한다. ResumeSessionID가 비어 있는 (default) 호출은 종전과 100% 동일하게 동작하므로 기존 호출자에게는 무영향 변경. Anthropic이 6/15부터 공식 허용하는 Agent SDK / `claude -p` 흐름의 핵심 비용 절감 메커니즘(다음 턴에서 system prompt를 다시 보내지 않고 세션 핸들만 넘기는 것)이 provider 레이어에서 이제 가능. `internal/llm/claude_code_cli_test.go`에 두 회귀 케이스를 추가했다: `TestClaudeCodeCLIClientChat_ResumeSessionPassesFlagAndSlimsPrompt`는 4-turn 트랜스크립트(system + user + assistant + user)를 ResumeSessionID="sess-abc"와 함께 호출해 셸 스크립트 stub으로 capture한 argv가 `--resume sess-abc`를 포함하고 `--no-session-persistence`가 빠졌으며 "follow-up please"만 들어가고 옛 transcript 흔적("USER:", "ASSISTANT:", "old turn 1", "Continue the conversation below")이 전혀 포함되지 않음을 검증하고, `TestClaudeCodeCLIClientChat_FreshSessionKeepsNoSessionPersistence`는 ResumeSessionID 미지정 시 `--no-session-persistence`가 유지되고 `--resume`이 없는지 잠근다. 세션 ID를 TARS `session.Session`에 매핑·저장하고 `handler_chat`이 그 값을 자동으로 ChatOptions에 넘기는 통합 작업(Phase 2b)은 별 PR로 들어간다 — 이 PR은 LLM 레이어의 capability 노출만 다룬다.

## [0.32.56] - 2026-05-14

### Added

- **claude-code-cli provider: tool_use 파싱 + session_id 캡처 (Epic #857 Phase 1)** — `internal/llm/claude_code_cli.go`의 `parseClaudeCodeCLIStream`이 그동안 stream-json의 `assistant.message.content[]` 안에 들어오는 `tool_use` 블록을 통째로 버리고 `text` 블록만 추출하던 동작을 고친다. 새 `extractClaudeCodeAssistantBlocks` 헬퍼는 같은 콘텐츠 배열을 한 번 순회하며 `text`는 누적 builder로, `tool_use`는 `ToolCall{ID, Name, Arguments}` 슬라이스로 분리한다. `input` 필드는 `json.Marshal`로 JSON 문자열화해 `ToolCall.Arguments`에 저장 — 다른 provider(anthropic native, openai 등)가 이미 사용하는 직렬화 형태와 동일하므로 `internal/agentruntime`/`internal/tool` 쪽 라우팅은 무수정으로 받는다. 파서는 또한 어느 이벤트든 최상위 `session_id` 키가 보이면 캡처하므로(`system.init` → `result` 순서로 두 번 들어와도 동일 ID로 수렴) Anthropic이 6/15부터 공식 허용하는 Agent SDK / `claude -p` 호환 흐름의 첫 단계인 "세션 핸들 확보"가 이루어진다. 새로 추가된 `SessionID` 필드는 `internal/llm.ChatResponse`에 노출되어(다른 provider는 빈 문자열 유지) Phase 2의 `--resume <session_id>` 멀티턴 호출이 이 값을 그대로 받아쓸 예정. 회귀 방지를 위해 `internal/llm/claude_code_cli_test.go`에 두 케이스를 추가했다: `TestClaudeCodeCLIClientChat_CapturesSessionInit`은 `result` 이벤트가 도착하기 전에 `system.init`에서 session_id가 캡처되는지 검증하고, `TestClaudeCodeCLIClientChat_ParsesToolUse`는 한 응답 안에 텍스트 + tool_use + tool_result + 후속 텍스트가 섞인 stream-json 픽스처를 셸 스크립트 stub으로 흘려보내 `ChatResponse.Message.ToolCalls`에 `Read` 호출이 `/tmp/a.txt` 인자와 함께 들어오고 최종 텍스트는 두 assistant 턴이 합쳐진 형태로 노출되는지 확인한다. 기존 `TestClaudeCodeCLIClientChat_ParsesStreamJSON`에도 `resp.SessionID == "sess-1"` 단언을 추가해 단일 턴 케이스에서도 동일 동작을 잠근다. 파싱 외 동작(플래그, working dir, system prompt 빌더, transcript 텍스트 직렬화)은 의도적으로 손대지 않았다 — `--input-format stream-json` 입력 모드 전환, `--resume` 사용, MCP/skills/settings 마운트, permission 모드 매핑은 모두 후속 페이즈에서 독립 PR로 들어간다.

## [0.32.55] - 2026-05-14

### Fixed

- **exec tool: drop streamed lines on CPU-saturated hosts** — `internal/tool/exec.go` ran `cmd.Wait()` before `wg.Wait()`. When the child process exited microseconds after `cmd.Start()` (typical for `printf` / `ls`) and Go's scheduler had not yet picked up the `scanAndCapture` goroutines (typical on CI runners where many packages run in parallel), `cmd.Wait()` returned first and closed our read fds before the scanner goroutines drained the pipes. The goroutines then woke up against a closed fd, `bufio.Scanner` saw immediate EOF, and zero events reached `ToolOutputStreamer` — even though the buffered output still made it into the final `stdout`/`stderr` strings (those reads happen before `cmd.Wait` closes the fds). Visible on saturated CI hosts (e.g. SonarCloud workflow on `ubuntu-latest`) as flaky `TestExecTool_StreamsStdoutLinesViaContext` / `TestExecTool_StreamsStderrSeparately` failures with `got 0 (events=[])`. Per the [`os/exec.Cmd.Wait`](https://pkg.go.dev/os/exec#Cmd.Wait) contract — *"it is incorrect to call Wait before all reads from the pipe have completed"* — the fix swaps the order: `wg.Wait()` first lets the scanner goroutines drain pipes naturally (they reach EOF when the child closes its stdout/stderr on exit), then `cmd.Wait()` reaps the process. New `TestExecTool_StreamsCaptureFastExitUnderContention` regression test forces `GOMAXPROCS=1` and runs the streaming exec 25 times in series; with the original code it fails 100% on the very first iteration with the same `got 0 (events=[])` message seen in CI, with the fix it stays green.

## [0.32.54] - 2026-05-14

### Added

- **Console: PWA shortcuts + topbar status pill (#850 Phase 2)** — Builds on the Phase 1 PWA shell with two thin desktop-style affordances. `manifest.webmanifest` now declares a `shortcuts` array with five entries (Chat → `/console/chat`, Sessions → `/console/sessions`, Ops → `/console/ops`, Pulse → `/console/pulse`, Reflection → `/console/reflection`), each carrying name/short_name/description/url and the existing 192px icon as a fallback. Installed PWAs surface these on right-click of the Dock/taskbar/launcher icon, so users can deep-link straight into the canonical routes without navigating from `/console/` home. New `frontend/console/src/components/StatusPill.svelte` replaces the old single connected/disconnected indicator in `Header.svelte` with an aggregated pill (`● ● ● · N`) showing one dot each for server / pulse / reflection plus an active visible-session count. The component polls `getPulseStatus`, `getReflectionStatus`, and `listSessions(false, 'active')` every 15s and derives a per-subsystem `ok | warn | error | idle` level using existing snapshot fields (pulse `last_err` / `last_tick_at` recency vs 5m staleness, reflection `consecutive_failures`). Clicking the pill opens a 280px popover with a clickable row per subsystem (Pulse → `/console/pulse`, Reflection → `/console/reflection`, Sessions → most-recent visible session's chat) and a footer with `Open Ops` + `Open active chat` buttons. The previous `header-indicator` markup and CSS are removed since the pill subsumes the connected/disconnected signal. No new endpoints are added; status is composed entirely from existing admin/session/pulse/reflection APIs. `docs/console-install.md` documents both surfaces. Source-grep tests in `frontend/console/tests/statusPill.test.ts` lock the wiring.

## [0.32.53] - 2026-05-14

### Added

- **Console: PWA-first install surface (#850 Phase 1)** — TARS Console is now installable as a desktop-style PWA. New `frontend/console/public/manifest.webmanifest` declares `name`/`short_name`/`id`/`start_url` rooted at `/console/`, `display: standalone` with `window-controls-overlay` override, `theme_color`/`background_color` matching the dark theme (`#141414`), and a four-entry icon set (`pwa-icon-192.png`, `pwa-icon-512.png`, `pwa-maskable-192.png`, `pwa-maskable-512.png`) generated from `docs/brand/tars-icon.png` with a 10% safe-zone padded variant for maskable purpose. `frontend/console/index.html` adds `<link rel="manifest">`, `<link rel="apple-touch-icon">` (180×180), `<meta name="theme-color">`, and `apple-mobile-web-app-*` meta tags so iOS/macOS Safari "Dock에 추가" picks up the correct title/icon. Server side, `serveConsoleAsset` in `internal/tarsserver/console.go` now sets `Content-Type: application/manifest+json` for `.webmanifest` (Go's mime registry returns the wrong type by default). New `docs/console-install.md` documents the four access paths (browser, PWA/Add to Dock, CLI, LaunchAgent service) with per-browser install instructions for Chrome/Edge/Brave/Arc/Safari and notes Firefox's lack of desktop PWA support. No backend behavior change; the existing `/console/*` static handler serves the new assets directly.

## [0.32.52] - 2026-05-14

### Changed

- **tools/go.mod — bump security-scan dependencies** (replaces Dependabot #841) — Bumps `golang.org/x/crypto` 0.35.0 → 0.45.0, `github.com/ulikunitz/xz` 0.5.12 → 0.5.15 (addresses GHSA-jc7w-c686-c4v9), and `github.com/nwaples/rardecode/v2` 2.1.0 → 2.2.0. The original Dependabot PR failed `security` CI because rardecode/v2 v2.2.0's `ReadCloser` no longer satisfies the internal `rarReader` interface used by `github.com/mholt/archives` v0.1.2 (`missing method WriteTo`). Bundles a co-bump of `github.com/mholt/archives` 0.1.2 → 0.1.5 to restore compilation, which pulls forward the rest of the transitive tree (`brotli`, `klauspost/compress`, `sevenzip`, `lz4/v4`, `lzip-go`, `STARRY-S/zip`, `afero`, `mikelolasagasti/xz`, `minlz`, `golang.org/x/{sync,sys,text}`). No production code change — these only affect the `make security-scan` toolchain.

## [0.32.51] - 2026-05-14

### Changed

- **Console: surface async critic feedback + assistant_turn trigger (PR-3 of 3)** — `SessionConfigPanel.svelte` Automation tab now labels the Critic agent as "Every assistant turn · async background review" and the max-rounds helper clarifies the budget only applies to plan transitions (assistant turns are unbounded). The runtime status row now also surfaces `last_trigger`, and the iteration counter is hidden for `assistant_turn` triggers where it would always read 0. New amber-bordered "Pending feedback queued" card appears whenever `critic.pending_feedback` is non-empty (with trigger + round badge and the full feedback body) so users can see what the reviewer flagged before it drains into their next prompt. `SessionCritic` TypeScript type extended with `last_reviewed_turn_sig` / `pending_feedback` / `pending_feedback_trigger` / `pending_feedback_round` / `pending_feedback_at` to match the backend payload from PR-2. Pure presentational change; no new API calls.

## [0.32.50] - 2026-05-14

### Changed

- **Critic agent: assistant_turn trigger + async execution (PR-2 of 3)** — The reviewer is no longer gated on plan transitions. A new `critic.TriggerAssistantTurn` fires at the end of every assistant turn that did not also cross a `plan_proposed` / `plan_completed` transition, with a dedicated response-quality system prompt. Execution flips from blocking to background: `buildCriticAwareTurnEndHook` now spawns a goroutine via the package-level `criticAsyncRunner` (test-overridable) on a 2-minute timeout context decoupled from the originating chat request. Non-acceptable verdicts queue into new `SessionCritic.PendingFeedback` / `PendingFeedbackTrigger` / `PendingFeedbackRound` / `PendingFeedbackAt` fields, which `buildSessionChatRunState` drains via `Store.TakePendingCriticFeedback` at the start of the next user turn and inserts as a system-role message immediately before the user message (`insertSystemMessageBeforeUser`). The chained `buildChatTurnEndHook` no longer pre-empts the goal judge with a critic injection — both run, but the goal judge keeps its existing synchronous auto-continue contract while the critic side-channels through the pending queue. Iteration budgeting still applies to plan triggers; `assistant_turn` does not bump `CurrentIteration` and dedupes through `LastReviewedTurnSig` (SHA-1 of response content + tool-call ids) so repeated firings on the same response are no-ops. SSE events (`started` / `feedback` / `satisfied` / `judge_error` / `exhausted`) keep the same shape for the console.

## [0.32.49] - 2026-05-13

### Changed

- **Critic agent now works in every session kind (PR-1 of 3)** — Removed the `main`-only guard from `Store.SetCritic` so the `/v1/admin/sessions/{id}/critic` API can attach a reviewer to worker, subagent, or forked sessions. New worker sessions created via `EnsureWorker` inherit the main session's critic config (Enabled + MaxIterations only — runtime fields reset to idle). Subagent sessions spawned through the agent runtime also copy their `ParentSessionID`'s critic at creation. The 400 "only main sessions support a critic agent" response from `PUT /v1/admin/sessions/{id}/critic` is gone; worker-kind PUTs now return 200 with the stored config. New `session.InheritCriticConfig` helper covers the config-only clone (disabled sources still yield nil so we never register a stub). This is purely the data-model change; the assistant-turn trigger and async execution model land in follow-up PRs.

## [0.32.48] - 2026-05-13

### Docs

- **Skill hub federation documentation pass** — Refreshed `docs/tutorials/14-skill-hub.md` with a Hub Federation section covering the four registered sources (`tars-hub` / `openclaw` / `hermes` / `anthropic`), the optional capability interfaces (`SkillContentConverter`, `LicenseFetcher`, `CompanionFileLister`), the external-hub install flow with `PreviewInstall` and post-confirm sha256 drift detection, license compliance automation (MIT/Apache-2.0 ATTRIBUTION.md + Proprietary block), and CLI usage examples. `docs/plugins.md` gains a short "External Hubs" subsection under "Installing From The Hub" with the same `--from <hub>` cheatsheet. `README.md` Extensibility section now lists external hubs as a first-class entry with links to the upstream repos. `GETTING_STARTED.md` Section 10 adds an "Installing from external hubs" block walking through `--dry-run`, `--format json`, `--yes`, and the Proprietary block. No code or behaviour changes; documentation only.

## [0.32.47] - 2026-05-13

### Added

- **Console hub selector + dry-run modal (skill hub federation Phase 5)** — Extensions page now drives external-hub installs through the same federation layer as the CLI. New backend endpoints `GET /v1/hub/sources` (lists every registered HubSource with a human-readable label) and `GET /v1/hub/skills?source=&q=` (federated skill search across all sources) power a hub selector dropdown above the Hub Skills list and a per-card source badge (`tars-hub` / `openclaw` / `hermes` / `anthropic`). External-hub installs open a new `DryRunModal.svelte` showing the converted frontmatter, every file with size and short sha256, adapter warnings (e.g. openclaw install-block skips), checksum mismatches, and the ATTRIBUTION.md notice; only after the user clicks Install does the materialize call go out (`yes: true`). `Installer.Update` now routes each installed skill back to its recorded HubSource (tars-hub keeps version-string compare, external hubs always re-download and let sha256 drift handling decide). `ensureSources` refreshes the built-in tars-hub adapter when callers swap `inst.Registry` mid-flight, preserving the pre-federation regression test. ko/en i18n keys cover every new label.

## [0.32.46] - 2026-05-13

### Added

- **hermes + Anthropic skill hub adapters (skill hub federation Phase 4)** — `tars skill install --from hermes <name>` and `--from anthropic <name>` now pull skills from `NousResearch/hermes-agent` and `anthropics/skills` respectively. The hermes adapter handles the two-level directory layout (`skills/<category>/<name>/SKILL.md`) by indexing the repo via a single recursive GitHub Trees API call and surfaces an ambiguous-name error if the same `<name>` appears in multiple categories. `metadata.hermes.tags` becomes the TARS `tags` field; `metadata.hermes.related_skills` and any other hermes-specific keys are preserved under `metadata.adapter_origin.hermes`. The Anthropic adapter handles the flat `skills/<name>/` layout and a **per-skill `LICENSE.txt`** (anthropics/skills has no repo-level LICENSE). Apache-2.0 skills materialize an ATTRIBUTION.md with a NOTICE section; the proprietary `docx`/`pdf`/`pptx`/`xlsx` skills are detected by `DetectLicenseLabel` and **refused** at the attribution layer with a clear error — TARS does not import source-available content. Both adapters auto-register in `cmd/tars/skill_installer.go` and `internal/tarsserver/main_serve_api.go`, so `tars skill search` (no `--from`) now spans all four hubs in one query.

## [0.32.45] - 2026-05-13

### Added

- **External-hub install dry-run (skill hub federation Phase 3)** — `tars skill install --dry-run [--format text|json]` now downloads + converts a skill from an external hub, prints a structured preview (source, target dir, converted frontmatter, per-file sha256 + size, adapter warnings, license label, ATTRIBUTION presence) and returns without touching the workspace. Non-interactive shells (pipes, `< /dev/null`, CI) without `--yes` or `--dry-run` are now refused up front with a clear message — previously they fell through to a confirm prompt that always aborted on EOF. TTY detection switched from `os.Stdin.Stat()` (which misclassifies `/dev/null` as interactive because it is a character device) to `github.com/mattn/go-isatty`. Internally `Installer.PreviewInstall(ctx, ref)` builds the new `DryRunResult` (source, original URL, converted skill snapshot, per-file `FilePreview` with computed and expected sha256, adapter warnings, license label, checksum-drift warnings); `Installer.InstallWithOptions` gained `DryRun bool` and `OnPreview func(*DryRunResult)` hooks, and the legacy `Install(ctx, ref)` keeps working unchanged by delegating with auto-approve. A post-confirm content-drift detector re-fetches before materialize and refuses to write files whose sha256 changed between preview and confirm. HTTP API: `POST /v1/hub/install` accepts `source`, `yes`, `dry_run` fields; dry-run responses include the preview JSON. Server's hub installer now auto-registers the openclaw source so the console (Phase 5) can drive external installs without separate wiring.

## [0.32.44] - 2026-05-13

### Added

- **openclaw skill import (skill hub federation Phase 2)** — `tars skill install --from openclaw <name>` now pulls a skill from the openclaw repo, converts its JSON-in-YAML frontmatter into TARS format, materializes both the rewritten `SKILL.md` and an `ATTRIBUTION.md` (MIT, with the original copyright and license body fetched from the openclaw LICENSE), and records the install with `source: "openclaw"` in `skillhub.json`. The CLI gains `--from <hub>` (search / install / info) and `--yes` (install), and external-hub installs route through a new `Installer.InstallWithOptions` that asks for explicit approval via a `ConfirmFn` callback before materializing files — non-interactive shells without `--yes` get a clear error. openclaw `install[]` blocks (brew / apt / npm package-manager hooks) are surfaced as `metadata.adapter_warnings.install_blocks_skipped` in the converted SKILL.md and printed in the preview but never executed. Internally, `HubSource` now has three optional capability interfaces (`SkillContentConverter`, `LicenseFetcher`, `CompanionFileLister`); the openclaw adapter implements all three and lives in `internal/skillhub/sources/openclaw/`. The wiring (which external sources auto-register with `NewInstaller`) lives in `cmd/tars/skill_installer.go` so the `skillhub` package does not import its own subpackages. hermes and Anthropic adapters land in Phase 4.

## [0.32.43] - 2026-05-13

### Added

- **Skill hub federation foundation (Phase 1)** — Introduced a `HubSource` interface and `SourceRegistry` in `internal/skillhub/` so the installer can route skill operations through pluggable hub adapters instead of the hard-coded `tars-hub` registry. The built-in tars-hub registry is now exposed as `TarsHubSource`, which `NewInstaller` registers by default so existing call sites and tests keep working. `Installer.Install` now accepts both bare names and `<source>:<name>` refs (via `ResolveSkillRef`), records the resolved source ID on `InstalledSkill.Source` instead of a hard-coded string, and raises an explicit ambiguity error when more than one registered source advertises the same name. Legacy `skillhub.json` rows with an empty `Source` are migrated to `tars-hub` at load time. No external hub adapters land in this phase — the openclaw / hermes / Anthropic adapters arrive in Phase 2 and Phase 4 of the federation work.

## [0.32.42] - 2026-05-12

### Added

- **Critic agent for plan transitions** — New session-scoped critic mode that auto-reviews the main LLM's freshly proposed or just-completed plans. Toggled per main session via `PUT /v1/admin/sessions/{id}/critic` (`{enabled, max_iterations}`) and the **Critic agent** entry in the chat **Session config → Automation** panel. When enabled, `buildChatTurnEndHook` runs the critic before the goal judge: on `plan_proposed` / `plan_completed` status transitions it calls the new `RoleCritic` LLM (defaults to the standard tier) and, if the verdict is not acceptable, injects concrete feedback as a system-tone message so the chat loop auto-iterates up to `max_iterations` rounds (default 3, hard-capped at 5). When the reviewer accepts the plan or the budget is exhausted, the chain falls through to the existing `/goal` judge — so critic and goal can coexist without stacking auto-continue verdicts in a single turn. New SSE event `critic_event` (`started` / `feedback` / `satisfied` / `exhausted` / `judge_error`) lets the console surface progress without polling. Critic state (`current_iteration`, `last_feedback`, `last_reviewed_plan_sig`) is persisted on the session so a concurrent admin clear or transcript edit takes effect on the next turn. Like the goal judge, the reviewer fails open: any parse/network error stops the cycle without marking the plan acceptable.

## [0.32.41] - 2026-05-12

### Added

- **`/goal` session-goal feature** — New `/goal <description>` slash command, REST API (`GET|PUT|DELETE /v1/admin/sessions/{id}/goal`), and goal-aware agent loop. When a main session has an active `SessionGoal`, the chat handler appends the goal to the system prompt and runs an independent judge LLM (role `goal_judge`, defaults to the standard tier) at the end of each turn: a `satisfied` verdict clears the goal automatically; a `not satisfied` verdict triggers up to `max_auto_continues` (default 3) auto-continue iterations before the goal is marked `exhausted`. Frontend additions: an amber `goal: …` chip in the chat session header (next to the cwd chip), an `onGoalEvent` SSE channel (`goal_event`: `auto_continue` / `satisfied` / `exhausted` / `judge_error`), and `getSessionGoal`/`setSessionGoal`/`clearSessionGoal` API clients. The judge fails open: any error or unparseable verdict stops the auto-continue loop without marking the goal satisfied, so a judge outage can never accidentally clear a real goal.

## [0.32.40] - 2026-05-12

### Security

- **Pin MCP HTTP RPC destination to validated origin** — Replaced `assertSameOriginAsServerURL` with `pinEndpointToServerOrigin`, which returns a `*url.URL` whose Scheme and Host are copied from the pre-validated `ps.serverURL` rather than from the caller-supplied endpoint string. `doHTTPRPC` now reassigns `httpReq.URL` to that pinned URL after `http.NewRequestWithContext` so the destination of `httpClient.Do` is structurally anchored to validated config, even though `http.NewRequestWithContext` itself re-parses from a string. Added boundary coverage in `TestPinEndpointToServerOrigin` (origin pin, scheme/host rejection, query/fragment passthrough, aliasing guard). CodeQL alert #11 (critical, `go/request-forgery`) is dismissed as a false positive after merge — the URL must derive from admin-configured MCP server config, so CodeQL's taint analysis flags this by design even though `pinEndpointToServerOrigin` structurally anchors Scheme/Host to validated config.

## [0.32.39] - 2026-05-12

### Security

- **Pin loopback-only cookie boundary** — Added `internal/tarsserver/handler_auth_loopback_cookie_test.go` to document and pin the trust boundary for the two intentional `Secure: false` cookie writes (`writeSessionCookie`/`writeClearSessionCookie` loopback branches). The audit asserts (1) every loopback Host literal (`127.0.0.1`, `localhost`, `[::1]`, with and without ports, case-insensitive) is the only configuration where `evaluateLoginRequest` returns `CookieSecure: false`, and (2) every non-loopback Host — public IP/DNS, RFC1918, Tailscale `.ts.net`, with or without TLS — must either be rejected outright or be granted `CookieSecure: true`. `shouldClearSecureCookie` is also verified to mirror the login decision so a logout cookie cannot strand a Secure session cookie on the user. The two CodeQL `go/cookie-secure-not-set` alerts (#100, #101) flagging the loopback fallback are dismissed as false positives with evidence pointing at this audit file.

## [0.32.38] - 2026-05-12

### Security

- **Pin static upper bound on analytics window allocation** — `Tracker.Analytics` now routes `days` through `capAnalyticsDays(clampAnalyticsDays(days))` immediately before `make([]*analyticsDailyAccumulator, 0, days)`. `clampAnalyticsDays` already returns one of {7, 30, 90}, but CodeQL `go/uncontrolled-allocation-size` cannot prove the switch is a sanitizer; `capAnalyticsDays` exposes the literal `analyticsDaysHardCap = 90` upper bound the rule (and reviewers) can see, and falls back to the cap for any out-of-range input. `TestCapAnalyticsDays` exercises both branches directly and asserts the cap is a no-op for every value `clampAnalyticsDays` can return. Resolves CodeQL alert #85 (high, `go/uncontrolled-allocation-size`).

## [0.32.37] - 2026-05-12

### Security

- **Force containment on workspace skill admin paths** — `resolveWorkspaceSkillPaths` now returns `(dir, file string, err error)` and rejects any name that fails `validateSkillCreatorName` *before* building a path. Both the snapshot-match branch and the `<workspace>/skills/<name>/` fallback share one `confine()` helper that resolves an absolute `<workspace>/skills/` root and checks `filepath.Rel` does not escape it. The admin `/v1/admin/skills/{name}` handler propagates resolver errors to a 400 so the skill name can never reach `os.ReadFile`/`MkdirAll`/`WriteFile`/`Stat`/`RemoveAll` without crossing the explicit containment barrier. Added `TestResolveWorkspaceSkillPaths_RejectsInvalidName` (covers empty, `..`, traversal, slashes, mixed-case, underscore, oversize) and updated `TestResolveWorkspaceSkillPaths_RejectsTraversal` to assert the returned paths are always anchored under the absolute skills root. Resolves CodeQL `go/path-injection` alerts #89, #90, #91, #92, #93 (high).

## [0.32.36] - 2026-05-12

### Security

- **Pin MCP HTTP RPC destination to validated origin** (#831) — Replaced `assertSameOriginAsServerURL` with `pinEndpointToServerOrigin`, which returns a `*url.URL` whose Scheme and Host are copied from the pre-validated `ps.serverURL` rather than from the caller-supplied endpoint string. `doHTTPRPC` now reassigns `httpReq.URL` to that pinned URL after `http.NewRequestWithContext` so the destination of `httpClient.Do` is structurally anchored to validated config, even though `http.NewRequestWithContext` itself re-parses from a string. This resolves CodeQL `go/request-forgery` alert #11 (critical) and added boundary coverage in `TestPinEndpointToServerOrigin` (origin pin, scheme/host rejection, query/fragment passthrough, aliasing guard).

## [0.32.35] - 2026-05-11

### Security

- **Audit local-state path construction** (#806) — Documented and pinned the trust boundary for the app-owned state writers that CodeQL `go/path-injection` flagged (atomicwrite, cron run files, session transcripts/tasks, sessionoverride loader, skill/command loaders). The path in every flagged `os.*` call is `filepath.Join(<app-owned root>, <id-or-name>, ...)`, where the id-or-name is either server-generated (`generateID` produces 16-char hex; `newJobID` produces `job_` + 8-byte hex), index-checked before the call (e.g. `session.Store.Delete` short-circuits if the id is not already in the on-disk index), or constrained to a kebab-case/allowlist pattern (skill/command names). Two new audit test files (`internal/atomicwrite/local_state_audit_test.go`, `internal/session/local_state_audit_test.go`) make that boundary explicit and exercise the round-trip + traversal-id no-op cases. The 18 alerts (#16–#33) flagging `os.*` calls downstream of these constructors are dismissed as false positives with evidence pointing at the audit files.

## [0.32.34] - 2026-05-11

### Security

- **Document and pin console/admin local-path API boundary** (#805) — `isValidAgentRuntimeAgentName` now rejects all-dot names ("..", "...", ...) so a subagent draft cannot save to `<workspace>/agents/../AGENT.md` (it still allows "." inside a name, e.g. `release.candidate`). Extracted `assertFilesystemBrowsePathShape` from the `/v1/filesystem/browse` GET handler so the absolute-path contract is reusable and testable, and added `internal/tarsserver/local_path_audit_test.go` to assert: (1) every local-path-touching handler (`/v1/admin/*`, `/v1/terminal/*`, agent runtime reload/restart, runtime extensions reload) sits behind `apiAdminPaths`; (2) `validateWorkspaceDirectoryName` rejects traversal, separators, and dot-only names; (3) `isValidAgentRuntimeAgentName` rejects dot-only and separator-bearing inputs; (4) `cleanSkillCreatorFilePath` rejects absolute paths and parent-escaping traversal while normalizing safe interior `..`; (5) `validateSkillCreatorName` enforces kebab-case. The 23 CodeQL `go/path-injection` alerts (#34–#56) flagging `os.*` calls downstream of these validators are dismissed as false positives with evidence pointing at this test file.

## [0.32.33] - 2026-05-11

### Security

- **Pin workspace file-tool path containment** (#804) — Added `internal/tool/workspace_path_test.go` to document and exercise the shared sanitizer boundary every workspace file tool (`read_file`, `write_file`, `edit_file`, `list_dir`, `project_skill`) routes through: `resolvePathWithPolicy`/`resolveWritePathWithPolicy` for the policy entrypoints, and `resolveWorkspacePath`/`resolveWorkspaceWritePath` for the workspace-anchored variants. Regression cases cover `..` traversal, absolute paths outside every `AllowedDir`, symlink escape on read, symlinked-ancestor escape on write, missing nested write targets, workspace-root-prefixed inputs, and the `policyRelativePath` display helper. The 13 CodeQL `go/path-injection` alerts (#67–#82) flagging `os.*` calls downstream of these sanitizers are now dismissed as false positives with evidence pointing at this test file.

## [0.32.32] - 2026-05-11

### Security

- **Replace regex-based markdown HTML sanitization** (#803) — The console markdown renderer no longer relies on a single `<script>...</script>` regex (which CodeQL flagged as both incomplete on `<scr<script>ipt>` obfuscation and unable to match whitespace-padded `</script >` end tags). `renderMarkdown` now runs DOMPurify with an allowlist that strips `<script>`, `<style>`, inline event handlers, and `javascript:`/`vbscript:`/`data:` URLs while preserving the toolbar/mermaid/code-preview markup the chat UI relies on. To avoid DOMPurify's mXSS heuristic stripping `data-graph`/`data-code` attributes whose values contain `-->` (e.g. mermaid arrow syntax), those payloads are now URL-encoded at render time and decoded on read via the new `readEncodedAttr` helper.

## [0.32.31] - 2026-05-11

### Security

- **Harden MCP and skill execution boundaries** (#802) — Stdio MCP servers now resolve their command via `exec.LookPath` and reject any binary outside `mcp_command_allowlist_json` before `getOrStartSession` spawns a process, instead of relying on the allowlist being checked only by `ListTools`/`BuildTools`. Remote MCP transports (streamable HTTP, legacy SSE, WebSocket) parse and validate the configured URL scheme/host once at session start, store the parsed `*url.URL`, and re-assert the same scheme+host for every subsequent HTTP RPC. The legacy SSE `endpoint` event is now constrained to the configured origin so a malicious server can no longer redirect JSON-RPC POSTs to an attacker-controlled host. The skill creator sandbox test runner resolves the companion CLI path inside the freshly created sandbox via `filepath.Abs` + containment check and hands an absolute path to `exec.CommandContext` rather than the user-supplied relative string.

## [0.32.30] - 2026-05-10

### Changed

- **Codex quota gets visual progress bars** — `/status` now prints a 10-cell ASCII bar (`██░░░░░░░░`) per window so usage is readable at a glance, with the window total length added to the reset countdown (e.g. `(resets 3h 45m / 5h)`). The chat feedback bar preserves newlines and switches to a monospace, left-aligned layout when the message contains line breaks, so multi-line `/status` output renders cleanly instead of being squashed into one wrapped paragraph (single-line feedbacks like `Chat view cleared` still show centered in the default font). The Analytics page Codex Quota card replaces the static percentage with a horizontal progress bar that tints amber/red in the warn/critical bands.

## [0.32.29] - 2026-05-10

### Added

- **`/status` slash command in chat** — typing `/status` (or selecting it from the slash popover) prints the current Codex subscription quota in the chat feedback bar, one line per `openai-codex` tier with primary/weekly used_percent and reset countdown. Saves a context switch when you want a quick check without leaving the conversation. Feedback duration extended to 10s so you have time to read the multi-line summary.

### Changed

- **Codex Quota card moved from Ops to Analytics** — the card now lives at `/console/analytics`, slotted between the summary grid and the token-spend chart, where it sits naturally with the other usage/spend metrics. `/console/ops` keeps its focus on operational actions (approvals, automation audit). The 60s polling, SSE refresh on `codex_quota` events, and warn/critical row tints all carry over unchanged.

## [0.32.28] - 2026-05-10

### Added

- **Codex subscription quota visible in Ops + threshold alerts** — `/console/ops` gains a "Codex subscription quota" card listing every `openai-codex` tier with its primary (5h) and weekly windows (`used_percent` + reset countdown). The card polls `/v1/admin/llm/codex/usage` every 60s and refreshes immediately when an SSE `codex_quota` event arrives. Backend gains a `codexThresholdWatcher` that observes every parsed `x-codex-*` snapshot and emits one notification when `used_percent` crosses 90 (`severity=warn`) and another at 95 (`severity=error`); recovery is silent so users aren't spammed when usage drains naturally between requests. Primary/weekly windows and tiers are tracked independently. The notifications flow through the existing dispatcher, so the chat header inbox surfaces them without any additional UI work. Window rows in the Ops card tint amber at warn and red at critical so the state is visible at a glance.

## [0.32.27] - 2026-05-10

### Added

- **Surface OpenAI Codex subscription quota in admin API** — `OpenAICodexClient` now parses the `x-codex-*` rate-limit headers (primary 5h-window and secondary weekly window: `used_percent`, `window_minutes`, `reset_after_seconds`) returned on every `/codex/responses` call and caches the latest snapshot per client. New endpoint `GET /v1/admin/llm/codex/usage` returns one entry per LLM tier with the resolved provider/model and the most recent snapshot, letting operators see remaining Codex quota without leaving the console (OpenAI does not expose a subscription-quota REST API). `TrackedClient` adds a passthrough so router-wrapped clients still satisfy the new `llm.CodexRateLimitSource` interface, and any unknown `x-codex-*` header is preserved verbatim in `raw_headers` so the surface is forward-compatible when OpenAI ships new fields. Authorization mirrors `/v1/admin/usage/today` (admin role required when auth mode is on). Console UI integration is a follow-up.

## [0.32.26] - 2026-05-10

### Changed

- **Tasks panel uses a 3-row task card** — each task in the chat-side Tasks tab now stacks as title row → status badge + actions row → details row (description + evidence list/form). The previous single-flex-row layout squeezed long titles and descriptions into a narrow column when the tasks panel sat in the right sidebar, producing a noisy column-of-one-character text. The evidence card inside the body row also gets a saner head (title + type badge wrap together) and a dedicated footer that right-aligns the Remove Evidence button so long summaries no longer push the action into the body text.

## [0.32.25] - 2026-05-10

### Changed

- **Chat renders multi-round assistant bubbles in chronological order** — when the model produces "text → tool → more text → another tool → final text" within a single turn, the console now keeps each text segment in its own bubble next to the surrounding tool calls, instead of collapsing every intermediate sentence into a single bubble that floats below all tool cards. `ChatPanel.handleChatEvent` tracks the streaming assistant id by reference and pushes a fresh placeholder whenever a `before_tool_call` arrives after the current bubble has already streamed text/reasoning content; empty placeholders still receive a tool spliced before them, preserving the existing one-shot tool-then-text flow.
- **Stop ending turns mid-task with status reports** — the chat system prompt's Task Management Policy gains an explicit rule: while pending or in_progress tasks remain, do not end the turn with a "이제 다음 단계로 갈게" / "Now I'll move on to X" status message; pair every "I will do X" with the actual tool call that does X, in the same turn. The model may still stop the turn when the next step needs user input it cannot reasonably infer, hits a blocker, or the plan is complete.

## [0.32.24] - 2026-05-10

### Fixed

- **Workspace skill edit survives a frontmatter rename** — `GET/PUT/DELETE /v1/admin/skills/<name>` now resolves the on-disk SKILL.md via the extensions snapshot when the URL name matches a workspace-source skill. Previously the handler assumed `<name>` was also the source directory name; renaming a skill via its frontmatter (e.g. `claude-code-cli` → `claude-code-cli2`) would 404 on subsequent edits because the source directory still kept the original name. The resolved path is confined to `<workspace>/skills/` to prevent traversal via a stale snapshot, and creating a brand-new skill (PUT to a not-yet-known name) still falls back to `<workspace>/skills/<name>/SKILL.md`.

## [0.32.23] - 2026-05-10

### Added

- **Skill Inbox unifies extraction and session-local promotion** — the chat-side Skill Extraction Inbox is renamed to "Skill Inbox" and gains a Session tab that lists skills generated by the `project_skill` builtin under `<active cwd>/.tars/skills/`. Each entry can be promoted to the shared workspace skills root (`${workspaceDir}/skills/`) with a Copy/Move toggle and a Rename/Overwrite/Cancel conflict dialog when a workspace skill of the same name already exists. The Session Config panel's Skills tab also gains an inline `↑ Promote` button next to the Session badge for one-shot copy with auto-rename. Backend ships two new endpoints — `GET /v1/admin/sessions/:id/local-skills` and `POST /v1/admin/sessions/:id/local-skills/promote` — both available to console (browser_session role=user) sessions, and triggers a single `extensionsManager.Reload` after a batch so the Extensions panel refreshes automatically.

## [0.32.22] - 2026-05-10

### Changed

- **Background-by-default for long-running commands** — the `process` tool gains a `wait` action that blocks server-side until the spawned process exits (or `timeout_ms` lapses, default 30 min), eliminating the polling loops that used to burn agent loop iterations. Background process timeouts move to a separate cap (`tools.process.max_timeout_ms`, default 30 min) so watchers like `gh pr checks --watch` can survive 20+ minutes without being killed by the foreground exec cap. Stdout/stderr buffers are bounded to a rolling 64 KB tail to prevent runaway memory on very long-running watchers. The system prompt now includes a "Long-running Commands" section that tells the LLM to prefer `exec background:true` + `process wait` over blocking foreground `exec` for builds, installs, CI watchers, and dev servers.

## [0.32.21] - 2026-05-10

### Changed

- **exec tool live streaming + raised limits** — exec stdout/stderr now stream line-by-line through SSE `tool_output_line` events, so the chat UI no longer freezes for the duration of a long-running command. The exec timeout cap moves from a hard-coded 30s to a configurable `tools.exec.max_timeout_ms` (default 5 minutes), and the chat agent loop's `automation.agent.max_iterations` default rises from 8 to 20 so multi-step workflows like github-flow have headroom. The console renders streamed lines in a collapsible panel under each tool call with stderr highlighted.

## [0.32.20] - 2026-05-09

### Fixed

- **SonarCloud security hotspot triage** — slash command parsing now avoids a full-input regex, console route parsing uses an HTTPS base URL, GitHub Actions references are pinned to full commit SHAs, and command execution hotspots now use fixed system paths or documented safe PATH resolution. Closes #783.

## [0.32.19] - 2026-05-09

### Fixed

- **Private runtime state permissions** — memory and skill extraction inboxes, ops approvals/events, and usage limit files now persist with owner-only file permissions, while auth cookie tests and inline rationale document the intentional local HTTP versus HTTPS/Tailscale `Secure` behavior. Closes #782.

## [0.32.18] - 2026-05-09

### Fixed

- **GitHub Actions install hardening** — CI and release workflows now use lockfile-enforcing npm installs with package lifecycle scripts disabled where builds do not require them, pin the gitleaks installer to a concrete version, and scope workflow permissions to read-only defaults or the release publish job that needs write access. Closes #781.

## [0.32.17] - 2026-05-09

### Fixed

- **Workspace file path handling** — file panel reads, directory listings, directory creation, and directory renames now resolve requests to root-local paths and execute through Go's `os.Root` API instead of reusing validated absolute paths, tightening traversal and symlink escape protection for the SonarCloud blocker finding. Closes #780.

## [0.32.16] - 2026-05-09

### Fixed

- **Deterministic console string sorting** — frontend config, onboarding, session permission, and related option lists now use explicit string comparators instead of argument-free `.sort()` calls, resolving SonarCloud's ambiguous sorting findings and guarding against regressions with a focused test. Closes #784.

## [0.32.15] - 2026-05-09

### Fixed

- **Remote MCP HTTP Accept headers** — streamable HTTP requests now advertise SSE responses only when that transport path can consume them, while plain JSON RPC posts keep an `application/json` Accept header. This removes the duplicated conditional flagged by SonarCloud without broadening legacy MCP request negotiation. Closes #785.

## [0.32.14] - 2026-05-09

### Added

- **SonarCloud evaluation workflow** — added an optional, non-blocking SonarCloud GitHub Actions workflow with documented `SONAR_TOKEN`, `SONAR_PROJECT_KEY`, and `SONAR_ORGANIZATION` setup. Go coverage is wired through `coverage.out`, while frontend LCOV coverage is intentionally deferred until the Svelte console has a stable LCOV-producing test command. Closes #776.

## [0.32.13] - 2026-05-09

### Added

- **CodeQL code scanning** — pull requests, pushes to `main`, and a weekly schedule now run CodeQL for Go, Svelte/TypeScript/JavaScript, and GitHub Actions workflows, publishing alerts through GitHub code scanning without duplicating the existing test and coverage jobs. Closes #774.

## [0.32.12] - 2026-05-09

### Changed

- **PR CI static-analysis hardening** — pull requests now run Svelte console type checks plus a stable frontend CI test slice, while `make lint-diff` adds `errcheck` and `staticcheck` only for newly changed Go lines so existing lint debt does not block unrelated work. Closes #775.

## [0.32.11] - 2026-05-09

### Added

- **Operator ergonomics sprint** — active Chat plans now expose one-click workbench actions for Tasks, Evidence, Agent Runtime, and Git Inspector. Project directories can also run `tars init local --cwd <path>` to scaffold `.tars/settings.json`, `.tars/settings.local.json`, `.tars/skills/`, `.tars/commands/`, and a local `.tars/.gitignore`.

### Changed

- **Project-local override control** — `.tars` layers that set `tools_custom`, `skills_custom`, `commands_custom`, or `mcp_custom` now replace the inherited allowlist for that surface, so local Console toggles can actually narrow shared project settings.
- **Pulse and legacy route cleanup** — `cleanup_stale_tmp` is no longer in the default Pulse autofix allowlist, keeping cleanup-like remediation opt-in, and the legacy `/v1/agent/*` Agent Runtime aliases have been retired in favor of `/v1/agentruntime/*`. Closes #758, #759, #760, #761.

## [0.32.10] - 2026-05-09

### Fixed

- **AI delete candidates for trivial archives** — recently archived empty or greeting-only sessions can now reach AI delete analysis, while recently archived substantive sessions remain protected by the 24-hour grace period. Closes #771.

## [0.32.9] - 2026-05-09

### Fixed

- **AI session cleanup on OpenAI Codex** — session cleanup suggestions now preserve the configured tier reasoning effort instead of forcing `minimal`, fixing `gpt-5.5` requests that reject that value. Closes #769.

## [0.32.8] - 2026-05-09

### Added

- **AI-assisted session cleanup** — Console Chat can ask the light-tier `session_cleanup` LLM role to review eligible regular sessions and propose user-reviewed archive candidates from the active list or delete candidates from the archived list. Pinned, main, worker, recent, and active-plan sessions stay protected, and delete suggestions are limited to already archived sessions. Closes #767.

## [0.32.7] - 2026-05-09

### Added

- **Console session organization** — chat session lists now support non-destructive archive/restore, pinned sessions, an Archived filter, and cleanup suggestions for stale or generic sessions. Archived sessions keep their transcripts and pinned sessions are excluded from cleanup suggestions. Closes #763, #764, #765.

## [0.32.6] - 2026-05-09

### Added

- **Task contract verification runner** — approved task contracts can now run their `verification_commands` from the Console Tasks panel through a bounded admin/session endpoint. Each command result is stored as task evidence with pass/fail status, exit code, and output summary so verification survives reloads, compaction injection, and plan archives. Closes #757.

## [0.32.5] - 2026-05-08

### Changed

- **Console route code-splitting** — heavy console pages and chat subpanels now load through memoized dynamic imports, keeping the first console route bundle small while loading Chat, Config, Agent Runtime, Mermaid/Markdown-heavy panels, and terminal WebGL support only when needed. The production build no longer emits the previous chunk-size warning. Closes #745.

## [0.32.4] - 2026-05-08

### Fixed

- **Pulse notification dedupe** — repeated Pulse chat-attention notifications now coalesce by signal family for 30 minutes, updating occurrence counts and the latest timestamp instead of adding duplicate unread rows. Severity changes and different Pulse families still create visible notifications. Closes #744.

## [0.32.3] - 2026-05-08

### Fixed

- **Pulse decider OpenAI Codex routing** — Pulse classifier calls now request streaming on `openai-codex` tiers immediately, avoiding the recurring non-stream `400: Stream must be set to true` retry path and reducing watchdog tick noise. Closes #743.

## [0.32.2] - 2026-05-07

### Added

- **Chat Zen mode** — collapse the navigation sidebar and global header into a focus-only chat layout. Toggle from the chat header (Zen / Exit Zen button), with `Ctrl/Cmd + .` to enter or exit and `Esc` to exit. State persists across reloads via localStorage and only takes visual effect on the chat route. Improves usability on mobile where chrome consumed most of the viewport. Closes #741.

## [0.32.1] - 2026-05-06

### Fixed

- **Tailscale Serve status detection** — `tars remote status`, Settings, and startup reconciliation now read `tailscale serve status --json` before falling back to `get-config --all`, fixing false idle/off states on Tailscale versions where `get-config --all` can return only a sparse version payload while HTTPS serving is active.

## [0.32.0] - 2026-05-06

### Added

- **Secure Tailscale remote access** — Settings and onboarding now include a Remote Access control for exposing the local console through a TARS-owned `tailscale serve` HTTPS target while keeping TARS bound to loopback. The backend adds Tailscale detection, preflight checks, desired-state config, startup/shutdown reconciliation, and `tars remote status|enable|disable|url` CLI commands.
- **Browser login with admin/user roles** — the console now supports password login, pairing-code login, session cookies, logout, and password management for separate `admin` and `user` roles. Remote/mobile sessions default to the user role and admin surfaces stay hidden or blocked.
- **Endpoint role policy for browser sessions** — API auth now distinguishes bearer tokens from browser sessions, gives bearer auth priority, applies a fail-closed user allowlist, blocks cross-site mutating browser requests, and prevents Tailscale Serve loopback traffic from bypassing auth accidentally.
- **Environment override visibility in Settings** — the config editor now separates YAML file values from effective runtime values and shows when environment variables such as `TARS_API_AUTH_MODE` are overriding the saved file.

### Changed

- **Config schema reads editable file state** — Settings now reloads config schema values from the YAML file without environment overrides, so saving `api_auth_mode: required` no longer appears to revert to `off` when the dev server was launched with `TARS_API_AUTH_MODE=off`.

## [0.31.180] - 2026-05-05

### Added

- **Extensions diagnostics and MCP repair** (#729) — the Extensions console can now run installed extension diagnostics for loaded skills and MCP servers, showing status badges plus per-check detail for missing skill files, required binaries/env vars, MCP manifest state, connection failures, and tool counts. Workspace MCP servers with a local `requirements.txt` or `package.json` can be repaired in place from the page; Python repairs install dependencies into `.python`, patch `tars.mcp.json` with `PYTHONPATH=${MCP_DIR}/.python`, and reload extensions afterward.

### Fixed

- **MCP server status display** — the console now reads the backend's `tool_count`, `connected`, and `error` MCP status fields directly, so disconnected servers surface their actual failure instead of appearing as an enabled-but-empty row.

## [0.31.179] - 2026-05-05

### Added

- **Session-scoped skills and commands** - chat sessions now load full project-only skills from `.tars/skills/<name>/SKILL.md` and standalone explicit slash commands from `.tars/commands/<name>.md`, with `project_skill` creating both forms in the active cwd.
- **Session Config controls for local extensions** - the chat Session Config panel can reload, enable, and disable session-local skills and commands, persisting per-session choices to `.tars/settings.local.json`.

### Changed

- **Slash command routing** - project commands are separate from auto-selected skills and only run when the user explicitly invokes them, so LLM skill selection no longer treats commands as ambient capabilities.

### Fixed

- **Files panel `.tars` visibility** - the workspace file browser and directory picker now show `.tars` while continuing to hide other noisy dot directories.

## [0.31.178] - 2026-05-05

### Added

- **Streaming reasoning content to the console** — providers that expose a separate chain-of-thought channel (kimi `reasoning_content`, Anthropic `thinking_delta`, OpenAI Responses reasoning summaries) now stream that text to the console in real time instead of being silently buffered. Previously, when a model spent time emitting only reasoning tokens, the chat showed just the "LLM 추론중" HUD with no body text — and any reasoning that was buffered ended up dropped on the floor. A new provider-agnostic `OnReasoningDelta` callback in `internal/llm` flows through the agent loop into a new `reasoning_delta` SSE event, which `ChatMessageItem` renders as a collapsible "추론 / Reasoning" panel above the assistant message.

## [0.31.177] - 2026-05-05

### Changed

- **Brand identity — generated graphite icon, wordmark, and console avatar** (#723) — replaced the previous robot mascot-style assets with transparent generated brand imagery tuned for the console's Warm Workshop palette. README now renders the refreshed icon and wordmark from `docs/brand/`; the console sidebar uses the optimized transparent mark, and assistant chat messages now carry a matching TARS avatar asset.

## [0.31.176] - 2026-05-05

### Changed

- **`tars init move --to <path>` auto-restarts the LaunchAgent service** (#719 phase 3) — previously the command moved the workspace, patched the config, and dumped a manual instruction (`tars service stop && tars service install && tars service start`) for the user to run. Forgetting that step left the running daemon pointing at the old (now nonexistent) workspace. The new flow stops the service before the move, performs the rename + config patch, restarts the service so it re-reads the patched workspace_dir, and polls `/v1/healthz` to confirm. The plist's `--api-addr` and `--config` stay untouched (only the workspace inside the config changed), so no reinstall is needed — just stop + start. Use `--no-restart` to keep the legacy "patch and walk away" behavior. On non-darwin or when no LaunchAgent is installed, init move prints a tailored hint instead of attempting launchctl.

## [0.31.175] - 2026-05-05

### Added

- **`tars init reset` subcommand** (#719 phase 2) — re-run onboarding from scratch without losing recoverable data. Stops the LaunchAgent service, backs up `~/.tars/config/config.yaml` to `config.yaml.bak` (single slot, overwritten each reset), regenerates the wizard skeleton, restarts the service (or detached `tars serve` on non-darwin), polls `/v1/healthz`, and reopens the browser. The workspace is preserved by default; `--wipe-workspace` *renames* (not deletes) it to `<workspace>.bak.<timestamp>` so sessions, memory, and installed plugins remain recoverable until the user `rm`s the .bak themselves. Confirmation prompt by default; `--yes` skips it for scripted use.
- **Plist port inheritance** — `tars init reset` reads the existing LaunchAgent plist's `--api-addr` from `ProgramArguments` and reuses it, so resetting doesn't silently drift the chosen port. Priority: `--api-addr` flag → `--port` flag → previous plist → auto-pick.
- **`tars onboard reset` alias** — same dispatch as `tars init reset`, since `onboard` is a hidden alias of `init`.

### Changed

- **`newRootCommand` wires `cmd.SetIn(stdin)`** so subcommands using `cmd.InOrStdin()` (e.g. `tars init reset`'s confirmation prompt) read from the caller-supplied reader. Without this, programmatic invokers (tests, scripted automation) couldn't pipe answers in.

## [0.31.174] - 2026-05-05

### Changed

- **`tars init` migration is now opt-in via `--migrate`** (#719) — auto-migration was a Phase 1 hold-over from the old init that surprised users running `tars init` from a directory containing one of the scanned-for legacy paths (e.g. the TARS source repo's `config/default.yaml`). The default is now a fresh wizard skeleton; if a legacy file is present we print a discovery hint pointing at `--migrate`. The flag also gives a clear error when no legacy is found, and is mutually exclusive with `--force` (which writes a fresh skeleton).
- **Migration no longer scaffolds a duplicate workspace.** The migrated config carries an authoritative `workspace_dir`; touching the default `~/.tars/workspace` would (a) fail when bundled plugins aren't on disk (dev binaries) and (b) leave the running server pointing at the wrong workspace. Init now skips `ensureStarterWorkspaceLayout` on the migration path and reads the migrated workspace_dir to pass into the server starter.
- **`updateMigratedWorkspaceDir` patches nested `runtime.workspace_dir`.** Most legacy configs put workspace_dir under `runtime:`. The patcher used to look only at the top-level key, so nested relative paths went unfixed AND the function added a stray top-level entry pointing at the default — both keys then flattened to the same field, leaving the resolved value to coin-flip iteration order.
- **`make build` auto-builds the embedded console assets when missing.** A fresh clone followed by `make build` produced a binary that served the "TARS Console build required" placeholder because `make build` was Go-only and the `internal/tarsserver/consoleassets/dist/` directory was empty. The new `ensure-console-assets` Make target runs `console-build` only when `dist/index.html` is missing — once present, subsequent builds stay Go-only and fast.
- **`ensureStarterWorkspaceLayout` treats a missing bundled-plugins dir as soft.** Bundled plugins seed the workspace's `plugins/` dir but the system boots without them. Returning an error from this code path made `tars init` unusable on dev binaries built outside a release tree. Production builds (where the bundled plugins live next to the binary via `assetpath` resolution) are unaffected.
- **`tars init` rejects unknown positional args.** Adding `cobra.NoArgs` so `tars init reset` (subcommand lands in phase 2) errors loudly with `unknown command "reset" for "tars init"` instead of silently re-running the orchestrator.

### Fixed

- **`tars` (no args) probes `/v1/healthz` before opening the browser.** Previously the no-args invocation fired the OS browser at `127.0.0.1:43180/console` regardless of whether a server was running, leaving fresh installs with a confusing connection-refused page. The probe has a 1.5 s timeout; if the server is unreachable we print the onboarding hint (covering `tars init`, `tars service start`, `tars serve`, and `TARS_SERVER_URL` for non-default ports) and exit with an error instead of opening the browser.

## [0.31.173] - 2026-05-05

### Fixed

- **`tars init` migration short-circuited the orchestrator** (#719) — when `tars init` found a legacy config at `workspace/config/tars.config.yaml` (or other legacy locations), it migrated the file and returned early, skipping the new server-start + browser-open flow. Users upgrading from the pre-0.31.172 layout would see "migrated legacy config" and an inert prompt — `tars` then opened the browser at `127.0.0.1:43180/console` against a server that wasn't running. Migration now flows through the orchestrator: it preserves the migrated payload (no overwrite — would silently destroy LLM creds), still picks a port, ensures the workspace, starts the server, polls `/v1/healthz`, and opens the wizard. The migration banner is followed by `starting server with migrated config`. `--force` keeps overriding everything with a fresh wizard skeleton (now also skips the migration probe, so it cannot accidentally pick up a legacy file).

## [0.31.172] - 2026-05-05

### Added

- **`tars init` is now an onboarding orchestrator** (#719 phase 1) — first install used to leave users staring at a `connection refused` page (`tars` opens the browser to a hard-coded `127.0.0.1:43180/console`, but no server is running yet). The new `tars init`: (1) auto-picks a free port from `43180..43199` (or `--port`/`--api-addr` if provided), (2) writes a wizard-driven skeleton `~/.tars/config/config.yaml` (workspace_dir + auth fields, no LLM section so `NeedsSetup=true` triggers the wizard), (3) starts the server (LaunchAgent on macOS by default; detached `tars serve` on other OSes or with `--no-server`), (4) polls `/v1/healthz` until ready, and (5) opens the setup wizard in the default browser. New flags: `--port`, `--api-addr`, `--no-server`, `--no-browser`, `--force`. Re-onboarding via `tars init reset` lands in phase 2. On non-default ports the output prints the `TARS_SERVER_URL=http://...` export the user needs for other CLI commands.
- **`tars onboard` alias** — same flag set and runner as `tars init`; exists for discoverability so users typing the more obvious verb land in the same orchestrator.
- **`tars service install --api-addr` / `--allow-needs-setup`** — `--api-addr` bakes `serve --api-addr <addr>` into the LaunchAgent's `ProgramArguments`, so a non-default port survives launchd restarts. `--allow-needs-setup` lets `service install` complete while the LLM section is still empty (the legitimate state during onboarding before the user finishes the wizard); the doctor gate still blocks any non-LLM failure (workspace missing, broken YAML, etc.) so we don't paper over real problems.

## [0.31.171] - 2026-05-05

### Changed

- **LLM provider config — drop redundant `oauth_provider` and provider-level `service_tier`** — both fields were noise in the provider editor: `oauth_provider` is fully derivable from `kind` (the kind→token-source map already lives in `internal/llmdefaults/`), and `service_tier` belongs on the per-tier binding, not the credential entry. The console's provider editor (Quick Start + Fields) and the onboarding wizard payload now omit both, the `LLMProviderSettings` struct loses both fields, and `ResolveLLMTier` derives `OAuthProvider` from kind only when `auth_mode=oauth`. The API Key input now hides automatically when `auth_mode != "api-key"`, removing the dead field that previously rendered for `oauth` and `cli` providers. Old `oauth_provider:` keys in YAML are silently dropped on load.

## [0.31.170] - 2026-05-05

### Changed

- **Brand assets — shrink icon payload, drop unused console logo** — the original `tars-icon.png` shipped at 1254×1254 / 1.4 MB, which is far larger than any consumer (README renders at 160 px, sidebar mark at 28 px, favicon at 16/32 px). Resized the brand source `docs/brand/tars-icon.png` to 512×512 with palette quantization (1.4 MB → 37 KB, −97 %) and the console-bundled `frontend/console/public/tars-icon.png` to 256×256 (1.4 MB → 12 KB, −99 %), eliminating the per-page-load weight from the console. Also removed `frontend/console/public/tars-logo.png` (was bundled "for future console use" but unreferenced); `docs/brand/tars-logo.png` remains as the single wordmark source for the README.

## [0.31.169] - 2026-05-05

### Changed

- **Brand identity — TARS robot mascot icon + wordmark logo** — replaced the placeholder purple `favicon.svg` with the new `tars-icon.png` mascot (terminal-faced robot with `>_` prompt and smile) and added a cropped `tars-logo.png` wordmark (mark + "TARS" + "local AI agent runtime" tagline). Console favicon now points at `/console/tars-icon.png` and the sidebar's "T" placeholder mark in `Nav.svelte` is now the icon image rendered at 28×28 with a rounded mask. README now leads with a centered icon avatar and wordmark logo, sourced from `docs/brand/`. Console dev-proxy favicon test updated to match the new path. Old `frontend/console/public/favicon.svg` removed.

## [0.31.168] - 2026-05-05

### Documentation

- **`.tars/` session-scoped overrides — full reference in CLAUDE.md + README** — Phase 7 closes Epic [#703](https://github.com/devlikebear/tars/issues/703). Adds a single authoritative section to `CLAUDE.md` covering the active-cwd model, the merge order (`sessions.json ← .tars/settings.json ← .tars/settings.local.json`), the allow-list / block-list, where overrides take effect (chat prompt, tool gating, skill discovery, Session Config badges), and the known follow-up gaps (no UI override layer yet, badges only on Tools/Skills tabs, no `tars init local` scaffolding command). README's Extensibility section gains a one-paragraph pointer so newcomers know the feature exists. Closes [#710](https://github.com/devlikebear/tars/issues/710).

## [0.31.167] - 2026-05-05

### Added

- **Console — source badges in the Session Config panel** — Phase 6 of the `.tars/` epic. Each tool and skill in the Session Config Tools / Skills tabs now shows a tiny `shared` (amber) or `local` (grey) badge when its effective value was contributed by `<active_cwd>/.tars/settings.json` or `.tars/settings.local.json` respectively; rows whose value comes from the session base render no badge so the panel stays quiet for plain sessions. Hovering a badge reveals the full file path via `title`. The panel reads the new effective-config sources map from Phase 3's `GET /v1/admin/sessions/{id}/effective-config`. **Note**: the existing toggle behaviour still writes to the session base (`tool_config`); a true 4th-layer "user override" that takes priority over `.tars/settings.local.json` (so that flipping a shared-injected tool actually disables it) is intentionally deferred to a follow-up issue — this PR is the visualization half. Closes [#709](https://github.com/devlikebear/tars/issues/709) (visualization scope); refs Epic [#703](https://github.com/devlikebear/tars/issues/703).

## [0.31.166] - 2026-05-05

### Added

- **Console — active-cwd HUD chip + `/cwd` slash command** — Phase 5 of the `.tars/` epic. The chat session header now shows an amber `cwd ~/path` chip next to the health badge that, on click, opens a dropdown listing every eligible directory (artifact dir + registered work_dirs) for the session and lets the user transition with one click. A new `/cwd` builtin slash command echoes the current cwd plus the eligible list (`/cwd` or `/cwd list`) or transitions to a path (`/cwd <path>`), surfacing backend errors via the existing feedback toast. Adds typed API helpers `getSessionCwd` / `setSessionCwd`, a new `SessionCwd` type, and DESIGN.md notes for the chip + dropdown styling. Closes [#708](https://github.com/devlikebear/tars/issues/708); refs Epic [#703](https://github.com/devlikebear/tars/issues/703).

## [0.31.165] - 2026-05-05

### Added

- **Session-cwd skill + command discovery** — Phase 4 of the `.tars/` epic. Chat sessions now expose any skills under `<active_cwd>/.tars/skills/` (each with the standard `SKILL.md` shape) on top of the bundled / user / workspace tiers, with cwd-local entries winning conflicts via the new `skill.SourceSessionCwd` source. A sibling `<active_cwd>/.tars/commands/<name>.md` directory provides "alias" commands: each file's `target_skill:` frontmatter resolves to an existing skill in the snapshot, and the alias is registered with the file's basename as both `Name` and `Slash` so `/<name>` invokes the underlying skill (description override is honoured; missing or invalid targets surface as snapshot diagnostics rather than failing the chat turn). Both the live chat run and the `/v1/chat/context` preview consult the augmented snapshot, so `tool_config.skills_enabled` filtering already works against cwd-local skills with no extra wiring. Closes [#707](https://github.com/devlikebear/tars/issues/707); refs Epic [#703](https://github.com/devlikebear/tars/issues/703).

## [0.31.164] - 2026-05-05

### Added

- **Effective-config service + API + chat-turn integration** — Phase 3 of the `.tars/` epic. New `sessionoverride.Service` resolves a session's `EffectiveConfig` (sessions.json base ← `.tars/settings.json` ← `.tars/settings.local.json`) with a per-session cache keyed by (active cwd, settings file mtimes); cache entries reload automatically when files change and are dropped explicitly when the cwd transitions. Exposes `GET /v1/admin/sessions/{id}/effective-config` returning `{effective, sources, diagnostics}` and emits a `session` SSE notification when a resolution actually changes (so the frontend HUD and Phase 4's skill registry can refresh). The chat turn now consults the service via the new `effectiveSessionView` helper before assembling tool gating and the system prompt — both `handler_chat_context.go` (preview) and `handler_chat.go` (live runs) honour `prompt_override` and `tool_config` from the settings files in addition to the raw session fields, while gracefully falling back when the service is nil. Closes [#706](https://github.com/devlikebear/tars/issues/706); refs Epic [#703](https://github.com/devlikebear/tars/issues/703).

## [0.31.163] - 2026-05-05

### Added

- **Session-cwd override loader + merger (`internal/sessionoverride`)** — new package that reads `<cwd>/.tars/settings.json` (team-shared) and `<cwd>/.tars/settings.local.json` (per-user) into a typed `Override`, then deep-merges them with the session's base `tool_config` / `prompt_override` to produce an `EffectiveConfig`. The schema is allow-list driven (`tool_config`, `prompt_override`, `mcp_servers_extra`, `model_tier_override`); a parallel block-list (`llm_providers`, `api_key`, `auth*`, `hooks`, `server_command`) generates `error`-severity diagnostics so credentials and hook registration can never be smuggled into a checked-in file. The merger tracks per-path provenance in a `sources` map (base/shared/local) — Phase 6's UI badges depend on that; collection fields like `tools_enabled` are union-deduped while scalars are last-write-wins. Closes [#705](https://github.com/devlikebear/tars/issues/705); refs Epic [#703](https://github.com/devlikebear/tars/issues/703).

## [0.31.162] - 2026-05-05

### Added

- **Session active-cwd transition API** — chat sessions can now switch their active working directory among the candidate set (artifact dir + registered `work_dirs`) via `GET /v1/admin/sessions/{id}/cwd` and `PUT /v1/admin/sessions/{id}/cwd`. The server returns the current dir alongside the eligible list, validates membership before persisting, and emits a `session` SSE notification on success so subscribers can refresh derived state. New `session.Store` helpers (`EligibleCwds`, `GetCurrentDir`) plus exported sentinels (`ErrSessionNotFound`, `ErrCwdNotEligible`) make the contract explicit. Foundation for the broader `.tars/` session-scoped overrides epic ([#703](https://github.com/devlikebear/tars/issues/703)); this PR closes [#704](https://github.com/devlikebear/tars/issues/704).

## [0.31.161] - 2026-05-04

### Fixed

- **Console — every page now fills the viewport instead of leaving a dark gap below the content** — `.chat-page` was sized with `height: calc(100vh - var(--header-height))` while sitting inside `.shell-content`, which has 24px of vertical padding (16px on mobile). The page overflowed the viewport by ~48px, pushing the composer below the fold and leaving the bottom of the chat looking cut off. Reworked the shell ancestry — `.shell { height: 100vh }`, `.shell-main { min-height: 0 }`, `.shell-content { display: flex; flex-direction: column; min-height: 0 }` — so flex children get a definite-height context, then converted each top-level page (Chat, Session Lineage, Memory, System Prompt, Extensions, Channels, Cron, Settings) to `flex: 1; min-height: 0` with internal `overflow-y: auto` where appropriate. Long-content pages (Logs) still scroll naturally via the body. The chat layout now fits without overflow on desktop (1280×1300) and mobile (600×900).

### Added

- **Console i18n — Korean coverage for 7 previously English-only pages** — Session Lineage (분기), Plans (계획), System Prompt (시스템 프롬프트), Extensions (확장), Agent Runtime (에이전트 런타임 — main runs view), Channels (채널), and Settings (설정 — main shell). Added matching namespaces to `i18n/types.ts`, `en.ts`, and `ko.ts` and routed every visible UI string through the `t` store so locale toggles take effect immediately. Field-level labels in `lib/quickStartFields.ts` and the deeper Agent Runtime subagent builder remain English and are tracked for a follow-up; the configWizardCard `kicker` now reads `설정 마법사` in Korean instead of leaking the English label.

## [0.31.160] - 2026-05-04

### Added

- **Console Git Inspector — Fetch button + remote-branch checkout** — the Branches tab previously only let you switch between local branches; remote rows just showed a "remote" badge with no action. Added a new `fetch` mutation (`git fetch --all --prune`, non-destructive) wired to a Fetch button at the top of the Branches tab, and replaced the inert "remote" badge with a Checkout button that strips the matching remote prefix (`origin/feat/foo` → `feat/foo`) and reuses `switch_branch`. Git's DWIM creates a local tracking branch on first checkout, so subsequent switches go to the local copy. Branch rows now show the short name as the title with the full ref in a smaller line below, so long remote-branch names stay readable in narrow docks.

## [0.31.159] - 2026-05-04

### Fixed

- **Console Git Inspector — diff fills the available panel height** — the diff table was capped at `max-height: 480px`, so on tall viewports it stopped halfway down the panel and the rest of the Files tab sat empty. The diff section now becomes a flex child that fills the remaining tab body (`.diff-section { flex: 1 1 auto }`) and the diff table itself takes the remaining vertical space inside it (`flex: 1 1 auto; min-height: 0`) with internal scroll. A `min-height: 240px` keeps the diff legible when the panel is short.

## [0.31.158] - 2026-05-04

### Fixed

- **Console Git Inspector — Files tab no longer pushes the diff to the bottom of the panel** — when the working tree was clean and a commit-mode diff was loaded, the "Working tree clean." placeholder stayed at the top of the Files tab while the diff section sat at the very bottom with a large empty band between them. `.tab-body` was a CSS Grid whose default `align-content: normal` (= `stretch`) spread the two auto-sized rows apart instead of packing them. Switched `.tab-body` to `display: flex; flex-direction: column` so children stack at the top and grow only as needed; `.files-body` was a no-op once the column override went away, so it was removed.

## [0.31.157] - 2026-05-04

### Changed

- **Console Git Inspector — readable diff renderer + scrollable panel** — the diff view used to dump the raw unified patch into a single `<pre>` (Unified) or two collapsed-context panes (Split), so add/remove rows looked identical and you couldn't tell which line changed. Diff is now parsed line-by-line and rendered as a structured grid: per-row line numbers (old + new in Unified, side-by-side in Split), a sign column, and color-coded backgrounds (green for adds, red for dels, accent for hunk headers). Hunk headers come from `@@ -a,b +c,d @@` so the line numbers stay accurate even for diffs that skip the file header. Split mode pairs consecutive `-`/`+` runs so changed lines line up across columns. Below 600px the Split pane collapses to a single column so it stays readable in narrow docks.
- **Console Git Inspector — fix overflow when expanding tall commit rows** — clicking a commit at the bottom of the Log used to expand the file list past the panel's bounds with no way to scroll, so the action row got cut off. The panel is now a flex column where the header / banners / tab strip are pinned and only the active tab body scrolls. Diff blocks gain their own `max-height: 480px` so the tab body never grows unbounded inside the dock.

## [0.31.156] - 2026-05-04

### Fixed

- **Console Git Inspector — commit-file click now renders diff on a clean working tree** — clicking a file inside an expanded Log commit row would silently land on an empty Files tab whenever the working tree was clean. The diff section was nested under the `files.length === 0` else-branch (introduced in P2 of #692), so the "Working tree clean." placeholder swallowed the entire diff view including any pending commit-mode diff. The diff section is now rendered as a sibling of the file list and shows whenever a diff is loaded, currently loading, or a commit-mode source is selected — independent of whether the worktree has changes.

## [0.31.155] - 2026-05-04

### Added

- **Console Git Inspector — checkout commit + worktree add/remove via approvals (P3 of #692, closes #692)** — three new mutation actions plug into the existing approval workflow: `checkout_commit` (`git checkout --detach <hash>`, optionally `-b <new>`), `worktree_add` (`git worktree add [-b <new>] <path> [<branch>]`), and `worktree_remove` (`git worktree remove <path>`). Detached checkouts and worktree removes are flagged `destructive` so the approval UI can warn before applying. The Log tab gains a per-commit input + Checkout button (red "detached" variant flips to a safe "Checkout as branch" once a name is entered). The Worktrees tab gains an Add form (path + optional existing/new branch) and per-row Remove buttons (current and bare worktrees omit the action). Backend additions (`internal/git/client.go`, `internal/ops/git_mutation.go`): `MutationCheckoutCommit / MutationWorktreeAdd / MutationWorktreeRemove`, `MutationOptions.Hash / WorktreePath / NewBranch`, plus matching `GitMutationPlan` fields and `Command` strings. Audit details now include `hash / worktree_path / new_branch` so the trail captures what was actually queued.

## [0.31.154] - 2026-05-04

### Added

- **Console Git Inspector — commit details, file diffs, worktree listing (P2 of #692)** — Log entries are now expandable: clicking a commit row fetches the new `GET /v1/git/commit?hash=` endpoint and shows the commit body plus its changed files with `+adds / −dels` per file. Clicking a file in that list opens the diff for that path at that commit (the existing Diff endpoint now accepts a `hash` query param and shells out to `git show --format=`). A new `Worktrees` tab calls `GET /v1/git/worktrees` (parsing `git worktree list --porcelain -z`) and shows each worktree's path, branch, HEAD, and any `detached / locked / prunable / bare` flags, with the current worktree highlighted. Backend additions (`internal/git/client.go`): `Client.CommitDetail`, `Client.Worktrees`, `DiffOptions.Hash`, plus parsers for `diff-tree --name-status -z` + `--numstat -z` (renames preserved, binary detected, initial commits handled via `--root`). All read-only — no new mutations; checkout / worktree add / remove still land in P3.

## [0.31.153] - 2026-05-04

### Changed

- **Console Git Inspector — layout & UX refactor (P1 of #692)** — the chat Git panel was a single long-scrolling column where every section (status / files / diff / branches / log) rendered at once, and the 5-column repo-summary grid forced the ROOT path to wrap one character per line in a narrow dock. The panel is now reorganized into a tab strip — `Status` / `Files` / `Branches` / `Log` — with a compact header that inlines branch · HEAD · Δ/S/U counts, dropping the bulky summary cards. The Files tab keeps the file list + diff together (most common pairing) and adds a Unified / Split toggle for the diff view (Unified by default; Split collapses to a single column under 600px). Status meta uses `repeat(auto-fit, minmax(140px, 1fr))` so cards reflow instead of stretching. No backend or API changes — read-only commit details, worktree listing, checkout, and worktree mutations land in P2/P3.

## [0.31.152] - 2026-05-04

### Fixed

- **Console terminal — survives dock zone moves without restarting** (#667) — dragging the integrated terminal between dock zones (left / right / bottom / fullscreen) used to unmount and remount `TerminalTabs`, which closed the WebSocket and tore down the running shell along with its scrollback. The terminal panel is now rendered from a single, stable parent at the chat-layout root with a `data-zone` attribute; CSS picks the matching `grid-area` (or fullscreen overlay positioning) per zone, so xterm + the PTY survive zone changes intact. The regular dock-pane wrapper is suppressed for whichever zone currently hosts the terminal to avoid duplicate panes; resizers stay on so the active pane can still be resized.

## [0.31.151] - 2026-05-04

### Added

- **Console terminal — font picker + larger default size** (#686) — added an `Aa` toolbar button next to `Find` that opens an inline settings panel with a font-family dropdown (JetBrains Mono / SF Mono / Consolas / Cascadia Code / Fira Code / system default), a font-size range slider (8–24), and a Reset button. Each preset's CSS string carries a system-monospace fallback so unavailable fonts degrade gracefully without breaking xterm's monospace assumption. Selections persist via `tars.terminal.fontFamily` / `tars.terminal.fontSize` in localStorage; existing Cmd/Ctrl +/-/0 zoom shortcuts continue to work and stay in sync with the slider.

### Changed

- **Console terminal — bumped default font size from 12px → 14px** (#686) for readability on hi-DPI panels.

### Fixed

- **Console terminal — refit after web fonts settle** (#686) — JetBrains Mono is loaded as a webfont, so xterm's first glyph-metric measurement could land on the fallback and produce uneven character spacing until the next resize. After mount we now await `document.fonts.ready` and re-fit + refresh, bringing xterm's metrics back in sync once the real font is available.

## [0.31.150] - 2026-05-04

### Fixed

- **Console terminal — context menu clamps to the frame edge** (#669) — right-clicking near the bottom or right edge of the integrated terminal used to render a menu that extended past the visible area, hiding "Clear" / "Save buffer". After the menu mounts we now measure its size and clamp `menuX`/`menuY` so it stays inside the terminal-frame wrap with a small margin.

## [0.31.149] - 2026-05-04

### Fixed

- **Console terminal — Esc closes the right-click context menu** (#668) — when the integrated terminal had focus, pressing Esc with the context menu open used to forward an ESC byte to the shell and leave the menu open. The custom xterm key handler now swallows Escape and dismisses the menu before xterm sees it. Clicking outside the menu (the existing overlay) still works as a secondary dismissal path.

## [0.31.148] - 2026-05-04

### Fixed

- **Onboarding wizard — Gemini base_url 404** (#671) — `defaultBaseURLForKind` in the wizard now returns the full backend-canonical paths (`/v1beta/openai` for `gemini`, `/v1beta` for `gemini-native`) instead of the bare host. Previously every chat/model call after a wizard-driven Gemini setup failed with `gemini status 404` and an empty payload. Added a regression test that pins both URLs to the backend `llmdefaults` constants. Existing users whose config was written by the broken wizard need to either re-run the wizard with `?reentry=1` and Apply, or edit the `gemini` provider's `base_url` directly in Providers.

## [0.31.147] - 2026-05-04

### Changed

- **Console chat — full i18n coverage** — extracted all hardcoded English strings in the chat surface (`Chat.svelte`, `ChatPanel.svelte`, `ChatMessageItem.svelte`, `SessionSidebar.svelte`) into the `chat.*` and `sessions.*` translation namespaces. Covers status strip (pulse ticks / last tick / unread), dock panel buttons + tooltips, session header actions (rename / AI title / compact / copy / download / extract skill / delete), plan strip (label / open title / progress aria / active task tooltip), tier recommendation card, mention menu, drop overlay, message roles + usage badge + copy/fork buttons, sidebar search/filter/sort, relative-time formatter, and all error/feedback toasts. Korean translations added for every new key.

## [0.31.146] - 2026-05-04

### Added

- **Run ↔ Task linkage + live "currently working on" indicator** (#679) — when an agentruntime run is spawned with a `task_id`, that ID is now preserved on the resulting `Run` record (`Run.TaskID`) so external clients can correlate run state with the task that triggered it. The session-side `Task.RunID` field complements the link in the other direction. Wired through `SpawnRequest.TaskID` → `Runtime.Spawn()` → `Run.TaskID` and exposed on the `POST /v1/agentruntime/runs` body.
- **Chat plan strip — active task title** — `summarizeTasks()` now surfaces the title of the first in-progress task as `active_task_title`. The chat plan strip renders it next to the goal with a small pulsing dot so the user can see at a glance which task the session is actively working on.

## [0.31.145] - 2026-05-04

### Added

- **Pulse — goal-driven plan auto-continue** (#680) — when a chat session's plan transitions to `completed` and the new `Plan.AutoContinueEnabled` flag is set, pulse can run one chat turn that asks the LLM to either declare the goal achieved (terminating the loop by clearing the flag) or propose a follow-up plan. The cap on iterations is enforced via the automation audit log over a 24-hour rolling window so it survives plan replacement when the LLM proposes a new plan to keep working.
  - New signal kind `auto_continue_goal` in `internal/pulse/signal_auto_continue_goal.go` and matching autofix `auto_continue_goal_plan` in `internal/pulse/autofix/`.
  - **Hard safety bounds**: per-plan `AutoContinueMaxIterations` (default 5, hard cap 10), `AutoContinueIterationWindow` (24 h), session-level escalation when the cap trips → `AutoContinueEnabled` is automatically cleared so detection cannot re-fire on every pulse tick.
  - **Termination semantics**: the LLM is instructed to flip `auto_continue_enabled` to `false` when the goal is reached. The controller re-reads the plan after each turn and classifies the outcome as `goal_completed`, `next_plan`, or `no_change` (no clean decision — re-attempt next tick up to the cap, then escalate).
  - **Opt-in**: gated by both `SessionAutomationConsent.AutoResumeEnabled` (existing per-session consent) and the global `pulse.allowed_autofixes` allow-list. Not in the default allow-list.
  - **Out of scope (deferred)**: `TaskContract.VerificationCommands` execution (currently still LLM-facing metadata only), per-goal token budget guard, frontend toggle UI, plan-repetition detector. Tracked in #680 follow-ups.

## [0.31.144] - 2026-05-04

### Added

- **Pulse — auto-resume halted chats** (#678) — pulse now detects chat sessions whose last activity is a halted turn (a tool error with no follow-up assistant message, or a user message the LLM never finished responding to) and can retry them via a new `auto_resume_failed_chat` autofix. This complements the existing `auto_continue_chat` (which only handles "stalled chat awaiting a user answer").
  - New signal kind `failed_chat` with two failure shapes: `tool_error` and `no_response`. Detection lives in `internal/pulse/signal_failed_chat.go` and reads from the same `ChatSessionSource` the stalled-chat detector uses.
  - **Side-effect-aware**: candidates whose failing tool matches `tool.IsHighRiskToolName` (exec, process, write_*, edit_*, apply_patch, workspace) are marked `block_reason="high_risk_failure"` and surfaced for human attention only — pulse never auto-retries a turn whose last action could already have mutated state.
  - Reuses the existing `sessionAutoResumeController` for transcript injection + agent loop; the retry prompt is failure-kind specific and asks the model to inspect the error before re-running.
  - Audit trail uses a separate action name (`auto_resume_failed_chat`) so the per-session retry counter is independent of the question-resume counter. Same 30-minute escalation window and 3-retry cap as the existing autofix.
  - Opt-in via existing `SessionAutomationConsent.AutoResumeEnabled` and the global `pulse.allowed_autofixes` allow-list — not in the default allow-list.
- **`tool.IsHighRiskToolName`** — promoted from a private helper in `tarsserver` so pulse can share the same risk classification used at chat policy enforcement time.

## [0.31.143] - 2026-05-04

### Fixed

- **Console chat — re-entry & background sync** (#677) — three frontend gaps that left the chat view stale after navigating away or backgrounding the tab:
  - `ChatPanel` only refreshed on `category === 'cron'` events, so pulse-driven auto-resumes (which emit `category: "pulse"` with the session id) never triggered a transcript reload. The listener now matches by `session_id` regardless of category.
  - `document.visibilitychange` is now wired up: returning to a backgrounded chat tab forces a transcript reload so any messages written while the tab was throttled or the SSE connection paused are visible immediately.
  - `lib/api.ts` `ensureStream()` now reconnects automatically when `EventSource` reaches `CLOSED` (capped exponential backoff 1s → 30s), and `streamEvents()` exposes a new `onReopen` callback that fires after a successful reconnect. `ChatPanel` uses it to backfill the transcript so events lost during the gap are recovered without a page reload.

## [0.31.142] - 2026-05-04

### Fixed

- **Console — right dock minimum width** — bumped the right dock's minimum resizable width from 260px to 320px (matching the default size). At 260px the Tasks/Git panel cards would squash their right-side controls (status badge, "+ Evidence") and leave the title column too narrow for CJK text, causing per-character vertical wrapping. The clamp is enforced via `dockSizeLimits` in `lib/dock/layout.ts` and re-applied to persisted layouts on load via `normalizeDockLayout`. Mobile (<900px) is unaffected — docks already render as fullscreen overlays via the existing media query.

## [0.31.141] - 2026-05-04

### Fixed

- **Onboarding wizard — Channels section** — disabling the Telegram channel now also clears `channels_telegram_polling_enabled`, so users who arrive in the section with polling=true prefilled from disk can actually save the section after turning Telegram off. The polling checkbox only renders while the channel is on, so the previous behavior trapped users behind the "polling requires the Telegram channel to be enabled" validator with no UI affordance to flip polling back. `channelsFromConfigValues` now also normalizes inconsistent on-disk state (channel=false + polling=true) on load.

## [0.31.140] - 2026-05-04

### Added

- **Onboarding wizard expansion** — the setup wizard is now a section-router shell with **Quick** vs **Full** modes. Quick keeps the original LLM-only path (Provider → Tiers → Review → Complete); Full adds three new optional sections between Tiers and Review:
  - **Tools & Permissions** — toggle `web_search` / `web_fetch`, edit the private-host allowlist (newline-separated textarea), and gate the high-risk-user permission with a strong warning.
  - **Integrations** — API keys for external augmentations (web search provider key + memory embedding provider/key/model/base URL/dimensions).
  - **Channels** — Telegram (enable + bot token + polling) and webhook toggle, with client-side guards (polling requires bot token + channel enable).
- **Per-section save** — each optional section patches only its own keys via `buildSectionPayload`, so editing Channels never disturbs Tools. Sections can be skipped without writing.
- **Deep-link reentry** — `/console/onboarding?section=<id>` opens the wizard directly on a given section. Optional-section deep-links fall back to the Provider step when no provider is configured.
- **Completion matrix** — the new `OnboardingComplete.svelte` shows ✓/✗/— per capability (LLM provider, tier bindings, web_search, web_fetch, memory embeddings, telegram, webhook) with jump-back links to each section, and surfaces a "restart required to activate Telegram/Webhook" notice when those workers were saved.
- **Setup status capability flags** — `GET /v1/setup/status` now returns a `capabilities` block (`tools_configured`, `integrations_configured`, `channels_configured`, plus per-capability booleans) so the wizard's matrix avoids refetching the schema. Sensitive values are not exposed (only "set" / "not set" booleans).

## [0.31.139] - 2026-05-04

### Fixed

- **Config wizard / provider editor** — provider rename and delete in the onboarding wizard or the Settings provider editor now actually take effect on disk. `PatchYAML` previously preserved any `llm_providers` alias not in the patch, so `kimi → moonshot` rename left both aliases on disk and provider deletion was a no-op. The merge is now alias-replace (drop aliases missing from the patch) with per-field merge inside each alias (api_key omission still preserved). The wizard now sends the full provider map on save instead of only the edited alias, so non-edited providers flow through the alias-replace cleanly.
- **Config wizard step 1 → step 2** — switching provider kind (e.g. `kimi → anthropic`) now clears tier model entries because the previous IDs (`kimi-k2.6` vs `claude-haiku-4-5`) are kind-specific. `base_url` is also re-seeded to the new kind's canonical default when it matched the previous kind's default; user-customized URLs are preserved.
- **Config wizard alias rename** — renaming a provider alias in step 1 now propagates to tier bindings that referenced the old alias (instead of only filling empty entries), and the review step shows a notice that the old alias will be dropped when saved.
- **Settings provider editor** — switching a provider's kind now re-seeds `auth_mode` (to a kind-valid option), `base_url` (to the new kind's default when the previous URL was the old default), and clears `oauth_provider`. Renaming an alias also rebinds any tier currently pointing at the old alias, so saving doesn't leave orphan tier references.

## [0.31.138] - 2026-05-04

### Changed

- **Console chat** — moved the LLM inference status (phase label, elapsed timer, step progress, KR/EN toggle) from the bottom action row into the empty `assistant` bubble that appears while the model is thinking. The bubble previously rendered only `…`; the form-actions row now keeps just the Stop button. Status disappears as soon as the first response token arrives. New `ChatStreamingStatus.svelte` is reused via a `streamingStatus` prop on `ChatMessageItem`.

## [0.31.137] - 2026-05-03

### Fixed

- **Integrated terminal tabs** — terminal area collapsed to a single row or grew to ~24,000 px (depending on layout phase), making the shell unusable. The dock panel body uses `display: block`, so `flex: 1` on `.terminal-tabs` had no effect and the WebGL canvas was sized against an unbounded container. Added `height: 100%` so the tabs container resolves against the dock body's flex-derived height.
- **Integrated terminal** — moved the WebGL renderer addon load to after the first `fit()` and call `terminal.refresh()` once the renderer is attached, so the canvas adopts the correct dimensions even when a tab mounts via a `display: none → flex` transition. Also refresh on `visible` change so a re-activated tab always repaints from the latest buffer state.

## [0.31.136] - 2026-05-03

### Fixed

- **Integrated terminal tabs** — first tab rendered as a single black row because the inactive `.tab-pane` panes used `position: absolute` and the resulting layout collapsed the active pane's measured height. Replaced the absolute-stacking with `display: none` / `display: flex` toggling; the existing `visible` `$effect` already refits the terminal when a tab is reactivated.
- **Integrated terminal tabs** — the `+` button on the tab strip silently re-activated the existing tab instead of opening a new shell because `addTerminalTab` routed through the same path that de-duplicates by `cwd`+`label`. Split the handler so `+` always appends a fresh tab.

## [0.31.135] - 2026-05-03

### Fixed

- **Integrated terminal** — `effect_update_depth_exceeded` crash when a terminal tab opened inside `TerminalTabs`. The status-emitting `$effect` in `IntegratedTerminal` was tracking the parent's `onStatusChange` callback as a dependency; each `statuses` write recreated the inline arrow, retriggering the effect. Now the callback is invoked through `untrack` so only the data fields drive re-runs.

## [0.31.134] - 2026-05-03

### Added

- **Integrated terminal** — multi-tab dock (#663 Phase 4, closes the epic). The terminal panel now renders a tab strip; each "Open shell" adds a new tab (or activates an existing one for the same `cwd`/label combination). Tabs run independent `xterm` + WebSocket sessions in parallel without losing scrollback. A `+` button on the strip opens another shell in the active tab's directory; closing the last tab closes the panel.

### Changed

- **Integrated terminal** — when embedded in the tab strip, the per-terminal header is compact (label/dot/Close hidden — the tab pill owns those). The Find button stays so search is one click away from any active tab.

## [0.31.133] - 2026-05-03

### Added

- **Integrated terminal** — right-click context menu with Copy / Paste / Clear / Save buffer (`@xterm/addon-serialize`); the saved file is timestamped per-session and preserves ANSI codes (#663 Phase 3).
- **Integrated terminal** — font-size shortcuts (`⌘=` / `⌘-` / `⌘0`, or `Ctrl+=` / `Ctrl+-` / `Ctrl+0`) clamped to 8–24 px, persisted in `localStorage` under `tars.terminal.fontSize`.
- **Integrated terminal** — `⌘K` / `Ctrl+Shift+K` clears the buffer and scrolls to bottom.
- **Integrated terminal** — bell flash animation on the connection-status dot when the shell rings the BEL character.

## [0.31.132] - 2026-05-03

### Added

- **Integrated terminal** — clickable URLs (`@xterm/addon-web-links`), in-terminal search bar with case/regex toggles (`@xterm/addon-search`), WebGL renderer with DOM fallback, and Unicode 11 width handling for CJK/emoji glyphs (#663 Phase 1+2).
- **Integrated terminal** — explicit clipboard shortcuts: `⌘C` / `Ctrl+Shift+C` to copy selection, `⌘V` / `Ctrl+Shift+V` to paste, `⌘F` / `Ctrl+Shift+F` to open search; right-click selects word.
- **Integrated terminal** — clickable status indicator that reconnects the WebSocket when the session is `Disconnected` or `Exited`.

### Changed

- **Integrated terminal** — brighter selection background (`#a45a1f`) and stronger mono font fallback chain so dragged text is visibly highlighted across platforms.

## [0.31.131] - 2026-05-03

### Added

- **Prior Context panel** — debounced auto-refresh as the user types (700ms after typing stops), so the preview always reflects the current draft without a manual click.
- **Prior Context panel** — empty-query fallback that surfaces recent experience entries / `MEMORY.md` lines / daily logs when no draft is present, with a banner explaining the live LLM prompt does not actually carry these.
- **Prior Context panel** — "Below threshold" collapsible section that shows score-filtered candidates (1..99) so users can understand why a query did not recall anything.

### Changed

- **Tasks panel** — introduced a Tasks / Contract / Evidence tab structure. The Contract tab inherits the form/approval flow that previously lived in a separate top-bar panel; the Evidence tab aggregates verification artifacts across all tasks.
- **Top toolbar** — removed the standalone Contract toggle. Contract editing now lives inside the Tasks panel, which already holds the plan it scopes.

## [0.31.130] - 2026-05-03

### Added

- Added localized, phase-aware live chat status feedback in the Chat panel while streaming: status labels, elapsed timer, tool-aware progress steps, and animated activity dots.

### Fixed

- Replaced static "..." streaming status text with explicit phase-aware messaging and consistent status state updates.

## [0.31.129] - 2026-05-03

### Added

- Added provider-aware model loading for LLM tier editing so the model field can be selected from that provider's model list.
- Added quick-start/view-mode parity so the field action buttons (Save/Discard) stay visible in quick start mode when there are pending changes.

### Fixed

- Fixed `/v1/models` provider lookup behavior to support `provider_alias` scoped queries and fall back to a warninged empty response on permission errors, instead of returning a 500.
- Fixed LLM history reconstruction so trailing unmatched tool outputs and stale tool-call metadata are no longer included in outbound chat payloads.

## [0.31.128] - 2026-05-02

### Fixed

- Fixed Kimi-compatible tool-calling request generation for thinking-enabled calls by avoiding `tool_choice=required` in OpenAI-compatible compatibility mode. `required` now maps to a direct tool choice when only one tool is available, and falls back to `auto` when multiple tools are passed.

## [0.31.127] - 2026-05-02

### Added

- Added Channels page (`/console/channels`) for Telegram pairing management: approve pairing codes, view pending/allowed users, and revoke access.
- Added `channels` navigation item to the Operate group in the console sidebar with i18n support (EN/KO).

### Fixed

- Fixed Config sensitive-field editing bug where fields like `telegram_bot_token` could not be edited in Fields or Quick Start views. Sensitive fields now render as password inputs and are always masked in display.

## [0.31.126] - 2026-05-02

### Fixed

- Fixed Kimi tool-calling payload handling for `openai_compat_client` so stale or orphaned `tool` role messages are dropped and only the matched latest tool result for each call is sent.
- Added tighter validation/sanitation for Kimi tool message IDs and tool-call metadata in request conversion.
- Added regression tests covering omitted `service_tier`/`reasoning_effort`, `reasoning_content` propagation, orphan/stale tool filtering, and trimmed tool-call IDs.

## [0.31.125] - 2026-05-02

### Added

- Added `kimi` as a first-class LLM provider kind for OpenAI-compatible usage, including `KIMI_API_KEY`-based environment fallback and default Moonshot base URL wiring.
- Added Kimi provider coverage to provider selection docs and live-provider API metadata so `/v1/providers` reports `kimi` with model-listing support.

## [0.31.124] - 2026-05-02

### Added

- Added a structured Console Settings editor for `llm.providers` so each provider can be edited as a card with alias, kind, auth mode, OAuth provider, base URL, API key, and service tier inputs instead of raw JSON.
- Added per-provider API key reveal toggle and Add/Remove provider controls in the new editor, mirroring the existing LLM tier editor flow.

### Tests

- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`

## [0.31.123] - 2026-05-01

### Added

- Added `POST /v1/agentruntime/subagents/recommendations` to analyze recent completed Agent Runtime runs and suggest reusable workspace `AGENT.md` profile drafts.
- Added recommendation provenance on subagent drafts so approved profiles preserve source run IDs, run metadata, and observed prompt context.
- Added Console Subagents controls for generating profile recommendations from recent runs, reviewing each recommended draft, and saving it through the existing approval flow.

### Documentation

- README and the Agent Runtime tutorial now describe run-derived subagent profile recommendations.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run TestAgentRuntimeSubagentsAPIHandler_RecommendsProfilesFromRecentRuns`

### Closed

- Closes #594.

## [0.31.122] - 2026-05-01

### Added

- Added Agent Runtime run checkpoints for prompt dispatch and failure snapshots so failed runs keep restartable state.
- Added `POST /v1/agentruntime/runs/{run_id}/restart` to start a derived retry from a checkpoint with optional agent, tier, provider/model, and prompt adjustment overrides.
- Added restart provenance fields on runs, including source run, source checkpoint, attempt number, and restart reason.
- Added Console Agent Runtime controls for restarting failed runs from checkpoints and navigating to the derived retry run.

### Documentation

- README and the Agent Runtime tutorial now describe failed-run checkpoint restart workflows.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/agentruntime -run TestRuntimeRestartFromCheckpointSpawnsDerivedRunWithOverrides`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run TestAgentRunsAPIHandler_RestartFromCheckpoint`

### Closed

- Closes #593.

## [0.31.121] - 2026-05-01

### Added

- Added `subagents_run` compare mode for 2-3 read-only subagents inspecting the same prompt independently.
- Added task-level `agent` selection for `subagents_run`, while preserving top-level agent fallback and existing safety validation.
- Added compare-mode results with side-by-side outputs, common findings, conflict candidates, sourced evidence snippets, and direct run links in Console Chat.

### Documentation

- README and the Agent Runtime tutorial now describe compare-mode subagent workflows.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tool -run 'TestSubagentsRunTool_CompareMode|TestSubagentsRunTool_.*ConsensusSchema'`
- `cd frontend/console && node --experimental-strip-types --test tests/subagentProgress.test.ts`

### Closed

- Closes #592.

## [0.31.120] - 2026-05-01

### Added

- Added global and per-session TARS style controls for directness, humor, caution, and autonomy.
- Added `/v1/admin/sessions/{id}/style` for reading effective style defaults and saving normalized session overrides.
- Added chat prompt wiring so style controls affect response tone, verification posture, and follow-through while autonomy remains bounded by explicit session consent and approval policy.
- Added a Console Session Config Style tab with sliders, default-value context, and concise behavior previews.

### Documentation

- README, config examples, and chat/config tutorials now describe session style controls and `runtime.style.*_default` settings.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/session -run 'TestStoreForkFromMessageCopiesTranscriptPrefixAndState|TestStoreSetStyleControl_NormalizesSliderScores'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/config -run 'TestLoad_SessionStyleDefaultFields|TestApplyDefaults_ClampsSessionStyleDefaultFields'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'TestSessionStylePromptLimitsAutonomyByConsent|TestEffectiveSessionStyleUsesSessionOverrides|TestSessionAPI_StyleControlRoundTrip'`
- `cd frontend/console && node --experimental-strip-types --test tests/sessionStyleControl.test.ts`

### Closed

- Closes #591.

## [0.31.119] - 2026-05-01

### Added

- Added first-turn cost/quality tier recommendations for Console Chat so TARS can suggest heavy, standard, or light before the first expensive LLM call.
- Added chat request support for accepted or overridden tier recommendations, explicit tier routing for the selected chat turn, and usage signal records with recommendation, chosen tier, provider/model, outcome, token usage, and estimated cost.
- Added Context HUD visibility for the chosen LLM tier and accepted/overridden recommendation path.

### Documentation

- README and the HTTP/SSE chat tutorial now describe first-turn tier recommendation and traceable selected-tier metadata.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/llm -run TestRecommendTierForTaskClassifiesCommonWork`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'TestResolveChatTierRecommendationFallsBackOnFirstTurn|TestResolveChatTierRecommendationCanDisableFallback|TestResolveChatClientForTierUsesExplicitTier'`
- `cd frontend/console && node --experimental-strip-types --test tests/tierRecommendation.test.ts`

### Closed

- Closes #590.

## [0.31.118] - 2026-05-01

### Added

- Expanded Settings config impact analysis with subsystem-level hints for LLM routing, Auth/API, Pulse, Reflection, Cron, Memory, Agent Runtime, Extensions, Tools, Channels, Usage, Compaction, Assistant, Logging, and Runtime fields.
- Added frontend fallback impact classification so pending config edits still show an affected subsystem even when a future field lacks explicit schema metadata.

### Documentation

- README and config-system tutorial docs now describe Settings impact previews as subsystem-aware before-save guidance.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/config -run 'TestSchemaIncludesImpactHintsForHighSignalFields|TestSchemaImpactHintsCoverCoreSubsystems'`
- `cd frontend/console && node --experimental-strip-types --test tests/configImpactPreview.test.ts`
- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- Browser smoke for `/console/config` pending-change subsystem impact preview and zero console errors

### Closed

- Closes #589.

## [0.31.117] - 2026-05-01

### Added

- Added a Chat session health badge and dockable Health panel with deterministic recommendations for long context, stale plans, broad high-risk permissions, noisy prior memory, and idle sessions.
- Added actionable health recommendations that jump to Compact, Tasks, Config, Prior Context, Skill Inbox, or the chat transcript review path.

### Documentation

- README and console tutorial docs now describe session health recommendations in the Chat workspace surface.

### Tests

- `cd frontend/console && node --experimental-strip-types --test tests/sessionHealth.test.ts`
- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/chat/{session}` Session Health critical badge, Health recommendation panel, Open Tasks action, and zero console errors

### Closed

- Closes #588.

## [0.31.116] - 2026-05-01

### Added

- Added Pulse incident cards that turn recent watchdog signals into actionable summaries with likely cause, evidence, severity, recommended action, and safe navigation/re-check buttons.
- Added deterministic incident-card mapping for cron failures, stuck Agent Runtime runs, disk pressure, Telegram delivery failures, reflection failures, stalled chats, and Pulse tick errors.

### Documentation

- README and realtime tutorial docs now describe Pulse incident cards as the actionable layer on top of raw watchdog signals.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/pulse` incident cards with mocked Pulse status, affected-page navigation, and zero console errors

### Closed

- Closes #587.

## [0.31.115] - 2026-05-01

### Added

- Added a Console permission change preview for per-session tool, group, skill, and MCP policy changes.
- Added deterministic permission impact summaries with low/medium/high risk labels, affected tool groups, high-risk tool detection, and shell/files/git/network capability chips before saving session overrides.

### Documentation

- README and human-in-the-loop docs now describe session policy previews as the review step before applying tool and skill permission changes.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/chat/{session}` Session Config permission previews, high-risk shell capability display, Apply persistence, and zero console errors

### Closed

- Closes #586.

## [0.31.114] - 2026-05-01

### Added

- Added Pulse `stalled_chat` detection for active sessions whose latest assistant turn is waiting on user input.
- Added the `auto_continue_chat` Pulse autofix, gated by session-scoped auto-resume consent, allowed resume modes, high-risk question blocking, and a 3-resumes-per-30-minutes escalation cap.
- Added session automation consent fields for `auto_resume_enabled`, `auto_resume_after_minutes`, and `allowed_resume_modes`.
- Added Console controls for auto-resume delay and allowed continuation modes.

### Documentation

- README and human-in-the-loop docs now describe session-scoped stalled-chat auto-resume and the `auto_continue_chat` allowlist entry.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/session`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/pulse ./internal/pulse/autofix`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'TestSessionAutoResumeController'`
- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/chat/{session}` session automation settings with auto-resume delay and mode controls

### Closed

- Closes #585.

## [0.31.113] - 2026-05-01

### Added

- Added Skill Hub domain pack registry metadata with skill, plugin, and MCP dependencies.
- Added pack install planning that shows package contents and install/update/skip actions before applying.
- Added `tars pack search`, `tars pack info`, and reviewed `tars pack install <name>` with `--yes` for non-interactive approval.
- Added pack install execution that reuses the existing sandbox-validated skill, plugin, and MCP installers for each pack member.

### Documentation

- README and Skill Hub tutorial now document domain packs and `tars pack install`.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./cmd/tars ./internal/skillhub`
- `GOCACHE=/tmp/tars-go-cache go run ./cmd/tars pack info github-maintainer-pack`
- `GOCACHE=/tmp/tars-go-cache go run ./cmd/tars pack install github-maintainer-pack --workspace-dir <tmp> --yes`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`

### Closed

- Closes #583.

## [0.31.112] - 2026-05-01

### Added

- Added Skill Hub quality metadata parsing for skills, plugins, and MCP packages.
- Added Extensions Hub quality score badges and install-time trust signals for last update, tests, required tools, permissions, companion CLI presence, and install count.

### Documentation

- README and Skill Hub tutorial now document registry quality metadata and Extensions Hub trust signals.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/skillhub ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/extensions` Hub quality score and trust signal rendering with a mock API

### Closed

- Closes #582.

## [0.31.111] - 2026-05-01

### Added

- Added sandbox validation for Skill Hub plugin installs and updates before writing to `workspace/plugins/<name>`.
- Added sandbox validation for Skill Hub MCP installs and updates before writing to `workspace/mcp-servers/<name>`.
- Added plugin manifest diagnostics, plugin-declared MCP gating checks, MCP manifest validation, and stdio/remote MCP smoke checks to install sandbox reports.
- Added generic extension sandbox report metadata so the Extensions console can render skill, plugin, and MCP install reports.

### Changed

- Hub install API responses now include sandbox reports for successful plugin and MCP installs, matching skill install behavior.
- Plugin and MCP install failures caused by sandbox validation return structured sandbox reports and leave the real workspace package directories untouched.

### Documentation

- README now documents sandbox-tested Skill Hub installs for skills, plugins, and MCP packages.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./cmd/tars ./internal/skillhub ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/extensions` Hub plugin and MCP installs rendering sandbox reports with a mock API

### Closed

- Closes #581.

## [0.31.110] - 2026-05-01

### Added

- Added Skill Extraction Inbox APIs for extracting reusable skill candidates from chat sessions.
- Added session transcript skill candidate detection with light-tier LLM extraction and deterministic fallback.
- Added reviewable skill extraction candidates with provenance, message range, repeated evidence, approve, and reject states.
- Added approval flow that saves accepted candidates as local `workspace/skills/<name>/` drafts using the existing Skill Creator scaffold.
- Added a dockable Chat Skill Inbox panel plus `/extract-skill` slash command for extracting from the active session.

### Documentation

- README now documents session-based skill extraction and local skill draft approval.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/skill ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/chat/{session}` Skill Inbox extract, candidate review, approval, and local skill draft path rendering with a mock API

### Closed

- Closes #579.

## [0.31.109] - 2026-05-01

### Added

- Added approved Git mutation approvals for stage, unstage, discard, commit, and branch switch actions.
- Added `/v1/git/mutations` to queue Git mutation approval cards instead of mutating the workspace directly.
- Added Git Inspector controls for queueing approved mutations and Approvals page rendering for Git mutation cards.
- Added Git mutation automation audit records for success, failure, and blocked consent states.

### Changed

- Destructive Git discard actions are highlighted and cannot run silently; they require session Git mutation consent plus approval.

### Documentation

- README now documents approved Git mutations from Git Inspector through Approvals and Automation Audit.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/git ./internal/ops ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/chat/{session}` Git Inspector mutation approval and `/console/approvals` Git mutation apply/audit with a mock API

### Closed

- Closes #572.

## [0.31.108] - 2026-05-01

### Added

- Added session-scoped automation consent settings for auto-resume, approved git mutations, and autonomous workspace mutations.
- Added durable automation audit entries with actor, action, reason, session, cwd, result, and timestamp metadata.
- Added `/v1/admin/sessions/{session_id}/automation-consent` and `/v1/ops/automation-audit` APIs.
- Added Console controls for session automation consent and an Automation Audit section on the Approvals page.

### Changed

- Session automation defaults remain conservative: no autonomous workspace mutation is allowed unless explicitly enabled for that session.

### Documentation

- README now documents session automation consent and the Automation Audit console surface.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/session ./internal/ops ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/chat/{session}` session automation consent and `/console/approvals` Automation Audit with a mock API

### Closed

- Closes #584.

## [0.31.107] - 2026-05-01

### Added

- Added fork insight promotion APIs at `/v1/admin/sessions/{session_id}/promotions`.
- Added deterministic post-fork insight extraction for reviewable decision, preference, and procedure candidates.
- Added Lineage page controls for reviewing fork insights and queueing selected items into Memory Inbox.

### Changed

- Fork insight promotion preserves parent transcripts and routes reusable fork findings through the existing Memory Inbox approval flow.

### Documentation

- README now documents fork insight review and Memory Inbox promotion from the Lineage page.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/session ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/sessions/graph` fork insight review, Memory Inbox queueing, and Inbox navigation with a mock API

### Closed

- Closes #570.

## [0.31.106] - 2026-05-01

### Added

- Added a Console session lineage graph at `/console/sessions/graph`.
- Added a session lineage row builder that renders root sessions before forked children with depth and fork metadata.
- Added fork point previews by resolving child `forked_from_message_id` values against the parent transcript history.

### Changed

- Console navigation now includes a dedicated Lineage entry for the session graph.

### Documentation

- README now documents the session lineage graph view alongside message-level session forking.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/sessions/graph` rendering and graph-node chat navigation with a mock API

### Closed

- Closes #569.

## [0.31.105] - 2026-05-01

### Added

- Added session forking from a transcript message, creating a child session with transcript history copied through the selected message.
- Added `/v1/admin/sessions/{session_id}/fork` to create forked sessions with lineage metadata.
- Added a Console chat message action for forking from a persisted transcript message and jumping into the new session.

### Changed

- Forked sessions now copy session state that should carry forward: tasks, tool/skill/MCP config, prompt override, work dirs, current dir, and compaction mode.

### Documentation

- README now documents message-level session forking as part of the Chat workflow.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/session ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/chat/{session}` message fork action with a mock API

### Closed

- Closes #568.

## [0.31.104] - 2026-05-01

### Added

- Added first-class session lineage fields: `parent_session_id`, `root_session_id`, `forked_from_message_id`, `forked_from_index`, and `fork_reason`.
- Added stable transcript message IDs for newly appended or rewritten messages.
- Added deterministic read-time virtual IDs for legacy transcript messages that do not yet have persisted IDs.

### Changed

- Session API responses and Console session types now expose lineage and message ID fields.

### Documentation

- README now documents the session lineage and transcript message ID foundation for future fork and graph workflows.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/session ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`

### Closed

- Closes #567.

## [0.31.103] - 2026-05-01

### Added

- Added a Memory Inbox review queue so reflection-derived memory candidates are stored for approve/reject/merge review before entering durable recall.
- Added `/v1/memory/inbox` and `/v1/memory/inbox/review` APIs with provenance, similarity hints, and conflict hints.
- Added a Console Memory Inbox tab with candidate provenance, similar/conflicting memory hints, and review actions.

### Changed

- Nightly reflection now enqueues memory candidates instead of directly appending auto-derived experiences.

### Documentation

- README now documents review-before-store memory extraction in the Chat + Memory and Console Memory workflows.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/memory ./internal/reflection ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/memory` Memory Inbox rendering and approve flow with a mock API

### Closed

- Closes #578.

## [0.31.102] - 2026-05-01

### Added

- Added skill install sandboxing so Skill Hub skill installs and skill updates materialize into a temporary workspace and run manifest/default smoke checks before touching the real workspace.
- Added `smoke_tests` skill frontmatter support for package-defined smoke commands.
- Hub install responses now include a readable skill sandbox report, and the Extensions console renders the latest sandbox pass/fail checks.

### Fixed

- Failed skill smoke tests no longer replace existing installed skill files or update `skillhub.json`.

### Documentation

- README now documents sandbox-smoke-tested Skill Hub installs in the Extensions workflow.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/skillhub ./internal/skill ./internal/tarsserver`

### Closed

- Closes #580.

## [0.31.101] - 2026-05-01

### Added

- Added a read-only Git Inspector dock panel in Console Chat for coding sessions.
- Added `/v1/git/status`, `/v1/git/diff`, `/v1/git/log`, and `/v1/git/branches` APIs backed by a thin read-only `internal/git` wrapper.
- Git Inspector now detects the active session git workspace, shows branch, HEAD, remotes, staged and unstaged files, and renders selected file diffs with a side-by-side summary plus unified patch.

### Documentation

- README now documents the Chat Git Inspector panel and read-only git workspace inspection.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/git ./internal/tarsserver -run 'TestClientStatusAndDiffAreReadOnly|TestGitAPIStatusAndDiffUseSessionCurrentDir|TestRegisterAPIRoutes_RegistersCoreRoutes'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/git ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/chat/:session` Git Inspector rendering with a mock git workspace payload

### Closed

- Closes #571.

## [0.31.100] - 2026-05-01

### Added

- Added task evidence records so plan steps can keep durable verification proof such as test results, screenshots or images, log excerpts, PR links, release tags, and command output summaries.
- Added `tasks(action="evidence_add")` and `tasks(action="evidence_remove")` so agents and users can attach or remove evidence directly from session tasks.
- Added Console Chat task evidence cards and a read-only Contract evidence summary so verification proof remains visible after session reloads.
- Included task evidence in active task prompt injection and archived plan summaries so future turns can see what was already verified.

### Documentation

- README now documents evidence-backed Chat Tasks and Contract panels for active plan verification.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/session -run TestSessionTasks_EvidencePersistsAndInjects`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tool -run 'TestTasks_EvidenceAddAndRemove|TestTasks_EvidenceAddRejectsInvalidType'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/prompt`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/session ./internal/tool ./internal/prompt ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/chat/:session` Tasks and Contract evidence rendering with a mock session payload

### Closed

- Closes #575.

## [0.31.99] - 2026-05-01

### Added

- Added Agent Runtime git diff timeline snapshots so completed runs can show which workspace files changed, including changes made through shell commands rather than file tools.
- Added per-run diff metadata with session, agent, plan flow/step, repo root, file status, additions, deletions, patch previews, and future Git Inspector targets.
- Added a Console Agent Runtime detail panel that groups captured file changes by run and links back to the owning Agent Runtime run.

### Documentation

- README now documents Agent Runtime diff timeline visibility for coding workflows.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/agentruntime -run 'TestRuntimeCapturesGitDiffTimelineForShellStyleChanges|TestRuntimeSpawnAndWait|TestRuntimeCapturesFileToolCallSummaryAndEvent'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/agentruntime`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/agentruntime/runs/:id` Diff Timeline rendering with a mock run payload

### Closed

- Closes #576.

## [0.31.98] - 2026-05-01

### Added

- Added session task contracts with explicit goal, scope, done criteria, verification commands, expected artifacts, and draft/approved status stored alongside active plan tasks.
- Added a dockable Console Chat Contract panel for reviewing, editing, saving, and approving the active session contract.
- Extended the `tasks` tool with `contract_update` and `contract_approve`, and taught `plan_set` to seed a task contract draft from the initial request.
- Included task contract details in compaction reinjection, archive summaries, and global active plan responses.

### Documentation

- README now documents the Chat Contract panel and task contract workflow.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/prompt ./internal/tool ./internal/session ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/chat` Contract dock edit/approve flow

### Closed

- Closes #574.

## [0.31.97] - 2026-05-01

### Added

- Revamped Console Home into Mission Control with live active plan, cron job, Agent Runtime run, Pulse, Reflection, session, notification, and delivery overview cards.
- Added 30-second Mission Control polling so the home overview refreshes without opening individual console pages.
- Added deep links from Mission Control cards to Plans, Agent Runtime, Cron, Pulse, Reflection, Chat, Approvals, GitHub releases, and pull requests.

### Documentation

- README now describes `/console` as Mission Control for active work, automation, runtime health, and delivery status.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`

### Closed

- Closes #577.

## [0.31.96] - 2026-05-01

### Added

- Added a Console Chat progress card for `subagents_run` so parallel subagent work shows running/completed/failed counts while the tool call is active.
- Added per-subagent run links in completed progress cards so users can jump directly into Agent Runtime run details.
- Added compact `subagents_run` status previews that omit prompt bodies while preserving titles, statuses, run IDs, summaries, and errors for chat rendering.

### Documentation

- README now documents the chat-visible parallel subagent progress card and Agent Runtime run links.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache make test`

### Closed

- Closes #573.

## [0.31.95] - 2026-05-01

### Changed

- Moved the integrated Console terminal out of the Files panel and into the Chat bottom dock.
- Files can now stay visible while the bottom-docked terminal remains open at the selected session workspace path.
- The integrated terminal now shrinks with dock split resize events instead of enforcing a fixed minimum frame height.

### Documentation

- README now documents that the Files shell opens in the bottom dock while the macOS Terminal fallback remains available.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke: `http://127.0.0.1:43216/console/chat` opened Files, selected the active session, clicked Shell, and verified Files stayed visible while Terminal opened in the bottom dock with cwd label and input focus.

### Closed

- Closes #566.

## [0.31.94] - 2026-05-01

### Added

- Added a Console Chat Dock Manager foundation with left, right, bottom, and fullscreen zones for Sessions and Chat tool panels.
- Added drag resize support for side and bottom docks, plus localStorage persistence for panel placement and dock sizes.
- Added focused dock layout tests covering default placement, panel moves, invalid stored layouts, resize clamping, and serialization.

### Documentation

- README now documents Chat dock placement, resizing, fullscreen mode, and persisted layout behavior.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- Browser smoke: `http://127.0.0.1:43215/console/chat` opened Files in the right dock, moved it to the bottom dock, switched it to fullscreen, and verified persistence after reload.

### Closed

- Closes #565.

## [0.31.93] - 2026-05-01

### Documentation

- README now documents that the TARS name is an homage to TARS from *Interstellar*.
- Refreshed the public extension docs around the current skill-first model, plugin-declared MCP server opt-in, hub MCP packages, and removed built-in Go plugin/HTTP-route surfaces.
- Updated Getting Started, Contributing, and tutorials for the current provider pool, auth middleware admin paths, default console/API address, and Skill Hub package flow.

### Tests

- `git diff --check`
- stale docs token scan for removed extension/provider/KB/gateway references
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`

### Closed

- Closes #563.

## [0.31.92] - 2026-05-01

### Fixed

- Fixed the Console Vite dev proxy so `/console` assets and HMR load from the mounted path without redirect loops or WebSocket 404s.
- Fixed the Console favicon reference so browsers request `/console/favicon.svg` instead of the unauthenticated root path.
- Fixed the MCP Server Creator draft action so invalid empty names are blocked client-side before a 400 API request is sent.
- Fixed narrow Console layouts so the sidebar status strip collapses before it can overlap main content.
- Fixed legacy workspace agent `tools_allow: [knowledge]` entries by aliasing them to the current `memory` tool and removing `knowledge` from the minimal default tool list.

### Documentation

- README now documents the Console dev proxy mount and the MCP Server Creator's client-side draft-name validation.

### Tests

- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `cd frontend/console && npm run build`
- `GOCACHE=/tmp/tars-go-cache go test ./...`
- Browser smoke: dev proxy at `http://127.0.0.1:43180/console/` with Vite HMR, `/console/favicon.svg`, MCP Creator disabled Draft state, and 496px/800px sidebar collapse verified.

### Closed

- Closes #557.
- Closes #558.
- Closes #559.
- Closes #560.
- Closes #561.

## [0.31.91] - 2026-05-01

### Added

- Added an always-visible Chat plan progress strip that shows the active plan goal, completed/total task count, and progress bar when a session has a plan.
- Added a shared Console task-progress helper so the Chat strip and Tasks panel use the same completed-over-total calculation.

### Documentation

- README now documents the Chat header plan progress strip.

### Tests

- `cd frontend/console && node --experimental-strip-types --test tests/taskProgress.test.ts tests/i18n.test.ts`
- `GOCACHE=/tmp/tars-go-cache make test`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `cd frontend/console && npm run build`
- `make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`
- `make console-build`

### Closed

- Closes #393.

## [0.31.90] - 2026-05-01

### Added

- Added `GET /v1/admin/tasks?active=true` for recently updated active plans across sessions.
- Added `/console/tasks` as a global Plans page with session plan progress cards and direct chat-session navigation.
- Added Console sidebar navigation for Plans.

### Documentation

- README now documents the global Plans page and active-plan API surface.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/session ./internal/tarsserver -run 'ListSessionsWithPlans|GlobalTasksAPI|RegisterAPIRoutes'`
- `cd frontend/console && node --experimental-strip-types --test tests/plansPage.test.ts tests/navGroups.test.ts tests/i18n.test.ts`
- `GOCACHE=/tmp/tars-go-cache make test`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `cd frontend/console && npm run build`
- `make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`
- `make console-build`

### Closed

- Closes #394.

## [0.31.89] - 2026-05-01

### Added

- Added archived-plan listing from `MEMORY.md` notes prefixed with `[archived plan]`, including multiline note grouping and newest-first sorting.
- Added `GET /v1/admin/plans/archive` and `GET /v1/admin/sessions/:id/plans/archive` for global and session-scoped plan archive reads.
- Added a collapsible Past plans section to the Console Chat Tasks panel with read-only archived plan summaries.

### Changed

- Newly archived plan memory notes now include session ID metadata so session-scoped archive views can avoid mixing unrelated plans.

### Documentation

- README now documents the Tasks panel archive and global archive API surface.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/memory ./internal/tarsserver -run 'ListMemoryNotesByPrefix|PlanArchiveAPI|RegisterAPIRoutes'`
- `cd frontend/console && node --experimental-strip-types --test tests/tasksArchive.test.ts tests/i18n.test.ts`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache make test`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run build`
- `make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`
- `make console-build`

### Closed

- Closes #395.

## [0.31.88] - 2026-05-01

### Added

- Mirrored `subagents_plan` output into the active session plan/tasks so staged subagent work appears in the Console Tasks panel.
- Mirrored `subagents_orchestrate` task lifecycle updates into session tasks, including `in_progress`, `completed`, and `cancelled` states with run summaries or error descriptions.

### Changed

- Chat tool registration now injects the active session store into staged subagent tools without changing existing standalone constructor call sites.

### Documentation

- README now documents that staged subagent tools update the session Tasks panel while they run.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tool`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'ChatPipelineTools|ChatTool|SessionTasks|Tasks|Subagents'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tool ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`
- `make console-build`

### Closed

- Closes #396.

## [0.31.87] - 2026-05-01

### Added

- Added a Console Analytics page at `/console/analytics` with 7d/30d/90d usage period controls, summary cards, pure-SVG daily stacked token bars, model rows, and tool/skill call rows.
- Added `GET /v1/admin/analytics?days=7|30|90` backed by the existing usage tracker JSONL data, including zero-filled daily rows and unique session counts.

### Changed

- The Console navigation now includes Analytics in the Operate group.

### Documentation

- README now documents the global Analytics page alongside the other console pages.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/usage ./internal/tarsserver -run 'Analytics|RegisterAPIRoutes'`
- `cd frontend/console && node --experimental-strip-types --test tests/analyticsPage.test.ts tests/navGroups.test.ts tests/i18n.test.ts`
- `cd frontend/console && npm run check`
- `cd frontend/console && npm test -- tests/analyticsPage.test.ts`
- `cd frontend/console && npm run build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make vet`
- `make security-scan`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make build`
- Browser smoke: opened `/console/analytics`, verified seeded usage totals, model rows, tool/skill rows, and switched 30d/90d periods.

### Closed

- Closes #397.

## [0.31.86] - 2026-05-01

### Added

- Added a Console Logs page at `/console/logs` with file, level, component, and line-count filters, manual refresh, 5-second auto-refresh, and level highlighting.
- Added `GET /v1/admin/logs` for safe logical log-file selection and tailing parsed JSON log lines from the configured runtime log sink.

### Changed

- The Console navigation now includes Logs in the Operate group.

### Documentation

- README now documents the global Logs page alongside the other console pages.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'TestLogsAPI|TestRegisterAPIRoutes'`
- `cd frontend/console && npm test -- tests/logsPage.test.ts`
- `cd frontend/console && npm run check`
- `cd frontend/console && npm run build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make vet`
- `make security-scan`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make build`
- Browser smoke: opened `/console/logs`, filtered to `ERROR` plus `component=runtime`, verified the Agent Runtime error line, and toggled 5-second auto-refresh.

### Closed

- Closes #398.

## [0.31.85] - 2026-05-01

### Fixed

- Serialized Agent Runtime persistence snapshots so older overlapping snapshot writes cannot overwrite newer run/channel state.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test -coverprofile=/tmp/agentruntime.cover ./internal/agentruntime -run 'TestRuntimePersistence_(TrimsRunsAndChannelMessages|ConcurrentSnapshotsKeepLatestChannels)' -count=20`

### Closed

- Closes #549.

## [0.31.84] - 2026-05-01

### Added

- Added a global Console Cron page at `/console/cron` for creating, monitoring, pausing, resuming, manually running, and deleting scheduled jobs.
- Cron jobs now show delivery target, status buckets, next-run context, and expandable run history from the existing cron API.

### Changed

- The Console navigation now includes Cron in the Operate group.
- Frontend cron API types now include delivery, wake, payload, and delete-after-run fields already supported by the backend.

### Documentation

- README now documents the global Cron page alongside the per-session cron panel.

### Tests

- `cd frontend/console && npm test -- tests/cronPage.test.ts`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `cd frontend/console && npm run build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make vet`
- `make security-scan`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make build`
- Browser smoke: created a global cron job on `/console/cron`, expanded run history, paused/resumed it, and deleted it.

### Closed

- Closes #399.

## [0.31.83] - 2026-05-01

### Added

- Chat session search now reuses session-inclusive memory search to surface matching transcript snippets in the session sidebar.
- Session search snippets highlight OR-matched query terms safely after escaping transcript text.

### Changed

- The standalone Sessions component uses the same transcript snippet grouping and highlight behavior as the active chat sidebar.

### Documentation

- README now notes transcript snippet matches in chat session search.

### Tests

- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `cd frontend/console && npm run build`
- Browser smoke: searched `meridian launch` in the chat session sidebar and verified transcript snippets plus highlighted terms.

### Closed

- Closes #400.

## [0.31.82] - 2026-05-01

### Added

- Chat composer slash commands now render through a dedicated `SlashPopover` component with built-in and skill sections.
- Added client-side `/clear`, `/memory search <query>`, and `/skill <name>` handling without sending those commands to the LLM.

### Changed

- Memory search links can prefill the search tab from query parameters, enabling `/memory search ...` routing from chat.

### Documentation

- README now notes the chat composer slash-command popover and first-pass client commands.

### Tests

- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `cd frontend/console && npm run build`

### Closed

- Closes #401.

## [0.31.81] - 2026-05-01

### Added

- Console sidebar now includes a persistent status strip for server, Pulse, Reflection, and active session state.
- Status strip rows refresh every 30 seconds, stop polling when the sidebar is destroyed, and navigate to their related console detail pages.

### Documentation

- README now notes the persistent console sidebar status strip.

### Tests

- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `cd frontend/console && npm run build`

### Closed

- Closes #402.

## [0.31.80] - 2026-05-01

### Added

- Console tool calls now render as collapsible rows with compact invocation previews, pretty-printed args/results, and live elapsed time while running.
- Chat SSE and persisted session transcripts now carry `tool_is_error` metadata so failed tool calls reopen with destructive styling after reload.

### Changed

- Tool-call state colors now use the console design tokens: primary for running, default for done, and error for failed calls.

### Documentation

- README now notes the console's live, collapsible tool-call rows.

### Tests

- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test -count=1 ./internal/tarsserver ./internal/session`

### Closed

- Closes #403.

## [0.31.79] - 2026-05-01

### Added

- Added `usage_daily_token_budget` / `usage.limits.daily_tokens` config metadata for a daily input+output token budget; `0` disables the console chip.
- Added `/v1/admin/usage/today` to summarize today's input, output, total tokens, configured budget, UTC reset boundary, percent used, and indicator level.
- The console header now shows a compact daily token budget chip when a budget is configured, with warning/error states and an error-state jump to today's analytics focus.

### Changed

- Usage limit PATCH requests can now update `daily_tokens` alongside the existing USD limits.

### Documentation

- README and the example config now document the daily token budget indicator setting.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test -count=1 ./internal/usage ./internal/tarsserver ./internal/config`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `cd frontend/console && npm run build`

### Closed

- Closes #404.

## [0.31.78] - 2026-05-01

### Added

- Console i18n now supports English and Korean locale maps with browser-language detection and `tars_console_locale` localStorage persistence.
- The console header includes a compact EN/KO language toggle.

### Changed

- Console navigation, header notifications, Sessions, Memory, and Tasks first-pass static labels now read from the shared translation store.

### Documentation

- README now notes the console EN/KO language toggle and persisted locale key.

### Tests

- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `cd frontend/console && npm run build`

### Closed

- Closes #405.

## [0.31.77] - 2026-05-01

### Changed

- Config YAML path metadata is now materialized from the config input-field registry instead of a large separate key switch.
- LLM provider-kind defaults now live in a shared defaults table used by config normalization and provider construction.

### Documentation

- README now notes that console Settings and YAML patch paths share the config field registry.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/config ./internal/llm ./internal/tarsserver`

### Closed

- Closes #406.

## [0.31.76] - 2026-05-01

### Changed

- Split Chat message rendering, Artifact panel header rendering, and Config pending-change rendering into focused Svelte subcomponents.
- Moved the console-only chat message shape into a dedicated frontend type module.

### Documentation

- Added a frontend API type contract note explaining why `types.ts` remains hand-curated until a smaller shared schema source exists.

### Tests

- `cd frontend/console && npm run check`

### Closed

- Closes #407.

## [0.31.75] - 2026-05-01

### Changed

- CLI console opening, internal CLI API calls, and public `pkg/tarsclient` requests now share the same URL resolver.
- Server URLs with proxy base paths now resolve consistently for both API paths and `/console`.

### Fixed

- Base URL query strings and fragments no longer leak into resolved API or console URLs.

### Documentation

- README now notes that `--server-url` may include a proxy base path.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./pkg/tarsclient ./internal/tarsclient ./cmd/tars`

### Closed

- Closes #408.

## [0.31.74] - 2026-05-01

### Changed

- `tars service start`, `stop`, and `status` now operate from LaunchAgent plist and `launchctl` state without requiring a readable runtime config.
- `tars service install --label/--domain` now records the launchd identity in the LaunchAgent environment so server restart detection uses the installed custom label/domain.
- Server and assistant LaunchAgent plist serialization now share the same internal helper.

### Documentation

- README now notes that macOS service inspection/control remains available even when config repair is needed.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./cmd/tars ./internal/assistant ./internal/tarsserver ./internal/launchagent`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make build`
- Manual macOS smoke: temp LaunchAgent install, then `service status` with intentionally broken config.

### Closed

- Closes #409.

## [0.31.73] - 2026-05-01

### Removed

- Fresh workspace bootstrap no longer creates legacy `memory/wiki`, `memory/wiki/notes`, `index.md`, or `graph.json` KB Wiki artifacts.
- Doctor required workspace checks no longer require legacy KB Wiki paths.
- Removed dead `/v1/memory/kb/*` route registrations from the API mux.

### Fixed

- Existing legacy `memory/wiki` files are preserved when `EnsureWorkspace` runs; they are no longer created or deleted automatically.

### Documentation

- README now clarifies that fresh workspace bootstrap omits legacy KB Wiki scaffolding.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./cmd/tars ./internal/memory ./internal/tarsserver`
- Fresh workspace smoke confirming `memory/wiki` is not created.

### Closed

- Closes #410.

## [0.31.72] - 2026-05-01

### Changed

- Assistant CLI defaults now use the core `~/.tars/workspace` location instead of falling back to `./workspace`.
- Hub command metadata now uses explicit plural nouns so help text renders `skills`, `plugins`, and `MCP servers` correctly.

### Fixed

- Legacy `tars init` config migration now returns an error when `workspace_dir` correction cannot be read, parsed, resolved, marshaled, or written.

### Documentation

- README now notes that assistant helpers share the core workspace default unless overridden.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./cmd/tars ./internal/assistant`
- Manual help checks for `tars skill update --help`, `tars plugin update --help`, and `tars mcp update --help`.

### Closed

- Closes #411.

## [0.31.71] - 2026-05-01

### Changed

- Hub update commands now report updated, skipped, and failed entries separately for skills, plugins, and MCP servers.
- `/v1/hub/update` now returns structured skill/plugin update results while preserving the existing `updated_skills` and `updated_plugins` arrays.

### Fixed

- Skill, plugin, and MCP updates now return final `skillhub.json` save errors instead of dropping them after package files were updated.
- MCP update failures now surface as per-entry diagnostics and aggregate errors instead of being silently skipped.
- macOS assistant popup result messages now build raw preview text first and AppleScript-escape it once, including quotes, backslashes, newlines, and CJK text.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test -count=1 ./internal/skillhub ./cmd/tars ./internal/assistant ./internal/tarsserver`

### Closed

- Closes #412.

## [0.31.70] - 2026-05-01

### Changed

- Clarified arbitrary root file previews by routing selected filesystem roots through `/v1/filesystem/files` while keeping `/v1/workspace/files` as the workspace-artifacts alias.
- Filesystem previews now have traversal, symlink, and explicit-root coverage around the selected root boundary.
- Workspace reset responses now report partial deletion/reinitialization failures and return an error when reset is incomplete.

### Fixed

- Settings reset messaging now uses the server's `removed` count instead of the stale `removed_dirs` field.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'WorkspaceFilesHandler_(Allows|Rejects)|ResetWorkspaceReports|RegisterAPIRoutes'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver`
- `npm run check` in `frontend/console`

### Closed

- Closes #413.

## [0.31.69] - 2026-05-01

### Changed

- Canonicalized Agent Runtime run and agent-list API calls under `/v1/agentruntime/*`.
- `pkg/tarsclient` and internal client tests now use `/v1/agentruntime/agents` and `/v1/agentruntime/runs` by default.
- Kept `/v1/agent/agents` and `/v1/agent/runs` as explicit legacy aliases for compatibility.

### Documentation

- Updated Agent Runtime tutorial and roadmap notes to label `/v1/agent/*` as legacy-only.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver ./internal/tarsclient ./pkg/tarsclient -run 'AgentRunsAPIHandler_AgentsList|RegisterAPIRoutes|RuntimeClientEndpoints|RunShowsPolicy|RunsShowsPolicy|ResolveURL'`

### Closed

- Closes #414.

## [0.31.68] - 2026-05-01

### Changed

- Replaced the ambiguous `tars serve --serve-api=false` opt-out path with explicit `tars serve --config-check`.
- `tars serve --config-check` now validates server config, workspace setup, auth safety, usage tracking, LLM routing, and semantic memory configuration before exiting without binding the HTTP API.
- Development serve targets now start the API directly instead of passing the removed `--serve-api` flag.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./cmd/tars ./internal/tarsserver -run 'ServeSubcommand|TestRun_|ValidateAPIAuthSecurity'`

### Closed

- Closes #417.

## [0.31.67] - 2026-05-01

### Added

- CON-034: Settings now opens on a Quick Start tab that curates the core onboarding fields before the full Fields and YAML views.
- Quick Start cards validate provider credentials, LLM tier coverage, workspace path, auth mode, Pulse, Reflection, log level, and Telegram session scope while keeping Telegram bot token optional.
- Added a Settings LLM connection action that reuses `/v1/models` to check the currently configured default provider.

### Tests

- `npm test -- --test-name-pattern "quick start"` in `frontend/console`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #436.

## [0.31.66] - 2026-05-01

### Added

- CON-035: Settings field rows now show metadata badges for default values, modified values, restart-required changes, live-apply fields, and masked secrets.
- Config schema metadata now includes per-field `default_value` and `requires_restart` information, plus config schema responses include the config file `updated_at` timestamp for modified badges.
- Field badges reuse the existing schema metadata flow so future live-apply fields can opt into the `live` badge without UI rewiring.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/config -run TestSchemaIncludesFieldMetaBadges`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run TestConfigAPI_SchemaReflectsPatchedValues`
- `npm test -- --test-name-pattern "config meta badges"` in `frontend/console`

### Closed

- Closes #437.

## [0.31.65] - 2026-05-01

### Added

- CON-036: Settings pending-change review now includes impact previews for high-signal fields.
- Config schema metadata now carries maintained `impact` hints for fields such as pulse interval, log level, usage limits, semantic memory, reflection cadence, and agent runtime concurrency.
- Pulse interval previews add dynamic latency and tick-volume hints based on the old and new durations.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/config -run TestSchemaIncludesImpactHintsForHighSignalFields`
- `npm test -- --test-name-pattern "impact preview"` in `frontend/console`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #438.

## [0.31.64] - 2026-05-01

### Added

- CON-037: `/console` now resolves to a Home dashboard instead of redirecting into Chat.
- Home surfaces Pulse, Reflection, disk pressure, active main sessions, recent notifications, recommended setup actions, and the latest session plan to continue.
- Chat remains available at the explicit `/console/chat` route, and the sidebar keeps Home on the TARS logo.

### Tests

- `npm test -- --test-name-pattern "Home"` in `frontend/console`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #439.

## [0.31.63] - 2026-05-01

### Added

- CON-038: The console sidebar now groups navigation into Work, Operate, and Setup sections, with Home remaining on the TARS logo.
- Settings now appears under Setup while the existing `/console/config` route remains intact.

### Tests

- `npm test -- --test-name-pattern "Console nav groups"` in `frontend/console`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #440.

## [0.31.62] - 2026-05-01

### Added

- CON-039: Memory's `Try a Search` panel now has `Tool path` and `Prefetch path` modes so explicit `memory_search` results can be compared with automatic Prior Context recall.
- Added `POST /v1/memory/prefetch`, returning the rendered `## Prior Context` section, source-tagged snippets, token usage, and budget percentage.
- Prefetch mode supports an optional session id and renders source badges plus the exact prompt section used by the automatic memory path.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'TestMemoryAPIHandler_PrefetchBuildsPriorContextPreview|TestRegisterAPIRoutes'`
- `npm test -- --test-name-pattern "Prefetch path"` in `frontend/console`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #441.

## [0.31.61] - 2026-05-01

### Added

- CON-040: Chat now includes a `Prior` side panel that previews the exact `## Prior Context` section the next draft message would add to the system prompt.
- Added `POST /v1/chat/prior-context/preview`, returning structured source badges, snippets, relevant token usage, budget percentage, and the rendered prompt section.
- The prompt builder now preserves structured Prior Context item metadata for conversation, experience, project, and daily sources while keeping the injected prompt text in sync.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/prompt -run TestBuildResultFor_ExposesPriorContextPreviewItems`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run TestChatAPIHandler_PriorContextPreviewEndpointReturnsExactSectionAndItems`
- `npm test -- --test-name-pattern "Prior Context"` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/prompt ./internal/tarsserver`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #442.

## [0.31.60] - 2026-05-01

### Added

- CON-041: Recorded the accepted decision to keep the Approvals workflow as TARS' human review queue for risky operational mutations.
- Added `docs/decisions/approvals-workflow.md` with the routing policy for manual cleanup plans, Pulse autofix, and future approval queue item types.
- Updated the ops approval tutorial and roadmap notes so CON-025/CON-026 follow-up work strengthens Approvals instead of removing it.

### Tests

- `npm test -- --test-name-pattern "Approvals workflow RFC"` in `frontend/console`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`

### Closed

- Closes #443.

## [0.31.59] - 2026-05-01

### Added

- CON-044: Extensions now includes an MCP Server Creator wizard for Python FastMCP and Node MCP SDK stdio boilerplate.
- New admin MCP Server Creator endpoints draft `tars.mcp.json` packages, save edited files into `workspace/mcp-servers/<name>/`, and prepare tars-skills draft PR handoff commands.
- The creator can run an isolated stdio validation sandbox that probes `tools/list` and `tools/call`, returning tool names, call output, worker/hidden sandbox metadata, and a tool trail.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'TestMCPServerCreator|TestRegisterAPIRoutes'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver`
- `npm test -- --test-name-pattern "MCP Server Creator"` in `frontend/console`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`

### Closed

- Closes #446.

## [0.31.58] - 2026-05-01

### Added

- CON-043: Skill Creator drafts can now run a sandbox Test before local save or draft PR preparation.
- The new `/v1/admin/skills/test` endpoint writes the edited draft into an isolated `workspace/tmp/skill-tests/` sandbox, executes the generated companion CLI with a timeout, and returns stdout, stderr, exit code, worker/hidden sandbox metadata, and a tool trail.
- The Extensions Skill Creator wizard now includes a Test action and inline pass/fail output so broken CLI stubs or missing runtime dependencies are visible before publishing.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'TestSkillCreatorAPI_Test|TestRegisterAPIRoutes'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver`
- `npm test -- --test-name-pattern "Skill Creator"` in `frontend/console`
- `npm run check` in `frontend/console`

### Closed

- Closes #445.

## [0.31.57] - 2026-05-01

### Added

- CON-042: Extensions now includes a Skill Creator wizard that drafts `SKILL.md` frontmatter/body plus Python, TypeScript, or Shell companion CLI boilerplate from a natural-language use case.
- New admin Skill Creator endpoints generate local drafts, save edited files into `workspace/skills/<name>/`, and expose a safe draft-PR readiness response for the external `tars-skills` publishing flow.
- The wizard supports language/layout selection, recommended tool inference and editing, file preview/editing, local save, and reloads the installed skills list after save.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'TestSkillCreator|TestRegisterAPIRoutes'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver`
- `npm test -- --test-name-pattern "Skill Creator"` in `frontend/console`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make console-build`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #444.

## [0.31.56] - 2026-05-01

### Added

- CON-047: Console Agent Runtime runs now include a Svelte Flow live graph mode with pan/zoom navigation, MiniMap, Controls, and Background.
- The Flow graph projects runs into tier-shaped/status-colored nodes, spawn edges, and consensus variant fan-out nodes with running animations.
- Flow filters support tier, status, and session, with a Replay control placeholder for the run-detail replay surface.

### Tests

- `npm test -- --test-name-pattern "Svelte Flow|buildAgentRuntimeFlowGraph"` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make console-build`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #449.

## [0.31.55] - 2026-05-01

### Added

- CON-046: Console Agent Runtime runs now include dependency-free Tree and Gantt visualization modes alongside the existing list view.
- The Tree mode renders parent/child run structure with depth, tier shape, status color, and direct run-detail navigation.
- The Gantt mode renders run duration bars and consensus variant sub-bars on a shared timeline for quick parallelism scanning.

### Tests

- `npm test -- --test-name-pattern "Agent Runtime.*(tree|Gantt)|buildAgentRuntime"` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make console-build`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #448.

## [0.31.54] - 2026-05-01

### Added

- CON-048: Console Agent Runtime run details now include a Replay scrubber that reconstructs run state from timestamped live events up to the selected cursor time.
- Replay supports Live lock, Play/Pause, 1x/2x/5x playback speed, first/last event timestamps, event progress, status, last event, message, and replayed file path chips.

### Tests

- `npm test -- --test-name-pattern "replay"` in `frontend/console`
- `npm test -- --test-name-pattern "Agent Runtime"` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #450.

## [0.31.53] - 2026-05-01

### Added

- CON-049: Console Agent Runtime run details now include a pure-SVG Cost Flow panel that visualizes parent/run → agent → variant flow by actual cost or token volume.
- Cost Flow includes tier-colored links, exact variant cost/token rows, and a budget summary when `consensus_budget_usd` is available.

### Tests

- `npm test -- --test-name-pattern "cost flow"` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #451.

## [0.31.52] - 2026-05-01

### Added

- CON-050: Agent Runtime now captures file-oriented tool calls (`read_file`, `list_dir`, `write_file`, `edit_file`) as `tool.call` run events and accumulates a per-run file attention summary.
- The Console Agent Runtime run detail now includes a File Attention panel with frequency-ranked files, read/edit counts, intensity cells, and mini sparklines.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/agentruntime -run TestRuntimeCapturesFileToolCallSummaryAndEvent`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run TestNewAgentPromptRunnerWithTools_ForwardsFileToolCallsToRuntimeRecorder`
- `npm test -- --test-name-pattern "file attention"` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/agentruntime ./internal/tarsserver`
- `npm test -- --test-name-pattern "Agent Runtime"` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #452.

## [0.31.51] - 2026-05-01

### Changed

- Q-015 follow-up: `subagents_run` no longer advertises `mode=consensus` or the `consensus` argument object while `agentruntime_consensus_enabled=false`.
- Consensus remains available as an advanced config opt-in; when enabled, the `subagents_run` schema exposes the consensus mode and runtime behavior is unchanged.
- Disabled consensus calls now return an immediate diagnostic before spawning an Agent Runtime run.
- Config schema, example config, README, usage-signal notes, and Console copy now describe consensus as advanced opt-in rather than routine run data.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tool -run 'TestSubagentsRunTool_(HidesConsensusSchemaWhenRuntimeGateDisabled|ExposesConsensusSchemaWhenRuntimeGateEnabled|RejectsConsensusBeforeSpawnWhenRuntimeGateDisabled|SpawnsParallelExplorerChildrenAndReturnsSummaries)'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/config -run 'TestSchema_UsesPreferredHierarchicalPaths|TestLoad_ExampleConfigHierarchicalSchema'`
- `npm test -- --test-name-pattern "Agent Runtime"` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #507.

## [0.31.50] - 2026-05-01

### Changed

- Q-017 follow-up: the per-session tool/skill configuration panel is no longer an always-visible Chat toolbar button after a fresh signal window again showed 0 `session.tool_config.updated` rows.
- The advanced `/config` command still opens session-scoped policy controls for an existing selected session, and backend `SessionToolConfig` filtering plus usage telemetry remain intact for explicit opt-in and diagnostics.

### Tests

- `npm test -- --test-name-pattern "session config"` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'Test(ChatAPIHandler_ContextEndpointReflectsSessionGroupConfigAfterPatchAPI|SessionAPIHandler_ConfigPatchRecordsUsageSignal)'`

### Closed

- Closes #508.

## [0.31.49] - 2026-05-01

### Changed

- Q-012 follow-up: `subagents_plan` and `subagents_orchestrate` are now advanced opt-in tools rather than default chat schema surfaces, while `subagents_run` remains the default path for parallel delegated work.
- README, chat runtime guidance, subagent mention hints, and Console Agent Runtime copy now steer users toward `subagents_run` by default.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'TestResolveInjectedToolSchemas_(HidesSubagentFlowToolsByDefault|AllowsSubagentFlowToolsWhenExplicitlyEnabled|AllowAdminHighRiskTools|AllowHighRiskUserOverride)|TestChatAPI_SubagentsToolCall'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tool ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `npm test -- --test-name-pattern "Agent Runtime"` in `frontend/console`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #509.

## [0.31.48] - 2026-05-01

### Changed

- Q-011 follow-up: the low-use `process` tool is no longer injected into the default chat tool schema, while explicit session tool allowlists can still opt into it and background `exec` keeps its shared process manager.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'TestResolveInjectedToolSchemas_(AllowAdminHighRiskTools|AllowHighRiskUserOverride|AllowsDeprecatedProcessWhenExplicitlyEnabled)'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tool ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #510.

## [0.31.47] - 2026-05-01

### Changed

- Usage signal docs now include the 2026-04-26 to 2026-04-30 decision snapshot for Q-011, Q-012, Q-014, Q-015, Q-017, and Q-018, with focused follow-up issue links for the low-use surfaces.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/usage ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #418.

## [0.31.46] - 2026-05-01

### Added

- Agent Runtime runs now support status chips, 24h/7d/all time ranges, prompt search, originating chat session links, and top-level cost summaries for today, seven days, and grouped plan totals.
- `/v1/agentruntime/runs` now accepts `status`, `since`, and `search` query parameters for filtered run lists.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run TestAgentRunsAPIHandler_ListFiltersStatusSinceAndSearch`
- `npm test -- --test-name-pattern "Agent Runtime runs page|Agent Runtime run API client"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #420.

## [0.31.45] - 2026-04-30

### Added

- Memory now opens with a dismissible introduction card that explains MEMORY.md, Experiences, Daily Logs, Semantic Index, Prior Context recall, and the Try a Search workflow before editing.

### Tests

- `npm test -- --test-name-pattern "Memory page introduces|Memory page uses friendly|memory asset metadata explains"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #421.

## [0.31.44] - 2026-04-30

### Changed

- Memory now uses friendlier Stored Knowledge and Try a Search tab labels, with asset cards showing human-readable descriptions and hover hints for common memory files.

### Tests

- `npm test -- --test-name-pattern "Memory page uses friendly|memory asset metadata explains"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #422.

## [0.31.43] - 2026-04-30

### Added

- Memory asset cards now explain who fills each durable asset, who reads it, and when experience logs become stale after seven quiet days.

### Tests

- `npm test -- --test-name-pattern "memory asset metadata"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #423.

## [0.31.42] - 2026-04-30

### Changed

- System Prompt diagnostics are now hidden behind a default-closed technical details toggle so role semantics and built-in tool descriptions stay available without adding first-view noise.

### Tests

- `npm test -- --test-name-pattern "System Prompt diagnostics"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #424.

## [0.31.41] - 2026-04-30

### Added

- System Prompt now shows per-file prompt impact metadata with estimated tokens, section mapping, section character limits, truncation warnings, and a reloadable main/sub-agent system prompt preview.

### Tests

- `npm test -- --test-name-pattern "System Prompt page surfaces"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/prompt ./internal/sysprompt ./internal/tarsserver`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #425.

## [0.31.40] - 2026-04-30

### Added

- System Prompt now offers starter templates for empty or placeholder `IDENTITY.md`, `AGENTS.md`, and `TOOLS.md` files so users can insert opinionated defaults before saving.

### Tests

- `npm test -- --test-name-pattern "sysprompt"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #426.

## [0.31.39] - 2026-04-30

### Changed

- Operations is now an Approvals-focused console page with Disk/Process/Cron readouts removed, the nav renamed to Approvals, and `/console/approvals` routed alongside the legacy `/console/ops` path.

### Tests

- `npm test -- --test-name-pattern "Operations becomes"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #427.

## [0.31.38] - 2026-04-30

### Added

- Approvals now shows an empty-state guide explaining the review queue, cleanup-plan trigger, future Pulse-triggered approvals, approval decisions, and result logs.

### Tests

- `npm test -- --test-name-pattern "Ops explains"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #428.

## [0.31.37] - 2026-04-30

### Added

- Pulse now opens with a System Watchdog introduction card that explains monitored signals, LLM classifier actions, and the Settings `pulse_*` policy source before status readouts.

### Tests

- `npm test -- --test-name-pattern "Pulse introduces"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #429.

## [0.31.36] - 2026-04-30

### Added

- Pulse now compresses all-clear Recent Ticks into a summary timeline and highlights only signal-bearing ticks with warning, error, and autofix counts.

### Tests

- `npm test -- --test-name-pattern "Pulse compresses"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #430.

## [0.31.35] - 2026-04-30

### Added

- Pulse now explains the Min Severity notification floor, signal-kind severity mappings, threshold sources, and last-seen times inline on the status card.

### Tests

- `npm test -- --test-name-pattern "Pulse explains"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #431.

## [0.31.34] - 2026-04-30

### Added

- Reflection now starts with a Nightly Maintenance introduction card explaining the sleep window, memory job, cleanup job, manual run behavior, and Pulse failure signal.

### Tests

- `npm test -- --test-name-pattern "Reflection page introduces"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #432.

## [0.31.33] - 2026-04-30

### Added

- Reflection now previews the expected `Run Reflection Now` output before the first run and shows run totals, job details, errors, duration, and previous-run deltas after a manual run.

### Tests

- `npm test -- --test-name-pattern "Reflection"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #433.

## [0.31.32] - 2026-04-30

### Added

- Extensions now explains Skills, Plugins, and MCP Servers inline in both Installed and Hub views so each extension surface has a concise definition near its controls.

### Tests

- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #434.

## [0.31.31] - 2026-04-30

### Added

- Extensions now keeps Plugins collapsed by default in both Installed and Hub views and labels them as an advanced legacy surface while preserving existing plugin controls behind expansion.

### Tests

- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #435.

## [0.31.30] - 2026-04-30

### Added

- Extensions now marks Plugins as deprecated in both Installed and Hub views and points new extension work toward Skills (`.md + CLI`) while keeping legacy plugin installs available.

### Tests

- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #447.

## [0.31.29] - 2026-04-30

### Fixed

- Extension disabled-state updates now preserve corrupt state files and return load errors instead of silently replacing them with empty state.
- Ops approvals and usage limits now use atomic writes so failed state writes preserve the previous file contents.
- Ops manager empty-workspace defaults now use the same core default workspace path as runtime config.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/extensions ./internal/ops ./internal/usage ./internal/tarsserver`

### Closed

- Closes #415.

## [0.31.28] - 2026-04-30

### Fixed

- Skill runtime mirroring now surfaces companion file copy failures and removes affected skills from the runtime snapshot instead of leaving partially mirrored skills available.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/skill ./internal/extensions`

### Closed

- Closes #416.

## [0.31.27] - 2026-04-30

### Added

- Chat `@` mention autocomplete now includes AgentRuntime subagents alongside Files context.
- Selected subagent mentions are sent as explicit `subagent_mentions` chat hints and injected into the LLM system prompt so `subagents_run`, `subagents_orchestrate`, and `subagents_plan` can target the named agent.
- Context HUD now reports mentioned subagents for the current turn.

### Tests

- `go test ./internal/tarsserver -run 'TestChatAPI(InjectsSubagentMentionHints|RejectsUnknownSubagentMention|ToolCallSubagentsRun)'`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make console-build`
- `make fmt`
- `make vet`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `make build`
- Playwright browser verification at `http://127.0.0.1:43195/console`

## [0.31.26] - 2026-04-29

### Added

- Chat composer now supports leading `/` command autocomplete with built-in console actions and user-invocable skills.
- Skills can declare `slash` and `aliases` frontmatter metadata, and explicit `/skill` or `/alias` chat messages select that skill before the LLM turn starts.
- Extensions displays each user-invocable skill's slash command so installed skill entrypoints are easier to discover.

### Fixed

- Skill runtime paths injected into chat prompts now remain readable even when the active session current directory is not the workspace root.

### Tests

- `go test ./internal/skill ./internal/tarsserver -run 'TestParseFrontmatter|TestLoad_DefaultUserInvocableTrue|TestResolveSkillForMessage_UsesSkillSlashAlias|TestPrepareChatContextWithExtensions_InvokedSkillHint|TestSkillRuntimeReadPathForPrompt'`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make fmt`
- `make vet`
- `make test`
- `make console-build`
- `make build`
- `make security-scan`
- Browser verification: Playwright opened `/console`, confirmed `/` lists built-in commands and skill entries, selected `/qa-check` from `/qa` autocomplete, sent `/qa-check smoke`, verified `skill selected: qa-check`, and confirmed the LLM successfully read `workspace/_shared/skills_runtime/qa_check/SKILL.md`.

### Closed

- Closes #469.

## [0.31.25] - 2026-04-28

### Added

- Files > Workspace now includes an embedded Shell view powered by xterm, WebSocket streaming, and a PTY-backed server process.
- Integrated terminals start in the selected session work directory or browsed subdirectory and support keyboard input, command output, resizing, and explicit close.
- The existing macOS external Terminal action remains available as an Open App fallback.

### Security

- Integrated terminal WebSocket requests require admin access and reuse the session Files root validation before any PTY process starts.
- Requests outside session work directories, missing directories, files, and relative traversal outside the selected root are rejected.

### Fixed

- Request logging middleware now preserves HTTP hijacking so WebSocket upgrades can pass through the shared API middleware stack.

### Tests

- `go test ./internal/tarsserver -run 'TestRequestDebugMiddlewareSupportsWebSocketHijack|TestTerminalAPI_WebSocket|TestRegisterAPIRoutes_RegistersCoreRoutes'`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make fmt`
- `make vet`
- `make test`
- `make console-build`
- `make build`
- `make security-scan`
- Browser verification: Playwright opened `/console`, selected the main session, opened Files > Shell, verified terminal input/output by running `echo TARS_TERMINAL_OK` and `pwd`, and checked zero browser console errors or warnings.

### Closed

- Closes #484.

## [0.31.24] - 2026-04-28

### Added

- Files > Workspace now includes a Terminal action that opens the macOS Terminal app at the current session work directory or browsed subdirectory.
- `/v1/terminal/open` launches an external terminal for a session after resolving the requested cwd against the session's registered Files roots.

### Security

- Terminal launch requests require admin access and reject paths outside the session work directories, missing directories, files, and relative traversal outside the selected root.

### Tests

- `go test ./internal/tarsserver -run 'TestTerminalAPI|TestRegisterAPIRoutes_RegistersCoreRoutes'`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make fmt`
- `make vet`
- `make test`
- `make console-build`
- `make build`
- `make security-scan`
- Browser verification: Playwright opened `/console/chat/<session>`, confirmed Files > Workspace shows an enabled Terminal action for the selected session workdir, clicked it, verified `/v1/terminal/open` returned 200, and checked zero browser console warnings.

### Closed

- Closes #482.

## [0.31.23] - 2026-04-28

### Added

- Chat composer now supports `@` file and directory mentions sourced from the session Files roots.
- Mention autocomplete resolves against the session current directory and registered Files paths, then injects selected file content or directory listings into the next LLM request.
- Mentioned context is reported in the Context HUD for turn-level visibility.

### Security

- File mention resolution is revalidated server-side and rejects parent traversal, missing paths, and roots outside the session Files paths.

### Tests

- `go test ./internal/tarsserver -run 'TestChat(FileMention|APIInjects|APIRejects)'`
- `go test ./internal/tarsserver`
- `go test ./...`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make fmt`
- `make vet`
- `make test`
- `make console-build`
- `make build`
- Browser verification: Playwright opened `/console/chat/<session>`, confirmed typing `@rea` shows the `README.md` autocomplete candidate from the session current Files root, selected it with Enter, and verified the composer shows an active `@README.md` mention chip.

### Closed

- Closes #468.

## [0.31.22] - 2026-04-28

### Added

- Agent Runtime Subagents now includes an LLM-assisted builder for drafting new workspace `AGENT.md` profiles from a natural-language request.
- Workspace subagents can be edited with the builder, previewed, approved, and saved back to their source profile.
- Editable workspace subagents can be archived with confirmation; archived `AGENT.md` files are renamed out of the active catalog and the runtime executor list is refreshed.
- Builder drafts use configured LLM tiers, expose safe tool allow/deny lists, and normalize common LLM action aliases such as `edit` into the persisted update workflow.

### Tests

- `go test ./internal/tarsserver -run 'TestAgentRuntimeSubagentsAPIHandler_(Builder|Patch|List|Detail)|TestAgentRuntimeSubagentBuilderLLMPromptMentionsJSON|TestNormalizeAgentRuntimeSubagentDraftMapsLLMEditAction'`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make fmt`
- `make vet`
- `make test`
- `make console-build`
- `make build`
- `make security-scan`
- Browser verification: Playwright opened `/console/agentruntime/subagents`, generated a new `frontend-reviewer` workspace subagent with the LLM builder on the `heavy` tier, approved and saved it, edited `researcher` with an accessibility-focused LLM draft, approved and saved the edit, archived `researcher`, confirmed the API catalog and workspace files reflected the changes, checked zero browser console warnings, and verified no horizontal overflow at 390px mobile width.

### Closed

- Closes #472.

## [0.31.21] - 2026-04-28

### Added

- Settings now opens `llm.tiers` in a typed tier editor instead of the generic JSON editor.
- The typed tier editor can add, rename, edit, and remove custom tier bindings with separate controls for provider, model, reasoning effort, thinking budget, and service tier.
- Tier provider choices are populated from configured `llm.providers`, and invalid tier rows show inline errors before changes can be staged or saved.

### Tests

- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make fmt`
- `make vet`
- `make test`
- `make console-build`
- `make build`
- `make security-scan`
- Browser verification: Playwright opened `/console/config`, confirmed `llm.tiers` uses the typed editor, verified missing model validation stays inline, added a `turbo` tier, changed `heavy` to `gpt-5.5`, removed `light`, saved via Settings, confirmed the refreshed schema API and config file include the saved tiers, checked zero browser console warnings, and verified no horizontal overflow at 390px mobile width.

### Closed

- Closes #475.

## [0.31.20] - 2026-04-28

### Fixed

- Settings JSON editor modals now center within the content area beside the fixed navigation instead of rendering underneath the sidebar.
- The JSON editor now keeps consistent viewport margins and a bounded editor height on desktop and mobile.

### Tests

- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make fmt`
- `make vet`
- `make test`
- `make console-build`
- `make build`
- `make security-scan`
- Browser verification: Playwright opened `/console/config`, opened `llm.tiers`, confirmed the modal and backdrop clear the fixed sidebar on a 1175px desktop viewport, verified the modal fits within the viewport, confirmed no horizontal overflow at 390px mobile width, and checked zero browser console warnings.

### Closed

- Closes #477.

## [0.31.19] - 2026-04-28

### Added

- Settings now summarizes object and array values with compact counts and key previews instead of rendering hard-to-read one-line JSON blobs.
- Object and array config fields now open a focused JSON editor modal with pretty-printed content, reset/cancel/apply actions, and inline parse errors before changes are staged.
- `/v1/admin/config/schema` now refreshes values from the config file after Settings saves, while preserving the runtime workspace override shown in the Console.

### Tests

- `TestConfigAPI_SchemaReflectsPatchedValues`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make fmt`
- `make vet`
- `make test`
- `make build`
- `make console-build`
- `make security-scan`
- Browser verification: Playwright opened `/console/config`, confirmed structured summaries for `llm.tiers`, verified invalid JSON stays in the editor with an inline parse error, saved a new `turbo` tier, confirmed the refreshed schema API and config file include the tier, checked no browser console warnings, and verified no horizontal overflow at 390px mobile width.

### Closed

- Closes #473.

## [0.31.18] - 2026-04-27

### Added

- Agent Runtime now has a `Runs | Subagents` tab split in the Console. The new Subagents tab shows the active agent catalog, default/effective LLM tier, resolved provider/model preview, source/entry metadata, tool policy, and recent run links.
- Workspace `AGENT.md` subagents can now update their default LLM tier from the Subagents detail panel using the configured LLM tier catalog.
- Added `/v1/agentruntime/subagents` and `/v1/agentruntime/subagents/{name}` endpoints that expose subagent metadata with Settings-defined LLM tier options, clear missing-tier diagnostics, and safe tier updates for editable workspace profiles.

### Fixed

- Agent Runtime runs now use an executor's configured agent tier when the spawn request does not provide a task-level tier, matching the documented priority of task `tier` > agent YAML `tier` > config default.
- Empty subagent tool-policy arrays are serialized as `[]` instead of `null`, preventing Console detail rendering errors.

### Tests

- `TestRuntimeSpawn_UsesExecutorTierWhenRequestTierEmpty`
- `TestAgentRuntimeSubagentsAPIHandler_ListIncludesTiersAndRunTelemetry`
- `TestAgentRuntimeSubagentsAPIHandler_DetailMarksMissingTier`
- `TestAgentRuntimeSubagentsAPIHandler_PatchWorkspaceTierReloadsExecutor`
- `TestAgentRuntimeSubagentsAPIHandler_PatchRejectsUnknownTier`
- `npm run check` in `frontend/console`
- `make console-build`
- `make fmt`
- `make vet`
- `make test`
- `make build`
- Browser verification: Playwright opened `/console/agentruntime/subagents` on a local TARS server, confirmed tier catalog/list/detail rendering, selected the `researcher` subagent, returned to the Runs tab, and verified no horizontal overflow at 390px mobile width.

### Closed

- Closes #471.

## [0.31.17] - 2026-04-27

### Added

- Agent Runtime page onboarding card explaining that the page records subagent work launched from chat, including the prompt, model tier, status, response, live events, and consensus cost data when available.
- Agent Runtime empty state guide with starter chat prompts for `subagents_run` / `subagents_orchestrate` / `subagents_plan` workflows plus a direct "Open Chat" action.

### Tests

- `npm run check` in `frontend/console`
- `make console-build`
- `make build`
- `make test`
- Browser verification: Playwright opened `/console/agentruntime` on a local TARS server, confirmed the onboarding/empty-state text on desktop and mobile widths, and verified no horizontal overflow at 390px.

### Closed

- Closes #419.

## [0.31.16] - 2026-04-27

### Fixed

- Context compaction now reinjects the session's active plan state immediately after the compaction summary for automatic chat compaction, manual session compaction, `/v1/compact`, cron-bound session runs, and Telegram-bound session runs. The reinjected block includes the active plan plus `pending` / `in_progress` tasks only, so completed or cancelled tasks are not resurfaced to the LLM.
- History loading now treats the active-plan injection as part of the compaction boundary, so tiny token budgets that force-include the compaction summary also keep the preserved task state visible.
- Repeated compactions replace older injected task blocks with fresh state instead of carrying stale task snapshots forward.

### Tests

- `TestFormatTasksForInjection_ExcludesInactivePlanWithNoActiveTasks`
- `TestLoadHistory_IncludeCompactionTaskInjectionBoundary`
- `TestCompactAPI_ReinjectsActiveTasks`
- `TestHandleChatRequest_EmitsCompactionAppliedEvent`
- Browser verification: Playwright opened a local TARS server, created a session/tasks, triggered manual compaction, and confirmed the transcript contains summary + active-plan injection with completed tasks omitted.

### Closed

- Closes #392.

## [0.31.15] - 2026-04-27

### Added

- `runtime.plan_clarify_mode` config (env: `TARS_PLAN_CLARIFY_MODE`, schema dropdown in Settings → Runtime). Three values:
  - `smart` (default) — LLM evaluates ambiguity itself: asks 1–3 clarifying questions when scope/success/constraints are unclear, drafts immediately when clear.
  - `auto` — never ask, always draft immediately.
  - `ask` — always front-load 1–3 clarifying questions before drafting.
- The Planning section in the system prompt now branches on the mode. Unknown / empty values fall back to `smart` so a typo can't silently flip planning into the noisier `ask` stance. The downstream propose/approve guidance (CON-053) and runtime intervention vocabulary (CON-054) remain identical across all three modes.

### Tests

- `TestBuild_PlanningSectionClarifyModes` — locks the per-mode prompt content and verifies unknown values fall back to smart.

### Closed

- Closes #455.

## [0.31.14] - 2026-04-27

### Added

- Runtime intervention buttons in TasksPanel during `executing` / `paused` states. Plan-level toolbar offers **⏸ Pause** (cancels the in-flight chat turn first via `cancelChat`, then `plan_pause`), **▶ Resume** (flips back to executing and auto-sends `continue`), **✎ Edit Plan** (reuses CON-053 inline editor for retitle/add/remove), and **⊘ Abort** (confirm + chat cancel + `plan_abort`). Each task card gains a per-row **⏭ Skip** button when the plan is live: marks the task `cancelled` and asks the LLM to move on with a structured follow-up message ("Skip task N (title) and continue with the next pending task").
- Resume sends `continue` so the LLM picks up the next turn without the user retyping; failure of the auto-send is best-effort.

### Closed

- Closes #457.

## [0.31.13] - 2026-04-27

### Added

- Propose/approve UI in the chat TasksPanel. When the LLM moves the plan to `proposed`, the panel surfaces a "Plan ready for review" banner with three CTAs: **✓ Approve & Run** (calls `plan_approve` and auto-sends `go` so the LLM picks the next turn), **✎ Edit Plan** (per-task title/description inputs, add/remove rows, save in one batch), and **✗ Discard** (confirm + `clear`). The plan card itself now shows a colored status badge (`drafting` / `proposed` / `executing` / `paused` / `completed` / `aborted`).
- `POST /v1/admin/sessions/{id}/tasks` — drives the tasks aggregator from the console without going through the chat loop. Body shape mirrors the LLM-side tool call (`{"action":"plan_approve"}`, `{"action":"add","title":"…"}`, etc.). Backed by the same `NewTasksTool` instance the chat path uses, so state-machine guarantees (CON-051) hold uniformly across both surfaces.
- `ChatPanel.sendMessageText(text)` — programmatic submit that lets sibling panels (TasksPanel) emit follow-up turns without forcing the user back to the composer.
- Planning section in the system prompt expanded to teach the propose/approve flow (`plan_propose` → STOP → user `go` → `plan_approve`) and explain the runtime intervention vocabulary (`paused` / `aborted`).

### Tests

- `TestSessionAPI_TasksPOSTInvokesAggregator` — drives plan_set → propose → approve → invalid-transition rejection through the new endpoint.
- Browser verification: open Tasks panel, click Approve & Run, observe the chat send `go` and the LLM resume work with task 1 marked in_progress.

### Closed

- Closes #456.

## [0.31.12] - 2026-04-27

### Changed

- Renamed `config/standalone.yaml` → `config/default.yaml`. The "standalone" name was a leftover from the now-removed `runtime.mode` field; "default" matches what the file actually is. `DefaultConfigFilename`, `tars init` legacy candidates, doctor hints, Makefile, CLAUDE.md, and three test fixtures updated. The pre-rename name stays in `tars init`'s legacy candidate list so existing workspaces continue to migrate cleanly.
- `tars init` starter no longer emits `runtime.mode: standalone` (field removed in 0.31.11).
- Refreshed `config/tars.config.example.yaml` so every value matches current defaults: `claude-opus-4-7` / `claude-haiku-4-5-20251001` model IDs, `gemini-embedding-2-preview` embed model, `pulse.cron_failure_threshold=3` / `stuck_run_minutes=60` / `reflection_failure_threshold=3`, `compaction.trigger_tokens=100000` / `keep_recent_tokens=12000`, `assistant.enabled=true` / `whisper-cli` / canonical hotkey, `tools.default_set=standard`, `telegram.polling.enabled=true`, `notify.when_no_clients=true`, plus the missing `memory_hook` and `reflection_kb` role mappings. Also notes that external Go plugins are deprecated.

## [0.31.11] - 2026-04-27

### Removed (BREAKING)

- `runtime.mode` config field, `TARS_MODE` env var, and `--mode` CLI flag. The field was cosmetic from day one — server execution is decided entirely by the separate `--serve-api` boolean flag, never by Mode. The schema option, the Settings page combobox, and the startup message simply showed the value back to the user without affecting any code path.
- Settings page no longer offers a "Mode" combobox under Runtime — the field disappears automatically once removed from the schema.
- Startup stdout message changed from `tars starting in <mode> mode` to `tars startup complete (no --serve-api flag, exiting)` — describes the actual outcome instead of repeating a meaningless config value.

### Migration

- YAML parser ignores unknown keys, so existing `runtime.mode` lines in `workspace/config/tars.config.yaml` are silently dropped on next load. No user action required.

## [0.31.10] - 2026-04-26

### Added

- Plan state machine — `session.Plan` now carries a `Status` field with six states (`drafting`, `proposed`, `executing`, `paused`, `completed`, `aborted`) plus an `UpdatedAt` timestamp. State validation helper `session.ValidPlanStatus`.
- Five new `tasks` tool actions: `plan_propose` (drafting → proposed), `plan_approve` (proposed → executing), `plan_pause` (executing → paused), `plan_resume` (paused → executing), `plan_abort` (any active state → aborted). Each rejects invalid transitions with an explicit error.
- Two automatic transitions guard against LLM omissions and surface real progress: a task moving to `in_progress` auto-promotes a `proposed` plan to `executing`, and a plan flips to `completed` once every task is `completed` or `cancelled`.
- `SessionPlan.status` / `updated_at` exposed in the frontend type for upcoming UI work (CON-053 propose/approve, CON-054 runtime intervention).

### Migration

- Plans saved before this field existed have empty `Status` on disk; on load they default to `executing` so existing sessions keep their prior behavior with zero user action.

### Tests

- `TestValidPlanStatus`, `TestLegacyPlanWithoutStatusDefaultsToExecuting`
- `TestTasks_PlanSetSetsDraftingStatus`
- `TestTasks_PlanProposeDraftingToProposed` / `TestTasks_PlanProposeRejectsExecuting`
- `TestTasks_PlanApproveProposedToExecuting` / `TestTasks_PlanApproveRejectsDrafting`
- `TestTasks_AutoExecutingOnFirstInProgress`
- `TestTasks_AutoCompletedWhenAllTasksDone`
- `TestTasks_PlanPauseAndResume` / `TestTasks_PlanPauseRejectsDrafting`
- `TestTasks_PlanAbortFromAnyActiveState` / `TestTasks_PlanAbortRejectsTerminal`
- `TestTasks_PlanTransitionWithoutPlan`

### Closed

- Closes #454.

## [0.31.9] - 2026-04-26

### Fixed

- Pulse-bar `Tasks` badge now shows `(completed / total)` instead of `(in_progress / total)`. Between turns the in-progress count is almost always 0, so the badge always read `0/N` and looked broken even when work had finished. Matches the TasksPanel header. Hover tooltip surfaces the full breakdown (`N done · N in progress · N pending`).

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
