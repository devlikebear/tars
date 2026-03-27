---
name: ops-plan
description: "Plan an ops-service project around log triage, issue creation, safe remediation, and human checkpoints."
user-invocable: true
requires_plugin: ops-service
requires_bins: [docker, gh, git, curl]
recommended_tools:
  - project_create
  - project_update
  - project_get
  - project_state_get
  - project_state_update
  - project_brief_get
  - project_brief_update
  - project_brief_finalize
---

# Ops Plan

Use this skill when the user wants TARS to operate a service continuously instead of building a one-off feature.

## Goals

- Define the monitored service, log triage loop, and escalation policy.
- Capture the GitHub repository, local runtime commands, and human checkpoints.
- Keep the first phase narrow: observability, incident detection, issue creation, then autonomous remediation.

## Workflow

1. Confirm the service repo, runtime commands, and the incident classes that are safe to handle automatically.
2. Finalize the project brief or create/update the project directly when the user already knows the repo and loop shape.
3. Set `workflow_profile` to `ops-service`.
4. Use `workflow_rules` to keep GitHub auth, issue, branch, PR, test, and build gates on unless the user explicitly disables them.
5. Store explicit instructions for:
   - the repo path or clone URL
   - the operational CLI surface (`./opsctl`)
   - when to stop for a human
   - whether Telegram is report-only or approval-capable
6. Stop once the first phase is clearly reviewable and waiting for approval.

## Human Checkpoints

- risky restart or rollback
- unclear root cause
- repeated remediation failure
- PR ready for merge or deployment
