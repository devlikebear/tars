package tarsserver

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/sessionoverride"
	"github.com/devlikebear/tars/internal/tool"
)

// Per-session tool permission modes (#970). One set of modes serves every
// provider; each maps onto what that provider can enforce:
//
//	mode          claude-code-cli        native tool gate (high-risk tools)
//	manual        --permission-mode default    ask for each
//	accept_edits  --permission-mode acceptEdits  file edits run, the rest ask
//	plan          --permission-mode plan       refused: read-only
//	auto          --permission-mode auto       run without asking
//
// A session without a mode keeps the configured default: claude-code-cli
// uses llm.claude_code_cli.permission_mode (or the .tars override), and the
// native gate asks for each high-risk tool as it always has.

const (
	chatPermissionModeManual      = "manual"
	chatPermissionModeAcceptEdits = "accept_edits"
	chatPermissionModePlan        = "plan"
	chatPermissionModeAuto        = "auto"
)

var chatPermissionModes = []string{chatPermissionModeManual, chatPermissionModeAcceptEdits, chatPermissionModePlan, chatPermissionModeAuto}

func validChatPermissionMode(mode string) bool {
	for _, m := range chatPermissionModes {
		if m == mode {
			return true
		}
	}
	return false
}

// claudeCodePermissionFlag is the --permission-mode value for a mode.
func claudeCodePermissionFlag(mode string) string {
	switch mode {
	case chatPermissionModeManual:
		return "default"
	case chatPermissionModeAcceptEdits:
		return "acceptEdits"
	case chatPermissionModePlan:
		return "plan"
	case chatPermissionModeAuto:
		return "auto"
	}
	return ""
}

// chatPermissionModeFromClaudeFlag reads a configured Claude Code mode as the
// closest TARS mode, for showing what a session inherits. Modes that do not
// ask (auto, dontAsk, bypassPermissions) read as auto; an empty or unknown
// value is auto too, because the provider degrades it to auto.
func chatPermissionModeFromClaudeFlag(flag string) string {
	switch strings.TrimSpace(flag) {
	case "default":
		return chatPermissionModeManual
	case "acceptEdits":
		return chatPermissionModeAcceptEdits
	case "plan":
		return chatPermissionModePlan
	}
	return chatPermissionModeAuto
}

// chatFileEditTools are the native tools accept_edits runs without asking.
var chatFileEditTools = map[string]bool{"write_file": true, "edit_file": true, "apply_patch": true}

// chatGateVerdict is what a mode says about a native high-risk tool call
// before anyone is asked.
type chatGateVerdict int

const (
	chatGateAsk chatGateVerdict = iota
	chatGateAllow
	chatGateRefuse
)

func chatGateVerdictFor(mode, toolName string) chatGateVerdict {
	name := tool.CanonicalToolName(toolName)
	if !tool.IsHighRiskToolName(name) {
		return chatGateAllow
	}
	switch mode {
	case chatPermissionModeAuto:
		return chatGateAllow
	case chatPermissionModePlan:
		return chatGateRefuse
	case chatPermissionModeAcceptEdits:
		if chatFileEditTools[name] {
			return chatGateAllow
		}
	}
	return chatGateAsk
}

// chatPlanModeMessage is what the model reads when plan mode refuses a tool.
const chatPlanModeMessage = "Plan mode is read-only: this tool was not run. Finish the plan; the user switches the mode to start making changes."

// chatPermissionModeView is GET /v1/admin/sessions/{id}/permission-mode.
//
// Without a session mode the two kinds of provider inherit different
// defaults, so the view carries both and the console shows the one for the
// session's provider.
type chatPermissionModeView struct {
	// Mode is the session's own choice, or "" when it inherits.
	Mode string `json:"mode"`
	// Effective is the mode native-provider turns run under: the session's
	// mode, or manual.
	Effective string `json:"effective"`
	// ClaudeCodeEffective is the mode claude-code-cli turns run under: the
	// session's mode, or the configured Claude Code mode read as a TARS mode.
	ClaudeCodeEffective string `json:"claude_code_effective"`
	// Source says where ClaudeCodeEffective comes from: session, override
	// (.tars), or config.
	Source string `json:"source"`
	// ClaudeCodeFlag is the --permission-mode a claude-code-cli turn gets.
	ClaudeCodeFlag string   `json:"claude_code_flag"`
	Modes          []string `json:"modes"`
}

