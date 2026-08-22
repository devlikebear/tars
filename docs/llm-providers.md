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
| `antigravity-cli` | local `agy` CLI            | `antigravity_cli.go`                |

## ChatOptions × Provider

| Field                        | anthropic | openai | kimi  | openai-codex | gemini (compat) | gemini-native | claude-code-cli | antigravity-cli |
|------------------------------|-----------|--------|------|--------------|-----------------|---------------|-----------------|-----------------|
| `OnDelta` (streaming)        | ✅        | ✅     | ✅   | ✅           | ✅              | ✅            | ✅              | ✅ |
| `Tools`                      | ✅        | ✅     | ✅   | ✅           | ✅              | ✅            | ❌ (silent)     | ❌ (silent) |
| `ToolChoice` auto/none/required | ✅     | ✅     | ✅   | ✅           | ✅              | ✅            | ❌ (silent)     | ❌ (silent) |
| `ToolChoice` specific        | ✅        | ✅     | ✅   | ✅           | ✅              | ✅            | ❌ (silent)     | ❌ (silent) |
| `ResponseFormat` text        | ✅ (default) | ✅  | ✅   | ✅           | ✅              | ❌ (silent)   | ❌ (silent)     | ✅ (default) |
| `ResponseFormat` json_object | ❌ (silent) | ✅  | ✅   | ✅           | ✅              | ❌ (silent)   | ❌ (silent)     | ❌ (silent) |
| `ResponseFormat` json_schema | ❌ (silent) | ✅  | ✅   | ✅           | ✅              | ❌ (silent)   | ❌ (silent)     | ✅ `--json-schema` |
| `ReasoningEffort`            | ❌ (silent) | ✅  | ✅   | ✅           | ❌ (skipped)    | partial¹      | ❌ (silent)     | ✅ `--effort` |
| `ThinkingBudget`             | ✅        | ❌     | ❌   | ❌           | ❌              | ✅            | ❌ (silent)     | ❌ (silent) |
| `ServiceTier`                | ❌ (silent) | ✅  | ✅   | ✅           | ❌ (skipped)    | ❌ (silent)   | ❌ (silent)     | ❌ (silent) |
| `ResumeSessionID`            | ❌ (silent) | ❌  | ❌   | ❌           | ❌              | ❌            | ✅ `--resume`   | ✅ `--conversation` |
| `ClaudeCodeMCPServers`       | ❌ (silent) | ❌  | ❌   | ❌           | ❌              | ❌            | ✅ temporary config | ❌ (silent) |
| `ClaudeCodePermissionMode`   | ❌ (silent) | ❌  | ❌   | ❌           | ❌              | ❌            | ✅ validated mode | ❌ (silent) |
| `ClaudeCodeSkills`           | ❌ (silent) | ❌  | ❌   | ❌           | ❌              | ❌            | ✅ temporary plugin | ❌ (silent) |
| `ClaudeCodePermissionDeny`   | ❌ (silent) | ❌  | ❌   | ❌           | ❌              | ❌            | ✅ tightening-only settings | ❌ (silent) |
| `ClaudeCodeHarness`          | ❌ (silent) | ❌  | ❌   | ❌           | ❌              | ❌            | ✅ safe/tool/budget controls | ❌ (silent) |
| `ContentBlocks` text         | ✅        | ✅     | ✅   | ✅           | ✅              | ✅            | ❌               | ❌ |
| `ContentBlocks` image        | ✅        | ✅     | ✅   | ✅           | ✅              | ✅            | ❌               | ❌ |
| `ContentBlocks` document/PDF | ✅        | ⛔ error² | ⛔ error² | ⛔ error²  | ⛔ error²       | partial³      | ❌ (silent)     | ❌ (silent) |

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
| antigravity-cli | `--json-schema '<schema>'` (applies to terminal `result`) |

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

### Antigravity CLI

`antigravity-cli` shells out to a locally installed `agy` the same way
`claude-code-cli` shells out to `claude`, via
`agy --output-format stream-json --print <text>`.

**Minimum CLI version: 1.1.12.** `stream-json` itself arrived in 1.1.8, but the
`tool_info` object this provider reads for the tool audit trail and the
`cache_read_tokens` field it maps into `llm.Usage` were both added in 1.1.12.
On 1.1.8-1.1.11 the turn still succeeds but those two lose their data
silently. Verified end to end against 1.1.13.

- **No credential passes through TARS.** `agy` owns its Google login and reads
  cached credentials from the system keyring. Authenticate once in an
  interactive `agy` session before using this provider. The kind takes no
  `api_key` and no `base_url`.
- **Model selection is explicit.** Use a slug reported by `agy models` in each
  TARS tier. TARS does not hard-code a default slug because the available set
  changes with the account and CLI release.
- **No system-prompt flag.** System messages are folded into a leading prompt
  block. On resumed turns that block and the old transcript are not replayed.
- **Resumable sessions.** The stream's `conversation_id` becomes
  `ChatResponse.SessionID`; subsequent turns pass it through
  `--conversation`, avoiding transcript replay.
- **Execution mode fails safe.** `AGY_CLI_MODE` may select `accept-edits` or
  `plan`. Unknown values omit `--mode`, leaving the CLI's own permission policy
  in force. TARS deliberately exposes no path to
  `--dangerously-skip-permissions`.
- **Tools are the CLI's, not TARS'.** `ChatOptions.Tools` and `ToolChoice` are
  ignored. Calls already executed by `agy` are reported on
  `ChatResponse.ProviderExecutedTools` for audit only and are never
  re-dispatched through TARS. The effective file/command authority comes from
  the user's Antigravity permission settings.
- **Structured output and reasoning.** `ResponseFormat` `json_schema` maps to
  `--json-schema`; `ReasoningEffort` maps to `--effort low|medium|high`.
- **Usage** comes from the terminal result's `usage` object. Input, output, and
  cache-read tokens map into `llm.Usage`; `thinking_tokens` and `total_tokens`
  have no provider-neutral destination today. The CLI reports no cost, so
  `Usage.CostUSD` stays 0.
- `AGY_CLI_PATH`, `AGY_CLI_TIMEOUT`, and `AGY_CLI_MODE` are the provider's
  environment overrides.

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
