# Public Agent Packages

TARS exposes a small set of public Go packages for building lightweight agent
apps without copying TARS internals or running the TARS server. These packages
are intentionally additive wrappers around TARS-tested primitives.

## Packages

| Package | Purpose |
| --- | --- |
| `github.com/devlikebear/tars/pkg/llm` | Provider-normalized chat messages, tool schemas, tool choices, provider clients, and tier routing. |
| `github.com/devlikebear/tars/pkg/tools` | Tool result types, registries, tool policies, file tools, shell/process tools, web tools, and memory tool adapters. |
| `github.com/devlikebear/tars/pkg/agentloop` | The iterative LLM tool-calling loop with hooks, repeated-call guards, provider-executed tool audit events, and streaming callbacks. |
| `github.com/devlikebear/tars/pkg/memory` | File-backed workspace memory, daily logs, reviewed experiences, semantic search primitives, and the memory `Backend` interface. |
| `github.com/devlikebear/tars/pkg/skill` | `SKILL.md` frontmatter parsing, source loading, availability filtering, slash-command loading, and prompt formatting. |
| `github.com/devlikebear/tars/pkg/mcp` | Public MCP server config, client wrapper, tool listing, tool-call execution, and MCP-to-tool adaptation. |

## Minimal Agent Shape

Use `pkg/llm` to pick a provider, `pkg/tools` to register tools, and
`pkg/agentloop` to run the tool-calling loop:

```go
client, err := llm.NewProvider(llm.ProviderOptions{
	Provider: "openai",
	Model:    "gpt-5.4-mini",
	APIKey:   os.Getenv("OPENAI_API_KEY"),
})
if err != nil {
	return err
}

registry := tools.NewRegistry()
registry.Register(tools.NewReadFileTool("/path/to/workspace"))

loop := agentloop.New(client, registry)
resp, err := loop.Run(ctx, []llm.ChatMessage{
	{Role: "user", Content: "Summarize this workspace."},
}, agentloop.RunOptions{Tools: registry.Schemas()})
```

`examples/min-agent` provides a no-network smoke example with a scripted client
and a tiny tool.

## Boundaries

These packages are for agent building blocks, not for embedding the whole TARS
application. The following surfaces remain internal for now:

- TARS server routes, auth middleware, console handlers, and session APIs.
- Full Agent Runtime orchestration, persistence, restart, consensus, usage, and
  workspace/session ownership logic.
- Pulse, reflection, ops, cron store wiring, and other TARS system surfaces.
- Session-bound skill extraction and other features that depend on TARS
  transcript storage.

When an external app needs a TARS server capability, prefer talking to the
HTTP API through `pkg/tarsclient` rather than importing server internals.
