import test from 'node:test';
import assert from 'node:assert/strict';
import {UndoStack} from './undoStack.js';

test('UndoStack restores snapshots in LIFO order', () => {
	const stack = new UndoStack<{value: string; cursor: number}>();
	stack.push({value: 'a', cursor: 1});
	stack.push({value: 'ab', cursor: 2});
	stack.push({value: 'abc', cursor: 3});

	assert.deepEqual(stack.pop(), {value: 'abc', cursor: 3});
	assert.deepEqual(stack.pop(), {value: 'ab', cursor: 2});
	assert.deepEqual(stack.pop(), {value: 'a', cursor: 1});
	assert.equal(stack.pop(), undefined);
});

test('UndoStack stores deep-cloned snapshots', () => {
	const stack = new UndoStack<{items: string[]}>();
	const source = {items: ['a']};
	stack.push(source);
	source.items.push('b');

	assert.deepEqual(stack.pop(), {items: ['a']});
});
