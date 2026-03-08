package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseManifest(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "sample", "tars.plugin.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := `{
  "id": "ops-tools",
  "name": "Ops Tools",
  "version": "1.0.0",
  "skills": ["skills"],
  "mcp_servers": [
    {
      "name": "filesystem",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "."]
    }
  ]
}`
	if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	manifest, err := parseManifestFile(manifestPath)
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if manifest.ID != "ops-tools" {
		t.Fatalf("expected id ops-tools, got %q", manifest.ID)
	}
	if len(manifest.Skills) != 1 || manifest.Skills[0] != "skills" {
		t.Fatalf("unexpected skills: %+v", manifest.Skills)
	}
	if len(manifest.MCPServers) != 1 || manifest.MCPServers[0].Name != "filesystem" {
		t.Fatalf("unexpected mcp servers: %+v", manifest.MCPServers)
	}
}

func TestParseManifest_RequiresID(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "bad", "tars.plugin.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(manifestPath, []byte(`{"name":"bad"}`), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, err := parseManifestFile(manifestPath)
	if err == nil {
		t.Fatal("expected parse error for missing id")
	}
}
