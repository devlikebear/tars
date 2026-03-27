---
name: ops-fix-implement
description: "Implement a safe repo change for the tracked ops incident using TDD and the local demo repo."
user-invocable: false
requires_plugin: ops-service
requires_bins: [git, gh]
recommended_tools:
  - exec
  - project_activity_append
  - project_autopilot_advance
---

# Ops Fix Implement

Use this skill when an ops incident already has tracking and is safe for autonomous remediation.

## Workflow

1. Work inside `project_artifacts_dir`, which should contain the cloned demo repo.
2. Follow TDD:
   - add or update a failing test
   - implement the smallest fix
   - rerun targeted tests, then broader tests
3. Create or reuse the issue branch before coding.
4. Record the exact test commands and results in the task report or project activity.
5. Stop at a review-ready state if the change affects restart logic, rollout behavior, or incident semantics.

## Rules

- Prefer minimal, reversible changes.
- Do not merge or deploy from this skill.
