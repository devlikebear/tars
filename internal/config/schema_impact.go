package config

var configFieldImpactHints = map[string][]string{
	"log_level": {
		"Debug logging can increase log volume and expose more operational detail.",
	},
	"api_auth_mode": {
		"Changing API auth mode affects access to /v1/* endpoints.",
		"Stricter modes may require valid bearer tokens immediately after restart.",
	},
	"dashboard_auth_mode": {
		"Changing dashboard auth mode affects browser access to the console.",
	},
	"api_max_inflight_chat": {
		"Higher concurrency can improve throughput but increases peak provider and CPU load.",
	},
	"usage_limit_daily_usd": {
		"Lower daily spend limits can stop or degrade LLM calls sooner.",
	},
	"usage_limit_mode": {
		"Hard mode can block requests when usage limits are reached.",
	},
	"memory_semantic_enabled": {
		"Semantic memory changes recall behavior and embedding API usage.",
	},
	"agent_max_iterations": {
		"Higher iteration limits allow longer agent loops and can increase cost.",
	},
	"pulse_interval": {
		"Changing this changes the pulse tick cadence for watchdog checks.",
		"Shorter intervals can raise CPU use and pulse log volume.",
	},
	"pulse_timeout": {
		"Shorter timeouts can reduce stuck checks but may skip slow LLM decisions.",
	},
	"pulse_min_severity": {
		"Raising the floor reduces notification noise; lowering it surfaces more signals.",
	},
	"pulse_disk_warn_percent": {
		"Lower thresholds make disk pressure warnings fire earlier.",
	},
	"pulse_disk_critical_percent": {
		"Lower thresholds make critical disk pressure alerts fire earlier.",
	},
	"reflection_tick_interval": {
		"Changing this affects how often the nightly reflection scheduler checks its window.",
	},
	"reflection_memory_lookback_hours": {
		"Higher lookback scans more history and can increase reflection runtime.",
	},
	"cron_run_history_limit": {
		"Higher limits retain more cron history and use more workspace storage.",
	},
	"tools_web_fetch_allow_private_hosts": {
		"Allowing private hosts expands the network surface available to web fetch tools.",
	},
	"agentruntime_subagents_max_threads": {
		"Higher thread counts can run more subagents in parallel and increase peak cost.",
	},
	"agentruntime_consensus_max_fanout": {
		"Higher fanout improves comparison breadth but increases parallel LLM calls.",
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
