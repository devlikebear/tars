# Decision: Keep Approvals Workflow

Status: accepted
Issue: CON-041 / #443
Date: 2026-05-01

## Context

The current Approvals surface is intentionally narrow. A manual cleanup plan can be created from the console or CLI, reviewed by a human, and then applied through the same ops API. Pulse autofix currently bypasses this queue for the small set of deterministic low-risk actions in its allowlist. Session-scoped `auto_continue_chat` is also allowlisted, but only after explicit session consent and only for bounded stalled-chat resume modes.

That makes Approvals look quiet today, but it is still the right boundary for destructive or high-blast-radius operations that should not run as fully automatic autofix.

## Decision

Keep Approvals as the human review queue.

Do not remove the following surfaces:

- `internal/ops` approval storage and cleanup plan review flow
- `/v1/ops/approvals` and related approve/reject/apply endpoints
- Console Approvals route and "New cleanup plan" trigger
- CLI `tars approve` commands

## Routing Policy

- Safe deterministic maintenance stays in Pulse autofix when the operation is explicitly allowlisted and bounded.
- Session auto-resume stays in Pulse autofix only while consent, high-risk-question blocking, and repeated-resume escalation gates pass.
- Risky or broad mutations should create an approval queue item instead of running as autofix.
- Manual cleanup plan remains the seed workflow and the compatibility baseline.
- Future approval item types should reuse the queue semantics instead of creating one-off confirmation stores.

Examples that should route to the approval queue:

- Bulk deletion beyond the stale tmp/log cleanup allowlist
- Permission or launch agent mutations that change persistent host state
- Large workspace or session pruning where preview and human review are useful
- External side effects that cannot be cleanly undone

## Consequences

- CON-025 / CON-026 style UI work should strengthen Approvals instead of removing it.
- Pulse follow-up work can add severity or risk routing from signal to autofix versus approval queue.
- Ops approval storage should remain reliable and should continue to receive durability fixes when code review finds them.
- Deprecation notes for Approvals are not needed unless a later decision supersedes this one.
