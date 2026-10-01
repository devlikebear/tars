import test from 'node:test'
import assert from 'node:assert/strict'

import { compileSvelteModule } from './helpers/compileSvelteModule.ts'
import type * as FocusStoreModule from '../src/lib/stores/focusStore.svelte.ts'
import type { ChatEvent, ChatRequest, FocusActionResult, FocusCard, FocusPipeline, SessionMessage } from '../src/lib/types.ts'

const { FocusStore } = await compileSvelteModule<typeof FocusStoreModule>('src/lib/stores/focusStore.svelte.ts')

function pipeline(updated: string, cards: FocusCard[] = [], extra: Partial<FocusPipeline> = {}): FocusPipeline {
  return {
    version: 1,
    session_id: 's1',
    goal: 'g',
    current: 'plan',
    stages: [
      { id: 'plan', status: 'active', iteration: 1 },
      { id: 'build', status: 'pending', iteration: 0 },
      { id: 'review', status: 'pending', iteration: 0 },
      { id: 'pr', status: 'pending', iteration: 0 },
      { id: 'pr_review', status: 'pending', iteration: 0 },
      { id: 'merge', status: 'pending', iteration: 0 },
    ],
    open_gate: '',
    cards,
    updated_at: updated,
    ...extra,
  }
}

function gateCard(): FocusCard {
  return { id: 'c1', kind: 'gate', stage: 'plan', turn: 1, title: 'Approve plan', state: 'unseen', created_at: '2026-10-01T00:00:01Z', payload: { goal: 'g', tasks: [{ title: 't', done: 'd' }], stages: ['plan', 'build'], verify: ['make test'] } }
}

function memoryStorage() {
  const data = new Map<string, string>()
  return { data, getItem: (k: string) => data.get(k) ?? null, setItem: (k: string, v: string) => { data.set(k, v) } }
}

type Fake = ReturnType<typeof fakeApi>

// A fake server: chat turns are scripted per call; attach replays `feed`.
function fakeApi(initial: FocusPipeline) {
  const state = {
    pipeline: initial,
    history: [] as SessionMessage[],
    sent: [] as ChatRequest[],
    turns: [] as ChatEvent[][],
    feed: null as ChatEvent[] | null,
    gateResult: null as FocusActionResult | null,
    cardResult: null as FocusActionResult | null,
    gateCalls: [] as unknown[][],
    cardCalls: [] as unknown[][],
    checkpoints: [] as { turn_id: string; files: number; additions: number; deletions: number; started_at: string; ended_at?: string; skipped?: string }[],
    diffs: {} as Record<string, { path: string; status: string; additions: number; deletions: number; patch?: string }[]>,
    diffCalls: 0,
    // Resolves the attach stream when set; lets a test hold it open.
    holdAttach: null as Promise<void> | null,
  }
  const api = {
    getPipeline: async () => state.pipeline,
    getSession: async (id: string) => ({ id, title: 'Task', current_dir: 'repo', created_at: '', updated_at: '' }),
    getHistory: async () => state.history,
    listCheckpoints: async (id: string) => ({ session_id: id, turns: state.checkpoints }),
    getCheckpointDiff: async (_id: string, turnId: string) => {
      state.diffCalls++
      return { turn_id: turnId, scope: 'turn', root: '', from: '', to: '', files: state.diffs[turnId] ?? [] }
    },
    streamChat: async (req: ChatRequest, onEvent: (e: ChatEvent) => void) => {
      state.sent.push(req)
      const events = state.turns.shift() ?? [{ type: 'done' }]
      for (const ev of events) onEvent(ev)
    },
    attachChatStream: async (_id: string, onEvent: (e: ChatEvent) => void) => {
      const feed = state.feed
      state.feed = null
      if (!feed) return false
      for (const ev of feed) onEvent(ev)
      if (state.holdAttach) await state.holdAttach
      return true
    },
    gate: async (...args: unknown[]) => {
      state.gateCalls.push(args)
      return state.gateResult!
    },
    card: async (...args: unknown[]) => {
      state.cardCalls.push(args)
      return state.cardResult ?? { pipeline: state.pipeline, next_prompt: '' }
    },
    advance: async () => ({ pipeline: state.pipeline, next_prompt: '' }),
    stop: async () => ({ pipeline: state.pipeline, next_prompt: '' }),
  }
  return { state, api }
}

