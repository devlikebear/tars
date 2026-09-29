import test from 'node:test'
import assert from 'node:assert/strict'

import { extractArtifact, extractArtifactsFromHistory } from '../src/lib/artifacts.ts'

test('a successful write becomes an artifact', () => {
  const artifact = extractArtifact('write_file', 'c1', '{"path":"notes.txt"}', '{"path":"notes.txt","created":true}')
  assert.equal(artifact?.path, 'notes.txt')
  assert.equal(artifact?.action, 'created')
})

// A denied or failed call wrote nothing, so the Files panel must not list it
// as changed (#970: denials made this common).
test('a failed or denied write is not an artifact', () => {
  assert.equal(
    extractArtifact('write_file', 'c1', '{"path":"notes.txt"}', 'Tool call denied: The user denied this tool call.', undefined, true),
    null,
  )
  const history = extractArtifactsFromHistory([
    { role: 'tool', toolName: 'write_file', toolCallId: 'c1', toolArgs: '{"path":"denied.txt"}', toolResult: 'Tool call denied', toolIsError: true },
    { role: 'tool', toolName: 'edit_file', toolCallId: 'c2', toolArgs: '{"path":"kept.txt"}', toolResult: '{"path":"kept.txt"}' },
  ])
  assert.deepEqual(history.map((a) => a.path), ['kept.txt'])
})
