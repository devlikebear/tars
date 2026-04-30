package agentruntime

import "context"

type runtimeToolCallRecorderKey struct{}

type RuntimeToolCall struct {
	ToolName    string
	ToolCallID  string
	ToolArgs    string
	ToolIsError bool
}

type RuntimeToolCallRecorder func(RuntimeToolCall)

func WithRuntimeToolCallRecorder(ctx context.Context, recorder RuntimeToolCallRecorder) context.Context {
	if recorder == nil {
		return ctx
	}
	return context.WithValue(ctx, runtimeToolCallRecorderKey{}, recorder)
}

func RuntimeToolCallRecorderFromContext(ctx context.Context) RuntimeToolCallRecorder {
	if ctx == nil {
		return nil
	}
	recorder, _ := ctx.Value(runtimeToolCallRecorderKey{}).(RuntimeToolCallRecorder)
	return recorder
}
