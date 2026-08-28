package llm

import (
	"context"
	"strings"
)

type SelectionMetadata struct {
	Role      Role
	Tier      Tier
	Provider  string
	Model     string
	Source    string
	SessionID string
	RunID     string
	AgentName string
	FlowID    string
	StepID    string
}

type selectionMetadataKey struct{}

func WithSelectionMetadata(ctx context.Context, meta SelectionMetadata) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	normalized := normalizeSelectionMetadata(meta)
	if existing, ok := SelectionMetadataFromContext(ctx); ok {
		normalized = mergeSelectionMetadata(existing, normalized)
	}
	return context.WithValue(ctx, selectionMetadataKey{}, normalized)
}

func SelectionMetadataFromContext(ctx context.Context) (SelectionMetadata, bool) {
	if ctx == nil {
		return SelectionMetadata{}, false
	}
	meta, ok := ctx.Value(selectionMetadataKey{}).(SelectionMetadata)
	if !ok {
		return SelectionMetadata{}, false
	}
	return meta, true
}

func mergeSelectionMetadata(base, override SelectionMetadata) SelectionMetadata {
	out := base
	if override.Role != "" {
		out.Role = override.Role
	}
	if override.Tier != "" {
		out.Tier = override.Tier
	}
	if override.Provider != "" {
		out.Provider = override.Provider
	}
	if override.Model != "" {
		out.Model = override.Model
	}
	if override.Source != "" {
		out.Source = override.Source
	}
	if override.SessionID != "" {
		out.SessionID = override.SessionID
	}
	if override.RunID != "" {
		out.RunID = override.RunID
	}
	if override.AgentName != "" {
		out.AgentName = override.AgentName
	}
	if override.FlowID != "" {
		out.FlowID = override.FlowID
	}
	if override.StepID != "" {
		out.StepID = override.StepID
	}
	return out
}

func normalizeSelectionMetadata(meta SelectionMetadata) SelectionMetadata {
	meta.Role = ParseRoleOrKeep(meta.Role)
	meta.Tier = ParseTierOrKeep(meta.Tier)
	meta.Provider = strings.TrimSpace(strings.ToLower(meta.Provider))
	meta.Model = strings.TrimSpace(meta.Model)
	meta.Source = strings.TrimSpace(strings.ToLower(meta.Source))
	meta.SessionID = strings.TrimSpace(meta.SessionID)
	meta.RunID = strings.TrimSpace(meta.RunID)
	meta.AgentName = strings.TrimSpace(meta.AgentName)
	meta.FlowID = strings.TrimSpace(meta.FlowID)
	meta.StepID = strings.TrimSpace(meta.StepID)
	return meta
}

func ParseRoleOrKeep(role Role) Role {
	if role == "" {
		return ""
	}
	if parsed, ok := ParseRole(role.String()); ok {
		return parsed
	}
	return Role(strings.ToLower(strings.TrimSpace(role.String())))
}

func ParseTierOrKeep(tier Tier) Tier {
	if tier == "" {
		return ""
	}
	if parsed, err := ParseTier(string(tier)); err == nil {
		return parsed
	}
	return Tier(strings.ToLower(strings.TrimSpace(string(tier))))
}
