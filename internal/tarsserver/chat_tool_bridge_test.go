package tarsserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/agent"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/devlikebear/tars/internal/toolbridge"
	"github.com/rs/zerolog"
)

func bridgeTestRegistry(names ...string) (*tool.Registry, []llm.ToolSchema) {
	registry := tool.NewRegistry()
	for _, name := range names {
		name := name
		registry.Register(tool.Tool{
			Name:        name,
			Description: name + " tool",
			Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
			Execute: func(context.Context, json.RawMessage) (tool.Result, error) {
				return tool.Result{Content: []tool.ContentBlock{{Type: "text", Text: name + " ran"}}}, nil
			},
		})
	}
	return registry, registry.SchemasForNames(names)
}

func toolNames(list []tool.Tool) []string {
	names := make([]string, 0, len(list))
	for _, tl := range list {
		names = append(names, tl.Name)
	}
	sort.Strings(names)
	return names
}

// Claude Code has its own file, shell and web tools, and its permission
// rules cover only those: a second exec or write_file would be a way around
// them.
func TestBridgedToolsLeaveOutWhatClaudeCodeHasItself(t *testing.T) {
	registry, schemas := bridgeTestRegistry(
		"memory_search", "cron_create", "tasks", "computer_use",
		"read_file", "write_file", "edit_file", "apply_patch", "list_dir", "glob", "project_skill",
		"exec", "process", "web_search", "web_fetch",
		// Claude Code mounts this server itself.
		"mcp.github.list_issues",
	)
	mounted := []llm.ClaudeCodeMCPServer{{Name: "GitHub", Command: "gh-mcp"}}
	served, renamed := bridgedTools(registry, schemas, mounted)
	got := strings.Join(toolNames(served), ",")
	if got != "computer_use,cron_create,memory_search,tasks" || len(renamed) != 0 {
		t.Fatalf("bridged tools = %s, renamed = %v", got, renamed)
	}
	// Only what the turn was given is served, whatever else is registered.
	served, _ = bridgedTools(registry, schemas[:1], mounted)
	if got = strings.Join(toolNames(served), ","); got != "memory_search" {
		t.Fatalf("bridged tools for one injected schema = %s", got)
	}
	if served, renamed = bridgedTools(nil, schemas, mounted); served != nil || renamed != nil {
		t.Fatal("no registry, no tools")
	}
}

// Claude Code's --mcp-config has no shape for a websocket server, so it is
// never mounted. TARS is connected to it already and serves its tools.
func TestBridgedToolsServeTheMCPServersClaudeCodeCannotMount(t *testing.T) {
	registry, schemas := bridgeTestRegistry(
		"tasks",
		"mcp.github.list_issues",
		"mcp.home-ws.lights.on",
		"mcp.home-ws.read state",
	)
	mounted := []llm.ClaudeCodeMCPServer{{Name: "github", Command: "gh-mcp"}}
	served, renamed := bridgedTools(registry, schemas, mounted)
	got := strings.Join(toolNames(served), ",")
	if got != "home-ws__lights__on,home-ws__read_state,tasks" {
		t.Fatalf("bridged tools = %s", got)
	}
	if renamed["home-ws__lights__on"] != "mcp.home-ws.lights.on" || renamed["home-ws__read_state"] != "mcp.home-ws.read state" || len(renamed) != 2 {
		t.Fatalf("renamed = %v", renamed)
	}
	// A server whose name is only a prefix of another's is not mistaken for it.
	served, _ = bridgedTools(registry, schemas, []llm.ClaudeCodeMCPServer{{Name: "home"}, {Name: " "}})
	if got = strings.Join(toolNames(served), ","); got != "github__list_issues,home-ws__lights__on,home-ws__read_state,tasks" {
		t.Fatalf("bridged tools with an unrelated server mounted = %s", got)
	}

	// The call reaches the tool, and what TARS records is the tool's own name.
	deps := bridgeTestDeps(t)
	state := bridgeTestState("claude-code-cli", "mcp.home-ws.lights.on")
	var after []agent.Event
	bridge := openChatToolBridge(context.Background(), deps, state, state.injectedSchemas, func(_ context.Context, evt agent.Event) { after = append(after, evt) })
	defer bridge.close()
	tars := bridge.servers[len(bridge.servers)-1]
	req, _ := http.NewRequest(http.MethodPost, tars.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"home-ws__lights__on","arguments":{}}}`))
	req.Header.Set("Authorization", tars.Headers["Authorization"])
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), "mcp.home-ws.lights.on ran") {
		t.Fatalf("tools/call = %s", body)
	}
	if len(after) != 1 || after[0].ToolName != "mcp.home-ws.lights.on" {
		t.Fatalf("after-tool hook saw %+v", after)
	}
}

func bridgeTestState(provider string, names ...string) chatRunState {
	registry, schemas := bridgeTestRegistry(names...)
	return chatRunState{
		sessionID:            "s1",
		registry:             registry,
		injectedSchemas:      schemas,
		llmResolution:        llm.TierResolution{Provider: provider},
		claudeCodeMCPServers: []llm.ClaudeCodeMCPServer{{Name: "github", Command: "gh-mcp"}},
	}
}

func bridgeTestDeps(t *testing.T) chatHandlerDeps {
	t.Helper()
	bridge := toolbridge.New()
	t.Cleanup(func() { _ = bridge.Close() })
	return chatHandlerDeps{logger: zerolog.Nop(), tooling: chatToolingOptions{ToolBridge: bridge}}
}

func TestOpenChatToolBridgeMountsTarsNextToTheSessionServers(t *testing.T) {
	deps := bridgeTestDeps(t)
	state := bridgeTestState("claude-code-cli", "memory_search", "tasks", "exec")
	var after []agent.Event
	bridge := openChatToolBridge(context.Background(), deps, state, state.injectedSchemas, func(_ context.Context, evt agent.Event) { after = append(after, evt) })
	defer bridge.close()

	if len(bridge.servers) != 2 || bridge.servers[0].Name != "github" || bridge.hold == nil {
		t.Fatalf("servers = %+v, hold = %v", bridge.servers, bridge.hold)
	}
	tars := bridge.servers[1]
	if tars.Name != "tars" || tars.Transport != "http" || !strings.HasPrefix(tars.URL, "http://127.0.0.1:") || !strings.HasPrefix(tars.Headers["Authorization"], "Bearer ") {
		t.Fatalf("tars server = %+v", tars)
	}
	if len(state.claudeCodeMCPServers) != 1 {
		t.Fatalf("the session's own server list was changed: %+v", state.claudeCodeMCPServers)
	}

	rpc := func(body string) (int, string) {
		req, _ := http.NewRequest(http.MethodPost, tars.URL, strings.NewReader(body))
		req.Header.Set("Authorization", tars.Headers["Authorization"])
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		data, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(data)
	}
	_, list := rpc(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if !strings.Contains(list, `"memory_search"`) || !strings.Contains(list, `"tasks"`) || strings.Contains(list, `"exec"`) {
		t.Fatalf("tools/list = %s", list)
	}
	_, called := rpc(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"tasks","arguments":{}}}`)
	if !strings.Contains(called, "tasks ran") {
		t.Fatalf("tools/call = %s", called)
	}
	if len(after) != 1 || after[0].Type != agent.EventAfterTool || after[0].ToolName != "tasks" || after[0].ToolResult != "tasks ran" {
		t.Fatalf("after-tool hook saw %+v", after)
	}

	bridge.close()
	if status, _ := rpc(`{"jsonrpc":"2.0","id":3,"method":"tools/list"}`); status != http.StatusUnauthorized {
		t.Fatalf("the turn's token still works after the turn: %d", status)
	}
}

