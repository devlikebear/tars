import test from 'node:test'
import assert from 'node:assert/strict'

import { remoteFocusApi } from '../src/lib/mobile/focusRemote.ts'
import { runningPipelines } from '../src/lib/mobile/inbox.ts'
import type { FocusListItem } from '../src/lib/types.ts'

type Seen = { url: string; method: string; auth: string | null; credentials?: RequestCredentials; body?: string }

async function withFetch(answer: (url: string, init?: RequestInit) => Response, run: () => Promise<void>): Promise<Seen[]> {
  const original = globalThis.fetch
  const seen: Seen[] = []
  globalThis.fetch = async (input, init) => {
    const url = String(input)
    seen.push({
      url,
      method: init?.method ?? 'GET',
      auth: new Headers(init?.headers).get('Authorization'),
      credentials: init?.credentials,
      body: typeof init?.body === 'string' ? init.body : undefined,
    })
    return answer(url, init)
  }
  try {
    await run()
  } finally {
    globalThis.fetch = original
  }
  return seen
}

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
const target = { baseURL: 'https://mac.ts.net/', token: 'phone' }

test('the pipeline screen reads through the pipeline routes on its desktop, never /v1/admin', async () => {
  const api = remoteFocusApi(target, 'p1')
  const seen = await withFetch(
    (url) => (url.includes('/diff') ? json({ files: [] }) : url.includes('/checkpoints') ? json({ turns: [], reverts: [] }) : json(url.includes('/history') ? [] : {})),
    async () => {
      await api.getPipeline('p1')
      await api.getSession('p1')
      await api.getHistory('p1')
      await api.getHistory('qa-1')
      await api.listCheckpoints('p1')
      await api.getCheckpointDiff('p1', 't 1', { scope: 'turn' })
      await api.activity?.()
    },
  )
  assert.deepEqual(seen.map((s) => s.url), [
    'https://mac.ts.net/v1/focus/pipelines/p1',
    'https://mac.ts.net/v1/focus/pipelines/p1/session',
    'https://mac.ts.net/v1/focus/pipelines/p1/history',
    // The Q&A session's transcript, through its pipeline.
    'https://mac.ts.net/v1/focus/pipelines/p1/history?session=qa',
    'https://mac.ts.net/v1/focus/pipelines/p1/checkpoints',
    'https://mac.ts.net/v1/focus/pipelines/p1/checkpoints/t%201/diff?scope=turn',
    'https://mac.ts.net/v1/chat/activity',
  ])
  for (const s of seen) {
    assert.equal(s.auth, 'Bearer phone')
    assert.equal(s.credentials, 'omit')
  }
})

test('actions go to the desktop, and a 409 with the pipeline is a conflict result', async () => {
  const api = remoteFocusApi(target, 'p1')
  const pipeline = { session_id: 'p1', current: 'build' }
  let gateResult: unknown
  const seen = await withFetch(
    (url) => (url.endsWith('/gates/plan') ? json({ error: 'gate closed', pipeline }, 409) : json({ pipeline, next_prompt: 'next' })),
    async () => {
      gateResult = await api.gate('p1', 'plan', 'approve', { card_id: 'g1' })
      await api.card('p1', 'c 1', 'decided', 'Hello')
      await api.advance('p1', 'build')
      await api.stop('p1')
      await api.goal?.('p1', false)
      await api.addFinding?.('p1', { title: 'x' })
      await api.ask?.('p1', 'c1', 'why?')
    },
  )
  assert.deepEqual(gateResult, { pipeline, next_prompt: '', conflict: true })
  assert.deepEqual(seen.map((s) => `${s.method} ${s.url.replace('https://mac.ts.net', '')} ${s.body}`), [
    'POST /v1/focus/pipelines/p1/gates/plan {"action":"approve","card_id":"g1"}',
    'POST /v1/focus/pipelines/p1/cards/c%201 {"state":"decided","decision":"Hello"}',
    'POST /v1/focus/pipelines/p1/advance {"stage":"build"}',
    'POST /v1/focus/pipelines/p1/stop {}',
    'POST /v1/focus/pipelines/p1/goal {"enabled":false}',
    'POST /v1/focus/pipelines/p1/findings {"title":"x"}',
    'POST /v1/focus/pipelines/p1/qa {"card_id":"c1","question":"why?"}',
  ])
  assert.ok(seen.every((s) => s.auth === 'Bearer phone'))
})

test('the turn feed streams from the desktop with the token', async () => {
  const api = remoteFocusApi(target, 'p1')
  const events: unknown[] = []
  let attached = false
  let idle = true
  const seen = await withFetch(
    (url) => (url.includes('session_id=qa') ? new Response(null, { status: 204 }) : new Response('data: {"type":"delta","text":"hi"}\n\n', { status: 200 })),
    async () => {
      attached = await api.attachChatStream('p1', (e) => events.push(e))
      idle = await api.attachChatStream('qa', () => {})
      await api.streamChat({ session_id: 'p1', message: 'go' } as never, (e) => events.push(e))
    },
  )
  assert.equal(attached, true)
  assert.equal(idle, false)
  assert.deepEqual(events, [{ type: 'delta', text: 'hi' }, { type: 'delta', text: 'hi' }])
  assert.deepEqual(seen.map((s) => `${s.method} ${s.url} ${s.auth} ${s.credentials}`), [
    'GET https://mac.ts.net/v1/chat/stream?session_id=p1 Bearer phone omit',
    'GET https://mac.ts.net/v1/chat/stream?session_id=qa Bearer phone omit',
    'POST https://mac.ts.net/v1/chat Bearer phone omit',
  ])
})

test('a desktop lists the pipelines that can be continued: active or blocked, latest first', () => {
  const item = (id: string, phase: FocusListItem['phase'], at: string): FocusListItem => ({
    session_id: id, title: id, goal: id, current: 'build', open_gate: '', needs_input: 0, updated_at: at, phase,
  })
  const list = runningPipelines([
    item('done', 'finished', '2026-11-14T12:00:00Z'),
    item('old', 'active', '2026-11-14T08:00:00Z'),
    item('gate', 'blocked', '2026-11-14T10:00:00Z'),
    item('halt', 'stopped', '2026-11-14T11:00:00Z'),
    item('legacy', undefined, '2026-11-14T09:00:00Z'),
  ])
  assert.deepEqual(list.map((p) => p.session_id), ['gate', 'legacy', 'old'])
})
