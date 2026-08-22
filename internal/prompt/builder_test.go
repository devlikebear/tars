package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		"done_criteria",
		"tasks(action=\"contract_update\"",
		"tasks(action=\"contract_approve\"",
		"tasks(action=\"add\"",
		"tasks(action=\"evidence_add\"",
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

	// Floor must stay above the always-on scaffolding (Response Formatting +
	// Planning + Long-running Commands, plus the Current Time tail, which is
	// charged up front even though it renders last). Each addition to
	// that scaffolding forces a bump here; 850 accommodates the
	// post-Phase-2 layout while still keeping the prioritization
	// assertion meaningful (USER fits, IDENTITY/TOOLS get clamped).
	result := BuildResultFor(BuildOptions{
		WorkspaceDir:       root,
		StaticBudgetTokens: 850,
		TotalBudgetTokens:  850,
	})

	if !strings.Contains(result.Prompt, files["USER.md"][:120]) {
		t.Fatalf("expected user section to survive tight budget, got %q", result.Prompt)
	}
	if result.TotalTokens > 850 {
		t.Fatalf("expected total tokens <= 850, got %d", result.TotalTokens)
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
	// for production. The total floor must accommodate the always-on
	// scaffolding (Response Formatting + Planning + Long-running Commands +
	// Current Time ≈ ~430 tokens) plus the static USER section, otherwise
	// relevant memory has nothing left to clamp. 850 keeps the assertion
	// meaningful with headroom for future tweaks.
	result := BuildResultFor(BuildOptions{
		WorkspaceDir:         root,
		Query:                "what coffee do i prefer?",
		StaticBudgetTokens:   460,
		RelevantBudgetTokens: 80,
		TotalBudgetTokens:    1000,
	})

	if result.TotalTokens > 1000 {
		t.Fatalf("expected total tokens <= 1000, got %d", result.TotalTokens)
	}
	if result.RelevantTokens > 0 && result.StaticTokens+result.RelevantTokens > 1000 {
		t.Fatalf("expected relevant memory to fit remaining budget, got static=%d relevant=%d", result.StaticTokens, result.RelevantTokens)
	}
}

// withFixedTime pins the builder's clock for the duration of the test.
func withFixedTime(t *testing.T, at time.Time) {
	t.Helper()
	prev := timeNow
	timeNow = func() time.Time { return at }
	t.Cleanup(func() { timeNow = prev })
}

// LP-001: the cacheable region must not move when the clock does. Two builds
// hours apart have to agree byte-for-byte through the end of the static
// sections, or every provider's prefix cache misses on every turn.
func TestBuildResult_StaticPrefixSurvivesClockChange(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"IDENTITY.md": "# IDENTITY.md\n\nName: TARS",
		"USER.md":     "# USER.md\n\nName: Alice",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	withFixedTime(t, time.Date(2026, 8, 22, 10, 23, 45, 0, time.UTC))
	first := BuildResultFor(BuildOptions{WorkspaceDir: root})

	withFixedTime(t, time.Date(2026, 8, 22, 17, 4, 9, 0, time.UTC))
	second := BuildResultFor(BuildOptions{WorkspaceDir: root})

	if first.StaticPrompt != second.StaticPrompt {
		t.Fatalf("static region changed with the clock:\nfirst=%q\nsecond=%q", first.StaticPrompt, second.StaticPrompt)
	}
	if first.DynamicTail == second.DynamicTail {
		t.Fatal("expected the dynamic tail to carry the clock change")
	}
	if !strings.HasPrefix(first.Prompt, first.StaticPrompt) {
		t.Fatal("expected Prompt to lead with StaticPrompt")
	}
	if first.Prompt != first.StaticPrompt+first.DynamicTail {
		t.Fatal("expected Prompt to be StaticPrompt+DynamicTail")
	}
	if strings.Contains(first.StaticPrompt, "Current time:") {
		t.Fatalf("expected no timestamp in the static region, got %q", first.StaticPrompt)
	}
}

