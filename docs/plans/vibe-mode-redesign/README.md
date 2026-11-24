# Vibe mode redesign: development plan

Status: plan, 2026-11-24. Owner decisions in this file were made that day.

Vibe mode (the console's easy mode, `docs/decisions/vibe-coder-identity.md`) works through phase 4 but its screens are a text box, a status pill and a stacked page. The owner judged them not good enough to release and approved a new design for the desktop console and the iPhone app. This plan turns that design into work units, one pull request each.

## What to read first

- `docs/decisions/vibe-coder-identity.md`: who the product is for and the rules that do not change.
- `screens/`: the approved design, one file per screen. `D-*.dc.html` are the desktop console, `M-*.dc.html` the iPhone app. They are static markup: read the copy, the structure and the inline styles. They do not render outside the design tool.
- `frontend/console/DESIGN.md`: the console's design rules. The design uses its tokens, so build with the tokens, not with the hex values in the screens.

## Owner decisions

1. **Vibe mode is the product.** Expert mode and focus mode are hidden. No screen a vibe-mode user sees links to them. The design files still show an "expert mode" link in places: leave those links out.
2. **Reuse what exists.** Every unit starts from the code that already does the job. A unit adds a new mechanism only where this plan says so, and says why in its pull request.
3. **The phone does everything the desk does in vibe mode**, by driving the desk remotely. The one exception is first-time setup of the desk itself (installing and signing in to the AI), which needs the desk.
4. **Development runs in sub-sessions** on Opus 5.5 at medium effort, directed from one planning session. One unit, one branch, one pull request. A sub-session never merges.

Decision 1 changes `vibe-coder-identity.md` §4 ("expert mode one click away") and principle 4. Unit 0 amends the record.

## Rules that do not change

- Every stage transition is decided by a fact the server reads (an exit code, a probe). No model is asked whether work is done. No unit adds a new way to move between stages.
- No vibe-mode string may contain a hidden word (`EASY_HIDDEN_WORDS` in `frontend/console/src/lib/easy.ts`). `tests/easy.test.ts` and `e2e/easy.spec.ts` enforce it; new screens join those tests.
- A pipeline that opts into nothing stays byte-identical: `internal/focuspipeline/testdata/no_opt_in.golden` is never rewritten.
- Publishing is always a person's click.
- After the plan is approved, vibe mode does not ask the person anything until the work is done, paused or stuck.
- New UI text goes through `$t` in English and Korean. Design tokens come from `app.css`.

## Cut from the first version

The review of the wireframes found these expensive or at odds with the rules. They are not in any unit below.

- Time estimates per project. The screens say "usually 10 to 20 minutes" as fixed text.
- Per-feature progress and a live preview while the team builds.
- A screenshot per check.
- Pointing at an element in the preview to change it.
- "Skip this part and finish" when the team is stuck.
- Attaching a picture or a site address to an idea.
- Resuming by itself after a usage limit. The screens show a checkbox for it; build the pause screen without the checkbox. It needs a change to the decision record first.

## Reuse map

| The design shows | Build it on |
| --- | --- |
| Project list with a state per project | `easyproject.Store`, `focuspipeline.WaitingOf`, the phase rule behind `GET /v1/focus/pipelines` (`pipelinePhase`), `Pipeline.Saved`, `Pipeline.UsageLimitStop` |
| Team row and "who is working" | the pipeline's stages by kind, already mapped to bots in `lib/easy.ts` (`EASY_BOTS`, `easyView`) |
| Team conversation | the session transcript and turn feed: `lib/sideSession.ts` (`applySideEvent`, `historyMessages`), `attachChatStream` |
| Leader's multiple-choice questions | decision cards (`focuspipeline.Decision`, `focusDecideCard`) |
| Blueprint approval | the plan gate (`focusGate` plan approve), `Pipeline.GoalAfterPlan` |
| "This is how we will check" and the check results | `Plan.Verify`, one `VerificationResult` per command, the `CHECK FAILED` block (`ParseCheckReport`, `FailureFact.Check`) |
| Stop and continue | `POST /v1/focus/pipelines/{id}/stop` and `/resume`, the usage-limit pause (phase 4-3) |
| The running app | `easyproject.Previews`, `EasyPreview.svelte` |
| Change it like this | `POST /v1/easy/projects/{id}/changes` |
| Versions and going back | `easyproject.Versions`, `RestoreVersion`, `EasyVersions.svelte` |
| Getting ready, first run | `EasySetup.svelte`, `EasyStart.svelte`, `GET /v1/easy/readiness` |
| Phone: desks, pairing, readiness, keep awake | the Desktops tab, `PairSheet.svelte`, `GET /v1/focus/readiness`, the keep-awake setting |
| Phone: ideas saved while the desk is off | the Ideas tab (`lib/mobile/ideas.ts`, `ideaStore`) |
| Phone: live updates | `GET /v1/focus/events`, `DesktopEvents` |
| Phone: screens shared with the console | the pattern of `FocusPipeline.svelte` mounted by `MobilePipeline.svelte` with a remote API object |

## Work units

Each unit is one pull request against `main`. "Done" lists what must be true; every unit also passes the pull-request preflight in `CLAUDE.md` for the files it touches. A change to a global console element needs the full `make console-e2e`.

### Unit 0. Vibe mode only

Hide expert mode and focus mode from a vibe-mode user.

- A config field (proposed: `console_expert_mode`, default `false`) decides whether the workbench is reachable. With it off, `/console` and every workbench route land in vibe mode, the sidebar and command palette offer no workbench or focus entry, and vibe-mode screens show no expert link.
- With it on, the console behaves as it does today. The maintainer's own workspace turns it on. The e2e server turns it on, so existing specs keep running; new specs cover it off.
- Amend `vibe-coder-identity.md` (§4, principle 4, phase 7) and `DESIGN.md`.
- Reuse: `console_default_mode`, `defaultModeRedirect` and `focusChromeHidden` in `lib/focus.ts`.
- Done: with the field off, no rendered vibe-mode screen links to the workbench, and a workbench address opens vibe mode.

### Unit 1. Project states, archive, rename, delete (server)

The board needs a state per project in one call. Today the record knows only `creating`, `ready`, `failed`, and the console reads one pipeline per project.

- A pure function gives a project its board state from the record and its pipeline: `your_turn` (plan gate open, or stuck), `working`, `paused` (usage limit), `stuck`, `done`, `published` (reserved for unit 9), `starting`, `failed`. `GET /v1/easy/projects` returns it with the one line the card shows (for example the waiting reason), reading the pipelines on the server.
- The list takes `state`, `limit` and a cursor, newest change first, and returns counts per state.
- `PATCH /v1/easy/projects/{id}` renames and archives or restores. An archived project leaves the board and appears under `?archived=1`.
- `DELETE /v1/easy/projects/{id}` is refused while the team works. It stops the preview and moves the project folder into a `.deleted` folder beside the projects, so a mistake can be undone by hand. It never removes a folder outside the projects directory.
- Session retention must not archive or delete the session of a project that still exists. Check `planSessionRetention` first; the review inferred this gap from the code and did not run it.
- Reuse: `easyproject.Store`, `focuspipeline.WaitingOf`, `session_retention.go`'s existing exclusions.
- Done: table tests for the state function; the list answers in one request for 30 projects; a project a month old still shows `done`.

### Unit 2. Shell, home and project board (console)

`screens/D-Home.dc.html`, `screens/D-Projects.dc.html`.

- A vibe-mode shell: the top bar with **New** and **My projects** (with the count of projects waiting on the person) and the AI connection dot.
- Home: the idea box, the team strip, example ideas, the three most recent projects with a waiting one first.
- Board: five columns by state, moved only by the server's state; search by name; five cards per column then "more"; the card menu (open, rename, archive, delete with a confirmation); the archive view; an empty state per column.
- Reuse: `EasyHome.svelte`'s create flow and example strings, the list from unit 1.
- Done: the hidden-word tests cover both screens; an e2e spec creates a project from the home screen and finds it on the board.

### Unit 3. A blueprint in the plan (server)

`screens/D-Plan.dc.html` shows more than a task list: the checks in plain words, what is saved, what is left out, rough screen sketches, and the leader's questions before it.

- The `webapp` template's plan carries, as optional fields: `checks` (a plain-language label and the `verify` command it stands for), `screens` (a name, one line, and a list of simple parts from a fixed vocabulary), `saved` and `not_now` (short lists). `fitPlan` puts each check's command into `Plan.Verify`, so transitions still read only exit codes.
- A check's result is the `VerificationResult` of its command. Expose results by check to the console.
- The leader asks at most three multiple-choice questions before the plan, as decision cards, each with a "you decide" option. Goal mode starts only after plan approval, so a person answers them.
- Templates that set none of these fields behave as before. The golden file stays as it is.
- Reuse: `Plan.Verify`, `VerificationResult`, `Decision`, `ParseCheckReport`, `template_webapp.go`.
- Done: a plan block with checks produces verify commands and labelled results; a plan without them is unchanged; the mock model in the e2e server emits a blueprint.

### Unit 4. The workspace: plan, making, checking (console)

`screens/D-Plan.dc.html`, `screens/D-Make.dc.html`, `screens/D-Check.dc.html`.

- One workspace per project: the header with four steps (plan, make, check, publish) and the state pill; the team row and conversation on the left; the stage's own content on the right; one bar at the bottom with the single thing to press.
- Plan: answer the questions, read the blueprint, approve, or say what to change.
- Making: who is working, the stage strip, the blueprint and the checks not yet run. No preview, no estimate.
- Checking: the results by check, a failed one with expected and seen, the count passed.
- During work the input is a note for later, kept on the project record (a small server addition) and offered on the result screen. **Stop** uses the pipeline's stop; the stopped state offers **Continue**.
- Reuse: `easyView`, the side-session feed helpers, `focusGate`, `focusDecideCard`, the stop and resume routes.
- Done: an e2e run goes from an idea through an approved blueprint to a passed check on these screens, with no hidden word.

### Unit 5. Result, versions, waiting and stopped states, first run (console)

`screens/D-Result.dc.html`, `screens/D-States.dc.html`, `screens/D-Start.dc.html`.

- Result: the running app with PC and phone widths, reload and open in a new window; the change box; saved notes offered as changes; the version strip with going back; the bar with **Publish**, disabled with a plain reason until unit 9.
- A change keeps today's flow (a plan, an approval, then goal mode). The workspace shows the change's small blueprint for approval. Building changes without an approval needs a decision-record change and is not in this unit.
- States: getting ready (three steps and the five failure codes with **Try again**), paused on the usage limit (with and without a known time), stuck (the failed check, a box to explain again, **Try again**).
- First run: restyle `EasyStart.svelte` and `EasySetup.svelte` to the design, and say that a paid Claude plan is needed. The "open the sign-in window" button needs the desktop shell and is not in this unit; keep the copyable command inside the folded help.
- A console opened from another computer shows a plain sentence in place of the preview, as today.
- Reuse: `EasyPreview.svelte`, `EasyVersions.svelte`, the limit pause, `easyLimitResets`.
- Done: the existing easy e2e specs pass on the new screens, plus specs for the stopped and paused states.

### Unit 6. The phone reaches vibe mode (server)

A paired phone has the `focus` role and reaches only `focusEndpointRules`. None of the easy routes are in it.

- Add the project list, create, read, rename and archive, change, versions and restore, notes, and readiness to the table. Each handler checks the device as the focus routes do. The session of an easy project counts as a session the device may reach.
- A phone may create a project (the server picks the folder, so no registered folder is involved) and may approve a plan. It still cannot reach anything outside the table.
- The demo desk (`lib/mobile/demoDesk.ts`) answers the new routes.
- Reuse: `focusDeviceScope`, `checkDeviceCreate`, the tests around the focus table.
- Done: policy tests show a phone reaches these routes and nothing new beside them.

### Unit 7. The running app on the phone (server, design first)

The preview answers only on the desk's loopback address, and the server refuses a phone on purpose. Showing it on the phone needs the server to relay the preview through its own address.

- Start with a short design note in this folder: how the relay is addressed and how a web view's requests are authorized without a bearer header (a short-lived token in the path is the likely answer), what a relayed app may reach, and how the dev server's live-reload socket is handled.
- Then build it. The relay serves only a running preview of a project the device may reach.
- This unit has the most risk. If the note finds no safe design, the phone shows the result's checks and versions and opens the app only after publishing.
- Done: the note is reviewed before code; the phone shows a running app over the tailnet in the simulator.

### Unit 8. The phone screens (app)

`screens/M-*.dc.html`.

- Tabs: **Projects**, **New**, **Computers**. The Waiting, Ideas and Chat tabs are folded in: waiting projects lead the list, ideas are what **New** saves while the desk is off, and the project's conversation replaces the chat tab.
- The project screens are the console's own workspace components mounted with a remote API object, as `MobilePipeline.svelte` does for focus. Phone-width differences live in the mobile wrapper.
- Notifications reuse what exists today. A push notification from the app itself is unit 10.
- Reuse: the Desktops tab, `PairSheet.svelte`, `ideaStore`, `DesktopEvents`, `requestJSONAt`.
- Done: on the demo desk and against a paired loopback desk, an idea goes to a finished project from the phone.

### Unit 9. Publish (phase 5 of the decision record)

`screens/D-Publish.dc.html`, `screens/M-Publish.dc.html`. Its own track; it does not block units 0 to 8.

- First check the publishing target's limits and sign-in flow against its official documentation, including whether a phone can start the sign-in and whether an address can be chosen per project. The screens leave the price line and the address rule as placeholders until then.
- Then: the deploy skill in `tars-skills`, the publish step, the Connect step, reading the published address before saying it is online, the `published` state, "see who signed up", and taking a publication down.

### Unit 10. Later

- Push notifications from the app.
- The "open the sign-in window" button on the first-run screen.
- Voice input on the phone.
- Anything in "Cut from the first version" that the owner brings back.

## Order

```text
0 ─┬─ 1 ── 2
   └─ 3 ── 4 ── 5
1 ── 6 ── 8 (after 4 and 5)
7 runs beside 8; 9 runs on its own track
```

Units 1 and 3 touch different packages and can run at the same time. Two units that touch the same files never run together.

## How a sub-session works

- It gets one unit, this folder, and the branch to start from.
- It reads the code before changing it and follows `CLAUDE.md`: the Makefile targets, layering, tests beside the code, both languages for UI text.
- It opens a pull request with the unit's "Done" list checked against what it ran, says plainly what it did not verify, and stops. It does not merge.
- If the unit as written cannot be built on the existing code, it says so in the pull request or asks before inventing a new mechanism.
