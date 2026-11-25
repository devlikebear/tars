// The iPhone app's vibe-mode screens (docs/plans/vibe-mode-redesign unit 8):
// the project list across desktops, a project's tabs at phone width, the
// Computers tab's lines, the projects store, and that no phone sentence
// uses a hidden word.
import test from 'node:test'
import assert from 'node:assert/strict'

import { APIRequestError } from '../src/lib/api/client.ts'
import type { EasyAPI, EasyPreviewTicket } from '../src/lib/api/easy.ts'
import { easyView, hiddenWordsIn, type EasyProject, type EasyReadiness } from '../src/lib/easy.ts'
import type { EasyBoardState, EasyListed } from '../src/lib/easyBoard.ts'
import type { PairedDesktop } from '../src/lib/mobile/desktops.ts'
import { deskLines, deskTakesIdeas, defaultPhoneTab, filterCounts, phoneSections, phoneTabs, waitingCount, type PhoneProject } from '../src/lib/mobile/projects.ts'
import { tryApp, tryAppFailure, tryAppWaitMs } from '../src/lib/mobile/tryApp.ts'
import type { EasyPreview } from '../src/lib/easy.ts'
import { phoneEn, phoneKo } from '../src/i18n/sections/phone.ts'
import { mobileAppEn, mobileAppKo } from '../src/i18n/sections/mobileApp.ts'
import { compileSvelteModule } from './helpers/compileSvelteModule.ts'
import type * as ProjectStoreModule from '../src/lib/mobile/projectStore.svelte.ts'
import type { FocusPipeline } from '../src/lib/types.ts'

const { ProjectStore } = await compileSvelteModule<typeof ProjectStoreModule>('src/lib/mobile/projectStore.svelte.ts')

function project(id: string, state: EasyBoardState, changedAt: string, desk = 'mac'): PhoneProject {
  return {
    id, name: id, idea: id, folder: `~/TARS Projects/${id}`, status: 'ready', session_id: `${id}-run`,
    created_at: changedAt, updated_at: changedAt, board: { state, changed_at: changedAt }, desktopID: desk, desktopName: desk,
  }
}

test('the list puts what waits on the person first, then making, stopped and finished, newest first in each', () => {
  const list = [
    project('done-old', 'done', '2026-11-20T09:00:00Z'),
    project('turn', 'your_turn', '2026-11-20T08:00:00Z', 'mini'),
    project('making', 'working', '2026-11-20T10:00:00Z'),
    project('paused', 'paused', '2026-11-20T11:00:00Z'),
    project('done-new', 'done', '2026-11-21T09:00:00Z', 'mini'),
    project('online', 'published', '2026-11-19T09:00:00Z'),
    project('ready', 'starting', '2026-11-20T09:30:00Z'),
  ]
  assert.deepEqual(
    phoneSections(list, 'all').map((s) => [s.section, s.projects.map((p) => p.id)]),
    [
      ['your_turn', ['turn']],
      ['working', ['making', 'ready']],
      ['stopped', ['paused']],
      ['done', ['done-new', 'done-old', 'online']],
    ],
  )
  // A filter shows only its section; stopped projects show under All only.
  assert.deepEqual(phoneSections(list, 'working').map((s) => s.section), ['working'])
  assert.deepEqual(phoneSections(list, 'your_turn').map((s) => s.projects.length), [1])
  assert.deepEqual(phoneSections([], 'all'), [])
  assert.deepEqual(filterCounts(list), { all: 7, your_turn: 1, working: 2, done: 3 })
  assert.equal(waitingCount(list), 1)
})

function pipeline(extra: Partial<FocusPipeline> = {}): FocusPipeline {
  return {
    version: 1, session_id: 's', goal: 'g', current: 'plan', open_gate: '', cards: [], updated_at: '2026-11-20T00:00:00Z',
    stages: [
      { id: 'plan', status: 'active', iteration: 1, kind: 'plan' },
      { id: 'make', status: 'pending', iteration: 0, kind: 'build' },
      { id: 'review', status: 'pending', iteration: 0, kind: 'review' },
    ],
    ...extra,
  }
}

