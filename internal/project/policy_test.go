package project

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeToolPolicy_CanonicalizesGroupsPatternsDenyAndRisk(t *testing.T) {
	known := map[string]struct{}{
		"read_file":     {},
		"exec":          {},
		"list_dir":      {},
		"memory_get":    {},
		"memory_save":   {},
		"memory_search": {},
		"glob":          {},
	}
	spec := ToolPolicySpec{
		ToolsAllow:               []string{"read_file", "shell_exec"},
		ToolsAllowExists:         true,
		ToolsAllowGroups:         []string{"memory"},
		ToolsAllowGroupsExists:   true,
		ToolsAllowPatterns:       []string{"^list_", "^gl"},
		ToolsAllowPatternsExists: true,
		ToolsDeny:                []string{"exec"},
		ToolsDenyExists:          true,
		ToolsRiskMax:             "low",
		ToolsRiskMaxExists:       true,
	}

	policy := NormalizeToolPolicy(spec, known, ToolPolicyOptions{})

	if got, want := strings.Join(policy.ToolsAllow, ","), "exec,read_file"; got != want {
		t.Fatalf("unexpected canonical tools_allow: got=%q want=%q", got, want)
	}
	if got, want := strings.Join(policy.ToolsAllowGroups, ","), "memory"; got != want {
		t.Fatalf("unexpected canonical tools_allow_groups: got=%q want=%q", got, want)
	}
	if got, want := strings.Join(policy.ToolsAllowPatterns, ","), "^gl,^list_"; got != want {
		t.Fatalf("unexpected canonical tools_allow_patterns: got=%q want=%q", got, want)
	}
	if got, want := strings.Join(policy.ToolsDeny, ","), "exec"; got != want {
		t.Fatalf("unexpected canonical tools_deny: got=%q want=%q", got, want)
	}
	if policy.ToolsRiskMax != "low" {
		t.Fatalf("expected tools_risk_max=low, got %q", policy.ToolsRiskMax)
	}
	if got, want := strings.Join(policy.AllowedTools, ","), "list_dir,memory_get,memory_save,memory_search,read_file"; got != want {
		t.Fatalf("unexpected expanded allowed tools: got=%q want=%q", got, want)
	}
}

func TestApplyToolPolicy_ConstrainsBaseTools(t *testing.T) {
	base := []string{"read_file", "exec", "glob"}
	policy := NormalizedToolPolicy{
		ToolsDeny:    []string{"exec"},
		ToolsRiskMax: "low",
	}

	got := ApplyToolPolicy(base, policy)

	if want := []string{"read_file"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected constrained tools: got=%v want=%v", got, want)
	}
}

func TestRenderPromptContext_ChatAndCron(t *testing.T) {
	item := Project{
		ID:         "proj_demo",
		Name:       "Demo Project",
		Type:       "operations",
		Status:     "active",
		Objective:  "Keep service green",
		ToolsAllow: []string{"read_file", "exec"},
		Body:       "Check alerts first",
	}

	chat := RenderPromptContext(item, PromptContextOptions{
		Header:           "## Active Project",
		IncludeObjective: true,
		IncludeToolsAllow: true,
		IncludeBody:      true,
	})
	if !strings.Contains(chat, "- objective: Keep service green") {
		t.Fatalf("expected objective in chat context, got %q", chat)
	}
	if !strings.Contains(chat, "- tools_allow: read_file, exec") {
		t.Fatalf("expected tools_allow in chat context, got %q", chat)
	}
	if !strings.Contains(chat, "Check alerts first") {
		t.Fatalf("expected instructions body in chat context, got %q", chat)
	}

	cron := RenderPromptContext(item, PromptContextOptions{
		Header:           "CRON_PROJECT_CONTEXT:",
		FieldPrefix:      "project_",
		ArtifactsDir:     filepath.Join("/workspace", "projects", item.ID),
		IncludeBody:      true,
		BodyHeader:       "PROJECT_INSTRUCTIONS:",
	})
	if !strings.Contains(cron, "- project_id: proj_demo") {
		t.Fatalf("expected project_id in cron context, got %q", cron)
	}
	if !strings.Contains(cron, "- artifacts_dir: /workspace/projects/proj_demo") {
		t.Fatalf("expected artifacts_dir in cron context, got %q", cron)
	}
	if !strings.Contains(cron, "PROJECT_INSTRUCTIONS:") {
		t.Fatalf("expected body header in cron context, got %q", cron)
	}
}
