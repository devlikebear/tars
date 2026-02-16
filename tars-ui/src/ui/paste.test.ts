import test from 'node:test';
import assert from 'node:assert/strict';
import {consumeBracketedPaste, PasteState} from './paste.js';

const startMarker = '\x1b[200~';
const endMarker = '\x1b[201~';

function initialState(): PasteState {
	return {inPaste: false, buffer: ''};
}

test('consumeBracketedPaste passes through normal input', () => {
	const result = consumeBracketedPaste(initialState(), 'hello');
	assert.deepEqual(result.chunks, [{type: 'text', value: 'hello'}]);
	assert.deepEqual(result.state, initialState());
});

test('consumeBracketedPaste extracts single bracketed paste block', () => {
	const result = consumeBracketedPaste(initialState(), `${startMarker}line1\nline2${endMarker}`);
	assert.deepEqual(result.chunks, [{type: 'paste', value: 'line1\nline2'}]);
	assert.deepEqual(result.state, initialState());
});

test('consumeBracketedPaste supports mixed normal text and paste block', () => {
	const result = consumeBracketedPaste(initialState(), `before ${startMarker}PASTE${endMarker} after`);
	assert.deepEqual(result.chunks, [
		{type: 'text', value: 'before '},
		{type: 'paste', value: 'PASTE'},
		{type: 'text', value: ' after'},
	]);
	assert.deepEqual(result.state, initialState());
});

test('consumeBracketedPaste keeps state when paste block is split across chunks', () => {
	const step1 = consumeBracketedPaste(initialState(), `${startMarker}partial`);
	assert.deepEqual(step1.chunks, []);
	assert.equal(step1.state.inPaste, true);
	assert.equal(step1.state.buffer, 'partial');

	const step2 = consumeBracketedPaste(step1.state, `-content${endMarker} tail`);
	assert.deepEqual(step2.chunks, [
		{type: 'paste', value: 'partial-content'},
		{type: 'text', value: ' tail'},
	]);
	assert.deepEqual(step2.state, initialState());
});

test('consumeBracketedPaste handles multiple paste blocks in a single chunk', () => {
	const result = consumeBracketedPaste(
		initialState(),
		`${startMarker}A${endMarker}:${startMarker}B${endMarker}`,
	);
	assert.deepEqual(result.chunks, [
		{type: 'paste', value: 'A'},
		{type: 'text', value: ':'},
		{type: 'paste', value: 'B'},
	]);
	assert.deepEqual(result.state, initialState());
});
