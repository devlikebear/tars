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
