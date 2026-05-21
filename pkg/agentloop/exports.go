package agentloop

import (
	internal "github.com/devlikebear/tars/internal/agent"
	"github.com/devlikebear/tars/pkg/llm"
	"github.com/devlikebear/tars/pkg/tools"
)

type EventType = internal.EventType

const (
	EventLoopStart      = internal.EventLoopStart
	EventBeforeLLM      = internal.EventBeforeLLM
	EventAfterLLM       = internal.EventAfterLLM
	EventBeforeTool     = internal.EventBeforeTool
	EventAfterTool      = internal.EventAfterTool
	EventProviderTool   = internal.EventProviderTool
	EventLoopEnd        = internal.EventLoopEnd
	EventLoopError      = internal.EventLoopError
	DefaultMaxLoopIters = internal.DefaultMaxLoopIters
)

type Event = internal.Event
type Hook = internal.Hook
type HookFunc = internal.HookFunc
type Loop = internal.Loop
type RunOptions = internal.RunOptions
type CounterHook = internal.CounterHook
type AuditEntry = internal.AuditEntry
type AuditHook = internal.AuditHook

func New(client llm.Client, registry *tools.Registry, hooks ...Hook) *Loop {
	return internal.NewLoop(client, registry, hooks...)
}

func NewCounterHook() *CounterHook { return internal.NewCounterHook() }

func NewAuditHook(maxEntries int) *AuditHook {
	return internal.NewAuditHook(maxEntries)
}
