package sessionoverride

import (
	"reflect"
	"sort"
	"testing"

	"github.com/devlikebear/tars/internal/session"
)

func ptr[T any](v T) *T { return &v }

func TestMerge_BaseOnly(t *testing.T) {
	base := session.SessionToolConfig{
		ToolsCustom:  true,
		ToolsEnabled: []string{"read_file"},
	}
	eff, sources := Merge(base, "base prompt", nil, nil)

	if !reflect.DeepEqual(eff.ToolConfig, base) {
		t.Fatalf("tool_config mismatch: got=%+v want=%+v", eff.ToolConfig, base)
	}
	if eff.PromptOverride != "base prompt" {
		t.Fatalf("prompt mismatch: %q", eff.PromptOverride)
	}
	for _, p := range AllPaths() {
		if sources[p] != SourceBase {
			t.Fatalf("source for %q should be base, got %q", p, sources[p])
		}
	}
}

func TestMerge_SharedReplacesPrompt_LocalReplacesAgain(t *testing.T) {
	base := session.SessionToolConfig{}
	shared := &Override{
		PromptOverride: ptr("shared prompt"),
		Presence:       map[string]bool{"prompt_override": true},
	}
	local := &Override{
		PromptOverride: ptr("local prompt"),
		Presence:       map[string]bool{"prompt_override": true},
	}

	eff, sources := Merge(base, "base prompt", shared, local)

	if eff.PromptOverride != "local prompt" {
		t.Fatalf("expected local to win, got %q", eff.PromptOverride)
	}
	if sources["prompt_override"] != SourceLocal {
		t.Fatalf("source should be local, got %q", sources["prompt_override"])
	}
}

func TestMerge_SharedSetsPrompt_LocalDoesnt(t *testing.T) {
	shared := &Override{
		PromptOverride: ptr("shared prompt"),
		Presence:       map[string]bool{"prompt_override": true},
	}
	eff, sources := Merge(session.SessionToolConfig{}, "", shared, nil)

	if eff.PromptOverride != "shared prompt" {
		t.Fatalf("expected shared, got %q", eff.PromptOverride)
	}
	if sources["prompt_override"] != SourceShared {
		t.Fatalf("source should be shared")
	}
}

func TestMerge_SliceUnionDedup_AcrossLayers(t *testing.T) {
	base := session.SessionToolConfig{
		ToolsEnabled: []string{"read_file", "list_dir"},
	}
	shared := &Override{
		ToolConfig: &session.SessionToolConfig{
			ToolsEnabled: []string{"list_dir", "glob"},
		},
		Presence: map[string]bool{"tool_config.tools_enabled": true},
	}
	local := &Override{
		ToolConfig: &session.SessionToolConfig{
			ToolsEnabled: []string{"glob", "exec"},
		},
		Presence: map[string]bool{"tool_config.tools_enabled": true},
	}

	eff, sources := Merge(base, "", shared, local)

	got := append([]string(nil), eff.ToolConfig.ToolsEnabled...)
	sort.Strings(got)
	want := []string{"exec", "glob", "list_dir", "read_file"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("union+dedup mismatch: got=%+v want=%+v", got, want)
	}
	if sources["tool_config.tools_enabled"] != SourceLocal {
		t.Fatalf("source should be local (highest layer that touched it), got %q",
			sources["tool_config.tools_enabled"])
	}
}

