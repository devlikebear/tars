# Feature Map

Last reviewed: 2026-10-10. Start an isolated instance with `make build` then `bin/tars serve --config <sandbox-config>`; use a throwaway workspace for verification.

## Focus mode

### Natural-language template editing
- Kind: console screen + admin API
- Reach: `/console/focus/templates` (linked from the focus home), or directly `POST /v1/focus/templates/draft` → review → `PUT`/`DELETE /v1/focus/templates/{id}` (admin token required for all three; see `docs/decisions/focus-mode.md` §4.1.1).
- Drive: open the template editor, click "New template", describe one in plain language (e.g. "a changelog template: gather commits, write the entry, check it reads well"), click Draft, review the per-stage diff, then Save. Click Edit on a saved template and describe a change (e.g. "add an illustration stage before revise") to refine it; describe wanting it removed to get a delete draft, then confirm. Editing a built-in template (Development/Writing/Research) always drafts a copy under a new id instead of replacing it.
- Healthy result: the draft response never touches `<workspace>/focus-templates/`; only Save/Delete does. A saved template appears in the New task template picker immediately. A built-in base produces `copied_from_builtin: true` and a warning naming the new id. A model response that cannot be validated retries once, then fails with 400 rather than saving something broken.
- Evidence: `go test ./internal/focuspipeline -run Template -v` (file-level save/delete rules) and `go test ./internal/tarsserver -run FocusTemplate -v` (draft retry, tool-attempt rejection, admin gating). Console: `cd frontend/console && node --experimental-strip-types --test tests/focusTemplateEdit.test.ts`. End-to-end against the mock LLM (`[e2e:focus-template-edit]`): `make console-e2e` (`frontend/console/e2e/focusTemplates.spec.ts`, English and Korean).

### Multiple folders per task
- Kind: API / console UI
- Reach: `POST /v1/focus/pipelines` and `POST /v1/admin/sessions` take `extra_dirs: string[]` alongside `cwd`; the focus home's New task form shows an "Add folder" list below the primary folder field (`FocusNewTask.svelte`).
- Drive: start a task with a primary folder and one extra folder (console: fill the primary field, click Add folder, fill the extra row, Start). The primary is the only one that can be isolated (checking Isolate in a worktree never touches the extras).
- Healthy result: `GET /v1/admin/sessions/{id}` shows both folders in `work_dirs` and only the primary as `current_dir`; a chat turn backed by claude-code-cli adds one `--add-dir` per extra folder (not for the active cwd or the configured workspace, both already covered). A folder that does not exist, or more than 8 total (primary + extras), fails the create before anything is made.
- Evidence: `go test ./internal/tarsserver -run 'TestNewSessionWithExtraDirs|TestFocusCreateWithExtraDirs|TestChatAPI_PassesExtraWorkDirsAsAddDirs' -v`, `go test ./pkg/llm -run TestClaudeCodeCLIClientChat_AddDirsAddsAnExtraFlagPerFolder -v`; console: `cd frontend/console && npx playwright test e2e/focus.spec.ts -g "registers both as the session"`.

### The developer's own findings
- Kind: API / console UI
- Reach: `POST /v1/focus/pipelines/{id}/findings` (`{title, scenario?, file?, line?, severity?, decision?}`); on the pipeline screen, **+ Add a finding** under the card deck during the review and PR review stages (`FocusAddFinding.svelte`). Rules: `docs/decisions/focus-mode.md` §4.
- Drive: run a task to its review stage, wait for the triage cards, click Add a finding, give a title (and a file and line to get the diff excerpt), then **Add to fix**; decide the agent's findings.
- Healthy result: triage progress counts the new card as decided (`1 / 3 decided`), its card carries the *added by you* badge, and the fix turn's prompt ("Fix these findings: …") lists it with `(added by the developer)`. Outside review/pr_review, behind a blocked gate or at the merge gate the input is hidden and the API answers 409 with the current pipeline; no title is 400.
- Evidence: `go test ./internal/focuspipeline -run 'AddFinding|AddedFinding|DeveloperFindings' -v`, `go test ./internal/tarsserver -run TestFocusDeveloperFinding -v`; console: `cd frontend/console && npx playwright test e2e/focus.spec.ts -g "developer adds"`.

