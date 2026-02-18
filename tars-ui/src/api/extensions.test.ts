import test from 'node:test';
import assert from 'node:assert/strict';
import {configureAPIClientContext} from './clientContext.js';
import {listMCPServers, listMCPTools, listPlugins, listSkills, reloadExtensions} from './extensions.js';

function installFetchMock(
	impl: (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>,
): () => void {
	const original = globalThis.fetch;
	globalThis.fetch = impl as typeof fetch;
	return () => {
		globalThis.fetch = original;
	};
}

test('extensions api decodes list responses', async () => {
	configureAPIClientContext({apiToken: 'user-token', adminApiToken: 'admin-token', workspaceId: 'ws-dev'});
	const authHeaders: string[] = [];
	const restore = installFetchMock(async (input, init) => {
		const url = String(input);
		const headers = (init?.headers as Record<string, string> | undefined) ?? {};
		authHeaders.push(headers.Authorization ?? '');
		if (url.endsWith('/v1/skills')) {
			return new Response(JSON.stringify([{name: 'deploy'}]), {status: 200});
		}
		if (url.endsWith('/v1/plugins')) {
			return new Response(JSON.stringify([{id: 'ops'}]), {status: 200});
		}
		if (url.endsWith('/v1/mcp/servers')) {
			return new Response(JSON.stringify([{name: 'fs', command: 'npx', connected: true, tool_count: 1}]), {status: 200});
		}
		if (url.endsWith('/v1/mcp/tools')) {
			return new Response(JSON.stringify([{server: 'fs', name: 'read_file'}]), {status: 200});
		}
		if (url.endsWith('/v1/runtime/extensions/reload')) {
			return new Response(JSON.stringify({reloaded: true, version: 7, skills: 2, plugins: 1, mcp_count: 3, gateway_refreshed: true, gateway_agents: 2}), {status: 200});
		}
		return new Response('not found', {status: 404});
	});

	try {
		const skills = await listSkills('http://127.0.0.1:8080/');
		const plugins = await listPlugins('http://127.0.0.1:8080/');
		const servers = await listMCPServers('http://127.0.0.1:8080/');
		const tools = await listMCPTools('http://127.0.0.1:8080/');
		const reload = await reloadExtensions('http://127.0.0.1:8080/');

		assert.equal(skills[0]?.name, 'deploy');
		assert.equal(plugins[0]?.id, 'ops');
		assert.equal(servers[0]?.name, 'fs');
		assert.equal(tools[0]?.name, 'read_file');
		assert.equal(reload.reloaded, true);
		assert.equal(reload.version, 7);
		assert.equal(reload.gateway_refreshed, true);
		assert.equal(reload.gateway_agents, 2);
		assert.equal(authHeaders[0], 'Bearer user-token');
		assert.equal(authHeaders[4], 'Bearer admin-token');
	} finally {
		restore();
		configureAPIClientContext({});
	}
});

test('extensions api treats null list payload as empty array', async () => {
	const restore = installFetchMock(async () => new Response('null', {status: 200}));
	try {
		const skills = await listSkills('http://127.0.0.1:8080/');
		const plugins = await listPlugins('http://127.0.0.1:8080/');
		const servers = await listMCPServers('http://127.0.0.1:8080/');
		const tools = await listMCPTools('http://127.0.0.1:8080/');
		assert.deepEqual(skills, []);
		assert.deepEqual(plugins, []);
		assert.deepEqual(servers, []);
		assert.deepEqual(tools, []);
	} finally {
		restore();
	}
});
