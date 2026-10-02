# ADR: Focus mode — a development pipeline view of a chat session

- Status: Accepted (design); implementation phased, see §9
- Decision date: 2026-10-01
- Scope: a second console mode next to today's workbench, which becomes **Advanced mode**

## Context

TARS is meant to be the maintainer's primary AI development tool, and the workbench epic (#967, `console-workbench.md`) made it a capable one. It also made it as busy as Claude Desktop or the Codex app: dock panels, tool-card streams, tier/cost/permission chips, and long free-form model output. When developing with it, it is hard to tell:

- which stage of the work we are in,
- what decision is being asked of the developer right now,
- which part of a long answer actually matters.

TARS's main job is development, and development already has a shape. The maintainer's own loop, which the 2026-09/10 dogfooding run followed by hand for 18 PRs, is:

```
plan → (implement → test)* → (review → end-to-end verify → fix)* → PR → (review → verify → fix)* → merge → release
```

Focus mode makes that loop the structure of the screen: show where we are in the pipeline, what the AI did in this stage in brief, and what the developer must decide — and hide everything else.

## Decisions

### 1. Focus and Advanced are two views of the same session

- A pipeline is attached to an ordinary main chat session (1 pipeline = 1 session = 1 feature/issue). Focus mode adds no separate chat store.
- "View in Advanced" in the focus header opens the same session in the existing workbench, and back. Debugging, which never fits a linear flow, happens there without losing the pipeline.
- The raw transcript is never deleted, only folded. Every card links to the turn it came from.

### 2. Default mode is a server config field

- New config field `console_default_mode` (YAML `console.default_mode`, values `focus` | `advanced`), wired the same way as `companion_enabled` so the browser console and the desktop shell agree.
- Completing the onboarding wizard writes `focus`. An empty value means `advanced`, so existing installs keep today's behaviour until they opt in.
- Routes: `/console/focus` (focus home) and `/console/focus/<session>` (pipeline screen). When the default is `focus`, `/console` lands on the focus home.

### 3. Screen

Top to bottom:

1. **Pipeline stepper** — done (✓), current (highlighted, with a `↻N` iteration badge for loops), pending, skipped (dashed, struck through). Clicking a stage shows that stage's card history.
2. **Card deck** — exactly one card visible at a time, navigated with ←/→. Parallel outputs (e.g. several reviewers) are serialized into the deck, so side-by-side review is impossible by construction.
3. **Q&A drawer** — questions about the current card only (§8).
4. **Stage instruction input** — always present; text goes to the main agent as an instruction for the current stage.

The **focus home** lists running pipelines and a "New task" entry (folder + one-line goal), a reduced session board.

Hidden in focus mode: dock panels, the tool-card stream, the sidebar, status-bar details (tier, cost, permission mode). While a turn runs, one progress line is shown instead ("Implementing · 3 files changed · running tests").

### 4. Pipeline skeleton and gates

