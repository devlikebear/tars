import test from 'node:test';
import assert from 'node:assert/strict';
import {cancelAgentRun, getAgentRun, getGatewayStatus, listAgentRuns, listAgents, reloadGateway, restartGateway, spawnAgentRun} from './runtime.js';

function installFetchMock(
	impl: (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>,
): () => void {
	const original = globalThis.fetch;
	globalThis.fetch = impl as typeof fetch;
	return () => {
		globalThis.fetch = original;
	};
}

test('runtime api decodes run and gateway responses', async () => {
	const restore = installFetchMock(async (input) => {
		const url = String(input);
		if (url.includes('/v1/agent/runs?')) {
			return new Response(JSON.stringify({count: 1, runs: [{run_id: 'run_1', accepted: true, status: 'running'}]}), {status: 200});
		}
		if (url.endsWith('/v1/agent/runs/run_1')) {
			return new Response(JSON.stringify({run_id: 'run_1', accepted: true, status: 'completed'}), {status: 200});
		}
		if (url.endsWith('/v1/agent/runs/run_1/cancel')) {
			return new Response(JSON.stringify({run_id: 'run_1', accepted: true, status: 'canceled'}), {status: 200});
		}
			if (url.endsWith('/v1/agent/runs')) {
				return new Response(JSON.stringify({run_id: 'run_2', accepted: true, status: 'accepted', session_id: 'sess_2'}), {status: 202});
			}
			if (url.endsWith('/v1/agent/agents')) {
				return new Response(JSON.stringify({count: 1, agents: [{name: 'default', description: 'Default in-process agent loop', enabled: true, kind: 'prompt', default: true}]}), {status: 200});
			}
		if (url.endsWith('/v1/gateway/status')) {
			return new Response(JSON.stringify({enabled: true, version: 2, runs_total: 1, runs_active: 0, agents_count: 2, agents_watch_enabled: true, agents_reload_version: 5, agents_last_reload_at: '2026-02-17T12:00:00Z', channels_local_enabled: true, channels_webhook_enabled: false, channels_telegram_enabled: false}), {status: 200});
		}
		if (url.endsWith('/v1/gateway/reload')) {
			return new Response(JSON.stringify({enabled: true, version: 3, runs_total: 1, runs_active: 0, agents_count: 2, agents_watch_enabled: true, agents_reload_version: 6, agents_last_reload_at: '2026-02-17T12:01:00Z', channels_local_enabled: true, channels_webhook_enabled: false, channels_telegram_enabled: false}), {status: 200});
		}
		if (url.endsWith('/v1/gateway/restart')) {
			return new Response(JSON.stringify({enabled: true, version: 4, runs_total: 1, runs_active: 0, agents_count: 2, agents_watch_enabled: true, agents_reload_version: 6, agents_last_reload_at: '2026-02-17T12:01:00Z', channels_local_enabled: true, channels_webhook_enabled: false, channels_telegram_enabled: false}), {status: 200});
		}
		return new Response('not found', {status: 404});
	});
	try {
		const runs = await listAgentRuns('http://127.0.0.1:8080');
		assert.equal(runs.length, 1);
		assert.equal(runs[0]?.run_id, 'run_1');

		const run = await getAgentRun('http://127.0.0.1:8080', 'run_1');
		assert.equal(run.status, 'completed');

		const canceled = await cancelAgentRun('http://127.0.0.1:8080', 'run_1');
		assert.equal(canceled.status, 'canceled');
			const spawned = await spawnAgentRun('http://127.0.0.1:8080', {message: 'hello'});
			assert.equal(spawned.run_id, 'run_2');
			assert.equal(spawned.status, 'accepted');
			const agents = await listAgents('http://127.0.0.1:8080');
			assert.equal(agents.length, 1);
			assert.equal(agents[0]?.name, 'default');

			const status = await getGatewayStatus('http://127.0.0.1:8080');
		assert.equal(status.version, 2);
		assert.equal(status.agents_count, 2);
		const reloaded = await reloadGateway('http://127.0.0.1:8080');
		assert.equal(reloaded.version, 3);
		const restarted = await restartGateway('http://127.0.0.1:8080');
		assert.equal(restarted.version, 4);
	} finally {
		restore();
	}
});