func TestMerge_CustomAllowlistsReplaceInheritedProjectSettings(t *testing.T) {
	base := session.SessionToolConfig{
		ToolsEnabled:    []string{"base-tool"},
		SkillsEnabled:   []string{"base-skill"},
		CommandsEnabled: []string{"base-command"},
		MCPEnabled:      []string{"base-mcp"},
	}
	shared := &Override{
		ToolConfig: &session.SessionToolConfig{
			ToolsEnabled:    []string{"shared-tool"},
			SkillsEnabled:   []string{"shared-skill"},
			CommandsEnabled: []string{"shared-command"},
			MCPEnabled:      []string{"shared-mcp"},
		},
		Presence: map[string]bool{
			"tool_config.tools_enabled":    true,
			"tool_config.skills_enabled":   true,
			"tool_config.commands_enabled": true,
			"tool_config.mcp_enabled":      true,
		},
	}
	local := &Override{
		ToolConfig: &session.SessionToolConfig{
			ToolsCustom:     true,
			ToolsEnabled:    []string{"local-tool"},
			SkillsCustom:    true,
			SkillsEnabled:   []string{"local-skill"},
			CommandsCustom:  true,
			CommandsEnabled: []string{"local-command"},
			MCPCustom:       true,
			MCPEnabled:      []string{"local-mcp"},
		},
		Presence: map[string]bool{
			"tool_config.tools_custom":     true,
			"tool_config.tools_enabled":    true,
			"tool_config.skills_custom":    true,
			"tool_config.skills_enabled":   true,
			"tool_config.commands_custom":  true,
			"tool_config.commands_enabled": true,
			"tool_config.mcp_custom":       true,
			"tool_config.mcp_enabled":      true,
		},
	}

	eff, sources := Merge(base, "", shared, local)

	if !reflect.DeepEqual(eff.ToolConfig.ToolsEnabled, []string{"local-tool"}) {
		t.Fatalf("local tools_custom should replace inherited tools, got %+v", eff.ToolConfig.ToolsEnabled)
	}
	if !reflect.DeepEqual(eff.ToolConfig.SkillsEnabled, []string{"local-skill"}) {
		t.Fatalf("local skills_custom should replace inherited skills, got %+v", eff.ToolConfig.SkillsEnabled)
	}
	if !reflect.DeepEqual(eff.ToolConfig.CommandsEnabled, []string{"local-command"}) {
		t.Fatalf("local commands_custom should replace inherited commands, got %+v", eff.ToolConfig.CommandsEnabled)
	}
	if !reflect.DeepEqual(eff.ToolConfig.MCPEnabled, []string{"local-mcp"}) {
		t.Fatalf("local mcp_custom should replace inherited MCP entries, got %+v", eff.ToolConfig.MCPEnabled)
	}
	for _, path := range []string{
		"tool_config.tools_enabled",
		"tool_config.skills_enabled",
		"tool_config.commands_enabled",
		"tool_config.mcp_enabled",
	} {
		if sources[path] != SourceLocal {
			t.Fatalf("source for %s should be local, got %q", path, sources[path])
		}
	}
}

func TestMerge_CustomFlagsClearOmittedAllowlists(t *testing.T) {
	base := session.SessionToolConfig{
		ToolsEnabled:    []string{"base-tool"},
		SkillsEnabled:   []string{"base-skill"},
		CommandsEnabled: []string{"base-command"},
		MCPEnabled:      []string{"base-mcp"},
	}
	local := &Override{
		ToolConfig: &session.SessionToolConfig{
			ToolsCustom:    true,
			SkillsCustom:   true,
			CommandsCustom: true,
			MCPCustom:      true,
		},
		Presence: map[string]bool{
			"tool_config.tools_custom":    true,
			"tool_config.skills_custom":   true,
			"tool_config.commands_custom": true,
			"tool_config.mcp_custom":      true,
		},
	}

	eff, sources := Merge(base, "", nil, local)

	if len(eff.ToolConfig.ToolsEnabled) != 0 {
		t.Fatalf("tools_custom without tools_enabled should clear inherited tools, got %+v", eff.ToolConfig.ToolsEnabled)
	}
	if len(eff.ToolConfig.SkillsEnabled) != 0 {
		t.Fatalf("skills_custom without skills_enabled should clear inherited skills, got %+v", eff.ToolConfig.SkillsEnabled)
	}
	if len(eff.ToolConfig.CommandsEnabled) != 0 {
		t.Fatalf("commands_custom without commands_enabled should clear inherited commands, got %+v", eff.ToolConfig.CommandsEnabled)
	}
	if len(eff.ToolConfig.MCPEnabled) != 0 {
		t.Fatalf("mcp_custom without mcp_enabled should clear inherited MCP, got %+v", eff.ToolConfig.MCPEnabled)
	}
	for _, path := range []string{
		"tool_config.tools_enabled",
		"tool_config.skills_enabled",
		"tool_config.commands_enabled",
		"tool_config.mcp_enabled",
	} {
		if sources[path] != SourceLocal {
			t.Fatalf("source for %s should be local, got %q", path, sources[path])
		}
	}
}

