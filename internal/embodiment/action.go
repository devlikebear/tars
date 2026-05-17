package embodiment

import (
	"encoding/json"
	"fmt"
	"strings"
)

const bodyActionFence = "tars-body-action"

func ExtractBodyActions(response string) ([]BodyAction, error) {
	blocks := extractBodyActionBlocks(response)
	if len(blocks) == 0 {
		return nil, nil
	}
	actions := make([]BodyAction, 0)
	for _, block := range blocks {
		parsed, err := parseBodyActionBlock(block)
		if err != nil {
			return nil, err
		}
		actions = append(actions, parsed...)
	}
	return actions, nil
}

func NormalizeBodyAction(action BodyAction) (BodyAction, error) {
	kind := ActionKind(strings.ToLower(strings.TrimSpace(string(action.Kind))))
	payload := cloneActionPayload(action.Payload)
	normalized := BodyAction{Kind: kind, Payload: payload}
	switch kind {
	case ActionSpeak:
		text := firstActionString(payload, "text", "message", "summary")
		if text == "" {
			return BodyAction{}, fmt.Errorf("body action speak requires text")
		}
		normalized.Payload["text"] = text
	case ActionExpress:
		emotion := firstActionString(payload, "emotion", "expression")
		if emotion == "" {
			return BodyAction{}, fmt.Errorf("body action express requires emotion")
		}
		normalized.Payload["emotion"] = emotion
	case ActionMove:
		if !hasActionPayloadValue(payload) {
			return BodyAction{}, fmt.Errorf("body action move requires payload")
		}
		if name := firstActionString(payload, "name", "motion", "preset"); name != "" {
			normalized.Payload["name"] = name
		}
	case ActionLED:
		if !hasActionPayloadValue(payload) {
			return BodyAction{}, fmt.Errorf("body action led requires payload")
		}
	default:
		return BodyAction{}, fmt.Errorf("unsupported body action kind %q", action.Kind)
	}
	return normalized, nil
}

func extractBodyActionBlocks(response string) []string {
	var blocks []string
	var current []string
	inBlock := false
	fence := ""
	for _, line := range strings.Split(response, "\n") {
		trimmed := strings.TrimSpace(line)
		if !inBlock {
			if markerFence, ok := bodyActionFenceStart(trimmed); ok {
				inBlock = true
				fence = markerFence
				current = current[:0]
			}
			continue
		}
		if strings.HasPrefix(trimmed, fence) {
			blocks = append(blocks, strings.TrimSpace(strings.Join(current, "\n")))
			inBlock = false
			fence = ""
			current = nil
			continue
		}
		current = append(current, line)
	}
	return blocks
}

func bodyActionFenceStart(line string) (string, bool) {
	for _, fence := range []string{"```", "~~~"} {
		if !strings.HasPrefix(line, fence) {
			continue
		}
		info := strings.TrimSpace(strings.TrimPrefix(line, fence))
		fields := strings.Fields(strings.ToLower(info))
		if len(fields) > 0 && fields[0] == bodyActionFence {
			return fence, true
		}
	}
	return "", false
}

func parseBodyActionBlock(block string) ([]BodyAction, error) {
	if strings.TrimSpace(block) == "" {
		return nil, nil
	}
	raws, err := decodeBodyActionJSON(block)
	if err != nil {
		return nil, err
	}
	actions := make([]BodyAction, 0, len(raws))
	for _, raw := range raws {
		action, err := rawBodyAction(raw)
		if err != nil {
			return nil, err
		}
		normalized, err := NormalizeBodyAction(action)
		if err != nil {
			return nil, err
		}
		actions = append(actions, normalized)
	}
	return actions, nil
}

func decodeBodyActionJSON(block string) ([]map[string]any, error) {
	trimmed := strings.TrimSpace(block)
	var raws []map[string]any
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal([]byte(trimmed), &raws); err != nil {
			return nil, fmt.Errorf("decode body actions: %w", err)
		}
		return raws, nil
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return nil, fmt.Errorf("decode body action: %w", err)
	}
	return []map[string]any{raw}, nil
}

func rawBodyAction(raw map[string]any) (BodyAction, error) {
	kind := strings.TrimSpace(asStringAny(raw["kind"]))
	if kind == "" {
		return BodyAction{}, fmt.Errorf("body action kind is required")
	}
	payload := map[string]any{}
	if rawPayload, ok := raw["payload"].(map[string]any); ok {
		payload = cloneActionPayload(rawPayload)
	} else {
		for key, value := range raw {
			if strings.EqualFold(strings.TrimSpace(key), "kind") {
				continue
			}
			payload[key] = value
		}
	}
	return BodyAction{Kind: ActionKind(kind), Payload: payload}, nil
}

func cloneActionPayload(payload map[string]any) map[string]any {
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		out[key] = value
	}
	return out
}

func firstActionString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(asStringAny(payload[key])); value != "" {
			return value
		}
	}
	return ""
}

func hasActionPayloadValue(payload map[string]any) bool {
	for _, value := range payload {
		if strings.TrimSpace(asStringAny(value)) != "" {
			return true
		}
	}
	return false
}
