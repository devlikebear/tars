package extensions

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/skill"
	"github.com/devlikebear/tars/internal/tool"
)

func TestManagerReload_AggregatesSkillsPluginsAndMCP(t *testing.T) {
	root := t.TempDir()
	workspaceDir := filepath.Join(root, "workspace")
	workspaceSkillsDir := filepath.Join(workspaceDir, "skills", "workspace-skill")
	hubMCPDir := filepath.Join(workspaceDir, "mcp-servers", "filesystem")
	pluginDir := filepath.Join(root, "plugins", "ops")
	pluginSkillsDir := filepath.Join(pluginDir, "skills", "plugin-skill")

	writeFile(t, filepath.Join(workspaceSkillsDir, "SKILL.md"), "# Workspace Skill\nFrom workspace")
	writeFile(t, filepath.Join(hubMCPDir, "tars.mcp.json"), `{
  "schema_version": 1,
  "server": {
    "name": "hub-filesystem",
    "command": "npx",
    "args": ["-y", "@modelcontextprotocol/server-filesystem", "${MCP_DIR}/sandbox"]
  }
}`)
	writeFile(t, filepath.Join(workspaceDir, "skillhub.json"), `{
  "mcps": [
    {
      "name": "filesystem",
      "version": "0.1.0",
      "source": "tars-hub",
      "dir": "`+hubMCPDir+`",
      "manifest": "tars.mcp.json"
    }
  ]
}`)
	writeFile(t, filepath.Join(pluginSkillsDir, "SKILL.md"), "# Plugin Skill\nFrom plugin")
	writeFile(t, filepath.Join(pluginDir, "tars.plugin.json"), `{
  "id":"ops",
  "skills":["skills"],
  "mcp_servers":[{"name":"plugin-fs","command":"npx"}]
}`)

	mcpRuntime := &stubMCPRuntime{
		tools: []tool.Tool{
			{
				Name:        "mcp.plugin-fs.read_file",
				Description: "read file",
				Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
			},
		},
	}
	manager, err := NewManager(Options{
		WorkspaceDir:           workspaceDir,
		SkillsEnabled:          true,
		PluginsEnabled:         true,
		PluginsAllowMCPServers: true,
		SkillSources: []skill.SourceDir{
			{Source: skill.SourceWorkspace, Dir: filepath.Join(workspaceDir, "skills")},
		},
		PluginSources: []PluginSourceDir{
			{Source: SourceWorkspace, Dir: filepath.Join(root, "plugins")},
		},
		MCPBaseServers: []config.MCPServer{
			{Name: "base-fs", Command: "base-cmd"},
		},
		MCPRuntime: mcpRuntime,
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	if err := manager.Reload(context.Background()); err != nil {
		t.Fatalf("reload manager: %v", err)
	}
	snapshot := manager.Snapshot()
	if snapshot.Version == 0 {
		t.Fatalf("expected non-zero version")
	}
	if len(snapshot.Skills) != 2 {
		t.Fatalf("expected 2 merged skills, got %d", len(snapshot.Skills))
	}
	if snapshot.SkillPrompt == "" {
		t.Fatalf("expected skill prompt block")
	}
	if len(snapshot.MCPServers) != 3 {
		t.Fatalf("expected merged mcp servers, got %d", len(snapshot.MCPServers))
	}
	if len(manager.ChatTools()) != 1 {
		t.Fatalf("expected 1 dynamic mcp tool, got %d", len(manager.ChatTools()))
	}
	if len(mcpRuntime.lastServers) != 3 {
		t.Fatalf("expected runtime to receive merged server config, got %+v", mcpRuntime.lastServers)
	}
	if mcpRuntime.lastServers[2].Name != "hub-filesystem" {
		t.Fatalf("expected hub-managed mcp to be merged, got %+v", mcpRuntime.lastServers)
	}
}

func TestManagerReload_DoesNotMergePluginMCPServersByDefault(t *testing.T) {
	root := t.TempDir()
	workspaceDir := filepath.Join(root, "workspace")
	pluginDir := filepath.Join(root, "plugins", "ops")
	writeFile(t, filepath.Join(pluginDir, "tars.plugin.json"), `{
  "id":"ops",
  "mcp_servers":[{"name":"plugin-fs","command":"npx"}]
}`)

	mcpRuntime := &stubMCPRuntime{}
	manager, err := NewManager(Options{
		WorkspaceDir:   workspaceDir,
		SkillsEnabled:  false,
		PluginsEnabled: true,
		PluginSources: []PluginSourceDir{
			{Source: SourceWorkspace, Dir: filepath.Join(root, "plugins")},
		},
		MCPBaseServers: []config.MCPServer{
			{Name: "base-fs", Command: "base-cmd"},
		},
		MCPRuntime: mcpRuntime,
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if err := manager.Reload(context.Background()); err != nil {
		t.Fatalf("reload manager: %v", err)
	}
	snapshot := manager.Snapshot()
	if len(snapshot.MCPServers) != 1 || snapshot.MCPServers[0].Name != "base-fs" {
		t.Fatalf("expected only base mcp server when plugin mcp is disabled, got %+v", snapshot.MCPServers)
	}
	if len(mcpRuntime.lastServers) != 1 || mcpRuntime.lastServers[0].Name != "base-fs" {
		t.Fatalf("expected runtime to receive base servers only, got %+v", mcpRuntime.lastServers)
	}
}

func TestManagerWatch_BumpsVersionOnSkillChange(t *testing.T) {
	root := t.TempDir()
	workspaceDir := filepath.Join(root, "workspace")
	skillFile := filepath.Join(workspaceDir, "skills", "watch-skill", "SKILL.md")
	writeFile(t, skillFile, "# Watch Skill\nv1")

	manager, err := NewManager(Options{
		WorkspaceDir:   workspaceDir,
		SkillsEnabled:  true,
		PluginsEnabled: false,
		SkillSources: []skill.SourceDir{
			{Source: skill.SourceWorkspace, Dir: filepath.Join(workspaceDir, "skills")},
		},
		WatchSkills:   true,
		WatchDebounce: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	defer manager.Close()

	before := manager.Snapshot().Version
	writeFile(t, skillFile, "# Watch Skill\nv2")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		after := manager.Snapshot().Version
		if after > before {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("expected snapshot version to increase after file update (before=%d after=%d)", before, manager.Snapshot().Version)
}

func TestManagerReload_SkipsUnavailableSkillsFromSnapshotAndPrompt(t *testing.T) {
	root := t.TempDir()
	workspaceDir := filepath.Join(root, "workspace")
	writeFile(t, filepath.Join(workspaceDir, "skills", "deploy", "SKILL.md"), `---
name: deploy
requires_env: [DEPLOY_TOKEN]
---
# Deploy`)
	writeFile(t, filepath.Join(workspaceDir, "skills", "notes", "SKILL.md"), `---
name: notes
---
# Notes`)

	t.Setenv("DEPLOY_TOKEN", "")
	manager, err := NewManager(Options{
		WorkspaceDir:   workspaceDir,
		SkillsEnabled:  true,
		PluginsEnabled: false,
		SkillSources: []skill.SourceDir{
			{Source: skill.SourceWorkspace, Dir: filepath.Join(workspaceDir, "skills")},
		},
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	if err := manager.Reload(context.Background()); err != nil {
		t.Fatalf("reload manager: %v", err)
	}
	snapshot := manager.Snapshot()
	if len(snapshot.Skills) != 1 || snapshot.Skills[0].Name != "notes" {
		t.Fatalf("expected only notes skill in snapshot, got %+v", snapshot.Skills)
	}
	if strings.Contains(snapshot.SkillPrompt, "<name>deploy</name>") {
		t.Fatalf("expected unavailable skill to be removed from prompt, got %q", snapshot.SkillPrompt)
	}
	if _, ok := manager.FindSkill("deploy"); ok {
		t.Fatalf("expected unavailable skill to be absent from manager lookup")
	}
	if _, ok := manager.FindSkill("notes"); !ok {
		t.Fatalf("expected available skill to remain in manager lookup")
	}
	if len(snapshot.Diagnostics) == 0 || !strings.Contains(strings.Join(snapshot.Diagnostics, "\n"), "DEPLOY_TOKEN") {
		t.Fatalf("expected diagnostics to mention missing env var, got %+v", snapshot.Diagnostics)
	}
}

func TestManagerReload_SkipsUnavailablePluginsAndAnnotatesMCPSource(t *testing.T) {
	root := t.TempDir()
	workspaceDir := filepath.Join(root, "workspace")
	pluginRoot := filepath.Join(root, "plugins")
	writeFile(t, filepath.Join(pluginRoot, "available", "skills", "deploy", "SKILL.md"), "# Deploy")
	writeFile(t, filepath.Join(pluginRoot, "available", "tars.plugin.json"), `{
  "schema_version": 2,
  "id":"available",
  "skills":["skills"],
  "mcp_servers":[{"name":"plugin-http","transport":"streamable_http","url":"https://example.com/mcp"}]
}`)
	writeFile(t, filepath.Join(pluginRoot, "blocked", "skills", "ops", "SKILL.md"), "# Ops")
	writeFile(t, filepath.Join(pluginRoot, "blocked", "tars.plugin.json"), `{
  "schema_version": 2,
  "id":"blocked",
  "requires":{"env":["PLUGIN_TOKEN"]},
  "skills":["skills"],
  "mcp_servers":[{"name":"blocked-http","transport":"streamable_http","url":"https://blocked.example.com/mcp"}]
}`)

	t.Setenv("PLUGIN_TOKEN", "")
	manager, err := NewManager(Options{
		WorkspaceDir:           workspaceDir,
		SkillsEnabled:          true,
		PluginsEnabled:         true,
		PluginsAllowMCPServers: true,
		PluginSources: []PluginSourceDir{
			{Source: SourceWorkspace, Dir: pluginRoot},
		},
		MCPBaseServers: []config.MCPServer{
			{Name: "base-http", Transport: "streamable_http", URL: "https://base.example.com/mcp"},
		},
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	if err := manager.Reload(context.Background()); err != nil {
		t.Fatalf("reload manager: %v", err)
	}
	snapshot := manager.Snapshot()
	if len(snapshot.Plugins) != 1 || snapshot.Plugins[0].ID != "available" {
		t.Fatalf("expected only available plugin in snapshot, got %+v", snapshot.Plugins)
	}
	if len(snapshot.Skills) != 1 || snapshot.Skills[0].Name != "deploy" {
		t.Fatalf("expected only skill from available plugin, got %+v", snapshot.Skills)
	}
	if len(snapshot.MCPServers) != 2 {
		t.Fatalf("expected base + available plugin mcp servers, got %+v", snapshot.MCPServers)
	}
	if snapshot.MCPServers[0].Source != "config" || snapshot.MCPServers[1].Source != "plugin" {
		t.Fatalf("expected mcp source labels config/plugin, got %+v", snapshot.MCPServers)
	}
	if len(snapshot.Diagnostics) == 0 || !strings.Contains(strings.Join(snapshot.Diagnostics, "\n"), "PLUGIN_TOKEN") {
		t.Fatalf("expected diagnostics to mention blocked plugin env, got %+v", snapshot.Diagnostics)
	}
}

type stubMCPRuntime struct {
	lastServers []config.MCPServer
	tools       []tool.Tool
}

func (s *stubMCPRuntime) SetServers(servers []config.MCPServer) {
	s.lastServers = append([]config.MCPServer(nil), servers...)
}

func (s *stubMCPRuntime) BuildTools(_ context.Context) ([]tool.Tool, error) {
	return append([]tool.Tool(nil), s.tools...), nil
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
