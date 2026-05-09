package tarsserver

import (
	"bytes"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/mcp"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
)

func TestBuildChatToolRegistry_UsesSharedUsageTrackerWithoutDuplicateWarning(t *testing.T) {
	workspace := t.TempDir()
	tracker, err := usage.NewTracker(workspace, usage.TrackerOptions{})
	if err != nil {
		t.Fatalf("new usage tracker: %v", err)
	}

	var logs bytes.Buffer
	prevLogger := zlog.Logger
	zlog.Logger = zerolog.New(&logs).Level(zerolog.DebugLevel)
	t.Cleanup(func() {
		zlog.Logger = prevLogger
	})

	registry := buildChatToolRegistry(
		session.NewStore(workspace),
		"default",
		"sess-1",
		workspace,
		tool.SingleDirPolicy(workspace),
		nil,
		chatHandlerDeps{
			tooling: chatToolingOptions{
				UsageTracker: tracker,
			},
		},
	)
	if _, ok := registry.Get("usage_report"); !ok {
		t.Fatal("expected usage_report to remain registered")
	}
	if strings.Contains(logs.String(), "tool registered with duplicate name") {
		t.Fatalf("expected no duplicate tool warning, got logs:\n%s", logs.String())
	}
}

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
