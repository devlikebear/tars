package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComputerUseDefaultAndOverrides(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ToolsComputerUseEnabled {
		t.Fatal("computer_use must be enabled by default")
	}
	if cfg.ToolsComputerUseBackend != "llm" {
		t.Fatalf("backend = %q", cfg.ToolsComputerUseBackend)
	}
	if cfg.LLMRoleDefaults["computer_use"] != "light" {
		t.Fatalf("computer_use role = %q", cfg.LLMRoleDefaults["computer_use"])
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("tools:\n  computer_use:\n    enabled: false\n    backend: jev\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ToolsComputerUseEnabled || cfg.ToolsComputerUseBackend != "jev" {
		t.Fatalf("override = %+v", cfg.ToolConfig)
	}
	t.Setenv("TARS_TOOLS_COMPUTER_USE_BACKEND", "llm")
	cfg, err = Load(path)
	if err != nil || cfg.ToolsComputerUseBackend != "llm" {
		t.Fatalf("env override: %q %v", cfg.ToolsComputerUseBackend, err)
	}
}

func TestComputerUseRejectsUnknownBackend(t *testing.T) {
	t.Setenv("TARS_TOOLS_COMPUTER_USE_BACKEND", "typo")
	if _, err := Load(""); err == nil {
		t.Fatal("unknown backend must be rejected")
	}
}

func TestComputerUseDriverDefaultsAndValidation(t *testing.T) {
	cfg, err := Load("")
	if err != nil || cfg.ToolsComputerUseDriver != "cua-driver" {
		t.Fatalf("default: %q %v", cfg.ToolsComputerUseDriver, err)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "tools:\n  computer_use:\n    driver: agent-desktop\n    agent_desktop_path: /opt/agent-desktop\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path)
	if err != nil || cfg.ToolsComputerUseDriver != "agent-desktop" || cfg.ToolsComputerUseAgentDesktopPath != "/opt/agent-desktop" {
		t.Fatalf("yaml: %q %q %v", cfg.ToolsComputerUseDriver, cfg.ToolsComputerUseAgentDesktopPath, err)
	}
	t.Setenv("TARS_TOOLS_COMPUTER_USE_DRIVER", "typo")
	if _, err := Load(""); err == nil {
		t.Fatal("unknown driver must be rejected")
	}
}

func TestComputerUseWithholdsValuesWhenExplicitlyDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("tools:\n  computer_use:\n    expose_values: false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ToolsComputerUseExposeValues {
		t.Fatal("explicit false must withhold field contents")
	}
}
