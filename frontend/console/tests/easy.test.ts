import test from 'node:test'
import assert from 'node:assert/strict'

import { EASY_BOTS, easyLive, easyView, hiddenWordsIn, type EasyProject } from '../src/lib/easy.ts'
import { easyEn, easyKo } from '../src/i18n/sections/easy.ts'
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

test('the easy routes', () => {
  assert.deepEqual(resolveRoute('/console/easy'), { view: 'easy' })
  assert.deepEqual(resolveRoute('/console/easy/'), { view: 'easy' })
  assert.deepEqual(resolveRoute('/console/easy/0123456789abcdef'), { view: 'easy', projectId: '0123456789abcdef' })
})
