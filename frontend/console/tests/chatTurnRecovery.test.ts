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

test('Stop leaves the stream to the server once it took the cancel', async () => {
  // The cancel answers when the turn is told to stop, before it has wound
  // down and freed the session. Aborting then ended the turn in the console
  // early: a queued message resumed at once was refused (409) and lost.
  const { stopTurn } = await import('../src/lib/chatTurnRecovery.ts')
  const calls: string[] = []
  await stopTurn({
    sessionId: 's1',
    cancel: async (id) => { calls.push(`cancel ${id}`); return true },
    abort: () => { calls.push('abort') },
  })
  assert.deepEqual(calls, ['cancel s1'])
})

test('Stop aborts the stream itself when the server took no cancel', async () => {
  const { stopTurn } = await import('../src/lib/chatTurnRecovery.ts')
  const calls: string[] = []
  const abort = () => { calls.push('abort') }
  // Refused or unreachable: nothing on the server will end the stream.
  await stopTurn({ sessionId: 's1', cancel: async () => { calls.push('cancel'); return false }, abort })
  assert.deepEqual(calls, ['cancel', 'abort'])
  // No session yet: there is nothing to cancel on the server.
  calls.length = 0
  await stopTurn({ sessionId: '', cancel: async () => { calls.push('cancel'); return true }, abort })
  assert.deepEqual(calls, ['abort'])
})

test('ChatPanel stops a turn through stopTurn', () => {
  const src = readFileSync(new URL('../src/components/ChatPanel.svelte', import.meta.url), 'utf8')
  const handler = src.slice(src.indexOf('async function handleCancel'), src.indexOf('// -- File attachments --'))
  assert.match(handler, /stopTurn\(/)
  assert.doesNotMatch(handler, /cancelChat\(/, 'the cancel goes through stopTurn, which decides on the abort')
})
