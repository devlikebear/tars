package skillhub

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestUpdate_SkipsUnknownSource exercises the "source no longer
// registered" branch of Installer.Update.
func TestUpdate_SkipsUnknownSource(t *testing.T) {
	workspace := t.TempDir()
	// Pre-populate skillhub.json with a row pointing at a hub that we
	// won't register.
	db := InstalledDB{
		Skills: []InstalledSkill{
			{Name: "foo", Version: "1.0.0", Source: "forgotten-hub", Dir: filepath.Join(workspace, "skills", "foo")},
		},
	}
	body, _ := json.MarshalIndent(&db, "", "  ")
	if err := os.WriteFile(filepath.Join(workspace, "skillhub.json"), body, 0o644); err != nil {
		t.Fatalf("write db: %v", err)
	}

	inst := &Installer{
		WorkspaceDir: workspace,
		Sources:      NewSourceRegistry(),
	}
	_ = inst.Sources.Register(&TarsHubSource{Registry: NewRegistry()})

	result, err := inst.Update(context.Background())
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("expected 1 skipped row, got %+v", result.Skipped)
	}
	if !contains(result.Skipped[0].Reason, "forgotten-hub") {
		t.Errorf("Skipped reason %q should mention forgotten-hub", result.Skipped[0].Reason)
	}
}

// TestSyncDefaultSource_RefreshesRegistryPointer locks in the
// behaviour the syncDefaultSource helper restores after legacy tests
// swap inst.Registry mid-test.
func TestSyncDefaultSource_RefreshesRegistryPointer(t *testing.T) {
	oldReg := NewRegistry()
	inst := &Installer{
		WorkspaceDir: t.TempDir(),
		Registry:     oldReg,
		Sources:      NewSourceRegistry(),
	}
	tarsHub := &TarsHubSource{Registry: oldReg}
	_ = inst.Sources.Register(tarsHub)

	// Caller swaps the registry pointer without rebuilding Sources.
	newReg := NewRegistry()
	inst.Registry = newReg

	inst.ensureSources() // triggers syncDefaultSource

	if tarsHub.Registry != newReg {
		t.Errorf("syncDefaultSource did not refresh tars-hub Registry pointer")
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
