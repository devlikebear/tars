package sessionoverride

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoad_NoTarsDir_ReturnsNils(t *testing.T) {
	cwd := t.TempDir()
	shared, local, diags, err := Load(cwd)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if shared != nil || local != nil {
		t.Fatalf("expected nil overrides, got shared=%v local=%v", shared, local)
	}
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %+v", diags)
	}
}

func TestLoad_EmptyCwd_ReturnsNils(t *testing.T) {
	shared, local, diags, err := Load("")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if shared != nil || local != nil || len(diags) != 0 {
		t.Fatalf("expected all nils for empty cwd")
	}
}

func TestLoad_OnlyShared_ParsesAndMarksPresence(t *testing.T) {
	cwd := t.TempDir()
	writeSettings(t, cwd, "settings.json", `{
		"prompt_override": "be terse",
		"tool_config": {
			"tools_custom": true,
			"tools_enabled": ["read_file", "list_dir"]
		}
	}`)

	shared, local, diags, err := Load(cwd)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if local != nil {
		t.Fatalf("expected nil local, got %+v", local)
	}
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	if shared == nil {
		t.Fatal("expected shared override")
	}
	if shared.PromptOverride == nil || *shared.PromptOverride != "be terse" {
		t.Fatalf("unexpected prompt_override: %+v", shared.PromptOverride)
	}
	if shared.ToolConfig == nil || !shared.ToolConfig.ToolsCustom ||
		!reflect.DeepEqual(shared.ToolConfig.ToolsEnabled, []string{"read_file", "list_dir"}) {
		t.Fatalf("unexpected tool_config: %+v", shared.ToolConfig)
	}
	wantPresence := map[string]bool{
		"prompt_override":           true,
		"tool_config.tools_custom":  true,
		"tool_config.tools_enabled": true,
	}
	if !reflect.DeepEqual(shared.Presence, wantPresence) {
		t.Fatalf("presence mismatch: got=%+v want=%+v", shared.Presence, wantPresence)
	}
}

func TestLoad_BothFilesPresent(t *testing.T) {
	cwd := t.TempDir()
	writeSettings(t, cwd, "settings.json", `{"prompt_override":"shared"}`)
	writeSettings(t, cwd, "settings.local.json", `{"prompt_override":"local"}`)

	shared, local, _, err := Load(cwd)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if shared == nil || *shared.PromptOverride != "shared" {
		t.Fatalf("shared mismatch: %+v", shared)
	}
	if local == nil || *local.PromptOverride != "local" {
		t.Fatalf("local mismatch: %+v", local)
	}
}

func TestLoad_BrokenJSON_ReturnsError(t *testing.T) {
	cwd := t.TempDir()
	writeSettings(t, cwd, "settings.json", `{not valid json`)

	_, _, _, err := Load(cwd)
	if err == nil {
		t.Fatal("expected error on broken JSON")
	}
	if !strings.Contains(err.Error(), "settings.json") {
		t.Fatalf("expected file path in error, got %v", err)
	}
}

func TestLoad_BlockedField_GeneratesErrorDiagnostic(t *testing.T) {
	cwd := t.TempDir()
	writeSettings(t, cwd, "settings.json", `{
		"llm_providers": {"anthropic": {"api_key": "sk-secret"}},
		"prompt_override": "ok"
	}`)

	shared, _, diags, err := Load(cwd)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if shared == nil || shared.PromptOverride == nil || *shared.PromptOverride != "ok" {
		t.Fatalf("expected allowed field to survive: %+v", shared)
	}
	var found bool
	for _, d := range diags {
		if d.Path == "llm_providers" && d.Severity == SeverityError {
			found = true
			if !strings.HasSuffix(d.File, "settings.json") {
				t.Fatalf("diagnostic should reference settings.json: %+v", d)
			}
		}
	}
	if !found {
		t.Fatalf("expected error diagnostic for llm_providers, got %+v", diags)
	}
}

func TestLoad_UnknownField_GeneratesWarnDiagnostic(t *testing.T) {
	cwd := t.TempDir()
	writeSettings(t, cwd, "settings.json", `{
		"unknown_field": 42,
		"prompt_override": "ok"
	}`)

	shared, _, diags, err := Load(cwd)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if shared == nil || *shared.PromptOverride != "ok" {
		t.Fatalf("expected allowed field to survive: %+v", shared)
	}
	var warn bool
	for _, d := range diags {
		if d.Path == "unknown_field" && d.Severity == SeverityWarn {
			warn = true
		}
	}
	if !warn {
		t.Fatalf("expected warn diagnostic for unknown_field, got %+v", diags)
	}
}

