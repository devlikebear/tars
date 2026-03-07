package tarsserver

import (
	"strings"

	"github.com/devlikebear/tarsncase/internal/tool"
)

func normalizeGatewayPolicyMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "allowlist" {
		return mode
	}
	return "full"
}

func normalizeGatewaySessionRoutingMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case "", "caller":
		return "caller"
	case "new", "fixed":
		return mode
	default:
		return "caller"
	}
}

func knownGatewayPromptTools(workspaceDir string) map[string]struct{} {
	out := map[string]struct{}{}
	registry := newBaseToolRegistry(workspaceDir)
	for _, schema := range registry.Schemas() {
		name := tool.CanonicalToolName(schema.Function.Name)
		if name == "" {
			continue
		}
		out[name] = struct{}{}
	}
	return out
}
