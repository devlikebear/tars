import test from 'node:test'
import assert from 'node:assert/strict'

import { compileSvelteModule } from './helpers/compileSvelteModule.ts'
import type * as QueueModule from '../src/lib/stores/messageQueue.svelte.ts'

const { MessageQueueStore } = await compileSvelteModule<typeof QueueModule>('src/lib/stores/messageQueue.svelte.ts')

test('messages queue per session and leave oldest first', () => {
  const q = new MessageQueueStore<string, string>()
  assert.equal(q.enqueue('s1', '  first  ', ['a.txt'], ['@x'])?.text, 'first')
  q.enqueue('s1', 'second')
  q.enqueue('s2', 'other')
  assert.equal(q.enqueue('s1', '   '), null, 'blank text is not queued')
  assert.equal(q.enqueue('', 'no session'), null)
  assert.deepEqual(q.items('s1').map((m) => m.text), ['first', 'second'])
  const first = q.take('s1')
  assert.deepEqual([first?.text, first?.files, first?.mentions], ['first', ['a.txt'], ['@x']])
  assert.equal(q.take('s1')?.text, 'second')
  assert.equal(q.take('s1'), null)
  assert.equal('s1' in q.queues, false, 'an empty queue is dropped')
  assert.deepEqual(q.items('s2').map((m) => m.text), ['other'])
})

test('edit, remove, and send-now reorder', () => {
  const q = new MessageQueueStore()
  const a = q.enqueue('s', 'a')!
  const b = q.enqueue('s', 'b')!
  const c = q.enqueue('s', 'c')!
  q.update('s', b.id, ' B ')
  q.moveToFront('s', c.id)
  q.moveToFront('s', 'missing')
  assert.deepEqual(q.items('s').map((m) => m.text), ['c', 'a', 'B'])
  assert.equal(q.remove('s', a.id)?.text, 'a')
  assert.equal(q.remove('s', a.id), null)
  q.update('s', b.id, '  ')
  assert.deepEqual(q.items('s').map((m) => m.text), ['c'], 'clearing the text removes the message')
  q.clear('s')
  assert.deepEqual(q.items('s'), [])
})

test('a paused queue sends nothing until resumed', () => {
  const q = new MessageQueueStore()
  q.pause('s')
  assert.equal(q.paused('s'), false, 'pausing an empty queue is a no-op')
  q.enqueue('s', 'a')
  q.pause('s')
  assert.equal(q.paused('s'), true)
  assert.equal(q.take('s'), null)
  q.resume('s')
  assert.equal(q.take('s')?.text, 'a')
  q.resume('s')
  assert.equal(q.paused('s'), false)
})

test('rekey moves a new chat queue to the session ID', () => {
  const q = new MessageQueueStore()
  q.enqueue('new', 'one')
  q.enqueue('s9', 'zero')
  q.pause('new')
  q.rekey('new', 's9')
  assert.deepEqual(q.items('s9').map((m) => m.text), ['zero', 'one'])
  assert.equal(q.paused('s9'), true)
  assert.deepEqual(q.items('new'), [])
  q.rekey('new', 's9')
  q.rekey('s9', 's9')
  q.rekey('', 's9')
  assert.equal(q.items('s9').length, 2)
})

test('a refused send goes back first in line and pauses the queue', () => {
  const q = new MessageQueueStore<string, string>()
  q.enqueue('s', 'first', ['a.txt'], ['@x'])
  q.enqueue('s', 'second')
  const sent = q.take('s')!
  q.enqueue('s', 'third')
  q.putBack('s', sent)
  assert.deepEqual(q.items('s').map((m) => m.text), ['first', 'second', 'third'])
  assert.deepEqual([q.items('s')[0].id, q.items('s')[0].files, q.items('s')[0].mentions], [sent.id, ['a.txt'], ['@x']], 'it goes back exactly as queued')
  assert.equal(q.paused('s'), true, 'nothing else goes out until the user resumes')
  assert.equal(q.take('s'), null)
  q.putBack('s', sent)
  assert.equal(q.items('s').length, 3, 'putting back a message already there does not copy it')
  q.resume('s')
  assert.equal(q.take('s')?.text, 'first')
})

test('a refused last message brings its queue back, paused', () => {
  const q = new MessageQueueStore()
  q.enqueue('s', 'only')
  const sent = q.take('s')!
  assert.equal('s' in q.queues, false)
  q.putBack('s', sent)
  assert.deepEqual(q.items('s').map((m) => m.text), ['only'])
  assert.equal(q.paused('s'), true)
  q.putBack('', sent)
  assert.equal('' in q.queues, false)
})
