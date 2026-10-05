# Console menu review (2026-10)

- Status: in progress
- Date: 2026-10-05
- Builds on: [`console-workbench.md`](console-workbench.md) (the console is the maintainer's daily AI development tool)

## Why

Every page of the console was opened on the maintainer's own server (0.47.6, one user, `openai-codex` and `claude-code-cli` providers, 28 sessions) and judged against one question: does this page help someone working with agents every day, on the data this server actually has? Each page gets one verdict: **keep**, **improve**, **shrink** or **remove**.

The verdicts lean on what was on screen, not on what the page could show in another setup. Where a page is empty because this server does not use the feature (cron jobs, skills, MCP servers), that is noted and the page is kept: the feature is the extension path, not dead weight.

## Verdicts

| Page | Route | Verdict | What was on screen |
|---|---|---|---|
| Session board | `/console` | keep | The home of the workbench. |
| Focus | `/console/focus` | keep | One pipeline, in use. |
| Chat | `/console/chat` | keep | Daily surface. Its dock panels are not part of this review. |
| Agent Runtime | `/console/agentruntime` | improve, then shrink | Run list is useful. Help text still called `subagents_orchestrate` opt-in. Four views (list, tree, Gantt, flow) and two cost cards that read "—" for every run on subscription providers. |
| Memory | `/console/memory` | improve | The inbox held ten candidates and every one was an agent's own completion report ("Verification complete", "Plan and contract ready"). |
| Extensions | `/console/extensions` | keep | Empty here (no skills, no MCP servers). Skills are the extension path. |
| System prompt | `/console/sysprompt` | keep | Four small files, clear. |
| Overview (Mission Control) | `/console/system` | shrink | Repeats the board, Plans, Agent Runtime and Cron as five lists. Showed "5 active plans" that were plans of sessions that ended a day ago. |
| Approvals | `/console/approvals` | improve | The subtitle described cleanup plans; the page is mostly the history of tool calls that waited for an answer. |
| Pulse | `/console/pulse` | shrink | Status and the last decision are a few lines; most of the page is a static description of thresholds. |
| Reflection | `/console/reflection` | keep | Small, accurate. |
| Cron | `/console/cron` | keep | Empty here. |
| Logs | `/console/logs` | improve | Opened at DEBUG: a page of `http request started`. The level filter matched one level only, so WARN hid the errors. |
| Analytics | `/console/analytics` | keep | Reworked in 0.45.x. |
| Config | `/console/config` | keep | Quick Start only, as decided in #931. |
| Plans | `/console/tasks` (palette) | improve | Lists plans stuck in `executing` after their session stopped. |
| Session lineage | `/console/sessions/graph` (palette) | **remove** | 30 sessions, 30 roots, 0 forks. Frozen out of the nav since #931. |
| Channels | `/console/channels` (palette) | keep | Telegram pairing; small. |

## Plan

### Done in this change

1. **Remove the session lineage page.** The route, component, `lib/sessionLineage.ts`, its strings and test are gone. Forking a chat from a message stays; only the graph of forks is removed. The palette e2e that jumped to it now jumps to Plans.
2. **Logs open at INFO, and a level is a floor.** `GET /v1/admin/logs?level=warn` returns warnings and errors. The page's default level is INFO.
3. **Agent Runtime help text** no longer calls `subagents_orchestrate` opt-in: with the durable scheduler on it is offered by default.
4. **Approvals subtitle** says what the page shows.

### Next, in this order

5. **Shrink Mission Control.** Keep the status strip (Pulse, Reflection, disk, counts) and the notification feed; drop the five lists that have their own page (active plans, agent runs, cron jobs, sessions, "continue working"). The loading for them goes with them.
6. **Plans that outlive their session.** A plan stays `executing` when its session just stops. Decide the rule (for example: no turn for 24 hours and no running turn means stale) and show such plans as stale on the Plans page and the Mission Control count, without changing the plan itself.
7. **Memory inbox quality.** Reflection proposes an agent's completion reports as memories. Filter candidates whose source is the assistant's own status report before they reach the inbox.
8. **Shrink Pulse.** Move the threshold reference behind a disclosure and lead with status, last decisions and signals.
9. **Agent Runtime views.** Measure which of tree, Gantt and flow is opened; remove the ones that are not, and hide the cost cards when no run has a cost.

### Left alone on purpose

- Extensions, Cron and Channels are empty on this server and kept: they are how the product is extended and reached.
- The chat dock (tasks, changes, git, terminal, artifacts, session config, session cron, health, context) needs its own pass. It is the densest surface and the one used most.

## Found on the way

- Runtime logging writes whole LLM request payloads at DEBUG; the runtime log on this server was 58 MB.
- The fork-insight endpoints the lineage page called (review and queue to the memory inbox) no longer have a console caller.
