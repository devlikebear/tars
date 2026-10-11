import test from 'node:test'
import assert from 'node:assert/strict'

import { compileSvelteModule } from './helpers/compileSvelteModule.ts'
import type * as CompanionAskStoreModule from '../src/lib/stores/companionAskStore.svelte.ts'
import type { AgentRuntimeTierOption, ChatEvent, ChatRequest, Session } from '../src/lib/types.ts'

const { CompanionAskStore } = await compileSvelteModule<typeof CompanionAskStoreModule>(
  'src/lib/stores/companionAskStore.svelte.ts',
)

function mainSession(overrides: Partial<Session> = {}): Session {
  return {
    id: 'main-1',
    title: 'Main',
    kind: 'main',
    created_at: '2026-10-01T00:00:00Z',
    updated_at: '2026-10-01T00:00:00Z',
    ...overrides,
  }
}

type Fake = ReturnType<typeof fakeApi>

function fakeApi(opts: {
  session?: Session
  tiers?: AgentRuntimeTierOption[]
  defaultTier?: string
  feed?: ChatEvent[]
  failWith?: unknown
  // No session with kind "main" in the list — ensureMainSession never
  // resolves an id, even though the server always has a real main
  // session by the time it serves a request (tars#1236 review f3).
  noMainSession?: boolean
  // Like noMainSession, but only for the first N listSessions() calls —
  // deterministically simulates "a later listSessions() succeeds" by
  // call count rather than a timing-dependent state flip.
  noMainSessionForCalls?: number
} = {}) {
  const sent: ChatRequest[] = []
  const state = {
    session: opts.session ?? mainSession(),
    tiers: opts.tiers ?? [{ name: 'light', kind: 'mock' }, { name: 'standard', kind: 'mock' }],
    defaultTier: opts.defaultTier ?? 'standard',
    feed: opts.feed ?? [{ type: 'delta', text: 'hi' }, { type: 'done' }],
    failWith: opts.failWith,
    noMainSession: opts.noMainSession ?? false,
    noMainSessionForCalls: opts.noMainSessionForCalls,
    sent,
    listSessionsCalls: 0,
  }
  const api = {
    async listSessions() {
      state.listSessionsCalls++
      const noMain = state.noMainSession ||
        (state.noMainSessionForCalls !== undefined && state.listSessionsCalls <= state.noMainSessionForCalls)
      return noMain ? [] : [state.session]
    },
    async listAgentRuntimeSubagents() {
      return { count: 0, agents: [], tiers: state.tiers, default_tier: state.defaultTier }
    },
    async streamChat(request: ChatRequest, onEvent: (event: ChatEvent) => void, signal?: AbortSignal) {
      sent.push(request)
      if (state.failWith) throw state.failWith
      for (const event of state.feed) {
        if (signal?.aborted) return
        onEvent(event)
      }
    },
  }
  return { api, state }
}

test('ensureMainSession resolves and caches the real id', async () => {
  const { api, state } = fakeApi()
  const store = new CompanionAskStore(api)
  await store.ensureMainSession()
  assert.equal(store.mainSessionId, 'main-1')
  await store.ensureMainSession()
  assert.equal(state.listSessionsCalls, 1)
})

test('ask streams the answer and finishes done', async () => {
  const { api, state } = fakeApi()
  const store = new CompanionAskStore(api)
  const ok = await store.ask('anything new?', 'guidance', false)
  assert.equal(ok, true)
  assert.equal(store.status, 'done')
  assert.equal(store.question, 'anything new?')
  assert.equal(store.answer, 'hi')
  assert.equal(state.sent[0].session_id, 'main')
  assert.equal(state.sent[0].console_context, 'guidance')
  assert.equal(state.sent[0].tier_recommendation?.chosen_tier, 'light')
})

test('ask refuses without a turn when the main session is busy, and does not clear the composer', async () => {
  const { api, state } = fakeApi()
  const store = new CompanionAskStore(api)
  const ok = await store.ask('are you there?', 'guidance', true)
  assert.equal(ok, false)
  assert.equal(store.status, 'error')
  assert.equal(store.errorKind, 'busy')
  assert.equal(store.question, 'are you there?')
  assert.equal(state.sent.length, 0)
})

test('ask classifies a thrown failure and keeps the question visible', async () => {
  const { api } = fakeApi({ failWith: { status: 429, message: 'rate limited' } })
  const store = new CompanionAskStore(api)
  const ok = await store.ask('one more thing', 'guidance', false)
  assert.equal(ok, false)
  assert.equal(store.status, 'error')
  assert.equal(store.errorKind, 'limit')
  assert.equal(store.question, 'one more thing')
})

test('ask truncates a long streamed answer and reports truncated', async () => {
  const longWord = 'word '.repeat(200)
  const { api } = fakeApi({ feed: [{ type: 'delta', text: longWord }, { type: 'done' }] })
  const store = new CompanionAskStore(api)
  await store.ask('tell me everything', 'guidance', false)
  assert.equal(store.truncated, true)
  assert.ok(store.answer.length < longWord.length)
})

