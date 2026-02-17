package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/gateway"
	"github.com/devlikebear/tarsncase/internal/session"
)

func NewSessionsListTool(store *session.Store) Tool {
	return Tool{
		Name:        "sessions_list",
		Description: "List chat sessions.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{},
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, _ json.RawMessage) (Result, error) {
			if store == nil {
				return jsonTextResult(map[string]any{"message": "session store is not configured"}, true), nil
			}
			sessions, err := store.List()
			if err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("list sessions failed: %v", err)}, true), nil
			}
			return jsonTextResult(map[string]any{"count": len(sessions), "sessions": sessions}, false), nil
		},
	}
}

func NewSessionsHistoryTool(store *session.Store) Tool {
	return Tool{
		Name:        "sessions_history",
		Description: "Read chat history for a session.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "session_id":{"type":"string"},
    "limit":{"type":"integer","minimum":1,"maximum":500,"default":50}
  },
  "required":["session_id"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if store == nil {
				return jsonTextResult(map[string]any{"message": "session store is not configured"}, true), nil
			}
			var input struct {
				SessionID string `json:"session_id"`
				Limit     int    `json:"limit,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			input.SessionID = strings.TrimSpace(input.SessionID)
			if input.SessionID == "" {
				return jsonTextResult(map[string]any{"message": "session_id is required"}, true), nil
			}
			if _, err := store.Get(input.SessionID); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("get session failed: %v", err)}, true), nil
			}
			messages, err := session.ReadMessages(store.TranscriptPath(input.SessionID))
			if err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("read history failed: %v", err)}, true), nil
			}
			limit := input.Limit
			if limit <= 0 {
				limit = 50
			}
			if len(messages) > limit {
				messages = messages[len(messages)-limit:]
			}
			return jsonTextResult(map[string]any{
				"session_id": input.SessionID,
				"count":      len(messages),
				"messages":   messages,
			}, false), nil
		},
	}
}

func NewSessionsSendTool(runtime *gateway.Runtime) Tool {
	return Tool{
		Name:        "sessions_send",
		Description: "Send a prompt to a session and wait for completion.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "session_id":{"type":"string"},
    "title":{"type":"string"},
    "message":{"type":"string"},
    "agent":{"type":"string"},
    "timeout_ms":{"type":"integer","minimum":1000,"maximum":120000,"default":30000}
  },
  "required":["message"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if runtime == nil {
				return jsonTextResult(map[string]any{"message": "gateway runtime is not configured"}, true), nil
			}
			var input struct {
				SessionID string `json:"session_id"`
				Title     string `json:"title"`
				Message   string `json:"message"`
				Agent     string `json:"agent"`
				TimeoutMS int    `json:"timeout_ms,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			run, err := runtime.Spawn(ctx, gateway.SpawnRequest{
				SessionID: input.SessionID,
				Title:     input.Title,
				Prompt:    input.Message,
				Agent:     input.Agent,
			})
			if err != nil {
				return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			timeout := input.TimeoutMS
			if timeout <= 0 {
				timeout = 30000
			}
			waitCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
			defer cancel()
			final, err := runtime.Wait(waitCtx, run.ID)
			if err != nil {
				return jsonTextResult(map[string]any{
					"accepted":   true,
					"run_id":     run.ID,
					"session_id": run.SessionID,
					"status":     run.Status,
					"message":    fmt.Sprintf("wait run failed: %v", err),
				}, true), nil
			}
			isError := final.Status != gateway.RunStatusCompleted
			return jsonTextResult(map[string]any{
				"accepted":     true,
				"run_id":       final.ID,
				"session_id":   final.SessionID,
				"status":       final.Status,
				"response":     final.Response,
				"error":        final.Error,
				"completed_at": final.CompletedAt,
			}, isError), nil
		},
	}
}

