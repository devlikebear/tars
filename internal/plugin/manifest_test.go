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

func TestParseManifest_SupportsV2Metadata(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "sample", "tars.plugin.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := `{
  "schema_version": 2,
  "id": "remote-ops",
  "name": "Remote Ops",
  "description": "Remote MCP plugin",
  "version": "2.0.0",
  "skills": ["skills"],
  "requires": {
    "bins": ["uv"],
    "env": ["OPENAI_API_KEY"]
  },
  "supported_os": ["darwin", "linux"],
  "supported_arch": ["arm64"],
  "default_project_profile": "swarm",
  "policies": {
    "tools_allow": ["read_file", "grep"],
    "tools_deny": ["write_file"]
  },
  "mcp_servers": [
    {
      "name": "remote-http",
      "transport": "streamable_http",
      "url": "https://mcp.example.com",
      "auth_mode": "bearer",
      "auth_token_env": "MCP_TOKEN"
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
	if manifest.SchemaVersion != 2 {
		t.Fatalf("expected schema version 2, got %d", manifest.SchemaVersion)
	}
	if got := manifest.Requires.Bins; len(got) != 1 || got[0] != "uv" {
		t.Fatalf("unexpected required bins: %+v", got)
	}
	if got := manifest.Requires.Env; len(got) != 1 || got[0] != "OPENAI_API_KEY" {
		t.Fatalf("unexpected required env: %+v", got)
	}
	if got := manifest.SupportedOS; len(got) != 2 || got[0] != "darwin" || got[1] != "linux" {
		t.Fatalf("unexpected supported os: %+v", got)
	}
	if got := manifest.SupportedArch; len(got) != 1 || got[0] != "arm64" {
		t.Fatalf("unexpected supported arch: %+v", got)
	}
	if manifest.DefaultProjectProfile != "swarm" {
		t.Fatalf("expected default project profile swarm, got %q", manifest.DefaultProjectProfile)
	}
	if got := manifest.Policies.ToolsAllow; len(got) != 2 || got[0] != "read_file" || got[1] != "grep" {
		t.Fatalf("unexpected tools_allow: %+v", got)
	}
	if got := manifest.Policies.ToolsDeny; len(got) != 1 || got[0] != "write_file" {
		t.Fatalf("unexpected tools_deny: %+v", got)
	}
	if len(manifest.MCPServers) != 1 {
		t.Fatalf("expected 1 mcp server, got %+v", manifest.MCPServers)
	}
	server := manifest.MCPServers[0]
	if server.Transport != "streamable_http" || server.URL != "https://mcp.example.com" {
		t.Fatalf("unexpected remote mcp server: %+v", server)
	}
	if server.AuthMode != "bearer" || server.AuthTokenEnv != "MCP_TOKEN" {
		t.Fatalf("unexpected mcp auth config: %+v", server)
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
