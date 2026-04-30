import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../src/components/Reflection.svelte', import.meta.url), 'utf8')

test('Reflection empty state previews the Run Now output shape', () => {
  assert.match(source, /Expected output/)
  assert.match(source, /experiences extracted/)
  assert.match(source, /empty sessions removed/)
  assert.match(source, /Pulse will surface repeated failures/)
  assert.match(source, /class="r-run-preview"/)
})

test('Reflection run result summarizes totals and job details', () => {
  assert.match(source, /Run totals/)
  assert.match(source, /lastRunBeforeManualRun/)
  assert.match(source, /jobDetailLine\(job\)/)
  assert.match(source, /firstDetailNumber\(job, \['kb_entries_compiled', 'knowledge_entries_compiled', 'entries_compiled'\]\)/)
  assert.match(source, /job.err/)
})
