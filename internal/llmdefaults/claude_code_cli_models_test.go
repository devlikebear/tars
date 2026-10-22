package llmdefaults

import (
	"slices"
	"testing"
)

func TestClaudeCodeCLIModelsHasAliasesAndNoDuplicates(t *testing.T) {
	for _, alias := range []string{"opus", "sonnet", "haiku", "claude-opus-5-5"} {
		if !slices.Contains(ClaudeCodeCLIModels, alias) {
			t.Fatalf("catalog missing %q", alias)
		}
	}
	seen := map[string]bool{}
	for _, m := range ClaudeCodeCLIModels {
		if m == "" || seen[m] {
			t.Fatalf("empty or duplicate entry %q", m)
		}
		seen[m] = true
	}
}
