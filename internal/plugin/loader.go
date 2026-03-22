package plugin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const (
	manifestFilename       = "tars.plugin.json"
	legacyManifestFilename = "tarsncase.plugin.json"
)

func Load(opts LoadOptions) (Snapshot, error) {
	snapshot := Snapshot{
		Plugins: make([]Definition, 0),
	}
	if len(opts.Sources) == 0 {
		return snapshot, nil
	}

	merged := map[string]Definition{}
	for _, source := range opts.Sources {
		plugins, diagnostics, err := loadSourcePlugins(source.Source, source.Dir)
		snapshot.Diagnostics = append(snapshot.Diagnostics, diagnostics...)
		if err != nil {
			return Snapshot{}, err
		}
		for _, plugin := range plugins {
			merged[strings.ToLower(plugin.ID)] = plugin
		}
	}

	for _, plugin := range merged {
		snapshot.Plugins = append(snapshot.Plugins, plugin)
	}
	sort.Slice(snapshot.Plugins, func(i, j int) bool {
		return strings.ToLower(snapshot.Plugins[i].ID) < strings.ToLower(snapshot.Plugins[j].ID)
	})
	snapshot = filterUnavailablePlugins(snapshot, opts.Availability)

	snapshot.SkillDirs = collectSkillDirs(snapshot.Plugins, &snapshot.Diagnostics)
	snapshot.MCPServers = collectMCPServers(snapshot.Plugins)
	return snapshot, nil
}

func loadSourcePlugins(source Source, dir string) ([]Definition, []Diagnostic, error) {
	root := strings.TrimSpace(dir)
	if root == "" {
		return nil, nil, nil
	}
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("stat plugins dir %q: %w", root, err)
	}

	defsByID := map[string]Definition{}
	priorities := map[string]int{}
	diagnostics := make([]Diagnostic, 0)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Path:    path,
				Message: walkErr.Error(),
			})
			return nil
		}
		if d.IsDir() || !isManifestFilename(path) {
			return nil
		}
		manifest, err := parseManifestFile(path)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Path:    path,
				Message: err.Error(),
			})
			return nil
		}
		rootDir := filepath.Dir(path)
		key := strings.ToLower(strings.TrimSpace(manifest.ID))
		definition := Definition{
			SchemaVersion:         manifest.SchemaVersion,
			ID:                    manifest.ID,
			Name:                  manifest.Name,
			Description:           manifest.Description,
			Version:               manifest.Version,
			Source:                source,
			RootDir:               rootDir,
			ManifestPath:          path,
			Skills:                append([]string(nil), manifest.Skills...),
			MCPServers:            append([]ServerConfig(nil), manifest.MCPServers...),
			Requires:              manifest.Requires,
			SupportedOS:           append([]string(nil), manifest.SupportedOS...),
			SupportedArch:         append([]string(nil), manifest.SupportedArch...),
			DefaultProjectProfile: manifest.DefaultProjectProfile,
			Policies: Policies{
				ToolsAllow: append([]string(nil), manifest.Policies.ToolsAllow...),
				ToolsDeny:  append([]string(nil), manifest.Policies.ToolsDeny...),
			},
		}
		priority := manifestPriority(path)
		if currentPriority, ok := priorities[key]; !ok || priority >= currentPriority {
			defsByID[key] = definition
			priorities[key] = priority
		}
		return nil
	})
	if err != nil {
		return nil, diagnostics, fmt.Errorf("walk plugins dir %q: %w", root, err)
	}

	defs := make([]Definition, 0, len(defsByID))
	for _, definition := range defsByID {
		defs = append(defs, definition)
	}
	sort.Slice(defs, func(i, j int) bool {
		return strings.ToLower(defs[i].ID) < strings.ToLower(defs[j].ID)
	})
	return defs, diagnostics, nil
}

func collectSkillDirs(plugins []Definition, diagnostics *[]Diagnostic) []string {
	out := make([]string, 0)
	seen := map[string]struct{}{}
	for _, plugin := range plugins {
		rootAbs, err := filepath.Abs(plugin.RootDir)
		if err != nil {
			*diagnostics = append(*diagnostics, Diagnostic{
				Path:    plugin.ManifestPath,
				Message: fmt.Sprintf("resolve plugin root: %v", err),
			})
			continue
		}
		for _, relPath := range plugin.Skills {
			if filepath.IsAbs(relPath) {
				*diagnostics = append(*diagnostics, Diagnostic{
					Path:    plugin.ManifestPath,
					Message: fmt.Sprintf("absolute skill path is not allowed: %s", relPath),
				})
				continue
			}
			candidate := filepath.Join(plugin.RootDir, relPath)
			absPath, err := filepath.Abs(candidate)
			if err != nil {
				*diagnostics = append(*diagnostics, Diagnostic{
					Path:    plugin.ManifestPath,
					Message: fmt.Sprintf("resolve skill path %q: %v", relPath, err),
				})
				continue
			}
			if !pathWithin(absPath, rootAbs) {
				*diagnostics = append(*diagnostics, Diagnostic{
					Path:    plugin.ManifestPath,
					Message: fmt.Sprintf("skill path escapes plugin root: %s", relPath),
				})
				continue
			}
			info, err := os.Stat(absPath)
			if err != nil || !info.IsDir() {
				*diagnostics = append(*diagnostics, Diagnostic{
					Path:    plugin.ManifestPath,
					Message: fmt.Sprintf("skill dir not found: %s", relPath),
				})
				continue
			}
			if _, ok := seen[absPath]; ok {
				continue
			}
			seen[absPath] = struct{}{}
			out = append(out, absPath)
		}
	}
	return out
}