async function settle(store: InstanceType<typeof FocusStore>) {
  for (let i = 0; i < 20; i++) {
    await new Promise((r) => setTimeout(r, 0))
    if (!store.running && !store.loading) return
  }
}

function newStore(fake: Fake, storage = memoryStorage()) {
  return new FocusStore(fake.api as never, storage)
}

test('load reads the pipeline, the session and its transcript', async () => {
  const fake = fakeApi(pipeline('2026-10-01T00:00:01Z', [gateCard()], { open_gate: 'plan' }))
  const store = newStore(fake)
  await store.load('s1')
  await settle(store)
  assert.equal(store.pipeline?.session_id, 's1')
  assert.equal(store.session?.title, 'Task')
  assert.equal(store.error, '')
  assert.deepEqual(store.deck.map((c) => c.id), ['c1'])
  assert.equal(store.running, false)
})

test('a 404 pipeline reads as not found', async () => {
  const fake = fakeApi(pipeline('t'))
  fake.api.getPipeline = async () => { throw Object.assign(new Error('pipeline not found'), { status: 404 }) }
  const store = newStore(fake)
  await store.load('s1')
  assert.equal(store.error, 'notFound')
  assert.equal(store.pipeline, null)
})

test('a pipeline event with next_prompt sends it once, after the turn ends', async () => {
  const fake = fakeApi(pipeline('2026-10-01T00:00:00Z'))
  const p1 = pipeline('2026-10-01T00:00:05Z', [gateCard()], { open_gate: 'plan' })
  const event: ChatEvent = { type: 'pipeline', session_id: 's1', pipeline: p1, next_prompt: 'Reply again.' }
  // The same event twice (a replay, say) must not send twice.
  fake.state.history = [{ id: 'u0', role: 'user', content: 'earlier', timestamp: '' }]
  fake.state.turns.push([{ type: 'turn_started' }, event, event, { type: 'done' }])
  const store = newStore(fake)
  await store.load('s1')
  await store.send('ship it')
  await settle(store)
  assert.deepEqual(fake.state.sent.map((r) => r.message), ['ship it', 'Reply again.'])
  assert.equal(fake.state.sent[0].session_id, 's1')
  assert.deepEqual(store.pipeline?.cards.map((c) => c.id), ['c1'])
})

test('an older pipeline never replaces a newer one', async () => {
  const fake = fakeApi(pipeline('2026-10-01T00:00:09Z', [gateCard()], { open_gate: 'plan' }))
  const store = newStore(fake)
  await store.load('s1')
  store.applyEvent({ type: 'pipeline', session_id: 's1', pipeline: pipeline('2026-10-01T00:00:01Z'), next_prompt: '' })
  assert.equal(store.pipeline?.updated_at, '2026-10-01T00:00:09Z')
  // Events for another session are ignored.
  store.applyEvent({ type: 'pipeline', session_id: 'other', pipeline: pipeline('2026-10-01T00:00:20Z'), next_prompt: '' })
  assert.equal(store.pipeline?.updated_at, '2026-10-01T00:00:09Z')
})

test('reload mid-turn rebuilds from GET and the stream replay without duplicate cards', async () => {
  const before = pipeline('2026-10-01T00:00:01Z')
  const fake = fakeApi(before)
  const after = pipeline('2026-10-01T00:00:05Z', [gateCard()], { open_gate: 'plan' })
  let release: () => void = () => {}
  fake.state.holdAttach = new Promise((r) => { release = r })
  fake.state.feed = [
    { type: 'turn_started', session_id: 's1' },
    { type: 'delta', session_id: 's1', text: 'Planning' },
    { type: 'pipeline', session_id: 's1', pipeline: after, next_prompt: '' },
  ]
  const store = newStore(fake)
  await store.load('s1')
  await new Promise((r) => setTimeout(r, 0))
  // The replayed turn is still running: the progress line follows it.
  assert.equal(store.running, true)
  assert.match(store.progress('plan'), /^Planning · writing$/)
  assert.deepEqual(store.deck.map((c) => c.id), ['c1'])
  // The GET after the turn returns the same pipeline: still one card.
  fake.state.pipeline = after
  release()
  await settle(store)
  assert.equal(store.running, false)
  assert.deepEqual(store.pipeline?.cards.map((c) => c.id), ['c1'])
  // A second load of the same session (switching back) does not resend.
  fake.state.feed = [{ type: 'pipeline', session_id: 's1', pipeline: after, next_prompt: '' }]
  await store.load('s1')
  await settle(store)
  assert.deepEqual(fake.state.sent, [])
})

