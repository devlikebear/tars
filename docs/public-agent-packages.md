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

`examples/real-provider` is the same shape built against a real provider
client. It exists because a scripted client never type-checks `llm.NewProvider`
— so the documented shape above could drift from the actual constructor and
every test would still pass. It reads its credential from the environment and
prints what it would have done when there is none, so it stays runnable in a
fresh checkout.

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

## Stability policy

The module is `v0`. Semver permits breaking changes at any `v0` minor, but
"semver permits it" is not a workflow when a real consumer exists — so the
following is what this project actually commits to.

### What you can rely on

| Package | Status | Meaning |
| --- | --- | --- |
| `pkg/llm` | **Stable** | Owns its types; a break here is announced (see below). |
| `pkg/session` | **Stable** | Same. |
| `pkg/tarsclient` | **Stable** | HTTP client for the TARS API; zero internal dependencies. |
| `pkg/tools` | Experimental | Expect movement as the core/app tool split settles. |
| `pkg/agentloop` | Experimental | The loop's hook surface is still growing. |
| `pkg/memory` | Experimental | Semantic-search options are not settled. |
| `pkg/skill` | Experimental | Follows the skill format, which is still evolving. |
| `pkg/mcp` | Experimental | Tracks the MCP spec. |

**Stable** means no identifier is removed or renamed without a deprecation
period, and a change to one is called out in the release notes.
**Experimental** means it may change in any release — though the API snapshot
still makes the change visible in review.

### Deprecation before removal

An identifier that is going away is marked first:

```go
// Deprecated: use NewFoo instead. Removed no earlier than the second
// minor release after this one.
func OldFoo() {}
```

It stays for at least two minor releases. Removal without that marking is a
mistake, not a policy choice.

### How a break is caught

`docs/public-api-surface.txt` lists every exported identifier, field, and
method in `pkg/*`. CI regenerates it and fails when it differs from the
checked-in copy, so a PR that changes the surface must carry the regenerated
file — the diff shows exactly which identifiers moved, in review, before the
merge.

```bash
make api-check     # fail if the snapshot is stale
make api-snapshot  # regenerate it
```

The snapshot cannot see a *signature* change, since the identifier's name is
unchanged. `pkg/apiconsumer_test.go` covers that gap: a compile-only consumer
that calls the main entry points the way an external caller would, so a
changed signature fails the build.

Neither mechanism prevents a change. Both make it a decision someone made,
rather than something a consumer discovers.

### Dependency weight is part of the contract

`pkg/agentloop` and `pkg/tools` must not pull the server's storage or
scheduling stack into a consumer's build. Concretely, these stay absent from
`go list -deps ./pkg/...`:

- `modernc.org/sqlite`
- `github.com/robfig/cron`
- `internal/workstore`, `internal/cron`, `internal/tarsserver`

This is enforced rather than merely documented: `internal/architecture/layers_test.go`
fails when anything under `pkg/` imports an app-layer package. Before the
`internal/tool` split, importing `pkg/agentloop` compiled an SQLite
implementation in order to run a tool loop — that is the regression the rule
exists to prevent.

### Known consumers

[**linetta**](https://github.com/devlikebear/linetta)'s desktop engine imports
`pkg/llm` and `pkg/session` and ships a Windows build, which is why
`make windows-build-check` cross-compiles the whole module.

It is named here so that whoever proposes a break to those two packages can
see what it costs. If you depend on these packages, opening an issue to be
added to this list is the cheapest way to be counted.
