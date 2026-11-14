import test from 'node:test'
import assert from 'node:assert/strict'

import { APIRequestError } from '../src/lib/api/client.ts'
import { ideaTitle, parseIdeas, sortedIdeas, type Idea } from '../src/lib/mobile/ideas.ts'
import { memoryStore } from '../src/lib/mobile/secureStore.ts'
import type { TaskAPI } from '../src/lib/mobile/desktopApi.ts'
import type { PairedDesktop } from '../src/lib/mobile/desktops.ts'
import { compileSvelteModule } from './helpers/compileSvelteModule.ts'
import type * as IdeaStoreModule from '../src/lib/mobile/ideaStore.svelte.ts'

const { IdeaStore } = await compileSvelteModule<typeof IdeaStoreModule>('src/lib/mobile/ideaStore.svelte.ts')

function backing() {
  const map = new Map<string, string>()
  return {
    map,
    getItem: (k: string) => map.get(k) ?? null,
    setItem: (k: string, v: string) => void map.set(k, v),
    removeItem: (k: string) => void map.delete(k),
  }
}

const mac: PairedDesktop = { id: 'https://mac.ts.net', name: 'mac', baseURL: 'https://mac.ts.net', token: 'tok', deviceID: 'd', pairedAt: '2026-11-14T00:00:00Z' }
const mini: PairedDesktop = { ...mac, id: 'https://mini.ts.net', name: 'mini', baseURL: 'https://mini.ts.net' }

function fakeTaskAPI(overrides: Partial<TaskAPI> = {}) {
  const created: Array<{ baseURL: string; token: string; body: unknown }> = []
  const api: TaskAPI = {
    async folders() {
      return [
        { path: '/work/app', name: 'app', exists: true },
        { path: '/work/gone', name: 'gone', exists: false },
      ]
    },
    async templates() {
      return [{ id: 'dev', name: 'Development', stages: [], builtin: true }]
    },
    async createPipeline(target, request) {
      created.push({ baseURL: target.baseURL, token: target.token, body: request })
      return { session_id: 'sess-1' }
    },
    ...overrides,
  }
  return { api, created }
}

let clock = 0
const deps = (b: ReturnType<typeof backing>, api: TaskAPI) => ({
  storage: memoryStore(b),
  api,
  now: () => new Date(Date.UTC(2026, 10, 14, 9, clock++)),
  newID: () => `idea-${clock}`,
})

test('ideas: first line is the title; unsent first, newest first; the stored list drops broken entries', () => {
  assert.equal(ideaTitle({ text: '  Dark mode\nfor the console ' }), 'Dark mode')
  const a: Idea = { id: 'a', text: 'a', createdAt: '2026-11-14T08:00:00Z' }
  const b: Idea = { id: 'b', text: 'b', createdAt: '2026-11-14T09:00:00Z' }
  const c: Idea = { id: 'c', text: 'c', createdAt: '2026-11-14T10:00:00Z', sent: { desktopID: 'x', desktopName: 'x', sessionID: 's', at: '2026-11-14T10:01:00Z' } }
  assert.deepEqual(sortedIdeas([a, c, b]).map((i) => i.id), ['b', 'a', 'c'])
  const raw = JSON.stringify([a, c, { id: 'blank', text: '  ', createdAt: 'x' }, { ...b, sent: { desktopID: 'x' } }, null, 'junk'])
  assert.deepEqual(parseIdeas(raw), [a, c, { ...b }])
  assert.deepEqual(parseIdeas('nope'), [])
  assert.deepEqual(parseIdeas('{}'), [])
})

