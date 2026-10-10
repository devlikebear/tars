# ADR: The product is the focus pipeline and goal mode

- Status: Accepted
- Decision date: 2026-10-10
- Scope: [#1204](https://github.com/devlikebear/tars/issues/1204) item 1. Items 2 (`internal/tarsserver` split) and 3 (release cadence) follow from this record.

## Context

Eight months in, TARS covers a chat server, seven LLM providers, cron, pulse, reflection, initiative, computer use, the focus pipeline, session worktrees, a desktop shell, self-update, Homebrew, winget and onboarding. One maintainer wrote all of it.

The repository's own numbers on 2026-10-10 say the breadth has a cost:

| Measure | Value |
| --- | --- |
| Go source / Go tests | 144k / 130k lines |
| `internal/tarsserver` | 42k lines, 29% of Go source |
| `internal/` packages | 59 |
| Last 200 commits | 80 `fix`, 61 `feat`, 6 `refactor` |
| Releases on 2026-10-08 and 10-09 | 5 (v0.57.0 → v0.59.0) |
| Stars / forks / outside pull requests | 4 / 0 / 0 |

Fixes outnumber features, and patch releases follow minor releases within hours. Features ship, then dogfooding finds the regressions. The test discipline is not the problem: tests are 90% of the source by line count and changed lines are gated at 80% coverage. The surface is simply wider than one person can keep correct.

There is also a positioning problem. Much of the chat workbench wraps a coding CLI, and the coding tools themselves now ship a desktop app, worktrees, background sessions and permission modes. Work that re-implements those is work a larger team does anyway.

Two areas do not overlap with that:

1. **The focus pipeline and goal mode** — a task runs plan → build → review → PR → PR review → merge → release, with transitions decided by facts the server reads itself (plan approval, verify exit codes, `gh` probes) and never by asking a model whether it is done. Goal mode walks the same gates with nobody present, on a fixed policy and a push budget. It works across providers.
2. **Unattended operation** — pulse, cron and the ops approval queue.

## Decision

**The focus pipeline and goal mode are the product.** Everything else is judged by whether a pipeline needs it.

Reasons for choosing it over unattended operation:

- It is the part the maintainer uses to build TARS itself, so it gets exercised every day and its failures are noticed.
- It is opinionated in a way a general chat tool is not: stages, gates and server-read facts. That is hard to get from a wrapper and is where the design work in `focus-mode.md` already went.
- It is provider-neutral. The same pipeline runs on a CLI provider or a native one, so it does not depend on one vendor's product decisions.
- It already contains the useful half of unattended operation. Goal mode is a pipeline with no one at the gates, and it reuses the unattended approval queue.
- It can be shown to someone else in one sentence and one screen, which the project needs more than another feature.

## Classification

Three classes. The class decides what kind of change an area accepts.

| Class | Accepts |
| --- | --- |
| **Core** | Features, refactors, fixes |
| **Maintained** | Fixes, and changes a core feature needs. No features of its own |
| **Frozen** | Fixes for data loss, security, or failure to start. Nothing else |

### Core

- `internal/focuspipeline`, the focus handlers in `internal/tarsserver` (`focus_*.go`), templates, goal mode, PR probing, the release train
- Verification: `verify` commands and their results
- What a pipeline turn stands on: sessions, background turns and the turn feed, session worktrees, checkpoints, permission modes, the unattended approval queue
- The provider layer a pipeline runs through: the router and tiers, `claude-code-cli`, the native `anthropic` / `openai` / `gemini` providers, usage and cost tracking
- The focus screens of the console, and the `tars-desktop` tray and notifications that surface a pipeline waiting on its developer

### Maintained

- The Advanced workbench (dock, panels, session config)
- cron, pulse on its Go rules, ops cleanup
- Memory, skills, MCP client, plugins
- Onboarding, `tars doctor`, config
- Distribution: releases, Homebrew, winget, `tars update`, the desktop self-updater
- Session retention, Telegram

### Frozen

- Initiative (epic #997): P2–P5 (#1000–#1003) stop here
- Companion (epic #1188): P3–P5 (#1191–#1193) finish as already scoped, with no added scope, and the area is frozen when the epic closes. It is mid-flight and half a face is worse than either end state
- Computer use, including the opt-in focus e2e that runs through it
- Reflection
- `antigravity-cli`, `gemini-native`, `openai-codex`, and the Jev / System One client
- Skill hub, the MCP server creator
- pulse's `decider: llm` path

Frozen does not mean removed. Nothing is deleted by this record. An area that stays frozen through a re-evaluation becomes a candidate for removal, decided then.

## Rules that follow

1. A `feat:` pull request outside Core needs a sentence saying which pipeline behaviour requires it. Without one it waits.
2. New `internal/` packages are Core or they are not added. `internal/architecture/layers.go` already forces a decision per package; this adds a second question to it.
3. The README opens with the pipeline: what a task looks like from plan to merge, then goal mode. The rest moves below it.
4. The `tarsserver` split (#1204 item 2) goes by measured coupling, not by which code changes most, and it is **paused as of 2026-10-11**. This rule first said "start with the focus handlers"; a dependency map of the package (in a comment on #1204) showed that to be the wrong order. What moved: the focus files that read git and `gh` (`internal/focusprobe`), the JSON request helpers (`internal/httpapi`), notifications (`internal/notification`) and six route groups that need nothing from the server (`internal/apihandlers`). `tarsserver` went from about 42,300 lines to 38,800. What is left is of two kinds, and neither is mechanical: five leaf groups that each share a piece with the server (a chat heuristic, the cron store resolver, test helpers, the auth middleware, the task verifier), and the core — chat, focus, Telegram, sessions, about 20,000 lines that call each other. The leaves that moved were not tangled to begin with, so moving more of them does little for the fix-to-feature ratio this record cares about. Resume the split when a change to the core is blocked by that coupling, and start it by writing down the interface between chat and focus, not by moving files.
5. The freeze runs to **2026-12-31**, then this record is re-evaluated.

## Consequences

- Open work is cut. #1000–#1003 are paused, and anything proposed for a frozen area is declined until re-evaluation.
- Some finished work stops improving. Computer use and initiative took real effort; they keep working and get no further investment.
- Bug reports against frozen areas below the bar (data loss, security, failure to start) stay open and unfixed. That is the intended trade.
- The fix-to-feature ratio should fall, because features land in fewer places. If it does not, the surface was not the cause and the split in item 2 matters more than this record assumed.
- Provider coverage narrows in practice. The frozen providers will drift as their CLIs and APIs change; when one breaks, the fix is to mark it unsupported, not to chase it.

## Re-evaluation criteria

Review on 2026-12-31, or earlier if either failure condition is already clear.

**The decision held if:**

1. At least three people other than the maintainer have run a pipeline from plan to a merged pull request on their own repository.
2. Over the freeze, `feat` commits are at least equal to `fix` commits.
3. No release was cut outside the cadence set under #1204 item 3 except for the reasons that cadence allows.

**Reconsider the choice if:**

1. Nobody outside has completed a pipeline by the review date. Then the limit is reach or the idea, not scope, and more pipeline features will not help.
2. A coding tool the pipeline runs on ships the same staged, gated flow natively. Then the remaining difference is provider neutrality alone, and that has to be enough on its own to continue.

**Unfreeze an area when** a pipeline feature needs it, stated in the pull request that does it. An area is not unfrozen because it would be nice to finish.
