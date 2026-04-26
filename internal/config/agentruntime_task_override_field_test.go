package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoad_AgentRuntimeTaskOverrideField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := []byte("agentruntime_task_override:\n  enabled: true\n  allowed_aliases: [anthropic_prod, anthropic_dev]\n  allowed_models: [claude-opus]\n")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.AgentRuntimeTaskOverride.Enabled {
		t.Fatalf("expected override to be enabled")
	}
	if !reflect.DeepEqual(cfg.AgentRuntimeTaskOverride.AllowedAliases, []string{"anthropic_prod", "anthropic_dev"}) {
		t.Fatalf("unexpected allowed aliases: %+v", cfg.AgentRuntimeTaskOverride.AllowedAliases)
	}
	if !reflect.DeepEqual(cfg.AgentRuntimeTaskOverride.AllowedModels, []string{"claude-opus"}) {
		t.Fatalf("unexpected allowed models: %+v", cfg.AgentRuntimeTaskOverride.AllowedModels)
	}
}
