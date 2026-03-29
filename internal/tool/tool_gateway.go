package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/gateway"
	"github.com/devlikebear/tars/internal/serverauth"
)

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
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if !enabled {
				return JSONTextResult(map[string]any{"message": "message tool is disabled"}, true), nil
			}
			if runtime == nil {
				return JSONTextResult(map[string]any{"message": "gateway runtime is not configured"}, true), nil
			}
			var input struct {
				Action    string `json:"action"`
				ChannelID string `json:"channel_id"`
				ThreadID  string `json:"thread_id,omitempty"`
				Text      string `json:"text,omitempty"`
				Limit     int    `json:"limit,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			workspaceID := serverauth.WorkspaceIDFromContext(ctx)
			switch strings.TrimSpace(input.Action) {
			case "send":
				msg, err := runtime.MessageSendByWorkspace(workspaceID, input.ChannelID, input.ThreadID, input.Text)
				if err != nil {
					return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
				}
				return JSONTextResult(msg, false), nil
			case "read":
				items, err := runtime.MessageReadByWorkspace(workspaceID, input.ChannelID, input.Limit)
				if err != nil {
					return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
				}
				return JSONTextResult(map[string]any{"count": len(items), "messages": items}, false), nil
			case "thread_reply":
				msg, err := runtime.ThreadReplyByWorkspace(workspaceID, input.ChannelID, input.ThreadID, input.Text)
				if err != nil {
					return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
				}
				return JSONTextResult(msg, false), nil
			default:
				return JSONTextResult(map[string]any{"message": "action must be one of: send|read|thread_reply"}, true), nil
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
				return JSONTextResult(map[string]any{"message": "nodes tool is disabled"}, true), nil
			}
			if runtime == nil {
				return JSONTextResult(map[string]any{"message": "gateway runtime is not configured"}, true), nil
			}
			var input struct {
				Action string         `json:"action"`
				Name   string         `json:"name,omitempty"`
				Args   map[string]any `json:"args,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			switch strings.TrimSpace(input.Action) {
			case "status":
				nodes := runtime.Nodes()
				return JSONTextResult(map[string]any{"count": len(nodes), "nodes": nodes}, false), nil
			case "describe":
				node, err := runtime.NodeDescribe(input.Name)
				if err != nil {
					return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
				}
				return JSONTextResult(node, false), nil
			case "invoke":
				resp, err := runtime.NodeInvoke(input.Name, input.Args)
				if err != nil {
					return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
				}
				return JSONTextResult(resp, false), nil
			default:
				return JSONTextResult(map[string]any{"message": "action must be one of: status|describe|invoke"}, true), nil
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
				return JSONTextResult(map[string]any{"message": "gateway tool is disabled"}, true), nil
			}
			if runtime == nil {
				return JSONTextResult(map[string]any{"message": "gateway runtime is not configured"}, true), nil
			}
			var input struct {
				Action string `json:"action"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			switch strings.TrimSpace(input.Action) {
			case "status":
				return JSONTextResult(runtime.Status(), false), nil
			case "reload":
				return JSONTextResult(runtime.Reload(), false), nil
			case "restart":
				return JSONTextResult(runtime.Restart(), false), nil
			default:
				return JSONTextResult(map[string]any{"message": "action must be one of: status|reload|restart"}, true), nil
			}
		},
	}
}
