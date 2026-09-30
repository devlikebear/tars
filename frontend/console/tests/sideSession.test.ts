import { test } from 'node:test'
import assert from 'node:assert/strict'
import { applySideEvent, historyMessages } from '../src/lib/sideSession.ts'
import type { ChatMessage } from '../src/lib/chatMessages.ts'

test('history shows the user words without the console context', () => {
  const got = historyMessages([
    { id: 'u', role: 'user', content: 'hi\n\n<console-context>\nbe brief\n</console-context>', timestamp: '' },
  ])
  assert.equal(got[0].text, 'hi')
})

test('history keeps words and one line per tool call', () => {
  const got = historyMessages([
    { id: 'u', role: 'user', content: ' hi ', timestamp: '' },
    { id: 's', role: 'system', content: 'hidden', timestamp: '' },
    { id: 't', role: 'tool', content: 'long output', tool_name: 'exec', tool_is_error: true, timestamp: '' },
    { id: 'x', role: 'tool', content: 'no name', timestamp: '' },
    { id: 'a', role: 'assistant', content: '', timestamp: '' },
    { id: 'b', role: 'assistant', content: 'done', timestamp: '' },
  ])
  assert.deepEqual(got.map((m) => `${m.role}:${m.text || m.toolName}`), ['user:hi', 'tool:exec', 'assistant:done'])
  assert.equal(got[1].toolIsError, true)
})

test('a turn streams into one reply bubble and its approval cards', () => {
  let messages: ChatMessage[] = [{ id: 'u', role: 'user', text: 'go' }]
  messages = applySideEvent(messages, { type: 'delta', text: '' }, 'r')
  assert.equal(messages.length, 1)
  messages = applySideEvent(messages, { type: 'delta', text: 'Hel' }, 'r')
  messages = applySideEvent(messages, { type: 'delta', text: 'lo' }, 'r')
  assert.equal(messages.at(-1)?.text, 'Hello')
  messages = applySideEvent(messages, { type: 'status', phase: 'before_tool_call', tool_name: 'write_file', tool_call_id: 'c1' }, 'r')
  messages = applySideEvent(messages, { type: 'status', phase: 'after_llm' }, 'r')
  assert.equal(messages.at(-1)?.toolName, 'write_file')
  const request = { type: 'permission_request', session_id: 's', request_id: 'p1', tool_name: 'write_file' }
  messages = applySideEvent(messages, request, 'r')
  messages = applySideEvent(messages, request, 'r')
  assert.equal(messages.filter((m) => m.role === 'approval').length, 1)
  messages = applySideEvent(messages, { type: 'permission_resolved', request_id: 'p1', outcome: 'allowed' }, 'r')
  assert.equal(messages.find((m) => m.role === 'approval')?.approval?.state, 'allowed')
  assert.equal(applySideEvent(messages, { type: 'permission_resolved' }, 'r'), messages)
  messages = applySideEvent(messages, { type: 'error', error: 'boom' }, 'r')
  assert.equal(messages.at(-1)?.text, 'boom')
  assert.equal(applySideEvent(messages, { type: 'context_info' }, 'r'), messages)
  assert.equal(applySideEvent(messages, { type: 'permission_request' }, 'r'), messages)
})

test('a CLI provider tool shows as one card that settles on its result, even when replayed', () => {
  const start = { type: 'status', phase: 'provider_tool', tool_name: 'Bash', tool_call_id: 'p1' }
  let messages: ChatMessage[] = applySideEvent([], start, 'r')
  messages = applySideEvent(messages, start, 'r')
  assert.equal(messages.length, 1)
  assert.equal(messages[0].toolDone, false)
  messages = applySideEvent(messages, { type: 'status', phase: 'provider_tool_result', tool_call_id: 'p1', tool_is_error: true }, 'r')
  assert.equal(messages[0].toolDone, true)
  assert.equal(messages[0].toolIsError, true)
})

test('tool lines keep their arguments so the side panel can label them like the main thread', () => {
  const got = historyMessages([
    { id: 't', role: 'tool', content: 'ok', tool_name: 'Bash', tool_args: '{"command":"git status"}', timestamp: '' },
  ])
  assert.equal(got[0].toolArgs, '{"command":"git status"}')
  let live: ChatMessage[] = applySideEvent([], { type: 'status', phase: 'before_tool_call', tool_name: 'read_file', tool_call_id: 'c', tool_args_preview: '{"path":"a"}' }, 'r')
  assert.equal(live[0].toolArgs, '{"path":"a"}')
  live = applySideEvent([], { type: 'status', phase: 'provider_tool', tool_name: 'Read', tool_call_id: 'p', tool_args_preview: '{"file_path":"/x"}' }, 'r')
  assert.equal(live[0].toolArgs, '{"file_path":"/x"}')
})
