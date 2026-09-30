package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/devlikebear/tars/pkg/llm/internal/ccproto"
)

// ClaudeCodePermissionHandler decides one Claude Code permission prompt. It
// may block — on a person, typically — until ctx is done; ctx is cancelled
// when Claude Code withdraws the prompt or the turn ends. An error is reported
// back to Claude Code as a failed request rather than a decision.
type ClaudeCodePermissionHandler func(ctx context.Context, req ClaudeCodePermissionRequest) (ClaudeCodePermissionDecision, error)

// ClaudeCodePermissionRequest is a can_use_tool prompt from Claude Code.
type ClaudeCodePermissionRequest struct {
	ToolName  string
	ToolUseID string
	// Input is the tool input exactly as Claude Code sent it.
	Input json.RawMessage
	// Title, DisplayName and Description are Claude Code's own wording for
	// the prompt, when it supplies any.
	Title       string
	DisplayName string
	Description string
	// DecisionReason explains why Claude Code asked; DecisionReasonType
	// classifies it ("rule", "mode", "classifier", ...).
	DecisionReason     string
	DecisionReasonType string
	BlockedPath        string
	// AgentID is set when a subagent made the call.
	AgentID string
	// Suggestions are Claude Code's proposed permission updates, such as
	// "allow Bash(npm test:*) for this session", kept verbatim so a caller
	// can hand one back in ClaudeCodePermissionDecision.UpdatedPermissions.
	Suggestions []json.RawMessage
}

// ClaudeCodePermissionDecision answers a ClaudeCodePermissionRequest.
type ClaudeCodePermissionDecision struct {
	Allow bool
	// UpdatedInput replaces the tool input on allow. Nil runs the tool with
	// its original input.
	UpdatedInput json.RawMessage
	// UpdatedPermissions are permission updates to apply along with an
	// allow, usually one of the request's Suggestions.
	UpdatedPermissions []json.RawMessage
	// Message is shown to the model on deny.
	Message string
	// Interrupt, on deny, also stops the turn.
	Interrupt bool
}

const defaultClaudeCodeDenyMessage = "The user denied this tool call."

// claudeCodeControlArgs switch the CLI from a prompt argument to the
// stream-json control protocol and route permission prompts to stdout.
var claudeCodeControlArgs = []string{"--input-format", "stream-json", "--permission-prompt-tool", "stdio"}

// runControlOnce runs one turn over the control protocol: initialize, send
// the prompt, answer the CLI's control requests while parsing the stream, and
// close stdin once the turn's result arrives so the CLI exits.
func (c *ClaudeCodeCLIClient) runControlOnce(ctx context.Context, args []string, dir string, env []string, clock *claudeCodeTurnClock, opts ChatOptions, prompt string) (ChatResponse, error) {
	cmd := exec.CommandContext(ctx, c.cliPath, args...)
	cmd.Dir = dir
	// Asks for session_state_changed frames, the SDKs' signal that a run is
	// over. 2.1.283 does not send them in this mode yet; claudeCodeRunEnd
	// falls back to tracking agent tasks until it does.
	cmd.Env = append(append([]string(nil), env...), "CLAUDE_CODE_SDK_READS_SESSION_STATE=1")
	configureClaudeCodeCLIProcess(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return ChatResponse{}, newProviderError(claudeCodeCLIProviderLabel, "request", fmt.Errorf("stdin pipe: %w", err))
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ChatResponse{}, newProviderError(claudeCodeCLIProviderLabel, "request", fmt.Errorf("stdout pipe: %w", err))
	}
	if err := cmd.Start(); err != nil {
		return ChatResponse{}, newProviderError(claudeCodeCLIProviderLabel, "request", fmt.Errorf("start cli: %w", err))
	}

	conn := ccproto.NewConn(stdin, claudeCodeControlHandler(opts.ClaudeCodePermissionHandler))
	var closeInput sync.Once
	endInput := func() { closeInput.Do(func() { _ = stdin.Close() }) }

	// The SDKs wait for initialize before the first user message; its
	// response arrives through the parse loop below, so this runs alongside.
	started := make(chan error, 1)
	go func() {
		if _, err := conn.Request(ctx, map[string]any{"subtype": "initialize"}); err != nil {
			endInput()
			started <- fmt.Errorf("initialize: %w", err)
			return
		}
		if err := conn.WriteUserMessage(prompt); err != nil {
			endInput()
			started <- err
			return
		}
		started <- nil
	}()

	var runEnd claudeCodeRunEnd
	resp, parseErr := parseClaudeCodeCLIStream(stdout, opts, claudeCodeStreamHooks{
		control:  conn.Dispatch,
		activity: clock.touch,
		// Closing stdin is what makes the CLI exit, so it waits until no
		// further turn can need a control response.
		event: func(payload map[string]any) {
			if runEnd.observe(payload) {
				endInput()
			}
		},
	})
	endInput()
	conn.Close()
	startErr := <-started
	waitErr := cmd.Wait()

	// The CLI refusing initialize is the real cause of whatever exit follows;
	// a start failure of any other kind (a closed pipe) is a symptom, and the
	// process outcome says more.
	var refused *ccproto.ResponseError
	if startErr != nil && errors.As(startErr, &refused) && ctx.Err() == nil {
		return ChatResponse{}, newProviderError(claudeCodeCLIProviderLabel, "request", startErr)
	}
	resp, err = finishClaudeCodeCLIRun(ctx, clock.idle, stderr.String(), resp, parseErr, waitErr)
	if err != nil {
		return resp, err
	}
	if startErr != nil {
		return ChatResponse{SessionID: resp.SessionID, Usage: resp.Usage, spentByModel: resp.spentByModel}, newProviderError(claudeCodeCLIProviderLabel, "request", startErr)
	}
	return resp, nil
}

