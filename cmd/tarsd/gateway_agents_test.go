package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadWorkspaceGatewayAgents_FiltersInvalidDuplicateAndEmptyPrompt(t *testing.T) {
	workspace := t.TempDir()
	first := filepath.Join(workspace, "agents", "a", "AGENT.md")
	second := filepath.Join(workspace, "agents", "b", "AGENT.md")
	invalid := filepath.Join(workspace, "agents", "invalid", "AGENT.md")
	emptyPrompt := filepath.Join(workspace, "agents", "empty", "AGENT.md")

	for _, path := range []string{first, second, invalid, emptyPrompt} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}

	rawFirst := `---
name: researcher
description: first
---
first prompt
`
	rawSecond := `---
name: Researcher
description: second
---
second prompt
`
	rawInvalid := `---
name: bad name
description: invalid
---
should be skipped
`
	rawEmptyPrompt := `---
name: empty
description: no body
---
`

	if err := os.WriteFile(first, []byte(rawFirst), 0o644); err != nil {
		t.Fatalf("write first: %v", err)
	}
	if err := os.WriteFile(second, []byte(rawSecond), 0o644); err != nil {
		t.Fatalf("write second: %v", err)
	}
	if err := os.WriteFile(invalid, []byte(rawInvalid), 0o644); err != nil {
		t.Fatalf("write invalid: %v", err)
	}
	if err := os.WriteFile(emptyPrompt, []byte(rawEmptyPrompt), 0o644); err != nil {
		t.Fatalf("write empty prompt: %v", err)
	}

	loaded, diagnostics, err := loadWorkspaceGatewayAgents(workspace)
	if err != nil {
		t.Fatalf("load workspace agents: %v", err)
	}
	_ = diagnostics
	if len(loaded) != 1 {
		t.Fatalf("expected one valid deduplicated agent, got %+v", loaded)
	}
	if loaded[0].Name != "researcher" {
		t.Fatalf("unexpected agent name: %+v", loaded[0])
	}
	if !strings.Contains(loaded[0].Prompt, "first prompt") {
		t.Fatalf("expected first prompt selected by path order, got %+v", loaded[0])
	}
	if loaded[0].PolicyMode != "full" {
		t.Fatalf("expected default full policy mode, got %+v", loaded[0])
	}
	if len(loaded[0].ToolsAllow) != 0 {
		t.Fatalf("expected empty tools allow for full mode, got %+v", loaded[0].ToolsAllow)
	}
}

func TestLoadWorkspaceGatewayAgents_ToolsAllowListCanonicalization(t *testing.T) {
	workspace := t.TempDir()
	agentPath := filepath.Join(workspace, "agents", "researcher", "AGENT.md")
	if err := os.MkdirAll(filepath.Dir(agentPath), 0o755); err != nil {
		t.Fatalf("mkdir agent dir: %v", err)
	}
	raw := `---
name: researcher
description: Research worker
tools_allow:
  - read_file
  - shell_exec
  - read_file
  - list_dir
---
Find evidence first and answer briefly.
`
	if err := os.WriteFile(agentPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("write agent: %v", err)
	}

	loaded, diagnostics, err := loadWorkspaceGatewayAgents(workspace)
	if err != nil {
		t.Fatalf("load workspace agents: %v", err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("expected no diagnostics, got %+v", diagnostics)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected one agent, got %+v", loaded)
	}
	if loaded[0].PolicyMode != "allowlist" {
		t.Fatalf("expected allowlist mode, got %+v", loaded[0])
	}
	if got, want := strings.Join(loaded[0].ToolsAllow, ","), "exec,list_dir,read_file"; got != want {
		t.Fatalf("unexpected tools allow list: got=%q want=%q", got, want)
	}
}

func TestLoadWorkspaceGatewayAgents_ToolsAllowUnknownOnlySkipsAgent(t *testing.T) {
	workspace := t.TempDir()
	agentPath := filepath.Join(workspace, "agents", "researcher", "AGENT.md")
	if err := os.MkdirAll(filepath.Dir(agentPath), 0o755); err != nil {
		t.Fatalf("mkdir agent dir: %v", err)
	}
	raw := `---
name: researcher
tools_allow:
  - totally_unknown_tool
---
Find evidence first and answer briefly.
`
	if err := os.WriteFile(agentPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("write agent: %v", err)
	}

	loaded, diagnostics, err := loadWorkspaceGatewayAgents(workspace)
	if err != nil {
		t.Fatalf("load workspace agents: %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("expected agent skipped when tools_allow is invalid-only, got %+v", loaded)
	}
	if len(diagnostics) == 0 {
		t.Fatalf("expected diagnostics for invalid tools_allow")
	}
	if !strings.Contains(strings.ToLower(strings.Join(diagnostics, "\n")), "tools_allow") {
		t.Fatalf("expected tools_allow diagnostics, got %+v", diagnostics)
	}
}
