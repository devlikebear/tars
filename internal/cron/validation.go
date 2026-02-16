package cron

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	cronv3 "github.com/robfig/cron/v3"
)

func normalizeSchedule(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "every:1h", nil
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "at:") {
		ts := strings.TrimSpace(s[len("at:"):])
		at, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			return "", fmt.Errorf("invalid schedule: %s (expected at:<rfc3339>, every:<duration>, or valid cron expression)", s)
		}
		return "at:" + at.UTC().Format(time.RFC3339), nil
	}
	if strings.HasPrefix(lower, "every:") {
		dur := strings.TrimSpace(s[len("every:"):])
		if dur == "" {
			return "", fmt.Errorf("invalid schedule: %s (expected at:<rfc3339>, every:<duration>, or valid cron expression)", s)
		}
		if _, err := time.ParseDuration(dur); err != nil {
			return "", fmt.Errorf("invalid schedule: %s (expected at:<rfc3339>, every:<duration>, or valid cron expression)", s)
		}
		return "every:" + dur, nil
	}
	if strings.HasPrefix(lower, "@every ") {
		dur := strings.TrimSpace(s[len("@every "):])
		if _, err := time.ParseDuration(dur); err != nil {
			return "", fmt.Errorf("invalid schedule: %s (expected at:<rfc3339>, every:<duration>, or valid cron expression)", s)
		}
		return "@every " + dur, nil
	}
	if _, err := cronv3.ParseStandard(s); err != nil {
		return "", fmt.Errorf("invalid schedule: %s (expected at:<rfc3339>, every:<duration>, or valid cron expression)", s)
	}
	return s, nil
}

func normalizeSessionTarget(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "isolated", nil
	}
	switch strings.ToLower(v) {
	case "isolated":
		return "isolated", nil
	case "main":
		return "main", nil
	default:
		return v, nil
	}
}

func normalizeWakeMode(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "agent_loop", nil
	}
	if strings.EqualFold(v, "agent_loop") {
		return "agent_loop", nil
	}
	return "", fmt.Errorf("invalid wake_mode: %s (expected agent_loop)", v)
}

func normalizeDeliveryMode(raw, sessionTarget string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		if strings.EqualFold(sessionTarget, "main") {
			return "session", nil
		}
		return "daily_log", nil
	}
	switch strings.ToLower(v) {
	case "none":
		return "none", nil
	case "daily_log":
		return "daily_log", nil
	case "session":
		return "session", nil
	case "both":
		return "both", nil
	default:
		return "", fmt.Errorf("invalid delivery_mode: %s (expected none|daily_log|session|both)", v)
	}
}

func normalizePayload(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	if !json.Valid([]byte(trimmed)) {
		return nil, fmt.Errorf("payload must be valid json")
	}
	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return nil, fmt.Errorf("payload decode failed: %w", err)
	}
	if _, ok := decoded.(map[string]any); !ok {
		return nil, fmt.Errorf("payload must be a json object")
	}
	buf := &bytes.Buffer{}
	if err := json.Compact(buf, []byte(trimmed)); err != nil {
		return nil, fmt.Errorf("payload compact failed: %w", err)
	}
	return json.RawMessage(buf.Bytes()), nil
}

func parseAtTime(schedule string) (time.Time, bool, error) {
	s := strings.TrimSpace(schedule)
	if !strings.HasPrefix(strings.ToLower(s), "at:") {
		return time.Time{}, false, nil
	}
	v := strings.TrimSpace(s[len("at:"):])
	at, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, true, err
	}
	return at.UTC(), true, nil
}

func looksOneShotCronSchedule(schedule string) bool {
	parts := strings.Fields(strings.TrimSpace(schedule))
	if len(parts) != 5 {
		return false
	}
	// Heuristic: concrete minute/hour/day/month + wildcard weekday is usually
	// intended as a one-time calendar trigger in this app's UX.
	if !isSimpleCronNumber(parts[0]) || !isSimpleCronNumber(parts[1]) {
		return false
	}
	if !isSimpleCronNumber(parts[2]) || !isSimpleCronNumber(parts[3]) {
		return false
	}
	return parts[4] == "*" || parts[4] == "?"
}

func isSimpleCronNumber(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return false
	}
	for _, ch := range v {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}
