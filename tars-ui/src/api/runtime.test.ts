import test from 'node:test';
import assert from 'node:assert/strict';
import {configureAPIClientContext} from './clientContext.js';
import {cancelAgentRun, getAgentRun, getGatewayReportChannels, getGatewayReportRuns, getGatewayReportSummary, getGatewayStatus, listAgentRuns, listAgents, reloadGateway, restartGateway, spawnAgentRun} from './runtime.js';

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
	configureAPIClientContext({apiToken: 'user-token', adminApiToken: 'admin-token', workspaceId: 'ws-dev'});
	const authHeaders: string[] = [];
	const restore = installFetchMock(async (input, init) => {
		const url = String(input);
		const headers = (init?.headers as Record<string, string> | undefined) ?? {};
		authHeaders.push(headers.Authorization ?? '');
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
				return new Response(JSON.stringify({
					count: 1,
					agents: [{
						name: 'default',
						description: 'Default in-process agent loop',
						enabled: true,
						kind: 'prompt',
						default: true,
						policy_mode: 'full',
						tools_allow_count: 0,
					}],
				}), {status: 200});
			}
		if (url.endsWith('/v1/gateway/status')) {
			return new Response(JSON.stringify({enabled: true, version: 2, runs_total: 1, runs_active: 0, agents_count: 2, agents_watch_enabled: true, agents_reload_version: 5, agents_last_reload_at: '2026-02-17T12:00:00Z', channels_local_enabled: true, channels_webhook_enabled: false, channels_telegram_enabled: false, persistence_enabled: true, runs_persistence_enabled: true, channels_persistence_enabled: true, restore_on_startup: true, persistence_dir: '/tmp/gateway', runs_restored: 1, channels_restored: 1, last_persist_at: '2026-02-17T12:00:01Z', last_restore_at: '2026-02-17T12:00:00Z'}), {status: 200});
		}
		if (url.endsWith('/v1/gateway/reload')) {
			return new Response(JSON.stringify({enabled: true, version: 3, runs_total: 1, runs_active: 0, agents_count: 2, agents_watch_enabled: true, agents_reload_version: 6, agents_last_reload_at: '2026-02-17T12:01:00Z', channels_local_enabled: true, channels_webhook_enabled: false, channels_telegram_enabled: false, persistence_enabled: true, runs_persistence_enabled: true, channels_persistence_enabled: true, restore_on_startup: true, persistence_dir: '/tmp/gateway', runs_restored: 1, channels_restored: 1, last_persist_at: '2026-02-17T12:01:01Z', last_restore_at: '2026-02-17T12:00:00Z'}), {status: 200});
		}
		if (url.endsWith('/v1/gateway/restart')) {
			return new Response(JSON.stringify({enabled: true, version: 4, runs_total: 1, runs_active: 0, agents_count: 2, agents_watch_enabled: true, agents_reload_version: 6, agents_last_reload_at: '2026-02-17T12:01:00Z', channels_local_enabled: true, channels_webhook_enabled: false, channels_telegram_enabled: false, persistence_enabled: true, runs_persistence_enabled: true, channels_persistence_enabled: true, restore_on_startup: true, persistence_dir: '/tmp/gateway', runs_restored: 1, channels_restored: 1, last_persist_at: '2026-02-17T12:02:01Z', last_restore_at: '2026-02-17T12:00:00Z'}), {status: 200});
		}
		if (url.includes('/v1/gateway/reports/summary')) {
			return new Response(JSON.stringify({
				generated_at: '2026-02-18T11:00:00Z',
				summary_enabled: true,
				archive_enabled: true,
				runs_total: 2,
				runs_active: 1,
				runs_by_status: {running: 1, completed: 1},
				channels_total: 1,
				messages_total: 3,
				messages_by_source: {local: 2, webhook: 1},
			}), {status: 200});
		}
		if (url.includes('/v1/gateway/reports/runs')) {
			return new Response(JSON.stringify({
				generated_at: '2026-02-18T11:00:00Z',
				archive_enabled: true,
				count: 1,
				runs: [{run_id: 'run_1', status: 'completed', accepted: true}],
			}), {status: 200});
		}
		if (url.includes('/v1/gateway/reports/channels')) {
			return new Response(JSON.stringify({
				generated_at: '2026-02-18T11:00:00Z',
				archive_enabled: true,
				count: 1,
				messages: {general: [{id: 'msg_1', channel_id: 'general', source: 'local', direction: 'outbound', text: 'hello', timestamp: '2026-02-18T11:00:00Z'}]},
			}), {status: 200});
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
			assert.equal(agents[0]?.policy_mode, 'full');
			assert.equal(agents[0]?.tools_allow_count, 0);

		const status = await getGatewayStatus('http://127.0.0.1:8080');
		assert.equal(status.version, 2);
		assert.equal(status.agents_count, 2);
		assert.equal(status.persistence_enabled, true);
		assert.equal(status.runs_restored, 1);
		const reloaded = await reloadGateway('http://127.0.0.1:8080');
		assert.equal(reloaded.version, 3);
		const restarted = await restartGateway('http://127.0.0.1:8080');
		assert.equal(restarted.version, 4);
		const summary = await getGatewayReportSummary('http://127.0.0.1:8080');
		assert.equal(summary.runs_total, 2);
		const reportRuns = await getGatewayReportRuns('http://127.0.0.1:8080', 10);
		assert.equal(reportRuns.count, 1);
		const reportChannels = await getGatewayReportChannels('http://127.0.0.1:8080', 10);
		assert.equal(reportChannels.count, 1);
		assert.equal(authHeaders[0], 'Bearer user-token');
		assert.equal(authHeaders[6], 'Bearer admin-token');
		assert.equal(authHeaders[7], 'Bearer admin-token');
	} finally {
		restore();
		configureAPIClientContext({});
	}
});