test('ideas are kept on the phone: add, edit, delete, and read back', async () => {
  const b = backing()
  const { api } = fakeTaskAPI()
  const store = new IdeaStore(deps(b, api))
  await store.load()
  assert.equal(await store.add('   '), null)
  const idea = await store.add('  Dark mode toggle  ')
  assert.equal(idea?.text, 'Dark mode toggle')
  await store.edit(idea!.id, 'Dark mode toggle\nin settings')
  const again = new IdeaStore(deps(b, api))
  await again.load()
  assert.deepEqual(again.ideas.map((i) => i.text), ['Dark mode toggle\nin settings'])
  await again.remove(idea!.id)
  assert.deepEqual(parseIdeas(b.map.get('tars-mobile:ideas')), [])
})

test('sending an idea starts a task in a registered folder on the desktop and remembers where it went', async () => {
  const b = backing()
  const { api, created } = fakeTaskAPI()
  const store = new IdeaStore(deps(b, api))
  await store.load()
  const idea = (await store.add('Dark mode'))!

  await store.prepare(mac)
  // Only folders that exist are offered.
  assert.deepEqual(store.options?.folders.map((f) => f.path), ['/work/app'])
  assert.equal(store.options?.desktopID, mac.id)

  const sent = await store.send(mac, idea.id, ' Dark mode, with a toggle ', '/work/app', '')
  assert.equal(sent?.sessionID, 'sess-1')
  assert.equal(sent?.desktopName, 'mac')
  // Goal, folder; no template when the desk's default is picked, and nothing
  // else (the desk starts the first turn itself and refuses goal mode).
  assert.deepEqual(created, [{ baseURL: mac.baseURL, token: 'tok', body: { goal: 'Dark mode, with a toggle', cwd: '/work/app' } }])
  const saved = parseIdeas(b.map.get('tars-mobile:ideas'))[0]
  assert.equal(saved.text, 'Dark mode, with a toggle')
  assert.equal(saved.sent?.sessionID, 'sess-1')

  // A sent idea is not sent twice, nor edited.
  assert.equal(await store.send(mac, idea.id, 'again', '/work/app', ''), null)
  await store.edit(idea.id, 'changed')
  assert.equal(store.ideas[0].text, 'Dark mode, with a toggle')
  assert.equal(created.length, 1)

  const other = (await store.add('Release notes'))!
  await store.send(mac, other.id, 'Release notes', '/work/app', 'writing')
  assert.deepEqual(created[1].body, { goal: 'Release notes', cwd: '/work/app', template: 'writing' })
})

test('a send that fails says why and keeps the idea unsent', async () => {
  for (const [err, failure] of [
    [new APIRequestError('folder is not registered for phones', 403), 'folder'],
    [new APIRequestError('registered folders are unavailable', 503), 'unavailable'],
    [new APIRequestError('unauthorized', 401), 'unpaired'],
    [new APIRequestError('boom', 500), 'failed'],
    [new TypeError('Load failed'), 'unreachable'],
  ] as const) {
    const b = backing()
    const { api } = fakeTaskAPI({ createPipeline: async () => { throw err } })
    const store = new IdeaStore(deps(b, api))
    await store.load()
    const idea = (await store.add('x'))!
    assert.equal(await store.send(mac, idea.id, 'x', '/work/app', ''), null)
    assert.equal(store.sendFailure, failure)
    assert.equal(store.sending, false)
    assert.equal(parseIdeas(b.map.get('tars-mobile:ideas'))[0].sent, undefined)
  }
})

test("reading a desktop's choices: a failure says why, and a late answer for another desktop is dropped", async () => {
  let release: () => void = () => {}
  const slow = new Promise<void>((r) => { release = r })
  const { api } = fakeTaskAPI({
    async folders(target) {
      if (target.baseURL === mac.baseURL) {
        await slow
        return [{ path: '/late', name: 'late', exists: true }]
      }
      throw new APIRequestError('registered folders are unavailable', 503)
    },
  })
  const store = new IdeaStore(deps(backing(), api))
  const first = store.prepare(mac)
  await store.prepare(mini)
  assert.equal(store.optionsFailure, 'unavailable')
  release()
  await first
  assert.equal(store.options, null)
  assert.equal(store.loadingOptions, false)
})
