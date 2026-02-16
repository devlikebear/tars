import test from 'node:test';
import assert from 'node:assert/strict';
import {
	initialInputHistoryState,
	navigateInputHistory,
	pushInputHistory,
	resetInputHistoryCursor,
} from './inputHistory.js';

test('pushInputHistory appends non-empty unique input', () => {
	let state = initialInputHistoryState;
	state = pushInputHistory(state, 'hello');
	state = pushInputHistory(state, 'hello');
	state = pushInputHistory(state, 'world');
	assert.deepEqual(state.entries, ['hello', 'world']);
	assert.equal(state.index, -1);
});

test('navigateInputHistory browses entries with up/down', () => {
	const base = {
		entries: ['one', 'two', 'three'],
		index: -1,
		draft: '',
	};
	const up1 = navigateInputHistory(base, 'draft text', 'up');
	assert.equal(up1.value, 'three');
	assert.equal(up1.state.index, 2);
	assert.equal(up1.state.draft, 'draft text');

	const up2 = navigateInputHistory(up1.state, up1.value, 'up');
	assert.equal(up2.value, 'two');
	assert.equal(up2.state.index, 1);

	const down1 = navigateInputHistory(up2.state, up2.value, 'down');
	assert.equal(down1.value, 'three');
	assert.equal(down1.state.index, 2);

	const down2 = navigateInputHistory(down1.state, down1.value, 'down');
	assert.equal(down2.value, 'draft text');
	assert.equal(down2.state.index, -1);
});

test('resetInputHistoryCursor clears browsing state', () => {
	const reset = resetInputHistoryCursor({
		entries: ['a'],
		index: 0,
		draft: 'tmp',
	});
	assert.equal(reset.index, -1);
	assert.equal(reset.draft, '');
});
