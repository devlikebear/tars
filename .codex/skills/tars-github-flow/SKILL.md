---
name: tars-github-flow
description: "Repository-specific GitHub Flow for the TARS project. Use when work in this repository should follow the full delivery lifecycle: plan the task, create a GitHub issue, create a dedicated worktree-backed feature branch, implement with TDD, update VERSION.txt plus CHANGELOG.md and README.md for shipped changes, commit and push, open a PR with the repository template, review CI/CD results, merge to main with squash, sync local main, and verify the automated tag and release flow."
---

# TARS GitHub Flow

## Quick Start

- Use this skill only for end-to-end repository work that should land through the standard TARS delivery path.
- Prefer `gh` for issue, PR, review, merge, and release inspection work.
- Default to a dedicated git worktree per feature branch: create the worktree from `main`, do the implementation inside that worktree, confirm `git status --short --branch` is clean there, then push that branch and open the PR.
- Read [references/repo-flow.md](references/repo-flow.md) before executing commands.

## Workflow

1. Plan the task and decide whether it should ship as a public release.
2. Create or update a GitHub issue before coding.
3. Create a dedicated worktree for a `feat/`, `fix/`, or `chore/` branch from `main`.
4. Add a failing test first when practical, then implement.
5. Review the diff before committing and keep the change set focused.
6. Update `VERSION.txt`, `CHANGELOG.md`, and `README.md` when the change is meant to ship publicly.
7. Commit with conventional commits inside the worktree-backed feature branch, then confirm that branch is clean with `git status --short --branch`.
8. Push that clean worktree-backed feature branch and only then open a PR with the repository template.
9. Wait for `security` and `test`, address review comments, and verify one approval.
10. Merge with `squash`, delete the remote branch, sync local `main`, and confirm automated release results.

## Repo Rules

- Use `feat:`, `fix:`, or `chore:` commit prefixes.
- Keep PRs small, test-proven, and easy to review.
- Do not open a PR from uncommitted local work or from a dirty feature branch worktree.
- Treat `VERSION.txt` and `CHANGELOG.md` as a pair for release PRs.
- Expect `main` protection to require a PR, one approval, resolved conversations, `security`, `test`, and squash merge only.
- Do not manually create GitHub releases unless the automated `release-on-version-bump` workflow failed and the user asked for recovery.

## Environment Notes

- Remote-only GitHub API fallbacks are the exception, not the default.
- Use a remote-only fallback only when local `.git` writes are truly blocked and the user understands that the normal requirement is still: local worktree-backed feature branch commit first, clean `git status`, then push and PR.
- When a remote-only fallback is unavoidable, tell the user exactly what did not happen locally and what local sync they still need to run in their own shell.

## References

- Read [references/repo-flow.md](references/repo-flow.md) for the exact phase checklist, commands, and release verification steps.
