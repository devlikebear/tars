// Package reflection implements the nightly batch runner that owns memory
// cleanup and knowledge-base cleanup for the TARS workspace.
//
// Reflection is one half of the system surface (the other being pulse).
// It is called reflection because it runs during a configured sleep window
// and consolidates work that should not block the per-turn chat hot path:
// experience extraction, knowledge compilation, and session hygiene.
//
// Reflection has no LLM tool surface. Its jobs are pure Go functions that
// may call llm.Client.Chat directly for knowledge compilation, but they
// never expose tools to the model. If a future job needs a tool it must
// register it on a tool.Registry constructed with RegistryScopeReflection.
package reflection

import (
	"fmt"
	"time"
)

// JobResult captures the outcome of a single reflection job execution.
// Jobs never panic; any error becomes Err and Success is false. Changed
// is true when the job modified workspace state (so the HTTP view can
// highlight meaningful runs vs. no-op days).
type JobResult struct {
	Name    string         `json:"name"`
	Success bool           `json:"success"`
	Changed bool           `json:"changed,omitempty"`
	Summary string         `json:"summary,omitempty"`
	Details map[string]any `json:"details,omitempty"`
	Err     string         `json:"err,omitempty"`
	// Duration is the wall-clock time the job took to execute. Helpful
	// for spotting creeping regressions in nightly performance.
	Duration time.Duration `json:"duration_ms"`
}

// RunSummary is the aggregate of one reflection run (all jobs for a
// given night). It is stored in state and exposed via HTTP.
type RunSummary struct {
	StartedAt  time.Time   `json:"started_at"`
	FinishedAt time.Time   `json:"finished_at"`
	Results    []JobResult `json:"results"`
	// Success is true only if every job in Results succeeded. One failed
	// job marks the whole run as failed, which is what drives pulse's
	// reflection-failure signal.
	Success bool `json:"success"`
	// Err is set when reflection was unable to even start jobs (e.g.
	// scheduler error). Individual job errors live on JobResult.
	Err string `json:"err,omitempty"`
}

// Severity categorizes a reflection log entry's urgency. Used only for
// exposition in state snapshots; reflection does not act on severity.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarn
	SeverityError
)

func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarn:
		return "warn"
	case SeverityError:
		return "error"
	default:
		return fmt.Sprintf("severity(%d)", int(s))
	}
}
