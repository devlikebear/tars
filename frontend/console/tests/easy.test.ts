import test from 'node:test'
import assert from 'node:assert/strict'

import { EASY_BOTS, easyCanChange, easyLimitResets, easyLive, easyView, engineName, hiddenWordsIn, previewPollMs, previewReachable, runEasySetup, versionLabel, type EasyProject } from '../src/lib/easy.ts'
import { easyEn, easyKo } from '../src/i18n/sections/easy.ts'

test('the first run sets TARS up and waits for it to come back set up', async () => {
  const answers: Array<Error | { needs_setup?: boolean }> = [{ needs_setup: true }, new Error('refused'), { needs_setup: false }]
  let setUps = 0
  const sleeps: number[] = []
  const result = await runEasySetup({
    setUp: async () => { setUps++ },
    health: async () => {
      const next = answers.shift()!
      if (next instanceof Error) throw next
      return next
    },
    sleep: async (ms) => { sleeps.push(ms) },
  })
  assert.equal(result, 'ready')
  assert.equal(setUps, 1)
  assert.equal(sleeps.length, 3, 'one wait before each read')

  const refused = await runEasySetup({
    setUp: async () => { throw new Error('500') },
    health: async () => { throw new Error('not reached') },
    sleep: async () => {},
  })
  assert.equal(refused, 'failed')

  const slow = await runEasySetup({
    setUp: async () => {},
    health: async () => ({ needs_setup: true }),
    sleep: async () => {},
  }, 3)
  assert.equal(slow, 'timeout')
})

test('the Start screen names the engine as the person knows it', () => {
  assert.equal(engineName('claude-code-cli'), 'Claude Code')
  assert.equal(engineName(undefined), 'Claude Code')
  assert.equal(engineName('antigravity-cli'), 'Antigravity')
  assert.equal(engineName('anthropic'), 'AI')
  for (const state of ['ok', 'missing', 'signed_out', 'not_set']) {
    assert.ok(easyEn.start.state[state] && easyKo.start.state[state], state)
  }
})
import { resolveRoute } from '../src/lib/router.ts'
import type { FocusCard, FocusPipeline } from '../src/lib/types.ts'

const project = (extra: Partial<EasyProject> = {}): EasyProject => ({
  id: '0123456789abcdef', name: 'Book club sign-up', idea: 'Book club sign-up', folder: '/p',
  status: 'ready', session_id: 's1', created_at: '', updated_at: '', ...extra,
})

function pipeline(extra: Partial<FocusPipeline> = {}): FocusPipeline {
  return {
    version: 1, session_id: 's1', goal: 'g', current: 'plan', cards: [], updated_at: '', template: 'webapp',
    stages: [
      { id: 'plan', status: 'active', iteration: 1 },
      { id: 'make', kind: 'build', status: 'pending', iteration: 0 },
      { id: 'review', kind: 'review', status: 'pending', iteration: 0 },
    ],
    ...extra,
  } as FocusPipeline
}

test('a project being set up or failed has no pipeline to read', () => {
  assert.equal(easyView(project({ status: 'creating' }), null).phase, 'creating')
  const failed = easyView(project({ status: 'failed', error_code: 'node_missing' }), null)
  assert.equal(failed.phase, 'failed')
  assert.equal(failed.errorCode, 'node_missing')
  assert.equal(easyView(project(), null).phase, 'creating', 'ready, but the pipeline is not read yet')
})

test('the plan waits for the person with the plan in its own words', () => {
  const v = easyView(project(), pipeline({
    open_gate: 'plan',
    plan: { goal: 'A sign-up page', tasks: [{ title: 'A sign-up form', done: 'saves a name' }], stages: ['plan', 'make', 'review'], verify: ['npm run build'] },
  } as Partial<FocusPipeline>))
  assert.equal(v.phase, 'approve')
  assert.equal(v.bots.leader, 'needs_you')
  assert.deepEqual(v.plan, { goal: 'A sign-up page', tasks: ['A sign-up form'] })
  assert.equal(v.bots.publisher, 'later')
})

test('the team works through the stages; the checker works while checks run', () => {
  const planning = easyView(project(), pipeline())
  assert.equal(planning.phase, 'planning')
  assert.equal(planning.bots.leader, 'working')

  const making = easyView(project(), pipeline({
    current: 'make', awaiting_verification: true,
    stages: [
      { id: 'plan', status: 'done', iteration: 1 },
      { id: 'make', kind: 'build', status: 'active', iteration: 1 },
      { id: 'review', kind: 'review', status: 'pending', iteration: 0 },
    ],
  }))
  assert.equal(making.phase, 'building')
  assert.deepEqual([making.bots.leader, making.bots.maker, making.bots.reviewer, making.bots.checker], ['done', 'working', 'waiting', 'working'])
})

