package agentruntime

import (
	"context"
	"strings"
)

type systemPromptAppendKey struct{}

func WithSystemPromptAppend(ctx context.Context, value string) context.Context {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ctx
	}
	return context.WithValue(ctx, systemPromptAppendKey{}, trimmed)
}

func SystemPromptAppendFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(systemPromptAppendKey{}).(string)
	return strings.TrimSpace(value)
}
