import test from 'node:test';
import assert from 'node:assert/strict';
import {KillRing} from './killRing.js';

test('KillRing pushes and peeks latest entry', () => {
	const ring = new KillRing();
	ring.push('abc', {prepend: false});
	ring.push('def', {prepend: false});
	assert.equal(ring.length, 2);
	assert.equal(ring.peek(), 'def');
});

test('KillRing accumulates consecutive kills', () => {
	const ring = new KillRing();
	ring.push('abc', {prepend: false});
	ring.push('def', {prepend: false, accumulate: true});
	assert.equal(ring.length, 1);
	assert.equal(ring.peek(), 'abcdef');

	ring.push('X', {prepend: true, accumulate: true});
	assert.equal(ring.peek(), 'Xabcdef');
});

test('KillRing rotates entries for yank-pop behavior', () => {
	const ring = new KillRing();
	ring.push('one', {prepend: false});
	ring.push('two', {prepend: false});
	ring.push('three', {prepend: false});

	assert.equal(ring.peek(), 'three');
	ring.rotate();
	assert.equal(ring.peek(), 'two');
	ring.rotate();
	assert.equal(ring.peek(), 'one');
});
