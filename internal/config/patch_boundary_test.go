package config

import "testing"

func TestPatchValidationInputShapes(t *testing.T) {
	for _, tc := range []struct {
		key   string
		value any
		valid bool
	}{
		{"agent_max_iterations", 3, true},
		{"pulse_enabled", true, true},
		{"mcp_command_allowlist_json", []string{"https://example.com"}, true},
		{"mcp_command_allowlist_json", `["https://example.com"]`, true},
		{"mcp_command_allowlist_json", `[42]`, false},
		{"llm_role_defaults", `{"chat_main":"heavy"}`, true},
		{"llm_role_defaults", `null`, false},
		{"llm_role_defaults", `{`, false},
	} {
		t.Run(tc.key, func(t *testing.T) {
			err := validatePatchValue(tc.key, tc.value)
			if (err == nil) != tc.valid {
				t.Fatalf("value %v: %v", tc.value, err)
			}
		})
	}
}

func TestRawProviderBoundaryRejectsInvalidShapes(t *testing.T) {
	for _, input := range []any{make(chan int), `{`, `{"":{}}`, `{"p":null}`, `{"p":{"unexpected":"value"}}`, `{"p":{"kind":false}}`} {
		if _, err := rawProviderObjects(input, submittedProviderObjects); err == nil {
			t.Fatalf("accepted %v", input)
		}
	}
	// Invalid existing objects must never pick a credential or overwrite a file.
	if _, err := mergeRawProviderEdit(map[string]any{"llm_providers": map[string]any{"p": 42}}, map[string]any{"p": map[string]any{"kind": "openai"}}); err == nil {
		t.Fatal("accepted malformed existing provider")
	}
}
