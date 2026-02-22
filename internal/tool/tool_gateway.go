package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/tarsncase/internal/gateway"
	"github.com/devlikebear/tarsncase/internal/serverauth"
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
			workspaceID := serverauth.WorkspaceIDFromContext(ctx)
			switch strings.TrimSpace(input.Action) {
			case "send":
				msg, err := runtime.MessageSendByWorkspace(workspaceID, input.ChannelID, input.ThreadID, input.Text)
				if err != nil {
					return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
				}
				return jsonTextResult(msg, false), nil
			case "read":
				items, err := runtime.MessageReadByWorkspace(workspaceID, input.ChannelID, input.Limit)
				if err != nil {
					return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
				}
				return jsonTextResult(map[string]any{"count": len(items), "messages": items}, false), nil
			case "thread_reply":
				msg, err := runtime.ThreadReplyByWorkspace(workspaceID, input.ChannelID, input.ThreadID, input.Text)
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
		Description: "Browser actions: status, profiles, start, stop, open, snapshot, act, screenshot, login, check, run. Prefer profile='chrome' when relay extension is connected.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "action":{"type":"string","enum":["status","profiles","start","stop","open","snapshot","act","screenshot","login","check","run"]},
    "profile":{"type":"string"},
    "site_id":{"type":"string"},
    "flow_action":{"type":"string"},
    "url":{"type":"string"},
    "name":{"type":"string"},
    "event":{"type":"string"},
    "target":{"type":"string"},
    "value":{"type":"string"}
  },
  "required":["action"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if !enabled {
				return jsonTextResult(map[string]any{"message": "browser tool is disabled"}, true), nil
			}
			if runtime == nil {
				return jsonTextResult(map[string]any{"message": "gateway runtime is not configured"}, true), nil
			}
			var input struct {
				Action     string `json:"action"`
				Profile    string `json:"profile,omitempty"`
				SiteID     string `json:"site_id,omitempty"`
				FlowAction string `json:"flow_action,omitempty"`
				URL        string `json:"url,omitempty"`
				Name       string `json:"name,omitempty"`
				Event      string `json:"event,omitempty"`
				Target     string `json:"target,omitempty"`
				Value      string `json:"value,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			action := strings.TrimSpace(input.Action)
			switch action {
			case "status":
				return jsonTextResult(runtime.BrowserStatus(), false), nil
			case "profiles":
				profiles := runtime.BrowserProfiles()
				return jsonTextResult(map[string]any{"count": len(profiles), "profiles": profiles}, false), nil
			case "start":
				status := runtime.BrowserStatus()
				profile := resolveBrowserStartProfile(input.Profile, status)
				return jsonTextResult(runtime.BrowserStartWithProfile(profile), false), nil
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
			case "login":
				result, err := runtime.BrowserLogin(ctx, input.SiteID, input.Profile)
				return jsonTextResult(result, err != nil), nil
			case "check":
				result, err := runtime.BrowserCheck(ctx, input.SiteID, input.Profile)
				return jsonTextResult(result, err != nil), nil
			case "run":
				result, err := runtime.BrowserRun(ctx, input.SiteID, input.FlowAction, input.Profile)
				return jsonTextResult(result, err != nil), nil
			default:
				return jsonTextResult(map[string]any{"message": "action must be one of: status|profiles|start|stop|open|snapshot|act|screenshot|login|check|run"}, true), nil
			}
		},
	}
}

func resolveBrowserStartProfile(requested string, status gateway.BrowserState) string {
	profile := strings.TrimSpace(strings.ToLower(requested))
	if !status.ExtensionConnected {
		return profile
	}
	if profile == "" || profile == "managed" {
		return "chrome"
	}
	return profile
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
