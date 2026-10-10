---
version: alpha
name: Graphite Signal
description: TARS Console — cool graphite base, one signal-green accent, engineering type, tight corners for a tool you work in all day.
colors:
  # Brand
  primary: "#3ee07f"
  primary-hover: "#2fc56b"
  primary-muted: "#192f26"
  primary-text: "#7cf0a8"
  primary-contrast: "#06140c"

  # Surface
  surface-base: "#0d0f11"
  surface: "#14171a"
  surface-elevated: "#1b1f23"
  surface-hover: "#21262b"
  surface-active: "#282e34"
  surface-inset: "#0a0c0e"

  # Border
  border-subtle: "#1f2428"
  border-default: "#2b3137"
  border-strong: "#3a4148"

  # Text
  text-primary: "#e6e9ec"
  text-secondary: "#9aa4ad"
  text-tertiary: "#6b757e"
  text-ghost: "#454d55"

  # Semantic
  success: "#3fd4b4"
  success-muted: "#192e2c"
  warning: "#f5c542"
  warning-muted: "#2f2c1f"
  error: "#ff5d5d"
  error-muted: "#2c1e21"
  info: "#5cb8ff"
  info-muted: "#1b2731"

typography:
  h1:
    fontFamily: IBM Plex Sans
    fontSize: 1.75rem
    fontWeight: 600
    lineHeight: 1.25
    letterSpacing: -0.01em
  h2:
    fontFamily: IBM Plex Sans
    fontSize: 1.375rem
    fontWeight: 600
    lineHeight: 1.25
    letterSpacing: -0.01em
  h3:
    fontFamily: IBM Plex Sans
    fontSize: 1.125rem
    fontWeight: 600
    lineHeight: 1.25
  h4:
    fontFamily: IBM Plex Sans
    fontSize: 1rem
    fontWeight: 600
    lineHeight: 1.25
  body-md:
    fontFamily: IBM Plex Sans
    fontSize: 0.875rem
    fontWeight: 400
    lineHeight: 1.55
  body-sm:
    fontFamily: IBM Plex Sans
    fontSize: 0.8125rem
    fontWeight: 400
    lineHeight: 1.5
  label-caps:
    fontFamily: IBM Plex Mono
    fontSize: 0.6875rem
    fontWeight: 500
    lineHeight: 1.5
    letterSpacing: 0.06em
  code:
    fontFamily: IBM Plex Mono
    fontSize: 0.875rem
    fontWeight: 400
    lineHeight: 1.5

rounded:
  sm: 2px
  md: 4px
  lg: 6px

spacing:
  xs: 4px
  sm: 8px
  md: 12px
  lg: 16px
  xl: 20px
  "2xl": 24px
  "3xl": 32px
  "4xl": 40px

components:
  card:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.lg}"
    padding: "{spacing.xl}"

  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.primary-contrast}"
    typography: "{typography.label-caps}"
    rounded: "{rounded.md}"
    padding: "{spacing.sm}"
  button-primary-hover:
    backgroundColor: "{colors.primary-hover}"

  button-secondary:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.text-primary}"
    typography: "{typography.label-caps}"
    rounded: "{rounded.md}"
    padding: "{spacing.sm}"
  button-secondary-hover:
    backgroundColor: "{colors.surface-elevated}"

  button-ghost:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.text-secondary}"
    typography: "{typography.label-caps}"
    rounded: "{rounded.md}"
    padding: "{spacing.sm}"
  button-ghost-hover:
    backgroundColor: "{colors.surface-elevated}"
    textColor: "{colors.text-primary}"

  button-danger:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.error}"
    typography: "{typography.label-caps}"
    rounded: "{rounded.md}"
    padding: "{spacing.sm}"
  button-danger-hover:
    backgroundColor: "{colors.error-muted}"

  button-warning:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.warning}"
    typography: "{typography.label-caps}"
    rounded: "{rounded.md}"
    padding: "{spacing.sm}"
  button-warning-hover:
    backgroundColor: "{colors.warning-muted}"

  button-sm:
    padding: "{spacing.xs}"

  badge-default:
    backgroundColor: "{colors.surface-elevated}"
    textColor: "{colors.text-secondary}"
    typography: "{typography.label-caps}"
    rounded: "{rounded.sm}"
    padding: "{spacing.xs}"
  badge-accent:
    backgroundColor: "{colors.primary-muted}"
    textColor: "{colors.primary-text}"
    typography: "{typography.label-caps}"
    rounded: "{rounded.sm}"
    padding: "{spacing.xs}"
  badge-success:
    backgroundColor: "{colors.success-muted}"
    textColor: "{colors.success}"
    typography: "{typography.label-caps}"
    rounded: "{rounded.sm}"
    padding: "{spacing.xs}"
  badge-warning:
    backgroundColor: "{colors.warning-muted}"
    textColor: "{colors.warning}"
    typography: "{typography.label-caps}"
    rounded: "{rounded.sm}"
    padding: "{spacing.xs}"
  badge-error:
    backgroundColor: "{colors.error-muted}"
    textColor: "{colors.error}"
    typography: "{typography.label-caps}"
    rounded: "{rounded.sm}"
    padding: "{spacing.xs}"
  badge-info:
    backgroundColor: "{colors.info-muted}"
    textColor: "{colors.info}"
    typography: "{typography.label-caps}"
    rounded: "{rounded.sm}"
    padding: "{spacing.xs}"

  input:
    backgroundColor: "{colors.surface-inset}"
    textColor: "{colors.text-primary}"
    typography: "{typography.body-md}"
    rounded: "{rounded.md}"
    padding: "{spacing.md}"
  input-focus:
    backgroundColor: "{colors.surface-inset}"
    textColor: "{colors.text-primary}"

  empty-state:
    textColor: "{colors.text-tertiary}"
    typography: "{typography.body-sm}"
    padding: "{spacing.3xl}"

  error-banner:
    backgroundColor: "{colors.error-muted}"
    textColor: "{colors.error}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.md}"
    padding: "{spacing.md}"
---

# TARS Console — Graphite Signal

A canonical specification of the TARS Console design system. The YAML front matter is the normative source of truth for tokens; the prose below explains intent and application.

## Overview

The TARS Console is the maintainer's primary AI development workbench, and it is also the operator surface for the runtime behind it. It is where you run and review agent sessions, and where you read system signals, edit memory, and run jobs. The workbench direction is recorded in [`docs/decisions/console-workbench.md`](../../docs/decisions/console-workbench.md) (#967). It should feel like a well-kept instrument: dark, exact, quiet until something needs you. Not a chat product and not a SaaS dashboard, closer to the editors and terminals it sits next to.

The aesthetic resists three temptations:

- **Decoration for its own sake.** Borders are thin, shadows are absent, accents are rationed. Visual noise distracts from signal.
- **Assistant-product styling.** Warm terracotta or amber accents on beige-tinted greys, soft rounded humanist type, and roomy chat bubbles read as a consumer AI chat app. This is a development tool: cool graphite neutrals, one signal-green accent, and engineering type (IBM Plex). Until 2026-09 the console used exactly that warm styling ("Warm Workshop"); the maintainer retired it because it looked like Claude.
- **Density theatre.** Spacing is generous because most surfaces (memory, sessions, pulse) involve reading prose, not packing rows.

The system is dark-first. There is no light theme; the green accent and the tonal graphite steps are tuned for dark backgrounds.

## Console Purpose & Surface Policy (#931)

