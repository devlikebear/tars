package tarsserver

import (
	"path/filepath"
	"strings"

	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/skill"
)

// augmentSnapshotWithCwdSkills returns a copy of base with any skills
// discovered under `<cwd>/.tars/skills` merged in. Skills carrying the same
// canonical name as one in base are replaced by the cwd-local version (cwd wins).
// A blank cwd, a missing `.tars/` directory, or an empty extensions
// snapshot all return base unchanged. Diagnostics from the skill
// loaders are appended to the snapshot's existing diagnostics so the
// /v1/chat/context preview surfaces malformed skill files without
// failing the request.
func augmentSnapshotWithCwdSkills(base extensions.Snapshot, cwd string) extensions.Snapshot {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return base
	}
	skillsDir := filepath.Join(cwd, ".tars", "skills")

	cwdSnapshot, err := skill.Load(skill.LoadOptions{
		Sources: []skill.SourceDir{{Source: skill.SourceSessionCwd, Dir: skillsDir}},
	})
	if err != nil {
		// A non-IsNotExist error is still recoverable: surface it as a
		// snapshot diagnostic and fall through with the base unchanged so
		// the chat path keeps working.
		base.Diagnostics = append(base.Diagnostics,
			"session-cwd skills load: "+err.Error())
		return base
	}
	cwdSkills := cwdSnapshot.Skills
	for _, d := range cwdSnapshot.Diagnostics {
		base.Diagnostics = append(base.Diagnostics,
			"session-cwd skill ("+d.Path+"): "+d.Message)
	}

	if len(cwdSkills) == 0 {
		return base
	}

	merged := mergeCwdSkills(base.Skills, cwdSkills)
	out := base
	out.Skills = merged
	return out
}

func loadSessionCwdCommands(cwd string) ([]skill.Definition, []string) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return nil, nil
	}
	commands, diags := skill.LoadCommands(filepath.Join(cwd, ".tars", "commands"))
	if len(diags) == 0 {
		return commands, nil
	}
	messages := make([]string, 0, len(diags))
	for _, d := range diags {
		messages = append(messages, "session-cwd command ("+d.Path+"): "+d.Message)
	}
	return commands, messages
}

// mergeCwdSkills replaces or appends cwdSkills into base, keyed by
// case-folded Name. cwd skills win on conflict (last-write-wins by source
// priority); base order is preserved for unaffected entries; new entries
// land at the end in input order.
func mergeCwdSkills(base, cwdSkills []skill.Definition) []skill.Definition {
	if len(cwdSkills) == 0 {
		out := make([]skill.Definition, len(base))
		copy(out, base)
		return out
	}
	overrides := map[string]skill.Definition{}
	cwdOrder := []string{}
	for _, d := range cwdSkills {
		key := strings.ToLower(strings.TrimSpace(d.Name))
		if key == "" {
			continue
		}
		if _, seen := overrides[key]; !seen {
			cwdOrder = append(cwdOrder, key)
		}
		overrides[key] = d
	}

	out := make([]skill.Definition, 0, len(base)+len(cwdSkills))
	consumed := map[string]struct{}{}
	for _, d := range base {
		key := strings.ToLower(strings.TrimSpace(d.Name))
		if rep, ok := overrides[key]; ok {
			out = append(out, rep)
			consumed[key] = struct{}{}
			continue
		}
		out = append(out, d)
	}
	for _, key := range cwdOrder {
		if _, used := consumed[key]; used {
			continue
		}
		out = append(out, overrides[key])
	}
	return out
}
