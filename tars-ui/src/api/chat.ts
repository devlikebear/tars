export type ChatSSEEvent = {
	type?: string;
	text?: string;
	error?: string;
	session_id?: string;
	phase?: string;
	message?: string;
	tool_name?: string;
};

export type StreamChatParams = {
	serverUrl: string;
	sessionId: string;
	message: string;
	onStatus: (line: string) => void;
	onStatusEvent?: (evt: ChatSSEEvent) => void;
	onDelta: (chunk: string) => void;
	onDebug?: (line: string) => void;
};

export type StreamChatResult = {
	sessionId: string;
	assistantText: string;
};

type DecodeResult = {
	events: ChatSSEEvent[];
	remainder: string;
};

function trimURL(url: string): string {
	return url.replace(/\/+$/, '');
}

export function decodeSSEBuffer(buffer: string): DecodeResult {
	const normalized = buffer.replace(/\r\n/g, '\n');
	const lines = normalized.split('\n');
	const remainder = lines.pop() ?? '';
	const events: ChatSSEEvent[] = [];

	for (const rawLine of lines) {
		const line = rawLine.trim();
		if (!line.startsWith('data:')) {
			continue;
		}
		const payload = line.slice('data:'.length).trim();
		if (payload === '') {
			continue;
		}
		let evt: ChatSSEEvent;
		try {
			evt = JSON.parse(payload) as ChatSSEEvent;
		} catch (err) {
			throw new Error(`decode sse event: ${String(err)}`);
		}
		events.push(evt);
	}

	return {events, remainder};
}

function statusLineFromEvent(evt: ChatSSEEvent): string {
	let status = (evt.message ?? '').trim();
	if (status === '') {
		status = (evt.phase ?? '').trim();
	}
	if ((evt.tool_name ?? '').trim() !== '') {
		status = `${status} (${evt.tool_name!.trim()})`;
	}
	return status;
}

export async function streamChat(params: StreamChatParams): Promise<StreamChatResult> {
	const endpoint = `${trimURL(params.serverUrl)}/v1/chat`;
	const payload: Record<string, string> = {message: params.message};
	if (params.sessionId.trim() !== '') {
		payload.session_id = params.sessionId.trim();
	}

	params.onDebug?.(`POST ${endpoint}`);
	const resp = await fetch(endpoint, {
		method: 'POST',
		headers: {'Content-Type': 'application/json'},
		body: JSON.stringify(payload),
	});
	if (!resp.ok) {
		const body = await resp.text();
		throw new Error(`chat endpoint status ${resp.status}: ${body.trim()}`);
	}
	if (!resp.body) {
		throw new Error('chat endpoint returned empty body');
	}

	const reader = resp.body.getReader();
	const decoder = new TextDecoder('utf-8');
	let buffer = '';
	let assistantText = '';
	let currentSessionID = params.sessionId.trim();

	while (true) {
		const {value, done} = await reader.read();
		if (done) {
			break;
		}

		buffer += decoder.decode(value, {stream: true});
		const decoded = decodeSSEBuffer(buffer);
		buffer = decoded.remainder;

		for (const evt of decoded.events) {
			switch (evt.type) {
			case 'status': {
				params.onStatusEvent?.(evt);
				const status = statusLineFromEvent(evt);
				if (status !== '') {
					params.onStatus(status);
				}
				break;
			}
			case 'delta': {
				const chunk = evt.text ?? '';
				assistantText += chunk;
				params.onDelta(chunk);
				break;
			}
			case 'error': {
				throw new Error((evt.error ?? 'chat stream error').trim());
			}
			case 'done': {
				const nextID = (evt.session_id ?? '').trim();
				if (nextID !== '') {
					currentSessionID = nextID;
				}
				return {sessionId: currentSessionID, assistantText};
			}
			}
		}
	}

	return {sessionId: currentSessionID, assistantText};
}
