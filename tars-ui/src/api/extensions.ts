import {MCPServerStatus, MCPToolInfo, PluginDefinition, SkillDefinition} from '../types.js';

function apiURL(serverURL: string, path: string): string {
	return `${serverURL.replace(/\/+$/, '')}${path}`;
}

async function requestJSON<T>(method: string, url: string): Promise<T> {
	const resp = await fetch(url, {method});
	const text = await resp.text();
	if (!resp.ok) {
		throw new Error(`${method} ${url} status ${resp.status}: ${text.trim()}`);
	}
	try {
		return JSON.parse(text) as T;
	} catch (err) {
		throw new Error(`decode response: ${String(err)}`);
	}
}

async function requestJSONWithBody<T>(method: string, url: string, body: unknown): Promise<T> {
	const resp = await fetch(url, {
		method,
		headers: {'Content-Type': 'application/json'},
		body: JSON.stringify(body),
	});
	const text = await resp.text();
	if (!resp.ok) {
		throw new Error(`${method} ${url} status ${resp.status}: ${text.trim()}`);
	}
	try {
		return JSON.parse(text) as T;
	} catch (err) {
		throw new Error(`decode response: ${String(err)}`);
	}
}

export async function listSkills(serverURL: string): Promise<SkillDefinition[]> {
	const payload = await requestJSON<SkillDefinition[] | null>('GET', apiURL(serverURL, '/v1/skills'));
	return normalizeArray(payload);
}

export async function listPlugins(serverURL: string): Promise<PluginDefinition[]> {
	const payload = await requestJSON<PluginDefinition[] | null>('GET', apiURL(serverURL, '/v1/plugins'));
	return normalizeArray(payload);
}

export async function listMCPServers(serverURL: string): Promise<MCPServerStatus[]> {
	const payload = await requestJSON<MCPServerStatus[] | null>('GET', apiURL(serverURL, '/v1/mcp/servers'));
	return normalizeArray(payload);
}

export async function listMCPTools(serverURL: string): Promise<MCPToolInfo[]> {
	const payload = await requestJSON<MCPToolInfo[] | null>('GET', apiURL(serverURL, '/v1/mcp/tools'));
	return normalizeArray(payload);
}

export type ReloadExtensionsResponse = {
	reloaded: boolean;
	version?: number;
	skills?: number;
	plugins?: number;
	mcp_count?: number;
};

export async function reloadExtensions(serverURL: string): Promise<ReloadExtensionsResponse> {
	return requestJSONWithBody<ReloadExtensionsResponse>('POST', apiURL(serverURL, '/v1/runtime/extensions/reload'), {});
}

function normalizeArray<T>(value: T[] | null): T[] {
	if (!Array.isArray(value)) {
		return [];
	}
	return value;
}
