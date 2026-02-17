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
	last_reload_at?: string;
	last_restart_at?: string;
};
