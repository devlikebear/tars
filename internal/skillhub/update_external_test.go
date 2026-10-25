package skillhub

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUpdate_ExternalSkillComparesUpstreamFiles covers how Update treats an
// external skill: unchanged upstream files are up to date, and changed
// ones are reported for review instead of being written.
func TestUpdate_ExternalSkillComparesUpstreamFiles(t *testing.T) {
	src := newExternalStub()
	inst := newInstallerWithSource(t, src)
	if _, err := inst.InstallWithOptions(context.Background(), "demo:foo", InstallOptions{Yes: true}); err != nil {
		t.Fatalf("install: %v", err)
	}
	skills, _ := inst.List()
	if len(skills) != 1 || skills[0].Upstream == "" {
		t.Fatalf("installed row = %+v, want an upstream checksum", skills)
	}
	skillDir := filepath.Join(inst.WorkspaceDir, "skills", "foo")
	installedManifest, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read installed SKILL.md: %v", err)
	}

	result, err := inst.Update(context.Background())
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(result.Updated) != 0 || len(result.Skipped) != 1 || result.Skipped[0].Reason != reasonUpToDate {
		t.Fatalf("unchanged upstream: %+v, want foo skipped as up to date", result)
	}

	// Upstream moves on, in SKILL.md or in a companion file.
	for name, change := range map[string]func(){
		"SKILL.md": func() {
			src.manifest = []byte("---\nname: foo\ndescription: hi\n---\nnew raw body")
			src.converted = []byte("---\nname: foo\ndescription: hi\nuser_invocable: true\n---\nnew body")
		},
		"companion": func() { src.companion = map[string][]byte{"scripts/run.sh": []byte("#!/bin/sh\necho new\n")} },
	} {
		change()
		result, err = inst.Update(context.Background())
		if err != nil {
			t.Fatalf("%s: Update: %v", name, err)
		}
		if len(result.Updated) != 0 || len(result.Skipped) != 1 || !strings.Contains(result.Skipped[0].Reason, "upstream changed") ||
			!strings.Contains(result.Skipped[0].Reason, "tars skill install demo:foo") {
			t.Fatalf("%s changed upstream: %+v, want foo reported for review", name, result)
		}
		manifest, _ := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
		script, _ := os.ReadFile(filepath.Join(skillDir, "scripts", "run.sh"))
		if string(manifest) != string(installedManifest) || string(script) != "#!/bin/sh\n" {
			t.Fatalf("%s: an update wrote new upstream content without review", name)
		}
	}
}

func TestUpdate_ExternalSkillWithoutRecordedUpstream(t *testing.T) {
	src := newExternalStub()
	inst := newInstallerWithSource(t, src)
	if _, err := inst.InstallWithOptions(context.Background(), "demo:foo", InstallOptions{Yes: true}); err != nil {
		t.Fatalf("install: %v", err)
	}
	// A row written before the upstream checksum was recorded.
	db, _ := inst.loadDB()
	db.Skills[0].Upstream = ""
	if err := inst.saveDB(db); err != nil {
		t.Fatalf("save db: %v", err)
	}

	result, err := inst.Update(context.Background())
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(result.Updated) != 0 || len(result.Skipped) != 1 || !strings.Contains(result.Skipped[0].Reason, "track updates") {
		t.Fatalf("legacy row: %+v, want a review hint", result)
	}
}
