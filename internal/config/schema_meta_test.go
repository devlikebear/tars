package config

import "testing"

func TestSchemaIncludesFieldMetaBadges(t *testing.T) {
	fields := Schema()
	byKey := make(map[string]FieldMeta, len(fields))
	for _, field := range fields {
		byKey[field.Key] = field
	}

	pulse := byKey["pulse_interval"]
	if !pulse.RequiresRestart {
		t.Fatalf("pulse_interval RequiresRestart = false, want true")
	}
	if pulse.DefaultValue != "1m" {
		t.Fatalf("pulse_interval DefaultValue = %#v, want 1m", pulse.DefaultValue)
	}

	workspace := byKey["workspace_dir"]
	if workspace.DefaultValue != DefaultWorkspaceDir() {
		t.Fatalf("workspace_dir DefaultValue = %#v, want %q", workspace.DefaultValue, DefaultWorkspaceDir())
	}

	token := byKey["api_admin_token"]
	if !token.Sensitive {
		t.Fatalf("api_admin_token Sensitive = false, want true")
	}
}
