package embodiment

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type NormalizeOptions struct {
	KnownBody bool
	Now       func() time.Time
}

func LooksLikePerceptPayload(provider string, payload map[string]any, knownBody bool) bool {
	if len(payload) == 0 {
		return false
	}
	if asBool(payload["x-embodiment"]) || asBool(payload["embodiment"]) {
		return true
	}
	source := strings.TrimSpace(asStringAny(payload["source"]))
	if source != "" && strings.EqualFold(source, strings.TrimSpace(provider)) && knownBody {
		return strings.TrimSpace(summaryFromPayload(payload)) != ""
	}
	if _, ok := payload["modality"]; ok {
		return strings.TrimSpace(summaryFromPayload(payload)) != ""
	}
	return false
}

func NormalizePercept(provider string, payload map[string]any, opts NormalizeOptions) (Percept, error) {
	normalizedProvider := normalizeName(provider)
	if normalizedProvider == "" {
		normalizedProvider = normalizeName(asStringAny(payload["source"]))
	}
	if normalizedProvider == "" {
		return Percept{}, fmt.Errorf("embodiment provider is required")
	}
	summary := strings.TrimSpace(summaryFromPayload(payload))
	if summary == "" {
		return Percept{}, fmt.Errorf("embodiment percept summary is required")
	}
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	capturedAt := capturedAtFromPayload(payload, now().UTC())
	owner := normalizeOwnerState(firstString(payload, "owner", "identity"))
	modality := normalizeModality(firstString(payload, "modality"))
	if modality == "" {
		modality = inferModality(payload)
	}
	if owner == "" {
		owner = OwnerUnknown
	}
	return Percept{
		ID:            strings.TrimSpace(firstString(payload, "id", "percept_id")),
		Provider:      normalizedProvider,
		Modality:      modality,
		Owner:         owner,
		Summary:       summary,
		Labels:        stringSliceFromAny(payload["labels"]),
		MediaRef:      mediaRefFromPayload(payload, modality),
		Trigger:       strings.TrimSpace(strings.ToLower(firstString(payload, "trigger"))),
		Salience:      floatFromAny(payload["salience"]),
		SessionID:     strings.TrimSpace(firstString(payload, "session_id")),
		ThreadID:      strings.TrimSpace(firstString(payload, "thread_id")),
		IsSelfSensory: opts.KnownBody,
		CapturedAt:    capturedAt,
		Raw:           clonePayload(payload),
	}, nil
}

func summaryFromPayload(payload map[string]any) string {
	if v := strings.TrimSpace(firstString(payload, "summary", "text")); v != "" {
		return v
	}
	if msg, ok := payload["message"].(map[string]any); ok {
		return strings.TrimSpace(firstString(msg, "summary", "text"))
	}
	return ""
}

func firstString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(asStringAny(payload[key])); v != "" {
			return v
		}
	}
	return ""
}

func asStringAny(v any) string {
	switch value := v.(type) {
	case nil:
		return ""
	case string:
		return value
	case json.Number:
		return value.String()
	case fmt.Stringer:
		return value.String()
	case float64:
		if value == float64(int64(value)) {
			return strconv.FormatInt(int64(value), 10)
		}
		return strconv.FormatFloat(value, 'f', -1, 64)
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case bool:
		if value {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func asBool(v any) bool {
	switch value := v.(type) {
	case bool:
		return value
	case string:
		parsed, _ := strconv.ParseBool(strings.TrimSpace(value))
		return parsed
	default:
		return false
	}
}

func floatFromAny(v any) float64 {
	switch value := v.(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case json.Number:
		parsed, _ := value.Float64()
		return parsed
	case string:
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
		return parsed
	default:
		return 0
	}
}

func normalizeOwnerState(value string) OwnerState {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "owner":
		return OwnerOwner
	case "stranger":
		return OwnerStranger
	case "unknown":
		return OwnerUnknown
	case "none", "ambient":
		return OwnerNone
	default:
		return ""
	}
}

func normalizeModality(value string) Modality {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "vision", "image", "camera":
		return ModalityVision
	case "audio", "hearing", "voice", "sound":
		return ModalityAudio
	case "sensor", "sensors":
		return ModalitySensor
	default:
		return ""
	}
}

func inferModality(payload map[string]any) Modality {
	identityModality := strings.ToLower(strings.TrimSpace(firstString(payload, "identity_modality")))
	if strings.Contains(identityModality, "voice") {
		return ModalityAudio
	}
	if strings.TrimSpace(firstString(payload, "audio_ref")) != "" {
		return ModalityAudio
	}
	if strings.TrimSpace(firstString(payload, "image_ref")) != "" {
		return ModalityVision
	}
	return ModalitySensor
}

func mediaRefFromPayload(payload map[string]any, modality Modality) string {
	if v := strings.TrimSpace(firstString(payload, "media_ref")); v != "" {
		return v
	}
	switch modality {
	case ModalityAudio:
		return strings.TrimSpace(firstString(payload, "audio_ref"))
	case ModalityVision:
		return strings.TrimSpace(firstString(payload, "image_ref"))
	default:
		if v := strings.TrimSpace(firstString(payload, "audio_ref")); v != "" {
			return v
		}
		return strings.TrimSpace(firstString(payload, "image_ref"))
	}
}

func capturedAtFromPayload(payload map[string]any, fallback time.Time) time.Time {
	raw := payload["captured_at"]
	if raw == nil {
		raw = payload["timestamp"]
	}
	if raw == nil {
		raw = payload["ts"]
	}
	switch value := raw.(type) {
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return fallback
		}
		if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
			return parsed.UTC()
		}
		if n, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return unixTimestamp(n)
		}
	case json.Number:
		if n, err := value.Int64(); err == nil {
			return unixTimestamp(n)
		}
	case float64:
		return unixTimestamp(int64(value))
	case int:
		return unixTimestamp(int64(value))
	case int64:
		return unixTimestamp(value)
	}
	return fallback
}

func unixTimestamp(value int64) time.Time {
	if value > 1_000_000_000_000 {
		return time.UnixMilli(value).UTC()
	}
	return time.Unix(value, 0).UTC()
}

func stringSliceFromAny(v any) []string {
	values, ok := v.([]any)
	if !ok {
		if typed, ok := v.([]string); ok {
			return normalizeStrings(typed)
		}
		return nil
	}
	out := make([]string, 0, len(values))
	for _, item := range values {
		if s := strings.TrimSpace(asStringAny(item)); s != "" {
			out = append(out, s)
		}
	}
	return normalizeStrings(out)
}

func normalizeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func clonePayload(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		out[key] = value
	}
	return out
}
