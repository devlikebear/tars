package skill

import "testing"

func TestParseFrontmatter(t *testing.T) {
	raw := `---
name: quick_notes
description: Quick note capture helper
user-invocable: false
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
	if meta.Name != "" || meta.Description != "" || meta.UserInvocable != nil {
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
