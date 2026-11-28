package tarsserver

import (
	"context"
	"strings"

	"github.com/devlikebear/tars/internal/agent"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/mcp"
	"github.com/devlikebear/tars/internal/secrets"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/devlikebear/tars/internal/toolbridge"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/rs/zerolog"
)

// TARS tools for a claude-code-cli run.
//
// The CLI runs its own tools and ignores the tool schemas a native provider
// is given, so without this a claude-code-cli run has no memory_search, no
// cron, no sessions or tasks tools — only what its skills can reach through
// Bash. For the length of one run the server opens the run's tools on the
// loopback MCP bridge (internal/toolbridge) and mounts it as one more MCP
// server, "tars", next to the session's own. Claude Code calls them as
// mcp__tars__<name>; each call runs here with the run's context.
//
// Every place that runs the agent loop opens one: a console chat turn, a
// cron prompt in its session, a Telegram turn, a pulse auto-resume and a
// subagent run (openToolBridge; a chat turn goes through openChatToolBridge).
//
// What is served is the run's injected set — already narrowed by the
// caller's role, the session's tool config and .tars overrides — minus the
// tools Claude Code has itself and the tools of the MCP servers Claude Code
// mounts directly. Serving exec or write_file a second time would hand the
// model a way around Claude Code's own permission rules and the session's
// deny list, which only cover Claude Code's tools.
//
// An MCP server Claude Code cannot mount (a websocket one: --mcp-config has
// no shape for it) is the exception among MCP tools. TARS is connected to it
// already, so its tools are served here under <server>__<tool>.
//
// A served call is asked about by Claude Code's permission system like any
// other MCP tool (the console's permission card, or the ops queue for an
// unattended run), so the native tool gate is not applied a second time.

// claudeCodeNativeToolGroups are the tool groups Claude Code covers with its
// own tools (Read/Write/Edit/Glob, Bash, WebFetch/WebSearch).
var claudeCodeNativeToolGroups = map[string]bool{"files": true, "shell": true, "web": true}

const chatToolBridgeInstructions = "TARS's own tools: memory, schedules, sessions, tasks and the like. " +
	"Where TARS's instructions name one of them (memory_search, for example), call it here as mcp__" + toolbridge.ServerName + "__<name>. " +
	"Use Claude Code's own tools for files, shell commands and the web."

// processToolBridge is the one listener every run in this process shares. It
// listens only once a run opens on it.
var processToolBridge = toolbridge.New()

// toolBridgeFor returns the bridge runs should use under cfg, or nil when
// serving TARS tools to claude-code-cli is turned off.
func toolBridgeFor(cfg config.Config) *toolbridge.Bridge {
	if !cfg.ClaudeCodeCLITarsTools {
		return nil
	}
	return processToolBridge
}

// chatToolBridge is one run's open bridge. The zero value is a run without
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

// apply hands the run's MCP servers (the bridge among them, when it opened)
// and its idle hold to the agent loop.
func (b chatToolBridge) apply(opts *agent.RunOptions) {
	opts.ClaudeCodeMCPServers = b.servers
	opts.ClaudeCodeTurnHold = b.hold
}

// mountedMCPToolPrefixes are the registry-name prefixes ("mcp.<server>.") of
// the servers Claude Code mounts itself.
func mountedMCPToolPrefixes(servers []llm.ClaudeCodeMCPServer) []string {
	prefixes := make([]string, 0, len(servers))
	for _, server := range servers {
		if name := strings.TrimSpace(server.Name); name != "" {
			prefixes = append(prefixes, strings.TrimSuffix(mcp.MCPToolName(name, "x"), "x"))
		}
	}
	return prefixes
}

