# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog and the project follows Semantic Versioning.

## [Unreleased]

## [0.1.1] - 2026-03-10

### Added

- Automated release workflow driven by 	a changes on , including tag/release publishing and Homebrew tap updates
- Public  for curl-based macOS installs from GitHub Releases
- Homebrew tap formula generation for 

### Changed

- Public documentation is maintained in English for the published repository surface
-  now installs the latest published GitHub Release by default
- Release PRs must update  and  together

## [0.1.0] - 2026-03-08

### Added

- Initial public release of the local-first TARS runtime
- Embedded build metadata via , Git commit, and build date
-  and 0.7.0

### Changed

- Primary Go module path is 
- Primary plugin manifest filename is 
- Primary user extension directories use 

### Security

- Repository publishing flow includes ./scripts/security_scan.sh
[security-scan] running gitleaks
[security-scan] checking tracked files for absolute local paths
[security-scan] checking tracked files for private key blocks
[security-scan] passed
- Gitleaks false-positive handling is documented via repository ignore metadata
