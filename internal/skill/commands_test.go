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

func TestLoadCommandAliases_LoadsLegacyTargetSkillFileAsStandaloneCommand(t *testing.T) {
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
		t.Fatalf("expected 1 command, got %d", len(defs))
	}
	d := defs[0]
	if d.Name != "tidy" || d.Slash != "tidy" {
		t.Fatalf("command name/slash mismatch: %+v", d)
	}
	if d.Source != SourceSessionCwd {
		t.Fatalf("command source should be SourceSessionCwd, got %q", d.Source)
	}
	if d.Description != "tidy up code style" {
		t.Fatalf("description should come from the command file, got %q", d.Description)
	}
	if d.Content != "ignored body" {
		t.Fatalf("command should use its own body, got %q", d.Content)
	}
	if len(d.Aliases) != 0 {
		t.Fatalf("command should not carry the target's aliases (%+v)", d.Aliases)
	}
}

func TestLoadCommandAliases_InfersDescriptionWhenOmitted(t *testing.T) {
	available := []Definition{{Name: "refactor", Description: "the refactor skill", Content: "x"}}
	dir := filepath.Join(t.TempDir(), "commands")
	writeAliasFile(t, dir, "rf.md", "---\ntarget_skill: refactor\n---\nRefactor only the files mentioned by the user.")

	defs, diags := LoadCommandAliases(dir, available)
	if len(diags) != 0 {
		t.Fatalf("diags: %+v", diags)
	}
	if len(defs) != 1 || !strings.Contains(defs[0].Description, "Refactor only") {
		t.Fatalf("command should infer description from its own body, got %+v", defs)
	}
}

func TestLoadCommandAliases_IgnoresMissingLegacyTargetSkill(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "commands")
	writeAliasFile(t, dir, "orphan.md", "---\ntarget_skill: nonexistent\n---\nUse this command body.")

	defs, diags := LoadCommandAliases(dir, []Definition{{Name: "other"}})
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics for legacy target_skill, got %+v", diags)
	}
	if len(defs) != 1 || defs[0].Name != "orphan" || !strings.Contains(defs[0].Content, "Use this command body") {
		t.Fatalf("expected standalone command, got defs=%+v diags=%+v", defs, diags)
	}
}

func TestLoadCommandAliases_LoadsStandaloneCommandWithoutTargetSkill(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "commands")
	writeAliasFile(t, dir, "메모.md", `---
description: save an explicit memory note
recommended_tools:
  - memory
---

# /메모

Save the text after the slash command.
`)

	defs, diags := LoadCommandAliases(dir, nil)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	if len(defs) != 1 {
		t.Fatalf("expected 1 standalone command, got %+v", defs)
	}
	d := defs[0]
	if d.Name != "메모" || d.Slash != "메모" {
		t.Fatalf("standalone command name/slash mismatch: %+v", d)
	}
	if d.Description != "save an explicit memory note" {
		t.Fatalf("description mismatch: %+v", d)
	}
	if d.Source != SourceSessionCwd || !d.UserInvocable {
		t.Fatalf("expected session user-invocable command, got %+v", d)
	}
	if d.RuntimePath == "" || d.FilePath == "" {
		t.Fatalf("expected command paths to be set, got %+v", d)
	}
	if len(d.RecommendedTools) != 1 || d.RecommendedTools[0] != "memory" {
		t.Fatalf("recommended tools mismatch: %+v", d.RecommendedTools)
	}
	if !strings.Contains(d.Content, "Save the text") {
		t.Fatalf("expected command body content, got %q", d.Content)
	}
}

func TestLoadCommandAliases_NonMarkdownFilesIgnored(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "commands")
	writeAliasFile(t, dir, "README", "this is not a command file")
	writeAliasFile(t, dir, "ok.md", "---\ndescription: ok command\n---\nRun ok.")

	defs, _ := LoadCommandAliases(dir, []Definition{{Name: "refactor", Content: "x"}})
	if len(defs) != 1 || defs[0].Name != "ok" {
		t.Fatalf("expected only ok.md to be loaded, got %+v", defs)
	}
}
