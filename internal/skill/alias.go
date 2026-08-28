// Package skill is a compatibility shim.
//
// The implementation moved to pkg/skill so that the public API owns its own
// types and renders on pkg.go.dev; see #928. This package aliases that
// surface so no in-repo call site had to change in the same commit as the
// move.
//
// New in-repo code should import github.com/devlikebear/tars/pkg/skill
// directly.
package skill

import pkgskill "github.com/devlikebear/tars/pkg/skill"

type (
	AvailabilityOptions            = pkgskill.AvailabilityOptions
	Definition                     = pkgskill.Definition
	Diagnostic                     = pkgskill.Diagnostic
	ExtractionCandidate            = pkgskill.ExtractionCandidate
	ExtractionCandidateAction      = pkgskill.ExtractionCandidateAction
	ExtractionCandidateListOptions = pkgskill.ExtractionCandidateListOptions
	ExtractionCandidateReview      = pkgskill.ExtractionCandidateReview
	ExtractionCandidateStatus      = pkgskill.ExtractionCandidateStatus
	ExtractionEvidence             = pkgskill.ExtractionEvidence
	ExtractionOptions              = pkgskill.ExtractionOptions
	ExtractionProvenance           = pkgskill.ExtractionProvenance
	ExtractionSignal               = pkgskill.ExtractionSignal
	ExtractionSignalKind           = pkgskill.ExtractionSignalKind
	Frontmatter                    = pkgskill.Frontmatter
	LoadOptions                    = pkgskill.LoadOptions
	PromoteAction                  = pkgskill.PromoteAction
	PromoteConflictPolicy          = pkgskill.PromoteConflictPolicy
	PromoteMode                    = pkgskill.PromoteMode
	PromoteRequest                 = pkgskill.PromoteRequest
	PromoteResult                  = pkgskill.PromoteResult
	Snapshot                       = pkgskill.Snapshot
	Source                         = pkgskill.Source
	SourceDir                      = pkgskill.SourceDir
)

const (
	ExtractionCandidateActionApprove  = pkgskill.ExtractionCandidateActionApprove
	ExtractionCandidateActionEvaluate = pkgskill.ExtractionCandidateActionEvaluate
	ExtractionCandidateActionPromote  = pkgskill.ExtractionCandidateActionPromote
	ExtractionCandidateActionReject   = pkgskill.ExtractionCandidateActionReject
	ExtractionCandidateActionRollback = pkgskill.ExtractionCandidateActionRollback
	ExtractionCandidateStatusApproved = pkgskill.ExtractionCandidateStatusApproved
	ExtractionCandidateStatusPending  = pkgskill.ExtractionCandidateStatusPending
	ExtractionCandidateStatusRejected = pkgskill.ExtractionCandidateStatusRejected
	ExtractionSignalDeadEnd           = pkgskill.ExtractionSignalDeadEnd
	ExtractionSignalFailure           = pkgskill.ExtractionSignalFailure
	ExtractionSignalSuccess           = pkgskill.ExtractionSignalSuccess
	ExtractionSignalUserCorrection    = pkgskill.ExtractionSignalUserCorrection
	PromoteActionCreated              = pkgskill.PromoteActionCreated
	PromoteActionOverwritten          = pkgskill.PromoteActionOverwritten
	PromoteActionRenamed              = pkgskill.PromoteActionRenamed
	PromoteModeCopy                   = pkgskill.PromoteModeCopy
	PromoteModeMove                   = pkgskill.PromoteModeMove
	PromoteOnConflictAbort            = pkgskill.PromoteOnConflictAbort
	PromoteOnConflictOverwrite        = pkgskill.PromoteOnConflictOverwrite
	PromoteOnConflictRename           = pkgskill.PromoteOnConflictRename
	SourceBundled                     = pkgskill.SourceBundled
	SourceSessionCwd                  = pkgskill.SourceSessionCwd
	SourceUser                        = pkgskill.SourceUser
	SourceWorkspace                   = pkgskill.SourceWorkspace
)

var (
	AppendExtractionCandidatesIfNew = pkgskill.AppendExtractionCandidatesIfNew
	DetectExtractionCandidates      = pkgskill.DetectExtractionCandidates
	DetectExtractionSignals         = pkgskill.DetectExtractionSignals
	ErrPromoteConflict              = pkgskill.ErrPromoteConflict
	ExtractionInboxPath             = pkgskill.ExtractionInboxPath
	FindExtractionCandidate         = pkgskill.FindExtractionCandidate
	FormatAvailableSkills           = pkgskill.FormatAvailableSkills
	ListExtractionCandidates        = pkgskill.ListExtractionCandidates
	Load                            = pkgskill.Load
	LoadCommandAliases              = pkgskill.LoadCommandAliases
	LoadCommands                    = pkgskill.LoadCommands
	MirrorToWorkspace               = pkgskill.MirrorToWorkspace
	ParseFrontmatter                = pkgskill.ParseFrontmatter
	Promote                         = pkgskill.Promote
	ReviewExtractionCandidate       = pkgskill.ReviewExtractionCandidate
)
