import test from 'node:test'
import assert from 'node:assert/strict'

import { APIRequestError } from '../src/lib/api/client.ts'
import type { RemoteChatAPI, RemoteChatHistory } from '../src/lib/mobile/remoteChat.ts'
import type { PairedDesktop } from '../src/lib/mobile/desktops.ts'
import type { ChatEvent, SessionMessage } from '../src/lib/types.ts'
import { compileSvelteModule } from './helpers/compileSvelteModule.ts'
import type * as RemoteChatStoreModule from '../src/lib/mobile/remoteChatStore.svelte.ts'

const { RemoteChatStore } = await compileSvelteModule<typeof RemoteChatStoreModule>('src/lib/mobile/remoteChatStore.svelte.ts')

function desktop(id: string): PairedDesktop {
  return { id, name: id, baseURL: `https://${id}.ts.net`, token: `token-${id}`, deviceID: `dev-${id}`, pairedAt: '2026-11-18T00:00:00Z' }
}

function message(id: string, role: string, content: string): SessionMessage {
  return { id, role, content, timestamp: '2026-11-18T00:00:00Z' }
}

// deferred lets a test control exactly when a fake API call resolves, to
// exercise what happens while one is still in flight.
function deferred<T>(): { promise: Promise<T>; resolve: (v: T) => void; reject: (e: unknown) => void } {
  let resolve!: (v: T) => void
  let reject!: (e: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

// fakeAPI builds a RemoteChatAPI whose ensureSession/history answer fixed
// values and whose send/attach/cancel/answerPermission are spies the test
// can inspect and drive by hand.
function fakeAPI(overrides: Partial<RemoteChatAPI> = {}): RemoteChatAPI & { calls: { send: unknown[]; attach: unknown[]; cancel: unknown[]; answerPermission: unknown[] } } {
  const calls = { send: [] as unknown[], attach: [] as unknown[], cancel: [] as unknown[], answerPermission: [] as unknown[] }
  return {
    calls,
    async ensureSession() {
      return 'sess-1'
    },
    async history() {
      return { sessionId: 'sess-1', messages: [] } as RemoteChatHistory
    },
    async send(target, sessionId, text, _onEvent, _signal) {
      calls.send.push([target.baseURL, sessionId, text])
    },
    async attach(target, sessionId, _onEvent, _signal) {
      calls.attach.push([target.baseURL, sessionId])
      return false
    },
    async cancel(target, sessionId) {
      calls.cancel.push([target.baseURL, sessionId])
      return true
    },
    async answerPermission(target, requestId, sessionId, decision) {
      calls.answerPermission.push([target.baseURL, requestId, sessionId, decision])
    },
    ...overrides,
  }
}

test('connect ensures the session and loads its history', async () => {
  const api = fakeAPI({
    async history() {
      return { sessionId: 'sess-1', messages: [message('m1', 'user', 'hi'), message('m2', 'assistant', 'hello')] }
    },
  })
  const store = new RemoteChatStore({ api })
  await store.connect(desktop('a'))
  assert.equal(store.sessionId, 'sess-1')
  assert.deepEqual(store.messages.map((m: { role: string; text: string }) => [m.role, m.text]), [
    ['user', 'hi'],
    ['assistant', 'hello'],
  ])
  assert.equal(store.error, '')
  assert.equal(store.loading, false)
})

test('connecting to a second desktop before the first finishes keeps only the second', async () => {
  const slowA = deferred<string>()
  const api = fakeAPI({
    async ensureSession(target) {
      return target.baseURL === 'https://a.ts.net' ? slowA.promise : 'sess-b'
    },
  })
  const store = new RemoteChatStore({ api })
  const first = store.connect(desktop('a'))
  // Before "a" answers, the screen moves to "b".
  await store.connect(desktop('b'))
  assert.equal(store.sessionId, 'sess-b')
  slowA.resolve('sess-from-a')
  await first
  // "a"'s late answer must not clobber "b"'s session.
  assert.equal(store.sessionId, 'sess-b')
})

test('a connect failure is unpaired for 401/403 and failed otherwise', async () => {
  const unpaired = new RemoteChatStore({
    api: fakeAPI({
      ensureSession: async () => {
        throw new APIRequestError('forbidden', 403)
      },
    }),
  })
  await unpaired.connect(desktop('a'))
  assert.equal(unpaired.errorKind, 'unpaired')
  assert.equal(unpaired.loading, false)

  const failed = new RemoteChatStore({
    api: fakeAPI({
      ensureSession: async () => {
        throw new Error('network down')
      },
    }),
  })
  await failed.connect(desktop('a'))
  assert.equal(failed.errorKind, 'failed')
  assert.equal(failed.error, 'network down')
})

test('send streams a reply and reloads history once the turn ends', async () => {
  let historyCalls = 0
  const api = fakeAPI({
    async history() {
      historyCalls += 1
      return historyCalls === 1
        ? { sessionId: 'sess-1', messages: [] }
        : { sessionId: 'sess-1', messages: [message('m1', 'user', 'hi'), message('m2', 'assistant', 'hello there')] }
    },
    async send(_target, _sessionId, _text, onEvent) {
      onEvent({ type: 'delta', text: 'hel' } as ChatEvent)
      onEvent({ type: 'delta', text: 'lo' } as ChatEvent)
    },
  })
  const store = new RemoteChatStore({ api })
  await store.connect(desktop('a'))
  await store.send('hi')
  assert.equal(store.running, false)
  // The optimistic send plus the streamed reply settle into the reloaded
  // transcript's two messages.
  assert.deepEqual(store.messages.map((m: { role: string; text: string }) => [m.role, m.text]), [
    ['user', 'hi'],
    ['assistant', 'hello there'],
  ])
})

test('send does nothing with blank text, no session, or a turn already running', async () => {
  const api = fakeAPI()
  const store = new RemoteChatStore({ api })
  await store.send('  ') // not connected yet: no session id
  assert.equal(api.calls.send.length, 0)
  await store.connect(desktop('a'))
  await store.send('   ')
  assert.equal(api.calls.send.length, 0)
})

test('a turn already running on the desk is followed, and disconnect stops watching it', async () => {
  let aborted = false
  const api = fakeAPI({
    async attach(_target, _sessionId, onEvent, signal) {
      onEvent({ type: 'delta', text: 'from the desk' } as ChatEvent)
      return new Promise<boolean>((resolve) => {
        signal?.addEventListener('abort', () => {
          aborted = true
          resolve(true)
        })
      })
    },
  })
  const store = new RemoteChatStore({ api })
  await store.connect(desktop('a'))
  assert.equal(store.running, true)
  assert.equal(store.messages.at(-1)?.text, 'from the desk')
  store.disconnect()
  assert.equal(aborted, true)
})

test('cancel and answerPermission reach the connected desktop with the session id', async () => {
  const api = fakeAPI()
  const store = new RemoteChatStore({ api })
  await store.connect(desktop('a'))
  await store.cancel()
  assert.deepEqual(api.calls.cancel, [['https://a.ts.net', 'sess-1']])

  await store.answerPermission(
    { requestId: 'r1', sessionId: 'sess-1', toolName: 'Bash', state: 'pending' },
    'allow_once',
  )
  assert.deepEqual(api.calls.answerPermission, [['https://a.ts.net', 'r1', 'sess-1', 'allow_once']])
})

test('a failed answerPermission puts the card back to pending with an error', async () => {
  const api = fakeAPI({
    async answerPermission() {
      throw new Error('boom')
    },
  })
  const store = new RemoteChatStore({ api })
  await store.connect(desktop('a'))
  store.messages = [
    { id: 'approval-r1', role: 'approval', text: '', approval: { requestId: 'r1', sessionId: 'sess-1', toolName: 'Bash', state: 'pending' } },
  ]
  await store.answerPermission(store.messages[0].approval!, 'deny')
  assert.equal(store.messages[0].approval?.state, 'pending')
  assert.equal(store.messages[0].approval?.error, 'failed')
})
