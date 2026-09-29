package tarsserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/devlikebear/tars/internal/llm"
)

// Inline tool approvals for claude-code-cli chat turns (#970).
//
// While a turn runs, Claude Code asks before a tool call its own rules do not
// already allow. The provider hands each question to a handler built here,
// which streams a permission_request event to the console and blocks until
// the console answers through POST /v1/chat/permissions/{request_id}, the
// turn is cancelled, or Claude Code withdraws the question.

var errChatPermissionNotFound = errors.New("no pending permission request")

// chatPermissionAnswer is the console's reply to one permission_request.
type chatPermissionAnswer struct {
	// Decision is allow_once, allow_session, allow_always or deny.
	Decision string `json:"decision"`
	// Message is shown to the model on deny.
	Message string `json:"message,omitempty"`
}

func validChatPermissionDecision(decision string) bool {
	switch decision {
	case "allow_once", "allow_session", "allow_always", "deny":
		return true
	}
	return false
}

// chatPermissionBroker holds the prompts running turns are waiting on, so the
// request carrying the answer can reach the request running the turn. It
// also keeps the "allow for this session" rules native-provider turns have
// granted (chat_tool_gate.go); Claude Code keeps its own.
type chatPermissionBroker struct {
	mu      sync.Mutex
	pending map[string]pendingChatPermission
	rules   map[string][]chatToolRule
	// always persists "always allow in this folder" rules. Nil disables the
	// choice.
	always *chatAlwaysRuleStore
}

type pendingChatPermission struct {
	sessionID string
	answers   chan chatPermissionAnswer
}

func newChatPermissionBroker() *chatPermissionBroker {
	return &chatPermissionBroker{pending: map[string]pendingChatPermission{}, rules: map[string][]chatToolRule{}}
}

// open registers a prompt for sessionID. done must be called once the prompt
// is settled either way.
func (b *chatPermissionBroker) open(sessionID string) (id string, answers <-chan chatPermissionAnswer, done func()) {
	id = newChatPermissionID()
	ch := make(chan chatPermissionAnswer, 1)
	b.mu.Lock()
	b.pending[id] = pendingChatPermission{sessionID: sessionID, answers: ch}
	b.mu.Unlock()
	return id, ch, func() {
		b.mu.Lock()
		delete(b.pending, id)
		b.mu.Unlock()
	}
}

// answer delivers an answer to a pending prompt of sessionID. A prompt takes
// one answer; a second, or one for another session's prompt, is not found.
func (b *chatPermissionBroker) answer(id, sessionID string, answer chatPermissionAnswer) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	pending, ok := b.pending[id]
	if !ok || pending.sessionID != sessionID {
		return errChatPermissionNotFound
	}
	delete(b.pending, id)
	pending.answers <- answer
	return nil
}

func newChatPermissionID() string {
	var raw [12]byte
	_, _ = rand.Read(raw[:])
	return hex.EncodeToString(raw[:])
}

// askChatPermission streams one permission_request and waits for the
// console's answer. The caller reports the outcome with permissionResolved
// under the returned request id. If ctx ends first — a cancelled turn, a
// disconnected console, or Claude Code dropping the question — the prompt is
// closed as withdrawn and ctx's error returned.
func askChatPermission(ctx context.Context, broker *chatPermissionBroker, sessionID string, stream *chatStreamWriter, prompt llm.ClaudeCodePermissionRequest, sessionRule, alwaysDir string) (string, chatPermissionAnswer, error) {
	id, answers, done := broker.open(sessionID)
	defer done()
	stream.permissionRequest(id, prompt, sessionRule, alwaysDir)
	select {
	case answer := <-answers:
		return id, answer, nil
	case <-ctx.Done():
		stream.permissionResolved(id, "withdrawn")
		return id, chatPermissionAnswer{}, ctx.Err()
	}
}

// alwaysDir is the folder "always allow" would cover, or "" when the choice
// is not offered: no store, no folder, or no rule to remember.
func (b *chatPermissionBroker) alwaysDir(cwd string, haveRule bool) string {
	if b.always == nil || !haveRule {
		return ""
	}
	return chatAlwaysRuleDir(cwd)
}

// newChatPermissionHandler answers Claude Code's permission prompts for one
// turn by asking the console. cwd is the folder the turn works in, which an
// "always allow" answer covers.
func newChatPermissionHandler(broker *chatPermissionBroker, sessionID, cwd string, stream *chatStreamWriter) llm.ClaudeCodePermissionHandler {
	return func(ctx context.Context, req llm.ClaudeCodePermissionRequest) (llm.ClaudeCodePermissionDecision, error) {
		ruleDisplay, ruleUpdate := chatPermissionSessionRule(req)
		alwaysDir := broker.alwaysDir(cwd, ruleUpdate != nil)
		id, answer, err := askChatPermission(ctx, broker, sessionID, stream, req, ruleDisplay, alwaysDir)
		if err != nil {
			return llm.ClaudeCodePermissionDecision{}, err
		}
		decision, outcome := chatPermissionDecision(answer, ruleUpdate)
		if answer.Decision == "allow_always" && alwaysDir != "" {
			tool, content, _ := chatPermissionClaudeRule(req)
			if broker.always.add(alwaysDir, chatAlwaysRule{Provider: chatRuleProviderClaudeCode, Tool: tool, Content: content}) == nil {
				outcome = "allowed_always"
			}
		}
		stream.permissionResolved(id, outcome)
		return decision, nil
	}
}

