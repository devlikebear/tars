package config

// Field tiers decide where the console shows a field (#1240):
//
//   - basic: most people touch it at least once, shown up front
//   - advanced: inside a collapsed "Advanced" group
//   - file: not shown in the console; YAML and the example file only
//   - remove: nothing reads it or it was replaced; it is going away and is
//     hidden like a file field until then
//
// The full table is rendered into docs/config-field-tiers.md
// (TestConfigFieldTierDocIsCurrent).
const (
	FieldTierBasic    = "basic"
	FieldTierAdvanced = "advanced"
	FieldTierFile     = "file"
	FieldTierRemove   = "remove"
)

// FieldCondition holds when the field's Key has one of Values, compared as
// strings (a bool is "true" or "false"). A field with several ShowWhen
// conditions is shown when any of them holds, and only while the field each
// condition names is itself shown.
type FieldCondition struct {
	Key    string   `json:"key"`
	Values []string `json:"values"`
}

type fieldTier struct {
	tier     string
	showWhen []FieldCondition
	note     string
}

func on(key string) FieldCondition { return FieldCondition{Key: key, Values: []string{"true"}} }

func is(key string, values ...string) FieldCondition {
	return FieldCondition{Key: key, Values: values}
}

func basic(when ...FieldCondition) fieldTier { return fieldTier{tier: FieldTierBasic, showWhen: when} }

func advanced(when ...FieldCondition) fieldTier {
	return fieldTier{tier: FieldTierAdvanced, showWhen: when}
}

func fileOnly(note string) fieldTier { return fieldTier{tier: FieldTierFile, note: note} }

func remove(note string) fieldTier { return fieldTier{tier: FieldTierRemove, note: note} }

var (
	authOn         = is("api_auth_mode", "required", "external-required")
	semanticOn     = on("memory_semantic_enabled")
	pulseOn        = on("pulse_enabled")
	reflectionOn   = on("reflection_enabled")
	webSearchOn    = on("tools_web_search_enabled")
	perplexity     = is("tools_web_search_provider", "perplexity")
	webFetchOn     = on("tools_web_fetch_enabled")
	computerUseOn  = on("tools_computer_use_enabled")
	agentRuntimeOn = on("agentruntime_enabled")
	consensusOn    = on("agentruntime_consensus_enabled")
	schedulerOn    = on("work_scheduler_enabled")
	telegramOn     = on("channels_telegram_enabled")
	initiativeOn   = on("initiative_enabled")
	initiativeLive = is("initiative_mode", "live")
	// jev.* serves both initiative's text signals and computer use.
	jevUsed = []FieldCondition{is("initiative_backend", "jev"), is("tools_computer_use_backend", "jev")}
)

const (
	noteTuning   = "internal tuning value"
	noteStorage  = "storage path or retention limit"
	noteReload   = "file-watch reload knob"
	noteJevProbs = "only for initiative.backend: jev, whose probabilities the default llm backend does not produce"
)

