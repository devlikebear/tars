import test from 'node:test';
import assert from 'node:assert/strict';
import {CommandAPIs, CommandExecutorContext, executeInputCommand} from './commandExecutor.js';
import {SessionSummary} from '../types.js';

function createDefaultAPIs(): CommandAPIs {
	const unexpected = async (): Promise<never> => {
		throw new Error('unexpected api call');
	};
	return {
		listSessions: unexpected,
		createSession: unexpected,
		getSession: unexpected,
		getHistory: unexpected,
		exportSession: unexpected,
		searchSessions: unexpected,
		getStatus: unexpected,
		runCompact: unexpected,
		runHeartbeatOnce: unexpected,
		listCronJobs: unexpected,
		createCronJob: unexpected,
		updateCronJob: unexpected,
		runCronJob: unexpected,
		deleteCronJob: unexpected,
	};
}

function createContext(raw: string, sessionID = ''): {
	ctx: CommandExecutorContext;
	errors: string[];
	messages: string[];
	tables: Array<{headers: string[]; rows: string[][]}>;
	resumeCandidates: SessionSummary[] | null;
	resumeIndex: number;
	activeSessionID: string;
	notificationFilter: string;
	notifications: Array<{id: number; category: string; severity: string; title: string; message: string; timestamp: string}>;
	markNotificationsSeenCalls: number;
	exited: boolean;
} {
	const errors: string[] = [];
	const messages: string[] = [];
	const tables: Array<{headers: string[]; rows: string[][]}> = [];

	let resumeCandidates: SessionSummary[] | null = null;
	let resumeIndex = -1;
	let activeSessionID = sessionID;
	let notificationFilter = 'all';
	let notifications: Array<{id: number; category: string; severity: string; title: string; message: string; timestamp: string}> = [];
	let markNotificationsSeenCalls = 0;
	let exited = false;

	const ctx: CommandExecutorContext = {
		raw,
		serverUrl: 'http://127.0.0.1:8080',
		sessionID,
		pushSystemMessage: (text) => messages.push(text),
		pushSystemTable: (headers, rows) => tables.push({headers, rows}),
		pushErrorMessage: (text) => errors.push(text),
		setSessionID: (id) => {
			activeSessionID = id;
		},
		setResumeCandidates: (sessions) => {
			resumeCandidates = sessions;
		},
		setResumeIndex: (index) => {
			resumeIndex = index;
		},
		setNotificationFilter: (next) => {
			notificationFilter = next;
		},
		getNotificationFilter: () => notificationFilter as 'all' | 'cron' | 'heartbeat' | 'error',
		getNotifications: () => notifications,
		clearNotifications: () => {
			notifications = [];
		},
		markNotificationsSeen: () => {
			markNotificationsSeenCalls++;
		},
		exit: () => {
			exited = true;
		},
	};

	return {
		ctx,
		errors,
		messages,
		tables,
		get resumeCandidates() {
			return resumeCandidates;
		},
		get resumeIndex() {
			return resumeIndex;
		},
		get activeSessionID() {
			return activeSessionID;
		},
		get notificationFilter() {
			return notificationFilter;
		},
		set notifications(value) {
			notifications = value;
		},
		get notifications() {
			return notifications;
		},
		get markNotificationsSeenCalls() {
			return markNotificationsSeenCalls;
		},
		get exited() {
			return exited;
		},
	};
}

test('executeInputCommand returns invalid command usage via error message', async () => {
	const state = createContext('/search');
	await executeInputCommand(state.ctx, createDefaultAPIs());

	assert.deepEqual(state.errors, ['usage: /search {keyword}']);
	assert.deepEqual(state.messages, []);
	assert.deepEqual(state.tables, []);
});

test('executeInputCommand handles /quit by exiting', async () => {
	const state = createContext('/quit');
	await executeInputCommand(state.ctx, createDefaultAPIs());

	assert.equal(state.exited, true);
});

test('executeInputCommand handles /resume without sessions list', async () => {
	const apis: CommandAPIs = {
		...createDefaultAPIs(),
		listSessions: async () => [],
	};
	const state = createContext('/resume');
	await executeInputCommand(state.ctx, apis);

	assert.deepEqual(state.messages, ['(no sessions)']);
	assert.equal(state.resumeCandidates, null);
	assert.equal(state.resumeIndex, -1);
});

test('executeInputCommand handles /resume with selection mode', async () => {
	const sessions = [
		{id: 'sess-1', title: 'first'},
		{id: 'sess-2', title: 'second'},
	];
	const apis: CommandAPIs = {
		...createDefaultAPIs(),
		listSessions: async () => sessions,
	};
	const state = createContext('/resume');
	await executeInputCommand(state.ctx, apis);

	assert.deepEqual(state.messages, ['Use ↑/↓ and Enter to select a session.']);
	assert.deepEqual(state.resumeCandidates, sessions);
	assert.equal(state.resumeIndex, 0);
});

test('executeInputCommand blocks history when session is missing', async () => {
	const state = createContext('/history', '');
	await executeInputCommand(state.ctx, createDefaultAPIs());

	assert.deepEqual(state.errors, ['no active session. use /new or /resume {session_id}']);
});

test('executeInputCommand handles /new and updates active session', async () => {
	const apis: CommandAPIs = {
		...createDefaultAPIs(),
		createSession: async () => ({id: 'sess-10', title: 'chat'}),
	};
	const state = createContext('/new hello');
	await executeInputCommand(state.ctx, apis);

	assert.equal(state.activeSessionID, 'sess-10');
	assert.deepEqual(state.messages, ['active session: sess-10']);
});

