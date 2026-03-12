# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog and the project follows Semantic Versioning.

## [Unreleased]

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
