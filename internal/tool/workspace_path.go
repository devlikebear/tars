package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func resolveWorkspacePath(workspaceDir, rawPath string) (string, error) {
	_, rootCanonical, err := resolveWorkspaceRoot(workspaceDir)
	if err != nil {
		return "", err
	}

	candidateAbs, err := resolveWorkspaceCandidatePath(rootCanonical, rawPath)
	if err != nil {
		return "", err
	}
	if !pathWithinWorkspace(rootCanonical, candidateAbs) {
		return "", fmt.Errorf("path escapes workspace: %s", rawPath)
	}

	if resolved, err := filepath.EvalSymlinks(candidateAbs); err == nil {
		if !pathWithinWorkspace(rootCanonical, resolved) {
			return "", fmt.Errorf("resolved path escapes workspace: %s", rawPath)
		}
		return resolved, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("resolve symlink: %w", err)
	}

	return candidateAbs, nil
}

func resolveWorkspaceWritePath(workspaceDir, rawPath string) (string, error) {
	_, rootCanonical, err := resolveWorkspaceRoot(workspaceDir)
	if err != nil {
		return "", err
	}

	candidateAbs, err := resolveWorkspaceCandidatePath(rootCanonical, rawPath)
	if err != nil {
		return "", err
	}
	if !pathWithinWorkspace(rootCanonical, candidateAbs) {
		return "", fmt.Errorf("path escapes workspace: %s", rawPath)
	}

	current := candidateAbs
	for {
		_, err := os.Lstat(current)
		if err == nil {
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", fmt.Errorf("resolve symlink: %w", err)
			}
			if !pathWithinWorkspace(rootCanonical, resolved) {
				return "", fmt.Errorf("resolved path escapes workspace: %s", rawPath)
			}
			if current == candidateAbs {
				return resolved, nil
			}
			rel, err := filepath.Rel(current, candidateAbs)
			if err != nil {
				return "", fmt.Errorf("resolve target path: %w", err)
			}
			target := filepath.Clean(filepath.Join(resolved, rel))
			if !pathWithinWorkspace(rootCanonical, target) {
				return "", fmt.Errorf("resolved path escapes workspace: %s", rawPath)
			}
			return target, nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("resolve symlink: %w", err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	return candidateAbs, nil
}

func resolveWorkspaceRoot(workspaceDir string) (string, string, error) {
	rootAbs, err := filepath.Abs(workspaceDir)
	if err != nil {
		return "", "", fmt.Errorf("resolve workspace path: %w", err)
	}
	rootCanonical := rootAbs
	if resolved, err := filepath.EvalSymlinks(rootAbs); err == nil {
		rootCanonical = resolved
	}
	return rootAbs, rootCanonical, nil
}

func resolveWorkspaceCandidatePath(rootCanonical, rawPath string) (string, error) {
	candidate := strings.TrimSpace(rawPath)
	if candidate == "" {
		candidate = "."
	}
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(rootCanonical, candidate)
	}
	candidateAbs, err := filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return "", fmt.Errorf("resolve target path: %w", err)
	}
	return candidateAbs, nil
}

func pathWithinWorkspace(rootAbs, pathAbs string) bool {
	rootClean := filepath.Clean(rootAbs)
	pathClean := filepath.Clean(pathAbs)
	if pathClean == rootClean {
		return true
	}
	return strings.HasPrefix(pathClean, rootClean+string(filepath.Separator))
}

// PathPolicy controls which directories file I/O tools can access.
type PathPolicy struct {
	PrimaryDir  string   // base for relative path resolution
	AllowedDirs []string // all directories the tool can access (absolute paths)
}

