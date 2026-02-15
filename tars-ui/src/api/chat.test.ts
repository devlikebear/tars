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
