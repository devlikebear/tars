# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog and the project follows Semantic Versioning.

## [Unreleased]

## [0.31.163] - 2026-05-05

### Added

- **Session-cwd override loader + merger (`internal/sessionoverride`)** — new package that reads `<cwd>/.tars/settings.json` (team-shared) and `<cwd>/.tars/settings.local.json` (per-user) into a typed `Override`, then deep-merges them with the session's base `tool_config` / `prompt_override` to produce an `EffectiveConfig`. The schema is allow-list driven (`tool_config`, `prompt_override`, `mcp_servers_extra`, `model_tier_override`); a parallel block-list (`llm_providers`, `api_key`, `auth*`, `hooks`, `server_command`) generates `error`-severity diagnostics so credentials and hook registration can never be smuggled into a checked-in file. The merger tracks per-path provenance in a `sources` map (base/shared/local) — Phase 6's UI badges depend on that; collection fields like `tools_enabled` are union-deduped while scalars are last-write-wins. Closes [#705](https://github.com/devlikebear/tars/issues/705); refs Epic [#703](https://github.com/devlikebear/tars/issues/703).

## [0.31.162] - 2026-05-05

### Added

- **Session active-cwd transition API** — chat sessions can now switch their active working directory among the candidate set (artifact dir + registered `work_dirs`) via `GET /v1/admin/sessions/{id}/cwd` and `PUT /v1/admin/sessions/{id}/cwd`. The server returns the current dir alongside the eligible list, validates membership before persisting, and emits a `session` SSE notification on success so subscribers can refresh derived state. New `session.Store` helpers (`EligibleCwds`, `GetCurrentDir`) plus exported sentinels (`ErrSessionNotFound`, `ErrCwdNotEligible`) make the contract explicit. Foundation for the broader `.tars/` session-scoped overrides epic ([#703](https://github.com/devlikebear/tars/issues/703)); this PR closes [#704](https://github.com/devlikebear/tars/issues/704).

## [0.31.161] - 2026-05-04

### Fixed

- **Console — every page now fills the viewport instead of leaving a dark gap below the content** — `.chat-page` was sized with `height: calc(100vh - var(--header-height))` while sitting inside `.shell-content`, which has 24px of vertical padding (16px on mobile). The page overflowed the viewport by ~48px, pushing the composer below the fold and leaving the bottom of the chat looking cut off. Reworked the shell ancestry — `.shell { height: 100vh }`, `.shell-main { min-height: 0 }`, `.shell-content { display: flex; flex-direction: column; min-height: 0 }` — so flex children get a definite-height context, then converted each top-level page (Chat, Session Lineage, Memory, System Prompt, Extensions, Channels, Cron, Settings) to `flex: 1; min-height: 0` with internal `overflow-y: auto` where appropriate. Long-content pages (Logs) still scroll naturally via the body. The chat layout now fits without overflow on desktop (1280×1300) and mobile (600×900).

### Added

- **Console i18n — Korean coverage for 7 previously English-only pages** — Session Lineage (분기), Plans (계획), System Prompt (시스템 프롬프트), Extensions (확장), Agent Runtime (에이전트 런타임 — main runs view), Channels (채널), and Settings (설정 — main shell). Added matching namespaces to `i18n/types.ts`, `en.ts`, and `ko.ts` and routed every visible UI string through the `t` store so locale toggles take effect immediately. Field-level labels in `lib/quickStartFields.ts` and the deeper Agent Runtime subagent builder remain English and are tracked for a follow-up; the configWizardCard `kicker` now reads `설정 마법사` in Korean instead of leaking the English label.

## [0.31.160] - 2026-05-04

### Added

- **Console Git Inspector — Fetch button + remote-branch checkout** — the Branches tab previously only let you switch between local branches; remote rows just showed a "remote" badge with no action. Added a new `fetch` mutation (`git fetch --all --prune`, non-destructive) wired to a Fetch button at the top of the Branches tab, and replaced the inert "remote" badge with a Checkout button that strips the matching remote prefix (`origin/feat/foo` → `feat/foo`) and reuses `switch_branch`. Git's DWIM creates a local tracking branch on first checkout, so subsequent switches go to the local copy. Branch rows now show the short name as the title with the full ref in a smaller line below, so long remote-branch names stay readable in narrow docks.

## [0.31.159] - 2026-05-04

### Fixed

- **Console Git Inspector — diff fills the available panel height** — the diff table was capped at `max-height: 480px`, so on tall viewports it stopped halfway down the panel and the rest of the Files tab sat empty. The diff section now becomes a flex child that fills the remaining tab body (`.diff-section { flex: 1 1 auto }`) and the diff table itself takes the remaining vertical space inside it (`flex: 1 1 auto; min-height: 0`) with internal scroll. A `min-height: 240px` keeps the diff legible when the panel is short.

## [0.31.158] - 2026-05-04

### Fixed

- **Console Git Inspector — Files tab no longer pushes the diff to the bottom of the panel** — when the working tree was clean and a commit-mode diff was loaded, the "Working tree clean." placeholder stayed at the top of the Files tab while the diff section sat at the very bottom with a large empty band between them. `.tab-body` was a CSS Grid whose default `align-content: normal` (= `stretch`) spread the two auto-sized rows apart instead of packing them. Switched `.tab-body` to `display: flex; flex-direction: column` so children stack at the top and grow only as needed; `.files-body` was a no-op once the column override went away, so it was removed.

## [0.31.157] - 2026-05-04

### Changed

- **Console Git Inspector — readable diff renderer + scrollable panel** — the diff view used to dump the raw unified patch into a single `<pre>` (Unified) or two collapsed-context panes (Split), so add/remove rows looked identical and you couldn't tell which line changed. Diff is now parsed line-by-line and rendered as a structured grid: per-row line numbers (old + new in Unified, side-by-side in Split), a sign column, and color-coded backgrounds (green for adds, red for dels, accent for hunk headers). Hunk headers come from `@@ -a,b +c,d @@` so the line numbers stay accurate even for diffs that skip the file header. Split mode pairs consecutive `-`/`+` runs so changed lines line up across columns. Below 600px the Split pane collapses to a single column so it stays readable in narrow docks.
- **Console Git Inspector — fix overflow when expanding tall commit rows** — clicking a commit at the bottom of the Log used to expand the file list past the panel's bounds with no way to scroll, so the action row got cut off. The panel is now a flex column where the header / banners / tab strip are pinned and only the active tab body scrolls. Diff blocks gain their own `max-height: 480px` so the tab body never grows unbounded inside the dock.

## [0.31.156] - 2026-05-04

### Fixed

- **Console Git Inspector — commit-file click now renders diff on a clean working tree** — clicking a file inside an expanded Log commit row would silently land on an empty Files tab whenever the working tree was clean. The diff section was nested under the `files.length === 0` else-branch (introduced in P2 of #692), so the "Working tree clean." placeholder swallowed the entire diff view including any pending commit-mode diff. The diff section is now rendered as a sibling of the file list and shows whenever a diff is loaded, currently loading, or a commit-mode source is selected — independent of whether the worktree has changes.

## [0.31.155] - 2026-05-04

### Added

- **Console Git Inspector — checkout commit + worktree add/remove via approvals (P3 of #692, closes #692)** — three new mutation actions plug into the existing approval workflow: `checkout_commit` (`git checkout --detach <hash>`, optionally `-b <new>`), `worktree_add` (`git worktree add [-b <new>] <path> [<branch>]`), and `worktree_remove` (`git worktree remove <path>`). Detached checkouts and worktree removes are flagged `destructive` so the approval UI can warn before applying. The Log tab gains a per-commit input + Checkout button (red "detached" variant flips to a safe "Checkout as branch" once a name is entered). The Worktrees tab gains an Add form (path + optional existing/new branch) and per-row Remove buttons (current and bare worktrees omit the action). Backend additions (`internal/git/client.go`, `internal/ops/git_mutation.go`): `MutationCheckoutCommit / MutationWorktreeAdd / MutationWorktreeRemove`, `MutationOptions.Hash / WorktreePath / NewBranch`, plus matching `GitMutationPlan` fields and `Command` strings. Audit details now include `hash / worktree_path / new_branch` so the trail captures what was actually queued.

## [0.31.154] - 2026-05-04

### Added

- **Console Git Inspector — commit details, file diffs, worktree listing (P2 of #692)** — Log entries are now expandable: clicking a commit row fetches the new `GET /v1/git/commit?hash=` endpoint and shows the commit body plus its changed files with `+adds / −dels` per file. Clicking a file in that list opens the diff for that path at that commit (the existing Diff endpoint now accepts a `hash` query param and shells out to `git show --format=`). A new `Worktrees` tab calls `GET /v1/git/worktrees` (parsing `git worktree list --porcelain -z`) and shows each worktree's path, branch, HEAD, and any `detached / locked / prunable / bare` flags, with the current worktree highlighted. Backend additions (`internal/git/client.go`): `Client.CommitDetail`, `Client.Worktrees`, `DiffOptions.Hash`, plus parsers for `diff-tree --name-status -z` + `--numstat -z` (renames preserved, binary detected, initial commits handled via `--root`). All read-only — no new mutations; checkout / worktree add / remove still land in P3.

## [0.31.153] - 2026-05-04

### Changed

- **Console Git Inspector — layout & UX refactor (P1 of #692)** — the chat Git panel was a single long-scrolling column where every section (status / files / diff / branches / log) rendered at once, and the 5-column repo-summary grid forced the ROOT path to wrap one character per line in a narrow dock. The panel is now reorganized into a tab strip — `Status` / `Files` / `Branches` / `Log` — with a compact header that inlines branch · HEAD · Δ/S/U counts, dropping the bulky summary cards. The Files tab keeps the file list + diff together (most common pairing) and adds a Unified / Split toggle for the diff view (Unified by default; Split collapses to a single column under 600px). Status meta uses `repeat(auto-fit, minmax(140px, 1fr))` so cards reflow instead of stretching. No backend or API changes — read-only commit details, worktree listing, checkout, and worktree mutations land in P2/P3.

## [0.31.152] - 2026-05-04

### Fixed

- **Console terminal — survives dock zone moves without restarting** (#667) — dragging the integrated terminal between dock zones (left / right / bottom / fullscreen) used to unmount and remount `TerminalTabs`, which closed the WebSocket and tore down the running shell along with its scrollback. The terminal panel is now rendered from a single, stable parent at the chat-layout root with a `data-zone` attribute; CSS picks the matching `grid-area` (or fullscreen overlay positioning) per zone, so xterm + the PTY survive zone changes intact. The regular dock-pane wrapper is suppressed for whichever zone currently hosts the terminal to avoid duplicate panes; resizers stay on so the active pane can still be resized.

## [0.31.151] - 2026-05-04

### Added

- **Console terminal — font picker + larger default size** (#686) — added an `Aa` toolbar button next to `Find` that opens an inline settings panel with a font-family dropdown (JetBrains Mono / SF Mono / Consolas / Cascadia Code / Fira Code / system default), a font-size range slider (8–24), and a Reset button. Each preset's CSS string carries a system-monospace fallback so unavailable fonts degrade gracefully without breaking xterm's monospace assumption. Selections persist via `tars.terminal.fontFamily` / `tars.terminal.fontSize` in localStorage; existing Cmd/Ctrl +/-/0 zoom shortcuts continue to work and stay in sync with the slider.

### Changed

- **Console terminal — bumped default font size from 12px → 14px** (#686) for readability on hi-DPI panels.

### Fixed

- **Console terminal — refit after web fonts settle** (#686) — JetBrains Mono is loaded as a webfont, so xterm's first glyph-metric measurement could land on the fallback and produce uneven character spacing until the next resize. After mount we now await `document.fonts.ready` and re-fit + refresh, bringing xterm's metrics back in sync once the real font is available.

## [0.31.150] - 2026-05-04

### Fixed

- **Console terminal — context menu clamps to the frame edge** (#669) — right-clicking near the bottom or right edge of the integrated terminal used to render a menu that extended past the visible area, hiding "Clear" / "Save buffer". After the menu mounts we now measure its size and clamp `menuX`/`menuY` so it stays inside the terminal-frame wrap with a small margin.

## [0.31.149] - 2026-05-04

### Fixed

- **Console terminal — Esc closes the right-click context menu** (#668) — when the integrated terminal had focus, pressing Esc with the context menu open used to forward an ESC byte to the shell and leave the menu open. The custom xterm key handler now swallows Escape and dismisses the menu before xterm sees it. Clicking outside the menu (the existing overlay) still works as a secondary dismissal path.

## [0.31.148] - 2026-05-04

### Fixed

- **Onboarding wizard — Gemini base_url 404** (#671) — `defaultBaseURLForKind` in the wizard now returns the full backend-canonical paths (`/v1beta/openai` for `gemini`, `/v1beta` for `gemini-native`) instead of the bare host. Previously every chat/model call after a wizard-driven Gemini setup failed with `gemini status 404` and an empty payload. Added a regression test that pins both URLs to the backend `llmdefaults` constants. Existing users whose config was written by the broken wizard need to either re-run the wizard with `?reentry=1` and Apply, or edit the `gemini` provider's `base_url` directly in Providers.

## [0.31.147] - 2026-05-04

### Changed

- **Console chat — full i18n coverage** — extracted all hardcoded English strings in the chat surface (`Chat.svelte`, `ChatPanel.svelte`, `ChatMessageItem.svelte`, `SessionSidebar.svelte`) into the `chat.*` and `sessions.*` translation namespaces. Covers status strip (pulse ticks / last tick / unread), dock panel buttons + tooltips, session header actions (rename / AI title / compact / copy / download / extract skill / delete), plan strip (label / open title / progress aria / active task tooltip), tier recommendation card, mention menu, drop overlay, message roles + usage badge + copy/fork buttons, sidebar search/filter/sort, relative-time formatter, and all error/feedback toasts. Korean translations added for every new key.

## [0.31.146] - 2026-05-04

### Added

- **Run ↔ Task linkage + live "currently working on" indicator** (#679) — when an agentruntime run is spawned with a `task_id`, that ID is now preserved on the resulting `Run` record (`Run.TaskID`) so external clients can correlate run state with the task that triggered it. The session-side `Task.RunID` field complements the link in the other direction. Wired through `SpawnRequest.TaskID` → `Runtime.Spawn()` → `Run.TaskID` and exposed on the `POST /v1/agentruntime/runs` body.
- **Chat plan strip — active task title** — `summarizeTasks()` now surfaces the title of the first in-progress task as `active_task_title`. The chat plan strip renders it next to the goal with a small pulsing dot so the user can see at a glance which task the session is actively working on.

## [0.31.145] - 2026-05-04

### Added

- **Pulse — goal-driven plan auto-continue** (#680) — when a chat session's plan transitions to `completed` and the new `Plan.AutoContinueEnabled` flag is set, pulse can run one chat turn that asks the LLM to either declare the goal achieved (terminating the loop by clearing the flag) or propose a follow-up plan. The cap on iterations is enforced via the automation audit log over a 24-hour rolling window so it survives plan replacement when the LLM proposes a new plan to keep working.
  - New signal kind `auto_continue_goal` in `internal/pulse/signal_auto_continue_goal.go` and matching autofix `auto_continue_goal_plan` in `internal/pulse/autofix/`.
  - **Hard safety bounds**: per-plan `AutoContinueMaxIterations` (default 5, hard cap 10), `AutoContinueIterationWindow` (24 h), session-level escalation when the cap trips → `AutoContinueEnabled` is automatically cleared so detection cannot re-fire on every pulse tick.
  - **Termination semantics**: the LLM is instructed to flip `auto_continue_enabled` to `false` when the goal is reached. The controller re-reads the plan after each turn and classifies the outcome as `goal_completed`, `next_plan`, or `no_change` (no clean decision — re-attempt next tick up to the cap, then escalate).
  - **Opt-in**: gated by both `SessionAutomationConsent.AutoResumeEnabled` (existing per-session consent) and the global `pulse.allowed_autofixes` allow-list. Not in the default allow-list.
  - **Out of scope (deferred)**: `TaskContract.VerificationCommands` execution (currently still LLM-facing metadata only), per-goal token budget guard, frontend toggle UI, plan-repetition detector. Tracked in #680 follow-ups.

## [0.31.144] - 2026-05-04

### Added

- **Pulse — auto-resume halted chats** (#678) — pulse now detects chat sessions whose last activity is a halted turn (a tool error with no follow-up assistant message, or a user message the LLM never finished responding to) and can retry them via a new `auto_resume_failed_chat` autofix. This complements the existing `auto_continue_chat` (which only handles "stalled chat awaiting a user answer").
  - New signal kind `failed_chat` with two failure shapes: `tool_error` and `no_response`. Detection lives in `internal/pulse/signal_failed_chat.go` and reads from the same `ChatSessionSource` the stalled-chat detector uses.
  - **Side-effect-aware**: candidates whose failing tool matches `tool.IsHighRiskToolName` (exec, process, write_*, edit_*, apply_patch, workspace) are marked `block_reason="high_risk_failure"` and surfaced for human attention only — pulse never auto-retries a turn whose last action could already have mutated state.
  - Reuses the existing `sessionAutoResumeController` for transcript injection + agent loop; the retry prompt is failure-kind specific and asks the model to inspect the error before re-running.
  - Audit trail uses a separate action name (`auto_resume_failed_chat`) so the per-session retry counter is independent of the question-resume counter. Same 30-minute escalation window and 3-retry cap as the existing autofix.
  - Opt-in via existing `SessionAutomationConsent.AutoResumeEnabled` and the global `pulse.allowed_autofixes` allow-list — not in the default allow-list.
- **`tool.IsHighRiskToolName`** — promoted from a private helper in `tarsserver` so pulse can share the same risk classification used at chat policy enforcement time.

## [0.31.143] - 2026-05-04

### Fixed

- **Console chat — re-entry & background sync** (#677) — three frontend gaps that left the chat view stale after navigating away or backgrounding the tab:
  - `ChatPanel` only refreshed on `category === 'cron'` events, so pulse-driven auto-resumes (which emit `category: "pulse"` with the session id) never triggered a transcript reload. The listener now matches by `session_id` regardless of category.
  - `document.visibilitychange` is now wired up: returning to a backgrounded chat tab forces a transcript reload so any messages written while the tab was throttled or the SSE connection paused are visible immediately.
  - `lib/api.ts` `ensureStream()` now reconnects automatically when `EventSource` reaches `CLOSED` (capped exponential backoff 1s → 30s), and `streamEvents()` exposes a new `onReopen` callback that fires after a successful reconnect. `ChatPanel` uses it to backfill the transcript so events lost during the gap are recovered without a page reload.

## [0.31.142] - 2026-05-04

### Fixed

- **Console — right dock minimum width** — bumped the right dock's minimum resizable width from 260px to 320px (matching the default size). At 260px the Tasks/Git panel cards would squash their right-side controls (status badge, "+ Evidence") and leave the title column too narrow for CJK text, causing per-character vertical wrapping. The clamp is enforced via `dockSizeLimits` in `lib/dock/layout.ts` and re-applied to persisted layouts on load via `normalizeDockLayout`. Mobile (<900px) is unaffected — docks already render as fullscreen overlays via the existing media query.

## [0.31.141] - 2026-05-04

### Fixed

- **Onboarding wizard — Channels section** — disabling the Telegram channel now also clears `channels_telegram_polling_enabled`, so users who arrive in the section with polling=true prefilled from disk can actually save the section after turning Telegram off. The polling checkbox only renders while the channel is on, so the previous behavior trapped users behind the "polling requires the Telegram channel to be enabled" validator with no UI affordance to flip polling back. `channelsFromConfigValues` now also normalizes inconsistent on-disk state (channel=false + polling=true) on load.

## [0.31.140] - 2026-05-04

### Added

- **Onboarding wizard expansion** — the setup wizard is now a section-router shell with **Quick** vs **Full** modes. Quick keeps the original LLM-only path (Provider → Tiers → Review → Complete); Full adds three new optional sections between Tiers and Review:
  - **Tools & Permissions** — toggle `web_search` / `web_fetch`, edit the private-host allowlist (newline-separated textarea), and gate the high-risk-user permission with a strong warning.
  - **Integrations** — API keys for external augmentations (web search provider key + memory embedding provider/key/model/base URL/dimensions).
  - **Channels** — Telegram (enable + bot token + polling) and webhook toggle, with client-side guards (polling requires bot token + channel enable).
- **Per-section save** — each optional section patches only its own keys via `buildSectionPayload`, so editing Channels never disturbs Tools. Sections can be skipped without writing.
- **Deep-link reentry** — `/console/onboarding?section=<id>` opens the wizard directly on a given section. Optional-section deep-links fall back to the Provider step when no provider is configured.
- **Completion matrix** — the new `OnboardingComplete.svelte` shows ✓/✗/— per capability (LLM provider, tier bindings, web_search, web_fetch, memory embeddings, telegram, webhook) with jump-back links to each section, and surfaces a "restart required to activate Telegram/Webhook" notice when those workers were saved.
- **Setup status capability flags** — `GET /v1/setup/status` now returns a `capabilities` block (`tools_configured`, `integrations_configured`, `channels_configured`, plus per-capability booleans) so the wizard's matrix avoids refetching the schema. Sensitive values are not exposed (only "set" / "not set" booleans).

## [0.31.139] - 2026-05-04

### Fixed

- **Config wizard / provider editor** — provider rename and delete in the onboarding wizard or the Settings provider editor now actually take effect on disk. `PatchYAML` previously preserved any `llm_providers` alias not in the patch, so `kimi → moonshot` rename left both aliases on disk and provider deletion was a no-op. The merge is now alias-replace (drop aliases missing from the patch) with per-field merge inside each alias (api_key omission still preserved). The wizard now sends the full provider map on save instead of only the edited alias, so non-edited providers flow through the alias-replace cleanly.
- **Config wizard step 1 → step 2** — switching provider kind (e.g. `kimi → anthropic`) now clears tier model entries because the previous IDs (`kimi-k2.6` vs `claude-haiku-4-5`) are kind-specific. `base_url` is also re-seeded to the new kind's canonical default when it matched the previous kind's default; user-customized URLs are preserved.
- **Config wizard alias rename** — renaming a provider alias in step 1 now propagates to tier bindings that referenced the old alias (instead of only filling empty entries), and the review step shows a notice that the old alias will be dropped when saved.
- **Settings provider editor** — switching a provider's kind now re-seeds `auth_mode` (to a kind-valid option), `base_url` (to the new kind's default when the previous URL was the old default), and clears `oauth_provider`. Renaming an alias also rebinds any tier currently pointing at the old alias, so saving doesn't leave orphan tier references.

## [0.31.138] - 2026-05-04

### Changed

- **Console chat** — moved the LLM inference status (phase label, elapsed timer, step progress, KR/EN toggle) from the bottom action row into the empty `assistant` bubble that appears while the model is thinking. The bubble previously rendered only `…`; the form-actions row now keeps just the Stop button. Status disappears as soon as the first response token arrives. New `ChatStreamingStatus.svelte` is reused via a `streamingStatus` prop on `ChatMessageItem`.

## [0.31.137] - 2026-05-03

### Fixed

- **Integrated terminal tabs** — terminal area collapsed to a single row or grew to ~24,000 px (depending on layout phase), making the shell unusable. The dock panel body uses `display: block`, so `flex: 1` on `.terminal-tabs` had no effect and the WebGL canvas was sized against an unbounded container. Added `height: 100%` so the tabs container resolves against the dock body's flex-derived height.
- **Integrated terminal** — moved the WebGL renderer addon load to after the first `fit()` and call `terminal.refresh()` once the renderer is attached, so the canvas adopts the correct dimensions even when a tab mounts via a `display: none → flex` transition. Also refresh on `visible` change so a re-activated tab always repaints from the latest buffer state.

## [0.31.136] - 2026-05-03

### Fixed

- **Integrated terminal tabs** — first tab rendered as a single black row because the inactive `.tab-pane` panes used `position: absolute` and the resulting layout collapsed the active pane's measured height. Replaced the absolute-stacking with `display: none` / `display: flex` toggling; the existing `visible` `$effect` already refits the terminal when a tab is reactivated.
- **Integrated terminal tabs** — the `+` button on the tab strip silently re-activated the existing tab instead of opening a new shell because `addTerminalTab` routed through the same path that de-duplicates by `cwd`+`label`. Split the handler so `+` always appends a fresh tab.

## [0.31.135] - 2026-05-03

### Fixed

- **Integrated terminal** — `effect_update_depth_exceeded` crash when a terminal tab opened inside `TerminalTabs`. The status-emitting `$effect` in `IntegratedTerminal` was tracking the parent's `onStatusChange` callback as a dependency; each `statuses` write recreated the inline arrow, retriggering the effect. Now the callback is invoked through `untrack` so only the data fields drive re-runs.

## [0.31.134] - 2026-05-03

### Added

- **Integrated terminal** — multi-tab dock (#663 Phase 4, closes the epic). The terminal panel now renders a tab strip; each "Open shell" adds a new tab (or activates an existing one for the same `cwd`/label combination). Tabs run independent `xterm` + WebSocket sessions in parallel without losing scrollback. A `+` button on the strip opens another shell in the active tab's directory; closing the last tab closes the panel.

### Changed

- **Integrated terminal** — when embedded in the tab strip, the per-terminal header is compact (label/dot/Close hidden — the tab pill owns those). The Find button stays so search is one click away from any active tab.

## [0.31.133] - 2026-05-03

### Added

- **Integrated terminal** — right-click context menu with Copy / Paste / Clear / Save buffer (`@xterm/addon-serialize`); the saved file is timestamped per-session and preserves ANSI codes (#663 Phase 3).
- **Integrated terminal** — font-size shortcuts (`⌘=` / `⌘-` / `⌘0`, or `Ctrl+=` / `Ctrl+-` / `Ctrl+0`) clamped to 8–24 px, persisted in `localStorage` under `tars.terminal.fontSize`.
- **Integrated terminal** — `⌘K` / `Ctrl+Shift+K` clears the buffer and scrolls to bottom.
- **Integrated terminal** — bell flash animation on the connection-status dot when the shell rings the BEL character.

## [0.31.132] - 2026-05-03

### Added

- **Integrated terminal** — clickable URLs (`@xterm/addon-web-links`), in-terminal search bar with case/regex toggles (`@xterm/addon-search`), WebGL renderer with DOM fallback, and Unicode 11 width handling for CJK/emoji glyphs (#663 Phase 1+2).
- **Integrated terminal** — explicit clipboard shortcuts: `⌘C` / `Ctrl+Shift+C` to copy selection, `⌘V` / `Ctrl+Shift+V` to paste, `⌘F` / `Ctrl+Shift+F` to open search; right-click selects word.
- **Integrated terminal** — clickable status indicator that reconnects the WebSocket when the session is `Disconnected` or `Exited`.

### Changed

- **Integrated terminal** — brighter selection background (`#a45a1f`) and stronger mono font fallback chain so dragged text is visibly highlighted across platforms.

## [0.31.131] - 2026-05-03

### Added

- **Prior Context panel** — debounced auto-refresh as the user types (700ms after typing stops), so the preview always reflects the current draft without a manual click.
- **Prior Context panel** — empty-query fallback that surfaces recent experience entries / `MEMORY.md` lines / daily logs when no draft is present, with a banner explaining the live LLM prompt does not actually carry these.
- **Prior Context panel** — "Below threshold" collapsible section that shows score-filtered candidates (1..99) so users can understand why a query did not recall anything.

### Changed

- **Tasks panel** — introduced a Tasks / Contract / Evidence tab structure. The Contract tab inherits the form/approval flow that previously lived in a separate top-bar panel; the Evidence tab aggregates verification artifacts across all tasks.
- **Top toolbar** — removed the standalone Contract toggle. Contract editing now lives inside the Tasks panel, which already holds the plan it scopes.

## [0.31.130] - 2026-05-03

### Added

- Added localized, phase-aware live chat status feedback in the Chat panel while streaming: status labels, elapsed timer, tool-aware progress steps, and animated activity dots.

### Fixed

- Replaced static "..." streaming status text with explicit phase-aware messaging and consistent status state updates.

## [0.31.129] - 2026-05-03

### Added

- Added provider-aware model loading for LLM tier editing so the model field can be selected from that provider's model list.
- Added quick-start/view-mode parity so the field action buttons (Save/Discard) stay visible in quick start mode when there are pending changes.

### Fixed

- Fixed `/v1/models` provider lookup behavior to support `provider_alias` scoped queries and fall back to a warninged empty response on permission errors, instead of returning a 500.
- Fixed LLM history reconstruction so trailing unmatched tool outputs and stale tool-call metadata are no longer included in outbound chat payloads.

## [0.31.128] - 2026-05-02

### Fixed

- Fixed Kimi-compatible tool-calling request generation for thinking-enabled calls by avoiding `tool_choice=required` in OpenAI-compatible compatibility mode. `required` now maps to a direct tool choice when only one tool is available, and falls back to `auto` when multiple tools are passed.

## [0.31.127] - 2026-05-02

### Added

- Added Channels page (`/console/channels`) for Telegram pairing management: approve pairing codes, view pending/allowed users, and revoke access.
- Added `channels` navigation item to the Operate group in the console sidebar with i18n support (EN/KO).

### Fixed

- Fixed Config sensitive-field editing bug where fields like `telegram_bot_token` could not be edited in Fields or Quick Start views. Sensitive fields now render as password inputs and are always masked in display.

## [0.31.126] - 2026-05-02

### Fixed

- Fixed Kimi tool-calling payload handling for `openai_compat_client` so stale or orphaned `tool` role messages are dropped and only the matched latest tool result for each call is sent.
- Added tighter validation/sanitation for Kimi tool message IDs and tool-call metadata in request conversion.
- Added regression tests covering omitted `service_tier`/`reasoning_effort`, `reasoning_content` propagation, orphan/stale tool filtering, and trimmed tool-call IDs.

## [0.31.125] - 2026-05-02

### Added

- Added `kimi` as a first-class LLM provider kind for OpenAI-compatible usage, including `KIMI_API_KEY`-based environment fallback and default Moonshot base URL wiring.
- Added Kimi provider coverage to provider selection docs and live-provider API metadata so `/v1/providers` reports `kimi` with model-listing support.

## [0.31.124] - 2026-05-02

### Added

- Added a structured Console Settings editor for `llm.providers` so each provider can be edited as a card with alias, kind, auth mode, OAuth provider, base URL, API key, and service tier inputs instead of raw JSON.
- Added per-provider API key reveal toggle and Add/Remove provider controls in the new editor, mirroring the existing LLM tier editor flow.

### Tests

- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`

## [0.31.123] - 2026-05-01

### Added

- Added `POST /v1/agentruntime/subagents/recommendations` to analyze recent completed Agent Runtime runs and suggest reusable workspace `AGENT.md` profile drafts.
- Added recommendation provenance on subagent drafts so approved profiles preserve source run IDs, run metadata, and observed prompt context.
- Added Console Subagents controls for generating profile recommendations from recent runs, reviewing each recommended draft, and saving it through the existing approval flow.

### Documentation

- README and the Agent Runtime tutorial now describe run-derived subagent profile recommendations.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run TestAgentRuntimeSubagentsAPIHandler_RecommendsProfilesFromRecentRuns`

### Closed

- Closes #594.

## [0.31.122] - 2026-05-01

### Added

- Added Agent Runtime run checkpoints for prompt dispatch and failure snapshots so failed runs keep restartable state.
- Added `POST /v1/agentruntime/runs/{run_id}/restart` to start a derived retry from a checkpoint with optional agent, tier, provider/model, and prompt adjustment overrides.
- Added restart provenance fields on runs, including source run, source checkpoint, attempt number, and restart reason.
- Added Console Agent Runtime controls for restarting failed runs from checkpoints and navigating to the derived retry run.

### Documentation

- README and the Agent Runtime tutorial now describe failed-run checkpoint restart workflows.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/agentruntime -run TestRuntimeRestartFromCheckpointSpawnsDerivedRunWithOverrides`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run TestAgentRunsAPIHandler_RestartFromCheckpoint`

### Closed

- Closes #593.

## [0.31.121] - 2026-05-01

### Added

- Added `subagents_run` compare mode for 2-3 read-only subagents inspecting the same prompt independently.
- Added task-level `agent` selection for `subagents_run`, while preserving top-level agent fallback and existing safety validation.
- Added compare-mode results with side-by-side outputs, common findings, conflict candidates, sourced evidence snippets, and direct run links in Console Chat.

### Documentation

- README and the Agent Runtime tutorial now describe compare-mode subagent workflows.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tool -run 'TestSubagentsRunTool_CompareMode|TestSubagentsRunTool_.*ConsensusSchema'`
- `cd frontend/console && node --experimental-strip-types --test tests/subagentProgress.test.ts`

### Closed

- Closes #592.

## [0.31.120] - 2026-05-01

### Added

- Added global and per-session TARS style controls for directness, humor, caution, and autonomy.
- Added `/v1/admin/sessions/{id}/style` for reading effective style defaults and saving normalized session overrides.
- Added chat prompt wiring so style controls affect response tone, verification posture, and follow-through while autonomy remains bounded by explicit session consent and approval policy.
- Added a Console Session Config Style tab with sliders, default-value context, and concise behavior previews.

### Documentation

- README, config examples, and chat/config tutorials now describe session style controls and `runtime.style.*_default` settings.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/session -run 'TestStoreForkFromMessageCopiesTranscriptPrefixAndState|TestStoreSetStyleControl_NormalizesSliderScores'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/config -run 'TestLoad_SessionStyleDefaultFields|TestApplyDefaults_ClampsSessionStyleDefaultFields'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'TestSessionStylePromptLimitsAutonomyByConsent|TestEffectiveSessionStyleUsesSessionOverrides|TestSessionAPI_StyleControlRoundTrip'`
- `cd frontend/console && node --experimental-strip-types --test tests/sessionStyleControl.test.ts`

### Closed

- Closes #591.

## [0.31.119] - 2026-05-01

### Added

- Added first-turn cost/quality tier recommendations for Console Chat so TARS can suggest heavy, standard, or light before the first expensive LLM call.
- Added chat request support for accepted or overridden tier recommendations, explicit tier routing for the selected chat turn, and usage signal records with recommendation, chosen tier, provider/model, outcome, token usage, and estimated cost.
- Added Context HUD visibility for the chosen LLM tier and accepted/overridden recommendation path.

### Documentation

- README and the HTTP/SSE chat tutorial now describe first-turn tier recommendation and traceable selected-tier metadata.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/llm -run TestRecommendTierForTaskClassifiesCommonWork`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'TestResolveChatTierRecommendationFallsBackOnFirstTurn|TestResolveChatTierRecommendationCanDisableFallback|TestResolveChatClientForTierUsesExplicitTier'`
- `cd frontend/console && node --experimental-strip-types --test tests/tierRecommendation.test.ts`

### Closed

- Closes #590.

## [0.31.118] - 2026-05-01

### Added

- Expanded Settings config impact analysis with subsystem-level hints for LLM routing, Auth/API, Pulse, Reflection, Cron, Memory, Agent Runtime, Extensions, Tools, Channels, Usage, Compaction, Assistant, Logging, and Runtime fields.
- Added frontend fallback impact classification so pending config edits still show an affected subsystem even when a future field lacks explicit schema metadata.

### Documentation

- README and config-system tutorial docs now describe Settings impact previews as subsystem-aware before-save guidance.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/config -run 'TestSchemaIncludesImpactHintsForHighSignalFields|TestSchemaImpactHintsCoverCoreSubsystems'`
- `cd frontend/console && node --experimental-strip-types --test tests/configImpactPreview.test.ts`
- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- Browser smoke for `/console/config` pending-change subsystem impact preview and zero console errors

### Closed

- Closes #589.

## [0.31.117] - 2026-05-01

### Added

- Added a Chat session health badge and dockable Health panel with deterministic recommendations for long context, stale plans, broad high-risk permissions, noisy prior memory, and idle sessions.
- Added actionable health recommendations that jump to Compact, Tasks, Config, Prior Context, Skill Inbox, or the chat transcript review path.

### Documentation

- README and console tutorial docs now describe session health recommendations in the Chat workspace surface.

### Tests

- `cd frontend/console && node --experimental-strip-types --test tests/sessionHealth.test.ts`
- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/chat/{session}` Session Health critical badge, Health recommendation panel, Open Tasks action, and zero console errors

### Closed

- Closes #588.

## [0.31.116] - 2026-05-01

### Added

- Added Pulse incident cards that turn recent watchdog signals into actionable summaries with likely cause, evidence, severity, recommended action, and safe navigation/re-check buttons.
- Added deterministic incident-card mapping for cron failures, stuck Agent Runtime runs, disk pressure, Telegram delivery failures, reflection failures, stalled chats, and Pulse tick errors.

### Documentation

- README and realtime tutorial docs now describe Pulse incident cards as the actionable layer on top of raw watchdog signals.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/pulse` incident cards with mocked Pulse status, affected-page navigation, and zero console errors

### Closed

- Closes #587.

## [0.31.115] - 2026-05-01

### Added

- Added a Console permission change preview for per-session tool, group, skill, and MCP policy changes.
- Added deterministic permission impact summaries with low/medium/high risk labels, affected tool groups, high-risk tool detection, and shell/files/git/network capability chips before saving session overrides.

### Documentation

- README and human-in-the-loop docs now describe session policy previews as the review step before applying tool and skill permission changes.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/chat/{session}` Session Config permission previews, high-risk shell capability display, Apply persistence, and zero console errors

### Closed

- Closes #586.

## [0.31.114] - 2026-05-01

### Added

- Added Pulse `stalled_chat` detection for active sessions whose latest assistant turn is waiting on user input.
- Added the `auto_continue_chat` Pulse autofix, gated by session-scoped auto-resume consent, allowed resume modes, high-risk question blocking, and a 3-resumes-per-30-minutes escalation cap.
- Added session automation consent fields for `auto_resume_enabled`, `auto_resume_after_minutes`, and `allowed_resume_modes`.
- Added Console controls for auto-resume delay and allowed continuation modes.

### Documentation

- README and human-in-the-loop docs now describe session-scoped stalled-chat auto-resume and the `auto_continue_chat` allowlist entry.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/session`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/pulse ./internal/pulse/autofix`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'TestSessionAutoResumeController'`
- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/chat/{session}` session automation settings with auto-resume delay and mode controls

### Closed

- Closes #585.

## [0.31.113] - 2026-05-01

### Added

- Added Skill Hub domain pack registry metadata with skill, plugin, and MCP dependencies.
- Added pack install planning that shows package contents and install/update/skip actions before applying.
- Added `tars pack search`, `tars pack info`, and reviewed `tars pack install <name>` with `--yes` for non-interactive approval.
- Added pack install execution that reuses the existing sandbox-validated skill, plugin, and MCP installers for each pack member.

### Documentation

- README and Skill Hub tutorial now document domain packs and `tars pack install`.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./cmd/tars ./internal/skillhub`
- `GOCACHE=/tmp/tars-go-cache go run ./cmd/tars pack info github-maintainer-pack`
- `GOCACHE=/tmp/tars-go-cache go run ./cmd/tars pack install github-maintainer-pack --workspace-dir <tmp> --yes`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`

### Closed

- Closes #583.

## [0.31.112] - 2026-05-01

### Added

- Added Skill Hub quality metadata parsing for skills, plugins, and MCP packages.
- Added Extensions Hub quality score badges and install-time trust signals for last update, tests, required tools, permissions, companion CLI presence, and install count.

### Documentation

- README and Skill Hub tutorial now document registry quality metadata and Extensions Hub trust signals.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/skillhub ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/extensions` Hub quality score and trust signal rendering with a mock API

### Closed

- Closes #582.

## [0.31.111] - 2026-05-01

### Added

- Added sandbox validation for Skill Hub plugin installs and updates before writing to `workspace/plugins/<name>`.
- Added sandbox validation for Skill Hub MCP installs and updates before writing to `workspace/mcp-servers/<name>`.
- Added plugin manifest diagnostics, plugin-declared MCP gating checks, MCP manifest validation, and stdio/remote MCP smoke checks to install sandbox reports.
- Added generic extension sandbox report metadata so the Extensions console can render skill, plugin, and MCP install reports.

### Changed

- Hub install API responses now include sandbox reports for successful plugin and MCP installs, matching skill install behavior.
- Plugin and MCP install failures caused by sandbox validation return structured sandbox reports and leave the real workspace package directories untouched.

### Documentation

- README now documents sandbox-tested Skill Hub installs for skills, plugins, and MCP packages.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./cmd/tars ./internal/skillhub ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/extensions` Hub plugin and MCP installs rendering sandbox reports with a mock API

### Closed

- Closes #581.

## [0.31.110] - 2026-05-01

### Added

- Added Skill Extraction Inbox APIs for extracting reusable skill candidates from chat sessions.
- Added session transcript skill candidate detection with light-tier LLM extraction and deterministic fallback.
- Added reviewable skill extraction candidates with provenance, message range, repeated evidence, approve, and reject states.
- Added approval flow that saves accepted candidates as local `workspace/skills/<name>/` drafts using the existing Skill Creator scaffold.
- Added a dockable Chat Skill Inbox panel plus `/extract-skill` slash command for extracting from the active session.

### Documentation

- README now documents session-based skill extraction and local skill draft approval.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/skill ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/chat/{session}` Skill Inbox extract, candidate review, approval, and local skill draft path rendering with a mock API

### Closed

- Closes #579.

## [0.31.109] - 2026-05-01

### Added

- Added approved Git mutation approvals for stage, unstage, discard, commit, and branch switch actions.
- Added `/v1/git/mutations` to queue Git mutation approval cards instead of mutating the workspace directly.
- Added Git Inspector controls for queueing approved mutations and Approvals page rendering for Git mutation cards.
- Added Git mutation automation audit records for success, failure, and blocked consent states.

### Changed

- Destructive Git discard actions are highlighted and cannot run silently; they require session Git mutation consent plus approval.

### Documentation

- README now documents approved Git mutations from Git Inspector through Approvals and Automation Audit.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/git ./internal/ops ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/chat/{session}` Git Inspector mutation approval and `/console/approvals` Git mutation apply/audit with a mock API

### Closed

- Closes #572.

## [0.31.108] - 2026-05-01

### Added

- Added session-scoped automation consent settings for auto-resume, approved git mutations, and autonomous workspace mutations.
- Added durable automation audit entries with actor, action, reason, session, cwd, result, and timestamp metadata.
- Added `/v1/admin/sessions/{session_id}/automation-consent` and `/v1/ops/automation-audit` APIs.
- Added Console controls for session automation consent and an Automation Audit section on the Approvals page.

### Changed

- Session automation defaults remain conservative: no autonomous workspace mutation is allowed unless explicitly enabled for that session.

### Documentation

- README now documents session automation consent and the Automation Audit console surface.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/session ./internal/ops ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/chat/{session}` session automation consent and `/console/approvals` Automation Audit with a mock API

### Closed

- Closes #584.

## [0.31.107] - 2026-05-01

### Added

- Added fork insight promotion APIs at `/v1/admin/sessions/{session_id}/promotions`.
- Added deterministic post-fork insight extraction for reviewable decision, preference, and procedure candidates.
- Added Lineage page controls for reviewing fork insights and queueing selected items into Memory Inbox.

### Changed

- Fork insight promotion preserves parent transcripts and routes reusable fork findings through the existing Memory Inbox approval flow.

### Documentation

- README now documents fork insight review and Memory Inbox promotion from the Lineage page.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/session ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/sessions/graph` fork insight review, Memory Inbox queueing, and Inbox navigation with a mock API

### Closed

- Closes #570.

## [0.31.106] - 2026-05-01

### Added

- Added a Console session lineage graph at `/console/sessions/graph`.
- Added a session lineage row builder that renders root sessions before forked children with depth and fork metadata.
- Added fork point previews by resolving child `forked_from_message_id` values against the parent transcript history.

### Changed

- Console navigation now includes a dedicated Lineage entry for the session graph.

### Documentation

- README now documents the session lineage graph view alongside message-level session forking.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/sessions/graph` rendering and graph-node chat navigation with a mock API

### Closed

- Closes #569.

## [0.31.105] - 2026-05-01

### Added

- Added session forking from a transcript message, creating a child session with transcript history copied through the selected message.
- Added `/v1/admin/sessions/{session_id}/fork` to create forked sessions with lineage metadata.
- Added a Console chat message action for forking from a persisted transcript message and jumping into the new session.

### Changed

- Forked sessions now copy session state that should carry forward: tasks, tool/skill/MCP config, prompt override, work dirs, current dir, and compaction mode.

### Documentation

- README now documents message-level session forking as part of the Chat workflow.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/session ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/chat/{session}` message fork action with a mock API

### Closed

- Closes #568.

## [0.31.104] - 2026-05-01

### Added

- Added first-class session lineage fields: `parent_session_id`, `root_session_id`, `forked_from_message_id`, `forked_from_index`, and `fork_reason`.
- Added stable transcript message IDs for newly appended or rewritten messages.
- Added deterministic read-time virtual IDs for legacy transcript messages that do not yet have persisted IDs.

### Changed

- Session API responses and Console session types now expose lineage and message ID fields.

### Documentation

- README now documents the session lineage and transcript message ID foundation for future fork and graph workflows.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/session ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`

### Closed

- Closes #567.

## [0.31.103] - 2026-05-01

### Added

- Added a Memory Inbox review queue so reflection-derived memory candidates are stored for approve/reject/merge review before entering durable recall.
- Added `/v1/memory/inbox` and `/v1/memory/inbox/review` APIs with provenance, similarity hints, and conflict hints.
- Added a Console Memory Inbox tab with candidate provenance, similar/conflicting memory hints, and review actions.

### Changed

- Nightly reflection now enqueues memory candidates instead of directly appending auto-derived experiences.

### Documentation

- README now documents review-before-store memory extraction in the Chat + Memory and Console Memory workflows.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/memory ./internal/reflection ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/memory` Memory Inbox rendering and approve flow with a mock API

### Closed

- Closes #578.

## [0.31.102] - 2026-05-01

### Added

- Added skill install sandboxing so Skill Hub skill installs and skill updates materialize into a temporary workspace and run manifest/default smoke checks before touching the real workspace.
- Added `smoke_tests` skill frontmatter support for package-defined smoke commands.
- Hub install responses now include a readable skill sandbox report, and the Extensions console renders the latest sandbox pass/fail checks.

### Fixed

- Failed skill smoke tests no longer replace existing installed skill files or update `skillhub.json`.

### Documentation

- README now documents sandbox-smoke-tested Skill Hub installs in the Extensions workflow.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/skillhub ./internal/skill ./internal/tarsserver`

### Closed

- Closes #580.

## [0.31.101] - 2026-05-01

### Added

- Added a read-only Git Inspector dock panel in Console Chat for coding sessions.
- Added `/v1/git/status`, `/v1/git/diff`, `/v1/git/log`, and `/v1/git/branches` APIs backed by a thin read-only `internal/git` wrapper.
- Git Inspector now detects the active session git workspace, shows branch, HEAD, remotes, staged and unstaged files, and renders selected file diffs with a side-by-side summary plus unified patch.

### Documentation

- README now documents the Chat Git Inspector panel and read-only git workspace inspection.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/git ./internal/tarsserver -run 'TestClientStatusAndDiffAreReadOnly|TestGitAPIStatusAndDiffUseSessionCurrentDir|TestRegisterAPIRoutes_RegistersCoreRoutes'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/git ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/chat/:session` Git Inspector rendering with a mock git workspace payload

### Closed

- Closes #571.

## [0.31.100] - 2026-05-01

### Added

- Added task evidence records so plan steps can keep durable verification proof such as test results, screenshots or images, log excerpts, PR links, release tags, and command output summaries.
- Added `tasks(action="evidence_add")` and `tasks(action="evidence_remove")` so agents and users can attach or remove evidence directly from session tasks.
- Added Console Chat task evidence cards and a read-only Contract evidence summary so verification proof remains visible after session reloads.
- Included task evidence in active task prompt injection and archived plan summaries so future turns can see what was already verified.

### Documentation

- README now documents evidence-backed Chat Tasks and Contract panels for active plan verification.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/session -run TestSessionTasks_EvidencePersistsAndInjects`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tool -run 'TestTasks_EvidenceAddAndRemove|TestTasks_EvidenceAddRejectsInvalidType'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/prompt`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/session ./internal/tool ./internal/prompt ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/chat/:session` Tasks and Contract evidence rendering with a mock session payload

### Closed

- Closes #575.

## [0.31.99] - 2026-05-01

### Added

- Added Agent Runtime git diff timeline snapshots so completed runs can show which workspace files changed, including changes made through shell commands rather than file tools.
- Added per-run diff metadata with session, agent, plan flow/step, repo root, file status, additions, deletions, patch previews, and future Git Inspector targets.
- Added a Console Agent Runtime detail panel that groups captured file changes by run and links back to the owning Agent Runtime run.

### Documentation

- README now documents Agent Runtime diff timeline visibility for coding workflows.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/agentruntime -run 'TestRuntimeCapturesGitDiffTimelineForShellStyleChanges|TestRuntimeSpawnAndWait|TestRuntimeCapturesFileToolCallSummaryAndEvent'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/agentruntime`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/agentruntime/runs/:id` Diff Timeline rendering with a mock run payload

### Closed

- Closes #576.

## [0.31.98] - 2026-05-01

### Added

- Added session task contracts with explicit goal, scope, done criteria, verification commands, expected artifacts, and draft/approved status stored alongside active plan tasks.
- Added a dockable Console Chat Contract panel for reviewing, editing, saving, and approving the active session contract.
- Extended the `tasks` tool with `contract_update` and `contract_approve`, and taught `plan_set` to seed a task contract draft from the initial request.
- Included task contract details in compaction reinjection, archive summaries, and global active plan responses.

### Documentation

- README now documents the Chat Contract panel and task contract workflow.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/prompt ./internal/tool ./internal/session ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke for `/console/chat` Contract dock edit/approve flow

### Closed

- Closes #574.

## [0.31.97] - 2026-05-01

### Added

- Revamped Console Home into Mission Control with live active plan, cron job, Agent Runtime run, Pulse, Reflection, session, notification, and delivery overview cards.
- Added 30-second Mission Control polling so the home overview refreshes without opening individual console pages.
- Added deep links from Mission Control cards to Plans, Agent Runtime, Cron, Pulse, Reflection, Chat, Approvals, GitHub releases, and pull requests.

### Documentation

- README now describes `/console` as Mission Control for active work, automation, runtime health, and delivery status.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`

### Closed

- Closes #577.

## [0.31.96] - 2026-05-01

### Added

- Added a Console Chat progress card for `subagents_run` so parallel subagent work shows running/completed/failed counts while the tool call is active.
- Added per-subagent run links in completed progress cards so users can jump directly into Agent Runtime run details.
- Added compact `subagents_run` status previews that omit prompt bodies while preserving titles, statuses, run IDs, summaries, and errors for chat rendering.

### Documentation

- README now documents the chat-visible parallel subagent progress card and Agent Runtime run links.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache make test`

### Closed

- Closes #573.

## [0.31.95] - 2026-05-01

### Changed

- Moved the integrated Console terminal out of the Files panel and into the Chat bottom dock.
- Files can now stay visible while the bottom-docked terminal remains open at the selected session workspace path.
- The integrated terminal now shrinks with dock split resize events instead of enforcing a fixed minimum frame height.

### Documentation

- README now documents that the Files shell opens in the bottom dock while the macOS Terminal fallback remains available.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `git diff --check`
- Browser smoke: `http://127.0.0.1:43216/console/chat` opened Files, selected the active session, clicked Shell, and verified Files stayed visible while Terminal opened in the bottom dock with cwd label and input focus.

### Closed

- Closes #566.

## [0.31.94] - 2026-05-01

### Added

- Added a Console Chat Dock Manager foundation with left, right, bottom, and fullscreen zones for Sessions and Chat tool panels.
- Added drag resize support for side and bottom docks, plus localStorage persistence for panel placement and dock sizes.
- Added focused dock layout tests covering default placement, panel moves, invalid stored layouts, resize clamping, and serialization.

### Documentation

- README now documents Chat dock placement, resizing, fullscreen mode, and persisted layout behavior.

### Tests

- `make console-build`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- Browser smoke: `http://127.0.0.1:43215/console/chat` opened Files in the right dock, moved it to the bottom dock, switched it to fullscreen, and verified persistence after reload.

### Closed

- Closes #565.

## [0.31.93] - 2026-05-01

### Documentation

- README now documents that the TARS name is an homage to TARS from *Interstellar*.
- Refreshed the public extension docs around the current skill-first model, plugin-declared MCP server opt-in, hub MCP packages, and removed built-in Go plugin/HTTP-route surfaces.
- Updated Getting Started, Contributing, and tutorials for the current provider pool, auth middleware admin paths, default console/API address, and Skill Hub package flow.

### Tests

- `git diff --check`
- stale docs token scan for removed extension/provider/KB/gateway references
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`

### Closed

- Closes #563.

## [0.31.92] - 2026-05-01

### Fixed

- Fixed the Console Vite dev proxy so `/console` assets and HMR load from the mounted path without redirect loops or WebSocket 404s.
- Fixed the Console favicon reference so browsers request `/console/favicon.svg` instead of the unauthenticated root path.
- Fixed the MCP Server Creator draft action so invalid empty names are blocked client-side before a 400 API request is sent.
- Fixed narrow Console layouts so the sidebar status strip collapses before it can overlap main content.
- Fixed legacy workspace agent `tools_allow: [knowledge]` entries by aliasing them to the current `memory` tool and removing `knowledge` from the minimal default tool list.

### Documentation

- README now documents the Console dev proxy mount and the MCP Server Creator's client-side draft-name validation.

### Tests

- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `cd frontend/console && npm run build`
- `GOCACHE=/tmp/tars-go-cache go test ./...`
- Browser smoke: dev proxy at `http://127.0.0.1:43180/console/` with Vite HMR, `/console/favicon.svg`, MCP Creator disabled Draft state, and 496px/800px sidebar collapse verified.

### Closed

- Closes #557.
- Closes #558.
- Closes #559.
- Closes #560.
- Closes #561.

## [0.31.91] - 2026-05-01

### Added

- Added an always-visible Chat plan progress strip that shows the active plan goal, completed/total task count, and progress bar when a session has a plan.
- Added a shared Console task-progress helper so the Chat strip and Tasks panel use the same completed-over-total calculation.

### Documentation

- README now documents the Chat header plan progress strip.

### Tests

- `cd frontend/console && node --experimental-strip-types --test tests/taskProgress.test.ts tests/i18n.test.ts`
- `GOCACHE=/tmp/tars-go-cache make test`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `cd frontend/console && npm run build`
- `make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`
- `make console-build`

### Closed

- Closes #393.

## [0.31.90] - 2026-05-01

### Added

- Added `GET /v1/admin/tasks?active=true` for recently updated active plans across sessions.
- Added `/console/tasks` as a global Plans page with session plan progress cards and direct chat-session navigation.
- Added Console sidebar navigation for Plans.

### Documentation

- README now documents the global Plans page and active-plan API surface.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/session ./internal/tarsserver -run 'ListSessionsWithPlans|GlobalTasksAPI|RegisterAPIRoutes'`
- `cd frontend/console && node --experimental-strip-types --test tests/plansPage.test.ts tests/navGroups.test.ts tests/i18n.test.ts`
- `GOCACHE=/tmp/tars-go-cache make test`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `cd frontend/console && npm run build`
- `make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`
- `make console-build`

### Closed

- Closes #394.

## [0.31.89] - 2026-05-01

### Added

- Added archived-plan listing from `MEMORY.md` notes prefixed with `[archived plan]`, including multiline note grouping and newest-first sorting.
- Added `GET /v1/admin/plans/archive` and `GET /v1/admin/sessions/:id/plans/archive` for global and session-scoped plan archive reads.
- Added a collapsible Past plans section to the Console Chat Tasks panel with read-only archived plan summaries.

### Changed

- Newly archived plan memory notes now include session ID metadata so session-scoped archive views can avoid mixing unrelated plans.

### Documentation

- README now documents the Tasks panel archive and global archive API surface.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/memory ./internal/tarsserver -run 'ListMemoryNotesByPrefix|PlanArchiveAPI|RegisterAPIRoutes'`
- `cd frontend/console && node --experimental-strip-types --test tests/tasksArchive.test.ts tests/i18n.test.ts`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache make test`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run build`
- `make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`
- `make console-build`

### Closed

- Closes #395.

## [0.31.88] - 2026-05-01

### Added

- Mirrored `subagents_plan` output into the active session plan/tasks so staged subagent work appears in the Console Tasks panel.
- Mirrored `subagents_orchestrate` task lifecycle updates into session tasks, including `in_progress`, `completed`, and `cancelled` states with run summaries or error descriptions.

### Changed

- Chat tool registration now injects the active session store into staged subagent tools without changing existing standalone constructor call sites.

### Documentation

- README now documents that staged subagent tools update the session Tasks panel while they run.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tool`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'ChatPipelineTools|ChatTool|SessionTasks|Tasks|Subagents'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tool ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`
- `make console-build`

### Closed

- Closes #396.

## [0.31.87] - 2026-05-01

### Added

- Added a Console Analytics page at `/console/analytics` with 7d/30d/90d usage period controls, summary cards, pure-SVG daily stacked token bars, model rows, and tool/skill call rows.
- Added `GET /v1/admin/analytics?days=7|30|90` backed by the existing usage tracker JSONL data, including zero-filled daily rows and unique session counts.

### Changed

- The Console navigation now includes Analytics in the Operate group.

### Documentation

- README now documents the global Analytics page alongside the other console pages.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/usage ./internal/tarsserver -run 'Analytics|RegisterAPIRoutes'`
- `cd frontend/console && node --experimental-strip-types --test tests/analyticsPage.test.ts tests/navGroups.test.ts tests/i18n.test.ts`
- `cd frontend/console && npm run check`
- `cd frontend/console && npm test -- tests/analyticsPage.test.ts`
- `cd frontend/console && npm run build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make vet`
- `make security-scan`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make build`
- Browser smoke: opened `/console/analytics`, verified seeded usage totals, model rows, tool/skill rows, and switched 30d/90d periods.

### Closed

- Closes #397.

## [0.31.86] - 2026-05-01

### Added

- Added a Console Logs page at `/console/logs` with file, level, component, and line-count filters, manual refresh, 5-second auto-refresh, and level highlighting.
- Added `GET /v1/admin/logs` for safe logical log-file selection and tailing parsed JSON log lines from the configured runtime log sink.

### Changed

- The Console navigation now includes Logs in the Operate group.

### Documentation

- README now documents the global Logs page alongside the other console pages.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'TestLogsAPI|TestRegisterAPIRoutes'`
- `cd frontend/console && npm test -- tests/logsPage.test.ts`
- `cd frontend/console && npm run check`
- `cd frontend/console && npm run build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make vet`
- `make security-scan`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make build`
- Browser smoke: opened `/console/logs`, filtered to `ERROR` plus `component=runtime`, verified the Agent Runtime error line, and toggled 5-second auto-refresh.

### Closed

- Closes #398.

## [0.31.85] - 2026-05-01

### Fixed

- Serialized Agent Runtime persistence snapshots so older overlapping snapshot writes cannot overwrite newer run/channel state.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test -coverprofile=/tmp/agentruntime.cover ./internal/agentruntime -run 'TestRuntimePersistence_(TrimsRunsAndChannelMessages|ConcurrentSnapshotsKeepLatestChannels)' -count=20`

### Closed

- Closes #549.

## [0.31.84] - 2026-05-01

### Added

- Added a global Console Cron page at `/console/cron` for creating, monitoring, pausing, resuming, manually running, and deleting scheduled jobs.
- Cron jobs now show delivery target, status buckets, next-run context, and expandable run history from the existing cron API.

### Changed

- The Console navigation now includes Cron in the Operate group.
- Frontend cron API types now include delivery, wake, payload, and delete-after-run fields already supported by the backend.

### Documentation

- README now documents the global Cron page alongside the per-session cron panel.

### Tests

- `cd frontend/console && npm test -- tests/cronPage.test.ts`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `cd frontend/console && npm run build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make vet`
- `make security-scan`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make build`
- Browser smoke: created a global cron job on `/console/cron`, expanded run history, paused/resumed it, and deleted it.

### Closed

- Closes #399.

## [0.31.83] - 2026-05-01

### Added

- Chat session search now reuses session-inclusive memory search to surface matching transcript snippets in the session sidebar.
- Session search snippets highlight OR-matched query terms safely after escaping transcript text.

### Changed

- The standalone Sessions component uses the same transcript snippet grouping and highlight behavior as the active chat sidebar.

### Documentation

- README now notes transcript snippet matches in chat session search.

### Tests

- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `cd frontend/console && npm run build`
- Browser smoke: searched `meridian launch` in the chat session sidebar and verified transcript snippets plus highlighted terms.

### Closed

- Closes #400.

## [0.31.82] - 2026-05-01

### Added

- Chat composer slash commands now render through a dedicated `SlashPopover` component with built-in and skill sections.
- Added client-side `/clear`, `/memory search <query>`, and `/skill <name>` handling without sending those commands to the LLM.

### Changed

- Memory search links can prefill the search tab from query parameters, enabling `/memory search ...` routing from chat.

### Documentation

- README now notes the chat composer slash-command popover and first-pass client commands.

### Tests

- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `cd frontend/console && npm run build`

### Closed

- Closes #401.

## [0.31.81] - 2026-05-01

### Added

- Console sidebar now includes a persistent status strip for server, Pulse, Reflection, and active session state.
- Status strip rows refresh every 30 seconds, stop polling when the sidebar is destroyed, and navigate to their related console detail pages.

### Documentation

- README now notes the persistent console sidebar status strip.

### Tests

- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `cd frontend/console && npm run build`

### Closed

- Closes #402.

## [0.31.80] - 2026-05-01

### Added

- Console tool calls now render as collapsible rows with compact invocation previews, pretty-printed args/results, and live elapsed time while running.
- Chat SSE and persisted session transcripts now carry `tool_is_error` metadata so failed tool calls reopen with destructive styling after reload.

### Changed

- Tool-call state colors now use the console design tokens: primary for running, default for done, and error for failed calls.

### Documentation

- README now notes the console's live, collapsible tool-call rows.

### Tests

- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `GOCACHE=/tmp/tars-go-cache go test -count=1 ./internal/tarsserver ./internal/session`

### Closed

- Closes #403.

## [0.31.79] - 2026-05-01

### Added

- Added `usage_daily_token_budget` / `usage.limits.daily_tokens` config metadata for a daily input+output token budget; `0` disables the console chip.
- Added `/v1/admin/usage/today` to summarize today's input, output, total tokens, configured budget, UTC reset boundary, percent used, and indicator level.
- The console header now shows a compact daily token budget chip when a budget is configured, with warning/error states and an error-state jump to today's analytics focus.

### Changed

- Usage limit PATCH requests can now update `daily_tokens` alongside the existing USD limits.

### Documentation

- README and the example config now document the daily token budget indicator setting.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test -count=1 ./internal/usage ./internal/tarsserver ./internal/config`
- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `cd frontend/console && npm run build`

### Closed

- Closes #404.

## [0.31.78] - 2026-05-01

### Added

- Console i18n now supports English and Korean locale maps with browser-language detection and `tars_console_locale` localStorage persistence.
- The console header includes a compact EN/KO language toggle.

### Changed

- Console navigation, header notifications, Sessions, Memory, and Tasks first-pass static labels now read from the shared translation store.

### Documentation

- README now notes the console EN/KO language toggle and persisted locale key.

### Tests

- `cd frontend/console && npm test`
- `cd frontend/console && npm run check`
- `cd frontend/console && npm run build`

### Closed

- Closes #405.

## [0.31.77] - 2026-05-01

### Changed

- Config YAML path metadata is now materialized from the config input-field registry instead of a large separate key switch.
- LLM provider-kind defaults now live in a shared defaults table used by config normalization and provider construction.

### Documentation

- README now notes that console Settings and YAML patch paths share the config field registry.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/config ./internal/llm ./internal/tarsserver`

### Closed

- Closes #406.

## [0.31.76] - 2026-05-01

### Changed

- Split Chat message rendering, Artifact panel header rendering, and Config pending-change rendering into focused Svelte subcomponents.
- Moved the console-only chat message shape into a dedicated frontend type module.

### Documentation

- Added a frontend API type contract note explaining why `types.ts` remains hand-curated until a smaller shared schema source exists.

### Tests

- `cd frontend/console && npm run check`

### Closed

- Closes #407.

## [0.31.75] - 2026-05-01

### Changed

- CLI console opening, internal CLI API calls, and public `pkg/tarsclient` requests now share the same URL resolver.
- Server URLs with proxy base paths now resolve consistently for both API paths and `/console`.

### Fixed

- Base URL query strings and fragments no longer leak into resolved API or console URLs.

### Documentation

- README now notes that `--server-url` may include a proxy base path.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./pkg/tarsclient ./internal/tarsclient ./cmd/tars`

### Closed

- Closes #408.

## [0.31.74] - 2026-05-01

### Changed

- `tars service start`, `stop`, and `status` now operate from LaunchAgent plist and `launchctl` state without requiring a readable runtime config.
- `tars service install --label/--domain` now records the launchd identity in the LaunchAgent environment so server restart detection uses the installed custom label/domain.
- Server and assistant LaunchAgent plist serialization now share the same internal helper.

### Documentation

- README now notes that macOS service inspection/control remains available even when config repair is needed.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./cmd/tars ./internal/assistant ./internal/tarsserver ./internal/launchagent`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make build`
- Manual macOS smoke: temp LaunchAgent install, then `service status` with intentionally broken config.

### Closed

- Closes #409.

## [0.31.73] - 2026-05-01

### Removed

- Fresh workspace bootstrap no longer creates legacy `memory/wiki`, `memory/wiki/notes`, `index.md`, or `graph.json` KB Wiki artifacts.
- Doctor required workspace checks no longer require legacy KB Wiki paths.
- Removed dead `/v1/memory/kb/*` route registrations from the API mux.

### Fixed

- Existing legacy `memory/wiki` files are preserved when `EnsureWorkspace` runs; they are no longer created or deleted automatically.

### Documentation

- README now clarifies that fresh workspace bootstrap omits legacy KB Wiki scaffolding.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./cmd/tars ./internal/memory ./internal/tarsserver`
- Fresh workspace smoke confirming `memory/wiki` is not created.

### Closed

- Closes #410.

## [0.31.72] - 2026-05-01

### Changed

- Assistant CLI defaults now use the core `~/.tars/workspace` location instead of falling back to `./workspace`.
- Hub command metadata now uses explicit plural nouns so help text renders `skills`, `plugins`, and `MCP servers` correctly.

### Fixed

- Legacy `tars init` config migration now returns an error when `workspace_dir` correction cannot be read, parsed, resolved, marshaled, or written.

### Documentation

- README now notes that assistant helpers share the core workspace default unless overridden.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./cmd/tars ./internal/assistant`
- Manual help checks for `tars skill update --help`, `tars plugin update --help`, and `tars mcp update --help`.

### Closed

- Closes #411.

## [0.31.71] - 2026-05-01

### Changed

- Hub update commands now report updated, skipped, and failed entries separately for skills, plugins, and MCP servers.
- `/v1/hub/update` now returns structured skill/plugin update results while preserving the existing `updated_skills` and `updated_plugins` arrays.

### Fixed

- Skill, plugin, and MCP updates now return final `skillhub.json` save errors instead of dropping them after package files were updated.
- MCP update failures now surface as per-entry diagnostics and aggregate errors instead of being silently skipped.
- macOS assistant popup result messages now build raw preview text first and AppleScript-escape it once, including quotes, backslashes, newlines, and CJK text.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test -count=1 ./internal/skillhub ./cmd/tars ./internal/assistant ./internal/tarsserver`

### Closed

- Closes #412.

## [0.31.70] - 2026-05-01

### Changed

- Clarified arbitrary root file previews by routing selected filesystem roots through `/v1/filesystem/files` while keeping `/v1/workspace/files` as the workspace-artifacts alias.
- Filesystem previews now have traversal, symlink, and explicit-root coverage around the selected root boundary.
- Workspace reset responses now report partial deletion/reinitialization failures and return an error when reset is incomplete.

### Fixed

- Settings reset messaging now uses the server's `removed` count instead of the stale `removed_dirs` field.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'WorkspaceFilesHandler_(Allows|Rejects)|ResetWorkspaceReports|RegisterAPIRoutes'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver`
- `npm run check` in `frontend/console`

### Closed

- Closes #413.

## [0.31.69] - 2026-05-01

### Changed

- Canonicalized Agent Runtime run and agent-list API calls under `/v1/agentruntime/*`.
- `pkg/tarsclient` and internal client tests now use `/v1/agentruntime/agents` and `/v1/agentruntime/runs` by default.
- Kept `/v1/agent/agents` and `/v1/agent/runs` as explicit legacy aliases for compatibility.

### Documentation

- Updated Agent Runtime tutorial and roadmap notes to label `/v1/agent/*` as legacy-only.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver ./internal/tarsclient ./pkg/tarsclient -run 'AgentRunsAPIHandler_AgentsList|RegisterAPIRoutes|RuntimeClientEndpoints|RunShowsPolicy|RunsShowsPolicy|ResolveURL'`

### Closed

- Closes #414.

## [0.31.68] - 2026-05-01

### Changed

- Replaced the ambiguous `tars serve --serve-api=false` opt-out path with explicit `tars serve --config-check`.
- `tars serve --config-check` now validates server config, workspace setup, auth safety, usage tracking, LLM routing, and semantic memory configuration before exiting without binding the HTTP API.
- Development serve targets now start the API directly instead of passing the removed `--serve-api` flag.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./cmd/tars ./internal/tarsserver -run 'ServeSubcommand|TestRun_|ValidateAPIAuthSecurity'`

### Closed

- Closes #417.

## [0.31.67] - 2026-05-01

### Added

- CON-034: Settings now opens on a Quick Start tab that curates the core onboarding fields before the full Fields and YAML views.
- Quick Start cards validate provider credentials, LLM tier coverage, workspace path, auth mode, Pulse, Reflection, log level, and Telegram session scope while keeping Telegram bot token optional.
- Added a Settings LLM connection action that reuses `/v1/models` to check the currently configured default provider.

### Tests

- `npm test -- --test-name-pattern "quick start"` in `frontend/console`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #436.

## [0.31.66] - 2026-05-01

### Added

- CON-035: Settings field rows now show metadata badges for default values, modified values, restart-required changes, live-apply fields, and masked secrets.
- Config schema metadata now includes per-field `default_value` and `requires_restart` information, plus config schema responses include the config file `updated_at` timestamp for modified badges.
- Field badges reuse the existing schema metadata flow so future live-apply fields can opt into the `live` badge without UI rewiring.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/config -run TestSchemaIncludesFieldMetaBadges`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run TestConfigAPI_SchemaReflectsPatchedValues`
- `npm test -- --test-name-pattern "config meta badges"` in `frontend/console`

### Closed

- Closes #437.

## [0.31.65] - 2026-05-01

### Added

- CON-036: Settings pending-change review now includes impact previews for high-signal fields.
- Config schema metadata now carries maintained `impact` hints for fields such as pulse interval, log level, usage limits, semantic memory, reflection cadence, and agent runtime concurrency.
- Pulse interval previews add dynamic latency and tick-volume hints based on the old and new durations.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/config -run TestSchemaIncludesImpactHintsForHighSignalFields`
- `npm test -- --test-name-pattern "impact preview"` in `frontend/console`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #438.

## [0.31.64] - 2026-05-01

### Added

- CON-037: `/console` now resolves to a Home dashboard instead of redirecting into Chat.
- Home surfaces Pulse, Reflection, disk pressure, active main sessions, recent notifications, recommended setup actions, and the latest session plan to continue.
- Chat remains available at the explicit `/console/chat` route, and the sidebar keeps Home on the TARS logo.

### Tests

- `npm test -- --test-name-pattern "Home"` in `frontend/console`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #439.

## [0.31.63] - 2026-05-01

### Added

- CON-038: The console sidebar now groups navigation into Work, Operate, and Setup sections, with Home remaining on the TARS logo.
- Settings now appears under Setup while the existing `/console/config` route remains intact.

### Tests

- `npm test -- --test-name-pattern "Console nav groups"` in `frontend/console`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #440.

## [0.31.62] - 2026-05-01

### Added

- CON-039: Memory's `Try a Search` panel now has `Tool path` and `Prefetch path` modes so explicit `memory_search` results can be compared with automatic Prior Context recall.
- Added `POST /v1/memory/prefetch`, returning the rendered `## Prior Context` section, source-tagged snippets, token usage, and budget percentage.
- Prefetch mode supports an optional session id and renders source badges plus the exact prompt section used by the automatic memory path.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'TestMemoryAPIHandler_PrefetchBuildsPriorContextPreview|TestRegisterAPIRoutes'`
- `npm test -- --test-name-pattern "Prefetch path"` in `frontend/console`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #441.

## [0.31.61] - 2026-05-01

### Added

- CON-040: Chat now includes a `Prior` side panel that previews the exact `## Prior Context` section the next draft message would add to the system prompt.
- Added `POST /v1/chat/prior-context/preview`, returning structured source badges, snippets, relevant token usage, budget percentage, and the rendered prompt section.
- The prompt builder now preserves structured Prior Context item metadata for conversation, experience, project, and daily sources while keeping the injected prompt text in sync.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/prompt -run TestBuildResultFor_ExposesPriorContextPreviewItems`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run TestChatAPIHandler_PriorContextPreviewEndpointReturnsExactSectionAndItems`
- `npm test -- --test-name-pattern "Prior Context"` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/prompt ./internal/tarsserver`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #442.

## [0.31.60] - 2026-05-01

### Added

- CON-041: Recorded the accepted decision to keep the Approvals workflow as TARS' human review queue for risky operational mutations.
- Added `docs/decisions/approvals-workflow.md` with the routing policy for manual cleanup plans, Pulse autofix, and future approval queue item types.
- Updated the ops approval tutorial and roadmap notes so CON-025/CON-026 follow-up work strengthens Approvals instead of removing it.

### Tests

- `npm test -- --test-name-pattern "Approvals workflow RFC"` in `frontend/console`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`

### Closed

- Closes #443.

## [0.31.59] - 2026-05-01

### Added

- CON-044: Extensions now includes an MCP Server Creator wizard for Python FastMCP and Node MCP SDK stdio boilerplate.
- New admin MCP Server Creator endpoints draft `tars.mcp.json` packages, save edited files into `workspace/mcp-servers/<name>/`, and prepare tars-skills draft PR handoff commands.
- The creator can run an isolated stdio validation sandbox that probes `tools/list` and `tools/call`, returning tool names, call output, worker/hidden sandbox metadata, and a tool trail.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'TestMCPServerCreator|TestRegisterAPIRoutes'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver`
- `npm test -- --test-name-pattern "MCP Server Creator"` in `frontend/console`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`

### Closed

- Closes #446.

## [0.31.58] - 2026-05-01

### Added

- CON-043: Skill Creator drafts can now run a sandbox Test before local save or draft PR preparation.
- The new `/v1/admin/skills/test` endpoint writes the edited draft into an isolated `workspace/tmp/skill-tests/` sandbox, executes the generated companion CLI with a timeout, and returns stdout, stderr, exit code, worker/hidden sandbox metadata, and a tool trail.
- The Extensions Skill Creator wizard now includes a Test action and inline pass/fail output so broken CLI stubs or missing runtime dependencies are visible before publishing.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'TestSkillCreatorAPI_Test|TestRegisterAPIRoutes'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver`
- `npm test -- --test-name-pattern "Skill Creator"` in `frontend/console`
- `npm run check` in `frontend/console`

### Closed

- Closes #445.

## [0.31.57] - 2026-05-01

### Added

- CON-042: Extensions now includes a Skill Creator wizard that drafts `SKILL.md` frontmatter/body plus Python, TypeScript, or Shell companion CLI boilerplate from a natural-language use case.
- New admin Skill Creator endpoints generate local drafts, save edited files into `workspace/skills/<name>/`, and expose a safe draft-PR readiness response for the external `tars-skills` publishing flow.
- The wizard supports language/layout selection, recommended tool inference and editing, file preview/editing, local save, and reloads the installed skills list after save.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'TestSkillCreator|TestRegisterAPIRoutes'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver`
- `npm test -- --test-name-pattern "Skill Creator"` in `frontend/console`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make console-build`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #444.

## [0.31.56] - 2026-05-01

### Added

- CON-047: Console Agent Runtime runs now include a Svelte Flow live graph mode with pan/zoom navigation, MiniMap, Controls, and Background.
- The Flow graph projects runs into tier-shaped/status-colored nodes, spawn edges, and consensus variant fan-out nodes with running animations.
- Flow filters support tier, status, and session, with a Replay control placeholder for the run-detail replay surface.

### Tests

- `npm test -- --test-name-pattern "Svelte Flow|buildAgentRuntimeFlowGraph"` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make console-build`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #449.

## [0.31.55] - 2026-05-01

### Added

- CON-046: Console Agent Runtime runs now include dependency-free Tree and Gantt visualization modes alongside the existing list view.
- The Tree mode renders parent/child run structure with depth, tier shape, status color, and direct run-detail navigation.
- The Gantt mode renders run duration bars and consensus variant sub-bars on a shared timeline for quick parallelism scanning.

### Tests

- `npm test -- --test-name-pattern "Agent Runtime.*(tree|Gantt)|buildAgentRuntime"` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make console-build`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #448.

## [0.31.54] - 2026-05-01

### Added

- CON-048: Console Agent Runtime run details now include a Replay scrubber that reconstructs run state from timestamped live events up to the selected cursor time.
- Replay supports Live lock, Play/Pause, 1x/2x/5x playback speed, first/last event timestamps, event progress, status, last event, message, and replayed file path chips.

### Tests

- `npm test -- --test-name-pattern "replay"` in `frontend/console`
- `npm test -- --test-name-pattern "Agent Runtime"` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #450.

## [0.31.53] - 2026-05-01

### Added

- CON-049: Console Agent Runtime run details now include a pure-SVG Cost Flow panel that visualizes parent/run → agent → variant flow by actual cost or token volume.
- Cost Flow includes tier-colored links, exact variant cost/token rows, and a budget summary when `consensus_budget_usd` is available.

### Tests

- `npm test -- --test-name-pattern "cost flow"` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #451.

## [0.31.52] - 2026-05-01

### Added

- CON-050: Agent Runtime now captures file-oriented tool calls (`read_file`, `list_dir`, `write_file`, `edit_file`) as `tool.call` run events and accumulates a per-run file attention summary.
- The Console Agent Runtime run detail now includes a File Attention panel with frequency-ranked files, read/edit counts, intensity cells, and mini sparklines.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/agentruntime -run TestRuntimeCapturesFileToolCallSummaryAndEvent`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run TestNewAgentPromptRunnerWithTools_ForwardsFileToolCallsToRuntimeRecorder`
- `npm test -- --test-name-pattern "file attention"` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/agentruntime ./internal/tarsserver`
- `npm test -- --test-name-pattern "Agent Runtime"` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #452.

## [0.31.51] - 2026-05-01

### Changed

- Q-015 follow-up: `subagents_run` no longer advertises `mode=consensus` or the `consensus` argument object while `agentruntime_consensus_enabled=false`.
- Consensus remains available as an advanced config opt-in; when enabled, the `subagents_run` schema exposes the consensus mode and runtime behavior is unchanged.
- Disabled consensus calls now return an immediate diagnostic before spawning an Agent Runtime run.
- Config schema, example config, README, usage-signal notes, and Console copy now describe consensus as advanced opt-in rather than routine run data.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tool -run 'TestSubagentsRunTool_(HidesConsensusSchemaWhenRuntimeGateDisabled|ExposesConsensusSchemaWhenRuntimeGateEnabled|RejectsConsensusBeforeSpawnWhenRuntimeGateDisabled|SpawnsParallelExplorerChildrenAndReturnsSummaries)'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/config -run 'TestSchema_UsesPreferredHierarchicalPaths|TestLoad_ExampleConfigHierarchicalSchema'`
- `npm test -- --test-name-pattern "Agent Runtime"` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache make test`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #507.

## [0.31.50] - 2026-05-01

### Changed

- Q-017 follow-up: the per-session tool/skill configuration panel is no longer an always-visible Chat toolbar button after a fresh signal window again showed 0 `session.tool_config.updated` rows.
- The advanced `/config` command still opens session-scoped policy controls for an existing selected session, and backend `SessionToolConfig` filtering plus usage telemetry remain intact for explicit opt-in and diagnostics.

### Tests

- `npm test -- --test-name-pattern "session config"` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'Test(ChatAPIHandler_ContextEndpointReflectsSessionGroupConfigAfterPatchAPI|SessionAPIHandler_ConfigPatchRecordsUsageSignal)'`

### Closed

- Closes #508.

## [0.31.49] - 2026-05-01

### Changed

- Q-012 follow-up: `subagents_plan` and `subagents_orchestrate` are now advanced opt-in tools rather than default chat schema surfaces, while `subagents_run` remains the default path for parallel delegated work.
- README, chat runtime guidance, subagent mention hints, and Console Agent Runtime copy now steer users toward `subagents_run` by default.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'TestResolveInjectedToolSchemas_(HidesSubagentFlowToolsByDefault|AllowsSubagentFlowToolsWhenExplicitlyEnabled|AllowAdminHighRiskTools|AllowHighRiskUserOverride)|TestChatAPI_SubagentsToolCall'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tool ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `npm test -- --test-name-pattern "Agent Runtime"` in `frontend/console`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #509.

## [0.31.48] - 2026-05-01

### Changed

- Q-011 follow-up: the low-use `process` tool is no longer injected into the default chat tool schema, while explicit session tool allowlists can still opt into it and background `exec` keeps its shared process manager.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run 'TestResolveInjectedToolSchemas_(AllowAdminHighRiskTools|AllowHighRiskUserOverride|AllowsDeprecatedProcessWhenExplicitlyEnabled)'`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/tool ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #510.

## [0.31.47] - 2026-05-01

### Changed

- Usage signal docs now include the 2026-04-26 to 2026-04-30 decision snapshot for Q-011, Q-012, Q-014, Q-015, Q-017, and Q-018, with focused follow-up issue links for the low-use surfaces.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/usage ./internal/tarsserver`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #418.

## [0.31.46] - 2026-05-01

### Added

- Agent Runtime runs now support status chips, 24h/7d/all time ranges, prompt search, originating chat session links, and top-level cost summaries for today, seven days, and grouped plan totals.
- `/v1/agentruntime/runs` now accepts `status`, `since`, and `search` query parameters for filtered run lists.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/tarsserver -run TestAgentRunsAPIHandler_ListFiltersStatusSinceAndSearch`
- `npm test -- --test-name-pattern "Agent Runtime runs page|Agent Runtime run API client"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #420.

## [0.31.45] - 2026-04-30

### Added

- Memory now opens with a dismissible introduction card that explains MEMORY.md, Experiences, Daily Logs, Semantic Index, Prior Context recall, and the Try a Search workflow before editing.

### Tests

- `npm test -- --test-name-pattern "Memory page introduces|Memory page uses friendly|memory asset metadata explains"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #421.

## [0.31.44] - 2026-04-30

### Changed

- Memory now uses friendlier Stored Knowledge and Try a Search tab labels, with asset cards showing human-readable descriptions and hover hints for common memory files.

### Tests

- `npm test -- --test-name-pattern "Memory page uses friendly|memory asset metadata explains"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #422.

## [0.31.43] - 2026-04-30

### Added

- Memory asset cards now explain who fills each durable asset, who reads it, and when experience logs become stale after seven quiet days.

### Tests

- `npm test -- --test-name-pattern "memory asset metadata"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #423.

## [0.31.42] - 2026-04-30

### Changed

- System Prompt diagnostics are now hidden behind a default-closed technical details toggle so role semantics and built-in tool descriptions stay available without adding first-view noise.

### Tests

- `npm test -- --test-name-pattern "System Prompt diagnostics"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #424.

## [0.31.41] - 2026-04-30

### Added

- System Prompt now shows per-file prompt impact metadata with estimated tokens, section mapping, section character limits, truncation warnings, and a reloadable main/sub-agent system prompt preview.

### Tests

- `npm test -- --test-name-pattern "System Prompt page surfaces"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `GOCACHE=/tmp/tars-go-cache go test ./internal/prompt ./internal/sysprompt ./internal/tarsserver`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #425.

## [0.31.40] - 2026-04-30

### Added

- System Prompt now offers starter templates for empty or placeholder `IDENTITY.md`, `AGENTS.md`, and `TOOLS.md` files so users can insert opinionated defaults before saving.

### Tests

- `npm test -- --test-name-pattern "sysprompt"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #426.

## [0.31.39] - 2026-04-30

### Changed

- Operations is now an Approvals-focused console page with Disk/Process/Cron readouts removed, the nav renamed to Approvals, and `/console/approvals` routed alongside the legacy `/console/ops` path.

### Tests

- `npm test -- --test-name-pattern "Operations becomes"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #427.

## [0.31.38] - 2026-04-30

### Added

- Approvals now shows an empty-state guide explaining the review queue, cleanup-plan trigger, future Pulse-triggered approvals, approval decisions, and result logs.

### Tests

- `npm test -- --test-name-pattern "Ops explains"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #428.

## [0.31.37] - 2026-04-30

### Added

- Pulse now opens with a System Watchdog introduction card that explains monitored signals, LLM classifier actions, and the Settings `pulse_*` policy source before status readouts.

### Tests

- `npm test -- --test-name-pattern "Pulse introduces"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #429.

## [0.31.36] - 2026-04-30

### Added

- Pulse now compresses all-clear Recent Ticks into a summary timeline and highlights only signal-bearing ticks with warning, error, and autofix counts.

### Tests

- `npm test -- --test-name-pattern "Pulse compresses"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #430.

## [0.31.35] - 2026-04-30

### Added

- Pulse now explains the Min Severity notification floor, signal-kind severity mappings, threshold sources, and last-seen times inline on the status card.

### Tests

- `npm test -- --test-name-pattern "Pulse explains"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #431.

## [0.31.34] - 2026-04-30

### Added

- Reflection now starts with a Nightly Maintenance introduction card explaining the sleep window, memory job, cleanup job, manual run behavior, and Pulse failure signal.

### Tests

- `npm test -- --test-name-pattern "Reflection page introduces"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #432.

## [0.31.33] - 2026-04-30

### Added

- Reflection now previews the expected `Run Reflection Now` output before the first run and shows run totals, job details, errors, duration, and previous-run deltas after a manual run.

### Tests

- `npm test -- --test-name-pattern "Reflection"` in `frontend/console`
- `npm run check` in `frontend/console`
- `npm test` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #433.

## [0.31.32] - 2026-04-30

### Added

- Extensions now explains Skills, Plugins, and MCP Servers inline in both Installed and Hub views so each extension surface has a concise definition near its controls.

### Tests

- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #434.

## [0.31.31] - 2026-04-30

### Added

- Extensions now keeps Plugins collapsed by default in both Installed and Hub views and labels them as an advanced legacy surface while preserving existing plugin controls behind expansion.

### Tests

- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #435.

## [0.31.30] - 2026-04-30

### Added

- Extensions now marks Plugins as deprecated in both Installed and Hub views and points new extension work toward Skills (`.md + CLI`) while keeping legacy plugin installs available.

### Tests

- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make console-build`
- `GOCACHE=/tmp/tars-go-cache make test`
- `GOCACHE=/tmp/tars-go-cache make vet`
- `make security-scan`
- `GOCACHE=/tmp/tars-go-cache make build`

### Closed

- Closes #447.

## [0.31.29] - 2026-04-30

### Fixed

- Extension disabled-state updates now preserve corrupt state files and return load errors instead of silently replacing them with empty state.
- Ops approvals and usage limits now use atomic writes so failed state writes preserve the previous file contents.
- Ops manager empty-workspace defaults now use the same core default workspace path as runtime config.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/extensions ./internal/ops ./internal/usage ./internal/tarsserver`

### Closed

- Closes #415.

## [0.31.28] - 2026-04-30

### Fixed

- Skill runtime mirroring now surfaces companion file copy failures and removes affected skills from the runtime snapshot instead of leaving partially mirrored skills available.

### Tests

- `GOCACHE=/tmp/tars-go-cache go test ./internal/skill ./internal/extensions`

### Closed

- Closes #416.

## [0.31.27] - 2026-04-30

### Added

- Chat `@` mention autocomplete now includes AgentRuntime subagents alongside Files context.
- Selected subagent mentions are sent as explicit `subagent_mentions` chat hints and injected into the LLM system prompt so `subagents_run`, `subagents_orchestrate`, and `subagents_plan` can target the named agent.
- Context HUD now reports mentioned subagents for the current turn.

### Tests

- `go test ./internal/tarsserver -run 'TestChatAPI(InjectsSubagentMentionHints|RejectsUnknownSubagentMention|ToolCallSubagentsRun)'`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make console-build`
- `make fmt`
- `make vet`
- `GOCACHE=/tmp/tars-go-cache make test`
- `make security-scan`
- `make build`
- Playwright browser verification at `http://127.0.0.1:43195/console`

## [0.31.26] - 2026-04-29

### Added

- Chat composer now supports leading `/` command autocomplete with built-in console actions and user-invocable skills.
- Skills can declare `slash` and `aliases` frontmatter metadata, and explicit `/skill` or `/alias` chat messages select that skill before the LLM turn starts.
- Extensions displays each user-invocable skill's slash command so installed skill entrypoints are easier to discover.

### Fixed

- Skill runtime paths injected into chat prompts now remain readable even when the active session current directory is not the workspace root.

### Tests

- `go test ./internal/skill ./internal/tarsserver -run 'TestParseFrontmatter|TestLoad_DefaultUserInvocableTrue|TestResolveSkillForMessage_UsesSkillSlashAlias|TestPrepareChatContextWithExtensions_InvokedSkillHint|TestSkillRuntimeReadPathForPrompt'`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make fmt`
- `make vet`
- `make test`
- `make console-build`
- `make build`
- `make security-scan`
- Browser verification: Playwright opened `/console`, confirmed `/` lists built-in commands and skill entries, selected `/qa-check` from `/qa` autocomplete, sent `/qa-check smoke`, verified `skill selected: qa-check`, and confirmed the LLM successfully read `workspace/_shared/skills_runtime/qa_check/SKILL.md`.

### Closed

- Closes #469.

## [0.31.25] - 2026-04-28

### Added

- Files > Workspace now includes an embedded Shell view powered by xterm, WebSocket streaming, and a PTY-backed server process.
- Integrated terminals start in the selected session work directory or browsed subdirectory and support keyboard input, command output, resizing, and explicit close.
- The existing macOS external Terminal action remains available as an Open App fallback.

### Security

- Integrated terminal WebSocket requests require admin access and reuse the session Files root validation before any PTY process starts.
- Requests outside session work directories, missing directories, files, and relative traversal outside the selected root are rejected.

### Fixed

- Request logging middleware now preserves HTTP hijacking so WebSocket upgrades can pass through the shared API middleware stack.

### Tests

- `go test ./internal/tarsserver -run 'TestRequestDebugMiddlewareSupportsWebSocketHijack|TestTerminalAPI_WebSocket|TestRegisterAPIRoutes_RegistersCoreRoutes'`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make fmt`
- `make vet`
- `make test`
- `make console-build`
- `make build`
- `make security-scan`
- Browser verification: Playwright opened `/console`, selected the main session, opened Files > Shell, verified terminal input/output by running `echo TARS_TERMINAL_OK` and `pwd`, and checked zero browser console errors or warnings.

### Closed

- Closes #484.

## [0.31.24] - 2026-04-28

### Added

- Files > Workspace now includes a Terminal action that opens the macOS Terminal app at the current session work directory or browsed subdirectory.
- `/v1/terminal/open` launches an external terminal for a session after resolving the requested cwd against the session's registered Files roots.

### Security

- Terminal launch requests require admin access and reject paths outside the session work directories, missing directories, files, and relative traversal outside the selected root.

### Tests

- `go test ./internal/tarsserver -run 'TestTerminalAPI|TestRegisterAPIRoutes_RegistersCoreRoutes'`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make fmt`
- `make vet`
- `make test`
- `make console-build`
- `make build`
- `make security-scan`
- Browser verification: Playwright opened `/console/chat/<session>`, confirmed Files > Workspace shows an enabled Terminal action for the selected session workdir, clicked it, verified `/v1/terminal/open` returned 200, and checked zero browser console warnings.

### Closed

- Closes #482.

## [0.31.23] - 2026-04-28

### Added

- Chat composer now supports `@` file and directory mentions sourced from the session Files roots.
- Mention autocomplete resolves against the session current directory and registered Files paths, then injects selected file content or directory listings into the next LLM request.
- Mentioned context is reported in the Context HUD for turn-level visibility.

### Security

- File mention resolution is revalidated server-side and rejects parent traversal, missing paths, and roots outside the session Files paths.

### Tests

- `go test ./internal/tarsserver -run 'TestChat(FileMention|APIInjects|APIRejects)'`
- `go test ./internal/tarsserver`
- `go test ./...`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make fmt`
- `make vet`
- `make test`
- `make console-build`
- `make build`
- Browser verification: Playwright opened `/console/chat/<session>`, confirmed typing `@rea` shows the `README.md` autocomplete candidate from the session current Files root, selected it with Enter, and verified the composer shows an active `@README.md` mention chip.

### Closed

- Closes #468.

## [0.31.22] - 2026-04-28

### Added

- Agent Runtime Subagents now includes an LLM-assisted builder for drafting new workspace `AGENT.md` profiles from a natural-language request.
- Workspace subagents can be edited with the builder, previewed, approved, and saved back to their source profile.
- Editable workspace subagents can be archived with confirmation; archived `AGENT.md` files are renamed out of the active catalog and the runtime executor list is refreshed.
- Builder drafts use configured LLM tiers, expose safe tool allow/deny lists, and normalize common LLM action aliases such as `edit` into the persisted update workflow.

### Tests

- `go test ./internal/tarsserver -run 'TestAgentRuntimeSubagentsAPIHandler_(Builder|Patch|List|Detail)|TestAgentRuntimeSubagentBuilderLLMPromptMentionsJSON|TestNormalizeAgentRuntimeSubagentDraftMapsLLMEditAction'`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make fmt`
- `make vet`
- `make test`
- `make console-build`
- `make build`
- `make security-scan`
- Browser verification: Playwright opened `/console/agentruntime/subagents`, generated a new `frontend-reviewer` workspace subagent with the LLM builder on the `heavy` tier, approved and saved it, edited `researcher` with an accessibility-focused LLM draft, approved and saved the edit, archived `researcher`, confirmed the API catalog and workspace files reflected the changes, checked zero browser console warnings, and verified no horizontal overflow at 390px mobile width.

### Closed

- Closes #472.

## [0.31.21] - 2026-04-28

### Added

- Settings now opens `llm.tiers` in a typed tier editor instead of the generic JSON editor.
- The typed tier editor can add, rename, edit, and remove custom tier bindings with separate controls for provider, model, reasoning effort, thinking budget, and service tier.
- Tier provider choices are populated from configured `llm.providers`, and invalid tier rows show inline errors before changes can be staged or saved.

### Tests

- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make fmt`
- `make vet`
- `make test`
- `make console-build`
- `make build`
- `make security-scan`
- Browser verification: Playwright opened `/console/config`, confirmed `llm.tiers` uses the typed editor, verified missing model validation stays inline, added a `turbo` tier, changed `heavy` to `gpt-5.5`, removed `light`, saved via Settings, confirmed the refreshed schema API and config file include the saved tiers, checked zero browser console warnings, and verified no horizontal overflow at 390px mobile width.

### Closed

- Closes #475.

## [0.31.20] - 2026-04-28

### Fixed

- Settings JSON editor modals now center within the content area beside the fixed navigation instead of rendering underneath the sidebar.
- The JSON editor now keeps consistent viewport margins and a bounded editor height on desktop and mobile.

### Tests

- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make fmt`
- `make vet`
- `make test`
- `make console-build`
- `make build`
- `make security-scan`
- Browser verification: Playwright opened `/console/config`, opened `llm.tiers`, confirmed the modal and backdrop clear the fixed sidebar on a 1175px desktop viewport, verified the modal fits within the viewport, confirmed no horizontal overflow at 390px mobile width, and checked zero browser console warnings.

### Closed

- Closes #477.

## [0.31.19] - 2026-04-28

### Added

- Settings now summarizes object and array values with compact counts and key previews instead of rendering hard-to-read one-line JSON blobs.
- Object and array config fields now open a focused JSON editor modal with pretty-printed content, reset/cancel/apply actions, and inline parse errors before changes are staged.
- `/v1/admin/config/schema` now refreshes values from the config file after Settings saves, while preserving the runtime workspace override shown in the Console.

### Tests

- `TestConfigAPI_SchemaReflectsPatchedValues`
- `npm test` in `frontend/console`
- `npm run check` in `frontend/console`
- `make fmt`
- `make vet`
- `make test`
- `make build`
- `make console-build`
- `make security-scan`
- Browser verification: Playwright opened `/console/config`, confirmed structured summaries for `llm.tiers`, verified invalid JSON stays in the editor with an inline parse error, saved a new `turbo` tier, confirmed the refreshed schema API and config file include the tier, checked no browser console warnings, and verified no horizontal overflow at 390px mobile width.

### Closed

- Closes #473.

## [0.31.18] - 2026-04-27

### Added

- Agent Runtime now has a `Runs | Subagents` tab split in the Console. The new Subagents tab shows the active agent catalog, default/effective LLM tier, resolved provider/model preview, source/entry metadata, tool policy, and recent run links.
- Workspace `AGENT.md` subagents can now update their default LLM tier from the Subagents detail panel using the configured LLM tier catalog.
- Added `/v1/agentruntime/subagents` and `/v1/agentruntime/subagents/{name}` endpoints that expose subagent metadata with Settings-defined LLM tier options, clear missing-tier diagnostics, and safe tier updates for editable workspace profiles.

### Fixed

- Agent Runtime runs now use an executor's configured agent tier when the spawn request does not provide a task-level tier, matching the documented priority of task `tier` > agent YAML `tier` > config default.
- Empty subagent tool-policy arrays are serialized as `[]` instead of `null`, preventing Console detail rendering errors.

### Tests

- `TestRuntimeSpawn_UsesExecutorTierWhenRequestTierEmpty`
- `TestAgentRuntimeSubagentsAPIHandler_ListIncludesTiersAndRunTelemetry`
- `TestAgentRuntimeSubagentsAPIHandler_DetailMarksMissingTier`
- `TestAgentRuntimeSubagentsAPIHandler_PatchWorkspaceTierReloadsExecutor`
- `TestAgentRuntimeSubagentsAPIHandler_PatchRejectsUnknownTier`
- `npm run check` in `frontend/console`
- `make console-build`
- `make fmt`
- `make vet`
- `make test`
- `make build`
- Browser verification: Playwright opened `/console/agentruntime/subagents` on a local TARS server, confirmed tier catalog/list/detail rendering, selected the `researcher` subagent, returned to the Runs tab, and verified no horizontal overflow at 390px mobile width.

### Closed

- Closes #471.

## [0.31.17] - 2026-04-27

### Added

- Agent Runtime page onboarding card explaining that the page records subagent work launched from chat, including the prompt, model tier, status, response, live events, and consensus cost data when available.
- Agent Runtime empty state guide with starter chat prompts for `subagents_run` / `subagents_orchestrate` / `subagents_plan` workflows plus a direct "Open Chat" action.

### Tests

- `npm run check` in `frontend/console`
- `make console-build`
- `make build`
- `make test`
- Browser verification: Playwright opened `/console/agentruntime` on a local TARS server, confirmed the onboarding/empty-state text on desktop and mobile widths, and verified no horizontal overflow at 390px.

### Closed

- Closes #419.

## [0.31.16] - 2026-04-27

### Fixed

- Context compaction now reinjects the session's active plan state immediately after the compaction summary for automatic chat compaction, manual session compaction, `/v1/compact`, cron-bound session runs, and Telegram-bound session runs. The reinjected block includes the active plan plus `pending` / `in_progress` tasks only, so completed or cancelled tasks are not resurfaced to the LLM.
- History loading now treats the active-plan injection as part of the compaction boundary, so tiny token budgets that force-include the compaction summary also keep the preserved task state visible.
- Repeated compactions replace older injected task blocks with fresh state instead of carrying stale task snapshots forward.

### Tests

- `TestFormatTasksForInjection_ExcludesInactivePlanWithNoActiveTasks`
- `TestLoadHistory_IncludeCompactionTaskInjectionBoundary`
- `TestCompactAPI_ReinjectsActiveTasks`
- `TestHandleChatRequest_EmitsCompactionAppliedEvent`
- Browser verification: Playwright opened a local TARS server, created a session/tasks, triggered manual compaction, and confirmed the transcript contains summary + active-plan injection with completed tasks omitted.

### Closed

- Closes #392.

## [0.31.15] - 2026-04-27

### Added

- `runtime.plan_clarify_mode` config (env: `TARS_PLAN_CLARIFY_MODE`, schema dropdown in Settings → Runtime). Three values:
  - `smart` (default) — LLM evaluates ambiguity itself: asks 1–3 clarifying questions when scope/success/constraints are unclear, drafts immediately when clear.
  - `auto` — never ask, always draft immediately.
  - `ask` — always front-load 1–3 clarifying questions before drafting.
- The Planning section in the system prompt now branches on the mode. Unknown / empty values fall back to `smart` so a typo can't silently flip planning into the noisier `ask` stance. The downstream propose/approve guidance (CON-053) and runtime intervention vocabulary (CON-054) remain identical across all three modes.

### Tests

- `TestBuild_PlanningSectionClarifyModes` — locks the per-mode prompt content and verifies unknown values fall back to smart.

### Closed

- Closes #455.

## [0.31.14] - 2026-04-27

### Added

- Runtime intervention buttons in TasksPanel during `executing` / `paused` states. Plan-level toolbar offers **⏸ Pause** (cancels the in-flight chat turn first via `cancelChat`, then `plan_pause`), **▶ Resume** (flips back to executing and auto-sends `continue`), **✎ Edit Plan** (reuses CON-053 inline editor for retitle/add/remove), and **⊘ Abort** (confirm + chat cancel + `plan_abort`). Each task card gains a per-row **⏭ Skip** button when the plan is live: marks the task `cancelled` and asks the LLM to move on with a structured follow-up message ("Skip task N (title) and continue with the next pending task").
- Resume sends `continue` so the LLM picks up the next turn without the user retyping; failure of the auto-send is best-effort.

### Closed

- Closes #457.

## [0.31.13] - 2026-04-27

### Added

- Propose/approve UI in the chat TasksPanel. When the LLM moves the plan to `proposed`, the panel surfaces a "Plan ready for review" banner with three CTAs: **✓ Approve & Run** (calls `plan_approve` and auto-sends `go` so the LLM picks the next turn), **✎ Edit Plan** (per-task title/description inputs, add/remove rows, save in one batch), and **✗ Discard** (confirm + `clear`). The plan card itself now shows a colored status badge (`drafting` / `proposed` / `executing` / `paused` / `completed` / `aborted`).
- `POST /v1/admin/sessions/{id}/tasks` — drives the tasks aggregator from the console without going through the chat loop. Body shape mirrors the LLM-side tool call (`{"action":"plan_approve"}`, `{"action":"add","title":"…"}`, etc.). Backed by the same `NewTasksTool` instance the chat path uses, so state-machine guarantees (CON-051) hold uniformly across both surfaces.
- `ChatPanel.sendMessageText(text)` — programmatic submit that lets sibling panels (TasksPanel) emit follow-up turns without forcing the user back to the composer.
- Planning section in the system prompt expanded to teach the propose/approve flow (`plan_propose` → STOP → user `go` → `plan_approve`) and explain the runtime intervention vocabulary (`paused` / `aborted`).

### Tests

- `TestSessionAPI_TasksPOSTInvokesAggregator` — drives plan_set → propose → approve → invalid-transition rejection through the new endpoint.
- Browser verification: open Tasks panel, click Approve & Run, observe the chat send `go` and the LLM resume work with task 1 marked in_progress.

### Closed

- Closes #456.

## [0.31.12] - 2026-04-27

### Changed

- Renamed `config/standalone.yaml` → `config/default.yaml`. The "standalone" name was a leftover from the now-removed `runtime.mode` field; "default" matches what the file actually is. `DefaultConfigFilename`, `tars init` legacy candidates, doctor hints, Makefile, CLAUDE.md, and three test fixtures updated. The pre-rename name stays in `tars init`'s legacy candidate list so existing workspaces continue to migrate cleanly.
- `tars init` starter no longer emits `runtime.mode: standalone` (field removed in 0.31.11).
- Refreshed `config/tars.config.example.yaml` so every value matches current defaults: `claude-opus-4-7` / `claude-haiku-4-5-20251001` model IDs, `gemini-embedding-2-preview` embed model, `pulse.cron_failure_threshold=3` / `stuck_run_minutes=60` / `reflection_failure_threshold=3`, `compaction.trigger_tokens=100000` / `keep_recent_tokens=12000`, `assistant.enabled=true` / `whisper-cli` / canonical hotkey, `tools.default_set=standard`, `telegram.polling.enabled=true`, `notify.when_no_clients=true`, plus the missing `memory_hook` and `reflection_kb` role mappings. Also notes that external Go plugins are deprecated.

## [0.31.11] - 2026-04-27

### Removed (BREAKING)

- `runtime.mode` config field, `TARS_MODE` env var, and `--mode` CLI flag. The field was cosmetic from day one — server execution is decided entirely by the separate `--serve-api` boolean flag, never by Mode. The schema option, the Settings page combobox, and the startup message simply showed the value back to the user without affecting any code path.
- Settings page no longer offers a "Mode" combobox under Runtime — the field disappears automatically once removed from the schema.
- Startup stdout message changed from `tars starting in <mode> mode` to `tars startup complete (no --serve-api flag, exiting)` — describes the actual outcome instead of repeating a meaningless config value.

### Migration

- YAML parser ignores unknown keys, so existing `runtime.mode` lines in `workspace/config/tars.config.yaml` are silently dropped on next load. No user action required.

## [0.31.10] - 2026-04-26

### Added

- Plan state machine — `session.Plan` now carries a `Status` field with six states (`drafting`, `proposed`, `executing`, `paused`, `completed`, `aborted`) plus an `UpdatedAt` timestamp. State validation helper `session.ValidPlanStatus`.
- Five new `tasks` tool actions: `plan_propose` (drafting → proposed), `plan_approve` (proposed → executing), `plan_pause` (executing → paused), `plan_resume` (paused → executing), `plan_abort` (any active state → aborted). Each rejects invalid transitions with an explicit error.
- Two automatic transitions guard against LLM omissions and surface real progress: a task moving to `in_progress` auto-promotes a `proposed` plan to `executing`, and a plan flips to `completed` once every task is `completed` or `cancelled`.
- `SessionPlan.status` / `updated_at` exposed in the frontend type for upcoming UI work (CON-053 propose/approve, CON-054 runtime intervention).

### Migration

- Plans saved before this field existed have empty `Status` on disk; on load they default to `executing` so existing sessions keep their prior behavior with zero user action.

### Tests

- `TestValidPlanStatus`, `TestLegacyPlanWithoutStatusDefaultsToExecuting`
- `TestTasks_PlanSetSetsDraftingStatus`
- `TestTasks_PlanProposeDraftingToProposed` / `TestTasks_PlanProposeRejectsExecuting`
- `TestTasks_PlanApproveProposedToExecuting` / `TestTasks_PlanApproveRejectsDrafting`
- `TestTasks_AutoExecutingOnFirstInProgress`
- `TestTasks_AutoCompletedWhenAllTasksDone`
- `TestTasks_PlanPauseAndResume` / `TestTasks_PlanPauseRejectsDrafting`
- `TestTasks_PlanAbortFromAnyActiveState` / `TestTasks_PlanAbortRejectsTerminal`
- `TestTasks_PlanTransitionWithoutPlan`

### Closed

- Closes #454.

## [0.31.9] - 2026-04-26

### Fixed

- Pulse-bar `Tasks` badge now shows `(completed / total)` instead of `(in_progress / total)`. Between turns the in-progress count is almost always 0, so the badge always read `0/N` and looked broken even when work had finished. Matches the TasksPanel header. Hover tooltip surfaces the full breakdown (`N done · N in progress · N pending`).

## [0.31.8] - 2026-04-26

### Added

- `tasks_changed` SSE event — emitted after every `tasks` tool call with the live plan-goal + per-status counts so the chat pulse-bar Tasks badge stays in sync without polling.
- Initial task counts are fetched on session change so the badge reflects state from prior turns, not just the current chat-stream lifetime.

### Changed

- The chat pulse-bar `Tasks` button now displays an `(in_progress / total)` count when a plan exists (e.g. `Tasks (1/3)`) — restores the live counter that PR #291 promised but had since regressed.

### Tests

- `TestChatAPI_TasksToolEmitsTasksChangedEvent`

### Closed

- Closes #391.

## [0.31.7] - 2026-04-26

### Added

- Hardcoded `## Planning` section in the main-agent system prompt that instructs the LLM to use the `tasks` aggregator (`plan_set` / `add` / `update`) for multi-step requests. Sub-agent prompts skip the section.

### Tests

- `TestBuild_PlanningSectionPresentForMainAgent`
- `TestBuild_PlanningSectionAbsentForSubAgent`
- `TestBuild_PlanningSectionWithinBudget`

### Closed

- Closes #390.

## [0.31.6] - 2026-04-26

### Added

- Workspace-local usage signal counters in `workspace/usage/signals-YYYY-MM-DD.jsonl` for unresolved code-review questions.
- `GET /v1/usage/signals?period={today|week|month}` and `/usage signals {period}` for operator inspection.
- Narrow counters for tool calls, session tool-config updates, agent runtime persistence retries/errors, and consensus activation.
- `docs/usage-signals.md` mapping Q-011 through Q-018 to their runtime evidence source.

### Tests

- `TestTracker_RecordSignalAndSummarize`
- `TestUsageAPI_Signals`

### Closed

- Closes #386.

## [0.31.5] - 2026-04-26

### Changed (BREAKING)

- **ID-005 hard cut**: external gateway naming is now canonical `agentruntime` across HTTP routes, console routes, config/env/YAML keys, persistence paths, CLI commands, tool names, and public client types.
- Runtime persistence now defaults to `workspace/_shared/agentruntime/`, and archive files use the `agentruntime-*.jsonl` prefix.
- The legacy `/v1/gateway/*` routes and `gateway_*` / `gateway.*` config keys are intentionally not kept as compatibility aliases.

### Migration

- Replace `/v1/gateway/*` calls with `/v1/agentruntime/*` and `/console/gateway` bookmarks with `/console/agentruntime`.
- Rename `gateway_*` env/config keys and `gateway.*` YAML sections to `agentruntime_*` / `agentruntime.*`.
- Move retained runtime state from `workspace/_shared/gateway/` to `workspace/_shared/agentruntime/` if old run/channel history must be preserved.

### Tests

- `TestAgentRuntimeAPIHandler_HardCutRoutes`
- `TestLoad_AgentRuntimeHardCutIgnoresLegacyGatewayConfigKeys`

### Closed

- Closes #384.

## [0.31.4] - 2026-04-26

### Changed

- **RF-004**: `runtimeDeps` no longer stores a backward-compat `llmClient`. Server bootstrap keeps only the shared `llmRouter`, and chat/API call sites resolve the chat client through `llm.RoleChatMain` at the boundary where the client is needed.
- Agent Runtime prompt runners now use the router-backed default agent runtime role path instead of inheriting the chat-main client fallback from bootstrap. Session-bound cron and Telegram inbound paths use the same resolved chat client as the normal chat handler.

### Tests

- `TestRuntimeDepsDoesNotExposeLegacyLLMClient`
- `TestChatAPI_ResolvesChatClientFromRouter`

### Closed

- Closes #385.

## [0.31.3] - 2026-04-26

### Changed (BREAKING)

- **ID-005 PR #1 — package rename `internal/gateway` → `internal/agentruntime`**:
  - `git mv internal/gateway internal/agentruntime`
  - 모든 import path `github.com/devlikebear/tars/internal/gateway` → `github.com/devlikebear/tars/internal/agentruntime`
  - 식별자 사용 `gateway.X` → `agentruntime.X` 일괄 변경 (Runtime / SpawnRequest / AgentInfo / Run / RunStatus 등 50+ 위치)
  - 패키지 선언 `package gateway` → `package agentruntime`

### Migration

- 외부 코드가 `internal/gateway` 를 import 하던 경우 (워크스페이스 외부) `internal/agentruntime` 로 변경 필요. 단 `internal/*` 는 정의상 사용자 외부 import 불가.
- **변경 안 됨 (이번 PR)**: HTTP URL prefix (`/v1/gateway/*` 그대로), Config 필드 (`gateway_*` 그대로), workspace persistence dir (`workspace/_shared/gateway/` 그대로), 변수명/파일명/콘솔 UI 라벨. 후속 PR (Phase 2-6) 에서 단계적으로 마이그레이션.

### Why split

5 PR 분할 의 첫 단계. 패키지 이름만 먼저 바꿔 외부 호환성 (HTTP/config/persistence) 은 그대로 유지 — 각 단계에서 e2e 검증 가능. ID-005 옵션 결정은 #367 / #378.

## [0.31.2] - 2026-04-26

### Changed

- **ID-004 wrap-up — RF-049 codex advanced fields + RF-047 capability docs**:
  - `openai_codex_client.go`: `buildOpenAICodexRequestBody` 가 이제 `ClientConfig` 를 받아 `effectiveReasoningEffort` / `effectiveServiceTier` 매핑. Responses API 의 `reasoning: {effort: ...}` 객체와 `service_tier` 필드로 직렬화. 이전엔 `ReasoningEffort` / `ServiceTier` 옵션이 silent 무시 (RF-049).
  - `docs/llm-providers.md` 신규 — ChatOptions × Provider capability 매트릭스 + wire format 디테일 (specific tool / json_schema / reasoning / service_tier 표). 향후 `internal/llm` 의 wire-format converter 변경 시 같은 PR 에서 갱신할 것 (문서에 명시) (RF-047).

### Tests

- `TestOpenAICodexClient_ReasoningEffortAndServiceTier` — `reasoning.effort=high` 객체 + `service_tier=priority` 직렬화 검증.

### Closed (#366 / ID-004 종결)

이 PR 로 ID-004 옵션 (2) Phase 1+2 + 작은 후속 (RF-049/047) 완전 종결. 잔여 항목 (옵션 (3) 의 Gemini-native responseSchema/caching, 옵션 (5) 의 gemini-compat deprecate) 은 별도 RF/issue 로 트래킹하거나 사용자 결정 시 재개.

## [0.31.1] - 2026-04-26

### Changed

- **ID-004 Phase 2 — openai_codex parity + PDF + default model (RF-046 / RF-048 / RF-064)**:
  - `openai_codex_client.go`: 하드코딩된 `tool_choice="auto"` 제거 → `toOpenAIToolChoice(opts.ToolChoice)` 헬퍼 사용 (Phase 1 와 동일 wire format). Caller 가 nil 을 보내면 `ToolChoiceAuto()` 로 fallback 해 종전 행동 유지.
  - `openai_codex_client.go`: 신규 `toCodexTextFormat` — Responses API 의 `text.format` 봉투 (Chat Completions 의 `response_format` 과는 다름). `json_schema` 변형은 봉투 최상위에 `name/schema/strict` 펼침.
  - **PDF placeholder 명시 에러 (RF-046)**: `openai_compat_client.go` / `openai_codex_client.go` 가 메시지에 `ContentBlocks[*].Type == "document"` 가 있으면 build 단계에서 `pdf_unsupported_by_provider` ProviderError 를 반환. 이전엔 `[Attached PDF document]` placeholder 로 조용히 흘려보냈음 — 모델은 그걸 throwaway 노트로 취급해 사용자 의도 손실.
  - **Default Anthropic 모델 갱신 (RF-064)**: `defaultAnthropicModel` `claude-3-5-haiku-latest` → `claude-haiku-4-5-20251001`. Claude 4.x 가 현재 최신 패밀리. `defaultOpenAIModel` (gpt-4o-mini), `defaultGeminiModel` (gemini-2.5-flash) 는 현재 가용 최신으로 유지.

### Tests

- `TestOpenAICodexClient_ToolChoice_Specific` — 객체 wire format 검증
- `TestOpenAICodexClient_ResponseFormat_JSONSchema` — `text.format` 봉투 + strict 검증
- `TestOpenAICodexClient_PDFUnsupportedError` / `TestOpenAICompatibleChat_PDFUnsupportedError` — PDF 차단 에러 검증

### Out of scope (별도 PR)

- Provider capability 매트릭스 문서화 (RF-047)
- Gemini-native responseSchema / caching (Phase 3+)
- Codex ChatOptions silent ignore 잔여 (RF-049)
- Provider registry (RF-066) / yaml_paths DRY (RF-065)

## [0.31.0] - 2026-04-26

### Changed (BREAKING)

- **ID-004 Phase 1 — ChatOptions strict tool & response controls (RF-042 / RF-048 partial)**: `llm.ChatOptions.ToolChoice` 가 `string` → `*llm.ToolChoice` 구조체로 변경. 새 헬퍼 `llm.ToolChoiceAuto/None/Required/Specific(name)` 로 모든 호출자 마이그레이션. `ChatOptions.ResponseFormat *llm.ResponseFormat` 신규 필드 (text / json_object / json_schema, OpenAI-style strict 토글).
- **OpenAI compat client**: `tool_choice` 가 mode 별로 정확한 wire format 으로 직렬화. specific tool 은 `{"type":"function","function":{"name":"…"}}` 객체. `response_format` 은 `{"type":"json_schema","json_schema":{"name","schema","strict"}}` 봉투. 이전엔 string 만 그대로 forwarding 했고 specific tool 은 사실상 미지원이었음.
- **Anthropic / Gemini-native**: 새 `*ToolChoice` 받게 변환 함수 시그니처 업데이트. anthropic 은 specific tool 이미 지원 (`tool_choice: {type: tool, name: …}`). gemini-native 는 specific 일 때 `functionCallingConfig.mode=ANY + allowedFunctionNames=[name]` 으로 매핑 (Google Live API 표준).
- **subagents_plan (RF-042 직접 해소)**: planner Chat 호출에 `ResponseFormat: json_schema` 적용. OpenAI-호환 planner 에서는 markdown 펜스/문장 잡음 없이 schema-검증된 JSON 출력. 기존 정규화 레이어 (id 유일성, 참조 재작성) 는 그대로 유지.
- `agent.RunOptions` 에 `ResponseFormat *llm.ResponseFormat` 신규 필드 (loop 전 iteration 으로 전달).

### Migration

`ChatOptions.ToolChoice = "required"` 같은 string 호출 코드는 더 이상 컴파일되지 않음. `llm.ToolChoiceRequired()` / `llm.ToolChoiceAuto()` / `llm.ToolChoiceNone()` / `llm.ToolChoiceSpecific("tool_name")` 헬퍼로 교체. ResponseFormat 사용 시: `&llm.ResponseFormat{Type: llm.ResponseFormatJSONSchema, Name: "…", Schema: rawJSON, Strict: true}`.

### Out of scope (Phase 2 — 별도 PR)

- `openai_codex_client.go` 의 `tool_choice="auto"` 하드코딩 제거 (RF-048 잔여)
- PDF placeholder 명시 에러 (RF-046)
- Default model 갱신 (RF-064)

## [0.30.2] - 2026-04-26

### Reverted

- **PR #377 (ID-003 B web aggregator)** 전체 revert. `web_search` 와 `web_fetch` 가 다시 분리 툴로 복귀. `web` 단일 aggregator + `web_search`/`web_fetch` alias + tool_groups 변경 모두 되돌림.

### Why

`web_search` 와 `web_fetch` 는 LLM workflow 상 성격이 다른 작업 (snippet 탐색 vs URL 본문 가져오기) 이고, 더 큰 정책상 *위험도가 다른 빌트인 툴은 단일 aggregator 로 묶지 않는다* 는 결정 (file aggregator 검토 중 발견된 권한 모델 한계 — `read_file → file` alias 가 `ToolsEnabled` allowlist 정밀도를 깨뜨림 + high-risk 분류 불가능). 같은 사유로 ID-003 issue 자체 폐기.

### Migration

기존 `web` 호출 LLM 코드/스킬은 다시 `web_search` / `web_fetch` 분리 호출로 돌아가야 함. (PR #377 머지 직후 한 세션 분량이라 외부 영향 거의 없음.)

## [0.30.0] - 2026-04-26

### Removed (BREAKING)

- **ID-001**: Knowledge Base (KB Wiki) 시스템 전체 제거. chat path 통합 0% 정량 증거 + KB note semantic 인덱스 등록 0% + read 통합 ~0% → 사용자 결정 *"완전 제거. 필요할 때 다시 구현하겠음"*.
  - `internal/memory/knowledge.go` (840 줄) + `internal/memory/knowledge_test.go` (137 줄) 삭제. `KnowledgeStore`, `KnowledgeNote`, `KnowledgeUpdate`, `KnowledgeListOptions`, `KnowledgeNotePatch`, `KnowledgeGraph`, `KnowledgeGraphNode`, `KnowledgeGraphEdge`, `KnowledgeLink` 모두 삭제.
  - `internal/tool/knowledge_aggregator.go` + `internal/tool/memory_kb.go` + `internal/tool/memory_kb_test.go` 삭제. `knowledge` aggregator 빌트인 툴 + `memory_kb_*` 4 alias 모두 제거.
  - `internal/tarsserver/handler_memory.go` 의 `/v1/memory/kb/graph` + `/v1/memory/kb/notes` (POST/GET) + `/v1/memory/kb/notes/{slug}` (GET/PATCH/DELETE) HTTP 라우트 + `decodeKnowledgePatchRequest` 헬퍼 모두 제거. tests 도 같이 정리.
  - `internal/reflection/job_memory.go` 의 `compileKnowledge` 함수 (nightly LLM KB 컴파일) + `derivation.go` 의 `shouldCompileKnowledge` 게이트 제거. nightly memory 작업은 experience derivation 만 남음.
  - `internal/tool/memory_search.go` 의 `include_knowledge` 파라미터 + `searchKnowledgeNotes` 헬퍼 제거.
  - `internal/memory/Backend` 인터페이스에서 6 KB 메서드 (`ListKnowledgeNotes`/`GetKnowledgeNote`/`ApplyKnowledgePatch`/`ApplyKnowledgeUpdate`/`DeleteKnowledgeNote`/`KnowledgeGraph`) 제거.
  - `internal/tool/tool_groups.go` 의 `knowledge` → `memory` group 매핑 제거.
  - **Frontend**: `MemoryCenter.svelte` 의 Knowledge 탭 + 관련 state/handlers 제거 (909 → 551 줄, **-358 줄**). `lib/api.ts` 의 6 KB 함수 + `KnowledgeGraph`/`KnowledgeNote` import 제거. `lib/types.ts` 의 KB 타입 5종 제거. `lib/router.ts` 의 `/console/knowledge` alias 제거.
  - `tarsserver/main_options.go` chat 시스템 프롬프트의 `include_knowledge`/`knowledge(action=...)` 가이드 제거.

### Migration

KB 가 필요해지면 미래 PR 에서 재구현. 현재 사용자 워크스페이스의 `workspace/memory/wiki/notes/*.md` 파일은 read-only 자료로 남아있음 (TARS 가 더 이상 read/write 안 함). 사용자가 직접 보존하거나 삭제 가능.

### Net diff

총 ~2.2k 줄 감소 (Go ~1.5k + Svelte 358 + TS ~50 + tests 정리).

## [0.29.2] - 2026-04-26

### Changed

- **ID-002 (a)**: 시스템 프롬프트의 정체성 헤더 (`You are TARS, a personal AI assistant.`) 가 코드에서 IDENTITY.md 의 default content 로 이동. 사용자가 워크스페이스의 IDENTITY.md 를 편집해 어시스턴트 정체성을 자유롭게 재정의 가능. `Current time` 동적 line 과 `## Response Formatting` 가이드라인은 그대로 builder.go 에 유지 (runtime 제약 + 출력 품질 일관성 보장).

## [0.29.1] - 2026-04-26

### Changed

- **RF-053**: `gateway.Runtime.finalizeRunLocked` (200줄) 분할 — `applyFailedFinalState`(실패 상태 + run_failed 이벤트), `applyCompletedFinalState`(성공 상태 + run_finished 이벤트), `commitRunFinalization`(공통 tail: history trim + state version bump + run summary append + 단일 publish) 3 함수로 분리. 동작 동일, 향후 동시성 invariant 변경이 한 곳에 집중되도록 정리.

### Removed

- **RF-055**: `gateway.Runtime` 의 dead/demo 노드 시스템 전체 제거. `Runtime.Nodes()` / `NodeDescribe` / `NodeInvoke` 메서드 + `defaultNodes()` (`echo`/`clock.now`/`sessions.latest` 데모 노드) + `internal/gateway/runtime_nodes.go` 파일 + `gateway.NodeInfo` 타입 + `GatewayStatus.Nodes` 필드 모두 삭제.
- **RF-055**: `tool.NewNodesTool` 빌트인 툴 + `cfg.ToolsNodesEnabled` config 필드 + 관련 schema/input/yaml/test 항목 삭제. 데모 외 사용 사례 0건이었음.

## [0.29.0] - 2026-04-26

### Removed (BREAKING)

- **RF-007**: 빌트인 Go 플러그인 시스템 자체 제거. `plugin.BuiltinPlugin` 인터페이스 + `RegisterBuiltin` / `BuiltinPlugins` + `extensions.Manager.initBuiltinPlugins` + `tools_provider: builtin:<id>` 분기 모두 삭제.
- **RF-007**: `internal/browserplugin` (브라우저 자동화 빌트인 플러그인 — 유일 사용자) 디렉토리 전체 삭제. 의존했던 `internal/browser` (Chrome/CDP/Playwright runtime), `internal/vaultclient` (HashiCorp Vault SDK 클라이언트), `internal/approval` (OTP manager) 패키지도 함께 삭제.
- **RF-007**: HTTP API `/v1/browser/*` (status/profiles/login/check/run) 6 엔드포인트 + `/v1/vault/status` 제거.
- **RF-007**: tarsclient 의 `/browser` / `/vault` REPL 명령 + `pkg/tarsclient` 의 `BrowserState` / `BrowserProfile` / `BrowserLoginResult` / `BrowserCheckResult` / `BrowserRunResult` / `VaultStatusInfo` 타입 + `BrowserStatus` / `BrowserProfiles` / `BrowserLogin` / `BrowserCheck` / `BrowserRun` / `VaultStatus` 클라이언트 메서드 모두 삭제.
- **RF-007**: config 의 `VaultConfig` + `BrowserConfig` 임베디드 그룹 전체 + `ToolsBrowserEnabled` 필드 제거 (총 20+ 필드, env var 매핑, schema 메타, defaults, yaml path 매핑까지). 사용자 워크스페이스의 `tars.config.yaml` 에 `vault:` 또는 `browser:` 블록이 있으면 silently 무시됨.
- **RF-009**: 외부 플러그인의 HTTP 라우트 등록 경로 폐쇄. `extensions.Manager.CollectHTTPHandlers` 함수 + `plugin.HTTPHandlerEntry` 타입 + 매니페스트 `http_routes` 처리 모두 제거. 외부 플러그인은 더 이상 HTTP 라우트를 노출할 수 없음 — RF-009 옵션 (a) 적용. 라우트가 필요한 도메인 기능은 sidecar 프로세스 + 자체 포트로 운영해야 함.

### Migration

브라우저 자동화 / vault auto-login 기능을 사용하던 사용자는 외부 도구로 마이그레이션 필요:
- **브라우저 자동화**: Chrome DevTools MCP server, Playwright MCP server, 또는 사용자 정의 skill+CLI (CLAUDE.md 의 *"Default pattern: skill (.md) + companion CLI"*).
- **OTP / vault**: 별도 secrets manager + skill+CLI 호출. TARS 코어는 더 이상 vault SDK 통합을 제공하지 않음.

## [0.28.5] - 2026-04-26

### Security

- **RF-008**: 플러그인 lifecycle 훅의 임의 shell 명령 실행 (`sh -c`) 완전 제거. 이전엔 플러그인 manifest 의 `lifecycle.on_start: "echo ..."` 같은 문자열이 그대로 셸로 실행되어 외부 install 플러그인이 TARS 프로세스 환경 (vault 토큰, `~/.aws`, `~/.kube` 등) 에 임의 접근 가능했음.
  - `Lifecycle.OnStart` / `OnStop` 타입을 `string` → `*LifecycleHook { Tool string, Args json.RawMessage }` 로 변경. 빌트인 툴 호출 디스크립터 형식만 허용.
  - `LifecycleDeniedTools` deny-list (`bash` / `exec` / `shell_exec` / `process`) — manifest 파싱 시 + 훅 실행 시 두 번 검증 (defense-in-depth).
  - 기존 string 형식 manifest 는 명확한 마이그레이션 메시지와 함께 거부 (`"plugin manifest uses removed string form lifecycle.on_start; replace with object {tool, args}"`).
  - `runLifecycleHooks` 가 `LifecycleToolResolver` 를 통해 빌트인 툴 호출. resolver 가 nil 이면 declared 훅마다 "no tool resolver available" diagnostic 1줄 (현재 wiring 미완 — 향후 PR 에서 user-surface tool registry 연결).
- **RF-008 보강**: `extensions.Manager.Reload` 가 `plugins_allow_mcp_servers=true` 활성화 + plugin-declared MCP server 가 있을 때 startup WARN 로그. 플러그인 manifest 의 `mcp_servers` 필드도 외부 프로세스 실행 = sh-c 와 같은 카테고리 위험 표면이므로, 활성화 시 운영자에게 *"verify each plugin source is trusted"* 강조.

### Changed

- `extensions.Options` 에 `LifecycleToolResolver` 필드 추가 (현재 caller 는 nil 전달; 빌트인 툴 호출 wiring 은 후속 PR).
- `internal/plugin/manifest.go` validation 강화 — `rejectLegacyShellLifecycle` + `validateLifecycleHook` 로 양 단계 검증.

### Tests

- `internal/extensions`: lifecycle 훅 6 케이스 (resolved tool 호출 / deny-listed / unknown / tool error / nil resolver / no hooks)
- `internal/plugin`: 2 신규 케이스 (legacy string form 거부 / deny-listed tool 거부)

## [0.28.4] - 2026-04-26

### Added

- `internal/atomicwrite` 패키지 — TARS state file 의 표준 crash-safe write 헬퍼. tmp 파일 생성 → write → fsync → close → rename. 부모 디렉토리 자동 생성. unit test 5개 (new file / parent dir / overwrite / no temp leftover / read-only dir failure preserves original) (RF-059/068)

### Changed

- `cron.Store.save` (jobs.json 저장) 와 `cron.Store.pruneRunFile` (run history 트리밍) 가 `os.WriteFile` 대신 `atomicwrite.Write` 사용 — partial write 가능성 제거 (RF-068)
- `session.Store.saveIndex` (sessions.json) 와 `session.Store.SaveTasks` (tasks per session) 가 `atomicwrite.Write` 사용 (RF-059)
- `memory.KnowledgeStore` 의 노트 마크다운 / `index.md` / `graph.json` 3 write 경로가 `atomicwrite.Write` 사용
- `memory.saveEntries` (semantic 인덱스 entries.jsonl) 가 weak local `writeAtomicFile` (tmp + rename, no fsync) 대신 `atomicwrite.Write` 사용 — fsync 추가
- `gateway.writeJSONAtomic` (runs.json + channels.json) 가 `atomicwrite.Write` 로 위임 — gateway 내부 중복 구현 제거

이번 PR 의 가치: persistence anti-pattern 카테고리 정리 (5 사례 누적 → 0). 향후 SQLite 마이그레이션 (RF-017/021/058/067) 결정 시 첫 단계에서 단일 helper 만 교체하면 되도록 사전 정리.

## [0.28.3] - 2026-04-26

### Changed

- `pulse.Runtime.Start()` 가 `pulse_active_hours` / `pulse_timezone` 를 startup 에 1회 검증. 잘못된 값이면 ERROR 로그 1줄을 남기고 fail-soft (always-active) 로 진행. 운영자가 부팅 직후 로그만 봐도 잘못된 설정을 발견 가능 (RF-014)
- `pulse.Scanner.scanCron` / `scanDisk` 가 source 호출 (`cron.List` / `ops.Status`) 실패 시 silent 가 아닌 WARN 로그 출력. 동작은 그대로 (해당 tick 의 시그널만 skip) (RF-014)
- `pulse.Scanner.scanStuckRuns` 의 `parseRunTimestamp` 실패 시 WARN 로그 (run_id 포함). 손상된 timestamp 가 있는 run 을 stuck-run 검사에서 제외하는 동작은 그대로 (RF-014)
- `memory.FileBackend.AppendExperience` 가 caller 의 context 를 `IndexExperience` 에 그대로 전달 (이전엔 `context.Background()` 를 강제 사용 → caller cancellation 무시). 또한 indexing 실패 시 WARN 로그 (experience 저장은 성공, 검색 인덱싱만 실패) (RF-014)
- `memory.loadEntries` 가 손상된 JSONL line 을 skip 할 때 WARN 로그 (path/line/error). 누적 skip 수가 있으면 함수 종료 시 요약 로그 — "consider rebuilding" 힌트 (RF-014)

## [0.28.2] - 2026-04-26

### Changed

- `tarsserver`: 부트스트랩 순서 재구성 — config 를 cobra.Execute 이전에 로드, 최종 logger config 를 CLI+config 에서 도출, `setupRuntimeLogger` 를 단 한 번만 호출. config 로드 실패 시 panic. `newRootCmd` 가 pre-loaded `cfg` 를 인자로 받음. `buildRuntimeDeps` 도 cfg 를 인자로 받아 두 번 로드하지 않음. 이전 두 단계 logger 셋업 (CLI-only → config 로드 후 reconfigure) 에서 첫 번째 lumberjack handle 이 누수되던 문제 해소 (RF-002)
- `plugin.Source` 에 `Priority()` 메서드 추가 + `Load` 가 sources 를 priority 로 자동 정렬. 호출자 슬라이스 순서와 무관하게 일관된 머지 결과 (workspace > user > bundled). 동일 priority 내 stable sort 유지 (RF-013)
- `reflection.MemoryJob.compileKnowledge` 시그니처 `bool → (bool, []string)`. router/list/chat/json/apply 5 silent failure 경로가 prefix 가 붙은 에러 문자열로 `JobResult.Details["errors"]` 에 누적 (RF-016)
- `memory.normalizeSemanticTerms` + `prompt.normalizeRelevantTerms` stopwords 에 한국어 조사/대명사/지시어 추가 (`나/내/너/그/이/저`, `는/은/이/가/을/를`, `의/도/와/과`, `이거/저거/그거`, `뭐/뭐였지/뭐지/뭐야`, `선호/취향/좋아/좋아요/좋아함`). KR 쿼리 매칭 점수가 조사로 부풀려지던 문제 해소 (RF-018)
- `tool.dispatchAction` 가 `aliasFns ...func(map[string]json.RawMessage)` variadic 옵션을 받도록 확장. `automation.normalizeAutomationActionInput` 가 본문 복제 대신 `dispatchAction(params, aliasAutomationJobID)` 한 줄 호출로 단순화 (RF-028)
- `gateway.Runtime.ReportsRuns/ChannelsByWorkspace` 에 `GatewayArchiveEnabled` 플래그가 *디스크 archive* + *report endpoint 가시성* 두 의미를 겸한다는 docstring 추가. 분리는 ID-005 config 마이그레이션과 결합 (RF-057)

### Removed

- `tarsserver/main_serve_api.go` 의 dead `_ = telegramDeliveryCounter` 라인. pulse wiring 은 helpers_pulse.go 에서 이미 완료된 상태였음 — 잔재 정리 (RF-006)
- `internal/memory/knowledge.go` 의 `KnowledgeStore.nowFn` 필드 + 초기화. `Upsert` 가 `time.Now().UTC()` 를 직접 호출. 외부 주입 경로가 없는 채 stub 만 있던 의존성 제거 (RF-020)
- `internal/llm/router.go` 의 `Router.Close()` interface 메서드 + `multiTierRouter.Close()` no-op 구현. 호출자 0건 + Client interface 에 Close 가 없어 cleanup 할 게 없는 reserved-for-future stub (RF-044)
- `internal/llm/fallback_client.go` + `fallback_client_test.go` 전체. production wiring 0건의 reserved-for-future 구현. 필요 시점에 다시 추가 (RF-044)

### Follow-ups

이번 PR 도 Tier B 의 mechanical/independent 항목만. 결정 의존 항목은 별도:
- ID-001 ~ ID-005: GitHub issues #363-#367
- 70+ RF 우선순위: `docs/code-review/findings/refactor.md`

## [0.28.1] - 2026-04-25

### Changed

- `memory_search` 의 `include_sessions` 기본값 `false` → `true` (description 의도와 일치, RF-038)
- `Compaction.CompactOptions` 에 `keepRecent` 3-strategy 우선순위 docstring 추가 (RF-060)
- `LLMProviderSettings.ServiceTier` 가 provider-level default 임을 docstring 으로 명시 (tier-level 이 우선) (RF-063)
- `gateway/runtime_run_execute.go` `finalizeRunLocked` 가 동일 event 를 두 번 publish 하던 동작을 한 번으로 통합 (RF-054)
- `KnowledgeStore.Graph()` 자기치유 로직 단순화: 누락/손상/legacy 3 경로 → 단일 rebuild fallback (RF-022)
- `tool.Registry.Register` 가 같은 이름 중복 등록 시 silent overwrite 대신 warn 로그 출력 (RF-026)
- `cron.computeBackoffDuration` 의 magic number 를 documented const (`backoffBaseDuration`/`backoffMaxMultiplier`/`backoffMaxDuration`) 로 추출 (RF-070)

### Removed

- 사용되지 않는 deprecated 플래그 `--run-once` / `--run-loop` (`tars serve`) 와 관련 ServeOptions 필드, mutually-exclusive 검증, deprecation warning 모두 제거. pulse 는 서버 시작 시 자동 실행됨. 외부 자동화 스크립트가 이 플래그를 넘기면 `unknown flag` 에러가 발생하므로 호출부 수정 필요 (RF-001)
- `runtimeDepsError` 의 `daily_log` 좀비 case label — 어디서도 생성되지 않는 dead branch (RF-005)
- `internal/memory/semantic.go` 의 dead code 7 블록 (`indexState` 타입, `loadIndexState`/`saveIndexState`, `readDoc`, `firstMeaningfulParagraph`, 자체 `min`) (RF-019)
- `internal/prompt/builder.go` 자체 `max`/`min` 함수 (Go 1.25 built-in 사용) (RF-019)
- `internal/tool/list_dir.go` 자체 `min`/`minInt` 함수 (Go 1.25 built-in 사용) (RF-019)
- `internal/cron/helpers.go` 자체 `min` 함수 (Go 1.25 built-in 사용) (RF-019/RF-069)
- `internal/prompt/memory_retrieval.go` 의 죽은 fallback matcher (`collectProjectDocumentMatches`, `collectBriefMatches`, `classifySourceTag` 의 `projects/` + `_shared/` 분기) — project 패키지 제거 후 잔재 (RF-023)
- `IsExecToolName` 의 이중 정규화 (CanonicalToolName 한 번 호출로 단순화) (RF-027)
- `exec` tool 의 undocumented `cmd` alias (schema 에 `command` 만 정의됨) (RF-031)
- `provider="codex-cli"` removed-alias error stub 3 줄 (RF-043)
- consensus strategy `vote` schema enum (구현 미완 — 사용 시 runtime 에러였음. enum 을 `["synthesize"]` 만으로 축소) (RF-052)
- `runtimeDeps.sessionStoreResolver` 의 잉여 첫 nil 초기화 (RF-003)

### Follow-ups

이번 PR 은 Tier A (mechanical / silent acceptance / docstring) 만 정리. 사용자 결정이 필요한 큰 작업은 별도 추적:

- ID-001 ~ ID-005 의사결정: GitHub issues #363-#367
- 70+ RF 우선순위 매트릭스: `docs/code-review/findings/refactor.md`

## [0.28.0] - 2026-04-19

### Removed

- `research_report` tool and `internal/research` package — the deep-research workflow is no longer part of the supported surface
- `internal/schedule` package, `/v1/schedules` HTTP routes, and `tars schedule` CLI subcommand — one-shot scheduling is replaced by cron entries (cron already accepts natural-language `@at` expressions)
- `schedule_*` tool aliases (e.g. `schedule_create`, `schedule_cancel`) — use cron tools instead

### Migration

- Users who relied on `tars schedule`/`schedule_*` tools should register a one-shot cron job: `cron_create` accepts `@at <natural language time>` expressions via the existing `scheduleexpr` parser (e.g. `@at "tomorrow 9am"`)
- The `/v1/schedules` endpoints return 404 — update any external client to call `/v1/cron` instead

## [0.27.1] - 2026-04-12

### Fixed

- Console no longer hangs when switching chat sessions rapidly; SSE connections are now shared via a singleton EventSource instead of per-component instances that exhausted the browser's HTTP/1.1 connection limit
- Compaction deadlock resolved: auto-compaction no longer re-acquires the transcript file lock by reusing already-read messages via PreloadedMessages
- `--verbose` flag now correctly overrides `log_level` from workspace config, fixing missing HTTP debug logs
- Manual compact via console now uses aggressive thresholds (keepRecent=5, tokens=2000) so it always performs meaningful work

### Added

- HTTP request start logging: "http request started" debug log emitted before handler execution for visibility even when handlers block
- Compact result feedback: API returns detailed token counts and reason; console shows a feedback banner with message count, percentage saved, or skip reason

## [0.27.0] - 2026-04-12

### Changed

- Session compaction now uses rune-based CJK-aware token estimation instead of byte-length heuristic, improving accuracy for Korean/Chinese/Japanese content
- Deterministic summary restructured from 6 overlapping sections to 4 deduplicated sections (Topic, Key Decisions, Active Identifiers, Current State), reducing summary size by 59%
- LLM summary input upgraded from fixed 80-message x 240-char limit to 8K token budget with proportional content allocation

### Added

- Pre-compaction tool result pruning: long tool outputs (>200 chars) replaced with placeholder to prevent code dump pollution in summaries
- Stacking carry-forward: previous compaction summaries are detected and passed as Prior Context, preserving information across re-compactions
- Exported `EstimateMessageTokenCost` function for external use

## [0.26.1] - 2026-04-12

### Fixed

- Console sidebar now displays the server version dynamically from `/v1/status` instead of a hardcoded string
- Zero-time dates (`0001-01-01T00:00:00Z`) now display as "never" instead of absurd relative times like "739717d ago" across all console components
- Console static assets are now accessible in all auth modes; previously `external-required` mode blocked the SPA from loading
- Legacy config key detection no longer false-positives on the valid `llm_role_defaults` key

## [0.26.0] - 2026-04-12

### Added

- Hierarchical YAML config loading and patching across runtime, automation, gateway, tools, browser, vault, channels, and extensions, including migration-safe reads from existing flat keys
- Structured `/console/config` metadata and editing support for provider pools, tier bindings, nested object settings, and list-based settings such as allowlists and extra directories

### Changed

- Starter config generation, checked-in standalone defaults, and the shipped example config now use the hierarchical schema as the canonical layout
- README and Getting Started examples now describe the current console-first flow and nested config model instead of removed flat-key and project-oriented flows

### Fixed

- Settings patches written from the console now preserve the preferred nested YAML layout instead of reintroducing legacy flat keys into updated config files

## [0.25.0] - 2026-04-12

### Added

- Group-based tool policy controls across session config, workspace gateway agents, and the console tool configuration surface, including structured blocked-tool diagnostics and a manual verification guide for the Hermes improvement bundle
- Gateway provider override metadata, run detail APIs, live run events, consensus execution mode, and a dedicated console run view for inspecting multi-agent executions
- A file-backed memory backend interface that now powers memory APIs and tools behind a common abstraction

### Changed

- Chat compaction now exposes configurable trigger and retention knobs, supports deterministic mode and timeout-bounded LLM fallback, and reports compaction telemetry to the console context monitor
- Subagent orchestration can now carry per-task provider override and consensus settings through the agent runtime runtime and persistence layer

### Fixed

- Session tool group allow/deny rules now remain effective even when custom session tool mode is enabled without an explicit tool allowlist
- Chat context previews now persist and report the last applied compaction mode, and gateway agent list responses now include tier and provider override metadata

## [0.24.1] - 2026-04-05

### Fixed

- Cron jobs created from the chat tool inside a regular console chat session are now correctly bound to that session instead of silently becoming global; empty-kind chat sessions are treated as session-bound contexts, matching the behavior already available to the `kind=session` and `kind=main` paths
- Chat page now auto-refreshes when a background cron job delivers a result to the currently open session, and `[CRON]`/`[REMINDER]` transcript entries are no longer hidden from history so users can see why a scheduled run fired

## [0.24.0] - 2026-04-05

### Added

- Chat right panel now includes a dedicated `Cron` tab so main chats can manage global cron jobs and regular chats can manage only their bound session cron jobs in context
- Reminder cron jobs now deliver deterministically: global reminders post into the main chat session and send Telegram notifications when a target chat is available, while session-bound reminders stay inside their bound chat session

### Fixed

- `cron(action=create)` now accepts reminder-style aliases like `task_type`, `message`, and `title`, and can parse natural schedules such as `in 1 minute`
- Cron creation from chat now respects the current session kind by defaulting main chats to global/main-target jobs and regular chats to session-bound jobs

## [0.23.0] - 2026-04-05

### Added

- Session-bound cron jobs with optional `session_id` binding, so scheduled runs can reuse a chat session's tool and skill policy, work dirs, prompt override, and recent history
- User-visible cron audit logs appended to `artifacts/<session_id>/cronjob-log.jsonl` for bound jobs and `artifacts/_global/cronjob-log.jsonl` for global jobs
- Cron API, CLI, and console surfaces now show cron execution scope and session binding metadata

### Fixed

- Tasks panel no longer crashes when empty or legacy session task payloads omit the `tasks` array

## [0.22.0] - 2026-04-05

### Added

- Session-aware Files panel flows for chat: artifact deep links from messages, typed file previews, and workspace folder creation from both the browser and the directory picker
- Rich file preview modes for markdown render/raw text, syntax-highlighted code, zoomable images, and binary-file notices

### Fixed

- Session artifact tracking now keeps canonical paths, avoids duplicate entries, and opens the correct file reliably from chat history and the Files panel
- Session workdirs now always keep the mandatory `artifacts/{sessionId}` directory first, normalize stored paths, and repair misresolved `workspace/workspace/artifacts/...` file writes
- Workspace file APIs now handle absolute and relative artifact paths consistently, preventing transient or persistent 404s in file preview dialogs

## [0.21.0] - 2026-04-04

### Added

- Tasks panel in chat UI — view session plan, task progress bar, and task cards with status badges
- `GET /v1/admin/sessions/{id}/tasks` API endpoint for fetching session tasks
- Workspace file browser API: `GET /v1/workspace/files?path=` for directory listing and file content preview
- Tasks toggle button in chat pulse bar with live task count

## [0.20.0] - 2026-04-04

### Added

- Session-scoped `tasks` tool with plan + task management (actions: plan_set, plan_get, add, update, remove, list, clear)
- Tasks are stored per-session in `{sessionID}.tasks.json`, archived to memory when replaced
- Tool group utilities (`tool.KnownToolGroups`, `tool.ExpandToolGroups`, `tool.ExpandToolPatterns`) for agent policy resolution

### Removed

- **Breaking:** Removed entire project system (`internal/project/` package, ~30 files)
- Removed project tools (`project`, `project_work`, `project_brief` aggregators)
- Removed project API routes (`/v1/projects`, `/v1/project-briefs/`)
- Removed project CLI commands (`tars project list/get/activity/autopilot`)
- Removed project-related gateway integration (`project_task_runner`)
- Removed `Session.ProjectID` field and `SetProjectID()`, `EnsureWorker()` methods
- Removed worker session type (sessions are now `main` or general)
- Removed project frontend pages (`Projects.svelte`, `ProjectView.svelte`)
- Removed `project-swarm` plugin

### Changed

- Session tasks replace project-based task management with a simpler, session-scoped model
- System prompt rules updated to guide LLM on tasks tool usage
- Gateway agent policy resolution simplified (no longer depends on project package)

## [0.19.0] - 2026-04-04

### Changed

- **Tool consolidation: 71 → 27 built-in tools** using aggregator pattern
  - `memory` aggregator (replaces memory_save, memory_search, memory_get)
  - `knowledge` aggregator (replaces memory_kb_list/get/upsert/delete)
  - `workspace` aggregator (replaces workspace_sysprompt_get/set, agent_sysprompt_get/set)
  - `project` aggregator (replaces project_create/list/get/update/delete/activate)
  - `project_work` aggregator (replaces project_board/activity/dispatch/state tools)
  - `project_brief` aggregator (replaces project_brief_get/update/finalize)
  - `session` aggregator (replaces sessions_list/history/send/spawn/runs, agents_list, session_status)
  - `ops` aggregator (replaces ops_status/cleanup_plan/cleanup_apply)
  - `cron`/`heartbeat` aggregators: individual sub-tools removed from registry
  - Schedule tools absorbed into cron; file I/O aliases (read/write/edit) removed
- System prompt tool routing rules now explicitly guide LLM to use `workspace` for user profile updates
- Tool group expansion updated to recognize aggregator names (`memory`, `knowledge`)
- High-risk tool classification updated for aggregator names

### Removed

- SOUL.md removed from sysprompt specs, bootstrap files, and prompt builder (fully absorbed into IDENTITY.md)
- Individual cron/heartbeat sub-tools removed from tool registry (aggregators remain)
- Schedule tools removed (use cron aggregator instead)
- File I/O short aliases (`read`, `write`, `edit`) removed — use `read_file`, `write_file`, `edit_file`

## [0.18.0] - 2026-04-04

### Added

- Dedicated system prompt built-in tools: `workspace_sysprompt_get`, `workspace_sysprompt_set`, `agent_sysprompt_get`, `agent_sysprompt_set`
- Explicit system prompt management API endpoints: `/v1/workspace/sysprompt/files` and `/v1/workspace/sysprompt/file`
- Dedicated System Prompt console page at `/console/sysprompt` for managing `USER.md`, `IDENTITY.md`, `AGENTS.md`, and `TOOLS.md`

### Changed

- Workspace bootstrap metadata now treats `USER.md` as user identity, `IDENTITY.md` as TARS persona, `AGENTS.md` as agent operating rules, and `TOOLS.md` as tool guidance
- Prompt-source files can now be managed through domain-specific sysprompt surfaces instead of relying only on generic file tools

## [0.17.0] - 2026-04-04

### Added

- Memory management API endpoints for durable memory assets and search testing: `/v1/memory/assets`, `/v1/memory/file`, `/v1/memory/search`
- Dedicated Memory console page at `/console/memory` for inspecting and editing `MEMORY.md`, `memory/experiences.jsonl`, daily durable memory files, semantic index artifacts, and the knowledge base in one place
- In-console memory search test harness with toggles for `MEMORY.md`, daily logs, session history, and opt-in knowledge-base lookup

### Changed

- `memory_save` now writes durable memory to both `memory/experiences.jsonl` and `MEMORY.md`
- `memory_search` now searches `experiences.jsonl` with term-based lexical scoring, improving recall for cross-session memory checks without semantic embeddings
- Knowledge-base lookup is no longer part of default `memory_search`; callers must explicitly opt in with `include_knowledge=true`
- Automatic KB compilation is now gated to durable-signal turns instead of every chat turn

### Fixed

- Korean remember requests such as `... 기억해줘` now trigger durable memory promotion
- Cross-session recall no longer depends on KB note creation when only structured durable memory was saved

## [0.16.1] - 2026-04-04

### Fixed

- Empty knowledge bases no longer break `/v1/memory/kb/graph` with a 500 when `graph.json` has a blank `updated_at`
- Existing legacy `memory/wiki/graph.json` artifacts with blank timestamps are now tolerated and automatically repaired on read

## [0.16.0] - 2026-04-04

### Added

- Obsidian-style knowledge base layer under `memory/wiki/`: durable markdown notes, `index.md`, and `graph.json`
- Automatic post-chat knowledge compilation: the LLM can turn each completed chat turn into durable wiki notes and graph links
- Built-in KB CRUD tools: `memory_kb_list`, `memory_kb_get`, `memory_kb_upsert`, `memory_kb_delete`
- Knowledge Base API endpoints: `/v1/memory/kb/notes`, `/v1/memory/kb/notes/{slug}`, `/v1/memory/kb/graph`
- Dedicated console Knowledge page for browsing, editing, creating, and deleting wiki notes plus reviewing graph relations

### Changed

- `memory_search` now searches knowledge-base notes alongside `MEMORY.md`, daily logs, semantic recall, and optional session transcripts
- Workspace init/doctor now provision and validate `memory/raw` plus `memory/wiki/{notes,index.md,graph.json}`

## [0.15.2] - 2026-04-04

### Changed

- Default workspace path changed from `./workspace` to `~/.tars/workspace`
- Config path is now fixed at `~/.tars/config/config.yaml` (not user-overridable)
- `tars service install/start` no longer requires `--workspace-dir` or `--config` flags
- `ResolveConfigPath` fallback chain now includes `~/.tars/config/config.yaml`

### Added

- `tars init move --to <dir>` subcommand to relocate workspace directory (updates config and advises service restart)
- Auto-migration of legacy configs (`./workspace/config/tars.config.yaml`) on `tars init`
- `config.TarsHomeDir()`, `config.FixedConfigPath()`, `config.DefaultWorkspaceDir()` helpers

## [0.15.1] - 2026-04-04

### Added

- Project onboarding flow with planning mode: new projects without `project.md` enter planning phase where AI guides project planning via conversation
- Phase-aware system prompt: planning phase injects structured prompts for collaborative project definition
- Auto-transition from planning to executing phase when `project.md` is created
- Frontend: phase badge display (planning/executing), auto-send onboarding message on project creation

## [0.15.0] - 2026-04-04

### Added

- Proactive memory search: LLM now MUST call memory_search before answering questions about prior conversations, decisions, preferences, or facts
- Session transcript search via `include_sessions` parameter in memory_search tool for conversational continuity
- In-process memory cache with TTL (5 min) — cache-first strategy skips semantic search on cache hit
- Async memory prefetch goroutine for next-turn cache warming (fire-and-forget)
- `memory_recall` SSE event type for frontend memory notification
- 20+ conversational continuity detection patterns (EN/KR): "그거", "지난번", "you mentioned", "last time", etc.
- Deep session content search fallback in relevant memory collection
- Source type tags in Prior Context section: `conversation`, `experience`, `project`, `daily`

### Changed

- Renamed "Relevant Memory" section to "Prior Context" with source-type-tagged format

## [0.14.3] - 2026-03-29

### Added

- Extension detail view: click skill name to expand full SKILL.md content with markdown rendering
- Works for both installed skills and hub skills with full usage/help documentation
- Detail panel shows metadata (source, invocable status) and scrollable content

## [0.14.2] - 2026-03-29

### Fixed

- Extensions Hub tab no longer crashes when registry response has missing or null `plugins`/`skills`/`mcp_servers` arrays

## [0.14.1] - 2026-03-29

### Fixed

- Workspace reset now fully reinitializes to `tars init` state: removes all runtime artifacts (sessions, projects, cron, gateway, skills, plugins, mcp-servers, skillhub.json, ops, memory data) while preserving config/ and .md template files, then re-runs EnsureWorkspace to recreate the pristine directory structure

## [0.14.0] - 2026-03-29

### Added

- **Config Management** — structured Settings UI with field-level editing, select dropdowns for enumerable options, YAML raw editor toggle, server restart (launchd/exec auto-detection), workspace reset, and Danger Zone actions
- **Console CRUD** — project create/edit/delete with physical removal, cron job create/edit/delete/manual-run, session chat with ChatPanel embedding
- **Multimodal Chat** — file upload (image/PDF/text) with base64 encoding, clipboard paste (Ctrl+V), ContentBlock support across all LLM providers (Anthropic, OpenAI Codex, OpenAI Compat, Gemini)
- **Notification Panel** — clickable header badge with dropdown, newest-first sort, All/Unread/Read filter tabs, mark-all-read via events API
- **Projects Page** — dedicated project list separated from Home dashboard, with search, status filter (All/Active/Archived), table view, and Ask AI button for natural language editing
- **Extensions Management** — new Extensions page with Hub tab (browse/install/uninstall from tars-skills registry) and Installed tab with ON/OFF toggle per skill/plugin/MCP server, persistent disable state via `extensions_disabled.json`
- **Skillhub API** — `/v1/hub/registry`, `/v1/hub/installed`, `/v1/hub/install`, `/v1/hub/uninstall`, `/v1/hub/update` endpoints wrapping existing `skillhub.Installer`
- **Ask AI** buttons on Projects and Ops pages that navigate to Home chat with context-prefilled prompts

### Fixed

- Cleanup approval now auto-applies on approve (no separate Apply step), with result stored in Approval.Note and displayed in Ops UI
- Blocked MCP servers no longer cause the entire `ListServers` API to return 500; blocked servers are included with error field set while others return normally
- Project DELETE now physically removes the directory instead of soft-archiving
- `requestJSON` handles 204 No Content responses without JSON parse errors
- `openai-codex` added to LLM provider select options in Settings UI

### Changed

- Home page redesigned with Chat as the primary feature (moved to top), summary widgets below
- Notification section removed from Home (replaced by header notification panel)

## [0.13.5] - 2026-03-28

### Fixed

- Source checkouts now serve an explicit `/console` placeholder page with build and dev-proxy instructions instead of a blank-looking shell when the Svelte console assets have not been built yet
- `tars serve` now logs a startup warning when it falls back to placeholder console assets, and the developer workflow documents the `make console-install` / `make console-build` steps for local source runs

## [0.13.4] - 2026-03-28

### Fixed

- The `ops-service-demo` Docker Compose template no longer pins a global `ops-service-demo` container name, so repeated seed repos do not collide on stale container names during local reruns
- The ops-service example tests now lock in the absence of a fixed container name, and the walkthrough clarifies that Compose names are project-scoped while the host port remains shared

## [0.13.3] - 2026-03-27

### Fixed

- The ops-service example now treats the bootstrapped repository as a seed repo only and moves all runtime `docker compose` and `opsctl` steps to the authoritative project clone under `projects/<project-id>/repo`
- The bootstrap helper output now explains the seed-repo role directly instead of suggesting runtime service commands before the TARS project clone exists

## [0.13.2] - 2026-03-27

### Fixed

- Project-linked cron jobs now inherit the owning project's tool allowlist during background agent runs, so approved shell/file tools are available to workflows such as the ops-service triage example
- The ops-service example walkthrough now switches the running demo service into the project's cloned repo and filters immediate cron runs by `project_id`, avoiding duplicate-job selection and repo-path mismatches

## [0.13.1] - 2026-03-27

### Fixed

- The `ops-service` example template no longer requires a nested Go module inside the TARS repository, so `go test ./examples/ops-service-demo/...` now works from the repo root
- The demo repo bootstrap script now writes a standalone `go.mod`, preserving independent `go test ./...` execution after the template is copied into its own repository

## [0.13.0] - 2026-03-27

### Added

- Bundled `ops-service` plugin with operational planning, log triage, issue creation, remediation, PR, and reporting skills
- `examples/ops-service-demo/` with a bootstrap script, standalone demo repo template, `opsctl` operational CLI, Docker Compose service, and example project/cron payloads

### Changed

- Workspace bootstrap and repair flows now restore the bundled `ops-service` plugin alongside the existing bundled project workflow plugin
- README documentation now includes the new end-to-end ops-service example walkthrough

## [0.12.1] - 2026-03-27

### Added

- Project autopilot status responses now include phase, phase status, summary, and next action metadata for CLI/API clients
- Typed chat events now expose `skill_name` and `skill_reason` when auto skill routing is announced

### Changed

- Planning blockers now age into an explicit timeout/escalation path instead of staying in an unbounded blocked-planning state forever
- Expired terminal `AUTOPILOT.json` snapshots are pruned during status/restore so stale runtime state does not linger indefinitely
- Telegram chat replies now surface auto-selected skill notices for active brief and explicit skill routing
- CI and release workflows now opt into the Node 24 GitHub Actions runtime and use the current checkout/setup action majors to avoid deprecation warnings

## [0.12.0] - 2026-03-27

### Added

- Typed `PhaseEngine` project runtime with a step-wise `advance` flow exposed through chat tools, REST, and TUI project commands
- Project workflow metadata fields `workflow_profile` and `workflow_rules` for per-project worker and verification policy overrides
- Chat status events that surface automatic skill routing decisions before execution starts

### Changed

- Project autopilot now follows a planning-first, phase-centric workflow instead of immediately seeding and cycling a Kanban board from an empty brief
- Empty backlog states now fall back to planning or approval instead of auto-seeding bootstrap tasks
- Dashboard project views now prioritize phase status, run status, pending human decisions, and blockers over raw board columns
- Built-in project-start and project-autopilot skills now align with the phase engine, approval gates, and one-step runtime control
- Non-software workflow profiles can disable software-specific worker defaults and GitHub/test/build gates without changing core code

## [0.11.0] - 2026-03-22

### Added

- Plugin manifest v2 metadata: `schema_version`, `requires`, `supported_os`, `supported_arch`, `default_project_profile`, and `policies`
- Remote MCP transports for `streamable_http`, legacy `sse`, and `websocket`, alongside existing `stdio`
- MCP server auth settings for bearer-token env injection and OAuth-backed bearer headers on remote transports

### Changed

- Plugin loading now applies runtime availability gating, so unavailable plugins no longer contribute skills or MCP servers
- MCP server status APIs now expose transport, source, URL, and auth mode metadata in addition to connectivity state
- Bundled `project-swarm` plugin manifest now declares schema version 2 and its default project profile

## [0.10.3] - 2026-03-22

### Added

- Skill runtime gating for `SKILL.md` frontmatter: `requires_plugin`, `requires_bins`, `requires_env`, `os`, and `arch`

### Changed

- Unavailable skills are now excluded from the runtime snapshot and prompt, with extension diagnostics explaining missing plugins, binaries, environment variables, or platform mismatches
- Plugin source priority documentation now matches runtime behavior: `workspace > user > bundled`

## [0.10.2] - 2026-03-22

### Added

- Manual `/compact [instructions]` now works from the single-main-session TUI/runtime path and forwards custom focus guidance to compaction

### Changed

- Session compaction now writes structured deterministic summaries with preserved identifiers, current-goal/open-state sections, and explicit requested-focus capture
- Auto and default manual compaction now preserve a safer recent tail using a 30% token-share policy with the existing 12K-token floor instead of relying only on a fixed recent-count fallback

## [0.10.1] - 2026-03-22

### Changed

- Built-in `read_file` now uses 2,000-line pagination with continuation guidance, 20MB file-size guards, and long-line shortening instead of raw byte-only truncation
- Built-in `write_file` now resolves create targets against the real workspace path and writes through an atomic temp-file rename to avoid symlink escapes and partial writes

## [0.10.0] - 2026-03-22

### Added

- `subagents_run` chat tool for parallel read-only delegation to gateway-backed explorer subagents
- Built-in `explorer` gateway agent with a read-only allowlist for codebase and project research tasks
- Gateway run metadata for subagent lineage and hidden subagent sessions
- Config knobs `agentruntime_subagents_max_threads` and `agentruntime_subagents_max_depth`

### Changed

- Hidden subagent runs now append compact system summaries back to the parent chat session instead of leaking raw child transcripts into the main conversation context

## [0.9.0] - 2026-03-22

### Added

- Trusted MCP Hub CLI: `tars mcp {search,install,uninstall,list,update,info}` for discovering and managing vetted MCP packages from `devlikebear/tars-skills`
- Registry v3 format with `mcp_servers` section and checksum-verified package files
- Hub-managed MCP runtime source that loads installed MCP manifests alongside base config and plugin-provided MCP servers

### Changed

- Extension reload diagnostics now report MCP source overrides and malformed installed MCP manifests
- Public docs now distinguish plugin-embedded MCP servers from hub-managed MCP packages and document the `mcp_command_allowlist_json` requirement

## [0.8.0] - 2026-03-21

### Changed

- Gemini native provider rewritten to raw HTTP, removing `google.golang.org/genai` SDK and all transitive dependencies (cloud.google.com, grpc, protobuf)
- Reduced binary dependency footprint and build time

### Added

- Plugin interface documentation (`docs/plugins.md`) covering manifest schema, skill directories, MCP servers, plugin sources, and the `project-swarm` reference implementation

## [0.7.1] - 2026-03-21

### Added

- TARS Plugin Hub CLI: `tars plugin {search,install,uninstall,list,update,info}` for managing plugins from the public registry
- Registry v2 format with `plugins` section in `devlikebear/tars-skills`
- Skill install now warns when a `requires_plugin` dependency is missing and suggests the install command
- CI coverage reporting with Codecov upload

### Changed

- README rewritten: repositioned as "local-first AI project autopilot" with badges, three-tier feature structure, and concise quick start
- GitHub repository description and topics updated
- `web/relay-extension/` extracted to standalone `devlikebear/tars-relay-extension` repository
- CI now runs `make test-cover` instead of `make test`

## [0.7.0] - 2026-03-21

### Added

- TARS Skill Hub CLI: `tars skill {search,install,uninstall,list,update,info}` for discovering and installing skills from the public `devlikebear/tars-skills` registry
- Companion file support for skills: scripts (`.sh`, `.py`, `.ts`), templates, and other reference files are installed alongside `SKILL.md` and mirrored to runtime
- `internal/skillhub` package with registry fetch, search, install, list, and update operations
- Skill registry `files` field for declaring companion files in `registry.json`

### Changed

- Skill runtime mirror now copies all companion files from the source skill directory, preserving subdirectory structure and executable permissions

## [0.6.3] - 2026-03-21

### Fixed

- MCP server failures no longer block server startup; continues without MCP tools

## [0.6.2] - 2026-03-21

### Fixed

- Startup LLM traffic storm: `RestorePersistedRuns` no longer auto-starts all project autopilot loops on startup; runs resume on next heartbeat instead
- Session 404 error: translate public session ID `"main"` to internal hash ID in chat handler
- Stale `AUTOPILOT.json` status correction: persisted `running` status with blocked/failed message is fixed on restore
- macOS build warning: suppress `-lobjc` duplicate library linker warning

### Added

- Log rotation config: `log_level`, `log_file`, `log_rotate_max_size_mb`, `log_rotate_max_days`, `log_rotate_max_backups` with lumberjack
- Logger configuration printed as INFO on server startup
- Config `log_file` takes precedence over CLI default; parent directory auto-created
- `make build` outputs binary to `bin/` directory

## [0.6.1] - 2026-03-20

### Changed

- Homebrew release automation now updates the unified `devlikebear/homebrew-tap` repository instead of the dedicated `homebrew-tars` tap
- Public install instructions now use `brew tap devlikebear/tap` and `brew install devlikebear/tap/tars`

## [0.6.0] - 2026-03-20

### Added

- Semantic Memory V2 with local derived indexing under `workspace/memory/index` for durable memories and project documents
- Gemini embedding configuration for semantic retrieval with `memory_semantic_enabled`, `memory_embed_*`, and default `gemini-embedding-2-preview` support

### Changed

- Prompt assembly now prefers semantic memory recall for paraphrases and project-scoped context, with lexical retrieval kept as the fallback path
- `memory_save` now dual-writes to both `experiences.jsonl` and the semantic memory index when semantic memory is enabled
- Session compaction now stores compaction summaries and extracted durable memory candidates in the semantic index without breaking compaction when extraction fails
- `memory_search` now uses semantic recall first and falls back to the existing file-based substring search when embeddings are unavailable

## [0.5.11] - 2026-03-14

### Fixed

- Project autopilot now stays alive in a periodic supervisor loop until the board reaches `done` instead of stopping after one bounded burst of dispatches
- Server startup now recreates autopilot loops for incomplete projects so active work resumes automatically after a TARS restart
- Heartbeat-triggered supervision now force-starts missing autopilot loops for incomplete projects as a safety net when a project is active but no live PM loop is attached

### Changed

- PM supervision now auto-requeues stalled `in_progress` work back to `todo`, records an automatic retry decision/replan, and keeps moving without asking the user for routine retry decisions

## [0.5.10] - 2026-03-14

### Added

- `/dashboards` now renders a workspace-wide project index that links to every project dashboard and summarizes status, phase, next action, and autopilot state

### Changed

- Project dashboard auth can now be disabled independently from API auth with `dashboard_auth_mode: off`, so trusted local browser monitoring can stay open while `/v1/*` routes remain protected

## [0.5.9] - 2026-03-14

### Fixed

- Natural-language project kickoff without an explicit `session_id` now starts in a fresh chat session instead of inheriting the current main session context
- Project board normalization now canonicalizes common Kanban aliases such as `backlog` and `doing` to the runtime statuses `todo` and `in_progress`, so dispatch, activity, and dashboard views stay aligned

### Changed

- The bundled `project-start` skill now explicitly seeds boards with the canonical status set `todo`, `in_progress`, `review`, `done`

## [0.5.7] - 2026-03-14

### Fixed

- Project worker runs now create a distinct hidden session per project run instead of reusing one shared hidden session across subagent work
- PM seed backlog dispatch now stages `pm-seed-bootstrap` ahead of dependent seed tasks so autopilot does not start the first vertical slice before bootstrap is underway
- Chat requests with an explicit stale `session_id` now create a fresh chat session instead of silently attaching to the current main session
- Project autopilot run status now persists to `AUTOPILOT.json`, survives server restart, and no longer disappears from `/v1/projects/{id}/autopilot` after the process restarts
- Persisted `running` autopilot runs now recover as `blocked` with restart guidance and an interrupted PM blocker entry instead of reporting a false in-progress state after restart

### Changed

- API startup now preloads persisted autopilot runs so project state, activity, and dashboard views are already synchronized before the first autopilot status request
- Autopilot persistence now uses atomic file replacement for `AUTOPILOT.json` writes

## [0.5.6] - 2026-03-14

### Fixed

- Project autopilot now preserves the logical worker kind even when task dispatch falls back to the runtime default gateway agent
- Failed worker runs now restore the task to `todo`, record the real worker error, and stop autopilot on the actual blocker instead of corrupting the next dispatch with an executor alias
- Empty project boards now block autopilot for backlog seeding instead of incorrectly marking the project complete
- `tars doctor` now fails fast when `gateway_default_agent` points to an enabled gateway executor with a missing local command or script path
- The flaky browser relay broadcast test now waits for both CDP clients to be fully registered before asserting fan-out delivery

### Changed

- The project dashboard now shows autopilot run status and dedicated worker report entries extracted from structured task reports
- The project dashboard now also shows PM blocker, decision, and replan notes from the supervisor loop
- Project autopilot now behaves more like a PM supervisor by seeding a minimal MVP backlog when a project starts with an empty board
- Bundled `project-start` and `project-autopilot` skill instructions now align with the runtime by defaulting low-risk kickoff decisions and by treating an empty board as blocked work rather than completed work

## [0.5.5] - 2026-03-14

### Added

- `llm_provider: claude-code-cli` to run chat requests through a locally installed Claude Code CLI without API keys

### Changed

- `tars doctor`, starter config comments, and public docs now explain the local Claude Code CLI provider path alongside API-backed providers

## [0.5.4] - 2026-03-14

### Fixed

- Terminal chat now recovers automatically when a stale local `session_id` causes `/v1/chat` to return `404 not_found: session not found`
- TUI and one-shot CLI chat retry once against the current main session, or fall back to creating a fresh session when no main session exists

## [0.5.3] - 2026-03-14

### Changed

- Project task dispatch now falls back to the runtime default gateway agent when a requested worker alias such as `codex-cli` is not explicitly registered
- Starter project autopilot can advance past gateway agent-name mismatches instead of failing immediately on `unknown agent`

## [0.5.2] - 2026-03-14

### Added

- `tars doctor` now warns when `gateway_enabled=false` would disable the bundled project workflow and autopilot

### Changed

- Starter workspaces created by `tars init` now enable the gateway path required by bundled project workflows out of the box

## [0.5.1] - 2026-03-14

### Added

- TUI project workflow commands for board inspection, activity inspection, task dispatch, and autopilot start/status
- `GET` and `POST /v1/projects/{id}/autopilot` so non-chat clients can start and inspect project autopilot runs

### Changed

- Project manager operations no longer require `curl` for common TUI workflows after a project has been created
- Dogfooding documentation now shows both TUI and HTTP routes for project manager operation

## [0.5.0] - 2026-03-14

### Added

- Starter workspace setup now installs bundled plugins such as `project-swarm` into `workspace/plugins`
- `tars doctor --fix` now restores missing bundled workspace plugins in addition to starter files

### Changed

- Bundled skill and plugin directories now resolve from installed package layouts such as `share/tars/{skills,plugins}` as well as repo-local paths
- Release archives, the curl installer, and the Homebrew formula now install bundled `share/tars` assets alongside the `tars` binary

## [0.4.0] - 2026-03-14

### Added

- Bundled `project-swarm` plugin with `project-start` and `project-autopilot` skills for workspace project kickoff and autonomous follow-through
- Built-in project runtime tools for board read/write, activity read/append, task dispatch, and background autopilot start
- Natural-language project kickoff routing for chat and Telegram when a project brief is being collected or a project start request is detected
- Background project autopilot loop that can keep dispatching `todo` and `review` stages while updating project state for the dashboard

### Changed

- Minimal chat tool injection now includes safe project runtime tools needed by the bundled project skills
- Project kickoff can proceed from a brief-driven interview instead of requiring only manual API calls
- Test chat helpers are synchronized for concurrent inflight chat coverage

## [0.3.0] - 2026-03-14

### Added

- Project manager workflow primitives: project activity log, Kanban board storage, and a server-rendered dashboard with live updates
- Project task orchestration with built-in `codex-cli` and `claude-code` worker profiles plus a gateway-backed task runner
- Review gate and GitHub Flow metadata tracking for project tasks, including issue/branch/PR and verification status
- `POST /v1/projects/{id}/dispatch` to run `todo` or `review` project task dispatch stages through the orchestrator

### Changed

- The project dashboard now renders board state, recent activity, and a dedicated GitHub Flow status block in one page
- Review-required tasks now stop at `review` until a reviewer run approves them
- Test/build and GitHub Flow metadata now gate task promotion to `review` or `done`

## [0.2.0] - 2026-03-11

### Added

- `tars init` to create a starter workspace plus minimal `workspace/config/tars.config.yaml`
- `tars doctor` and `tars doctor --fix` to validate or repair local starter files before first run
- `tars service install/start/stop/status` to manage `tars serve` as a macOS LaunchAgent

### Changed

- Quick start documentation now prefers `init -> doctor -> service` before manual `tars serve`
- The public example config comments now point packaged installs to the starter onboarding flow

## [0.1.2] - 2026-03-10

### Changed

- Release assets now build both macOS archives on a single `macos-14` runner so GitHub Release publishing is not blocked by a second runner matrix leg

## [0.1.1] - 2026-03-10

### Added

- Automated release workflow driven by `VERSION.txt` changes on `main`, including tag/release publishing and Homebrew tap updates
- Public `install.sh` for curl-based macOS installs from GitHub Releases
- Homebrew tap formula generation for `devlikebear/homebrew-tap`

### Changed

- Public documentation is maintained in English for the published repository surface
- `install.sh` now installs the latest published GitHub Release by default
- Release PRs must update `VERSION.txt` and `CHANGELOG.md` together

## [0.1.0] - 2026-03-08

### Added

- Initial public release of the local-first TARS runtime
- Embedded build metadata via `VERSION.txt`, Git commit, and build date
- `tars version` and `tars --version`

### Changed

- Primary Go module path is `github.com/devlikebear/tars`
- Primary plugin manifest filename is `tars.plugin.json`
- Primary user extension directories use `~/.tars`

### Security

- Repository publishing flow includes `make security-scan`
- Gitleaks false-positive handling is documented via repository ignore metadata
