export type NotificationEvent = {
	type?: string;
	category?: string;
	severity?: string;
	title?: string;
	message?: string;
	timestamp?: string;
	job_id?: string;
	session_id?: string;
};

export type WatchNotificationsParams = {
	serverUrl: string;
	onEvent: (evt: NotificationEvent) => void;
	onDebug?: (line: string) => void;
	signal?: AbortSignal;
};

function trimURL(url: string): string {
	return url.replace(/\/+$/, '');
}

function decodeSSEEvents(chunk: string): {events: NotificationEvent[]; remainder: string} {
	const normalized = chunk.replace(/\r\n/g, '\n');
	const lines = normalized.split('\n');
	const remainder = lines.pop() ?? '';
	const events: NotificationEvent[] = [];
	for (const lineRaw of lines) {
		const line = lineRaw.trim();
		if (!line.startsWith('data:')) {
			continue;
		}
		const payload = line.slice('data:'.length).trim();
		if (payload === '') {
			continue;
		}
		let evt: NotificationEvent;
		try {
			evt = JSON.parse(payload) as NotificationEvent;
		} catch (error) {
			throw new Error(`decode notification event: ${String(error)}`);
		}
		events.push(evt);
	}
	return {events, remainder};
}

export async function watchNotifications(params: WatchNotificationsParams): Promise<void> {
	const endpoint = `${trimURL(params.serverUrl)}/v1/events/stream`;
	params.onDebug?.(`GET ${endpoint}`);
	const startTime = Date.now();
	let resp: Response;
	try {
		resp = await fetch(endpoint, {
			method: 'GET',
			headers: {'Accept': 'text/event-stream'},
			signal: params.signal,
		});
	} catch (error) {
		throw new Error(`event stream ${endpoint} request failed: ${String(error)}`);
	}
	if (!resp.ok) {
		const body = await resp.text();
		if (resp.status === 503) {
			throw new Error(`event broker not available at ${endpoint} (check tarsd configuration)`);
		}
		throw new Error(`event stream ${endpoint} status ${resp.status}: ${body.trim()}`);
	}
	if (!resp.body) {
		throw new Error('event stream returned empty body');
	}
	const reader = resp.body.getReader();
	const decoder = new TextDecoder('utf-8');
	let buffer = '';
	let eventCount = 0;
	while (true) {
		const {value, done} = await reader.read();
		if (done) {
			const elapsed = Date.now() - startTime;
			if (elapsed < 2000 && eventCount === 0) {
				throw new Error(`event stream ${endpoint} closed immediately without sending data (check server_url and running tarsd)`);
			}
			return;
		}
		buffer += decoder.decode(value, {stream: true});
		const decoded = decodeSSEEvents(buffer);
		buffer = decoded.remainder;
		for (const evt of decoded.events) {
			eventCount++;
			if ((evt.type ?? '').trim() === 'notification') {
				params.onEvent(evt);
			}
		}
	}
}
