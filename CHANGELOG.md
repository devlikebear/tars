# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog and the project follows Semantic Versioning.

## [Unreleased]

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
- Homebrew tap formula generation for `devlikebear/homebrew-tars`

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
