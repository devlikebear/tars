# Contributing to TARS

Thanks for contributing.

This repository is maintained with a small-team workflow. Keep changes focused, tested, and reviewable.

## Scope and Principles

- Keep PRs small and goal-focused.
- Follow existing architecture boundaries:
  - server/runtime logic in `tars`
  - client UX in `cmd/tars`
- Avoid speculative abstractions and unrelated refactors.

## Versioning Policy

TARS uses Semantic Versioning (`MAJOR.MINOR.PATCH`) for releases.

- `MAJOR`: breaking API/behavior changes
- `MINOR`: backward-compatible feature additions
- `PATCH`: backward-compatible fixes/docs/chore

Current stage is pre-`1.0.0`.

- Breaking changes may still happen in minor releases.
- Any behavior/config/API break must include migration notes in `CHANGE.log`.

## Commit Message Policy

Use Conventional-style prefixes:

- `feat:` new functionality
- `fix:` bug fix
- `chore:` maintenance/docs/CI/non-functional updates

Examples:

- `feat(gateway): add browser profile status API`
- `fix(tool): prevent empty exec command retry loop`
- `chore(ci): run security scan on PR synchronize`

## Branch and PR Policy

- Branch naming (recommended):
  - `feat/<short-topic>`
  - `fix/<short-topic>`
  - `chore/<short-topic>`
- One PR should solve one primary problem.
- Update docs/config examples when behavior changes.

## Required Checks Before Merge

Run locally:

```bash
make security-scan
make test
```

CI must pass on:

- push to `main`
- pull request updates (`opened`, `synchronize`, `reopened`, `ready_for_review`)

## Pull Request Checklist

- [ ] Problem and scope are clearly described.
- [ ] Tests added/updated for behavior changes.
- [ ] `make test` passes.
- [ ] `make security-scan` passes.
- [ ] Docs updated (`README*`, `CHANGE.log`, config examples) when needed.
- [ ] No secrets, tokens, private keys, or local absolute paths committed.

## Security and Sensitive Data

Never commit:

- API keys / tokens / passwords
- private keys / certificates
- personal local paths (e.g. `/Users/<name>/...`)

If found in history, rewrite history before publishing.

## Release Notes

Record user-visible changes in `CHANGE.log`.

For breaking changes, include:

- what changed
- impact scope
- migration steps
