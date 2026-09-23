import test from 'node:test'
import assert from 'node:assert/strict'

import { compileSvelteModule } from './helpers/compileSvelteModule.ts'
import type * as QueueModule from '../src/lib/stores/chatCommandQueue.svelte.ts'

const { ChatCommandQueue } = await compileSvelteModule<typeof QueueModule>('src/lib/stores/chatCommandQueue.svelte.ts')

test('take returns queued commands oldest first and empties the queue', () => {
  const queue = new ChatCommandQueue()
  queue.request({ kind: 'new-session' })
  queue.request({ kind: 'slash', command: 'compact' })
  assert.deepEqual(queue.take(), [{ kind: 'new-session' }, { kind: 'slash', command: 'compact' }])
  assert.deepEqual(queue.take(), [])
  assert.equal(queue.pending.length, 0)
})

test('sessionAt follows the sidebar order when published, else the fallback', () => {
  const queue = new ChatCommandQueue()
  assert.equal(queue.sessionAt(1, ['a', 'b', 'c']), 'b')
  queue.visibleSessionOrder = ['z', 'y']
  assert.equal(queue.sessionAt(0, ['a', 'b']), 'z')
  assert.equal(queue.sessionAt(5, ['a', 'b']), undefined)
})
