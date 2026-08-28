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
		Skills:      make([]Definition, 0, len(snapshot.Skills)),
		Diagnostics: append([]Diagnostic(nil), snapshot.Diagnostics...),
	}

	for i := range snapshot.Skills {
		def := snapshot.Skills[i]
		slug := sanitizeSkillName(def.Name)
		if slug == "" {
			slug = "unknown_skill"
		}
		dstDir := filepath.Join(root, slug)
		if err := os.MkdirAll(dstDir, 0o755); err != nil {
			return Snapshot{}, fmt.Errorf("create mirrored skill dir: %w", err)
		}
		content := def.Content
		if strings.TrimSpace(content) == "" && strings.TrimSpace(def.FilePath) != "" {
			data, err := os.ReadFile(def.FilePath)
			if err != nil {
				return Snapshot{}, fmt.Errorf("read source skill file %q: %w", def.FilePath, err)
			}
			content = string(data)
		}
		target := filepath.Join(dstDir, "SKILL.md")
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			return Snapshot{}, fmt.Errorf("write mirrored skill file %q: %w", target, err)
		}

		// Copy companion files (scripts, configs, etc.) from the source directory.
		srcDir := filepath.Dir(def.FilePath)
		if strings.TrimSpace(srcDir) != "" && srcDir != "." {
			if err := copyCompanionFiles(srcDir, dstDir); err != nil {
				next.Diagnostics = append(next.Diagnostics, Diagnostic{
					Path:    def.FilePath,
					Message: fmt.Sprintf("mirror companion files: %v", err),
				})
				if removeErr := os.RemoveAll(dstDir); removeErr != nil {
					next.Diagnostics = append(next.Diagnostics, Diagnostic{
						Path:    target,
						Message: fmt.Sprintf("remove failed mirrored skill dir: %v", removeErr),
					})
				}
				continue
			}
		}

		def.RuntimePath = filepath.ToSlash(filepath.Join(runtimeMirrorRoot, slug, "SKILL.md"))
		next.Skills = append(next.Skills, def)
		keep[slug] = struct{}{}
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
			return fmt.Errorf("walk companion path %q: %w", path, walkErr)
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("resolve companion path %q: %w", path, err)
		}
		if rel == "." {
			return nil
		}
		dst := filepath.Join(dstDir, rel)
		if d.IsDir() {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return fmt.Errorf("create companion directory %q from %q: %w", dst, path, err)
			}
			return nil
		}
		if strings.EqualFold(filepath.Base(path), "SKILL.md") {
			return nil // already written above
		}
		if err := copyFile(path, dst); err != nil {
			return fmt.Errorf("copy companion file %q to %q: %w", path, dst, err)
		}
		return nil
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
