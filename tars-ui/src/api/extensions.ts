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

export async function listSkills(serverURL: string): Promise<SkillDefinition[]> {
	return requestJSON<SkillDefinition[]>('GET', apiURL(serverURL, '/v1/skills'));
}

export async function listPlugins(serverURL: string): Promise<PluginDefinition[]> {
	return requestJSON<PluginDefinition[]>('GET', apiURL(serverURL, '/v1/plugins'));
}

export async function listMCPServers(serverURL: string): Promise<MCPServerStatus[]> {
	return requestJSON<MCPServerStatus[]>('GET', apiURL(serverURL, '/v1/mcp/servers'));
}

export async function listMCPTools(serverURL: string): Promise<MCPToolInfo[]> {
	return requestJSON<MCPToolInfo[]>('GET', apiURL(serverURL, '/v1/mcp/tools'));
}