func TestMerge_BoolReplaceSemantics(t *testing.T) {
	base := session.SessionToolConfig{ToolsCustom: false}
	shared := &Override{
		ToolConfig: &session.SessionToolConfig{ToolsCustom: true},
		Presence:   map[string]bool{"tool_config.tools_custom": true},
	}
	local := &Override{
		ToolConfig: &session.SessionToolConfig{ToolsCustom: false},
		Presence:   map[string]bool{"tool_config.tools_custom": true},
	}

	eff, sources := Merge(base, "", shared, local)
	if eff.ToolConfig.ToolsCustom {
		t.Fatalf("local should win and set false, got true")
	}
	if sources["tool_config.tools_custom"] != SourceLocal {
		t.Fatalf("source should be local")
	}
}

func TestMerge_MCPServersExtra_MergedByName(t *testing.T) {
	shared := &Override{
		MCPServersExtra: []MCPServerExtra{
			{Name: "fs", Command: "shared-fs"},
			{Name: "web", Command: "shared-web"},
		},
		Presence: map[string]bool{"mcp_servers_extra": true},
	}
	local := &Override{
		MCPServersExtra: []MCPServerExtra{
			{Name: "fs", Command: "local-fs", Args: []string{"--read-only"}},
		},
		Presence: map[string]bool{"mcp_servers_extra": true},
	}

	eff, sources := Merge(session.SessionToolConfig{}, "", shared, local)

	byName := map[string]MCPServerExtra{}
	for _, e := range eff.MCPServersExtra {
		byName[e.Name] = e
	}
	if len(byName) != 2 {
		t.Fatalf("expected 2 MCP entries, got %+v", eff.MCPServersExtra)
	}
	if byName["fs"].Command != "local-fs" || !reflect.DeepEqual(byName["fs"].Args, []string{"--read-only"}) {
		t.Fatalf("local should override fs entry, got %+v", byName["fs"])
	}
	if byName["web"].Command != "shared-web" {
		t.Fatalf("web should remain from shared, got %+v", byName["web"])
	}
	if sources["mcp_servers_extra"] != SourceLocal {
		t.Fatalf("source should be local")
	}
}

func TestMerge_OnlyTouchedFieldsGetOverrideSource(t *testing.T) {
	// shared touches prompt_override only; local touches nothing.
	// All other paths should report base.
	shared := &Override{
		PromptOverride: ptr("shared"),
		Presence:       map[string]bool{"prompt_override": true},
	}
	_, sources := Merge(session.SessionToolConfig{}, "base", shared, nil)

	if sources["prompt_override"] != SourceShared {
		t.Fatalf("prompt_override source should be shared")
	}
	for _, p := range AllPaths() {
		if p == "prompt_override" {
			continue
		}
		if sources[p] != SourceBase {
			t.Fatalf("path %q should be base when no layer touched it, got %q",
				p, sources[p])
		}
	}
}

