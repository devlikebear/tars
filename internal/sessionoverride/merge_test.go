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
			MCPEnabled:       []string{"web"},
		},
		Presence: map[string]bool{
			"tool_config.tools_enabled":      true,
			"tool_config.tools_disabled":     true,
			"tool_config.tools_allow_groups": true,
			"tool_config.tools_deny_groups":  true,
			"tool_config.skills_enabled":     true,
			"tool_config.skills_custom":      true,
			"tool_config.mcp_enabled":        true,
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
	checkUnion("SkillsEnabled", eff.ToolConfig.SkillsEnabled, []string{"s1", "s2"})
	checkUnion("MCPEnabled", eff.ToolConfig.MCPEnabled, []string{"fs", "web"})
	if !eff.ToolConfig.SkillsCustom {
		t.Fatalf("SkillsCustom should be true from shared")
	}
	for _, p := range []string{
		"tool_config.tools_enabled",
		"tool_config.tools_disabled",
		"tool_config.tools_allow_groups",
		"tool_config.tools_deny_groups",
		"tool_config.skills_enabled",
		"tool_config.skills_custom",
		"tool_config.mcp_enabled",
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
