import test from 'node:test'
import assert from 'node:assert/strict'

import { requestJSONAt, APIRequestError } from '../src/lib/api/client.ts'
import {
  addressToBaseURL,
  blocksTasks,
  readinessLine,
  readinessProblems,
  desktopName,
  parseDesktops,
  statusFrom,
  withDesktop,
  withoutDesktop,
  type PairedDesktop,
} from '../src/lib/mobile/desktops.ts'
import { memoryStore } from '../src/lib/mobile/secureStore.ts'
import { parsePairingLink } from '../src/lib/devicePairing.ts'
import { compileSvelteModule } from './helpers/compileSvelteModule.ts'
import type * as DesktopStoreModule from '../src/lib/mobile/desktopStore.svelte.ts'
import type { DesktopAPI } from '../src/lib/mobile/desktopApi.ts'

const { DesktopStore } = await compileSvelteModule<typeof DesktopStoreModule>('src/lib/mobile/desktopStore.svelte.ts')

function desktop(id: string, extra: Partial<PairedDesktop> = {}): PairedDesktop {
  return { id, name: desktopName(id), baseURL: id, token: 't', deviceID: 'd', pairedAt: '2026-11-14T00:00:00.000Z', ...extra }
}

test('requestJSONAt sends the desktop token, never a cookie, at the desktop address', async () => {
  const original = globalThis.fetch
  const seen: Array<{ url: string; init?: RequestInit }> = []
  globalThis.fetch = async (input, init) => {
    seen.push({ url: String(input), init })
    return new Response(JSON.stringify({ ok: true }), { status: 200, headers: { 'Content-Type': 'application/json' } })
  }
  try {
    await requestJSONAt({ baseURL: 'https://mac.tail1.ts.net/', token: 'tok' }, '/v1/focus/pipelines', {
      headers: { 'Content-Type': 'application/json' },
    })
    await requestJSONAt({ baseURL: 'https://mac.tail1.ts.net', token: '' }, '/v1/healthz')
  } finally {
    globalThis.fetch = original
  }
  assert.equal(seen[0].url, 'https://mac.tail1.ts.net/v1/focus/pipelines')
  assert.equal(seen[0].init?.credentials, 'omit')
  const headers = new Headers(seen[0].init?.headers)
  assert.equal(headers.get('Authorization'), 'Bearer tok')
  assert.equal(headers.get('Content-Type'), 'application/json')
  assert.equal(new Headers(seen[1].init?.headers).get('Authorization'), null)
})

test('requestJSONAt reports the status of a refusal', async () => {
  const original = globalThis.fetch
  globalThis.fetch = async () => new Response(JSON.stringify({ error: 'nope' }), { status: 401 })
  try {
    await assert.rejects(requestJSONAt({ baseURL: 'https://a', token: 'x' }, '/v1/auth/whoami'), (err: unknown) => {
      return err instanceof APIRequestError && err.status === 401 && err.message === 'nope'
    })
  } finally {
    globalThis.fetch = original
  }
})

test('addresses: a bare Tailscale name means https; loopback http only for development', () => {
  assert.equal(addressToBaseURL(' mac-mini.tail1.ts.net '), 'https://mac-mini.tail1.ts.net')
  assert.equal(addressToBaseURL('https://mac-mini.tail1.ts.net:8443/console/'), 'https://mac-mini.tail1.ts.net:8443')
  assert.equal(addressToBaseURL('http://mac-mini.tail1.ts.net'), '')
  assert.equal(addressToBaseURL('http://127.0.0.1:43180'), '')
  assert.equal(addressToBaseURL('http://127.0.0.1:43180', { allowLoopbackHTTP: true }), 'http://127.0.0.1:43180')
  assert.equal(addressToBaseURL('http://192.168.0.5:43180', { allowLoopbackHTTP: true }), '')
  assert.equal(addressToBaseURL(''), '')
  // A scanned link follows the same rule.
  const devLink = 'tars-mobile://pair?url=http%3A%2F%2F127.0.0.1%3A43180&code=1'
  assert.equal(parsePairingLink(devLink), null)
  assert.deepEqual(parsePairingLink(devLink, { allowLoopbackHTTP: true }), { baseURL: 'http://127.0.0.1:43180', code: '1' })
})

test('desktop names and the list', () => {
  assert.equal(desktopName('https://mac-mini.tail1.ts.net'), 'mac-mini')
  assert.equal(desktopName('http://127.0.0.1:43180'), '127.0.0.1:43180')
  assert.equal(desktopName('http://localhost:43180'), 'localhost:43180')
  const a = desktop('https://a.ts.net')
  const b = desktop('https://b.ts.net')
  const a2 = desktop('https://a.ts.net', { token: 'new' })
  assert.deepEqual(withDesktop([a, b], a2), [a2, b])
  assert.deepEqual(withDesktop([a], b), [a, b])
  assert.deepEqual(withoutDesktop([a, b], a.id), [b])
})

