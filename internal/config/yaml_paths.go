package config

import "strings"

func preferredYAMLPathForKey(key string) string {
	key = strings.TrimSpace(strings.ToLower(key))
	switch key {
	case "workspace_dir":
		return "runtime.workspace_dir"
	case "plan_clarify_mode":
		return "runtime.plan_clarify_mode"
	case "session_default_id":
		return "runtime.session.default_id"
	case "session_telegram_scope":
		return "runtime.session.telegram_scope"
	case "log_level":
		return "log.level"
	case "log_file":
		return "log.file"
	case "log_rotate_max_size_mb":
		return "log.rotate.max_size_mb"
	case "log_rotate_max_days":
		return "log.rotate.max_days"
	case "log_rotate_max_backups":
		return "log.rotate.max_backups"
	case "dashboard_auth_mode":
		return "api.dashboard.auth_mode"
	case "api_max_inflight_chat":
		return "api.max_inflight.chat"
	case "api_max_inflight_agent_runs":
		return "api.max_inflight.agent_runs"
	case "llm_providers":
		return "llm.providers"
	case "llm_tiers":
		return "llm.tiers"
	case "llm_default_tier":
		return "llm.default_tier"
	case "llm_role_defaults":
		return "llm.role_defaults"
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
	case "usage_limit_daily_usd":
		return "usage.limits.daily_usd"
	case "usage_limit_weekly_usd":
		return "usage.limits.weekly_usd"
	case "usage_limit_monthly_usd":
		return "usage.limits.monthly_usd"
	case "usage_limit_mode":
		return "usage.limits.mode"
	case "usage_price_overrides_json":
		return "usage.price_overrides"
	case "memory_backend":
		return "memory.backend"
	case "memory_semantic_enabled":
		return "memory.semantic.enabled"
	case "memory_embed_provider":
		return "memory.embed.provider"
	case "memory_embed_base_url":
		return "memory.embed.base_url"
	case "memory_embed_api_key":
		return "memory.embed.api_key"
	case "memory_embed_model":
		return "memory.embed.model"
	case "memory_embed_dimensions":
		return "memory.embed.dimensions"
	case "assistant_enabled":
		return "assistant.enabled"
	case "assistant_hotkey":
		return "assistant.hotkey"
	case "assistant_whisper_bin":
		return "assistant.whisper_bin"
	case "assistant_ffmpeg_bin":
		return "assistant.ffmpeg_bin"
	case "assistant_tts_bin":
		return "assistant.tts_bin"
	case "compaction_trigger_tokens":
		return "compaction.trigger_tokens"
	case "compaction_keep_recent_tokens":
		return "compaction.keep_recent_tokens"
	case "compaction_keep_recent_fraction":
		return "compaction.keep_recent_fraction"
	case "compaction_llm_mode":
		return "compaction.llm_mode"
	case "compaction_llm_timeout_seconds":
		return "compaction.llm_timeout_seconds"
	case "mcp_command_allowlist_json":
		return "extensions.mcp.command_allowlist"
	case "mcp_servers_json":
		return "extensions.mcp.servers"
	case "agentruntime_enabled":
		return "agentruntime.enabled"
	case "agentruntime_default_agent":
		return "agentruntime.default_agent"
	case "agentruntime_task_override":
		return "agentruntime.task_override"
	case "agentruntime_agents_json":
		return "agentruntime.agents.list"
	case "agentruntime_agents_watch":
		return "agentruntime.agents.watch"
	case "agentruntime_agents_watch_debounce_ms":
		return "agentruntime.agents.watch_debounce_ms"
	case "agentruntime_persistence_enabled":
		return "agentruntime.persistence.enabled"
	case "agentruntime_persistence_dir":
		return "agentruntime.persistence.dir"
	case "agentruntime_runs_persistence_enabled":
		return "agentruntime.runs.persistence_enabled"
	case "agentruntime_runs_max_records":
		return "agentruntime.runs.max_records"
	case "agentruntime_channels_persistence_enabled":
		return "agentruntime.channels.persistence_enabled"
	case "agentruntime_channels_max_messages_per_channel":
		return "agentruntime.channels.max_messages_per_channel"
	case "agentruntime_subagents_max_threads":
		return "agentruntime.subagents.max_threads"
	case "agentruntime_subagents_max_depth":
		return "agentruntime.subagents.max_depth"
	case "agentruntime_consensus_enabled":
		return "agentruntime.consensus.enabled"
	case "agentruntime_consensus_max_fanout":
		return "agentruntime.consensus.max_fanout"
	case "agentruntime_consensus_budget_tokens":
		return "agentruntime.consensus.budget_tokens"
	case "agentruntime_consensus_budget_usd":
		return "agentruntime.consensus.budget_usd"
	case "agentruntime_consensus_timeout_seconds":
		return "agentruntime.consensus.timeout_seconds"
	case "agentruntime_consensus_allowed_aliases_json":
		return "agentruntime.consensus.allowed_aliases"
	case "agentruntime_consensus_concurrent_runs":
		return "agentruntime.consensus.concurrent_runs"
	case "agentruntime_restore_on_startup":
		return "agentruntime.restore_on_startup"
	case "agentruntime_report_summary_enabled":
		return "agentruntime.report.summary_enabled"
	case "agentruntime_archive_enabled":
		return "agentruntime.archive.enabled"
	case "agentruntime_archive_dir":
		return "agentruntime.archive.dir"
	case "agentruntime_archive_retention_days":
		return "agentruntime.archive.retention_days"
	case "agentruntime_archive_max_file_bytes":
		return "agentruntime.archive.max_file_bytes"
	case "telegram_bot_token":
		return "channels.telegram.bot_token"
	case "skills_enabled":
		return "extensions.skills.enabled"
	case "skills_watch":
		return "extensions.skills.watch"
	case "skills_watch_debounce_ms":
		return "extensions.skills.watch_debounce_ms"
	case "skills_extra_dirs_json":
		return "extensions.skills.extra_dirs"
	case "skills_bundled_dir":
		return "extensions.skills.bundled_dir"
	case "plugins_enabled":
		return "extensions.plugins.enabled"
	case "plugins_watch":
		return "extensions.plugins.watch"
	case "plugins_watch_debounce_ms":
		return "extensions.plugins.watch_debounce_ms"
	case "plugins_extra_dirs_json":
		return "extensions.plugins.extra_dirs"
	case "plugins_bundled_dir":
		return "extensions.plugins.bundled_dir"
	case "plugins_allow_mcp_servers":
		return "extensions.plugins.allow_mcp_servers"
	}

	switch {
	case strings.HasPrefix(key, "api_"):
		return "api." + strings.TrimPrefix(key, "api_")
	case strings.HasPrefix(key, "pulse_"):
		return "automation.pulse." + strings.TrimPrefix(key, "pulse_")
	case strings.HasPrefix(key, "reflection_"):
		return "automation.reflection." + strings.TrimPrefix(key, "reflection_")
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
