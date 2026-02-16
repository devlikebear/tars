package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const runtimeMirrorRoot = "_shared/skills_runtime"

func MirrorToWorkspace(workspaceDir string, snapshot Snapshot) (Snapshot, error) {
	root := filepath.Join(workspaceDir, filepath.FromSlash(runtimeMirrorRoot))
	if err := os.MkdirAll(root, 0o755); err != nil {
		return Snapshot{}, fmt.Errorf("create skill mirror root: %w", err)
	}

	keep := map[string]struct{}{}
	next := Snapshot{
		Version:     snapshot.Version,
		Skills:      append([]Definition(nil), snapshot.Skills...),
		Diagnostics: append([]Diagnostic(nil), snapshot.Diagnostics...),
	}

	for i := range next.Skills {
		slug := sanitizeSkillName(next.Skills[i].Name)
		if slug == "" {
			slug = "unknown_skill"
		}
		keep[slug] = struct{}{}
		dstDir := filepath.Join(root, slug)
		if err := os.MkdirAll(dstDir, 0o755); err != nil {
			return Snapshot{}, fmt.Errorf("create mirrored skill dir: %w", err)
		}
		content := next.Skills[i].Content
		if strings.TrimSpace(content) == "" && strings.TrimSpace(next.Skills[i].FilePath) != "" {
			data, err := os.ReadFile(next.Skills[i].FilePath)
			if err != nil {
				return Snapshot{}, fmt.Errorf("read source skill file %q: %w", next.Skills[i].FilePath, err)
			}
			content = string(data)
		}
		target := filepath.Join(dstDir, "SKILL.md")
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			return Snapshot{}, fmt.Errorf("write mirrored skill file %q: %w", target, err)
		}
		next.Skills[i].RuntimePath = filepath.ToSlash(filepath.Join(runtimeMirrorRoot, slug, "SKILL.md"))
	}

	if err := cleanupMirroredSkills(root, keep); err != nil {
		return Snapshot{}, err
	}
	return next, nil
}

func cleanupMirroredSkills(root string, keep map[string]struct{}) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read mirrored skill root: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if _, ok := keep[name]; ok {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, name)); err != nil {
			return fmt.Errorf("remove stale mirrored skill dir %q: %w", name, err)
		}
	}
	return nil
}

func sanitizeSkillName(name string) string {
	value := strings.ToLower(strings.TrimSpace(name))
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "_")
}
