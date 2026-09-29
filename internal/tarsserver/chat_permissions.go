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
	// Decision is allow_once, allow_session or deny.
	Decision string `json:"decision"`
	// Message is shown to the model on deny.
	Message string `json:"message,omitempty"`
}

func validChatPermissionDecision(decision string) bool {
	switch decision {
	case "allow_once", "allow_session", "deny":
		return true
	}
	return false
}

// chatPermissionBroker holds the prompts running turns are waiting on, so the
// request carrying the answer can reach the request running the turn.
type chatPermissionBroker struct {
	mu      sync.Mutex
	pending map[string]pendingChatPermission
}

type pendingChatPermission struct {
	sessionID string
	answers   chan chatPermissionAnswer
}

func newChatPermissionBroker() *chatPermissionBroker {
	return &chatPermissionBroker{pending: map[string]pendingChatPermission{}}
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

// newChatPermissionHandler answers Claude Code's permission prompts for one
// turn by asking the console.
func newChatPermissionHandler(broker *chatPermissionBroker, sessionID string, stream *chatStreamWriter) llm.ClaudeCodePermissionHandler {
	return func(ctx context.Context, req llm.ClaudeCodePermissionRequest) (llm.ClaudeCodePermissionDecision, error) {
		id, answers, done := broker.open(sessionID)
		defer done()

		ruleDisplay, ruleUpdate := chatPermissionSessionRule(req)
		stream.permissionRequest(id, req, ruleDisplay)
		select {
		case answer := <-answers:
			decision, outcome := chatPermissionDecision(answer, ruleUpdate)
			stream.permissionResolved(id, outcome)
			return decision, nil
		case <-ctx.Done():
			// Cancelled turn, disconnected console, or Claude Code dropping
			// the question: nobody is waiting for an answer any more.
			stream.permissionResolved(id, "withdrawn")
			return llm.ClaudeCodePermissionDecision{}, ctx.Err()
		}
	}
}

const chatPermissionDenyMessage = "The user denied this tool call in the TARS console."

func chatPermissionDecision(answer chatPermissionAnswer, sessionRule json.RawMessage) (llm.ClaudeCodePermissionDecision, string) {
	switch answer.Decision {
	case "allow_session":
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
	var input map[string]any
	_ = json.Unmarshal(req.Input, &input)
	tool := strings.TrimSpace(req.ToolName)
	content := ""
	switch tool {
	case "":
		return "", nil
	case "Bash":
		command, _ := input["command"].(string)
		prefix := chatPermissionBashPrefix(command)
		if prefix == "" {
			return "", nil
		}
		content = prefix + ":*"
	case "WebFetch":
		raw, _ := input["url"].(string)
		parsed, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || parsed.Hostname() == "" {
			return "", nil
		}
		content = "domain:" + parsed.Hostname()
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

func chatPermissionBashPrefix(command string) string {
	command = strings.TrimSpace(command)
	for _, marker := range chatPermissionShellSyntax {
		if strings.Contains(command, marker) {
			return ""
		}
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
		writeError(w, http.StatusBadRequest, "", "decision must be allow_once, allow_session or deny")
		return
	}
	if err := broker.answer(requestID, strings.TrimSpace(body.SessionID), body.chatPermissionAnswer); err != nil {
		writeError(w, http.StatusNotFound, "", "no pending permission request with that id")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