test('the same next_prompt replayed after a page reload is not sent again', async () => {
  const storage = memoryStorage()
  const p1 = pipeline('2026-10-01T00:00:05Z', [gateCard()], { open_gate: 'plan' })
  const event: ChatEvent = { type: 'pipeline', session_id: 's1', pipeline: p1, next_prompt: 'Reply again.' }
  const fake = fakeApi(pipeline('2026-10-01T00:00:00Z'))
  fake.state.history = [{ id: 'u0', role: 'user', content: 'earlier', timestamp: '' }]
  fake.state.turns.push([event, { type: 'done' }])
  const first = newStore(fake, storage)
  await first.load('s1')
  await first.send('go')
  await settle(first)
  assert.equal(fake.state.sent.length, 2)
  // A new store (page reload) sees the same event in a replay.
  fake.state.feed = [event]
  const second = newStore(fake, storage)
  await second.load('s1')
  await settle(second)
  assert.equal(fake.state.sent.length, 2)
})

test('gate approve sends next_prompt as the next chat turn', async () => {
  const fake = fakeApi(pipeline('2026-10-01T00:00:01Z', [gateCard()], { open_gate: 'plan' }))
  const approved = pipeline('2026-10-01T00:00:02Z', [{ ...gateCard(), state: 'decided', decision: 'approve' }], { current: 'build' })
  fake.state.gateResult = { pipeline: approved, next_prompt: 'Plan approved. Start the build stage with task 1.' }
  const store = newStore(fake)
  await store.load('s1')
  const edits = { goal: 'g', tasks: [{ title: 't', done: 'd' }], stages: ['plan', 'build'] as never, verify: ['make test'] }
  await store.gate('plan', 'approve', undefined, edits)
  await settle(store)
  assert.deepEqual(fake.state.gateCalls[0], ['s1', 'plan', 'approve', { note: undefined, edits }])
  assert.equal(store.pipeline?.current, 'build')
  assert.deepEqual(fake.state.sent.map((r) => r.message), ['Plan approved. Start the build stage with task 1.'])
})

test('a 409 on a gate replaces the state with the server pipeline and sends nothing', async () => {
  const fake = fakeApi(pipeline('2026-10-01T00:00:01Z', [gateCard()], { open_gate: 'plan' }))
  const current = pipeline('2026-10-01T00:00:03Z', [{ ...gateCard(), state: 'decided', decision: 'approve' }], { current: 'build' })
  fake.state.gateResult = { pipeline: current, next_prompt: '', conflict: true }
  const store = newStore(fake)
  await store.load('s1')
  await store.gate('plan', 'approve')
  assert.equal(store.pipeline?.current, 'build')
  assert.equal(store.notice, 'stale')
  assert.deepEqual(fake.state.sent, [])
})

test('deciding a decision card sends the answer turn', async () => {
  const decision: FocusCard = { id: 'c3', kind: 'decision', stage: 'build', turn: 2, title: 'Which flag?', state: 'unseen', created_at: '2026-10-01T00:00:03Z', payload: { id: 'd1', question: 'Which flag?', options: ['--a', '--b'] } }
  const fake = fakeApi(pipeline('2026-10-01T00:00:03Z', [decision], { current: 'build' }))
  fake.state.cardResult = { pipeline: pipeline('2026-10-01T00:00:04Z', [{ ...decision, state: 'decided', decision: '--b' }], { current: 'build' }), next_prompt: 'Which flag? → --b' }
  const store = newStore(fake)
  await store.load('s1')
  await store.markCard('c3', 'decided', '--b')
  await settle(store)
  assert.deepEqual(fake.state.cardCalls[0], ['s1', 'c3', 'decided', '--b'])
  assert.deepEqual(fake.state.sent.map((r) => r.message), ['Which flag? → --b'])
})

