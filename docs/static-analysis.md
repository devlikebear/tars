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

## SonarCloud Evaluation

`.github/workflows/sonarcloud.yml` is an optional evaluation workflow. It runs
on pull requests, pushes to `main`, and manual dispatch, but it exits
successfully with a notice until the repository has all required configuration:

```bash
gh secret set SONAR_TOKEN --repo devlikebear/tars
gh variable set SONAR_PROJECT_KEY --repo devlikebear/tars --body '<sonar-project-key>'
gh variable set SONAR_ORGANIZATION --repo devlikebear/tars --body '<sonar-organization>'
```

Once configured, the workflow generates Go coverage with `make test-cover` and
uploads it through `sonar.go.coverage.reportPaths=coverage.out`.

Frontend LCOV coverage is intentionally deferred because the console currently
uses Node's built-in test runner and has no stable `lcov.info` producing command.
Svelte-specific correctness remains anchored in `svelte-check` and the stable
frontend CI test slice in `.github/workflows/ci.yml`.

The SonarCloud scan is deliberately non-blocking while the baseline is
evaluated:

- the workflow does not wait for the Sonar quality gate
- the scanner step uses `continue-on-error: true`
- the check should not be configured as a required merge gate until the first
  baseline is reviewed and the quality gate policy is agreed

Run the local guard before changing the workflow:

```bash
make sonarcloud-workflow-check
```
