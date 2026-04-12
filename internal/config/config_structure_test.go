package config

import (
	"reflect"
	"testing"
)

func TestConfig_UsesFocusedEmbeddedGroups(t *testing.T) {
	t.Parallel()

	cfgType := reflect.TypeOf(Config{})
	wantEmbedded := []string{
		"RuntimeConfig",
		"APIConfig",
		"LLMConfig",
		"MemoryConfig",
		"UsageConfig",
		"AutomationConfig",
		"AssistantConfig",
		"CompactionConfig",
		"ToolConfig",
		"VaultConfig",
		"BrowserConfig",
		"GatewayConfig",
		"ChannelConfig",
		"ExtensionConfig",
	}

	if cfgType.NumField() != len(wantEmbedded) {
		t.Fatalf("expected %d top-level config groups, got %d", len(wantEmbedded), cfgType.NumField())
	}

	for index, name := range wantEmbedded {
		field := cfgType.Field(index)
		if field.Name != name {
			t.Fatalf("expected config field %d to be %q, got %q", index, name, field.Name)
		}
		if !field.Anonymous {
			t.Fatalf("expected %q to be embedded", name)
		}
	}
}
