import {AgentDescriptor, AgentRunSummary, GatewayReportChannels, GatewayReportRuns, GatewayReportSummary, GatewayStatus} from '../types.js';
import {tarsAdminHeaders, tarsHeaders} from './clientContext.js';

function apiURL(serverURL: string, path: string): string {
	return `${serverURL.replace(/\/+$/, '')}${path}`;
}

async function requestJSON<T>(method: string, url: string): Promise<T> {
	const resp = await fetch(url, {method, headers: tarsHeaders()});
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

async function requestJSONWithBody<T>(method: string, url: string, payload: unknown): Promise<T> {
	const resp = await fetch(url, {
		method,
		headers: tarsHeaders({'Content-Type': 'application/json'}),
		body: JSON.stringify(payload),
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

export async function listAgentRuns(serverURL: string, limit = 30): Promise<AgentRunSummary[]> {
	const payload = await requestJSON<{count: number; runs: AgentRunSummary[]}>('GET', apiURL(serverURL, `/v1/agent/runs?limit=${limit}`));
	if (!Array.isArray(payload.runs)) {
		return [];
	}
	return payload.runs;
}

export async function spawnAgentRun(
	serverURL: string,
	input: {session_id?: string; title?: string; message: string; agent?: string},
): Promise<AgentRunSummary> {
	return requestJSONWithBody<AgentRunSummary>('POST', apiURL(serverURL, '/v1/agent/runs'), input);
}

export async function listAgents(serverURL: string): Promise<AgentDescriptor[]> {
	const payload = await requestJSON<{count: number; agents: AgentDescriptor[]}>('GET', apiURL(serverURL, '/v1/agent/agents'));
	if (!Array.isArray(payload.agents)) {
		return [];
	}
	return payload.agents;
}

export async function getAgentRun(serverURL: string, runID: string): Promise<AgentRunSummary> {
	const id = encodeURIComponent(runID.trim());
	return requestJSON<AgentRunSummary>('GET', apiURL(serverURL, `/v1/agent/runs/${id}`));
}

export async function cancelAgentRun(serverURL: string, runID: string): Promise<AgentRunSummary> {
	const id = encodeURIComponent(runID.trim());
	return requestJSON<AgentRunSummary>('POST', apiURL(serverURL, `/v1/agent/runs/${id}/cancel`));
}

export async function getGatewayStatus(serverURL: string): Promise<GatewayStatus> {
	return requestJSON<GatewayStatus>('GET', apiURL(serverURL, '/v1/gateway/status'));
}

export async function reloadGateway(serverURL: string): Promise<GatewayStatus> {
	const url = apiURL(serverURL, '/v1/gateway/reload');
	const resp = await fetch(url, {method: 'POST', headers: tarsAdminHeaders()});
	const text = await resp.text();
	if (!resp.ok) {
		throw new Error(`POST ${url} status ${resp.status}: ${text.trim()}`);
	}
	try {
		return JSON.parse(text) as GatewayStatus;
	} catch (err) {
		throw new Error(`decode response: ${String(err)}`);
	}
}

export async function restartGateway(serverURL: string): Promise<GatewayStatus> {
	const url = apiURL(serverURL, '/v1/gateway/restart');
	const resp = await fetch(url, {method: 'POST', headers: tarsAdminHeaders()});
	const text = await resp.text();
	if (!resp.ok) {
		throw new Error(`POST ${url} status ${resp.status}: ${text.trim()}`);
	}
	try {
		return JSON.parse(text) as GatewayStatus;
	} catch (err) {
		throw new Error(`decode response: ${String(err)}`);
	}
}

export async function getGatewayReportSummary(serverURL: string): Promise<GatewayReportSummary> {
	return requestJSON<GatewayReportSummary>('GET', apiURL(serverURL, '/v1/gateway/reports/summary'));
}

export async function getGatewayReportRuns(serverURL: string, limit = 50): Promise<GatewayReportRuns> {
	return requestJSON<GatewayReportRuns>('GET', apiURL(serverURL, `/v1/gateway/reports/runs?limit=${limit}`));
}

export async function getGatewayReportChannels(serverURL: string, limit = 50): Promise<GatewayReportChannels> {
	return requestJSON<GatewayReportChannels>('GET', apiURL(serverURL, `/v1/gateway/reports/channels?limit=${limit}`));
}
