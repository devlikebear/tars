---
name: ops-pr-open
description: "Open a GitHub PR for the current ops fix branch and summarize operational impact."
user-invocable: false
requires_plugin: ops-service
requires_bins: [gh, git]
recommended_tools:
  - exec
  - project_activity_append
---

# Ops PR Open

Use this skill when the implementation branch is ready for review.

## Workflow

1. Confirm tests/build commands passed.
2. Open or update the PR from the cloned repo with `gh pr create` or `gh pr edit`.
3. Summarize:
   - incident signature
   - root cause
   - fix scope
   - how the service should be verified after merge
4. Append project activity with the PR URL or identifier.

## Rules

- Stop after the PR is open and the human merge checkpoint is clear.
