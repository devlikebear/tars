import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { compileSvelteModule } from './helpers/compileSvelteModule.ts'
import type * as CaseBridgeStoreModule from '../src/lib/stores/caseBridgeStore.svelte.ts'
import type { CaseBridgeAnswer, CaseBridgeCard, CaseBridgeScope, CaseBridgeView } from '../src/lib/api/caseBridge.ts'
import type { ChatEvent } from '../src/lib/types.ts'
import { canScopeToScreen, caseBridgeFollowUps, caseBridgeView, caseBridgeVisibleText } from '../src/lib/caseBridge.ts'
import { matchShortcut, shortcutDocs } from '../src/lib/shortcuts.ts'

const { CaseBridgeStore } = await compileSvelteModule<typeof CaseBridgeStoreModule>('src/lib/stores/caseBridgeStore.svelte.ts')

const panelSource = readFileSync(new URL('../src/components/CaseBridge.svelte', import.meta.url), 'utf8')
const appSource = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8')

test('caseBridgeView names the route and, on a chat or focus screen, its session', () => {
  assert.deepEqual(caseBridgeView({ view: 'chat', sessionId: 's1' }, ' Fix login '), { route: 'chat', session_id: 's1', title: 'Fix login' })
  assert.deepEqual(caseBridgeView({ view: 'focus', sessionId: 'p1' }), { route: 'focus', session_id: 'p1' })
  assert.deepEqual(caseBridgeView({ view: 'ops', sessionId: 'x' }), { route: 'ops' })
  assert.deepEqual(caseBridgeView({ view: '' }), { route: 'board' })
  assert.equal(canScopeToScreen({ route: 'chat', session_id: 's1' }), true)
  assert.equal(canScopeToScreen({ route: 'board' }), false)
})

test('caseBridgeVisibleText never shows the card block, even half-streamed', () => {
  assert.equal(caseBridgeVisibleText('All clear.'), 'All clear.')
  assert.equal(caseBridgeVisibleText('One waits.\n<case-cards>[{"kind":'), 'One waits.')
  assert.equal(caseBridgeVisibleText('One waits.\n<case-c'), 'One waits.')
  assert.equal(caseBridgeVisibleText('One waits. <'), 'One waits.')
  // A "<" that cannot start the block stays.
  assert.equal(caseBridgeVisibleText('a < b'), 'a < b')
})

test('caseBridgeFollowUps follows the cards, then the openers, three at most', () => {
  const card = (kind: CaseBridgeCard['kind']): CaseBridgeCard => ({ kind, title: kind, severity: 'info', href: '/' })
  assert.deepEqual(caseBridgeFollowUps([]), ['stuck', 'missed'])
  assert.deepEqual(caseBridgeFollowUps([card('focus'), card('focus'), card('session')]), ['focusWaiting', 'stuck', 'missed'])
  assert.deepEqual(caseBridgeFollowUps([card('cron'), card('approvals'), card('usage'), card('pulse')]), ['cronWhy', 'approvalsOldest', 'usageTop'])
})

test('Mod+/ opens the Bridge on every route, with or without Shift', () => {
  const ev = (over: Partial<KeyboardEvent>) => ({ key: '/', code: 'Slash', metaKey: true, ctrlKey: false, altKey: false, shiftKey: false, defaultPrevented: false, ...over })
  assert.deepEqual(matchShortcut(ev({})), { action: 'case-bridge' })
  assert.deepEqual(matchShortcut(ev({ metaKey: false, ctrlKey: true })), { action: 'case-bridge' })
  assert.deepEqual(matchShortcut(ev({ shiftKey: true })), { action: 'case-bridge' })
  assert.equal(matchShortcut(ev({ metaKey: false })), null)
  assert.ok(shortcutDocs.some((d) => d.action === 'case-bridge'))
  // Mod+J stays the terminal's.
  assert.deepEqual(matchShortcut(ev({ key: 'j', code: 'KeyJ' })), { action: 'toggle-terminal' })
})

