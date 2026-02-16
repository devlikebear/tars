import test from 'node:test';
import assert from 'node:assert/strict';
import {chatUIReducer, initialChatUIState} from './state.js';

test('state reducer commits draft into assistant message on done', () => {
	let state = initialChatUIState;
	state = chatUIReducer(state, {type: 'stream_start'});
	state = chatUIReducer(state, {type: 'stream_delta', chunk: '안'});
	state = chatUIReducer(state, {type: 'stream_delta', chunk: '녕'});
	state = chatUIReducer(state, {type: 'stream_done', assistantText: ''});

	assert.equal(state.busy, false);
	assert.equal(state.assistantDraft, '');
	assert.equal(state.messages.length, 1);
	assert.equal(state.messages[0]?.role, 'assistant');
	assert.equal(state.messages[0]?.text, '안녕');
});

test('state reducer clears busy and appends error on stream_error', () => {
	let state = initialChatUIState;
	state = chatUIReducer(state, {type: 'stream_start'});
	state = chatUIReducer(state, {type: 'stream_error', errorText: 'timeout'});

	assert.equal(state.busy, false);
	assert.equal(state.assistantDraft, '');
	assert.equal(state.messages.length, 1);
	assert.equal(state.messages[0]?.role, 'error');
	assert.equal(state.messages[0]?.text, 'timeout');
});

test('state reducer stops streaming and keeps partial draft on stream_cancel', () => {
	let state = initialChatUIState;
	state = chatUIReducer(state, {type: 'stream_start'});
	state = chatUIReducer(state, {type: 'stream_delta', chunk: 'part'});
	state = chatUIReducer(state, {type: 'stream_cancel'});

	assert.equal(state.busy, false);
	assert.equal(state.assistantDraft, '');
	assert.equal(state.messages.length, 1);
	assert.equal(state.messages[0]?.role, 'assistant');
	assert.equal(state.messages[0]?.text, 'part');
});