test('the stored list drops entries that are not whole', () => {
  const good = desktop('https://a.ts.net')
  const raw = JSON.stringify([good, { ...good, token: '' }, { id: 'x' }, null, 'junk'])
  assert.deepEqual(parseDesktops(raw), [good])
  assert.deepEqual(parseDesktops('not json'), [])
  assert.deepEqual(parseDesktops('{}'), [])
  assert.deepEqual(parseDesktops(null), [])
})

test('status from the two probes', () => {
  assert.deepEqual(statusFrom(null, 0), { state: 'unreachable' })
  assert.deepEqual(statusFrom({ ok: true, version: '0.69.0' }, 200), { state: 'online', version: '0.69.0' })
  assert.equal(statusFrom({ ok: true }, 401).state, 'unpaired')
  assert.equal(statusFrom({ ok: true }, 403).state, 'unpaired')
  assert.equal(statusFrom({ ok: true, needs_setup: true }, 200).state, 'setup')
  assert.equal(statusFrom({ ok: true }, 0).state, 'unreachable')
})

function fakeAPI(overrides: Partial<DesktopAPI> = {}) {
  const pairs: Array<{ baseURL: string; code: string; name: string }> = []
  const api: DesktopAPI = {
    async pair(target, name) {
      pairs.push({ ...target, name })
      return { device_id: 'dev-1', name, role: 'focus', token: 'secret-token' }
    },
    async status() {
      return { state: 'online' }
    },
    ...overrides,
  }
  return { api, pairs }
}

function backing() {
  const map = new Map<string, string>()
  return {
    map,
    getItem: (k: string) => map.get(k) ?? null,
    setItem: (k: string, v: string) => void map.set(k, v),
    removeItem: (k: string) => void map.delete(k),
  }
}

test('pairing stores the desktop with its token and replaces an earlier pairing', async () => {
  const b = backing()
  const { api, pairs } = fakeAPI()
  const store = new DesktopStore({ storage: memoryStore(b), api, now: () => new Date('2026-11-14T01:02:03Z') })
  await store.load()
  assert.equal(store.desktops.length, 0)

  const paired = await store.pair('mac-mini.tail1.ts.net', ' 123456 ', ' Pixel ')
  assert.equal(paired?.name, 'mac-mini')
  assert.deepEqual(pairs, [{ baseURL: 'https://mac-mini.tail1.ts.net', code: '123456', name: 'Pixel' }])
  assert.equal(store.statuses['https://mac-mini.tail1.ts.net'].state, 'online')

  const saved = parseDesktops(b.map.get('tars-mobile:desktops'))
  assert.deepEqual(saved, [{
    id: 'https://mac-mini.tail1.ts.net',
    name: 'mac-mini',
    baseURL: 'https://mac-mini.tail1.ts.net',
    token: 'secret-token',
    deviceID: 'dev-1',
    pairedAt: '2026-11-14T01:02:03.000Z',
  }])

  await store.pair('https://mac-mini.tail1.ts.net', '654321', '')
  assert.equal(store.desktops.length, 1)
  assert.equal(pairs[1].name, 'iPhone')

  // A fresh store reads the same list back.
  const again = new DesktopStore({ storage: memoryStore(b), api })
  await again.load()
  assert.equal(again.desktops.length, 1)

  await again.forget('https://mac-mini.tail1.ts.net')
  assert.deepEqual(parseDesktops(b.map.get('tars-mobile:desktops')), [])
})

test('pairing failures say why and store nothing', async () => {
  const cases: Array<[unknown, string]> = [
    [new APIRequestError('invalid pairing code', 401), 'invalid_code'],
    [new APIRequestError('too many', 429), 'locked'],
    [new APIRequestError('login not allowed', 403), 'failed'],
    [new TypeError('Load failed'), 'unreachable'],
  ]
  for (const [err, failure] of cases) {
    const b = backing()
    const { api } = fakeAPI({ pair: async () => { throw err } })
    const store = new DesktopStore({ storage: memoryStore(b), api })
    await store.load()
    assert.equal(await store.pair('mac.tail1.ts.net', '1', 'x'), null)
    assert.equal(store.pairFailure, failure)
    assert.equal(store.pairing, false)
    assert.equal(b.map.size, 0)
  }

  const store = new DesktopStore({ storage: memoryStore(backing()), api: fakeAPI().api })
  assert.equal(await store.pair('http://mac.tail1.ts.net', '1', 'x'), null)
  assert.equal(store.pairFailure, 'invalid_address')
  assert.equal(await store.pair('mac.tail1.ts.net', '  ', 'x'), null)
  assert.equal(store.pairFailure, 'invalid_code')
})

