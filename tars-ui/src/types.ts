export type ChatRole = 'user' | 'assistant' | 'error' | 'system';

export type ChatLine = {
	role: ChatRole;
	text: string;
};

export type SessionSummary = {
	id: string;
	title: string;
};

export type SessionHistoryItem = {
	role: string;
	content: string;
	timestamp: string;
};

export type CronJob = {
	id: string;
	name: string;
	prompt: string;
	schedule: string;
	enabled: boolean;
	delete_after_run?: boolean;
	session_target?: string;
	wake_mode?: string;
	delivery_mode?: string;
	last_run_at?: string;
	last_run_error?: string;
};

export type CronRunRecord = {
	job_id: string;
	ran_at: string;
	response?: string;
	error?: string;
};

export type NotificationFilter = 'all' | 'cron' | 'heartbeat' | 'error';

export type NotificationItem = {
	id: number;
	category: string;
	severity: string;
	title: string;
	message: string;
	timestamp: string;
};

export type SkillDefinition = {
	name: string;
	description: string;
	user_invocable: boolean;
	source: 'workspace' | 'user' | 'bundled' | string;
	file_path: string;
	runtime_path?: string;
};

export type PluginDefinition = {
	id: string;
	name?: string;
	description?: string;
	version?: string;
	source: 'workspace' | 'user' | 'bundled' | string;
	root_dir: string;
	manifest_path: string;
};

export type MCPServerStatus = {
	name: string;
	command: string;
	connected: boolean;
	tool_count: number;
	error?: string;
};

export type MCPToolInfo = {
	server: string;
	name: string;
	description?: string;
};

export type SentinelStatus = {
	enabled: boolean;
	supervision_state: 'starting' | 'running' | 'paused' | 'cooldown' | 'stopped' | 'error' | string;
	target: {
		command: string;
		args?: string[];
		cwd?: string;
	};
	target_pid?: number;
	target_started_at?: string;
	target_last_exit_at?: string;
	target_last_exit_code?: number;
	health_ok: boolean;
	health_last_ok_at?: string;
	health_last_error?: string;
	restart_attempt: number;
	restart_max_attempts: number;
	cooldown_until?: string;
	last_restart_at?: string;
	start_grace_until?: string;
	consecutive_failures?: number;
	last_probe_duration_ms?: number;
	event_persistence_enabled?: boolean;
	events_restored?: number;
	last_event_persist_at?: string;
	last_event_restore_at?: string;
	last_event_restore_error?: string;
	event_count: number;
};

export type SentinelEvent = {
	id: number;
	time: string;
	level: string;
	type: string;
	message: string;
	meta?: Record<string, unknown>;
};

export type AgentRunSummary = {
	run_id: string;
	session_id?: string;
	agent?: string;
	status: string;
	accepted: boolean;
	response?: string;
	error?: string;
	created_at?: string;
	started_at?: string;
	completed_at?: string;
};

export type AgentDescriptor = {
	name: string;
	description?: string;
	enabled?: boolean;
	kind?: string;
	source?: string;
	entry?: string;
	default?: boolean;
	policy_mode?: 'full' | 'allowlist' | string;
	tools_allow?: string[];
	tools_allow_count?: number;
	tools_allow_groups?: string[];
	tools_allow_patterns?: string[];
	session_routing_mode?: 'caller' | 'new' | 'fixed' | string;
	session_fixed_id?: string;
};

export type GatewayStatus = {
	enabled: boolean;
	version: number;
	runs_total: number;
	runs_active: number;
	agents_count: number;
	agents_watch_enabled: boolean;
	agents_reload_version: number;
	agents_last_reload_at?: string;
	channels_local_enabled: boolean;
	channels_webhook_enabled: boolean;
	channels_telegram_enabled: boolean;
	persistence_enabled: boolean;
	runs_persistence_enabled: boolean;
	channels_persistence_enabled: boolean;
	restore_on_startup: boolean;
	persistence_dir?: string;
	runs_restored: number;
	channels_restored: number;
	last_persist_at?: string;
	last_restore_at?: string;
	last_restore_error?: string;
	last_reload_at?: string;
	last_restart_at?: string;
};

export type GatewayReportSummary = {
	generated_at: string;
	summary_enabled: boolean;
	archive_enabled: boolean;
	runs_total: number;
	runs_active: number;
	runs_by_status: Record<string, number>;
	channels_total: number;
	messages_total: number;
	messages_by_source: Record<string, number>;
};

export type GatewayReportRuns = {
	generated_at: string;
	archive_enabled: boolean;
	count: number;
	runs: AgentRunSummary[];
};

export type GatewayReportChannels = {
	generated_at: string;
	archive_enabled: boolean;
	count: number;
	messages: Record<string, Array<{id: string; channel_id: string; source: string; direction: string; text: string; timestamp: string}>>;
};
