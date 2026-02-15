package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func resolveWorkspacePath(workspaceDir, rawPath string) (string, error) {
	rootAbs, err := filepath.Abs(workspaceDir)
	if err != nil {
		return "", fmt.Errorf("resolve workspace path: %w", err)
	}
	rootCanonical := rootAbs
	if resolved, err := filepath.EvalSymlinks(rootAbs); err == nil {
		rootCanonical = resolved
	}

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

func pathWithinWorkspace(rootAbs, pathAbs string) bool {
	rootClean := filepath.Clean(rootAbs)
	pathClean := filepath.Clean(pathAbs)
	if pathClean == rootClean {
		return true
	}
	return strings.HasPrefix(pathClean, rootClean+string(filepath.Separator))
}

func workspaceRelativePath(workspaceDir, absPath string) string {
	rootAbs, err := filepath.Abs(workspaceDir)
	if err != nil {
		return filepath.ToSlash(absPath)
	}
	rootCanonical := rootAbs
	if resolved, err := filepath.EvalSymlinks(rootAbs); err == nil {
		rootCanonical = resolved
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
