import test from 'node:test'
import assert from 'node:assert/strict'

import { compileSvelteModule } from './helpers/compileSvelteModule.ts'
import type * as DockModule from '../src/lib/stores/chatDockStore.svelte.ts'

const { ChatDockStore, chatDockStorageKey } = await compileSvelteModule<typeof DockModule>('src/lib/stores/chatDockStore.svelte.ts')

function memoryStorage(initial: Record<string, string> = {}) {
  const data = new Map(Object.entries(initial))
  return {
    data,
    getItem: (key: string) => data.get(key) ?? null,
    setItem: (key: string, value: string) => { data.set(key, value) },
  }
}

test('panels open in their default zone and toggle closed again', () => {
  const dock = new ChatDockStore()
  dock.open('git')
  assert.equal(dock.right, 'git')
  assert.equal(dock.isOpen('git'), true)
  dock.toggle('git')
  assert.equal(dock.isOpen('git'), false)
  dock.toggle('terminal')
  assert.equal(dock.bottom, 'terminal')
})

test('opening another panel in the same zone stacks it as a tab on top', () => {
  const dock = new ChatDockStore()
  dock.open('git')
  dock.open('tasks')
  assert.deepEqual(dock.tabs('right'), ['git', 'tasks'])
  assert.equal(dock.right, 'tasks')
  assert.equal(dock.isOpen('git'), true)
  assert.equal(dock.isVisible('git'), false)
})

test('toggle brings a covered tab forward and closes only a visible one', () => {
  const dock = new ChatDockStore()
  dock.open('git')
  dock.open('tasks')
  dock.toggle('git')
  assert.equal(dock.right, 'git')
  assert.deepEqual(dock.tabs('right'), ['git', 'tasks'])
  dock.toggle('git')
  assert.equal(dock.isOpen('git'), false)
  assert.equal(dock.right, 'tasks')
})

test('closing the top tab hands the zone to its neighbor', () => {
  const dock = new ChatDockStore()
  dock.open('git')
  dock.open('tasks')
  dock.open('health')
  dock.open('tasks')
  dock.close('tasks')
  assert.equal(dock.right, 'health')
  dock.close('health')
  assert.equal(dock.right, 'git')
  dock.close('git')
  assert.equal(dock.right, undefined)
  assert.deepEqual(dock.tabs('right'), [])
})

test('a covered terminal tab still reports its zone', () => {
  const dock = new ChatDockStore()
  dock.move('terminal', 'right')
  dock.open('git')
  assert.equal(dock.right, 'git')
  assert.equal(dock.zoneOf('terminal'), 'right')
})

test('anyToolPanelOpen ignores the session list and the terminal', () => {
  const dock = new ChatDockStore()
  dock.closeToolPanels()
  dock.open('terminal')
  assert.equal(dock.anyToolPanelOpen, false)
  dock.open('config')
  assert.equal(dock.anyToolPanelOpen, true)
})

test('closeToolPanels keeps the session list and closes everything else', () => {
  const dock = new ChatDockStore()
  dock.open('sessions')
  dock.open('git')
  dock.open('terminal')
  dock.closeToolPanels()
  assert.equal(dock.isOpen('sessions'), true)
  assert.equal(dock.isOpen('git'), false)
  assert.equal(dock.isOpen('terminal'), false)
})

test('zoneOf follows a panel when it is re-docked', () => {
  const dock = new ChatDockStore()
  dock.open('terminal')
  assert.equal(dock.zoneOf('terminal'), 'bottom')
  dock.move('terminal', 'right')
  assert.equal(dock.zoneOf('terminal'), 'right')
  dock.move('terminal', 'fullscreen')
  assert.equal(dock.zoneOf('terminal'), 'fullscreen')
  dock.close('terminal')
  assert.equal(dock.zoneOf('terminal'), null)
})

test('layout survives a persist and restore round trip', () => {
  const storage = memoryStorage()
  const first = new ChatDockStore()
  first.open('git')
  first.move('git', 'left')
  first.resize('left', 400)
  first.persist(storage)
  assert.ok(storage.data.has(chatDockStorageKey))

  const second = new ChatDockStore()
  second.restore(storage)
  assert.equal(second.left, 'git')
  assert.deepEqual(second.tabs('left'), ['sessions', 'git'])
  assert.equal(second.layout.sizes.left, 400)
})

test('a covered tab and a hidden session list survive a reload', () => {
  const storage = memoryStorage()
  const first = new ChatDockStore()
  first.open('git')
  first.open('tasks')
  first.toggle('git')
  first.close('sessions')
  first.persist(storage)

  const second = new ChatDockStore()
  second.restore(storage)
  assert.deepEqual(second.tabs('right'), ['git', 'tasks'])
  assert.equal(second.right, 'git')
  assert.equal(second.isOpen('sessions'), false)
})

test('a layout stored before tabs restores one tab per zone', () => {
  const dock = new ChatDockStore()
  dock.restore(memoryStorage({
    [chatDockStorageKey]: JSON.stringify({ placements: { git: 'bottom' }, active: { right: 'tasks', bottom: 'git' } }),
  }))
  assert.deepEqual(dock.tabs('left'), ['sessions'])
  assert.deepEqual(dock.tabs('right'), ['tasks'])
  assert.deepEqual(dock.tabs('bottom'), ['git'])
})

test('a corrupt stored layout falls back to the default', () => {
  const dock = new ChatDockStore()
  dock.open('git')
  dock.restore(memoryStorage({ [chatDockStorageKey]: '{not json' }))
  assert.equal(dock.isOpen('git'), false)
  assert.equal(dock.left, 'sessions')
})
