package tool

import (
	"context"
	"strings"
)

type currentSessionIDKey struct{}
type currentSessionKindKey struct{}
type currentTelegramTargetKey struct{}

type currentTelegramTarget struct {
	ChatID   string
	ThreadID string
	BotID    string
}

// WithCurrentSessionID exposes the active chat session id to automation tools
// that need to resolve special aliases such as "current".
func WithCurrentSessionID(ctx context.Context, sessionID string) context.Context {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ctx
	}
	return context.WithValue(ctx, currentSessionIDKey{}, sessionID)
}

func WithCurrentSessionKind(ctx context.Context, sessionKind string) context.Context {
	sessionKind = strings.TrimSpace(sessionKind)
	if sessionKind == "" {
		return ctx
	}
	return context.WithValue(ctx, currentSessionKindKey{}, sessionKind)
}

func WithCurrentSessionInfo(ctx context.Context, sessionID string, sessionKind string) context.Context {
	ctx = WithCurrentSessionID(ctx, sessionID)
	return WithCurrentSessionKind(ctx, sessionKind)
}

func WithCurrentTelegramTarget(ctx context.Context, chatID string, threadID string, botID string) context.Context {
	target := currentTelegramTarget{
		ChatID:   strings.TrimSpace(chatID),
		ThreadID: strings.TrimSpace(threadID),
		BotID:    strings.TrimSpace(botID),
	}
	if target.ChatID == "" && target.ThreadID == "" && target.BotID == "" {
		return ctx
	}
	return context.WithValue(ctx, currentTelegramTargetKey{}, target)
}

func currentSessionIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	sessionID, _ := ctx.Value(currentSessionIDKey{}).(string)
	return strings.TrimSpace(sessionID)
}

func currentSessionKindFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	sessionKind, _ := ctx.Value(currentSessionKindKey{}).(string)
	return strings.TrimSpace(sessionKind)
}

func currentTelegramTargetFromContext(ctx context.Context) currentTelegramTarget {
	if ctx == nil {
		return currentTelegramTarget{}
	}
	target, _ := ctx.Value(currentTelegramTargetKey{}).(currentTelegramTarget)
	target.ChatID = strings.TrimSpace(target.ChatID)
	target.ThreadID = strings.TrimSpace(target.ThreadID)
	target.BotID = strings.TrimSpace(target.BotID)
	return target
}
