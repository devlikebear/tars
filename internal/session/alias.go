// Package session is a compatibility shim.
//
// The implementation moved to pkg/session so that the public API owns its own
// types and renders on pkg.go.dev; see #928. This package aliases that
// surface so no in-repo call site had to change in the same commit as the
// move.
//
// New in-repo code should import github.com/devlikebear/tars/pkg/session
// directly.
package session

import pkgsession "github.com/devlikebear/tars/pkg/session"

type (
	CompactOptions           = pkgsession.CompactOptions
	CompactResult            = pkgsession.CompactResult
	CompactionSummaryOptions = pkgsession.CompactionSummaryOptions
	ForkOptions              = pkgsession.ForkOptions
	ForkPromotionCandidate   = pkgsession.ForkPromotionCandidate
	ForkPromotionOptions     = pkgsession.ForkPromotionOptions
	HistorySnapshot          = pkgsession.HistorySnapshot
	Message                  = pkgsession.Message
	PendingCriticFeedback    = pkgsession.PendingCriticFeedback
	Plan                     = pkgsession.Plan
	ReasoningBlock           = pkgsession.ReasoningBlock
	Session                  = pkgsession.Session
	SessionAutomationConsent = pkgsession.SessionAutomationConsent
	SessionCritic            = pkgsession.SessionCritic
	SessionGoal              = pkgsession.SessionGoal
	SessionStyleControl      = pkgsession.SessionStyleControl
	SessionTasks             = pkgsession.SessionTasks
	SessionToolConfig        = pkgsession.SessionToolConfig
	SessionWithPlanTasks     = pkgsession.SessionWithPlanTasks
	Store                    = pkgsession.Store
	Task                     = pkgsession.Task
	TaskContract             = pkgsession.TaskContract
	TaskEvidence             = pkgsession.TaskEvidence
	TaskProofPolicy          = pkgsession.TaskProofPolicy
)

const (
	AutoContinueIterationWindow              = pkgsession.AutoContinueIterationWindow
	AutoContinueIterationsHardCap            = pkgsession.AutoContinueIterationsHardCap
	AutoResumeModeMoveToNextTask             = pkgsession.AutoResumeModeMoveToNextTask
	AutoResumeModeProceedWithAssumption      = pkgsession.AutoResumeModeProceedWithAssumption
	AutoResumeModeRecordAssumptionAndProceed = pkgsession.AutoResumeModeRecordAssumptionAndProceed
	ContractStatusApproved                   = pkgsession.ContractStatusApproved
	ContractStatusDraft                      = pkgsession.ContractStatusDraft
	DefaultAutoContinueMaxIterations         = pkgsession.DefaultAutoContinueMaxIterations
	DefaultAutoResumeAfterMinutes            = pkgsession.DefaultAutoResumeAfterMinutes
	DefaultCriticMaxIterations               = pkgsession.DefaultCriticMaxIterations
	DefaultGoalMaxAutoContinues              = pkgsession.DefaultGoalMaxAutoContinues
	DefaultKeepRecentFraction                = pkgsession.DefaultKeepRecentFraction
	DefaultKeepRecentMessages                = pkgsession.DefaultKeepRecentMessages
	DefaultKeepRecentTokens                  = pkgsession.DefaultKeepRecentTokens
	EvidenceTypeCommandOutputSummary         = pkgsession.EvidenceTypeCommandOutputSummary
	EvidenceTypeImage                        = pkgsession.EvidenceTypeImage
	EvidenceTypeLogExcerpt                   = pkgsession.EvidenceTypeLogExcerpt
	EvidenceTypePRLink                       = pkgsession.EvidenceTypePRLink
	EvidenceTypeReleaseTag                   = pkgsession.EvidenceTypeReleaseTag
	EvidenceTypeTestResult                   = pkgsession.EvidenceTypeTestResult
	MaxCriticFeedbackLen                     = pkgsession.MaxCriticFeedbackLen
	MaxCriticMaxIterations                   = pkgsession.MaxCriticMaxIterations
	MaxGoalDescriptionLen                    = pkgsession.MaxGoalDescriptionLen
	MaxGoalMaxAutoContinues                  = pkgsession.MaxGoalMaxAutoContinues
	MaxKeepRecentFraction                    = pkgsession.MaxKeepRecentFraction
	MaxKeepRecentMessages                    = pkgsession.MaxKeepRecentMessages
	MaxKeepRecentTokens                      = pkgsession.MaxKeepRecentTokens
	MinKeepRecentFraction                    = pkgsession.MinKeepRecentFraction
	MinKeepRecentMessages                    = pkgsession.MinKeepRecentMessages
	MinKeepRecentTokens                      = pkgsession.MinKeepRecentTokens
	PlanStatusAborted                        = pkgsession.PlanStatusAborted
	PlanStatusCompleted                      = pkgsession.PlanStatusCompleted
	PlanStatusDrafting                       = pkgsession.PlanStatusDrafting
	PlanStatusExecuting                      = pkgsession.PlanStatusExecuting
	PlanStatusPaused                         = pkgsession.PlanStatusPaused
	PlanStatusProposed                       = pkgsession.PlanStatusProposed
	SessionCriticStatusExhausted             = pkgsession.SessionCriticStatusExhausted
	SessionCriticStatusIdle                  = pkgsession.SessionCriticStatusIdle
	SessionCriticStatusReviewing             = pkgsession.SessionCriticStatusReviewing
	SessionCriticStatusSatisfied             = pkgsession.SessionCriticStatusSatisfied
	SessionGoalStatusActive                  = pkgsession.SessionGoalStatusActive
	SessionGoalStatusExhausted               = pkgsession.SessionGoalStatusExhausted
	SessionGoalStatusSatisfied               = pkgsession.SessionGoalStatusSatisfied
	TasksInjectionHeader                     = pkgsession.TasksInjectionHeader
)

