// Package agent is a compatibility shim.
//
// The implementation moved to pkg/agentloop so that the public API owns its
// own types and renders on pkg.go.dev; see #928. This package aliases that
// surface so no in-repo call site had to change in the same commit as the
// move.
//
// New in-repo code should import github.com/devlikebear/tars/pkg/agentloop
// directly.
package agent

import pkgagentloop "github.com/devlikebear/tars/pkg/agentloop"

type (
	AuditEntry        = pkgagentloop.AuditEntry
	AuditHook         = pkgagentloop.AuditHook
	CounterHook       = pkgagentloop.CounterHook
	Event             = pkgagentloop.Event
	EventType         = pkgagentloop.EventType
	Hook              = pkgagentloop.Hook
	HookFunc          = pkgagentloop.HookFunc
	Loop              = pkgagentloop.Loop
	RunOptions        = pkgagentloop.RunOptions
	ToolReplayRequest = pkgagentloop.ToolReplayRequest
	ToolReplayResult  = pkgagentloop.ToolReplayResult
)

const (
	DefaultMaxLoopIters = pkgagentloop.DefaultMaxLoopIters
	EventAfterLLM       = pkgagentloop.EventAfterLLM
	EventAfterTool      = pkgagentloop.EventAfterTool
	EventBeforeLLM      = pkgagentloop.EventBeforeLLM
	EventBeforeTool     = pkgagentloop.EventBeforeTool
	EventLoopEnd        = pkgagentloop.EventLoopEnd
	EventLoopError      = pkgagentloop.EventLoopError
	EventLoopStart      = pkgagentloop.EventLoopStart
	EventProviderTool   = pkgagentloop.EventProviderTool
)

var (
	NewAuditHook   = pkgagentloop.NewAuditHook
	NewCounterHook = pkgagentloop.NewCounterHook
	NewLoop        = pkgagentloop.NewLoop
)
