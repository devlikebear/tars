package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/tool"
)

type EventType string

const (
	EventLoopStart     EventType = "loop_start"
	EventBeforeLLM     EventType = "before_llm"
	EventAfterLLM      EventType = "after_llm"
	EventBeforeTool    EventType = "before_tool_call"
	EventAfterTool     EventType = "after_tool_call"
	EventLoopEnd       EventType = "loop_end"
	EventLoopError     EventType = "error"
	DefaultMaxLoopIters          = 8
)

type Event struct {
	Type         EventType
	Iteration    int
	MessageCount int
	ToolName     string
	ToolCallID   string
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
	MaxIterations int
	OnDelta       func(text string)
}

func (l *Loop) Run(ctx context.Context, initial []llm.ChatMessage, opts RunOptions) (llm.ChatResponse, error) {
	maxIters := opts.MaxIterations
	if maxIters <= 0 {
		maxIters = DefaultMaxLoopIters
	}

	messages := append([]llm.ChatMessage(nil), initial...)
	l.emit(ctx, Event{Type: EventLoopStart, MessageCount: len(messages)})

	for i := 0; i < maxIters; i++ {
		l.emit(ctx, Event{Type: EventBeforeLLM, Iteration: i + 1, MessageCount: len(messages)})
		resp, err := l.client.Chat(ctx, messages, llm.ChatOptions{
			OnDelta: opts.OnDelta,
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
			l.emit(ctx, Event{
				Type:       EventBeforeTool,
				Iteration:  i + 1,
				ToolName:   call.Name,
				ToolCallID: call.ID,
			})

			tl, ok := l.registry.Get(call.Name)
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

			params := json.RawMessage(call.Arguments)
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
			l.emit(ctx, Event{
				Type:       EventAfterTool,
				Iteration:  i + 1,
				ToolName:   call.Name,
				ToolCallID: call.ID,
			})

			messages = append(messages, llm.ChatMessage{
				Role:       "tool",
				Content:    result.Text(),
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
