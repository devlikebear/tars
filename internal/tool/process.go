package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type processResponse struct {
	Action   string            `json:"action"`
	Sessions []ProcessSnapshot `json:"sessions,omitempty"`
	Session  *ProcessSnapshot  `json:"session,omitempty"`
	Removed  int               `json:"removed,omitempty"`
	Message  string            `json:"message,omitempty"`
}

func NewProcessTool(manager *ProcessManager) Tool {
	return Tool{
		Name:        "process",
		Description: "Manage background exec sessions (list/poll/log/write/kill/clear/remove).",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "action":{"type":"string","enum":["list","poll","log","write","kill","clear","remove"]},
    "session_id":{"type":"string"},
    "chars":{"type":"string"}
  },
  "required":["action"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if manager == nil {
				return JSONTextResult(processResponse{Message: "process manager is not configured"}, true), nil
			}
			var input struct {
				Action    string `json:"action"`
				SessionID string `json:"session_id,omitempty"`
				Chars     string `json:"chars,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(processResponse{Message: fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			action := strings.ToLower(strings.TrimSpace(input.Action))
			switch action {
			case "list":
				return JSONTextResult(processResponse{Action: action, Sessions: manager.List()}, false), nil
			case "poll":
				s, err := manager.Poll(input.SessionID)
				if err != nil {
					return JSONTextResult(processResponse{Action: action, Message: err.Error()}, true), nil
				}
				return JSONTextResult(processResponse{Action: action, Session: &s}, false), nil
			case "log":
				s, err := manager.Log(input.SessionID)
				if err != nil {
					return JSONTextResult(processResponse{Action: action, Message: err.Error()}, true), nil
				}
				return JSONTextResult(processResponse{Action: action, Session: &s}, false), nil
			case "write":
				s, err := manager.Write(input.SessionID, input.Chars)
				if err != nil {
					return JSONTextResult(processResponse{Action: action, Session: &s, Message: err.Error()}, true), nil
				}
				return JSONTextResult(processResponse{Action: action, Session: &s}, false), nil
			case "kill":
				s, err := manager.Kill(input.SessionID)
				if err != nil {
					return JSONTextResult(processResponse{Action: action, Session: &s, Message: err.Error()}, true), nil
				}
				return JSONTextResult(processResponse{Action: action, Session: &s}, false), nil
			case "remove":
				if err := manager.Remove(input.SessionID); err != nil {
					return JSONTextResult(processResponse{Action: action, Message: err.Error()}, true), nil
				}
				return JSONTextResult(processResponse{Action: action, Removed: 1}, false), nil
			case "clear":
				removed := manager.ClearDone()
				return JSONTextResult(processResponse{Action: action, Removed: removed}, false), nil
			default:
				return JSONTextResult(processResponse{Message: "action is required and must be one of: list,poll,log,write,kill,clear,remove"}, true), nil
			}
		},
	}
}
