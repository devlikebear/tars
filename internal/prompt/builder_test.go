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
		"IDENTITY.md": "# IDENTITY.md\n\nName: TARS",
		"USER.md":     "# USER.md\n\nName: Alice",
		"AGENTS.md":   "# AGENTS.md\n\nOperating guidelines",
		"TOOLS.md":    "# TOOLS.md\n\nAvailable tools",
		"MEMORY.md":   "# MEMORY.md\n\nKey facts",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	result := Build(BuildOptions{WorkspaceDir: root})

	// Static bootstrap should include identity/user.
	wantIncluded := []string{
		files["IDENTITY.md"],
		files["USER.md"],
	}
	for _, content := range wantIncluded {
		if !strings.Contains(result, content) {
			t.Errorf("expected prompt to contain %q", content)
		}
	}
	if strings.Contains(result, files["MEMORY.md"]) {
		t.Errorf("expected static prompt to exclude MEMORY.md content")
	}
	// Should have section headers
	if !strings.Contains(result, "IDENTITY") {
		t.Error("expected IDENTITY section")
	}
}

func TestBuild_SubAgent(t *testing.T) {
	root := t.TempDir()

	files := map[string]string{
		"IDENTITY.md": "# IDENTITY.md\n\nName: TARS",
		"USER.md":     "# USER.md\n\nName: Alice",
		"AGENTS.md":   "# AGENTS.md\n\nOperating guidelines",
		"TOOLS.md":    "# TOOLS.md\n\nAvailable tools",
		"MEMORY.md":   "# MEMORY.md\n\nKey facts",
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
	if strings.Contains(result, "Key facts") {
		t.Error("sub-agent prompt should not contain MEMORY.md content")
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

func TestBuild_PlanningSectionPresentForMainAgent(t *testing.T) {
	root := t.TempDir()
	result := Build(BuildOptions{WorkspaceDir: root})

	if !strings.Contains(result, "## Planning") {
		t.Error("expected main agent prompt to contain Planning section header")
	}
	// Spot-check the canonical action vocabulary including the propose/
	// approve workflow added in CON-053.
	for _, want := range []string{
		"tasks(action=\"plan_set\"",
		"tasks(action=\"add\"",
		"tasks(action=\"plan_propose\"",
		"tasks(action=\"plan_approve\"",
		"STOP and wait",
		"in_progress",
		"completed",
		"paused",
		"aborted",
	} {
		if !strings.Contains(result, want) {
			t.Errorf("expected Planning section to mention %q", want)
		}
	}
}

func TestBuild_PlanningSectionAbsentForSubAgent(t *testing.T) {
	root := t.TempDir()
	result := Build(BuildOptions{WorkspaceDir: root, SubAgent: true})

	if strings.Contains(result, "## Planning") {
		t.Error("sub-agent prompt should not contain Planning section")
	}
	if strings.Contains(result, "tasks(action=\"plan_propose\"") {
		t.Error("sub-agent prompt should not reference tasks tool actions")
	}
}

// TestBuild_PlanningSectionClarifyModes locks the user-visible behavior
// of CON-052: each mode value must produce a Planning section that
// matches the documented stance, and unknown / empty values fall back to
// "smart" so a typo can't silently flip planning into "always ask".
func TestBuild_PlanningSectionClarifyModes(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		mode     string
		mustHave []string
		mustMiss []string
		desc     string
	}{
		{
			mode:     "smart",
			mustHave: []string{"evaluate ambiguity", "If clear → draft immediately"},
			mustMiss: []string{"draft a plan immediately — do not ask", "ALWAYS ask 1–3 clarifying questions FIRST"},
			desc:     "smart",
		},
		{
			mode:     "auto",
			mustHave: []string{"draft a plan immediately — do not ask"},
			mustMiss: []string{"evaluate ambiguity", "ALWAYS ask 1–3"},
			desc:     "auto",
		},
		{
			mode:     "ask",
			mustHave: []string{"ALWAYS ask 1–3 clarifying questions FIRST"},
			mustMiss: []string{"evaluate ambiguity", "draft a plan immediately — do not ask"},
			desc:     "ask",
		},
		{
			// Unknown / empty modes must fall back to smart — never silently
			// flip into the noisier "ask" or the more aggressive "auto".
			mode:     "garbage",
			mustHave: []string{"evaluate ambiguity"},
			mustMiss: []string{"draft a plan immediately — do not ask", "ALWAYS ask 1–3"},
			desc:     "unknown→smart",
		},
		{
			mode:     "",
			mustHave: []string{"evaluate ambiguity"},
			mustMiss: []string{"draft a plan immediately — do not ask", "ALWAYS ask 1–3"},
			desc:     "empty→smart",
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			result := Build(BuildOptions{WorkspaceDir: root, PlanClarifyMode: tc.mode})
			for _, want := range tc.mustHave {
				if !strings.Contains(result, want) {
					t.Errorf("mode %q: expected prompt to contain %q", tc.mode, want)
				}
			}
			for _, miss := range tc.mustMiss {
				if strings.Contains(result, miss) {
					t.Errorf("mode %q: expected prompt to NOT contain %q", tc.mode, miss)
				}
			}
			// Every mode still leads into the same propose/approve guidance.
			if !strings.Contains(result, "tasks(action=\"plan_propose\"") {
				t.Errorf("mode %q: expected propose/approve guidance regardless of clarify stance", tc.mode)
			}
		})
	}
}

func TestBuild_PlanningSectionWithinBudget(t *testing.T) {
	// Default budgets must absorb the new Planning section without truncating
	// the workspace bootstrap content. This guards against a future PR
	// expanding Planning past its allowance and silently squeezing IDENTITY/USER.
	root := t.TempDir()
	files := map[string]string{
		"USER.md":     "# USER.md\n\nAlice",
		"IDENTITY.md": "# IDENTITY.md\n\nTARS",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	result := BuildResultFor(BuildOptions{WorkspaceDir: root})

	if result.TotalTokens > defaultTotalBudgetTokens {
		t.Fatalf("total tokens %d exceeds default budget %d", result.TotalTokens, defaultTotalBudgetTokens)
	}
	if !strings.Contains(result.Prompt, "Alice") {
		t.Error("expected USER.md content to remain after Planning section was added")
	}
	if !strings.Contains(result.Prompt, "TARS") {
		t.Error("expected IDENTITY.md content to remain after Planning section was added")
	}
}

func TestBuildResult_PrioritizesHigherOrderStaticSections(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"USER.md":     strings.Repeat("user-", 120),
		"IDENTITY.md": strings.Repeat("identity-", 120),
		"TOOLS.md":    strings.Repeat("tools-", 120),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	// Floor must stay above the hardcoded header (Current time + Response
	// Formatting + Planning). The Planning section grew with CON-052 +
	// CON-053 + CON-054 to ~340 tokens, so the older 460-token cap no
	// longer leaves room for any workspace bootstrap content. 700 keeps
	// the prioritization assertion meaningful (USER fits, IDENTITY/TOOLS
	// get clamped) without the test doubling as an upper-bound on the
	// header itself.
	result := BuildResultFor(BuildOptions{
		WorkspaceDir:       root,
		StaticBudgetTokens: 700,
		TotalBudgetTokens:  700,
	})

	if !strings.Contains(result.Prompt, files["USER.md"][:120]) {
		t.Fatalf("expected user section to survive tight budget, got %q", result.Prompt)
	}
	if result.TotalTokens > 700 {
		t.Fatalf("expected total tokens <= 700, got %d", result.TotalTokens)
	}
}

func TestBuildResult_ClampsRelevantMemoryToRemainingTotalBudget(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "USER.md"), []byte(strings.Repeat("user ", 160)), 0o644); err != nil {
		t.Fatalf("write USER.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "MEMORY.md"), []byte("User prefers black coffee with oat milk.\n"), 0o644); err != nil {
		t.Fatalf("write MEMORY.md: %v", err)
	}

	// Budget here is a stress test for the clamping logic, not a target
	// for production. The total floor must accommodate the hardcoded
	// header (Current time + Response Formatting + Planning ≈ ~280 tokens)
	// plus the static USER section, otherwise relevant memory has nothing
	// left to clamp. 700 keeps the assertion meaningful while leaving
	// headroom for any future hardcoded-header tweaks.
	result := BuildResultFor(BuildOptions{
		WorkspaceDir:         root,
		Query:                "what coffee do i prefer?",
		StaticBudgetTokens:   460,
		RelevantBudgetTokens: 80,
		TotalBudgetTokens:    700,
	})

	if result.TotalTokens > 700 {
		t.Fatalf("expected total tokens <= 700, got %d", result.TotalTokens)
	}
	if result.RelevantTokens > 0 && result.StaticTokens+result.RelevantTokens > 700 {
		t.Fatalf("expected relevant memory to fit remaining budget, got static=%d relevant=%d", result.StaticTokens, result.RelevantTokens)
	}
}
