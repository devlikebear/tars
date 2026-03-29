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

func TestParseManifest_V3WithToolsProvider(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "browser", "tars.plugin.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := `{
  "schema_version": 3,
  "id": "browser-automation",
  "name": "Browser Automation",
  "version": "1.0.0",
  "tools_provider": {
    "type": "script",
    "entry": "bin/browser-tools"
  },
  "lifecycle": {
    "on_start": "echo starting",
    "on_stop": "echo stopping"
  },
  "http_routes": [
    {"path": "/v1/browser/*", "handler": "browser_handler"}
  ]
}`
	if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	manifest, err := parseManifestFile(manifestPath)
	if err != nil {
		t.Fatalf("parse v3 manifest: %v", err)
	}
	if manifest.SchemaVersion != 3 {
		t.Fatalf("expected schema version 3, got %d", manifest.SchemaVersion)
	}
	if manifest.ToolsProvider == nil {
		t.Fatal("expected tools_provider to be set")
	}
	if manifest.ToolsProvider.Type != "script" {
		t.Fatalf("expected tools_provider type script, got %q", manifest.ToolsProvider.Type)
	}
	if manifest.ToolsProvider.Entry != "bin/browser-tools" {
		t.Fatalf("expected entry bin/browser-tools, got %q", manifest.ToolsProvider.Entry)
	}
	if manifest.Lifecycle == nil {
		t.Fatal("expected lifecycle to be set")
	}
	if manifest.Lifecycle.OnStart != "echo starting" {
		t.Fatalf("expected on_start echo starting, got %q", manifest.Lifecycle.OnStart)
	}
	if manifest.Lifecycle.OnStop != "echo stopping" {
		t.Fatalf("expected on_stop echo stopping, got %q", manifest.Lifecycle.OnStop)
	}
	if len(manifest.HTTPRoutes) != 1 {
		t.Fatalf("expected 1 http route, got %d", len(manifest.HTTPRoutes))
	}
	if manifest.HTTPRoutes[0].Path != "/v1/browser/*" {
		t.Fatalf("expected route path /v1/browser/*, got %q", manifest.HTTPRoutes[0].Path)
	}
	if manifest.HTTPRoutes[0].Handler != "browser_handler" {
		t.Fatalf("expected handler browser_handler, got %q", manifest.HTTPRoutes[0].Handler)
	}
}

func TestParseManifest_V3FieldsOnV2Rejected(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "tools_provider on v2",
			content: `{"schema_version":2,"id":"bad","tools_provider":{"type":"script","entry":"x"}}`,
		},
		{
			name:    "lifecycle on v2",
			content: `{"schema_version":2,"id":"bad","lifecycle":{"on_start":"echo hi"}}`,
		},
		{
			name:    "http_routes on v2",
			content: `{"schema_version":2,"id":"bad","http_routes":[{"path":"/foo"}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(root, tt.name)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			manifestPath := filepath.Join(dir, "tars.plugin.json")
			if err := os.WriteFile(manifestPath, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("write manifest: %v", err)
			}
			_, err := parseManifestFile(manifestPath)
			if err == nil {
				t.Fatal("expected error for v3 field on v2 manifest")
			}
		})
	}
}

func TestParseManifest_V3InvalidToolsProviderType(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "bad", "tars.plugin.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := `{"schema_version":3,"id":"bad","tools_provider":{"type":"unknown"}}`
	if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	_, err := parseManifestFile(manifestPath)
	if err == nil {
		t.Fatal("expected error for unsupported tools_provider type")
	}
}
