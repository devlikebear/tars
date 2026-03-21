package skill

import (
	"fmt"
	"io"
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

		// Copy companion files (scripts, configs, etc.) from the source directory.
		srcDir := filepath.Dir(next.Skills[i].FilePath)
		if strings.TrimSpace(srcDir) != "" && srcDir != "." {
			_ = copyCompanionFiles(srcDir, dstDir)
		}

		next.Skills[i].RuntimePath = filepath.ToSlash(filepath.Join(runtimeMirrorRoot, slug, "SKILL.md"))
	}

	if err := cleanupMirroredSkills(root, keep); err != nil {
		return Snapshot{}, err
	}
	return next, nil
}

// copyCompanionFiles copies all non-SKILL.md files from srcDir to dstDir,
// preserving subdirectory structure. This allows scripts, configs, and other
// reference files to be available at runtime alongside the skill.
func copyCompanionFiles(srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil || rel == "." {
			return nil
		}
		dst := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		if strings.EqualFold(filepath.Base(path), "SKILL.md") {
			return nil // already written above
		}
		return copyFile(path, dst)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	// Preserve executable bit if source is executable.
	info, err := in.Stat()
	if err == nil && info.Mode()&0o111 != 0 {
		_ = out.Chmod(info.Mode())
	}
	return nil
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
