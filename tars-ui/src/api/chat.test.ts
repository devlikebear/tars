import test from 'node:test';
import assert from 'node:assert/strict';
import {decodeSSEBuffer, streamChat} from './chat.js';

test('decodeSSEBuffer parses status/delta/done events', () => {
	const input =
		'data: {"type":"status","message":"calling llm"}\n\n' +
		'data: {"type":"delta","text":"hello"}\n\n' +
		'data: {"type":"done","session_id":"sess-1"}\n';

	const result = decodeSSEBuffer(input);
	assert.equal(result.events.length, 3);
	assert.equal(result.events[0]?.type, 'status');
	assert.equal(result.events[1]?.type, 'delta');
	assert.equal(result.events[2]?.type, 'done');
	assert.equal(result.remainder, '');
});

test('streamChat restores split chunks and returns assistant/session', async () => {
	const chunkA = 'data: {"type":"status","phase":"before_llm"}\n\ndata: {"type":"delta","text":"hel';
	const chunkB = 'lo"}\n\ndata: {"type":"delta","text":" world"}\n\ndata: {"type":"done","session_id":"sess-2"}\n\n';

	const stream = new ReadableStream<Uint8Array>({
		start(controller) {
			controller.enqueue(new TextEncoder().encode(chunkA));
			controller.enqueue(new TextEncoder().encode(chunkB));
			controller.close();
		},
	});

	globalThis.fetch = (async () =>
		new Response(stream, {
			status: 200,
			headers: {'Content-Type': 'text/event-stream'},
		})) as typeof fetch;

	const statuses: string[] = [];
	const phases: string[] = [];
	const deltas: string[] = [];
	const result = await streamChat({
		serverUrl: 'http://127.0.0.1:8080',
		sessionId: '',
		message: 'hi',
		onStatus: (line) => statuses.push(line),
		onStatusEvent: (evt) => phases.push(evt.phase ?? ''),
		onDelta: (chunk) => deltas.push(chunk),
	});

	assert.deepEqual(statuses, ['before_llm']);
	assert.deepEqual(phases, ['before_llm']);
	assert.deepEqual(deltas, ['hello', ' world']);
	assert.equal(result.assistantText, 'hello world');
	assert.equal(result.sessionId, 'sess-2');
});

test('streamChat formats status line with message/phase and tool name', async () => {
	const chunk =
		'data: {"type":"status","message":"calling tool","phase":"before_tool_call","tool_name":"session_status"}\n\n' +
		'data: {"type":"status","phase":"after_tool_call","tool_name":"session_status"}\n\n' +
		'data: {"type":"done","session_id":"sess-3"}\n\n';

	const stream = new ReadableStream<Uint8Array>({
		start(controller) {
			controller.enqueue(new TextEncoder().encode(chunk));
			controller.close();
		},
	});

	globalThis.fetch = (async () =>
		new Response(stream, {
			status: 200,
			headers: {'Content-Type': 'text/event-stream'},
		})) as typeof fetch;

	const statuses: string[] = [];
	const result = await streamChat({
		serverUrl: 'http://127.0.0.1:8080',
		sessionId: '',
		message: 'hi',
		onStatus: (line) => statuses.push(line),
		onDelta: () => {},
	});

	assert.deepEqual(statuses, ['calling tool (session_status)', 'after_tool_call (session_status)']);
	assert.equal(result.sessionId, 'sess-3');
	assert.equal(result.assistantText, '');
});

test('streamChat reports endpoint when fetch fails', async () => {
	const originalFetch = globalThis.fetch;
	globalThis.fetch = (async () => {
		throw new TypeError('fetch failed');
	}) as typeof fetch;

	try {
		await assert.rejects(
			() =>
				streamChat({
					serverUrl: 'http://127.0.0.1:43180',
					sessionId: '',
					message: 'hi',
					onStatus: () => {},
					onDelta: () => {},
				}),
			/chat endpoint http:\/\/127\.0\.0\.1:43180\/v1\/chat request failed: TypeError: fetch failed/,
		);
	} finally {
		globalThis.fetch = originalFetch;
	}
});
