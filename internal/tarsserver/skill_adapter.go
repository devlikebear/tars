package tarsserver

import (
	"strings"

	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/project"
)

// extensionsSkillResolver adapts extensions.Manager into the
// project.SkillResolver interface so that the orchestrator can inject
// project-scoped skill content into task prompts.
type extensionsSkillResolver struct {
	manager *extensions.Manager
}

func newExtensionsSkillResolver(m *extensions.Manager) project.SkillResolver {
	if m == nil {
		return nil
	}
	return &extensionsSkillResolver{manager: m}
}

func (r *extensionsSkillResolver) ResolveSkills(names []string) []project.SkillContent {
	if r == nil || r.manager == nil || len(names) == 0 {
		return nil
	}
	out := make([]project.SkillContent, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		def, ok := r.manager.FindSkill(name)
		if !ok {
			continue
		}
		content := strings.TrimSpace(def.Content)
		if content == "" {
			continue
		}
		out = append(out, project.SkillContent{
			Name:    def.Name,
			Content: content,
		})
	}
	return out
}
