package sessionoverride

import "github.com/devlikebear/tars/internal/session"

// Merge folds (base session config + base prompt) with optional shared and
// local overrides into a single EffectiveConfig and reports, for every
// trackable path in AllPaths(), which layer last touched it.
//
// Semantics:
//   - String / scalar fields: replaced by the highest layer that set them.
//   - Slice fields inside tool_config (tools_enabled, tools_disabled,
//     tools_allow_groups, tools_deny_groups, skills_enabled, commands_enabled,
//     mcp_enabled):
//     union of every layer's values, dedup'd, preserving first-seen order.
//   - tools_custom, skills_custom, commands_custom, and mcp_custom make that
//     layer's corresponding allowlist replace earlier entries. If the custom
//     flag is explicitly true and the allowlist is omitted, inherited entries
//     are cleared.
//   - mcp_servers_extra: merged by Name, later layers replacing earlier
//     entries with the same name; new names append.
func Merge(base session.SessionToolConfig, basePrompt string, shared, local *Override) (EffectiveConfig, map[string]Source) {
	sources := map[string]Source{}
	for _, p := range AllPaths() {
		sources[p] = SourceBase
	}

	eff := EffectiveConfig{
		ToolConfig:     base,
		PromptOverride: basePrompt,
	}

	// Track scalar overrides in priority order: shared then local.
	for _, layer := range []struct {
		o   *Override
		src Source
	}{{shared, SourceShared}, {local, SourceLocal}} {
		if layer.o == nil {
			continue
		}
		applyLayer(&eff, layer.o, layer.src, sources)
	}

	return eff, sources
}

func applyLayer(eff *EffectiveConfig, o *Override, src Source, sources map[string]Source) {
	if o.PromptOverride != nil && o.Presence["prompt_override"] {
		eff.PromptOverride = *o.PromptOverride
		sources["prompt_override"] = src
	}
	if o.ModelTierOverride != nil && o.Presence["model_tier_override"] {
		eff.ModelTierOverride = *o.ModelTierOverride
		sources["model_tier_override"] = src
	}
	if o.ClaudeCodeCLIPermissionMode != nil && o.Presence["claude_code_cli_permission_mode"] {
		eff.ClaudeCodeCLIPermissionMode = *o.ClaudeCodeCLIPermissionMode
		sources["claude_code_cli_permission_mode"] = src
	}
	if o.Presence["mcp_servers_extra"] {
		eff.MCPServersExtra = mergeMCPServers(eff.MCPServersExtra, o.MCPServersExtra)
		sources["mcp_servers_extra"] = src
	}
	if o.ToolConfig != nil {
		applyToolConfigLayer(&eff.ToolConfig, o, src, sources)
	}
}

