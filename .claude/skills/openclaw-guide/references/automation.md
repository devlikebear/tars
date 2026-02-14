# OpenClaw Automation Reference (Heartbeat, Cron, Hooks)

> Source: `https://github.com/openclaw/openclaw`

## Table of Contents

- [Heartbeat System](#heartbeat-system)
- [Cron Job System](#cron-job-system)
- [Cron vs Heartbeat Selection](#cron-vs-heartbeat-selection)
- [Hooks System](#hooks-system)

---

## Heartbeat System

**OpenClaw doc**: `docs/gateway/heartbeat.md`

### Concept (AI First)
- `HEARTBEAT.md` is a natural language instruction document (checklist)
- Every heartbeat: read HEARTBEAT.md → agent loop with tools → AI autonomously judges and executes
- Agent has full tool access during heartbeat (exec, web, memory, etc.)

### HEARTBEAT.md Example
```markdown
# Heartbeat Checklist

## Every Run
- [ ] Check for new emails and summarize important ones
- [ ] Review calendar for upcoming meetings

## Daily (Morning)
- [ ] Compile daily news digest on AI topics
- [ ] Check project status on GitHub

## Weekly (Monday)
- [ ] Generate weekly activity summary
- [ ] Review and update MEMORY.md
```

### Response Contract
- **`HEARTBEAT_OK`**: response starts/ends with this → ack, skip daily log recording
- **No `HEARTBEAT_OK`**: response has actionable content → record to daily log
- Purpose: avoid cluttering logs with "nothing to do" entries

### Active Hours
- `activeHours` config: e.g., `"09:00-18:00"`
- Heartbeats outside active hours are skipped
- Prevents unnecessary LLM calls during off-hours

### Configuration
```yaml
heartbeat:
  interval: 30m
  active_hours: "09:00-18:00"
  max_heartbeats: 0  # 0 = unlimited
```

**TARS mapping**: `internal/heartbeat/`

---

## Cron Job System

**OpenClaw doc**: `docs/automation/cron-jobs.md`

### Schedule Types
| Type | Example | Description |
|------|---------|-------------|
| `at` | `"2026-02-14T09:00:00Z"` | One-shot execution at specific time |
| `every` | `"30m"`, `"2h"`, `"1d"` | Interval-based repetition |
| `cron` | `"0 9 * * *"` | Standard cron expression |

### Job Structure
```json
{
  "id": "job_001",
  "name": "Morning News Digest",
  "schedule": {"type": "cron", "expression": "0 9 * * *"},
  "prompt": "Search for the latest AI news and compile a digest",
  "session": "isolated",
  "agent_id": "main",
  "enabled": true,
  "delete_after_run": false
}
```

### Execution (AI First)
1. Schedule triggers → job's `prompt` sent to agent loop
2. AI receives prompt + full tool access
3. AI autonomously decides which tools to use
4. Results recorded to daily log
5. `session: "isolated"` → separate session (no impact on main)
6. `session: "main"` → injected as system event into main session heartbeat

### Storage
```
{workspace}/cron/
  jobs.json    # array of Job objects
```

### API
```
GET    /v1/cron/jobs           # List jobs
POST   /v1/cron/jobs           # Create job
PUT    /v1/cron/jobs/{id}      # Update job
DELETE /v1/cron/jobs/{id}      # Delete job
POST   /v1/cron/jobs/{id}/run  # Trigger immediate execution
```

**TARS mapping**: `internal/cron/`

---

## Cron vs Heartbeat Selection

**OpenClaw doc**: `docs/automation/cron-vs-heartbeat.md`

| Criteria | Heartbeat | Cron Job |
|----------|-----------|----------|
| Trigger | Periodic interval (e.g., every 30min) | Specific time/schedule |
| Instructions | HEARTBEAT.md (shared checklist) | Per-job prompt (isolated) |
| Session | Main session context | Main or isolated session |
| Use case | Ongoing monitoring, status checks | Scheduled tasks, one-time actions |
| Overhead | Every interval, even if nothing to do | Only at scheduled times |
| Examples | "Check emails", "Monitor system" | "9am daily news", "Weekly report" |

### When to Use Heartbeat
- Continuous monitoring tasks
- Tasks that depend on current context
- High-frequency checks (every few minutes)

### When to Use Cron
- Tasks at specific times
- Tasks that should run independently
- One-shot tasks (schedule + delete)
- Tasks with isolated sessions (no context contamination)

---

## Hooks System

**OpenClaw doc**: `docs/automation/hooks.md`

### Concept
- Event-driven automation triggered by agent actions
- Hooks fire on specific events (tool execution, message, session events)
- Lightweight alternative to full cron jobs

### Event Types
- `on_tool_call` — before/after a tool executes
- `on_message` — when a new message is added
- `on_session_create` — when a new session starts
- `on_heartbeat` — on each heartbeat cycle

### Hook Configuration
```yaml
hooks:
  - event: on_tool_call
    tool: exec
    action: log_command
  - event: on_message
    pattern: "URGENT"
    action: notify_user
```

**TARS mapping**: Future consideration (not in initial phases)
