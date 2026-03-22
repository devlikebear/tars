package skill

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	snapshot = filterUnavailableSkills(snapshot, opts.Availability)
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
			RequiresPlugin:          strings.TrimSpace(meta.RequiresPlugin),
			RequiresBins:            append([]string(nil), meta.RequiresBins...),
			RequiresEnv:             append([]string(nil), meta.RequiresEnv...),
			OS:                      append([]string(nil), meta.OS...),
			Arch:                    append([]string(nil), meta.Arch...),
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

func filterUnavailableSkills(snapshot Snapshot, opts AvailabilityOptions) Snapshot {
	checker := buildAvailabilityChecker(opts)
	if len(snapshot.Skills) == 0 {
		return snapshot
	}
	available := make([]Definition, 0, len(snapshot.Skills))
	for _, def := range snapshot.Skills {
		reasons := checker.unavailableReasons(def)
		if len(reasons) == 0 {
			available = append(available, def)
			continue
		}
		snapshot.Diagnostics = append(snapshot.Diagnostics, Diagnostic{
			Path:    def.FilePath,
			Message: fmt.Sprintf("skill %q unavailable: %s", def.Name, strings.Join(reasons, "; ")),
		})
	}
	snapshot.Skills = available
	return snapshot
}

type availabilityChecker struct {
	os               string
	arch             string
	installedPlugins map[string]struct{}
	hasEnv           func(string) bool
	hasCommand       func(string) bool
}

func buildAvailabilityChecker(opts AvailabilityOptions) availabilityChecker {
	checker := availabilityChecker{
		os:               strings.ToLower(strings.TrimSpace(opts.OS)),
		arch:             strings.ToLower(strings.TrimSpace(opts.Arch)),
		installedPlugins: map[string]struct{}{},
		hasEnv:           opts.HasEnv,
		hasCommand:       opts.HasCommand,
	}
	if checker.os == "" {
		checker.os = runtime.GOOS
	}
	if checker.arch == "" {
		checker.arch = runtime.GOARCH
	}
	for _, name := range opts.InstalledPlugins {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" {
			continue
		}
		checker.installedPlugins[key] = struct{}{}
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
	reasons := make([]string, 0, 5)
	if key := strings.ToLower(strings.TrimSpace(def.RequiresPlugin)); key != "" {
		if _, ok := c.installedPlugins[key]; !ok {
			reasons = append(reasons, fmt.Sprintf("missing required plugin %q", def.RequiresPlugin))
		}
	}
	for _, bin := range def.RequiresBins {
		if !c.hasCommand(bin) {
			reasons = append(reasons, fmt.Sprintf("missing required binary %q", bin))
		}
	}
	for _, key := range def.RequiresEnv {
		if !c.hasEnv(key) {
			reasons = append(reasons, fmt.Sprintf("missing required env %q", key))
		}
	}
	if !matchesPlatform(c.os, def.OS) {
		reasons = append(reasons, fmt.Sprintf("os %q not in supported set [%s]", c.os, strings.Join(def.OS, ", ")))
	}
	if !matchesPlatform(c.arch, def.Arch) {
		reasons = append(reasons, fmt.Sprintf("arch %q not in supported set [%s]", c.arch, strings.Join(def.Arch, ", ")))
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
