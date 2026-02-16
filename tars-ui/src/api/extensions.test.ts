import test from 'node:test';
import assert from 'node:assert/strict';
import {listMCPServers, listMCPTools, listPlugins, listSkills} from './extensions.js';

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
	const restore = installFetchMock(async (input) => {
		const url = String(input);
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
		return new Response('not found', {status: 404});
	});

	try {
		const skills = await listSkills('http://127.0.0.1:8080/');
		const plugins = await listPlugins('http://127.0.0.1:8080/');
		const servers = await listMCPServers('http://127.0.0.1:8080/');
		const tools = await listMCPTools('http://127.0.0.1:8080/');

		assert.equal(skills[0]?.name, 'deploy');
		assert.equal(plugins[0]?.id, 'ops');
		assert.equal(servers[0]?.name, 'fs');
		assert.equal(tools[0]?.name, 'read_file');
	} finally {
		restore();
	}
});

