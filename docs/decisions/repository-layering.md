# ADR: Enforce layering in-repo instead of splitting the repository

- Status: Accepted
- Decision date: 2026-08-28
- Scope: [LP-011 / #930](https://github.com/devlikebear/tars/issues/930), part of epic [#919](https://github.com/devlikebear/tars/issues/919)

## Context

The recurring proposal is to split TARS into `tars-app` / `tars-cli` / `tars-console` repositories. The motivation is real and worth stating plainly: keep the public library clean, and stop application changes from breaking external consumers.

The question is whether a repository split is the instrument that achieves it.

## What the evidence said

**The vertical boundary already held.** Before any of this epic's work, `go list -deps ./pkg/...` contained no `internal/tarsserver`. The server did not leak into the core in the import direction. A repository split enforces a boundary that was not being crossed.

**The real coupling was horizontal.** One oversized `internal/tool` package mixed generic primitives with TARS application machinery. Because Go dependencies are per-package, importing `pkg/agentloop` pulled 25 internal packages including an SQLite implementation and a cron parser. A repository split does not touch that; splitting the package does. [#927](https://github.com/devlikebear/tars/issues/927) did, and the numbers moved:

| Package | Before | After |
| --- | --- | --- |
| `pkg/tools` | 24 internal packages | 10 |
| `pkg/agentloop` | 25 | 11 |
| `pkg/mcp` | 25 | 12 |

**There is no separable desktop application.** No Tauri, no Electron. What gets described as the desktop app is `golang.design/x/hotkey`, `internal/tarsserver/notify.go`, and the same binary serving an embedded console. There is no third codebase to extract.

**The costs are concrete.** `frontend/console` is ~41k LOC embedded via `go:embed`; splitting it requires a new artifact or npm pipeline just to keep `make build` working. CI already runs security, windows-build, windows-test, pr-diff, three CodeQL analyses, and SonarCloud. For a single maintainer, every change crossing a new repository boundary becomes a multi-repo PR — and provider work routinely spans `internal/llm`, `internal/config`, `internal/tarsserver`, and `frontend/console` in one change.

**The problem a split really solves is API stability**, and [#928](https://github.com/devlikebear/tars/issues/928) plus [#929](https://github.com/devlikebear/tars/issues/929) address that directly and reversibly.

## Decision

Do not split the repository. Enforce the layering in-repo with a test, and keep the discipline a split would have imposed without paying its cost.

A layer rule is cheap to add and cheap to remove. A repository split is neither.

## Layers

```
cmd/          entry points
app layer     orchestration: server, schedulers, watchdogs, runtimes, TARS tools
core layer    primitives: llm, tool, session, memory, skill, mcp, prompt, agent
pkg/          public API surface
```

Imports point inward. The enforced rule is the reverse:

1. **No core package may import an app package.**
2. **No `pkg/*` package may import an app package.**
3. **Every `internal/` package must be classified** as core, app, or shared.

Membership lives in `internal/architecture/layers.go`; `internal/architecture/layers_test.go` enforces all three. Run it with `make arch-check`; it also runs as part of `make test`.

### Why lists rather than directories

The issue sketched `internal/app/*` and `internal/core/*` directories. Membership is a list instead, because moving ~60 packages would break every import path in the tree and bury the actual constraint inside a rename large enough that nobody would review it. The list is greppable, the test messages name both offending packages, and rule 3 means a newly added package fails the build until someone decides where it belongs — which is the drift protection a directory convention would have provided.

The `shared` category is for leaf utilities with no layer opinion. It is deliberately small; when in doubt a package belongs in core or app, because "shared" exempts it from rule 1.

## Consequences

- A reverse import fails `make test` and CI, naming both packages and pointing at the file to edit.
- Adding a package is a layering decision, made once, in one file.
- The public surface's dependency weight is now a property of the layering rather than an accident, so a regression in it is a test failure rather than something discovered by an external consumer.
- Reclassifying a package is a one-line change — deliberately easy, because the point is to make the layering *visible*, not to make it immovable. The diff shows the decision.

## Re-evaluation criteria

Revisit the repository split when **all three** hold:

1. **Two or more external consumers** depend on `pkg/*`. (Today there is one: linetta's desktop engine imports `pkg/llm` and `pkg/session`.)
2. **Two consecutive quarters with no breaking change** to `pkg/*`, measured against the API snapshot from #929.
3. **The release pipeline no longer needs the console embedded at build time** — that is what makes `frontend/console` separable without inventing an artifact pipeline.

Until all three hold, the split costs more than the layer rule and buys less.
