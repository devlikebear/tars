import test from 'node:test';
import assert from 'node:assert/strict';
import {parseArgs} from './parseArgs.js';

test('parseArgs uses defaults', () => {
	const parsed = parseArgs([]);
	assert.equal(parsed.serverUrl, 'http://127.0.0.1:8080');
	assert.equal(parsed.sessionId, '');
	assert.equal(parsed.verbose, false);
});

test('parseArgs reads verbose/server/session flags', () => {
	const parsed = parseArgs(['--verbose', '--server-url', 'http://localhost:18080', '--session', 'sess-1']);
	assert.equal(parsed.serverUrl, 'http://localhost:18080');
	assert.equal(parsed.sessionId, 'sess-1');
	assert.equal(parsed.verbose, true);
});

test('parseArgs reads equals-form flags', () => {
	const parsed = parseArgs(['--server-url=http://127.0.0.1:9000', '--session=sess-42']);
	assert.equal(parsed.serverUrl, 'http://127.0.0.1:9000');
	assert.equal(parsed.sessionId, 'sess-42');
});

