package sessionoverride

import "github.com/devlikebear/tars/internal/session"

// Merge folds (base session config + base prompt) with optional shared and
// local overrides into a single EffectiveConfig and reports, for every
// trackable path in AllPaths(), which layer last touched it.
//
// Semantics:
//   - String / scalar fields: replaced by the highest layer that set them.
//   - Slice fields inside tool_config (tools_enabled, tools_disabled,
//     tools_allow_groups, tools_deny_groups, skills_enabled, mcp_enabled):
//     union of every layer's values, dedup'd, preserving first-seen order.
//   - mcp_custom makes that layer's mcp_enabled list replace earlier MCP
//     allowlists, so an explicit empty local list can disable MCP for a session.
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
	if o.Presence["mcp_servers_extra"] {
		eff.MCPServersExtra = mergeMCPServers(eff.MCPServersExtra, o.MCPServersExtra)
		sources["mcp_servers_extra"] = src
	}
	if o.ToolConfig != nil {
		applyToolConfigLayer(&eff.ToolConfig, o, src, sources)
	}
}

func applyToolConfigLayer(dst *session.SessionToolConfig, o *Override, src Source, sources map[string]Source) {
	if o.Presence["tool_config.tools_enabled"] {
		dst.ToolsEnabled = unionDedup(dst.ToolsEnabled, o.ToolConfig.ToolsEnabled)
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
		dst.SkillsEnabled = unionDedup(dst.SkillsEnabled, o.ToolConfig.SkillsEnabled)
		sources["tool_config.skills_enabled"] = src
	}
	if o.Presence["tool_config.commands_enabled"] {
		dst.CommandsEnabled = unionDedup(dst.CommandsEnabled, o.ToolConfig.CommandsEnabled)
		sources["tool_config.commands_enabled"] = src
	}
	if o.Presence["tool_config.mcp_enabled"] {
		if o.ToolConfig.MCPCustom {
			dst.MCPEnabled = unionDedup(nil, o.ToolConfig.MCPEnabled)
		} else {
			dst.MCPEnabled = unionDedup(dst.MCPEnabled, o.ToolConfig.MCPEnabled)
		}
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
		if o.ToolConfig.MCPCustom && !o.Presence["tool_config.mcp_enabled"] {
			dst.MCPEnabled = nil
			sources["tool_config.mcp_enabled"] = src
		}
	}
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
