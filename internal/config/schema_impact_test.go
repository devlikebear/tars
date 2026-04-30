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

func containsImpact(items []string, want string) bool {
	want = strings.ToLower(want)
	for _, item := range items {
		if strings.Contains(strings.ToLower(item), want) {
			return true
		}
	}
	return false
}
