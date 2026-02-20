package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/secrets"
	"github.com/devlikebear/tarsncase/internal/tool"
)

type EventType string

const (
	EventLoopStart          EventType = "loop_start"
	EventBeforeLLM          EventType = "before_llm"
	EventAfterLLM           EventType = "after_llm"
	EventBeforeTool         EventType = "before_tool_call"
	EventAfterTool          EventType = "after_tool_call"
	EventLoopEnd            EventType = "loop_end"
	EventLoopError          EventType = "error"
	DefaultMaxLoopIters               = 8
	repeatedToolCallLimit             = 3
	autoExecCommandFallback           = "pwd"
)

type Event struct {
	Type         EventType
	Iteration    int
	MessageCount int
	ToolName     string
	ToolCallID   string
	ToolArgs     string
	ToolResult   string
	Err          error
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
	MaxIterations  int
	OnDelta        func(text string)
	Tools          []llm.ToolSchema
	ToolChoice     string
	AutoExpandOnce bool
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

	for i := 0; i < maxIters; i++ {
		l.emit(ctx, Event{Type: EventBeforeLLM, Iteration: i + 1, MessageCount: len(messages)})
		resp, err := l.client.Chat(ctx, messages, llm.ChatOptions{
			OnDelta:    opts.OnDelta,
			Tools:      llmTools,
			ToolChoice: opts.ToolChoice,
		})
		if err != nil {
			l.emit(ctx, Event{Type: EventLoopError, Iteration: i + 1, Err: err})
			return llm.ChatResponse{}, err
		}
		l.emit(ctx, Event{Type: EventAfterLLM, Iteration: i + 1, MessageCount: len(messages)})

		messages = append(messages, resp.Message)
		if len(resp.Message.ToolCalls) == 0 {
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
			effectiveArgs := call.Arguments
			if callName == "exec" {
				correctedArgs, corrected := autoCorrectExecArguments(effectiveArgs, !execAutoCorrectUsed)
				if corrected {
					effectiveArgs = correctedArgs
					execAutoCorrectUsed = true
				}
			}
			if _, ok := allowedTools[callName]; !ok {
				if !opts.AutoExpandOnce || autoExpanded {
					err := fmt.Errorf("tool not injected for this request: %s", call.Name)
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
					err := fmt.Errorf("tool not injected for this request: %s", call.Name)
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

			l.emit(ctx, Event{
				Type:       EventBeforeTool,
				Iteration:  i + 1,
				ToolName:   call.Name,
				ToolCallID: call.ID,
				ToolArgs:   effectiveArgs,
			})

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

			params := json.RawMessage(effectiveArgs)
			if len(params) == 0 {
				params = json.RawMessage(`{}`)
			}

			result, err := tl.Execute(ctx, params)
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
			redactedResult := secrets.RedactText(result.Text())
			l.emit(ctx, Event{
				Type:       EventAfterTool,
				Iteration:  i + 1,
				ToolName:   call.Name,
				ToolCallID: call.ID,
				ToolResult: redactedResult,
			})

			if callName == "exec" && isMissingCommandExecResult(effectiveArgs, redactedResult) {
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

	err := fmt.Errorf("agent loop exceeded max iterations: %d", maxIters)
	l.emit(ctx, Event{Type: EventLoopError, Iteration: maxIters, Err: err})
	return llm.ChatResponse{}, err
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
