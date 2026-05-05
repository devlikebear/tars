package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeAliasFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestLoadCommandAliases_MissingDirIsNoop(t *testing.T) {
	defs, diags := LoadCommandAliases(filepath.Join(t.TempDir(), "nope"), nil)
	if len(defs) != 0 || len(diags) != 0 {
		t.Fatalf("expected no defs/diags, got defs=%+v diags=%+v", defs, diags)
	}
}

func TestLoadCommandAliases_ClonesTargetSkill(t *testing.T) {
	available := []Definition{
		{
			Name:        "refactor",
			Description: "the refactor skill",
			Slash:       "refactor",
			Aliases:     []string{"rf"},
			Content:     "do the thing",
			Source:      SourceBundled,
			FilePath:    "/some/SKILL.md",
		},
	}
	dir := filepath.Join(t.TempDir(), "commands")
	writeAliasFile(t, dir, "tidy.md", `---
target_skill: refactor
description: tidy up code style
---

ignored body
`)

	defs, diags := LoadCommandAliases(dir, available)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	if len(defs) != 1 {
		t.Fatalf("expected 1 alias, got %d", len(defs))
	}
	d := defs[0]
	if d.Name != "tidy" || d.Slash != "tidy" {
		t.Fatalf("alias name/slash mismatch: %+v", d)
	}
	if d.Source != SourceSessionCwd {
		t.Fatalf("alias source should be SourceSessionCwd, got %q", d.Source)
	}
	if d.Description != "tidy up code style" {
		t.Fatalf("description should be the alias-provided one, got %q", d.Description)
	}
	if d.Content != "do the thing" {
		t.Fatalf("alias should inherit target content, got %q", d.Content)
	}
	if len(d.Aliases) != 0 {
		t.Fatalf("alias should not carry the target's aliases (%+v)", d.Aliases)
	}
}

func TestLoadCommandAliases_InheritsDescriptionWhenOmitted(t *testing.T) {
	available := []Definition{{Name: "refactor", Description: "the refactor skill", Content: "x"}}
	dir := filepath.Join(t.TempDir(), "commands")
	writeAliasFile(t, dir, "rf.md", "---\ntarget_skill: refactor\n---\n")

	defs, diags := LoadCommandAliases(dir, available)
	if len(diags) != 0 {
		t.Fatalf("diags: %+v", diags)
	}
	if len(defs) != 1 || defs[0].Description != "the refactor skill" {
		t.Fatalf("alias should inherit description, got %+v", defs)
	}
}

func TestLoadCommandAliases_MissingTargetSkill(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "commands")
	writeAliasFile(t, dir, "orphan.md", "---\ntarget_skill: nonexistent\n---\n")

	defs, diags := LoadCommandAliases(dir, []Definition{{Name: "other"}})
	if len(defs) != 0 {
		t.Fatalf("expected no defs, got %+v", defs)
	}
	if len(diags) != 1 || !strings.Contains(diags[0].Message, "not found") {
		t.Fatalf("expected single 'not found' diagnostic, got %+v", diags)
	}
}

func TestLoadCommandAliases_RequiresTargetSkillFrontmatter(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "commands")
	writeAliasFile(t, dir, "broken.md", "---\ndescription: missing target\n---\n")

	defs, diags := LoadCommandAliases(dir, nil)
	if len(defs) != 0 {
		t.Fatalf("expected no defs, got %+v", defs)
	}
	if len(diags) != 1 || !strings.Contains(diags[0].Message, "target_skill") {
		t.Fatalf("expected diagnostic about missing target_skill, got %+v", diags)
	}
}

func TestLoadCommandAliases_NonMarkdownFilesIgnored(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "commands")
	writeAliasFile(t, dir, "README", "this is not a command file")
	writeAliasFile(t, dir, "ok.md", "---\ntarget_skill: refactor\n---\n")

	defs, _ := LoadCommandAliases(dir, []Definition{{Name: "refactor", Content: "x"}})
	if len(defs) != 1 || defs[0].Name != "ok" {
		t.Fatalf("expected only ok.md to be loaded, got %+v", defs)
	}
}
