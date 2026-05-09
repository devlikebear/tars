package sessionoverride

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func TestScaffoldLocalCreatesProjectTarsLayout(t *testing.T) {
	cwd := t.TempDir()

	result, err := ScaffoldLocal(cwd, false)
	if err != nil {
		t.Fatalf("scaffold local: %v", err)
	}

	for _, path := range []string{
		result.SettingsPath,
		result.LocalSettingsPath,
		result.SkillsDir,
		result.CommandsDir,
		result.GitignorePath,
	} {
		assertPathExists(t, path)
	}

	rawShared, err := os.ReadFile(result.SettingsPath)
	if err != nil {
		t.Fatalf("read shared settings: %v", err)
	}
	var shared Override
	if err := json.Unmarshal(rawShared, &shared); err != nil {
		t.Fatalf("decode shared settings: %v", err)
	}
	if shared.ToolConfig == nil {
		t.Fatalf("expected shared settings to include tool_config, got %s", string(rawShared))
	}

	rawGitignore, err := os.ReadFile(result.GitignorePath)
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !strings.Contains(string(rawGitignore), localSettingsName) {
		t.Fatalf("expected .tars/.gitignore to ignore %s, got %s", localSettingsName, string(rawGitignore))
	}

	second, err := ScaffoldLocal(cwd, false)
	if err != nil {
		t.Fatalf("scaffold local second time: %v", err)
	}
	if len(second.Created) != 0 {
		t.Fatalf("expected idempotent second run to create nothing, got %+v", second.Created)
	}
}

func TestScaffoldLocalRejectsBlankCWD(t *testing.T) {
	if _, err := ScaffoldLocal(" \t ", false); err == nil {
		t.Fatal("expected blank cwd to fail")
	}
}

func TestScaffoldLocalPreservesExistingFilesAndCanForceOverwrite(t *testing.T) {
	cwd := t.TempDir()
	tarsDir := filepath.Join(cwd, ".tars")
	if err := os.MkdirAll(filepath.Join(tarsDir, "skills"), 0o755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tarsDir, "commands"), 0o755); err != nil {
		t.Fatalf("mkdir commands: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tarsDir, "settings.json"), []byte(`{"prompt_override":"keep"}`), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tarsDir, "settings.local.json"), []byte(`{"prompt_override":"local"}`), 0o644); err != nil {
		t.Fatalf("write local settings: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tarsDir, ".gitignore"), []byte("notes"), 0o644); err != nil {
		t.Fatalf("write gitignore: %v", err)
	}

	result, err := ScaffoldLocal(cwd, false)
	if err != nil {
		t.Fatalf("scaffold existing local: %v", err)
	}
	if len(result.Created) != 1 || result.Created[0] != result.GitignorePath {
		t.Fatalf("expected only .gitignore append to be reported as created, got %+v", result.Created)
	}
	rawShared, err := os.ReadFile(result.SettingsPath)
	if err != nil {
		t.Fatalf("read shared settings: %v", err)
	}
	if string(rawShared) != `{"prompt_override":"keep"}` {
		t.Fatalf("expected existing shared settings preserved, got %s", string(rawShared))
	}
	rawGitignore, err := os.ReadFile(result.GitignorePath)
	if err != nil {
		t.Fatalf("read gitignore: %v", err)
	}
	if string(rawGitignore) != "notes\nsettings.local.json\n" {
		t.Fatalf("expected local settings appended to .gitignore, got %q", string(rawGitignore))
	}

	forced, err := ScaffoldLocal(cwd, true)
	if err != nil {
		t.Fatalf("force scaffold local: %v", err)
	}
	rawShared, err = os.ReadFile(forced.SettingsPath)
	if err != nil {
		t.Fatalf("read forced shared settings: %v", err)
	}
	if !strings.Contains(string(rawShared), `"tool_config": {}`) {
		t.Fatalf("expected forced scaffold to rewrite shared settings, got %s", string(rawShared))
	}
}

func TestScaffoldLocalReportsPathConflicts(t *testing.T) {
	cwd := t.TempDir()
	tarsDir := filepath.Join(cwd, ".tars")
	if err := os.MkdirAll(tarsDir, 0o755); err != nil {
		t.Fatalf("mkdir .tars: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tarsDir, "skills"), []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("write skills file: %v", err)
	}
	if _, err := ScaffoldLocal(cwd, false); err == nil {
		t.Fatal("expected skills file conflict to fail")
	}

	cwd = t.TempDir()
	tarsDir = filepath.Join(cwd, ".tars")
	if err := os.MkdirAll(filepath.Join(tarsDir, "skills"), 0o755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tarsDir, "commands"), 0o755); err != nil {
		t.Fatalf("mkdir commands: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tarsDir, "settings.json"), 0o755); err != nil {
		t.Fatalf("mkdir settings conflict: %v", err)
	}
	if _, err := ScaffoldLocal(cwd, false); err == nil {
		t.Fatal("expected settings directory conflict to fail")
	}
}

func assertPathExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected path to exist %s: %v", path, err)
	}
}
