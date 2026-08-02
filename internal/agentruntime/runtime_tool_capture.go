package agentruntime

import "context"

type runtimeToolCallRecorderKey struct{}

type RuntimeToolCall struct {
	Phase                      RuntimeToolPhase
	Iteration                  int
	ToolName                   string
	ToolCallID                 string
	ToolArgs                   string
	ToolResult                 string
	ToolIsError                bool
	ToolEffectClass            string
	ToolIdempotencyKeyArgument string
	ToolReplayed               bool
	ToolReceiptID              string
	ContinuationID             string
}

type runtimeExecutionRecorderKey struct{}

type RuntimeExecutionRecorder func(RuntimeToolCall) error

func WithRuntimeExecutionRecorder(ctx context.Context, recorder RuntimeExecutionRecorder) context.Context {
	if recorder == nil {
		return ctx
	}
	return context.WithValue(ctx, runtimeExecutionRecorderKey{}, recorder)
}

func RuntimeExecutionRecorderFromContext(ctx context.Context) RuntimeExecutionRecorder {
	if ctx == nil {
		return nil
	}
	recorder, _ := ctx.Value(runtimeExecutionRecorderKey{}).(RuntimeExecutionRecorder)
	return recorder
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
