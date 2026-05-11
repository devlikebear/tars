package mcp

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/devlikebear/tars/internal/config"
)

// validateRemoteMCPURL parses and structurally validates an admin-configured
// remote MCP endpoint URL.
//
// TARS intentionally allows admins to point at any host (including loopback
// or private-network targets used for local MCP servers), so this check is a
// boundary sanitizer rather than a destination allowlist:
//
//   - The URL must parse.
//   - The scheme must match the transport (http/https for HTTP/SSE,
//     ws/wss for WebSocket). This rejects file://, gopher://, etc. that
//     would otherwise be exploitable as request-forgery sinks.
//   - The host must be non-empty.
//
// Callers should pass the parsed URL into downstream dialers rather than
// re-parsing the raw string, so any structural guarantees here carry through.
func validateRemoteMCPURL(transport, raw string) (*url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("remote mcp url is required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("parse remote mcp url: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	switch transport {
	case config.MCPTransportStreamableHTTP, config.MCPTransportSSE:
		if scheme != "http" && scheme != "https" {
			return nil, fmt.Errorf("remote mcp url scheme %q is not allowed for %s transport (expected http or https)", parsed.Scheme, transport)
		}
	case config.MCPTransportWebSocket:
		if scheme != "ws" && scheme != "wss" {
			return nil, fmt.Errorf("remote mcp url scheme %q is not allowed for %s transport (expected ws or wss)", parsed.Scheme, transport)
		}
	default:
		return nil, fmt.Errorf("transport %q does not accept a remote url", transport)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("remote mcp url %q has no host", raw)
	}
	return parsed, nil
}

// resolveLegacySSEPostURL takes the configured base URL and the raw "endpoint"
// payload announced by an SSE server, and returns the resolved POST URL only
// when it shares the same scheme and host as the configured base.
//
// SSE event data is server-controlled, so without this check a remote MCP
// server could redirect subsequent JSON-RPC POSTs to an attacker-controlled
// origin (or to a different internal service the TARS process can reach).
func resolveLegacySSEPostURL(base *url.URL, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("legacy sse endpoint event missing data")
	}
	endpoint, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse legacy sse endpoint: %w", err)
	}
	if base == nil {
		return "", fmt.Errorf("legacy sse endpoint cannot be resolved without a base url")
	}
	resolved := base.ResolveReference(endpoint)
	if !strings.EqualFold(resolved.Scheme, base.Scheme) || !strings.EqualFold(resolved.Host, base.Host) {
		return "", fmt.Errorf("legacy sse endpoint %q resolves to %s://%s outside the configured origin %s://%s", raw, resolved.Scheme, resolved.Host, base.Scheme, base.Host)
	}
	return resolved.String(), nil
}
