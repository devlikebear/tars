import test from 'node:test'
import assert from 'node:assert/strict'

import { APIRequestError } from '../src/lib/api/client.ts'
import { ideaTitle, parseIdeas, sortedIdeas, type Idea } from '../src/lib/mobile/ideas.ts'
import { memoryStore } from '../src/lib/mobile/secureStore.ts'
import type { IdeaAPI } from '../src/lib/mobile/ideaStore.svelte.ts'
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

function fakeIdeaAPI(create?: IdeaAPI['create']) {
  const created: Array<{ baseURL: string; token: string; idea: string }> = []
  const api: IdeaAPI = {
    create:
      create ??
      (async (target, idea) => {
        created.push({ baseURL: target.baseURL, token: target.token, idea })
        return { project: { id: 'proj-1', name: idea, idea, folder: '~/TARS Projects/x', status: 'ready', session_id: 's1', created_at: '', updated_at: '' } }
      }),
  }
  return { api, created }
}

let clock = 0
const deps = (b: ReturnType<typeof backing>, api: IdeaAPI) => ({
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

test('an idea sent before vibe mode (a focus task) and one sent as a project both read back', () => {
  const old: Idea = { id: 'o', text: 'o', createdAt: '2026-11-14T08:00:00Z', sent: { desktopID: 'x', desktopName: 'x', sessionID: 's', at: '2026-11-14T08:01:00Z' } }
  const now: Idea = { id: 'n', text: 'n', createdAt: '2026-11-14T09:00:00Z', sent: { desktopID: 'x', desktopName: 'x', projectID: 'p', at: '2026-11-14T09:01:00Z' } }
  assert.deepEqual(parseIdeas(JSON.stringify([old, now])), [old, now])
})

test('ideas are kept on the phone: add, delete, and read back', async () => {
  const b = backing()
  const { api } = fakeIdeaAPI()
  const store = new IdeaStore(deps(b, api))
  await store.load()
  assert.equal(await store.add('   '), null)
  const idea = await store.add('  A to-do list  ')
  assert.equal(idea?.text, 'A to-do list')
  const again = new IdeaStore(deps(b, api))
  await again.load()
  assert.deepEqual(again.ideas.map((i) => i.text), ['A to-do list'])
  await again.remove(idea!.id)
  assert.deepEqual(parseIdeas(b.map.get('tars-mobile:ideas')), [])
})

test("sending an idea starts a project on the desktop's team and remembers where it went", async () => {
  const b = backing()
  const { api, created } = fakeIdeaAPI()
  const store = new IdeaStore(deps(b, api))
  await store.load()
  const idea = (await store.add('A sign-up page'))!

  const sent = await store.send(mac, idea.id)
  assert.equal(sent?.projectID, 'proj-1')
  assert.equal(sent?.desktopName, 'mac')
  assert.deepEqual(created, [{ baseURL: mac.baseURL, token: 'tok', idea: 'A sign-up page' }])
  assert.equal(parseIdeas(b.map.get('tars-mobile:ideas'))[0].sent?.projectID, 'proj-1')
  assert.equal(store.sending, '')

  // A sent idea is not sent twice.
  assert.equal(await store.send(mini, idea.id), null)
  assert.equal(created.length, 1)
})

test('a send that fails says why and keeps the idea unsent', async () => {
  for (const [err, failure] of [
    [new APIRequestError('cannot run', 503, { error: 'x', code: 'cannot_run' } as never), 'cannot_run'],
    [new APIRequestError('unauthorized', 401), 'unpaired'],
    [new APIRequestError('boom', 500), 'unknown'],
    [new TypeError('Load failed'), 'unreachable'],
  ] as const) {
    const b = backing()
    const { api } = fakeIdeaAPI(async () => {
      throw err
    })
    const store = new IdeaStore(deps(b, api))
    await store.load()
    const idea = (await store.add('x'))!
    assert.equal(await store.send(mac, idea.id), null)
    assert.equal(store.sendFailure, failure)
    assert.equal(store.sending, '')
    assert.equal(parseIdeas(b.map.get('tars-mobile:ideas'))[0].sent, undefined)
  }
})
