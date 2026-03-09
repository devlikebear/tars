# Contributing to TARS

Thanks for contributing.

Keep changes focused, test-proven, and easy to review.

## Development Rules

- Follow TDD when practical: add or update a failing test first, then implement
- Keep PRs small and goal-focused
- Preserve existing architecture boundaries
- Avoid speculative abstractions and unrelated cleanup

## Commit Messages

Use these prefixes:

- `feat:` new functionality
- `fix:` behavior or bug fix
- `chore:` maintenance, docs, tooling, or non-functional changes

Examples:

- `feat(runtime): add version command`
- `fix(plugin): prefer primary manifest filename`
- `chore(docs): publish english public docs`

## Versioning and Releases

TARS uses Semantic Versioning: `MAJOR.MINOR.PATCH`.

- `MAJOR`: breaking API or behavior changes
- `MINOR`: backward-compatible features
- `PATCH`: backward-compatible fixes, docs, and maintenance

Release metadata rules:

- Update [`VERSION.txt`](VERSION.txt) and [`CHANGELOG.md`](CHANGELOG.md) together when preparing a release PR
- Record user-visible changes in [`CHANGELOG.md`](CHANGELOG.md)
- Add migration notes for any breaking change
- Tag releases as `vX.Y.Z`
- Merging a release PR to `main` is the release approval event: it creates the tag, publishes the GitHub Release, updates the Homebrew tap, and powers the curl installer

## Required Checks

Run these locally before merging:

```bash
make test
make security-scan
```

## Pull Request Checklist

- [ ] Problem and scope are clearly described
- [ ] Tests added or updated for behavior changes
- [ ] `make test` passes
- [ ] `make security-scan` passes
- [ ] `CHANGELOG.md` and docs are updated when needed
- [ ] Release PRs update `VERSION.txt` and `CHANGELOG.md` together
- [ ] No secrets, private keys, or local absolute paths are committed

## Compatibility Notes

- The primary plugin manifest filename is `tars.plugin.json`
- The primary user extension directories are `~/.tars/skills` and `~/.tars/plugins`
- One release of fallback compatibility is kept for legacy pre-publication extension paths and manifest names

## Release Automation Notes

- `release-on-version-bump` runs only when `VERSION.txt` changes on `main`
- The release workflow validates `CHANGELOG.md`, builds macOS archives, creates or updates the GitHub Release, and then updates the Homebrew tap
- The release workflow fails if `CHANGELOG.md` is not updated in the same push
- GitHub Releases publish macOS `arm64` and `amd64` archives plus `checksums.txt`
- `HOMEBREW_TAP_TOKEN` is required; if it is missing or the tap update fails, the release workflow fails
- `install.sh` installs the latest published GitHub Release by default, or a pinned version when `VERSION=...` is provided
