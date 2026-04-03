package project

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/devlikebear/tars/internal/skill"
)

// loadSourcePathSkills loads all SKILL.md files from {sourcePath}/skills/.
func loadSourcePathSkills(sourcePath string) []SkillContent {
	skillsDir := filepath.Join(sourcePath, "skills")
	if _, err := os.Stat(skillsDir); err != nil {
		return nil
	}
	snapshot, err := skill.Load(skill.LoadOptions{
		Sources: []skill.SourceDir{{Source: skill.SourceWorkspace, Dir: skillsDir}},
	})
	if err != nil {
		return nil
	}
	out := make([]SkillContent, 0, len(snapshot.Skills))
	for _, def := range snapshot.Skills {
		if c := strings.TrimSpace(def.Content); c != "" {
			out = append(out, SkillContent{Name: def.Name, Content: c})
		}
	}
	return out
}
