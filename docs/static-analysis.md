# Static Analysis

TARS uses layered static analysis so pull requests get fast feedback while the
repository also publishes security findings to GitHub code scanning.

## Pull Request Guards

The primary CI workflow keeps the PR path focused on changed code:

- Svelte console type checks and the stable frontend CI test slice
- `make lint-diff` for new Go lint findings since the PR base
- `make test-cover-diff` for changed Go package tests and changed-line coverage
  (80% of changed lines; pushes to `main` also require 60% in total)

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

## Which jobs a change runs

The `changes` job in `ci.yml` and `codeql.yml` runs `scripts/ci_changes.sh`,
which lists the files of the pull request or push through the GitHub API and
sets two outputs:

| Output | `false` when | Jobs skipped |
| --- | --- | --- |
| `code` | every file is documentation: `docs/**` (except `docs/public-api-surface.txt`), a top-level `*.md`, `LICENSE` | `windows-build`, `windows-test`, `pr-diff`, `test`, and CodeQL on pull requests |
| `desktop` | nothing under `desktop/` changed, nor the `Makefile`, `scripts/desktop_package.sh`, this script or `ci.yml` | `desktop`, `desktop-macos` |

`format` and `security` always run. Jobs skip only on an explicit `false`: if
the script fails or cannot list the files, every job runs.

The skipping is per job, not a workflow `paths` filter. A required check whose
workflow never starts stays pending and blocks the merge; a skipped job counts
as passed. CodeQL's three required checks come from a matrix, and a matrix job
skipped at job level never reports those names, so that job always starts and
skips its steps instead.

Markdown below the top level counts as code, because skills and prompts are
Markdown files that tests read. Run the classifier's tests after changing it:

```bash
make ci-changes-test
```

## Removed: SonarCloud and Codecov

Both were removed in [#1206](https://github.com/devlikebear/tars/issues/1206).
SonarCloud ran in a non-blocking evaluation mode: its quality gate on `main`
was failing and nothing acted on it, while the workflow ran the whole Go test
suite again on every pull request and push. Codecov uploaded on `main` only
and could not fail a build. Coverage is gated by `make test-cover-diff` and
`make test-cover-check`, and security findings come from CodeQL.

The two reviews under `docs/static-analysis/` are kept as a record of what the
first SonarCloud baseline found.

A check either blocks the merge or does not run. Do not add a reporting-only
check back.
