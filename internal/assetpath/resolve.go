package assetpath

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var ExecutablePathFunc = os.Executable

func ResolveDir(raw string) string {
	if path, ok := ResolveExistingDir(raw); ok {
		return path
	}
	return strings.TrimSpace(os.ExpandEnv(raw))
}

func ResolveExistingDir(raw string) (string, bool) {
	trimmed := strings.TrimSpace(os.ExpandEnv(raw))
	if trimmed == "" {
		return "", false
	}

	candidates := bundledDirCandidates(trimmed)
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

func bundledDirCandidates(trimmed string) []string {
	candidates := make([]string, 0, 6)
	seen := map[string]struct{}{}
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if !filepath.IsAbs(path) {
			abs, err := filepath.Abs(path)
			if err == nil {
				path = abs
			}
		}
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		candidates = append(candidates, path)
	}

	if filepath.IsAbs(trimmed) {
		add(trimmed)
		return candidates
	}

	cleanedRel := filepath.Clean(trimmed)
	add(cleanedRel)
	add(filepath.Join("..", cleanedRel))

	execPaths := executableCandidates()
	base := filepath.Base(cleanedRel)
	for _, exePath := range execPaths {
		exeDir := filepath.Dir(exePath)
		add(filepath.Join(exeDir, cleanedRel))
		add(filepath.Join(filepath.Dir(exeDir), cleanedRel))
		if base != "." && base != string(filepath.Separator) {
			add(filepath.Join(exeDir, "share", "tars", base))
			add(filepath.Join(filepath.Dir(exeDir), "share", "tars", base))
		}
	}
	for _, sourceRoot := range sourceTreeRoots() {
		add(filepath.Join(sourceRoot, cleanedRel))
	}
	return candidates
}

func executableCandidates() []string {
	if ExecutablePathFunc == nil {
		return nil
	}
	exePath, err := ExecutablePathFunc()
	if err != nil {
		return nil
	}
	exePath = strings.TrimSpace(exePath)
	if exePath == "" {
		return nil
	}

	out := []string{exePath}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil && strings.TrimSpace(resolved) != "" && filepath.Clean(resolved) != filepath.Clean(exePath) {
		out = append(out, resolved)
	}
	return out
}

func sourceTreeRoots() []string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return nil
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	return []string{root}
}