func NewSessionsSpawnTool(runtime *gateway.Runtime) Tool {
	return Tool{
		Name:        "sessions_spawn",
		Description: "Spawn an async agent run and return accepted + run_id.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "session_id":{"type":"string"},
    "title":{"type":"string"},
    "message":{"type":"string"},
    "agent":{"type":"string"}
  },
  "required":["message"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if runtime == nil {
				return jsonTextResult(map[string]any{"message": "gateway runtime is not configured"}, true), nil
			}
			var input struct {
				SessionID string `json:"session_id"`
				Title     string `json:"title"`
				Message   string `json:"message"`
				Agent     string `json:"agent"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			run, err := runtime.Spawn(ctx, gateway.SpawnRequest{
				SessionID: input.SessionID,
				Title:     input.Title,
				Prompt:    input.Message,
				Agent:     input.Agent,
			})
			if err != nil {
				return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return jsonTextResult(map[string]any{
				"accepted":   true,
				"run_id":     run.ID,
				"session_id": run.SessionID,
				"status":     run.Status,
			}, false), nil
		},
	}
}

func NewSessionsRunsTool(runtime *gateway.Runtime) Tool {
	return Tool{
		Name:        "sessions_runs",
		Description: "List/get/cancel async runs.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "action":{"type":"string","enum":["list","get","cancel"]},
    "run_id":{"type":"string"},
    "limit":{"type":"integer","minimum":1,"maximum":200,"default":50}
  },
  "required":["action"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if runtime == nil {
				return jsonTextResult(map[string]any{"message": "gateway runtime is not configured"}, true), nil
			}
			var input struct {
				Action string `json:"action"`
				RunID  string `json:"run_id,omitempty"`
				Limit  int    `json:"limit,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			switch strings.TrimSpace(input.Action) {
			case "list":
				runs := runtime.List(input.Limit)
				return jsonTextResult(map[string]any{"count": len(runs), "runs": runs}, false), nil
			case "get":
				runID := strings.TrimSpace(input.RunID)
				if runID == "" {
					return jsonTextResult(map[string]any{"message": "run_id is required"}, true), nil
				}
				run, ok := runtime.Get(runID)
				if !ok {
					return jsonTextResult(map[string]any{"message": "run not found"}, true), nil
				}
				return jsonTextResult(run, false), nil
			case "cancel":
				runID := strings.TrimSpace(input.RunID)
				if runID == "" {
					return jsonTextResult(map[string]any{"message": "run_id is required"}, true), nil
				}
				run, err := runtime.Cancel(runID)
				if err != nil {
					return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
				}
				return jsonTextResult(run, false), nil
			default:
				return jsonTextResult(map[string]any{"message": "action must be one of: list|get|cancel"}, true), nil
			}
		},
	}
}

func NewAgentsListTool(runtime *gateway.Runtime) Tool {
	return Tool{
		Name:        "agents_list",
		Description: "List available agents for sessions_spawn/sessions_send.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Execute: func(_ context.Context, _ json.RawMessage) (Result, error) {
			if runtime == nil {
				return jsonTextResult(map[string]any{"count": 0, "agents": []map[string]any{}}, false), nil
			}
			agents := runtime.Agents()
			return jsonTextResult(map[string]any{"count": len(agents), "agents": agents}, false), nil
		},
	}
}

