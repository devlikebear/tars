package skill

import internal "github.com/devlikebear/tars/internal/skill"

type Source = internal.Source

const (
	SourceWorkspace  = internal.SourceWorkspace
	SourceUser       = internal.SourceUser
	SourceBundled    = internal.SourceBundled
	SourceSessionCwd = internal.SourceSessionCwd
)

type Definition = internal.Definition
type Diagnostic = internal.Diagnostic
type Snapshot = internal.Snapshot
type SourceDir = internal.SourceDir
type AvailabilityOptions = internal.AvailabilityOptions
type LoadOptions = internal.LoadOptions
type Frontmatter = internal.Frontmatter
type PromoteMode = internal.PromoteMode
type PromoteConflictPolicy = internal.PromoteConflictPolicy
type PromoteAction = internal.PromoteAction
type PromoteRequest = internal.PromoteRequest
type PromoteResult = internal.PromoteResult

const (
	PromoteModeCopy            = internal.PromoteModeCopy
	PromoteModeMove            = internal.PromoteModeMove
	PromoteOnConflictRename    = internal.PromoteOnConflictRename
	PromoteOnConflictOverwrite = internal.PromoteOnConflictOverwrite
	PromoteOnConflictAbort     = internal.PromoteOnConflictAbort
	PromoteActionCreated       = internal.PromoteActionCreated
	PromoteActionOverwritten   = internal.PromoteActionOverwritten
	PromoteActionRenamed       = internal.PromoteActionRenamed
)

var ErrPromoteConflict = internal.ErrPromoteConflict

func Load(opts LoadOptions) (Snapshot, error) { return internal.Load(opts) }

func LoadCommands(dir string) ([]Definition, []Diagnostic) {
	return internal.LoadCommands(dir)
}

func LoadCommandAliases(dir string, available []Definition) ([]Definition, []Diagnostic) {
	return internal.LoadCommandAliases(dir, available)
}

func ParseFrontmatter(raw string) (Frontmatter, string, error) {
	return internal.ParseFrontmatter(raw)
}

func FormatAvailableSkills(skills []Definition) string {
	return internal.FormatAvailableSkills(skills)
}

func Promote(req PromoteRequest) (PromoteResult, error) {
	return internal.Promote(req)
}
