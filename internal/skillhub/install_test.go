package skillhub

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallAndList(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	tmpDir := t.TempDir()
	inst := &Installer{
		WorkspaceDir: tmpDir,
		Registry: &Registry{
			RegistryURL:  srv.URL + "/registry.json",
			SkillBaseURL: srv.URL,
			HTTPClient:   srv.Client(),
		},
	}

	if err := inst.Install(context.Background(), "project-start"); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Verify file exists.
	skillFile := filepath.Join(tmpDir, "skills", "project-start", "SKILL.md")
	if _, err := os.Stat(skillFile); err != nil {
		t.Fatalf("skill file not found: %v", err)
	}

	// List should show it.
	skills, err := inst.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(skills) != 1 || skills[0].Name != "project-start" {
		t.Fatalf("expected [project-start], got %v", skills)
	}
}

func TestUninstall(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	tmpDir := t.TempDir()
	inst := &Installer{
		WorkspaceDir: tmpDir,
		Registry: &Registry{
			RegistryURL:  srv.URL + "/registry.json",
			SkillBaseURL: srv.URL,
			HTTPClient:   srv.Client(),
		},
	}

	if err := inst.Install(context.Background(), "project-start"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if err := inst.Uninstall("project-start"); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	skills, err := inst.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("expected empty list, got %v", skills)
	}

	// Directory should be removed.
	skillDir := filepath.Join(tmpDir, "skills", "project-start")
	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Fatalf("expected skill dir to be removed")
	}
}

func TestUninstallNotInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	inst := NewInstaller(tmpDir)
	err := inst.Uninstall("nonexistent")
	if err == nil {
		t.Fatal("expected error for uninstalling non-installed skill")
	}
}

func TestListEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	inst := NewInstaller(tmpDir)
	skills, err := inst.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("expected empty list, got %v", skills)
	}
}

func TestUpdate(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	tmpDir := t.TempDir()
	inst := &Installer{
		WorkspaceDir: tmpDir,
		Registry: &Registry{
			RegistryURL:  srv.URL + "/registry.json",
			SkillBaseURL: srv.URL,
			HTTPClient:   srv.Client(),
		},
	}

	// Install first.
	if err := inst.Install(context.Background(), "project-start"); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Manually downgrade version in DB to simulate outdated.
	db, _ := inst.loadDB()
	db.Skills[0].Version = "0.1.0"
	_ = inst.saveDB(db)

	updated, err := inst.Update(context.Background())
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(updated) != 1 || updated[0] != "project-start" {
		t.Fatalf("expected [project-start] updated, got %v", updated)
	}

	// Verify version was updated.
	db, _ = inst.loadDB()
	if db.Skills[0].Version != "0.6.0" {
		t.Fatalf("expected version 0.6.0, got %s", db.Skills[0].Version)
	}
}
