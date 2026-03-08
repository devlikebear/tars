package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func Load(opts LoadOptions) (Snapshot, error) {
	snapshot := Snapshot{
		Skills: make([]Definition, 0),
	}
	if len(opts.Sources) == 0 {
		return snapshot, nil
	}

	merged := map[string]Definition{}
	for _, source := range opts.Sources {
		sourceDefs, diagnostics, err := loadSourceSkills(source.Source, source.Dir)
		snapshot.Diagnostics = append(snapshot.Diagnostics, diagnostics...)
		if err != nil {
			return Snapshot{}, err
		}
		for _, def := range sourceDefs {
			key := canonicalSkillKey(def.Name)
			if key == "" {
				continue
			}
			merged[key] = def
		}
	}

	snapshot.Skills = make([]Definition, 0, len(merged))
	for _, def := range merged {
		snapshot.Skills = append(snapshot.Skills, def)
	}
	sort.Slice(snapshot.Skills, func(i, j int) bool {
		return strings.ToLower(snapshot.Skills[i].Name) < strings.ToLower(snapshot.Skills[j].Name)
	})
	return snapshot, nil
}

func loadSourceSkills(source Source, dir string) ([]Definition, []Diagnostic, error) {
	root := strings.TrimSpace(dir)
	if root == "" {
		return nil, nil, nil
	}
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("stat skills dir %q: %w", root, err)
	}

	defs := make([]Definition, 0)
	diagnostics := make([]Diagnostic, 0)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Path:    path,
				Message: walkErr.Error(),
			})
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Base(path), "SKILL.md") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Path:    path,
				Message: fmt.Sprintf("read skill file: %v", err),
			})
			return nil
		}
		raw := string(data)
		meta, body, err := ParseFrontmatter(raw)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Path:    path,
				Message: fmt.Sprintf("parse frontmatter: %v", err),
			})
			return nil
		}

		name := strings.TrimSpace(meta.Name)
		if name == "" {
			name = strings.TrimSpace(filepath.Base(filepath.Dir(path)))
		}
		if name == "" {
			name = "unknown_skill"
		}

		description := strings.TrimSpace(meta.Description)
		content := body
		if content == "" {
			content = raw
		}
		if description == "" {
			description = inferDescription(content)
		}
		if description == "" {
			description = "No description provided."
		}

		userInvocable := true
		if meta.UserInvocable != nil {
			userInvocable = *meta.UserInvocable
		}

		defs = append(defs, Definition{
			Name:                    name,
			Description:             description,
			UserInvocable:           userInvocable,
			Source:                  source,
			FilePath:                path,
			RecommendedTools:        append([]string(nil), meta.RecommendedTools...),
			RecommendedProjectFiles: append([]string(nil), meta.RecommendedProjectFiles...),
			WakePhases:              append([]string(nil), meta.WakePhases...),
			Content:                 content,
		})
		return nil
	})
	if err != nil {
		return nil, diagnostics, fmt.Errorf("walk skills dir %q: %w", root, err)
	}

	return defs, diagnostics, nil
}

func inferDescription(content string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		trimmed = strings.TrimLeft(trimmed, "#")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == "" {
			continue
		}
		if len(trimmed) > 140 {
			return trimmed[:140] + "..."
		}
		return trimmed
	}
	return ""
}

func canonicalSkillKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
