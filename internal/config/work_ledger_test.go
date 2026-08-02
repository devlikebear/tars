package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkLedgerConfigSupportsNonDestructiveRollback(t *testing.T) {
	t.Run("default enabled", func(t *testing.T) {
		cfg, err := Load("")
		if err != nil {
			t.Fatalf("load defaults: %v", err)
		}
		if !cfg.WorkLedger.Enabled {
			t.Fatal("work ledger should be enabled by default")
		}
	})

	t.Run("yaml disables default", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte("work_ledger:\n  enabled: false\n"), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("load yaml: %v", err)
		}
		if cfg.WorkLedger.Enabled {
			t.Fatal("work ledger should be disabled by yaml")
		}
	})

	t.Run("environment disables default", func(t *testing.T) {
		t.Setenv("TARS_WORK_LEDGER_ENABLED", "false")
		cfg, err := Load("")
		if err != nil {
			t.Fatalf("load environment: %v", err)
		}
		if cfg.WorkLedger.Enabled {
			t.Fatal("work ledger should be disabled by environment")
		}
	})

	t.Run("settings patch preserves false", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte("work_ledger:\n  enabled: true\n"), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}
		if err := PatchYAML(path, map[string]any{"work_ledger_enabled": false}); err != nil {
			t.Fatalf("patch config: %v", err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read patched config: %v", err)
		}
		if !strings.Contains(string(raw), "enabled: false") {
			t.Fatalf("patched config did not preserve false:\n%s", raw)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("load patched config: %v", err)
		}
		if cfg.WorkLedger.Enabled {
			t.Fatal("work ledger should remain disabled after settings patch")
		}
		if got := ConfigToMap(cfg)["work_ledger_enabled"]; got != false {
			t.Fatalf("config map work_ledger_enabled = %#v, want false", got)
		}
	})
}

func TestConfigSchemaIncludesWorkLedgerRollbackSwitch(t *testing.T) {
	for _, field := range Schema() {
		if field.Key != "work_ledger_enabled" {
			continue
		}
		if field.Section != "Work Ledger" || field.Path != "work_ledger.enabled" || field.Type != "bool" {
			t.Fatalf("unexpected work ledger schema: %+v", field)
		}
		if !field.RequiresRestart {
			t.Fatal("work ledger rollback switch should require restart")
		}
		return
	}
	t.Fatal("schema missing work_ledger_enabled")
}
