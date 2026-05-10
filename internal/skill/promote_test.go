package skill_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/skill"
)

func writeSkillSource(t *testing.T, dir, name, body string) string {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	return skillDir
}

func TestPromoteCopyCreatesTarget(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "session", "skills")
	target := filepath.Join(tmp, "workspace", "skills")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	srcDir := writeSkillSource(t, src, "alpha", "---\nname: alpha\n---\nbody\n")

	res, err := skill.Promote(skill.PromoteRequest{
		SourceSkillDir:   srcDir,
		TargetSkillsRoot: target,
		Name:             "alpha",
		Mode:             skill.PromoteModeCopy,
		OnConflict:       skill.PromoteOnConflictRename,
	})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if res.Action != skill.PromoteActionCreated {
		t.Fatalf("expected created, got %s", res.Action)
	}
	if res.TargetName != "alpha" {
		t.Fatalf("unexpected target name: %s", res.TargetName)
	}
	if res.SourceDeleted {
		t.Fatalf("copy should not delete source")
	}
	if _, err := os.Stat(filepath.Join(target, "alpha", "SKILL.md")); err != nil {
		t.Fatalf("target SKILL.md missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(srcDir, "SKILL.md")); err != nil {
		t.Fatalf("source SKILL.md should still exist: %v", err)
	}
}

func TestPromoteMoveDeletesSource(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "session", "skills")
	target := filepath.Join(tmp, "workspace", "skills")
	srcDir := writeSkillSource(t, src, "beta", "---\nname: beta\n---\nbody\n")

	res, err := skill.Promote(skill.PromoteRequest{
		SourceSkillDir:   srcDir,
		TargetSkillsRoot: target,
		Name:             "beta",
		Mode:             skill.PromoteModeMove,
		OnConflict:       skill.PromoteOnConflictRename,
	})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !res.SourceDeleted {
		t.Fatalf("move should delete source")
	}
	if _, err := os.Stat(srcDir); !os.IsNotExist(err) {
		t.Fatalf("source dir should be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "beta", "SKILL.md")); err != nil {
		t.Fatalf("target SKILL.md missing: %v", err)
	}
}

func TestPromoteRenameOnConflict(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "session", "skills")
	target := filepath.Join(tmp, "workspace", "skills")
	writeSkillSource(t, target, "gamma", "existing")
	writeSkillSource(t, target, "gamma-2", "existing-2")
	srcDir := writeSkillSource(t, src, "gamma", "fresh")

	res, err := skill.Promote(skill.PromoteRequest{
		SourceSkillDir:   srcDir,
		TargetSkillsRoot: target,
		Name:             "gamma",
		Mode:             skill.PromoteModeCopy,
		OnConflict:       skill.PromoteOnConflictRename,
	})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if res.Action != skill.PromoteActionRenamed {
		t.Fatalf("expected renamed, got %s", res.Action)
	}
	if res.TargetName != "gamma-3" {
		t.Fatalf("expected gamma-3, got %s", res.TargetName)
	}
	contents, err := os.ReadFile(filepath.Join(target, "gamma-3", "SKILL.md"))
	if err != nil {
		t.Fatalf("read renamed target: %v", err)
	}
	if string(contents) != "fresh" {
		t.Fatalf("expected fresh body, got %q", string(contents))
	}
	if existing, err := os.ReadFile(filepath.Join(target, "gamma", "SKILL.md")); err != nil || string(existing) != "existing" {
		t.Fatalf("original target was modified: %q err=%v", string(existing), err)
	}
}

