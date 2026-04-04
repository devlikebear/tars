package tool

import (
	"encoding/json"
	"fmt"
	"strings"
)

// dispatchAction extracts the "action" field from params and returns the
// remaining payload and action string. This is the generic version of
// normalizeAutomationActionInput without cron-specific id aliasing.
func dispatchAction(params json.RawMessage) (json.RawMessage, string, error) {
	raw := strings.TrimSpace(string(params))
	if raw == "" || raw == "null" {
		return nil, "", fmt.Errorf("action is required")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(params, &payload); err != nil {
		return nil, "", fmt.Errorf("invalid arguments: %v", err)
	}
	if payload == nil {
		payload = map[string]json.RawMessage{}
	}
	var action string
	if v, ok := payload["action"]; ok {
		if err := json.Unmarshal(v, &action); err != nil {
			return nil, "", fmt.Errorf("action must be string")
		}
	}
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" {
		return nil, "", fmt.Errorf("action is required")
	}
	delete(payload, "action")
	normalized, err := json.Marshal(payload)
	if err != nil {
		return nil, "", fmt.Errorf("marshal normalized payload: %v", err)
	}
	return normalized, action, nil
}

func aggregatorError(message string) Result {
	return JSONTextResult(map[string]string{
		"error": strings.TrimSpace(message),
	}, true)
}
