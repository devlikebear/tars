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
  assert.match(src, /sendFailure\(err\)/, 'only a dropped stream, not a user stop, is recovered')
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

test('a send the server refused before the turn started is told from a dropped stream', async () => {
  const { sendFailure } = await import('../src/lib/chatTurnRecovery.ts')
  const { APIRequestError } = await import('../src/lib/api/client.ts')
  assert.equal(sendFailure(new APIRequestError('turn_running', 409)), 'refused', 'one turn at a time: the claim is still held')
  assert.equal(sendFailure(new APIRequestError('boom', 500)), 'refused', 'any non-2xx answer means no turn started')
  assert.equal(sendFailure(new TypeError('network error')), 'dropped', 'a broken stream may have a turn running on')
  assert.equal(sendFailure(new Error('chat stream body missing')), 'dropped')
  assert.equal(sendFailure(new DOMException('aborted', 'AbortError')), 'stopped')
})

test('a refused queued send goes back in the queue instead of being recovered as a dropped turn', () => {
  const src = readFileSync(new URL('../src/components/ChatPanel.svelte', import.meta.url), 'utf8')
  const send = src.slice(src.indexOf('async function submitChat'), src.indexOf('async function afterTurn'))
  assert.match(send, /sendFailure\(err\)/)
  assert.match(send, /const item = queuedPayload \?\?/, 'a refused queued message keeps its id')
  assert.match(send, /messageQueue\.putBack\(queueKey, item\)/, 'the refused message is put back, not lost')
  const refused = send.indexOf("=== 'refused'")
  assert.ok(refused > 0 && refused < send.indexOf('recoverDroppedTurn('), 'refusal is handled before dropped-turn recovery')
})

test('a refused send goes back where nothing the user typed since is overwritten', async () => {
  const { refusedSendReturn } = await import('../src/lib/chatTurnRecovery.ts')
  assert.equal(refusedSendReturn({ queued: true, composerHasDraft: false }), 'queue', 'a queued message keeps its place in line')
  assert.equal(refusedSendReturn({ queued: false, composerHasDraft: false }), 'composer', 'what the user just sent is back in front of them')
  assert.equal(refusedSendReturn({ queued: false, composerHasDraft: true }), 'queue', 'a new draft is kept; the refused message waits first in the queue')
})

test('a refused composer send is returned, not recovered as a dropped turn', () => {
  const src = readFileSync(new URL('../src/components/ChatPanel.svelte', import.meta.url), 'utf8')
  const send = src.slice(src.indexOf('async function submitChat'), src.indexOf('async function afterTurn'))
  assert.match(send, /refusedSendReturn\(/)
  assert.match(send, /changes\.restoreNotes\(/, 'review notes taken for the send come back')
  assert.doesNotMatch(send, /failure === 'refused' && queuedPayload/, 'every refused send is handled, not only queued ones')
})

test('recovery reports whether the history came back and the turn was found running', async () => {
  let r = recorder({ attached: true, ended: false })
  assert.deepEqual(await recoverDroppedTurn(r.deps), { reloaded: true, attached: true })
  r = recorder(new TypeError('network error'))
  r.deps.reloadHistory = async () => { throw new Error('offline') }
  assert.deepEqual(await recoverDroppedTurn(r.deps), { reloaded: false, attached: false })
})

test('a dropped send counts as lost only when the reloaded history shows it never arrived', async () => {
  const { droppedSendDelivery } = await import('../src/lib/chatTurnRecovery.ts')
  const sent = 'rework the parser'
  assert.equal(droppedSendDelivery({ reloaded: true, attached: true }, undefined, sent), 'delivered', 'its turn is running')
  assert.equal(droppedSendDelivery({ reloaded: true, attached: false }, `${sent}\n\n<review-notes>\n1. a.txt\n</review-notes>`, sent), 'delivered', 'its turn ran during the gap')
  assert.equal(droppedSendDelivery({ reloaded: true, attached: false }, 'an earlier message', sent), 'lost')
  assert.equal(droppedSendDelivery({ reloaded: true, attached: false }, undefined, sent), 'lost', 'a new chat with nothing in it')
  assert.equal(droppedSendDelivery({ reloaded: false, attached: false }, undefined, sent), 'unknown', 'the server is unreachable')
})

test('a dropped send that never arrived gives back its message and notes', () => {
  const src = readFileSync(new URL('../src/components/ChatPanel.svelte', import.meta.url), 'utf8')
  const send = src.slice(src.indexOf('async function submitChat'), src.indexOf('async function afterTurn'))
  assert.match(send, /droppedSendDelivery\(/)
  assert.match(send, /=== 'lost'/)
  assert.match(send, /giveBackSend\(/)
})
