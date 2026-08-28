// Package tool is a compatibility shim.
//
// The implementation moved to pkg/tools so that the public API owns its
// own types and renders on pkg.go.dev; see #928. This package aliases that
// surface so no in-repo call site had to change in the same commit as the
// move.
//
// New in-repo code should import github.com/devlikebear/tars/pkg/tools
// directly.
package tool

import pkgtools "github.com/devlikebear/tars/pkg/tools"

type (
	BlockedToolError   = pkgtools.BlockedToolError
	ContentBlock       = pkgtools.ContentBlock
	ExecToolOptions    = pkgtools.ExecToolOptions
	LineEmitter        = pkgtools.LineEmitter
	PathPolicy         = pkgtools.PathPolicy
	Policy             = pkgtools.Policy
	PolicyResolution   = pkgtools.PolicyResolution
	ProcessManager     = pkgtools.ProcessManager
	ProcessSnapshot    = pkgtools.ProcessSnapshot
	Registry           = pkgtools.Registry
	RegistryScope      = pkgtools.RegistryScope
	Result             = pkgtools.Result
	SessionStatus      = pkgtools.SessionStatus
	Tool               = pkgtools.Tool
	ToolEffectClass    = pkgtools.ToolEffectClass
	ToolOutputStreamer = pkgtools.ToolOutputStreamer
	ToolRecoveryPolicy = pkgtools.ToolRecoveryPolicy
	WebFetchOptions    = pkgtools.WebFetchOptions
	WebSearchOptions   = pkgtools.WebSearchOptions
)

const (
	RegistryScopeAny        = pkgtools.RegistryScopeAny
	RegistryScopePulse      = pkgtools.RegistryScopePulse
	RegistryScopeReflection = pkgtools.RegistryScopeReflection
	RegistryScopeUser       = pkgtools.RegistryScopeUser
	StreamStderr            = pkgtools.StreamStderr
	StreamStdout            = pkgtools.StreamStdout
	ToolEffectIdempotent    = pkgtools.ToolEffectIdempotent
	ToolEffectReadOnly      = pkgtools.ToolEffectReadOnly
	ToolEffectUnsafe        = pkgtools.ToolEffectUnsafe
)

var (
	AggregatorError               = pkgtools.AggregatorError
	BindLineEmitter               = pkgtools.BindLineEmitter
	CanonicalToolName             = pkgtools.CanonicalToolName
	DispatchAction                = pkgtools.DispatchAction
	ErrPatchUnavailable           = pkgtools.ErrPatchUnavailable
	ExpandToolGroups              = pkgtools.ExpandToolGroups
	ExpandToolPatterns            = pkgtools.ExpandToolPatterns
	IsExecToolName                = pkgtools.IsExecToolName
	IsHighRiskToolName            = pkgtools.IsHighRiskToolName
	JSONTextResult                = pkgtools.JSONTextResult
	KnownToolGroupNames           = pkgtools.KnownToolGroupNames
	KnownToolGroups               = pkgtools.KnownToolGroups
	LineEmitterFromContext        = pkgtools.LineEmitterFromContext
	NewApplyPatchTool             = pkgtools.NewApplyPatchTool
	NewEditFileTool               = pkgtools.NewEditFileTool
	NewEditFileToolWithPolicy     = pkgtools.NewEditFileToolWithPolicy
	NewEditTool                   = pkgtools.NewEditTool
	NewExecTool                   = pkgtools.NewExecTool
	NewExecToolWithManager        = pkgtools.NewExecToolWithManager
	NewExecToolWithOptions        = pkgtools.NewExecToolWithOptions
	NewExecToolWithPolicy         = pkgtools.NewExecToolWithPolicy
	NewGlobTool                   = pkgtools.NewGlobTool
	NewGlobToolWithPolicy         = pkgtools.NewGlobToolWithPolicy
	NewListDirTool                = pkgtools.NewListDirTool
	NewListDirToolWithPolicy      = pkgtools.NewListDirToolWithPolicy
	NewMemoryGetTool              = pkgtools.NewMemoryGetTool
	NewMemorySaveTool             = pkgtools.NewMemorySaveTool
	NewMemorySearchTool           = pkgtools.NewMemorySearchTool
	NewMemoryTool                 = pkgtools.NewMemoryTool
	NewPathPolicy                 = pkgtools.NewPathPolicy
	NewProcessManager             = pkgtools.NewProcessManager
	NewProcessTool                = pkgtools.NewProcessTool
	NewProjectSkillToolWithPolicy = pkgtools.NewProjectSkillToolWithPolicy
	NewReadFileTool               = pkgtools.NewReadFileTool
	NewReadFileToolWithPolicy     = pkgtools.NewReadFileToolWithPolicy
	NewReadTool                   = pkgtools.NewReadTool
	NewRegistry                   = pkgtools.NewRegistry
	NewRegistryWithScope          = pkgtools.NewRegistryWithScope
	NewSessionStatusTool          = pkgtools.NewSessionStatusTool
	NewWebFetchTool               = pkgtools.NewWebFetchTool
	NewWebFetchToolWithOptions    = pkgtools.NewWebFetchToolWithOptions
	NewWebSearchTool              = pkgtools.NewWebSearchTool
	NewWebSearchToolWithOptions   = pkgtools.NewWebSearchToolWithOptions
	NewWriteFileTool              = pkgtools.NewWriteFileTool
	NewWriteFileToolWithPolicy    = pkgtools.NewWriteFileToolWithPolicy
	NewWriteTool                  = pkgtools.NewWriteTool
	NormalizeToolGroupName        = pkgtools.NormalizeToolGroupName
	ParseBlockedToolError         = pkgtools.ParseBlockedToolError
	RecoveryPolicyForTool         = pkgtools.RecoveryPolicyForTool
	SingleDirPolicy               = pkgtools.SingleDirPolicy
	ToolGroupForName              = pkgtools.ToolGroupForName
	ToolNameAliases               = pkgtools.ToolNameAliases
	ToolOutputStreamerFromContext = pkgtools.ToolOutputStreamerFromContext
	WithLineEmitter               = pkgtools.WithLineEmitter
	WithToolOutputStreamer        = pkgtools.WithToolOutputStreamer
)
