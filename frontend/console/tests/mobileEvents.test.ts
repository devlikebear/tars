import test from 'node:test'
import assert from 'node:assert/strict'

import { APIRequestError, type APITarget } from '../src/lib/api/client.ts'
import { DesktopEvents, pipelineChanges, touches, type DesktopChange, type EventsAPI, type FocusStreamEvent } from '../src/lib/mobile/desktopEvents.ts'
import type { PairedDesktop } from '../src/lib/mobile/desktops.ts'

// A stream the test drives: send events, end it, or break it.
type FakeStream = {
  target: APITarget
  signal: AbortSignal
  send(event: FocusStreamEvent): void
  end(): void
  fail(err: unknown): void
}

function fakeAPI() {
  const streams: FakeStream[] = []
  const api: EventsAPI = {
    open(target, onEvent, signal) {
      return new Promise<void>((resolve, reject) => {
        streams.push({ target, signal, send: onEvent, end: resolve, fail: reject })
      })
    },
  }
  return { api, streams }
}

function desk(id: string, token = 'tok'): PairedDesktop {
  return { id, name: id, baseURL: id, token, deviceID: 'd', pairedAt: '2026-11-15T00:00:00Z' }
}

const tick = (ms = 0) => new Promise((resolve) => setTimeout(resolve, ms))

function setup() {
  const { api, streams } = fakeAPI()
  const events = new DesktopEvents({ api, retryMs: [5], settleMs: 5 })
  const heard: string[] = []
  const changes: DesktopChange[] = []
  events.listen((change) => {
    heard.push(change.desktopID)
    changes.push(change)
  })
  return { events, streams, heard, changes }
}

test('a pipeline event reads that desktop again, once per burst', async () => {
  const { events, streams, heard } = setup()
  events.watch([desk('https://a.ts.net'), desk('https://b.ts.net')])
  assert.equal(streams.length, 2)
  assert.equal(streams[0].target.token, 'tok')

  streams[0].send({ type: 'connected' })
  streams[0].send({ type: 'keepalive' })
  await tick(15)
  assert.deepEqual(heard, [], 'connecting the first time reads nothing: the app just read')

  streams[0].send({ type: 'pipeline', session_id: 's1' })
  streams[0].send({ type: 'pipeline', session_id: 's2' })
  streams[1].send({ type: 'pipeline', session_id: 's3' })
  await tick(15)
  assert.deepEqual(heard.sort(), ['https://a.ts.net', 'https://b.ts.net'])
  events.close()
})

test('watching again keeps open streams, opens new ones and closes the rest', () => {
  const { events, streams } = setup()
  events.watch([desk('https://a.ts.net'), desk('https://b.ts.net')])
  events.watch([desk('https://a.ts.net'), desk('https://c.ts.net')])
  assert.equal(streams.length, 3)
  assert.equal(streams[0].signal.aborted, false)
  assert.equal(streams[1].signal.aborted, true, 'b is no longer watched')
  assert.equal(streams[2].target.baseURL, 'https://c.ts.net')

  // A new token (paired again) opens a new stream.
  events.watch([desk('https://a.ts.net', 'new'), desk('https://c.ts.net')])
  assert.equal(streams[0].signal.aborted, true)
  assert.equal(streams[3].target.token, 'new')
  events.close()
  assert.equal(streams[3].signal.aborted, true)
})

test('a dropped stream opens again and reads the desktop once it is back', async () => {
  const { events, streams, heard } = setup()
  events.watch([desk('https://a.ts.net')])
  streams[0].send({ type: 'connected' })
  streams[0].fail(new TypeError('network connection was lost'))
  await tick(15)
  assert.equal(streams.length, 2, 'retried')
  // A desktop that ended the stream (restarting) is retried too.
  streams[1].end()
  await tick(15)
  assert.equal(streams.length, 3)
  assert.deepEqual(heard, [])
  streams[2].send({ type: 'connected' })
  await tick(15)
  assert.deepEqual(heard, ['https://a.ts.net'], 'changes may have been missed while it was down')
  events.close()
})

test('a desktop that refuses the stream is not asked again', async () => {
  const { events, streams } = setup()
  events.watch([desk('https://old.ts.net')])
  streams[0].fail(new APIRequestError('404 Not Found', 404))
  await tick(15)
  assert.equal(streams.length, 1)
  events.reconnect()
  assert.equal(streams.length, 1, 'too old for the stream')
  // Watched afresh (paired again), it is asked again.
  events.watch([desk('https://old.ts.net', 'again')])
  assert.equal(streams.length, 2)
  events.close()
})

