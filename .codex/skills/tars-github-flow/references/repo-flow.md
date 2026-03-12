# TARS Repo Flow

## Use This Flow For

- Feature, fix, or chore work that should go through the full repository lifecycle
- Changes that need issue tracking, PR review, CI checks, and optional release verification
- Publicly shipped work that updates `VERSION.txt`, `CHANGELOG.md`, and `README.md`

## Phase 1: Plan

- Start with a bounded plan before coding.
- Split the work into at most three work orders when the task is non-trivial.
- Call out whether the task is expected to ship as part of the next public release.

## Phase 2: Create or Update a GitHub Issue

- Prefer one issue per logical change.
- Use `gh issue create` when no issue exists yet.
- Keep the issue body short: problem, scope, acceptance criteria.

Example:

```bash
gh issue create \
  --title "Add onboarding and service lifecycle commands" \
  --body $'## Summary\n- Add init/doctor/service flow\n\n## Acceptance Criteria\n- `tars init` creates starter files\n- `tars doctor --fix` repairs local setup\n- `tars service ...` manages launchd'
```

## Phase 3: Create a Feature Branch

- Branch from `main`.
- Use `feat/<name>`, `fix/<name>`, or `chore/<name>`.
- Keep names lowercase kebab-case.

Example:

```bash
git fetch origin
git switch main
git pull --rebase
git switch -c feat/onboarding-service-flow
```

Fallback when `.git` writes are blocked in the Codex environment:

- Create the remote branch with `gh api repos/<owner>/<repo>/git/refs`.
- Commit through the GitHub Git Data API when needed.
- Tell the user that local branch creation and local main sync still need to happen in their own shell.

## Phase 4: Develop With TDD

- Add or update a failing test first when practical.
- Implement the smallest change that makes the test pass.
- Re-run targeted tests before widening to broader validation.
- Review the diff before committing. Focus on regressions, missing tests, and release/documentation gaps.

Repository expectations:

- Follow `feat:`, `fix:`, `chore:` commit prefixes.
- Stage specific files only.
- Do not use `--no-verify` unless the user explicitly approves it.

## Phase 5: Prepare Release Metadata and Docs

- For shipped public changes, update `VERSION.txt`.
- Update `CHANGELOG.md` with a dated release entry and user-visible notes.
- Update `README.md` when the public workflow, install flow, or user commands change.
- Keep `VERSION.txt` and `CHANGELOG.md` together in the same release PR.

Current repository release rules:

- TARS uses SemVer.
- Merge of a release PR to `main` is the release approval event.
- `release-on-version-bump` validates `CHANGELOG.md`, publishes GitHub Release assets, and updates the Homebrew tap.

## Phase 6: Commit and Push

- Keep the commit subject imperative and under 72 characters.
- Prefer one commit per logical unit when possible.

Example:

```bash
git add cmd/tars/main.go cmd/tars/init_main.go cmd/tars/init_main_test.go
git commit -m "feat: add onboarding and service lifecycle commands"
git push -u origin feat/onboarding-service-flow
```

## Phase 7: Open a Pull Request

- Use the repository PR template at `.github/pull_request_template.md`.
- Fill all sections: `Summary`, `Changes`, `Validation`, `Checklist`, `Risks / Rollback`.
- Explicitly document any local-only test failure that is unrelated to the change.

Required checks and merge rules on `main`:

- `security` must pass.
- `test` must pass.
- One approval is required.
- Review threads must be resolved.
- Squash merge only.

Example:

```bash
gh pr create --base main --head feat/onboarding-service-flow --title "feat: add onboarding and service lifecycle commands"
```

## Phase 8: Review, CI/CD, and Merge

- Inspect PR checks with `gh pr view` or `gh run list`.
- Address review comments before merging.
- Merge only after the rules above are satisfied, unless an authorized bypass is intentionally used.

Example:

```bash
gh pr view <number> --json mergeable,mergeStateStatus,reviewDecision,statusCheckRollup
gh pr merge <number> --squash --delete-branch
```

## Phase 9: Sync Local Main

- After merge, sync local `main`.

Preferred:

```bash
git fetch origin
git switch main
git pull --rebase
```

If remote-only fallback was used because `.git` writes were blocked:

- Tell the user to run the commands above in their own shell.

## Phase 10: Tag and Release Verification

- Do not manually create a tag or release for standard TARS release PRs.
- Instead, verify that the automated workflow completed.

Verify:

```bash
gh run list --workflow release-on-version-bump.yml --limit 5
gh release view v<version> --repo devlikebear/tars
gh api repos/devlikebear/tars/git/ref/tags/v<version>
gh api 'repos/devlikebear/homebrew-tars/contents/Formula/tars.rb?ref=main'
```

- Confirm the tag points to the merge commit.
- Confirm the release has both macOS archives and `checksums.txt`.
- Confirm `homebrew-tars` points to the same version and checksums.

## Completion

- Report the issue URL, PR URL, merge commit, and release URL when applicable.
- Call out anything the user still needs to do locally, especially when local `.git` writes were blocked.
