package tarsserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindWorkspaceGatewayAgentFiles_SortsNestedAgentDocuments(t *testing.T) {
	workspace := t.TempDir()
	first := filepath.Join(workspace, "agents", "b", "AGENT.md")
	second := filepath.Join(workspace, "agents", "a", "AGENT.md")
	ignored := filepath.Join(workspace, "agents", "a", "README.md")

	for _, path := range []string{first, second, ignored} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte("content"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	got, err := findWorkspaceGatewayAgentFiles(workspace)
	if err != nil {
		t.Fatalf("find workspace gateway agent files: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 AGENT.md files, got %+v", got)
	}
	if got[0] != second || got[1] != first {
		t.Fatalf("expected sorted agent files [%s %s], got %+v", second, first, got)
	}
}

func TestBuildWorkspaceGatewayAgent_InvalidFixedRoutingReturnsDiagnostics(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "agents", "researcher", "AGENT.md")
	raw := `---
name: researcher
tools_allow:
  - read_file
session_routing_mode: fixed
---
Find evidence first and answer briefly.
`

	agent, diagnostics, ok, err := buildWorkspaceGatewayAgent(path, raw, knownGatewayPromptTools(workspace))
	if err != nil {
		t.Fatalf("build workspace gateway agent: %v", err)
	}
	if ok {
		t.Fatalf("expected invalid fixed routing to skip agent, got %+v", agent)
	}
	if len(diagnostics) == 0 {
		t.Fatalf("expected diagnostics, got none")
	}
	if !strings.Contains(strings.ToLower(strings.Join(diagnostics, "\n")), "session_routing_mode") {
		t.Fatalf("expected session_routing_mode diagnostics, got %+v", diagnostics)
	}
}

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

func TestLoadWorkspaceGatewayAgents_ToolsAllowGroupsPatternsAndSessionRouting(t *testing.T) {
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
tools_allow_groups:
  - memory
tools_allow_patterns:
  - "^exec$"
  - "^list_"
session_routing_mode: fixed
session_fixed_id: sess_fixed
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
	agent := loaded[0]
	if agent.PolicyMode != "allowlist" {
		t.Fatalf("expected allowlist mode, got %+v", agent)
	}
	if got, want := strings.Join(agent.ToolsAllow, ","), "exec,knowledge,list_dir,memory,read_file"; got != want {
		t.Fatalf("unexpected tools allow list: got=%q want=%q", got, want)
	}
	if got, want := strings.Join(agent.ToolsAllowGroups, ","), "memory"; got != want {
		t.Fatalf("unexpected tools allow groups: got=%q want=%q", got, want)
	}
	if got, want := strings.Join(agent.ToolsAllowPatterns, ","), "^exec$,^list_"; got != want {
		t.Fatalf("unexpected tools allow patterns: got=%q want=%q", got, want)
	}
	if agent.SessionRoutingMode != "fixed" {
		t.Fatalf("expected fixed session routing mode, got %+v", agent)
	}
	if agent.SessionFixedID != "sess_fixed" {
		t.Fatalf("expected fixed session id, got %+v", agent)
	}
}

func TestLoadWorkspaceGatewayAgents_InvalidGroupsPatternsAndFixedRoutingSkipAgent(t *testing.T) {
	workspace := t.TempDir()
	agentPath := filepath.Join(workspace, "agents", "researcher", "AGENT.md")
	if err := os.MkdirAll(filepath.Dir(agentPath), 0o755); err != nil {
		t.Fatalf("mkdir agent dir: %v", err)
	}
	raw := `---
name: researcher
tools_allow:
  - read_file
tools_allow_groups:
  - unknown_group
tools_allow_patterns:
  - "("
session_routing_mode: fixed
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
		t.Fatalf("expected agent skipped for invalid policy/routing, got %+v", loaded)
	}
	diagText := strings.ToLower(strings.Join(diagnostics, "\n"))
	if !strings.Contains(diagText, "tools_allow_groups") {
		t.Fatalf("expected tools_allow_groups diagnostics, got %+v", diagnostics)
	}
	if !strings.Contains(diagText, "tools_allow_patterns") {
		t.Fatalf("expected tools_allow_patterns diagnostics, got %+v", diagnostics)
	}
	if !strings.Contains(diagText, "session_routing_mode") {
		t.Fatalf("expected session_routing_mode diagnostics, got %+v", diagnostics)
	}
}

func TestLoadWorkspaceGatewayAgents_ToolsDenyAndRiskMax(t *testing.T) {
	workspace := t.TempDir()
	agentPath := filepath.Join(workspace, "agents", "researcher", "AGENT.md")
	if err := os.MkdirAll(filepath.Dir(agentPath), 0o755); err != nil {
		t.Fatalf("mkdir agent dir: %v", err)
	}
	raw := `---
name: researcher
tools_allow:
  - read_file
  - exec
  - glob
tools_deny:
  - exec
tools_risk_max: low
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
	agent := loaded[0]
	if agent.PolicyMode != "allowlist" {
		t.Fatalf("expected allowlist mode, got %+v", agent)
	}
	if got, want := strings.Join(agent.ToolsAllow, ","), "read_file"; got != want {
		t.Fatalf("unexpected tools allow list after deny/risk filter: got=%q want=%q", got, want)
	}
	if got, want := strings.Join(agent.ToolsDeny, ","), "exec"; got != want {
		t.Fatalf("unexpected tools_deny: got=%q want=%q", got, want)
	}
	if agent.ToolsRiskMax != "low" {
		t.Fatalf("unexpected tools_risk_max: %+v", agent)
	}
}