test('coming back to the front opens every stream again at once', async () => {
  const { events, streams, heard } = setup()
  events.watch([desk('https://a.ts.net')])
  streams[0].send({ type: 'connected' })
  events.reconnect()
  assert.equal(streams[0].signal.aborted, true)
  assert.equal(streams.length, 2)
  // The old stream's end after the abort is ignored.
  streams[0].fail(new DOMException('aborted', 'AbortError'))
  streams[1].send({ type: 'connected' })
  await tick(15)
  assert.equal(streams.length, 2)
  assert.deepEqual(heard, [], 'the app reads everything on resume itself')
  events.close()
})

test('events from a stream that was replaced or closed are dropped', async () => {
  const { events, streams, heard } = setup()
  const stop = events.listen(() => heard.push('second'))
  events.watch([desk('https://a.ts.net')])
  events.watch([])
  streams[0].send({ type: 'pipeline', session_id: 's1' })
  await tick(15)
  assert.deepEqual(heard, [])
  stop()
  events.watch([desk('https://a.ts.net')])
  streams[1].send({ type: 'pipeline', session_id: 's1' })
  await tick(15)
  assert.deepEqual(heard, ['https://a.ts.net'])
  events.close()
})

test('a call back names the pipelines that changed, or all of them after a drop', async () => {
  const { events, streams, changes } = setup()
  events.watch([desk('https://a.ts.net')])
  streams[0].send({ type: 'connected' })
  streams[0].send({ type: 'pipeline', session_id: 's2' })
  streams[0].send({ type: 'pipeline', session_id: 's1' })
  streams[0].send({ type: 'pipeline', session_id: 's2' })
  await tick(15)
  assert.deepEqual(changes, [{ desktopID: 'https://a.ts.net', sessions: ['s1', 's2'], all: false }])

  streams[0].fail(new TypeError('lost'))
  await tick(15)
  streams[1].send({ type: 'connected' })
  streams[1].send({ type: 'pipeline', session_id: 's3' })
  await tick(15)
  assert.deepEqual(changes[1], { desktopID: 'https://a.ts.net', sessions: ['s3'], all: true })
  events.close()
})

test('a stream is live from "connected" until it ends', async () => {
  const { events, streams } = setup()
  events.watch([desk('https://a.ts.net')])
  assert.equal(events.isLive('https://a.ts.net'), false, 'opening is not yet live')
  streams[0].send({ type: 'connected' })
  assert.equal(events.isLive('https://a.ts.net'), true)
  streams[0].fail(new TypeError('lost'))
  await tick(0)
  assert.equal(events.isLive('https://a.ts.net'), false, 'a dropped stream is not live while it retries')
  await tick(15)
  streams[1].send({ type: 'connected' })
  assert.equal(events.isLive('https://a.ts.net'), true)
  events.reconnect()
  assert.equal(events.isLive('https://a.ts.net'), false)
  events.close()
  assert.equal(events.isLive('https://b.ts.net'), false, 'a desktop not watched')
})

test('a pipeline screen hears only its own pipeline, and everything after a drop', async () => {
  const change = (sessions: string[], all = false): DesktopChange => ({ desktopID: 'https://a.ts.net', sessions, all })
  assert.equal(touches(change(['s1']), 'https://a.ts.net', 's1'), true)
  assert.equal(touches(change(['s2']), 'https://a.ts.net', 's1'), false)
  assert.equal(touches(change([], true), 'https://a.ts.net', 's1'), true)
  assert.equal(touches(change(['s1']), 'https://b.ts.net', 's1'), false)

  const { events, streams } = setup()
  events.watch([desk('https://a.ts.net'), desk('https://b.ts.net')])
  const screen = pipelineChanges(events, 'https://a.ts.net', 's1')
  let polls = 0
  const stop = screen.listen(() => polls++)
  streams[0].send({ type: 'connected' })
  assert.equal(screen.live(), true)
  streams[0].send({ type: 'pipeline', session_id: 's2' })
  streams[1].send({ type: 'pipeline', session_id: 's1' })
  await tick(15)
  assert.equal(polls, 0, 'another pipeline, or the same id on another desktop')
  streams[0].send({ type: 'pipeline', session_id: 's1' })
  await tick(15)
  assert.equal(polls, 1)
  stop()
  streams[0].send({ type: 'pipeline', session_id: 's1' })
  await tick(15)
  assert.equal(polls, 1, 'stopped')
  events.close()
})
