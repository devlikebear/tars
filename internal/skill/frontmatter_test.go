package skill

import "testing"

func TestParseFrontmatter(t *testing.T) {
	raw := `---
name: quick_notes
description: Quick note capture helper
user-invocable: false
recommended_tools:
  - read_file
  - write_file
recommended_project_files: [BRIEF.md, STATE.md]
wake_phases: plan, draft
---
# Quick Notes
Use this skill for short capture.
`
	meta, body, err := ParseFrontmatter(raw)
	if err != nil {
		t.Fatalf("parse frontmatter: %v", err)
	}
	if meta.Name != "quick_notes" {
		t.Fatalf("expected name quick_notes, got %q", meta.Name)
	}
	if meta.Description != "Quick note capture helper" {
		t.Fatalf("unexpected description: %q", meta.Description)
	}
	if meta.UserInvocable == nil || *meta.UserInvocable {
		t.Fatalf("expected user-invocable=false, got %+v", meta.UserInvocable)
	}
	if got := len(meta.RecommendedTools); got != 2 {
		t.Fatalf("expected 2 recommended tools, got %+v", meta.RecommendedTools)
	}
	if got := len(meta.RecommendedProjectFiles); got != 2 {
		t.Fatalf("expected 2 recommended project files, got %+v", meta.RecommendedProjectFiles)
	}
	if got := len(meta.WakePhases); got != 2 {
		t.Fatalf("expected 2 wake phases, got %+v", meta.WakePhases)
	}
	if body == "" || body[0] != '#' {
		t.Fatalf("expected markdown body without frontmatter, got %q", body)
	}
}

func TestParseFrontmatter_DefaultWhenMissing(t *testing.T) {
	raw := "# Skill\nNo frontmatter."
	meta, body, err := ParseFrontmatter(raw)
	if err != nil {
		t.Fatalf("parse frontmatter without metadata: %v", err)
	}
	if meta.Name != "" || meta.Description != "" || meta.UserInvocable != nil || len(meta.RecommendedTools) != 0 {
		t.Fatalf("expected empty frontmatter for plain markdown, got %+v", meta)
	}
	if body != raw {
		t.Fatalf("expected unchanged body, got %q", body)
	}
}

func TestParseFrontmatter_InvalidBlock(t *testing.T) {
	raw := `---
name: broken`
	_, _, err := ParseFrontmatter(raw)
	if err == nil {
		t.Fatal("expected error for unterminated frontmatter")
	}
}