### Second opinion review
- Kind: API / console UI (opt-in per pipeline, off by default)
- Reach: `POST /v1/focus/pipelines` with `second_opinion: {tier: "heavy"|"standard"|"light"}`; on the focus home's New task form, the **Second opinion review** checkbox and its tier select (`FocusNewTask.svelte`). Rules: `docs/decisions/focus-mode.md` §5.1.
- Drive: map a tier to another provider or model than chat's, start a task with the box ticked on that tier, run it to its review stage. The progress line reads *Reviewing · second opinion review · <tier>* while the hidden worker session reviews the diff; then triage opens.
- Healthy result: triage holds the stage's own findings plus the second opinion's new ones (*second opinion* badge); a finding both reported is one card with a *both reviews* badge. The fix prompt marks second-opinion findings `(second opinion: <tier>)`; the round after a fix gets none; `pipeline.json` has `second_opinion.done: ["review"]` and the worker session id, and the pipeline's own transcript has no second-opinion turn. A tier on antigravity-cli or unmapped is a 400 at create; one with chat's own model answers `second_opinion_warning`. A failed review leaves a *second opinion not received* notice and the round goes on. A pipeline without the option has no `second_opinion` in its file.
- Evidence: `go test ./internal/focuspipeline -run 'SecondOpinion|NoOptInGolden' -v`, `go test ./internal/focusprobe -run ReviewDiff -v`, `go test ./internal/tarsserver -run 'SecondOpinion' -v`; console: `cd frontend/console && node --experimental-strip-types --test tests/focus.test.ts` and `npx playwright test e2e/focus.spec.ts -g "second opinion"` (mock LLM `[e2e:focus-second-opinion]`, `[e2e:focus-second-opinion-noblock]`).

## CASE

### Bridge
- Kind: console panel + API
- Reach: Mod+/ (⌘/ or Ctrl+/) on any screen, the palette's "Ask CASE about TARS (Bridge)", or **Ask about TARS** in CASE's bubble options menu (⋯); `POST /v1/case/ask` then `GET /v1/case/answer` (user token). Rules: `docs/decisions/case-bridge.md`.
- Drive: with a focus pipeline at a gate, a turn running in another session, or a failed cron job, open the Bridge and ask "is anything stuck?". Click a card.
- Healthy result: the answer names what waits and why, in one or two sentences, with up to four cards; a card opens its screen (`/console/focus/<id>`, `/console/chat/<id>`, `/console/cron`, `/console/ops`, `/console/analytics`, `/console/pulse`) and closes the panel. The main session's transcript has no Bridge question; a hidden "CASE Bridge" worker session does (`/v1/admin/sessions?hidden=1`), one per day. A second question while one is answered is 409. Asking for a change ("cancel that cron") gets a pointer to the screen, never an action.
- Evidence: `go test ./internal/casebridge -v`, `go test ./internal/tarsserver -run 'CaseBridge|LatestPulse|LastAssistantReply' -v`; console: `cd frontend/console && node --experimental-strip-types --test tests/caseBridge.test.ts` and `npx playwright test e2e/case-bridge.spec.ts` (mock LLM `[e2e:bridge]`).

## Initiative

### Arrival brief and check-in answers (case-companion phase 3)
- Kind: console bubble + API
- Reach: on by default in live mode (`initiative.arrival_brief`, `initiative.check_in_buttons`); the brief rides on an arrival greet's companion event, the buttons on a check-in's. Answers: `POST /v1/initiative/checkin/answer {entry_id, action: ok|snooze|quiet_today}` (user token). Snapshots: `workspace/_shared/initiative/snapshot.json`.
- Drive: with initiative live, keep the console tab hidden for over ten minutes after four quiet hours; meanwhile let a focus pipeline reach a gate. Come back. For the buttons, wait for a check-in on a screen other than the main session and press **Again in 20 min**.
- Healthy result: the greeting's bubble lists at most three lines ("Needs you · "<title>" is waiting at G2 (review)"), block lines first; each opens its screen. Nothing changed → the greeting alone. The main session's transcript holds only the greeting's words. After **Again in 20 min**, `GET /v1/initiative/status` has `check_in_snoozed_until`; no check-in until then, a greet is unaffected; the ledger has an `answer` line. **Quiet for today** makes later ticks `stopped_today`, across a restart.
- Evidence: `go test ./internal/initiative -run 'Brief|CheckIn|AnswerLines' -v`, `go test ./internal/tarsserver -run 'BriefSnapshot|Brief|CheckInAnswer|InitiativeSpeakerAttaches' -v`; console: `cd frontend/console && node --experimental-strip-types --test tests/companionPet.test.ts`, `npx playwright test e2e/companion-events.spec.ts e2e/initiative-live.spec.ts`.

