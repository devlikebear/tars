package apptool

import "github.com/devlikebear/tars/internal/tool"

// Aliases for the core tool primitives this package builds on.
//
// They exist so the split that created this package is a pure file move: the
// moved sources are byte-identical to their internal/tool originals apart
// from the package clause. That matters twice over — it makes "no tool name,
// description, or schema changed" verifiable by reading the diff rather than
// by trusting a summary, and it keeps a mechanical relocation from reading as
// thousands of lines of new code to changed-line coverage.
//
// Prefer tool.X directly in new code here; these are for the moved files.
type (
	Tool          = tool.Tool
	Result        = tool.Result
	SessionStatus = tool.SessionStatus
)

// Function aliases. Go has no func-alias syntax, so these are variables —
// which is fine, since nothing reassigns them.
var (
	JSONTextResult       = tool.JSONTextResult
	CanonicalToolName    = tool.CanonicalToolName
	NewSessionStatusTool = tool.NewSessionStatusTool

	// dispatchAction and aggregatorError were unexported helpers shared by
	// aggregators that now live on both sides of the split, so core exports
	// them. The lowercase names here keep the moved aggregators unchanged.
	dispatchAction  = tool.DispatchAction
	aggregatorError = tool.AggregatorError
)