Loops run automatically inside a stage; the developer acts only at gates. Stage transitions are decided by **facts**, never by asking the model whether a stage is done (the lesson of initiative P0, #998: combined judgements asked of a model were no better than chance).

| Stage | What the AI does automatically | Human gate | Exit condition (facts) |
|---|---|---|---|
| ① plan | Proposes tasks, the stages that apply, a contract (done criteria + verification commands), loop limits | **G1 approve plan** — stage skipping and verification commands are confirmed here | approved |
| ② build ↻ | Implements; the server then runs the contract's verification commands; on failure the next turn starts automatically with the failure excerpt | none (only when blocked) | all tasks completed and verification exit 0 |
| ③ review ↻ | A review turn reports findings; a fix turn addresses accepted ones; verification (incl. e2e commands) re-runs | **G2 triage findings** — one card at a time: fix / dismiss / ask | no accepted finding open and verification passes |
| ④ PR | Drafts the PR title/body | **G3 open PR** | PR number known |
| ⑤ PR review ↻ | CI failures and review comments become finding cards; fixes are pushed | G2-style triage | all checks green and no accepted finding open |
| ⑥ merge | — | **G4 merge** | merged → session worktree cleaned up |

- **G4 follows the PR head.** G4 shows the facts of the head commit it opened on, so the server keeps probing while it is open: when the open PR's head differs, G4 is superseded with a notice and pr_review resumes on the new head, reopening G4 only after a later probe finds that head green. Approving G4 probes first and answers 409 with the current pipeline if the head moved (#1087).
- **One skeleton, skippable stages.** The plan proposes which stages apply (a small fix may go build → PR → merge); the developer confirms at G1. No per-type templates.
- **Release is outside the feature pipeline.** TARS releases by batching merged PRs into one release PR (v0.41.0, v0.42.0). A separate *release train* view (§9 P5) collects pipelines merged since the last release.
- **Blocked.** Reaching a loop limit (defaults: build 3, review 2, PR 3) or repeating the same failure raises a *blocked* gate: try once more / give an instruction / open in Advanced. Build also blocks on **no progress**: after 2 turns per planned task plus the build limit, passing turns whose report never sets `tasks_done` stop at the same gate instead of looping. A failure counts as "the same" when its first failing command and output excerpt match the previous one with numbers (durations, counts) ignored. Retry and instruct allow one more iteration.
- **Review loop (P3).** The review diff starts at the pipeline's *base commit*: HEAD of the session folder recorded at the pipeline's first turn, before any change, so review sees the build stage's work (`git diff <base>...HEAD`, plus uncommitted changes). Each finding card carries the server's excerpt of that diff around its `file:line`; the model never supplies it. Triage closes only through card decisions (`fix` | `dismiss`); gate actions other than stop are refused on it, and a turn sent while it is open is the developer's question — no block required, its findings ignored, nothing changes. Dismissed findings (file, line, title) carry across the stage's rounds: the re-review guidance lists them as already dismissed, and a finding reported again with the same file and title is dropped. One fix turn carries every accepted finding, then verification runs the verify **and** end-to-end commands. Passing after fixes starts a new review round (the limit counts rounds); passing with nothing fixed ends the stage. A verification failure in review becomes a failure card and a fix turn, like build; the same failure twice, or more failures in one round than the limit, blocks with *Review blocked*, whose retry starts a new round and whose instruction is sent as a fix turn.
- **A decision card pauses the loop.** When the agent asks a question, that question is an implicit gate; answering continues the loop. The questions one turn asked are answered as one turn (the last answer sends them all), answers queued while a turn runs merge into one, and an answer whose stage or round has ended is never sent as a turn of a later stage: it rides in the next turn as context, or leaves a notice when none follows. An answer given while a gate that takes questions is open (G1, triage, G3, G4) goes out right away as the developer's question, like a typed instruction: the gate stays open and the pipeline owes nothing; only behind a *blocked* gate does it wait (#1079).

### 5. Facts come from deterministic sources

| Fact | Source |
|---|---|
| Tests / verification passed | The existing `TaskContract` verification run (`handler_task_verification.go`): exit code + proof. The commands were approved by a human at G1, so this does not reopen the "repository config runs commands" problem that blocks `worktree_setup`. |
| What changed | Turn checkpoint diffs (`internal/checkpoint`) |
| PR / CI / review state | Server-side **read-only** `gh pr view --json …` probe. Bot comments and reviews (gh-flagged, a `[bot]` login, or a known status-note app such as `sonarqubecloud` — gh drops `[bot]` for GitHub Apps; the list is `knownBotApps` in `internal/focuspipeline/pr.go`) do not become findings unless they request changes: they are informational, and a failing quality signal arrives as a failing check (#1094) |
| Write actions (open PR, merge, push) | Executed by the agent with its own permissions, only after the developer passes the gate |

### 6. Structured blocks, parsed by the server

Each focus turn ends with a structured block the server parses into pipeline state. This works for every provider, including `claude-code-cli`, which never calls TARS's native `tasks` tool.

```
<focus-plan>{"goal":"…","tasks":[{"title":"…","done":"…"}],"stages":["plan","build","review","pr","pr_review","merge"],"verify":["make test"],"e2e":["make console-e2e"],"limits":{"build":3,"review":2,"pr":3}}</focus-plan>
<focus-report>{"summary":"…","decisions":[{"id":"d1","question":"…","options":["…","…"]}],"risks":["…"]}</focus-report>
<focus-findings>[{"id":"f1","severity":"high|medium|low","file":"…","line":42,"title":"…","scenario":"…"}]</focus-findings>
<focus-pr>{"title":"…","body":"…"}</focus-pr>
```

- Stage instructions and the block format are appended to every focus turn's user message in a `<focus-stage>` block, the same hidden-guidance pattern as `<console-context>` (`chat_console_context.go`) but without its 2000-byte cap, so they never appear as the user's words. Stage instructions are specific (review: "report findings, do not edit").
- The console folds the blocks out of the visible transcript, as it already does for `<review-notes>`.
- A missing or malformed block is never guessed around: a "report format missing" card appears and the server sends one re-request turn.

### 7. Cards

Ordered by priority:

| # | Card | Content | Actions |
|---|---|---|---|
| 1 | Gate | G1 plan (tasks, stages, verification commands — editable), G3 PR draft, G4 merge summary, blocked | approve / request changes / stop |
| 2 | Decision | One question with options | option buttons (number keys), free answer |
| 3 | Finding | severity, `file:line`, problem, failure scenario, diff excerpt | fix / dismiss / ask |
| 4 | Verification failure | command, exit code, excerpt | informational (the loop fixes it) |
| 5 | Stage report | two-sentence summary, fact chips (diff stats, verification) | acknowledge |
| 6 | Change | one file diff at a time | acknowledge, leave a review note |

- Cards 1–3 must be handled one by one. Cards 4–6 can be acknowledged together ("acknowledge the remaining n").
- Card state (unseen / seen / decided) lives on the server so the desktop shell and browser agree.
- Every card records its source (turn number, message range). "View raw" (`o`) expands that slice of the transcript read-only under the card, reusing the existing message renderer.
- Keys: ←/→ move, 1–9 choose an option, `?` ask, `o` view raw.

### 8. Q&A side channel

- One hidden Q&A session per pipeline (`worker` kind), same cwd, permission mode **plan** (read-only): it can read code to answer but never edits.
- Each question carries the current card and its source excerpt as `console_context`; the upstream session is resumed, so follow-ups do not re-bill the context.
- Answers thread per card in the drawer. "Promote to instruction" puts a summary into the main input as a draft the developer edits and sends.
- Q&A never changes pipeline state and never enters the main agent's context unless promoted.

### 9. Architecture and phases

**Server.** A new app-layer package `internal/focuspipeline` (classified in `internal/architecture/layers.go`):

- pipeline model persisted next to the session as `pipeline.json`: stages with status (`pending` | `active` | `done` | `skipped` | `blocked`), iteration counts, gates, cards and their state, findings, PR info;
- block parser and a **pure** state machine `Next(state, facts) → (state, action)` tested table-driven, like `initiative.Decide`;
- a driver that reacts to turn completion: evaluates facts, then either enqueues the next automatic turn or stops at a gate. The developer's own messages — the first one (the goal, or a release pipeline's kickoff) and typed instructions — come from the console; every turn the pipeline asks for after that is sent by the server, which records it as owed (`pending_turn`) so a restart raises an *interrupted* gate instead of losing or silently resuming it. It runs server-side on top of background turns (#971), so loops continue when the console is closed; tool permissions needed while unattended go to the existing ops approval queue (#970).
- tasks and the contract reuse the existing session `Plan`/`Task`/`TaskContract`; the durable work ledger is not used (it is built for the unattended scheduler).
- API under `/v1/focus/…` and an SSE `pipeline` event for state changes.

**Console.** `lib/focus*.ts` pure functions (deck ordering, stepper layout, block folding), focus components, i18n section `src/i18n/sections/focus.ts` (en/ko), and a Focus mode section in `frontend/console/DESIGN.md`.

| Phase | Scope |
|---|---|
| **P1a server foundation** | `pipeline.json` model, block parser, pure state machine, API, SSE `pipeline`, `console_default_mode` config, per-stage `console_context` injection |
| **P1b console shell** | mode switch and post-onboarding default, `/console/focus` home and pipeline screen, stepper, card deck (gate, decision, report, change), view raw, plan gate G1 |
| **P2 build loop + Q&A** | verification after each turn, automatic next turn, blocked gate, Q&A drawer |
| **P3 review loop** | finding cards, G2 triage, fix turns, e2e verification re-run |
| **P4 PR and merge** | read-only `gh` probe, G3/G4, PR loop (CI and comments → findings), worktree cleanup |
| **P5 follow-ups** | release train, full pipeline graph view (`@xyflow/svelte`, already a dependency) |

From P2 on, focus mode is developed **in focus mode** — the most honest test of the design.

### 10. Testing

- Go: table-driven tests for the parser and state machine; handler tests for the API; driver tests with a fake chat runner and fake fact sources.
- Console: Node tests for `lib/focus*.ts`; behaviour tests for the focus store via `tests/helpers/compileSvelteModule.ts`.
- E2E: extend `e2e/mock-llm.mjs` to emit focus blocks; Playwright specs for the focus flow, including the Korean workbench check.

## Consequences

- The trust risk is real: if a stage report hides something that matters, the developer goes back to the raw transcript and focus mode goes unused. Hence facts are always shown from deterministic sources (§5) and every card has "view raw".
- Agents must follow the block format. Compliance is enforced by re-request, not by guessing, and a missing block is visible as a card.
- `gh` becomes a soft dependency for P4. Without it, the PR stages report "gh unavailable" and the developer passes them manually.

## Re-evaluate when

- Developers routinely switch to Advanced mid-stage to understand what happened — the cards are not carrying enough.
- Block-format failures exceed occasional re-requests on the primary provider.
- A real need for per-task-type pipelines appears that stage skipping cannot express.
