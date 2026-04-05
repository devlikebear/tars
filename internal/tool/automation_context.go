package tool

import (
	"context"
	"strings"
)

type currentSessionIDKey struct{}

// WithCurrentSessionID exposes the active chat session id to automation tools
// that need to resolve special aliases such as "current".
func WithCurrentSessionID(ctx context.Context, sessionID string) context.Context {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ctx
	}
	return context.WithValue(ctx, currentSessionIDKey{}, sessionID)
}

func currentSessionIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	sessionID, _ := ctx.Value(currentSessionIDKey{}).(string)
	return strings.TrimSpace(sessionID)
}
