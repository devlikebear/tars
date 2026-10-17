# Config field tiers

<!-- Generated from internal/config/schema_tier.go by
     go test ./internal/config -run TestConfigFieldTierDocIsCurrent -update-tier-doc -->

Where the console shows each field of `/console/config` (#1240). The schema
(`GET /v1/admin/config/schema`) carries the same `tier` and `show_when`.

| Tier | Meaning | Fields |
|---|---|---|
| basic | Most people touch it at least once; shown up front | 30 |
| advanced | Inside a collapsed Advanced group | 78 |
| file | Not in the console; YAML and `config/tars.config.example.yaml` only | 66 |
| remove | Nothing reads it or it was replaced; to be removed, hidden until then | 0 |

Total: 174 fields in 19 sections.

`show_when` lists conditions on other fields; the field shows when any one holds
and the field it names is itself shown.

## Runtime

| Key | YAML path | Tier | Shown when | Why |
|---|---|---|---|---|
| `workspace_dir` | `runtime.workspace_dir` | advanced |  |  |
| `plan_clarify_mode` | `runtime.plan_clarify_mode` | basic |  |  |
| `session_default_id` | `runtime.session.default_id` | file |  | rarely changed after init |
| `session_auto_archive_days` | `runtime.session.auto_archive_days` | basic |  |  |
| `session_auto_delete_days` | `runtime.session.auto_delete_days` | basic |  |  |
| `session_telegram_scope` | `runtime.session.telegram_scope` | advanced | `channels.telegram.enabled` = true |  |
| `style_directness_default` | `runtime.style.directness_default` | advanced |  |  |
| `style_humor_default` | `runtime.style.humor_default` | advanced |  |  |
| `style_caution_default` | `runtime.style.caution_default` | advanced |  |  |
| `style_autonomy_default` | `runtime.style.autonomy_default` | advanced |  |  |
| `log_level` | `log.level` | advanced |  |  |
| `log_file` | `log.file` | advanced |  |  |
| `log_rotate_max_size_mb` | `log.rotate.max_size_mb` | file |  | storage path or retention limit |
| `log_rotate_max_days` | `log.rotate.max_days` | file |  | storage path or retention limit |
| `log_rotate_max_backups` | `log.rotate.max_backups` | file |  | storage path or retention limit |

## API

| Key | YAML path | Tier | Shown when | Why |
|---|---|---|---|---|
| `api_auth_mode` | `api.auth_mode` | basic |  |  |
| `dashboard_auth_mode` | `api.dashboard.auth_mode` | advanced | `api.auth_mode` = required / external-required |  |
| `api_auth_token` | `api.auth_token` | file |  | legacy single token, replaced by the user and admin tokens; still read by the auth middleware |
| `api_user_token` | `api.user_token` | basic | `api.auth_mode` = required / external-required |  |
| `api_admin_token` | `api.admin_token` | basic | `api.auth_mode` = required / external-required |  |
| `api_allow_insecure_local_auth` | `api.allow_insecure_local_auth` | advanced | `api.auth_mode` = required / external-required |  |
| `api_max_inflight_chat` | `api.max_inflight.chat` | file |  | internal tuning value |
| `api_max_inflight_agent_runs` | `api.max_inflight.agent_runs` | file |  | internal tuning value |

## Remote Access

| Key | YAML path | Tier | Shown when | Why |
|---|---|---|---|---|
| `remote_access_tailscale_serve_enabled` | `remote_access.tailscale_serve.enabled` | basic |  |  |
| `remote_access_tailscale_serve_https_port` | `remote_access.tailscale_serve.https_port` | advanced | `remote_access.tailscale_serve.enabled` = true |  |

## LLM

| Key | YAML path | Tier | Shown when | Why |
|---|---|---|---|---|
| `llm_providers` | `llm.providers` | basic |  |  |
| `llm_tiers` | `llm.tiers` | basic |  |  |
| `llm_default_tier` | `llm.default_tier` | basic |  |  |
| `llm_role_defaults` | `llm.role_defaults` | advanced |  |  |

## Memory

| Key | YAML path | Tier | Shown when | Why |
|---|---|---|---|---|
| `memory_semantic_enabled` | `memory.semantic.enabled` | basic |  |  |
| `memory_embed_provider` | `memory.embed.provider` | basic | `memory.semantic.enabled` = true |  |
| `memory_embed_base_url` | `memory.embed.base_url` | advanced | `memory.semantic.enabled` = true |  |
| `memory_embed_api_key` | `memory.embed.api_key` | basic | `memory.semantic.enabled` = true |  |
| `memory_embed_model` | `memory.embed.model` | advanced | `memory.semantic.enabled` = true |  |
| `memory_embed_dimensions` | `memory.embed.dimensions` | advanced | `memory.semantic.enabled` = true |  |

## Usage

| Key | YAML path | Tier | Shown when | Why |
|---|---|---|---|---|
| `usage_limit_daily_usd` | `usage.limits.daily_usd` | basic |  |  |
| `usage_limit_weekly_usd` | `usage.limits.weekly_usd` | basic |  |  |
| `usage_limit_monthly_usd` | `usage.limits.monthly_usd` | basic |  |  |
| `usage_daily_token_budget` | `usage.limits.daily_tokens` | advanced |  |  |
| `usage_limit_mode` | `usage.limits.mode` | basic |  |  |
| `usage_price_overrides_json` | `usage.price_overrides` | file |  | per-model price map, edited as YAML |

## Automation

| Key | YAML path | Tier | Shown when | Why |
|---|---|---|---|---|
| `agent_max_iterations` | `automation.agent.max_iterations` | advanced |  |  |
| `cron_run_history_limit` | `automation.cron.run_history_limit` | file |  | storage path or retention limit |
| `pulse_enabled` | `automation.pulse.enabled` | advanced |  |  |
| `pulse_interval` | `automation.pulse.interval` | file |  | internal tuning value |
| `pulse_timeout` | `automation.pulse.timeout` | file |  | only used by the opt-in llm decider |
| `pulse_active_hours` | `automation.pulse.active_hours` | advanced | `automation.pulse.enabled` = true |  |
| `pulse_timezone` | `automation.pulse.timezone` | advanced | `automation.pulse.enabled` = true |  |
| `pulse_min_severity` | `automation.pulse.min_severity` | advanced | `automation.pulse.enabled` = true |  |
| `pulse_decider` | `automation.pulse.decider` | advanced | `automation.pulse.enabled` = true |  |
| `pulse_allowed_autofixes_json` | `automation.pulse.allowed_autofixes` | advanced | `automation.pulse.enabled` = true |  |
| `pulse_notify_telegram` | `automation.pulse.notify_telegram` | advanced | `automation.pulse.enabled` = true |  |
| `pulse_notify_session_events` | `automation.pulse.notify_session_events` | advanced | `automation.pulse.enabled` = true |  |
| `pulse_cron_failure_threshold` | `automation.pulse.cron_failure_threshold` | file |  | internal tuning value |
| `pulse_stuck_run_minutes` | `automation.pulse.stuck_run_minutes` | file |  | internal tuning value |
| `pulse_disk_warn_percent` | `automation.pulse.disk_warn_percent` | file |  | internal tuning value |
| `pulse_disk_critical_percent` | `automation.pulse.disk_critical_percent` | file |  | internal tuning value |
| `pulse_delivery_failure_threshold` | `automation.pulse.delivery_failure_threshold` | file |  | internal tuning value |
| `pulse_delivery_failure_window` | `automation.pulse.delivery_failure_window` | file |  | internal tuning value |
| `pulse_reflection_failure_threshold` | `automation.pulse.reflection_failure_threshold` | file |  | internal tuning value |
| `reflection_enabled` | `automation.reflection.enabled` | advanced |  |  |
| `reflection_sleep_window` | `automation.reflection.sleep_window` | advanced | `automation.reflection.enabled` = true |  |
| `reflection_timezone` | `automation.reflection.timezone` | advanced | `automation.reflection.enabled` = true |  |
| `reflection_tick_interval` | `automation.reflection.tick_interval` | file |  | internal tuning value |
| `reflection_empty_session_age` | `automation.reflection.empty_session_age` | file |  | internal tuning value |
| `reflection_memory_lookback_hours` | `automation.reflection.memory_lookback_hours` | file |  | internal tuning value |
| `reflection_max_turns_per_session` | `automation.reflection.max_turns_per_session` | file |  | internal tuning value |
| `notify_command` | `automation.notify.command` | advanced |  |  |
| `notify_when_no_clients` | `automation.notify.when_no_clients` | advanced |  |  |

## Compaction

| Key | YAML path | Tier | Shown when | Why |
|---|---|---|---|---|
| `compaction_trigger_tokens` | `compaction.trigger_tokens` | advanced |  |  |
| `compaction_keep_recent_tokens` | `compaction.keep_recent_tokens` | file |  | internal tuning value |
| `compaction_keep_recent_fraction` | `compaction.keep_recent_fraction` | file |  | internal tuning value |
| `compaction_llm_mode` | `compaction.llm_mode` | advanced |  |  |
| `compaction_llm_timeout_seconds` | `compaction.llm_timeout_seconds` | file |  | internal tuning value |

## Tools

| Key | YAML path | Tier | Shown when | Why |
|---|---|---|---|---|
| `tools_web_search_enabled` | `tools.web_search.enabled` | basic |  |  |
| `tools_web_fetch_enabled` | `tools.web_fetch.enabled` | basic |  |  |
| `tools_default_set` | `tools.default_set` | advanced |  |  |
| `tools_allow_high_risk_user` | `tools.allow_high_risk_user` | advanced |  |  |
| `tools_web_search_api_key` | `tools.web_search.api_key` | basic | `tools.web_search.provider` = brave |  |
| `tools_web_search_provider` | `tools.web_search.provider` | basic | `tools.web_search.enabled` = true |  |
| `tools_web_search_perplexity_api_key` | `tools.web_search.perplexity.api_key` | basic | `tools.web_search.provider` = perplexity |  |
| `tools_web_search_perplexity_model` | `tools.web_search.perplexity.model` | advanced | `tools.web_search.provider` = perplexity |  |
| `tools_web_search_perplexity_base_url` | `tools.web_search.perplexity.base_url` | advanced | `tools.web_search.provider` = perplexity |  |
| `tools_web_search_cache_ttl_seconds` | `tools.web_search.cache_ttl_seconds` | file |  | internal tuning value |
| `tools_exec_max_timeout_ms` | `tools.exec.max_timeout_ms` | advanced |  |  |
| `tools_process_max_timeout_ms` | `tools.process.max_timeout_ms` | advanced |  |  |
| `tools_web_fetch_allow_private_hosts` | `tools.web_fetch.allow_private_hosts` | advanced | `tools.web_fetch.enabled` = true |  |
| `tools_web_fetch_private_host_allowlist_json` | `tools.web_fetch.private_host_allowlist` | advanced | `tools.web_fetch.enabled` = true |  |
| `tools_apply_patch_enabled` | `tools.apply_patch.enabled` | advanced |  |  |
| `tools_message_enabled` | `tools.message.enabled` | advanced |  |  |
| `tools_agentruntime_enabled` | `tools.agentruntime.enabled` | advanced |  |  |
| `tools_computer_use_backend` | `tools.computer_use.backend` | advanced | `tools.computer_use.enabled` = true |  |
| `tools_computer_use_enabled` | `tools.computer_use.enabled` | basic |  |  |
| `tools_computer_use_cua_driver_path` | `tools.computer_use.cua_driver_path` | advanced | `tools.computer_use.enabled` = true |  |
| `tools_computer_use_max_steps` | `tools.computer_use.max_steps` | advanced | `tools.computer_use.enabled` = true |  |
| `tools_computer_use_step_timeout_seconds` | `tools.computer_use.step_timeout_seconds` | file |  | internal tuning value |
| `tools_computer_use_total_timeout_seconds` | `tools.computer_use.total_timeout_seconds` | advanced | `tools.computer_use.enabled` = true |  |
| `tools_computer_use_expose_values` | `tools.computer_use.expose_values` | advanced | `tools.computer_use.enabled` = true |  |

## MCP

| Key | YAML path | Tier | Shown when | Why |
|---|---|---|---|---|
| `mcp_command_allowlist_json` | `extensions.mcp.command_allowlist` | advanced |  |  |
| `mcp_servers_json` | `extensions.mcp.servers` | advanced |  |  |

## Agent Runtime

| Key | YAML path | Tier | Shown when | Why |
|---|---|---|---|---|
| `agentruntime_enabled` | `agentruntime.enabled` | advanced |  |  |
| `agentruntime_default_agent` | `agentruntime.default_agent` | advanced | `agentruntime.enabled` = true |  |
| `agentruntime_agents_json` | `agentruntime.agents.list` | file |  | agent catalog, edited as YAML |
| `agentruntime_task_override` | `agentruntime.task_override` | file |  | provider/model allowlist, edited as YAML |
| `agentruntime_agents_watch` | `agentruntime.agents.watch` | file |  | file-watch reload knob |
| `agentruntime_agents_watch_debounce_ms` | `agentruntime.agents.watch_debounce_ms` | file |  | file-watch reload knob |
| `agentruntime_persistence_enabled` | `agentruntime.persistence.enabled` | file |  | storage path or retention limit |
| `agentruntime_persistence_dir` | `agentruntime.persistence.dir` | file |  | storage path or retention limit |
| `agentruntime_runs_persistence_enabled` | `agentruntime.runs.persistence_enabled` | file |  | storage path or retention limit |
| `agentruntime_channels_persistence_enabled` | `agentruntime.channels.persistence_enabled` | file |  | storage path or retention limit |
| `agentruntime_runs_max_records` | `agentruntime.runs.max_records` | file |  | storage path or retention limit |
| `agentruntime_channels_max_messages_per_channel` | `agentruntime.channels.max_messages_per_channel` | file |  | storage path or retention limit |
| `agentruntime_subagents_max_threads` | `agentruntime.subagents.max_threads` | advanced | `agentruntime.enabled` = true |  |
| `agentruntime_subagents_max_depth` | `agentruntime.subagents.max_depth` | advanced | `agentruntime.enabled` = true |  |
| `agentruntime_consensus_enabled` | `agentruntime.consensus.enabled` | advanced | `agentruntime.enabled` = true |  |
| `agentruntime_consensus_max_fanout` | `agentruntime.consensus.max_fanout` | advanced | `agentruntime.consensus.enabled` = true |  |
| `agentruntime_consensus_budget_tokens` | `agentruntime.consensus.budget_tokens` | file |  | internal tuning value |
| `agentruntime_consensus_budget_usd` | `agentruntime.consensus.budget_usd` | advanced | `agentruntime.consensus.enabled` = true |  |
| `agentruntime_consensus_timeout_seconds` | `agentruntime.consensus.timeout_seconds` | file |  | internal tuning value |
| `agentruntime_consensus_allowed_aliases_json` | `agentruntime.consensus.allowed_aliases` | file |  | provider alias allowlist for an opt-in mode |
| `agentruntime_consensus_concurrent_runs` | `agentruntime.consensus.concurrent_runs` | file |  | internal tuning value |
| `agentruntime_restore_on_startup` | `agentruntime.restore_on_startup` | file |  | storage path or retention limit |
| `agentruntime_report_summary_enabled` | `agentruntime.report.summary_enabled` | file |  | internal tuning value |
| `agentruntime_archive_enabled` | `agentruntime.archive.enabled` | file |  | storage path or retention limit |
| `agentruntime_archive_dir` | `agentruntime.archive.dir` | file |  | storage path or retention limit |
| `agentruntime_archive_retention_days` | `agentruntime.archive.retention_days` | file |  | storage path or retention limit |
| `agentruntime_archive_max_file_bytes` | `agentruntime.archive.max_file_bytes` | file |  | storage path or retention limit |

## Work Ledger

| Key | YAML path | Tier | Shown when | Why |
|---|---|---|---|---|
| `work_ledger_enabled` | `work_ledger.enabled` | advanced |  |  |

## Work Scheduler

| Key | YAML path | Tier | Shown when | Why |
|---|---|---|---|---|
| `work_scheduler_enabled` | `work_ledger.scheduler.enabled` | advanced | `work_ledger.enabled` = true |  |
| `work_scheduler_max_workers` | `work_ledger.scheduler.max_workers` | advanced | `work_ledger.scheduler.enabled` = true |  |
| `work_scheduler_lease_seconds` | `work_ledger.scheduler.lease_seconds` | file |  | internal tuning value |
| `work_scheduler_heartbeat_seconds` | `work_ledger.scheduler.heartbeat_seconds` | file |  | internal tuning value |
| `work_scheduler_poll_milliseconds` | `work_ledger.scheduler.poll_milliseconds` | file |  | internal tuning value |

## Channels

| Key | YAML path | Tier | Shown when | Why |
|---|---|---|---|---|
| `channels_local_enabled` | `channels.local.enabled` | file |  | agent runtime's local CLI channel |
| `channels_webhook_enabled` | `channels.webhook.enabled` | advanced |  |  |
| `channels_telegram_enabled` | `channels.telegram.enabled` | basic |  |  |
| `channels_telegram_dm_policy` | `channels.telegram.dm_policy` | advanced | `channels.telegram.enabled` = true |  |
| `channels_telegram_polling_enabled` | `channels.telegram.polling.enabled` | advanced | `channels.telegram.enabled` = true |  |
| `telegram_bot_token` | `channels.telegram.bot_token` | basic | `channels.telegram.enabled` = true |  |

## Console

| Key | YAML path | Tier | Shown when | Why |
|---|---|---|---|---|
| `console_default_mode` | `console.default_mode` | basic |  |  |

## Companion

| Key | YAML path | Tier | Shown when | Why |
|---|---|---|---|---|
| `companion_enabled` | `companion.enabled` | basic |  |  |

## Embodiment

| Key | YAML path | Tier | Shown when | Why |
|---|---|---|---|---|
| `embodiment_enabled` | `embodiment.enabled` | file |  | dormant subsystem |
| `embodiment_providers_json` | `embodiment.providers` | file |  | dormant subsystem |

## Initiative

| Key | YAML path | Tier | Shown when | Why |
|---|---|---|---|---|
| `initiative_enabled` | `initiative.enabled` | basic |  |  |
| `initiative_mode` | `initiative.mode` | basic | `initiative.enabled` = true |  |
| `initiative_backend` | `initiative.backend` | advanced | `initiative.enabled` = true |  |
| `initiative_tick` | `initiative.tick` | file |  | internal tuning value |
| `initiative_quiet_hours` | `initiative.quiet_hours` | basic | `initiative.enabled` = true |  |
| `initiative_timezone` | `initiative.timezone` | advanced | `initiative.enabled` = true |  |
| `initiative_daily_cap` | `initiative.daily_cap` | advanced | `initiative.enabled` = true |  |
| `initiative_cooldown` | `initiative.cooldown` | advanced | `initiative.enabled` = true |  |
| `initiative_body_provider` | `initiative.body_provider` | file |  | needs the dormant embodiment subsystem |
| `initiative_daily_text_calls` | `initiative.daily_text_calls` | advanced | `initiative.enabled` = true |  |
| `initiative_daily_speak_calls` | `initiative.daily_speak_calls` | advanced | `initiative.mode` = live |  |
| `initiative_reply_window` | `initiative.reply_window` | file |  | internal tuning value |
| `initiative_telegram` | `initiative.telegram` | advanced | `initiative.mode` = live |  |
| `initiative_threshold_quiet_requested` | `initiative.thresholds.quiet_requested` | file |  | only for initiative.backend: jev, whose probabilities the default llm backend does not produce |
| `initiative_threshold_user_strained` | `initiative.thresholds.user_strained` | file |  | only for initiative.backend: jev, whose probabilities the default llm backend does not produce |
| `initiative_threshold_special_day` | `initiative.thresholds.special_day` | file |  | only for initiative.backend: jev, whose probabilities the default llm backend does not produce |
| `jev_base_url` | `jev.base_url` | advanced | `initiative.backend` = jev or `tools.computer_use.backend` = jev |  |
| `jev_api_key` | `jev.api_key` | advanced | `initiative.backend` = jev or `tools.computer_use.backend` = jev |  |
| `jev_model` | `jev.model` | advanced | `initiative.backend` = jev or `tools.computer_use.backend` = jev |  |
| `jev_timeout_seconds` | `jev.timeout_seconds` | file |  | internal tuning value |

## Extensions

| Key | YAML path | Tier | Shown when | Why |
|---|---|---|---|---|
| `skills_enabled` | `extensions.skills.enabled` | advanced |  |  |
| `skills_watch` | `extensions.skills.watch` | file |  | file-watch reload knob |
| `skills_watch_debounce_ms` | `extensions.skills.watch_debounce_ms` | file |  | file-watch reload knob |
| `skills_extra_dirs_json` | `extensions.skills.extra_dirs` | advanced |  |  |
| `skills_bundled_dir` | `extensions.skills.bundled_dir` | file |  | storage path or retention limit |
| `plugins_enabled` | `extensions.plugins.enabled` | advanced |  |  |
| `plugins_watch` | `extensions.plugins.watch` | file |  | file-watch reload knob |
| `plugins_watch_debounce_ms` | `extensions.plugins.watch_debounce_ms` | file |  | file-watch reload knob |
| `plugins_extra_dirs_json` | `extensions.plugins.extra_dirs` | advanced |  |  |
| `plugins_bundled_dir` | `extensions.plugins.bundled_dir` | file |  | storage path or retention limit |
| `plugins_allow_mcp_servers` | `extensions.plugins.allow_mcp_servers` | advanced |  |  |
