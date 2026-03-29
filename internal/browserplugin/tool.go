package browserplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/browser"
	"github.com/devlikebear/tars/internal/tool"
)

func (p *Plugin) Tools() []tool.Tool {
	if p.service == nil {
		return nil
	}
	return []tool.Tool{newBrowserTool(p.service)}
}

func newBrowserTool(svc *browser.Service) tool.Tool {
	return tool.Tool{
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
		Execute: func(ctx context.Context, params json.RawMessage) (tool.Result, error) {
			if svc == nil {
				return tool.JSONTextResult(map[string]any{"message": "browser service is not initialized"}, true), nil
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
				return tool.JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			action := strings.TrimSpace(input.Action)
			switch action {
			case "status":
				return tool.JSONTextResult(svc.Status(), false), nil
			case "profiles":
				profiles := svc.Profiles()
				return tool.JSONTextResult(map[string]any{"count": len(profiles), "profiles": profiles}, false), nil
			case "start":
				profile := resolveBrowserStartProfile(input.Profile, svc.Status())
				return tool.JSONTextResult(svc.Start(profile), false), nil
			case "stop":
				return tool.JSONTextResult(svc.Stop(), false), nil
			case "open":
				state, err := svc.Open(input.URL)
				return tool.JSONTextResult(state, err != nil), nil
			case "snapshot":
				state, err := svc.Snapshot()
				return tool.JSONTextResult(state, err != nil), nil
			case "act":
				state, err := svc.Act(input.Event, input.Target, input.Value)
				return tool.JSONTextResult(state, err != nil), nil
			case "screenshot":
				state, err := svc.Screenshot(input.Name)
				return tool.JSONTextResult(state, err != nil), nil
			case "login":
				result, err := svc.Login(ctx, input.SiteID, input.Profile)
				return tool.JSONTextResult(result, err != nil), nil
			case "check":
				result, err := svc.Check(ctx, input.SiteID, input.Profile)
				return tool.JSONTextResult(result, err != nil), nil
			case "run":
				result, err := svc.Run(ctx, input.SiteID, input.FlowAction, input.Profile)
				return tool.JSONTextResult(result, err != nil), nil
			default:
				return tool.JSONTextResult(map[string]any{"message": "action must be one of: status|profiles|start|stop|open|snapshot|act|screenshot|login|check|run"}, true), nil
			}
		},
	}
}

func resolveBrowserStartProfile(requested string, status browser.State) string {
	profile := strings.TrimSpace(strings.ToLower(requested))
	if !status.ExtensionConnected {
		return profile
	}
	if profile == "" || profile == "managed" {
		return "chrome"
	}
	return profile
}
