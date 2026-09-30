package tarsserver

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
)

// Plan approval (#970). In plan mode Claude Code ends planning by calling
// ExitPlanMode with the plan; that call is a permission question like any
// other, so it reaches the console as a permission_request whose tool is
// ExitPlanMode and whose input carries the plan. Allowing it approves the
// plan and picks the mode to carry on in: the session keeps that mode for
// later turns, and the running CLI switches at once through a setMode
// update. Denying it keeps planning, with the person's note as feedback.

const chatExitPlanModeTool = "ExitPlanMode"

const chatPlanKeepPlanningMessage = "The user did not approve the plan yet. Keep planning and revise it."

// chatModeSwitch persists a session's permission mode when a plan is
// approved. A nil switch approves the plan without changing the session.
type chatModeSwitch struct {
	set func(mode string) error
}

func sessionModeSwitch(store *session.Store, sessionID string) *chatModeSwitch {
	if store == nil {
		return nil
	}
	return &chatModeSwitch{set: func(mode string) error { return store.SetPermissionMode(sessionID, mode) }}
}

func askPlanApproval(ctx context.Context, broker *chatPermissionBroker, sessionID, cwd string, stream *chatStreamWriter, req llm.ClaudeCodePermissionRequest, modes *chatModeSwitch) (llm.ClaudeCodePermissionDecision, error) {
	id, answer, err := askChatPermission(ctx, broker, sessionID, stream, req, "", "")
	if err != nil {
		return llm.ClaudeCodePermissionDecision{}, err
	}
	info := chatPermissionAudit{sessionID: sessionID, cwd: cwd, tool: req.ToolName, mode: chatPermissionModePlan}
	if answer.Decision == "deny" {
		broker.settle(stream, id, "plan_rejected", info)
		message := strings.TrimSpace(answer.Message)
		if message == "" {
			message = chatPlanKeepPlanningMessage
		} else {
			message = chatPlanKeepPlanningMessage + " Their note: " + message
		}
		return llm.ClaudeCodePermissionDecision{Message: message}, nil
	}
	mode := answer.Mode
	if mode != chatPermissionModeAuto {
		mode = chatPermissionModeAcceptEdits
	}
	if modes != nil && modes.set != nil {
		_ = modes.set(mode)
	}
	info.mode = mode
	broker.settle(stream, id, "plan_approved", info)
	stream.permissionMode(mode)
	update, err := json.Marshal(map[string]string{
		"type":        "setMode",
		"mode":        claudeCodePermissionFlag(mode),
		"destination": "session",
	})
	if err != nil {
		return llm.ClaudeCodePermissionDecision{Allow: true}, nil
	}
	return llm.ClaudeCodePermissionDecision{Allow: true, UpdatedPermissions: []json.RawMessage{update}}, nil
}