test("a project's screen shows one pane at a time: the phase's own first", () => {
  const p: EasyProject = { id: 'p', name: 'p', idea: 'i', folder: 'f', status: 'ready', session_id: 's', created_at: '', updated_at: '' }
  const plan = { goal: 'g', tasks: [{ title: 't', done: 'd' }], stages: ['plan', 'make', 'review'], verify: [] }
  const cases: Array<[string, ReturnType<typeof easyView>, string[]]> = [
    ['creating', easyView({ ...p, status: 'creating' }, null), ['progress', 'team']],
    ['planning', easyView(p, pipeline()), ['team']],
    ['approve', easyView(p, pipeline({ open_gate: 'plan', plan })), ['blueprint', 'team']],
    [
      'building',
      easyView(p, pipeline({ current: 'make', plan, stages: [{ id: 'plan', status: 'done', iteration: 1, kind: 'plan' }, { id: 'make', status: 'active', iteration: 1, kind: 'build' }] })),
      ['progress', 'team', 'blueprint'],
    ],
    ['finished', easyView(p, pipeline({ plan, finished_at: '2026-11-20T01:00:00Z' })), ['app', 'team', 'versions', 'checks']],
  ]
  for (const [phase, view, tabs] of cases) {
    assert.equal(view.phase, phase)
    assert.deepEqual(phoneTabs(view), tabs, phase)
    assert.equal(defaultPhoneTab(view), tabs[0])
  }
})

test("the Computers tab says in plain words what a desk still needs", () => {
  const ready: EasyReadiness = { ready: true, engine: 'ok', tools: 'ok' }
  assert.deepEqual(deskLines(ready), [{ key: 'on', ok: true }, { key: 'ai', ok: true }, { key: 'tools', ok: true }])
  assert.deepEqual(deskLines({ ready: false, next: 'sign_in', engine: 'signed_out', tools: 'missing' }).map((l) => l.key), ['on', 'aiSignedOut', 'toolsMissing'])
  assert.deepEqual(deskLines({ ready: false, engine: 'missing', tools: 'ok' }).map((l) => l.key), ['on', 'aiMissing', 'tools'])
  assert.deepEqual(deskLines({ ready: false, engine: 'not_set', tools: 'ok' }).map((l) => l.key), ['on', 'aiNotSet', 'tools'])
  // A desk too old to say has only "it is on".
  assert.deepEqual(deskLines(null), [{ key: 'on', ok: true }])
  for (const line of ['on', 'ai', 'aiMissing', 'aiSignedOut', 'aiNotSet', 'tools', 'toolsMissing'] as const) {
    assert.ok(phoneEn.computers.lines[line] && phoneKo.computers.lines[line], line)
  }

  assert.equal(deskTakesIdeas(true, ready), true)
  assert.equal(deskTakesIdeas(true, { ...ready, ready: false }), false)
  assert.equal(deskTakesIdeas(false, ready), false)
  assert.equal(deskTakesIdeas(true, null), true)
})

const mac: PairedDesktop = { id: 'https://mac.ts.net', name: 'mac', baseURL: 'https://mac.ts.net', token: 't', deviceID: 'd', pairedAt: '' }
const mini: PairedDesktop = { ...mac, id: 'https://mini.ts.net', name: 'mini', baseURL: 'https://mini.ts.net' }

function listed(id: string, state: EasyBoardState, extra: Partial<EasyListed> = {}): EasyListed {
  return {
    id, name: id, idea: id, folder: 'f', status: 'ready', session_id: `${id}-run`, created_at: '', updated_at: '',
    board: { state, changed_at: '2026-11-20T00:00:00Z' }, ...extra,
  }
}

function fakeEasy(projects: EasyListed[], overrides: Partial<EasyAPI> = {}): EasyAPI {
  const refuse = async () => {
    throw new Error('not in this test')
  }
  return {
    readiness: async () => ({ ready: true, engine: 'ok', tools: 'ok' }),
    board: async () => ({ projects, counts: {}, archived: 0 }),
    create: refuse, get: refuse, update: refuse, remove: refuse, change: refuse, versions: refuse, restore: refuse,
    preview: refuse, startPreview: refuse, previewTicket: refuse, pipeline: refuse, gate: refuse, card: refuse,
    ...overrides,
  }
}

