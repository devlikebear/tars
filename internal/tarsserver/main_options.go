package tarsserver

import (
	"github.com/devlikebear/tars/internal/session"
)

// ServeOptions configures Serve API execution without CLI argument parsing.
type ServeOptions struct {
	ConfigPath   string
	Mode         string
	WorkspaceDir string
	LogFile      string
	Verbose      bool
	RunOnce      bool // deprecated — no longer runs heartbeat; pulse is automatic
	RunLoop      bool // deprecated — no longer runs heartbeat; pulse is automatic
	ServeAPI     bool
	APIAddr      string
}

type options struct {
	ConfigPath   string
	Mode         string
	WorkspaceDir string
	LogFile      string
	Verbose      bool
	RunOnce      bool
	RunLoop      bool
	ServeAPI     bool
	APIAddr      string
}

// Exported defaults used by cmd/tars entry wiring.
// Keep this list minimal to avoid growing cmd<->server coupling.
const (
	DefaultAPIAddr = "127.0.0.1:43180"
)

const (
	chatHistoryMaxTokens     = 120000
	autoCompactTriggerTokens = 100000
	autoCompactKeepRecent    = 0
	autoCompactKeepTokens    = session.DefaultKeepRecentTokens
	autoCompactKeepShare     = session.DefaultKeepRecentFraction
)

const memoryToolSystemRule = `
## Memory Tool Policy
- Before answering questions that may relate to prior conversations, decisions, dates, people, preferences, habits, or any topic discussed in past sessions, you MUST call memory(action=search) first.
- Do not guess memory-backed facts without first checking tools.
- When calling memory(action=search), ALWAYS pass include_sessions=true. This searches past chat session transcripts across all sessions, enabling cross-session context recall.
- Keep include_knowledge=false unless the user explicitly asks to inspect or search the knowledge base.
- When the user references something from a previous conversation (e.g., "that thing we discussed", "last time", "continue", "그거", "아까 그", "전에 말한", "지난번"), memory(action=search) is mandatory — do not skip it.
- If memory search returns relevant prior context, weave it naturally into your response — do not dump raw search results.
- When you discover useful context from memory, briefly acknowledge it (e.g., "Based on our previous conversation...") before continuing.
- Use knowledge(action=list|get|upsert|delete) when you need to inspect or manage the long-term wiki-style knowledge base directly.
- When the user introduces themselves, shares personal info (name, preferred language, timezone), or asks to be remembered as someone (e.g. "나는 찰리야", "Call me X", "기억해줘"), use workspace(action=set, scope=workspace, file=USER.md) to update the user profile — NOT memory(action=save).
- Tool-call arguments must be valid JSON.

## Automation Tool Policy
- For cron jobs managed by this app, use cron(action=list|create|update|delete|run|get|runs) instead of OS commands like crontab.
- For reminders/todos from natural language, use cron(action=create) with natural schedule expressions.
- The pulse watchdog and reflection nightly runner live on the system surface — they are not user-callable tools. To inspect them or trigger a run, direct the user to the /console/pulse and /console/reflection pages.

## Task Management Policy
- For complex tasks with 3+ steps, use tasks(action=plan_set) to set a plan goal, then tasks(action=add) to create individual tasks.
- Only ONE task should be in_progress at a time. Mark completed immediately when done.
- When setting a new plan, the previous plan and tasks are automatically archived to memory.
- Use tasks(action=list) to review current progress. Use tasks(action=clear) to reset when done.

## Runtime Tool Policy
- For session management, use session(action=list|history|send|spawn|runs|agents|status).
- For parallel read-only codebase exploration or diff review, prefer subagents_run.
- For channel or gateway runtime operations, use message / gateway tools when available.
`
