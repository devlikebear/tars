import test from 'node:test';
import assert from 'node:assert/strict';
import {appendBounded, renderTable, requireSessionOrError, truncate} from './format.js';

test('appendBounded ignores blank input and keeps max size', () => {
	assert.deepEqual(appendBounded(['a'], '   ', 3), ['a']);
	assert.deepEqual(appendBounded(['a', 'b'], 'c', 3), ['a', 'b', 'c']);
	assert.deepEqual(appendBounded(['a', 'b', 'c'], 'd', 3), ['b', 'c', 'd']);
});

test('requireSessionOrError validates empty session', () => {
	assert.equal(requireSessionOrError(''), 'no active session. use /new or /resume {session_id}');
	assert.equal(requireSessionOrError('sess-1'), null);
});

test('truncate returns ellipsis for long values', () => {
	assert.equal(truncate('abcdef', 5), 'ab...');
	assert.equal(truncate('abc', 5), 'abc');
	assert.equal(truncate('abcdef', 2), 'ab');
});

test('renderTable builds aligned rows', () => {
	const lines = renderTable(['ID', 'TITLE'], [['s1', 'short']]);
	assert.equal(lines.length, 3);
	assert.equal(lines[0]?.includes('ID'), true);
	assert.equal(lines[0]?.includes('TITLE'), true);
	assert.equal(lines[1]?.includes('-|-'), true);
	assert.equal(lines[2]?.includes('s1'), true);
	assert.equal(lines[2]?.includes('short'), true);
});
