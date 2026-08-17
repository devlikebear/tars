package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/secrets"
	"github.com/devlikebear/tars/internal/tool"
)

type EventType string

const (
	EventLoopStart  EventType = "loop_start"
	EventBeforeLLM  EventType = "before_llm"
	EventAfterLLM   EventType = "after_llm"
	EventBeforeTool EventType = "before_tool_call"
	EventAfterTool  EventType = "after_tool_call"
	// EventProviderTool fires once for each tool the upstream provider
	// already executed internally (currently only claude-code-cli). It
	// surfaces audit data — ToolName, ToolCallID, ToolArgs — without
	// triggering local execution. Observers (console, ops) treat it as a
	// read-only signal of what the upstream agent did.
	EventProviderTool       EventType = "provider_tool"
	EventLoopEnd            EventType = "loop_end"
	EventLoopError          EventType = "error"
	DefaultMaxLoopIters               = 20
	repeatedToolCallLimit             = 3
	autoExecCommandFallback           = "pwd"
)

type Event struct {
	Type                       EventType
	Iteration                  int
	MessageCount               int
	ToolName                   string
	ToolCallID                 string
	ToolArgs                   string
	ToolResult                 string
	ToolIsError                bool
	ToolEffectClass            string
	ToolIdempotencyKeyArgument string
	ToolReplayed               bool
	ToolReceiptID              string
	SessionID                  string
	Err                        error
}

type Hook interface {
	OnEvent(ctx context.Context, evt Event)
}

type HookFunc func(ctx context.Context, evt Event)

func (f HookFunc) OnEvent(ctx context.Context, evt Event) {
	f(ctx, evt)
}

type Loop struct {
	client   llm.Client
	registry *tool.Registry
	hooks    []Hook
}

func NewLoop(client llm.Client, registry *tool.Registry, hooks ...Hook) *Loop {
	return &Loop{
		client:   client,
		registry: registry,
		hooks:    hooks,
	}
}

type RunOptions struct {
	MaxIterations    int
	OnDelta          func(text string)
	OnReasoningDelta func(text string)
	Tools            []llm.ToolSchema
	BlockedTools     map[string]tool.BlockedToolError
	ToolChoice       *llm.ToolChoice
	ResponseFormat   *llm.ResponseFormat
	AutoExpandOnce   bool
	// OnTurnEnd is invoked when the LLM produces a turn with zero tool calls
	// (i.e. the natural stopping point). It can request another iteration by
	// returning a non-empty `injectInput` string, which will be appended as a
	// user-role message and the loop will continue. Returning an empty
	// `injectInput` lets the loop terminate as usual. Returning an error
	// aborts the loop.
	OnTurnEnd        func(ctx context.Context, lastResp llm.ChatResponse) (injectInput string, err error)
	BeforeTool       func(ctx context.Context, event Event) error
	AfterTool        func(ctx context.Context, event Event) error
	AfterLLM         func(ctx context.Context, event Event) error
	ProviderTool     func(ctx context.Context, event Event) error
	ReplayToolResult func(ctx context.Context, request ToolReplayRequest) (ToolReplayResult, bool)
	// ResumeSessionID seeds the first iteration's ChatOptions.ResumeSessionID
	// so resumable providers (claude-code-cli and antigravity-cli) continue an
	// existing upstream session rather than starting a new one. Subsequent iterations
	// auto-update from the previous response's SessionID so the whole loop
	// stays attached to the same upstream session.
	ResumeSessionID string
	// ClaudeCodeMCPServers is forwarded to ChatOptions on every iteration so
	// the claude-code-cli provider can inject the same MCP server set per
	// turn. Other providers ignore it.
	ClaudeCodeMCPServers []llm.ClaudeCodeMCPServer
	// ClaudeCodePermissionMode selects --permission-mode for the
	// claude-code-cli provider. Forwarded as-is; invalid values fall back to
	// "auto" inside the provider.
	ClaudeCodePermissionMode string
	// ClaudeCodeSkills is forwarded to ChatOptions on every iteration so the
	// claude-code-cli provider materializes the same session skill catalog
	// as a --plugin-dir per turn. Other providers ignore it.
	ClaudeCodeSkills []llm.ClaudeCodeSkill
	// ClaudeCodePermissionDeny is forwarded to ChatOptions on every iteration
	// so the claude-code-cli provider materializes the same session-scoped
	// permission deny rules as a --settings file per turn. Other providers
	// ignore it.
	ClaudeCodePermissionDeny []string
}

