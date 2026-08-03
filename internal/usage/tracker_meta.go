package usage

import (
	"context"
	"strings"
)

type callMetaKey struct{}

type CallMeta struct {
	Source               string
	SessionID            string
	RunID                string
	CapabilityVersionIDs []string
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
		Source:               strings.TrimSpace(strings.ToLower(meta.Source)),
		SessionID:            strings.TrimSpace(meta.SessionID),
		RunID:                strings.TrimSpace(meta.RunID),
		CapabilityVersionIDs: normalizeCapabilityVersionIDs(meta.CapabilityVersionIDs),
	}
	switch out.Source {
	case "chat", "cron", "pulse", "reflection", "agent_run", "api":
	default:
		out.Source = "chat"
	}
	return out
}

func normalizeCapabilityVersionIDs(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	result := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, candidate := range raw {
		id := strings.TrimSpace(candidate)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}
