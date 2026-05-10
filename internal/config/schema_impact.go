package config

var configFieldImpactHints = map[string][]string{
	"workspace_dir": {
		"Changing the workspace moves sessions, memory, skills, plugins, MCP packages, logs, and agent runtime state.",
		"Restart is required for most workspace path changes to take full effect.",
	},
	"plan_clarify_mode": {
		"Changing this affects how new planning requests decide between asking questions and drafting immediately.",
	},
	"session_default_id": {
		"Changing this affects which session one-shot CLI and default chat requests target.",
	},
	"session_telegram_scope": {
		"Changing this affects how Telegram messages are mapped to chat sessions.",
	},
	"style_directness_default": {
		"Changing this affects the default TARS prompt style for directness in sessions without overrides.",
	},
	"style_humor_default": {
		"Changing this affects the default TARS prompt style for humor in sessions without overrides.",
	},
	"style_caution_default": {
		"Changing this affects the default TARS prompt style for risk checks and clarification behavior.",
	},
	"style_autonomy_default": {
		"Changing this affects the default TARS prompt style for autonomous follow-through, bounded by session consent.",
	},
	"log_level": {
		"Debug logging can increase log volume and expose more operational detail.",
	},
	"log_file": {
		"Changing the log destination affects where Logs and operational troubleshooting look for runtime output.",
	},
	"log_rotate_max_size_mb": {
		"Changing rotation size affects log retention and disk usage.",
	},
	"log_rotate_max_days": {
		"Changing retention days affects how long rotated logs remain available for incident review.",
	},
	"log_rotate_max_backups": {
		"Changing backup count affects how much historical log evidence is retained.",
	},
	"api_auth_mode": {
		"Changing API auth mode affects Auth/API access to /v1/* endpoints.",
		"Stricter modes may require valid bearer tokens immediately after restart.",
	},
	"dashboard_auth_mode": {
		"Changing dashboard auth mode affects Auth/API browser access to the console.",
	},
	"api_auth_token": {
		"Changing the legacy API token can immediately break older clients that still use it.",
	},
	"api_user_token": {
		"Changing the user token affects chat, read, and general API clients.",
	},
	"api_admin_token": {
		"Changing the admin token affects admin-only Settings, session config, approvals, and Agent Runtime control APIs.",
	},
	"api_allow_insecure_local_auth": {
		"Allowing insecure local auth expands loopback access and should stay limited to local development.",
	},
	"api_max_inflight_chat": {
		"Higher concurrency can improve throughput but increases peak provider and CPU load.",
	},
	"api_max_inflight_agent_runs": {
		"Higher agent-run concurrency can increase peak CPU, workspace I/O, and provider usage.",
	},
	"llm_providers": {
		"Changing providers affects LLM routing credentials, base URLs, auth mode, and provider availability.",
		"Invalid provider aliases can break every tier that references them.",
	},
	"llm_tiers": {
		"Changing tiers affects LLM routing for chat, pulse, reflection, compaction, and Agent Runtime roles.",
		"Model, reasoning, and service tier changes can alter quality, latency, and cost.",
	},
	"llm_default_tier": {
		"Changing the default tier affects roles that do not have an explicit LLM routing override.",
	},
	"llm_role_defaults": {
		"Changing role defaults reroutes specific subsystems such as chat, pulse, reflection, compaction, and Agent Runtime.",
	},
	"usage_limit_daily_usd": {
		"Lower daily spend limits can stop or degrade LLM calls sooner.",
	},
	"usage_limit_weekly_usd": {
		"Lower weekly spend limits can stop or degrade LLM calls sooner.",
	},
	"usage_limit_monthly_usd": {
		"Lower monthly spend limits can stop or degrade LLM calls sooner.",
	},
	"usage_daily_token_budget": {
		"Enabling a token budget shows today's token consumption in the console header.",
	},
	"usage_limit_mode": {
		"Hard mode can block requests when usage limits are reached.",
	},
	"usage_price_overrides_json": {
		"Changing price overrides affects usage analytics, budget enforcement, and cost comparisons.",
	},
	"memory_backend": {
		"Changing memory backend affects durable recall storage and may require migration before restart.",
	},
	"memory_semantic_enabled": {
		"Semantic memory changes recall behavior and embedding API usage.",
	},
	"memory_embed_provider": {
		"Changing the embedding provider affects Memory semantic recall and embedding API compatibility.",
	},
	"memory_embed_base_url": {
		"Changing the embedding base URL affects where Memory semantic indexing sends embedding requests.",
	},
	"memory_embed_api_key": {
		"Changing the embedding API key affects Memory semantic indexing authentication.",
	},
	"memory_embed_model": {
		"Changing the embedding model affects Memory vector compatibility and may require re-indexing embeddings.",
	},
	"memory_embed_dimensions": {
		"Changing embedding dimensions affects Memory vector compatibility with existing stored embeddings.",
	},
	"agent_max_iterations": {
		"Higher iteration limits allow longer agent loops and can increase cost.",
	},
	"cron_run_history_limit": {
		"Changing cron history retention affects Cron run evidence, troubleshooting, and workspace storage.",
	},
	"pulse_enabled": {
		"Disabling Pulse stops background watchdog detection for cron, disk, delivery, reflection, stalled chat, and stuck run signals.",
	},
	"pulse_interval": {
		"Changing this changes the pulse tick cadence for watchdog checks.",
		"Shorter intervals can raise CPU use and pulse log volume.",
	},
	"pulse_timeout": {
		"Shorter timeouts can reduce stuck checks but may skip slow LLM decisions.",
	},
	"pulse_active_hours": {
		"Changing active hours affects when Pulse can notify or autofix.",
	},
	"pulse_timezone": {
		"Changing timezone shifts Pulse active-hour evaluation.",
	},
	"pulse_min_severity": {
		"Raising the floor reduces notification noise; lowering it surfaces more signals.",
	},
	"pulse_allowed_autofixes_json": {
		"Changing the Pulse autofix allowlist controls which deterministic autofix actions may run.",
	},
	"pulse_notify_telegram": {
		"Changing Telegram notification delivery affects whether Pulse incidents leave the console.",
	},
	"pulse_notify_session_events": {
		"Changing session event delivery affects whether Pulse incidents appear in live console notifications.",
	},
	"pulse_cron_failure_threshold": {
		"Changing this threshold affects how quickly repeated Cron failures become Pulse incidents.",
	},
	"pulse_stuck_run_minutes": {
		"Changing this threshold affects when long Agent Runtime runs are considered stuck.",
	},
	"pulse_disk_warn_percent": {
		"Lower thresholds make disk pressure warnings fire earlier.",
	},
	"pulse_disk_critical_percent": {
		"Lower thresholds make critical disk pressure alerts fire earlier.",
	},
	"pulse_delivery_failure_threshold": {
		"Changing this threshold affects how quickly Telegram delivery failures become Pulse incidents.",
	},
	"pulse_delivery_failure_window": {
		"Changing this window affects how Pulse groups recent delivery failures.",
	},
	"pulse_reflection_failure_threshold": {
		"Changing this threshold affects when Reflection failures become Pulse incidents.",
	},
	"reflection_enabled": {
		"Disabling Reflection stops nightly memory extraction and empty-session cleanup.",
	},
	"reflection_sleep_window": {
		"Changing the Reflection sleep window shifts when nightly memory and cleanup jobs may run.",
	},
	"reflection_timezone": {
		"Changing timezone shifts Reflection sleep-window evaluation.",
	},
	"reflection_tick_interval": {
		"Changing this affects how often the nightly reflection scheduler checks its window.",
	},
	"reflection_empty_session_age": {
		"Changing this age affects when Reflection can prune old empty non-main sessions.",
	},
	"reflection_memory_lookback_hours": {
		"Higher lookback scans more history and can increase reflection runtime.",
	},
	"reflection_max_turns_per_session": {
		"Changing this cap affects how much transcript history Reflection reviews per session.",
	},
	"notify_command": {
		"Changing notify command affects external notification delivery for background events.",
	},
	"notify_when_no_clients": {
		"Changing this controls whether notifications fire when no console clients are connected.",
	},
	"schedule_timezone": {
		"Changing schedule timezone affects Cron and scheduled trigger interpretation.",
	},
	"assistant_enabled": {
		"Changing assistant availability affects voice assistant hotkey and audio workflows.",
	},
	"assistant_hotkey": {
		"Changing the assistant hotkey affects global activation behavior.",
	},
	"assistant_whisper_bin": {
		"Changing Whisper path affects speech-to-text execution.",
	},
	"assistant_ffmpeg_bin": {
		"Changing FFmpeg path affects assistant audio processing.",
	},
	"assistant_tts_bin": {
		"Changing TTS path affects assistant speech output.",
	},
	"compaction_trigger_tokens": {
		"Changing trigger tokens affects when long chat transcripts auto-compact.",
	},
	"compaction_keep_recent_tokens": {
		"Changing keep-recent tokens affects how much fresh transcript context survives compaction.",
	},
	"compaction_keep_recent_fraction": {
		"Changing keep-recent fraction affects compaction aggressiveness.",
	},
	"compaction_llm_mode": {
		"Changing compaction LLM mode affects whether compaction may call an LLM or stays deterministic.",
	},
	"compaction_llm_timeout_seconds": {
		"Changing timeout affects how long LLM-assisted compaction waits before fallback.",
	},
	"tools_web_search_enabled": {
		"Changing web search availability affects chat tool access and prompt tool inventory.",
	},
	"tools_web_fetch_enabled": {
		"Changing web fetch availability affects network access from chat tools.",
	},
	"tools_default_set": {
		"Changing default tool set affects new session tool policy.",
	},
	"tools_allow_high_risk_user": {
		"Allowing high-risk tools expands user-facing write, shell, or mutation capabilities.",
	},
	"tools_web_search_api_key": {
		"Changing search credentials affects web search tool authentication.",
	},
	"tools_web_search_provider": {
		"Changing search provider affects query behavior, latency, and provider cost.",
	},
	"tools_web_search_perplexity_api_key": {
		"Changing Perplexity credentials affects Perplexity-backed search authentication.",
	},
	"tools_web_search_perplexity_model": {
		"Changing Perplexity model affects search answer quality and cost.",
	},
	"tools_web_search_perplexity_base_url": {
		"Changing Perplexity base URL affects where search requests are sent.",
	},
	"tools_web_search_cache_ttl_seconds": {
		"Changing cache TTL affects search freshness and repeated request volume.",
	},
	"tools_exec_max_timeout_ms": {
		"Raising the cap lets long-running commands (builds, CI waits, installs) complete without being killed; lowering it tightens the upper bound on shell hangs.",
	},
	"tools_web_fetch_allow_private_hosts": {
		"Allowing private hosts expands the network surface available to web fetch tools.",
	},
	"tools_web_fetch_private_host_allowlist_json": {
		"Changing the private host allowlist controls which internal hosts web fetch may access.",
	},
	"tools_apply_patch_enabled": {
		"Changing apply-patch availability affects whether chat sessions can edit files through the patch tool.",
	},
	"tools_message_enabled": {
		"Changing message tool availability affects user-facing notification behavior.",
	},
	"tools_agentruntime_enabled": {
		"Changing Agent Runtime tool availability affects whether chat can dispatch subagent runs.",
	},
	"mcp_command_allowlist_json": {
		"Changing MCP command allowlist affects which local commands MCP packages may execute.",
	},
	"mcp_servers_json": {
		"Changing MCP server catalog affects external tool inventory and connection startup.",
	},
	"agentruntime_enabled": {
		"Disabling Agent Runtime stops multi-agent orchestration and subagent run APIs.",
	},
	"agentruntime_default_agent": {
		"Changing default agent affects which Agent Runtime profile handles unspecified tasks.",
	},
	"agentruntime_agents_json": {
		"Changing agent catalog affects available Agent Runtime profiles and tool policies.",
	},
	"agentruntime_task_override": {
		"Changing task overrides affects provider/model routing constraints for Agent Runtime tasks.",
	},
	"agentruntime_agents_watch": {
		"Changing watch mode affects whether Agent Runtime profiles reload from disk automatically.",
	},
	"agentruntime_agents_watch_debounce_ms": {
		"Changing debounce affects how quickly Agent Runtime profile edits reload.",
	},
	"agentruntime_persistence_enabled": {
		"Changing persistence affects whether Agent Runtime state survives restart.",
	},
	"agentruntime_persistence_dir": {
		"Changing persistence directory affects where Agent Runtime state is stored.",
	},
	"agentruntime_runs_persistence_enabled": {
		"Changing run persistence affects run history, replay, and diff timeline availability.",
	},
	"agentruntime_channels_persistence_enabled": {
		"Changing channel persistence affects retained Agent Runtime channel messages.",
	},
	"agentruntime_runs_max_records": {
		"Changing max run records affects Agent Runtime history retention and storage.",
	},
	"agentruntime_channels_max_messages_per_channel": {
		"Changing channel retention affects how much Agent Runtime conversation history is kept.",
	},
	"agentruntime_subagents_max_threads": {
		"Higher thread counts can run more subagents in parallel and increase peak cost.",
	},
	"agentruntime_subagents_max_depth": {
		"Changing max depth affects how deeply subagents can spawn additional work.",
	},
	"agentruntime_consensus_enabled": {
		"Changing consensus availability affects advanced parallel comparison workflows.",
	},
	"agentruntime_consensus_max_fanout": {
		"Higher fanout improves comparison breadth but increases parallel LLM calls.",
	},
	"agentruntime_consensus_budget_tokens": {
		"Changing consensus token budget affects parallel comparison run limits.",
	},
	"agentruntime_consensus_budget_usd": {
		"Changing consensus cost budget affects parallel comparison run limits.",
	},
	"agentruntime_consensus_timeout_seconds": {
		"Changing consensus timeout affects how long comparison variants may run.",
	},
	"agentruntime_consensus_allowed_aliases_json": {
		"Changing allowed aliases affects which providers consensus variants may use.",
	},
	"agentruntime_consensus_concurrent_runs": {
		"Changing concurrent consensus runs affects peak provider load.",
	},
	"agentruntime_restore_on_startup": {
		"Changing restore behavior affects whether persisted runs return after restart.",
	},
	"agentruntime_report_summary_enabled": {
		"Changing report summaries affects Agent Runtime run report output.",
	},
	"agentruntime_archive_enabled": {
		"Changing archive availability affects long-term Agent Runtime run storage.",
	},
	"agentruntime_archive_dir": {
		"Changing archive directory affects where Agent Runtime run archives are written.",
	},
	"agentruntime_archive_retention_days": {
		"Changing archive retention affects how long Agent Runtime run archives remain.",
	},
	"agentruntime_archive_max_file_bytes": {
		"Changing archive file size affects Agent Runtime archive rollover behavior.",
	},
	"channels_local_enabled": {
		"Changing local channel availability affects CLI dispatch.",
	},
	"channels_webhook_enabled": {
		"Changing webhook channel availability affects inbound webhook dispatch.",
	},
	"channels_telegram_enabled": {
		"Changing Telegram channel availability affects Telegram bot dispatch.",
	},
	"channels_telegram_dm_policy": {
		"Changing Telegram DM policy affects who can send direct messages to TARS.",
	},
	"channels_telegram_polling_enabled": {
		"Changing Telegram polling affects how bot updates are received.",
	},
	"telegram_bot_token": {
		"Changing Telegram bot token affects Telegram channel authentication.",
	},
	"skills_enabled": {
		"Disabling skills removes skill instructions from Extensions and chat slash selection.",
	},
	"skills_watch": {
		"Changing skill watch affects whether skill files reload automatically.",
	},
	"skills_watch_debounce_ms": {
		"Changing skill debounce affects how quickly skill file edits reload.",
	},
	"skills_extra_dirs_json": {
		"Changing extra skill directories affects which local skills are discoverable.",
	},
	"skills_bundled_dir": {
		"Changing skills directory affects where bundled skills are loaded from.",
	},
	"plugins_enabled": {
		"Disabling plugins removes plugin definitions from Extensions.",
	},
	"plugins_watch": {
		"Changing plugin watch affects whether plugin files reload automatically.",
	},
	"plugins_watch_debounce_ms": {
		"Changing plugin debounce affects how quickly plugin file edits reload.",
	},
	"plugins_extra_dirs_json": {
		"Changing extra plugin directories affects which local plugins are discoverable.",
	},
	"plugins_bundled_dir": {
		"Changing plugins directory affects where bundled plugins are loaded from.",
	},
	"plugins_allow_mcp_servers": {
		"Allowing plugin-declared MCP servers expands Extensions startup behavior and MCP tool availability.",
	},
}

func withConfigImpactHints(fields []FieldMeta) []FieldMeta {
	for i := range fields {
		if hints := configFieldImpactHints[fields[i].Key]; len(hints) > 0 {
			fields[i].Impact = append([]string(nil), hints...)
		}
	}
	return fields
}
