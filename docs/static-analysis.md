# Static Analysis

TARS uses layered static analysis so pull requests get fast feedback while the
repository also publishes security findings to GitHub code scanning.

## Pull Request Guards

The primary CI workflow keeps the PR path focused on changed code:

- Svelte console type checks and the stable frontend CI test slice
- `make lint-diff` for new Go lint findings since the PR base
- `make test-cover-diff` for changed Go package tests and changed-line coverage

This avoids duplicating the full push-only coverage workflow on every PR while
still blocking regressions introduced by the branch.

## CodeQL

`.github/workflows/codeql.yml` runs on pull requests, pushes to `main`, and a
weekly scheduled scan. It analyzes:

- Go (`go`)
- Svelte/TypeScript/JavaScript (`javascript-typescript`)
- GitHub Actions workflow definitions (`actions`)

The workflow grants `security-events: write` so CodeQL can publish alerts to
GitHub code scanning. It uses CodeQL's autobuild only for Go and `build-mode:
none` for JavaScript/TypeScript and workflow analysis, keeping it separate from
the existing test and coverage jobs.

Run the local guard before changing the workflow:

```bash
make codeql-workflow-check
```
