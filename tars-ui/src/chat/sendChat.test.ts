import test from 'node:test';
import assert from 'node:assert/strict';
import {sendChatMessage, SendChatAPIs, SendChatContext} from './sendChat.js';
import {ChatSSEEvent} from '../api/chat.js';
import {ChatUIAction} from './state.js';

function createContext(message: string): {
	ctx: SendChatContext;
	actions: ChatUIAction[];
	statuses: string[];
	debugs: string[];
	statusEvents: ChatSSEEvent[];
	sessionID: string;
	clearedResume: boolean;
	scrollReset: boolean;
} {
	const actions: ChatUIAction[] = [];
	const statuses: string[] = [];
	const debugs: string[] = [];
	const statusEvents: ChatSSEEvent[] = [];
	let sessionID = '';
	let clearedResume = false;
	let scrollReset = false;

	const ctx: SendChatContext = {
		serverUrl: 'http://127.0.0.1:8080',
		sessionID: '',
		message,
		dispatchChat: (action) => {
			actions.push(action);
		},
		clearResumeSelection: () => {
			clearedResume = true;
		},
		resetChatScroll: () => {
			scrollReset = true;
		},
		pushStatus: (line) => {
			statuses.push(line);
		},
		pushDebug: (line) => {
			debugs.push(line);
		},
		handleStatusEvent: (evt) => {
			statusEvents.push(evt);
		},
		setSessionID: (next) => {
			sessionID = next;
		},
	};

	return {
		ctx,
		actions,
		statuses,
		debugs,
		statusEvents,
		get sessionID() {
			return sessionID;
		},
		get clearedResume() {
			return clearedResume;
		},
		get scrollReset() {
			return scrollReset;
		},
	};
}

test('sendChatMessage handles success path', async () => {
	const state = createContext('hello');
	const apis: SendChatAPIs = {
		streamChat: async (params) => {
			params.onStatus('before_llm');
			params.onStatusEvent?.({type: 'status', phase: 'before_llm'});
			params.onDelta('hel');
			params.onDelta('lo');
			return {assistantText: 'hello', sessionId: 'sess-5'};
		},
	};

	await sendChatMessage(state.ctx, apis);

	assert.equal(state.clearedResume, true);
	assert.equal(state.scrollReset, true);
	assert.deepEqual(state.statuses, ['before_llm']);
	assert.deepEqual(state.statusEvents.map((evt) => evt.phase), ['before_llm']);
	assert.deepEqual(
		state.actions.map((action) => action.type),
		['append_message', 'stream_start', 'stream_delta', 'stream_delta', 'stream_done'],
	);
	assert.equal(state.sessionID, 'sess-5');
});

test('sendChatMessage handles stream error path', async () => {
	const state = createContext('hello');
	const apis: SendChatAPIs = {
		streamChat: async () => {
			throw new Error('network failed');
		},
	};

	await sendChatMessage(state.ctx, apis);

	assert.deepEqual(state.actions.map((action) => action.type), ['append_message', 'stream_start', 'stream_error']);
	assert.deepEqual(state.statuses, ['error: network failed']);
	assert.deepEqual(state.debugs, ['error: network failed']);
	assert.equal(state.sessionID, '');
});

test('sendChatMessage handles abort as stream_cancel', async () => {
	const state = createContext('hello');
	const controller = new AbortController();
	const apis: SendChatAPIs = {
		streamChat: async () => {
			controller.abort();
			throw new DOMException('The operation was aborted.', 'AbortError');
		},
	};

	await sendChatMessage(
		{
			...state.ctx,
			abortSignal: controller.signal,
		},
		apis,
	);

	assert.deepEqual(state.actions.map((action) => action.type), ['append_message', 'stream_start', 'stream_cancel']);
	assert.deepEqual(state.statuses, ['stopped by user']);
	assert.deepEqual(state.debugs, ['stream canceled by user']);
});
