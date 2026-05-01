package config

import (
	"strings"
	"testing"
)

func TestSchemaIncludesImpactHintsForHighSignalFields(t *testing.T) {
	fields := Schema()
	byKey := make(map[string]FieldMeta, len(fields))
	for _, field := range fields {
		byKey[field.Key] = field
	}

	pulse := byKey["pulse_interval"]
	if len(pulse.Impact) == 0 {
		t.Fatalf("pulse_interval Impact is empty")
	}
	if !containsImpact(pulse.Impact, "tick cadence") {
		t.Fatalf("pulse_interval Impact = %#v, want tick cadence hint", pulse.Impact)
	}

	logLevel := byKey["log_level"]
	if !containsImpact(logLevel.Impact, "debug") {
		t.Fatalf("log_level Impact = %#v, want debug hint", logLevel.Impact)
	}
}

func TestSchemaImpactHintsCoverCoreSubsystems(t *testing.T) {
	fields := Schema()
	byKey := make(map[string]FieldMeta, len(fields))
	for _, field := range fields {
		byKey[field.Key] = field
	}

	checks := map[string][]string{
		"llm_tiers":                    {"LLM routing"},
		"api_admin_token":              {"admin"},
		"style_autonomy_default":       {"consent"},
		"pulse_allowed_autofixes_json": {"autofix"},
		"reflection_sleep_window":      {"nightly"},
		"cron_run_history_limit":       {"cron"},
		"memory_embed_model":           {"embedding"},
		"agentruntime_enabled":         {"Agent Runtime"},
		"skills_enabled":               {"skill"},
		"plugins_allow_mcp_servers":    {"MCP"},
	}
	for key, wants := range checks {
		field, ok := byKey[key]
		if !ok {
			t.Fatalf("schema field %q not found", key)
		}
		for _, want := range wants {
			if !containsImpact(field.Impact, want) {
				t.Fatalf("%s Impact = %#v, want %q", key, field.Impact, want)
			}
		}
	}
}

func containsImpact(items []string, want string) bool {
	want = strings.ToLower(want)
	for _, item := range items {
		if strings.Contains(strings.ToLower(item), want) {
			return true
		}
	}
	return false
}
