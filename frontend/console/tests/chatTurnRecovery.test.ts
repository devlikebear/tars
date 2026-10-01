import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { recoverDroppedTurn, streamDropped } from '../src/lib/chatTurnRecovery.ts'

function recorder(outcome: { attached: boolean; ended: boolean } | Error) {
  const calls: string[] = []
  const deps = {
    reloadHistory: async () => { calls.push('reloadHistory') },
    reattach: async () => {
      calls.push('reattach')
      if (outcome instanceof Error) throw outcome
      return outcome
    },
    settle: async () => { calls.push('settle') },
  }
  return { calls, deps }
}

test('a dropped send stream is a network failure, not the user stopping it', () => {
  assert.equal(streamDropped(new TypeError('network error')), true)
  assert.equal(streamDropped(new Error('502 Bad Gateway')), true)
  assert.equal(streamDropped(new DOMException('aborted', 'AbortError')), false)
})

test('a turn that finished while the console was cut off is settled, so its cost shows', async () => {
  // The server ran the turn to the end during the gap: nothing to attach to.
  const { calls, deps } = recorder({ attached: false, ended: false })
  await recoverDroppedTurn(deps)
  assert.deepEqual(calls, ['reloadHistory', 'reattach', 'settle'])
})

test('a turn still running is followed again; its own done event settles it', async () => {
  const { calls, deps } = recorder({ attached: true, ended: true })
  await recoverDroppedTurn(deps)
  assert.deepEqual(calls, ['reloadHistory', 'reattach'])
})

test('a reattached stream that drops again before done still settles', async () => {
  const { calls, deps } = recorder({ attached: true, ended: false })
  await recoverDroppedTurn(deps)
  assert.deepEqual(calls, ['reloadHistory', 'reattach', 'settle'])
})

test('a failed reattach or history reload still settles', async () => {
  const { calls, deps } = recorder(new TypeError('network error'))
  deps.reloadHistory = async () => { calls.push('reloadHistory'); throw new Error('offline') }
  await recoverDroppedTurn(deps)
  assert.deepEqual(calls, ['reloadHistory', 'reattach', 'settle'])
})

test('ChatPanel recovers a dropped turn and re-reads usage when the event stream comes back', () => {
  const src = readFileSync(new URL('../src/components/ChatPanel.svelte', import.meta.url), 'utf8')
  assert.match(src, /recoverDroppedTurn\(/, 'the send path recovers a dropped stream')
  assert.match(src, /streamDropped\(err\)/, 'only a dropped stream, not a user stop, is recovered')
  const refresh = src.slice(src.indexOf('const refreshActiveSession'), src.indexOf('stopEventStream = streamEvents'))
  assert.match(refresh, /chatSession\.refreshUsage\(\)/, 'reconnect and refocus re-read the session cost')
})

test('a focus verification feed leaves no empty assistant bubble; other feeds keep theirs', async () => {
  const { dropVerificationPlaceholder } = await import('../src/lib/chatTurnRecovery.ts')
  const messages = [
    { id: 'u1', role: 'user', text: 'go' },
    { id: 'resumed', role: 'assistant', text: '' },
  ]
  assert.deepEqual(dropVerificationPlaceholder(messages, 'resumed', true).map((m) => m.id), ['u1'])
  assert.deepEqual(dropVerificationPlaceholder(messages, 'resumed', false).map((m) => m.id), ['u1', 'resumed'])
  const replied = [{ id: 'resumed', role: 'assistant', text: 'done' }]
  assert.deepEqual(dropVerificationPlaceholder(replied, 'resumed', true), replied)
})
