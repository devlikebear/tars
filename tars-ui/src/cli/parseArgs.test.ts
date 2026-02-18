import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {parseArgs} from './parseArgs.js';

test('parseArgs uses defaults', () => {
	const parsed = parseArgs([]);
	assert.equal(parsed.serverUrl, 'http://127.0.0.1:43180');
	assert.equal(parsed.casedServerUrl, 'http://127.0.0.1:43181');
	assert.equal(parsed.sessionId, '');
	assert.equal(parsed.verbose, false);
});

test('parseArgs reads verbose/server/session flags', () => {
	const parsed = parseArgs(['--verbose', '--server-url', 'http://localhost:43180', '--cased-url', 'http://localhost:43181', '--session', 'sess-1']);
	assert.equal(parsed.serverUrl, 'http://localhost:43180');
	assert.equal(parsed.casedServerUrl, 'http://localhost:43181');
	assert.equal(parsed.sessionId, 'sess-1');
	assert.equal(parsed.verbose, true);
});

test('parseArgs reads equals-form flags', () => {
	const parsed = parseArgs(['--server-url=http://127.0.0.1:9000', '--cased-url=http://127.0.0.1:9001', '--session=sess-42']);
	assert.equal(parsed.serverUrl, 'http://127.0.0.1:9000');
	assert.equal(parsed.casedServerUrl, 'http://127.0.0.1:9001');
	assert.equal(parsed.sessionId, 'sess-42');
});

test('parseArgs reads config file', () => {
	const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'tars-ui-config-'));
	const configPath = path.join(dir, 'config.yaml');
	fs.writeFileSync(configPath, ['server_url: http://localhost:19090', 'cased_server_url: http://localhost:19091', 'session_id: sess-from-file', 'verbose: true', ''].join('\n'));

	const parsed = parseArgs(['--config', configPath]);
	assert.equal(parsed.serverUrl, 'http://localhost:19090');
	assert.equal(parsed.casedServerUrl, 'http://localhost:19091');
	assert.equal(parsed.sessionId, 'sess-from-file');
	assert.equal(parsed.verbose, true);
});

test('parseArgs gives cli flags precedence over config file', () => {
	const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'tars-ui-config-'));
	const configPath = path.join(dir, 'config.yaml');
	fs.writeFileSync(configPath, ['server_url: http://localhost:19090', 'cased_server_url: http://localhost:19091', 'session_id: sess-from-file', 'verbose: false', ''].join('\n'));

	const parsed = parseArgs([
		'--config',
		configPath,
		'--server-url=http://127.0.0.1:9000',
		'--cased-url=http://127.0.0.1:9001',
		'--session=sess-cli',
		'--verbose',
	]);
	assert.equal(parsed.serverUrl, 'http://127.0.0.1:9000');
	assert.equal(parsed.casedServerUrl, 'http://127.0.0.1:9001');
	assert.equal(parsed.sessionId, 'sess-cli');
	assert.equal(parsed.verbose, true);
});
