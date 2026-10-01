import { test } from 'node:test'
import assert from 'node:assert/strict'
import { transcriptChatMessages } from '../src/lib/transcriptMessages.ts'
import type { SessionMessage } from '../src/lib/types.ts'

const at = ''

test('a reopened turn keeps text and tool cards in the order they streamed', () => {
  const history: SessionMessage[] = [
    { id: 'u', role: 'user', content: 'fix the tests', timestamp: at },
    { id: 'a1', role: 'assistant', content: 'Reading the file first.', interim: true, timestamp: at },
    { id: 't1', role: 'tool', content: 'file contents', tool_name: 'Read', tool_call_id: 'toolu_r', tool_args: '{}', timestamp: at },
    { id: 'a2', role: 'assistant', content: 'Now the tests.', interim: true, timestamp: at },
    { id: 't2', role: 'tool', content: 'FAIL', tool_name: 'Bash', tool_call_id: 'toolu_b', tool_is_error: true, timestamp: at },
    { id: 'a3', role: 'assistant', content: 'All green.', timestamp: at },
  ]
  const got = transcriptChatMessages(history)
  assert.deepEqual(
    got.map((m) => `${m.role}:${m.role === 'tool' ? m.toolName : m.text}`),
    ['user:fix the tests', 'assistant:Reading the file first.', 'tool:Read', 'assistant:Now the tests.', 'tool:Bash', 'assistant:All green.'],
  )
  const bash = got[4]
  assert.equal(bash.id, 'tool-toolu_b')
  assert.equal(bash.toolResult, 'FAIL')
  assert.equal(bash.toolIsError, true)
  assert.equal(bash.toolDone, true)
  assert.equal(got[1].sourceMessageId, 'a1')
})

test('a turn that ended on a tool call shows no empty reply bubble', () => {
  const got = transcriptChatMessages([
    { id: 'a1', role: 'assistant', content: 'Running it.', interim: true, timestamp: at },
    { id: 't1', role: 'tool', content: 'ok', tool_name: 'Bash', tool_call_id: 'toolu_b', timestamp: at },
    { id: 'a2', role: 'assistant', content: '  ', timestamp: at },
  ])
  assert.deepEqual(got.map((m) => m.role), ['assistant', 'tool'])
})

test('an old transcript reads as before: tools first, then the reply', () => {
  const got = transcriptChatMessages([
    { id: 'u', role: 'user', content: 'go', timestamp: at },
    { id: 't1', role: 'tool', content: 'ok', tool_name: 'Read', tool_call_id: 'toolu_r', timestamp: at },
    { id: 'a', role: 'assistant', content: 'Read it.\nAll done.', timestamp: at },
  ])
  assert.deepEqual(got.map((m) => m.role), ['user', 'tool', 'assistant'])
  assert.equal(got[2].text, 'Read it.\nAll done.')
})

test('heartbeat and compaction markers stay hidden; a tool without an ID still gets a unique card', () => {
  const got = transcriptChatMessages([
    { id: 's1', role: 'system', content: '[HEARTBEAT] tick', timestamp: at },
    { id: 's2', role: 'system', content: '[COMPACTION SUMMARY] earlier', timestamp: at },
    { id: 's3', role: 'system', content: 'kept', timestamp: at },
    { id: 't1', role: 'tool', content: 'a', tool_name: 'exec', timestamp: at },
    { id: 't2', role: 'tool', content: 'b', tool_name: 'exec', timestamp: at },
  ])
  assert.deepEqual(got.map((m) => m.text || m.toolResult), ['kept', 'a', 'b'])
  assert.notEqual(got[1].id, got[2].id)
})
