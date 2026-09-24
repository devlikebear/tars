# ADR: Turn the console into an agent workbench

- Status: Accepted
- Decision date: 2026-09-23
- Scope: epic [#967](https://github.com/devlikebear/tars/issues/967), recorded as the first deliverable of P0 [#968](https://github.com/devlikebear/tars/issues/968)
- Supersedes: the **freeze** half of [#931](https://github.com/devlikebear/tars/issues/931) and the nav freeze in `frontend/console/DESIGN.md`

## Context

The goal is for TARS to be the maintainer's **primary daily AI development tool**. The console that exists today was not built for that. It is an operator dashboard with a coding screen bolted on top, and the #931 freeze (2026-08) deliberately stopped it from growing. Research done on 2026-09-17, which read the code but did not change it, found the following:

- The chat header stacks 11 panel toggles, pulse stats, plan/workbench strips, and goal/health/cwd chips. Entry points are duplicated; Tasks alone has four.
- A dock area shows one panel at a time. Opening Git hides Tasks.
- File-edit tool results render as raw JSON. There is no inline diff and no accept/reject flow.
- Approvals live on the Ops page, so approving means leaving the chat.
- There is no model/tier picker, command palette, keyboard shortcut map, or parallel session view.
- Ten pages (Agent Runtime, Memory, Extensions, …) are reachable only by typing a URL.
- `Chat.svelte` is 2,167 lines with no shared session store. Most frontend tests are regex checks against `.svelte` source.

The backend is mostly already there: the PTY terminal WebSocket, git read/write APIs, filesystem routes, tool-call SSE events, session cwd, managed worktrees (`internal/executionplane`), LaunchAgent service install, and the global hotkey.

Industry tools (Claude Code Desktop, Codex app, Cursor, Zed, Conductor, Goose, OpenCode) have converged on the same patterns: a session board with status badges, session isolation, a diff review queue, plan/approval gates, a terminal panel, OS notifications, and keyboard-first control.

## Decisions

### 1. `claude-code-cli` is the primary provider, so diffs and approvals must not depend on the provider

This constrains P1–P3 more than anything else does. In a `claude-code-cli` session **the Claude Code CLI runs tools in its own process**. TARS's `pkg/tools` and `pkg/agentloop` are never called. In `pkg/llm/claude_code_cli.go`:

- stream-json parsing handles `system`, `assistant`, and `result` events. `tool_result` is not a case at all.
- `tool_use` blocks are scraped from assistant content into `ProviderExecutedTools` as observation-only records.
- `--permission-mode` is one setting for the whole process. It cannot pause an individual tool call.

A design that adds structured fields to tool results would therefore produce nothing for the primary provider. Instead:

- **Diffs are built from turn checkpoints (git snapshots).** They work the same no matter who edited the file. This is P1's first priority.
- **Approvals go through `claude -p --permission-prompt-tool`, backed by a TARS MCP tool.** The flag works in headless `--print` mode. TARS already injects a per-call `--mcp-config` file (`internal/tarsserver/claude_code_cli_mcp.go`), so the injection path exists. It is gated on a Go/No-Go spike in P2.
- **Native providers** (anthropic/openai/gemini/gemini-native) also get `pkg/agentloop` gates and structured `pkg/tools` results, **as a reinforcement path, not the default one**.
- `antigravity-cli` has the same constraint but no delegation flag. Its sessions get checkpoint diffs only; inline approval is out of scope.

The frontend must never branch on provider. Both the diff and the approval surfaces consume one event shape.

### 2. The #931 freeze is lifted; the #931 config policy stays

The freeze was the right call while the console was not a primary tool. That premise is gone. This ADR reverses:

- "No new console routes."
- The nav slimmed to chat, approvals, logs, pulse, and config.
- "No new investment" in the console surface.

This ADR does **not** reverse:

- The file-first configuration policy. `workspace/config/tars.config.yaml` is the source of truth, and `/console/config` stays Quick Start only.
- The credential handling rules (masked, never echoed; long-tail tokens rotated in YAML).
- The rule that a feature which does not need a browser belongs in a skill, a CLI command, or the YAML file. The workbench adds routes for **working with agents**, not for configuration.

The frozen long-tail internal packages (embodiment, a2a, workstore, workscheduler, skillhub, plugin, remoteaccess) remain frozen. This ADR only concerns the console surface.

### 3. Information architecture

The console becomes a workbench:

- **Home is a session board** (P3). It groups sessions by repo (the git toplevel of the active cwd) and shows status badges: `needs input`, `running`, `done·unread`, `idle`. The current Home dashboard moves into System.
- **Three-column working layout.** Session sidebar, then the conversation, then a dock that can show more than one panel at a time (Changes, Terminal, Git, Tasks, Files).
- **The nav has three groups:**

  | Group | Routes |
  |---|---|
  | Work | Sessions (board), Chat |
  | Build | Agent Runtime, Memory, Extensions, Sysprompt |
  | System | Ops, Pulse, Reflection, Cron, Logs, Analytics |

  Config and Onboarding stay reachable from System and the palette. Channels, lineage, and tasks remain routes, reached through the palette and in-context links.
- **`⌘K` command palette** reaches every page (hidden ones included) in at most two keystrokes. It also toggles panels, searches and switches sessions, and runs slash commands.
- **Approvals and diffs live inside the conversation.** Changing files or approving a tool call must never require leaving the chat.

### 4. Worktrees are hybrid

- A foreground session works in its current directory and takes the single **write lease** for its repo.
- A session gets an **automatic worktree** when another session already holds that repo's lease, or when the run is unattended (cron, worker, goal auto-continue).
- The session header has a manual "isolate" toggle. `.tars/settings.json` lists the gitignored files to copy into a worktree.
- **Every turn leaves a checkpoint, whether or not the session is isolated.** The snapshot is taken with a temporary `GIT_INDEX_FILE` and stored at `refs/tars/checkpoints/<session>/<turn>`. The user's index, stash, and HEAD are never touched.

Why not isolate every session: always-on worktrees drop gitignored files (`.env`, `node_modules`, `.tars/settings.local.json`), collide on ports and caches, add a merge step, and mean nothing for non-coding sessions. A single user mostly works in one main session.

### 5. The desktop app is a thin shell, built after the workbench

- The server stays a LaunchAgent (`tars service`), so cron, pulse, and reflection keep running when the window closes.
- The shell attaches to `http://127.0.0.1:<port>/console`. The origin is the same, so CORS, cookie auth, and the terminal WebSocket origin check need no changes. The codebase has no CORS handling today.
- Wails v3 is first choice, gated on a spike. Electron is the fallback. Tauri is excluded: its sidecar and Rust costs buy nothing here.
- All UI logic stays in the Svelte console. Shell-only features (tray, global hotkey, notifications with action buttons, deep links, auto-update) are exposed through a narrow bridge. This keeps the #854 rule: no console fork.
- **Sequencing (decided 2026-09-23):** P4 starts **after P0–P3**, and no Wails spike runs in parallel. A shell is only as good as the console it wraps, and P0–P3 work unchanged in a browser in the meantime. This overrides the "spike can run in parallel" note in #972.

### 6. Windows scope is decided per phase, before the phase starts

P1 checkpoints and P3 worktrees depend on git plumbing and on `internal/executionplane`. That package is excluded wholesale from `scripts/windows_test.sh` ("artifact URIs, symlinks, and POSIX runner assumptions"), and that exclusion list is debt, not policy.

Before starting P1 or P3, the phase picks one of these and records the choice in its issue:

- (a) support Windows for that feature path;
- (b) degrade gracefully on Windows, with a `tars doctor` warning;
- (c) shrink the exclusion list first, as a prerequisite.

No phase may add to the exclusion list.

## Phases

| Phase | Issue | Delivers |
|---|---|---|
| P0 | #968 | This ADR, DESIGN.md revision, shared session store, `Chat.svelte` split, Playwright E2E baseline, `⌘K` palette and shortcuts, header cleanup, composer status bar, multi-panel dock, nav restructure |
| P1 | #969 | Turn checkpoints, checkpoint diff/revert API, Changes panel, hunk comments; structured tool diffs for native providers |
| P2 | #970 | Inline approvals (`--permission-prompt-tool` first, `agentloop` gate for native providers), per-session permission modes |
| P3 | #971 | Session board home, write lease, hybrid worktrees, background streams, message queue, notifications |
| P4 | #972 | Thin desktop shell |

P0 shipped in #977–#984, #987, and #988. On a fresh session after one exchange, the controls stacked above the conversation went from 15 to 3 ([before](../screenshots/p0-chat-before.webp), [after](../screenshots/p0-chat-after.webp)). Panels moved to the rail and `⌘K`, and tier, cwd, and cost moved to the composer status bar. The dock stacks panels as tabs; a split inside one zone was not built.

## Consequences

- `frontend/console/DESIGN.md` is the design source of truth. Its freeze section now points here, and it carries the palette and shortcut rules.
- The nav follows this document since #982: `lib/navGroups.ts` derives the Work, Build, and System groups from the palette's page table, and `tests/navGroups.test.ts` checks them.
- New console routes are allowed again. Each one must still serve working with agents, and each one must be reachable from the palette.
- The frontend's regex-over-source tests are not enough for a workbench. P0 adds a Playwright E2E baseline, and later phases extend it rather than adding more source regexes.

## Re-evaluate if

- The primary provider moves off `claude-code-cli`. Decision 1's ordering (checkpoints before structured diffs, `--permission-prompt-tool` before `agentloop` gates) would then flip.
- The P2 `--permission-prompt-tool` spike is No-Go. `claude-code-cli` sessions would then get pre-set policy UI only (`--permission-mode` plus deny rules), and inline approval would narrow to native providers.
- Always-on worktrees become cheap. Copying gitignored files and avoiding port collisions would have to be solved generically first.
