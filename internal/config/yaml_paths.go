package config

import (
	"maps"
	"strings"
)

func preferredYAMLPathForKey(key string) string {
	key = strings.TrimSpace(strings.ToLower(key))
	if field, ok := configInputFieldByYAMLKey(key); ok && strings.TrimSpace(field.yamlPath) != "" {
		return field.yamlPath
	}
	return inferPreferredYAMLPathForKey(key)
}

func inferPreferredYAMLPathForKey(key string) string {
	key = strings.TrimSpace(strings.ToLower(key))
	switch key {
	case "workspace_dir":
		return "runtime.workspace_dir"
	case "plan_clarify_mode":
		return "runtime.plan_clarify_mode"
	case "dashboard_auth_mode":
		return "api.dashboard.auth_mode"
	case "agent_max_iterations":
		return "automation.agent.max_iterations"
	case "cron_run_history_limit":
		return "automation.cron.run_history_limit"
	case "pulse_allowed_autofixes_json":
		return "automation.pulse.allowed_autofixes"
	case "notify_command":
		return "automation.notify.command"
	case "notify_when_no_clients":
		return "automation.notify.when_no_clients"
	case "schedule_timezone":
		return "automation.schedule.timezone"
	case "usage_price_overrides_json":
		return "usage.price_overrides"
	case "mcp_command_allowlist_json":
		return "extensions.mcp.command_allowlist"
	case "mcp_servers_json":
		return "extensions.mcp.servers"
	case "telegram_bot_token":
		return "channels.telegram.bot_token"
	}

	switch {
	case strings.HasPrefix(key, "session_"):
		return "runtime.session." + strings.TrimPrefix(key, "session_")
	case strings.HasPrefix(key, "log_rotate_"):
		return "log.rotate." + strings.TrimPrefix(key, "log_rotate_")
	case strings.HasPrefix(key, "log_"):
		return "log." + strings.TrimPrefix(key, "log_")
	case strings.HasPrefix(key, "api_max_inflight_"):
		return "api.max_inflight." + strings.TrimPrefix(key, "api_max_inflight_")
	case strings.HasPrefix(key, "api_"):
		return "api." + strings.TrimPrefix(key, "api_")
	case strings.HasPrefix(key, "llm_"):
		return "llm." + strings.TrimPrefix(key, "llm_")
	case strings.HasPrefix(key, "usage_limit_"):
		return "usage.limits." + strings.TrimPrefix(key, "usage_limit_")
	case strings.HasPrefix(key, "memory_semantic_"):
		return "memory.semantic." + strings.TrimPrefix(key, "memory_semantic_")
	case strings.HasPrefix(key, "memory_embed_"):
		return "memory.embed." + strings.TrimPrefix(key, "memory_embed_")
	case strings.HasPrefix(key, "memory_"):
		return "memory." + strings.TrimPrefix(key, "memory_")
	case strings.HasPrefix(key, "pulse_"):
		return "automation.pulse." + strings.TrimPrefix(key, "pulse_")
	case strings.HasPrefix(key, "reflection_"):
		return "automation.reflection." + strings.TrimPrefix(key, "reflection_")
	case strings.HasPrefix(key, "assistant_"):
		return "assistant." + strings.TrimPrefix(key, "assistant_")
	case strings.HasPrefix(key, "compaction_"):
		return "compaction." + strings.TrimPrefix(key, "compaction_")
	case strings.HasPrefix(key, "tools_web_search_perplexity_"):
		return "tools.web_search.perplexity." + strings.TrimPrefix(key, "tools_web_search_perplexity_")
	case strings.HasPrefix(key, "tools_web_search_"):
		return "tools.web_search." + strings.TrimPrefix(key, "tools_web_search_")
	case strings.HasPrefix(key, "tools_web_fetch_"):
		trimmed := strings.TrimPrefix(key, "tools_web_fetch_")
		trimmed = strings.TrimSuffix(trimmed, "_json")
		return "tools.web_fetch." + trimmed
	case key == "tools_default_set":
		return "tools.default_set"
	case key == "tools_allow_high_risk_user":
		return "tools.allow_high_risk_user"
	case strings.HasPrefix(key, "tools_") && strings.HasSuffix(key, "_enabled"):
		name := strings.TrimSuffix(strings.TrimPrefix(key, "tools_"), "_enabled")
		return "tools." + name + ".enabled"
	case strings.HasPrefix(key, "agentruntime_agents_"):
		trimmed := strings.TrimPrefix(key, "agentruntime_agents_")
		if trimmed == "json" {
			return "agentruntime.agents.list"
		}
		return "agentruntime.agents." + strings.TrimSuffix(trimmed, "_json")
	case strings.HasPrefix(key, "agentruntime_persistence_"):
		return "agentruntime.persistence." + strings.TrimPrefix(key, "agentruntime_persistence_")
	case strings.HasPrefix(key, "agentruntime_runs_"):
		return "agentruntime.runs." + strings.TrimPrefix(key, "agentruntime_runs_")
	case strings.HasPrefix(key, "agentruntime_channels_"):
		return "agentruntime.channels." + strings.TrimPrefix(key, "agentruntime_channels_")
	case strings.HasPrefix(key, "agentruntime_subagents_"):
		return "agentruntime.subagents." + strings.TrimPrefix(key, "agentruntime_subagents_")
	case strings.HasPrefix(key, "agentruntime_consensus_"):
		trimmed := strings.TrimPrefix(key, "agentruntime_consensus_")
		return "agentruntime.consensus." + strings.TrimSuffix(trimmed, "_json")
	case strings.HasPrefix(key, "agentruntime_report_"):
		return "agentruntime.report." + strings.TrimPrefix(key, "agentruntime_report_")
	case strings.HasPrefix(key, "agentruntime_archive_"):
		return "agentruntime.archive." + strings.TrimPrefix(key, "agentruntime_archive_")
	case strings.HasPrefix(key, "agentruntime_"):
		return "agentruntime." + strings.TrimPrefix(key, "agentruntime_")
	case key == "channels_local_enabled":
		return "channels.local.enabled"
	case key == "channels_webhook_enabled":
		return "channels.webhook.enabled"
	case strings.HasPrefix(key, "channels_telegram_"):
		trimmed := strings.TrimPrefix(key, "channels_telegram_")
		trimmed = strings.ReplaceAll(trimmed, "polling_", "polling.")
		return "channels.telegram." + trimmed
	case strings.HasPrefix(key, "channels_"):
		return "channels." + strings.TrimPrefix(key, "channels_")
	case strings.HasPrefix(key, "skills_"):
		trimmed := strings.TrimPrefix(key, "skills_")
		return "extensions.skills." + strings.TrimSuffix(trimmed, "_json")
	case strings.HasPrefix(key, "plugins_"):
		trimmed := strings.TrimPrefix(key, "plugins_")
		return "extensions.plugins." + strings.TrimSuffix(trimmed, "_json")
	}

	return key
}