Decision recorded in the console-narrowing first cut (`refactor/console-narrow-surface`, Refs #931, Part of epic #919 LP-012). The surface policy is still normative, with one change: working with agents is now a first-class pillar (see below). When a proposed feature does not fit one of the pillars below, it belongs in a skill, a CLI command, or the YAML file, not a new console route.

### Freeze — superseded (2026-09, #967)

The 2026-08 freeze is lifted by [`docs/decisions/console-workbench.md`](../../docs/decisions/console-workbench.md). What changed:

- **Reversed:** the nav slimmed to chat, approvals, logs, pulse, and config; "no new console routes"; no new investment in the console surface. The workbench IA below replaces them.
- **Kept:** `/console/config` stays Quick Start checks only. The Inspect (165-field) pane and the YAML read view stay removed. The file-first config policy and the credential rules below are unchanged.
- **Kept:** the long-tail internal packages (embodiment, workstore, workscheduler, skillhub, plugin, remoteaccess) stay frozen. The workbench decision concerns the console surface only.
- **Outdated line, removed:** the freeze listed #930 layering as not being implemented. It has since landed; see `docs/decisions/repository-layering.md` and `make arch-check`.

The nav now follows the Work · Build · System groups below (`lib/navGroups.ts`, tested by `tests/navGroups.test.ts`).

### Workbench IA (#967 — target)

This describes the target. Each item names the phase that delivers it.

- **Home is the session board** (P3, #971, shipped). `/console` shows every visible chat session grouped by repository (the git top level of the active cwd; sessions still in their own artifact folder form a last "No working folder" group) with a status badge: `needs input`, `running`, `done · unread`, or `idle`. The dashboard that used to be home is **Overview** at `/console/system`, first in System. See Components → Session board.
- **Three-column working layout** (P0). The session sidebar, the conversation, and a dock that shows more than one panel at once for Changes, Terminal, Git, Tasks, and Files. Tabs shipped (see Dock tabs below). A vertical split inside one zone was not built, because the three zones already show three panels side by side.
- **Nav groups** (P0, shipped):

  | Group | Items |
  |---|---|
  | Work | Sessions (the board), Chat |
  | Build | Agent Runtime, Memory, Extensions, System Prompt |
  | System | Overview, Approvals, Pulse, Reflection, Cron, Logs, Analytics, Settings |

- **Diffs and approvals stay in the conversation** (P1 #969, P2 #970). Reviewing a change or approving a tool call never requires leaving the chat. The UI does not branch on provider; one event shape serves every provider. Shipped so far: turn checkpoints for every provider, the Changes panel with revert and undo, the change card under each turn (see Components), and approval cards for tool calls — Claude Code's own prompts on `claude-code-cli`, and high-risk TARS tools (`exec`, file writes and edits) on native providers, and review notes that carry comments and reverts back to the agent (see Components).

  Nav paths and role gating come from the palette's page table (`lib/commands.ts`), so the nav, the palette, and `App.svelte`'s gating cannot drift apart. The `user` role sees only the pages it can open (Sessions, Chat, Agent Runtime, Memory, System Prompt, Overview), and groups left empty are dropped. The active item is the one whose view the router resolves for the current path, so aliases such as `/console/ops` highlight Approvals. Lineage, Plans, Channels, and the setup wizard keep their routes and are reached through `⌘K` and in-context links.
- **Chat header budget** (P0, shipped). The header above the conversation now holds the title, health and goal chips, the session menu, and one **work strip** (plan progress on the left, workbench jumps on the right; hidden when both are empty). The pulse mini-dashboard is gone; Pulse has its own page.
- **Panel rail** (P0, shipped, `ChatRail.svelte`). The eleven text toggles became a 44px icon rail on the workbench's right edge: one icon per dock panel with an `aria-label` and tooltip, count badges for files, tasks, and health issues, and the active panel marked like the palette's selected row (`surface-active` plus a 2px `primary` edge). `⌘K` leads the rail, and the palette reaches everything the rail omits. The rail's width is the `--chat-rail-width` token. On the workbench the floating companion pet (CASE, `CompanionPet.svelte`) sits just left of the rail (`.beside-rail`), never over it: its bubble opens by itself on a new approval wait or a new failure and closes itself again once that is resolved, and would otherwise cover the rail's lower icons (and any other dock panel) while it is open. The session the chat route has on screen leaves its own pending approval and running turn out of CASE's lines — its thread already shows them — so CASE never pops open over the very panel the user is using in that session. Below 900px the rail becomes a scrolling row above the chat.
- **Dock tabs** (P0, shipped, `lib/dock/layout.ts`). A zone keeps every panel opened in it as a tab instead of replacing one panel with the next, so opening Git no longer closes Tasks. With two or more tabs, the panel header shows them as a strip in place of the title. The top tab takes the active-tab border (Elevation rule 3), and each closeable tab has its own ×. A rail icon or palette entry brings a covered tab to the front and closes only a panel that is already on screen, so the rail marks what is visible, not what is open. Tabs and the top tab of each zone survive a reload, except the terminal: its shells end with the page, so it is not restored. A covered terminal stays mounted under `visibility: hidden`, which keeps its shell running and its size unchanged.
- **Composer status bar** (P0, shipped, `ChatStatusBar.svelte`), in mono `text-xs`:
  - **Tier**: Auto, or a pinned `heavy`/`standard`/`light`. A pin is per session and rides on every turn as `tier_recommendation.chosen_tier` with `source: user`, because the server accepts a per-turn tier only through that field. Custom tiers show disabled until the server can take them. After a turn the bar shows which tier and model served it (`context_info.llm_tier`/`llm_model`).
  - **Permissions** (#970): a select per session with `Default (…)`, `Ask`, `Accept edits`, `Plan`, `Auto`, saved through `PUT /v1/admin/sessions/{id}/permission-mode`. `Default` names what the session inherits for its provider: `Ask` on native providers, the `.tars` override or `llm.claude_code_cli.permission_mode` on `claude-code-cli`. `Plan` takes an `info` tint and `Auto` a `warning` tint so the modes that change what runs stand out. On `antigravity-cli` the select is replaced by a dim `CLI policy` chip, because that CLI's own settings decide. `⇧Tab` in the composer steps Ask → Accept edits → Plan → Auto. A mode change from the server (plan approval) arrives as a `permission_mode` event and updates the select without a reload.
  - **cwd**: the chip moved here from the header; its dropdown opens upward.
  - **Cost**: this session's usage this month from `GET /v1/usage/summary?session_id=` (the usage tracker's estimate), with total tokens.

- **Background turns** (#971). Leaving a session does not stop its turn. Opening a session whose turn is still running attaches to it: the reply bubble, tool cards and any open approval card are rebuilt from the turn's events, the composer shows Stop, and the turn finishes in place. Nothing marks a replayed turn as different from a live one. Stop is the only control that ends a turn.

- **Side session** (#971, `SideSessionPanel.svelte`, rail icon `◫`, dock panel `side`, right zone by default). A second session next to the active one, so two can be worked without switching. The head is a session select (styled like the tier select, on `surface-inset`), a ghost `Open` that makes it the main session, and ×. The log is compact: user messages right-aligned on `surface-active`, replies as markdown on `surface-inset`, one dim mono `⚙ tool` line per tool call, and the same approval cards as the main thread. It follows the side session's running turn through the turn feed and re-reads the history when it ends. The composer is a textarea with `Send` under it on the left, because the companion pet floats over the panel's bottom-right corner. The chosen session is remembered in `localStorage`; the side never shows the active session.

- **Worktree chip** (#971, `SessionWorktreeChip.svelte`), in the header's title row after the goal chip. When the session's folder is in a git repository and the session is not isolated, it is a ghost `⑂ Isolate` button whose tooltip says which session holds the repository, if any. When isolated, it is a mono `text-xs` chip with an `info` outline on `info-muted`: `⑂ worktree · 3 files`, with the branch in the tooltip. It never shrinks below its label; the title row wraps instead. Its popover spans the header (the header is `position: relative`) on `surface-elevated`: a bold count line, branch / worktree / source facts, why it was isolated, the changed files in a `surface-inset` list (8, then "and N more"), then primary `Apply to checkout`, secondary `Keep branch`, and a red ghost `Discard` that turns into a danger `Discard for good` on the first click. A checkbox under the actions turns automatic isolation on or off. Actions are disabled while a turn runs. When the server moves a turn into a worktree, the feedback line says `Now working on <branch>`.

- **New chat in a folder** (`NewChatFolderMenu.svelte`). A caret button (`▾`, `aria-label` "New chat in a folder…") sits right after the board's primary **New chat** and the sidebar's **+ New Chat**, in the same button style, so the plain click still starts a chat at once. It opens a popover on `surface-elevated` with a `border-strong` edge — right-aligned under the board's actions, spanning the sidebar header in the dock — holding: **Recent folders** (a `surface-inset` list, mono `text-xs` paths, a small `info` `git` tag on repositories, the picked one marked like a selected palette row), a mono **Folder** field that the server checks as you type (`Checking…`, then `Git repository · <root>`, the not-a-repository note, or a red reason), and, only for a repository, a `⑂ Isolate in a worktree` checkbox whose hint appears when ticked. Actions are a ghost `Cancel` and a primary `Start chat`, disabled until there is a usable folder; Enter in the field starts, Esc closes. Errors stay in the popover. `/new [path] [--isolate]` does the same from the composer; without a path it uses this chat's folder (the checkout, for an isolated chat), and the feedback line names where the chat started.

### Command palette and shortcuts (#968)

The palette is the universal way in. Every route, including ones not in the nav, is reachable from `⌘K`: typing the first letters of its name makes it the top result (at most four letters where names share a prefix, like Chat and Channels). A new route or panel is not done until the palette lists it. In code, `lib/commands.ts` derives the page list from the router's `Route` views, and `tests/commands.test.ts` fails when a view has no entry or its admin gating drifts from `App.svelte`.

| Shortcut | Action | Scope |
|---|---|---|
| `⌘K` | Command palette: actions, pages, panel toggles, sessions, slash commands (slash commands only on the chat route) | Everywhere, including text fields |
| `⌘⇧O` | New session | Everywhere; opens the chat route |
| `⌥1` … `⌥9` | Switch to the Nth session as the sidebar lists it | Chat route |
| `⌘J` | Toggle the terminal: close it, reopen this session's tabs, or open one at the active cwd (without a cwd, the Files panel opens instead) | Chat route |
| `⌘B` | Toggle the session sidebar | Chat route |
| `⌘.` | Toggle focus (zen) mode | Chat route |
| `?` | Shortcut help overlay | Everywhere, outside text fields |
| `⇧Tab` | Cycle permission mode (Ask → Accept edits → Plan → Auto) | Composer |

Rules:

- `⌘` means `Ctrl` on Windows and Linux and `⌥` means `Alt`. The help overlay and palette hints show the platform's glyphs (`lib/shortcuts.ts`).
- Never bind a key the browser keeps for itself. Chrome delivers neither `⌘N`/`⌘T`/`⌘W` nor `⌘1…9` (tab switching) to the page, so `preventDefault` cannot help. That is why a new session is `⌘⇧O` (the convention web chat apps use) and switching is `⌥1…9`, where the original plan had `⌘N` and `⌘1…9`. The desktop shell (P4) may add native bindings through its bridge.
- A panel toggle, whether from the rail, the palette, `⌘B`, or `⌘J`, acts on what is on screen: a panel covered by another dock tab comes to the front, and only a visible panel closes.
- Shortcuts yield to whatever consumed the key first. A focused terminal keeps `Ctrl+K`, `Ctrl+B`, and `Ctrl+J` for readline. To toggle the terminal from the keyboard, move focus out of it first.
- Single-key shortcuts (`?`, and `y`/`s`/`a`/`n` on approval cards in P2) fire only when focus is outside text inputs.
- While the palette or help overlay is open, it owns the keyboard; only `⌘K` (close) passes through. Closing returns focus to where it was.
- The palette uses the `surface-elevated` background and a `border-default` outline, with no shadow (see Elevation). The selected row takes `surface-active` plus a 2px `primary` left edge, which is the same accent rule as focused inputs.

### Purpose

The TARS Console exists for exactly three things:

1. **Conversation** — run and steer agent sessions: chat, review the changes a turn made, approve tool calls inline, manage sessions (title/archive/pin/fork, goal/critic, prompt override, isolation), switch active cwd, and inspect transcripts and work timelines.
2. **Observability** — read system signals: pulse findings, reflection runs, ops health and cleanup plans, memory state, agent-runtime runs/subagents/costs, logs, analytics, event stream.
3. **Targeted control** — the small set of mutations that genuinely need UI: onboarding (provider/tier setup), credential entry, approvals (approve/reject destructive ops), cron job CRUD, extension install/enable/disable, channel pairing, remote access, restart/reset.

Everything else is **file-first**. Configuration is owned by `workspace/config/tars.config.yaml` (documented in `config/tars.config.example.yaml`, validated server-side at load). The console offers *inspection and validation* of configuration, not full editing: long-tail fields are read-only in the UI with a pointer to their documented YAML key.

### Route inventory

Every route must map to a pillar. Rows marked *(target)* describe the #967 workbench and are not built yet.

| Route | Pillar | Rationale |
|---|---|---|
| `/console` (home) | Conversation | Session board (#971): sessions by repository with live status |
| `/console/system` | Observability | Overview: dashboard summary of all signals (was home until #971) |
| Changes panel (dock, in chat) *(target, P1)* | Conversation | Turn/session checkpoint diffs, file and hunk revert, hunk comments |
| Inline approval cards (in chat) *(P2)* | Conversation + Control | Approve/deny tool calls without leaving the session; `/console/approvals` stays for unattended runs |
| `/console/focus`, `/console/focus/<session>`, `/console/focus/release` | Conversation | Focus mode (#1068): pipelines, one task's stepper, card deck and graph over the same session, and the release train |
| `/console/chat`, `/console/sessions` | Conversation | Chat transcript, session list |
| `/console/tasks` | Conversation | Work timeline / task contracts |
| `/console/sysprompt`, `/console/workspace` | Conversation | System-prompt authoring (adjacent to chat behavior) |
| `/console/memory` | Observability | Memory assets, inbox review |
| `/console/agentruntime` | Observability | Runs, subagents, cost flow, replay |
| `/console/pulse`, `/console/heartbeat` | Observability | Watchdog findings |
| `/console/reflection` | Observability | Nightly batch results |
| `/console/ops`, `/console/approvals` | Observability + Control | Health/cleanup plus approve/reject |
| `/console/logs` | Observability | Log tail/filter |
| `/console/analytics` | Observability | Usage trends (consolidation candidate: overlaps home cards) |
| `/console/cron` | Control | Cron CRUD + run history |
| `/console/extensions` | Control | Skill/plugin/MCP management |
| `/console/channels` | Control | Telegram pairing |
| `/console/config` | Inspection | File-first config policy (see below) |
| `/console/onboarding` | Control (setup) | Provider/tier wizard; capability-preserving reentry |

### Config policy: file-first, inspection-not-editing

- Source of truth: `workspace/config/tars.config.yaml`. Reference doc: `config/tars.config.example.yaml`. Server validates on load and reports via `/v1/admin/config/schema`.
- Console **keeps UI editing** only for: onboarding flows (wizard), credential entry (API keys/tokens marked sensitive), and Quick Start basics that gate first-run readiness.
- Console **stops editing** the long tail. First cut: the Fields tab became a validated read view (current value, effective value under env override, default/restart/secret badges) with pointers to the documented YAML key instead of inline editors. Structured LLM provider/tier editing remains available through the onboarding wizard reentry.
- The raw YAML tab became a read view; editing happens in an editor against the real file, then restart applies it. **Freeze update:** both the Fields tab and the YAML tab are gone — the page renders Quick Start only, and inspecting the long tail means reading `workspace/config/tars.config.yaml`.
- Server write routes (`PUT /v1/admin/config`, `PATCH /v1/admin/config/values`) remain part of the HTTP API for CLI/scripting use even where the console stops calling them.

### Config surface audit (#931, first cut)

Server schema exposes 165 fields (`internal/config/schema.go`) grouped into 15 sections. Audit of `Config.svelte` and `SessionConfigPanel.svelte` against actual usage classifies them as follows:

**(a) Keep UI editing** — onboarding, credentials, session control:

- Quick Start set (13 curated gates in `lib/quickStartFields.ts`): `api_auth_mode`, `llm_providers`, `llm_tiers`, `llm_default_tier`, `workspace_dir`, `telegram_bot_token`, `companion_enabled`, `embodiment_enabled`, `embodiment_providers_json`, `pulse_enabled`, `reflection_enabled`, `log_level`, `session_telegram_scope`. These gate first-run readiness and stay interactive — **except `embodiment_providers_json`**, which is a `json` field and follows the (c) rule below: its Quick Start card reports readiness but the value is edited in YAML. `llm_providers` / `llm_tiers` stay interactive via the wizard deep link (next bullet), not via a local editor.
- Credential/sensitive fields render masked and never echo a stored secret back. Only two still have a console input, both in the wizard: `memory_embed_api_key` (`OnboardingIntegrations.svelte`) and `tools_web_search_api_key` (`OnboardingTools.svelte`). The rest — `api_auth_token`, `api_user_token`, `api_admin_token`, `tools_web_search_perplexity_api_key` — are read-only in the console and rotated in YAML under the (c) policy. This is a deliberate consequence of the file-first cut, and it means **token rotation requires filesystem access to the host**; a console-only operator (remote/Tailscale) cannot rotate them. Revisit as a follow-up if that proves too strict.
- Structured LLM provider/tier editing stays in the console product — but lives in the onboarding wizard reentry (`/console/onboarding?reentry=1&section=provider|tiers`), which already implements alias-replace saves with masked-key preservation. `Config.svelte` links there instead of hosting duplicate editors.
- Quick Start cards are a wrapping flex row, not a two-column grid: the text block keeps at least 9rem, and when the control does not fit beside it the control drops under it (right-aligned) instead of squeezing the title to a word per line. Lines pack to the top (`align-content: flex-start`), so a card stretched to its row's height never pushes a wrapped control to the bottom.
- The `llm_providers` card's ghost **Test connection** probes every provider alias (`POST /v1/admin/providers/test`, `lib/providerProbe.ts`). Results sit in a full-width section under both columns (top `border-subtle` rule), one row per alias, the default tier's first: mono alias, tertiary mono kind, a `default` badge, and a status badge on the right — `success` OK, `info` Unverified (a CLI too old to report its sign-in), `warning` Warning, `error` Failed — with a 2px left rule in the same hue. Under it a `text-secondary` summary and, when there is one, a mono `text-tertiary` detail (the CLI path or the provider's own error) that wraps anywhere. CLI providers are never reported as failing just because they have no model list.
- `SessionConfigPanel.svelte` (session-scoped tools/skills/commands/MCP allowlists, automation consent, style controls) is session/cwd control — core purpose. Unchanged.

**(b) Read-only inspection candidates** — validated read view + YAML key pointer. **Superseded by the freeze:** the read view described here was the first cut's Fields tab, which no longer exists. These fields have no console surface at all now; inspect them in `workspace/config/tars.config.yaml` against `config/tars.config.example.yaml`. The classification is kept as the record of which fields were never editing candidates.

- Operational tuning long-tail across sections: Runtime logging/rotation, API inflight caps, Remote Access, Memory embedding tuning, Usage limits/budgets, Pulse thresholds/windows, Reflection windows, Compaction numbers, Tools toggles/timeouts/providers, MCP allowlist, Agent Runtime persistence/archive/watch/consensus knobs, Work Ledger / Work Scheduler internals (leases, polling, A2A), Channels enables, Assistant binaries, notify/schedule settings.
- First cut: these rendered value + effective-value-under-env-override + default/restart/secret badges, and editing happened in the YAML file. Since the freeze only the Quick Start cards render those badges; everything in this list is file-only.

**(c) YAML-first removals** — editors deleted from `Config.svelte` in this cut (values remain visible read-only):

- All four heavyweight modal editors: generic JSON editor, LLM tier editor, LLM provider editor, embodiment provider preset editor.
- Consequence table: `llm_providers` / `llm_tiers` → deep link to onboarding wizard sections (capability preserved). `embodiment_providers_json`, `llm_role_defaults`, `usage_price_overrides_json`, `mcp_servers_json`, `agentruntime_agents_json`, `agentruntime_task_override`, and every other `json`/`string_list` field → read-only summary plus documented YAML key (`config/tars.config.example.yaml`). If a dedicated UI for embodiment presets proves necessary later, it should be a follow-up issue scoped against this policy.
- **Retained but currently unreferenced:** deleting the modal editors left `lib/configStructured.ts`'s draft builders (`makeLLMTierDrafts`, `buildLLMTiersFromDrafts`, `makeLLMProviderDrafts`, `buildLLMProvidersFromDrafts`, `makeEmbodimentProviderDrafts`, `makeEmbodimentProviderPresetDraft`, `buildEmbodimentProvidersFromDrafts`) and their constant tables with no component consumer — `Config.svelte` imports only the four display helpers. They are kept, still unit-tested, for the embodiment-preset follow-up named above; the client wrappers `saveConfig` and `getProviders` in `lib/api/config.ts` are unreferenced for the same reason. Treat this list as the inventory to delete if that follow-up is declined, rather than as code to quietly reuse.

### Shared form primitives (#931)

**`FormField` — extracted, onboarding migrated.** `src/components/onboarding/FormField.svelte` owns the label + optional hint shell that every wizard control sits in. The control is passed as a snippet child rather than described by props: the wizard's fields bind, dispatch, and validate in too many different ways for a control-generating component to cover without growing a prop per variant. Owning only the shell keeps every `bind:value` and `oninput` handler untouched.

All 18 call sites across the five section components now use it. The styles moved with it: `Onboarding.svelte` previously carried five `:global(.onboarding-field …)` rule blocks purely because the markup was duplicated across children, and those are gone.

Two consequences worth knowing before adding a field:

- **Controls carry the caller's style scope, not `FormField`'s.** Svelte scopes a snippet's markup to the component that wrote it, so `FormField`'s control rules must stay `:global()`, nested under `.onboarding-field` so they cannot leak. A per-control override — like the monospace treatment on the web-fetch allowlist textarea — belongs in the calling component, on a class attached to the control itself.
- **A conditional hint becomes a ternary prop.** The old markup inlined `{#if …}<em>…</em>{/if}` inside the label span; the equivalent is `hint={condition ? text : undefined}`.

**Still to do:** `Config.svelte`'s Quick Start controls are a *different* primitive — a schema-driven type dispatch (`bool` toggle / `select` / inline edit / sensitive button / `json` readonly) rather than a label shell — so they do not share `FormField` and want their own extraction. `bool-toggle` appears only in `Config.svelte`, so there is nothing to share yet. `SessionConfigPanel.svelte` has not been audited against `FormField`. Per the original plan these stay separate PRs with their own visual verification.

**Known issue found during the extraction:** the wizard's styles reference `var(--border-soft)` 16 times and that token is never defined, so every onboarding input, select, and textarea has silently rendered with no border. `FormField` preserves the broken token verbatim so the extraction is provably behavior-identical; fixing it is #948, because making borders appear across the whole wizard is a real visual change that wants its own review.

### CLI deprecation candidates (#931 — analysis only, nothing removed)

The 37 CLI subcommands overlap with console-only paths in a few places. Candidates marked for a future deprecation decision; each has a working replacement path today:

| Command | Why candidate | Replacement path |
|---|---|---|
| `tars approve list / run / reject` | Mirrors Ops page approvals workflow one-to-one (`/v1/ops/approvals`) | Console Ops page |
| `tars cron list / get / runs / run` | Mirrors Cron page CRUD + run history (`/v1/cron/*`) | Console Cron page |
| `tars auth passwd` | Mirrors RemoteAccessCard password change (`PATCH /v1/auth/users/{role}/password`) | Console Remote Access card |
| `tars remote status / enable / disable / url` | Mirrors RemoteAccessCard Tailscale controls (`/v1/admin/remote-access/*`) | Console Remote Access card (CLI keeps `url` value for scripts) |
| `tars skill / plugin / mcp search · install · uninstall · update · info` | Hub operations duplicated by Extensions page (`/v1/hub/*`) | Console Extensions page |

Keep-list rationale: `serve/service/status/health/doctor/init/version/worker/assistant/pack/reset/auth init|pairing-code` have no console equivalent or are needed when the console is unavailable (headless recovery, scripting, CI). Deprecation of any candidate above should be its own issue with an exit survey of scripts using them.

### Orphaned-route check (#931 first cut)

Endpoints the console stopped calling in this cut:

- `PUT /v1/admin/config` (was YAML-tab save): no longer called from any component. Server route retained at `internal/tarsserver/main_serve_api.go`; reachable via HTTP API for scripts/tooling. No CLI equivalent exists (`tars serve --config-check` only validates). Follow-up candidate if it stays uncalled, not removed now.
- `GET /v1/providers` (was tier-editor provider metadata): no longer called from any component. Route retained; useful as a raw API for integrations. Same disposition.
- `PATCH /v1/admin/config/values`: still called (Quick Start saves). Unchanged.
- All other endpoints previously called by the console remain called; nothing was deleted server-side.

## Colors

The palette is rooted in cool graphite neutrals, a single signal-green accent, and four semantic states.

- **Primary (`#3ee07f`):** Signal green, like a terminal's ready light. The sole driver of interaction: primary buttons, focus rings, the selected row, link emphasis, focused borders. Used sparingly so that when it appears, it commands attention. Pair `primary-hover` (`#2fc56b`) for hover/pressed, `primary-muted` (alpha 0.12) for accent backgrounds, and `primary-text` (`#7cf0a8`) for text on muted accent surfaces. Text on a solid green fill is always `primary-contrast` (`#06140c`), never white: white on this green fails contrast. In CSS, tint with `rgba(var(--primary-rgb), α)`; never hard-code the accent (a test fails on the retired amber). Canvas and Mermaid colors, which cannot read CSS variables, come from `src/lib/themeColors.ts`, tested against these tokens.
- **Surfaces (`surface-base` → `surface-active`):** A six-step neutral elevation ladder. `surface-base` (`#141414`) is the page; `surface` (`#1c1c1c`) is a card; `surface-elevated` (`#242424`) is hover/header; `surface-inset` (`#111111`) is for inputs and code. Steps are tonal, not shadow-driven.
- **Borders (`border-subtle` → `border-strong`):** Three weights of grey for separation. `border-subtle` divides cards from background; `border-default` outlines secondary buttons and inputs; `border-strong` is reserved for hover/focus states on outlined surfaces.
- **Text (`text-primary` → `text-ghost`):** Four levels of cool, neutral off-white. `text-primary` for body, `text-secondary` for labels and metadata, `text-tertiary` for placeholders and inactive captions, `text-ghost` for nearly-decorative micro-labels (code-block lang badges, etc.).
- **Semantic (`success`, `warning`, `error`, `info`):** Each ships with a `*-muted` tinted-background variant. Use the muted variant as the background and the solid variant as the text — never the reverse, and never solid-on-solid.

> **On muted tokens.** The DESIGN.md spec requires opaque sRGB hex values, so each `*-muted` token (including `primary-muted`) is the pre-blended result of the corresponding tint at low alpha (`primary` 12%, semantic states 10–12%) composited over `surface` (`#14171a`). The runtime CSS in `src/app.css` still expresses these as `rgba()` so they remain translucent on whatever surface they sit on; the design tokens here are the canonical *flat* values for tooling that requires opaque colors (Tailwind, Figma, DTCG export). If you change the runtime alpha, recompute the hex here.

### Pairings to avoid

- `text-ghost` on `surface-base` — readable only as decoration; do not use for any meaningful copy.
- Solid `success`/`warning`/`error`/`info` as a backgroundColor for body text — saturated; always reach for the muted variant.

## Typography

Three families, each with one job:

- **IBM Plex Sans** handles headings (`h1`–`h4`, 600) and running prose (`body-md`, `body-sm`, 400). It is an engineering grotesque: neutral, compact, readable at small sizes, with none of the friendly roundness of consumer chat apps.
- **IBM Plex Mono** handles code (`code`) and `label-caps`: inline code, code blocks, log output, and every small uppercase label (card titles, badges, group labels). Mono labels are what make the UI read as a tool.

Headings step down narrowly (1.75 → 1.375 → 1.125 → 1 rem) because most of the UI is mid-density list/card surfaces — large hero type would feel out of register. `label-caps` (IBM Plex Mono 0.6875rem, uppercase, 0.06em tracking) is reserved for slot labels above editable fields, card section titles, group labels, and badges.

### Rules

- Do not introduce a fourth font family.
- Do not use weights below 400 or above 600 — the scale only exercises 400/500/600.
- Do not set `text-transform: uppercase` outside of `label-caps`-derived classes.

## Layout

Spacing follows an 8-rooted scale with a 4-unit half-step (`xs`) for micro-adjustments. The default rhythm of a card is `lg` (16px) inside `lg` (16px) outside; the default gap between siblings is `md` (12px). Larger pages use `xl`/`2xl` for outer page padding.

The console uses a fixed left navigation (`220px`) and a fixed top header (`52px`); the remainder is a single content column with a max-readable width on long-form views (memory, sysprompt). On viewports below 768px the nav collapses to zero width and a header-driven menu is expected to take over.

The chat route is moving to a three-column workbench (#967): the session sidebar, then the conversation, then the dock. The conversation column keeps its max-readable width. The dock and sidebar give up width first, and each can be collapsed with `⌘J` and `⌘B`. Below 768px only the conversation shows, and the sidebar and dock open as overlays.

Don't crowd. The system has so much spacing tokenized because the worst failure mode of a dark dense UI is a panel of 12 cards with 4-pixel gaps that read as a single grey wall.

## Elevation & Depth

There are no shadows. Depth is conveyed entirely through:

1. **Tonal layering.** Stack `surface-base` → `surface` → `surface-elevated` to suggest hierarchy.
2. **Border subtlety.** A single `border-subtle` (`#282828`) line is enough to separate a card from its background — anything heavier reads as a heavy outline.
3. **Accent borders for focus.** Focused inputs and active tabs get a 1px `primary` border. This is the only place pure accent appears as an outline.

Reasoning: shadows on dark surfaces tend to either disappear (low contrast) or look like halos (high contrast). Tonal stepping reads cleanly at every brightness setting.

## Shapes

Three corner radii: `sm` (2px), `md` (4px), `lg` (6px). Buttons, inputs, and badges use `md` or smaller; cards and elevated surfaces use `lg`. Nothing is fully pill-shaped — the system has no `full` radius.

Rationale: generous rounding reads as approachable consumer software. Tight 2–6px corners read as a precise tool and keep dense rows aligned.

## Motion

Motion is decoration, never the only signal. Every repeating or decorative animation (floating, blinking, pulses, enter animations) stops under `prefers-reduced-motion: reduce`; the state it hints at must still read from text, colour or position. A reduced-motion block must name every animated selector on its own, not lean on a bare base selector that a more specific rule would otherwise outrank. `tests/companionPet.test.ts` checks this for the companion pet (CASE).

### CASE's expressions

CASE's face (`CompanionPet.svelte`) is CSS and markup only — no image, font icon, or animation-library asset, ever. Eyebrows, eyes, mouth and the antenna tip are always in the DOM; one class on `.companion-body`, `expr-<name>`, decided by the pure function `companionExpression` in `lib/companion.ts`, changes their shape. There is no colour-only face part: the old glow dot next to the mouth read as a second mouth and is gone. When a state colour is useful it rides only on the antenna tip — two expressions that only differed in that colour would be the same expression, not two, so nothing else on the face carries colour as its only difference. Eyebrows stay invisible unless they carry meaning (`alert`/`upset`/`wary`); everywhere else a faint brow line was noise above eyes that already read fine without it.

Eight expressions:

| Expression | Eyes | Mouth | Brows | Antenna tip |
|---|---|---|---|---|
| `neutral` | default oval, blinking | short flat line | hidden | default (filled, signal green) |
| `working` | both glance up and to one side | small, closed/pursed | hidden | a ring (hollow), not a filled dot |
| `alert` | wide open, stop blinking | round "oh" | both lift | warning colour, brightens |
| `happy` | curve upward (closed, joyful) | wide smile | hidden | success colour |
| `upset` | pinch inward, flatten | frown | both furrow down | error colour |
| `wary` | one eye narrows to a squint | tilts | one lifts, thick and sharply angled | warning colour |
| `sleepy` | both close to a thin line | tiny dot | hidden | dims |
| `greeting` | each a four-point sparkle (clip-path star) | wide smile | hidden | success colour, brightens |

`working` and `wary` are each told apart from their nearest neighbour (`neutral`, `alert`) by at least two shape changes at once, not a single small nudge — a 3px eye shift alone did not read at the companion's actual on-screen size. `greeting`'s sparkle is a clip-path star, not a plain "+": a cross read as a broken eye rather than a glint.

What picks the expression — `companionExpression(snapshot, cues?)`, pure, tested in `tests/companionPet.test.ts`:

1. `snapshot.lines` (from `companionState`, i.e. real waiting/running/failed state) always wins over a cue: a failure line → `upset`, a pending/queued approval line → `alert`, a running-turn line → `working`.
2. Only when there is no such line, an optional `cues` argument picks the rest, in order: `warning` → `wary`, `justFinished` → `happy`, `justArrived` → `greeting`, `longQuiet` → `sleepy`.
3. Nothing at all → `neutral`.

The six old moods (`idle`/`spark`/`focus`/`warn`/`error`/`success`) are gone; they mapped onto these eight one-to-one or were never produced (`spark`, `success`) and would otherwise be dead code sitting next to the real thing.

`prefers-reduced-motion: reduce` stops the float and blink, same as any other decorative animation here — the expression itself is read from shape alone, not from the motion that goes with it.

### Cues (#1191)

`CompanionPet.svelte` fills in the `cues` argument from real events and timers, each a pure function in `lib/companion.ts` the component calls on a real clock:

- **`warning`**: a warn-severity SSE event (`companionWarningFromEvent`) — something is off, but not yet a failure. Never opens the bubble.
- **`justFinished`**: another session's running turn disappears from `GET /v1/chat/activity` without landing in a recent failure (`companionTurnsJustFinished`/`companionFailedSessionIds`) — it finished, not failed. The session the chat route has on screen is left out, same rule as `companionState`'s lines.
- **`justArrived`**: the console is opened, or a tab hidden for ten minutes or more (`COMPANION_JUST_ARRIVED_HIDDEN_MS`) becomes visible again (`companionJustArrived`). A quick tab switch is not an arrival.
- **`longQuiet`**: no input for five minutes (`COMPANION_LONG_QUIET_MS`) with nothing waiting (`companionIsLongQuiet`) — level state, not an edge, so it clears the instant an input event arrives, before the next timer tick even runs. "Input" is a click, a keystroke, a drifting pointer, or a scroll (`pointerdown`/`keydown`/`pointermove`/`wheel`) — a reader who is only looking, not clicking, must not have CASE doze off under them. `pointermove`/`wheel` fire far more than once a second, so the write to `lastInputAt` is throttled to at most once a second (`companionShouldRecordInput`, pure, injected clock) rather than on every event; the first input after a long quiet stretch always passes the throttle, which is what wakes a sleeping CASE immediately. This tracking is a separate listener set from cursor gaze's own `pointermove` (`startInputTracking`/`stopInputTracking`, not `startGaze`/`stopGaze`): it runs under `prefers-reduced-motion: reduce` too (a longQuiet→`sleepy` transition is a shape change, not a motion) and only stops while the tab is hidden.

`warning`/`justFinished`/`justArrived` are edges, not level state: `companionCueTick` holds at most one of the three at a time (a higher-priority signal — in that same order — replaces a still-showing lower one; a same-or-lower one never interrupts) and each expires on its own after a few seconds (`COMPANION_CUE_DURATIONS_MS`), falling back to whatever `snapshot.lines` or `longQuiet` says next. `companionCuesFor` assembles the `CompanionExpressionCues` `companionExpression` reads. None of this ever outranks a real line — `snapshot.lines` still wins, as above.

### Server-driven cue (#1192)

A `category: "companion"` SSE event lets the server (initiative, #1000, out of scope here) tell CASE what to show, through the same cue machine above rather than a separate path: it is a fifth `CompanionCueKind`, `'server'`, outranking `warning`/`justFinished`/`justArrived`/`longQuiet` — real waiting/failure/running state in `snapshot.lines` still outranks it.

Wire format — one optional field added to the shared notification shape (`notify.go`'s `notificationEvent`, the same pattern `job_id`/`open_path` already use), carried over `/v1/events/stream` only:

| Field | Meaning |
|---|---|
| `category` | `"companion"` |
| `expression` | one of `COMPANION_EXPRESSIONS` — an unrecognized value drops the *whole* event, not just the face |
| `message` | optional. The bubble's one line, shown as plain text only (never `{@html}`), clipped to 140 chars |
| `session_id` | optional. Where clicking that line goes (`/console/chat/<id>`) |

The console validates every field on receipt (`companionServerCueFromEvent`, pure): besides an unknown `expression`, an event older than 60s (`COMPANION_SERVER_EVENT_MAX_AGE_MS` — a reconnect/replay guard) or a `coalesced` replay is dropped outright, and a `session_id` matching the session already on screen keeps only the expression — that thread already shows the message, so the line is suppressed (same rule `companionState` applies to the active session's own pending/running lines).

A body_only event (no `message`) only changes the face: it never opens the bubble or touches the badge, and the cue fades after `COMPANION_SERVER_CUE_NO_LINE_MS` (6s). An event with a line opens the bubble by itself, same trigger as a new approval wait or failure, and the line sits above the "waiting on you" list — tinted with the brand colour (`.companion-line-server`), not the neutral/failure borders those use, so it reads as CASE talking rather than another queue item. It holds for up to `COMPANION_SERVER_CUE_LINE_MS` (5 minutes), until the user dismisses it — closing the bubble, or clicking the line when it has a `session_id` (which also navigates there) — or until a *newer line* replaces it, whichever comes first; a later body_only companion event never cuts a still-showing line short (`companionCueTick`'s server sub-rule, #1192 review). A line with no `session_id` is a static row (`.companion-line-server-static`) that only closing removes.

Server-side, this category never reaches the general notification path: `notificationDispatcher.Emit` special-cases `category: "companion"` to publish straight to the live broker, skipping the notification store (`/v1/events/history`, unread count) and the desktop notifier — it is CASE's own face, not a general alert. The console mirrors this on receipt: `companionFailureFromEvent`/`companionWarningFromEvent` return nothing for a companion event regardless of the `severity` it happens to carry, and `App.svelte`/`Home.svelte` route it to the cue machine instead of their "recent notifications" list or unread counter.

### Actions (#1191)

A one-shot animation plays exactly once when the expression *changes* into one of five, and does not replay while that expression holds (`companionActionFor(previousExpression, expression)` in `lib/companion.ts`, a pure table lookup; the component tracks `previousExpression` and clears the action on its own CSS `animationend`, matched by name so an unrelated animation bubbling up — blink, breathe — never clears it early):

| Expression | Action | Where it plays |
|---|---|---|
| `alert` | bounce | `.companion-body` |
| `happy` | nod | `.companion-body` |
| `upset` | shake | `.companion-body` |
| `wary` | tilt | `.companion-body` |
| `greeting` | wave | `.companion-antenna` (CASE's own gesture, not the body nudging it) |

`working` and `sleepy` have no one-shot action; they are the two *continuous* states instead, repeating for as long as they hold: working's antenna tip pulses, sleepy's body breathes (a slow scale) alongside a drifting `z` (`.companion-zzz`, hidden everywhere else, CSS only).

### Idle play (#1191)

While CASE is `neutral`, nothing is waiting, and the bubble is closed (`companionIdleEligible`), it plays a short look-around, yawn, or wink every 60–180 seconds (`companionNextIdle(random)`, pure — the delay and the kind both come from an injected `() => number` so the schedule is deterministic under test; the component injects `Math.random`). Purely decorative: it never changes the expression or the badge. A real event can still move `expression` off neutral while a look/yawn/wink is mid-flight, and unlike the one-shot actions above, these animate the same eye/mouth width, height and border-radius an expr-* rule also sets — so staying alongside `expr-neutral` is enforced twice, not just intended: the CSS itself is scoped (`.companion-body.expr-neutral.play-look .companion-eye`, and so on), and the component clears `idlePlay` the instant `expression` leaves neutral rather than waiting for the animation's own end (which would not come anyway — losing `.expr-neutral` cancels it instead of finishing it).

### Cursor gaze (#1191)

When the pointer is within 200px of CASE's centre, the eyes lean up to 3px toward it, scaling linearly with distance and clamped at both ends (`companionGazeOffset`, pure geometry — no DOM). `CompanionPet.svelte` batches `pointermove` into one `requestAnimationFrame` via a `passive` listener and writes `--eye-x`/`--eye-y` custom properties on `.companion-eyes`; the eyes consume them with the separate `translate` property (not `transform`), so the lean composes with whatever an expression's own `transform` already does (working's glance, upset's pinch) instead of replacing it. Four expressions whose eyes are not pupil-shaped — `happy`, `upset`, `sleepy`, `greeting` (a curve, a flat line, a star) — never get a lean; that is a CSS selector guarantee (`.companion-body:not(.expr-happy):not(.expr-upset):not(.expr-sleepy):not(.expr-greeting) .companion-eye`), not just the component's own `companionGazeApplies` check, so a stale offset can never leak onto one of those four.

### Stop conditions (#1191)

- **Hidden tab** (`document.hidden`): every timer (the cue/long-quiet tick, the idle schedule) and listener (cursor gaze's `pointermove`, and the input-tracking `pointerdown`/`keydown`/`pointermove`/`wheel` set) stops. The idle schedule resumes from the same absolute fire time it had before pausing, not a freshly rolled delay, so hiding the tab never resets how soon CASE plays next; the cue/long-quiet tick just restarts, since cue expiry is computed from wall-clock timestamps that stay valid regardless of whether anything was ticking while hidden.
- **`prefers-reduced-motion: reduce`**: actions, idle play and cursor gaze are off, and their timers/listeners are never even started — not merely hidden by CSS. Cue-driven expression changes (`happy`/`wary`/`greeting`/`sleepy`) still happen; only the motion that would otherwise go with them does not. Every new `animation`/`transition` selector this adds is named, one for one, in the `prefers-reduced-motion: reduce` block — `tests/companionPet.test.ts` checks this for the whole stylesheet, including these.
- **Unmount**: every timer and listener started in `onMount` is cleared in its cleanup.

### Hit area (#1191)

`.companion-button` — the clickable element — has no animation or transform of its own and never changes size: the float, every action, idle play and the antenna/breathing loops all live on `.companion-figure` inside it, which is `pointer-events: none`. A `z` or any other decoration is `pointer-events: none` too. This is the same hazard #1194 hit with the bubble covering a neighbouring button, applied to motion: nothing CASE does to itself may grow or move its own hit area over a screen element next to it.

## Components

### Buttons

Five variants — `primary`, `secondary`, `ghost`, `danger`, `warning` — plus a `sm` size modifier.

- `button-primary` is the only filled button. Use it once per screen for the main commit/run/save action. Background is the green accent; text is `primary-contrast` at weight 600, which passes WCAG AA comfortably. (The old amber/white pair failed at about 2.5:1.)
- `button-secondary` is the workhorse outlined button. Transparent background, `border-default`, `text-primary`. Most actions land here.
- `button-ghost` removes the border entirely. Use for tertiary or in-table actions where outlines would multiply visual weight.
- `button-danger` is a ghost variant tinted red. Always confirm the destructive action with a second affordance (modal, inline confirmation) before letting the click do work.
- `button-warning` mirrors `button-danger` but uses the `warning` color. Reserved for "this is reversible but unusual" actions (e.g., "Run pulse now" out of cadence).

`button-sm` reduces padding to `xs`. Use for inline actions inside dense rows; never for a primary CTA.

### Badges

Six tonal tints (`default`, `accent`, `success`, `warning`, `error`, `info`). Each pairs a muted background with a solid foreground from the same hue.

- Badges are read-only. Do not attach click handlers — if a label needs to be interactive, it's a button.
- Keep badge copy under 18 characters. Longer text breaks the rhythm of the row.
- Do not use more than two distinct badge tints in a single dense list — it overwhelms the row.

### Card

The default container. `surface` background, `border-subtle` outline, `lg` rounded, `xl` internal padding. A card may have a header row (`card-header` / `card-title`) using `label-caps` for the title.

Cards are the primary chunking device. When you want users to perceive "these things go together", put them in a card. When you don't, leave them on `surface-base`.

### CWD chip (composer status bar)

Sits in the status bar under the composer (it lived in the session header until #968). Compact mono chip (3px / `space-2` padding, `radius-sm`) that shrinks and truncates before crowding the bar. The active path renders in **`primary` (green)** with `font-mono`, prefixed by the dimmed label `cwd`. Click toggles a `position: absolute` dropdown that opens upward from the chip's right edge, listing every eligible cwd; the active row also reads in green with a small bullet marker. Disabled state uses `opacity 0.7` while a transition is in flight. The chip is hidden until the session has loaded (no eligible-cwd payload → no chip), so empty states never show a phantom widget.

### Folder picker (shared `FolderPickerDialog.svelte`)

A server-backed folder browser over `GET/POST /v1/filesystem/browse` (helpers in `lib/folderPicker.ts`). One component, two homes: the Files panel's "+" shows it inline, filling the panel at the server's home folder (`ArtifactPanel.svelte`); Focus mode's new-task form (below) opens it in a modal via the **Browse…** button next to the folder field and next to each extra-folder row, starting at that field's checked folder, else the primary folder, else home. Top to bottom: the `label-caps` title with `Select Here` / `Cancel`; the **path field** — a full-width mono `10px` input on `surface-inset` that reads as the old path strip until focused, then takes the 1px `primary` border (the Input fields rule). Typing or pasting a path and pressing Enter opens it: a leading `~` expands to the home folder the picker opened in, surrounding quotes and trailing slashes are dropped, and relative or `~user` paths are refused before any request. Escape puts back the folder being shown (and, with nothing to put back, is left for an enclosing modal to close on). Then `New Folder`, the error line (`error`, `role="alert"`), and a **filter** input (`Filter by name`, part-of-name, case-insensitive; Enter opens the first match, Escape clears it) that appears once a folder has subfolders and resets on every navigation.

As a modal (Focus mode): a `rgba(0,0,0,0.6)` backdrop centers a `surface-elevated` panel (`radius-md`, `border-subtle`, fixed `70vh` up to 640px) that clips the dialog's own border-top to its rounded corners. It closes on a backdrop click (the dialog's inner click stops the event so clicking inside it doesn't close) or on Escape. Escape is handled on the window, not the panel: opening a folder replaces the list and drops focus to `<body>`, where a panel handler would never hear it; a field that uses Escape for itself (a typed path to put back, a filter to clear, the new-folder prompt) stops the event first. `role="dialog"` `aria-modal="true"` on the focusable (`tabindex="-1"`) panel, focused on open; closing returns focus to the Browse… button that opened it.

A failed browse — missing path, a file, an unreadable folder — shows the console's own wording (by status, not the server's English message) with the path, and **keeps the current listing**, so a typo never loses the user's place.

**Dot folders are folded, not dropped.** Folders starting with `.` (`.cache`, `.config`, `.claude`, …) usually outnumber the ones people pick and pushed them below the fold, but some are real destinations (`.claude/worktrees/…`). So the list shows ordinary folders first and ends with a mono `text-secondary` toggle row, `▸ Hidden folders (N)` (`aria-expanded`, caret turns on open), collapsed each time the picker opens and kept open while navigating. While a filter is active the dot-folder matches are listed directly after the others — typing is intent enough — and an empty result reads `No folders match "…"`.

### "TARS spoke first" badge (chat thread, #1220)

Initiative's live mode (`initiative.mode: live`) can write a real assistant message to the main session on its own — a greet or check-in nobody asked for. The bubble that message lands in (`ChatMessageItem.svelte`) carries one small marker so the user can tell it from a reply: a mono `10px` pill (`.chat-spoke-first-badge`) right after the role label in the bubble's heading row, signal-green on a `rgba(var(--primary-rgb), 0.12)` tint with a matching `0.28`-alpha 1px border — quieter than the usage badge's neutral grey, since this is provenance, not metering, but still restrained: the message text is the point, not the badge. Text comes from `$t.chatThread.spokeFirst` (`TARS spoke first` / `TARS가 먼저 말함`). Shown only on an `assistant` message whose transcript entry carries `Initiative` metadata (`ChatMessage.spokeFirst`); every other message — including a normal reply that happens to follow one — has no badge.

### Turn change card (chat thread, #969)

Sits under the last message of each turn that changed files, in the thread column (not inside a message bubble, since the Korean E2E reads the card's chrome). It is a `surface` box with a `border-subtle` outline and `radius-md`. Collapsed, it is one line: a caret and the mono `text-xs` summary `3 files +6 −2`, plus a ghost `Open in Changes` button on the right. Expanded, it lists each file as a header row (mono path, status word in `text-tertiary`, `+n` in `success`, `−n` in `error`) above a unified `DiffView`. The body scrolls past 480px. A turn whose recording was skipped shows one dim mono line instead (`± Not recorded: too many files changed`), and a turn that changed nothing shows no card at all. The card is keyed by the turn's user-message ID, so it returns when history reloads.

### Tool call changes (chat thread, #1032)

On native providers, a file tool's card (`write_file`, `edit_file`, `apply_patch`) learns what the call changed as it finishes, from the stream's `file_change` event. The collapsed card's header gains a mono `text-xs` `+n −m` (`success` / `error`) between the call preview and the elapsed time. Expanded, a `ToolCallChanges` block sits between the output and the result: the dim mono label `2 files changed`, then one foldable row per file laid out like the turn change card's file header (caret, mono path, status word in `text-tertiary`, `+n −m`), opening to the same unified `DiffView`, which scrolls past 360px. Binary and over-long files show the change card's dim notes instead of a diff. CLI providers send no event, so their cards stay as they were and only the turn change card shows their edits; a reloaded history shows tool cards without these rows, the turn card covering the same files.

The chat log is one `minmax(0, 1fr)` grid column, so a wide diff line or tool call scrolls or truncates inside its own box instead of pushing the whole log sideways.

### Tool approval card (chat thread, #970)

Appears in the thread where a tool call waits on the user — a `claude-code-cli` prompt or a high-risk TARS tool on a native provider, in the same event shape — at the point the prompt arrived; a bubble that already holds text is frozen and the turn continues in a new one after the card, as tool cards do. While pending it is a `warning-muted` box with a `warning` outline and `radius-md`: a bold heading (`Allow Bash?`, or Claude Code's own title), an `info` badge when a subagent asked, the mono `label-caps` kind (`COMMAND`, `FILE`, `URL`, `INPUT`) over the command, path, URL or JSON in a `surface-inset` block that scrolls past 240px, and the reason line. The actions are a secondary `Allow once`, a ghost `Allow Bash(npm test:*) for this session` (only when the server offers a rule — never for compound commands, `rm` or `sudo`), a ghost `Always allow Bash(npm test:*) in this folder` (when the server also offers a folder; its tooltip names the folder), and a danger `Deny`, with the dim mono key hint `y allow · s this session · a always · n deny` on the right. The card takes focus when it appears, so the keys work at once; they are read only while focus is inside the card, never from the composer. Answered, the card settles to a `surface` box with one mono outcome line (`success` for allowed, `error` for denied, `text-secondary` when the turn ended first). It settles from the server's `permission_resolved` event rather than the click, so a cancelled turn closes it too. Cards are not persisted: a reloaded history shows the tool calls, not the prompts. Always-allow rules are listed per folder in Session Config → Permissions (`PermissionRulesList`): one row per rule — mono rule text, dim provider label, danger `Remove` — and they can only be added from a card.

**Plan approval card** (#970). When Claude Code calls `ExitPlanMode`, the card shows the plan through `MarkdownContent` in place of the command preview, with three actions: primary `Approve · accept edits`, secondary `Approve · auto`, and ghost `Keep planning` (keys `y` / `n`). Approving saves the mode on the session and switches the running CLI at once. **Plan bar**: while the session is in `Plan`, no turn is running and the last message is the assistant's, an `info-muted` bar with an `info` outline above the composer says `Plan mode: read-only` and offers the same two approvals; each sets the mode and sends "The plan is approved. Go ahead and carry it out." as the next message. On native providers plan mode refuses high-risk tools without a card, so the bar is how a plan gets approved there.

**Needs-input bar** (#970, `ChatUnattendedApprovals.svelte`). When an unattended run of the open session (cron, Telegram, a subagent) waits on a tool call its permission mode asks about, a `warning-muted` bar with a `warning` outline sits above the composer, one per question, oldest first: bold `warning` `A cron run needs input`, the tool, and the mono preview (command, path or URL), with secondary `Approve` and danger `Reject`. It polls the ops approvals every 3s while the session is open. The Ops page lists the same questions as `tool call: exec` approvals: preview, reason and folder, an `Open session` link, and a dim note that the run skips the call after 30 minutes. Answered or expired questions keep their status badge (`expired` uses the default badge).

### Changes panel (dock, #969)

A right-zone dock panel (`±` on the rail) that reads the same store as the cards (`lib/stores/changes.svelte.ts`). From top to bottom:

- The heading (the Git Inspector's small caps `section-title`), a two-button segment (`This turn` / `Session so far`), and Refresh.
- The turns that changed files, newest first. Each is a row with the prompt preview on top and a mono meta line below (summary on the left, time on the right). Skipped turns show their reason in `warning`. Next to each recorded turn a narrow `↺` button (`surface`, `border-subtle`, `warning` on hover) reverts that whole turn through the same preview bar as `Revert turn`, without selecting it first.
- The recorded folder.
- The selected turn's files as a folder tree: folder rows (caret, mono `name/`, file count and summed counts, `text-secondary`, no border) fold and unfold, a folder holding only one folder shows as one row (`src/lib/`), and file rows keep their bordered style with the file name, status and counts. Rows indent 14px per level. Folded folders stay folded across turns.
- The selected file's `DiffView`, with the same Unified/Split toggle as the Git Inspector.

Selected rows take the `primary` border on `surface-elevated`, as in the Git Inspector.

Reverting (P1 PR6) happens in the panel, never in the thread's card:

- **Where the actions sit.** Under the folder line, the selected turn offers `Revert turn` (secondary) and `Restore to before this turn` (ghost, with a title that says later turns' edits go too). The diff header has `Revert file`. In `This turn` scope, each hunk header row has a small outlined `Revert hunk` button. `Session so far` hides the hunk and file buttons, because its hunks span turns.
- **Preview, then confirm.** Every action first asks the server for a preview. A bar under the header (`surface-elevated`, `border-default`, `radius-md`) states what will happen ("Revert 1 hunk in base.txt?") and lists each file with its outcome word (revert, merge with later edits, delete, already reverted). `Revert` is the panel's one primary button; `Cancel` is ghost.
- **Conflicts.** When later edits overlap, the bar says so in `warning` and each conflicting file has a "Show the overlap" disclosure with the merge markers in a `surface-inset` block. The only way forward is the danger button `Overwrite anyway`.
- **Undo.** After an apply, the bar turns into a status line with a `success`-tinted border ("Reverted 1 file.") and a secondary `Undo` button. Undo can conflict too, and then offers `Undo anyway`. Only the last revert has an Undo.
- **Marks.** A turn's diff never changes after a revert, so reverted hunks and files carry a small caps `reverted` badge (`surface-active`, 10px mono), read from the session's revert list.
- **Review notes (P1 PR7).** In `This turn` scope, each hunk header also has `Comment`, and the diff header has `Comment on file`. Either opens a small form above the diff (`surface-elevated`, `border-default`) with a textarea and `Add note`. Notes wait above the composer as chips: mono `text-xs`, a `primary` tint for comments and a neutral `surface-elevated` chip for reverts (each applied revert adds one per hunk or file; undoing it takes them back), each with a × to drop it. The panel says how many notes will go with the next message, in `primary-text`. Sending a message (not a slash command) attaches them; the server appends one `<review-notes>` block with each note's code, and the thread folds it under the message as a mono `Review notes (n)` disclosure.
- **Waiting.** Revert buttons are disabled while a turn streams. A server that is still running a turn answers "A turn is running. Try again when it finishes."

### Session board (home, #971)

`SessionBoard.svelte` on `/console`. One request, `GET /v1/chat/board`, returns every visible main session with its server status, pending approvals, last turn time, working folder, repository top level, branch, the latest turn that changed files (from its checkpoint), and this month's cost; the board refreshes every 15 seconds and whenever the activity store sees a change.

- **Header**: title and subtitle on the left; on the right a ghost **Notify me** toggle (hidden without the Notification API, replaced by a dim note when the browser blocks it) and the screen's one `button-primary`, **New chat**.
- **Controls**: filter chips (All, Needs input, Running, Done · unread, Idle), each with a mono count; the active chip takes the accent border and `primary-muted` fill. A search field (title, repository, branch, folder) and a Sort select (Recent, Status, Title, Cost). Filter and sort persist per browser.
- **Groups**: one section per repository, headed by the folder name in mono `text-sm` and a session count; groups with sessions waiting for input come first, then the most recently active; "No working folder" comes last.
- **Cards**: a `surface` card per session in an auto-fill grid (min 260px): title and status badge (`warning` needs input, `accent` running, `info` done · unread, `default` idle); branch in mono; then mono `text-xs` facts: approvals waiting and unattended approvals waiting in the ops queue (both in `warning`; the second has a title saying where to answer), the latest change `n files +a −d` (`success`/`error`), running time or time since the last turn, and this month's cost. A card waiting for input takes a `warning` border. Pinned sessions lead their group. Clicking a card opens its chat.
- **Done · unread** is per browser: the console remembers in localStorage when each session was last on screen (and, for sessions never opened here, when it started keeping track). A finished turn after that reads as unread.
- **Live state elsewhere**: the chat sidebar shows the same `running` / `needs input` states as tiny mono badges on each row, from the activity store (`lib/stores/sessionActivity.svelte.ts`), which polls `GET /v1/chat/activity` every 4 seconds and at once on an `approval` event. With notifications on, it sends a browser notification when a session starts waiting for input or finishes, unless that session is on screen; clicking it opens the chat.

### Focus mode (#1068, `docs/decisions/focus-mode.md`)

A second console mode next to the workbench (Advanced): one development pipeline per chat session, shown as where we are, what the AI did, and what the developer must decide. Routes `/console/focus` (home) and `/console/focus/<session>`; components in `src/components/focus/`, pure helpers in `lib/focus.ts` (PR stages in `lib/focusPR.ts`), screen state in `lib/stores/focusStore.svelte.ts`, strings in `src/i18n/sections/focus.ts`.

- **Default mode**: `console_default_mode` (`focus` | `advanced`, empty = advanced). With `focus`, the *landing* on `/console` goes to the focus home; the board stays in the nav. Finishing the onboarding wizard (not re-entry) writes `focus`. In the English UI the workbench's distraction-free toggle is "zen mode" so the two never share a name.
- **Home**: header with the screen's one `button-primary`, **New task** (folder — recent folders, a typed/pasted path, or **Browse…** opening the shared folder picker dialog (see above) at the checked folder or home, also on every extra-folder row; the same rules as New chat in a folder — a one-line goal, and *Isolate in a worktree* in a git repository). One `surface` row per pipeline: title and goal, then a `default` badge for the current stage, `success` *finished*, `warning` *waiting for you* while a gate is open, and a mono `warning` count of cards needing input. A row waiting for input takes a `warning` border.
- **Chrome**: focus routes hide the app sidebar and the companion; the top header stays. Each focus screen leads back out: the home has *Open the board*, the pipeline screen *All tasks*, **View in Advanced** and *Open the board*.
- **Screen**, top to bottom, max-readable width (880px, `min-width: 0` so wide content never widens the page): header (← All tasks, title, mono cwd and — for an isolated pipeline — an `accent` `⑂ branch` chip for the session worktree, secondary **View in Advanced**, *Open the board*, ⋯ menu with *Stop pipeline* behind a confirm), the stepper with a ghost *Mark stage done* when the current stage has no open gate (never for plan), one progress line while a turn runs, the card deck, and the stage instruction input (disabled while a turn runs on the session, wherever it was started). Hidden here: dock, tool cards, sidebar, tier/cost/permission details.
- **Stepper**: mono chips joined by 1px lines. Done ✓ in `primary`; current takes the accent border and `primary-muted` fill with a `↻N` badge from round 2; pending ○; skipped dashed and struck through; blocked in `warning`. Clicking a stage shows its card history, with *Back to the current stage*.
- **Deck**: exactly one card, `←`/`→` and `n / m` in mono. The order on screen holds while the same cards stay (`deckOrder`; a card marked seen does not move), so each arrow is one card; cards arriving or leaving re-sort it, and a newcomer takes the screen only when it outranks the card being read — a gate over a report, a new report over one already seen, anything over a handled card — never because change cards finished loading under it (`deckCursor`); a handled card hands over to the next one. While the current stage has no cards yet (its first turn runs), the deck keeps showing the last stage that has some. Order: gate, decision, finding, failure, notice, report, change; unseen before seen; gates, decisions and findings oldest first (the order asked), informational cards newest first (the latest report leads even once both are seen); decided last. Gates, decisions and findings are handled one by one (accent border while open); reports, changes, failures and notices offer *Acknowledge* and, together, *Acknowledge the remaining n* (failure/notice cards take a `warning` border). Keys: `1`–`9` choose a decision option, `?` asks about the card, `o` toggles **View raw** — the source turn read-only, drawn by `ChatMessageItem`; long lines wrap, code blocks and tables scroll inside the slice.
- **Cards** are `surface`, `lg` radius, `xl` padding, a `default` kind badge, the title (sentence case in Plex Sans, one line with an ellipsis — not the label-caps `.card-title`), the turn in mono and an `accent` *new* / `default` state badge. G1 (plan gate) lists tasks with their done criteria, the optional stages as toggle chips (unticked = dashed, struck through), the verification commands in an editable mono textarea, and Approve (`button-primary`) / Request changes (secondary, opens a note) / Stop (danger). A decision lists its options as numbered buttons plus a free answer. A change card is one turn's checkpoint diff ("Changes in turn N"): a mono summary (file count, +/−), the turn's files as a compact list (current file with an accent border), and one file's diff at a time in `DiffView` (max 420px tall) with ‹ › and `n / m`; it is acknowledged as a whole, so a busy build adds one card per turn, not one per file.
- **Progress line**: mono `text-sm`, a pulsing `primary` dot (static under reduced motion): stage verb · files changed · what runs now ("Implementing · 3 files changed · running tests"). From P2 the server drives the loop: between turns it runs the plan's verification commands on the same feed, so the line reads "Implementing · verifying make test (1/2)". Under it, while the server's turn runs, one `text-xs` tertiary line labelled *Sent by the pipeline* shows that turn's prompt (one line, ellipsis) — the console never sends it.
- **Build loop cards (P2)**: a *Verification failed* card shows `$ command → exit` in mono, the round in muted text, and the output excerpt in a mono `pre`; it is informational (the next turn fixes it). The **blocked gate** ("Build blocked") states the reason in one sentence (loop limit, same failure twice, or no progress), repeats the failing command and excerpt, and offers **Try once more** (`button-primary`), **Give an instruction** (secondary, opens a note sent with *Send instruction*) and **Stop** (danger). The stepper shows the stage `blocked` (`warning`) while it is open.
- **Interrupted and failed steps (P2 review)**: a server restart that cut a step off raises the same blocked gate titled *Pipeline interrupted*, and a server turn that failed (provider timeout) raises *Turn failed* with the error in a mono `pre`; both show the owed turn's prompt in muted text, and **Try once more** resumes exactly that step (the turn, or the verification) in the same round. Nothing resumes on its own.
- **Review loop cards (P3)**: a review turn's findings open the triage gate and arrive one card each, handled one at a time like decisions. A finding card shows a severity chip (`error` high, `warning` medium, `default` low, label localized), `file:line` in mono, the failure scenario, and — when the server found it — the diff since the stage started around that line in a mono `pre` (added lines `primary-text` on a faint `primary` tint, removed lines `error` on `error-muted`, hunk headers tertiary, the finding's own line marked by a 2px `warning` left rule and semibold). Actions: **Fix** (`button-primary`, small), **Dismiss** (secondary), and a ghost **Ask** with a `?` key hint that opens the card's Q&A drawer. While triage is open the deck bar shows a mono `warning` *k / n decided*; the last decision closes the gate — accepted findings go to one fix turn, and verification (verify and end-to-end commands) runs after it. A review that keeps needing fixes past its limit raises the blocked gate titled *Review blocked*, whose retry starts a new review round.
- **Your own finding (#1196, `FocusAddFinding.svelte`)**: between the deck and the stage input, only where a round takes it (`lib/focus.ts` `findingEntry`: a review stage with no gate or triage open, or pr_review with no gate open, on the current stage's deck) — a ghost small **+ Add a finding**. It opens a `surface` box (`md` radius, `border-subtle`) with the shared input visual: a one-line title (required, focused on open), a two-row scenario textarea, then one wrapping row of a mono file path, a narrow mono line number and a severity select (medium by default). Actions: **Add to fix** (`button-primary`, small; also Enter in a field) adds it already decided as fix, **Add, decide later** (secondary) adds an open card to decide like the agent's, Cancel (ghost) closes the box; one `text-xs` tertiary line says which round it joins. Adding closes and clears the box; a refusal (the stage moved on) shows the *changed elsewhere* banner. The card is an ordinary finding card with an `accent` *added by you* badge after the location, counts in triage progress, and gets the same diff excerpt when it names a file. It stays available while a turn runs — a card is not a turn.
- **Q&A drawer (P2)**: under the card, a ghost *Ask about this card* toggle (a mono count of questions when closed) opens a `surface-inset` box with `md` radius: the card's questions oldest first, each answer indented behind a 2px `primary`-tinted rule with a ghost **Promote to instruction** that puts *About "card": answer* into the stage input as an editable draft; then a question box with a secondary **Ask**. `?` opens the drawer and focuses the box (on this screen the global shortcut help yields `?`; it stays on every other route). Questions thread by card and survive reloads (the server keeps the card → turn map); answers come from a hidden plan-mode session and never change the pipeline. While one is being answered the row shows *Answering…* and a second question waits.
- **PR stages (P4)**: facts come from the server's read-only `gh pr view` probe (`lib/focusPR.ts`). **G3** (*Open the pull request*, `FocusPRGate.svelte`) shows the agent's draft as an editable title input and body textarea (`surface-inset`, accent border on focus, a `warning` hint when the title is empty) and **Open PR** (`button-primary`, disabled without a title) / Request changes / Stop; decided, it shows the text read-only. Approving has the agent push and open the PR; until the probe finds it, one mono line with the pulsing dot reads *Waiting for the pull request to appear* (likewise *for the fix to be pushed*, *for the merge*). The stepper's pr, pr_review and merge chips gain a mono `#N` link to the PR beside the chip and, inside the chip on pr_review/merge, **CI chips** (`FocusCIChips.svelte`): `✓n` on `primary` at 15%, `✕n` on `error-muted`, `…n` on `surface-elevated`, each titled with the full count. CI failures and review comments arrive as ordinary finding cards. **G4** (*Merge the pull request*, `FocusMergeSummary.svelte`) is a summary of facts — the `#N` link and title, the CI chips (or *No checks reported*), *Findings: n fixed, n dismissed, n open*, the merge state in mono — with **Merge** (`button-primary`) / Request changes / Stop. When the latest probe could not run gh, a `warning-muted` banner names gh's error and offers a secondary **Pass by hand** (the same manual pass as *Mark stage done*, refused while a turn runs); the next probe that runs takes the banner down (the stage's notice card stays in the deck). A finding from a commenter who is not the PR's author or a repository owner, member or collaborator carries a muted line saying the fix turn links to it but does not quote it. A finished pipeline with a merge stage says only what the server recorded, in the `done` banner: *Merged. The session worktree was removed.*, *Merged.* plus *The session worktree is kept on its branch: <reason>.* (uncommitted changes, commits the PR does not hold, a merge passed by hand), *…was left as it is: <reason>.*, or just *Merged.* / *The pipeline is complete.* when there was no worktree. A PR stage that waits too long, a PR closed unmerged, or fix rounds at their limit raise the blocked gate titled *Pull request blocked*.
- **Templates and goal mode**: the New task form gains a **Template** select (`surface-inset`, the input's border and focus rule) listing the built-in templates in the console's language and workspace ones as written (suffixed *from focus-templates*), with one `text-xs` tertiary line under it — the description and *Stages: Outline → Draft → Revise* — and a **Goal mode** checkbox whose hint line (what gets approved automatically) shows only when ticked. A template pipeline's stepper, graph, plan gate and history label use the template's stage names; its blocked gate is titled *<Stage> blocked*. On the pipeline screen the header carries a **◎ Goal mode** toggle (`aria-pressed`): ghost when off, `button-primary` when on, with the retries used of the budget (`3/20`, mono `text-xs`, the button's own colour at 80%) once any are spent. Turning it on asks for confirmation, since it approves the merge too. When goal mode ended without finishing (retries used up, PR closed, turn cancelled) a plain banner above the stepper says why. The home list marks such tasks with an accent *Goal mode* badge.
- **Hidden blocks**: user messages carry the server's `<focus-stage>` guidance and replies carry `<focus-plan|report|findings|pr>` blocks. Every chat surface (thread, side session, auto titles, view raw) folds them away; a block inside a code fence stays.
- **Advanced**: a session with a pipeline shows a secondary **Focus view** button in the chat header.
- **Release train** (`/console/focus/release`, `FocusReleaseTrain.svelte`, P5): reached from a ghost *Release train* button on the home. One `surface` group per repository (mono short path, a `default` badge *since vX* or *no release tag yet*, a mono count), listing the pipelines finished since the latest `v*` tag oldest first: `#PR title` linked to the PR when there is one, else the session title opening its pipeline, the goal's first line in `text-secondary`, the finish date in mono. When the server could not fetch tags from the remote, the group shows a `warning-muted` banner naming `git fetch --tags` (the list may hold released work). Release pipelines are never listed. While a repository's release pipeline runs, its group shows mono `primary-text` *release in progress* and a secondary **Open release** in place of Start release; a Start from a stale tab that the server refuses (409) opens the running release. Each group's **Start release** (`button-primary`, small) opens an inline gate — accent border on `primary-muted`, the fixed plan in one sentence, Cancel / Start release — and nothing starts until it is confirmed. Starting creates an isolated focus pipeline of kind `release` with a one-line goal and a first turn (`kickoff`) carrying the fixed plan and the merged list (`lib/focusRelease.ts`); its plan gate still runs. Empty state is the dashed `.empty`; a load failure is the error banner.
- **Pipeline graph** (`FocusGraph.svelte`, P5): a ghost *Graph* toggle beside the stepper opens a read-only `@xyflow/svelte` canvas (`surface-inset`, `colorMode="dark"`, no attribution, no drag/select/connect) under the stage bar, as tall as its content. Readability first: node text is 13px and the view never zooms below 0.85, so text is at least 11px on screen; when one row of stages would need a smaller zoom (a pane under about 900px), the stages wrap into rows that snake (the next row turns back under the last stage, so the wrap edge is a short drop — `focusGraphColumns`). Stages use the stepper's colours (done `primary` at half strength, current accent border on `primary-muted`, blocked `warning`, skipped dashed at 0.6 opacity); forward edges are `border-strong` and dashed into or out of a skipped stage. Each looping stage (build, review, PR) has a U-shaped self-loop under it labelled in mono `↻N` or `↻N/limit`, `primary` from the second round. Plan tasks sit inside the build node as numbered `surface` rows. The horizontal zoom controls sit bottom-left under the last row, on `surface-elevated` with `border-default` (xyflow's light defaults are overridden through its `--xy-*` variables). Nodes come from the pure `lib/focusGraph.ts`.

### Follow-up queue (composer, #971)

While a turn runs, the composer queues instead of sending (`lib/stores/messageQueue.svelte.ts`, per session). The placeholder says so, Enter queues, and a `button-secondary` **Queue** appears beside **Stop** once something is typed. A slash command picked from the menu mid-turn queues too, since running `/compact` or `/new` then would cut the turn off.

Queued messages sit in a `surface` box above the composer: a mono `text-xs` count, a hint (or a `warning` **Paused** badge), and **Clear** on the right; then one row per message, with the text truncated to a line, a file count when it carries attachments, and ghost **Send now**, **Edit**, and **×** buttons. A paused queue takes a `warning` border.

- When a turn ends on its own, the next message sends, one turn each; the draft in the composer is never swept in.
- **Stop**, a failed turn, or reopening a chat with messages still queued pauses the queue; **Resume** sends again.
- **Send now** moves a message first and stops the running turn so it goes next.
- **Edit** moves a message back into the composer; anything already typed takes its place at the end of the queue.
- Queues live in memory for the page's lifetime. Text, files, and mentions are kept, so a queued message sends exactly as typed.

### Goal chip (chat session header)

Sits after the health badge in `.session-title-row` whenever the session has a `SessionGoal` set via `/goal <description>`. Same dimensions as the cwd chip (3px / `space-2` padding, `radius-sm`, `font-mono` body). Default state is **tinted green** — `border` and `text` use `primary` mixed with the muted accent, signaling that the session is steering itself toward an autonomous target. The label `goal` reads in a dim caps font, the truncated description in green, and a small `0/3` style counter at the trailing edge shows auto-continue progress (`auto_continue_count / max_auto_continues`). The chip flips to neutral grey when the goal is cleared after a `satisfied` verdict and to a warning tint when the budget is `exhausted`. Click invokes the same handler as `/goal status` and prints the full description plus remaining budget into the chat feedback strip. The chip is hidden when no goal is set.

### Source badges (Session Config panel)

Tools and skills that the session inherited from a `.tars/settings*.json` override file are tagged with a tiny `source-badge` chip on the right side of `.config-item`. The badge uses 9px display-font caps with 4–5px padding so it fits inside the existing config row without breaking layout. **`shared`** entries (from `.tars/settings.json`, the team-shared file) render in the accent color so the user immediately spots project-level overrides; **`local`** entries (from `.tars/settings.local.json`, the per-user gitignored file) use a neutral grey to avoid implying parity with shared. Items whose value comes from the session base (`sessions.json`) render no badge — keeping the row visually quiet for the common case. Hovering a badge reveals the full file path the override came from via the native `title` attribute.

### Input fields

Text inputs and textareas share a single visual: `surface-inset` background, `border-default` outline, `body-md` text. On focus the border switches to `primary`. There is no fill change on focus — the accent border carries the affordance.

### Empty state

A centered `text-tertiary` block with `body-sm` typography, padded `3xl`. Always include both a one-line "what's missing" headline and a follow-up sentence with the next action ("Run a pulse to start collecting signals"). Empty states without action guidance feel like dead ends.

### Error banner

`error-muted` background, `error` text, full-width inline. Use for synchronous form validation errors and API failure surfacing. Do not use a banner for confirmation messages — that's what `success`-tinted toasts/badges are for.

## Onboarding wizard

The wizard (`src/components/Onboarding.svelte`) is a section-router shell that delegates each section to a child component under `src/components/onboarding/`. The shell owns the form state and the cross-section orchestration; each child owns its own UI and emits navigation callbacks (`onNext`, `onBack`, `onSkip`).

### Modes

- **Quick** — first-run default. Sections: Provider → Tiers → Review → Complete. Fastest path to a working console.
- **Full** — reentry default and the path users opt into via the "Configure more" CTA on the completion screen. Adds **Tools & Permissions**, **Integrations**, and **Channels** between Tiers and Review. Each new section can be skipped without writing.

The mode badge appears in the header. Switching from Quick to Full only adds sections after Tiers; previously-saved provider+tier values are kept.

### Deep links

`/console/onboarding?section=<id>` deep-links into a section on reentry. Supported ids: `provider`, `tiers`, `tools`, `integrations`, `channels`, `review`. The shell rejects an optional-section deep-link when no provider is configured (would write a useless partial config) and falls back to the Provider step.

### Section save semantics

Provider/Tiers/Review save the LLM block via the existing alias-replace `buildConfigPayload`, which preserves untouched providers. Tools/Integrations/Channels each save their own keys via `buildSectionPayload(section, form)` — a partial PATCH that does not touch other sections. This makes per-section reentry safe (editing Channels never disturbs Tools).

Secrets follow the existing `keepExistingApiKey` pattern: when the schema endpoint returns a masked value, the wizard sets a `keep*Key` flag and drops the field from the patch payload, preserving the on-disk credential.

### Completion matrix

`OnboardingComplete.svelte` renders a row per capability — LLM provider, tier bindings, web_search, web_fetch, memory embeddings, telegram, webhook — with a ✓ / ✗ / — glyph and a contextual "Edit <section>" jump-back link. Status derives from in-memory form state first (so it reflects what the user just edited) with `setupStatus.capabilities` as a fallback for refresh-after-save scenarios.

The matrix surfaces a "restart required to activate Telegram/Webhook" notice when those workers were saved during the current run — they only start after a server restart per the setup-only-mode constraints in `internal/tarsserver/main_serve_api_setup.go`.

## Do's and Don'ts

- **Do** use `primary` (green) only once per screen, for the single most consequential action.
- **Do** layer surfaces tonally (`surface-base` → `surface` → `surface-elevated`) instead of reaching for shadows or thicker borders.
- **Do** pair muted-background semantic tokens (`success-muted`) with solid foreground (`success`) — never the reverse.
- **Do** keep the two font families (IBM Plex Sans / IBM Plex Mono); each has a defined role.
- **Don't** introduce a light theme. The tonal palette and green accent are calibrated for dark backgrounds.
- **Don't** bring back warm accents (amber, terracotta, orange) or warm-tinted greys. That styling was retired because it read as a consumer AI chat app.
- **Don't** mix sharp corners and pill shapes in the same view; use the `sm`/`md`/`lg` radii consistently.
- **Don't** use `text-ghost` for any copy that the user is meant to read — it's for purely decorative micro-labels.
- **Don't** add another button variant before exhausting the existing five; new variants dilute meaning.
- **Don't** stack two muted semantic backgrounds (e.g. `success-muted` over `warning-muted`) — they fight for attention.