func TestPromoteOverwriteReplacesTarget(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "session", "skills")
	target := filepath.Join(tmp, "workspace", "skills")
	writeSkillSource(t, target, "delta", "old")
	// Add a stale auxiliary file that should disappear after overwrite.
	if err := os.WriteFile(filepath.Join(target, "delta", "stale.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write stale: %v", err)
	}
	srcDir := writeSkillSource(t, src, "delta", "new")

	res, err := skill.Promote(skill.PromoteRequest{
		SourceSkillDir:   srcDir,
		TargetSkillsRoot: target,
		Name:             "delta",
		Mode:             skill.PromoteModeCopy,
		OnConflict:       skill.PromoteOnConflictOverwrite,
	})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if res.Action != skill.PromoteActionOverwritten {
		t.Fatalf("expected overwritten, got %s", res.Action)
	}
	body, err := os.ReadFile(filepath.Join(target, "delta", "SKILL.md"))
	if err != nil || string(body) != "new" {
		t.Fatalf("target not overwritten: body=%q err=%v", string(body), err)
	}
	if _, err := os.Stat(filepath.Join(target, "delta", "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("stale file should have been removed, err=%v", err)
	}
}

func TestPromoteAbortOnConflict(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "session", "skills")
	target := filepath.Join(tmp, "workspace", "skills")
	writeSkillSource(t, target, "epsilon", "existing")
	srcDir := writeSkillSource(t, src, "epsilon", "fresh")

	_, err := skill.Promote(skill.PromoteRequest{
		SourceSkillDir:   srcDir,
		TargetSkillsRoot: target,
		Name:             "epsilon",
		Mode:             skill.PromoteModeCopy,
		OnConflict:       skill.PromoteOnConflictAbort,
	})
	if !errors.Is(err, skill.ErrPromoteConflict) {
		t.Fatalf("expected ErrPromoteConflict, got %v", err)
	}
	body, err := os.ReadFile(filepath.Join(target, "epsilon", "SKILL.md"))
	if err != nil || string(body) != "existing" {
		t.Fatalf("target should be untouched on abort: body=%q err=%v", string(body), err)
	}
}

func TestPromoteCopiesNestedFiles(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "session", "skills")
	target := filepath.Join(tmp, "workspace", "skills")
	srcDir := writeSkillSource(t, src, "zeta", "body")
	if err := os.MkdirAll(filepath.Join(srcDir, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "scripts", "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write nested: %v", err)
	}

	if _, err := skill.Promote(skill.PromoteRequest{
		SourceSkillDir:   srcDir,
		TargetSkillsRoot: target,
		Name:             "zeta",
		Mode:             skill.PromoteModeCopy,
		OnConflict:       skill.PromoteOnConflictRename,
	}); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "zeta", "scripts", "run.sh")); err != nil {
		t.Fatalf("nested file missing: %v", err)
	}
}

func TestPromoteRejectsInvalidName(t *testing.T) {
	tmp := t.TempDir()
	srcDir := writeSkillSource(t, filepath.Join(tmp, "src"), "valid", "x")
	cases := []string{"", "..", "../oops", "with/slash", "."}
	for _, name := range cases {
		_, err := skill.Promote(skill.PromoteRequest{
			SourceSkillDir:   srcDir,
			TargetSkillsRoot: filepath.Join(tmp, "target"),
			Name:             name,
		})
		if err == nil {
			t.Fatalf("expected error for invalid name %q", name)
		}
	}
}

func TestPromoteRejectsMissingSkillFile(t *testing.T) {
	tmp := t.TempDir()
	srcDir := filepath.Join(tmp, "src", "broken")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := skill.Promote(skill.PromoteRequest{
		SourceSkillDir:   srcDir,
		TargetSkillsRoot: filepath.Join(tmp, "target"),
		Name:             "broken",
	})
	if err == nil {
		t.Fatalf("expected error for missing SKILL.md")
	}
}

func TestPromoteRejectsEmptyFields(t *testing.T) {
	cases := []struct {
		name string
		req  skill.PromoteRequest
		want string
	}{
		{"empty source", skill.PromoteRequest{TargetSkillsRoot: "/tmp", Name: "x"}, "source skill"},
		{"empty target", skill.PromoteRequest{SourceSkillDir: "/tmp/src", Name: "x"}, "target skills"},
		{"empty name", skill.PromoteRequest{SourceSkillDir: "/tmp/src", TargetSkillsRoot: "/tmp"}, "name is required"},
	}
	for _, tc := range cases {
		_, err := skill.Promote(tc.req)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: expected %q error, got %v", tc.name, tc.want, err)
		}
	}
}