func TestMerge_ExercisesAllToolConfigSubfields(t *testing.T) {
	base := session.SessionToolConfig{
		ToolsEnabled:     []string{"a"},
		ToolsDisabled:    []string{"b"},
		ToolsAllowGroups: []string{"files"},
		ToolsDenyGroups:  []string{"shell"},
		SkillsEnabled:    []string{"s1"},
		CommandsEnabled:  []string{"cmd1"},
		MCPEnabled:       []string{"fs"},
	}
	shared := &Override{
		ToolConfig: &session.SessionToolConfig{
			ToolsEnabled:     []string{"c"},
			ToolsDisabled:    []string{"d"},
			ToolsAllowGroups: []string{"web"},
			ToolsDenyGroups:  []string{"net"},
			SkillsEnabled:    []string{"s2"},
			SkillsCustom:     true,
			CommandsEnabled:  []string{"cmd2"},
			CommandsCustom:   true,
			MCPEnabled:       []string{"web"},
			MCPCustom:        true,
		},
		Presence: map[string]bool{
			"tool_config.tools_enabled":      true,
			"tool_config.tools_disabled":     true,
			"tool_config.tools_allow_groups": true,
			"tool_config.tools_deny_groups":  true,
			"tool_config.skills_enabled":     true,
			"tool_config.skills_custom":      true,
			"tool_config.commands_enabled":   true,
			"tool_config.commands_custom":    true,
			"tool_config.mcp_enabled":        true,
			"tool_config.mcp_custom":         true,
		},
	}

	eff, sources := Merge(base, "", shared, nil)

	checkUnion := func(name string, got, want []string) {
		t.Helper()
		gs := append([]string(nil), got...)
		ws := append([]string(nil), want...)
		sort.Strings(gs)
		sort.Strings(ws)
		if !reflect.DeepEqual(gs, ws) {
			t.Fatalf("%s mismatch: got=%+v want=%+v", name, gs, ws)
		}
	}
	checkUnion("ToolsEnabled", eff.ToolConfig.ToolsEnabled, []string{"a", "c"})
	checkUnion("ToolsDisabled", eff.ToolConfig.ToolsDisabled, []string{"b", "d"})
	checkUnion("ToolsAllowGroups", eff.ToolConfig.ToolsAllowGroups, []string{"files", "web"})
	checkUnion("ToolsDenyGroups", eff.ToolConfig.ToolsDenyGroups, []string{"shell", "net"})
	checkUnion("SkillsEnabled", eff.ToolConfig.SkillsEnabled, []string{"s2"})
	checkUnion("CommandsEnabled", eff.ToolConfig.CommandsEnabled, []string{"cmd2"})
	checkUnion("MCPEnabled", eff.ToolConfig.MCPEnabled, []string{"web"})
	if !eff.ToolConfig.SkillsCustom {
		t.Fatalf("SkillsCustom should be true from shared")
	}
	if !eff.ToolConfig.CommandsCustom {
		t.Fatalf("CommandsCustom should be true from shared")
	}
	if !eff.ToolConfig.MCPCustom {
		t.Fatalf("MCPCustom should be true from shared")
	}
	for _, p := range []string{
		"tool_config.tools_enabled",
		"tool_config.tools_disabled",
		"tool_config.tools_allow_groups",
		"tool_config.tools_deny_groups",
		"tool_config.skills_enabled",
		"tool_config.skills_custom",
		"tool_config.commands_enabled",
		"tool_config.commands_custom",
		"tool_config.mcp_enabled",
		"tool_config.mcp_custom",
	} {
		if sources[p] != SourceShared {
			t.Fatalf("source %q should be shared, got %q", p, sources[p])
		}
	}
}

func TestMerge_ModelTierOverride(t *testing.T) {
	shared := &Override{
		ModelTierOverride: ptr("fast"),
		Presence:          map[string]bool{"model_tier_override": true},
	}
	eff, sources := Merge(session.SessionToolConfig{}, "", shared, nil)
	if eff.ModelTierOverride != "fast" {
		t.Fatalf("expected fast tier, got %q", eff.ModelTierOverride)
	}
	if sources["model_tier_override"] != SourceShared {
		t.Fatalf("source should be shared")
	}
}