### Morning brief and evening recap (tars#1001)
- Kind: console bubble + initiative message
- Reach: opt-in, `initiative.morning_brief` (live) and `initiative.evening_recap` (+ `initiative.evening_recap_window`, default the hour before quiet hours). Status: `GET /v1/initiative/status` → `morning_brief`, `evening_recap`; `tars doctor`.
- Drive: with initiative live and `morning_brief: true`, have a cron job due later today and a daily log for yesterday, then arrive for the day's first greeting. For the recap, set `evening_recap: true` and a window around now, with a focus pipeline open or a cron job that failed today.
- Healthy result: the first greeting's bubble lists "Cron today: …", "Yesterday: N chat turns in M sessions …", "In progress: …" after any arrival lines; a follow-up leaves the in-progress line out. The recap is one main-session message ("Here's today's wrap-up." / "오늘 하루 정리예요." + up to three lines), once a day, never composed by a model; its rows open their screens.
- Evidence: `go test ./internal/initiative -run 'EveningRecap|MorningBrief|RecapDue|DefaultRecapWindow|WithoutSaid' -v`, `go test ./internal/tarsserver -run 'OpenWork|DaySources|TodayCron|InitiativeSpeakerMorningBrief|InitiativeSpeakerEveningRecap|WiresDayBrief' -v`, `go test ./internal/cron -run NextRunIn -v`.

### Offline tuning tools
- Kind: CLI
- Reach: `tars initiative export|scenarios|calibrate|adapt` (`--config`, or `--ledger <path>` without one). Guide: `docs/initiative-tuning.md`.
- Drive: with a workspace whose initiative ledger has some ticks, run `tars initiative export --out initiative.jsonl`, set one record's `label.speak` to `false`, then `tars initiative scenarios --in initiative.jsonl`; `tars initiative adapt` on a live-mode ledger with replies.
- Healthy result: the export has one line per tick with the reply outcome joined and `null` label slots, and no `error`/`body` text; `scenarios` prints a `decide.jsonl`-shaped line per correction and names the ones the current policy fails; `adapt` prints a verdict and the daily_cap/cooldown it would set; `calibrate` reports per-question AUC and a candidate only with enough labeled jev readings. None of them writes the config.
- Evidence: `go test ./internal/initiative -run 'TestEval|Dataset|Scenarios|Calibrat|Pacing' -v`, `go test ./cmd/tars -run TestInitiative -v`.

## Computer use

### Desktop action tool
- Kind: chat tool
- Reach: console chat or `POST /v1/chat`; ask to call `computer_use(goal, app?, inputs?, max_steps?)`.
- Drive: with cua-driver running and permitted, ask Calculator to clear and calculate 2 + 3. The default backend resolves the `computer_use` role to light; no Jev configuration is needed. With `tools.computer_use.driver: agent-desktop` the same goal runs through the agent-desktop binary instead (no daemon).
- Healthy result: `status: done`, visible result 5, action trace followed by a final observation, and LLM input/output usage. Risky controls/secure typing return `needs_confirmation`; answer via `resume` and `confirm`.
- Evidence: capture tool JSON and the final Calculator observation. Automated driver/provider boundary coverage: `go test ./internal/tarsserver -run 'TestComputerUse.*EndToEnd' -v`; live tool coverage: `TARS_COMPUTER_USE_LIVE_CONFIG=<config> CUA_DRIVER_PATH=<driver> go test -tags integration ./internal/tarsserver -run TestLiveComputerUseLight -v`.

### Backend configuration and readiness
- Kind: configuration / command
- Reach: `tools.computer_use` in YAML and `tars doctor --config <config>`.
- Drive: check default `enabled: true`, `backend: llm`; set `enabled: false` to disable or `backend: jev` with `jev.base_url` to switch. Set `expose_values: false` to withhold field contents.
- Healthy result: doctor reports the selected LLM tier/model or Jev location and missing driver guidance. Jev keeps confidence gates. Missing setup returns `unavailable`; no computer action runs.
- Evidence: configured-model compatibility without GUI access: `TARS_COMPUTER_USE_MODEL_CONFIG=<config> go test -tags integration ./internal/tarsserver -run TestConfiguredLightRecordedObservation -v` (synthetic screen data only). Doctor output and `go test ./internal/config ./cmd/tars -run 'TestComputerUse|TestCheckDoctorComputerUse' -v`.
