# LLM Provider Capability Matrix

How each `internal/llm` provider handles the fields in `llm.ChatOptions`. Caller-side reference so you don't have to grep the wire-format converter to know whether a knob takes effect.

> Last updated post-PR #380/#381/#382 (ID-004 Phase 1+2+wrap-up). Source of truth: the converter functions in `internal/llm/`. If this table disagrees with the code, the code wins — file an issue.

## Providers

| Alias (`kind`)    | Wire API                   | File                                |
|-------------------|----------------------------|-------------------------------------|
| `anthropic`       | Anthropic Messages         | `anthropic.go`                      |
| `openai`          | OpenAI Chat Completions    | `openai_compat_client.go`           |
| `kimi`            | OpenAI Chat Completions    | `openai_compat_client.go` (label)   |
| `openai-codex`    | OpenAI Responses           | `openai_codex_client.go`            |
| `gemini`          | Gemini OpenAI-compat       | `openai_compat_client.go` (label)   |
| `gemini-native`   | Google Gemini REST         | `gemini_native*.go`                 |
| `claude-code-cli` | local `claude` CLI         | `claude_code_cli.go`                |

## ChatOptions × Provider

| Field                        | anthropic | openai | kimi  | openai-codex | gemini (compat) | gemini-native | claude-code-cli |
|------------------------------|-----------|--------|------|--------------|-----------------|---------------|-----------------|
| `OnDelta` (streaming)        | ✅        | ✅     | ✅   | ✅           | ✅              | ✅            | ✅              |
| `Tools`                      | ✅        | ✅     | ✅   | ✅           | ✅              | ✅            | ❌ (silent)     |
| `ToolChoice` auto/none/required | ✅     | ✅     | ✅   | ✅           | ✅              | ✅            | ❌ (silent)     |
| `ToolChoice` specific        | ✅        | ✅     | ✅   | ✅           | ✅              | ✅            | ❌ (silent)     |
| `ResponseFormat` text        | ✅ (default) | ✅  | ✅   | ✅           | ✅              | ❌ (silent)   | ❌ (silent)     |
| `ResponseFormat` json_object | ❌ (silent) | ✅  | ✅   | ✅           | ✅              | ❌ (silent)   | ❌ (silent)     |
| `ResponseFormat` json_schema | ❌ (silent) | ✅  | ✅   | ✅           | ✅              | ❌ (silent)   | ❌ (silent)     |
| `ReasoningEffort`            | ❌ (silent) | ✅  | ✅   | ✅           | ❌ (skipped)    | partial¹      | ❌ (silent)     |
| `ThinkingBudget`             | ✅        | ❌     | ❌   | ❌           | ❌              | ✅            | ❌ (silent)     |
| `ServiceTier`                | ❌ (silent) | ✅  | ✅   | ✅           | ❌ (skipped)    | ❌ (silent)   | ❌ (silent)     |
| `ResumeSessionID`            | ❌ (silent) | ❌  | ❌   | ❌           | ❌              | ❌            | ✅ `--resume`   |
| `ClaudeCodeMCPServers`       | ❌ (silent) | ❌  | ❌   | ❌           | ❌              | ❌            | ✅ temporary config |
| `ClaudeCodePermissionMode`   | ❌ (silent) | ❌  | ❌   | ❌           | ❌              | ❌            | ✅ validated mode |
| `ClaudeCodeSkills`           | ❌ (silent) | ❌  | ❌   | ❌           | ❌              | ❌            | ✅ temporary plugin |
| `ClaudeCodePermissionDeny`   | ❌ (silent) | ❌  | ❌   | ❌           | ❌              | ❌            | ✅ tightening-only settings |
| `ClaudeCodeHarness`          | ❌ (silent) | ❌  | ❌   | ❌           | ❌              | ❌            | ✅ safe/tool/budget controls |
| `ContentBlocks` text         | ✅        | ✅     | ✅   | ✅           | ✅              | ✅            | ❌               |
| `ContentBlocks` image        | ✅        | ✅     | ✅   | ✅           | ✅              | ✅            | ❌               |
| `ContentBlocks` document/PDF | ✅        | ⛔ error² | ⛔ error² | ⛔ error²  | ⛔ error²       | partial³      | ❌ (silent)     |