func applyToolConfigLayer(dst *session.SessionToolConfig, o *Override, src Source, sources map[string]Source) {
	toolsCustom := o.Presence["tool_config.tools_custom"] && o.ToolConfig.ToolsCustom
	skillsCustom := o.Presence["tool_config.skills_custom"] && o.ToolConfig.SkillsCustom
	commandsCustom := o.Presence["tool_config.commands_custom"] && o.ToolConfig.CommandsCustom
	mcpCustom := o.Presence["tool_config.mcp_custom"] && o.ToolConfig.MCPCustom

	if o.Presence["tool_config.tools_enabled"] {
		dst.ToolsEnabled = mergeAllowlist(dst.ToolsEnabled, o.ToolConfig.ToolsEnabled, toolsCustom)
		sources["tool_config.tools_enabled"] = src
	} else if toolsCustom {
		dst.ToolsEnabled = nil
		sources["tool_config.tools_enabled"] = src
	}
	if o.Presence["tool_config.tools_disabled"] {
		dst.ToolsDisabled = unionDedup(dst.ToolsDisabled, o.ToolConfig.ToolsDisabled)
		sources["tool_config.tools_disabled"] = src
	}
	if o.Presence["tool_config.tools_allow_groups"] {
		dst.ToolsAllowGroups = unionDedup(dst.ToolsAllowGroups, o.ToolConfig.ToolsAllowGroups)
		sources["tool_config.tools_allow_groups"] = src
	}
	if o.Presence["tool_config.tools_deny_groups"] {
		dst.ToolsDenyGroups = unionDedup(dst.ToolsDenyGroups, o.ToolConfig.ToolsDenyGroups)
		sources["tool_config.tools_deny_groups"] = src
	}
	if o.Presence["tool_config.skills_enabled"] {
		dst.SkillsEnabled = mergeAllowlist(dst.SkillsEnabled, o.ToolConfig.SkillsEnabled, skillsCustom)
		sources["tool_config.skills_enabled"] = src
	} else if skillsCustom {
		dst.SkillsEnabled = nil
		sources["tool_config.skills_enabled"] = src
	}
	if o.Presence["tool_config.commands_enabled"] {
		dst.CommandsEnabled = mergeAllowlist(dst.CommandsEnabled, o.ToolConfig.CommandsEnabled, commandsCustom)
		sources["tool_config.commands_enabled"] = src
	} else if commandsCustom {
		dst.CommandsEnabled = nil
		sources["tool_config.commands_enabled"] = src
	}
	if o.Presence["tool_config.mcp_enabled"] {
		dst.MCPEnabled = mergeAllowlist(dst.MCPEnabled, o.ToolConfig.MCPEnabled, mcpCustom)
		sources["tool_config.mcp_enabled"] = src
	} else if mcpCustom {
		dst.MCPEnabled = nil
		sources["tool_config.mcp_enabled"] = src
	}
	if o.Presence["tool_config.tools_custom"] {
		dst.ToolsCustom = o.ToolConfig.ToolsCustom
		sources["tool_config.tools_custom"] = src
	}
	if o.Presence["tool_config.skills_custom"] {
		dst.SkillsCustom = o.ToolConfig.SkillsCustom
		sources["tool_config.skills_custom"] = src
	}
	if o.Presence["tool_config.commands_custom"] {
		dst.CommandsCustom = o.ToolConfig.CommandsCustom
		sources["tool_config.commands_custom"] = src
	}
	if o.Presence["tool_config.mcp_custom"] {
		dst.MCPCustom = o.ToolConfig.MCPCustom
		sources["tool_config.mcp_custom"] = src
	}
}

func mergeAllowlist(prev, next []string, replace bool) []string {
	if replace {
		return unionDedup(nil, next)
	}
	return unionDedup(prev, next)
}

// unionDedup returns a new slice containing every element from a and b in
// first-seen order, without duplicates.
func unionDedup(a, b []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(a)+len(b))
	push := func(values []string) {
		for _, v := range values {
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	push(a)
	push(b)
	return out
}

// mergeMCPServers merges by Name: entries in `next` with a name already in
// `prev` replace the prior entry; new names append. Order: prior entries
// first (in their original order, with replacements in place), then any
// brand-new names from `next` in their input order.
func mergeMCPServers(prev, next []MCPServerExtra) []MCPServerExtra {
	if len(next) == 0 {
		out := make([]MCPServerExtra, len(prev))
		copy(out, prev)
		return out
	}
	// Index next by name for replacement lookup.
	replacements := map[string]MCPServerExtra{}
	order := []string{}
	seenInPrev := map[string]struct{}{}
	for _, e := range next {
		if _, ok := replacements[e.Name]; !ok {
			order = append(order, e.Name)
		}
		replacements[e.Name] = e
	}
	out := make([]MCPServerExtra, 0, len(prev)+len(next))
	for _, e := range prev {
		if rep, ok := replacements[e.Name]; ok {
			out = append(out, rep)
		} else {
			out = append(out, e)
		}
		seenInPrev[e.Name] = struct{}{}
	}
	for _, name := range order {
		if _, ok := seenInPrev[name]; ok {
			continue
		}
		out = append(out, replacements[name])
	}
	return out
}
