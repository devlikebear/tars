---
name: tars-github-flow
description: "Repository-specific GitHub Flow for the TARS project. Use when work in this repository should follow the full delivery lifecycle: plan the task, create a GitHub issue, create a feature branch, implement with TDD, update VERSION.txt plus CHANGELOG.md and README.md for shipped changes, commit and push, open a PR with the repository template, review CI/CD results, merge to main with squash, sync local main, and verify the automated tag and release flow."
---

# TARS GitHub Flow

## Quick Start

- Use this skill only for end-to-end repository work that should land through the standard TARS delivery path.
- Prefer `gh` for issue, PR, review, merge, and release inspection work.
- Read [references/repo-flow.md](references/repo-flow.md) before executing commands.

## Workflow

1. Plan the task and decide whether it should ship as a public release.
2. Create or update a GitHub issue before coding.
3. Create a `feat/`, `fix/`, or `chore/` branch from `main`.
4. Add a failing test first when practical, then implement.
5. Review the diff before committing and keep the change set focused.
6. Update `VERSION.txt`, `CHANGELOG.md`, and `README.md` when the change is meant to ship publicly.
7. Commit with conventional commits and push the branch.
8. Open a PR with the repository template.
9. Wait for `security` and `test`, address review comments, and verify one approval.
10. Merge with `squash`, delete the remote branch, sync local `main`, and confirm automated release results.

## Repo Rules

- Use `feat:`, `fix:`, or `chore:` commit prefixes.
- Keep PRs small, test-proven, and easy to review.
- Treat `VERSION.txt` and `CHANGELOG.md` as a pair for release PRs.
- Expect `main` protection to require a PR, one approval, resolved conversations, `security`, `test`, and squash merge only.
- Do not manually create GitHub releases unless the automated `release-on-version-bump` workflow failed and the user asked for recovery.

## Environment Notes

- If local `.git` writes fail in the Codex environment, use `gh api` or other GitHub API fallbacks for remote branch, commit, PR, and merge operations.
- When using a remote-only fallback, tell the user that local `main` sync must be run in their own shell.

## References

- Read [references/repo-flow.md](references/repo-flow.md) for the exact phase checklist, commands, and release verification steps.
