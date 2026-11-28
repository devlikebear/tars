import test from 'node:test'
import assert from 'node:assert/strict'

import { EASY_BOTS, EASY_SCREEN_PARTS, EASY_STEPS, START_STEPS, checksPassed, currentVersion, hasBlueprint, easyCanChange, easyLimitResets, easyLive, easyRetryAction, easyStuck, easyView, engineName, hiddenWordsIn, previewPollMs, previewReachable, recreateEasyProject, restoreErrorKey, runEasySetup, startSteps, versionLabel, versionRows, type EasyProject, type EasyVersion } from '../src/lib/easy.ts'
import { easyEn, easyKo } from '../src/i18n/sections/easy.ts'
import { phonesEn, phonesKo } from '../src/i18n/sections/phones.ts'

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
  for (const step of START_STEPS) assert.ok(easyEn.start.steps[step] && easyKo.start.steps[step], step)
})

test('the Start screen marks its three steps from the server\'s answer', () => {
  const r = (extra: Record<string, unknown>) => ({ ready: false, engine: 'ok', tools: 'ok', ...extra }) as Parameters<typeof startSteps>[0]
  assert.deepEqual(startSteps(r({ next: 'install_engine', engine: 'missing', tools: 'missing' })), {
    steps: { install: 'now', signIn: 'next', tools: 'next' }, at: 1,
  })
  assert.deepEqual(startSteps(r({ next: 'sign_in', engine: 'signed_out' })), {
    steps: { install: 'done', signIn: 'now', tools: 'done' }, at: 2,
  })
  assert.deepEqual(startSteps(r({ next: 'install_tools', tools: 'missing' })), {
    steps: { install: 'done', signIn: 'done', tools: 'now' }, at: 3,
  })
  // Choosing an AI comes before the three: none of them is now.
  assert.deepEqual(startSteps(r({ next: 'choose_engine', engine: 'not_set', tools: 'ok' })), {
    steps: { install: 'next', signIn: 'next', tools: 'done' },
  })
})
import { resolveRoute } from '../src/lib/router.ts'
import { vibeOnlyRedirect } from '../src/lib/focus.ts'
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
  assert.equal(exhausted.retry, undefined, 'no blocked step to try again')
})

