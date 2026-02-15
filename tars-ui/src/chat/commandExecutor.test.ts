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
	exited: boolean;
} {
	const errors: string[] = [];
	const messages: string[] = [];
	const tables: Array<{headers: string[]; rows: string[][]}> = [];

	let resumeCandidates: SessionSummary[] | null = null;
	let resumeIndex = -1;
	let activeSessionID = sessionID;
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
		listCronJobs: async () => [{id: 'job_1', name: 'morning', schedule: 'every:1h', enabled: true, delete_after_run: false}],
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
		createCronJob: async () => ({id: 'job_2', name: 'nightly', schedule: 'every:30m', enabled: true, delete_after_run: false}),
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
});
