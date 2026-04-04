package tarsserver

import (
	"time"

	"github.com/devlikebear/tars/internal/session"
)

// ServeOptions configures Serve API execution without CLI argument parsing.
type ServeOptions struct {
	ConfigPath        string
	Mode              string
	WorkspaceDir      string
	LogFile           string
	Verbose           bool
	RunOnce           bool
	RunLoop           bool
	ServeAPI          bool
	APIAddr           string
	HeartbeatInterval time.Duration
	MaxHeartbeats     int
}

type options struct {
	ConfigPath        string
	Mode              string
	WorkspaceDir      string
	LogFile           string
	Verbose           bool
	RunOnce           bool
	RunLoop           bool
	ServeAPI          bool
	APIAddr           string
	HeartbeatInterval time.Duration
	MaxHeartbeats     int
}

// Exported defaults used by cmd/tars entry wiring.
// Keep this list minimal to avoid growing cmd<->server coupling.
const (
	DefaultAPIAddr           = "127.0.0.1:43180"
	DefaultHeartbeatInterval = 30 * time.Minute
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
- Before answering questions that may relate to prior conversations, decisions, dates, people, preferences, habits, or any topic discussed in past sessions, you MUST call memory_search first.
- Do not guess memory-backed facts without first checking tools.
- When the user references something from a previous conversation (e.g., "that thing we discussed", "last time", "continue", "그거", "아까 그", "전에 말한", "지난번"), call memory_search with include_sessions=true to find the relevant conversation context.
- If memory_search returns relevant prior context, weave it naturally into your response — do not dump raw search results.
- When you discover useful context from memory, briefly acknowledge it (e.g., "Based on our previous conversation...") before continuing.
- Tool-call arguments must be valid JSON.

## Automation Tool Policy
- If the user asks about cron jobs managed by this app, call cron (preferred) or cron_list / cron_get / cron_runs / cron_create / cron_update / cron_delete / cron_run instead of OS commands like crontab.
- If the user asks to create reminders/todos from natural language, prefer schedule_create first.
- If schedule_create fails with a natural parse error, convert the user intent to at:<rfc3339> or every:<duration> and call cron_create as fallback.
- If the user asks about heartbeat status or asks to trigger heartbeat, call heartbeat (preferred) or heartbeat_status / heartbeat_run_once instead of inferring from process or file guesses.

## Runtime Tool Policy
- For async background agent tasks across sessions, use sessions_spawn and sessions_runs.
- For parallel read-only codebase exploration or diff review, prefer subagents_run before manually coordinating multiple sessions.
- For channel or gateway runtime operations, use message / gateway tools when available.
`