const chatPermissionDenyMessage = "The user denied this tool call in the TARS console."

func chatPermissionDecision(answer chatPermissionAnswer, sessionRule json.RawMessage) (llm.ClaudeCodePermissionDecision, string) {
	switch answer.Decision {
	case "allow_session", "allow_always":
		// An always rule is persisted by the caller; the running CLI is told
		// the session rule so it stops asking now too.
		if sessionRule != nil {
			return llm.ClaudeCodePermissionDecision{Allow: true, UpdatedPermissions: []json.RawMessage{sessionRule}}, "allowed_session"
		}
		// Nothing safe to remember for this call; allow it once.
		return llm.ClaudeCodePermissionDecision{Allow: true}, "allowed"
	case "allow_once":
		return llm.ClaudeCodePermissionDecision{Allow: true}, "allowed"
	default:
		message := strings.TrimSpace(answer.Message)
		if message == "" {
			message = chatPermissionDenyMessage
		}
		return llm.ClaudeCodePermissionDecision{Message: message}, "denied"
	}
}

// chatPermissionSessionRule proposes what "allow for this session" should
// remember, as Claude Code rule text for the card and as the permission
// update to send. It returns no rule when remembering one would allow more
// than the person can see in the prompt.
//
// Claude Code's own suggestions are not used: they name the exact command
// and default to the localSettings destination, which writes the project's
// .claude/settings.local.json.
func chatPermissionSessionRule(req llm.ClaudeCodePermissionRequest) (string, json.RawMessage) {
	tool, content, ok := chatPermissionClaudeRule(req)
	if !ok {
		return "", nil
	}
	rule := map[string]string{"toolName": tool}
	display := tool
	if content != "" {
		rule["ruleContent"] = content
		display = tool + "(" + content + ")"
	}
	update, err := json.Marshal(map[string]any{
		"type":        "addRules",
		"rules":       []map[string]string{rule},
		"behavior":    "allow",
		"destination": "session",
	})
	if err != nil {
		return "", nil
	}
	return display, update
}

// chatPermissionClaudeRule is the Claude Code rule (tool and content) that
// remembering this prompt would add, or !ok when none is safe to offer.
func chatPermissionClaudeRule(req llm.ClaudeCodePermissionRequest) (tool, content string, ok bool) {
	var input map[string]any
	_ = json.Unmarshal(req.Input, &input)
	tool = strings.TrimSpace(req.ToolName)
	switch tool {
	case "":
		return "", "", false
	case "Bash":
		command, _ := input["command"].(string)
		prefix := chatPermissionBashPrefix(command)
		if prefix == "" {
			return "", "", false
		}
		return tool, prefix + ":*", true
	case "WebFetch":
		raw, _ := input["url"].(string)
		parsed, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || parsed.Hostname() == "" {
			return "", "", false
		}
		return tool, "domain:" + parsed.Hostname(), true
	}
	return tool, "", true
}

// chatPermissionShellSyntax marks commands that chain, redirect or substitute:
// a prefix rule for their first word would cover whatever else they run.
var chatPermissionShellSyntax = []string{"&&", "||", ";", "|", ">", "<", "`", "$(", "\n", "&"}

// chatPermissionNeverRemembered are commands too broad to allow wholesale.
var chatPermissionNeverRemembered = map[string]bool{"rm": true, "sudo": true, "su": true, "dd": true}

// chatPermissionSubcommandTools are commands whose first argument is the
// real action, so the rule keeps it ("git status", not every git command).
var chatPermissionSubcommandTools = map[string]bool{
	"git": true, "npm": true, "pnpm": true, "yarn": true, "bun": true, "go": true,
	"cargo": true, "make": true, "docker": true, "kubectl": true, "gh": true, "uv": true,
}

// chatPermissionHasShellSyntax reports whether command chains, redirects or
// substitutes, so its first word says nothing about everything it runs.
func chatPermissionHasShellSyntax(command string) bool {
	for _, marker := range chatPermissionShellSyntax {
		if strings.Contains(command, marker) {
			return true
		}
	}
	return false
}

func chatPermissionBashPrefix(command string) string {
	command = strings.TrimSpace(command)
	if chatPermissionHasShellSyntax(command) {
		return ""
	}
	fields := strings.Fields(command)
	if len(fields) == 0 || chatPermissionNeverRemembered[fields[0]] {
		return ""
	}
	prefix := fields[0]
	if chatPermissionSubcommandTools[prefix] && len(fields) > 1 && !strings.HasPrefix(fields[1], "-") {
		prefix += " " + fields[1]
	}
	return prefix
}

// handleChatPermissionAnswer serves POST /v1/chat/permissions/{request_id}.
func handleChatPermissionAnswer(w http.ResponseWriter, r *http.Request, broker *chatPermissionBroker) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	requestID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/v1/chat/permissions/"))
	var body struct {
		SessionID string `json:"session_id"`
		chatPermissionAnswer
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}
	body.Decision = strings.TrimSpace(body.Decision)
	if requestID == "" || strings.TrimSpace(body.SessionID) == "" {
		writeError(w, http.StatusBadRequest, "", "request id and session_id are required")
		return
	}
	if !validChatPermissionDecision(body.Decision) {
		writeError(w, http.StatusBadRequest, "", "decision must be allow_once, allow_session, allow_always or deny")
		return
	}
	if err := broker.answer(requestID, strings.TrimSpace(body.SessionID), body.chatPermissionAnswer); err != nil {
		writeError(w, http.StatusNotFound, "", "no pending permission request with that id")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