test('the projects store reads every desk that answers, drops one that stopped, and finds a project by its run', async () => {
  let miniFails = false
  const apis = new Map<string, EasyAPI>([
    [mac.id, fakeEasy([listed('a', 'done', { changes: [{ request: 'green', session_id: 'a-change', at: '' }] })])],
    [
      mini.id,
      fakeEasy([listed('b', 'your_turn')], {
        board: async () => {
          if (miniFails) throw new TypeError('Load failed')
          return { projects: [listed('b', 'your_turn')], counts: {}, archived: 0 }
        },
        readiness: async () => {
          throw new APIRequestError('not found', 404)
        },
      }),
    ],
  ])
  const store = new ProjectStore({ apiFor: (d) => apis.get(d.id)! })
  await store.refresh([mac, mini])
  assert.equal(store.loaded, true)
  assert.deepEqual(store.projects.map((p) => [p.id, p.desktopName]), [['a', 'mac'], ['b', 'mini']])
  assert.equal(store.readiness[mac.id]?.ready, true)
  assert.equal(store.readiness[mini.id], null)
  assert.equal(store.findBySession(mac.id, 'a-change')?.id, 'a')
  assert.equal(store.findBySession(mac.id, 'a-run')?.id, 'a')
  assert.equal(store.findBySession(mini.id, 'a-run'), undefined)

  // A desk that answers health but not the list keeps its last projects
  // and is named as failed; a desk that stopped answering leaves the list.
  miniFails = true
  await store.refresh([mac, mini])
  assert.deepEqual(store.failed, [mini.id])
  await store.refresh([mac])
  assert.deepEqual(store.projects.map((p) => p.id), ['a'])
  assert.deepEqual(store.failed, [])
})

test('rename, archive and a new idea go to the desk the project is on; a refusal says why', async () => {
  const calls: unknown[] = []
  const api = fakeEasy([listed('a', 'done'), listed('b', 'working')], {
    update: async (id, change) => {
      calls.push(['update', id, change])
      if (id === 'b') throw new APIRequestError('busy', 409, { error: 'busy', code: 'busy' } as never)
      return listed(id, 'done', { name: change.name ?? id })
    },
    create: async (idea) => {
      calls.push(['create', idea])
      if (idea === 'nope') throw new APIRequestError('unauthorized', 401)
      return { project: listed('new', 'starting') }
    },
  })
  const store = new ProjectStore({ apiFor: () => api })
  await store.refresh([mac])
  const [a, b] = store.projects

  assert.equal(await store.rename(mac, a, '  '), 'name_required')
  assert.equal(await store.rename(mac, a, ' Recipes '), '')
  assert.equal(store.find(mac.id, 'a')?.name, 'Recipes')
  assert.equal(await store.archive(mac, b), 'busy')
  assert.equal(await store.archive(mac, a), '')
  assert.equal(store.find(mac.id, 'a'), undefined)

  assert.deepEqual(await store.create(mac, '  '), { failure: 'idea_required' })
  assert.equal(('project' in (await store.create(mac, ' A vote page '))), true)
  assert.deepEqual(await store.create(mac, 'nope'), { failure: 'unpaired' })
  assert.deepEqual(calls.slice(0, 3), [['update', 'a', { name: 'Recipes' }], ['update', 'b', { archived: true }], ['update', 'a', { archived: true }]])
  assert.deepEqual(calls[3], ['create', 'A vote page'])
})

test('no phone sentence uses a hidden word', () => {
  // The phone section, and what the phone still shows of the older mobile
  // section: the pairing sheet and its failures.
  const pick = (m: typeof mobileAppEn) => ({
    pair: [m.pairTitle, m.pairHint, m.addressLabel, m.codeLabel, m.phoneNameLabel, m.phoneNameHint, m.pair, m.pairing, m.cancel],
    failures: m.failures,
  })
  for (const [lang, sections] of [['en', [phoneEn, pick(mobileAppEn)]], ['ko', [phoneKo, pick(mobileAppKo)]]] as const) {
    const texts: string[] = []
    const walk = (v: unknown) => {
      if (typeof v === 'string') texts.push(v)
      else if (typeof v === 'function') texts.push(String((v as (...a: unknown[]) => string)('x', 2)))
      else if (v && typeof v === 'object') Object.values(v).forEach(walk)
    }
    sections.forEach(walk)
    assert.ok(texts.length > 40)
    for (const text of texts) assert.deepEqual(hiddenWordsIn(text), [], `${lang}: ${text}`)
  }
})

// Try it (vibe redesign unit 7): a fake desk that records each call.
function tryDesk(script: { tickets: Array<EasyPreviewTicket | Error>; reads?: Array<EasyPreview | Error>; starts?: Array<EasyPreview | Error> }) {
  const calls: string[] = []
  const next = <T>(list: Array<T | Error> | undefined, name: string): T => {
    const item = list?.shift()
    if (item === undefined) throw new Error(`no ${name} left`)
    if (item instanceof Error) throw item
    return item
  }
  let clock = 0
  const api = fakeEasy([], {
    previewTicket: async () => {
      calls.push('ticket')
      return next(script.tickets, 'ticket')
    },
    preview: async () => {
      calls.push('read')
      return next(script.reads, 'read')
    },
    startPreview: async () => {
      calls.push('start')
      return next(script.starts, 'start')
    },
  })
  const options = {
    now: () => clock,
    sleep: async (ms: number) => {
      calls.push(`wait ${ms}`)
      clock += ms
    },
  }
  return { api, calls, options, run: () => tryApp(api, 'p', options) }
}

