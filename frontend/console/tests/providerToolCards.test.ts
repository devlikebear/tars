import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  isProviderToolPhase,
  providerToolCard,
  settleInterruptedProviderTools,
  settleProviderToolCard,
} from '../src/lib/providerToolCards.ts'
import type { ChatMessage } from '../src/lib/chatMessages.ts'

const start = { type: 'status', phase: 'provider_tool', tool_name: 'Bash', tool_call_id: 't1', tool_args_preview: '{"command":"make test"}' }

test('a provider tool start becomes a running card shaped like a native one', () => {
  const card = providerToolCard(start, 1000)
  assert.deepEqual(card, {
    id: 'tool-t1',
    role: 'tool',
    text: '',
    toolName: 'Bash',
    toolCallId: 't1',
    toolArgs: '{"command":"make test"}',
    toolDone: false,
    toolStartedAt: 1000,
    toolUpstream: true,
  })
  assert.equal(providerToolCard({ type: 'status', phase: 'provider_tool', tool_call_id: 't1' }, 1), null, 'no name, no card')
})

test('the result settles the card as done or failed', () => {
  const messages: ChatMessage[] = [providerToolCard(start, 1000)!, { id: 'a', role: 'assistant', text: '' }]
  const failed = settleProviderToolCard(messages, { type: 'status', phase: 'provider_tool_result', tool_name: 'Bash', tool_call_id: 't1', tool_result_preview: 'FAIL', tool_is_error: true }, 2500)
  assert.ok(failed)
  assert.equal(failed[0].toolDone, true)
  assert.equal(failed[0].toolIsError, true)
  assert.equal(failed[0].toolResult, 'FAIL')
  assert.equal(failed[0].toolFinishedAt, 2500)
  assert.equal(messages[0].toolDone, false, 'input is not mutated')

  const ok = settleProviderToolCard(messages, { type: 'status', phase: 'provider_tool_result', tool_call_id: 't1', tool_result_preview: 'ok' }, 2000)
  assert.equal(ok?.[0].toolIsError, false)
  assert.equal(settleProviderToolCard(messages, { type: 'status', phase: 'provider_tool_result', tool_call_id: 'nope' }, 1), null)
})

test('only provider phases are claimed', () => {
  assert.equal(isProviderToolPhase('provider_tool'), true)
  assert.equal(isProviderToolPhase('provider_tool_result'), true)
  assert.equal(isProviderToolPhase('before_tool_call'), false)
})

test('a turn that ends early stops its provider cards spinning and leaves native ones alone', () => {
  const messages: ChatMessage[] = [
    providerToolCard(start, 1000)!,
    { id: 'tool-n', role: 'tool', text: '', toolName: 'exec', toolCallId: 'n', toolDone: false },
    { id: 'tool-d', role: 'tool', text: '', toolName: 'Read', toolCallId: 'd', toolDone: true, toolUpstream: true, toolResult: 'x' },
  ]
  const got = settleInterruptedProviderTools(messages, 5000)
  assert.equal(got[0].toolDone, true)
  assert.equal(got[0].toolFinishedAt, 5000)
  assert.equal(got[1].toolDone, false)
  assert.equal(got[2], messages[2])
  assert.equal(settleInterruptedProviderTools([messages[1]], 1)[0], messages[1])
})