func TestMerge_MCPServersExtra_NilNext(t *testing.T) {
	shared := &Override{
		MCPServersExtra: []MCPServerExtra{{Name: "fs", Command: "shared-fs"}},
		Presence:        map[string]bool{"mcp_servers_extra": true},
	}
	local := &Override{
		MCPServersExtra: nil,
		Presence:        map[string]bool{"mcp_servers_extra": true},
	}
	eff, _ := Merge(session.SessionToolConfig{}, "", shared, local)
	if len(eff.MCPServersExtra) != 1 || eff.MCPServersExtra[0].Command != "shared-fs" {
		t.Fatalf("nil-next should preserve prior, got %+v", eff.MCPServersExtra)
	}
}

func TestMerge_NilOverridesAreNoop(t *testing.T) {
	base := session.SessionToolConfig{ToolsEnabled: []string{"read_file"}}
	eff, sources := Merge(base, "base", nil, nil)

	if !reflect.DeepEqual(eff.ToolConfig.ToolsEnabled, []string{"read_file"}) {
		t.Fatalf("base preserved")
	}
	if sources["prompt_override"] != SourceBase {
		t.Fatalf("source should be base when no overrides given")
	}
}

// TestMerge_ClaudeCodeCLIPermissionMode_LocalBeatsShared verifies the new
// scalar field rides the same last-write-wins precedence as prompt_override.
func TestMerge_ClaudeCodeCLIPermissionMode_LocalBeatsShared(t *testing.T) {
	shared := &Override{
		ClaudeCodeCLIPermissionMode: ptr("plan"),
		Presence:                    map[string]bool{"claude_code_cli_permission_mode": true},
	}
	local := &Override{
		ClaudeCodeCLIPermissionMode: ptr("acceptEdits"),
		Presence:                    map[string]bool{"claude_code_cli_permission_mode": true},
	}
	eff, sources := Merge(session.SessionToolConfig{}, "", shared, local)
	if eff.ClaudeCodeCLIPermissionMode != "acceptEdits" {
		t.Fatalf("expected local 'acceptEdits' to win, got %q", eff.ClaudeCodeCLIPermissionMode)
	}
	if sources["claude_code_cli_permission_mode"] != SourceLocal {
		t.Fatalf("source: got %q want local", sources["claude_code_cli_permission_mode"])
	}
}

// TestMerge_ClaudeCodeCLIPermissionMode_OnlyShared confirms a shared-only
// override is honored and its source attribution is correct.
func TestMerge_ClaudeCodeCLIPermissionMode_OnlyShared(t *testing.T) {
	shared := &Override{
		ClaudeCodeCLIPermissionMode: ptr("plan"),
		Presence:                    map[string]bool{"claude_code_cli_permission_mode": true},
	}
	eff, sources := Merge(session.SessionToolConfig{}, "", shared, nil)
	if eff.ClaudeCodeCLIPermissionMode != "plan" {
		t.Fatalf("expected 'plan' from shared, got %q", eff.ClaudeCodeCLIPermissionMode)
	}
	if sources["claude_code_cli_permission_mode"] != SourceShared {
		t.Fatalf("source: got %q want shared", sources["claude_code_cli_permission_mode"])
	}
}

// TestMerge_ClaudeCodeCLIPermissionMode_NoOverride_LeavesBlank verifies that
// when no override sets the field, the merged value stays empty so the
// handler falls back to the global config (which itself fallbacks to "auto"
// in the provider).
func TestMerge_ClaudeCodeCLIPermissionMode_NoOverride_LeavesBlank(t *testing.T) {
	eff, sources := Merge(session.SessionToolConfig{}, "", nil, nil)
	if eff.ClaudeCodeCLIPermissionMode != "" {
		t.Fatalf("expected empty effective when no overrides, got %q", eff.ClaudeCodeCLIPermissionMode)
	}
	if sources["claude_code_cli_permission_mode"] != SourceBase {
		t.Fatalf("source: got %q want base", sources["claude_code_cli_permission_mode"])
	}
}