func TestLoad_UnknownToolConfigField_GeneratesWarn(t *testing.T) {
	cwd := t.TempDir()
	writeSettings(t, cwd, "settings.json", `{
		"tool_config": {
			"tools_enabled": ["read_file"],
			"unknown_subfield": true
		}
	}`)

	shared, _, diags, err := Load(cwd)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if shared == nil || shared.ToolConfig == nil ||
		!reflect.DeepEqual(shared.ToolConfig.ToolsEnabled, []string{"read_file"}) {
		t.Fatalf("expected tools_enabled to survive: %+v", shared)
	}
	var warn bool
	for _, d := range diags {
		if d.Path == "tool_config.unknown_subfield" && d.Severity == SeverityWarn {
			warn = true
		}
	}
	if !warn {
		t.Fatalf("expected warn for tool_config.unknown_subfield, got %+v", diags)
	}
}

func TestLoad_EmptyObject_NoPresence(t *testing.T) {
	cwd := t.TempDir()
	writeSettings(t, cwd, "settings.json", `{}`)

	shared, _, diags, err := Load(cwd)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if shared == nil {
		t.Fatal("expected non-nil Override even when empty")
	}
	if len(shared.Presence) != 0 {
		t.Fatalf("expected empty presence, got %+v", shared.Presence)
	}
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %+v", diags)
	}
}

func TestLoad_AllToolConfigSubfields(t *testing.T) {
	cwd := t.TempDir()
	writeSettings(t, cwd, "settings.json", `{
		"tool_config": {
			"tools_enabled": ["a"],
			"tools_disabled": ["b"],
			"tools_allow_groups": ["files"],
			"tools_deny_groups": ["shell"],
			"skills_enabled": ["s1"],
			"skills_custom": true,
			"commands_enabled": ["c1"],
			"commands_custom": true,
			"mcp_enabled": ["fs"],
			"mcp_custom": true,
			"tools_custom": true
		}
	}`)
	shared, _, _, err := Load(cwd)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if shared == nil || shared.ToolConfig == nil {
		t.Fatal("expected tool_config")
	}
	tc := shared.ToolConfig
	if !reflect.DeepEqual(tc.ToolsEnabled, []string{"a"}) ||
		!reflect.DeepEqual(tc.ToolsDisabled, []string{"b"}) ||
		!reflect.DeepEqual(tc.ToolsAllowGroups, []string{"files"}) ||
		!reflect.DeepEqual(tc.ToolsDenyGroups, []string{"shell"}) ||
		!reflect.DeepEqual(tc.SkillsEnabled, []string{"s1"}) ||
		!reflect.DeepEqual(tc.CommandsEnabled, []string{"c1"}) ||
		!reflect.DeepEqual(tc.MCPEnabled, []string{"fs"}) ||
		!tc.ToolsCustom || !tc.SkillsCustom || !tc.CommandsCustom || !tc.MCPCustom {
		t.Fatalf("subfields mismatch: %+v", tc)
	}
	for _, key := range []string{
		"tool_config.tools_enabled", "tool_config.tools_disabled",
		"tool_config.tools_allow_groups", "tool_config.tools_deny_groups",
		"tool_config.skills_enabled", "tool_config.skills_custom",
		"tool_config.commands_enabled", "tool_config.commands_custom",
		"tool_config.mcp_enabled", "tool_config.mcp_custom", "tool_config.tools_custom",
	} {
		if !shared.Presence[key] {
			t.Fatalf("missing presence for %q", key)
		}
	}
}

func TestLoad_ModelTierOverride(t *testing.T) {
	cwd := t.TempDir()
	writeSettings(t, cwd, "settings.json", `{"model_tier_override": "fast"}`)
	shared, _, _, err := Load(cwd)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if shared.ModelTierOverride == nil || *shared.ModelTierOverride != "fast" {
		t.Fatalf("expected fast, got %+v", shared.ModelTierOverride)
	}
	if !shared.Presence["model_tier_override"] {
		t.Fatalf("presence missing")
	}
}

func TestLoad_TarsIsFile_NotDir(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, settingsDirName), []byte("oops"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	shared, local, diags, err := Load(cwd)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if shared != nil || local != nil || len(diags) != 0 {
		t.Fatalf("expected nils when .tars is a file")
	}
}

func TestLoad_MCPServersExtra(t *testing.T) {
	cwd := t.TempDir()
	writeSettings(t, cwd, "settings.json", `{
		"mcp_servers_extra": [
			{"name": "fs", "command": "mcp-fs", "args": ["--root", "."]}
		]
	}`)

	shared, _, _, err := Load(cwd)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if shared == nil || len(shared.MCPServersExtra) != 1 ||
		shared.MCPServersExtra[0].Name != "fs" || shared.MCPServersExtra[0].Command != "mcp-fs" {
		t.Fatalf("unexpected mcp_servers_extra: %+v", shared)
	}
	if !shared.Presence["mcp_servers_extra"] {
		t.Fatalf("presence should mark mcp_servers_extra")
	}
}

func writeSettings(t *testing.T, cwd, name, body string) {
	t.Helper()
	dir := filepath.Join(cwd, ".tars")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir .tars: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
