package tarsserver

import (
	"context"
	"strings"

	"github.com/devlikebear/tars/internal/agent"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/secrets"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/devlikebear/tars/internal/toolbridge"
)

// TARS tools for a claude-code-cli turn.
//
// The CLI runs its own tools and ignores the tool schemas a native provider
// is given, so without this a claude-code-cli session has no memory_search,
// no cron, no sessions or tasks tools — only what its skills can reach
// through Bash. For the length of one turn the server opens the turn's tools
// on the loopback MCP bridge (internal/toolbridge) and mounts it as one more
// MCP server, "tars", next to the session's own. Claude Code calls them as
// mcp__tars__<name>; each call runs here with the turn's context.
//
// What is served is the turn's injected set — already narrowed by the
// caller's role, the session's tool config and .tars overrides — minus the
// tools Claude Code has itself and the tools of the session's MCP servers,
// which Claude Code mounts directly. Serving exec or write_file a second
// time would hand the model a way around Claude Code's own permission rules
// and the session's deny list, which only cover Claude Code's tools.
//
// A served call is asked about by Claude Code's permission system like any
// other MCP tool (the console's permission card, or the ops queue for an
// unattended turn), so the native tool gate is not applied a second time.

// claudeCodeNativeToolGroups are the tool groups Claude Code covers with its
// own tools (Read/Write/Edit/Glob, Bash, WebFetch/WebSearch).
var claudeCodeNativeToolGroups = map[string]bool{"files": true, "shell": true, "web": true}

const chatToolBridgeInstructions = "TARS's own tools: memory, schedules, sessions, tasks and the like. " +
	"Where TARS's instructions name one of them (memory_search, for example), call it here as mcp__" + toolbridge.ServerName + "__<name>. " +
	"Use Claude Code's own tools for files, shell commands and the web."

// chatToolBridge is one turn's open bridge. The zero value is a turn without
// one: servers is then the session's own MCP servers and close does nothing.
type chatToolBridge struct {
	servers []llm.ClaudeCodeMCPServer
	hold    *llm.TurnHold
	done    func()
}

func (b chatToolBridge) close() {
	if b.done != nil {
		b.done()
	}
}

// bridgedTools picks the tools a claude-code-cli turn is served: the
// injected ones Claude Code has no tool of its own for.
func bridgedTools(registry *tool.Registry, injected []llm.ToolSchema) []tool.Tool {
	if registry == nil {
		return nil
	}
	var out []tool.Tool
	seen := map[string]bool{}
	for _, schema := range injected {
		name := strings.TrimSpace(schema.Function.Name)
		if name == "" || seen[name] || claudeCodeNativeToolGroups[tool.ToolGroupForName(name)] {
			continue
		}
		// An MCP server's tools (registered here as mcp.<server>.<tool>)
		// reach Claude Code through the server itself, mounted by
		// --mcp-config; serving them again would list each one twice.
		if strings.HasPrefix(tool.CanonicalToolName(name), "mcp.") {
			continue
		}
		seen[name] = true
		if tl, ok := registry.Get(name); ok && tl.Execute != nil {
			out = append(out, tl)
		}
	}
	return out
}

// openChatToolBridge opens the bridge for a claude-code-cli turn. Any reason
// not to (another provider, the bridge turned off, nothing to serve, a
// session MCP server already named "tars", a listener that will not start)
// leaves the turn as it was before the bridge existed.
func openChatToolBridge(ctx context.Context, deps chatHandlerDeps, state chatRunState, afterTool func(context.Context, agent.Event)) chatToolBridge {
	plain := chatToolBridge{servers: state.claudeCodeMCPServers}
	if deps.tooling.ToolBridge == nil || state.llmResolution.Provider != "claude-code-cli" {
		return plain
	}
	for _, server := range state.claudeCodeMCPServers {
		if strings.EqualFold(strings.TrimSpace(server.Name), toolbridge.ServerName) {
			deps.logger.Warn().Str("session_id", state.sessionID).
				Msg("tool bridge: an MCP server named \"tars\" is configured; TARS tools are not served to this turn")
			return plain
		}
	}
	served := bridgedTools(state.registry, state.injectedSchemas)
	if len(served) == 0 {
		return plain
	}
	hold := &llm.TurnHold{}
	endpoint, done, err := deps.tooling.ToolBridge.Open(toolbridge.Turn{
		Context:      ctx,
		Tools:        served,
		Instructions: chatToolBridgeInstructions,
		Redact:       secrets.RedactText,
		Busy:         hold.Begin,
		AfterCall: func(callCtx context.Context, call toolbridge.Call) {
			// The tool card comes from the CLI's own tool_use report; this
			// is the part only a TARS-run tool has: the usage signal and the
			// tasks badge.
			evt := agent.Event{
				Type:        agent.EventAfterTool,
				ToolName:    call.Name,
				ToolArgs:    call.Args,
				ToolResult:  call.Result,
				ToolIsError: call.IsError,
			}
			recordToolUsageSignal(callCtx, deps.tooling.UsageTracker, state.sessionID, evt)
			if afterTool != nil {
				afterTool(callCtx, evt)
			}
		},
	})
	if err != nil {
		deps.logger.Warn().Err(err).Str("session_id", state.sessionID).Msg("tool bridge: not opened; the turn runs without TARS tools")
		return plain
	}
	servers := make([]llm.ClaudeCodeMCPServer, 0, len(state.claudeCodeMCPServers)+1)
	servers = append(servers, state.claudeCodeMCPServers...)
	servers = append(servers, llm.ClaudeCodeMCPServer{
		Name:      toolbridge.ServerName,
		Transport: "http",
		URL:       endpoint.URL,
		Headers:   endpoint.Headers(),
	})
	deps.logger.Debug().Str("session_id", state.sessionID).Int("tools", len(served)).Msg("tool bridge: opened")
	return chatToolBridge{servers: servers, hold: hold, done: done}
}
