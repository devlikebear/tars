package skillhub

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installOutdatedProjectStart installs project-start from the test hub and
// marks it one version behind, so an update would replace it.
func installOutdatedProjectStart(t *testing.T) *Installer {
	t.Helper()
	srv := newTestServer(t)
	t.Cleanup(srv.Close)
	inst := &Installer{
		WorkspaceDir: t.TempDir(),
		Registry: &Registry{
			RegistryURL:  srv.URL + "/registry.json",
			SkillBaseURL: srv.URL,
			HTTPClient:   srv.Client(),
		},
	}
	if _, err := inst.Install(context.Background(), "project-start"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	db, _ := inst.loadDB()
	if db.Skills[0].SHA256 == "" {
		t.Fatalf("installed row has no SKILL.md checksum: %+v", db.Skills[0])
	}
	db.Skills[0].Version = "0.1.0"
	if err := inst.saveDB(db); err != nil {
		t.Fatalf("save db: %v", err)
	}
	return inst
}

func TestUpdate_LeavesLocallyEditedSkillAlone(t *testing.T) {
	inst := installOutdatedProjectStart(t)
	manifest := filepath.Join(inst.WorkspaceDir, "skills", "project-start", "SKILL.md")
	edited := []byte("---\nname: project-start\ndescription: mine now\n---\nmy edits\n")
	if err := os.WriteFile(manifest, edited, 0o644); err != nil {
		t.Fatalf("edit: %v", err)
	}

	result, err := inst.Update(context.Background())
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(result.Updated) != 0 || len(result.Skipped) != 1 || !strings.Contains(result.Skipped[0].Reason, "edited locally") {
		t.Fatalf("result = %+v, want project-start skipped as edited", result)
	}
	if got, _ := os.ReadFile(manifest); string(got) != string(edited) {
		t.Fatalf("update overwrote the edited SKILL.md: %q", got)
	}
}

func TestUpdate_ReplacesUneditedSkillAndRecordsNewChecksum(t *testing.T) {
	inst := installOutdatedProjectStart(t)
	before, _ := inst.loadDB()

	result, err := inst.Update(context.Background())
	if err != nil || len(result.Updated) != 1 {
		t.Fatalf("Update = %+v, %v; want project-start updated", result, err)
	}
	after, _ := inst.loadDB()
	if after.Skills[0].Version != "0.6.0" || after.Skills[0].SHA256 != before.Skills[0].SHA256 {
		t.Fatalf("row after update = %+v", after.Skills[0])
	}
}

// A hub skill deleted outside the hub (the console's workspace Delete,
// rm -rf) must not stay "installed" in the Hub tab with no way back.
func TestDeletedSkillDropsOutOfListAndCanBeReinstalled(t *testing.T) {
	inst := installOutdatedProjectStart(t)
	if err := os.RemoveAll(filepath.Join(inst.WorkspaceDir, "skills", "project-start")); err != nil {
		t.Fatalf("remove: %v", err)
	}

	listed, err := inst.List()
	if err != nil || len(listed) != 0 {
		t.Fatalf("List = %+v, %v; want the deleted skill left out", listed, err)
	}
	db, _ := inst.loadDB()
	if _, ok := installedSkillVersion(db, "project-start"); ok {
		t.Fatal("a pack would treat the deleted skill as already installed")
	}
	result, err := inst.Update(context.Background())
	if err != nil || len(result.Updated) != 0 || len(result.Skipped) != 1 || !strings.Contains(result.Skipped[0].Reason, "removed") {
		t.Fatalf("Update = %+v, %v; want the deleted skill skipped, not reinstalled", result, err)
	}

	if _, err := inst.Install(context.Background(), "project-start"); err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	if listed, _ := inst.List(); len(listed) != 1 {
		t.Fatalf("List after reinstall = %+v", listed)
	}
}
