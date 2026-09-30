package tarsserver

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/devlikebear/tars/pkg/agentloop"
)

// chatToolGate asks the console before a native-provider turn runs a
// high-risk TARS tool (exec, file writes and edits, ...; see
// tool.IsHighRiskToolName). Read-only tools run as before. It is the native
// counterpart of the claude-code-cli permission handler and streams the same
// permission_request event, so the console does not care which provider
// asked (#970).
type chatToolGate struct {
	broker    *chatPermissionBroker
	sessionID string
	// cwd is the folder the turn works in, which "always allow" covers.
	cwd    string
	stream *chatStreamWriter
}

func newChatToolGate(broker *chatPermissionBroker, sessionID, cwd string, stream *chatStreamWriter) *chatToolGate {
	return &chatToolGate{broker: broker, sessionID: sessionID, cwd: cwd, stream: stream}
}

func (g *chatToolGate) Authorize(ctx context.Context, call agentloop.ToolCallRequest) (agentloop.ToolDecision, error) {
	name := tool.CanonicalToolName(call.ToolName)
	if !tool.IsHighRiskToolName(name) || g.broker.allows(g.sessionID, name, call.ToolArgs) || g.broker.alwaysAllows(g.cwd, name, call.ToolArgs) {
		return agentloop.ToolDecision{Allow: true}, nil
	}
	rule, display, remembered := chatToolSessionRule(name, call.ToolArgs)
	alwaysDir := g.broker.alwaysDir(g.cwd, remembered)

	prompt := llm.ClaudeCodePermissionRequest{ToolName: call.ToolName, ToolUseID: call.ToolCallID}
	if json.Valid([]byte(call.ToolArgs)) {
		prompt.Input = json.RawMessage(call.ToolArgs)
	}
	id, answer, err := askChatPermission(ctx, g.broker, g.sessionID, g.stream, prompt, display, alwaysDir)
	if err != nil {
		return agentloop.ToolDecision{}, err
	}
	switch answer.Decision {
	case "allow_session", "allow_always":
		if remembered {
			g.broker.remember(g.sessionID, rule)
			outcome := "allowed_session"
			if answer.Decision == "allow_always" && alwaysDir != "" && g.broker.always.add(alwaysDir, rule.always()) == nil {
				outcome = "allowed_always"
			}
			g.stream.permissionResolved(id, outcome)
			return agentloop.ToolDecision{Allow: true}, nil
		}
		// Nothing safe to remember for this call; allow it once.
		g.stream.permissionResolved(id, "allowed")
		return agentloop.ToolDecision{Allow: true}, nil
	case "allow_once":
		g.stream.permissionResolved(id, "allowed")
		return agentloop.ToolDecision{Allow: true}, nil
	default:
		g.stream.permissionResolved(id, "denied")
		message := strings.TrimSpace(answer.Message)
		if message == "" {
			message = chatPermissionDenyMessage
		}
		return agentloop.ToolDecision{Message: message}, nil
	}
}

// chatToolRule is an "allow for this session" grant for a native tool: the
// whole tool, or for exec only commands starting with Prefix.
type chatToolRule struct {
	Tool   string
	Prefix string
}

// chatToolSessionRule proposes the rule "allow for this session" should
// remember, with its display text. Like the claude-code-cli rules, exec
// commands that chain or redirect, and rm or sudo, get none; so does process,
// which can start arbitrary commands.
func chatToolSessionRule(name, args string) (chatToolRule, string, bool) {
	switch name {
	case "process":
		return chatToolRule{}, "", false
	case "exec":
		prefix := chatPermissionBashPrefix(chatToolCommand(args))
		if prefix == "" {
			return chatToolRule{}, "", false
		}
		return chatToolRule{Tool: name, Prefix: prefix}, name + "(" + prefix + ":*)", true
	}
	return chatToolRule{Tool: name}, name, true
}

func (r chatToolRule) matches(name, args string) bool {
	if r.Tool != name {
		return false
	}
	if r.Prefix == "" {
		return true
	}
	command := strings.TrimSpace(chatToolCommand(args))
	if chatPermissionHasShellSyntax(command) {
		return false
	}
	want := strings.Fields(r.Prefix)
	got := strings.Fields(command)
	if len(got) < len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// always is the rule as stored for "always allow": exec keeps its prefix in
// Claude Code's notation (exec(ls:*)).
func (r chatToolRule) always() chatAlwaysRule {
	content := ""
	if r.Prefix != "" {
		content = r.Prefix + ":*"
	}
	return chatAlwaysRule{Provider: chatRuleProviderTARS, Tool: r.Tool, Content: content}
}

// alwaysAllows reports whether a stored always rule for cwd covers the call.
func (b *chatPermissionBroker) alwaysAllows(cwd, name, args string) bool {
	if b.always == nil {
		return false
	}
	for _, stored := range b.always.list(cwd) {
		if stored.Provider != chatRuleProviderTARS {
			continue
		}
		rule := chatToolRule{Tool: stored.Tool, Prefix: strings.TrimSuffix(stored.Content, ":*")}
		if rule.matches(name, args) {
			return true
		}
	}
	return false
}

func chatToolCommand(args string) string {
	var input struct {
		Command string `json:"command"`
	}
	_ = json.Unmarshal([]byte(args), &input)
	return input.Command
}

func (b *chatPermissionBroker) remember(sessionID string, rule chatToolRule) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rules[sessionID] = append(b.rules[sessionID], rule)
}

func (b *chatPermissionBroker) allows(sessionID, name, args string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, rule := range b.rules[sessionID] {
		if rule.matches(name, args) {
			return true
		}
	}
	return false
}
