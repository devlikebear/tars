import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../src/components/Reflection.svelte', import.meta.url), 'utf8')

test('Reflection page introduces nightly maintenance before status readouts', () => {
  assert.match(source, /Reflection - Nightly Maintenance/)
  assert.match(source, /runs once per day during the configured sleep window/)
  assert.match(source, /moves slower maintenance out of chat turns/)
  assert.match(source, /memory: extracts experiences and compiles durable knowledge/)
  assert.match(source, /kb_cleanup: removes old empty sessions/)
  assert.match(source, /Run Reflection Now bypasses the sleep-window gate/)
  assert.match(source, /Pulse sends a signal after repeated failures/)
  assert.match(source, /class="card r-intro-card"/)
})

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
