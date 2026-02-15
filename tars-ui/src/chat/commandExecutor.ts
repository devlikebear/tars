import {createSession, exportSession, getHistory, getSession, listSessions, searchSessions} from '../api/session.js';
import {getStatus, runCompact, runHeartbeatOnce} from '../api/system.js';
import {parseInputCommand} from '../commands/router.js';
import {SessionHistoryItem, SessionSummary} from '../types.js';
import {commandHelpText, requireSessionOrError, truncate} from '../ui/format.js';

export type CommandAPIs = {
	listSessions: (serverUrl: string) => Promise<SessionSummary[]>;
	createSession: (serverUrl: string, title: string) => Promise<SessionSummary>;
	getSession: (serverUrl: string, sessionID: string) => Promise<SessionSummary>;
	getHistory: (serverUrl: string, sessionID: string) => Promise<SessionHistoryItem[]>;
	exportSession: (serverUrl: string, sessionID: string) => Promise<string>;
	searchSessions: (serverUrl: string, keyword: string) => Promise<SessionSummary[]>;
	getStatus: (serverUrl: string) => Promise<{workspace_dir: string; session_count: number}>;
	runCompact: (serverUrl: string, sessionID: string) => Promise<string>;
	runHeartbeatOnce: (serverUrl: string) => Promise<string>;
};

const defaultAPIs: CommandAPIs = {
	listSessions,
	createSession,
	getSession,
	getHistory,
	exportSession,
	searchSessions,
	getStatus,
	runCompact,
	runHeartbeatOnce,
};

export type CommandExecutorContext = {
	raw: string;
	serverUrl: string;
	sessionID: string;
	pushSystemMessage: (text: string) => void;
	pushSystemTable: (headers: string[], rows: string[][]) => void;
	pushErrorMessage: (text: string) => void;
	setSessionID: (sessionID: string) => void;
	setResumeCandidates: (sessions: SessionSummary[] | null) => void;
	setResumeIndex: (index: number) => void;
	exit: () => void;
};

function clearResumeSelection(ctx: CommandExecutorContext): void {
	ctx.setResumeCandidates(null);
	ctx.setResumeIndex(0);
}

function renderSessionRows(sessions: SessionSummary[]): string[][] {
	return sessions.map((session) => [session.id, truncate(session.title, 48)]);
}

function missingSessionError(sessionID: string): string | null {
	return requireSessionOrError(sessionID);
}

export async function executeInputCommand(ctx: CommandExecutorContext, apis: CommandAPIs = defaultAPIs): Promise<void> {
	const cmd = parseInputCommand(ctx.raw);

	switch (cmd.kind) {
	case 'noop':
		return;
	case 'chat':
		throw new Error('internal command router mismatch');
	case 'invalid':
		ctx.pushErrorMessage(cmd.message);
		return;
	case 'quit':
		ctx.exit();
		return;
	case 'help':
		ctx.pushSystemMessage(commandHelpText());
		return;
	case 'sessions': {
		const sessions = await apis.listSessions(ctx.serverUrl);
		if (sessions.length === 0) {
			ctx.pushSystemMessage('(no sessions)');
			return;
		}
		ctx.pushSystemTable(['ID', 'TITLE'], renderSessionRows(sessions));
		return;
	}
	case 'new': {
		const created = await apis.createSession(ctx.serverUrl, cmd.title);
		ctx.setSessionID(created.id);
		ctx.pushSystemMessage(`active session: ${created.id}`);
		return;
	}
	case 'resume': {
		await apis.getSession(ctx.serverUrl, cmd.sessionID);
		ctx.setSessionID(cmd.sessionID);
		ctx.pushSystemMessage(`resumed session: ${cmd.sessionID}`);
		clearResumeSelection(ctx);
		return;
	}
	case 'resume_select': {
		const sessions = await apis.listSessions(ctx.serverUrl);
		if (sessions.length === 0) {
			ctx.pushSystemMessage('(no sessions)');
			return;
		}
		ctx.setResumeCandidates(sessions);
		ctx.setResumeIndex(0);
		ctx.pushSystemMessage('Use ↑/↓ and Enter to select a session.');
		return;
	}
	case 'history': {
		const missing = missingSessionError(ctx.sessionID);
		if (missing !== null) {
			ctx.pushErrorMessage(missing);
			return;
		}
		const history = await apis.getHistory(ctx.serverUrl, ctx.sessionID);
		if (history.length === 0) {
			ctx.pushSystemMessage('(no history)');
			return;
		}
		ctx.pushSystemTable(
			['TIME', 'ROLE', 'CONTENT'],
			history.map((item) => [item.timestamp, item.role, truncate(item.content.replace(/\s+/g, ' '), 64)]),
		);
		return;
	}
	case 'export': {
		const missing = missingSessionError(ctx.sessionID);
		if (missing !== null) {
			ctx.pushErrorMessage(missing);
			return;
		}
		const markdown = await apis.exportSession(ctx.serverUrl, ctx.sessionID);
		ctx.pushSystemMessage(markdown);
		return;
	}
	case 'search': {
		const sessions = await apis.searchSessions(ctx.serverUrl, cmd.keyword);
		if (sessions.length === 0) {
			ctx.pushSystemMessage('(no sessions)');
			return;
		}
		ctx.pushSystemTable(['ID', 'TITLE'], renderSessionRows(sessions));
		return;
	}
	case 'status': {
		const status = await apis.getStatus(ctx.serverUrl);
		ctx.pushSystemMessage(`workspace=${status.workspace_dir} sessions=${status.session_count}`);
		return;
	}
	case 'compact': {
		const missing = missingSessionError(ctx.sessionID);
		if (missing !== null) {
			ctx.pushErrorMessage(missing);
			return;
		}
		const message = await apis.runCompact(ctx.serverUrl, ctx.sessionID);
		ctx.pushSystemMessage(message);
		return;
	}
	case 'heartbeat': {
		const response = await apis.runHeartbeatOnce(ctx.serverUrl);
		ctx.pushSystemMessage(response);
		return;
	}
	}
}