test('ask still sends to session_id "main" when listSessions never finds a kind-main entry', async () => {
  const { api, state } = fakeApi({ noMainSession: true })
  const store = new CompanionAskStore(api)
  const ok = await store.ask('anything new?', 'guidance', false)
  assert.equal(ok, true)
  assert.equal(state.sent[0].session_id, 'main')
  assert.equal(state.sent[0].tier_recommendation?.chosen_tier, 'light')
  // The id is still unknown — the busy pre-check and "Continue in chat"
  // just have nothing to read until a later listSessions() succeeds.
  assert.equal(store.mainSessionId, '')
})

test('ask re-resolves mainSessionId after a successful turn once listSessions recovers', async () => {
  // The first listSessions() (ask()'s own ensureMainSession) finds no
  // main session; the second (ask()'s post-success retry) does.
  const { api, state } = fakeApi({ noMainSessionForCalls: 1 })
  const store = new CompanionAskStore(api)
  await store.ask('anything new?', 'guidance', false)
  assert.equal(store.status, 'done')
  // The retry is fire-and-forget (ask() does not await it) — give it a
  // tick to land.
  await new Promise((resolve) => setTimeout(resolve, 0))
  assert.equal(store.mainSessionId, 'main-1')
  assert.equal(state.listSessionsCalls, 2)
})

test('ask refuses a second, overlapping call while one is already streaming', async () => {
  let resolveFirst: (() => void) | undefined
  const { api, state } = fakeApi({
    feed: [{ type: 'delta', text: 'first answer' }],
  })
  // Hold the first call's streamChat open until the second ask() has had
  // a chance to run, so both would be "in flight" without the guard. The
  // request itself is still recorded right away, same as a real fetch
  // sends its body long before the response finishes streaming back.
  let callCount = 0
  api.streamChat = async (request, onEvent, signal) => {
    callCount++
    state.sent.push(request)
    if (callCount === 1) {
      await new Promise<void>((resolve) => { resolveFirst = resolve })
    }
    if (state.failWith) throw state.failWith
    for (const event of state.feed) {
      if (signal?.aborted) return
      onEvent(event)
    }
  }
  const store = new CompanionAskStore(api)
  const first = store.ask('first question', 'guidance', false)
  // Let the chain ahead of the held streamChat call (ensureMainSession,
  // loadTierOptionsOnce, the tier lookup's own listSessions) fully drain
  // — a macrotask tick flushes every microtask queued so far, which a
  // fixed count of `await Promise.resolve()` cannot guarantee.
  await new Promise((resolve) => setTimeout(resolve, 0))
  assert.equal(store.busy, true)
  assert.equal(store.question, 'first question')
  const second = await store.ask('second question', 'guidance', false)
  assert.equal(second, false)
  // The second call never touched the store — the first question is
  // still what is showing.
  assert.equal(store.question, 'first question')
  assert.equal(state.sent.length, 1)
  resolveFirst?.()
  assert.equal(await first, true)
  assert.equal(store.question, 'first question')
  assert.equal(store.answer, 'first answer')
})

test('ask falls back off the light tier when the main session resumes a different CLI', async () => {
  const session = mainSession({ tier_pin: 'standard', upstream_session_id: 'claude-sess-1' })
  const tiers = [
    { name: 'light', kind: 'antigravity-cli' },
    { name: 'standard', kind: 'claude-code-cli' },
  ]
  const { api, state } = fakeApi({ session, tiers })
  const store = new CompanionAskStore(api)
  await store.ask('quick check', 'guidance', false)
  assert.equal(state.sent[0].tier_recommendation, undefined)
})

test('beginInThread/finishInThread report an outcome without an answer or a streamChat call', async () => {
  const { api, state } = fakeApi()
  const store = new CompanionAskStore(api)
  store.beginInThread('are you there?')
  assert.equal(store.inThread, true)
  assert.equal(store.busy, true)
  assert.equal(store.question, 'are you there?')
  assert.equal(store.answer, '')
  assert.equal(state.sent.length, 0)
  store.finishInThread(true)
  assert.equal(store.status, 'done')
  assert.equal(store.errorKind, null)
  store.beginInThread('one more')
  store.finishInThread(false, 'busy')
  assert.equal(store.status, 'error')
  assert.equal(store.errorKind, 'busy')
})

test('reset clears the question/answer and status', async () => {
  const { api } = fakeApi()
  const store = new CompanionAskStore(api)
  await store.ask('anything new?', 'guidance', false)
  assert.equal(store.hasContent, true)
  store.reset()
  assert.equal(store.hasContent, false)
  assert.equal(store.answer, '')
  assert.equal(store.status, 'idle')
  assert.equal(store.errorKind, null)
})
