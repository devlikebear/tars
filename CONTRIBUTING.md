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

- Update [`VERSION.txt`](VERSION.txt) when preparing a release
- Record user-visible changes in [`CHANGELOG.md`](CHANGELOG.md)
- Add migration notes for any breaking change
- Tag releases as `vX.Y.Z`

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
- [ ] No secrets, private keys, or local absolute paths are committed

## Compatibility Notes

- The primary plugin manifest filename is `tars.plugin.json`
- The primary user extension directories are `~/.tars/skills` and `~/.tars/plugins`
- One release of fallback compatibility is kept for legacy pre-publication extension paths and manifest names
