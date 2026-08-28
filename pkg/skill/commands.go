package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadCommands walks dir for *.md command files and returns standalone,
// explicit slash commands. The command name is derived from the basename
// without the .md suffix — e.g. `commands/refactor.md` registers
// `/refactor`.
//
// Commands are intentionally not aliases for skills. Even if a legacy file
// still contains `target_skill`, the file's own body remains the command
// prompt. A missing dir is treated as a no-op (no error, no diagnostics).
func LoadCommands(dir string) ([]Definition, []Diagnostic) {
	return loadCommandFiles(dir)
}

// LoadCommandAliases is kept for older call sites. New code should prefer
// LoadCommands; command files are now standalone prompts, not skill aliases.
func LoadCommandAliases(dir string, available []Definition) ([]Definition, []Diagnostic) {
	_ = available
	return LoadCommands(dir)
}

func loadCommandFiles(dir string) ([]Definition, []Diagnostic) {
	root := strings.TrimSpace(dir)
	if root == "" {
		return nil, nil
	}
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []Diagnostic{{Path: root, Message: fmt.Sprintf("stat commands dir: %v", err)}}
	}

	var defs []Definition
	var diags []Diagnostic
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			diags = append(diags, Diagnostic{Path: path, Message: walkErr.Error()})
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			diags = append(diags, Diagnostic{Path: path, Message: fmt.Sprintf("read command file: %v", err)})
			return nil
		}
		meta, body, err := ParseFrontmatter(string(raw))
		if err != nil {
			diags = append(diags, Diagnostic{Path: path, Message: fmt.Sprintf("parse frontmatter: %v", err)})
			return nil
		}
		defs = append(defs, standaloneCommandDefinition(path, string(raw), meta, body))
		return nil
	})
	if walkErr != nil {
		diags = append(diags, Diagnostic{Path: root, Message: fmt.Sprintf("walk commands dir: %v", walkErr)})
	}
	return defs, diags
}

func standaloneCommandDefinition(path, raw string, meta Frontmatter, body string) Definition {
	name := strings.TrimSpace(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	description := strings.TrimSpace(meta.Description)
	content := strings.TrimSpace(body)
	if content == "" {
		content = strings.TrimSpace(raw)
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
	slash := normalizeSkillSlash(meta.Slash)
	if slash == "" {
		slash = name
	}
	return Definition{
		Name:                    name,
		Description:             description,
		Slash:                   slash,
		Aliases:                 normalizeSkillAliases(meta.Aliases),
		UserInvocable:           userInvocable,
		Source:                  SourceSessionCwd,
		FilePath:                path,
		RuntimePath:             path,
		RequiresPlugin:          strings.TrimSpace(meta.RequiresPlugin),
		RequiresBins:            append([]string(nil), meta.RequiresBins...),
		RequiresEnv:             append([]string(nil), meta.RequiresEnv...),
		OS:                      append([]string(nil), meta.OS...),
		Arch:                    append([]string(nil), meta.Arch...),
		RecommendedTools:        append([]string(nil), meta.RecommendedTools...),
		RecommendedProjectFiles: append([]string(nil), meta.RecommendedProjectFiles...),
		WakePhases:              append([]string(nil), meta.WakePhases...),
		Tags:                    append([]string(nil), meta.Tags...),
		SmokeTests:              append([]string(nil), meta.SmokeTests...),
		Content:                 content,
	}
}
