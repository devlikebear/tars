import {CronJob} from '../types.js';

function apiURL(serverURL: string, path: string): string {
	return `${serverURL.replace(/\/+$/, '')}${path}`;
}

async function requestJSON<T>(method: string, url: string, body?: unknown): Promise<T> {
	const resp = await fetch(url, {
		method,
		headers: body === undefined ? undefined : {'Content-Type': 'application/json'},
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

export async function listCronJobs(serverURL: string): Promise<CronJob[]> {
	return requestJSON<CronJob[]>('GET', apiURL(serverURL, '/v1/cron/jobs'));
}

export async function createCronJob(
	serverURL: string,
	input: {name?: string; prompt: string; schedule: string; enabled?: boolean; delete_after_run?: boolean},
): Promise<CronJob> {
	return requestJSON<CronJob>('POST', apiURL(serverURL, '/v1/cron/jobs'), input);
}

export async function runCronJob(serverURL: string, jobID: string): Promise<string> {
	const payload = await requestJSON<{response: string}>('POST', apiURL(serverURL, `/v1/cron/jobs/${jobID}/run`));
	return payload.response.trim();
}

export async function deleteCronJob(serverURL: string, jobID: string): Promise<void> {
	const resp = await fetch(apiURL(serverURL, `/v1/cron/jobs/${jobID}`), {method: 'DELETE'});
	if (!resp.ok) {
		const text = await resp.text();
		throw new Error(`DELETE /v1/cron/jobs/${jobID} status ${resp.status}: ${text.trim()}`);
	}
}