// chatPermissionModeResolver works out a session's effective mode.
type chatPermissionModeResolver struct {
	overrides  *sessionoverride.Service
	configFlag string
}

func (r chatPermissionModeResolver) view(sess session.Session) chatPermissionModeView {
	view := chatPermissionModeView{Mode: strings.TrimSpace(sess.PermissionMode), Modes: chatPermissionModes}
	if validChatPermissionMode(view.Mode) {
		view.Effective, view.ClaudeCodeEffective, view.Source = view.Mode, view.Mode, "session"
		view.ClaudeCodeFlag = claudeCodePermissionFlag(view.Mode)
		return view
	}
	view.Mode = ""
	view.Effective = chatPermissionModeManual
	flag := effectiveClaudeCodePermissionMode(r.overrides, sess, "")
	view.Source = "override"
	if flag == "" {
		flag = strings.TrimSpace(r.configFlag)
		view.Source = "config"
	}
	view.ClaudeCodeFlag = flag
	view.ClaudeCodeEffective = chatPermissionModeFromClaudeFlag(flag)
	return view
}

// claudeCodeFlag is the --permission-mode for a turn in sess.
func (r chatPermissionModeResolver) claudeCodeFlag(sess session.Session) string {
	return r.view(sess).ClaudeCodeFlag
}

// gateMode is the mode the native tool gate applies: the session's choice,
// or manual (ask) when it has none, as before modes existed.
func gateModeFor(sess session.Session) string {
	mode := strings.TrimSpace(sess.PermissionMode)
	if validChatPermissionMode(mode) {
		return mode
	}
	return chatPermissionModeManual
}

// handlePermissionMode serves GET and PUT
// /v1/admin/sessions/{id}/permission-mode. PUT {"mode": ""} clears the
// session's choice.
func newPermissionModeHandler(store *session.Store, resolver chatPermissionModeResolver, audit func(ops.AutomationAuditEntry)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionID := strings.TrimSpace(r.PathValue("id"))
		sess, err := store.Get(sessionID)
		if err != nil {
			if errors.Is(err, session.ErrSessionNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, resolver.view(sess))
		case http.MethodPut:
			var req struct {
				Mode string `json:"mode"`
			}
			if !decodeJSONBody(w, r, &req) {
				return
			}
			mode := strings.TrimSpace(req.Mode)
			if mode != "" && !validChatPermissionMode(mode) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "mode must be manual, accept_edits, plan, auto, or empty"})
				return
			}
			if err := store.SetPermissionMode(sessionID, mode); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			before := resolver.view(sess)
			sess.PermissionMode = mode
			after := resolver.view(sess)
			if audit != nil {
				audit(ops.AutomationAuditEntry{
					Timestamp: time.Now().UTC(),
					Actor:     "console",
					Action:    "chat_permission_mode",
					SessionID: sessionID,
					Result:    "changed",
					Details:   map[string]any{"from": before.Mode, "to": mode},
				})
			}
			writeJSON(w, http.StatusOK, after)
		default:
			writeMethodNotAllowed(w)
		}
	})
}

// auditTo records into the ops manager's automation audit log, or nowhere
// when there is none. A failed write does not fail the decision.
func auditTo(manager *ops.Manager) func(ops.AutomationAuditEntry) {
	if manager == nil {
		return nil
	}
	return func(entry ops.AutomationAuditEntry) {
		_, _ = manager.RecordAutomationAudit(entry)
	}
}