func NewMessageTool(runtime *gateway.Runtime, enabled bool) Tool {
	return Tool{
		Name:        "message",
		Description: "Messaging actions: send, read, thread_reply.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "action":{"type":"string","enum":["send","read","thread_reply"]},
    "channel_id":{"type":"string"},
    "thread_id":{"type":"string"},
    "text":{"type":"string"},
    "limit":{"type":"integer","minimum":1,"maximum":200,"default":20}
  },
  "required":["action","channel_id"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if !enabled {
				return jsonTextResult(map[string]any{"message": "message tool is disabled"}, true), nil
			}
			if runtime == nil {
				return jsonTextResult(map[string]any{"message": "gateway runtime is not configured"}, true), nil
			}
			var input struct {
				Action    string `json:"action"`
				ChannelID string `json:"channel_id"`
				ThreadID  string `json:"thread_id,omitempty"`
				Text      string `json:"text,omitempty"`
				Limit     int    `json:"limit,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			switch strings.TrimSpace(input.Action) {
			case "send":
				msg, err := runtime.MessageSend(input.ChannelID, input.ThreadID, input.Text)
				if err != nil {
					return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
				}
				return jsonTextResult(msg, false), nil
			case "read":
				items, err := runtime.MessageRead(input.ChannelID, input.Limit)
				if err != nil {
					return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
				}
				return jsonTextResult(map[string]any{"count": len(items), "messages": items}, false), nil
			case "thread_reply":
				msg, err := runtime.ThreadReply(input.ChannelID, input.ThreadID, input.Text)
				if err != nil {
					return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
				}
				return jsonTextResult(msg, false), nil
			default:
				return jsonTextResult(map[string]any{"message": "action must be one of: send|read|thread_reply"}, true), nil
			}
		},
	}
}

func NewBrowserTool(runtime *gateway.Runtime, enabled bool) Tool {
	return Tool{
		Name:        "browser",
		Description: "Browser actions: status, start, stop, open, snapshot, act, screenshot.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "action":{"type":"string","enum":["status","start","stop","open","snapshot","act","screenshot"]},
    "url":{"type":"string"},
    "name":{"type":"string"},
    "event":{"type":"string"},
    "target":{"type":"string"},
    "value":{"type":"string"}
  },
  "required":["action"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if !enabled {
				return jsonTextResult(map[string]any{"message": "browser tool is disabled"}, true), nil
			}
			if runtime == nil {
				return jsonTextResult(map[string]any{"message": "gateway runtime is not configured"}, true), nil
			}
			var input struct {
				Action string `json:"action"`
				URL    string `json:"url,omitempty"`
				Name   string `json:"name,omitempty"`
				Event  string `json:"event,omitempty"`
				Target string `json:"target,omitempty"`
				Value  string `json:"value,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			action := strings.TrimSpace(input.Action)
			switch action {
			case "status":
				return jsonTextResult(runtime.BrowserStatus(), false), nil
			case "start":
				return jsonTextResult(runtime.BrowserStart(), false), nil
			case "stop":
				return jsonTextResult(runtime.BrowserStop(), false), nil
			case "open":
				state, err := runtime.BrowserOpen(input.URL)
				return jsonTextResult(state, err != nil), nil
			case "snapshot":
				state, err := runtime.BrowserSnapshot()
				return jsonTextResult(state, err != nil), nil
			case "act":
				state, err := runtime.BrowserAct(input.Event, input.Target, input.Value)
				return jsonTextResult(state, err != nil), nil
			case "screenshot":
				state, err := runtime.BrowserScreenshot(input.Name)
				return jsonTextResult(state, err != nil), nil
			default:
				return jsonTextResult(map[string]any{"message": "action must be one of: status|start|stop|open|snapshot|act|screenshot"}, true), nil
			}
		},
	}
}

func NewNodesTool(runtime *gateway.Runtime, enabled bool) Tool {
	return Tool{
		Name:        "nodes",
		Description: "Nodes actions: status, describe, invoke.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "action":{"type":"string","enum":["status","describe","invoke"]},
    "name":{"type":"string"},
    "args":{"type":"object"}
  },
  "required":["action"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if !enabled {
				return jsonTextResult(map[string]any{"message": "nodes tool is disabled"}, true), nil
			}
			if runtime == nil {
				return jsonTextResult(map[string]any{"message": "gateway runtime is not configured"}, true), nil
			}
			var input struct {
				Action string         `json:"action"`
				Name   string         `json:"name,omitempty"`
				Args   map[string]any `json:"args,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			switch strings.TrimSpace(input.Action) {
			case "status":
				nodes := runtime.Nodes()
				return jsonTextResult(map[string]any{"count": len(nodes), "nodes": nodes}, false), nil
			case "describe":
				node, err := runtime.NodeDescribe(input.Name)
				if err != nil {
					return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
				}
				return jsonTextResult(node, false), nil
			case "invoke":
				resp, err := runtime.NodeInvoke(input.Name, input.Args)
				if err != nil {
					return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
				}
				return jsonTextResult(resp, false), nil
			default:
				return jsonTextResult(map[string]any{"message": "action must be one of: status|describe|invoke"}, true), nil
			}
		},
	}
}

func NewGatewayTool(runtime *gateway.Runtime, enabled bool) Tool {
	return Tool{
		Name:        "gateway",
		Description: "Gateway actions: status, reload, restart.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "action":{"type":"string","enum":["status","reload","restart"]}
  },
  "required":["action"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if !enabled {
				return jsonTextResult(map[string]any{"message": "gateway tool is disabled"}, true), nil
			}
			if runtime == nil {
				return jsonTextResult(map[string]any{"message": "gateway runtime is not configured"}, true), nil
			}
			var input struct {
				Action string `json:"action"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			switch strings.TrimSpace(input.Action) {
			case "status":
				return jsonTextResult(runtime.Status(), false), nil
			case "reload":
				return jsonTextResult(runtime.Reload(), false), nil
			case "restart":
				return jsonTextResult(runtime.Restart(), false), nil
			default:
				return jsonTextResult(map[string]any{"message": "action must be one of: status|reload|restart"}, true), nil
			}
		},
	}
}