const ticket = (url: string, state: EasyPreview['state'] = 'running'): EasyPreviewTicket => ({ url, expires_at: '', preview: { state } })
const refused = (status: number, code?: string) => new APIRequestError('refused', status, code ? { error: 'refused', code } : undefined)

test('Try it opens a running app with the first ticket', async () => {
  const d = tryDesk({ tickets: [ticket('https://mac.ts.net:8443/__tars/enter?t=a')] })
  assert.deepEqual(await d.run(), { kind: 'open', url: 'https://mac.ts.net:8443/__tars/enter?t=a' })
  assert.deepEqual(d.calls, ['ticket'])
})

test('Try it waits while the app starts, then asks for a fresh ticket', async () => {
  const d = tryDesk({
    tickets: [ticket('https://mac/__tars/enter?t=old', 'starting'), ticket('https://mac/__tars/enter?t=new')],
    reads: [{ state: 'starting' }, { state: 'running' }],
  })
  assert.deepEqual(await d.run(), { kind: 'open', url: 'https://mac/__tars/enter?t=new' })
  assert.deepEqual(d.calls, ['ticket', 'wait 1000', 'read', 'wait 1000', 'read', 'ticket'])
})

test('Try it starts an app stopped while it waited once, and gives up on one that fails or never answers', async () => {
  const restarted = tryDesk({
    tickets: [ticket('u', 'starting'), ticket('https://mac/t')],
    reads: [{ state: 'stopped' }],
    starts: [{ state: 'running' }],
  })
  assert.deepEqual(await restarted.run(), { kind: 'open', url: 'https://mac/t' })
  assert.deepEqual(restarted.calls, ['ticket', 'wait 1000', 'read', 'start', 'ticket'])

  const failed = tryDesk({ tickets: [ticket('u', 'starting')], reads: [{ state: 'failed', error_code: 'preview_failed' }] })
  assert.deepEqual(await failed.run(), { kind: 'failed', reason: 'could_not_start' })
  assert.deepEqual(failed.calls, ['ticket', 'wait 1000', 'read'])

  const slow = tryDesk({ tickets: [ticket('u', 'starting')], reads: Array.from({ length: 200 }, () => ({ state: 'starting' as const })) })
  assert.deepEqual(await slow.run(), { kind: 'failed', reason: 'could_not_start' })
  assert.equal(slow.calls.filter((c) => c === 'read').length, tryAppWaitMs / 1000)
  assert.ok(!slow.calls.slice(1).includes('ticket'))
})

test('Try it names each failure the desk can give', async () => {
  const cases: Array<[Error, string]> = [
    [refused(409, 'not_ready'), 'not_ready'],
    [refused(404, 'not_found'), 'not_ready'],
    [refused(409, 'preview_failed'), 'could_not_start'],
    [refused(503, 'remote_access_off'), 'phone_off'],
    [refused(503, 'relay_not_served'), 'phone_off'],
    [refused(503, 'tailscale_unavailable'), 'phone_off'],
    [refused(503, 'relay_down'), 'phone_off'],
    [refused(403), 'too_old'],
    [new TypeError('Load failed'), 'no_answer'],
    [new DOMException('timed out', 'TimeoutError'), 'no_answer'],
    [refused(500, 'ticket_failed'), 'failed'],
  ]
  for (const [err, reason] of cases) {
    assert.equal(tryAppFailure(err), reason, String(err))
    assert.deepEqual(await tryDesk({ tickets: [err] }).run(), { kind: 'failed', reason })
  }
  // A refusal of the second ticket is said the same way.
  const late = tryDesk({ tickets: [ticket('u', 'starting'), refused(503, 'relay_down')], reads: [{ state: 'running' }] })
  assert.deepEqual(await late.run(), { kind: 'failed', reason: 'phone_off' })
})

test('Try it strings say every failure without a hidden word', () => {
  for (const strings of [phoneEn, phoneKo]) {
    const tryIt = strings.project.tryIt
    const errors = tryIt.errors
    const said = [
      tryIt.intro, tryIt.button, tryIt.starting, tryIt.opened, tryIt.demo,
      errors.not_ready, errors.could_not_start('Mac'), errors.phone_off('Mac'), errors.too_old, errors.no_answer('Mac'), errors.failed,
    ]
    for (const text of said) assert.deepEqual(hiddenWordsIn(text), [], text)
  }
})
