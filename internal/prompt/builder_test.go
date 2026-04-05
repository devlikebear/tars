package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuild(t *testing.T) {
	root := t.TempDir()

	// Create bootstrap files
	files := map[string]string{
		"IDENTITY.md":  "# IDENTITY.md\n\nName: TARS",
		"USER.md":      "# USER.md\n\nName: Alice",
		"PROJECT.md":   "# PROJECT.md\n\nProject policy",
		"AGENTS.md":    "# AGENTS.md\n\nOperating guidelines",
		"TOOLS.md":     "# TOOLS.md\n\nAvailable tools",
		"HEARTBEAT.md": "# HEARTBEAT.md\n\nCheck daily tasks",
		"MEMORY.md":    "# MEMORY.md\n\nKey facts",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	result := Build(BuildOptions{WorkspaceDir: root})

	// Static bootstrap should include identity/user/heartbeat.
	wantIncluded := []string{
		files["IDENTITY.md"],
		files["USER.md"],
		files["HEARTBEAT.md"],
	}
	for _, content := range wantIncluded {
		if !strings.Contains(result, content) {
			t.Errorf("expected prompt to contain %q", content)
		}
	}
	if strings.Contains(result, files["MEMORY.md"]) {
		t.Errorf("expected static prompt to exclude MEMORY.md content")
	}
	if strings.Contains(result, files["PROJECT.md"]) {
		t.Errorf("expected prompt to exclude PROJECT.md content (project system removed)")
	}

	// Should have section headers
	if !strings.Contains(result, "IDENTITY") {
		t.Error("expected IDENTITY section")
	}
}

func TestBuild_SubAgent(t *testing.T) {
	root := t.TempDir()

	files := map[string]string{
		"IDENTITY.md":  "# IDENTITY.md\n\nName: TARS",
		"USER.md":      "# USER.md\n\nName: Alice",
		"PROJECT.md":   "# PROJECT.md\n\nProject policy",
		"AGENTS.md":    "# AGENTS.md\n\nOperating guidelines",
		"TOOLS.md":     "# TOOLS.md\n\nAvailable tools",
		"HEARTBEAT.md": "# HEARTBEAT.md\n\nCheck daily tasks",
		"MEMORY.md":    "# MEMORY.md\n\nKey facts",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	result := Build(BuildOptions{WorkspaceDir: root, SubAgent: true})

	// Sub-agent should only include AGENTS.md and TOOLS.md
	if !strings.Contains(result, "Operating guidelines") {
		t.Error("expected AGENTS.md content in sub-agent prompt")
	}
	if !strings.Contains(result, "Available tools") {
		t.Error("expected TOOLS.md content in sub-agent prompt")
	}

	// Sub-agent should NOT include other files
	if strings.Contains(result, "Name: TARS") {
		t.Error("sub-agent prompt should not contain IDENTITY.md content")
	}
	if strings.Contains(result, "Name: Alice") {
		t.Error("sub-agent prompt should not contain USER.md content")
	}
	if strings.Contains(result, "Check daily tasks") {
		t.Error("sub-agent prompt should not contain HEARTBEAT.md content")
	}
	if strings.Contains(result, "Key facts") {
		t.Error("sub-agent prompt should not contain MEMORY.md content")
	}
	if strings.Contains(result, "Project policy") {
		t.Error("sub-agent prompt should not contain PROJECT.md content")
	}
}

func TestBuild_TruncateLargeFile(t *testing.T) {
	root := t.TempDir()

	// Create a file larger than 20000 chars
	large := strings.Repeat("x", 25000)
	if err := os.WriteFile(filepath.Join(root, "IDENTITY.md"), []byte(large), 0o644); err != nil {
		t.Fatalf("write large file: %v", err)
	}

	result := Build(BuildOptions{WorkspaceDir: root})

	// The full 25000-char content should NOT appear
	if strings.Contains(result, large) {
		t.Error("expected large file to be truncated")
	}
	// But some content should appear (first 20000 chars)
	if !strings.Contains(result, strings.Repeat("x", 1000)) {
		t.Error("expected truncated content to still be present")
	}
}

func TestBuild_MissingFiles(t *testing.T) {
	root := t.TempDir()
	// No files at all — should not error, return non-empty base prompt
	result := Build(BuildOptions{WorkspaceDir: root})
	if result == "" {
		t.Error("expected non-empty prompt even with no workspace files")
	}
}

func TestBuild_IdentitySection(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "IDENTITY.md"), []byte("identity core"), 0o644); err != nil {
		t.Fatalf("write IDENTITY.md: %v", err)
	}

	result := Build(BuildOptions{WorkspaceDir: root})
	if !strings.Contains(result, "identity core") {
		t.Fatalf("expected identity content in prompt, got %q", result)
	}
}

func TestBuildResult_PrioritizesHigherOrderStaticSections(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"USER.md":      strings.Repeat("user-", 120),
		"IDENTITY.md":  strings.Repeat("identity-", 120),
		"HEARTBEAT.md": strings.Repeat("heartbeat-", 120),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	result := BuildResultFor(BuildOptions{
		WorkspaceDir:       root,
		StaticBudgetTokens: 460,
		TotalBudgetTokens:  460,
	})

	if !strings.Contains(result.Prompt, files["USER.md"][:120]) {
		t.Fatalf("expected user section to survive tight budget, got %q", result.Prompt)
	}
	if strings.Contains(result.Prompt, files["HEARTBEAT.md"][:80]) {
		t.Fatalf("expected lower-priority heartbeat section to be trimmed first, got %q", result.Prompt)
	}
	if result.TotalTokens > 460 {
		t.Fatalf("expected total tokens <= 460, got %d", result.TotalTokens)
	}
}

func TestBuildResult_ClampsRelevantMemoryToRemainingTotalBudget(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "PROJECT.md"), []byte(strings.Repeat("project ", 160)), 0o644); err != nil {
		t.Fatalf("write PROJECT.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "MEMORY.md"), []byte("User prefers black coffee with oat milk.\n"), 0o644); err != nil {
		t.Fatalf("write MEMORY.md: %v", err)
	}

	result := BuildResultFor(BuildOptions{
		WorkspaceDir:         root,
		Query:                "what coffee do i prefer?",
		StaticBudgetTokens:   260,
		RelevantBudgetTokens: 80,
		TotalBudgetTokens:    500,
	})

	if result.TotalTokens > 500 {
		t.Fatalf("expected total tokens <= 500, got %d", result.TotalTokens)
	}
	if result.RelevantTokens > 0 && result.StaticTokens+result.RelevantTokens > 500 {
		t.Fatalf("expected relevant memory to fit remaining budget, got static=%d relevant=%d", result.StaticTokens, result.RelevantTokens)
	}
}
