package skill_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/pkg/skill"
)

func TestLoadSkillThroughPublicPackage(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	dir := filepath.Join(root, "summarize")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	raw := `---
name: summarize
description: Summarize a codebase.
recommended_tools: [read_file, glob]
---
Read the code and summarize the architecture.
`
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(raw), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	snapshot, err := skill.Load(skill.LoadOptions{
		Sources: []skill.SourceDir{{Source: skill.SourceWorkspace, Dir: root}},
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(snapshot.Skills) != 1 {
		t.Fatalf("len(Skills) = %d, want 1", len(snapshot.Skills))
	}
	formatted := skill.FormatAvailableSkills(snapshot.Skills)
	if !strings.Contains(formatted, "<name>summarize</name>") {
		t.Fatalf("formatted skills = %q", formatted)
	}
}

func TestPublicCommandAndPromoteHelpers(t *testing.T) {
	root := t.TempDir()
	commandsDir := filepath.Join(root, "commands")
	if err := os.MkdirAll(commandsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(commands) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(commandsDir, "review.md"), []byte("---\ndescription: Review code.\n---\nReview the current diff."), 0o644); err != nil {
		t.Fatalf("WriteFile(command) error = %v", err)
	}
	commands, diagnostics := skill.LoadCommands(commandsDir)
	if len(diagnostics) != 0 {
		t.Fatalf("LoadCommands() diagnostics = %+v", diagnostics)
	}
	if len(commands) != 1 || commands[0].Name != "review" {
		t.Fatalf("LoadCommands() = %+v", commands)
	}
	aliases, diagnostics := skill.LoadCommandAliases(commandsDir, nil)
	if len(diagnostics) != 0 || len(aliases) != 1 {
		t.Fatalf("LoadCommandAliases() = %+v diagnostics=%+v", aliases, diagnostics)
	}
	meta, body, err := skill.ParseFrontmatter("---\nname: x\n---\nbody")
	if err != nil {
		t.Fatalf("ParseFrontmatter() error = %v", err)
	}
	if meta.Name != "x" || strings.TrimSpace(body) != "body" {
		t.Fatalf("ParseFrontmatter() = %+v %q", meta, body)
	}

	sourceRoot := filepath.Join(root, "source")
	targetRoot := filepath.Join(root, "target")
	sourceSkill := filepath.Join(sourceRoot, "helper")
	if err := os.MkdirAll(sourceSkill, 0o755); err != nil {
		t.Fatalf("MkdirAll(skill) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceSkill, "SKILL.md"), []byte("---\nname: helper\n---\nHelp."), 0o644); err != nil {
		t.Fatalf("WriteFile(skill) error = %v", err)
	}
	result, err := skill.Promote(skill.PromoteRequest{
		SourceSkillsRoot: sourceRoot,
		TargetSkillsRoot: targetRoot,
		Name:             "helper",
		Mode:             skill.PromoteModeCopy,
		OnConflict:       skill.PromoteOnConflictAbort,
	})
	if err != nil {
		t.Fatalf("Promote() error = %v", err)
	}
	if result.Action != skill.PromoteActionCreated {
		t.Fatalf("Promote().Action = %q", result.Action)
	}
}
