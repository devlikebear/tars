import test from 'node:test';
import assert from 'node:assert/strict';
import {configureAPIClientContext} from './clientContext.js';
import {getSentinelStatus, listSentinelEvents, pauseSentinel, restartSentinel, resumeSentinel} from './sentinel.js';

function installFetchMock(
	impl: (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>,
): () => void {
	const original = globalThis.fetch;
	globalThis.fetch = impl as typeof fetch;
	return () => {
		globalThis.fetch = original;
	};
}

test('sentinel api decodes status/events and control responses', async () => {
	configureAPIClientContext({
		casedApiToken: 'cased-token',
		casedAdminApiToken: 'cased-admin-token',
		workspaceId: 'ws-dev',
	});
	const authHeaders: string[] = [];
	const restore = installFetchMock(async (input, init) => {
		const headers = (init?.headers as Record<string, string> | undefined) ?? {};
		authHeaders.push(headers.Authorization ?? '');
		const url = String(input);
		if (url.endsWith('/v1/sentinel/status')) {
			return new Response(JSON.stringify({
				enabled: true,
				supervision_state: 'running',
				target: {command: 'go', args: ['run', './cmd/tarsd']},
				health_ok: true,
				restart_attempt: 0,
				restart_max_attempts: 3,
				event_count: 2,
			}), {status: 200});
		}
		if (url.includes('/v1/sentinel/events')) {
			return new Response(JSON.stringify({count: 1, events: [{id: 1, time: '2026-02-18T10:00:00Z', level: 'info', type: 'start', message: 'started'}]}), {status: 200});
		}
		if (url.endsWith('/v1/sentinel/restart') || url.endsWith('/v1/sentinel/pause') || url.endsWith('/v1/sentinel/resume')) {
			return new Response(JSON.stringify({
				enabled: true,
				supervision_state: 'running',
				target: {command: 'go'},
				health_ok: true,
				restart_attempt: 0,
				restart_max_attempts: 3,
				event_count: 3,
			}), {status: 200});
		}
		return new Response('not found', {status: 404});
	});
	try {
		const status = await getSentinelStatus('http://127.0.0.1:43181');
		assert.equal(status.supervision_state, 'running');
		assert.equal(status.target.command, 'go');

		const events = await listSentinelEvents('http://127.0.0.1:43181', 10);
		assert.equal(events.length, 1);
		assert.equal(events[0]?.type, 'start');

		const restarted = await restartSentinel('http://127.0.0.1:43181');
		assert.equal(restarted.enabled, true);
		const paused = await pauseSentinel('http://127.0.0.1:43181');
		assert.equal(paused.enabled, true);
		const resumed = await resumeSentinel('http://127.0.0.1:43181');
		assert.equal(resumed.enabled, true);
		assert.equal(authHeaders[0], 'Bearer cased-token');
		assert.equal(authHeaders[1], 'Bearer cased-token');
		assert.equal(authHeaders[2], 'Bearer cased-admin-token');
		assert.equal(authHeaders[3], 'Bearer cased-admin-token');
		assert.equal(authHeaders[4], 'Bearer cased-admin-token');
	} finally {
		restore();
		configureAPIClientContext({});
	}
});