test('executeInputCommand clears resume selection when creating a new session', async () => {
	const apis: CommandAPIs = {
		...createDefaultAPIs(),
		createSession: async () => ({id: 'sess-11', title: 'new chat'}),
	};
	const state = createContext('/new');
	state.ctx.setResumeCandidates([
		{id: 'sess-old-1', title: 'old one'},
		{id: 'sess-old-2', title: 'old two'},
	]);
	state.ctx.setResumeIndex(1);

	await executeInputCommand(state.ctx, apis);

	assert.equal(state.activeSessionID, 'sess-11');
	assert.equal(state.resumeCandidates, null);
	assert.equal(state.resumeIndex, 0);
	assert.deepEqual(state.messages, ['active session: sess-11']);
});

test('executeInputCommand handles /cron list', async () => {
	const apis: CommandAPIs = {
		...createDefaultAPIs(),
		listCronJobs: async () => [{id: 'job_1', name: 'morning', prompt: 'ping', schedule: 'every:1h', enabled: true, delete_after_run: false}],
	};
	const state = createContext('/cron list');
	await executeInputCommand(state.ctx, apis);

	assert.equal(state.tables.length, 1);
	assert.deepEqual(state.tables[0]?.headers, ['ID', 'NAME', 'SCHEDULE', 'ENABLED']);
	assert.deepEqual(state.tables[0]?.rows[0], ['job_1', 'morning', 'every:1h', 'yes']);
});

test('executeInputCommand handles /cron add and /cron run /cron delete', async () => {
	const apis: CommandAPIs = {
		...createDefaultAPIs(),
		createCronJob: async () => ({id: 'job_2', name: 'nightly', prompt: 'check mail', schedule: 'every:30m', enabled: true, delete_after_run: false}),
		updateCronJob: async () => ({id: 'job_2', name: 'nightly', prompt: 'check mail', schedule: 'every:30m', enabled: true, delete_after_run: false}),
		runCronJob: async () => 'ran',
		deleteCronJob: async () => undefined,
	};
	const addState = createContext('/cron add every:30m check mail');
	await executeInputCommand(addState.ctx, apis);
	assert.deepEqual(addState.messages, ['cron job created: job_2']);

	const runState = createContext('/cron run job_2');
	await executeInputCommand(runState.ctx, apis);
	assert.deepEqual(runState.messages, ['ran']);

	const delState = createContext('/cron delete job_2');
	await executeInputCommand(delState.ctx, apis);
	assert.deepEqual(delState.messages, ['cron job deleted: job_2']);

	const enableState = createContext('/cron enable job_2');
	await executeInputCommand(enableState.ctx, apis);
	assert.deepEqual(enableState.messages, ['cron job enabled: job_2']);

	const disableState = createContext('/cron disable job_2');
	await executeInputCommand(disableState.ctx, apis);
	assert.deepEqual(disableState.messages, ['cron job disabled: job_2']);
});

test('executeInputCommand handles /notify list/filter/open/clear', async () => {
	const state = createContext('/notify list');
	state.notifications = [
		{id: 1, category: 'cron', severity: 'info', title: 'Cron done', message: 'job complete', timestamp: '2026-02-16T10:00:00Z'},
		{id: 2, category: 'heartbeat', severity: 'error', title: 'Heartbeat failed', message: 'auth failed', timestamp: '2026-02-16T10:01:00Z'},
	];

	await executeInputCommand(state.ctx, createDefaultAPIs());
	assert.equal(state.tables.length, 1);
	assert.deepEqual(state.tables[0]?.headers, ['#', 'CATEGORY', 'SEVERITY', 'TITLE', 'TIME']);
	assert.equal(state.markNotificationsSeenCalls, 1);

	const filterState = createContext('/notify filter cron');
	await executeInputCommand(filterState.ctx, createDefaultAPIs());
	assert.equal(filterState.notificationFilter, 'cron');
	assert.deepEqual(filterState.messages, ['notification filter: cron']);

	const openState = createContext('/notify open 2');
	openState.notifications = state.notifications;
	await executeInputCommand(openState.ctx, createDefaultAPIs());
	assert.equal(openState.messages.length, 1);
	assert.equal(openState.messages[0]?.includes('Heartbeat failed'), true);
	assert.equal(openState.markNotificationsSeenCalls, 1);

	const clearState = createContext('/notify clear');
	clearState.notifications = state.notifications;
	await executeInputCommand(clearState.ctx, createDefaultAPIs());
	assert.equal(clearState.notifications.length, 0);
	assert.deepEqual(clearState.messages, ['notifications cleared']);
	assert.equal(clearState.markNotificationsSeenCalls, 1);
});

test('executeInputCommand notify filter error uses severity', async () => {
	const state = createContext('/notify filter error');
	state.notifications = [
		{id: 1, category: 'cron', severity: 'info', title: 'Cron done', message: 'ok', timestamp: '2026-02-16T10:00:00Z'},
		{id: 2, category: 'heartbeat', severity: 'error', title: 'Heartbeat failed', message: 'down', timestamp: '2026-02-16T10:01:00Z'},
	];
	await executeInputCommand(state.ctx, createDefaultAPIs());
	assert.equal(state.notificationFilter, 'error');

	state.ctx.raw = '/notify list';
	await executeInputCommand(state.ctx, createDefaultAPIs());
	assert.equal(state.tables.length, 1);
	assert.equal(state.tables[0]?.rows.length, 1);
	assert.equal(state.tables[0]?.rows[0]?.[2], 'error');
});
