import {SentinelEvent, SentinelStatus} from '../types.js';
import {casedAdminHeaders, casedHeaders} from './clientContext.js';

function apiURL(serverURL: string, path: string): string {
	return `${serverURL.replace(/\/+$/, '')}${path}`;
}

async function requestJSON<T>(method: string, url: string): Promise<T> {
	const resp = await fetch(url, {method, headers: casedHeaders()});
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

export async function getSentinelStatus(serverURL: string): Promise<SentinelStatus> {
	return requestJSON<SentinelStatus>('GET', apiURL(serverURL, '/v1/sentinel/status'));
}

export async function listSentinelEvents(serverURL: string, limit = 20): Promise<SentinelEvent[]> {
	const payload = await requestJSON<{count: number; events: SentinelEvent[]}>('GET', apiURL(serverURL, `/v1/sentinel/events?limit=${limit}`));
	if (!Array.isArray(payload.events)) {
		return [];
	}
	return payload.events;
}

export async function restartSentinel(serverURL: string): Promise<SentinelStatus> {
	const url = apiURL(serverURL, '/v1/sentinel/restart');
	const resp = await fetch(url, {method: 'POST', headers: casedAdminHeaders()});
	const text = await resp.text();
	if (!resp.ok) {
		throw new Error(`POST ${url} status ${resp.status}: ${text.trim()}`);
	}
	try {
		return JSON.parse(text) as SentinelStatus;
	} catch (err) {
		throw new Error(`decode response: ${String(err)}`);
	}
}

export async function pauseSentinel(serverURL: string): Promise<SentinelStatus> {
	const url = apiURL(serverURL, '/v1/sentinel/pause');
	const resp = await fetch(url, {method: 'POST', headers: casedAdminHeaders()});
	const text = await resp.text();
	if (!resp.ok) {
		throw new Error(`POST ${url} status ${resp.status}: ${text.trim()}`);
	}
	try {
		return JSON.parse(text) as SentinelStatus;
	} catch (err) {
		throw new Error(`decode response: ${String(err)}`);
	}
}

export async function resumeSentinel(serverURL: string): Promise<SentinelStatus> {
	const url = apiURL(serverURL, '/v1/sentinel/resume');
	const resp = await fetch(url, {method: 'POST', headers: casedAdminHeaders()});
	const text = await resp.text();
	if (!resp.ok) {
		throw new Error(`POST ${url} status ${resp.status}: ${text.trim()}`);
	}
	try {
		return JSON.parse(text) as SentinelStatus;
	} catch (err) {
		throw new Error(`decode response: ${String(err)}`);
	}
}
