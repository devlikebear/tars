package config

import "strings"

const (
	MCPTransportStdio          = "stdio"
	MCPTransportStreamableHTTP = "streamable_http"
	MCPTransportSSE            = "sse"
	MCPTransportWebSocket      = "websocket"
)

const (
	MCPAuthModeNone   = "none"
	MCPAuthModeBearer = "bearer"
	MCPAuthModeOAuth  = "oauth"
)

func NormalizeMCPServer(server MCPServer) MCPServer {
	out := MCPServer{
		Name:          strings.TrimSpace(server.Name),
		Command:       strings.TrimSpace(server.Command),
		URL:           strings.TrimSpace(server.URL),
		Transport:     normalizeMCPTransport(server.Transport),
		AuthMode:      normalizeMCPAuthMode(server.AuthMode),
		AuthTokenEnv:  strings.TrimSpace(server.AuthTokenEnv),
		OAuthProvider: strings.TrimSpace(strings.ToLower(server.OAuthProvider)),
		Source:        strings.TrimSpace(strings.ToLower(server.Source)),
		Args:          make([]string, 0, len(server.Args)),
	}
	for _, arg := range server.Args {
		if trimmed := strings.TrimSpace(arg); trimmed != "" {
			out.Args = append(out.Args, trimmed)
		}
	}
	if len(server.Env) > 0 {
		out.Env = make(map[string]string, len(server.Env))
		for key, value := range server.Env {
			trimmedKey := strings.TrimSpace(key)
			if trimmedKey == "" {
				continue
			}
			out.Env[trimmedKey] = value
		}
	}
	if len(server.Headers) > 0 {
		out.Headers = make(map[string]string, len(server.Headers))
		for key, value := range server.Headers {
			trimmedKey := strings.TrimSpace(key)
			if trimmedKey == "" {
				continue
			}
			out.Headers[trimmedKey] = value
		}
	}
	return out
}

func MCPServerEnabled(server MCPServer) bool {
	server = NormalizeMCPServer(server)
	if server.Name == "" {
		return false
	}
	if MCPServerIsRemote(server) {
		return server.URL != ""
	}
	return server.Command != ""
}

func MCPServerIsRemote(server MCPServer) bool {
	switch normalizeMCPTransport(server.Transport) {
	case MCPTransportStreamableHTTP, MCPTransportSSE, MCPTransportWebSocket:
		return true
	default:
		return false
	}
}

func normalizeMCPTransport(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "", MCPTransportStdio:
		return MCPTransportStdio
	case MCPTransportStreamableHTTP, "http", "streamable-http", "streamablehttp":
		return MCPTransportStreamableHTTP
	case MCPTransportSSE:
		return MCPTransportSSE
	case MCPTransportWebSocket, "ws":
		return MCPTransportWebSocket
	default:
		return MCPTransportStdio
	}
}

func normalizeMCPAuthMode(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "", MCPAuthModeNone:
		return ""
	case MCPAuthModeBearer:
		return MCPAuthModeBearer
	case MCPAuthModeOAuth:
		return MCPAuthModeOAuth
	default:
		return ""
	}
}