// claudeCodeRunEnd decides when a control-protocol run is over and stdin can
// close. A result ends one turn, not necessarily the run: an async subagent
// can outlive it, ask for permission afterwards, and wake the parent for a
// follow-up turn when it finishes (observed on 2.1.283). The CLI only exits on
// stdin EOF and cannot answer a prompt after it, so the run ends at a result
// with no agent task in flight — or, from a CLI that reports session state,
// at "idle" after a result. This follows claude-agent-sdk-python's Query.
//
// Background shells are not tracked: one may never finish (a dev server, a
// tail -f), and the CLI stops them itself after stdin closes.
type claudeCodeRunEnd struct {
	inflight   map[string]struct{}
	state      string
	sawResult  bool
	reportsRun bool
}

// claudeCodeDeferringTaskTypes are the task types whose completion wakes the
// parent for another turn.
var claudeCodeDeferringTaskTypes = map[string]bool{"local_agent": true, "local_workflow": true}

var claudeCodeTerminalTaskStatuses = map[string]bool{"completed": true, "failed": true, "stopped": true, "killed": true}

// observe records one stream event and reports whether the run just ended.
func (r *claudeCodeRunEnd) observe(payload map[string]any) bool {
	switch asString(payload["type"]) {
	case "result":
		r.sawResult = true
		if r.reportsRun && r.state != "idle" {
			return false
		}
		return len(r.inflight) == 0
	case "system":
	default:
		return false
	}
	taskID := asString(payload["task_id"])
	switch asString(payload["subtype"]) {
	case "session_state_changed":
		r.reportsRun = true
		r.state = asString(payload["state"])
		return r.state == "idle" && r.sawResult && len(r.inflight) == 0
	case "task_started":
		if taskID != "" && claudeCodeDeferringTaskTypes[asString(payload["task_type"])] {
			if r.inflight == nil {
				r.inflight = map[string]struct{}{}
			}
			r.inflight[taskID] = struct{}{}
		}
	case "task_notification":
		delete(r.inflight, taskID)
	case "task_updated":
		if patch, ok := payload["patch"].(map[string]any); ok && claudeCodeTerminalTaskStatuses[asString(patch["status"])] {
			delete(r.inflight, taskID)
		}
	}
	return false
}

// claudeCodeControlHandler answers the control requests the CLI sends. Only
// can_use_tool is expected: TARS registers no hooks or SDK MCP servers.
func claudeCodeControlHandler(permission ClaudeCodePermissionHandler) ccproto.Handler {
	return func(ctx context.Context, subtype string, raw json.RawMessage) (any, error) {
		if subtype != "can_use_tool" || permission == nil {
			return nil, fmt.Errorf("unsupported control request %q", subtype)
		}
		req, err := decodeClaudeCodePermissionRequest(raw)
		if err != nil {
			return nil, err
		}
		decision, err := permission(ctx, req)
		if err != nil {
			return nil, err
		}
		return encodeClaudeCodePermissionDecision(req, decision), nil
	}
}

func decodeClaudeCodePermissionRequest(raw json.RawMessage) (ClaudeCodePermissionRequest, error) {
	var wire struct {
		ToolName           string            `json:"tool_name"`
		ToolUseID          string            `json:"tool_use_id"`
		Input              json.RawMessage   `json:"input"`
		Title              string            `json:"title"`
		DisplayName        string            `json:"display_name"`
		Description        string            `json:"description"`
		DecisionReason     string            `json:"decision_reason"`
		DecisionReasonType string            `json:"decision_reason_type"`
		BlockedPath        string            `json:"blocked_path"`
		AgentID            string            `json:"agent_id"`
		Suggestions        []json.RawMessage `json:"permission_suggestions"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return ClaudeCodePermissionRequest{}, fmt.Errorf("decode can_use_tool: %w", err)
	}
	return ClaudeCodePermissionRequest{
		ToolName:           wire.ToolName,
		ToolUseID:          wire.ToolUseID,
		Input:              wire.Input,
		Title:              wire.Title,
		DisplayName:        wire.DisplayName,
		Description:        wire.Description,
		DecisionReason:     wire.DecisionReason,
		DecisionReasonType: wire.DecisionReasonType,
		BlockedPath:        wire.BlockedPath,
		AgentID:            wire.AgentID,
		Suggestions:        wire.Suggestions,
	}, nil
}

// encodeClaudeCodePermissionDecision builds the PermissionResult Claude Code
// expects. An allow always carries updatedInput: the CLI runs the tool with
// it, so an unmodified allow echoes the original input (as the SDKs do).
func encodeClaudeCodePermissionDecision(req ClaudeCodePermissionRequest, decision ClaudeCodePermissionDecision) map[string]any {
	if !decision.Allow {
		message := strings.TrimSpace(decision.Message)
		if message == "" {
			message = defaultClaudeCodeDenyMessage
		}
		out := map[string]any{"behavior": "deny", "message": message}
		if decision.Interrupt {
			out["interrupt"] = true
		}
		return out
	}
	input := decision.UpdatedInput
	if len(input) == 0 {
		input = req.Input
	}
	if len(input) == 0 {
		input = json.RawMessage(`{}`)
	}
	out := map[string]any{"behavior": "allow", "updatedInput": input}
	if len(decision.UpdatedPermissions) > 0 {
		out["updatedPermissions"] = decision.UpdatedPermissions
	}
	return out
}
