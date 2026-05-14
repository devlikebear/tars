package tarsserver

import (
	"reflect"
	"sort"
	"testing"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/llm"
)

func TestToClaudeCodeMCPServers_StdioAndRemote(t *testing.T) {
	got := toClaudeCodeMCPServers([]config.MCPServer{
		{
			Name:    "fs",
			Command: "/usr/bin/mcp-fs",
			Args:    []string{"--root", "/tmp"},
			Env:     map[string]string{"DEBUG": "1"},
		},
		{
			Name:      "remote-http",
			Transport: config.MCPTransportStreamableHTTP,
			URL:       "https://mcp.example.com/streamable",
			Headers:   map[string]string{"Authorization": "Bearer x"},
		},
		{
			Name:      "remote-sse",
			Transport: config.MCPTransportSSE,
			URL:       "https://mcp.example.com/sse",
		},
		{
			// websocket is unsupported by Claude Code's --mcp-config; the
			// helper must silently skip rather than emit a broken server.
			Name:      "ws-skipped",
			Transport: config.MCPTransportWebSocket,
			URL:       "wss://mcp.example.com/ws",
		},
	})

	if len(got) != 3 {
		t.Fatalf("expected 3 servers (ws dropped), got %d: %+v", len(got), got)
	}

	byName := make(map[string]llm.ClaudeCodeMCPServer, len(got))
	for _, srv := range got {
		byName[srv.Name] = srv
	}

	fs, ok := byName["fs"]
	if !ok {
		t.Fatal("fs missing")
	}
	// stdio in TARS canonical form maps to empty Transport, so the provider
	// defaults to stdio when materializing --mcp-config.
	if fs.Transport != "" {
		t.Fatalf("stdio fs should map to empty Transport, got %q", fs.Transport)
	}
	if fs.Command != "/usr/bin/mcp-fs" {
		t.Fatalf("fs command: %q", fs.Command)
	}
	if !reflect.DeepEqual(fs.Args, []string{"--root", "/tmp"}) {
		t.Fatalf("fs args: %v", fs.Args)
	}
	if fs.Env["DEBUG"] != "1" {
		t.Fatalf("fs env DEBUG: %v", fs.Env)
	}

	http := byName["remote-http"]
	if http.Transport != "http" {
		t.Fatalf("streamable_http should map to 'http' for Claude Code, got %q", http.Transport)
	}
	if http.URL != "https://mcp.example.com/streamable" {
		t.Fatalf("remote-http url: %q", http.URL)
	}
	if http.Headers["Authorization"] != "Bearer x" {
		t.Fatalf("remote-http headers: %v", http.Headers)
	}

	sse := byName["remote-sse"]
	if sse.Transport != "sse" {
		t.Fatalf("sse should map to 'sse', got %q", sse.Transport)
	}

	if _, ok := byName["ws-skipped"]; ok {
		t.Fatal("websocket transport should be silently skipped")
	}
}

func TestToClaudeCodeMCPServers_FiltersEmptyAndDisabled(t *testing.T) {
	got := toClaudeCodeMCPServers([]config.MCPServer{
		// empty name → drop
		{Name: "", Command: "x"},
		{Name: "   ", URL: "https://x"},
		// no command and no url → NormalizeMCPServer + MCPServerEnabled drops it
		{Name: "broken"},
		// valid
		{Name: "ok", Command: "/bin/true"},
	})

	if len(got) != 1 {
		names := make([]string, 0, len(got))
		for _, s := range got {
			names = append(names, s.Name)
		}
		sort.Strings(names)
		t.Fatalf("expected only 'ok' to survive, got %v", names)
	}
	if got[0].Name != "ok" {
		t.Fatalf("expected 'ok', got %q", got[0].Name)
	}
}

func TestToClaudeCodeMCPServers_NilOnEmptyInput(t *testing.T) {
	if got := toClaudeCodeMCPServers(nil); got != nil {
		t.Fatalf("expected nil for nil input, got %+v", got)
	}
	if got := toClaudeCodeMCPServers([]config.MCPServer{}); got != nil {
		t.Fatalf("expected nil for empty input, got %+v", got)
	}
	if got := toClaudeCodeMCPServers([]config.MCPServer{{Name: ""}}); got != nil {
		t.Fatalf("expected nil when all entries get filtered, got %+v", got)
	}
}

func TestToClaudeCodeMCPServers_DefensiveCopies(t *testing.T) {
	src := []config.MCPServer{
		{
			Name:    "fs",
			Command: "/bin/x",
			Args:    []string{"a", "b"},
			Env:     map[string]string{"K": "V"},
		},
	}
	got := toClaudeCodeMCPServers(src)
	if len(got) != 1 {
		t.Fatalf("expected 1 server, got %d", len(got))
	}
	// Mutate originals; the converted value must not change.
	src[0].Args[0] = "MUTATED"
	src[0].Env["K"] = "MUTATED"
	if got[0].Args[0] != "a" {
		t.Fatalf("converted Args slice should be a defensive copy, got %v", got[0].Args)
	}
	if got[0].Env["K"] != "V" {
		t.Fatalf("converted Env map should be a defensive copy, got %v", got[0].Env)
	}
}