test('a failed check is told in its own words', () => {
  const card: FocusCard = {
    id: 'c9', kind: 'failure', stage: 'make', turn: 2, state: 'unseen', created_at: '',
    title: 'Check failed on Cart page: expected 30, but saw 25',
    payload: { command: 'sh check.sh', exit_code: 1, check: { screen: 'Cart page', expected: 'the total reads 30', seen: 'the total reads 25' } },
  } as FocusCard
  const v = easyView(project(), pipeline({ current: 'make', cards: [card] }))
  assert.deepEqual(v.check, { screen: 'Cart page', expected: 'the total reads 30', seen: 'the total reads 25' })
  // A later failure without a check report hides the earlier one.
  const plain = { ...card, id: 'c10', payload: { command: 'npm run build', exit_code: 2 } } as FocusCard
  assert.equal(easyView(project(), pipeline({ cards: [card, plain] })).check, undefined)
  assert.match(easyKo.project.checkFailed('Cart page', 'the total reads 30', 'the total reads 25'), /Cart page/)
})

test('finished, saved or not; stopped and stuck', () => {
  const done = pipeline({
    finished_at: 'x',
    stages: [
      { id: 'plan', status: 'done', iteration: 1 },
      { id: 'make', kind: 'build', status: 'done', iteration: 1 },
      { id: 'review', kind: 'review', status: 'done', iteration: 1 },
    ],
  })
  const saved = easyView(project(), { ...done, saved: { commit: 'abc' } } as FocusPipeline)
  assert.equal(saved.phase, 'finished')
  assert.equal(saved.saved, true)
  assert.equal(saved.bots.checker, 'done')
  assert.equal(easyView(project(), { ...done, saved: { error: 'save_failed' } } as FocusPipeline).saved, false)
  assert.equal(easyLive(saved), false)

  const blocked = easyView(project(), pipeline({ open_gate: 'blocked', current: 'make' }))
  assert.equal(blocked.phase, 'stuck')
  assert.equal(easyLive(blocked), false)
  const exhausted = easyView(project(), pipeline({ current: 'make', goal_mode: { enabled: false, pushes: 20, max_pushes: 20, end_reason: 'exhausted' } }))
  assert.equal(exhausted.phase, 'stuck')
})

test('a spent usage limit pauses the team until the person says to go on', () => {
  const gate = (payload: Record<string, unknown>, state = 'open'): FocusCard =>
    ({ id: 'c9', kind: 'gate', stage: 'make', turn: 1, title: 'turn failed', payload, state, created_at: '' }) as FocusCard
  const stopped = (payload: Record<string, unknown>) =>
    easyView(project(), pipeline({
      open_gate: 'blocked', current: 'make', cards: [gate(payload)],
      goal_mode: { enabled: false, pushes: 1, max_pushes: 20, end_reason: 'usage_limit' },
    }))

  const limit = stopped({ reason: 'turn_failed', usage_limit: true, resets: '2:40pm (Asia/Seoul)', error: 'session limit' })
  assert.equal(limit.phase, 'limit')
  assert.deepEqual(limit.limit, { cardId: 'c9', resets: '2:40pm (Asia/Seoul)', resetsAt: undefined })
  assert.equal(easyLive(limit), false)
  assert.equal(easyLimitResets(limit.limit!, 'en'), '2:40pm (Asia/Seoul)')

  // Any other failed turn still needs a look.
  assert.equal(stopped({ reason: 'turn_failed', error: 'provider timeout' }).phase, 'stuck')
  assert.equal(stopped({ reason: 'no_progress' }).phase, 'stuck')
  // A gate already answered is not the stop.
  const answered = easyView(project(), pipeline({ open_gate: 'blocked', current: 'make', cards: [gate({ reason: 'turn_failed', usage_limit: true }, 'decided')] }))
  assert.equal(answered.phase, 'stuck')

  // A time the provider gave is told as a local clock time, with the day
  // when it is not today; nothing at all when it said nothing.
  const now = new Date(2026, 10, 24, 9, 0)
  const today = new Date(2026, 10, 24, 14, 40).toISOString()
  assert.equal(easyLimitResets({ cardId: 'c', resetsAt: today }, 'en-US', now), '2:40 PM')
  const later = easyLimitResets({ cardId: 'c', resetsAt: new Date(2026, 10, 26, 14, 40).toISOString() }, 'en-US', now)
  assert.match(later, /Nov 26/)
  assert.equal(easyLimitResets({ cardId: 'c', resetsAt: 'not a time', resets: '3pm' }, 'en-US', now), '3pm')
  assert.equal(easyLimitResets({ cardId: 'c' }, 'en-US', now), '')

  // Both sentences, with and without a time, in both languages, use no
  // hidden word and name the time when there is one.
  for (const section of [easyEn, easyKo]) {
    assert.match(section.project.limitPaused('2:40pm'), /2:40pm/)
    assert.deepEqual(hiddenWordsIn(section.project.limitPaused('')), [])
  }
})

