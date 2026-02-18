import {tarsHeaders} from './clientContext.js';

function apiURL(serverURL: string, path: string): string {
	return `${serverURL.replace(/\/+$/, '')}${path}`;
}

async function requestJSON<T>(method: string, url: string, body?: unknown): Promise<T> {
	const resp = await fetch(url, {
		method,
		headers: body === undefined ? tarsHeaders() : tarsHeaders({'Content-Type': 'application/json'}),
		body: body === undefined ? undefined : JSON.stringify(body),
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

export type StatusResponse = {
	workspace_dir: string;
	session_count: number;
	workspace_id?: string;
	auth_role?: string;
};

export async function getStatus(serverURL: string): Promise<StatusResponse> {
	return requestJSON<StatusResponse>('GET', apiURL(serverURL, '/v1/status'));
}

export async function runCompact(serverURL: string, sessionID: string): Promise<string> {
	const payload = await requestJSON<{message: string}>('POST', apiURL(serverURL, '/v1/compact'), {
		session_id: sessionID.trim(),
	});
	return payload.message.trim();
}

export async function runHeartbeatOnce(serverURL: string): Promise<string> {
	const payload = await requestJSON<{response: string}>('POST', apiURL(serverURL, '/v1/heartbeat/run-once'));
	return payload.response.trim();
}