¹ Gemini-native maps `ReasoningEffort` to `thinkingBudget` heuristically; explicit `ThinkingBudget` overrides.
² Returns a `pdf_unsupported_by_provider` ProviderError at build time (RF-046). Convert PDFs to text/images before sending.
³ Gemini-native accepts inline base64 PDFs but currently has no caching path — large PDFs are re-uploaded each turn.

## Wire format details

### `ToolChoice` specific tool name

| Provider     | Wire shape                                                      |
|--------------|-----------------------------------------------------------------|
| anthropic    | `{"type":"tool","name":"<n>"}`                                  |
| openai (Chat)| `{"type":"function","function":{"name":"<n>"}}`                 |
| kimi (Chat)  | `{"type":"function","function":{"name":"<n>"}}`                 |
| openai-codex | `{"type":"function","function":{"name":"<n>"}}` (Responses API) |
| gemini-native| `functionCallingConfig.{mode:"ANY", allowedFunctionNames:["<n>"]}` |

### `ResponseFormat` json_schema

| Provider     | Wire shape                                                      |
|--------------|-----------------------------------------------------------------|
| openai (Chat)| `response_format: {type:"json_schema", json_schema:{name,schema,strict}}` |
| kimi (Chat)  | `response_format: {type:"json_schema", json_schema:{name,schema,strict}}` |
| openai-codex | `text.format: {type:"json_schema", name, schema, strict}` (Responses API flattens) |

The two OpenAI surfaces use different envelopes — `internal/llm` exposes a single `ResponseFormat` struct and each client serializes accordingly.

### Claude Code execution-harness controls

Normal `claude-code-cli` chat keeps the existing session MCP, skill, permission,
and resume behavior. `ClaudeCodeHarness` is a separate caller-only control used
by the Execution Plane. It can enable safe mode, strict MCP discovery, Chrome
disablement, an isolated child environment, explicit tool/allow lists, and
turn/USD limits; it cannot inject arbitrary arguments, environment variables,
settings, plugins, MCP servers, or credentials. Stream result events also map
`num_turns` and `total_cost_usd` into `ChatResponse.Turns` and
`ChatResponse.Usage.CostUSD` so the durable scheduler can enforce and report
actual usage.

### `ReasoningEffort` and `ServiceTier` (OpenAI)

Both Chat Completions and Responses API surface these:

| Provider     | Reasoning wire shape                | Service tier wire shape |
|--------------|-------------------------------------|--------------------------|
| openai (Chat)| `reasoning_effort: "<value>"` (top-level) | `service_tier: "<value>"` |
| kimi (Chat)  | `reasoning_effort: "<value>"` (top-level) | `service_tier: "<value>"` |
| openai-codex | `reasoning: {effort: "<value>"}` (object) | `service_tier: "<value>"` |
| gemini compat| skipped (label-gated in openai_compat) | skipped |

## Forward-looking gaps (out of scope for ID-004)

- Gemini-native structured output (`responseSchema`) and prompt caching — Phase 4 (not selected from #366 option matrix; revisit if Gemini usage grows).
- `gemini-compat` deprecation — duplicates `gemini-native` capability with worse fidelity. Phase 5 of #366; pending separate decision.
- Provider registry refactor (RF-066) and `yaml_paths` DRY (RF-065) — independent of capability work.

## How to update this matrix

When you change a provider's wire-format converter (any of: `toAnthropicToolChoice`, `toOpenAIToolChoice`, `toOpenAIResponseFormat`, `toCodexTextFormat`, `toGeminiNativeToolConfig`, `containsPDFDocumentBlock`), update the corresponding row here in the same PR. The ChatOptions struct in `internal/llm/provider.go` should never gain a field whose support is undocumented.
