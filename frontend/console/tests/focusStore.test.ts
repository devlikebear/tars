import test from 'node:test'
import assert from 'node:assert/strict'

import { compileSvelteModule } from './helpers/compileSvelteModule.ts'
import { stashKickoffAttachments } from '../src/lib/focusKickoff.ts'
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
    // Sessions /v1/chat/activity reports as running.
    runningSessions: [] as string[],
    // The next streamChat call fails before the server accepts it.
    failNextStream: false,
    // The Q&A session: its transcript, its answer feed, the asks made.
    qaHistory: [] as SessionMessage[],
    qaFeed: null as ChatEvent[] | null,
    askCalls: [] as unknown[][],
    askResult: { qa_session_id: 'qa1', turn: 1 },
  }
  const api = {
    getPipeline: async () => state.pipeline,
    getSession: async (id: string) => ({ id, title: 'Task', current_dir: 'repo', created_at: '', updated_at: '' }),
    getHistory: async (id: string) => (id === 'qa1' ? state.qaHistory : state.history),
    listCheckpoints: async (id: string) => ({ session_id: id, turns: state.checkpoints }),
    getCheckpointDiff: async (_id: string, turnId: string) => {
      state.diffCalls++
      return { turn_id: turnId, scope: 'turn', root: '', from: '', to: '', files: state.diffs[turnId] ?? [] }
    },
    streamChat: async (req: ChatRequest, onEvent: (e: ChatEvent) => void) => {
      if (state.failNextStream) {
        state.failNextStream = false
        throw new Error('network down')
      }
      state.sent.push(req)
      const events = state.turns.shift() ?? [{ type: 'done' }]
      for (const ev of events) onEvent(ev)
    },
    attachChatStream: async (id: string, onEvent: (e: ChatEvent) => void) => {
      if (id === 'qa1') {
        const qa = state.qaFeed
        state.qaFeed = null
        if (!qa) return false
        for (const ev of qa) onEvent(ev)
        return true
      }
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
    activity: async () => ({ running: state.runningSessions.map((session_id) => ({ session_id })), pending_approvals: [] }),
    ask: async (...args: unknown[]) => {
      state.askCalls.push(args)
      return state.askResult
    },
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
  const store = new FocusStore(fake.api as never, storage)
  store.timing = { chaseTries: 3, chaseDelayMs: 1, qaAttachTries: 5, qaAttachDelayMs: 1, qaMaxWaitMs: 200 }
  return store
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

test('a pipeline event next_prompt is shown, never sent: the server sends it', async () => {
  const fake = fakeApi(pipeline('2026-10-01T00:00:00Z'))
  const p1 = pipeline('2026-10-01T00:00:05Z', [gateCard()], { open_gate: 'plan' })
  const event: ChatEvent = { type: 'pipeline', session_id: 's1', pipeline: p1, next_prompt: 'Reply again.' }
  fake.state.history = [{ id: 'u0', role: 'user', content: 'earlier', timestamp: '' }]
  fake.state.turns.push([{ type: 'turn_started' }, event, event, { type: 'done' }])
  const store = newStore(fake)
  await store.load('s1')
  await store.send('ship it')
  await settle(store)
  assert.deepEqual(fake.state.sent.map((r) => r.message), ['ship it'])
  assert.equal(store.nextPrompt, 'Reply again.')
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

test('a next_prompt replayed after a page reload sends nothing', async () => {
  const p1 = pipeline('2026-10-01T00:00:05Z', [gateCard()], { open_gate: 'plan' })
  const fake = fakeApi(pipeline('2026-10-01T00:00:00Z'))
  fake.state.history = [{ id: 'u0', role: 'user', content: 'earlier', timestamp: '' }]
  fake.state.feed = [{ type: 'pipeline', session_id: 's1', pipeline: p1, next_prompt: 'Reply again.' }]
  const store = newStore(fake)
  await store.load('s1')
  await settle(store)
  assert.deepEqual(fake.state.sent, [])
  assert.equal(store.nextPrompt, 'Reply again.')
})

test('gate approve: the server sends the next turn and the store follows it', async () => {
  const fake = fakeApi(pipeline('2026-10-01T00:00:01Z', [gateCard()], { open_gate: 'plan' }))
  fake.state.history = [{ id: 'u0', role: 'user', content: 'earlier', timestamp: '' }]
  const approved = pipeline('2026-10-01T00:00:02Z', [{ ...gateCard(), state: 'decided', decision: 'approve' }], { current: 'build' })
  fake.state.gateResult = { pipeline: approved, next_prompt: 'Plan approved. Start the build stage with task 1.' }
  const store = newStore(fake)
  await store.load('s1')
  // The server's turn shows up as running on the session.
  let release: () => void = () => {}
  fake.state.holdAttach = new Promise((r) => { release = r })
  fake.state.feed = [{ type: 'turn_started', session_id: 's1' }, { type: 'delta', session_id: 's1', text: 'Implementing' }]
  fake.state.runningSessions = ['s1']
  const edits = { goal: 'g', tasks: [{ title: 't', done: 'd' }], stages: ['plan', 'build'] as never, verify: ['make test'] }
  await store.gate('plan', 'approve', undefined, edits)
  for (let i = 0; i < 20 && !store.running; i++) await new Promise((r) => setTimeout(r, 1))
  assert.deepEqual(fake.state.gateCalls[0], ['s1', 'plan', 'approve', { note: undefined, edits }])
  assert.equal(store.pipeline?.current, 'build')
  assert.equal(store.running, true)
  assert.equal(store.nextPrompt, 'Plan approved. Start the build stage with task 1.')
  fake.state.runningSessions = []
  release()
  await settle(store)
  assert.deepEqual(fake.state.sent, [])
})

test('the PR gate posts its edited title and body with approve', async () => {
  const g3: FocusCard = { id: 'c4', kind: 'gate', stage: 'pr', turn: 3, title: 'Open pull request', state: 'unseen', created_at: '2026-10-01T00:00:04Z', payload: { title: 'draft', body: 'b' } }
  const fake = fakeApi(pipeline('2026-10-01T00:00:04Z', [g3], { current: 'pr', open_gate: 'pr' }))
  fake.state.gateResult = { pipeline: pipeline('2026-10-01T00:00:05Z', [{ ...g3, state: 'decided', decision: 'approve' }], { current: 'pr', pr_wait: 'open' }), next_prompt: 'PR gate approved.' }
  const store = newStore(fake)
  await store.load('s1')
  const draft = { title: 'feat: edited', body: 'new body' }
  assert.equal(await store.gate('pr', 'approve', undefined, undefined, draft), true)
  assert.deepEqual(fake.state.gateCalls[0], ['s1', 'pr', 'approve', { note: undefined, edits: undefined, pr: draft }])
  assert.equal(store.pipeline?.pr_wait, 'open')
  await settle(store)
})

// Review round 2 (#1087): approving G4 names the card on screen, so a G4
// the server reopened on a new head meanwhile refuses the approval.
test('the merge gate posts the card it shows', async () => {
  const old: FocusCard = { id: 'c5', kind: 'gate', stage: 'merge', turn: 0, title: 'Merge pull request', state: 'decided', decision: 'superseded', created_at: '2026-10-01T00:00:05Z', payload: {} }
  const g4: FocusCard = { id: 'c7', kind: 'gate', stage: 'merge', turn: 0, title: 'Merge pull request', state: 'unseen', created_at: '2026-10-01T00:00:07Z', payload: {} }
  const fake = fakeApi(pipeline('2026-10-01T00:00:07Z', [old, g4], { current: 'merge', open_gate: 'merge' }))
  fake.state.gateResult = { pipeline: pipeline('2026-10-01T00:00:08Z', [old, { ...g4, state: 'decided', decision: 'approve' }], { current: 'merge', pr_wait: 'merge' }), next_prompt: 'Merge gate approved.' }
  const store = newStore(fake)
  await store.load('s1')
  assert.equal(await store.gate('merge', 'approve'), true)
  assert.deepEqual(fake.state.gateCalls[0], ['s1', 'merge', 'approve', { note: undefined, edits: undefined, card_id: 'c7' }])
  await settle(store)
})

test('the blocked gate takes retry and instruct with its note', async () => {
  const blocked: FocusCard = { id: 'c5', kind: 'gate', stage: 'build', turn: 4, title: 'Build blocked', state: 'unseen', created_at: '2026-10-01T00:00:05Z', payload: { reason: 'repeated', iteration: 2, limit: 3 } }
  const fake = fakeApi(pipeline('2026-10-01T00:00:05Z', [blocked], { current: 'build', open_gate: 'blocked' }))
  fake.state.gateResult = { pipeline: pipeline('2026-10-01T00:00:06Z', [{ ...blocked, state: 'decided', decision: 'instruct' }], { current: 'build' }), next_prompt: 'skip the flaky test' }
  const store = newStore(fake)
  await store.load('s1')
  assert.equal(await store.gate('blocked', 'instruct', 'skip the flaky test'), true)
  await store.gate('blocked', 'retry')
  assert.deepEqual(fake.state.gateCalls.map((c) => c.slice(1, 3).concat([(c[3] as { note?: string }).note])), [['blocked', 'instruct', 'skip the flaky test'], ['blocked', 'retry', undefined]])
  await settle(store)
  assert.deepEqual(fake.state.sent, [])
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

test('deciding a decision card sends nothing itself: the server runs the answer turn', async () => {
  const decision: FocusCard = { id: 'c3', kind: 'decision', stage: 'build', turn: 2, title: 'Which flag?', state: 'unseen', created_at: '2026-10-01T00:00:03Z', payload: { id: 'd1', question: 'Which flag?', options: ['--a', '--b'] } }
  const fake = fakeApi(pipeline('2026-10-01T00:00:03Z', [decision], { current: 'build' }))
  fake.state.cardResult = { pipeline: pipeline('2026-10-01T00:00:04Z', [{ ...decision, state: 'decided', decision: '--b' }], { current: 'build' }), next_prompt: 'Which flag? → --b' }
  const store = newStore(fake)
  await store.load('s1')
  await store.markCard('c3', 'decided', '--b')
  await settle(store)
  assert.deepEqual(fake.state.cardCalls[0], ['s1', 'c3', 'decided', '--b'])
  assert.deepEqual(fake.state.sent, [])
  assert.equal(store.nextPrompt, 'Which flag? → --b')
})

test('change cards come from checkpoint diffs, one per turn, acknowledged locally', async () => {
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
  // One card for the turn, with both files (U1).
  assert.deepEqual(store.deck.map((c) => [c.id, c.stage, c.turn]), [['change:u2', 'build', 2]])
  assert.deepEqual((store.deck[0].payload as { files: { path: string }[] }).files.map((f) => f.path), ['a.go', 'b.go'])
  await store.markCard('change:u2', 'decided', 'acknowledged')
  assert.equal(store.cards.find((c) => c.id === 'change:u2')?.state, 'decided')
  assert.deepEqual(fake.state.cardCalls, [])
  // The acknowledgement survives a reload; diffs of a turn are fetched once.
  const again = newStore(fake, storage)
  await again.load('s1')
  await settle(again)
  assert.equal(again.cards.find((c) => c.id === 'change:u2')?.state, 'decided')
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

test('a typed instruction held by a running turn survives a reload, and goes out once', async () => {
  const storage = memoryStorage()
  const fake = fakeApi(pipeline('2026-10-01T00:00:02Z', [], { current: 'build' }))
  fake.state.history = [{ id: 'u0', role: 'user', content: 'earlier', timestamp: '' }]
  // The server's turn runs, so the instruction waits; then the page goes away.
  fake.state.runningSessions = ['s1']
  const first = newStore(fake, storage)
  await first.load('s1')
  await first.send('also rename it')
  first.dispose()
  assert.deepEqual(fake.state.sent, [])
  fake.state.runningSessions = []
  const second = newStore(fake, storage)
  await second.load('s1')
  await settle(second)
  assert.deepEqual(fake.state.sent.map((r) => r.message), ['also rename it'])
  const third = newStore(fake, storage)
  await third.load('s1')
  await settle(third)
  assert.equal(fake.state.sent.length, 1)
})

test('a prompt whose chat request failed stays queued and is retried', async () => {
  const storage = memoryStorage()
  const fake = fakeApi({ ...pipeline('2026-10-01T00:00:00Z'), goal: 'Ship it' })
  fake.state.failNextStream = true
  const first = newStore(fake, storage)
  await first.load('s1')
  await settle(first)
  assert.deepEqual(fake.state.sent, [])
  assert.match(first.actionError, /network down/)
  const second = newStore(fake, storage)
  await second.load('s1')
  await settle(second)
  assert.deepEqual(fake.state.sent.map((r) => r.message), ['Ship it'])
})

test('a failed transcript read never kicks off the goal', async () => {
  const fake = fakeApi({ ...pipeline('2026-10-01T00:00:00Z'), goal: 'Ship it' })
  fake.api.getHistory = async () => { throw new Error('boom') }
  const store = newStore(fake)
  await store.load('s1')
  await settle(store)
  assert.deepEqual(fake.state.sent, [])
})

test('a disposed store sends nothing more after its turn ends', async () => {
  const fake = fakeApi(pipeline('2026-10-01T00:00:00Z'))
  fake.state.history = [{ id: 'u0', role: 'user', content: 'earlier', timestamp: '' }]
  const p1 = pipeline('2026-10-01T00:00:05Z', [], { current: 'plan' })
  let release: () => void = () => {}
  const hold = new Promise<void>((r) => { release = r })
  const store = newStore(fake)
  await store.load('s1')
  fake.api.streamChat = async (req, onEvent) => {
    fake.state.sent.push(req)
    onEvent({ type: 'turn_started' })
    onEvent({ type: 'pipeline', session_id: 's1', pipeline: p1, next_prompt: 'Reply again.' })
    await hold
  }
  const sending = store.send('go')
  await new Promise((r) => setTimeout(r, 0))
  store.dispose()
  release()
  await sending
  await settle(store)
  assert.deepEqual(fake.state.sent.map((r) => r.message), ['go'])
})

test('a turn started elsewhere is followed, and an instruction waits for it', async () => {
  const fake = fakeApi(pipeline('2026-10-01T00:00:00Z'))
  fake.state.history = [{ id: 'u0', role: 'user', content: 'earlier', timestamp: '' }]
  const store = newStore(fake)
  await store.load('s1')
  await settle(store)
  assert.equal(store.running, false)
  // A turn starts in Advanced: the next poll attaches to it.
  let release: () => void = () => {}
  fake.state.holdAttach = new Promise((r) => { release = r })
  fake.state.feed = [{ type: 'turn_started', session_id: 's1' }]
  fake.state.runningSessions = ['s1']
  await store.poll()
  await new Promise((r) => setTimeout(r, 0))
  assert.equal(store.running, true)
  // An instruction now does not start a parallel turn.
  const sending = store.send('also do x')
  await new Promise((r) => setTimeout(r, 0))
  assert.deepEqual(fake.state.sent, [])
  fake.state.runningSessions = []
  release()
  await sending
  await settle(store)
  assert.deepEqual(fake.state.sent.map((r) => r.message), ['also do x'])
})

test('an instruction held back by a running turn goes out on the next poll once it ends', async () => {
  const fake = fakeApi(pipeline('2026-10-01T00:00:02Z', [], { current: 'build' }))
  fake.state.history = [{ id: 'u0', role: 'user', content: 'earlier', timestamp: '' }]
  fake.state.runningSessions = ['s1']
  const store = newStore(fake)
  await store.load('s1')
  await store.send('Start the build.')
  assert.deepEqual(fake.state.sent, [])
  fake.state.runningSessions = []
  await store.poll()
  await settle(store)
  assert.deepEqual(fake.state.sent.map((r) => r.message), ['Start the build.'])
})

test('while the current stage has no cards yet, the deck keeps the last stage with cards', async () => {
  const decided = { ...gateCard(), state: 'decided' as const, decision: 'approve' }
  const fake = fakeApi(pipeline('2026-10-01T00:00:02Z', [decided], { current: 'build' }))
  fake.state.history = [{ id: 'u0', role: 'user', content: 'earlier', timestamp: '' }]
  const store = newStore(fake)
  await store.load('s1')
  await settle(store)
  assert.equal(store.stage, 'plan')
  assert.deepEqual(store.deck.map((c) => c.id), ['c1'])
  // Cards of the current stage take over as soon as there are any.
  store.applyEvent({ type: 'pipeline', session_id: 's1', next_prompt: '', pipeline: pipeline('2026-10-01T00:00:03Z', [decided, { id: 'c2', kind: 'report', stage: 'build', turn: 2, title: 'r', state: 'unseen', created_at: '2026-10-01T00:00:03Z' }], { current: 'build' }) })
  assert.equal(store.stage, 'build')
  assert.deepEqual(store.deck.map((c) => c.id), ['c2'])
})

test('a pipeline with a kickoff sends the kickoff, not the goal, as its first turn', async () => {
  const fake = fakeApi({ ...pipeline('2026-10-01T00:00:00Z'), goal: 'Release: ship 2 changes', kickoff: 'Release: ship 2 changes\n- a\n- b' })
  const store = newStore(fake)
  await store.load('s1')
  await settle(store)
  assert.deepEqual(fake.state.sent.map((r) => r.message), ['Release: ship 2 changes\n- a\n- b'])
})

test('an image pasted into the goal field (FocusNewTask) rides the first turn as an attachment', async () => {
  stashKickoffAttachments('s1', [{ name: 'clipboard-1.png', mime_type: 'image/png', data: 'AA==' }])
  const fake = fakeApi(pipeline('2026-10-01T00:00:00Z'))
  const store = newStore(fake)
  await store.load('s1')
  await settle(store)
  assert.deepEqual(fake.state.sent[0]?.attachments, [{ name: 'clipboard-1.png', mime_type: 'image/png', data: 'AA==' }])

  // Taken once: a later instruction (and a reload, which would otherwise
  // find nothing new to kick off) never resends it.
  fake.state.pipeline = pipeline('2026-10-01T00:00:02Z', [], { current: 'build' })
  fake.state.history = [{ id: 'u0', role: 'user', content: 'g', timestamp: '' }]
  await store.send('also do this')
  await settle(store)
  assert.equal(fake.state.sent.length, 2)
  assert.equal(fake.state.sent[1]?.attachments, undefined)
})

test('a pipeline with no stashed images sends the first turn without attachments, as before', async () => {
  const fake = fakeApi(pipeline('2026-10-01T00:00:00Z'))
  const store = newStore(fake)
  await store.load('s1')
  await settle(store)
  assert.equal(fake.state.sent[0]?.attachments, undefined)
})

test('the driver\'s verification step shows on the progress line', async () => {
  const fake = fakeApi(pipeline('2026-10-01T00:00:02Z', [], { current: 'build' }))
  fake.state.history = [{ id: 'u0', role: 'user', content: 'earlier', timestamp: '' }]
  let release: () => void = () => {}
  fake.state.holdAttach = new Promise((r) => { release = r })
  fake.state.feed = [
    { type: 'status', session_id: 's1', phase: 'stream_open' },
    { type: 'focus_progress', session_id: 's1', phase: 'verifying', command: 'make test', index: 1, total: 1 },
  ]
  const store = newStore(fake)
  await store.load('s1')
  await new Promise((r) => setTimeout(r, 0))
  assert.equal(store.progress('build'), 'Implementing · verifying make test (1/1)')
  release()
  await settle(store)
})

test('questions thread by card; the answer streams in, then comes from the Q&A history', async () => {
  const report: FocusCard = { id: 'c2', kind: 'report', stage: 'build', turn: 2, title: 'Done', state: 'seen', created_at: '2026-10-01T00:00:02Z' }
  const fake = fakeApi(pipeline('2026-10-01T00:00:03Z', [gateCard(), report], { current: 'build', qa_session_id: 'qa1', qa_turns: { c1: [1] } }))
  fake.state.history = [{ id: 'u0', role: 'user', content: 'earlier', timestamp: '' }]
  fake.state.qaHistory = [
    { id: 'q1', role: 'user', content: 'why?\n\n<console-context>\nCard c1\n</console-context>', timestamp: '' },
    { id: 'q1a', role: 'assistant', content: 'Because.', timestamp: '' },
  ]
  const store = newStore(fake)
  await store.load('s1')
  await settle(store)
  for (let i = 0; i < 10 && store.qaHistory.length === 0; i++) await new Promise((r) => setTimeout(r, 1))
  assert.deepEqual(store.qaThread('c1'), [{ turn: 1, question: 'why?', answer: 'Because.' }])
  assert.deepEqual(store.qaThread('c2'), [])

  // A question about c2: the answer streams, then the history holds it.
  fake.state.askResult = { qa_session_id: 'qa1', turn: 2 }
  // The server records the turn on the pipeline as it accepts the question.
  fake.state.pipeline = { ...fake.state.pipeline, qa_turns: { c1: [1], c2: [2] } }
  fake.state.qaFeed = [{ type: 'turn_started' }, { type: 'delta', text: 'It is ' }, { type: 'delta', text: 'a report.' }]
  // The server saves the turn before its feed ends.
  fake.state.qaHistory = [
    ...fake.state.qaHistory,
    { id: 'q2', role: 'user', content: 'what is this?\n\n<console-context>\nCard c2\n</console-context>', timestamp: '' },
    { id: 'q2a', role: 'assistant', content: 'It is a report.', timestamp: '' },
  ]
  const asked = store.ask('c2', '  what is this?  ')
  assert.equal(await asked, true)
  assert.deepEqual(fake.state.askCalls[0], ['s1', 'c2', 'what is this?'])
  assert.deepEqual(store.pipeline?.qa_turns, { c1: [1], c2: [2] })
  for (let i = 0; i < 20 && store.qaPending; i++) await new Promise((r) => setTimeout(r, 1))
  assert.equal(store.qaPending, null)
  assert.deepEqual(store.qaThread('c2'), [{ turn: 2, question: 'what is this?', answer: 'It is a report.' }])
  // Q&A sent nothing to the pipeline's own session and changed no card.
  assert.deepEqual(fake.state.sent, [])
  assert.deepEqual(store.pipeline?.cards.map((c) => c.id), ['c1', 'c2'])
})

test('a question while one is being answered is refused', async () => {
  const fake = fakeApi(pipeline('2026-10-01T00:00:03Z', [gateCard()], { current: 'build' }))
  fake.state.history = [{ id: 'u0', role: 'user', content: 'earlier', timestamp: '' }]
  const store = newStore(fake)
  store.timing = { chaseTries: 1, chaseDelayMs: 1, qaAttachTries: 50, qaAttachDelayMs: 5, qaMaxWaitMs: 5000 }
  await store.load('s1')
  assert.equal(await store.ask('c1', 'first?'), true)
  assert.equal(await store.ask('c1', 'second?'), false)
  assert.equal(store.qaError, 'busy')
  assert.equal(fake.state.askCalls.length, 1)
  assert.deepEqual(store.qaThread('c1'), [{ turn: 1, question: 'first?', answer: '' }])
  store.dispose()
})

test('an instruction the developer sends clears the pipeline prompt shown (f7)', async () => {
  const fake = fakeApi(pipeline('2026-10-01T00:00:02Z', [], { current: 'build' }))
  fake.state.history = [{ id: 'u0', role: 'user', content: 'earlier', timestamp: '' }]
  const store = newStore(fake)
  await store.load('s1')
  store.applyEvent({ type: 'pipeline', session_id: 's1', pipeline: pipeline('2026-10-01T00:00:03Z', [], { current: 'build' }), next_prompt: 'Verification failed … fix it' })
  assert.equal(store.nextPrompt, 'Verification failed … fix it')
  let seen = ''
  fake.api.streamChat = async (req, onEvent) => {
    fake.state.sent.push(req)
    seen = store.nextPrompt
    onEvent({ type: 'done' })
  }
  await store.send('rename greet to hello')
  await settle(store)
  assert.equal(seen, '', 'the running turn is the developer\'s own')
})

test('a transient error clears once a later request succeeds (U3)', async () => {
  const fake = fakeApi(pipeline('2026-10-01T00:00:02Z', [], { current: 'build' }))
  fake.state.history = [{ id: 'u0', role: 'user', content: 'earlier', timestamp: '' }]
  const store = newStore(fake)
  await store.load('s1')
  fake.state.failNextStream = true
  await store.send('go')
  await settle(store)
  assert.match(store.actionError, /network down/)
  // The stream comes back: the server's turn is followed, and the error goes.
  fake.state.feed = [{ type: 'turn_started', session_id: 's1' }, { type: 'done', session_id: 's1' }]
  fake.state.runningSessions = ['s1']
  await store.poll()
  await settle(store)
  assert.equal(store.actionError, '')
})

test('a Q&A answer whose turn starts late is still followed to the end (R5)', async () => {
  const fake = fakeApi(pipeline('2026-10-01T00:00:03Z', [gateCard()], { current: 'build' }))
  fake.state.history = [{ id: 'u0', role: 'user', content: 'earlier', timestamp: '' }]
  const store = newStore(fake)
  store.timing = { chaseTries: 1, chaseDelayMs: 1, qaAttachTries: 3, qaAttachDelayMs: 2, qaMaxWaitMs: 5000 }
  await store.load('s1')
  // The answer turn waits behind something for longer than the old retry
  // budget: no feed yet, and the activity lists it as not running.
  assert.equal(await store.ask('c1', 'why?'), true)
  await new Promise((r) => setTimeout(r, 40))
  assert.ok(store.qaPending, 'still waiting for the answer')
  fake.state.qaHistory = [
    { id: 'q1', role: 'user', content: 'why?\n\n<console-context>\nfocus-card: c1\nCard c1\n</console-context>', timestamp: '' },
    { id: 'q1a', role: 'assistant', content: 'Because.', timestamp: '' },
  ]
  fake.state.qaFeed = [{ type: 'turn_started' }, { type: 'delta', text: 'Because.' }, { type: 'done' }]
  for (let i = 0; i < 100 && store.qaPending; i++) await new Promise((r) => setTimeout(r, 5))
  assert.equal(store.qaPending, null)
  assert.deepEqual(store.qaThread('c1'), [{ turn: 1, question: 'why?', answer: 'Because.' }])
  store.dispose()
})

test('an instruction refused because a turn just started (409) stays queued, without an error', async () => {
  const fake = fakeApi(pipeline('2026-10-01T00:00:02Z', [], { current: 'build' }))
  fake.state.history = [{ id: 'u0', role: 'user', content: 'earlier', timestamp: '' }]
  const store = newStore(fake)
  await store.load('s1')
  let refused = false
  fake.api.streamChat = async (req) => {
    if (!refused) {
      refused = true
      throw Object.assign(new Error('a turn is already running on this session'), { status: 409 })
    }
    fake.state.sent.push(req)
  }
  await store.send('also rename it')
  await settle(store)
  assert.equal(store.actionError, '')
  await store.poll()
  await settle(store)
  assert.deepEqual(fake.state.sent.map((r) => r.message), ['also rename it'])
})

test('the kickoff is the one first message the console sends; every later prompt is the server\'s', async () => {
  const storage = memoryStorage()
  const fake = fakeApi({ ...pipeline('2026-10-01T00:00:00Z'), goal: 'Release: ship 2 changes', kickoff: 'Release: ship 2 changes\n- a\n- b' })
  const store = newStore(fake, storage)
  await store.load('s1')
  await settle(store)
  assert.deepEqual(fake.state.sent.map((r) => r.message), ['Release: ship 2 changes\n- a\n- b'])
  // The plan comes back; approving it starts the build turn on the server.
  fake.state.history = [{ id: 'u1', role: 'user', content: 'Release: ship 2 changes', timestamp: '' }]
  fake.state.pipeline = pipeline('2026-10-01T00:00:01Z', [gateCard()], { open_gate: 'plan' })
  fake.state.gateResult = { pipeline: pipeline('2026-10-01T00:00:02Z', [{ ...gateCard(), state: 'decided', decision: 'approve' }], { current: 'build' }), next_prompt: 'Plan approved. Start the build stage with task 1.' }
  await store.load('s1')
  await store.gate('plan', 'approve')
  await settle(store)
  // A reload does not send the kickoff again either.
  const again = newStore(fake, storage)
  await again.load('s1')
  await settle(again)
  assert.deepEqual(fake.state.sent.map((r) => r.message), ['Release: ship 2 changes\n- a\n- b'])
})

test('poll re-reads the pipeline when no turn runs: server-side facts (the gh probe) reach the screen', async () => {
  // CI #1077: the open-PR turn ran between two activity checks, so no turn
  // was ever followed; the probe's "gh unavailable" only exists server-side.
  const fake = fakeApi(pipeline('2026-10-01T00:00:01Z', [], { current: 'pr', pr_wait: 'open' }))
  const store = newStore(fake)
  await store.load('s1')
  fake.state.pipeline = pipeline('2026-10-01T00:00:09Z', [], { current: 'pr', pr_wait: 'open', pr_unavailable: 'e2e gh stub: not logged in' })
  // A turn that ended unseen wrote the transcript too: a newer pipeline
  // re-reads it with the pipeline, so its cards' raw slices exist.
  fake.state.history = [{ id: 'a9', role: 'assistant', content: 'Opened the PR.', timestamp: '' }]
  await store.poll()
  assert.equal(store.pipeline?.pr_unavailable, 'e2e gh stub: not logged in')
  assert.deepEqual(store.history.map((m) => m.content), ['Opened the PR.'])
  // An older read never replaces a newer one.
  fake.state.pipeline = pipeline('2026-10-01T00:00:05Z', [], { current: 'pr' })
  await store.poll()
  assert.equal(store.pipeline?.pr_unavailable, 'e2e gh stub: not logged in')
})
