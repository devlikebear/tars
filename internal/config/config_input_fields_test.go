package config

import "testing"

func TestConfigInputFieldsCarryPreferredYAMLPaths(t *testing.T) {
	tests := []struct {
		key  string
		path string
	}{
		{key: "api_max_inflight_chat", path: "api.max_inflight.chat"},
		{key: "llm_providers", path: "llm.providers"},
		{key: "log_rotate_max_size_mb", path: "log.rotate.max_size_mb"},
		{key: "memory_embed_base_url", path: "memory.embed.base_url"},
		{key: "usage_daily_token_budget", path: "usage.limits.daily_tokens"},
		{key: "pulse_allowed_autofixes_json", path: "automation.pulse.allowed_autofixes"},
		{key: "remote_access_tailscale_serve_enabled", path: "remote_access.tailscale_serve.enabled"},
		{key: "remote_access_tailscale_serve_https_port", path: "remote_access.tailscale_serve.https_port"},
	}

	for _, tt := range tests {
		field, ok := configInputFieldByYAMLKey(tt.key)
		if !ok {
			t.Fatalf("field %q not registered", tt.key)
		}
		if field.yamlPath != tt.path {
			t.Fatalf("field %q yamlPath = %q, want %q", tt.key, field.yamlPath, tt.path)
		}
		if got := preferredYAMLPathForKey(tt.key); got != tt.path {
			t.Fatalf("preferred YAML path for %q = %q, want %q", tt.key, got, tt.path)
		}
	}
}
