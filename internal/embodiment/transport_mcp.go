package embodiment

import (
	"context"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/tool"
	"github.com/rs/zerolog"
)

type MCPToolCaller interface {
	CallTool(context.Context, string, string, map[string]any) (tool.Result, error)
}

type MCPTransport struct {
	caller   MCPToolCaller
	logger   zerolog.Logger
	attempts int
}

func NewMCPTransport(caller MCPToolCaller, logger zerolog.Logger) *MCPTransport {
	return &MCPTransport{caller: caller, logger: logger, attempts: 2}
}

func (t *MCPTransport) Dispatch(ctx context.Context, provider ProviderDescriptor, action BodyAction) error {
	if t == nil || t.caller == nil {
		return fmt.Errorf("mcp transport is not configured")
	}
	if provider.Transport != TransportMCP {
		return fmt.Errorf("unsupported embodiment transport %q", provider.Transport)
	}
	normalized, err := NormalizeBodyAction(action)
	if err != nil {
		return err
	}
	serverName := strings.TrimSpace(provider.Endpoint)
	if serverName == "" {
		serverName = strings.TrimSpace(provider.Name)
	}
	if serverName == "" {
		return fmt.Errorf("mcp provider endpoint is required")
	}
	toolName := mcpActionToolName(provider, normalized)
	attempts := t.attempts
	if attempts <= 0 {
		attempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		result, err := t.caller.CallTool(ctx, serverName, toolName, cloneActionPayload(normalized.Payload))
		if err == nil && result.IsError {
			err = fmt.Errorf("mcp tool %s returned error", toolName)
		}
		if err == nil {
			return nil
		}
		lastErr = err
		t.logger.Info().
			Err(err).
			Str("provider", normalizeName(provider.Name)).
			Str("server", serverName).
			Str("tool", toolName).
			Int("attempt", attempt).
			Msg("embodiment mcp action dispatch failed")
	}
	return lastErr
}

func mcpActionToolName(provider ProviderDescriptor, action BodyAction) string {
	prefix := mcpToolPrefix(provider.Name)
	switch action.Kind {
	case ActionSpeak:
		return prefix + "_speak"
	case ActionExpress:
		return prefix + "_set_expression"
	case ActionMove:
		if firstActionString(action.Payload, "name", "motion", "preset") != "" {
			return prefix + "_run_motion"
		}
		return prefix + "_move_head"
	case ActionLED:
		return prefix + "_set_led"
	default:
		return prefix + "_" + strings.TrimSpace(string(action.Kind))
	}
}

func mcpToolPrefix(providerName string) string {
	normalized := normalizeName(providerName)
	if normalized == "" {
		return "embodiment"
	}
	var b strings.Builder
	for _, r := range normalized {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_':
			b.WriteRune(r)
		case r == '-', r == '.', r == ' ':
			b.WriteRune('_')
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "embodiment"
	}
	return out
}
