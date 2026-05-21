package mcp_test

import (
	"context"
	"testing"

	"github.com/devlikebear/tars/pkg/mcp"
)

func TestClientNoServersUsesPublicTypes(t *testing.T) {
	client := mcp.NewClient(nil)
	servers, err := client.ListServers(context.Background())
	if err != nil {
		t.Fatalf("ListServers() error = %v", err)
	}
	if len(servers) != 0 {
		t.Fatalf("len(servers) = %d, want 0", len(servers))
	}
	built, err := client.BuildTools(context.Background())
	if err != nil {
		t.Fatalf("BuildTools() error = %v", err)
	}
	if len(built) != 0 {
		t.Fatalf("len(tools) = %d, want 0", len(built))
	}
}

func TestMCPToolName(t *testing.T) {
	if got := mcp.MCPToolName("File System", "read/file"); got != "mcp.file_system.read_file" {
		t.Fatalf("MCPToolName() = %q", got)
	}
}

func TestClientMethodsUsePublicConfig(t *testing.T) {
	ctx := context.Background()
	client := mcp.NewClient([]mcp.ServerConfig{{
		Name:      "blocked",
		Command:   "missing-mcp-command",
		Args:      []string{"--flag"},
		Env:       map[string]string{"A": "B"},
		Transport: mcp.TransportStdio,
		Headers:   map[string]string{"X-Test": "1"},
		Source:    "test",
	}})
	client.SetCommandAllowlist([]string{})
	statuses, err := client.ListServers(ctx)
	if err != nil {
		t.Fatalf("ListServers() error = %v", err)
	}
	if len(statuses) != 1 || statuses[0].Name != "blocked" {
		t.Fatalf("statuses = %+v", statuses)
	}
	if _, err := client.ListTools(ctx); err == nil {
		t.Fatalf("ListTools() expected blocked command error")
	}
	if _, err := client.CallTool(ctx, "missing", "tool", map[string]any{}); err == nil {
		t.Fatalf("CallTool() expected missing server error")
	}
	client.SetServers([]mcp.ServerConfig{{
		Name:      "remote",
		Transport: mcp.TransportStreamableHTTP,
		URL:       "http://127.0.0.1:1/mcp",
	}})
	client.Close()

	var nilClient *mcp.Client
	if servers, err := nilClient.ListServers(ctx); err != nil || len(servers) != 0 {
		t.Fatalf("nil ListServers() = %+v, %v", servers, err)
	}
	nilClient.SetCommandAllowlist(nil)
	nilClient.SetServers(nil)
	nilClient.Close()
}
