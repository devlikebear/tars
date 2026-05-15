package tarsserver

import (
	"strings"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/skill"
)

// toClaudeCodeSkills converts the session-effective TARS skill catalog
// (already filtered by session tool_config via filterExtensionsSnapshotForSession
// + augmentSnapshotWithCwdSkills) into the provider-agnostic shape consumed by
// claude-code-cli's --plugin-dir materialization.
//
// Skills with an empty Name or empty Content are dropped — the former would
// produce a nameless SKILL.md the provider's sanitizer rejects anyway, the
// latter a body-less skill Claude Code has nothing to act on. Description is
// optional (Claude Code tolerates a name-only frontmatter).
func toClaudeCodeSkills(skills []skill.Definition) []llm.ClaudeCodeSkill {
	if len(skills) == 0 {
		return nil
	}
	out := make([]llm.ClaudeCodeSkill, 0, len(skills))
	for _, sk := range skills {
		name := strings.TrimSpace(sk.Name)
		if name == "" {
			continue
		}
		if strings.TrimSpace(sk.Content) == "" {
			continue
		}
		out = append(out, llm.ClaudeCodeSkill{
			Name:        name,
			Description: strings.TrimSpace(sk.Description),
			Content:     sk.Content,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