func TestPromoteRejectsInvalidMode(t *testing.T) {
	tmp := t.TempDir()
	srcDir := writeSkillSource(t, filepath.Join(tmp, "src"), "alpha", "x")
	_, err := skill.Promote(skill.PromoteRequest{
		SourceSkillDir:   srcDir,
		TargetSkillsRoot: filepath.Join(tmp, "target"),
		Name:             "alpha",
		Mode:             "invalid-mode",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid mode") {
		t.Fatalf("expected invalid mode error, got %v", err)
	}
}

func TestPromoteRejectsInvalidConflictPolicy(t *testing.T) {
	tmp := t.TempDir()
	srcDir := writeSkillSource(t, filepath.Join(tmp, "src"), "alpha", "x")
	_, err := skill.Promote(skill.PromoteRequest{
		SourceSkillDir:   srcDir,
		TargetSkillsRoot: filepath.Join(tmp, "target"),
		Name:             "alpha",
		OnConflict:       "panic",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid conflict policy") {
		t.Fatalf("expected invalid conflict policy error, got %v", err)
	}
}

func TestPromoteRejectsSourceFile(t *testing.T) {
	tmp := t.TempDir()
	srcFile := filepath.Join(tmp, "src", "file.md")
	if err := os.MkdirAll(filepath.Dir(srcFile), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(srcFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := skill.Promote(skill.PromoteRequest{
		SourceSkillDir:   srcFile,
		TargetSkillsRoot: filepath.Join(tmp, "target"),
		Name:             "x",
	})
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("expected not a directory error, got %v", err)
	}
}

func TestPromoteRejectsMissingSourceDir(t *testing.T) {
	tmp := t.TempDir()
	_, err := skill.Promote(skill.PromoteRequest{
		SourceSkillDir:   filepath.Join(tmp, "missing"),
		TargetSkillsRoot: filepath.Join(tmp, "target"),
		Name:             "x",
	})
	if err == nil || !strings.Contains(err.Error(), "read source") {
		t.Fatalf("expected read source error, got %v", err)
	}
}

func TestPromoteAppliesDefaults(t *testing.T) {
	tmp := t.TempDir()
	srcDir := writeSkillSource(t, filepath.Join(tmp, "src"), "alpha", "body")
	target := filepath.Join(tmp, "target")
	res, err := skill.Promote(skill.PromoteRequest{
		SourceSkillDir:   srcDir,
		TargetSkillsRoot: target,
		Name:             "alpha",
		// Mode and OnConflict empty — defaults to copy + rename.
	})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if res.Action != skill.PromoteActionCreated || res.SourceDeleted {
		t.Fatalf("expected default copy+created, got %+v", res)
	}
	if _, err := os.Stat(filepath.Join(srcDir, "SKILL.md")); err != nil {
		t.Fatalf("default mode should preserve source: %v", err)
	}
}

func TestPromoteRejectsSymlinkInSource(t *testing.T) {
	tmp := t.TempDir()
	srcDir := writeSkillSource(t, filepath.Join(tmp, "src"), "alpha", "body")
	linkTarget := filepath.Join(tmp, "elsewhere.txt")
	if err := os.WriteFile(linkTarget, []byte("x"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Symlink(linkTarget, filepath.Join(srcDir, "link.txt")); err != nil {
		t.Skipf("symlinks unsupported on this filesystem: %v", err)
	}
	_, err := skill.Promote(skill.PromoteRequest{
		SourceSkillDir:   srcDir,
		TargetSkillsRoot: filepath.Join(tmp, "target"),
		Name:             "alpha",
	})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink error, got %v", err)
	}
}

// keep errors import alive when future cases need errors.Is matchers.
var _ = errors.New