test('change cards come from checkpoint diffs, one per file, acknowledged locally', async () => {
  const storage = memoryStorage()
  const fake = fakeApi(pipeline('2026-10-01T00:00:03Z', [], { current: 'build' }))
  fake.state.history = [
    { id: 'u1', role: 'user', content: 'goal\n\n<focus-stage>\nFocus mode — current stage: plan.\n</focus-stage>', timestamp: '' },
    { id: 'a1', role: 'assistant', content: 'plan', timestamp: '' },
    { id: 'u2', role: 'user', content: 'go\n\n<focus-stage>\nFocus mode — current stage: build (iteration 1 of 3).\n</focus-stage>', timestamp: '' },
    { id: 'a2', role: 'assistant', content: 'built', timestamp: '' },
  ]
  fake.state.checkpoints = [
    { turn_id: 'u1', files: 0, additions: 0, deletions: 0, started_at: '' },
    { turn_id: 'u2', files: 2, additions: 3, deletions: 1, started_at: '', ended_at: '2026-10-01T00:00:04Z' },
  ]
  fake.state.diffs.u2 = [
    { path: 'a.go', status: 'modified', additions: 2, deletions: 1, patch: '@@ -1 +1 @@\n-a\n+b\n' },
    { path: 'b.go', status: 'added', additions: 1, deletions: 0, patch: '@@ -0,0 +1 @@\n+b\n' },
  ]
  const store = newStore(fake, storage)
  await store.load('s1')
  await settle(store)
  assert.deepEqual(store.deck.map((c) => [c.id, c.stage, c.turn]), [['change:u2:a.go', 'build', 2], ['change:u2:b.go', 'build', 2]])
  await store.markCard('change:u2:a.go', 'decided', 'acknowledged')
  assert.equal(store.cards.find((c) => c.id === 'change:u2:a.go')?.state, 'decided')
  assert.deepEqual(fake.state.cardCalls, [])
  // The acknowledgement survives a reload; diffs of a turn are fetched once.
  const again = newStore(fake, storage)
  await again.load('s1')
  await settle(again)
  assert.equal(again.cards.find((c) => c.id === 'change:u2:a.go')?.state, 'decided')
  await store.refreshChanges()
  assert.equal(fake.state.diffCalls, 2)
})

test('acknowledgeRest decides every remaining report, change and notice card', async () => {
  const report: FocusCard = { id: 'c2', kind: 'report', stage: 'build', turn: 2, title: 'Done', state: 'seen', created_at: '2026-10-01T00:00:02Z' }
  const notice: FocusCard = { id: 'c4', kind: 'notice', stage: 'build', turn: 2, title: 'report format missing', state: 'unseen', created_at: '2026-10-01T00:00:02Z' }
  const decision: FocusCard = { id: 'c3', kind: 'decision', stage: 'build', turn: 2, title: 'Q', state: 'unseen', created_at: '2026-10-01T00:00:02Z' }
  const fake = fakeApi(pipeline('2026-10-01T00:00:03Z', [report, decision, notice], { current: 'build' }))
  const store = newStore(fake)
  await store.load('s1')
  await store.acknowledgeRest(store.deck)
  assert.deepEqual(fake.state.cardCalls.map((c) => c[1]).sort(), ['c2', 'c4'])
  assert.ok(fake.state.cardCalls.every((c) => c[2] === 'decided'))
})

test('a new pipeline sends its goal as the first turn, once', async () => {
  const storage = memoryStorage()
  const fake = fakeApi({ ...pipeline('2026-10-01T00:00:00Z'), goal: 'Add a --dry-run flag' })
  const store = newStore(fake, storage)
  await store.load('s1')
  await settle(store)
  assert.deepEqual(fake.state.sent.map((r) => r.message), ['Add a --dry-run flag'])
  // Reloaded before the transcript shows the turn: not sent again.
  const again = newStore(fake, storage)
  await again.load('s1')
  await settle(again)
  assert.equal(fake.state.sent.length, 1)
})

test('an empty transcript the server answers as null still kicks off the goal', async () => {
  const fake = fakeApi({ ...pipeline('2026-10-01T00:00:00Z'), goal: 'Ship it' })
  fake.api.getHistory = async () => null as never
  const store = newStore(fake)
  await store.load('s1')
  await settle(store)
  assert.deepEqual(fake.state.sent.map((r) => r.message), ['Ship it'])
  assert.deepEqual(store.history.length >= 0, true)
})
