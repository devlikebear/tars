// Package pulse implements the TARS system-surface watchdog.
//
// Pulse is one half of the system surface (the other being reflection).
// Every tick (1 minute by default) it deterministically collects signals
// from cron, gateway, ops, and telegram delivery, and — only when some
// threshold is exceeded — asks an LLM to classify the situation into one
// of three actions: ignore, notify the user, or run a whitelisted autofix.
//
// Pulse is strictly separated from the user surface: its LLM calls may
// only invoke the pulse_decide tool, and its Go runtime directly calls
// internal/ops and related packages rather than going through user-facing
// tool wrappers. Cross-surface leakage is enforced at Registry.Register
// time — see internal/tool.RegistryScope.
package pulse

import (
	"fmt"
	"time"
)

// Severity categorizes the urgency of a pulse signal or decision.
//
// The severity ladder mirrors log levels so it can flow into existing
// notification routing without translation.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarn
	SeverityError
	SeverityCritical
)

// String returns the lowercase severity name used in config, logs, and
// API responses.
func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarn:
		return "warn"
	case SeverityError:
		return "error"
	case SeverityCritical:
		return "critical"
	default:
		return fmt.Sprintf("severity(%d)", int(s))
	}
}

// ParseSeverity parses a lowercase severity string. Unknown strings
// return SeverityInfo and an error — callers should treat unknown values
// as safe defaults rather than failing hard.
func ParseSeverity(s string) (Severity, error) {
	switch s {
	case "info":
		return SeverityInfo, nil
	case "warn", "warning":
		return SeverityWarn, nil
	case "error":
		return SeverityError, nil
	case "critical", "crit":
		return SeverityCritical, nil
	default:
		return SeverityInfo, fmt.Errorf("unknown severity %q", s)
	}
}

// AtLeast returns true if s meets or exceeds the given minimum severity.
// Used by notify routing to filter decisions below the configured floor.
func (s Severity) AtLeast(min Severity) bool {
	return int(s) >= int(min)
}

// SignalKind identifies the domain a signal came from. It is used as a
// tag in the LLM prompt so the decider can reason about signal origin.
type SignalKind string

const (
	SignalKindCronFailures      SignalKind = "cron_failures"
	SignalKindStuckGatewayRun   SignalKind = "stuck_gateway_run"
	SignalKindDiskUsage         SignalKind = "disk_usage"
	SignalKindDeliveryFailures  SignalKind = "delivery_failures"
	SignalKindReflectionFailure SignalKind = "reflection_failure"
)

// Signal is a single observation made by the signal scanner. A pulse tick
// collects zero or more Signals; if none exceed their threshold the LLM
// decider is never invoked.
type Signal struct {
	Kind     SignalKind     `json:"kind"`
	Severity Severity       `json:"severity"`
	Summary  string         `json:"summary"`
	Details  map[string]any `json:"details,omitempty"`
	At       time.Time      `json:"at"`
}

// Action is the category of response pulse may take for a tick.
type Action string

const (
	ActionIgnore  Action = "ignore"
	ActionNotify  Action = "notify"
	ActionAutofix Action = "autofix"
)

// ParseAction validates an action string coming from LLM output.
// Unknown values return an error so the decider can reject malformed
// responses rather than silently ignoring them.
func ParseAction(s string) (Action, error) {
	switch Action(s) {
	case ActionIgnore, ActionNotify, ActionAutofix:
		return Action(s), nil
	default:
		return "", fmt.Errorf("unknown action %q", s)
	}
}

// Decision is the LLM's classification for a tick that exceeded signal
// thresholds. When Action is ActionAutofix, AutofixName must name an
// autofix in the configured whitelist; otherwise the runtime rejects it.
type Decision struct {
	Action      Action         `json:"action"`
	Severity    Severity       `json:"severity"`
	Title       string         `json:"title,omitempty"`
	Summary     string         `json:"summary,omitempty"`
	Details     map[string]any `json:"details,omitempty"`
	AutofixName string         `json:"autofix_name,omitempty"`
}

// TickOutcome captures everything that happened in a single pulse tick,
// suitable for appending to a ring buffer of recent activity.
type TickOutcome struct {
	At              time.Time `json:"at"`
	Skipped         bool      `json:"skipped,omitempty"`
	SkipReason      string    `json:"skip_reason,omitempty"`
	Signals         []Signal  `json:"signals,omitempty"`
	DeciderInvoked  bool      `json:"decider_invoked,omitempty"`
	Decision        *Decision `json:"decision,omitempty"`
	AutofixAttempt  string    `json:"autofix_attempt,omitempty"`
	AutofixOK       bool      `json:"autofix_ok,omitempty"`
	AutofixErr      string    `json:"autofix_err,omitempty"`
	NotifyDelivered bool      `json:"notify_delivered,omitempty"`
	Err             string    `json:"err,omitempty"`
}