type ToolReplayRequest struct {
	ToolName               string
	ToolCallID             string
	ToolArgs               string
	EffectClass            string
	IdempotencyKeyArgument string
}

type ToolReplayResult struct {
	Result    string
	IsError   bool
	ReceiptID string
}

func (l *Loop) Run(ctx context.Context, initial []llm.ChatMessage, opts RunOptions) (llm.ChatResponse, error) {
	maxIters := opts.MaxIterations
	if maxIters <= 0 {
		maxIters = DefaultMaxLoopIters
	}

	messages := append([]llm.ChatMessage(nil), initial...)
	allowedTools := allowedToolSetFromSchemas(opts.Tools)
	llmTools := append([]llm.ToolSchema(nil), opts.Tools...)
	autoExpanded := false
	lastToolOutcomeSig := ""
	repeatedToolOutcomeCount := 0
	repeatedInvalidExecCount := 0
	execAutoCorrectUsed := false
	l.emit(ctx, Event{Type: EventLoopStart, MessageCount: len(messages)})

	// activeResumeID threads the upstream session handle through each
	// iteration. Starts from caller intent, then follows whatever the
	// provider returns so we stay attached to the same session even when the
	// provider mints a fresh ID on the first (fresh) call.
	activeResumeID := strings.TrimSpace(opts.ResumeSessionID)

	for i := 0; i < maxIters; i++ {
		l.emit(ctx, Event{Type: EventBeforeLLM, Iteration: i + 1, MessageCount: len(messages)})
		resp, err := l.client.Chat(ctx, messages, llm.ChatOptions{
			OnDelta:                  opts.OnDelta,
			OnReasoningDelta:         opts.OnReasoningDelta,
			Tools:                    llmTools,
			ToolChoice:               opts.ToolChoice,
			ResponseFormat:           opts.ResponseFormat,
			ResumeSessionID:          activeResumeID,
			ClaudeCodeMCPServers:     opts.ClaudeCodeMCPServers,
			ClaudeCodePermissionMode: opts.ClaudeCodePermissionMode,
			ClaudeCodeSkills:         opts.ClaudeCodeSkills,
			ClaudeCodePermissionDeny: opts.ClaudeCodePermissionDeny,
		})
		if err != nil {
			l.emit(ctx, Event{Type: EventLoopError, Iteration: i + 1, Err: err})
			return llm.ChatResponse{}, err
		}
		afterLLMEvent := Event{Type: EventAfterLLM, Iteration: i + 1, MessageCount: len(messages), SessionID: strings.TrimSpace(resp.SessionID)}
		if opts.AfterLLM != nil {
			if hookErr := opts.AfterLLM(ctx, afterLLMEvent); hookErr != nil {
				l.emit(ctx, Event{Type: EventLoopError, Iteration: i + 1, Err: hookErr})
				return llm.ChatResponse{}, hookErr
			}
		}
		l.emit(ctx, afterLLMEvent)

		// Surface tools the upstream provider already executed (CLI-backed
		// providers run tools inside their own subprocesses and report them
		// via stream-json tool_use blocks). These do NOT enter the tool
		// execution branch below; they're observation-only audit signals.
		for _, ptc := range resp.ProviderExecutedTools {
			providerEvent := Event{
				Type:            EventProviderTool,
				Iteration:       i + 1,
				ToolName:        ptc.Name,
				ToolCallID:      ptc.ID,
				ToolArgs:        ptc.Arguments,
				ToolEffectClass: string(tool.RecoveryPolicyForTool(tool.Tool{Name: ptc.Name}).EffectClass),
			}
			if opts.ProviderTool != nil {
				if hookErr := opts.ProviderTool(ctx, providerEvent); hookErr != nil {
					l.emit(ctx, Event{Type: EventLoopError, Iteration: i + 1, ToolName: ptc.Name, ToolCallID: ptc.ID, Err: hookErr})
					return llm.ChatResponse{}, hookErr
				}
			}
			l.emit(ctx, providerEvent)
		}

		if sid := strings.TrimSpace(resp.SessionID); sid != "" {
			activeResumeID = sid
		}

		messages = append(messages, resp.Message)
		if len(resp.Message.ToolCalls) == 0 {
			if opts.OnTurnEnd != nil {
				injectInput, hookErr := opts.OnTurnEnd(ctx, resp)
				if hookErr != nil {
					l.emit(ctx, Event{Type: EventLoopError, Iteration: i + 1, Err: hookErr})
					return llm.ChatResponse{}, hookErr
				}
				if strings.TrimSpace(injectInput) != "" {
					messages = append(messages, llm.ChatMessage{
						Role:    "user",
						Content: injectInput,
					})
					continue
				}
			}
			l.emit(ctx, Event{Type: EventLoopEnd, Iteration: i + 1, MessageCount: len(messages)})
			return resp, nil
		}

		if l.registry == nil {
			err := fmt.Errorf("tool registry is not configured")
			l.emit(ctx, Event{Type: EventLoopError, Iteration: i + 1, Err: err})
			return llm.ChatResponse{}, err
		}

		for _, call := range resp.Message.ToolCalls {
			callName := normalizeToolName(call.Name)
			isExecCall := tool.IsExecToolName(callName)
			effectiveArgs := call.Arguments
			if isExecCall {
				correctedArgs, corrected := autoCorrectExecArguments(effectiveArgs, !execAutoCorrectUsed)
				if corrected {
					effectiveArgs = correctedArgs
					execAutoCorrectUsed = true
				}
			}
			if _, ok := allowedTools[callName]; !ok {
				blockedErr := blockedToolErrorForName(opts.BlockedTools, call.Name, callName)
				if !opts.AutoExpandOnce || autoExpanded {
					err := blockedErr
					if err.Tool == "" {
						err = tool.BlockedToolError{Tool: callNameOrOriginal(callName, call.Name), Rule: "tool_allow", Source: "request"}
					}
					l.emit(ctx, Event{
						Type:       EventLoopError,
						Iteration:  i + 1,
						ToolName:   call.Name,
						ToolCallID: call.ID,
						Err:        err,
					})
					return llm.ChatResponse{}, err
				}
				extra := l.registry.SchemasForNames([]string{call.Name, callName})
				if len(extra) == 0 {
					err := blockedErr
					if err.Tool == "" {
						err = tool.BlockedToolError{Tool: callNameOrOriginal(callName, call.Name), Rule: "tool_allow", Source: "request"}
					}
					l.emit(ctx, Event{
						Type:       EventLoopError,
						Iteration:  i + 1,
						ToolName:   call.Name,
						ToolCallID: call.ID,
						Err:        err,
					})
					return llm.ChatResponse{}, err
				}
				llmTools = appendToolSchemas(llmTools, extra...)
				allowedTools[callName] = struct{}{}
				autoExpanded = true
			}

			tl, ok := l.registry.Get(call.Name)
			if !ok {
				tl, ok = l.registry.Get(callName)
			}
			if !ok {
				err := fmt.Errorf("tool not found: %s", call.Name)
				l.emit(ctx, Event{
					Type:       EventLoopError,
					Iteration:  i + 1,
					ToolName:   call.Name,
					ToolCallID: call.ID,
					Err:        err,
				})
				return llm.ChatResponse{}, err
			}
			recoveryPolicy := tool.RecoveryPolicyForTool(tl)
			beforeToolEvent := Event{
				Type:                       EventBeforeTool,
				Iteration:                  i + 1,
				ToolName:                   call.Name,
				ToolCallID:                 call.ID,
				ToolArgs:                   effectiveArgs,
				ToolEffectClass:            string(recoveryPolicy.EffectClass),
				ToolIdempotencyKeyArgument: recoveryPolicy.IdempotencyKeyArgument,
			}

			params := json.RawMessage(effectiveArgs)
			if len(params) == 0 {
				params = json.RawMessage(`{}`)
			}
			var replay ToolReplayResult
			replayed := false
			if opts.ReplayToolResult != nil {
				replay, replayed = opts.ReplayToolResult(ctx, ToolReplayRequest{
					ToolName:               call.Name,
					ToolCallID:             call.ID,
					ToolArgs:               effectiveArgs,
					EffectClass:            string(recoveryPolicy.EffectClass),
					IdempotencyKeyArgument: recoveryPolicy.IdempotencyKeyArgument,
				})
			}
			beforeToolEvent.ToolReplayed = replayed
			beforeToolEvent.ToolReceiptID = strings.TrimSpace(replay.ReceiptID)
			if !replayed && opts.BeforeTool != nil {
				if hookErr := opts.BeforeTool(ctx, beforeToolEvent); hookErr != nil {
					l.emit(ctx, Event{Type: EventLoopError, Iteration: i + 1, ToolName: call.Name, ToolCallID: call.ID, Err: hookErr})
					return llm.ChatResponse{}, hookErr
				}
			}
			l.emit(ctx, beforeToolEvent)

			var result tool.Result
			if replayed {
				result = tool.Result{
					Content: []tool.ContentBlock{{Type: "text", Text: replay.Result}},
					IsError: replay.IsError,
				}
			} else {
				callCtx := ctx
				if emitter := tool.LineEmitterFromContext(ctx); emitter != nil {
					if streamer := tool.BindLineEmitter(emitter, call.ID); streamer != nil {
						callCtx = tool.WithToolOutputStreamer(ctx, streamer)
					}
				}
				result, err = tl.Execute(callCtx, params)
				if err != nil {
					l.emit(ctx, Event{
						Type:       EventLoopError,
						Iteration:  i + 1,
						ToolName:   call.Name,
						ToolCallID: call.ID,
						Err:        err,
					})
					return llm.ChatResponse{}, err
				}
			}
			redactedResult := secrets.RedactText(result.Text())
			afterToolEvent := Event{
				Type:                       EventAfterTool,
				Iteration:                  i + 1,
				ToolName:                   call.Name,
				ToolCallID:                 call.ID,
				ToolArgs:                   effectiveArgs,
				ToolResult:                 redactedResult,
				ToolIsError:                result.IsError,
				ToolEffectClass:            string(recoveryPolicy.EffectClass),
				ToolIdempotencyKeyArgument: recoveryPolicy.IdempotencyKeyArgument,
				ToolReplayed:               replayed,
				ToolReceiptID:              strings.TrimSpace(replay.ReceiptID),
			}
			if opts.AfterTool != nil {
				if hookErr := opts.AfterTool(ctx, afterToolEvent); hookErr != nil {
					l.emit(ctx, Event{Type: EventLoopError, Iteration: i + 1, ToolName: call.Name, ToolCallID: call.ID, Err: hookErr})
					return llm.ChatResponse{}, hookErr
				}
			}
			l.emit(ctx, afterToolEvent)

			if isExecCall && isMissingCommandExecResult(effectiveArgs, redactedResult) {
				repeatedInvalidExecCount++
			} else {
				repeatedInvalidExecCount = 0
			}
			if repeatedInvalidExecCount >= 2 {
				err := fmt.Errorf(`agent loop blocked repeated invalid exec call: missing "command" argument`)
				l.emit(ctx, Event{
					Type:       EventLoopError,
					Iteration:  i + 1,
					ToolName:   call.Name,
					ToolCallID: call.ID,
					Err:        err,
				})
				return llm.ChatResponse{}, err
			}

			outcomeSig := callName + "\n" + effectiveArgs + "\n" + redactedResult
			if outcomeSig == lastToolOutcomeSig {
				repeatedToolOutcomeCount++
			} else {
				lastToolOutcomeSig = outcomeSig
				repeatedToolOutcomeCount = 1
			}
			if repeatedToolOutcomeCount >= repeatedToolCallLimit {
				err := fmt.Errorf("agent loop detected repeated tool call pattern: tool=%s args=%s", call.Name, effectiveArgs)
				l.emit(ctx, Event{
					Type:       EventLoopError,
					Iteration:  i + 1,
					ToolName:   call.Name,
					ToolCallID: call.ID,
					Err:        err,
				})
				return llm.ChatResponse{}, err
			}

			messages = append(messages, llm.ChatMessage{
				Role:       "tool",
				Content:    redactedResult,
				ToolCallID: call.ID,
			})
		}
	}

	finalIter := maxIters + 1
	l.emit(ctx, Event{Type: EventBeforeLLM, Iteration: finalIter, MessageCount: len(messages)})
	finalResp, finalErr := l.client.Chat(ctx, messages, llm.ChatOptions{
		OnDelta:          opts.OnDelta,
		OnReasoningDelta: opts.OnReasoningDelta,
		Tools:            nil,
		ToolChoice:       llm.ToolChoiceNone(),
	})
	if finalErr == nil {
		afterLLMEvent := Event{Type: EventAfterLLM, Iteration: finalIter, MessageCount: len(messages), SessionID: strings.TrimSpace(finalResp.SessionID)}
		if opts.AfterLLM != nil {
			if hookErr := opts.AfterLLM(ctx, afterLLMEvent); hookErr != nil {
				l.emit(ctx, Event{Type: EventLoopError, Iteration: finalIter, Err: hookErr})
				return llm.ChatResponse{}, hookErr
			}
		}
		l.emit(ctx, afterLLMEvent)
		if strings.TrimSpace(finalResp.Message.Content) != "" || len(finalResp.Message.ToolCalls) == 0 {
			messages = append(messages, finalResp.Message)
			l.emit(ctx, Event{Type: EventLoopEnd, Iteration: finalIter, MessageCount: len(messages)})
			return finalResp, nil
		}
	}

	err := fmt.Errorf("agent loop exceeded max iterations: %d", maxIters)
	if finalErr != nil {
		err = fmt.Errorf("agent loop exceeded max iterations: %d (finalization failed: %v)", maxIters, finalErr)
	}
	l.emit(ctx, Event{Type: EventLoopError, Iteration: maxIters, Err: err})
	return llm.ChatResponse{}, err
}

