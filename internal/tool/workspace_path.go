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