var configFieldTiers = map[string]fieldTier{
	// Runtime
	"workspace_dir":             advanced(),
	"plan_clarify_mode":         basic(),
	"session_default_id":        fileOnly("rarely changed after init"),
	"session_auto_archive_days": basic(),
	"session_auto_delete_days":  basic(),
	"session_telegram_scope":    advanced(telegramOn),
	"style_directness_default":  advanced(),
	"style_humor_default":       advanced(),
	"style_caution_default":     advanced(),
	"style_autonomy_default":    advanced(),
	"log_level":                 advanced(),
	"log_file":                  advanced(),
	"log_rotate_max_size_mb":    fileOnly(noteStorage),
	"log_rotate_max_days":       fileOnly(noteStorage),
	"log_rotate_max_backups":    fileOnly(noteStorage),

	// API
	"api_auth_mode":                 basic(),
	"dashboard_auth_mode":           advanced(authOn),
	"api_auth_token":                fileOnly("legacy single token, replaced by the user and admin tokens; still read by the auth middleware"),
	"api_user_token":                basic(authOn),
	"api_admin_token":               basic(authOn),
	"api_allow_insecure_local_auth": advanced(authOn),
	"api_max_inflight_chat":         fileOnly(noteTuning),
	"api_max_inflight_agent_runs":   fileOnly(noteTuning),

	// Remote Access
	"remote_access_tailscale_serve_enabled":    basic(),
	"remote_access_tailscale_serve_https_port": advanced(on("remote_access_tailscale_serve_enabled")),

	// LLM
	"llm_providers":     basic(),
	"llm_tiers":         basic(),
	"llm_default_tier":  basic(),
	"llm_role_defaults": advanced(),

	// Memory
	"memory_backend":          remove("file is the only accepted value; the server only validates it"),
	"memory_semantic_enabled": basic(),
	"memory_embed_provider":   basic(semanticOn),
	"memory_embed_api_key":    basic(semanticOn),
	"memory_embed_base_url":   advanced(semanticOn),
	"memory_embed_model":      advanced(semanticOn),
	"memory_embed_dimensions": advanced(semanticOn),

	// Usage
	"usage_limit_daily_usd":      basic(),
	"usage_limit_weekly_usd":     basic(),
	"usage_limit_monthly_usd":    basic(),
	"usage_limit_mode":           basic(),
	"usage_daily_token_budget":   advanced(),
	"usage_price_overrides_json": fileOnly("per-model price map, edited as YAML"),

	// Automation
	"agent_max_iterations":               advanced(),
	"cron_run_history_limit":             fileOnly(noteStorage),
	"pulse_enabled":                      advanced(),
	"pulse_interval":                     fileOnly(noteTuning),
	"pulse_timeout":                      fileOnly("only used by the opt-in llm decider"),
	"pulse_active_hours":                 advanced(pulseOn),
	"pulse_timezone":                     advanced(pulseOn),
	"pulse_min_severity":                 advanced(pulseOn),
	"pulse_decider":                      advanced(pulseOn),
	"pulse_allowed_autofixes_json":       advanced(pulseOn),
	"pulse_notify_telegram":              advanced(pulseOn),
	"pulse_notify_session_events":        advanced(pulseOn),
	"pulse_cron_failure_threshold":       fileOnly(noteTuning),
	"pulse_stuck_run_minutes":            fileOnly(noteTuning),
	"pulse_disk_warn_percent":            fileOnly(noteTuning),
	"pulse_disk_critical_percent":        fileOnly(noteTuning),
	"pulse_delivery_failure_threshold":   fileOnly(noteTuning),
	"pulse_delivery_failure_window":      fileOnly(noteTuning),
	"pulse_reflection_failure_threshold": fileOnly(noteTuning),
	"reflection_enabled":                 advanced(),
	"reflection_sleep_window":            advanced(reflectionOn),
	"reflection_timezone":                advanced(reflectionOn),
	"reflection_tick_interval":           fileOnly(noteTuning),
	"reflection_empty_session_age":       fileOnly(noteTuning),
	"reflection_memory_lookback_hours":   fileOnly(noteTuning),
	"reflection_max_turns_per_session":   fileOnly(noteTuning),
	"notify_command":                     advanced(),
	"notify_when_no_clients":             advanced(),
	"schedule_timezone":                  remove("loaded and defaulted, but nothing outside internal/config reads it"),

	// Assistant: `tars assistant` takes its own flags and never reads these.
	"assistant_enabled":     remove("nothing reads it; tars assistant uses its own flags"),
	"assistant_hotkey":      remove("nothing reads it; tars assistant uses its own flags"),
	"assistant_whisper_bin": remove("nothing reads it; tars assistant uses its own flags"),
	"assistant_ffmpeg_bin":  remove("nothing reads it; tars assistant uses its own flags"),
	"assistant_tts_bin":     remove("nothing reads it; tars assistant uses its own flags"),

	// Compaction
	"compaction_trigger_tokens":       advanced(),
	"compaction_llm_mode":             advanced(),
	"compaction_keep_recent_tokens":   fileOnly(noteTuning),
	"compaction_keep_recent_fraction": fileOnly(noteTuning),
	"compaction_llm_timeout_seconds":  fileOnly(noteTuning),

	// Tools
	"tools_web_search_enabled":                    basic(),
	"tools_web_search_provider":                   basic(webSearchOn),
	"tools_web_search_api_key":                    basic(is("tools_web_search_provider", "brave")),
	"tools_web_search_perplexity_api_key":         basic(perplexity),
	"tools_web_search_perplexity_model":           advanced(perplexity),
	"tools_web_search_perplexity_base_url":        advanced(perplexity),
	"tools_web_search_cache_ttl_seconds":          fileOnly(noteTuning),
	"tools_web_fetch_enabled":                     basic(),
	"tools_web_fetch_allow_private_hosts":         advanced(webFetchOn),
	"tools_web_fetch_private_host_allowlist_json": advanced(webFetchOn),
	"tools_default_set":                           advanced(),
	"tools_allow_high_risk_user":                  advanced(),
	"tools_exec_max_timeout_ms":                   advanced(),
	"tools_process_max_timeout_ms":                advanced(),
	"tools_apply_patch_enabled":                   advanced(),
	"tools_message_enabled":                       advanced(),
	"tools_agentruntime_enabled":                  advanced(),
	"tools_computer_use_enabled":                  basic(),
	"tools_computer_use_backend":                  advanced(computerUseOn),
	"tools_computer_use_cua_driver_path":          advanced(computerUseOn),
	"tools_computer_use_max_steps":                advanced(computerUseOn),
	"tools_computer_use_total_timeout_seconds":    advanced(computerUseOn),
	"tools_computer_use_expose_values":            advanced(computerUseOn),
	"tools_computer_use_step_timeout_seconds":     fileOnly(noteTuning),

	// MCP
	"mcp_command_allowlist_json": advanced(),
	"mcp_servers_json":           advanced(),

	// Agent Runtime
	"agentruntime_enabled":                           advanced(),
	"agentruntime_default_agent":                     advanced(agentRuntimeOn),
	"agentruntime_subagents_max_threads":             advanced(agentRuntimeOn),
	"agentruntime_subagents_max_depth":               advanced(agentRuntimeOn),
	"agentruntime_consensus_enabled":                 advanced(agentRuntimeOn),
	"agentruntime_consensus_max_fanout":              advanced(consensusOn),
	"agentruntime_consensus_budget_usd":              advanced(consensusOn),
	"agentruntime_consensus_budget_tokens":           fileOnly(noteTuning),
	"agentruntime_consensus_timeout_seconds":         fileOnly(noteTuning),
	"agentruntime_consensus_allowed_aliases_json":    fileOnly("provider alias allowlist for an opt-in mode"),
	"agentruntime_consensus_concurrent_runs":         fileOnly(noteTuning),
	"agentruntime_agents_json":                       fileOnly("agent catalog, edited as YAML"),
	"agentruntime_task_override":                     fileOnly("provider/model allowlist, edited as YAML"),
	"agentruntime_agents_watch":                      fileOnly(noteReload),
	"agentruntime_agents_watch_debounce_ms":          fileOnly(noteReload),
	"agentruntime_persistence_enabled":               fileOnly(noteStorage),
	"agentruntime_persistence_dir":                   fileOnly(noteStorage),
	"agentruntime_runs_persistence_enabled":          fileOnly(noteStorage),
	"agentruntime_channels_persistence_enabled":      fileOnly(noteStorage),
	"agentruntime_runs_max_records":                  fileOnly(noteStorage),
	"agentruntime_channels_max_messages_per_channel": fileOnly(noteStorage),
	"agentruntime_restore_on_startup":                fileOnly(noteStorage),
	"agentruntime_report_summary_enabled":            fileOnly(noteTuning),
	"agentruntime_archive_enabled":                   fileOnly(noteStorage),
	"agentruntime_archive_dir":                       fileOnly(noteStorage),
	"agentruntime_archive_retention_days":            fileOnly(noteStorage),
	"agentruntime_archive_max_file_bytes":            fileOnly(noteStorage),

	// Work Ledger / Work Scheduler
	"work_ledger_enabled":              advanced(),
	"work_scheduler_enabled":           advanced(on("work_ledger_enabled")),
	"work_scheduler_max_workers":       advanced(schedulerOn),
	"work_scheduler_lease_seconds":     fileOnly(noteTuning),
	"work_scheduler_heartbeat_seconds": fileOnly(noteTuning),
	"work_scheduler_poll_milliseconds": fileOnly(noteTuning),

	// Channels
	"channels_telegram_enabled":         basic(),
	"telegram_bot_token":                basic(telegramOn),
	"channels_telegram_dm_policy":       advanced(telegramOn),
	"channels_telegram_polling_enabled": advanced(telegramOn),
	"channels_webhook_enabled":          advanced(),
	"channels_local_enabled":            fileOnly("agent runtime's local CLI channel"),

	// Console / Companion
	"console_default_mode": basic(),
	"companion_enabled":    basic(),

	// Embodiment
	"embodiment_enabled":        fileOnly("dormant subsystem"),
	"embodiment_providers_json": fileOnly("dormant subsystem"),

	// Initiative
	"initiative_enabled":                   basic(),
	"initiative_mode":                      basic(initiativeOn),
	"initiative_quiet_hours":               basic(initiativeOn),
	"initiative_backend":                   advanced(initiativeOn),
	"initiative_timezone":                  advanced(initiativeOn),
	"initiative_daily_cap":                 advanced(initiativeOn),
	"initiative_cooldown":                  advanced(initiativeOn),
	"initiative_daily_text_calls":          advanced(initiativeOn),
	"initiative_daily_speak_calls":         advanced(initiativeLive),
	"initiative_telegram":                  advanced(initiativeLive),
	"initiative_tick":                      fileOnly(noteTuning),
	"initiative_reply_window":              fileOnly(noteTuning),
	"initiative_body_provider":             fileOnly("needs the dormant embodiment subsystem"),
	"initiative_threshold_quiet_requested": fileOnly(noteJevProbs),
	"initiative_threshold_user_strained":   fileOnly(noteJevProbs),
	"initiative_threshold_special_day":     fileOnly(noteJevProbs),
	"jev_base_url":                         advanced(jevUsed...),
	"jev_api_key":                          advanced(jevUsed...),
	"jev_model":                            advanced(jevUsed...),
	"jev_timeout_seconds":                  fileOnly(noteTuning),

	// Extensions
	"skills_enabled":            advanced(),
	"skills_extra_dirs_json":    advanced(),
	"skills_watch":              fileOnly(noteReload),
	"skills_watch_debounce_ms":  fileOnly(noteReload),
	"skills_bundled_dir":        fileOnly(noteStorage),
	"plugins_enabled":           advanced(),
	"plugins_extra_dirs_json":   advanced(),
	"plugins_allow_mcp_servers": advanced(),
	"plugins_watch":             fileOnly(noteReload),
	"plugins_watch_debounce_ms": fileOnly(noteReload),
	"plugins_bundled_dir":       fileOnly(noteStorage),
}

func withConfigFieldTiers(fields []FieldMeta) []FieldMeta {
	for i := range fields {
		tier, ok := configFieldTiers[fields[i].Key]
		if !ok {
			continue
		}
		fields[i].Tier = tier.tier
		fields[i].ShowWhen = tier.showWhen
	}
	return fields
}