func collectMCPServers(plugins []Definition) []ServerConfig {
	out := make([]ServerConfig, 0)
	seen := map[string]struct{}{}
	for _, plugin := range plugins {
		for _, server := range plugin.MCPServers {
			name := strings.TrimSpace(server.Name)
			if name == "" {
				continue
			}
			key := strings.ToLower(name)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, server)
		}
	}
	return out
}

func filterUnavailablePlugins(snapshot Snapshot, opts AvailabilityOptions) Snapshot {
	checker := buildAvailabilityChecker(opts)
	if len(snapshot.Plugins) == 0 {
		return snapshot
	}
	available := make([]Definition, 0, len(snapshot.Plugins))
	for _, def := range snapshot.Plugins {
		reasons := checker.unavailableReasons(def)
		if len(reasons) == 0 {
			available = append(available, def)
			continue
		}
		snapshot.Diagnostics = append(snapshot.Diagnostics, Diagnostic{
			Path:    def.ManifestPath,
			Message: fmt.Sprintf("plugin %q unavailable: %s", def.ID, strings.Join(reasons, "; ")),
		})
	}
	snapshot.Plugins = available
	return snapshot
}

type availabilityChecker struct {
	os         string
	arch       string
	hasEnv     func(string) bool
	hasCommand func(string) bool
}

func buildAvailabilityChecker(opts AvailabilityOptions) availabilityChecker {
	checker := availabilityChecker{
		os:         strings.ToLower(strings.TrimSpace(opts.OS)),
		arch:       strings.ToLower(strings.TrimSpace(opts.Arch)),
		hasEnv:     opts.HasEnv,
		hasCommand: opts.HasCommand,
	}
	if checker.os == "" {
		checker.os = runtime.GOOS
	}
	if checker.arch == "" {
		checker.arch = runtime.GOARCH
	}
	if checker.hasEnv == nil {
		checker.hasEnv = func(key string) bool {
			value, ok := os.LookupEnv(strings.TrimSpace(key))
			return ok && strings.TrimSpace(value) != ""
		}
	}
	if checker.hasCommand == nil {
		checker.hasCommand = func(name string) bool {
			_, err := exec.LookPath(strings.TrimSpace(name))
			return err == nil
		}
	}
	return checker
}

func (c availabilityChecker) unavailableReasons(def Definition) []string {
	reasons := make([]string, 0, 4)
	for _, bin := range def.Requires.Bins {
		if !c.hasCommand(bin) {
			reasons = append(reasons, fmt.Sprintf("missing required binary %q", bin))
		}
	}
	for _, key := range def.Requires.Env {
		if !c.hasEnv(key) {
			reasons = append(reasons, fmt.Sprintf("missing required env %q", key))
		}
	}
	if !matchesPlatform(c.os, def.SupportedOS) {
		reasons = append(reasons, fmt.Sprintf("os %q not in supported set [%s]", c.os, strings.Join(def.SupportedOS, ", ")))
	}
	if !matchesPlatform(c.arch, def.SupportedArch) {
		reasons = append(reasons, fmt.Sprintf("arch %q not in supported set [%s]", c.arch, strings.Join(def.SupportedArch, ", ")))
	}
	return uniqueReasons(reasons)
}

func matchesPlatform(current string, supported []string) bool {
	if len(supported) == 0 {
		return true
	}
	current = strings.ToLower(strings.TrimSpace(current))
	for _, item := range supported {
		if current == strings.ToLower(strings.TrimSpace(item)) {
			return true
		}
	}
	return false
}

func uniqueReasons(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func pathWithin(path string, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if path == root {
		return true
	}
	prefix := root + string(os.PathSeparator)
	return strings.HasPrefix(path, prefix)
}

func isManifestFilename(path string) bool {
	base := filepath.Base(path)
	return strings.EqualFold(base, manifestFilename) || strings.EqualFold(base, legacyManifestFilename)
}

func manifestPriority(path string) int {
	if strings.EqualFold(filepath.Base(path), manifestFilename) {
		return 1
	}
	return 0
}
