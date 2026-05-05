package tarsserver

import (
	"testing"

	"github.com/devlikebear/tars/internal/mcp"
	"github.com/devlikebear/tars/internal/session"
)

func TestResolveSessionToolPolicyFiltersMCPServers(t *testing.T) {
	echoTool := mcp.MCPToolName("echo", "reply")
	clockTool := mcp.MCPToolName("clock", "now")
	resolved := resolveSessionToolPolicy([]string{"read_file", echoTool, clockTool}, session.SessionToolConfig{
		MCPCustom:  true,
		MCPEnabled: []string{"echo"},
	}, "session")

	if !containsString(resolved.Allowed, "read_file") {
		t.Fatalf("expected built-in tools to remain allowed, got %+v", resolved.Allowed)
	}
	if !containsString(resolved.Allowed, echoTool) {
		t.Fatalf("expected echo MCP tool to remain allowed, got %+v", resolved.Allowed)
	}
	if containsString(resolved.Allowed, clockTool) {
		t.Fatalf("expected clock MCP tool to be blocked, got %+v", resolved.Allowed)
	}
	if _, ok := resolved.Blocked[clockTool]; !ok {
		t.Fatalf("expected blocked reason for %s, got %+v", clockTool, resolved.Blocked)
	}
}

func TestResolveSessionToolPolicyCanDisableAllMCPServers(t *testing.T) {
	echoTool := mcp.MCPToolName("echo", "reply")
	resolved := resolveSessionToolPolicy([]string{"read_file", echoTool}, session.SessionToolConfig{
		MCPCustom: true,
	}, "session")

	if !containsString(resolved.Allowed, "read_file") {
		t.Fatalf("expected built-in tools to remain allowed, got %+v", resolved.Allowed)
	}
	if containsString(resolved.Allowed, echoTool) {
		t.Fatalf("expected MCP tool to be blocked, got %+v", resolved.Allowed)
	}
	if _, ok := resolved.Blocked[echoTool]; !ok {
		t.Fatalf("expected blocked reason for %s, got %+v", echoTool, resolved.Blocked)
	}
}
