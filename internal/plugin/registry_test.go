package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_PriorityWorkspaceOverUserAndBundled(t *testing.T) {
	root := t.TempDir()
	bundledDir := filepath.Join(root, "bundled")
	userDir := filepath.Join(root, "user")
	workspaceDir := filepath.Join(root, "workspace")

	writeManifest(t, filepath.Join(bundledDir, "same", "tars.plugin.json"), `{
  "id":"same",
  "name":"bundled",
  "skills":["skills"],
  "mcp_servers":[{"name":"bundled-fs","command":"b"}]
}`)
	writeManifest(t, filepath.Join(userDir, "same", "tars.plugin.json"), `{
  "id":"same",
  "name":"user",
  "skills":["skills"],
  "mcp_servers":[{"name":"user-fs","command":"u"}]
}`)
	writeManifest(t, filepath.Join(workspaceDir, "plugins", "same", "tars.plugin.json"), `{
  "id":"same",
  "name":"workspace",
  "skills":["skills"],
  "mcp_servers":[{"name":"workspace-fs","command":"w"}]
}`)

	snapshot, err := Load(LoadOptions{
		Sources: []SourceDir{
			{Source: SourceBundled, Dir: bundledDir},
			{Source: SourceUser, Dir: userDir},
			{Source: SourceWorkspace, Dir: filepath.Join(workspaceDir, "plugins")},
		},
	})
	if err != nil {
		t.Fatalf("load plugins: %v", err)
	}
	if len(snapshot.Plugins) != 1 {
		t.Fatalf("expected merged plugin count 1, got %d", len(snapshot.Plugins))
	}
	if snapshot.Plugins[0].Name != "workspace" {
		t.Fatalf("expected workspace plugin to win, got %q", snapshot.Plugins[0].Name)
	}
	if len(snapshot.MCPServers) != 1 || snapshot.MCPServers[0].Name != "workspace-fs" {
		t.Fatalf("expected workspace mcp server to win, got %+v", snapshot.MCPServers)
	}
}

func TestLoad_RejectsSkillPathTraversal(t *testing.T) {
	root := t.TempDir()
	pluginsDir := filepath.Join(root, "plugins")
	writeManifest(t, filepath.Join(pluginsDir, "danger", "tars.plugin.json"), `{
  "id":"danger",
  "skills":["../outside"]
}`)

	snapshot, err := Load(LoadOptions{
		Sources: []SourceDir{{Source: SourceWorkspace, Dir: pluginsDir}},
	})
	if err != nil {
		t.Fatalf("load plugins: %v", err)
	}
	if len(snapshot.SkillDirs) != 0 {
		t.Fatalf("expected no accepted skill dirs, got %+v", snapshot.SkillDirs)
	}
	if len(snapshot.Diagnostics) == 0 {
		t.Fatalf("expected diagnostics for path traversal")
	}
}

func TestLoad_PrefersPrimaryManifestFilenameOverLegacy(t *testing.T) {
	root := t.TempDir()
	pluginsDir := filepath.Join(root, "plugins")
	writeManifest(t, filepath.Join(pluginsDir, "ops", "tars.plugin.json"), `{
  "id":"ops",
  "name":"primary"
}`)
	writeManifest(t, filepath.Join(pluginsDir, "ops", "tarsncase.plugin.json"), `{
  "id":"ops",
  "name":"legacy"
}`)

	snapshot, err := Load(LoadOptions{
		Sources: []SourceDir{{Source: SourceWorkspace, Dir: pluginsDir}},
	})
	if err != nil {
		t.Fatalf("load plugins: %v", err)
	}
	if len(snapshot.Plugins) != 1 {
		t.Fatalf("expected one merged plugin, got %d", len(snapshot.Plugins))
	}
	if snapshot.Plugins[0].Name != "primary" {
		t.Fatalf("expected primary manifest to win, got %q", snapshot.Plugins[0].Name)
	}
}

func writeManifest(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