func blockedToolErrorForName(blocked map[string]tool.BlockedToolError, rawName, canonical string) tool.BlockedToolError {
	if len(blocked) == 0 {
		return tool.BlockedToolError{}
	}
	if err, ok := blocked[canonical]; ok {
		return err
	}
	if err, ok := blocked[normalizeToolName(rawName)]; ok {
		return err
	}
	return tool.BlockedToolError{}
}

func callNameOrOriginal(canonical, raw string) string {
	if strings.TrimSpace(canonical) != "" {
		return strings.TrimSpace(canonical)
	}
	return strings.TrimSpace(raw)
}

func (l *Loop) emit(ctx context.Context, evt Event) {
	for _, h := range l.hooks {
		h.OnEvent(ctx, evt)
	}
}

func allowedToolSetFromSchemas(schemas []llm.ToolSchema) map[string]struct{} {
	out := map[string]struct{}{}
	for _, schema := range schemas {
		name := normalizeToolName(schema.Function.Name)
		if name == "" {
			continue
		}
		out[name] = struct{}{}
	}
	return out
}

func normalizeToolName(name string) string {
	return tool.CanonicalToolName(name)
}

func appendToolSchemas(existing []llm.ToolSchema, extras ...llm.ToolSchema) []llm.ToolSchema {
	if len(extras) == 0 {
		return existing
	}
	seen := allowedToolSetFromSchemas(existing)
	for _, schema := range extras {
		name := normalizeToolName(schema.Function.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		existing = append(existing, schema)
	}
	return existing
}

func isMissingCommandExecResult(args string, resultText string) bool {
	if hasExecCommandArgument(args) {
		return false
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(resultText)), "command is required")
}

func autoCorrectExecArguments(rawArgs string, allow bool) (string, bool) {
	if !allow || hasExecCommandArgument(rawArgs) {
		return rawArgs, false
	}
	payload := map[string]any{}
	trimmed := strings.TrimSpace(rawArgs)
	if trimmed != "" && trimmed != "null" {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil && parsed != nil {
			payload = parsed
		}
	}
	payload["command"] = autoExecCommandFallback
	normalized, err := json.Marshal(payload)
	if err != nil {
		return rawArgs, false
	}
	return string(normalized), true
}

func hasExecCommandArgument(rawArgs string) bool {
	v := strings.TrimSpace(rawArgs)
	if v == "" || v == "{}" || v == "null" {
		return false
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(v), &payload); err != nil {
		return false
	}
	for _, key := range []string{"command", "cmd"} {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		var cmd string
		if err := json.Unmarshal(raw, &cmd); err != nil {
			continue
		}
		if strings.TrimSpace(cmd) != "" {
			return true
		}
	}
	return false
}
