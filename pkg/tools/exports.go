package tools

import (
	"context"
	"encoding/json"
	"time"

	internal "github.com/devlikebear/tars/internal/tool"
	internalmemory "github.com/devlikebear/tars/pkg/memory"
)

type RegistryScope = internal.RegistryScope

const (
	RegistryScopeAny        = internal.RegistryScopeAny
	RegistryScopeUser       = internal.RegistryScopeUser
	RegistryScopePulse      = internal.RegistryScopePulse
	RegistryScopeReflection = internal.RegistryScopeReflection
)

type ContentBlock = internal.ContentBlock
type Result = internal.Result
type Tool = internal.Tool
type Registry = internal.Registry
type Policy = internal.Policy
type BlockedToolError = internal.BlockedToolError
type PolicyResolution = internal.PolicyResolution
type PathPolicy = internal.PathPolicy
type ExecToolOptions = internal.ExecToolOptions
type ProcessSnapshot = internal.ProcessSnapshot
type ProcessManager = internal.ProcessManager
type WebFetchOptions = internal.WebFetchOptions
type WebSearchOptions = internal.WebSearchOptions
type LineEmitter = internal.LineEmitter
type ToolOutputStreamer = internal.ToolOutputStreamer

const (
	StreamStdout = internal.StreamStdout
	StreamStderr = internal.StreamStderr
)

func NewRegistry() *Registry { return internal.NewRegistry() }

func NewRegistryWithScope(scope RegistryScope) *Registry {
	return internal.NewRegistryWithScope(scope)
}

func ParseBlockedToolError(err error) (BlockedToolError, bool) {
	return internal.ParseBlockedToolError(err)
}

func NewPathPolicy(workspaceDir string, workDirs []string, currentDir string) PathPolicy {
	return internal.NewPathPolicy(workspaceDir, workDirs, currentDir)
}

func SingleDirPolicy(workspaceDir string) PathPolicy {
	return internal.SingleDirPolicy(workspaceDir)
}

func NewProcessManager() *ProcessManager { return internal.NewProcessManager() }

func NewReadTool(workspaceDir string) Tool { return internal.NewReadTool(workspaceDir) }

func NewReadFileTool(workspaceDir string) Tool { return internal.NewReadFileTool(workspaceDir) }

func NewReadFileToolWithPolicy(policy PathPolicy) Tool {
	return internal.NewReadFileToolWithPolicy(policy)
}

func NewWriteTool(workspaceDir string) Tool { return internal.NewWriteTool(workspaceDir) }

func NewWriteFileTool(workspaceDir string) Tool { return internal.NewWriteFileTool(workspaceDir) }

func NewWriteFileToolWithPolicy(policy PathPolicy) Tool {
	return internal.NewWriteFileToolWithPolicy(policy)
}

func NewEditTool(workspaceDir string) Tool { return internal.NewEditTool(workspaceDir) }

func NewEditFileTool(workspaceDir string) Tool { return internal.NewEditFileTool(workspaceDir) }

func NewEditFileToolWithPolicy(policy PathPolicy) Tool {
	return internal.NewEditFileToolWithPolicy(policy)
}

func NewListDirTool(workspaceDir string) Tool { return internal.NewListDirTool(workspaceDir) }

func NewListDirToolWithPolicy(policy PathPolicy) Tool {
	return internal.NewListDirToolWithPolicy(policy)
}

func NewGlobTool(workspaceDir string) Tool { return internal.NewGlobTool(workspaceDir) }

func NewGlobToolWithPolicy(policy PathPolicy) Tool {
	return internal.NewGlobToolWithPolicy(policy)
}

func NewProjectSkillToolWithPolicy(policy PathPolicy) Tool {
	return internal.NewProjectSkillToolWithPolicy(policy)
}

func NewApplyPatchTool(workspaceDir string, enabled bool) Tool {
	return internal.NewApplyPatchTool(workspaceDir, enabled)
}

