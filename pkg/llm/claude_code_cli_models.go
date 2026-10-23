package llm

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/devlikebear/tars/internal/llmdefaults"
)

// The claude CLI has no command that lists models, so a picker for
// claude-code-cli has to assemble one. These are the pieces, shared by the
// TARS server and any app that drives the CLI through this package: the
// built-in catalog, the options the CLI itself cached, a filter for a live
// Anthropic Models API list, and the merge that orders them.

// maxClaudeCodeCLIConfigBytes bounds how much of ~/.claude.json is read; the
// file only grows with usage history, and the model cache sits in it as plain
// JSON.
const maxClaudeCodeCLIConfigBytes = 32 << 20

// ClaudeCodeCLIModelCatalog returns the built-in model list for
// claude-code-cli, the last fallback behind a live list and the CLI's own
// cache. The aliases (opus, sonnet, haiku) come first: the CLI maps them to
// its current model, so they never go stale. The full ids let a caller pin an
// exact model (`claude --model claude-opus-5-5`). The result is a copy.
func ClaudeCodeCLIModelCatalog() []string {
	return slices.Clone(llmdefaults.ClaudeCodeCLIModels)
}

// ReadClaudeCodeCLIModelCache returns the model ids the Claude Code CLI cached
// in its own config file, $CLAUDE_CONFIG_DIR/.claude.json or ~/.claude.json.
// Only `additionalModelOptionsCache[].value` is read, never anything else in
// the file (it also holds OAuth account data). A missing, oversized or
// malformed file yields nil.
func ReadClaudeCodeCLIModelCache() []string {
	dir := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR"))
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		dir = home
	}
	f, err := os.Open(filepath.Join(dir, ".claude.json"))
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxClaudeCodeCLIConfigBytes+1))
	if err != nil || len(data) > maxClaudeCodeCLIConfigBytes {
		return nil
	}
	return parseClaudeCodeCLIModelCache(data)
}

func parseClaudeCodeCLIModelCache(data []byte) []string {
	var doc struct {
		Options []struct {
			Value string `json:"value"`
		} `json:"additionalModelOptionsCache"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return nil
	}
	var out []string
	for _, o := range doc.Options {
		v := strings.TrimSpace(o.Value)
		// "claude-fable-5-1[1m]": the suffix selects a context window; the
		// bare id is what the picker should hold.
		if i := strings.Index(v, "["); i >= 0 {
			v = strings.TrimSpace(v[:i])
		}
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// ClaudeModelIDs keeps the claude- ids of a model list, such as the one the
// Anthropic Models API returns, so it can feed MergeClaudeCodeCLIModels.
func ClaudeModelIDs(ids []string) []string {
	var out []string
	for _, m := range ids {
		if m = strings.TrimSpace(m); strings.HasPrefix(m, "claude-") {
			out = append(out, m)
		}
	}
	return out
}

// MergeClaudeCodeCLIModels joins model lists without duplicates or blanks.
// Aliases (no "claude-" prefix) come first because the CLI maps them to its
// newest model, then the full ids in source order — so pass the sources
// freshest first.
func MergeClaudeCodeCLIModels(sources ...[]string) []string {
	seen := map[string]bool{}
	var aliases, full []string
	for _, src := range sources {
		for _, m := range src {
			m = strings.TrimSpace(m)
			if m == "" || seen[m] {
				continue
			}
			seen[m] = true
			if strings.HasPrefix(m, "claude-") {
				full = append(full, m)
			} else {
				aliases = append(aliases, m)
			}
		}
	}
	return append(aliases, full...)
}
