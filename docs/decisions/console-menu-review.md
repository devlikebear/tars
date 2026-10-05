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
| Overview (Mission Control) | `/console/system` | shrink (done) | Repeats the board, Plans, Agent Runtime and Cron as five lists. Showed "5 active plans" that were plans of sessions that ended a day ago. |
| Approvals | `/console/approvals` | improve | The subtitle described cleanup plans; the page is mostly the history of tool calls that waited for an answer. |
| Pulse | `/console/pulse` | shrink (done) | Status and the last decision are a few lines; most of the page is a static description of thresholds. |
| Reflection | `/console/reflection` | keep | Small, accurate. |
| Cron | `/console/cron` | keep | Empty here. |
| Logs | `/console/logs` | improve | Opened at DEBUG: a page of `http request started`. The level filter matched one level only, so WARN hid the errors. |
| Analytics | `/console/analytics` | keep | Reworked in 0.45.x. |
| Config | `/console/config` | keep | Quick Start only, as decided in #931. |
| Plans | `/console/tasks` (palette) | improve | Lists plans stuck in `executing` after their session stopped. |
| Session lineage | `/console/sessions/graph` (palette) | **remove** (done) | 30 sessions, 30 roots, 0 forks. Frozen out of the nav since #931. |
| Channels | `/console/channels` (palette) | keep | Telegram pairing; small. |

## Plan

### Done in this change

1. **Remove the session lineage page.** The route, component, `lib/sessionLineage.ts`, its strings and test are gone. Forking a chat from a message stays; only the graph of forks is removed. The palette e2e that jumped to it now jumps to Plans.
2. **Logs open at INFO, and a level is a floor.** `GET /v1/admin/logs?level=warn` returns warnings and errors. The page's default level is INFO.
3. **Agent Runtime help text** no longer calls `subagents_orchestrate` opt-in: with the durable scheduler on it is offered by default.
4. **Approvals subtitle** says what the page shows.
5. **Shrink Mission Control.** Keep the status strip (Pulse, Reflection, disk, counts), notifications, recommendations, and delivery; remove the five duplicate lists (active plans, agent runs, cron jobs, sessions, "continue working") and their dedicated code and translations. Count-loading calls and tile links stay; per-session task loading is gone.

6. **Plans that outlive their session are marked stalled.** A plan with pending or in-progress tasks that nothing has updated for 24 hours gets a "Stalled" badge, its own summary card and filter on the Plans page. The plan itself is not changed. On the reviewed server two of the five "active" plans were stalled.
7. **Pulse leads with status.** The two blocks of reference text (what Pulse watches, the severity thresholds) are collapsed; status, last seen per signal, the last decision and recent ticks come first.

### Next, in this order

1. **Memory inbox quality.** Reflection proposes an agent's completion reports as memories. Fixed separately (#1140): the assistant's reply is no longer mined for candidates.
2. **Mission Control's "active plans" tile** still counts stalled plans; it should show them apart, as the Plans page does.
3. **Agent Runtime views.** Measure which of tree, Gantt and flow is opened; remove the ones that are not, and hide the cost cards when no run has a cost.
4. **The chat dock.** Reviewed; see "Chat dock" below.

### Left alone on purpose

- Extensions, Cron and Channels are empty on this server and kept: they are how the product is extended and reached.

## Chat dock

All fourteen dock panels were opened on a session that had done real work (83 messages, 45 changed files, its worktree since kept).

| Panel | Verdict | Why |
|-------|---------|-----|
| Session list | improve | "All" listed every subagent worker session next to the chats: 13 of 32 rows on this server. |
| Session health | improve | Said "action required" on a healthy session. See below. |
| Changes | improve | "Could not load changes" for every turn once the session's worktree had ended. |
| Side session | improve | Its picker offered the same worker sessions. |
| Tasks, context, prior context, prompt, session config, git, files, terminal | keep | Each showed what it claims to and nothing else on the page does. |
| Skill inbox, session cron | keep | Empty here; they stay for the same reason Extensions and Cron do. |

Done:

1. **Session health follows what can go wrong.** Context length is judged by the share of the window in use when that is known (83 messages at 5.7% is not long); the message count is the fallback. High-risk tools are on by default and the permission mode decides whether they run unasked, so a native-provider session is flagged only in auto mode, and as a warning.
2. **A turn's changes stay readable after its worktree is gone.** Diffs between two checkpoint commits ran git inside the checkpointed folder; they run in the shadow repository now, which is where the commits are.
3. **"All" is the chats.** Worker sessions are under their own filter and out of the side-session picker.

Checked and left: the files panel showing one age for every file (a fresh worktree's files do share a modification time), and the git panel showing the original checkout for a session whose worktree ended (that is the folder the session is in again).

## Found on the way

- A server with no `log.level` ran at DEBUG, not the documented INFO, and logged every request and LLM payload: 58 MB of runtime log and a 264 MB service log on this server. Fixed separately (#1141).
- The fork-insight endpoints the lineage page called (review and queue to the memory inbox) no longer have a console caller.