// NewPathPolicy builds a PathPolicy. When currentDir is set, it becomes PrimaryDir;
// otherwise workspaceDir is used. AllowedDirs includes workspaceDir and all workDirs.
func NewPathPolicy(workspaceDir string, workDirs []string, currentDir string) PathPolicy {
	primary := workspaceDir
	if strings.TrimSpace(currentDir) != "" {
		primary = currentDir
	}
	seen := map[string]struct{}{}
	var allowed []string
	addDir := func(d string) {
		d = strings.TrimSpace(d)
		if d == "" {
			return
		}
		abs, err := filepath.Abs(d)
		if err != nil {
			return
		}
		if _, ok := seen[abs]; ok {
			return
		}
		seen[abs] = struct{}{}
		allowed = append(allowed, abs)
	}
	addDir(workspaceDir)
	for _, d := range workDirs {
		addDir(d)
	}
	return PathPolicy{PrimaryDir: primary, AllowedDirs: allowed}
}

// SingleDirPolicy creates a PathPolicy equivalent to the old single-workspaceDir behavior.
func SingleDirPolicy(workspaceDir string) PathPolicy {
	abs, _ := filepath.Abs(workspaceDir)
	return PathPolicy{PrimaryDir: workspaceDir, AllowedDirs: []string{abs}}
}

// resolvePathWithPolicy resolves a path against a PathPolicy.
// Relative paths resolve against PrimaryDir.
// Absolute paths are allowed if within any AllowedDir.
func resolvePathWithPolicy(policy PathPolicy, rawPath string) (string, error) {
	candidate := strings.TrimSpace(rawPath)
	if candidate == "" {
		candidate = "."
	}
	if !filepath.IsAbs(candidate) {
		return resolveWorkspacePath(policy.PrimaryDir, candidate)
	}
	candidateAbs, err := filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return "", fmt.Errorf("resolve target path: %w", err)
	}
	for _, dir := range policy.AllowedDirs {
		rootAbs, rootCanonical, err := resolveWorkspaceRoot(dir)
		if err != nil {
			continue
		}
		if pathWithinWorkspace(rootAbs, candidateAbs) || pathWithinWorkspace(rootCanonical, candidateAbs) {
			resolved, err := filepath.EvalSymlinks(candidateAbs)
			if err == nil {
				return resolved, nil
			}
			if os.IsNotExist(err) {
				return candidateAbs, nil
			}
			return "", fmt.Errorf("resolve symlink: %w", err)
		}
	}
	return "", fmt.Errorf("path outside allowed directories: %s", rawPath)
}

// resolveWritePathWithPolicy is the write-safe variant.
func resolveWritePathWithPolicy(policy PathPolicy, rawPath string) (string, error) {
	candidate := strings.TrimSpace(rawPath)
	if candidate == "" {
		return "", fmt.Errorf("path is required")
	}
	if !filepath.IsAbs(candidate) {
		return resolveWorkspaceWritePath(policy.PrimaryDir, candidate)
	}
	candidateAbs, err := filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return "", fmt.Errorf("resolve target path: %w", err)
	}
	for _, dir := range policy.AllowedDirs {
		rootAbs, _, err := resolveWorkspaceRoot(dir)
		if err != nil {
			continue
		}
		if pathWithinWorkspace(rootAbs, candidateAbs) {
			return resolveWorkspaceWritePath(dir, candidate)
		}
	}
	return "", fmt.Errorf("path outside allowed directories: %s", rawPath)
}

// policyRelativePath returns a display-friendly path.
// If within PrimaryDir, returns relative. Otherwise returns absolute.
func policyRelativePath(policy PathPolicy, absPath string) string {
	rel := workspaceRelativePath(policy.PrimaryDir, absPath)
	if !filepath.IsAbs(rel) && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return absPath
}

func workspaceRelativePath(workspaceDir, absPath string) string {
	_, rootCanonical, err := resolveWorkspaceRoot(workspaceDir)
	if err != nil {
		return filepath.ToSlash(absPath)
	}
	pathCanonical := filepath.Clean(absPath)
	if resolved, err := filepath.EvalSymlinks(pathCanonical); err == nil {
		pathCanonical = resolved
	}

	rel, err := filepath.Rel(rootCanonical, pathCanonical)
	if err != nil {
		return filepath.ToSlash(absPath)
	}
	return filepath.ToSlash(rel)
}
