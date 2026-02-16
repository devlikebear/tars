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
