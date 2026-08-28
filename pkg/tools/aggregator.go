package tools

import (
	"encoding/json"
	"fmt"
	"strings"
)

// dispatchAction extracts the "action" field from params and returns the
// remaining payload and action string. Optional aliasFns run after the
// action is removed, letting aggregators rename or fold input fields
// without each duplicating the JSON-decode boilerplate.
func dispatchAction(params json.RawMessage, aliasFns ...func(map[string]json.RawMessage)) (json.RawMessage, string, error) {
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
	for _, alias := range aliasFns {
		if alias != nil {
			alias(payload)
		}
	}
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

// DispatchAction and AggregatorError expose the two helpers above across the
// package boundary, for the TARS-specific aggregators that live in
// internal/apptool.
//
// They are wrappers rather than a rename of the originals on purpose. Inside
// this package the helpers are ordinary internals and read better unexported;
// what the app package needs is a small, deliberate seam, and naming it
// separately makes it visible which calls cross a package boundary.
func DispatchAction(params json.RawMessage, aliasFns ...func(map[string]json.RawMessage)) (json.RawMessage, string, error) {
	return dispatchAction(params, aliasFns...)
}

func AggregatorError(message string) Result {
	return aggregatorError(message)
}
