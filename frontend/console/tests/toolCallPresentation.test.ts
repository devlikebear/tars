import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import {
  formatElapsedSeconds,
  formatToolInvocationPreview,
  formatToolJSON,
  toolCallTone,
} from '../src/lib/toolCalls.ts'

const itemSource = readFileSync(new URL('../src/components/ChatMessageItem.svelte', import.meta.url), 'utf8')
const panelSource = readFileSync(new URL('../src/components/ChatPanel.svelte', import.meta.url), 'utf8')

test('tool call presentation formats elapsed time and compact invocation previews', () => {
  assert.equal(formatElapsedSeconds(1_000, undefined, 3_340), '2.3s')
  assert.equal(formatElapsedSeconds(1_000, 5_080, 9_000), '4.1s')

  assert.equal(
    formatToolInvocationPreview('read_file', '{"path":"/tmp/example.txt","offset":10,"limit":20}'),
    'read_file(path=/tmp/example.txt, offset=10)',
  )
  assert.equal(
    formatToolInvocationPreview('tasks', '{"action":"add","title":"Ship tool timer","priority":"P2"}'),
    'tasks(action="add", title="Ship tool timer")',
  )
})

test('tool call presentation pretty-prints JSON and assigns state tones', () => {
  assert.equal(formatToolJSON('{"path":"README.md","limit":5}'), '{\n  "path": "README.md",\n  "limit": 5\n}')
  assert.equal(formatToolJSON('plain result'), 'plain result')
  assert.equal(toolCallTone({ toolDone: false }), 'running')
  assert.equal(toolCallTone({ toolDone: true }), 'done')
  assert.equal(toolCallTone({ toolDone: true, toolIsError: true }), 'error')
})

test('ChatMessageItem renders collapsible live tool call details', () => {
  assert.match(itemSource, /setInterval\(\(\) =>/)
  assert.match(itemSource, /<details/)
  assert.match(itemSource, /open=\{message\.toolIsError \|\| !message\.toolDone\}/)
  assert.match(itemSource, /formatToolInvocationPreview/)
  assert.match(itemSource, /formatElapsedSeconds/)
  assert.match(itemSource, /formatToolJSON/)
  assert.match(itemSource, /chat-tool-\{tone\}/)
  assert.match(itemSource, /badge-error/)
  assert.match(itemSource, /SubagentProgressCard/)
  assert.match(itemSource, /buildSubagentProgress/)
})

test('ChatPanel records tool timing and error metadata from stream and history', () => {
  assert.match(panelSource, /toolStartedAt: Date\.now\(\)/)
  assert.match(panelSource, /toolFinishedAt: Date\.now\(\)/)
  assert.match(panelSource, /toolIsError: event\.tool_is_error/)
  assert.match(panelSource, /toolIsError: msg\.tool_is_error/)
})
