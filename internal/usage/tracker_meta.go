package usage

import (
	"context"
	"strings"
)

type callMetaKey struct{}

type CallMeta struct {
	Source    string
	SessionID string
	RunID     string
}

func WithCallMeta(ctx context.Context, meta CallMeta) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, callMetaKey{}, normalizeCallMeta(meta))
}

func CallMetaFromContext(ctx context.Context) CallMeta {
	if ctx == nil {
		return CallMeta{Source: "chat"}
	}
	if value, ok := ctx.Value(callMetaKey{}).(CallMeta); ok {
		return normalizeCallMeta(value)
	}
	return CallMeta{Source: "chat"}
}

func normalizeCallMeta(meta CallMeta) CallMeta {
	out := CallMeta{
		Source:    strings.TrimSpace(strings.ToLower(meta.Source)),
		SessionID: strings.TrimSpace(meta.SessionID),
		RunID:     strings.TrimSpace(meta.RunID),
	}
	switch out.Source {
	case "chat", "cron", "heartbeat", "agent_run":
	default:
		out.Source = "chat"
	}
	return out
}
