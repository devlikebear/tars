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

	loaded, err := loadWorkspaceGatewayAgents(workspace)
	if err != nil {
		t.Fatalf("load workspace agents: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected one valid deduplicated agent, got %+v", loaded)
	}
	if loaded[0].Name != "researcher" {
		t.Fatalf("unexpected agent name: %+v", loaded[0])
	}
	if !strings.Contains(loaded[0].Prompt, "first prompt") {
		t.Fatalf("expected first prompt selected by path order, got %+v", loaded[0])
	}
}
