package agentruntime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type promptExecutionContextKey struct{}
type executionRootContextKey struct{}
type parentSessionContextKey struct{}

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

func WithExecutionRoot(ctx context.Context, root string) context.Context {
	return context.WithValue(ctx, executionRootContextKey{}, strings.TrimSpace(root))
}

func ExecutionRootFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	root, _ := ctx.Value(executionRootContextKey{}).(string)
	return strings.TrimSpace(root)
}

func normalizeExecutionRoot(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	absolute, err := filepath.Abs(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("resolve execution root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve execution root: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("execution root is unavailable")
	}
	return filepath.Clean(resolved), nil
}

// WithParentSession records the session that spawned a run, so the prompt
// runner can apply that session's permission mode to the run's tools.
func WithParentSession(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, parentSessionContextKey{}, strings.TrimSpace(sessionID))
}

// ParentSessionFromContext returns the session that spawned the running
// run, or "" when none did.
func ParentSessionFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(parentSessionContextKey{}).(string)
	return id
}
