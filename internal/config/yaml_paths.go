package config

import "strings"

func preferredYAMLPathForKey(key string) string {
	key = strings.TrimSpace(strings.ToLower(key))
	switch key {
	case "mode":
		return "runtime.mode"
	case "workspace_dir":
		return "runtime.workspace_dir"
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
	case "browser_runtime_enabled":
		return "browser.runtime.enabled"
	case "browser_default_profile":
		return "browser.default_profile"
	case "browser_managed_headless":
		return "browser.managed.headless"
	case "browser_managed_executable_path":
		return "browser.managed.executable_path"
	case "browser_managed_user_data_dir":
		return "browser.managed.user_data_dir"
	case "browser_site_flows_dir":
		return "browser.site_flows_dir"
	case "browser_auto_login_site_allowlist_json":
		return "browser.auto_login.site_allowlist"
	case "gateway_enabled":
		return "gateway.enabled"
	case "gateway_default_agent":
		return "gateway.default_agent"
	case "gateway_task_override":
		return "gateway.task_override"
	case "gateway_agents_json":
		return "gateway.agents.list"
	case "gateway_agents_watch":
		return "gateway.agents.watch"
	case "gateway_agents_watch_debounce_ms":
		return "gateway.agents.watch_debounce_ms"
	case "gateway_persistence_enabled":
		return "gateway.persistence.enabled"
	case "gateway_persistence_dir":
		return "gateway.persistence.dir"
	case "gateway_runs_persistence_enabled":
		return "gateway.runs.persistence_enabled"
	case "gateway_runs_max_records":
		return "gateway.runs.max_records"
	case "gateway_channels_persistence_enabled":
		return "gateway.channels.persistence_enabled"
	case "gateway_channels_max_messages_per_channel":
		return "gateway.channels.max_messages_per_channel"
	case "gateway_subagents_max_threads":
		return "gateway.subagents.max_threads"
	case "gateway_subagents_max_depth":
		return "gateway.subagents.max_depth"
	case "gateway_consensus_enabled":
		return "gateway.consensus.enabled"
	case "gateway_consensus_max_fanout":
		return "gateway.consensus.max_fanout"
	case "gateway_consensus_budget_tokens":
		return "gateway.consensus.budget_tokens"
	case "gateway_consensus_budget_usd":
		return "gateway.consensus.budget_usd"
	case "gateway_consensus_timeout_seconds":
		return "gateway.consensus.timeout_seconds"
	case "gateway_consensus_allowed_aliases_json":
		return "gateway.consensus.allowed_aliases"
	case "gateway_consensus_concurrent_runs":
		return "gateway.consensus.concurrent_runs"
	case "gateway_restore_on_startup":
		return "gateway.restore_on_startup"
	case "gateway_report_summary_enabled":
		return "gateway.report.summary_enabled"
	case "gateway_archive_enabled":
		return "gateway.archive.enabled"
	case "gateway_archive_dir":
		return "gateway.archive.dir"
	case "gateway_archive_retention_days":
		return "gateway.archive.retention_days"
	case "gateway_archive_max_file_bytes":
		return "gateway.archive.max_file_bytes"
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
	case strings.HasPrefix(key, "vault_kv_"):
		return "vault.kv." + strings.TrimPrefix(key, "vault_kv_")
	case strings.HasPrefix(key, "vault_approle_"):
		return "vault.approle." + strings.TrimPrefix(key, "vault_approle_")
	case key == "vault_auth_mode":
		return "vault.auth.mode"
	case key == "vault_secret_path_allowlist_json":
		return "vault.secret_path_allowlist"
	case strings.HasPrefix(key, "vault_"):
		return "vault." + strings.TrimPrefix(key, "vault_")
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
