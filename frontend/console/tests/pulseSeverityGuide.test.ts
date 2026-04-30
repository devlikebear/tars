import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../src/components/Pulse.svelte', import.meta.url), 'utf8')

test('Pulse explains Min Severity and signal severity mappings inline', () => {
  assert.match(source, /Min Severity guide/)
  assert.match(source, /pulse_\* config fields/)
  assert.match(source, /Notifications below this floor are dropped/)
  assert.match(source, /cron_failures/)
  assert.match(source, /disk_usage/)
  assert.match(source, /stuck_agentruntime_run/)
  assert.match(source, /delivery_failures/)
  assert.match(source, /reflection_failure/)
  assert.match(source, /Last seen by signal/)
  assert.match(source, /lastSignalSeen/)
  assert.match(source, /class="pulse-severity-guide"/)
})

test('Pulse compresses all-clear recent ticks and highlights signal ticks', () => {
  assert.match(source, /recentTickSummary/)
  assert.match(source, /all clear \(no signals\)/)
  assert.match(source, /warningCount/)
  assert.match(source, /errorCount/)
  assert.match(source, /autofixCount/)
  assert.match(source, /Signal ticks/)
  assert.match(source, /pulse-recent-dots/)
  assert.match(source, /pulse-signal-ticks/)
})
