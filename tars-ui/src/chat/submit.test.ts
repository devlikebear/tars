import test from 'node:test';
import assert from 'node:assert/strict';
import {submitInput, SubmitInputContext} from './submit.js';
import {SessionSummary} from '../types.js';

function createContext(overrides: Partial<SubmitInputContext>): {
	ctx: SubmitInputContext;
	errors: string[];
	messages: string[];
	debugs: string[];
	inputs: string[];
	resumeCandidates: SessionSummary[] | null;
	resumeIndex: number;
	sessionID: string;
	executedCommands: string[];
	sentChats: string[];
} {
	const errors: string[] = [];
	const messages: string[] = [];
	const debugs: string[] = [];
	const inputs: string[] = [];
	const executedCommands: string[] = [];
	const sentChats: string[] = [];

	let resumeCandidates: SessionSummary[] | null = null;
	let resumeIndex = 0;
	let sessionID = '';

	const ctx: SubmitInputContext = {
		input: '',
		busy: false,
		resumeCandidates: null,
		resumeIndex: 0,
		setInput: (value) => {
			inputs.push(value);
		},
		setSessionID: (value) => {
			sessionID = value;
		},
		setResumeCandidates: (value) => {
			resumeCandidates = value;
		},
		setResumeIndex: (value) => {
			resumeIndex = value;
		},
		pushSystemMessage: (value) => {
			messages.push(value);
		},
		pushErrorMessage: (value) => {
			errors.push(value);
		},
		pushDebug: (value) => {
			debugs.push(value);
		},
		executeCommand: async (line) => {
			executedCommands.push(line);
		},
		sendChat: async (line) => {
			sentChats.push(line);
		},
		...overrides,
	};

	return {
		ctx,
		errors,
		messages,
		debugs,
		inputs,
		get resumeCandidates() {
			return resumeCandidates;
		},
		get resumeIndex() {
			return resumeIndex;
		},
		get sessionID() {
			return sessionID;
		},
		executedCommands,
		sentChats,
	};
}

test('submitInput returns immediately when busy', async () => {
	const state = createContext({input: 'hello', busy: true});
	await submitInput(state.ctx);

	assert.deepEqual(state.inputs, []);
	assert.deepEqual(state.sentChats, []);
});

test('submitInput sends normal chat message', async () => {
	const state = createContext({input: 'hello'});
	await submitInput(state.ctx);

	assert.deepEqual(state.inputs, ['']);
	assert.deepEqual(state.sentChats, ['hello']);
});

test('submitInput resumes highlighted candidate on empty selection', async () => {
	const candidates = [
		{id: 'sess-1', title: 'first'},
		{id: 'sess-2', title: 'second'},
	];
	const state = createContext({
		input: '',
		resumeCandidates: candidates,
		resumeIndex: 1,
	});
	await submitInput(state.ctx);

	assert.equal(state.sessionID, 'sess-2');
	assert.equal(state.resumeCandidates, null);
	assert.equal(state.resumeIndex, 0);
	assert.deepEqual(state.messages, ['resumed session: sess-2']);
});

test('submitInput returns invalid selection error', async () => {
	const candidates = [
		{id: 'sess-1', title: 'first'},
	];
	const state = createContext({
		input: '3',
		resumeCandidates: candidates,
		resumeIndex: 0,
	});
	await submitInput(state.ctx);

	assert.deepEqual(state.errors, ['invalid selection']);
});

test('submitInput executes slash command and captures command error', async () => {
	const state = createContext({
		input: '/status',
		executeCommand: async () => {
			throw new Error('boom');
		},
	});
	await submitInput(state.ctx);

	assert.deepEqual(state.errors, ['boom']);
	assert.deepEqual(state.debugs, ['command error: boom']);
	assert.deepEqual(state.executedCommands, []);
});

test('submitInput routes unknown slash to chat as skill invoke', async () => {
	const state = createContext({input: '/deploy now'});
	await submitInput(state.ctx);

	assert.deepEqual(state.sentChats, ['/deploy now']);
	assert.deepEqual(state.executedCommands, []);
});
