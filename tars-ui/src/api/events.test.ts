import test from 'node:test';
import assert from 'node:assert/strict';
import {watchNotifications} from './events.js';

test('watchNotifications forwards notification events', async () => {
	const chunks = [
		'data: {"type":"notification","title":"Cron done","message":"ok"}\n',
		'\n',
	];
	const stream = new ReadableStream<Uint8Array>({
		start(controller) {
			for (const chunk of chunks) {
				controller.enqueue(new TextEncoder().encode(chunk));
			}
			controller.close();
		},
	});
	const originalFetch = globalThis.fetch;
	globalThis.fetch = (async () =>
		new Response(stream, {
			status: 200,
			headers: {'Content-Type': 'text/event-stream'},
		})) as typeof fetch;

	const seen: Array<{title?: string; message?: string}> = [];
	try {
		await watchNotifications({
			serverUrl: 'http://127.0.0.1:8080',
			onEvent: (evt) => seen.push({title: evt.title, message: evt.message}),
		});
	} finally {
		globalThis.fetch = originalFetch;
	}

	assert.equal(seen.length, 1);
	assert.equal(seen[0]?.title, 'Cron done');
});