test('a stuck project: its blocked step to try again, the failed check, the count', () => {
  const make = [
    { id: 'plan', status: 'done', iteration: 1 },
    { id: 'make', kind: 'build', status: 'blocked', iteration: 4 },
    { id: 'review', kind: 'review', status: 'pending', iteration: 0 },
  ] as FocusPipeline['stages']
  const gate = { id: 'g7', kind: 'gate', stage: 'make', turn: 9, title: 'limit reached', payload: { reason: 'limit' }, state: 'open', created_at: '' } as FocusCard
  const results = [
    { label: 'The page opens', command: 'a', state: 'passed' },
    { label: 'The newest member is at the top', command: 'b', state: 'failed', check: { screen: 'Members', expected: 'at the top', seen: 'at the bottom' } },
  ] as FocusPipeline['check_results']
  const ended = { enabled: false, pushes: 20, max_pushes: 20, end_reason: 'exhausted' } as FocusPipeline['goal_mode']
  const stuck = easyView(project(), pipeline({ current: 'make', stages: make, open_gate: 'blocked', cards: [gate], check_results: results, goal_mode: ended }))
  assert.equal(stuck.phase, 'stuck')
  assert.deepEqual(stuck.retry, { cardId: 'g7' })
  assert.equal(stuck.stoppedAt, 'maker')
  // Goal mode that stopped on retries changing nothing reads the same way,
  // with the same blocked step for Try again.
  const noProgress = { ...ended, pushes: 2, end_reason: 'no_progress' } as FocusPipeline['goal_mode']
  const idle = easyView(project(), pipeline({ current: 'make', stages: make, open_gate: 'blocked', cards: [gate], goal_mode: noProgress }))
  assert.equal(idle.phase, 'stuck')
  assert.deepEqual(idle.retry, { cardId: 'g7' })
  assert.deepEqual(easyStuck(stuck), {
    label: 'The newest member is at the top', report: { screen: 'Members', expected: 'at the top', seen: 'at the bottom' }, timedOut: undefined, passed: 1, total: 2,
  })

  // While goal mode is still on it tries again by itself: the team is at
  // work, and nothing asks the person.
  const retrying = easyView(project(), pipeline({ current: 'make', stages: make, open_gate: 'blocked', cards: [gate], goal_mode: { enabled: true, pushes: 3, max_pushes: 20 } }))
  assert.equal(retrying.phase, 'building')
  assert.equal(retrying.bots.maker, 'working')
  assert.equal(retrying.retry, undefined)
  assert.equal(easyLive(retrying), true)

  // Without check results, the last failed check's report; without that, the count.
  const card = { id: 'f1', kind: 'failure', stage: 'make', turn: 1, state: 'unseen', created_at: '', title: 't', payload: { check: { expected: 'e', seen: 's' } } } as FocusCard
  assert.deepEqual(easyStuck(easyView(project(), pipeline({ current: 'make', open_gate: 'blocked', cards: [card] }))), { report: { screen: undefined, expected: 'e', seen: 's' }, passed: 0, total: 0 })
  assert.deepEqual(easyStuck(easyView(project(), pipeline({ current: 'make', open_gate: 'blocked' }))), { passed: 0, total: 0 })

  // Try again: a plain retry, or the person's words.
  assert.deepEqual(easyRetryAction('  '), { action: 'retry' })
  assert.deepEqual(easyRetryAction(' newest first '), { action: 'instruct', note: 'newest first' })
})