func NewExecTool(workspaceDir string) Tool { return internal.NewExecTool(workspaceDir) }

func NewExecToolWithManager(workspaceDir string, manager *ProcessManager) Tool {
	return internal.NewExecToolWithManager(workspaceDir, manager)
}

func NewExecToolWithPolicy(policy PathPolicy, manager *ProcessManager) Tool {
	return internal.NewExecToolWithPolicy(policy, manager)
}

func NewExecToolWithOptions(policy PathPolicy, manager *ProcessManager, opts ExecToolOptions) Tool {
	return internal.NewExecToolWithOptions(policy, manager, opts)
}

func NewProcessTool(manager *ProcessManager) Tool {
	return internal.NewProcessTool(manager)
}

func NewWebFetchTool(enabled bool) Tool { return internal.NewWebFetchTool(enabled) }

func NewWebFetchToolWithOptions(opts WebFetchOptions) Tool {
	return internal.NewWebFetchToolWithOptions(opts)
}

func NewWebSearchTool(enabled bool, apiKey string) Tool {
	return internal.NewWebSearchTool(enabled, apiKey)
}

func NewWebSearchToolWithOptions(opts WebSearchOptions) Tool {
	return internal.NewWebSearchToolWithOptions(opts)
}

func NewMemoryTool(workspaceDir string, backend internalmemory.Backend, nowFn func() time.Time) Tool {
	return internal.NewMemoryTool(workspaceDir, backend, nowFn)
}

func NewMemorySaveTool(backend internalmemory.Backend, nowFn func() time.Time) Tool {
	return internal.NewMemorySaveTool(backend, nowFn)
}

func NewMemorySearchTool(workspaceDir string, backend internalmemory.Backend) Tool {
	return internal.NewMemorySearchTool(workspaceDir, backend)
}

func NewMemoryGetTool(workspaceDir string, backend internalmemory.Backend) Tool {
	return internal.NewMemoryGetTool(workspaceDir, backend)
}

func JSONTextResult(value any, isError bool) Result {
	return internal.JSONTextResult(value, isError)
}

func CanonicalToolName(name string) string { return internal.CanonicalToolName(name) }

func ToolNameAliases() map[string]string { return internal.ToolNameAliases() }

func IsExecToolName(name string) bool { return internal.IsExecToolName(name) }

func IsHighRiskToolName(name string) bool { return internal.IsHighRiskToolName(name) }

func KnownToolGroupNames() []string { return internal.KnownToolGroupNames() }

func NormalizeToolGroupName(name string) string {
	return internal.NormalizeToolGroupName(name)
}

func ToolGroupForName(name string) string { return internal.ToolGroupForName(name) }

func KnownToolGroups(known map[string]struct{}) map[string][]string {
	return internal.KnownToolGroups(known)
}

func ExpandToolGroups(groupNames []string, known map[string]struct{}) (validGroups []string, expandedTools []string, unknownGroups []string) {
	return internal.ExpandToolGroups(groupNames, known)
}

func ExpandToolPatterns(patterns []string, known map[string]struct{}) (validPatterns []string, matchedTools []string, invalidPatterns []string) {
	return internal.ExpandToolPatterns(patterns, known)
}

func WithLineEmitter(ctx context.Context, emitter LineEmitter) context.Context {
	return internal.WithLineEmitter(ctx, emitter)
}

func LineEmitterFromContext(ctx context.Context) LineEmitter {
	return internal.LineEmitterFromContext(ctx)
}

func WithToolOutputStreamer(ctx context.Context, streamer ToolOutputStreamer) context.Context {
	return internal.WithToolOutputStreamer(ctx, streamer)
}

func ToolOutputStreamerFromContext(ctx context.Context) ToolOutputStreamer {
	return internal.ToolOutputStreamerFromContext(ctx)
}

func BindLineEmitter(emitter LineEmitter, toolCallID string) ToolOutputStreamer {
	return internal.BindLineEmitter(emitter, toolCallID)
}

func MustJSON(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}
