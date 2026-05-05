package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadCommandAliases walks dir for *.md alias files and returns a slice of
// Definitions that mirror the behavior of an existing skill (looked up by
// the file's `target_skill:` frontmatter) under a new invocation name.
//
// The alias name is derived from the file's basename without the .md
// suffix — e.g. `commands/refactor.md` registers `refactor` as both the
// skill `Name` and the slash command. The alias inherits the target's
// `Content` (system prompt body), `Aliases`, and runtime requirements,
// but is reported with `Source = SourceSessionCwd` so it sits at the top
// of the merge order.
//
// Files whose frontmatter is missing or whose `target_skill` does not
// match an entry in `available` are skipped with a Diagnostic; this
// keeps a malformed alias from breaking the rest of the snapshot. A
// missing dir is treated as a no-op (no error, no diagnostics).
func LoadCommandAliases(dir string, available []Definition) ([]Definition, []Diagnostic) {
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

	index := map[string]Definition{}
	for _, def := range available {
		index[strings.ToLower(strings.TrimSpace(def.Name))] = def
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
			diags = append(diags, Diagnostic{Path: path, Message: fmt.Sprintf("read alias file: %v", err)})
			return nil
		}
		meta, _, err := ParseFrontmatter(string(raw))
		if err != nil {
			diags = append(diags, Diagnostic{Path: path, Message: fmt.Sprintf("parse frontmatter: %v", err)})
			return nil
		}
		target := strings.TrimSpace(meta.TargetSkill)
		if target == "" {
			diags = append(diags, Diagnostic{Path: path, Message: "alias file is missing required `target_skill` frontmatter"})
			return nil
		}
		base, ok := index[strings.ToLower(target)]
		if !ok {
			diags = append(diags, Diagnostic{
				Path:    path,
				Message: fmt.Sprintf("alias target_skill %q not found in current snapshot; alias skipped", target),
			})
			return nil
		}
		aliasName := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		clone := base
		clone.Name = aliasName
		clone.Slash = aliasName
		clone.Source = SourceSessionCwd
		clone.FilePath = path
		clone.RuntimePath = ""
		// Allow the alias file to override description; otherwise inherit
		// the target's so the LLM still sees a meaningful summary.
		if v := strings.TrimSpace(meta.Description); v != "" {
			clone.Description = v
		}
		// Drop the prior aliases list so the slash autocomplete doesn't
		// surface unrelated invocation names from the underlying skill.
		clone.Aliases = nil
		defs = append(defs, clone)
		return nil
	})
	if walkErr != nil {
		diags = append(diags, Diagnostic{Path: root, Message: fmt.Sprintf("walk commands dir: %v", walkErr)})
	}
	return defs, diags
}
