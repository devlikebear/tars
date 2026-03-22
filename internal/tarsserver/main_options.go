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
- If the user asks about past facts, preferences, prior chat context, or "what you remember", you must call memory_search and/or memory_get before answering.
- Do not guess memory-backed facts without first checking tools.
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
