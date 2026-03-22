package config

import "testing"

func TestLoad_MCPServersFromEnv_SupportsRemoteTransportAndAuth(t *testing.T) {
	t.Setenv("MCP_SERVERS_JSON", `[
	  {
	    "name":"remote-http",
	    "transport":"streamable_http",
	    "url":"https://mcp.example.com",
	    "headers":{"X-Team":"core"},
	    "auth_mode":"bearer",
	    "auth_token_env":"MCP_HTTP_TOKEN"
	  },
	  {
	    "name":"remote-ws",
	    "transport":"websocket",
	    "url":"wss://mcp.example.com/ws",
	    "auth_mode":"oauth",
	    "oauth_provider":"claude-code"
	  }
	]`)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.MCPServers) != 2 {
		t.Fatalf("expected 2 mcp servers, got %d", len(cfg.MCPServers))
	}
	httpServer := cfg.MCPServers[0]
	if httpServer.Transport != "streamable_http" || httpServer.URL != "https://mcp.example.com" {
		t.Fatalf("unexpected http mcp server: %+v", httpServer)
	}
	if httpServer.Headers["X-Team"] != "core" {
		t.Fatalf("unexpected headers: %+v", httpServer.Headers)
	}
	if httpServer.AuthMode != "bearer" || httpServer.AuthTokenEnv != "MCP_HTTP_TOKEN" {
		t.Fatalf("unexpected bearer auth config: %+v", httpServer)
	}
	wsServer := cfg.MCPServers[1]
	if wsServer.Transport != "websocket" || wsServer.URL != "wss://mcp.example.com/ws" {
		t.Fatalf("unexpected websocket mcp server: %+v", wsServer)
	}
	if wsServer.AuthMode != "oauth" || wsServer.OAuthProvider != "claude-code" {
		t.Fatalf("unexpected oauth config: %+v", wsServer)
	}
}