// The clock still has to reach the model — just from the tail.
func TestBuildResult_KeepsCurrentTimeInTail(t *testing.T) {
	root := t.TempDir()
	withFixedTime(t, time.Date(2026, 8, 22, 10, 23, 45, 0, time.UTC))

	result := BuildResultFor(BuildOptions{WorkspaceDir: root})

	// Truncated to the minute so a burst of turns shares one prefix.
	const want = "Current time: 2026-08-22T10:23:00Z"
	if !strings.Contains(result.Prompt, want) {
		t.Fatalf("expected %q in prompt, got %q", want, result.Prompt)
	}
	if !strings.Contains(result.DynamicTail, want) {
		t.Fatalf("expected the clock in the dynamic tail, got %q", result.DynamicTail)
	}
}

func TestBuildResult_SubAgentPromptStillCarriesTime(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# AGENTS.md\n\nrules"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	withFixedTime(t, time.Date(2026, 8, 22, 10, 23, 45, 0, time.UTC))

	result := BuildResultFor(BuildOptions{WorkspaceDir: root, SubAgent: true})

	if !strings.Contains(result.Prompt, "Current time: 2026-08-22T10:23:00Z") {
		t.Fatalf("expected sub-agent prompt to carry the clock, got %q", result.Prompt)
	}
}

// Callers that cache recall must be able to replay it without re-running the
// search and without touching the static region.
func TestBuildResult_PresetRelevantReplacesSearch(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "MEMORY.md"), []byte("User prefers black coffee.\n"), 0o644); err != nil {
		t.Fatalf("write MEMORY.md: %v", err)
	}
	withFixedTime(t, time.Date(2026, 8, 22, 10, 23, 45, 0, time.UTC))

	live := BuildResultFor(BuildOptions{WorkspaceDir: root, Query: "what coffee do i prefer?"})
	if live.RelevantSection == "" {
		t.Fatal("expected the live path to produce a prior-context section")
	}

	// No Query and no searcher: the preset alone must reproduce the prompt.
	replayed := BuildResultFor(BuildOptions{
		WorkspaceDir: root,
		PresetRelevant: &PresetRelevantMemory{
			Section: live.RelevantSection,
			Items:   live.RelevantMemoryItems,
			Tokens:  live.RelevantTokens,
		},
	})

	if replayed.Prompt != live.Prompt {
		t.Fatalf("replayed prompt differs:\nlive=%q\nreplayed=%q", live.Prompt, replayed.Prompt)
	}
	if replayed.RelevantMemoryCount != live.RelevantMemoryCount {
		t.Fatalf("expected %d recalled items, got %d", live.RelevantMemoryCount, replayed.RelevantMemoryCount)
	}
}

// The tail must close the prompt: recall first, clock last, so an identical
// query re-run inside the same minute matches all the way through.
func TestBuildResult_DynamicTailOrdersRecallBeforeClock(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "MEMORY.md"), []byte("User prefers black coffee.\n"), 0o644); err != nil {
		t.Fatalf("write MEMORY.md: %v", err)
	}
	withFixedTime(t, time.Date(2026, 8, 22, 10, 23, 45, 0, time.UTC))

	result := BuildResultFor(BuildOptions{WorkspaceDir: root, Query: "what coffee do i prefer?"})

	recallAt := strings.Index(result.DynamicTail, "## Prior Context")
	clockAt := strings.Index(result.DynamicTail, "## Current Time")
	if recallAt < 0 || clockAt < 0 {
		t.Fatalf("expected both dynamic sections in the tail, got %q", result.DynamicTail)
	}
	if recallAt > clockAt {
		t.Fatalf("expected recall before the clock, got %q", result.DynamicTail)
	}
	if !strings.HasSuffix(result.Prompt, result.DynamicTail) {
		t.Fatal("expected the dynamic tail to close the prompt")
	}
}
