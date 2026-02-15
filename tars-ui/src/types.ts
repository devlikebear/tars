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
	last_run_at?: string;
	last_run_error?: string;
};
