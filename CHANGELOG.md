# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog and the project follows Semantic Versioning.

## [Unreleased]

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
- Config knobs `gateway_subagents_max_threads` and `gateway_subagents_max_depth`

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
