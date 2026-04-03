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
	if err := inst.InstallPlugin(context.Background(), "project-swarm"); err != nil {
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

	if err := inst.InstallPlugin(context.Background(), "project-swarm"); err != nil {
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

	err := inst.InstallPlugin(context.Background(), "tampered-plugin")
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

	if err := inst.InstallPlugin(context.Background(), "project-swarm"); err != nil {
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
	if len(updated) != 1 || updated[0] != "project-start" {
		t.Fatalf("expected [project-start] updated, got %v", updated)
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
	if len(updated) != 0 {
		t.Fatalf("expected no updated skills, got %v", updated)
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

	if err := inst.InstallPlugin(context.Background(), "project-swarm"); err != nil {
		t.Fatalf("InstallPlugin: %v", err)
	}

	db, _ := inst.loadDB()
	db.Plugins[0].Version = "0.1.0"
	_ = inst.saveDB(db)

	updated, err := inst.UpdatePlugins(context.Background())
	if err != nil {
		t.Fatalf("UpdatePlugins: %v", err)
	}
	if len(updated) != 1 || updated[0] != "project-swarm" {
		t.Fatalf("expected [project-swarm] updated, got %v", updated)
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

	if err := inst.InstallPlugin(context.Background(), "project-swarm"); err != nil {
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
	if len(updated) != 0 {
		t.Fatalf("expected no updated plugins, got %v", updated)
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

	if err := inst.InstallMCP(context.Background(), "filesystem"); err != nil {
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

	err := inst.InstallMCP(context.Background(), "broken-checksum")
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

	if err := inst.InstallMCP(context.Background(), "filesystem"); err != nil {
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
