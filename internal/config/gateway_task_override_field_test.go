package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoad_GatewayTaskOverrideField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := []byte("gateway_task_override:\n  enabled: true\n  allowed_aliases: [anthropic_prod, anthropic_dev]\n  allowed_models: [claude-opus]\n")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.GatewayTaskOverride.Enabled {
		t.Fatalf("expected override to be enabled")
	}
	if !reflect.DeepEqual(cfg.GatewayTaskOverride.AllowedAliases, []string{"anthropic_prod", "anthropic_dev"}) {
		t.Fatalf("unexpected allowed aliases: %+v", cfg.GatewayTaskOverride.AllowedAliases)
	}
	if !reflect.DeepEqual(cfg.GatewayTaskOverride.AllowedModels, []string{"claude-opus"}) {
		t.Fatalf("unexpected allowed models: %+v", cfg.GatewayTaskOverride.AllowedModels)
	}
}
