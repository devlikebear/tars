package tarsserver

import (
	"strings"

	"github.com/devlikebear/tars/internal/extensions"
)

// skillContent holds resolved skill name and content.
type skillContent struct {
	Name    string
	Content string
}

// skillResolver resolves skill names to their content.
type skillResolver interface {
	ResolveSkills(names []string) []skillContent
}

// extensionsSkillResolver adapts extensions.Manager into the
// skillResolver interface so that task prompts can inject skill content.
type extensionsSkillResolver struct {
	manager *extensions.Manager
}

func newExtensionsSkillResolver(m *extensions.Manager) skillResolver {
	if m == nil {
		return nil
	}
	return &extensionsSkillResolver{manager: m}
}

func (r *extensionsSkillResolver) ResolveSkills(names []string) []skillContent {
	if r == nil || r.manager == nil || len(names) == 0 {
		return nil
	}
	out := make([]skillContent, 0, len(names))
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
		out = append(out, skillContent{
			Name:    def.Name,
			Content: content,
		})
	}
	return out
}
