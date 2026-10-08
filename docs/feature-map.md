# Feature Map

Last reviewed: 2026-10-08. Start an isolated instance with `make build` then `bin/tars serve --config <sandbox-config>`; use a throwaway workspace for verification.

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

## Computer use

### Desktop action tool
- Kind: chat tool
- Reach: console chat or `POST /v1/chat`; ask to call `computer_use(goal, app?, inputs?, max_steps?)`.
- Drive: with cua-driver running and permitted, ask Calculator to clear and calculate 2 + 3. The default backend resolves the `computer_use` role to light; no Jev configuration is needed.
- Healthy result: `status: done`, visible result 5, action trace followed by a final observation, and LLM input/output usage. Risky controls/secure typing return `needs_confirmation`; answer via `resume` and `confirm`.
- Evidence: capture tool JSON and the final Calculator observation. Automated driver/provider boundary coverage: `go test ./internal/tarsserver -run 'TestComputerUse.*EndToEnd' -v`; live tool coverage: `TARS_COMPUTER_USE_LIVE_CONFIG=<config> CUA_DRIVER_PATH=<driver> go test -tags integration ./internal/tarsserver -run TestLiveComputerUseLight -v`.

### Backend configuration and readiness
- Kind: configuration / command
- Reach: `tools.computer_use` in YAML and `tars doctor --config <config>`.
- Drive: check default `enabled: true`, `backend: llm`; set `enabled: false` to disable or `backend: jev` with `jev.base_url` to switch. Set `expose_values: false` to withhold field contents.
- Healthy result: doctor reports the selected LLM tier/model or Jev location and missing driver guidance. Jev keeps confidence gates. Missing setup returns `unavailable`; no computer action runs.
- Evidence: configured-model compatibility without GUI access: `TARS_COMPUTER_USE_MODEL_CONFIG=<config> go test -tags integration ./internal/tarsserver -run TestConfiguredLightRecordedObservation -v` (synthetic screen data only). Doctor output and `go test ./internal/config ./cmd/tars -run 'TestComputerUse|TestCheckDoctorComputerUse' -v`.
