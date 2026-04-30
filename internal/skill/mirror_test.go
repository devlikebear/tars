package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMirrorToWorkspace(t *testing.T) {
	workspaceDir := t.TempDir()
	externalSkillFile := filepath.Join(t.TempDir(), "external", "my-skill", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(externalSkillFile), 0o755); err != nil {
		t.Fatalf("mkdir external skill: %v", err)
	}
	if err := os.WriteFile(externalSkillFile, []byte("# External Skill"), 0o644); err != nil {
		t.Fatalf("write external skill: %v", err)
	}

	snapshot := Snapshot{
		Skills: []Definition{
			{
				Name:     "external_skill",
				Source:   SourceUser,
				FilePath: externalSkillFile,
				Content:  "# External Skill",
			},
		},
	}
	next, err := MirrorToWorkspace(workspaceDir, snapshot)
	if err != nil {
		t.Fatalf("mirror to workspace: %v", err)
	}
	if len(next.Skills) != 1 {
		t.Fatalf("expected 1 mirrored skill, got %d", len(next.Skills))
	}
	if next.Skills[0].RuntimePath == "" {
		t.Fatalf("expected runtime path after mirroring")
	}

	mirroredPath := filepath.Join(workspaceDir, filepath.FromSlash(next.Skills[0].RuntimePath))
	data, err := os.ReadFile(mirroredPath)
	if err != nil {
		t.Fatalf("read mirrored file: %v", err)
	}
	if string(data) != "# External Skill" {
		t.Fatalf("unexpected mirrored content: %q", string(data))
	}
}

func TestMirrorToWorkspace_CompanionFiles(t *testing.T) {
	workspaceDir := t.TempDir()

	// Create a source skill directory with SKILL.md + companion files.
	srcDir := filepath.Join(t.TempDir(), "skills", "my-skill")
	if err := os.MkdirAll(filepath.Join(srcDir, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "SKILL.md"), []byte("# My Skill"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "helper.sh"), []byte("#!/bin/bash\necho hello"), 0o755); err != nil {
		t.Fatalf("write helper.sh: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "scripts", "run.py"), []byte("print('hello')"), 0o644); err != nil {
		t.Fatalf("write run.py: %v", err)
	}

	snapshot := Snapshot{
		Skills: []Definition{
			{
				Name:     "my-skill",
				Source:   SourceUser,
				FilePath: filepath.Join(srcDir, "SKILL.md"),
				Content:  "# My Skill",
			},
		},
	}

	next, err := MirrorToWorkspace(workspaceDir, snapshot)
	if err != nil {
		t.Fatalf("mirror: %v", err)
	}

	runtimeDir := filepath.Join(workspaceDir, "_shared", "skills_runtime", "my_skill")

	// SKILL.md should exist.
	if _, err := os.Stat(filepath.Join(runtimeDir, "SKILL.md")); err != nil {
		t.Fatalf("SKILL.md not mirrored: %v", err)
	}

	// helper.sh should be copied.
	helperData, err := os.ReadFile(filepath.Join(runtimeDir, "helper.sh"))
	if err != nil {
		t.Fatalf("helper.sh not mirrored: %v", err)
	}
	if string(helperData) != "#!/bin/bash\necho hello" {
		t.Fatalf("unexpected helper.sh content: %q", string(helperData))
	}

	// Check executable bit preserved.
	info, err := os.Stat(filepath.Join(runtimeDir, "helper.sh"))
	if err != nil {
		t.Fatalf("stat helper.sh: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("helper.sh should be executable, got %v", info.Mode())
	}

	// Subdirectory file should be copied.
	pyData, err := os.ReadFile(filepath.Join(runtimeDir, "scripts", "run.py"))
	if err != nil {
		t.Fatalf("scripts/run.py not mirrored: %v", err)
	}
	if string(pyData) != "print('hello')" {
		t.Fatalf("unexpected run.py content: %q", string(pyData))
	}

	_ = next
}

func TestMirrorToWorkspace_CompanionCopyFailureSkipsSkillWithDiagnostic(t *testing.T) {
	workspaceDir := t.TempDir()
	srcDir := filepath.Join(t.TempDir(), "skills", "my-skill")
	if err := os.MkdirAll(filepath.Join(srcDir, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir source scripts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "SKILL.md"), []byte("# My Skill"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "scripts", "run.py"), []byte("print('hello')"), 0o644); err != nil {
		t.Fatalf("write run.py: %v", err)
	}

	runtimeDir := filepath.Join(workspaceDir, "_shared", "skills_runtime", "my_skill")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "scripts"), []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write conflicting runtime file: %v", err)
	}

	snapshot := Snapshot{
		Skills: []Definition{
			{
				Name:     "my-skill",
				Source:   SourceUser,
				FilePath: filepath.Join(srcDir, "SKILL.md"),
				Content:  "# My Skill",
			},
		},
	}

	next, err := MirrorToWorkspace(workspaceDir, snapshot)
	if err != nil {
		t.Fatalf("mirror: %v", err)
	}
	if len(next.Skills) != 0 {
		t.Fatalf("expected skill with failed companion copy to be skipped, got %+v", next.Skills)
	}
	if len(next.Diagnostics) != 1 {
		t.Fatalf("expected companion copy diagnostic, got %+v", next.Diagnostics)
	}
	diag := next.Diagnostics[0]
	if diag.Path != filepath.Join(srcDir, "SKILL.md") {
		t.Fatalf("expected diagnostic path to reference source skill, got %q", diag.Path)
	}
	if !strings.Contains(diag.Message, "mirror companion files") || !strings.Contains(diag.Message, "scripts") {
		t.Fatalf("expected failed companion path in diagnostic, got %q", diag.Message)
	}
	if _, err := os.Stat(runtimeDir); !os.IsNotExist(err) {
		t.Fatalf("expected failed mirrored dir to be removed, stat err=%v", err)
	}
}

func TestMirrorToWorkspace_CompanionReadFailureSkipsSkillWithDiagnostic(t *testing.T) {
	workspaceDir := t.TempDir()
	srcDir := filepath.Join(t.TempDir(), "skills", "my-skill")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "SKILL.md"), []byte("# My Skill"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	missingTarget := filepath.Join(srcDir, "missing-helper.sh")
	brokenHelper := filepath.Join(srcDir, "helper.sh")
	if err := os.Symlink(missingTarget, brokenHelper); err != nil {
		t.Skipf("symlinks are not supported here: %v", err)
	}

	snapshot := Snapshot{
		Skills: []Definition{
			{
				Name:     "my-skill",
				Source:   SourceUser,
				FilePath: filepath.Join(srcDir, "SKILL.md"),
				Content:  "# My Skill",
			},
		},
	}

	next, err := MirrorToWorkspace(workspaceDir, snapshot)
	if err != nil {
		t.Fatalf("mirror: %v", err)
	}
	if len(next.Skills) != 0 {
		t.Fatalf("expected skill with unreadable companion to be skipped, got %+v", next.Skills)
	}
	if len(next.Diagnostics) != 1 {
		t.Fatalf("expected companion read diagnostic, got %+v", next.Diagnostics)
	}
	if !strings.Contains(next.Diagnostics[0].Message, "helper.sh") {
		t.Fatalf("expected failed companion path in diagnostic, got %q", next.Diagnostics[0].Message)
	}
}
