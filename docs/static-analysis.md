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

### Path barriers

`go/path-injection` reports every filesystem call whose path can be traced to
a request or a tool argument. CodeQL understands a few standard-library checks
written inline (`filepath.IsLocal`, `strings.HasPrefix`, a `..` test, a regexp
match), but it does not follow TARS's own resolvers, and a containment check
written with `filepath.Rel` is not one it recognizes. Before the model pack,
every file tool was reported, and 65 alerts had been dismissed by hand.

`.github/codeql/extensions/tars-go/` is a CodeQL model pack that names the
functions whose result is a confined path. Code scanning loads a pack in that
folder without any workflow setting. Its 5 rows closed 16 of the 41 open
`go/path-injection` alerts (#1206).

A function listed there is trusted by the scanner from then on, so the bar is:

- it rejects `..` and absolute paths outside its root,
- it checks again after resolving symlinks, and
- a test proves both, including the symlink case.

`resolvePathWithPolicy` did not meet the second point until #1212, which is
how that bug was found. Read the function before adding a row for it.

Two things to know when checking a change to the pack:

- **A pull request scan cannot show the effect.** Pull requests are analyzed
  diff-informed: only alerts on changed lines are reported, so every pull
  request shows 0 results. Compare full scans instead — push the branch with a
  temporary `push` trigger for it in `codeql.yml`, then
  `gh api 'repos/devlikebear/tars/code-scanning/alerts?state=open&ref=refs/heads/<branch>'`.
- **A wrong row fails silently.** A misspelled package, function or kind is
  not an error; the alerts just stay.

What stays open after the pack is one of two kinds, and neither is modeled:
a check inside a resolver while it is still validating (the `os.Lstat` in
`resolveWorkspaceWritePath`), and a path whose "user-controlled" part is a
folder the person chose for the session, which is the feature and not a leak.

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