// bridgedMCPToolName is the name an unmounted MCP server's tool is served
// under: mcp.<server>.<tool> as <server>__<tool>, in the characters a Claude
// Code tool name may have.
func bridgedMCPToolName(canonical string) string {
	var b strings.Builder
	for _, r := range strings.TrimPrefix(canonical, "mcp.") {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		case r == '.':
			b.WriteString("__")
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// bridgedTools picks the tools a claude-code-cli run is served: the injected
// ones Claude Code has no tool of its own for and does not already reach
// through a server in mounted. The second result maps a served name that
// differs from the tool's own name back to it.
func bridgedTools(registry *tool.Registry, injected []llm.ToolSchema, mounted []llm.ClaudeCodeMCPServer) ([]tool.Tool, map[string]string) {
	if registry == nil {
		return nil, nil
	}
	prefixes := mountedMCPToolPrefixes(mounted)
	var out []tool.Tool
	renamed := map[string]string{}
	seen := map[string]bool{}
	served := map[string]bool{}
	for _, schema := range injected {
		name := strings.TrimSpace(schema.Function.Name)
		if name == "" || seen[name] || claudeCodeNativeToolGroups[tool.ToolGroupForName(name)] {
			continue
		}
		seen[name] = true
		tl, ok := registry.Get(name)
		if !ok || tl.Execute == nil {
			continue
		}
		if canonical := tool.CanonicalToolName(name); strings.HasPrefix(canonical, "mcp.") {
			// A mounted server's tools reach Claude Code through the server
			// itself; serving them again would list each one twice.
			if hasAnyPrefix(canonical, prefixes) {
				continue
			}
			tl.Name = bridgedMCPToolName(canonical)
			if tl.Name == "" {
				continue
			}
			renamed[tl.Name] = name
		}
		if served[tl.Name] {
			continue
		}
		served[tl.Name] = true
		out = append(out, tl)
	}
	return out, renamed
}

func hasAnyPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

// toolBridgeRun is one agent-loop run asking for its TARS tools.
type toolBridgeRun struct {
	bridge *toolbridge.Bridge
	// provider is the kind of the provider the run uses.
	provider string
	registry *tool.Registry
	// schemas are the tools the run was given.
	schemas []llm.ToolSchema
	// servers are the MCP servers the run mounts by itself.
	servers []llm.ClaudeCodeMCPServer
	// owner names the run in logs and usage signals: a session ID or a run
	// label.
	owner   string
	tracker *usage.Tracker
	logger  zerolog.Logger
	// afterTool, when set, hears about each served call as an after-tool
	// event (without a tool call ID: the provider does not send one).
	afterTool func(context.Context, agent.Event)
}

// openToolBridge opens the bridge for a claude-code-cli run. Any reason not
// to (another provider, the bridge turned off, nothing to serve, a mounted
// MCP server already named "tars", a listener that will not start) leaves
// the run with the servers it came with.
func openToolBridge(ctx context.Context, run toolBridgeRun) chatToolBridge {
	plain := chatToolBridge{servers: run.servers}
	if run.bridge == nil || run.provider != "claude-code-cli" {
		return plain
	}
	for _, server := range run.servers {
		if strings.EqualFold(strings.TrimSpace(server.Name), toolbridge.ServerName) {
			run.logger.Warn().Str("owner", run.owner).
				Msg("tool bridge: an MCP server named \"tars\" is configured; TARS tools are not served to this run")
			return plain
		}
	}
	served, renamed := bridgedTools(run.registry, run.schemas, run.servers)
	if len(served) == 0 {
		return plain
	}
	hold := &llm.TurnHold{}
	endpoint, done, err := run.bridge.Open(toolbridge.Turn{
		Context:      ctx,
		Tools:        served,
		Instructions: chatToolBridgeInstructions,
		Redact:       secrets.RedactText,
		Busy:         hold.Begin,
		AfterCall: func(callCtx context.Context, call toolbridge.Call) {
			// The tool card comes from the CLI's own tool_use report; this
			// is the part only a TARS-run tool has: the usage signal and
			// whatever the caller hangs on a finished tool.
			name := call.Name
			if original, ok := renamed[name]; ok {
				name = original
			}
			evt := agent.Event{
				Type:        agent.EventAfterTool,
				ToolName:    name,
				ToolArgs:    call.Args,
				ToolResult:  call.Result,
				ToolIsError: call.IsError,
			}
			recordToolUsageSignal(callCtx, run.tracker, run.owner, evt)
			if run.afterTool != nil {
				run.afterTool(callCtx, evt)
			}
		},
	})
	if err != nil {
		run.logger.Warn().Err(err).Str("owner", run.owner).Msg("tool bridge: not opened; the run goes on without TARS tools")
		return plain
	}
	servers := make([]llm.ClaudeCodeMCPServer, 0, len(run.servers)+1)
	servers = append(servers, run.servers...)
	servers = append(servers, llm.ClaudeCodeMCPServer{
		Name:      toolbridge.ServerName,
		Transport: "http",
		URL:       endpoint.URL,
		Headers:   endpoint.Headers(),
	})
	run.logger.Debug().Str("owner", run.owner).Int("tools", len(served)).Msg("tool bridge: opened")
	return chatToolBridge{servers: servers, hold: hold, done: done}
}

// openChatToolBridge opens the bridge for a run built from a chat run state
// (a console turn, a cron prompt in its session, a pulse auto-resume).
// schemas are the tools the run was given, which a cron job may have narrowed
// from the state's injected set.
func openChatToolBridge(ctx context.Context, deps chatHandlerDeps, state chatRunState, schemas []llm.ToolSchema, afterTool func(context.Context, agent.Event)) chatToolBridge {
	return openToolBridge(ctx, toolBridgeRun{
		bridge:    deps.tooling.ToolBridge,
		provider:  state.llmResolution.Provider,
		registry:  state.registry,
		schemas:   schemas,
		servers:   state.claudeCodeMCPServers,
		owner:     state.sessionID,
		tracker:   deps.tooling.UsageTracker,
		logger:    deps.logger,
		afterTool: afterTool,
	})
}
