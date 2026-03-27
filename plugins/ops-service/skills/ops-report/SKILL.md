---
name: ops-report
description: "Summarize current phase, incident state, issue/PR links, and blockers for a human operator."
user-invocable: false
requires_plugin: ops-service
recommended_tools:
  - project_get
  - project_state_get
  - project_activity_get
  - project_autopilot_advance
  - telegram_send
---

# Ops Report

Use this skill for periodic summaries and blocker notifications.

## Workflow

1. Read the current phase, state, and recent activity.
2. Summarize:
   - service health
   - active incident or absence of one
   - issue and PR status
   - next action
   - explicit blocker or human decision
3. When Telegram context is available and the summary is useful, send the report with `telegram_send`.
4. Keep the human checkpoint obvious: approval needed, blocker cleared, or no action required.

## Rules

- Telegram is report-only in this example. Do not assume a Telegram reply itself is the project approval surface.