func preferredYAMLPathSegmentsForKey(key string) []string {
	path := preferredYAMLPathForKey(key)
	parts := strings.Split(path, ".")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func normalizeConfigUpdateKey(raw string, value any) string {
	key := strings.TrimSpace(strings.ToLower(raw))
	if _, ok := configInputFieldByYAMLKey(key); ok {
		return key
	}
	parts := strings.Split(key, ".")
	if resolved, ok := resolveConfigYAMLPath(parts, value); ok {
		return resolved
	}
	return ""
}

func normalizePatchedConfigValue(key string, value any) any {
	field, ok := configInputFieldByYAMLKey(key)
	if !ok {
		return value
	}
	var cfg Config
	field.apply(&cfg, yamlValueString(value))
	return extractValue(key, cfg)
}

func setConfigYAMLValue(dst map[string]any, key string, value any) {
	parts := preferredYAMLPathSegmentsForKey(key)
	if len(parts) == 0 {
		return
	}
	current := dst
	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return
		}
		next, ok := current[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
}

// readConfigYAMLMap navigates the existing YAML map to find the current
// value at the given config key's preferred path and returns it as a
// map[string]any. Returns nil if absent or not a map.
func readConfigYAMLMap(src map[string]any, key string) map[string]any {
	parts := preferredYAMLPathSegmentsForKey(key)
	if len(parts) == 0 {
		return nil
	}
	current := src
	for i, part := range parts {
		if i == len(parts)-1 {
			m, ok := current[part].(map[string]any)
			if !ok {
				return nil
			}
			return m
		}
		next, ok := current[part].(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}
	return nil
}

// anyToStringMap converts a value to map[string]any when possible.
// Handles map[string]any and map[string]map[string]any.
func anyToStringMap(v any) map[string]any {
	switch m := v.(type) {
	case map[string]any:
		return m
	case map[string]map[string]any:
		out := make(map[string]any, len(m))
		for k, v := range m {
			out[k] = v
		}
		return out
	default:
		return nil
	}
}

// nestedMapMerge updates dst in-place with entries from src.
// Existing entries in dst not present in src are preserved.
// For entries present in both where both values are maps, inner keys
// from src are merged into dst (existing inner keys not in src kept).
func nestedMapMerge(dst, src map[string]any) {
	for k, srcVal := range src {
		dstVal, exists := dst[k]
		if !exists {
			dst[k] = srcVal
			continue
		}
		srcInner, srcIsMap := srcVal.(map[string]any)
		dstInner, dstIsMap := dstVal.(map[string]any)
		if srcIsMap && dstIsMap {
			maps.Copy(dstInner, srcInner)
		} else {
			dst[k] = srcVal
		}
	}
}

// isAliasKeyedConfigField reports whether resolvedKey identifies a
// config field whose YAML value is a top-level "alias → fields" map
// where the editor sends the full authoritative alias set on each
// PATCH. Currently only llm_providers qualifies; tier bindings already
// flow through a typed map and bypass the merge path entirely.
func isAliasKeyedConfigField(resolvedKey string) bool {
	return resolvedKey == "llm_providers"
}

// replaceTopMergeInner authoritatively replaces dst's top-level key
// set with src's, but for keys present in both where both values are
// maps, it merges inner keys (preserving inner fields like api_key
// that the patch omits). Top-level keys present in dst but not in src
// are deleted.
//
// Used for "alias-keyed" config fields (e.g. llm_providers) where the
// editor sends the full authoritative alias map: aliases not in the
// patch should be removed from disk, but per-alias fields not in the
// patch (such as api_key when the user opts to keep the existing
// credential) should still be preserved.
func replaceTopMergeInner(dst, src map[string]any) {
	for k := range dst {
		if _, ok := src[k]; !ok {
			delete(dst, k)
		}
	}
	for k, srcVal := range src {
		dstVal, exists := dst[k]
		if !exists {
			dst[k] = srcVal
			continue
		}
		srcInner, srcIsMap := srcVal.(map[string]any)
		dstInner, dstIsMap := dstVal.(map[string]any)
		if srcIsMap && dstIsMap {
			maps.Copy(dstInner, srcInner)
		} else {
			dst[k] = srcVal
		}
	}
}

func deleteConfigYAMLRepresentations(dst map[string]any, key string) {
	deleteConfigYAMLRepresentationsFromMap(dst, nil, key)
}

func deleteConfigYAMLRepresentationsFromMap(dst map[string]any, path []string, key string) bool {
	for childKey, raw := range dst {
		childPath := append(append([]string(nil), path...), normalizeConfigYAMLPathSegment(childKey))
		if resolved, ok := resolveConfigYAMLPath(childPath, raw); ok && resolved == key {
			delete(dst, childKey)
			continue
		}
		childMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if deleteConfigYAMLRepresentationsFromMap(childMap, childPath, key) && len(childMap) == 0 {
			delete(dst, childKey)
		}
	}
	return len(dst) == 0
}