test('the panel shows answers as text and goes with CASE', () => {
  assert.doesNotMatch(panelSource, /\{@html/)
  assert.match(appSource, /<CaseBridge route=\{route\} onNavigate=\{navigate\} \/>/)
  assert.match(appSource, /if \(showCompanion\) caseBridge\.toggle\(\)/)
})

type Call = { question: string; scope: CaseBridgeScope; view: CaseBridgeView }

function fakeApi(opts: {
  answers: CaseBridgeAnswer[]
  feed?: ChatEvent[]
  askError?: { status: number }
  attachThrows?: boolean
}) {
  const asked: Call[] = []
  let answerCalls = 0
  return {
    asked,
    get answerCalls() {
      return answerCalls
    },
    api: {
      askCaseBridge: async (question: string, scope: CaseBridgeScope, view: CaseBridgeView) => {
        asked.push({ question, scope, view })
        if (opts.askError) throw Object.assign(new Error('nope'), opts.askError)
        return { session_id: 'case-1' }
      },
      getCaseBridgeAnswer: async () => {
        const next = opts.answers[Math.min(answerCalls, opts.answers.length - 1)]
        answerCalls++
        return next
      },
      attachChatStream: async (_id: string, onEvent: (e: ChatEvent) => void) => {
        if (opts.attachThrows) throw new Error('dropped')
        for (const e of opts.feed ?? []) onEvent(e)
        return true
      },
    },
  }
}

const answer = (over: Partial<CaseBridgeAnswer>): CaseBridgeAnswer => ({ session_id: 'case-1', busy: false, question: '', answer: '', cards: [], ...over })

async function settle(store: { busy: boolean }) {
  for (let i = 0; i < 200 && store.busy; i++) await new Promise((r) => setTimeout(r, 1))
}

test('the store streams an answer without its card block, then shows the checked cards', async () => {
  const cards: CaseBridgeCard[] = [{ kind: 'focus', id: 'p1', title: 'Fix login', severity: 'block', href: '/console/focus/p1' }]
  const fake = fakeApi({
    feed: [{ type: 'delta', text: 'One pipeline waits.' } as ChatEvent, { type: 'delta', text: '\n<case-cards>[' } as ChatEvent],
    answers: [answer({ busy: true }), answer({ question: 'stuck?', answer: 'One pipeline waits.', cards })],
  })
  const store = new CaseBridgeStore(fake.api, { retryMs: 1, maxWaitMs: 5000 })
  store.scope = 'screen'
  assert.equal(await store.ask('  stuck?  ', { route: 'board' }), true)
  // "This screen" without a session on screen asks about all of TARS.
  assert.deepEqual(fake.asked[0], { question: 'stuck?', scope: 'all', view: { route: 'board' } })
  assert.equal(store.busy, true)
  assert.equal(await store.ask('again', { route: 'board' }), false)
  await settle(store)
  assert.equal(store.entries.length, 1)
  assert.deepEqual({ ...store.entries[0] }, { id: 1, question: 'stuck?', answer: 'One pipeline waits.', cards, status: 'done', error: null })

  // Scoped to a screen that shows a session.
  await store.ask('this one?', { route: 'chat', session_id: 's1' })
  assert.equal(fake.asked[1].scope, 'screen')
  await settle(store)
  assert.equal(await store.ask('   ', { route: 'board' }), false)
})

test('the store reports a refused, failed or unanswered question and keeps going', async () => {
  const busy = new CaseBridgeStore(fakeApi({ answers: [answer({})], askError: { status: 409 } }).api, { retryMs: 1, maxWaitMs: 100 })
  assert.equal(await busy.ask('q', { route: 'board' }), false)
  assert.equal(busy.entries[0].error, 'busy')

  const down = new CaseBridgeStore(fakeApi({ answers: [answer({})], askError: { status: 0 } }).api, { retryMs: 1, maxWaitMs: 100 })
  await down.ask('q', { route: 'board' })
  assert.equal(down.entries[0].error, 'network')

  const failed = new CaseBridgeStore(fakeApi({ answers: [answer({ error: "CASE couldn't answer" })], attachThrows: true }).api, { retryMs: 1, maxWaitMs: 1000 })
  await failed.ask('q', { route: 'board' })
  await settle(failed)
  assert.equal(failed.entries[0].status, 'error')
  assert.equal(failed.entries[0].error, 'unanswered')

  const stuck = new CaseBridgeStore(fakeApi({ answers: [answer({ busy: true })] }).api, { retryMs: 1, maxWaitMs: 20 })
  await stuck.ask('q', { route: 'board' })
  await settle(stuck)
  assert.equal(stuck.entries[0].error, 'unanswered')
})

test('opening the panel the first time brings back the last answer, once', async () => {
  const fake = fakeApi({ answers: [answer({ question: 'earlier?', answer: 'Yes.', cards: [] })] })
  const store = new CaseBridgeStore(fake.api, { retryMs: 1, maxWaitMs: 100 })
  store.toggle()
  assert.equal(store.open, true)
  await new Promise((r) => setTimeout(r, 5))
  assert.equal(store.entries.length, 1)
  assert.equal(store.entries[0].answer, 'Yes.')
  store.toggle()
  store.show()
  await new Promise((r) => setTimeout(r, 5))
  assert.equal(fake.answerCalls, 1)
  assert.equal(store.entries.length, 1)
  store.close()
  assert.equal(store.open, false)
  store.dispose()

  // Nothing asked yet: the panel starts empty.
  const empty = new CaseBridgeStore(fakeApi({ answers: [answer({})] }).api)
  empty.show()
  await new Promise((r) => setTimeout(r, 5))
  assert.equal(empty.entries.length, 0)

  // Still answering: the panel picks the stream up.
  const running = new CaseBridgeStore(
    fakeApi({ feed: [{ type: 'delta', text: 'Half' } as ChatEvent], answers: [answer({ busy: true, question: 'now?' }), answer({ question: 'now?', answer: 'Half done.' })] }).api,
    { retryMs: 1, maxWaitMs: 1000 },
  )
  running.show()
  await new Promise((r) => setTimeout(r, 5))
  await settle(running)
  assert.equal(running.entries[0].answer, 'Half done.')
})
