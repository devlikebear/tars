---
name: ops-log-triage
description: "Inspect the demo service through opsctl, classify incidents, and decide whether to create or reuse GitHub tracking."
user-invocable: false
requires_plugin: ops-service
requires_bins: [docker, gh, curl]
recommended_tools:
  - exec
  - project_get
  - project_state_get
  - project_activity_get
  - project_activity_append
  - web_search
---

# Ops Log Triage

Use this skill when a cron run or a user asks TARS to inspect operational health.

## Workflow

1. Read the active project and current state.
2. Use `exec` inside `project_artifacts_dir` to run:
   - `./opsctl status`
   - `./opsctl health`
   - `./opsctl errors`
3. Group failures by signature and severity.
4. If the service is healthy and no new error signature exists:
   - append a short activity entry
   - stop
5. If an incident exists:
   - gather the most recent evidence from logs
   - search for a matching open issue or PR with `gh`
   - reuse existing tracking when the signature clearly matches
6. Escalate instead of proceeding when the failure is ambiguous, high-risk, or requires production credentials beyond the local demo repo.

## Rules

- Prefer the operational CLI over ad-hoc Docker commands when both can answer the question.
- Keep the incident summary short, evidence-backed, and reproducible.
- Do not restart the service just to hide evidence before issue creation.