func TestOpenChatToolBridgeLeavesOtherTurnsAlone(t *testing.T) {
	deps := bridgeTestDeps(t)
	off := chatHandlerDeps{logger: zerolog.Nop()}
	named := bridgeTestState("claude-code-cli", "memory_search")
	named.claudeCodeMCPServers = []llm.ClaudeCodeMCPServer{{Name: "TARS", Command: "mine"}}
	cases := map[string]struct {
		deps  chatHandlerDeps
		state chatRunState
	}{
		"native provider":           {deps, bridgeTestState("anthropic", "memory_search")},
		"another cli provider":      {deps, bridgeTestState("antigravity-cli", "memory_search")},
		"bridge turned off":         {off, bridgeTestState("claude-code-cli", "memory_search")},
		"only claude code's own":    {deps, bridgeTestState("claude-code-cli", "exec", "read_file", "web_fetch")},
		"a session server named so": {deps, named},
		"no tools injected at all":  {deps, bridgeTestState("claude-code-cli")},
	}
	for name, tc := range cases {
		bridge := openChatToolBridge(context.Background(), tc.deps, tc.state, tc.state.injectedSchemas, nil)
		if bridge.hold != nil || bridge.done != nil || len(bridge.servers) != len(tc.state.claudeCodeMCPServers) {
			t.Errorf("%s: bridge = %+v", name, bridge)
		}
		for i, server := range bridge.servers {
			if server.Name != tc.state.claudeCodeMCPServers[i].Name || server.URL != "" {
				t.Errorf("%s: servers = %+v", name, bridge.servers)
			}
		}
		bridge.close()
	}
}

// A cron job may narrow its session's tools; the bridge serves the narrowed
// set, and the run options carry it to the provider.
func TestToolBridgeServesTheRunsOwnToolsAndFillsRunOptions(t *testing.T) {
	deps := bridgeTestDeps(t)
	state := bridgeTestState("claude-code-cli", "memory_search", "tasks", "cron")
	bridge := openChatToolBridge(context.Background(), deps, state, state.injectedSchemas[:1], nil)
	defer bridge.close()

	var opts agent.RunOptions
	bridge.apply(&opts)
	if len(opts.ClaudeCodeMCPServers) != 2 || opts.ClaudeCodeMCPServers[1].Name != "tars" || opts.ClaudeCodeTurnHold == nil {
		t.Fatalf("run options = %+v", opts)
	}
	tars := opts.ClaudeCodeMCPServers[1]
	req, _ := http.NewRequest(http.MethodPost, tars.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set("Authorization", tars.Headers["Authorization"])
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if list := string(body); !strings.Contains(list, `"memory_search"`) || strings.Contains(list, `"tasks"`) || strings.Contains(list, `"cron"`) {
		t.Fatalf("tools/list = %s", list)
	}

	// A run without a bridge still hands over the servers it came with.
	var plain agent.RunOptions
	chatToolBridge{servers: state.claudeCodeMCPServers}.apply(&plain)
	if len(plain.ClaudeCodeMCPServers) != 1 || plain.ClaudeCodeTurnHold != nil {
		t.Fatalf("plain run options = %+v", plain)
	}
}

func TestToolBridgeForFollowsTheConfig(t *testing.T) {
	if toolBridgeFor(config.Config{}) != nil {
		t.Fatal("the bridge must be off when the config turns it off")
	}
	on := config.Config{}
	on.ClaudeCodeCLITarsTools = true
	first, second := toolBridgeFor(on), toolBridgeFor(on)
	if first == nil || first != second {
		t.Fatal("every run shares one bridge")
	}
}
