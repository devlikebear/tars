---
name: ops-issue-create
description: "Create or reuse GitHub issues for a concrete ops incident and capture reproduction evidence."
user-invocable: false
requires_plugin: ops-service
requires_bins: [gh, git]
recommended_tools:
  - exec
  - project_activity_append
---

# Ops Issue Create

Use this skill when triage found a real incident that needs tracking.

## Workflow

1. Prefer a matching open issue when the signature, repro steps, and service area align.
2. Otherwise create a new GitHub issue from the cloned repo using `gh issue create`.
3. Include:
   - exact failure signature
   - how to reproduce it with `./opsctl`
   - impact and current phase
   - links or excerpts for the supporting logs
4. Append a project activity entry with the selected issue identifier.

## Rules

- Do not create duplicate issues for the same active incident.
- Keep titles searchable by signature, for example `ops-demo: timeout synthetic probe fails`.
