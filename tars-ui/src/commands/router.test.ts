import test from 'node:test';
import assert from 'node:assert/strict';
import {parseInputCommand} from './router.js';

test('router parses chat and quit commands', () => {
	assert.deepEqual(parseInputCommand('hello'), {kind: 'chat', message: 'hello'});
	assert.deepEqual(parseInputCommand('/quit'), {kind: 'quit'});
	assert.deepEqual(parseInputCommand('/exit'), {kind: 'quit'});
	assert.deepEqual(parseInputCommand('₩resume sess-kor'), {kind: 'resume', sessionID: 'sess-kor'});
});

test('router parses slash command options', () => {
	assert.deepEqual(parseInputCommand('/new'), {kind: 'new', title: 'chat'});
	assert.deepEqual(parseInputCommand('/new my title'), {kind: 'new', title: 'my title'});
	assert.deepEqual(parseInputCommand('/resume abc'), {kind: 'resume', sessionID: 'abc'});
	assert.deepEqual(parseInputCommand('/resume'), {kind: 'resume_select'});
	assert.deepEqual(parseInputCommand('/search keyword'), {kind: 'search', keyword: 'keyword'});
	assert.deepEqual(parseInputCommand('／help'), {kind: 'help'});
	assert.deepEqual(parseInputCommand('\\sessions'), {kind: 'sessions'});
});

test('router returns invalid for malformed or unknown command', () => {
	assert.deepEqual(parseInputCommand('/search'), {kind: 'invalid', message: 'usage: /search {keyword}'});
	assert.deepEqual(parseInputCommand('/what'), {kind: 'invalid', message: 'unknown command: /what'});
	assert.deepEqual(parseInputCommand('   '), {kind: 'noop'});
});