test('the hidden words are found as words, in either language', () => {
  assert.deepEqual(hiddenWordsIn('The pipeline opened a PR to merge').sort(), ['PR', 'merge', 'pipeline'])
  assert.deepEqual(hiddenWordsIn('Your projects. Progress, prices, legitimate, Gateway'), [])
  assert.deepEqual(hiddenWordsIn('파이프라인을 머지했어요'), ['머지', '파이프라인'])
})

test('no easy-mode sentence uses a hidden word', () => {
  for (const [lang, section] of [['en', easyEn], ['ko', easyKo]] as const) {
    const texts: string[] = []
    const walk = (v: unknown) => {
      if (typeof v === 'string') texts.push(v)
      else if (typeof v === 'function') texts.push(String((v as (...a: string[]) => string)('x', 'y', 'z')))
      else if (v && typeof v === 'object') Object.values(v).forEach(walk)
    }
    walk(section)
    for (const text of texts) assert.deepEqual(hiddenWordsIn(text), [], `${lang}: ${text}`)
  }
  for (const bot of EASY_BOTS) assert.ok(easyEn.bots[bot] && easyKo.bots[bot], bot)
})

test('a finished project takes a change once its version is saved, and a change run shows the request', () => {
  const finished = pipeline({
    finished_at: 'x',
    stages: [
      { id: 'plan', status: 'done', iteration: 1 },
      { id: 'make', kind: 'build', status: 'done', iteration: 1 },
      { id: 'review', kind: 'review', status: 'done', iteration: 1 },
    ],
  })
  const saving = easyView(project(), finished)
  assert.equal(saving.saved, undefined)
  assert.equal(easyLive(saving), true, 'keeps reading until the save is recorded')
  assert.equal(easyCanChange(saving), false)
  const saved = easyView(project(), { ...finished, saved: { commit: 'abc' } } as FocusPipeline)
  assert.equal(easyCanChange(saved), true)
  assert.equal(easyCanChange(easyView(project(), { ...finished, saved: { error: 'save_failed' } } as FocusPipeline)), true)
  assert.equal(easyCanChange(easyView(project(), pipeline())), false)

  const changed = project({
    session_id: 's2',
    changes: [
      { request: 'Make it green', session_id: 's1', at: '' },
      { request: 'Make it bigger', session_id: 's2', at: '' },
    ],
  })
  assert.equal(easyView(changed, pipeline({ session_id: 's2' })).change, 'Make it bigger')
  assert.equal(easyView(project(), pipeline()).change, undefined, 'the first version is no change')
})

test('a version is named by its own words, the start and a going back by the screen', () => {
  const words = easyEn.versions
  const v = (kind: 'start' | 'saved' | 'restored', title: string) => ({ commit: 'c', title, kind, at: '' })
  assert.equal(versionLabel(v('start', 'Start the project'), words), 'The empty starting project')
  assert.equal(versionLabel(v('saved', 'A sign-up page'), words), 'A sign-up page')
  assert.equal(versionLabel(v('restored', 'A sign-up page'), words), 'Back to: A sign-up page')
  assert.equal(versionLabel(v('restored', 'A sign-up page'), easyKo.versions), '되돌림: A sign-up page')
})

test('the preview is read soon while it starts, rarely while it runs, never after', () => {
  assert.equal(previewPollMs({ state: 'starting' }), 1000)
  assert.equal(previewPollMs({ state: 'running', url: 'http://127.0.0.1:5173/' }), 30_000)
  assert.equal(previewPollMs({ state: 'failed', error_code: 'preview_failed' }), null)
  assert.equal(previewPollMs({ state: 'stopped' }), null)
})

test('the preview shows only on the computer TARS runs on', () => {
  for (const host of ['127.0.0.1', 'localhost', '[::1]', '::1', 'tars.localhost', '127.0.0.2']) {
    assert.equal(previewReachable(host), true, host)
  }
  for (const host of ['desk.tailnet.ts.net', '192.168.0.10', 'example.com', 'localhost.example.com']) {
    assert.equal(previewReachable(host), false, host)
  }
})

test('the easy routes', () => {
  assert.deepEqual(resolveRoute('/console/easy'), { view: 'easy' })
  assert.deepEqual(resolveRoute('/console/easy/'), { view: 'easy' })
  assert.deepEqual(resolveRoute('/console/easy/0123456789abcdef'), { view: 'easy', projectId: '0123456789abcdef' })
  assert.deepEqual(resolveRoute('/console/easy/projects'), { view: 'easy', page: 'projects' })
  assert.deepEqual(resolveRoute('/console/easy/archive'), { view: 'easy', page: 'archive' })
})
