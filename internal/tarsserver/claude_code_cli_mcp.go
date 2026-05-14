package tarsserver

import (
	"strings"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/llm"
)

// toClaudeCodeMCPServers converts the TARS-side MCP server slice (used by the
// extensions manager + session overrides) into the provider-agnostic shape
// understood by the claude-code-cli provider. Servers without a Name or that
// are explicitly disabled per config.MCPServerEnabled (e.g. stdio without
// Command, remote without URL) are dropped — they'd produce an invalid
// `{"mcpServers": {"": ...}}` JSON object or a dead reference.
//
// Transport translation maps TARS' canonical transport names (produced by
// config.NormalizeMCPServer) into Claude Code's `type` values:
//
//	stdio           → "" (provider defaults to stdio)
//	streamable_http → "http"
//	sse             → "sse"
//	websocket       → skipped (Claude Code's --mcp-config has no ws shape)
func toClaudeCodeMCPServers(servers []config.MCPServer) []llm.ClaudeCodeMCPServer {
	if len(servers) == 0 {
		return nil
	}
	out := make([]llm.ClaudeCodeMCPServer, 0, len(servers))
	for _, raw := range servers {
		srv := config.NormalizeMCPServer(raw)
		name := strings.TrimSpace(srv.Name)
		if name == "" {
			continue
		}
		if !config.MCPServerEnabled(srv) {
			continue
		}
		var transport string
		switch srv.Transport {
		case config.MCPTransportStdio, "":
			transport = ""
		case config.MCPTransportStreamableHTTP:
			transport = "http"
		case config.MCPTransportSSE:
			transport = "sse"
		case config.MCPTransportWebSocket:
			// Claude Code's --mcp-config schema doesn't expose websocket;
			// silently drop rather than emit a server claude can't load.
			continue
		default:
			transport = ""
		}
		out = append(out, llm.ClaudeCodeMCPServer{
			Name:      name,
			Transport: transport,
			Command:   srv.Command,
			Args:      append([]string(nil), srv.Args...),
			Env:       cloneStringMap(srv.Env),
			URL:       srv.URL,
			Headers:   cloneStringMap(srv.Headers),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
