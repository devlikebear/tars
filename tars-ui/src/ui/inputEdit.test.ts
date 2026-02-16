import test from 'node:test';
import assert from 'node:assert/strict';
import {backspace, deleteToLineEnd, deleteToLineStart, insertText, replaceRange} from './inputEdit.js';

test('insertText inserts at cursor position', () => {
	const next = insertText({value: 'abef', cursor: 2}, 'cd');
	assert.deepEqual(next, {value: 'abcdef', cursor: 4});
});

test('backspace removes character before cursor', () => {
	const next = backspace({value: 'abcd', cursor: 3});
	assert.deepEqual(next, {value: 'abd', cursor: 2});
	assert.deepEqual(backspace({value: 'abcd', cursor: 0}), {value: 'abcd', cursor: 0});
});

test('replaceRange replaces selected range and moves cursor', () => {
	const next = replaceRange({value: 'hello world', cursor: 0}, 6, 11, 'tars');
	assert.deepEqual(next, {value: 'hello tars', cursor: 10});
});

test('deleteToLineStart uses current line boundary', () => {
	const result = deleteToLineStart({value: 'a\nbcde\nfg', cursor: 5});
	assert.equal(result.deleted, 'bcd');
	assert.deepEqual(result.next, {value: 'a\ne\nfg', cursor: 2});
});

test('deleteToLineEnd uses current line boundary', () => {
	const result = deleteToLineEnd({value: 'a\nbcde\nfg', cursor: 3});
	assert.equal(result.deleted, 'cde');
	assert.deepEqual(result.next, {value: 'a\nb\nfg', cursor: 3});
});
