package skillhub

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

	if _, err := inst.Install(context.Background(), "project-start"); err != nil {
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

	if _, err := inst.Install(context.Background(), "project-start"); err != nil {
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

func TestInstallRequiresPluginWarning(t *testing.T) {
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

	result, err := inst.Install(context.Background(), "project-start")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if result.RequiresPlugin != "project-swarm" {
		t.Fatalf("expected RequiresPlugin=project-swarm, got %q", result.RequiresPlugin)
	}
}

func TestInstallRejectsTamperedSkill(t *testing.T) {
	srv := newRegistryServer(t, testIntegrityIndex(), testHubFiles())
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

	_, err := inst.Install(context.Background(), "tampered-skill")
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "checksum") {
		t.Fatalf("expected checksum error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(tmpDir, "skills", "tampered-skill")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no installed tampered skill, got stat err %v", statErr)
	}
	skills, listErr := inst.List()
	if listErr != nil && !os.IsNotExist(listErr) {
		t.Fatalf("List: %v", listErr)
	}
	if len(skills) != 0 {
		t.Fatalf("expected no installed skills, got %v", skills)
	}
}

func TestInstallRejectsMissingSkillChecksum(t *testing.T) {
	srv := newRegistryServer(t, testIntegrityIndex(), testHubFiles())
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

	_, err := inst.Install(context.Background(), "missing-skill-checksum")
	if err == nil {
		t.Fatal("expected missing checksum error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "sha256") {
		t.Fatalf("expected sha256 error, got %v", err)
	}
}

func TestInstallRejectsSkillWhenSandboxSmokeFails(t *testing.T) {
	files := map[string][]byte{
		"/skills/broken-smoke/SKILL.md": []byte("---\nname: broken-smoke\nsmoke_tests:\n  - test -f SKILL.md\n  - exit 7\n---\n# Broken Smoke\n"),
	}
	index := RegistryIndex{
		Version: 4,
		Skills: []RegistryEntry{
			{
				Name:          "broken-smoke",
				Description:   "Smoke failure fixture",
				Version:       "0.1.0",
				Author:        "devlikebear",
				Path:          "skills/broken-smoke",
				UserInvocable: true,
				Files: RegistryFiles{
					{Path: "SKILL.md", SHA256: sha256Hex(files["/skills/broken-smoke/SKILL.md"])},
				},
			},
		},
	}
	srv := newRegistryServer(t, index, files)
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

	result, err := inst.Install(context.Background(), "broken-smoke")
	if err == nil {
		t.Fatal("expected sandbox smoke failure")
	}
	if result != nil {
		t.Fatalf("expected no install result on failure, got %+v", result)
	}
	var sandboxErr *SandboxError
	if !errors.As(err, &sandboxErr) {
		t.Fatalf("expected SandboxError, got %T %v", err, err)
	}
	if sandboxErr.Report.Passed {
		t.Fatalf("expected failed sandbox report: %+v", sandboxErr.Report)
	}
	if len(sandboxErr.Report.Checks) != 3 {
		t.Fatalf("expected three sandbox checks, got %+v", sandboxErr.Report.Checks)
	}
	if sandboxErr.Report.Checks[0].Status != SandboxCheckPassed || sandboxErr.Report.Checks[1].Status != SandboxCheckPassed || sandboxErr.Report.Checks[2].Status != SandboxCheckFailed {
		t.Fatalf("unexpected sandbox check statuses: %+v", sandboxErr.Report.Checks)
	}
	if _, statErr := os.Stat(filepath.Join(tmpDir, "skills", "broken-smoke")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no real skill install after sandbox failure, got stat err %v", statErr)
	}
	skills, listErr := inst.List()
	if listErr != nil && !os.IsNotExist(listErr) {
		t.Fatalf("List: %v", listErr)
	}
	if len(skills) != 0 {
		t.Fatalf("expected no installed skills after sandbox failure, got %+v", skills)
	}
}

func TestInstallReturnsSandboxReportWhenSmokePasses(t *testing.T) {
	files := map[string][]byte{
		"/skills/good-smoke/SKILL.md":       []byte("---\nname: good-smoke\nsmoke_tests:\n  - test -f SKILL.md\n---\n# Good Smoke\n"),
		"/skills/good-smoke/scripts/run.sh": []byte("#!/usr/bin/env bash\nprintf ok\n"),
	}
	index := RegistryIndex{
		Version: 4,
		Skills: []RegistryEntry{
			{
				Name:          "good-smoke",
				Description:   "Smoke pass fixture",
				Version:       "0.1.0",
				Author:        "devlikebear",
				Path:          "skills/good-smoke",
				UserInvocable: true,
				Files: RegistryFiles{
					{Path: "SKILL.md", SHA256: sha256Hex(files["/skills/good-smoke/SKILL.md"])},
					{Path: "scripts/run.sh", SHA256: sha256Hex(files["/skills/good-smoke/scripts/run.sh"])},
				},
			},
		},
	}
	srv := newRegistryServer(t, index, files)
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

	result, err := inst.Install(context.Background(), "good-smoke")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if result == nil || !result.Sandbox.Passed {
		t.Fatalf("expected passed sandbox report, got %+v", result)
	}
	if len(result.Sandbox.Checks) != 2 {
		t.Fatalf("expected default manifest check plus smoke command, got %+v", result.Sandbox.Checks)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "skills", "good-smoke", "SKILL.md")); err != nil {
		t.Fatalf("expected real skill install after sandbox pass: %v", err)
	}
}

func TestUpdateKeepsExistingSkillWhenSandboxSmokeFails(t *testing.T) {
	v1Files := map[string][]byte{
		"/skills/steady/SKILL.md": []byte("---\nname: steady\n---\n# Steady v1\n"),
	}
	v1Index := RegistryIndex{
		Version: 4,
		Skills: []RegistryEntry{
			{
				Name:          "steady",
				Description:   "Existing skill fixture",
				Version:       "0.1.0",
				Author:        "devlikebear",
				Path:          "skills/steady",
				UserInvocable: true,
				Files: RegistryFiles{
					{Path: "SKILL.md", SHA256: sha256Hex(v1Files["/skills/steady/SKILL.md"])},
				},
			},
		},
	}
	srv := newRegistryServer(t, v1Index, v1Files)
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
	if _, err := inst.Install(context.Background(), "steady"); err != nil {
		t.Fatalf("initial Install: %v", err)
	}
	skillFile := filepath.Join(tmpDir, "skills", "steady", "SKILL.md")
	before, err := os.ReadFile(skillFile)
	if err != nil {
		t.Fatalf("read installed skill: %v", err)
	}

	v2Files := map[string][]byte{
		"/skills/steady/SKILL.md": []byte("---\nname: steady\nsmoke_tests:\n  - exit 9\n---\n# Steady v2\n"),
	}
	v2Index := RegistryIndex{
		Version: 4,
		Skills: []RegistryEntry{
			{
				Name:          "steady",
				Description:   "Existing skill fixture",
				Version:       "0.2.0",
				Author:        "devlikebear",
				Path:          "skills/steady",
				UserInvocable: true,
				Files: RegistryFiles{
					{Path: "SKILL.md", SHA256: sha256Hex(v2Files["/skills/steady/SKILL.md"])},
				},
			},
		},
	}
	srv2 := newRegistryServer(t, v2Index, v2Files)
	defer srv2.Close()
	inst.Registry.RegistryURL = srv2.URL + "/registry.json"
	inst.Registry.SkillBaseURL = srv2.URL
	inst.Registry.HTTPClient = srv2.Client()

	result, err := inst.Update(context.Background())
	if err == nil {
		t.Fatal("expected update sandbox failure")
	}
	if len(result.Failed) != 1 || result.Failed[0].Name != "steady" {
		t.Fatalf("expected failed steady update, got %+v", result)
	}
	after, err := os.ReadFile(skillFile)
	if err != nil {
		t.Fatalf("read installed skill after failed update: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("expected existing skill to remain unchanged after sandbox failure\nbefore=%q\nafter=%q", before, after)
	}
	db, err := inst.loadDB()
	if err != nil {
		t.Fatalf("load DB: %v", err)
	}
	if len(db.Skills) != 1 || db.Skills[0].Version != "0.1.0" {
		t.Fatalf("expected DB to retain v0.1.0, got %+v", db.Skills)
	}
}

func TestInstallNoPluginWarningWhenInstalled(t *testing.T) {
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

	// Install the plugin first.
	if _, err := inst.InstallPlugin(context.Background(), "project-swarm"); err != nil {
		t.Fatalf("InstallPlugin: %v", err)
	}

	result, err := inst.Install(context.Background(), "project-start")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if result.RequiresPlugin != "" {
		t.Fatalf("expected no RequiresPlugin warning, got %q", result.RequiresPlugin)
	}
}

func TestInstallPluginAndList(t *testing.T) {
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

	if _, err := inst.InstallPlugin(context.Background(), "project-swarm"); err != nil {
		t.Fatalf("InstallPlugin: %v", err)
	}

	// Verify manifest exists.
	manifest := filepath.Join(tmpDir, "plugins", "project-swarm", "tars.plugin.json")
	if _, err := os.Stat(manifest); err != nil {
		t.Fatalf("plugin manifest not found: %v", err)
	}

	// List should show it.
	plugins, err := inst.ListPlugins()
	if err != nil {
		t.Fatalf("ListPlugins: %v", err)
	}
	if len(plugins) != 1 || plugins[0].Name != "project-swarm" {
		t.Fatalf("expected [project-swarm], got %v", plugins)
	}
}

func TestInstallPluginReturnsSandboxReport(t *testing.T) {
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

	result, err := inst.InstallPlugin(context.Background(), "project-swarm")
	if err != nil {
		t.Fatalf("InstallPlugin: %v", err)
	}
	if result == nil || !result.Sandbox.Passed || result.Sandbox.PackageType != "plugin" || result.Sandbox.PackageName != "project-swarm" {
		t.Fatalf("expected plugin sandbox report, got %+v", result)
	}
	if len(result.Sandbox.Checks) == 0 || result.Sandbox.Checks[0].Name != "plugin_manifest" {
		t.Fatalf("expected plugin manifest check, got %+v", result.Sandbox.Checks)
	}
}

func TestInstallPluginRejectsInvalidManifestInSandbox(t *testing.T) {
	files := map[string][]byte{
		"/plugins/broken-plugin/tars.plugin.json": []byte(`{"name":"Broken Plugin"}`),
	}
	index := RegistryIndex{
		Version: 4,
		Plugins: []PluginEntry{
			{
				Name:        "broken-plugin",
				Description: "Invalid plugin manifest fixture",
				Version:     "0.1.0",
				Author:      "devlikebear",
				Path:        "plugins/broken-plugin",
				Files: RegistryFiles{
					{Path: "tars.plugin.json", SHA256: sha256Hex(files["/plugins/broken-plugin/tars.plugin.json"])},
				},
			},
		},
	}
	srv := newRegistryServer(t, index, files)
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

	result, err := inst.InstallPlugin(context.Background(), "broken-plugin")
	if err == nil {
		t.Fatal("expected plugin sandbox failure")
	}
	if result != nil {
		t.Fatalf("expected no install result on failure, got %+v", result)
	}
	var sandboxErr *SandboxError
	if !errors.As(err, &sandboxErr) {
		t.Fatalf("expected SandboxError, got %T %v", err, err)
	}
	if sandboxErr.Report.Passed || sandboxErr.Report.PackageType != "plugin" {
		t.Fatalf("expected failed plugin sandbox report, got %+v", sandboxErr.Report)
	}
	if _, statErr := os.Stat(filepath.Join(tmpDir, "plugins", "broken-plugin")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no real plugin install after sandbox failure, got stat err %v", statErr)
	}
	plugins, listErr := inst.ListPlugins()
	if listErr != nil && !os.IsNotExist(listErr) {
		t.Fatalf("ListPlugins: %v", listErr)
	}
	if len(plugins) != 0 {
		t.Fatalf("expected no installed plugins after sandbox failure, got %+v", plugins)
	}
}

func TestInstallPluginRejectsTamperedPayload(t *testing.T) {
	srv := newRegistryServer(t, testIntegrityIndex(), testHubFiles())
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

	_, err := inst.InstallPlugin(context.Background(), "tampered-plugin")
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "checksum") {
		t.Fatalf("expected checksum error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(tmpDir, "plugins", "tampered-plugin")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no installed tampered plugin, got stat err %v", statErr)
	}
}

func TestUninstallPlugin(t *testing.T) {
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

	if _, err := inst.InstallPlugin(context.Background(), "project-swarm"); err != nil {
		t.Fatalf("InstallPlugin: %v", err)
	}
	if err := inst.UninstallPlugin("project-swarm"); err != nil {
		t.Fatalf("UninstallPlugin: %v", err)
	}

	plugins, err := inst.ListPlugins()
	if err != nil {
		t.Fatalf("ListPlugins: %v", err)
	}
	if len(plugins) != 0 {
		t.Fatalf("expected empty list, got %v", plugins)
	}

	pluginDir := filepath.Join(tmpDir, "plugins", "project-swarm")
	if _, err := os.Stat(pluginDir); !os.IsNotExist(err) {
		t.Fatalf("expected plugin dir to be removed")
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
	if _, err := inst.Install(context.Background(), "project-start"); err != nil {
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
	if len(updated.Updated) != 1 || updated.Updated[0] != "project-start" {
		t.Fatalf("expected [project-start] updated, got %v", updated.Updated)
	}

	// Verify version was updated.
	db, _ = inst.loadDB()
	if db.Skills[0].Version != "0.6.0" {
		t.Fatalf("expected version 0.6.0, got %s", db.Skills[0].Version)
	}
}

func TestUpdateRejectsTamperedSkillAndKeepsExistingInstall(t *testing.T) {
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

	if _, err := inst.Install(context.Background(), "project-start"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	skillPath := filepath.Join(tmpDir, "skills", "project-start", "SKILL.md")
	originalContent, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	db, _ := inst.loadDB()
	db.Skills[0].Version = "0.1.0"
	_ = inst.saveDB(db)

	tamperedIndex := testIndex()
	tamperedIndex.Skills[0].Version = "0.7.0"
	tamperedIndex.Skills[0].Files[0].SHA256 = "deadbeef"
	tamperedSrv := newRegistryServer(t, tamperedIndex, testHubFiles())
	defer tamperedSrv.Close()
	inst.Registry = &Registry{
		RegistryURL:  tamperedSrv.URL + "/registry.json",
		SkillBaseURL: tamperedSrv.URL,
		HTTPClient:   tamperedSrv.Client(),
	}

	updated, err := inst.Update(context.Background())
	if err == nil {
		t.Fatal("expected checksum mismatch update error")
	}
	if len(updated.Updated) != 0 {
		t.Fatalf("expected no updated skills, got %v", updated.Updated)
	}
	if len(updated.Failed) != 1 || updated.Failed[0].Name != "project-start" {
		t.Fatalf("expected project-start failure diagnostic, got %+v", updated.Failed)
	}

	currentContent, readErr := os.ReadFile(skillPath)
	if readErr != nil {
		t.Fatalf("ReadFile after update: %v", readErr)
	}
	if string(currentContent) != string(originalContent) {
		t.Fatalf("expected skill content to remain unchanged after failed update")
	}
	db, _ = inst.loadDB()
	if db.Skills[0].Version != "0.1.0" {
		t.Fatalf("expected version to remain 0.1.0, got %s", db.Skills[0].Version)
	}
}

func TestMaterializePackageFilesRollsBackOnActivationFailure(t *testing.T) {
	tmpDir := t.TempDir()
	dstDir := filepath.Join(tmpDir, "package")
	oldFile := filepath.Join(dstDir, "SKILL.md")
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(oldFile, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	origRename := renamePackagePath
	renamePackagePath = func(oldpath, newpath string) error {
		if strings.HasSuffix(oldpath, ".tmp") && newpath == dstDir {
			return errors.New("simulated activation failure")
		}
		return os.Rename(oldpath, newpath)
	}
	defer func() {
		renamePackagePath = origRename
	}()

	err := materializePackageFiles(dstDir, map[string][]byte{
		"SKILL.md": []byte("new"),
	})
	if err == nil {
		t.Fatal("expected activation failure")
	}

	content, readErr := os.ReadFile(oldFile)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	if string(content) != "old" {
		t.Fatalf("expected old content to remain, got %q", string(content))
	}
	if _, statErr := os.Stat(dstDir + ".tmp"); !os.IsNotExist(statErr) {
		t.Fatalf("expected temp dir cleanup, got %v", statErr)
	}
	if _, statErr := os.Stat(dstDir + ".bak"); !os.IsNotExist(statErr) {
		t.Fatalf("expected backup dir cleanup, got %v", statErr)
	}
}

func TestUpdatePlugins(t *testing.T) {
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

	if _, err := inst.InstallPlugin(context.Background(), "project-swarm"); err != nil {
		t.Fatalf("InstallPlugin: %v", err)
	}

	db, _ := inst.loadDB()
	db.Plugins[0].Version = "0.1.0"
	_ = inst.saveDB(db)

	updated, err := inst.UpdatePlugins(context.Background())
	if err != nil {
		t.Fatalf("UpdatePlugins: %v", err)
	}
	if len(updated.Updated) != 1 || updated.Updated[0] != "project-swarm" {
		t.Fatalf("expected [project-swarm] updated, got %v", updated.Updated)
	}

	db, _ = inst.loadDB()
	if db.Plugins[0].Version != "0.7.0" {
		t.Fatalf("expected version 0.7.0, got %s", db.Plugins[0].Version)
	}
}

func TestUpdatePluginsRollsBackOnActivationFailure(t *testing.T) {
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

	if _, err := inst.InstallPlugin(context.Background(), "project-swarm"); err != nil {
		t.Fatalf("InstallPlugin: %v", err)
	}
	pluginPath := filepath.Join(tmpDir, "plugins", "project-swarm", "tars.plugin.json")
	originalContent, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	db, _ := inst.loadDB()
	db.Plugins[0].Version = "0.1.0"
	_ = inst.saveDB(db)

	updatedIndex := testIndex()
	updatedIndex.Plugins[0].Version = "0.8.0"
	files := testHubFiles()
	files["/plugins/project-swarm/tars.plugin.json"] = []byte(`{"id":"project-swarm","name":"Project Swarm v2"}`)
	updatedIndex.Plugins[0].Files[0].SHA256 = sha256Hex(files["/plugins/project-swarm/tars.plugin.json"])
	updatedSrv := newRegistryServer(t, updatedIndex, files)
	defer updatedSrv.Close()
	inst.Registry = &Registry{
		RegistryURL:  updatedSrv.URL + "/registry.json",
		SkillBaseURL: updatedSrv.URL,
		HTTPClient:   updatedSrv.Client(),
	}

	pluginDir := filepath.Join(tmpDir, "plugins", "project-swarm")
	origRename := renamePackagePath
	renamePackagePath = func(oldpath, newpath string) error {
		if strings.HasSuffix(oldpath, ".tmp") && newpath == pluginDir {
			return errors.New("simulated activation failure")
		}
		return os.Rename(oldpath, newpath)
	}
	defer func() {
		renamePackagePath = origRename
	}()

	updated, err := inst.UpdatePlugins(context.Background())
	if err == nil {
		t.Fatal("expected activation failure")
	}
	if len(updated.Updated) != 0 {
		t.Fatalf("expected no updated plugins, got %v", updated.Updated)
	}
	if len(updated.Failed) != 1 || updated.Failed[0].Name != "project-swarm" {
		t.Fatalf("expected project-swarm failure diagnostic, got %+v", updated.Failed)
	}

	currentContent, readErr := os.ReadFile(pluginPath)
	if readErr != nil {
		t.Fatalf("ReadFile after update: %v", readErr)
	}
	if string(currentContent) != string(originalContent) {
		t.Fatalf("expected plugin content to remain unchanged after failed update")
	}
	db, _ = inst.loadDB()
	if db.Plugins[0].Version != "0.1.0" {
		t.Fatalf("expected version to remain 0.1.0, got %s", db.Plugins[0].Version)
	}
	if _, statErr := os.Stat(pluginDir + ".tmp"); !os.IsNotExist(statErr) {
		t.Fatalf("expected temp dir cleanup, got %v", statErr)
	}
	if _, statErr := os.Stat(pluginDir + ".bak"); !os.IsNotExist(statErr) {
		t.Fatalf("expected backup dir cleanup, got %v", statErr)
	}
}

func TestUpdateReturnsFinalSaveDBError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission-based save failure is not reliable as root")
	}
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
	if _, err := inst.Install(context.Background(), "project-start"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	db, _ := inst.loadDB()
	db.Skills[0].Version = "0.1.0"
	if err := inst.saveDB(db); err != nil {
		t.Fatalf("save downgraded db: %v", err)
	}
	dbPath := inst.dbPath()
	if err := os.Chmod(dbPath, 0o400); err != nil {
		t.Fatalf("chmod db: %v", err)
	}
	defer func() { _ = os.Chmod(dbPath, 0o600) }()

	updated, err := inst.Update(context.Background())
	if err == nil {
		t.Fatal("expected final saveDB error")
	}
	if len(updated.Updated) != 1 || updated.Updated[0] != "project-start" {
		t.Fatalf("expected project-start to be reported updated before save failure, got %+v", updated)
	}
}

func TestUpdatePluginsReturnsFinalSaveDBError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission-based save failure is not reliable as root")
	}
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
	if _, err := inst.InstallPlugin(context.Background(), "project-swarm"); err != nil {
		t.Fatalf("InstallPlugin: %v", err)
	}
	db, _ := inst.loadDB()
	db.Plugins[0].Version = "0.1.0"
	if err := inst.saveDB(db); err != nil {
		t.Fatalf("save downgraded db: %v", err)
	}
	dbPath := inst.dbPath()
	if err := os.Chmod(dbPath, 0o400); err != nil {
		t.Fatalf("chmod db: %v", err)
	}
	defer func() { _ = os.Chmod(dbPath, 0o600) }()

	updated, err := inst.UpdatePlugins(context.Background())
	if err == nil {
		t.Fatal("expected final saveDB error")
	}
	if len(updated.Updated) != 1 || updated.Updated[0] != "project-swarm" {
		t.Fatalf("expected project-swarm to be reported updated before save failure, got %+v", updated)
	}
}

func TestInstallMCPAndList(t *testing.T) {
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

	if _, err := inst.InstallMCP(context.Background(), "filesystem"); err != nil {
		t.Fatalf("InstallMCP: %v", err)
	}

	manifest := filepath.Join(tmpDir, "mcp-servers", "filesystem", "tars.mcp.json")
	if _, err := os.Stat(manifest); err != nil {
		t.Fatalf("manifest not found: %v", err)
	}

	mcps, err := inst.ListMCPs()
	if err != nil {
		t.Fatalf("ListMCPs: %v", err)
	}
	if len(mcps) != 1 || mcps[0].Name != "filesystem" {
		t.Fatalf("expected [filesystem], got %v", mcps)
	}

	servers, diagnostics := LoadInstalledMCPServers(tmpDir)
	if len(diagnostics) != 0 {
		t.Fatalf("expected no diagnostics, got %v", diagnostics)
	}
	if len(servers) != 1 || servers[0].Name != "filesystem" {
		t.Fatalf("expected one filesystem server, got %+v", servers)
	}
	if len(servers[0].Args) < 3 || !strings.Contains(servers[0].Args[2], filepath.Join(tmpDir, "mcp-servers", "filesystem")) {
		t.Fatalf("expected MCP_DIR placeholder expansion, got %+v", servers[0].Args)
	}
}

func TestInstallMCPReturnsSandboxReport(t *testing.T) {
	files := map[string][]byte{
		"/mcp-servers/local-echo/tars.mcp.json": []byte(`{"schema_version":1,"server":{"name":"local-echo","command":"sh","args":["-c","cat"]}}`),
	}
	index := RegistryIndex{
		Version: 4,
		MCPServers: []MCPEntry{
			{
				Name:        "local-echo",
				Description: "Local MCP fixture",
				Version:     "0.1.0",
				Author:      "devlikebear",
				Path:        "mcp-servers/local-echo",
				Manifest:    "tars.mcp.json",
				Files: RegistryFiles{
					{Path: "tars.mcp.json", SHA256: sha256Hex(files["/mcp-servers/local-echo/tars.mcp.json"])},
				},
			},
		},
	}
	srv := newRegistryServer(t, index, files)
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

	result, err := inst.InstallMCP(context.Background(), "local-echo")
	if err != nil {
		t.Fatalf("InstallMCP: %v", err)
	}
	if result == nil || !result.Sandbox.Passed || result.Sandbox.PackageType != "mcp" || result.Sandbox.PackageName != "local-echo" {
		t.Fatalf("expected mcp sandbox report, got %+v", result)
	}
	if len(result.Sandbox.Checks) < 2 || result.Sandbox.Checks[0].Name != "mcp_manifest" {
		t.Fatalf("expected mcp manifest checks, got %+v", result.Sandbox.Checks)
	}
}

func TestInstallRemoteMCPReturnsRemoteSandboxReport(t *testing.T) {
	files := map[string][]byte{
		"/mcp-servers/remote-http/tars.mcp.json": []byte(`{"schema_version":1,"server":{"name":"remote-http","transport":"streamable_http","url":"https://mcp.example.com"}}`),
	}
	index := RegistryIndex{
		Version: 4,
		MCPServers: []MCPEntry{
			{
				Name:        "remote-http",
				Description: "Remote MCP fixture",
				Version:     "0.1.0",
				Author:      "devlikebear",
				Path:        "mcp-servers/remote-http",
				Manifest:    "tars.mcp.json",
				Files: RegistryFiles{
					{Path: "tars.mcp.json", SHA256: sha256Hex(files["/mcp-servers/remote-http/tars.mcp.json"])},
				},
			},
		},
	}
	srv := newRegistryServer(t, index, files)
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

	result, err := inst.InstallMCP(context.Background(), "remote-http")
	if err != nil {
		t.Fatalf("InstallMCP: %v", err)
	}
	if result == nil || !result.Sandbox.Passed {
		t.Fatalf("expected passed remote MCP sandbox report, got %+v", result)
	}
	if len(result.Sandbox.Checks) < 2 || result.Sandbox.Checks[1].Name != "mcp_remote_smoke" {
		t.Fatalf("expected remote MCP smoke check, got %+v", result.Sandbox.Checks)
	}
}

func TestInstallMCPRejectsInvalidManifestInSandbox(t *testing.T) {
	files := map[string][]byte{
		"/mcp-servers/broken-mcp/tars.mcp.json": []byte(`{"schema_version":1,"server":{"name":"broken-mcp"}}`),
	}
	index := RegistryIndex{
		Version: 4,
		MCPServers: []MCPEntry{
			{
				Name:        "broken-mcp",
				Description: "Invalid MCP manifest fixture",
				Version:     "0.1.0",
				Author:      "devlikebear",
				Path:        "mcp-servers/broken-mcp",
				Manifest:    "tars.mcp.json",
				Files: RegistryFiles{
					{Path: "tars.mcp.json", SHA256: sha256Hex(files["/mcp-servers/broken-mcp/tars.mcp.json"])},
				},
			},
		},
	}
	srv := newRegistryServer(t, index, files)
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

	result, err := inst.InstallMCP(context.Background(), "broken-mcp")
	if err == nil {
		t.Fatal("expected mcp sandbox failure")
	}
	if result != nil {
		t.Fatalf("expected no install result on failure, got %+v", result)
	}
	var sandboxErr *SandboxError
	if !errors.As(err, &sandboxErr) {
		t.Fatalf("expected SandboxError, got %T %v", err, err)
	}
	if sandboxErr.Report.Passed || sandboxErr.Report.PackageType != "mcp" {
		t.Fatalf("expected failed mcp sandbox report, got %+v", sandboxErr.Report)
	}
	if _, statErr := os.Stat(filepath.Join(tmpDir, "mcp-servers", "broken-mcp")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no real mcp install after sandbox failure, got stat err %v", statErr)
	}
	mcps, listErr := inst.ListMCPs()
	if listErr != nil && !os.IsNotExist(listErr) {
		t.Fatalf("ListMCPs: %v", listErr)
	}
	if len(mcps) != 0 {
		t.Fatalf("expected no installed MCP servers after sandbox failure, got %+v", mcps)
	}
}

func TestInstallMCPChecksumMismatch(t *testing.T) {
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

	_, err := inst.InstallMCP(context.Background(), "broken-checksum")
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "checksum") {
		t.Fatalf("expected checksum error, got %v", err)
	}
}

func TestUninstallMCP(t *testing.T) {
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

	if _, err := inst.InstallMCP(context.Background(), "filesystem"); err != nil {
		t.Fatalf("InstallMCP: %v", err)
	}
	if err := inst.UninstallMCP("filesystem"); err != nil {
		t.Fatalf("UninstallMCP: %v", err)
	}

	mcps, err := inst.ListMCPs()
	if err != nil {
		t.Fatalf("ListMCPs: %v", err)
	}
	if len(mcps) != 0 {
		t.Fatalf("expected empty list, got %v", mcps)
	}
}

func TestUpdateMCPsReportsPerEntryFailures(t *testing.T) {
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
	if err := inst.saveDB(&InstalledDB{MCPs: []InstalledMCP{
		{Name: "broken-checksum", Version: "0.0.1", Source: "tars-hub", Dir: filepath.Join(tmpDir, "mcp-servers", "broken-checksum"), Manifest: "tars.mcp.json"},
	}}); err != nil {
		t.Fatalf("save db: %v", err)
	}

	updated, err := inst.UpdateMCPs(context.Background())
	if err == nil {
		t.Fatal("expected MCP update failure")
	}
	if len(updated.Updated) != 0 {
		t.Fatalf("expected no updated MCP servers, got %+v", updated.Updated)
	}
	if len(updated.Failed) != 1 || updated.Failed[0].Name != "broken-checksum" {
		t.Fatalf("expected broken-checksum failure diagnostic, got %+v", updated.Failed)
	}
}

func TestUpdateMCPsReturnsFinalSaveDBError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission-based save failure is not reliable as root")
	}
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
	if _, err := inst.InstallMCP(context.Background(), "filesystem"); err != nil {
		t.Fatalf("InstallMCP: %v", err)
	}
	db, _ := inst.loadDB()
	db.MCPs[0].Version = "0.0.1"
	if err := inst.saveDB(db); err != nil {
		t.Fatalf("save downgraded db: %v", err)
	}
	dbPath := inst.dbPath()
	if err := os.Chmod(dbPath, 0o400); err != nil {
		t.Fatalf("chmod db: %v", err)
	}
	defer func() { _ = os.Chmod(dbPath, 0o600) }()

	updated, err := inst.UpdateMCPs(context.Background())
	if err == nil {
		t.Fatal("expected final saveDB error")
	}
	if len(updated.Updated) != 1 || updated.Updated[0] != "filesystem" {
		t.Fatalf("expected filesystem to be reported updated before save failure, got %+v", updated)
	}
}