var (
	AppendMessage                     = pkgsession.AppendMessage
	ArchiveSummary                    = pkgsession.ArchiveSummary
	BuildCompactionSummary            = pkgsession.BuildCompactionSummary
	BuildCompactionSummaryWithOptions = pkgsession.BuildCompactionSummaryWithOptions
	CompactTranscript                 = pkgsession.CompactTranscript
	CompactTranscriptWithOptions      = pkgsession.CompactTranscriptWithOptions
	DetectForkPromotionCandidates     = pkgsession.DetectForkPromotionCandidates
	ErrCwdNotEligible                 = pkgsession.ErrCwdNotEligible
	ErrSessionKindUnsupported         = pkgsession.ErrSessionKindUnsupported
	ErrSessionNotFound                = pkgsession.ErrSessionNotFound
	EstimateMessageTokenCost          = pkgsession.EstimateMessageTokenCost
	EstimateTokens                    = pkgsession.EstimateTokens
	EvidenceTypeLabel                 = pkgsession.EvidenceTypeLabel
	FormatTasksForInjection           = pkgsession.FormatTasksForInjection
	InheritCriticConfig               = pkgsession.InheritCriticConfig
	IsTasksInjectionMessage           = pkgsession.IsTasksInjectionMessage
	LoadHistory                       = pkgsession.LoadHistory
	LoadHistorySnapshot               = pkgsession.LoadHistorySnapshot
	NewStore                          = pkgsession.NewStore
	NextEvidenceID                    = pkgsession.NextEvidenceID
	NextTaskID                        = pkgsession.NextTaskID
	NormalizeAutoResumeModes          = pkgsession.NormalizeAutoResumeModes
	NormalizeCritic                   = pkgsession.NormalizeCritic
	NormalizeGoal                     = pkgsession.NormalizeGoal
	NormalizeStyleControl             = pkgsession.NormalizeStyleControl
	NowRFC3339                        = pkgsession.NowRFC3339
	ReadMessages                      = pkgsession.ReadMessages
	RewriteMessages                   = pkgsession.RewriteMessages
	TaskSummary                       = pkgsession.TaskSummary
	ValidEvidenceType                 = pkgsession.ValidEvidenceType
	ValidPlanStatus                   = pkgsession.ValidPlanStatus
	ValidTaskStatus                   = pkgsession.ValidTaskStatus
)
