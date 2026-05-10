package tool

import (
	"context"
	"strings"
)

const (
	StreamStdout = "stdout"
	StreamStderr = "stderr"
)

// LineEmitter is the chat-handler-side sink that receives streaming output
// lines emitted by tools (e.g. exec stdout/stderr). Implementations route
// them onward — typically to SSE as `tool_output_line` events keyed by
// tool_call_id.
type LineEmitter interface {
	EmitToolLine(toolCallID, stream, text string)
}

// ToolOutputStreamer is the per-tool-call view that tools see. It binds a
// tool_call_id once at the agent-loop boundary so tool code can call
// EmitLine without threading the id through every call site.
type ToolOutputStreamer interface {
	EmitLine(stream, text string)
}

type lineEmitterKey struct{}
type toolOutputStreamerKey struct{}

func WithLineEmitter(ctx context.Context, emitter LineEmitter) context.Context {
	if emitter == nil {
		return ctx
	}
	return context.WithValue(ctx, lineEmitterKey{}, emitter)
}

func LineEmitterFromContext(ctx context.Context) LineEmitter {
	if ctx == nil {
		return nil
	}
	if v, ok := ctx.Value(lineEmitterKey{}).(LineEmitter); ok {
		return v
	}
	return nil
}

func WithToolOutputStreamer(ctx context.Context, streamer ToolOutputStreamer) context.Context {
	if streamer == nil {
		return ctx
	}
	return context.WithValue(ctx, toolOutputStreamerKey{}, streamer)
}

func ToolOutputStreamerFromContext(ctx context.Context) ToolOutputStreamer {
	if ctx == nil {
		return nil
	}
	if v, ok := ctx.Value(toolOutputStreamerKey{}).(ToolOutputStreamer); ok {
		return v
	}
	return nil
}

// BindLineEmitter wraps a chat-level LineEmitter into a per-tool-call
// ToolOutputStreamer. Returns nil when the parent emitter is nil.
func BindLineEmitter(emitter LineEmitter, toolCallID string) ToolOutputStreamer {
	if emitter == nil {
		return nil
	}
	return &boundLineEmitter{
		emitter:    emitter,
		toolCallID: strings.TrimSpace(toolCallID),
	}
}

type boundLineEmitter struct {
	emitter    LineEmitter
	toolCallID string
}

func (b *boundLineEmitter) EmitLine(stream, text string) {
	if b == nil || b.emitter == nil {
		return
	}
	b.emitter.EmitToolLine(b.toolCallID, stream, text)
}
