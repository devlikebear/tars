import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../src/components/Reflection.svelte', import.meta.url), 'utf8')
const i18nEnSource = readFileSync(new URL('../src/i18n/en.ts', import.meta.url), 'utf8')

test('Reflection page introduces nightly maintenance before status readouts', () => {
  assert.match(i18nEnSource, /introTitle: 'Reflection - Nightly Maintenance'/)
  assert.match(source, /class="card r-intro-card"/)
})

test('Reflection empty state previews the Run Now output shape', () => {
  assert.match(i18nEnSource, /expectedOutput: 'Expected output'/)
  assert.match(i18nEnSource, /experiences extracted/)
  assert.match(source, /class="r-run-preview"/)
})

test('Reflection run result summarizes totals and job details', () => {
  assert.match(source, /Run totals/)
  assert.match(source, /lastRunBeforeManualRun/)
  assert.match(source, /jobDetailLine\(job\)/)
  assert.match(source, /firstDetailNumber\(job, \['kb_entries_compiled', 'knowledge_entries_compiled', 'entries_compiled'\]\)/)
  assert.match(source, /job.err/)
})
