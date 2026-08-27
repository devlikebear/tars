# LLM Provider Capability Matrix

How each `internal/llm` provider handles the fields in `llm.ChatOptions`. Caller-side reference so you don't have to grep the wire-format converter to know whether a knob takes effect.

> **Source of truth: `internal/llm/capabilities.go`.** That file declares what each provider supports, and the conformance suite in `conformance_test.go` verifies each declaration against real client behavior with a stub transport. A wrong declaration is a failing test, not a stale table.
>
> This page stays hand-written because it carries the *wire-level* detail — exact JSON shapes, error codes, CLI flags — that a boolean matrix cannot. Where the two disagree, `capabilities.go` and its tests win.
>
> Requesting a capability a provider does not declare now produces a warning at provider construction naming the setting and the provider (`reportUnsupportedCapabilities`). Nothing is dropped in silence any more.

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
| `ReasoningEffort`            | ✅⁴       | ✅     | ✅⁵  | ❌ (dropped)⁶ | ❌ (warned)    | partial¹      | ❌ (warned)     | ✅ `--effort` |
| `ThinkingBudget`             | ✅⁴       | ❌     | ❌   | ❌           | ❌              | ✅            | ❌ (silent)     | ❌ (silent) |
| `ServiceTier`                | ❌ (warned) | ✅  | ✅⁵  | ❌ (dropped)⁶ | ❌ (warned)    | ❌ (warned)   | ❌ (warned)     | ❌ (warned) |
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
⁴ Anthropic renders reasoning depth by model generation (#943): adaptive-thinking models take `output_config.effort`, budget-generation models take `thinking.budget_tokens`. `ThinkingBudget` is rejected at config-resolve time on an adaptive model — it cannot be expressed there.
⁵ Kimi drops both on tool-calling turns; the Moonshot endpoint rejects the pair alongside `tools`.
⁶ `openai-codex` is constructed with `DefaultClientConfig()` in `NewProvider`, which discards `MaxTokens`, `ReasoningEffort`, `ServiceTier`, and `BetaFeatures` before they reach the client — the codex client implements `reasoning_effort` and `service_tier`, but nothing hands them to it. Declared unsupported so the settings warn rather than vanish; wiring the tier's knobs through is separate work, tracked outside this epic.

"(warned)" means the setting is accepted by config, found unsupported at provider construction, and logged — see `reportUnsupportedCapabilities`. "(silent)" marks the fields that still have no capability entry because they are provider-specific request plumbing rather than portable knobs.

## Adding a provider

1. Add the construction case to `NewProvider` in `internal/llm/provider.go`.
2. Add a `providerCapabilities` entry in `internal/llm/capabilities.go`. **This is not optional** — `TestEveryProviderKindDeclaresCapabilities` fails the build for a constructible kind with no declaration, and for a declaration naming a kind that cannot be built.
3. If the provider speaks HTTP, add a `conformanceProvider` row to `conformanceProviders()` in `conformance_test.go` with its chat path, base-URL suffix, and three canned response bodies (plain, tool call, usage). The whole scenario table then runs against it.
4. Run the suite. Declarations are verified against real behavior, so a capability you claimed but did not implement fails immediately — as does one you implemented but did not declare (`TestConformance_ServiceTierReachesTheRequestOnlyWhereDeclared` checks both directions).

CLI-backed providers are declared but not run through the scenario table: driving them needs a process stub, and `internal/llm` must stay Windows-clean. Cover their conversion logic with pure-function tests instead, the way `antigravity_cli_test.go` parses its NDJSON stream without a subprocess.

The suite makes no network calls. Live tests live behind `//go:build integration` and skip without credentials.

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
