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

test('opening another panel in the same zone replaces the active one', () => {
  const dock = new ChatDockStore()
  dock.open('git')
  dock.open('tasks')
  assert.equal(dock.right, 'tasks')
  assert.equal(dock.isOpen('git'), false)
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
  assert.equal(second.layout.sizes.left, 400)
})

test('a corrupt stored layout falls back to the default', () => {
  const dock = new ChatDockStore()
  dock.open('git')
  dock.restore(memoryStorage({ [chatDockStorageKey]: '{not json' }))
  assert.equal(dock.isOpen('git'), false)
  assert.equal(dock.left, 'sessions')
})
