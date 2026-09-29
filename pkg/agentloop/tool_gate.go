package agentloop

import (
	"context"
	"strings"
)

// ToolGate decides whether a tool call may run. A Hook only observes; a gate
// can stop a call. The loop then reports the denial to the model as the
// call's result and carries on, so the model can adjust instead of failing
// the turn. An error from Authorize means no decision could be made (the
// turn was cancelled, the person who would decide went away) and stops the
// turn.
//
// The loop consults the gate for every call it is about to execute, after
// the allow-list checks and before BeforeTool; calls answered from a replay
// are not re-authorized.
type ToolGate interface {
	Authorize(ctx context.Context, call ToolCallRequest) (ToolDecision, error)
}

// ToolCallRequest describes the call awaiting a decision.
type ToolCallRequest struct {
	ToolName   string
	ToolCallID string
	// ToolArgs is the JSON argument string the tool would receive.
	ToolArgs string
	// EffectClass is the tool's recovery effect class ("read", "write", ...).
	EffectClass string
	Iteration   int
}

// ToolDecision answers a ToolCallRequest.
type ToolDecision struct {
	Allow bool
	// Message tells the model why a call was denied.
	Message string
}

const defaultToolDenialMessage = "The user denied this tool call."

// toolDenialResult is the tool result the model sees for a denied call.
func toolDenialResult(decision ToolDecision) string {
	message := strings.TrimSpace(decision.Message)
	if message == "" {
		message = defaultToolDenialMessage
	}
	return "Tool call denied: " + message
}
