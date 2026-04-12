package gateway

import "context"

type promptExecutionContextKey struct{}

type PromptExecutionContext struct {
	ProviderOverride *ProviderOverride
	OverrideSource   string
	Metadata         *PromptExecutionMetadata
}

func WithPromptExecution(ctx context.Context, override *ProviderOverride, source string, metadata *PromptExecutionMetadata) context.Context {
	return context.WithValue(ctx, promptExecutionContextKey{}, PromptExecutionContext{
		ProviderOverride: CloneProviderOverride(override),
		OverrideSource:   source,
		Metadata:         metadata,
	})
}

func PromptExecutionFromContext(ctx context.Context) PromptExecutionContext {
	if ctx == nil {
		return PromptExecutionContext{}
	}
	value, _ := ctx.Value(promptExecutionContextKey{}).(PromptExecutionContext)
	value.ProviderOverride = CloneProviderOverride(value.ProviderOverride)
	return value
}
