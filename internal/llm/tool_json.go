package llm

import (
	"encoding/json"
	"strings"
)

func sanitizeToolArgumentsJSON(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "{}"
	}
	if json.Valid([]byte(v)) {
		return v
	}
	return "{}"
}

func parseToolArgumentsObject(raw string) map[string]any {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(sanitizeToolArgumentsJSON(raw)), &parsed); err != nil {
		return map[string]any{}
	}
	if parsed == nil {
		return map[string]any{}
	}
	return parsed
}

func normalizeJSONRaw(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "{}"
	}
	if !json.Valid([]byte(trimmed)) {
		return "{}"
	}
	return trimmed
}
