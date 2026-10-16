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

| YAML path | Tier | Shown when | Why |
|---|---|---|---|
| `runtime.workspace_dir` | advanced |  |  |
| `runtime.plan_clarify_mode` | basic |  |  |
| `runtime.session.default_id` | file |  | rarely changed after init |
| `runtime.session.auto_archive_days` | basic |  |  |
| `runtime.session.auto_delete_days` | basic |  |  |
| `runtime.session.telegram_scope` | advanced | `channels.telegram.enabled` = true |  |
| `runtime.style.directness_default` | advanced |  |  |
| `runtime.style.humor_default` | advanced |  |  |
| `runtime.style.caution_default` | advanced |  |  |
| `runtime.style.autonomy_default` | advanced |  |  |
| `log.level` | advanced |  |  |
| `log.file` | advanced |  |  |
| `log.rotate.max_size_mb` | file |  | storage path or retention limit |
| `log.rotate.max_days` | file |  | storage path or retention limit |
| `log.rotate.max_backups` | file |  | storage path or retention limit |

## API

| YAML path | Tier | Shown when | Why |
|---|---|---|---|
| `api.auth_mode` | basic |  |  |
| `api.dashboard.auth_mode` | advanced | `api.auth_mode` = required / external-required |  |
| `api.auth_token` | file |  | legacy single token, replaced by the user and admin tokens; still read by the auth middleware |
| `api.user_token` | basic | `api.auth_mode` = required / external-required |  |
| `api.admin_token` | basic | `api.auth_mode` = required / external-required |  |
| `api.allow_insecure_local_auth` | advanced | `api.auth_mode` = required / external-required |  |
| `api.max_inflight.chat` | file |  | internal tuning value |
| `api.max_inflight.agent_runs` | file |  | internal tuning value |

## Remote Access

| YAML path | Tier | Shown when | Why |
|---|---|---|---|
| `remote_access.tailscale_serve.enabled` | basic |  |  |
| `remote_access.tailscale_serve.https_port` | advanced | `remote_access.tailscale_serve.enabled` = true |  |

## LLM

| YAML path | Tier | Shown when | Why |
|---|---|---|---|
| `llm.providers` | basic |  |  |
| `llm.tiers` | basic |  |  |
| `llm.default_tier` | basic |  |  |
| `llm.role_defaults` | advanced |  |  |

## Memory

| YAML path | Tier | Shown when | Why |
|---|---|---|---|
| `memory.semantic.enabled` | basic |  |  |
| `memory.embed.provider` | basic | `memory.semantic.enabled` = true |  |
| `memory.embed.base_url` | advanced | `memory.semantic.enabled` = true |  |
| `memory.embed.api_key` | basic | `memory.semantic.enabled` = true |  |
| `memory.embed.model` | advanced | `memory.semantic.enabled` = true |  |
| `memory.embed.dimensions` | advanced | `memory.semantic.enabled` = true |  |

## Usage

| YAML path | Tier | Shown when | Why |
|---|---|---|---|
| `usage.limits.daily_usd` | basic |  |  |
| `usage.limits.weekly_usd` | basic |  |  |
| `usage.limits.monthly_usd` | basic |  |  |
| `usage.limits.daily_tokens` | advanced |  |  |
| `usage.limits.mode` | basic |  |  |
| `usage.price_overrides` | file |  | per-model price map, edited as YAML |

## Automation

| YAML path | Tier | Shown when | Why |
|---|---|---|---|
| `automation.agent.max_iterations` | advanced |  |  |
| `automation.cron.run_history_limit` | file |  | storage path or retention limit |
| `automation.pulse.enabled` | advanced |  |  |
| `automation.pulse.interval` | file |  | internal tuning value |
| `automation.pulse.timeout` | file |  | only used by the opt-in llm decider |
| `automation.pulse.active_hours` | advanced | `automation.pulse.enabled` = true |  |
| `automation.pulse.timezone` | advanced | `automation.pulse.enabled` = true |  |
| `automation.pulse.min_severity` | advanced | `automation.pulse.enabled` = true |  |
| `automation.pulse.decider` | advanced | `automation.pulse.enabled` = true |  |
| `automation.pulse.allowed_autofixes` | advanced | `automation.pulse.enabled` = true |  |
| `automation.pulse.notify_telegram` | advanced | `automation.pulse.enabled` = true |  |
| `automation.pulse.notify_session_events` | advanced | `automation.pulse.enabled` = true |  |
| `automation.pulse.cron_failure_threshold` | file |  | internal tuning value |
| `automation.pulse.stuck_run_minutes` | file |  | internal tuning value |
| `automation.pulse.disk_warn_percent` | file |  | internal tuning value |
| `automation.pulse.disk_critical_percent` | file |  | internal tuning value |
| `automation.pulse.delivery_failure_threshold` | file |  | internal tuning value |
| `automation.pulse.delivery_failure_window` | file |  | internal tuning value |
| `automation.pulse.reflection_failure_threshold` | file |  | internal tuning value |
| `automation.reflection.enabled` | advanced |  |  |
| `automation.reflection.sleep_window` | advanced | `automation.reflection.enabled` = true |  |
| `automation.reflection.timezone` | advanced | `automation.reflection.enabled` = true |  |
| `automation.reflection.tick_interval` | file |  | internal tuning value |
| `automation.reflection.empty_session_age` | file |  | internal tuning value |
| `automation.reflection.memory_lookback_hours` | file |  | internal tuning value |
| `automation.reflection.max_turns_per_session` | file |  | internal tuning value |
| `automation.notify.command` | advanced |  |  |
| `automation.notify.when_no_clients` | advanced |  |  |

