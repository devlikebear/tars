package skill

import (
	"os"
	"path/filepath"
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