test('refresh checks every desktop and a failing probe reads as unreachable', async () => {
  const b = backing()
  b.setItem('tars-mobile:desktops', JSON.stringify([desktop('https://a.ts.net'), desktop('https://b.ts.net'), desktop('https://c.ts.net')]))
  const { api } = fakeAPI({
    async status(target) {
      if (target.baseURL === 'https://b.ts.net') return { state: 'unpaired' }
      if (target.baseURL === 'https://c.ts.net') throw new Error('boom')
      return { state: 'online', version: '1' }
    },
  })
  const store = new DesktopStore({ storage: memoryStore(b), api })
  await store.load()
  await store.refresh()
  assert.deepEqual(store.statuses, {
    'https://a.ts.net': { state: 'online', version: '1' },
    'https://b.ts.net': { state: 'unpaired' },
    'https://c.ts.net': { state: 'unreachable' },
  })
})

test('a scanned link fills the pairing form', () => {
  const store = new DesktopStore({ storage: memoryStore(backing()), api: fakeAPI().api })
  assert.deepEqual(store.readLink('tars-mobile://pair?url=https%3A%2F%2Fmac.tail1.ts.net&code=42'), { baseURL: 'https://mac.tail1.ts.net', code: '42' })
  assert.equal(store.readLink('https://example.com'), null)
})

test('readiness: problems in plain words; only a missing model stops a task', () => {
  const text = {
    modelMissing: 'no model',
    modelMissingCLI: (cli: string) => `no ${cli}`,
    gh: { missing: 'no gh', signed_out: 'gh out' },
    tailscale: { not_serving: 'no serve' },
    unknown: '?',
  }
  const checks = [
    { id: 'model', state: 'missing', kind: 'claude-code-cli' },
    { id: 'gh', state: 'signed_out' },
    { id: 'tailscale', state: 'ok' },
  ]
  assert.deepEqual(readinessProblems(checks).map((c) => readinessLine(c, text)), ['no Claude Code (claude)', 'gh out'])
  assert.equal(readinessLine({ id: 'model', state: 'missing', kind: '' }, text), 'no model')
  assert.equal(readinessLine({ id: 'model', state: 'missing', kind: 'antigravity-cli' }, text), 'no Antigravity (agy)')
  assert.equal(readinessLine({ id: 'tailscale', state: 'not_serving' }, text), 'no serve')
  assert.equal(readinessLine({ id: 'gh', state: 'unknown' }, text), '?')
  assert.equal(readinessLine({ id: 'other', state: 'missing' }, text), '?')
  assert.equal(blocksTasks(checks), true)
  assert.equal(blocksTasks(checks.slice(1)), false)
  assert.equal(blocksTasks(undefined), false)
  assert.deepEqual(readinessProblems(undefined), [])
})

test('refresh reads readiness of desktops that answer, apart from the status', async () => {
  const b = backing()
  b.setItem('tars-mobile:desktops', JSON.stringify([desktop('https://a.ts.net'), desktop('https://b.ts.net'), desktop('https://old.ts.net')]))
  let release: () => void = () => {}
  const slow = new Promise<void>((r) => {
    release = r
  })
  const read: string[] = []
  const { api } = fakeAPI({
    async status(target) {
      return target.baseURL === 'https://b.ts.net' ? { state: 'unreachable' } : { state: 'online' }
    },
    async readiness(target) {
      read.push(target.baseURL)
      if (target.baseURL === 'https://old.ts.net') return null
      await slow
      return [{ id: 'model', state: 'ok' }, { id: 'gh', state: 'missing' }, { id: 'tailscale', state: 'ok' }]
    },
  })
  const store = new DesktopStore({ storage: memoryStore(b), api })
  await store.load()
  // refresh does not wait for readiness.
  await store.refresh()
  assert.equal(store.statuses['https://a.ts.net'].state, 'online')
  assert.deepEqual(store.readiness, {})
  release()
  await new Promise((r) => setTimeout(r, 0))
  assert.deepEqual(read.sort(), ['https://a.ts.net', 'https://old.ts.net'])
  assert.deepEqual(Object.keys(store.readiness), ['https://a.ts.net'])
  assert.equal(store.readiness['https://a.ts.net'][1].state, 'missing')

  await store.forget('https://a.ts.net')
  assert.deepEqual(store.readiness, {})
})