## Compaction

| YAML path | Tier | Shown when | Why |
|---|---|---|---|
| `compaction.trigger_tokens` | advanced |  |  |
| `compaction.keep_recent_tokens` | file |  | internal tuning value |
| `compaction.keep_recent_fraction` | file |  | internal tuning value |
| `compaction.llm_mode` | advanced |  |  |
| `compaction.llm_timeout_seconds` | file |  | internal tuning value |

## Tools

| YAML path | Tier | Shown when | Why |
|---|---|---|---|
| `tools.web_search.enabled` | basic |  |  |
| `tools.web_fetch.enabled` | basic |  |  |
| `tools.default_set` | advanced |  |  |
| `tools.allow_high_risk_user` | advanced |  |  |
| `tools.web_search.api_key` | basic | `tools.web_search.provider` = brave |  |
| `tools.web_search.provider` | basic | `tools.web_search.enabled` = true |  |
| `tools.web_search.perplexity.api_key` | basic | `tools.web_search.provider` = perplexity |  |
| `tools.web_search.perplexity.model` | advanced | `tools.web_search.provider` = perplexity |  |
| `tools.web_search.perplexity.base_url` | advanced | `tools.web_search.provider` = perplexity |  |
| `tools.web_search.cache_ttl_seconds` | file |  | internal tuning value |
| `tools.exec.max_timeout_ms` | advanced |  |  |
| `tools.process.max_timeout_ms` | advanced |  |  |
| `tools.web_fetch.allow_private_hosts` | advanced | `tools.web_fetch.enabled` = true |  |
| `tools.web_fetch.private_host_allowlist` | advanced | `tools.web_fetch.enabled` = true |  |
| `tools.apply_patch.enabled` | advanced |  |  |
| `tools.message.enabled` | advanced |  |  |
| `tools.agentruntime.enabled` | advanced |  |  |
| `tools.computer_use.backend` | advanced | `tools.computer_use.enabled` = true |  |
| `tools.computer_use.enabled` | basic |  |  |
| `tools.computer_use.cua_driver_path` | advanced | `tools.computer_use.enabled` = true |  |
| `tools.computer_use.max_steps` | advanced | `tools.computer_use.enabled` = true |  |
| `tools.computer_use.step_timeout_seconds` | file |  | internal tuning value |
| `tools.computer_use.total_timeout_seconds` | advanced | `tools.computer_use.enabled` = true |  |
| `tools.computer_use.expose_values` | advanced | `tools.computer_use.enabled` = true |  |

## MCP

| YAML path | Tier | Shown when | Why |
|---|---|---|---|
| `extensions.mcp.command_allowlist` | advanced |  |  |
| `extensions.mcp.servers` | advanced |  |  |

## Agent Runtime

| YAML path | Tier | Shown when | Why |
|---|---|---|---|
| `agentruntime.enabled` | advanced |  |  |
| `agentruntime.default_agent` | advanced | `agentruntime.enabled` = true |  |
| `agentruntime.agents.list` | file |  | agent catalog, edited as YAML |
| `agentruntime.task_override` | file |  | provider/model allowlist, edited as YAML |
| `agentruntime.agents.watch` | file |  | file-watch reload knob |
| `agentruntime.agents.watch_debounce_ms` | file |  | file-watch reload knob |
| `agentruntime.persistence.enabled` | file |  | storage path or retention limit |
| `agentruntime.persistence.dir` | file |  | storage path or retention limit |
| `agentruntime.runs.persistence_enabled` | file |  | storage path or retention limit |
| `agentruntime.channels.persistence_enabled` | file |  | storage path or retention limit |
| `agentruntime.runs.max_records` | file |  | storage path or retention limit |
| `agentruntime.channels.max_messages_per_channel` | file |  | storage path or retention limit |
| `agentruntime.subagents.max_threads` | advanced | `agentruntime.enabled` = true |  |
| `agentruntime.subagents.max_depth` | advanced | `agentruntime.enabled` = true |  |
| `agentruntime.consensus.enabled` | advanced | `agentruntime.enabled` = true |  |
| `agentruntime.consensus.max_fanout` | advanced | `agentruntime.consensus.enabled` = true |  |
| `agentruntime.consensus.budget_tokens` | file |  | internal tuning value |
| `agentruntime.consensus.budget_usd` | advanced | `agentruntime.consensus.enabled` = true |  |
| `agentruntime.consensus.timeout_seconds` | file |  | internal tuning value |
| `agentruntime.consensus.allowed_aliases` | file |  | provider alias allowlist for an opt-in mode |
| `agentruntime.consensus.concurrent_runs` | file |  | internal tuning value |
| `agentruntime.restore_on_startup` | file |  | storage path or retention limit |
| `agentruntime.report.summary_enabled` | file |  | internal tuning value |
| `agentruntime.archive.enabled` | file |  | storage path or retention limit |
| `agentruntime.archive.dir` | file |  | storage path or retention limit |
| `agentruntime.archive.retention_days` | file |  | storage path or retention limit |
| `agentruntime.archive.max_file_bytes` | file |  | storage path or retention limit |

