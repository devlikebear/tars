import {SessionHistoryItem, SessionSummary} from '../types.js';
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

export async function listSessions(serverURL: string): Promise<SessionSummary[]> {
	return requestJSON<SessionSummary[]>('GET', apiURL(serverURL, '/v1/sessions'));
}

export async function createSession(serverURL: string, title: string): Promise<SessionSummary> {
	return requestJSON<SessionSummary>('POST', apiURL(serverURL, '/v1/sessions'), {title});
}

export async function getSession(serverURL: string, sessionID: string): Promise<SessionSummary> {
	const id = encodeURIComponent(sessionID.trim());
	return requestJSON<SessionSummary>('GET', apiURL(serverURL, `/v1/sessions/${id}`));
}

export async function getHistory(serverURL: string, sessionID: string): Promise<SessionHistoryItem[]> {
	const id = encodeURIComponent(sessionID.trim());
	return requestJSON<SessionHistoryItem[]>('GET', apiURL(serverURL, `/v1/sessions/${id}/history`));
}

export async function exportSession(serverURL: string, sessionID: string): Promise<string> {
	const id = encodeURIComponent(sessionID.trim());
	const url = apiURL(serverURL, `/v1/sessions/${id}/export`);
	const resp = await fetch(url, {method: 'POST', headers: tarsHeaders()});
	const text = await resp.text();
	if (!resp.ok) {
		throw new Error(`POST ${url} status ${resp.status}: ${text.trim()}`);
	}
	return text.trim();
}

export async function searchSessions(serverURL: string, keyword: string): Promise<SessionSummary[]> {
	const q = encodeURIComponent(keyword);
	return requestJSON<SessionSummary[]>('GET', apiURL(serverURL, `/v1/sessions/search?q=${q}`));
}
