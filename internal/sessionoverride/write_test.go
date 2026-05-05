package sessionoverride

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/devlikebear/tars/internal/session"
)

func TestWriteLocalToolConfigPreservesOtherLocalSettings(t *testing.T) {
	cwd := t.TempDir()
	writeSettings(t, cwd, "settings.local.json", `{
		"prompt_override": "local prompt"
	}`)

	config := session.SessionToolConfig{
		ToolsCustom:     true,
		ToolsEnabled:    []string{"read_file"},
		SkillsCustom:    true,
		SkillsEnabled:   []string{"project-review"},
		CommandsCustom:  true,
		CommandsEnabled: []string{"메모"},
	}
	if err := WriteLocalToolConfig(cwd, config); err != nil {
		t.Fatalf("write local config: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(cwd, ".tars", "settings.local.json"))
	if err != nil {
		t.Fatalf("read settings.local.json: %v", err)
	}
	var payload struct {
		PromptOverride string                    `json:"prompt_override"`
		ToolConfig     session.SessionToolConfig `json:"tool_config"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode local settings: %v", err)
	}
	if payload.PromptOverride != "local prompt" {
		t.Fatalf("prompt override was not preserved: %q", payload.PromptOverride)
	}
	if !payload.ToolConfig.ToolsCustom || !reflect.DeepEqual(payload.ToolConfig.ToolsEnabled, []string{"read_file"}) {
		t.Fatalf("tool config mismatch: %+v", payload.ToolConfig)
	}
	if !payload.ToolConfig.SkillsCustom || !reflect.DeepEqual(payload.ToolConfig.SkillsEnabled, []string{"project-review"}) {
		t.Fatalf("skill config mismatch: %+v", payload.ToolConfig)
	}
	if !payload.ToolConfig.CommandsCustom || !reflect.DeepEqual(payload.ToolConfig.CommandsEnabled, []string{"메모"}) {
		t.Fatalf("command config mismatch: %+v", payload.ToolConfig)
	}
}

func TestWriteLocalToolConfigClearsEmptyToolConfig(t *testing.T) {
	cwd := t.TempDir()
	writeSettings(t, cwd, "settings.local.json", `{
		"prompt_override": "local prompt",
		"tool_config": {"tools_custom": true, "tools_enabled": ["read_file"]}
	}`)

	if err := WriteLocalToolConfig(cwd, session.SessionToolConfig{}); err != nil {
		t.Fatalf("write empty local config: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(cwd, ".tars", "settings.local.json"))
	if err != nil {
		t.Fatalf("read settings.local.json: %v", err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode local settings: %v", err)
	}
	if _, ok := payload["tool_config"]; ok {
		t.Fatalf("expected tool_config to be removed, got %s", string(raw))
	}
	if _, ok := payload["prompt_override"]; !ok {
		t.Fatalf("expected prompt_override to be preserved, got %s", string(raw))
	}
}

func TestWriteLocalToolConfigKeepsEmptyMCPCustomAllowlist(t *testing.T) {
	cwd := t.TempDir()

	if err := WriteLocalToolConfig(cwd, session.SessionToolConfig{MCPCustom: true}); err != nil {
		t.Fatalf("write mcp custom config: %v", err)
	}

	_, local, diags, err := Load(cwd)
	if err != nil {
		t.Fatalf("load local config: %v", err)
	}
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	if local == nil || local.ToolConfig == nil || !local.ToolConfig.MCPCustom {
		t.Fatalf("expected local mcp_custom to be preserved, got %+v", local)
	}
	eff, sources := Merge(session.SessionToolConfig{MCPEnabled: []string{"base-fs"}}, "", nil, local)
	if !eff.ToolConfig.MCPCustom {
		t.Fatalf("expected effective mcp_custom")
	}
	if len(eff.ToolConfig.MCPEnabled) != 0 {
		t.Fatalf("expected empty custom MCP allowlist to clear inherited servers, got %+v", eff.ToolConfig.MCPEnabled)
	}
	if sources["tool_config.mcp_enabled"] != SourceLocal {
		t.Fatalf("expected mcp_enabled source to be local, got %q", sources["tool_config.mcp_enabled"])
	}
}