## Work Ledger

| YAML path | Tier | Shown when | Why |
|---|---|---|---|
| `work_ledger.enabled` | advanced |  |  |

## Work Scheduler

| YAML path | Tier | Shown when | Why |
|---|---|---|---|
| `work_ledger.scheduler.enabled` | advanced | `work_ledger.enabled` = true |  |
| `work_ledger.scheduler.max_workers` | advanced | `work_ledger.scheduler.enabled` = true |  |
| `work_ledger.scheduler.lease_seconds` | file |  | internal tuning value |
| `work_ledger.scheduler.heartbeat_seconds` | file |  | internal tuning value |
| `work_ledger.scheduler.poll_milliseconds` | file |  | internal tuning value |

## Channels

| YAML path | Tier | Shown when | Why |
|---|---|---|---|
| `channels.local.enabled` | file |  | agent runtime's local CLI channel |
| `channels.webhook.enabled` | advanced |  |  |
| `channels.telegram.enabled` | basic |  |  |
| `channels.telegram.dm_policy` | advanced | `channels.telegram.enabled` = true |  |
| `channels.telegram.polling.enabled` | advanced | `channels.telegram.enabled` = true |  |
| `channels.telegram.bot_token` | basic | `channels.telegram.enabled` = true |  |

## Console

| YAML path | Tier | Shown when | Why |
|---|---|---|---|
| `console.default_mode` | basic |  |  |

## Companion

| YAML path | Tier | Shown when | Why |
|---|---|---|---|
| `companion.enabled` | basic |  |  |

## Embodiment

| YAML path | Tier | Shown when | Why |
|---|---|---|---|
| `embodiment.enabled` | file |  | dormant subsystem |
| `embodiment.providers` | file |  | dormant subsystem |

## Initiative

| YAML path | Tier | Shown when | Why |
|---|---|---|---|
| `initiative.enabled` | basic |  |  |
| `initiative.mode` | basic | `initiative.enabled` = true |  |
| `initiative.backend` | advanced | `initiative.enabled` = true |  |
| `initiative.tick` | file |  | internal tuning value |
| `initiative.quiet_hours` | basic | `initiative.enabled` = true |  |
| `initiative.timezone` | advanced | `initiative.enabled` = true |  |
| `initiative.daily_cap` | advanced | `initiative.enabled` = true |  |
| `initiative.cooldown` | advanced | `initiative.enabled` = true |  |
| `initiative.body_provider` | file |  | needs the dormant embodiment subsystem |
| `initiative.daily_text_calls` | advanced | `initiative.enabled` = true |  |
| `initiative.daily_speak_calls` | advanced | `initiative.mode` = live |  |
| `initiative.reply_window` | file |  | internal tuning value |
| `initiative.telegram` | advanced | `initiative.mode` = live |  |
| `initiative.thresholds.quiet_requested` | file |  | only for initiative.backend: jev, whose probabilities the default llm backend does not produce |
| `initiative.thresholds.user_strained` | file |  | only for initiative.backend: jev, whose probabilities the default llm backend does not produce |
| `initiative.thresholds.special_day` | file |  | only for initiative.backend: jev, whose probabilities the default llm backend does not produce |
| `jev.base_url` | advanced | `initiative.backend` = jev or `tools.computer_use.backend` = jev |  |
| `jev.api_key` | advanced | `initiative.backend` = jev or `tools.computer_use.backend` = jev |  |
| `jev.model` | advanced | `initiative.backend` = jev or `tools.computer_use.backend` = jev |  |
| `jev.timeout_seconds` | file |  | internal tuning value |

## Extensions

| YAML path | Tier | Shown when | Why |
|---|---|---|---|
| `extensions.skills.enabled` | advanced |  |  |
| `extensions.skills.watch` | file |  | file-watch reload knob |
| `extensions.skills.watch_debounce_ms` | file |  | file-watch reload knob |
| `extensions.skills.extra_dirs` | advanced |  |  |
| `extensions.skills.bundled_dir` | file |  | storage path or retention limit |
| `extensions.plugins.enabled` | advanced |  |  |
| `extensions.plugins.watch` | file |  | file-watch reload knob |
| `extensions.plugins.watch_debounce_ms` | file |  | file-watch reload knob |
| `extensions.plugins.extra_dirs` | advanced |  |  |
| `extensions.plugins.bundled_dir` | file |  | storage path or retention limit |
| `extensions.plugins.allow_mcp_servers` | advanced |  |  |
