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