test('a project that could not be set up starts its idea again as a new one', async () => {
  const calls: string[] = []
  const failed = project({ id: 'old', status: 'failed', idea: 'A guest book', error_code: 'scaffold_failed' })
  const id = await recreateEasyProject({
    create: async (idea) => { calls.push(`create ${idea}`); return { project: project({ id: 'new', idea }) } },
    remove: async (old) => { calls.push(`remove ${old}`) },
  }, failed)
  assert.equal(id, 'new')
  assert.deepEqual(calls, ['create A guest book', 'remove old'])
  // The old record staying behind is not a failure.
  assert.equal(await recreateEasyProject({ create: async () => ({ project: project({ id: 'n2' }) }), remove: async () => { throw new Error('409') } }, failed), 'n2')
  // A refused creation is.
  await assert.rejects(recreateEasyProject({ create: async () => { throw new Error('503') }, remove: async () => {} }, failed))
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

  // Where the team stopped: the bot of the stage the limit stopped.
  assert.equal(limit.stoppedAt, 'maker')
  for (const section of [easyEn, easyKo]) assert.match(section.states.limit.stoppedAt('Make', 'Maker'), /Make.*Maker/)
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

test('the settings screen speaks plainly, in both languages, phone card included', () => {
  for (const [lang, section] of [['en', easyEn], ['ko', easyKo]] as const) {
    const settings = section.settings
    // The phone card's own words on this screen are the expert card's with
    // these in their place; the ones left are checked here too.
    const phones = { ...(lang === 'en' ? phonesEn : phonesKo), ...settings.phones }
    const shown = [
      phones.intro, phones.needsAddress, phones.answeredOnPhone(1, 2, 3), phones.pair, phones.newCode, phones.scanHint,
      phones.codeLabel, phones.expired, phones.devicesTitle, phones.noDevices, phones.revoke, phones.revoked,
      phones.keepAwakeTitle, phones.keepAwakeLabel, phones.keepAwakeHint, phones.keepAwakeActive, phones.keepAwakeNoPhone,
      phones.keepAwakeFailed, phones.keepAwakeUnsupported,
    ]
    for (const text of shown) assert.deepEqual(hiddenWordsIn(text), [], `${lang}: ${text}`)
    for (const tier of ['heavy', 'standard', 'light']) assert.ok(settings.ai.kinds[tier] && settings.ai.kindHints[tier], `${lang}: ${tier}`)
    for (const key of ['checking', 'on', 'off', 'noTailscale', 'signedOut', 'portTaken', 'needsPassword', 'failed'] as const) {
      assert.ok(settings.phone.access[key], `${lang}: ${key}`)
    }
  }
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

test('the result numbers its versions from the oldest saved one', () => {
  const v = (commit: string, kind: EasyVersion['kind'], title = commit, current = false): EasyVersion => ({ commit, title, kind, at: '2026-11-25T10:00:00Z', current })
  const words = { start: 'The start', backTo: (title: string) => `Back to: ${title}` }
  const rows = versionRows([v('c4', 'restored', 'First', true), v('c3', 'saved', 'Green button'), v('c2', 'saved', 'First'), v('c1', 'start')], words)
  assert.deepEqual(rows.map((r) => [r.number, r.label, r.current]), [
    [3, 'Back to: First', true], [2, 'Green button', false], [1, 'First', false], [undefined, 'The start', false],
  ])
  assert.equal(currentVersion(rows), 3)
  assert.equal(currentVersion(versionRows([v('c1', 'start', 'x', true)], words)), undefined)
  assert.equal(restoreErrorKey('busy'), 'busy')
  assert.equal(restoreErrorKey('not_ready'), 'busy')
  assert.equal(restoreErrorKey('not_found'), 'notFound')
  assert.equal(restoreErrorKey(''), 'unknown')
  for (const section of [easyEn, easyKo]) {
    for (const key of ['busy', 'notFound', 'unknown'] as const) assert.ok(section.versions.errors[key], key)
  }
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
  assert.deepEqual(resolveRoute('/console/easy/settings'), { view: 'easy', page: 'settings' })
  assert.equal(vibeOnlyRedirect(false, resolveRoute('/console/easy/settings')), null, 'settings stay reachable in vibe mode')
})

// The workspace (vibe redesign unit 4): questions, blueprint, steps, checks
// and the team's log, all read from the pipeline.
const question = (id: string, extra: Partial<FocusCard> = {}): FocusCard =>
  ({
    id, kind: 'decision', stage: 'plan', turn: 1, state: 'unseen', created_at: '', title: `Question ${id}?`,
    payload: { id, question: `Question ${id}?`, options: ['Yes', 'No', ' '], you_decide: true },
    ...extra,
  }) as FocusCard

test('the leader asks before the plan, and the answers wait on the person', () => {
  const asking = easyView(project(), pipeline({ cards: [question('q1'), question('q2')] }))
  assert.equal(asking.phase, 'questions')
  assert.equal(asking.bots.leader, 'needs_you')
  assert.equal(asking.step, 'plan')
  assert.deepEqual(asking.questions.map((q) => q.cardId), ['q1', 'q2'])
  assert.deepEqual(asking.questions[0].options, ['Yes', 'No'], 'blank choices are dropped')
  assert.equal(asking.questions[0].youDecide, true)
  assert.equal(asking.questions[0].answer, undefined)
  assert.equal(easyLive(asking), false, 'nothing changes until the person answers')
  assert.deepEqual(asking.log.map((l) => l.line), ['idea', 'asked'])
  assert.equal(asking.log[0].text, 'Book club sign-up')

  // One answered, one open: still the person's turn, with the answer kept.
  const half = easyView(project(), pipeline({ cards: [question('q1', { state: 'decided', decision: 'You decide' }), question('q2')] }))
  assert.equal(half.phase, 'questions')
  assert.equal(half.questions[0].answer, 'You decide')

  // All answered: the leader writes the blueprint.
  const answered = easyView(project(), pipeline({ cards: [question('q1', { state: 'decided', decision: 'Yes' }), question('q2', { state: 'decided', decision: 'No' })] }))
  assert.equal(answered.phase, 'planning')
  assert.equal(answered.working, 'leader')
  assert.deepEqual(answered.log.map((l) => l.line), ['idea', 'asked', 'drawing'])

  // A question from a later stage is not one of the leader's.
  assert.equal(easyView(project(), pipeline({ current: 'make', cards: [question('d1', { stage: 'make' })] })).questions.length, 0)
})

const blueprintPlan = {
  goal: 'A sign-up page',
  tasks: [{ title: 'A sign-up form', done: 'saves a name' }],
  stages: ['plan', 'make', 'review'],
  verify: ['sh checks/signup.sh', 'sh checks/list.sh'],
  checks: [{ label: 'Signing up shows "Thanks"', command: 'sh checks/signup.sh' }, { label: 'The list shows the newest first', command: 'sh checks/list.sh' }],
  screens: [{ name: 'Sign-up', line: 'Where members join', parts: [{ kind: 'heading', text: 'Join' }, { kind: 'video', text: 'x' }, { kind: 'button' }] }],
  saved: ['Names', ''],
  not_now: ['Payments'],
}

test('the blueprint is the plan in plain words, with its checks not run yet', () => {
  const v = easyView(project(), pipeline({ open_gate: 'plan', plan: blueprintPlan } as Partial<FocusPipeline>))
  assert.equal(v.phase, 'approve')
  assert.deepEqual(v.blueprint, {
    goal: 'A sign-up page',
    tasks: ['A sign-up form'],
    screens: [{ name: 'Sign-up', line: 'Where members join', parts: [{ kind: 'heading', text: 'Join' }, { kind: 'button', text: '' }] }],
    checks: ['Signing up shows "Thanks"', 'The list shows the newest first'],
    saved: ['Names'],
    notNow: ['Payments'],
  })
  assert.ok(hasBlueprint(v.blueprint!))
  assert.deepEqual(v.checks.map((c) => c.state), ['not_run', 'not_run'])
  assert.deepEqual(v.steps, { plan: 'now', make: 'next', check: 'next', publish: 'next' })
  assert.deepEqual(v.log.map((l) => l.line), ['idea', 'drew'])

  // A plan from before blueprints has only its goal and tasks.
  const old = easyView(project(), pipeline({ open_gate: 'plan', plan: { goal: 'g', tasks: [{ title: 't', done: 'd' }], stages: [], verify: [] } }))
  assert.equal(hasBlueprint(old.blueprint!), false)
  assert.deepEqual(old.checks, [])
})

const stages = (plan: string, make: string, review: string) => [
  { id: 'plan', status: plan, iteration: 1 },
  { id: 'make', kind: 'build', status: make, iteration: 1 },
  { id: 'review', kind: 'review', status: review, iteration: 1 },
] as FocusPipeline['stages']

test('making: who works, the steps, and the checks still to run', () => {
  const v = easyView(project(), pipeline({ current: 'make', plan: blueprintPlan, stages: stages('done', 'active', 'pending') } as Partial<FocusPipeline>))
  assert.equal(v.phase, 'building')
  assert.equal(v.step, 'make')
  assert.equal(v.working, 'maker')
  assert.deepEqual(v.steps, { plan: 'done', make: 'now', check: 'next', publish: 'next' })
  assert.deepEqual(v.log.map((l) => [l.who, l.line]), [['you', 'idea'], ['leader', 'handed'], ['maker', 'making']])
  assert.deepEqual(checksPassed(v.checks), { passed: 0, total: 2 })

  const checking = easyView(project(), pipeline({ current: 'make', awaiting_verification: true, plan: blueprintPlan, stages: stages('done', 'active', 'pending') } as Partial<FocusPipeline>))
  assert.equal(checking.step, 'check')
  assert.equal(checking.working, 'checker')

  const fixing = easyView(project(), pipeline({ current: 'review', review: { fixing: true }, plan: blueprintPlan, stages: stages('done', 'done', 'active') } as Partial<FocusPipeline>))
  assert.equal(fixing.working, 'maker')
  assert.deepEqual(fixing.log.slice(-2).map((l) => [l.who, l.line]), [['maker', 'made'], ['maker', 'fixing']])
})

test('checking: the server\'s result per check, a failed one with its report', () => {
  const results = [
    { label: 'Signing up shows "Thanks"', command: 'sh checks/signup.sh', state: 'passed', at: 'x' },
    { label: 'The list shows the newest first', command: 'sh checks/list.sh', state: 'failed', at: 'x', check: { screen: 'Members', expected: '"Kim" at the top', seen: '"Kim" at the bottom' } },
    { label: 'It survives a restart', command: 'sh checks/keep.sh', state: 'failed', timed_out: true, at: 'x' },
  ] as FocusPipeline['check_results']
  const v = easyView(project(), pipeline({ current: 'review', plan: blueprintPlan, stages: stages('done', 'done', 'active'), check_results: results } as Partial<FocusPipeline>))
  assert.equal(v.step, 'check')
  assert.deepEqual(v.checks, [
    { label: 'Signing up shows "Thanks"', state: 'passed' },
    { label: 'The list shows the newest first', state: 'failed', report: { screen: 'Members', expected: '"Kim" at the top', seen: '"Kim" at the bottom' } },
    { label: 'It survives a restart', state: 'failed', timedOut: true },
  ])
  assert.deepEqual(checksPassed(v.checks), { passed: 1, total: 3 })
  assert.deepEqual(v.log.slice(-2), [{ who: 'checker', line: 'checked', passed: 1, total: 3 }, { who: 'maker', line: 'fixingCheck' }])

  const done = easyView(project(), pipeline({ finished_at: 'x', plan: blueprintPlan, stages: stages('done', 'done', 'done'), check_results: results!.slice(0, 1) } as Partial<FocusPipeline>))
  assert.equal(done.phase, 'finished')
  assert.deepEqual(done.steps, { plan: 'done', make: 'done', check: 'done', publish: 'next' })
  assert.equal(done.working, undefined)
  assert.equal(done.log.at(-1)?.line, 'finished')
})

test('a change run opens its log with what the person asked', () => {
  const v = easyView(project({ session_id: 's2', changes: [{ request: 'Make the button green', session_id: 's2', at: '' }] }), pipeline({ session_id: 's2' }))
  assert.deepEqual(v.log[0], { who: 'you', line: 'change', text: 'Make the button green' })
})

test('every workspace sentence exists in both languages', () => {
  for (const section of [easyEn, easyKo]) {
    for (const step of EASY_STEPS) assert.ok(section.workspace.steps[step], step)
    for (const kind of EASY_SCREEN_PARTS) assert.ok(section.workspace.blueprint.part[kind], kind)
    for (const line of ['idea', 'change', 'asked', 'drawing', 'drew', 'handed', 'making', 'made', 'reviewing', 'fixing', 'checked', 'fixingCheck', 'finished'] as const) {
      assert.ok(section.workspace.log[line], line)
    }
    for (const state of ['not_run', 'passed', 'failed'] as const) assert.ok(section.workspace.check.state[state], state)
    assert.ok(section.status.questions)
  }
})

test('a project reads the same state on its screen as on the desk board (shared with easyproject.BoardOf)', async () => {
  const { readFileSync } = await import('node:fs')
  const { boardStateOfView } = await import('../src/lib/easyBoard.ts')
  const doc = JSON.parse(readFileSync(new URL('../../../internal/easyproject/testdata/board_states.json', import.meta.url), 'utf8')) as {
    cases: Array<{ name: string; project: EasyProject; pipeline: FocusPipeline | null; state: string }>
  }
  assert.ok(doc.cases.length > 0)
  for (const c of doc.cases) {
    assert.equal(boardStateOfView(easyView(c.project, c.pipeline)), c.state, c.name)
  }
})
